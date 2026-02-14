package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
)

const defaultGCInterval = 10 * time.Minute

type ManagerOptions struct {
	Store Store
	TTL   time.Duration
}

type Manager struct {
	agentID string
	sessMap sync.Map
	storeMu sync.RWMutex
	store   Store
	ttlNS   atomic.Int64
}

func NewManager(agentID string, opts ...ManagerOptions) *Manager {
	mgr := &Manager{
		agentID: agentID,
	}
	mgr.ttlNS.Store(0)

	if len(opts) > 0 {
		mgr.SetStore(opts[0].Store)
		mgr.SetTTL(opts[0].TTL)
	}

	return mgr
}

func (m *Manager) AgentID() string {
	return m.agentID
}

func (m *Manager) BuildKey(channelType channel.Type, chatID string) string {
	return GenerateKey(m.agentID, channelType, chatID)
}

func (m *Manager) GetOrCreateFor(channelType channel.Type, chatID string) *Session {
	return m.GetOrCreate(m.BuildKey(channelType, chatID))
}

func (m *Manager) GetOrCreate(sessKey string) *Session {
	raw, ok := m.sessMap.Load(sessKey)
	if ok {
		existing := raw.(*Session)
		if existing.IsExpired(time.Now()) {
			_ = m.Delete(sessKey)
		} else {
			return existing
		}
	}

	store := m.getStore()
	if store != nil {
		loaded, err := store.Load(context.Background(), sessKey)
		if err != nil {
			logs.Warn("[session:%s] load failed for key=%s: %v", m.agentID, sessKey, err)
		} else if loaded != nil {
			actual, _ := m.sessMap.LoadOrStore(sessKey, loaded)
			return actual.(*Session)
		}
	}

	return m.Create(sessKey)
}

func (m *Manager) Create(sessKey string) *Session {
	if raw, ok := m.sessMap.Load(sessKey); ok {
		return raw.(*Session)
	}

	timeNow := time.Now()
	sess := &Session{
		SessionKey: sessKey,
		messages:   make([]*schema.Message, 0, 8),
		createTime: timeNow,
		updateTime: timeNow,
	}
	if agentID, channelType, userID, err := ParseKey(sessKey); err == nil {
		sess.AgentID = agentID
		sess.Channel = channelType
		sess.UserID = userID
	}

	actual, _ := m.sessMap.LoadOrStore(sessKey, sess)
	return actual.(*Session)
}

func (m *Manager) Save(sess *Session) error {
	if sess == nil {
		return nil
	}
	if ttl := m.TTL(); ttl > 0 {
		sess.SetExpireAt(time.Now().Add(ttl))
	}

	store := m.getStore()
	if store == nil {
		return nil
	}

	return store.Save(context.Background(), sess)
}

func (m *Manager) Delete(sessKey string) error {
	m.sessMap.Delete(sessKey)

	store := m.getStore()
	if store == nil {
		return nil
	}

	return store.Delete(context.Background(), sessKey)
}

func (m *Manager) SetStore(store Store) {
	m.storeMu.Lock()
	defer m.storeMu.Unlock()
	m.store = store
}

func (m *Manager) SetTTL(ttl time.Duration) {
	if ttl < 0 {
		ttl = 0
	}
	m.ttlNS.Store(ttl.Nanoseconds())
}

func (m *Manager) TTL() time.Duration {
	ns := m.ttlNS.Load()
	if ns <= 0 {
		return 0
	}
	return time.Duration(ns)
}

func (m *Manager) GC() (int, error) {
	store := m.getStore()
	if store == nil {
		return 0, nil
	}
	return store.GC(context.Background(), time.Now())
}

func (m *Manager) StartGCLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultGCInterval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed, err := m.GC()
				if err != nil {
					logs.CtxWarn(ctx, "[session] GC failed: %v", err)
					continue
				}
				if removed > 0 {
					logs.CtxInfo(ctx, "[session] GC removed %d expired session file(s)", removed)
				}
			}
		}
	}()
}

func (m *Manager) getStore() Store {
	m.storeMu.RLock()
	defer m.storeMu.RUnlock()
	return m.store
}
