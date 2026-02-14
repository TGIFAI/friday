package pairing

import (
	"strings"
	"sync"
)

var (
	defaultRegistry = &managerRegistry{managers: make(map[string]*Manager, 8)}

	Get    = defaultRegistry.Get
	Delete = defaultRegistry.Delete
)

type managerRegistry struct {
	mu       sync.RWMutex
	managers map[string]*Manager
}

func (r *managerRegistry) Get(channelKey string) *Manager {
	channelKey = strings.TrimSpace(channelKey)
	if channelKey == "" {
		return newManager("")
	}

	r.mu.RLock()
	manager, ok := r.managers[channelKey]
	r.mu.RUnlock()
	if ok && manager != nil {
		return manager
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	manager, ok = r.managers[channelKey]
	if ok && manager != nil {
		return manager
	}

	channelID := parsePairingChannelID(channelKey)
	silentManager := newManager(channelID)
	silentManager.chanId = channelID
	r.managers[channelKey] = silentManager
	return silentManager
}

func (r *managerRegistry) Delete(channelKey string) {
	channelKey = strings.TrimSpace(channelKey)
	if channelKey == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.managers, channelKey)
}

func GetKey(chType string, chanID string) string {
	chanType := strings.ToLower(strings.TrimSpace(chType))
	chanID = strings.TrimSpace(chanID)
	if chanType == "" || chanID == "" {
		return ""
	}
	return chanType + ":" + chanID
}

func parsePairingChannelID(channelKey string) string {
	parts := strings.SplitN(strings.TrimSpace(channelKey), ":", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(channelKey)
	}
	return strings.TrimSpace(parts[1])
}
