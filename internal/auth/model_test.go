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

func TestClaims_SessionVersionJSON(t *testing.T) {
	encoded, err := json.Marshal(Claims{SessionVersion: 3})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"sv":3`) {
		t.Errorf("serialized Claims = %s, want it to contain \"sv\":3", encoded)
	}

	var claims Claims
	if err := json.Unmarshal([]byte(`{"sub":"7"}`), &claims); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if claims.SessionVersion != 0 || claims.Subject != "7" {
		t.Errorf("claims without sv: SessionVersion = %d, Subject = %q; want 0 and 7", claims.SessionVersion, claims.Subject)
	}
}

func TestTheme_IsValid(t *testing.T) {
	tests := map[Theme]bool{
		ThemeLight: true,
		ThemeDark:  true,
		"":         false,
		"light":    false,
		"dark":     false,
		"AUTO":     false,
	}
	for theme, want := range tests {
		if got := theme.IsValid(); got != want {
			t.Errorf("Theme(%q).IsValid() = %v, want %v", theme, got, want)
		}
	}
}

func TestUserResponse_ThemeNullJSON(t *testing.T) {
	encoded, err := json.Marshal(UserResponse{ID: 7, Name: "Jonas Silva", Email: "jonas@example.com"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"theme":null`) {
		t.Errorf("serialized UserResponse = %s, want \"theme\":null", encoded)
	}

	dark := ThemeDark
	encoded, err = json.Marshal(UserResponse{ID: 7, Theme: &dark})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"theme":"DARK"`) {
		t.Errorf("serialized UserResponse = %s, want \"theme\":\"DARK\"", encoded)
	}
}
