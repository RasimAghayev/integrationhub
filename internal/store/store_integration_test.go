//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestConnectionStoreCRUD(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			Env:          map[string]string{"POSTGRES_USER": "integrationhub", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "integrationhub"},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := "host=localhost port=" + port.Port() + " user=integrationhub password=test dbname=integrationhub sslmode=disable"

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, Migrate(ctx, pool))

	s := NewConnectionStore(pool)
	c := &domain.Connection{Name: "GitHub", Type: "rest", AuthType: domain.AuthOAuth2, Config: map[string]any{"base_url": "https://api.github.com"}}
	require.NoError(t, c.Validate())
	require.NoError(t, s.Create(ctx, c))
	require.NotEmpty(t, c.ID)

	got, err := s.Get(ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, "GitHub", got.Name)

	list, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	c.Name = "GitLab"
	require.NoError(t, s.Update(ctx, c))
	got, _ = s.Get(ctx, c.ID)
	require.Equal(t, "GitLab", got.Name)

	require.NoError(t, s.Delete(ctx, c.ID))
	_, err = s.Get(ctx, c.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestIntegrationStoreCRUD(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			Env:          map[string]string{"POSTGRES_USER": "integrationhub", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "integrationhub"},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := "host=localhost port=" + port.Port() + " user=integrationhub password=test dbname=integrationhub sslmode=disable"

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, Migrate(ctx, pool))

	conn := &domain.Connection{Name: "GitHub", Type: "rest", AuthType: domain.AuthNone}
	require.NoError(t, NewConnectionStore(pool).Create(ctx, conn))

	is := NewIntegrationStore(pool)
	i := &domain.Integration{
		Name: "GitHub", Type: domain.IntegrationREST, ConnectionID: conn.ID,
		Endpoints: []domain.Endpoint{{Name: "list", Method: "GET", Path: "/users"}},
	}
	require.NoError(t, i.Validate())
	require.NoError(t, is.Create(ctx, i))
	require.NotEmpty(t, i.ID)

	got, err := is.Get(ctx, i.ID)
	require.NoError(t, err)
	require.Len(t, got.Endpoints, 1)
	require.Equal(t, "/users", got.Endpoints[0].Path)

	require.NoError(t, is.Delete(ctx, i.ID))
	_, err = is.Get(ctx, i.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestExecutionStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			Env:          map[string]string{"POSTGRES_USER": "integrationhub", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "integrationhub"},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := "host=localhost port=" + port.Port() + " user=integrationhub password=test dbname=integrationhub sslmode=disable"

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, Migrate(ctx, pool))

	conn := &domain.Connection{Name: "GitHub", Type: "rest", AuthType: domain.AuthNone}
	require.NoError(t, NewConnectionStore(pool).Create(ctx, conn))
	i := &domain.Integration{
		Name: "GitHub", Type: domain.IntegrationREST, ConnectionID: conn.ID,
		Endpoints: []domain.Endpoint{{Name: "list", Method: "GET", Path: "/users"}},
	}
	require.NoError(t, NewIntegrationStore(pool).Create(ctx, i))

	es := NewExecutionStore(pool)
	e := &domain.Execution{IntegrationID: i.ID, Endpoint: "list", Status: domain.ExecutionQueued}
	require.NoError(t, es.Create(ctx, e))
	require.NotEmpty(t, e.ID)

	claimed, err := es.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, e.ID, claimed.ID)
	require.Equal(t, domain.ExecutionRunning, claimed.Status)
	require.NotNil(t, claimed.StartedAt)

	_, err = es.ClaimNext(ctx)
	require.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, es.Complete(ctx, e.ID, domain.ExecutionSucceeded, 200, `{"ok":true}`, 1, ""))
	got, err := es.Get(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ExecutionSucceeded, got.Status)
	require.Equal(t, 200, got.StatusCode)
	require.Equal(t, 1, got.Attempts)
	require.NotNil(t, got.FinishedAt)

	dead := &domain.Execution{IntegrationID: i.ID, Endpoint: "list", Status: domain.ExecutionQueued}
	require.NoError(t, es.Create(ctx, dead))
	_, err = es.ClaimNext(ctx)
	require.NoError(t, err)
	require.NoError(t, es.Complete(ctx, dead.ID, domain.ExecutionDead, 500, "", 3, "boom"))

	deadList, err := es.ListDead(ctx, 10)
	require.NoError(t, err)
	require.Len(t, deadList, 1)
	require.Equal(t, dead.ID, deadList[0].ID)

	require.NoError(t, es.Requeue(ctx, dead.ID))
	requeued, err := es.Get(ctx, dead.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ExecutionQueued, requeued.Status)

	items, total, err := es.List(ctx, i.ID, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, items, 2)

	counts, err := es.CountByStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, counts[domain.ExecutionQueued])
}

func TestAPIKeyStore(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			Env:          map[string]string{"POSTGRES_USER": "integrationhub", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "integrationhub"},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	dsn := "host=localhost port=" + port.Port() + " user=integrationhub password=test dbname=integrationhub sslmode=disable"

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	require.NoError(t, Migrate(ctx, pool))

	ks := NewAPIKeyStore(pool)
	k := &domain.APIKey{Subject: "svc", KeyHash: "hash-1", Roles: []string{"operator"}}
	require.NoError(t, ks.Create(ctx, k))
	require.NotEmpty(t, k.ID)

	rec, err := ks.GetByHash(ctx, "hash-1")
	require.NoError(t, err)
	require.Equal(t, "svc", rec.Subject)
	require.Equal(t, []string{"operator"}, rec.Roles)

	list, err := ks.List(ctx, "svc")
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, ks.Delete(ctx, k.ID))
	_, err = ks.GetByHash(ctx, "hash-1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}
