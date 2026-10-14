package auth

import (
	"testing"
	"time"
)

func TestHS256TokenValidation(t *testing.T) {
	cfg := JWTConfig{Issuer: "office-ai-dev", Audience: "office-ai-api", Key: []byte("01234567890123456789012345678901"), TTL: time.Hour}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	token, err := SignHS256("dev:admin@dev.local", cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := VerifyHS256(token, cfg, now.Add(time.Minute)); err != nil || got != "dev:admin@dev.local" {
		t.Fatalf("VerifyHS256() = %q, %v", got, err)
	}
	tampered := token[:len(token)-1] + "A"
	if token[len(token)-1] == 'A' {
		tampered = token[:len(token)-1] + "B"
	}
	for _, bad := range []string{token + "x", tampered} {
		if _, err := VerifyHS256(bad, cfg, now.Add(time.Minute)); err == nil {
			t.Errorf("VerifyHS256 accepted tampered token")
		}
	}
	if _, err := VerifyHS256(token, cfg, now.Add(2*time.Hour)); err == nil {
		t.Error("VerifyHS256 accepted expired token")
	}
}
