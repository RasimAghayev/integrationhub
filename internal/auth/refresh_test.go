package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
)

type memRefreshStore struct {
	mu     sync.Mutex
	byHash map[string]*RefreshRecord
	byID   map[string]*RefreshRecord
	seq    int
}

func newMemRefreshStore() *memRefreshStore {
	return &memRefreshStore{byHash: map[string]*RefreshRecord{}, byID: map[string]*RefreshRecord{}}
}

func (m *memRefreshStore) Create(_ context.Context, rec *RefreshRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	rec.ID = "rt-" + string(rune('0'+m.seq))
	m.byHash[rec.TokenHash] = rec
	m.byID[rec.ID] = rec
	return nil
}

func (m *memRefreshStore) GetByHash(_ context.Context, hash string) (*RefreshRecord, error) {
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

func TestRefreshIssueAndRotate(t *testing.T) {
	store := newMemRefreshStore()
	m := NewRefreshManager(store, time.Hour)

	raw, err := m.Issue(context.Background(), "alice", []string{"admin"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if raw == "" || store.byHash[HashRefresh(raw)] == nil {
		t.Fatal("token was not stored as a hash")
	}

	next, subject, roles, err := m.Rotate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if subject != "alice" || len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("subject/roles = %s %v", subject, roles)
	}
	old := store.byHash[HashRefresh(raw)]
	if old.RevokedAt == nil || old.ReplacedBy == nil {
		t.Fatal("old token must be revoked and linked to its replacement")
	}
	if store.byHash[HashRefresh(next)] == nil {
		t.Fatal("new token was not stored")
	}
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	store := newMemRefreshStore()
	m := NewRefreshManager(store, time.Hour)
	raw, _ := m.Issue(context.Background(), "bob", nil)
	next, _, _, err := m.Rotate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if _, _, _, err := m.Rotate(context.Background(), raw); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("err = %v, want reuse", err)
	}
	revoked := store.byHash[HashRefresh(next)]
	if revoked.RevokedAt == nil {
		t.Fatal("reuse must revoke the whole family")
	}
}

func TestRefreshRejectsUnknownAndExpired(t *testing.T) {
	m := NewRefreshManager(newMemRefreshStore(), time.Hour)
	if _, _, _, err := m.Rotate(context.Background(), "nope"); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("unknown token err = %v", err)
	}
	if _, _, _, err := m.Rotate(context.Background(), ""); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("empty token err = %v", err)
	}

	store := newMemRefreshStore()
	expired := NewRefreshManager(store, time.Hour)
	clock := time.Now()
	expired.now = func() time.Time { return clock }
	raw, _ := expired.Issue(context.Background(), "c", nil)
	clock = clock.Add(2 * time.Hour)
	if _, _, _, err := expired.Rotate(context.Background(), raw); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("expired token err = %v", err)
	}
}

func TestNewRefreshManagerDefaultTTL(t *testing.T) {
	m := NewRefreshManager(newMemRefreshStore(), 0)
	if m.TTL() != 30*24*time.Hour {
		t.Fatalf("ttl = %s", m.TTL())
	}
}
