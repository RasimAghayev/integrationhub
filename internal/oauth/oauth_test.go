package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStateSignVerify(t *testing.T) {
	m := NewManager([]byte("test-state-key"))
	state := m.SignState("conn-1")
	if !m.VerifyState(state, "conn-1") {
		t.Fatal("expected valid state for matching conn")
	}
	if m.VerifyState(state, "conn-2") {
		t.Fatal("state must not validate for a different conn")
	}
	if m.VerifyState(state+"x", "conn-1") {
		t.Fatal("tampered state must not validate")
	}
	if m.VerifyState("", "conn-1") {
		t.Fatal("empty state must not validate")
	}
}

func TestPKCEVerifierLifecycle(t *testing.T) {
	m := NewManager([]byte("k"))
	v := m.NewVerifier()
	if v == "" {
		t.Fatal("empty verifier")
	}
	state := m.SignState("c1")
	m.SaveVerifier(state, v)
	got, ok := m.TakeVerifier(state)
	if !ok || got != v {
		t.Fatalf("verifier mismatch: got %q ok=%v", got, ok)
	}
	if _, ok := m.TakeVerifier(state); ok {
		t.Fatal("verifier must be consumed after first take")
	}
}

func TestAuthCodeURLHasPKCEChallenge(t *testing.T) {
	m := NewManager([]byte("k"))
	cfg := Config{ClientID: "id", AuthURL: "https://provider/auth", TokenURL: "https://provider/token", RedirectURL: "http://localhost/cb"}
	u := m.AuthCodeURL(cfg, "state123", m.NewVerifier())
	if !strings.Contains(u, "code_challenge=") {
		t.Fatalf("missing PKCE challenge: %s", u)
	}
	if !strings.Contains(u, "state=state123") {
		t.Fatalf("missing state: %s", u)
	}
}

func TestExchangeAndRefresh(t *testing.T) {
	var gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "refresh_token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-access", "token_type": "Bearer", "expires_in": 3600,
			})
			return
		}
		gotCode = r.Form.Get("code")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 3600,
		})
	}))
	defer srv.Close()

	m := NewManager([]byte("k"))
	cfg := Config{ClientID: "id", ClientSecret: "sec", AuthURL: srv.URL + "/auth", TokenURL: srv.URL, RedirectURL: "http://localhost/cb"}

	tok, err := m.Exchange(context.Background(), cfg, "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if gotCode != "the-code" {
		t.Fatalf("code not forwarded: %q", gotCode)
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" {
		t.Fatalf("unexpected token: %+v", tok)
	}

	tok.Expiry = time.Now().Add(-time.Hour) // force refresh
	tok2, err := m.Refresh(context.Background(), cfg, tok)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tok2.AccessToken != "new-access" {
		t.Fatalf("expected refreshed access token, got %q", tok2.AccessToken)
	}
}
