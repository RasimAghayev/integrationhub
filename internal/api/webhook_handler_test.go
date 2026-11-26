package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/events"
	"github.com/RasimAghayev/integrationhub/internal/execution"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"github.com/RasimAghayev/integrationhub/internal/webhook"
	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"
)

func webhookRouter(t *testing.T, secret string) (http.Handler, *memExecStore) {
	t.Helper()
	ints := newFakeIntegrationStore()
	ints.items["int-1"] = &domain.Integration{
		ID:           "int-1",
		Name:         "demo",
		Type:         domain.IntegrationWebhook,
		Status:       domain.StatusActive,
		ConnectionID: "conn-1",
		Endpoints:    []domain.Endpoint{{Name: "hook", Method: http.MethodPost, Path: "/hook"}},
	}
	cfg := executor.DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.Rate = rate.Inf

	store := newMemExecStore()
	conn := &domain.Connection{ID: "conn-1", Config: map[string]any{}}
	if secret != "" {
		conn.Config["webhook_secret"] = secret
	}
	svc := execution.New(store, ints, memConnStore{c: conn}, executor.New(cfg, nil), nil)
	h := NewWebhookHandler(svc, ints, memConnStore{c: conn})

	r := chi.NewRouter()
	r.Post("/api/v1/webhooks/{id}/{endpoint}", h.Receive)
	return r, store
}

func doWebhook(t *testing.T, r http.Handler, path, body, sig, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	if sig != "" {
		req.Header.Set("webhook-signature", sig)
	}
	if id != "" {
		req.Header.Set("webhook-id", id)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestWebhookQueuesExecution(t *testing.T) {
	r, store := webhookRouter(t, "")
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", `{"a":1}`, "", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.items) != 1 {
		t.Fatalf("executions = %d", len(store.items))
	}
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	r, store := webhookRouter(t, "s3cret")
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", `{"a":1}`, "t=1,v1=deadbeef", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(store.items) != 0 {
		t.Fatal("execution must not be queued")
	}
}

func TestWebhookAcceptsValidSignature(t *testing.T) {
	r, store := webhookRouter(t, "s3cret")
	body := `{"a":1}`
	sig := webhook.Sign("s3cret", time.Now(), []byte(body))
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", body, sig, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.items) != 1 {
		t.Fatalf("executions = %d", len(store.items))
	}
}

func TestWebhookDeduplicates(t *testing.T) {
	r, store := webhookRouter(t, "")
	first := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", `{}`, "", "evt-1")
	if first.Code != http.StatusAccepted {
		t.Fatalf("first = %d", first.Code)
	}
	second := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", `{}`, "", "evt-1")
	if second.Code != http.StatusOK {
		t.Fatalf("second = %d", second.Code)
	}
	if len(store.items) != 1 {
		t.Fatalf("executions = %d", len(store.items))
	}
}

func TestWebhookUnknownEndpoint(t *testing.T) {
	r, _ := webhookRouter(t, "")
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/nope", `{}`, "", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWebhookUnknownIntegration(t *testing.T) {
	r, _ := webhookRouter(t, "")
	rec := doWebhook(t, r, "/api/v1/webhooks/missing/hook", `{}`, "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWebhookOversizedBody(t *testing.T) {
	r, _ := webhookRouter(t, "")
	big := bytes.Repeat([]byte("x"), maxWebhookBody+1)
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", string(big), "", "")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWebhookRequiresSignatureWhenConfigured(t *testing.T) {
	r, _ := webhookRouter(t, "s3cret")
	rec := doWebhook(t, r, "/api/v1/webhooks/int-1/hook", `{}`, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestExecutionServicePublishesEvents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	ints := newFakeIntegrationStore()
	ints.items["int-1"] = &domain.Integration{
		ID: "int-1", Name: "demo", Type: domain.IntegrationREST, Status: domain.StatusActive,
		ConnectionID: "conn-1",
		Endpoints:    []domain.Endpoint{{Name: "get", Method: http.MethodGet, Path: "/ok"}},
	}
	cfg := executor.DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.Rate = rate.Inf
	svc := execution.New(newMemExecStore(), ints, memConnStore{c: &domain.Connection{
		ID: "conn-1", Config: map[string]any{"base_url": upstream.URL},
	}}, executor.New(cfg, nil), nil)

	var got []string
	svc.SetPublisher(recorder{fn: func(e events.Event) { got = append(got, e.Type) }})
	e, err := svc.Submit(context.Background(), ints.items["int-1"], "get", "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := svc.Run(context.Background(), e); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 || got[0] != string(domain.ExecutionSucceeded) {
		t.Fatalf("events = %v", got)
	}
}

type recorder struct{ fn func(events.Event) }

func (r recorder) Publish(_ context.Context, e events.Event) error {
	r.fn(e)
	return nil
}
