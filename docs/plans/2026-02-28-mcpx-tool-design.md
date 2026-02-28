# mcpx Tool Design

## Overview

Add an `mcpx` tool to friday's agent tool system that acts as an MCP (Model Context Protocol) client. This enables agents to dynamically connect to external MCP Servers and use their tools, significantly extending agent capabilities without writing new tool code.

## Requirements

- **MCP Client**: friday acts as MCP client, connecting to external MCP servers
- **Transports**: stdio (local subprocess) + Streamable HTTP (remote)
- **Config**: Pre-defined servers loaded from `workspace/.friday/mcp.json` at agent startup + dynamic connect/disconnect via tool calls at runtime
- **Tool Exposure**: Agent calls MCP tools through the mcpx tool proxy (not injected into tool registry)
- **SDK**: Official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk/mcp`)

## Architecture

```
Agent Loop
  │
  ├── action: connect      ► Launch stdio subprocess or open HTTP connection
  ├── action: disconnect   ► Close connection, cleanup resources
  ├── action: list_servers ► List connected servers with status
  ├── action: list_tools   ► List tools from a server (or all servers)
  ├── action: call_tool    ► Proxy tool call to target MCP server
  │
  └── mcpx internals:
       ├── MCPTool (implements tool.Tool interface)
       ├── Manager (connection pool, sync.RWMutex, max 8 servers)
       │    ├── Server{name, client, session, transport, status}
       │    └── Server{...}
       └── Config loader (workspace/.friday/mcp.json)
```

## Configuration

### File: `workspace/.friday/mcp.json`

```json
{
  "mcpServers": {
    "context7": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"],
      "env": {
        "CONTEXT7_API_KEY": "xxx"
      }
    },
    "figma": {
      "transport": "http",
      "url": "http://127.0.0.1:3845/mcp"
    },
    "lark": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@larksuiteoapi/lark-mcp", "mcp", "-a", "<app_id>", "-s", "<secret>"]
    }
  }
}
```

### Loading Behavior

1. On `agent.Init()`, mcpx reads `workspace/.friday/mcp.json` if it exists
2. For each configured server, mcpx attempts to connect in the background
3. Connection failures are logged but do not block agent startup
4. Runtime `connect`/`disconnect` actions manage additional servers dynamically

## Tool Interface

### Actions

| Action | Required Params | Optional Params | Description |
|---|---|---|---|
| `connect` | `name`, `transport` | `command`, `args`, `env` (stdio); `url` (http) | Connect to an MCP server |
| `disconnect` | `server` | | Disconnect from a server |
| `list_servers` | | | List all connected servers |
| `list_tools` | | `server` | List tools (all servers or specific) |
| `call_tool` | `server`, `tool` | `arguments` | Call a tool on a server |

### Usage Examples

```
// Connect to Context7 (stdio)
action: connect
name: context7
transport: stdio
command: npx
args: ["-y", "@upstash/context7-mcp"]

// Connect to Figma (HTTP)
action: connect
name: figma
transport: http
url: http://127.0.0.1:3845/mcp

// List available tools
action: list_tools
server: context7

// Call a tool
action: call_tool
server: context7
tool: resolve-library-id
arguments: {"libraryName": "react", "query": "hooks"}

// Disconnect
action: disconnect
server: context7
```

## File Structure

```
internal/agent/tool/mcpx/
├── mcpx.go         // MCPTool: tool.Tool interface + action dispatch
├── server.go       // Server: MCP client/session/transport wrapper
├── manager.go      // Manager: connection pool with sync.RWMutex
└── config.go       // Config: load/parse workspace/.friday/mcp.json
```

## Key Components

### MCPTool (`mcpx.go`)

- Implements `tool.Tool` interface (Name, Description, ToolInfo, Execute)
- Single tool named `"mcp"` with action-based dispatch (same pattern as `agentx`)
- Holds reference to Manager

### Manager (`manager.go`)

- Thread-safe connection pool (sync.RWMutex)
- Max 8 concurrent server connections
- Methods: Connect, Disconnect, Get, List, Close
- `LoadConfig(workspace)` reads mcp.json and connects configured servers

### Server (`server.go`)

- Wraps `mcp.Client` + `mcp.ClientSession` + transport
- Tracks status (connecting, connected, error, closed)
- `ListTools()` calls `session.ListTools()`
- `CallTool()` calls `session.CallTool()`
- `Close()` cleans up session and transport

### Config (`config.go`)

- Parses `workspace/.friday/mcp.json`
- Validates transport type and required fields
- Returns `[]ServerConfig` for Manager to connect

## Dependencies

- `github.com/modelcontextprotocol/go-sdk/mcp` (Official MCP Go SDK)
  - `mcp.NewClient()` for client creation
  - `mcp.CommandTransport` for stdio
  - Streamable HTTP transport for remote servers
  - `session.ListTools()` for tool discovery
  - `session.CallTool()` for tool invocation

## Security

- Maximum 8 concurrent MCP server connections (consistent with agentx)
- stdio: child process lifecycle management with cleanup on disconnect
- HTTP: connection timeout control
- All MCP connections closed on agent shutdown via `Manager.Close()`
- Environment variables in config support secrets (API keys)

## Integration Point

In `agent.go` `Init()`:

```go
// mcp tools
mcpTool := mcpx.NewMCPTool()
if err := mcpTool.LoadConfig(ag.workspace); err != nil {
    logs.Warn("[agent:%s] failed to load mcp config: %v", ag.id, err)
}
_ = ag.tools.Register(mcpTool)
```

Agent shutdown should call `mcpTool.Close()` to clean up all connections.
