package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

const testSecret = "01234567890123456789012345678901"

func TestAuthService_RegisterHashesPassword(t *testing.T) {
	var stored string
	svc := NewService(&mockAuthRepository{
		CreateUserFn: func(_ context.Context, name, email, passwordHash string) (*User, error) {
			stored = passwordHash
			return &User{ID: 1, Name: name, Email: email}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	got, err := svc.Register(context.Background(), &RegisterRequest{Name: "Ana Souza", Email: "user@example.com", Password: "password123"})
	if err != nil || got.ID != 1 || got.Name != "Ana Souza" || got.Email != "user@example.com" {
		t.Fatalf("Register() = %+v, %v", got, err)
	}
	if stored == "password123" || bcrypt.CompareHashAndPassword([]byte(stored), []byte("password123")) != nil {
		t.Fatal("password was not stored as a valid bcrypt hash")
	}
}

func TestAuthService_LoginInvalidCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) {
			return &User{ID: 1, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	_, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "wrongpass"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || err.Error() != "invalid credentials" {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestAuthService_LoginIssuesClaimsAndHashesRefreshToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	var storedHash string
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) {
			return &User{ID: 7, PasswordHash: string(hash)}, nil
		},
		CreateRefreshTokenFn: func(_ context.Context, _ int64, tokenHash string, _ time.Time) error {
			storedHash = tokenHash
			return nil
		},
	}, []byte(testSecret), 15*time.Minute, time.Hour)
	response, err := svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if storedHash != sha256Hex(response.RefreshToken) || storedHash == response.RefreshToken {
		t.Fatal("refresh token was not persisted as a SHA-256 hash")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(response.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil || !token.Valid || claims.Subject != "7" {
		t.Fatalf("invalid access token: valid=%v err=%v subject=%q", token.Valid, err, claims.Subject)
	}
	if claims.Issuer != JWTIssuer || len(claims.Audience) != 1 || claims.Audience[0] != JWTAudience {
		t.Fatalf("unexpected issuer/audience: %q %v", claims.Issuer, claims.Audience)
	}
}

func TestAuthService_NormalizesEmailForRegisterAndLogin(t *testing.T) {
	var registeredEmail, loginEmail string
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&mockAuthRepository{
		CreateUserFn: func(_ context.Context, _, email, _ string) (*User, error) {
			registeredEmail = email
			return &User{ID: 1, Email: email}, nil
		},
		GetUserByEmailFn: func(_ context.Context, email string) (*User, error) {
			loginEmail = email
			return &User{ID: 1, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	if _, err := svc.Register(context.Background(), &RegisterRequest{Name: "Ana Souza", Email: " User@Example.COM ", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), &LoginRequest{Email: " User@Example.COM ", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if registeredEmail != "user@example.com" || loginEmail != registeredEmail {
		t.Fatalf("emails = %q, %q", registeredEmail, loginEmail)
	}
}

func TestAuthService_ValidatePasswordProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.String().Draw(t, "password")
		err := validatePassword(password)
		if (len(password) < 8 || len(password) > 72) != (err != nil) {
			t.Fatalf("validation mismatch for length %d: %v", len(password), err)
		}
	})
}

func TestAuthService_LoginErrorMessageProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := withMinPasswordCost(NewService(&mockAuthRepository{
			GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
		}, []byte(testSecret), time.Minute, time.Hour))
		// Prefixes guarantee non-empty strings, so the early validation for
		// blank email/password is never triggered and the path that reaches
		// the repository (user not found) always returns "invalid credentials".
		_, err := svc.Login(context.Background(), &LoginRequest{
			Email:    "user@example-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "email"),
			Password: "wrong-" + rapid.StringMatching(`[a-z0-9]+`).Draw(t, "password"),
		})
		if err == nil || err.Error() != "invalid credentials" {
			t.Fatalf("error = %v", err)
		}
	})
}

func newTestAuthService(repo Repository) *Service {
	return NewService(repo, []byte(testSecret), 15*time.Minute, time.Hour)
}

// withMinPasswordCost lowers the bcrypt cost so tests that hash many times
// (e.g. property tests hitting the unknown-user path) stay fast.
func withMinPasswordCost(svc *Service) *Service {
	svc.passwordCost = bcrypt.MinCost
	return svc
}

func TestAuthService_Login_UnknownUserComparesDummyHash(t *testing.T) {
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
	}, []byte(testSecret), time.Minute, time.Hour)

	_, err := svc.Login(context.Background(), &LoginRequest{Email: "nobody@example.com", Password: "password123"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || err.Error() != "invalid credentials" {
		t.Fatalf("Login() error = %v", err)
	}
	// The dummy hash must cost the same as a stored hash, otherwise timing leaks existence.
	cost, err := bcrypt.Cost(svc.dummyPasswordHash())
	if err != nil {
		t.Fatalf("dummy hash is not a valid bcrypt hash: %v", err)
	}
	if cost != defaultPasswordCost {
		t.Fatalf("dummy hash cost = %d, want %d", cost, defaultPasswordCost)
	}
}

func TestRecordAuthError_RecordsOnlyUnexpectedErrors(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantEvents int
	}{
		"validation":          {apperrors.NewValidationError("email is invalid"), 0},
		"invalid credentials": {newInvalidCredentialsError(), 0},
		"wrapped conflict":    {fmt.Errorf("authService.Register: %w", apperrors.ErrConflict), 0},
		"unauthorized":        {apperrors.ErrUnauthorized, 0},
		"user not found":      {fmt.Errorf("user %w", apperrors.ErrNotFound), 0},
		"unexpected":          {errors.New("connection refused"), 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			_, span := provider.Tracer("test").Start(context.Background(), "op")

			if got := recordAuthError(span, tt.err); got != tt.err {
				t.Fatalf("recordAuthError() returned %v, want the same error", got)
			}
			span.End()

			ended := recorder.Ended()[0]
			if ended.Status().Code != codes.Error {
				t.Errorf("status = %v, want Error", ended.Status().Code)
			}
			if len(ended.Events()) != tt.wantEvents {
				t.Errorf("events = %d, want %d", len(ended.Events()), tt.wantEvents)
			}
		})
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		CreateUserFn: func(context.Context, string, string, string) (*User, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Register(context.Background(), &RegisterRequest{Name: "Ana Souza", Email: "user@example.com", Password: "password123"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("Register() error = %v, want ErrConflict", err)
	}
}

func TestAuthService_Register_InvalidInput(t *testing.T) {
	tests := map[string]struct {
		req     RegisterRequest
		wantMsg string
	}{
		"invalid email":  {RegisterRequest{Name: "Ana Souza", Email: "not-an-email", Password: "password123"}, "email is invalid"},
		"email too long": {RegisterRequest{Name: "Ana Souza", Email: strings.Repeat("a", 250) + "@x.io", Password: "password123"}, "email is invalid"},
		"short password": {RegisterRequest{Name: "Ana Souza", Email: "user@example.com", Password: "short"}, "password must be between 8 and 72 bytes"},
		"long password":  {RegisterRequest{Name: "Ana Souza", Email: "user@example.com", Password: strings.Repeat("a", 73)}, "password must be between 8 and 72 bytes"},
		"empty fields":   {RegisterRequest{}, "name must be between 2 and 60 characters"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			called := false
			svc := newTestAuthService(&mockAuthRepository{
				CreateUserFn: func(context.Context, string, string, string) (*User, error) {
					called = true
					return &User{ID: 1}, nil
				},
			})
			_, err := svc.Register(context.Background(), &tc.req)
			if !errors.Is(err, apperrors.ErrValidation) {
				t.Fatalf("Register() error = %v, want ErrValidation", err)
			}
			if err.Error() != tc.wantMsg {
				t.Fatalf("Register() error = %q, want %q", err.Error(), tc.wantMsg)
			}
			if called {
				t.Fatal("repository must not be called for invalid input")
			}
		})
	}
}

func TestAuthService_Refresh_ValidToken(t *testing.T) {
	const rawToken = "old-refresh-token"
	var rotatedOldID, rotatedUserID int64
	var newHash string
	var newExpiresAt time.Time
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(_ context.Context, tokenHash string) (*RefreshToken, error) {
			if tokenHash != sha256Hex(rawToken) {
				t.Fatalf("lookup hash = %q, want SHA-256 of raw token", tokenHash)
			}
			return &RefreshToken{ID: 10, UserID: 7, TokenHash: tokenHash, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		RotateRefreshTokenFn: func(_ context.Context, oldID, userID int64, hash string, expiresAt time.Time) error {
			rotatedOldID, rotatedUserID, newHash, newExpiresAt = oldID, userID, hash, expiresAt
			return nil
		},
	})

	response, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: rawToken})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if rotatedOldID != 10 || rotatedUserID != 7 {
		t.Fatalf("rotated oldID=%d userID=%d, want 10 and 7", rotatedOldID, rotatedUserID)
	}
	if response.RefreshToken == rawToken || newHash != sha256Hex(response.RefreshToken) {
		t.Fatal("new refresh token must differ from the old one and be persisted as its SHA-256 hash")
	}
	if time.Until(newExpiresAt) <= 0 || response.TokenType != "Bearer" {
		t.Fatalf("unexpected expiresAt=%v tokenType=%q", newExpiresAt, response.TokenType)
	}
	claims := &Claims{}
	if _, err := jwt.ParseWithClaims(response.AccessToken, claims, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	}); err != nil || claims.Subject != "7" {
		t.Fatalf("invalid access token: err=%v subject=%q", err, claims.Subject)
	}
}

