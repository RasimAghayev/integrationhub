package rbac

import (
	"encoding/json"
	"net/http"

	"github.com/RasimAghayev/integrationhub/internal/auth"
)

// ResourceFunc maps a request to the resource being accessed. An empty result
// means the route is not access-controlled.
type ResourceFunc func(*http.Request) string

// Middleware authorizes the request against the enforcer. It must run after the
// authentication middleware.
func (e *Enforcer) Middleware(resource ResourceFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if e == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := auth.IdentityFrom(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			res := ""
			if resource != nil {
				res = resource(r)
			}
			if res == "" {
				next.ServeHTTP(w, r)
				return
			}
			action := "read"
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				action = "write"
			}
			if !e.Allow(id.Roles, res, action) {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
