package auth

import (
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "superSecret123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !VerifyPassword(hash, password) {
		t.Errorf("VerifyPassword returned false for correct password")
	}

	if VerifyPassword(hash, "wrongPassword") {
		t.Errorf("VerifyPassword returned true for wrong password")
	}
}

func TestSignAndVerifySession(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	now := time.Now()

	token := SignSession(secret, now)
	if token == "" {
		t.Fatalf("SignSession returned empty token")
	}

	valid, err := VerifySession(secret, token, now.Add(1*time.Hour))
	if err != nil || !valid {
		t.Errorf("VerifySession expected valid token, got valid=%v, err=%v", valid, err)
	}

	// Verify tampered token fails
	tampered := token + "tampered"
	valid, _ = VerifySession(secret, tampered, now.Add(1*time.Hour))
	if valid {
		t.Errorf("VerifySession expected false for tampered token")
	}

	// Verify expired token fails (older than 30 days)
	valid, _ = VerifySession(secret, token, now.Add(31*24*time.Hour))
	if valid {
		t.Errorf("VerifySession expected false for expired token")
	}
}
