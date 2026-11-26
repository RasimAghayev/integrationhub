// Package secret provides authenticated encryption for connection credentials
// at rest using AES-256-GCM.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrInvalidKey is returned when the configured key is not 32 bytes.
var ErrInvalidKey = errors.New("secret: key must be 32 bytes for AES-256")

// currentVersion is prefixed to every ciphertext so keys can be rotated
// (versioned blobs allow dual-key reads during rotation).
const currentVersion byte = 1

// Cipher encrypts and decrypts credential blobs with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a 32-byte key.
func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext and returns a base64 string of version||nonce||ciphertext.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	out := make([]byte, 0, 1+len(sealed))
	out = append(out, currentVersion)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt opens a base64 string produced by Encrypt.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secret: decode: %w", err)
	}
	if len(raw) < 1 {
		return nil, errors.New("secret: ciphertext too short")
	}
	if raw[0] != currentVersion {
		return nil, fmt.Errorf("secret: unsupported version %d", raw[0])
	}
	raw = raw[1:]
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("secret: ciphertext too short")
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("secret: open: %w", err)
	}
	return plaintext, nil
}
