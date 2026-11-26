//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCheckPostgresWithContainer(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		Env:          map[string]string{"POSTGRES_USER": "integrationhub", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "integrationhub"},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	t.Setenv("INTEGRATIONHUB_DB_HOST", "localhost")
	t.Setenv("INTEGRATIONHUB_DB_PORT", port.Port())
	t.Setenv("INTEGRATIONHUB_DB_USER", "integrationhub")
	t.Setenv("INTEGRATIONHUB_DB_PASSWORD", "test")
	t.Setenv("INTEGRATIONHUB_DB_NAME", "integrationhub")

	require.True(t, checkPostgres(ctx))
}