func TestAuthService_Refresh_InvalidToken(t *testing.T) {
	tests := map[string]*RefreshToken{
		"not found": nil,
		"expired":   {ID: 1, UserID: 7, ExpiresAt: time.Now().Add(-time.Second)},
		"revoked":   {ID: 1, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), Revoked: true},
	}
	for name, stored := range tests {
		t.Run(name, func(t *testing.T) {
			rotated := false
			svc := newTestAuthService(&mockAuthRepository{
				GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) { return stored, nil },
				RotateRefreshTokenFn: func(context.Context, int64, int64, string, time.Time) error {
					rotated = true
					return nil
				},
			})
			_, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
			if !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Fatalf("Refresh() error = %v, want ErrUnauthorized", err)
			}
			if rotated {
				t.Fatal("token must not be rotated")
			}
		})
	}
}

func TestAuthService_Refresh_ConcurrentReuse(t *testing.T) {
	// The repository returns ErrUnauthorized when another request already
	// revoked the token between the lookup and the rotation.
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) {
			return &RefreshToken{ID: 1, UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		RotateRefreshTokenFn: func(context.Context, int64, int64, string, time.Time) error {
			return apperrors.ErrUnauthorized
		},
	})
	response, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
	if !errors.Is(err, apperrors.ErrUnauthorized) || response != nil {
		t.Fatalf("Refresh() = %+v, %v, want nil and ErrUnauthorized", response, err)
	}
}

