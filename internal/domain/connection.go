// Package domain defines the core business entities of IntegrationHub.
package domain

import (
	"errors"
	"strings"
	"time"
)

// AuthType enumerates supported authentication schemes for a connection.
type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthAPIKey AuthType = "api_key"
	AuthBasic  AuthType = "basic"
	AuthOAuth2 AuthType = "oauth2"
)

// ValidAuthTypes is the set of accepted authentication schemes.
var ValidAuthTypes = map[AuthType]bool{
	AuthNone:   true,
	AuthAPIKey: true,
	AuthBasic:  true,
	AuthOAuth2: true,
}

// Connection is an external system IntegrationHub talks to. Credentials are
// stored encrypted (see the secret package) and never returned by the API.
type Connection struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	AuthType  AuthType       `json:"auth_type"`
	Config    map[string]any `json:"config"`
	SecretRef string         `json:"-"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ValidationError is returned when a connection fails validation.
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Validate checks the required fields and enum values of a connection.
func (c *Connection) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return &ValidationError{"name", "is required"}
	}
	if strings.TrimSpace(c.Type) == "" {
		return &ValidationError{"type", "is required"}
	}
	if !ValidAuthTypes[c.AuthType] {
		return &ValidationError{"auth_type", "must be one of none, api_key, basic, oauth2"}
	}
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	return nil
}

// ErrNotFound is returned by stores when a connection does not exist.
var ErrNotFound = errors.New("connection not found")
