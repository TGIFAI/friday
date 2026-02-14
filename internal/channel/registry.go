package channel

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/bytedance/gg/gmap"
)

var (
	defaultRegistry = NewRegistry()

	Get        = defaultRegistry.Get
	Len        = defaultRegistry.Len
	List       = defaultRegistry.List
	Register   = defaultRegistry.Register
	Unregister = defaultRegistry.Unregister
)

type Registry struct {
	chans map[string]Channel

	cnt atomic.Int64
	mu  sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		chans: make(map[string]Channel, 8),
	}
}

func (r *Registry) Register(ch Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chans[ch.ID()] = ch
	r.cnt.Add(1)
	return nil
}

func (r *Registry) Get(id string) (Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.chans[id]
	if !ok {
		return nil, errors.New("channel not found")
	}
	return ch, nil
}

func (r *Registry) List() []Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return gmap.ToSlice(
		r.chans,
		func(k string, v Channel) Channel { return v },
	)
}

func (r *Registry) Len() int {
	return int(r.cnt.Load())
}

func (r *Registry) Unregister(id string) {
	if id == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.chans[id]; ok {
		delete(r.chans, id)
		r.cnt.Add(-1)
	}
}
