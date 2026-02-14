package provider

import (
	"sync"
	"sync/atomic"

	"github.com/bytedance/gg/gmap"
)

var (
	defaultRegistry = NewRegistry()

	Get      = defaultRegistry.Get
	Register = defaultRegistry.Register
)

type Registry struct {
	providers map[string]Provider
	cnt       atomic.Int64
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
	r.cnt.Add(1)
	return nil
}

func (r *Registry) Get(id string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id], nil
}

func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return gmap.ToSlice(r.providers, func(k string, v Provider) Provider { return v })
}

func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id] != nil
}

func (r *Registry) Unregister(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[id]; ok {
		delete(r.providers, id)
		r.cnt.Add(-1)
	}
}

func List() []Provider {
	return defaultRegistry.List()
}

func Exists(id string) bool {
	return defaultRegistry.Exists(id)
}

func Unregister(id string) {
	defaultRegistry.Unregister(id)
}
