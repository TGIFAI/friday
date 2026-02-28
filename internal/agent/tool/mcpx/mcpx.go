package mcpx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tgifai/friday/internal/pkg/logs"
)

// MCPManager proxies tool calls to external MCP servers.
type MCPManager struct {
	manager *Manager
}

// NewMCPTool creates a new MCP tool instance.
func NewMCPTool() *MCPManager {
	return &MCPManager{
		manager: NewManager(),
	}
}

func (t *MCPManager) Name() string { return "mcp" }

func (t *MCPManager) Description() string {
	return "Connect to external MCP (Model Context Protocol) servers and call their tools. Supports stdio and HTTP transports. Use list_servers/list_tools to discover available tools, then call_tool to invoke them."
}

func (t *MCPManager) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     `Action: "connect" (add server), "disconnect" (remove server), "list_servers" (show connected), "list_tools" (show available tools), "call_tool" (invoke a tool)`,
				Required: true,
				Enum:     []string{"connect", "disconnect", "list_servers", "list_tools", "call_tool"},
			},
			"name": {
				Type: schema.String,
				Desc: `Server name. Required for "connect".`,
			},
			"transport": {
				Type: schema.String,
				Desc: `Transport type: "stdio" or "http". Required for "connect".`,
				Enum: []string{"stdio", "http"},
			},
			"command": {
				Type: schema.String,
				Desc: `Command to run for stdio transport. Required for "connect" with stdio.`,
			},
			"args": {
				Type: schema.Array,
				Desc: `Command arguments for stdio transport. Optional for "connect" with stdio.`,
			},
			"env": {
				Type: schema.Object,
				Desc: `Environment variables for stdio transport. Optional for "connect" with stdio.`,
			},
			"url": {
				Type: schema.String,
				Desc: `Server URL for HTTP transport. Required for "connect" with http.`,
			},
			"server": {
				Type: schema.String,
				Desc: `Server name. Required for "disconnect", "list_tools", "call_tool". Optional for "list_tools" to filter.`,
			},
			"tool": {
				Type: schema.String,
				Desc: `Tool name. Required for "call_tool".`,
			},
			"arguments": {
				Type: schema.Object,
				Desc: `Tool arguments as a JSON object. Optional for "call_tool".`,
			},
		}),
	}
}

func (t *MCPManager) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(gconv.To[string](args["action"])))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "connect":
		return t.executeConnect(ctx, args)
	case "disconnect":
		return t.executeDisconnect(args)
	case "list_servers":
		return t.executeListServers()
	case "list_tools":
		return t.executeListTools(ctx, args)
	case "call_tool":
		return t.executeCallTool(ctx, args)
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (t *MCPManager) executeConnect(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name := strings.TrimSpace(gconv.To[string](args["name"]))
	if name == "" {
		return nil, fmt.Errorf("name is required for connect action")
	}
	transport := strings.TrimSpace(gconv.To[string](args["transport"]))
	if transport == "" {
		return nil, fmt.Errorf("transport is required for connect action")
	}

	cfg := ServerConfig{Transport: transport}
	switch transport {
	case "stdio":
		cfg.Command = gconv.To[string](args["command"])
		if cfg.Command == "" {
			return nil, fmt.Errorf("command is required for stdio transport")
		}
		if rawArgs, ok := args["args"]; ok && rawArgs != nil {
			if arr, ok := rawArgs.([]interface{}); ok {
				for _, a := range arr {
					cfg.Args = append(cfg.Args, gconv.To[string](a))
				}
			}
		}
		if rawEnv, ok := args["env"]; ok && rawEnv != nil {
			if envMap, ok := rawEnv.(map[string]interface{}); ok {
				cfg.Env = make(map[string]string, len(envMap))
				for k, v := range envMap {
					cfg.Env[k] = gconv.To[string](v)
				}
			}
		}
	case "http":
		cfg.URL = gconv.To[string](args["url"])
		if cfg.URL == "" {
			return nil, fmt.Errorf("url is required for http transport")
		}
	default:
		return nil, fmt.Errorf("unsupported transport: %s", transport)
	}

	logs.CtxInfo(ctx, "[tool:mcp] connecting to server %s (%s)", name, transport)
	if err := t.manager.Connect(ctx, name, cfg); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status": "connected",
		"server": name,
	}, nil
}

