package api

import (
	"net/http"

	"github.com/RasimAghayev/integrationhub/internal/auth"
)

// AuthHandler issues JWTs to callers that already hold an admin API key.
type AuthHandler struct {
	jwt  *auth.JWTManager
	keys auth.KeyStore
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(jwt *auth.JWTManager, keys auth.KeyStore) *AuthHandler {
	return &AuthHandler{jwt: jwt, keys: keys}
}

type tokenRequest struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

// Token exchanges an admin API key for a short-lived JWT.
func (h *AuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.jwt == nil || h.keys == nil {
		writeError(w, http.StatusServiceUnavailable, "token issuance not configured")
		return
	}
	raw := r.Header.Get("X-API-Key")
	if raw == "" {
		writeError(w, http.StatusUnauthorized, "missing api key")
		return
	}
	rec, err := h.keys.GetByHash(r.Context(), auth.HashKey(raw))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	if !hasRole(rec.Roles, "admin") {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}
	var req tokenRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	subject := req.Subject
	if subject == "" {
		subject = rec.Subject
	}
	roles := req.Roles
	if len(roles) == 0 {
		roles = rec.Roles
	}
	token, err := h.jwt.Issue(subject, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token_type":   "Bearer",
		"expires_in":   int(h.jwt.TTL().Seconds()),
		"access_token": token,
	})
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}
