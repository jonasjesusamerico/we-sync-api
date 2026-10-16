package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

func JWT(secret []byte) func(http.Handler) http.Handler {
	return JWTWithConfig(secret, auth.JWTIssuer, auth.JWTAudience, 24*time.Hour)
}

func JWTWithConfig(secret []byte, issuer, audience string, maxTTL time.Duration) func(http.Handler) http.Handler {
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
				claims.ExpiresAt.Time.Before(now) || !claims.ExpiresAt.Time.After(claims.IssuedAt.Time) ||
				claims.IssuedAt.Time.After(now.Add(time.Minute)) ||
				(maxTTL > 0 && claims.ExpiresAt.Time.After(claims.IssuedAt.Time.Add(maxTTL))) {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}

			userID, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil || userID <= 0 {
				httputil.WriteUnauthorized(w, "invalid authentication token")
				return
			}

			ctx := auth.WithUserID(r.Context(), userID)
			ctx = logger.WithLogger(ctx, logger.FromContext(ctx).With("user_id", userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
