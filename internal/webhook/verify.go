// Package webhook verifies inbound webhook signatures (Standard-Webhooks style:
// a header of "t=<unix>,v1=<hex>" authenticating timestamp + raw body).
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrMissing is returned when the signature header is absent or malformed.
	ErrMissing = errors.New("webhook: missing or malformed signature header")
	// ErrSignature is returned when no signature matches.
	ErrSignature = errors.New("webhook: signature does not match")
	// ErrStale is returned when the timestamp is outside the tolerance window.
	ErrStale = errors.New("webhook: timestamp outside tolerance")
)

// Sign returns the header value a sender would put on body at time t.
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return "t=" + ts + ",v1=" + sign(secret, ts, body)
}

func sign(secret, ts string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(ts))
	h.Write([]byte("."))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify checks header against body using any of secrets (rotation) and a
// constant-time comparison. now/tolerance bound replay via the signed timestamp.
func Verify(secrets []string, header string, body []byte, now time.Time, tolerance time.Duration) error {
	ts, sigs := parse(header)
	if ts == "" || len(sigs) == 0 {
		return ErrMissing
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrMissing
	}
	matched := false
	for _, secret := range secrets {
		want := sign(secret, ts, body)
		for _, got := range sigs {
			if hmac.Equal([]byte(got), []byte(want)) {
				matched = true
			}
		}
	}
	if !matched {
		return ErrSignature
	}
	age := now.Sub(time.Unix(unix, 0))
	if age > tolerance || age < -tolerance {
		return ErrStale
	}
	return nil
}

func parse(header string) (string, []string) {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	return ts, sigs
}

// ReplayCache suppresses duplicate event IDs within a TTL window.
type ReplayCache struct {
	mu  sync.Mutex
	m   map[string]time.Time
	ttl time.Duration
}

// NewReplayCache builds a cache that remembers IDs for ttl.
func NewReplayCache(ttl time.Duration) *ReplayCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &ReplayCache{m: map[string]time.Time{}, ttl: ttl}
}

// Seen reports whether id was already seen; the first call records it.
func (c *ReplayCache) Seen(id string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.m[id]; ok && now.Sub(t) <= c.ttl {
		return true
	}
	c.m[id] = now
	return false
}
