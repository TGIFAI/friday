package consts

import _ "embed"

var (
	//go:embed tpl/AGENTS.md
	WorkspaceAgentsTemplate string

	//go:embed tpl/SOUL.md
	WorkspaceSoulTemplate string

	//go:embed tpl/USER.md
	WorkspaceUserTemplate string

	//go:embed tpl/TOOLS.md
	WorkspaceToolsTemplate string

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

const WorkspaceMemoryFile = "memory/MEMORY.md"

var WorkspaceMarkdownTemplates = map[string]string{
	"AGENTS.md":        WorkspaceAgentsTemplate,
	"SOUL.md":          WorkspaceSoulTemplate,
	"USER.md":          WorkspaceUserTemplate,
	"TOOLS.md":         WorkspaceToolsTemplate,
	"IDENTITY.md":      WorkspaceIdentityTemplate,
	"HEARTBEAT.md":     WorkspaceHeartbeatTemplate,
	"SECURITY.md":      WorkspaceSecurityTemplate,
	"memory/MEMORY.md": WorkspaceMemoryTemplate,
}
