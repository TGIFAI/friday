package sandbox

import "strings"

type SecurityConfig struct {
	Sandbox SandboxConfig `yaml:"sandbox"`
}

type SandboxConfig struct {
	Enable       bool          `yaml:"enable"`
	Runtime      string        `yaml:"runtime"` // local | go_judge
	ApplyToTools []string      `yaml:"apply_to_tools"`
	GoJudge      GoJudgeConfig `yaml:"go_judge"`
}

type GoJudgeConfig struct {
	Endpoint          string `yaml:"endpoint"`
	AuthToken         string `yaml:"auth_token"`
	RequestTimeoutSec int    `yaml:"request_timeout_sec"`
	WorkdirMount      string `yaml:"workdir_mount"`
	CPULimitMS        int    `yaml:"cpu_limit_ms"`
	WallLimitMS       int    `yaml:"wall_limit_ms"`
	MemoryLimitKB     int    `yaml:"memory_limit_kb"`
	ProcLimit         int    `yaml:"proc_limit"`
	MaxStdoutBytes    int    `yaml:"max_stdout_bytes"`
	MaxStderrBytes    int    `yaml:"max_stderr_bytes"`
}

func (s SandboxConfig) AppliesToTool(toolName string) bool {
	name := strings.ToLower(strings.TrimSpace(toolName))
	for _, one := range s.ApplyToTools {
		if one == name {
			return true
		}
	}
	return false
}
