// Package api contains HTTP handlers for the IntegrationHub REST API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/connauth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/oauth"
	"github.com/RasimAghayev/integrationhub/internal/secret"
	"github.com/go-chi/chi/v5"
)

// ConnectionStore is the persistence contract used by ConnectionHandler.
type ConnectionStore interface {
	Create(ctx context.Context, c *domain.Connection) error
	Get(ctx context.Context, id string) (*domain.Connection, error)
	List(ctx context.Context) ([]*domain.Connection, error)
	Update(ctx context.Context, c *domain.Connection) error
	Delete(ctx context.Context, id string) error
}

// ConnectionHandler serves connection CRUD endpoints.
type ConnectionHandler struct {
	store  ConnectionStore
	cipher *secret.Cipher
	oauth  *oauth.Manager
}

// NewConnectionHandler builds a ConnectionHandler. cipher may be nil when
// credential encryption is not configured; credentials are then rejected. An
// optional oauth.Manager enables the OAuth2 flow endpoints.
func NewConnectionHandler(store ConnectionStore, cipher *secret.Cipher, managers ...*oauth.Manager) *ConnectionHandler {
	h := &ConnectionHandler{store: store, cipher: cipher}
	if len(managers) > 0 {
		h.oauth = managers[0]
	}
	return h
}

type connectionRequest struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	AuthType    domain.AuthType `json:"auth_type"`
	Config      map[string]any  `json:"config"`
	Credentials json.RawMessage `json:"credentials"`
}

func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	c := &domain.Connection{
		Name:     req.Name,
		Type:     req.Type,
		AuthType: req.AuthType,
		Config:   req.Config,
	}
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credentials) > 0 && string(req.Credentials) != "null" {
		if h.cipher == nil {
			writeError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}
		enc, err := h.cipher.Encrypt(req.Credentials)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt credentials")
			return
		}
		c.SecretRef = enc
	}
	if err := h.store.Create(r.Context(), c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create connection")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *ConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list connections")
		return
	}
	total := len(items)
	if limit, offset, ok := parsePagination(r); ok {
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		writeJSON(w, http.StatusOK, page{Items: items[offset:end], Limit: limit, Offset: offset, Total: total})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ConnectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req connectionRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	existing.Name = req.Name
	existing.Type = req.Type
	existing.AuthType = req.AuthType
	existing.Config = req.Config
	if err := existing.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credentials) > 0 && string(req.Credentials) != "null" {
		if h.cipher == nil {
			writeError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}
		enc, err := h.cipher.Encrypt(req.Credentials)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt credentials")
			return
		}
		existing.SecretRef = enc
	}
	if err := h.store.Update(r.Context(), existing); err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *ConnectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeNotFound(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// OAuthAuthorize redirects the caller to the provider's authorization page.
func (h *ConnectionHandler) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil {
		writeError(w, http.StatusServiceUnavailable, "oauth not configured")
		return
	}
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	creds, err := connauth.Credentials(h.cipher, c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	cfg := oauthConfigFrom(c, creds)
	verifier := h.oauth.NewVerifier()
	state := h.oauth.SignState(c.ID)
	h.oauth.SaveVerifier(state, verifier)
	http.Redirect(w, r, h.oauth.AuthCodeURL(cfg, state, verifier), http.StatusFound)
}

// OAuthCallback handles the provider redirect, exchanges the code, and stores
// the resulting tokens encrypted.
func (h *ConnectionHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil || h.cipher == nil {
		writeError(w, http.StatusServiceUnavailable, "oauth not configured")
		return
	}
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	if !h.oauth.VerifyState(r.URL.Query().Get("state"), c.ID) {
		writeError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing authorization code")
		return
	}
	verifier, ok := h.oauth.TakeVerifier(r.URL.Query().Get("state"))
	if !ok {
		writeError(w, http.StatusBadRequest, "missing or expired PKCE verifier")
		return
	}
	creds, err := connauth.Credentials(h.cipher, c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	tok, err := h.oauth.Exchange(r.Context(), oauthConfigFrom(c, creds), code, verifier)
	if err != nil {
		writeError(w, http.StatusBadGateway, "token exchange failed")
		return
	}
	if err := h.storeToken(r.Context(), c, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store tokens")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// OAuthRefresh rotates the access token using the stored refresh token.
func (h *ConnectionHandler) OAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil || h.cipher == nil {
		writeError(w, http.StatusServiceUnavailable, "oauth not configured")
		return
	}
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	creds, err := connauth.Credentials(h.cipher, c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	prev := &oauth.Token{}
	if raw, ok := creds["access_token"]; ok {
		prev.AccessToken, _ = raw.(string)
	}
	if raw, ok := creds["refresh_token"]; ok {
		prev.RefreshToken, _ = raw.(string)
	}
	if raw, ok := creds["expiry"].(string); ok && raw != "" {
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			prev.Expiry = ts
		}
	}
	if prev.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "no refresh token available")
		return
	}
	tok, err := h.oauth.Refresh(r.Context(), oauthConfigFrom(c, creds), prev)
	if err != nil {
		writeError(w, http.StatusBadGateway, "token refresh failed")
		return
	}
	if err := h.storeToken(r.Context(), c, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store tokens")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"refreshed": true})
}

// Test issues a live request against the connection's health endpoint and
// reports the outcome. It never returns credentials.
func (h *ConnectionHandler) Test(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	base := strings.TrimRight(connauth.String(c.Config["base_url"]), "/")
	if base == "" {
		writeError(w, http.StatusBadRequest, "connection has no base_url")
		return
	}
	healthPath := connauth.String(c.Config["health_path"])
	if healthPath == "" {
		healthPath = "/health"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+healthPath, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid base_url")
		return
	}
	creds, err := connauth.Credentials(h.cipher, c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	connauth.Apply(req, c, creds)

	timeout := 10 * time.Second
	if v, ok := c.Config["timeout_ms"]; ok {
		if ms, err := strconv.Atoi(connauth.String(v)); err == nil && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}
	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latency_ms": latency, "error": err.Error()})
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          resp.StatusCode < 400,
		"status_code": resp.StatusCode,
		"latency_ms":  latency,
	})
}

func (h *ConnectionHandler) storeToken(ctx context.Context, c *domain.Connection, tok *oauth.Token) error {
	blob, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	enc, err := h.cipher.Encrypt(blob)
	if err != nil {
		return err
	}
	c.SecretRef = enc
	return h.store.Update(ctx, c)
}

func oauthConfigFrom(c *domain.Connection, creds map[string]any) oauth.Config {
	return oauth.Config{
		ClientID:     connauth.String(creds["client_id"]),
		ClientSecret: connauth.String(creds["client_secret"]),
		AuthURL:      connauth.String(c.Config["auth_url"]),
		TokenURL:     connauth.String(c.Config["token_url"]),
		RedirectURL:  connauth.String(c.Config["redirect_url"]),
		Scopes:       connauth.Strings(c.Config["scopes"]),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeNotFound(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}
