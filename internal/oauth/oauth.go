// Package oauth implements the OAuth2 authorization-code flow used by
// connections of type oauth2, plus signed state handling.
package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Config holds the per-connection OAuth2 client settings. ClientID and
// ClientSecret are secret and come from the encrypted credential blob; the
// remaining fields are non-secret connection config.
type Config struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
}

func (c Config) oauth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: c.AuthURL, TokenURL: c.TokenURL},
		RedirectURL:  c.RedirectURL,
		Scopes:       c.Scopes,
	}
}

// Token is the credential blob persisted (encrypted) for an oauth2 connection.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

func tokenFromOAuth2(t *oauth2.Token) *Token {
	return &Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

func (t *Token) oauth2Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// Manager signs/validates OAuth2 state and performs token exchanges. It also
// holds the short-lived PKCE code verifiers between authorize and callback.
type Manager struct {
	stateKey []byte

	mu        sync.Mutex
	verifiers map[string]verifierEntry
}

type verifierEntry struct {
	verifier string
	expires  time.Time
}

// NewManager builds a Manager. stateKey should be a stable server secret.
func NewManager(stateKey []byte) *Manager {
	return &Manager{stateKey: stateKey, verifiers: map[string]verifierEntry{}}
}

// NewVerifier returns a fresh PKCE code verifier.
func (m *Manager) NewVerifier() string { return oauth2.GenerateVerifier() }

// SaveVerifier stores a PKCE verifier keyed by state, dropping expired entries.
func (m *Manager) SaveVerifier(state, verifier string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.verifiers {
		if v.expires.Before(now) {
			delete(m.verifiers, k)
		}
	}
	m.verifiers[state] = verifierEntry{verifier: verifier, expires: now.Add(10 * time.Minute)}
}

// TakeVerifier returns and removes the verifier for state.
func (m *Manager) TakeVerifier(state string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.verifiers[state]
	if !ok || v.expires.Before(time.Now()) {
		delete(m.verifiers, state)
		return "", false
	}
	delete(m.verifiers, state)
	return v.verifier, true
}

// AuthCodeURL returns the provider authorization URL for the given state,
// including the S256 PKCE code challenge for verifier.
func (m *Manager) AuthCodeURL(cfg Config, state, verifier string) string {
	return cfg.oauth2Config().AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	)
}

// Exchange trades an authorization code for tokens, presenting the PKCE verifier.
func (m *Manager) Exchange(ctx context.Context, cfg Config, code, verifier string) (*Token, error) {
	t, err := cfg.oauth2Config().Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("oauth: exchange: %w", err)
	}
	return tokenFromOAuth2(t), nil
}

// Refresh obtains a new access token using the stored refresh token.
func (m *Manager) Refresh(ctx context.Context, cfg Config, t *Token) (*Token, error) {
	src := cfg.oauth2Config().TokenSource(ctx, t.oauth2Token())
	nt, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("oauth: refresh: %w", err)
	}
	return tokenFromOAuth2(nt), nil
}

// SignState produces a signed, expiring state bound to a connection ID.
func (m *Manager) SignState(connID string) string {
	payload := fmt.Sprintf("%s|%d", connID, time.Now().Add(10*time.Minute).Unix())
	mac := hmac.New(sha256.New, m.stateKey)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

// VerifyState reports whether state is authentic, unexpired, and bound to connID.
func (m *Manager) VerifyState(state, connID string) bool {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, m.stateKey)
	mac.Write(payload)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return false
	}
	segs := strings.SplitN(string(payload), "|", 2)
	if len(segs) != 2 || segs[0] != connID {
		return false
	}
	exp, err := strconv.ParseInt(segs[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}
