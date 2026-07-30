// Package crypto provides field-level encryption for chat text, display names,
// and session credentials (doc bab 18.1). The key must come from KMS/Vault in
// production and never be stored alongside the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrKeySize     = errors.New("encryption key must be 32 bytes")
	ErrCiphertext  = errors.New("ciphertext too short")
	ErrKeyRequired = errors.New("encryption key not configured")
)

type Cipher struct {
	aead cipher.AEAD
	// hashKey keeps identifier hashing deterministic yet unguessable, so the
	// same JID always maps to the same lookup value without storing it raw.
	hashKey []byte
}

// New builds a Cipher from a base64-encoded 32-byte key.
func New(base64Key string) (*Cipher, error) {
	if base64Key == "" {
		return nil, ErrKeyRequired
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, ErrKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	// Derive a separate hashing key so the encryption key is never used for two
	// different purposes.
	sum := sha256.Sum256(append([]byte("ditalk-hash-v1"), key...))

	return &Cipher{aead: aead, hashKey: sum[:]}, nil
}

// Encrypt returns nonce||ciphertext. A fresh random nonce per call is required
// for GCM safety.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	if plaintext == nil {
		return nil, nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *Cipher) Decrypt(payload []byte) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}

	n := c.aead.NonceSize()
	if len(payload) < n {
		return nil, ErrCiphertext
	}

	plaintext, err := c.aead.Open(nil, payload[:n], payload[n:], nil)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return plaintext, nil
}

func (c *Cipher) EncryptString(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return c.Encrypt([]byte(s))
}

func (c *Cipher) DecryptString(payload []byte) (string, error) {
	b, err := c.Decrypt(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Hash produces a stable lookup value for identifiers such as JIDs and emails.
func (c *Cipher) Hash(value string) string {
	mac := hmac.New(sha256.New, c.hashKey)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
