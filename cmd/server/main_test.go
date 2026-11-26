package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handleRoot(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "integrationhub")
}

func TestHandleReadyz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handleReadyz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ready")
}

func TestMetricsEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	newRouter(routerDeps{}).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.Contains(rec.Body.String(), "go_"))
}

func TestBoolStatus(t *testing.T) {
	assert.Equal(t, http.StatusOK, boolStatus(true))
	assert.Equal(t, http.StatusServiceUnavailable, boolStatus(false))
}

func TestStatusString(t *testing.T) {
	assert.Equal(t, "up", statusString(true))
	assert.Equal(t, "degraded", statusString(false))
}

func TestEnv(t *testing.T) {
	t.Setenv("INTEGRATIONHUB_TEST_ENV", "value")
	assert.Equal(t, "value", env("INTEGRATIONHUB_TEST_ENV", "default"))
	assert.Equal(t, "default", env("INTEGRATIONHUB_TEST_ENV_MISSING", "default"))
}
