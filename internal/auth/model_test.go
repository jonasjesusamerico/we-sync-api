package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestUser_PasswordHashIsNotSerialized(t *testing.T) {
	user := User{
		ID:           1,
		Email:        "user@example.com",
		PasswordHash: "sensitive-hash",
		CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	encoded, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), user.PasswordHash) {
		t.Errorf("serialized User contains password hash: %s", encoded)
	}
}

func TestClaims_EmbedsRegisteredClaims(t *testing.T) {
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42"}}
	if claims.Subject != "42" {
		t.Errorf("Claims.Subject = %q, want 42", claims.Subject)
	}
}
