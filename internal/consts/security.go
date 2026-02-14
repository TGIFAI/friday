package consts

type SecurityPolicy string

const (
	SecurityPolicyWelcome SecurityPolicy = "welcome"
	SecurityPolicySilent  SecurityPolicy = "silent"
	SecurityPolicyCustom  SecurityPolicy = "custom"
)
