package mcpx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerStatus represents the connection state of an MCP server.
type ServerStatus string

const (
	StatusConnecting ServerStatus = "connecting"
	StatusConnected  ServerStatus = "connected"
	StatusError      ServerStatus = "error"
	StatusClosed     ServerStatus = "closed"
)

// Server wraps an MCP client connection to a single server.
type Server struct {
	Name   string
	Config ServerConfig
	Status ServerStatus
	Error  string // last error message if status == "error"

	client  *mcp.Client
	session *mcp.ClientSession
	mu      sync.RWMutex
}

// NewServer creates a Server instance (not yet connected).
func NewServer(name string, cfg ServerConfig) *Server {
	return &Server{
		Name:   name,
		Config: cfg,
		Status: StatusClosed,
	}
}

// Connect establishes the connection to the MCP server.
func (s *Server) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Status = StatusConnecting
	s.Error = ""

	s.client = mcp.NewClient(&mcp.Implementation{
		Name:    "friday",
		Version: "v1.0.0",
	}, nil)

	transport, err := s.buildTransport()
	if err != nil {
		s.Status = StatusError
		s.Error = err.Error()
		return fmt.Errorf("build transport for %s: %w", s.Name, err)
	}

	session, err := s.client.Connect(ctx, transport, nil)
	if err != nil {
		s.Status = StatusError
		s.Error = err.Error()
		return fmt.Errorf("connect to %s: %w", s.Name, err)
	}

	s.session = session
	s.Status = StatusConnected
	return nil
}

func (s *Server) buildTransport() (mcp.Transport, error) {
	switch s.Config.Transport {
	case "stdio":
		cmd := exec.Command(s.Config.Command, s.Config.Args...)
		cmd.Stderr = os.Stderr
		if len(s.Config.Env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range s.Config.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case "http":
		return &mcp.StreamableClientTransport{Endpoint: s.Config.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported transport: %s", s.Config.Transport)
	}
}

// ListTools returns the tools available on this server.
func (s *Server) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return nil, fmt.Errorf("server %s is not connected", s.Name)
	}

	var tools []mcp.Tool
	for t, err := range s.session.Tools(ctx, nil) {
		if err != nil {
			return tools, fmt.Errorf("list tools from %s: %w", s.Name, err)
		}
		tools = append(tools, *t)
	}
	return tools, nil
}

// CallTool invokes a tool on this server.
func (s *Server) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return nil, fmt.Errorf("server %s is not connected", s.Name)
	}

	return s.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// Close shuts down the connection to the MCP server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Status = StatusClosed
	if s.session != nil {
		err := s.session.Close()
		s.session = nil
		s.client = nil
		return err
	}
	return nil
}
