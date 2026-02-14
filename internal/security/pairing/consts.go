package pairing

import (
	"time"

	"github.com/tgifai/friday/internal/consts"
)

const (
	securityPolicyWelcome = consts.SecurityPolicyWelcome
	securityPolicySilent  = consts.SecurityPolicySilent
	securityPolicyCustom  = consts.SecurityPolicyCustom

	defaultPairingWelcomeWindowSec = 300
	defaultPairingMaxResp          = 3
	maxPairingPersistCASRetries    = 3
	defaultPairingCodeTTL          = 5 * time.Minute
	defaultPairingCodeLen          = 10

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