func TestAuthService_Refresh_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := newTestAuthService(&mockAuthRepository{
		GetRefreshTokenFn: func(context.Context, string) (*RefreshToken, error) { return nil, dbErr },
	})
	_, err := svc.Refresh(context.Background(), &RefreshRequest{RefreshToken: "token"})
	if !errors.Is(err, dbErr) || errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("Refresh() error = %v, want wrapped repository error", err)
	}
}

func TestAuthService_Logout_RevokesTokenHash(t *testing.T) {
	var revokedHash string
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(_ context.Context, tokenHash string) error {
			revokedHash = tokenHash
			return nil
		},
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{RefreshToken: "raw-token"}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if revokedHash != sha256Hex("raw-token") {
		t.Fatalf("revoked hash = %q, want SHA-256 of raw token", revokedHash)
	}
}

func TestAuthService_Logout_MissingToken(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(context.Context, string) error {
			t.Fatal("repository must not be called")
			return nil
		},
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{}); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("Logout() error = %v, want ErrValidation", err)
	}
}

func TestAuthService_Logout_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := newTestAuthService(&mockAuthRepository{
		RevokeRefreshTokenFn: func(context.Context, string) error { return dbErr },
	})
	if err := svc.Logout(context.Background(), &RefreshRequest{RefreshToken: "token"}); !errors.Is(err, dbErr) {
		t.Fatalf("Logout() error = %v, want wrapped repository error", err)
	}
}

