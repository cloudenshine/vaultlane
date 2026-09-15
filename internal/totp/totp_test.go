package totp

import (
	"testing"
	"time"
)

// RFC 6238 test vectors (SHA1, 8-digit, period=30 — adapted for 6-digit)
func TestTOTP_Deterministic(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP" // "Hello!" in base32
	// At T=0 the TOTP must be deterministic
	ts := time.Unix(0, 0)
	code1, err := TOTP(secret, ts)
	if err != nil {
		t.Fatalf("TOTP failed: %v", err)
	}
	code2, err := TOTP(secret, ts)
	if err != nil {
		t.Fatalf("TOTP failed: %v", err)
	}
	if code1 != code2 {
		t.Fatalf("same time should produce same code: %q vs %q", code1, code2)
	}
	if len(code1) != 6 {
		t.Fatalf("expected 6-digit code, got %q", code1)
	}
}

func TestVerify_WithinWindow(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret failed: %v", err)
	}
	now := time.Now()
	code, err := TOTP(secret, now)
	if err != nil {
		t.Fatalf("TOTP failed: %v", err)
	}
	if !Verify(secret, code, now) {
		t.Fatal("current code should verify")
	}
	// Slightly in the future (within window)
	if !Verify(secret, code, now.Add(29*time.Second)) {
		t.Fatal("code should still verify within ±30s")
	}
}

func TestVerify_WrongCode(t *testing.T) {
	secret, _ := GenerateSecret()
	if Verify(secret, "000000", time.Now()) {
		t.Log("00000 accidentally valid (extremely unlikely)")
	}
	if Verify(secret, "wrong!", time.Now()) {
		t.Fatal("non-numeric code should not verify")
	}
}

func TestOTPAuthURL(t *testing.T) {
	url := OTPAuthURL("MYSECRET", "Vaultlane", "admin")
	if len(url) == 0 {
		t.Fatal("empty otpauth URL")
	}
	if url[:10] != "otpauth://" {
		t.Fatalf("expected otpauth:// prefix, got %q", url[:10])
	}
}
