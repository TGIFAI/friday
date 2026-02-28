package consts

import (
	_ "embed"
	"path/filepath"
	"time"
)

var (
	//go:embed tpl/AGENTS.md
	WorkspaceAgentsTemplate string

	//go:embed tpl/SOUL.md
	WorkspaceSoulTemplate string

	//go:embed tpl/USER.md
	WorkspaceUserTemplate string

	//go:embed tpl/TOOLS.md
	WorkspaceToolsTemplate string

	//go:embed tpl/CUSTOM_TOOLS.md
	WorkspaceCustomToolsTemplate string

	//go:embed tpl/IDENTITY.md
	WorkspaceIdentityTemplate string

	//go:embed tpl/HEARTBEAT.md
	WorkspaceHeartbeatTemplate string

	//go:embed tpl/SECURITY.md
	WorkspaceSecurityTemplate string

	//go:embed tpl/MEMORY.md
	WorkspaceMemoryTemplate string
)

var WorkspaceMarkdownFiles = []string{
	"AGENTS.md",
	"SOUL.md",
	"USER.md",
	"TOOLS.md",
	"IDENTITY.md",
	"HEARTBEAT.md",
	"SECURITY.md",
}

// WorkspaceManagedFiles lists files that are always overwritten on startup
// to stay in sync with the binary. These must not contain user-editable content.
var WorkspaceManagedFiles = map[string]bool{
	"SECURITY.md": true,
}

const WorkspaceMemoryFile = "memory/MEMORY.md"

const WorkspaceMemoryDailyDir = "memory/daily"

func DailyMemoryFile(date time.Time) string {
	return filepath.Join(WorkspaceMemoryDailyDir, date.Format("2006-01-02")+".md")
}

var WorkspaceMarkdownTemplates = map[string]string{
	"AGENTS.md":        WorkspaceAgentsTemplate,
	"SOUL.md":          WorkspaceSoulTemplate,
	"USER.md":          WorkspaceUserTemplate,
	"TOOLS.md":         WorkspaceCustomToolsTemplate,
	"IDENTITY.md":      WorkspaceIdentityTemplate,
	"HEARTBEAT.md":     WorkspaceHeartbeatTemplate,
	"SECURITY.md":      WorkspaceSecurityTemplate,
	"memory/MEMORY.md": WorkspaceMemoryTemplate,
}
