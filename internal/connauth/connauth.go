// Package connauth resolves connection credentials and applies them to
// outbound HTTP requests. It is shared by the API layer and the execution
// service so authentication is implemented exactly once.
package connauth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/secret"
)

// Credentials decrypts and returns the credential map stored for a connection.
// A connection without a secret yields an empty map.
func Credentials(cipher *secret.Cipher, c *domain.Connection) (map[string]any, error) {
	if c == nil || c.SecretRef == "" || cipher == nil {
		return map[string]any{}, nil
	}
	raw, err := cipher.Decrypt(c.SecretRef)
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

// Apply sets the authentication headers described by the connection's auth type.
func Apply(req *http.Request, c *domain.Connection, creds map[string]any) {
	ApplyHeader(req.Header, c, creds)
}

// ApplyHeader sets authentication headers on an arbitrary header set. It is the
// form used by the execution engine, which builds requests itself.
func ApplyHeader(h http.Header, c *domain.Connection, creds map[string]any) {
	if c == nil {
		return
	}
	switch c.AuthType {
	case domain.AuthAPIKey:
		header := String(c.Config["api_key_header"])
		if header == "" {
			header = "X-API-Key"
		}
		h.Set(header, String(creds["api_key"]))
	case domain.AuthBasic:
		cred := String(creds["username"]) + ":" + String(creds["password"])
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cred)))
	case domain.AuthOAuth2:
		h.Set("Authorization", "Bearer "+String(creds["access_token"]))
	}
}

// String coerces a config value to a string, returning "" for other types.
func String(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Strings coerces a config value to a string slice. A comma-separated string
// is split into its trimmed elements.
func Strings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, String(e))
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return strings.Split(t, ",")
	}
	return nil
}
