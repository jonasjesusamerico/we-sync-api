package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// SessionVersions reads a user's current session version (ADR-01 of feature 006). It is exported only
// because the router receives it; *auth.PostgresRepository satisfies it.
type SessionVersions interface {
	// SessionVersion returns found == false when the user no longer exists.
	SessionVersion(ctx context.Context, userID int64) (version int64, found bool, err error)
}

// JWT validates the access token with the API's issuer, audience and maximum TTL.
func JWT(secret []byte, sessions SessionVersions) func(http.Handler) http.Handler {
	return JWTWithConfig(secret, auth.JWTIssuer, auth.JWTAudience, 24*time.Hour, sessions)
}

// JWTWithConfig validates the access token and, after every token check passes, compares its "sv"
// claim with the user's current session version: a password change or an account deletion ends the
// session on the next request. sessions must not be nil.
func JWTWithConfig(secret []byte, issuer, audience string, maxTTL time.Duration, sessions SessionVersions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
				httputil.WriteUnauthorized(w, "missing authentication token")
				return
			}

			rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			claims := &auth.Claims{}
			token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
				jwt.WithIssuer(issuer), jwt.WithAudience(audience))
			if err != nil || !token.Valid {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}
			now := time.Now()
			if claims.IssuedAt == nil || claims.ExpiresAt == nil ||
				claims.ExpiresAt.Before(now) || !claims.ExpiresAt.After(claims.IssuedAt.Time) ||
				claims.IssuedAt.After(now.Add(time.Minute)) ||
				(maxTTL > 0 && claims.ExpiresAt.After(claims.IssuedAt.Add(maxTTL))) {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}

			userID, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil || userID <= 0 {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}

			// Only a token that passed every check above costs a database read. The 401 message is
			// the same as for any other token failure: it does not reveal why the session ended.
			version, found, err := sessions.SessionVersion(r.Context(), userID)
			if err != nil {
				logger.FromContext(r.Context()).Error("session version lookup failed", "error", err, "user_id", userID)
				httputil.WriteInternalError(w)
				return
			}
			if !found || version != claims.SessionVersion {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}

			ctx := identity.WithUserID(r.Context(), userID)
			ctx = logger.WithLogger(ctx, logger.FromContext(ctx).With("user_id", userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
