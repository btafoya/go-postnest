package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	s := NewService(nil, 1, 64*1024, 4, "test-session-key")

	hash, err := s.hashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("hashPassword returned empty string")
	}

	if !s.verifyPassword("correct-horse-battery-staple", hash) {
		t.Error("verifyPassword should succeed for correct password")
	}
	if s.verifyPassword("wrong-password", hash) {
		t.Error("verifyPassword should fail for wrong password")
	}
}

func TestHashPassword(t *testing.T) {
	s := NewService(nil, 1, 64*1024, 4, "test-session-key")

	hash, err := s.HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if !strings.Contains(hash, "$") {
		t.Errorf("HashPassword returned %q, expected '$' separator", hash)
	}

	emptyHash, err := s.HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(\"\") failed: %v", err)
	}
	if emptyHash == "" {
		t.Fatal("HashPassword(\"\") returned empty string")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	s := NewService(nil, 1, 64*1024, 4, "test-session-key")

	if s.verifyPassword("any", "nope") {
		t.Error("verifyPassword should fail for malformed hash")
	}
	if s.verifyPassword("any", "") {
		t.Error("verifyPassword should fail for empty hash")
	}
}