func (t *MCPManager) executeDisconnect(args map[string]interface{}) (interface{}, error) {
	server := strings.TrimSpace(gconv.To[string](args["server"]))
	if server == "" {
		return nil, fmt.Errorf("server is required for disconnect action")
	}

	if err := t.manager.Disconnect(server); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status": "disconnected",
		"server": server,
	}, nil
}

func (t *MCPManager) executeListServers() (interface{}, error) {
	servers := t.manager.List()
	list := make([]map[string]interface{}, 0, len(servers))
	for _, srv := range servers {
		entry := map[string]interface{}{
			"name":      srv.Name,
			"transport": srv.Config.Transport,
			"status":    string(srv.Status),
		}
		if srv.Error != "" {
			entry["error"] = srv.Error
		}
		list = append(list, entry)
	}
	return map[string]interface{}{"servers": list}, nil
}

func (t *MCPManager) executeListTools(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	serverName := strings.TrimSpace(gconv.To[string](args["server"]))

	var servers []*Server
	if serverName != "" {
		srv, ok := t.manager.Get(serverName)
		if !ok {
			return nil, fmt.Errorf("server %q not found", serverName)
		}
		servers = []*Server{srv}
	} else {
		servers = t.manager.List()
	}

	result := make([]map[string]interface{}, 0)
	for _, srv := range servers {
		tools, err := srv.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		for _, tool := range tools {
			entry := map[string]interface{}{
				"server": srv.Name,
				"name":   tool.Name,
			}
			if tool.Description != "" {
				entry["description"] = tool.Description
			}
			result = append(result, entry)
		}
	}
	return map[string]interface{}{"tools": result}, nil
}

func (t *MCPManager) executeCallTool(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	serverName := strings.TrimSpace(gconv.To[string](args["server"]))
	if serverName == "" {
		return nil, fmt.Errorf("server is required for call_tool action")
	}
	toolName := strings.TrimSpace(gconv.To[string](args["tool"]))
	if toolName == "" {
		return nil, fmt.Errorf("tool is required for call_tool action")
	}

	srv, ok := t.manager.Get(serverName)
	if !ok {
		return nil, fmt.Errorf("server %q not found", serverName)
	}

	var toolArgs map[string]any
	if rawArgs, ok := args["arguments"]; ok && rawArgs != nil {
		if m, ok := rawArgs.(map[string]interface{}); ok {
			toolArgs = m
		}
	}

	logs.CtxInfo(ctx, "[tool:mcp] calling %s/%s", serverName, toolName)
	res, err := srv.CallTool(ctx, toolName, toolArgs)
	if err != nil {
		return nil, fmt.Errorf("call tool %s/%s: %w", serverName, toolName, err)
	}

	// Extract text content from the result.
	var contents []map[string]interface{}
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			contents = append(contents, map[string]interface{}{
				"type": "text",
				"text": v.Text,
			})
		case *mcp.ImageContent:
			contents = append(contents, map[string]interface{}{
				"type":      "image",
				"mime_type": v.MIMEType,
				"data":      v.Data,
			})
		default:
			contents = append(contents, map[string]interface{}{
				"type": "unknown",
				"data": fmt.Sprintf("%v", c),
			})
		}
	}

	result := map[string]interface{}{
		"server":  serverName,
		"tool":    toolName,
		"content": contents,
	}
	if res.IsError {
		result["is_error"] = true
	}
	return result, nil
}

// loadConfig loads MCP server configs from workspace and auto-connects.
func (t *MCPManager) LoadConfig(ctx context.Context, workspace string) error {
	cfg, err := loadConfig(workspace)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil
	}

	for name, sc := range cfg.MCPServers {
		if err := t.manager.Connect(ctx, name, sc); err != nil {
			logs.CtxWarn(ctx, "[tool:mcp] failed to connect to %s: %v", name, err)
		} else {
			logs.CtxInfo(ctx, "[tool:mcp] connected to %s (%s)", name, sc.Transport)
		}
	}
	return nil
}

// Close disconnects all MCP servers.
func (t *MCPManager) Close() error {
	return t.manager.Close()
}
