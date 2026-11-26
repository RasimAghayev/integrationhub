package secret

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	c, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plaintext := []byte(`{"token":"abc","refresh":"def"}`)

	enc, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "" {
		t.Fatal("Encrypt returned empty string")
	}

	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	c, _ := New(key)
	enc, _ := c.Encrypt([]byte("secret"))
	// Flip a character in the base64 payload.
	tampered := enc[:len(enc)-2] + "AA"
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}
