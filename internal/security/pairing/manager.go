package pairing

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/pkg/utils"
)

type Manager struct {
	mu sync.Mutex

	chanId string

	challenges map[string]Challenge
	windows    map[string][]time.Time
}

func newManager(chanId string) *Manager {
	return &Manager{
		chanId:     strings.TrimSpace(chanId),
		challenges: make(map[string]Challenge, 16),
		windows:    make(map[string][]time.Time, 16),
	}
}

func (m *Manager) EvaluateUnknownUser(principalKey string, welcomeTemplate string) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	principalKey = strings.TrimSpace(principalKey)
	if principalKey == "" {
		return Decision{}, errors.New("principalKey cannot be empty")
	}

	security := m.loadPairingSecurityLocked()

	decision := Decision{Policy: security.Policy}
	now := time.Now()
	m.compactStateLocked(now, security.WelcomeWindow)

	switch security.Policy {
	case securityPolicySilent:
		decision.Respond = false
		return decision, nil
	case securityPolicyCustom, securityPolicyWelcome:
	default:
		return decision, fmt.Errorf("invalid security policy: %s", security.Policy)
	}

	challenge, ok := m.challenges[principalKey]
	if !ok || !now.Before(challenge.ExpiresAt) {
		code := utils.RandDigits(6)
		challenge = Challenge{
			ReqID:     uuid.NewString(),
			Code:      code,
			CreatedAt: now,
			ExpiresAt: now.Add(defaultPairingCodeTTL),
		}
		m.challenges[principalKey] = challenge
	}
	decision.Challenge = challenge

	window := time.Duration(security.WelcomeWindow) * time.Second
	if window <= 0 {
		window = defaultPairingCodeTTL
	}
	points := m.windows[principalKey]
	filtered := points[:0]
	for _, one := range points {
		if now.Sub(one) < window {
			filtered = append(filtered, one)
		}
	}
	m.windows[principalKey] = filtered
	if len(filtered) >= security.MaxResp {
		decision.Respond = false
		return decision, nil
	}
	m.windows[principalKey] = append(filtered, now)

	template := welcomeTemplate
	if security.Policy == securityPolicyCustom {
		template = security.CustomText
	}

	reqID := strings.TrimSpace(challenge.ReqID)
	template = strings.TrimSpace(template)
	if reqID == "" {
		decision.Respond = true
		decision.Message = template
		return decision, nil
	}
	if template == "" {
		template = defaultPairingWelcomeTemplate
	}
	if strings.Contains(template, "%s") {
		decision.Respond = true
		decision.Message = strings.TrimSpace(fmt.Sprintf(template, reqID))
		return decision, nil
	}
	msg := strings.ReplaceAll(template, "{reqId}", reqID)
	if msg == template {
		msg = msg + " <reqId:" + reqID + ">"
	}
	decision.Respond = true
	decision.Message = strings.TrimSpace(msg)
	return decision, nil
}

func (m *Manager) loadPairingSecurityLocked() config.ChannelSecurityConfig {
	silent := config.ChannelSecurityConfig{
		Policy:        securityPolicySilent,
		WelcomeWindow: defaultPairingWelcomeWindowSec,
		MaxResp:       defaultPairingMaxResp,
	}

	if strings.TrimSpace(m.chanId) == "" {
		return silent
	}

	cfg, err := config.Get()
	if err != nil {
		return silent
	}

	_, channelCfg, err := findPairingChannel(cfg, m.chanId)
	if err != nil {
		return silent
	}

	return channelCfg.Security
}

func (m *Manager) VerifyCode(principalKey string, code string) (Challenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	principalKey = strings.TrimSpace(principalKey)
	code = strings.TrimSpace(code)
	if principalKey == "" {
		return Challenge{}, errors.New("principalKey cannot be empty")
	}
	if code == "" {
		return Challenge{}, errors.New("pairing code cannot be empty")
	}

	challenge, ok := m.challenges[principalKey]
	now := time.Now()
	if ok && !now.Before(challenge.ExpiresAt) {
		delete(m.challenges, principalKey)
		delete(m.windows, principalKey)
		m.compactStateLocked(now, defaultPairingWelcomeWindowSec)
		return Challenge{}, errors.New("pairing challenge expired")
	}

	m.compactStateLocked(now, defaultPairingWelcomeWindowSec)
	if !ok {
		return Challenge{}, errors.New("pairing challenge not found")
	}
	if subtle.ConstantTimeCompare([]byte(challenge.Code), []byte(code)) != 1 {
		return Challenge{}, errors.New("invalid pairing code")
	}

	delete(m.challenges, principalKey)
	delete(m.windows, principalKey)
	return challenge, nil
}

