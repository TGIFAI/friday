package consts

import (
	"os"
	"path/filepath"
)

const (
	FridayDirName      = ".friday"
	ConfigFileName     = "config.yaml"
	DefaultWorkspaceID = "default"
	SkillsDirName      = "skills"
	SkillsRepoURL      = "https://github.com/TGIFAI/skills.git"
)

func FridayHomeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, FridayDirName)
}

func DefaultConfigPath() string {
	return filepath.Join(FridayHomeDir(), ConfigFileName)
}

func DefaultWorkspaceDir() string {
	return filepath.Join(FridayHomeDir(), "workspaces", DefaultWorkspaceID)
}

func GlobalSkillsDir() string {
	return filepath.Join(FridayHomeDir(), SkillsDirName)
}
