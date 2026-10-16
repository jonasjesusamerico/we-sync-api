package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

const authTracerName = "we-sync-api/auth"

// defaultPasswordCost is the bcrypt cost used for stored password hashes.
const defaultPasswordCost = 12

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Service implements the authentication business rules.
type Service struct {
	repo         Repository
	jwtSecret    []byte
	accessTTL    time.Duration
	refreshTTL   time.Duration
	passwordCost int

	// dummyPasswordHash is compared against when the e-mail is unknown, so a
	// failed login always costs one bcrypt comparison and response time does not
	// reveal whether an account exists. Computed lazily, once per service.
	dummyPasswordHash func() []byte
}

// NewService creates a Service using the supplied JWT settings.
func NewService(repo Repository, jwtSecret []byte, accessTTL, refreshTTL time.Duration) *Service {
	s := &Service{
		repo:         repo,
		jwtSecret:    jwtSecret,
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
		passwordCost: defaultPasswordCost,
	}
	s.dummyPasswordHash = sync.OnceValue(func() []byte {
		hash, err := bcrypt.GenerateFromPassword([]byte("we-sync-api-dummy-password"), s.passwordCost)
		if err != nil {
			// Only fails for an invalid cost or a password over 72 bytes, both constants here.
			panic(fmt.Sprintf("authService: generate dummy password hash: %v", err))
		}
		return hash
	})
	return s
}

// Register validates and creates a user with a bcrypt password hash.
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	ctx, span := otel.Tracer(authTracerName).Start(ctx, "authService.Register")
	defer span.End()

	if req == nil {
		return nil, recordAuthError(span, apperrors.NewValidationError("request is required"))
	}
	email := normalizeEmail(req.Email)
	if err := validateEmail(email); err != nil {
		return nil, recordAuthError(span, err)
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, recordAuthError(span, err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.passwordCost)
	if err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Register hash password: %w", err))
	}

	user, err := s.repo.CreateUser(ctx, email, string(passwordHash))
	if err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Register: %w", err))
	}
	if user == nil {
		err := errors.New("authService.Register: repository returned nil user")
		return nil, recordAuthError(span, err)
	}

	span.SetAttributes(
		attribute.Int64("user.id", user.ID),
		attribute.String("auth.method", "register"),
	)
	return &RegisterResponse{ID: user.ID, Email: user.Email}, nil
}

// Login validates credentials and returns a JWT access token and refresh token.
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*TokenResponse, error) {
	ctx, span := otel.Tracer(authTracerName).Start(ctx, "authService.Login")
	defer span.End()

	if req == nil || req.Email == "" || req.Password == "" {
		return nil, recordAuthError(span, apperrors.NewValidationError("email and password are required"))
	}

	user, err := s.repo.GetUserByEmail(ctx, normalizeEmail(req.Email))
	if err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Login: %w", err))
	}
	hash := s.dummyPasswordHash()
	if user != nil {
		hash = []byte(user.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(req.Password))
	if user == nil || passwordErr != nil {
		logInvalidLogin(ctx)
		return nil, recordAuthError(span, newInvalidCredentialsError())
	}

	response, err := s.issueTokens(ctx, user.ID)
	if err != nil {
		return nil, recordAuthError(span, err)
	}
	span.SetAttributes(
		attribute.Int64("user.id", user.ID),
		attribute.String("auth.method", "login"),
	)
	return response, nil
}

// Refresh rotates a valid refresh token and returns a new token pair.
func (s *Service) Refresh(ctx context.Context, req *RefreshRequest) (*TokenResponse, error) {
	ctx, span := otel.Tracer(authTracerName).Start(ctx, "authService.Refresh")
	defer span.End()

	if req == nil || req.RefreshToken == "" {
		return nil, recordAuthError(span, apperrors.NewValidationError("refresh_token is required"))
	}

	token, err := s.repo.GetRefreshToken(ctx, sha256Hex(req.RefreshToken))
	if err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Refresh: %w", err))
	}
	if token == nil || token.Revoked || !token.ExpiresAt.After(time.Now()) {
		return nil, recordAuthError(span, newInvalidCredentialsError())
	}

	accessToken, err := s.issueAccessToken(token.UserID)
	if err != nil {
		return nil, recordAuthError(span, err)
	}
	rawRefreshToken, err := newRefreshToken()
	if err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Refresh generate token: %w", err))
	}
	newExpiresAt := time.Now().Add(s.refreshTTL)
	if err := s.repo.RotateRefreshToken(ctx, token.ID, token.UserID, sha256Hex(rawRefreshToken), newExpiresAt); err != nil {
		return nil, recordAuthError(span, fmt.Errorf("authService.Refresh: %w", err))
	}

	span.SetAttributes(attribute.Int64("user.id", token.UserID))
	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
	}, nil
}

// Logout revokes the given refresh token. Unknown or already revoked tokens are
// treated as success so the endpoint does not reveal token state.
func (s *Service) Logout(ctx context.Context, req *RefreshRequest) error {
	ctx, span := otel.Tracer(authTracerName).Start(ctx, "authService.Logout")
	defer span.End()
	span.SetAttributes(attribute.String("auth.method", "logout"))

	if req == nil || req.RefreshToken == "" {
		return recordAuthError(span, apperrors.NewValidationError("refresh_token is required"))
	}
	if err := s.repo.RevokeRefreshToken(ctx, sha256Hex(req.RefreshToken)); err != nil {
		return recordAuthError(span, fmt.Errorf("authService.Logout: %w", err))
	}
	return nil
}

func (s *Service) issueTokens(ctx context.Context, userID int64) (*TokenResponse, error) {
	accessToken, err := s.issueAccessToken(userID)
	if err != nil {
		return nil, err
	}
	rawRefreshToken, err := newRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("authService.issueTokens generate token: %w", err)
	}
	if err := s.repo.CreateRefreshToken(ctx, userID, sha256Hex(rawRefreshToken), time.Now().Add(s.refreshTTL)); err != nil {
		return nil, fmt.Errorf("authService.issueTokens: %w", err)
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
	}, nil
}

func (s *Service) issueAccessToken(userID int64) (string, error) {
	now := time.Now()
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    JWTIssuer,
		Audience:  jwt.ClaimStrings{JWTAudience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func newRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) error {
	if len(email) == 0 || len(email) > 254 || !emailPattern.MatchString(email) {
		return apperrors.NewValidationError("email is invalid")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 72 {
		return apperrors.NewValidationError("password must be between 8 and 72 bytes")
	}
	return nil
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

type invalidCredentialsError struct{}

func (invalidCredentialsError) Error() string { return "invalid credentials" }
func (invalidCredentialsError) Unwrap() error { return apperrors.ErrUnauthorized }

func newInvalidCredentialsError() error { return invalidCredentialsError{} }

// recordAuthError marks the span as failed. Expected client errors (validation,
// invalid credentials, conflict, ...) only set the span status; unexpected errors
// are also recorded as span events, following the convention of the other services.
func recordAuthError(span trace.Span, err error) error {
	if !isClientError(err) {
		span.RecordError(err)
	}
	span.SetStatus(codes.Error, err.Error())
	return err
}

func isClientError(err error) bool {
	return errors.Is(err, apperrors.ErrValidation) ||
		errors.Is(err, apperrors.ErrUnauthorized) ||
		errors.Is(err, apperrors.ErrForbidden) ||
		errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrConflict)
}

func logInvalidLogin(ctx context.Context) {
	logger.FromContext(ctx).Warn("invalid login credentials")
}