func (m *Manager) GetActiveChallenge(principalKey string) (Challenge, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	principalKey = strings.TrimSpace(principalKey)
	if principalKey == "" {
		return Challenge{}, false
	}

	m.compactStateLocked(time.Now(), defaultPairingWelcomeWindowSec)
	challenge, ok := m.challenges[principalKey]
	if !ok {
		return Challenge{}, false
	}
	return challenge, true
}

func (m *Manager) IsAllowed(chatKey string, userID string) (bool, error) {
	chatKey = strings.TrimSpace(chatKey)
	userID = strings.TrimSpace(userID)
	if chatKey == "" {
		return false, errors.New("chatKey cannot be empty")
	}
	if userID == "" {
		return false, errors.New("userID cannot be empty")
	}
	if strings.TrimSpace(m.chanId) == "" {
		return false, errors.New("pairing manager channel identity is not set")
	}

	cfg, err := config.Get()
	if err != nil {
		return false, err
	}
	_, channelCfg, err := findPairingChannel(cfg, m.chanId)
	if err != nil {
		return false, err
	}
	return isAuthorizedByPairingACL(channelCfg.ACL, chatKey, userID), nil
}

func (m *Manager) GrantACL(chatKey string, userID string) (bool, error) {
	chatKey = strings.TrimSpace(chatKey)
	userID = strings.TrimSpace(userID)
	if chatKey == "" {
		return false, errors.New("chatKey cannot be empty")
	}
	if userID == "" {
		return false, errors.New("userID cannot be empty")
	}
	if strings.TrimSpace(m.chanId) == "" {
		return false, errors.New("pairing manager channel identity is not set")
	}

	for attempt := 0; attempt < maxPairingPersistCASRetries; attempt++ {
		cfg, err := config.Get()
		if err != nil {
			return false, err
		}
		expectedHash, err := config.Hash()
		if err != nil {
			return false, err
		}
		channelKey, channelCfg, err := findPairingChannel(cfg, m.chanId)
		if err != nil {
			return false, err
		}

		nextACL, changed, err := upsertPairingChatUser(channelCfg.ACL, chatKey, userID)
		if err != nil {
			return false, err
		}
		if !changed {
			return false, nil
		}
		channelCfg.ACL = nextACL
		cfg.Channels[channelKey] = channelCfg

		if err := config.ApplyWithCAS("config", cfg, expectedHash); err != nil {
			if errors.Is(err, config.ErrConfigConflict) {
				continue
			}
			return false, fmt.Errorf("apply config update: %w", err)
		}
		if err := config.Save(); err != nil {
			return false, fmt.Errorf("save config update: %w", err)
		}
		return true, nil
	}

	return false, fmt.Errorf("persist pairing allowlist conflict after %d retries", maxPairingPersistCASRetries)
}

func (m *Manager) compactStateLocked(now time.Time, welcomeWindowSec int) {
	for principalKey, challenge := range m.challenges {
		if !now.Before(challenge.ExpiresAt) {
			delete(m.challenges, principalKey)
		}
	}

	window := time.Duration(welcomeWindowSec) * time.Second
	if window <= 0 {
		window = defaultPairingCodeTTL
	}
	for principalKey, points := range m.windows {
		filtered := points[:0]
		for _, one := range points {
			if now.Sub(one) < window {
				filtered = append(filtered, one)
			}
		}
		if len(filtered) == 0 {
			delete(m.windows, principalKey)
			continue
		}
		m.windows[principalKey] = filtered
	}
}

func defaultPairingCodeFn() (string, error) {
	b := make([]byte, defaultPairingCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	if len(code) > defaultPairingCodeLen {
		code = code[:defaultPairingCodeLen]
	}
	return code, nil
}
