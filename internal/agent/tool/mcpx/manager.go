package mcpx

import (
	"context"
	"fmt"
	"sync"
)

const maxServers = 8

// Manager manages connections to multiple MCP servers.
type Manager struct {
	servers map[string]*Server
	mu      sync.RWMutex
}

// NewManager creates a new connection pool manager.
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]*Server, maxServers),
	}
}

// Connect creates and connects to an MCP server.
func (m *Manager) Connect(ctx context.Context, name string, cfg ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[name]; exists {
		return fmt.Errorf("server %q is already connected", name)
	}
	if len(m.servers) >= maxServers {
		return fmt.Errorf("maximum number of MCP servers (%d) reached", maxServers)
	}

	srv := NewServer(name, cfg)
	if err := srv.Connect(ctx); err != nil {
		return err
	}

	m.servers[name] = srv
	return nil
}

// Disconnect closes and removes an MCP server connection.
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, ok := m.servers[name]
	if !ok {
		return fmt.Errorf("server %q not found", name)
	}

	err := srv.Close()
	delete(m.servers, name)
	return err
}

// Get returns a connected server by name.
func (m *Manager) Get(name string) (*Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	srv, ok := m.servers[name]
	return srv, ok
}

// List returns all connected servers.
func (m *Manager) List() []*Server {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Server, 0, len(m.servers))
	for _, srv := range m.servers {
		list = append(list, srv)
	}
	return list
}

// Close disconnects all servers.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, srv := range m.servers {
		if err := srv.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.servers, name)
	}
	return firstErr
}