func TestService_AcquirePasswordSlot_LimitsConcurrency(t *testing.T) {
	svc := NewService(&mockAuthRepository{}, []byte(testSecret), time.Minute, time.Hour)
	svc.passwordSlots = make(chan struct{}, 1)

	release, err := svc.acquirePasswordSlot(context.Background())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// With the only slot taken, a done context must give up instead of waiting.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.acquirePasswordSlot(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire with slot taken = %v, want context.Canceled", err)
	}

	release()
	release2, err := svc.acquirePasswordSlot(context.Background())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release2()
}

func TestService_Login_ContextDoneWhileWaitingForPasswordSlot(t *testing.T) {
	svc := NewService(&mockAuthRepository{
		GetUserByEmailFn: func(context.Context, string) (*User, error) { return nil, nil },
	}, []byte(testSecret), time.Minute, time.Hour)
	svc.passwordSlots = make(chan struct{}, 1)
	svc.passwordSlots <- struct{}{} // every slot busy

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Login(ctx, &LoginRequest{Email: "user@example.com", Password: "password123"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Login() error = %v, want context.Canceled", err)
	}
	if errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("Login() must not report invalid credentials when it never compared the password")
	}
}

func TestService_Register_ContextDoneWhileWaitingForPasswordSlot(t *testing.T) {
	svc := NewService(&mockAuthRepository{
		CreateUserFn: func(context.Context, string, string, string) (*User, error) {
			t.Fatal("CreateUser must not be called without a password hash")
			return nil, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour)
	svc.passwordSlots = make(chan struct{}, 1)
	svc.passwordSlots <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Register(ctx, &RegisterRequest{Name: "Ana Souza", Email: "user@example.com", Password: "password123"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Register() error = %v, want context.Canceled", err)
	}
}

func TestService_Login_ConcurrentCallsReleaseSlots(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	svc := withMinPasswordCost(NewService(&mockAuthRepository{
		GetUserByEmailFn: func(_ context.Context, email string) (*User, error) {
			return &User{ID: 1, Email: email, PasswordHash: string(hash)}, nil
		},
	}, []byte(testSecret), time.Minute, time.Hour))
	svc.passwordSlots = make(chan struct{}, 2)

	const calls = 16
	errs := make(chan error, calls)
	for range calls {
		go func() {
			_, err := svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "password123"})
			errs <- err
		}()
	}
	for range calls {
		if err := <-errs; err != nil {
			t.Fatalf("Login() error = %v", err)
		}
	}
	if n := len(svc.passwordSlots); n != 0 {
		t.Fatalf("slots still held after all logins = %d, want 0", n)
	}
}

// recordingRegisterRepo returns a repository whose CreateUser records the
// arguments it receives and echoes them back as the stored user.
func recordingRegisterRepo(called *bool, gotName, gotEmail, gotHash *string) *mockAuthRepository {
	return &mockAuthRepository{
		CreateUserFn: func(_ context.Context, name, email, passwordHash string) (*User, error) {
			*called = true
			*gotName, *gotEmail, *gotHash = name, email, passwordHash
			return &User{ID: 1, Name: name, Email: email}, nil
		},
	}
}

func TestService_Register_NameRules(t *testing.T) {
	rejected := map[string]string{
		"empty":           "",
		"spaces":          "   ",
		"tab and newline": "\t\n",
		"one char":        "A",
		"one char padded": " A ",
		"61 ascii":        strings.Repeat("a", 61),
		"61 accented":     strings.Repeat("ç", 61),
	}
	for name, input := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			var called bool
			var gotName, gotEmail, gotHash string
			svc := withMinPasswordCost(newTestAuthService(recordingRegisterRepo(&called, &gotName, &gotEmail, &gotHash)))
			_, err := svc.Register(context.Background(), &RegisterRequest{Name: input, Email: "ana@exemplo.com", Password: "Senha123!"})
			if !errors.Is(err, apperrors.ErrValidation) {
				t.Fatalf("Register() error = %v, want ErrValidation", err)
			}
			if err.Error() != "name must be between 2 and 60 characters" {
				t.Fatalf("Register() error = %q", err.Error())
			}
			if called {
				t.Fatal("repository must not be called for an invalid name")
			}
		})
	}

	accepted := map[string]struct{ input, want string }{
		"two runes three bytes": {"Jô", "Jô"},
		"two ascii":             {"Ab", "Ab"},
		"60 ascii":              {strings.Repeat("a", 60), strings.Repeat("a", 60)},
		"60 accented":           {strings.Repeat("ç", 60), strings.Repeat("ç", 60)},
		"60 accented padded":    {"  " + strings.Repeat("ç", 60) + "  ", strings.Repeat("ç", 60)},
	}
	for name, tc := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			var called bool
			var gotName, gotEmail, gotHash string
			svc := withMinPasswordCost(newTestAuthService(recordingRegisterRepo(&called, &gotName, &gotEmail, &gotHash)))
			if _, err := svc.Register(context.Background(), &RegisterRequest{Name: tc.input, Email: "ana@exemplo.com", Password: "Senha123!"}); err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			if !called || gotName != tc.want {
				t.Fatalf("repository name = %q (called=%v), want %q", gotName, called, tc.want)
			}
		})
	}
}

