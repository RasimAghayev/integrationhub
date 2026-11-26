package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/RasimAghayev/integrationhub/internal/auth"
)

// AuthHandler issues JWTs to callers that already hold an admin API key.
type AuthHandler struct {
	jwt     *auth.JWTManager
	keys    auth.KeyStore
	refresh *auth.RefreshManager
}

// NewAuthHandler builds an AuthHandler. refresh may be nil.
func NewAuthHandler(jwt *auth.JWTManager, keys auth.KeyStore, refresh *auth.RefreshManager) *AuthHandler {
	return &AuthHandler{jwt: jwt, keys: keys, refresh: refresh}
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
		"token_type":    "Bearer",
		"expires_in":    int(h.jwt.TTL().Seconds()),
		"access_token":  token,
		"refresh_token": h.issueRefresh(r, subject, roles),
	})
}

func (h *AuthHandler) issueRefresh(r *http.Request, subject string, roles []string) string {
	if h.refresh == nil {
		return ""
	}
	raw, err := h.refresh.Issue(r.Context(), subject, roles)
	if err != nil {
		slog.Warn("failed to issue refresh token", "error", err)
		return ""
	}
	return raw
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh exchanges a refresh token for a new access token, rotating the
// refresh token. Reuse of a revoked token revokes the whole family.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.jwt == nil || h.refresh == nil {
		writeError(w, http.StatusServiceUnavailable, "refresh not configured")
		return
	}
	var req refreshRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	raw, subject, roles, err := h.refresh.Rotate(r.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRefreshReuse):
			writeError(w, http.StatusUnauthorized, "refresh token reuse detected")
		case errors.Is(err, auth.ErrInvalidRefresh):
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
		default:
			writeError(w, http.StatusInternalServerError, "failed to rotate refresh token")
		}
		return
	}
	access, err := h.jwt.Issue(subject, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token_type":    "Bearer",
		"expires_in":    int(h.jwt.TTL().Seconds()),
		"access_token":  access,
		"refresh_token": raw,
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
