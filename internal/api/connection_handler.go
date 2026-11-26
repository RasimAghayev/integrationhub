// Package api contains HTTP handlers for the IntegrationHub REST API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	creds, err := h.decryptCreds(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	cfg := oauthConfigFrom(c, creds)
	state := h.oauth.SignState(c.ID)
	http.Redirect(w, r, h.oauth.AuthCodeURL(cfg, state), http.StatusFound)
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
	creds, err := h.decryptCreds(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}
	tok, err := h.oauth.Exchange(r.Context(), oauthConfigFrom(c, creds), code)
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
	creds, err := h.decryptCreds(c)
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
	base := strings.TrimRight(stringVal(c.Config["base_url"]), "/")
	if base == "" {
		writeError(w, http.StatusBadRequest, "connection has no base_url")
		return
	}
	healthPath := stringVal(c.Config["health_path"])
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
	if err := h.applyAuth(req, c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply auth")
		return
	}

	timeout := 10 * time.Second
	if v, ok := c.Config["timeout_ms"]; ok {
		if ms, err := strconv.Atoi(stringVal(v)); err == nil && ms > 0 {
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
	defer resp.Body.Close()
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

func (h *ConnectionHandler) decryptCreds(c *domain.Connection) (map[string]any, error) {
	if c.SecretRef == "" || h.cipher == nil {
		return map[string]any{}, nil
	}
	raw, err := h.cipher.Decrypt(c.SecretRef)
	if err != nil {
		return nil, err
	}
	creds := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &creds); err != nil {
			return nil, err
		}
	}
	return creds, nil
}

func (h *ConnectionHandler) applyAuth(req *http.Request, c *domain.Connection) error {
	creds, err := h.decryptCreds(c)
	if err != nil {
		return err
	}
	switch c.AuthType {
	case domain.AuthAPIKey:
		header := stringVal(c.Config["api_key_header"])
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, stringVal(creds["api_key"]))
	case domain.AuthBasic:
		req.SetBasicAuth(stringVal(creds["username"]), stringVal(creds["password"]))
	case domain.AuthOAuth2:
		req.Header.Set("Authorization", "Bearer "+stringVal(creds["access_token"]))
	}
	return nil
}

func oauthConfigFrom(c *domain.Connection, creds map[string]any) oauth.Config {
	return oauth.Config{
		ClientID:     stringVal(creds["client_id"]),
		ClientSecret: stringVal(creds["client_secret"]),
		AuthURL:      stringVal(c.Config["auth_url"]),
		TokenURL:     stringVal(c.Config["token_url"]),
		RedirectURL:  stringVal(c.Config["redirect_url"]),
		Scopes:       stringSlice(c.Config["scopes"]),
	}
}

func stringVal(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, stringVal(e))
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return strings.Split(t, ",")
	}
	return nil
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