func TestService_Register_TrimsName(t *testing.T) {
	var called bool
	var gotName, gotEmail, gotHash string
	svc := withMinPasswordCost(newTestAuthService(recordingRegisterRepo(&called, &gotName, &gotEmail, &gotHash)))
	got, err := svc.Register(context.Background(), &RegisterRequest{Name: "  Ana  ", Email: "ana@exemplo.com", Password: "Senha123!"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if gotName != "Ana" || got.Name != "Ana" {
		t.Fatalf("repository name = %q, response name = %q, want %q", gotName, got.Name, "Ana")
	}
}

func TestService_Register_KeepsNameCase(t *testing.T) {
	var called bool
	var gotName, gotEmail, gotHash string
	svc := withMinPasswordCost(newTestAuthService(recordingRegisterRepo(&called, &gotName, &gotEmail, &gotHash)))
	if _, err := svc.Register(context.Background(), &RegisterRequest{Name: "ana DE souza", Email: "ana@exemplo.com", Password: "Senha123!"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if gotName != "ana DE souza" {
		t.Fatalf("repository name = %q, want %q", gotName, "ana DE souza")
	}
}

func TestService_Register_ValidationOrder(t *testing.T) {
	tests := map[string]struct {
		req     RegisterRequest
		wantMsg string
	}{
		"name first":    {RegisterRequest{Name: "A", Email: "x", Password: "short"}, "name must be between 2 and 60 characters"},
		"then email":    {RegisterRequest{Name: "Ana Souza", Email: "x", Password: "short"}, "email is invalid"},
		"then password": {RegisterRequest{Name: "Ana Souza", Email: "ana@exemplo.com", Password: "short"}, "password must be between 8 and 72 bytes"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := newTestAuthService(&mockAuthRepository{
				CreateUserFn: func(context.Context, string, string, string) (*User, error) {
					t.Fatal("repository must not be called for invalid input")
					return nil, nil
				},
			})
			_, err := svc.Register(context.Background(), &tc.req)
			if !errors.Is(err, apperrors.ErrValidation) || err.Error() != tc.wantMsg {
				t.Fatalf("Register() error = %v, want %q", err, tc.wantMsg)
			}
		})
	}
}

func TestService_Register_ResponseHasName(t *testing.T) {
	svc := withMinPasswordCost(newTestAuthService(&mockAuthRepository{
		CreateUserFn: func(context.Context, string, string, string) (*User, error) {
			return &User{ID: 42, Name: "Ana Souza", Email: "ana@exemplo.com"}, nil
		},
	}))
	got, err := svc.Register(context.Background(), &RegisterRequest{Name: "Ana Souza", Email: "ana@exemplo.com", Password: "Senha123!"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	want := RegisterResponse{ID: 42, Name: "Ana Souza", Email: "ana@exemplo.com"}
	if *got != want {
		t.Fatalf("Register() = %+v, want %+v", *got, want)
	}
}

func TestService_Register_PassesArgumentsInOrder(t *testing.T) {
	var called bool
	var gotName, gotEmail, gotHash string
	svc := withMinPasswordCost(newTestAuthService(recordingRegisterRepo(&called, &gotName, &gotEmail, &gotHash)))
	if _, err := svc.Register(context.Background(), &RegisterRequest{Name: "Ana Souza", Email: " Ana@Exemplo.com ", Password: "Senha123!"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if gotName != "Ana Souza" || gotEmail != "ana@exemplo.com" {
		t.Fatalf("repository got name=%q email=%q", gotName, gotEmail)
	}
	if bcrypt.CompareHashAndPassword([]byte(gotHash), []byte("Senha123!")) != nil {
		t.Fatal("password hash argument is not a valid bcrypt hash of the password")
	}
}

func TestService_ValidateNameProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		name := strings.TrimSpace(rapid.String().Draw(t, "name"))
		n := utf8.RuneCountInString(name)
		err := validateName(name)
		if (n < minNameRunes || n > maxNameRunes) != (err != nil) {
			t.Fatalf("validation mismatch for %d runes: %v", n, err)
		}
	})
}

func TestService_Me_ReturnsUserFromContext(t *testing.T) {
	var gotID int64
	svc := newTestAuthService(&mockAuthRepository{
		GetUserByIDFn: func(_ context.Context, id int64) (*User, error) {
			gotID = id
			return &User{ID: 42, Name: "Ana Souza", Email: "ana@exemplo.com", PasswordHash: "hash"}, nil
		},
	})
	got, err := svc.Me(identity.WithUserID(context.Background(), 42))
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if gotID != 42 {
		t.Fatalf("repository id = %d, want the id from the context (42)", gotID)
	}
	want := UserResponse{ID: 42, Name: "Ana Souza", Email: "ana@exemplo.com"}
	if *got != want {
		t.Fatalf("Me() = %+v, want %+v", *got, want)
	}
}

func TestService_Me_NoUserInContext(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		GetUserByIDFn: func(context.Context, int64) (*User, error) {
			t.Fatal("repository must not be called without a user in the context")
			return nil, nil
		},
	})
	if _, err := svc.Me(context.Background()); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("Me() error = %v, want ErrUnauthorized", err)
	}
}

func TestService_Me_UserNotFound(t *testing.T) {
	svc := newTestAuthService(&mockAuthRepository{
		GetUserByIDFn: func(context.Context, int64) (*User, error) { return nil, nil },
	})
	_, err := svc.Me(identity.WithUserID(context.Background(), 42))
	if !errors.Is(err, apperrors.ErrNotFound) || err.Error() != "user not found" {
		t.Fatalf("Me() error = %v, want ErrNotFound with message %q", err, "user not found")
	}
}

func TestService_Me_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := newTestAuthService(&mockAuthRepository{
		GetUserByIDFn: func(context.Context, int64) (*User, error) { return nil, dbErr },
	})
	_, err := svc.Me(identity.WithUserID(context.Background(), 42))
	if !errors.Is(err, dbErr) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("Me() error = %v, want wrapped repository error", err)
	}
}
