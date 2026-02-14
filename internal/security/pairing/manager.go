package pairing

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gg/gslice"
	"github.com/google/uuid"

	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/utils"
)

const (
	securityPolicyWelcome = consts.SecurityPolicyWelcome
	securityPolicySilent  = consts.SecurityPolicySilent
	securityPolicyCustom  = consts.SecurityPolicyCustom

	defaultPairingWelcomeWindowSec = 300
	defaultPairingMaxResp          = 3
	maxPairingPersistCASRetries    = 3
	defaultPairingCodeTTL          = 5 * time.Minute
	defaultPairingCodeLen          = 8

	defaultPairingWelcomeTemplate = "Welcome to Friday. Please enter your pairing code \n\n---\n<reqId:%s>"
)

type Challenge struct {
	ReqID     string
	Code      string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Decision struct {
	Respond   bool
	Message   string
	Policy    consts.SecurityPolicy
	Challenge Challenge
}

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

	security := m.loadSecurityConfigLocked()

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
		challenge = Challenge{
			ReqID:     uuid.NewString(),
			Code:      utils.RandStr(defaultPairingCodeLen),
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
	if chatKey == "" || userID == "" {
		return false, errors.New("chatKey and userID cannot be empty")
	}

	cfg, err := config.Get()
	if err != nil {
		return false, err
	}
	chCfg, ok := cfg.Channels[m.chanId]
	if !ok {
		return false, fmt.Errorf("channel not found: %s", m.chanId)
	}

	entry, exists := chCfg.ACL[chatKey]
	if !exists {
		return false, nil
	}
	if gslice.Contains(entry.Block, userID) {
		return false, nil
	}
	if len(entry.Allow) == 0 {
		return true, nil
	}
	return gslice.Contains(entry.Allow, userID), nil
}

func (m *Manager) GrantACL(chatKey string, userID string) (bool, error) {
	chatKey = strings.TrimSpace(chatKey)
	userID = strings.TrimSpace(userID)
	if chatKey == "" || userID == "" {
		return false, errors.New("chatKey and userID cannot be empty")
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
		chCfg, ok := cfg.Channels[m.chanId]
		if !ok {
			return false, fmt.Errorf("channel not found: %s", m.chanId)
		}

		if chCfg.ACL == nil {
			return false, nil
		}
		entry, entryOK := chCfg.ACL[chatKey]
		if !entryOK {
			chCfg.ACL[chatKey] = config.ChannelACLConfig{Allow: []string{userID}}
		} else {
			changed := false
			if !gslice.Contains(entry.Allow, userID) {
				entry.Allow = append(entry.Allow, userID)
				changed = true
			}
			if gslice.Contains(entry.Block, userID) {
				filtered := make([]string, 0, len(entry.Block))
				for _, id := range entry.Block {
					if id != userID {
						filtered = append(filtered, id)
					}
				}
				entry.Block = filtered
				changed = true
			}
			if !changed {
				return false, nil
			}
			chCfg.ACL[chatKey] = entry
		}
		cfg.Channels[m.chanId] = chCfg

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

func (m *Manager) loadSecurityConfigLocked() config.ChannelSecurityConfig {
	silent := config.ChannelSecurityConfig{
		Policy:        securityPolicySilent,
		WelcomeWindow: defaultPairingWelcomeWindowSec,
		MaxResp:       defaultPairingMaxResp,
	}

	if m.chanId == "" {
		return silent
	}
	cfg, err := config.Get()
	if err != nil {
		return silent
	}
	chCfg, ok := cfg.Channels[m.chanId]
	if !ok {
		return silent
	}
	return chCfg.Security
}

func (m *Manager) compactStateLocked(now time.Time, welcomeWindowSec int) {
	for key, challenge := range m.challenges {
		if !now.Before(challenge.ExpiresAt) {
			delete(m.challenges, key)
		}
	}

	window := time.Duration(welcomeWindowSec) * time.Second
	if window <= 0 {
		window = defaultPairingCodeTTL
	}
	for key, points := range m.windows {
		filtered := points[:0]
		for _, one := range points {
			if now.Sub(one) < window {
				filtered = append(filtered, one)
			}
		}
		if len(filtered) == 0 {
			delete(m.windows, key)
			continue
		}
		m.windows[key] = filtered
	}
}
