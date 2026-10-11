package auth

import (
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecretPassword123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	if !CheckPassword(password, hash) {
		t.Fatalf("Password verification failed for correct password")
	}

	if CheckPassword("WrongPassword", hash) {
		t.Fatalf("Password verification succeeded for wrong password")
	}
}

func TestAdminJWT(t *testing.T) {
	username := "admin"
	secret := "test-secret-key-32-chars-long-123456"

	token, err := GenerateAdminToken(username, secret, 2)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	claims, err := ValidateAdminToken(token, secret)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if claims.Username != username {
		t.Fatalf("Expected username %s, got %s", username, claims.Username)
	}

	// Validate with wrong secret
	_, err = ValidateAdminToken(token, "wrong-secret")
	if err == nil {
		t.Fatalf("Validation should fail with wrong secret")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(3, 100*time.Millisecond)

	ip := "192.168.1.50"
	if !rl.Allow(ip) {
		t.Fatalf("Attempt 1 should be allowed")
	}
	if !rl.Allow(ip) {
		t.Fatalf("Attempt 2 should be allowed")
	}
	if !rl.Allow(ip) {
		t.Fatalf("Attempt 3 should be allowed")
	}
	if rl.Allow(ip) {
		t.Fatalf("Attempt 4 should be rejected")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow(ip) {
		t.Fatalf("Attempt should be allowed after window expiration")
	}
}
