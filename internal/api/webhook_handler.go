package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/connauth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/execution"
	"github.com/RasimAghayev/integrationhub/internal/webhook"
	"github.com/go-chi/chi/v5"
)

// maxWebhookBody caps the accepted webhook payload size.
const maxWebhookBody = 1 << 20

// ConnectionGetter is the subset of the connection store the webhook receiver
// needs to look up the signing secret.
type ConnectionGetter interface {
	Get(ctx context.Context, id string) (*domain.Connection, error)
}

// WebhookHandler receives inbound webhooks and turns them into executions.
type WebhookHandler struct {
	svc       *execution.Service
	ints      IntegrationStore
	conns     ConnectionGetter
	replay    *webhook.ReplayCache
	tolerance time.Duration
}

// NewWebhookHandler builds a WebhookHandler.
func NewWebhookHandler(svc *execution.Service, ints IntegrationStore, conns ConnectionGetter) *WebhookHandler {
	return &WebhookHandler{
		svc:       svc,
		ints:      ints,
		conns:     conns,
		replay:    webhook.NewReplayCache(10 * time.Minute),
		tolerance: 5 * time.Minute,
	}
}

// Receive authenticates the request (HMAC when a webhook secret is configured),
// suppresses replays and queues an execution for the named endpoint.
func (h *WebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	in, err := h.ints.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if len(body) > maxWebhookBody {
		writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
		return
	}

	if secret := h.webhookSecret(r, in); secret != "" {
		header := r.Header.Get("webhook-signature")
		if header == "" {
			header = r.Header.Get("X-Hub-Signature-256")
		}
		if err := webhook.Verify([]string{secret}, header, body, time.Now(), h.tolerance); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
	}

	if id := r.Header.Get("webhook-id"); id != "" && h.replay.Seen(id, time.Now()) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate", "ignored": true})
		return
	}

	e, err := h.svc.Submit(r.Context(), in, chi.URLParam(r, "endpoint"), string(body))
	if err != nil {
		var unknown *execution.UnknownEndpointError
		if errors.As(err, &unknown) {
			writeError(w, http.StatusBadRequest, unknown.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to queue execution")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"execution_id": e.ID, "status": e.Status})
}

func (h *WebhookHandler) webhookSecret(r *http.Request, in *domain.Integration) string {
	if h.conns == nil {
		return ""
	}
	conn, err := h.conns.Get(r.Context(), in.ConnectionID)
	if err != nil || conn == nil {
		return ""
	}
	return connauth.String(conn.Config["webhook_secret"])
}
