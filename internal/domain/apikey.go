package domain

import "time"

// APIKey is a hashed machine credential used to authenticate service callers.
// The plaintext key is shown once at creation and never stored.
type APIKey struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	KeyHash   string    `json:"-"`
	Roles     []string  `json:"roles"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
