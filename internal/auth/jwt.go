// Package auth authenticates callers with JWT bearer tokens or hashed API keys
// and exposes the resulting identity through the request context.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload issued by IntegrationHub.
type Claims struct {
	Subject string   `json:"sub"`
	Roles   []string `json:"roles"`
	jwt.RegisteredClaims
}

// JWTManager issues and verifies HS256 tokens.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// NewJWTManager builds a JWTManager. ttl must be positive.
func NewJWTManager(secret []byte, ttl time.Duration, issuer string) *JWTManager {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &JWTManager{secret: secret, ttl: ttl, issuer: issuer}
}

// Issue signs a token for the subject with the given roles.
func (m *JWTManager) Issue(subject string, roles []string) (string, error) {
	now := time.Now()
	claims := Claims{
		Subject: subject,
		Roles:   roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// TTL returns the configured token lifetime.
func (m *JWTManager) TTL() time.Duration { return m.ttl }

// Verify parses and validates a token, returning its claims.
func (m *JWTManager) Verify(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// GenerateKey returns a new random API key. The value is shown once and must be
// stored only as HashKey(key).
func GenerateKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashKey derives the lookup hash for an API key.
func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two hashes without leaking timing information.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
