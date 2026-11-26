package auth

import (
	"testing"
	"time"
)

func TestJWTIssueVerify(t *testing.T) {
	m := NewJWTManager([]byte("secret"), time.Hour, "test")
	tok, err := m.Issue("alice", []string{"admin"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "alice" || len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestJWTRejectsWrongSecret(t *testing.T) {
	a := NewJWTManager([]byte("secret"), time.Hour, "test")
	b := NewJWTManager([]byte("other"), time.Hour, "test")
	tok, err := a.Issue("alice", nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("expected verification to fail")
	}
}

func TestJWTRejectsExpired(t *testing.T) {
	m := NewJWTManager([]byte("secret"), time.Millisecond, "test")
	tok, err := m.Issue("alice", nil)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestGenerateAndHashKey(t *testing.T) {
	k1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	k2, _ := GenerateKey()
	if k1 == k2 {
		t.Fatal("keys must be unique")
	}
	if HashKey(k1) == HashKey(k2) {
		t.Fatal("different keys must hash differently")
	}
	if len(HashKey(k1)) != 64 {
		t.Fatalf("unexpected hash length %d", len(HashKey(k1)))
	}
	if HashKey(k1) == k1 {
		t.Fatal("hash must not equal the key")
	}
	if !ConstantTimeEqual(HashKey(k1), HashKey(k1)) {
		t.Fatal("equal hashes must compare equal")
	}
}
