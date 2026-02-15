package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	friConsts "github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/security/pairing"
)

const pairCommandPrefix = "/pair"

// SecurityGuard performs channel-agnostic security checks (ACL + pairing).
// It is called by the gateway before command routing or agent dispatch.
type SecurityGuard struct{}

// Check evaluates whether a message should be allowed through.
// It returns (allowed, reply). If reply is non-empty the gateway should send
// it back to the user regardless of whether allowed is true.
func (g *SecurityGuard) Check(ctx context.Context, msg *channel.Message, chCfg config.ChannelConfig) (bool, string) {
	pairingEnabled := chCfg.Security.Policy != "" && chCfg.Security.Policy != friConsts.SecurityPolicySilent

	if pairingEnabled {
		return g.checkPairing(ctx, msg, chCfg)
	}
	return g.checkACL(msg, chCfg)
}

// checkACL applies simple allow/block list rules. When no ACL is configured
// the message is allowed through.
func (g *SecurityGuard) checkACL(msg *channel.Message, chCfg config.ChannelConfig) (bool, string) {
	if len(chCfg.ACL) == 0 {
		return true, ""
	}

	chatKey := g.buildChatKey(msg)
	entry, exists := chCfg.ACL[chatKey]
	if !exists {
		// No rule for this chat — check user-level key.
		userKey := "user:" + msg.ChatID
		entry, exists = chCfg.ACL[userKey]
		if !exists {
			return true, "" // no rules → allow
		}
	}

	if contains(entry.Block, msg.UserID) {
		return false, ""
	}
	if len(entry.Allow) == 0 {
		return true, ""
	}
	if contains(entry.Allow, msg.UserID) {
		return true, ""
	}
	return false, "Sorry, you are not authorized to use this bot."
}

// checkPairing handles the pairing flow for channels with a security policy.
func (g *SecurityGuard) checkPairing(ctx context.Context, msg *channel.Message, chCfg config.ChannelConfig) (bool, string) {
	mgr := pairing.Get(pairing.GetKey(string(msg.ChannelType), msg.ChannelID))

	chatKey := g.buildChatKey(msg)
	if chatKey == "" {
		return false, ""
	}

	// Already allowed?
	allowed, err := mgr.IsAllowed(chatKey, msg.UserID)
	if err != nil {
		logs.CtxError(ctx, "[security] pairing allowlist check failed: %v", err)
		return false, ""
	}
	if allowed {
		return true, ""
	}

	// Is this a /pair command?
	if code, ok := parsePairCommand(msg.Content); ok {
		return g.handlePairCommand(ctx, mgr, msg, chatKey, code)
	}

	// Unknown user — issue challenge.
	principal := g.buildPrincipal(msg, chatKey)
	decision, err := mgr.EvaluateUnknownUser(principal, msg.ChatID, msg.UserID, "")
	if err != nil {
		logs.CtxError(ctx, "[security] pairing evaluate failed: %v", err)
		return false, ""
	}

	logs.CtxInfo(ctx,
		"[security] pairing_user_reached channel_id=%s user_id=%s chat_id=%s chat_key=%s req_id=%s code=%s expire=%s",
		msg.ChannelID, msg.UserID, msg.ChatID, chatKey,
		decision.Challenge.ReqID, decision.Challenge.Code,
		decision.Challenge.ExpiresAt.Format(time.RFC3339),
	)

	if decision.Respond && strings.TrimSpace(decision.Message) != "" {
		return false, decision.Message
	}
	return false, ""
}

func (g *SecurityGuard) handlePairCommand(
	ctx context.Context,
	mgr *pairing.Manager,
	msg *channel.Message,
	chatKey, code string,
) (bool, string) {
	principal := g.buildPrincipal(msg, chatKey)
	challenge, err := mgr.VerifyCode(principal, code)
	if err != nil {
		logs.CtxInfo(ctx,
			"[security] pairing_result channel_id=%s user_id=%s chat_key=%s success=false reason=%v",
			msg.ChannelID, msg.UserID, chatKey, err,
		)
		return false, "Invalid or expired pairing code."
	}

	changed, grantErr := mgr.GrantACL(chatKey, msg.UserID)
	if grantErr != nil {
		logs.CtxError(ctx, "[security] grant acl failed: %v", grantErr)
		return false, "Pairing failed due to an internal error."
	}

	reason := "ok"
	if !changed {
		reason = "already_granted"
	}
	logs.CtxInfo(ctx,
		"[security] pairing_result channel_id=%s user_id=%s chat_key=%s req_id=%s success=true reason=%s",
		msg.ChannelID, msg.UserID, chatKey, challenge.ReqID, reason,
	)
	return false, "Pairing successful. You can now use this bot."
}

func (g *SecurityGuard) buildChatKey(msg *channel.Message) string {
	chatType, _ := msg.Metadata["chat_type"]
	if strings.EqualFold(chatType, "private") || chatType == "" {
		return "user:" + msg.ChatID
	}
	return "group:" + msg.ChatID
}

func (g *SecurityGuard) buildPrincipal(msg *channel.Message, chatKey string) string {
	return fmt.Sprintf("%s:%s:%s:%s", msg.ChannelType, msg.ChannelID, chatKey, msg.UserID)
}

func parsePairCommand(content string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(content))
	if len(fields) < 2 {
		return "", false
	}
	cmd := strings.ToLower(fields[0])
	if strings.HasPrefix(cmd, pairCommandPrefix+"@") {
		cmd = pairCommandPrefix
	}
	if cmd != pairCommandPrefix {
		return "", false
	}
	code := strings.TrimSpace(fields[1])
	if code == "" {
		return "", false
	}
	return code, true
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

// groupAllowed checks if a group chat is allowed based on channel config.
// Used for non-private chats when ACL has no matching group entry.
func groupAllowed(msg *channel.Message, chCfg config.ChannelConfig) bool {
	chatType, _ := msg.Metadata["chat_type"]
	if chatType == "private" || chatType == "" {
		return true // not a group
	}

	// Check allowed_groups from channel config (Telegram-specific config
	// stores int64 IDs, but the generic check works on ACL).
	groupKey := "group:" + msg.ChatID
	if _, exists := chCfg.ACL[groupKey]; exists {
		return true // will be checked by ACL
	}

	// If there are user-level ACL rules but no group rules, allow the group
	// (the user-level check handles authorization).
	return true
}

// chatIDToInt64 is a helper for platforms that use numeric chat IDs.
func chatIDToInt64(chatID string) (int64, error) {
	return strconv.ParseInt(chatID, 10, 64)
}
