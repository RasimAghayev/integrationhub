package api

import (
	"context"
	"sync"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/auth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
)

type memRefreshStore struct {
	mu     sync.Mutex
	byHash map[string]*auth.RefreshRecord
	byID   map[string]*auth.RefreshRecord
	seq    int
}

func newMemRefreshStoreForAPI() *memRefreshStore {
	return &memRefreshStore{
		byHash: map[string]*auth.RefreshRecord{},
		byID:   map[string]*auth.RefreshRecord{},
	}
}

func (m *memRefreshStore) Create(_ context.Context, rec *auth.RefreshRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	rec.ID = "rt-" + string(rune('0'+m.seq))
	m.byHash[rec.TokenHash] = rec
	m.byID[rec.ID] = rec
	return nil
}

func (m *memRefreshStore) GetByHash(_ context.Context, hash string) (*auth.RefreshRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.byHash[hash]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rec, nil
}

func (m *memRefreshStore) Revoke(_ context.Context, id, replacedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now()
	rec.RevokedAt = &now
	rec.ReplacedBy = &replacedBy
	return nil
}

func (m *memRefreshStore) RevokeFamily(_ context.Context, family string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, rec := range m.byID {
		if rec.Family == family && rec.RevokedAt == nil {
			rec.RevokedAt = &now
		}
	}
	return nil
}
