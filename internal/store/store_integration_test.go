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
