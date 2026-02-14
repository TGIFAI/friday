package pairing

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bytedance/gg/gslice"

	"github.com/tgifai/friday/internal/config"
)

func isAuthorizedByPairingACL(acl map[string]config.ChannelACLConfig, chatID string, userID string) bool {
	chatID = strings.TrimSpace(chatID)
	userID = strings.TrimSpace(userID)
	if chatID == "" || userID == "" {
		return false
	}

	one, ok := acl[chatID]
	if !ok {
		return false
	}
	if gslice.Contains(one.Block, userID) {
		return false
	}
	if len(one.Allow) == 0 {
		return true
	}
	return gslice.Contains(one.Allow, userID)
}

func upsertPairingChatUser(chats map[string]config.ChannelACLConfig, chatID string, userID string) (map[string]config.ChannelACLConfig, bool, error) {
	chatID = strings.TrimSpace(chatID)
	userID = strings.TrimSpace(userID)
	switch {
	case chatID == "":
		return nil, false, errors.New("chatID cannot be empty")
	case userID == "":
		return nil, false, errors.New("userID cannot be empty")
	case !strings.HasPrefix(chatID, "group:") && !strings.HasPrefix(chatID, "user:"):
		return nil, false, fmt.Errorf("chatID must start with group: or user:, got %s", chatID)
	}
	if chats == nil {
		chats = make(map[string]config.ChannelACLConfig, 1)
	}

	entry, ok := chats[chatID]
	if !ok {
		entry = config.ChannelACLConfig{Allow: []string{userID}}
		chats[chatID] = entry
		return chats, true, nil
	}

	changed := false
	if !gslice.Contains(entry.Allow, userID) {
		entry.Allow = append(entry.Allow, userID)
		changed = true
	}

	if len(entry.Block) > 0 {
		filtered := make([]string, 0, len(entry.Block))
		for _, one := range entry.Block {
			if strings.TrimSpace(one) == userID {
				changed = true
				continue
			}
			filtered = append(filtered, one)
		}
		entry.Block = filtered
	}

	normalizedAllow, allowChanged := normalizePairingIDList(entry.Allow)
	normalizedBlock, blockChanged := normalizePairingIDList(entry.Block)
	entry.Allow = normalizedAllow
	entry.Block = normalizedBlock
	if allowChanged || blockChanged {
		changed = true
	}

	chats[chatID] = entry
	return chats, changed, nil
}

func normalizePairingIDList(in []string) ([]string, bool) {
	if len(in) == 0 {
		return nil, false
	}

	changed := false
	uniq := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		one := strings.TrimSpace(raw)
		if one != raw {
			changed = true
		}
		if one == "" {
			changed = true
			continue
		}
		if _, ok := uniq[one]; ok {
			changed = true
			continue
		}
		uniq[one] = struct{}{}
		out = append(out, one)
	}

	sort.Strings(out)
	if len(out) == 0 {
		return nil, true
	}
	if len(out) != len(in) {
		changed = true
	}
	if !changed {
		for i := range out {
			if strings.TrimSpace(in[i]) != out[i] {
				changed = true
				break
			}
		}
	}
	return out, changed
}

func findPairingChannel(cfg *config.Config, channelID string) (string, config.ChannelConfig, error) {
	if cfg == nil {
		return "", config.ChannelConfig{}, errors.New("config cannot be nil")
	}

	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return "", config.ChannelConfig{}, errors.New("chanId cannot be empty")
	}

	chCfg, ok := cfg.Channels[channelID]
	if !ok {
		return "", config.ChannelConfig{}, fmt.Errorf("channel not found: %s", channelID)
	}
	if strings.TrimSpace(chCfg.ID) == "" {
		chCfg.ID = channelID
	}
	return channelID, chCfg, nil
}
