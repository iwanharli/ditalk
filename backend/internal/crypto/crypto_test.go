package crypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
}

func newCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New(testKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := newCipher(t)

	for _, plaintext := range []string{
		"halo, gapapa kok 😊",
		"",
		strings.Repeat("panjang ", 1000),
		"multi\nbaris\ttab",
	} {
		payload, err := c.EncryptString(plaintext)
		if err != nil {
			t.Fatalf("EncryptString(%q): %v", plaintext, err)
		}

		got, err := c.DecryptString(payload)
		if err != nil {
			t.Fatalf("DecryptString: %v", err)
		}
		if got != plaintext {
			t.Errorf("round trip = %q, want %q", got, plaintext)
		}
	}
}

func TestEmptyStringStoresNil(t *testing.T) {
	c := newCipher(t)

	payload, err := c.EncryptString("")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	// An empty message must not occupy a ciphertext blob in the database.
	if payload != nil {
		t.Errorf("payload = %v, want nil for empty input", payload)
	}
}

func TestNonceIsFreshPerCall(t *testing.T) {
	c := newCipher(t)

	a, err := c.EncryptString("pesan sama")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := c.EncryptString("pesan sama")
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	// Identical plaintext must not produce identical ciphertext, otherwise an
	// observer could tell that two messages have the same content.
	if bytes.Equal(a, b) {
		t.Error("ciphertext repeated for the same plaintext; nonce is not fresh")
	}
}

func TestTamperedCiphertextIsRejected(t *testing.T) {
	c := newCipher(t)

	payload, err := c.EncryptString("saldo 1000")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	tampered := bytes.Clone(payload)
	tampered[len(tampered)-1] ^= 0xff

	if _, err := c.Decrypt(tampered); err == nil {
		t.Error("tampered ciphertext decrypted without error; GCM auth not enforced")
	}
}

func TestWrongKeyCannotDecrypt(t *testing.T) {
	c := newCipher(t)

	payload, err := c.EncryptString("rahasia")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	other, err := New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := other.Decrypt(payload); err == nil {
		t.Error("decrypted with the wrong key")
	}
}

func TestShortCiphertextRejected(t *testing.T) {
	c := newCipher(t)

	if _, err := c.Decrypt([]byte{1, 2, 3}); !errors.Is(err, ErrCiphertext) {
		t.Errorf("err = %v, want ErrCiphertext", err)
	}
}

func TestKeyValidation(t *testing.T) {
	if _, err := New(""); !errors.Is(err, ErrKeyRequired) {
		t.Errorf("empty key err = %v, want ErrKeyRequired", err)
	}

	short := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))
	if _, err := New(short); !errors.Is(err, ErrKeySize) {
		t.Errorf("16-byte key err = %v, want ErrKeySize", err)
	}

	if _, err := New("not base64!!"); err == nil {
		t.Error("invalid base64 accepted")
	}
}

func TestHashIsStableAndKeyed(t *testing.T) {
	c := newCipher(t)

	a := c.Hash("6281234567890@s.whatsapp.net")
	b := c.Hash("6281234567890@s.whatsapp.net")
	if a != b {
		t.Error("hash is not stable for the same input")
	}
	if a == c.Hash("6289999999999@s.whatsapp.net") {
		t.Error("different identifiers produced the same hash")
	}

	other, err := New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// A keyed hash means an attacker cannot precompute a rainbow table of
	// phone numbers without also holding the key.
	if a == other.Hash("6281234567890@s.whatsapp.net") {
		t.Error("hash is not keyed; same value across different keys")
	}
}

func TestNilPassthrough(t *testing.T) {
	c := newCipher(t)

	got, err := c.Encrypt(nil)
	if err != nil || got != nil {
		t.Errorf("Encrypt(nil) = %v, %v; want nil, nil", got, err)
	}
	dec, err := c.Decrypt(nil)
	if err != nil || dec != nil {
		t.Errorf("Decrypt(nil) = %v, %v; want nil, nil", dec, err)
	}
}
