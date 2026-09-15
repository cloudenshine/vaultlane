package crypto

import (
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	c := NewFromSecret("test-secret-12345")
	plain := "card-content-abc-xyz-001"

	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if enc == plain {
		t.Fatal("encrypted text should not equal plaintext")
	}
	if !IsEncrypted(enc) {
		t.Fatal("expected ENC: prefix")
	}

	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec != plain {
		t.Fatalf("decrypted %q != original %q", dec, plain)
	}
}

func TestDecrypt_LegacyPlaintext(t *testing.T) {
	c := NewFromSecret("test-secret-12345")
	plain := "old-card-not-encrypted"
	dec, err := c.Decrypt(plain)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec != plain {
		t.Fatalf("expected legacy passthrough, got %q", dec)
	}
}

func TestEncryptDecrypt_EmptyKey(t *testing.T) {
	c := NewFromSecret("")
	plain := "card-test"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if enc != plain {
		t.Fatal("empty key should passthrough")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec != plain {
		t.Fatalf("expected %q, got %q", plain, dec)
	}
}

func TestEncrypt_NonceUniqueness(t *testing.T) {
	c := NewFromSecret("another-secret")
	plain := "same-card-content"
	enc1, _ := c.Encrypt(plain)
	enc2, _ := c.Encrypt(plain)
	if enc1 == enc2 {
		t.Fatal("two encryptions of same plaintext should differ (random nonce)")
	}
}
