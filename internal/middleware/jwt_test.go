package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"pgregory.net/rapid"
)

const jwtTestSecret = "01234567890123456789012345678901"

func TestJWTRejectsMissingToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	JWT([]byte(jwtTestSecret))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next was called")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestJWTRejectsInvalidTokensProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+rapid.String().Draw(t, "token"))
		recorder := httptest.NewRecorder()
		JWT([]byte(jwtTestSecret))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next was called for invalid token")
		})).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})
}

func signedToken(t *testing.T, secret []byte, subject string, expires time.Time) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: subject, ExpiresAt: jwt.NewNumericDate(expires)},
	}).SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestJWTInjectsUserID(t *testing.T) {
	raw := signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	recorder := httptest.NewRecorder()
	JWT([]byte(jwtTestSecret))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.UserIDFromContext(r.Context())
		if !ok || id != 42 {
			t.Fatalf("context user ID = %d, %t", id, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestJWTRejectsExpiredAndWrongSignature(t *testing.T) {
	tests := []struct {
		name, token string
	}{
		{"expired", signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(-time.Minute))},
		{"wrong signature", signedToken(t, []byte("wrong-secret-012345678901234567890"), "42", time.Now().Add(time.Minute))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			recorder := httptest.NewRecorder()
			JWT([]byte(jwtTestSecret))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next was called")
			})).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
		})
	}
}
