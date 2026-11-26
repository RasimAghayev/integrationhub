package webhook

import (
	"errors"
	"testing"
	"time"
)

func TestVerifyOK(t *testing.T) {
	secret := "s3cr3t"
	now := time.Unix(1767268800, 0)
	body := []byte(`{"hello":"world"}`)
	header := Sign(secret, now, body)

	if err := Verify([]string{secret}, header, body, now, 5*time.Minute); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyTamperedBody(t *testing.T) {
	secret := "s3cr3t"
	now := time.Unix(1767268800, 0)
	header := Sign(secret, now, []byte(`{"a":1}`))

	if err := Verify([]string{secret}, header, []byte(`{"a":2}`), now, 5*time.Minute); !errors.Is(err, ErrSignature) {
		t.Fatalf("expected ErrSignature, got %v", err)
	}
}

func TestVerifyRotation(t *testing.T) {
	now := time.Unix(1767268800, 0)
	body := []byte("x")
	header := Sign("new-secret", now, body)

	if err := Verify([]string{"old-secret", "new-secret"}, header, body, now, 5*time.Minute); err != nil {
		t.Fatalf("rotation Verify: %v", err)
	}
}

func TestVerifyStale(t *testing.T) {
	secret := "s3cr3t"
	sent := time.Unix(1767268800, 0)
	body := []byte("x")
	header := Sign(secret, sent, body)

	now := sent.Add(10 * time.Minute)
	if err := Verify([]string{secret}, header, body, now, 5*time.Minute); !errors.Is(err, ErrStale) {
		t.Fatalf("expected ErrStale, got %v", err)
	}
}

func TestVerifyMissing(t *testing.T) {
	if err := Verify([]string{"s"}, "", []byte("x"), time.Now(), time.Minute); !errors.Is(err, ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}
}

func TestReplayCache(t *testing.T) {
	c := NewReplayCache(time.Minute)
	now := time.Unix(1000, 0)
	if c.Seen("evt-1", now) {
		t.Fatal("first Seen should be false")
	}
	if !c.Seen("evt-1", now) {
		t.Fatal("second Seen should be true")
	}
}
