package configs

import (
	"strings"
	"testing"
	"time"
)

func TestLoadProperties_JWTSecret(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		wantError string
	}{
		{
			name:      "missing",
			secret:    "",
			wantError: "JWT_SECRET",
		},
		{
			name:      "too short",
			secret:    "too-short",
			wantError: "pelo menos 32 bytes",
		},
		{
			name:      "insecure development default",
			secret:    "local-development-jwt-secret-change-before-production-2026",
			wantError: "segredo de desenvolvimento padrão",
		},
		{
			name:   "valid",
			secret: strings.Repeat("a", 32),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("JWT_SECRET", tt.secret)

			config, err := LoadProperties()
			if tt.wantError != "" {
				if err == nil {
					t.Fatal("LoadProperties() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("LoadProperties() error = %q, want it to contain %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadProperties() error = %v", err)
			}
			if config.JWT.Secret != tt.secret {
				t.Errorf("JWT.Secret = %q, want %q", config.JWT.Secret, tt.secret)
			}
		})
	}
}

func TestLoadProperties_JWTDurations(t *testing.T) {
	tests := []struct {
		name         string
		accessValue  string
		refreshValue string
		wantAccess   time.Duration
		wantRefresh  time.Duration
	}{
		{
			name:        "defaults",
			wantAccess:  15 * time.Minute,
			wantRefresh: 168 * time.Hour,
		},
		{
			name:         "configured values",
			accessValue:  "30m",
			refreshValue: "72h",
			wantAccess:   30 * time.Minute,
			wantRefresh:  72 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
			t.Setenv("JWT_ACCESS_TOKEN_DURATION", tt.accessValue)
			t.Setenv("JWT_REFRESH_TOKEN_DURATION", tt.refreshValue)

			config, err := LoadProperties()
			if err != nil {
				t.Fatalf("LoadProperties() error = %v", err)
			}
			if config.JWT.AccessTokenDuration != tt.wantAccess {
				t.Errorf("AccessTokenDuration = %s, want %s", config.JWT.AccessTokenDuration, tt.wantAccess)
			}
			if config.JWT.RefreshTokenDuration != tt.wantRefresh {
				t.Errorf("RefreshTokenDuration = %s, want %s", config.JWT.RefreshTokenDuration, tt.wantRefresh)
			}
		})
	}
}

func TestLoadProperties_RejectsInvalidJWTDurations(t *testing.T) {
	for _, value := range []string{"0s", "-1m", "25h"} {
		t.Run("access_"+value, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
			t.Setenv("JWT_ACCESS_TOKEN_DURATION", value)
			if _, err := LoadProperties(); err == nil {
				t.Fatal("LoadProperties() error = nil, want invalid access duration error")
			}
		})
	}
	for _, value := range []string{"0s", "-1m", "721h"} {
		t.Run("refresh_"+value, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("JWT_SECRET", strings.Repeat("a", 32))
			t.Setenv("JWT_REFRESH_TOKEN_DURATION", value)
			if _, err := LoadProperties(); err == nil {
				t.Fatal("LoadProperties() error = nil, want invalid refresh duration error")
			}
		})
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"DATABASE_WRITE_DSN":                "postgres://write",
		"DATABASE_WRITE_MAX_OPEN_CONNS":     "5",
		"DATABASE_WRITE_MAX_IDLE_CONNS":     "2",
		"DATABASE_WRITE_CONN_MAX_LIFETIME":  "30",
		"DATABASE_WRITE_CONN_MAX_IDLE_TIME": "5",
		"DATABASE_READ_DSN":                 "postgres://read",
		"DATABASE_READ_MAX_OPEN_CONNS":      "5",
		"DATABASE_READ_MAX_IDLE_CONNS":      "2",
		"DATABASE_READ_CONN_MAX_LIFETIME":   "30",
		"DATABASE_READ_CONN_MAX_IDLE_TIME":  "5",
	} {
		t.Setenv(key, value)
	}
}
