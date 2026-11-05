package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"pgregory.net/rapid"
)

const jwtTestSecret = "01234567890123456789012345678901"

func TestJWTRejectsMissingToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	JWT([]byte(jwtTestSecret), &stubSessions{found: true})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
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
		JWT([]byte(jwtTestSecret), &stubSessions{found: true})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next was called for invalid token")
		})).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})
}

// stubSessions is the session-version test double: it counts the lookups and keeps the last user id.
type stubSessions struct {
	version    int64
	found      bool
	err        error
	calls      atomic.Int64
	lastUserID atomic.Int64
}

func (s *stubSessions) SessionVersion(_ context.Context, userID int64) (int64, bool, error) {
	s.calls.Add(1)
	s.lastUserID.Store(userID)
	return s.version, s.found, s.err
}

// signedToken signs a token without the "sv" claim, as issued before feature 006.
func signedToken(t *testing.T, secret []byte, subject string, expires time.Time) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    auth.JWTIssuer,
		Audience:  jwt.ClaimStrings{auth.JWTAudience},
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(expires),
	}).SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// signedTokenWithVersion signs a token carrying the "sv" claim.
func signedTokenWithVersion(t *testing.T, secret []byte, subject string, expires time.Time, sv int64) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    auth.JWTIssuer,
			Audience:  jwt.ClaimStrings{auth.JWTAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
		SessionVersion: sv,
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
	JWT([]byte(jwtTestSecret), &stubSessions{found: true})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := identity.UserIDFromContext(r.Context())
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
			JWT([]byte(jwtTestSecret), &stubSessions{found: true})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next was called")
			})).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
		})
	}
}

func TestJWTRejectsMissingClaimsAndWrongAlgorithm(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name: "missing expiration",
			token: mustSignClaims(t, jwt.SigningMethodHS256, auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
				Subject:  "42",
				Issuer:   auth.JWTIssuer,
				Audience: jwt.ClaimStrings{auth.JWTAudience},
				IssuedAt: jwt.NewNumericDate(time.Now()),
			}}, []byte(jwtTestSecret)),
		},
		{
			name: "missing subject",
			token: mustSignClaims(t, jwt.SigningMethodHS256, auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    auth.JWTIssuer,
				Audience:  jwt.ClaimStrings{auth.JWTAudience},
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			}}, []byte(jwtTestSecret)),
		},
		{
			name: "wrong algorithm",
			token: mustSignClaims(t, jwt.SigningMethodHS512, auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "42",
				Issuer:    auth.JWTIssuer,
				Audience:  jwt.ClaimStrings{auth.JWTAudience},
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			}}, []byte(jwtTestSecret)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			recorder := httptest.NewRecorder()
			JWT([]byte(jwtTestSecret), &stubSessions{found: true})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next was called")
			})).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
		})
	}
}

func mustSignClaims(t *testing.T, method jwt.SigningMethod, claims auth.Claims, secret []byte) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// serveWithSessions runs the JWT middleware with the given stub and reports whether next was called.
func serveWithSessions(t *testing.T, sessions *stubSessions, rawToken string) (*httptest.ResponseRecorder, bool, int64) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+rawToken)
	recorder := httptest.NewRecorder()
	nextCalled := false
	var contextUserID int64
	JWT([]byte(jwtTestSecret), sessions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		contextUserID, _ = identity.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
	return recorder, nextCalled, contextUserID
}

func assertUnauthorizedInvalidToken(t *testing.T, recorder *httptest.ResponseRecorder, nextCalled bool) {
	t.Helper()
	if nextCalled {
		t.Fatal("next was called")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"message":"invalid authentication token"`) {
		t.Fatalf("body = %s, want the invalid authentication token message", body)
	}
}

func TestJWT_SessionVersionMatches(t *testing.T) {
	sessions := &stubSessions{version: 3, found: true}
	raw := signedTokenWithVersion(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute), 3)

	recorder, nextCalled, userID := serveWithSessions(t, sessions, raw)
	if !nextCalled || recorder.Code != http.StatusNoContent || userID != 42 {
		t.Fatalf("next=%v status=%d userID=%d, want next with user 42", nextCalled, recorder.Code, userID)
	}
	if got := sessions.lastUserID.Load(); got != 42 {
		t.Fatalf("session lookup userID = %d, want 42", got)
	}
}

func TestJWT_SessionVersionDiffers(t *testing.T) {
	sessions := &stubSessions{version: 4, found: true}
	raw := signedTokenWithVersion(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute), 3)

	recorder, nextCalled, _ := serveWithSessions(t, sessions, raw)
	assertUnauthorizedInvalidToken(t, recorder, nextCalled)
}

func TestJWT_UserNotFound(t *testing.T) {
	sessions := &stubSessions{version: 0, found: false}
	raw := signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute))

	recorder, nextCalled, _ := serveWithSessions(t, sessions, raw)
	assertUnauthorizedInvalidToken(t, recorder, nextCalled)
}

func TestJWT_SessionLookupError(t *testing.T) {
	sessions := &stubSessions{err: errors.New("connection refused")}
	raw := signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute))

	recorder, nextCalled, _ := serveWithSessions(t, sessions, raw)
	if nextCalled {
		t.Fatal("next was called")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"code":"INTERNAL_SERVER_ERROR"`) || strings.Contains(body, "connection refused") {
		t.Fatalf("body = %s, want INTERNAL_SERVER_ERROR without internal details", body)
	}
}

func TestJWT_TokenWithoutSVCountsAsZero(t *testing.T) {
	sessions := &stubSessions{version: 0, found: true}
	raw := signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute))

	recorder, nextCalled, _ := serveWithSessions(t, sessions, raw)
	if !nextCalled || recorder.Code != http.StatusNoContent {
		t.Fatalf("next=%v status=%d, want a token without sv accepted as version 0", nextCalled, recorder.Code)
	}
}

func TestJWT_InvalidTokenDoesNotQuerySessions(t *testing.T) {
	sessions := &stubSessions{found: true}
	raw := signedToken(t, []byte(jwtTestSecret), "42", time.Now().Add(-time.Minute))

	recorder, nextCalled, _ := serveWithSessions(t, sessions, raw)
	assertUnauthorizedInvalidToken(t, recorder, nextCalled)
	if calls := sessions.calls.Load(); calls != 0 {
		t.Fatalf("session lookups = %d, want 0 for an invalid token", calls)
	}
}

func TestJWT_ConcurrentRequests(t *testing.T) {
	sessions := &stubSessions{version: 2, found: true}
	handler := JWT([]byte(jwtTestSecret), sessions)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	raw := signedTokenWithVersion(t, []byte(jwtTestSecret), "42", time.Now().Add(time.Minute), 2)

	const workers = 50
	statuses := make([]int, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+raw)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			statuses[i] = recorder.Code
		}()
	}
	wg.Wait()

	for i, status := range statuses {
		if status != http.StatusNoContent {
			t.Fatalf("request %d status = %d, want 204", i, status)
		}
	}
	if calls := sessions.calls.Load(); calls != workers {
		t.Fatalf("session lookups = %d, want %d", calls, workers)
	}
}
