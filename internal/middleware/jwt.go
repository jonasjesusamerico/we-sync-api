package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

func JWT(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
				httputil.WriteUnauthorized(w, "token de autenticação ausente")
				return
			}

			rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			claims := &auth.Claims{}
			token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			})
			if err != nil || !token.Valid {
				httputil.WriteUnauthorized(w, "token de autenticação inválido")
				return
			}

			userID, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil || userID <= 0 {
				httputil.WriteUnauthorized(w, "token de autenticação inválido")
				return
			}

			ctx := auth.WithUserID(r.Context(), userID)
			ctx = logger.WithLogger(ctx, logger.FromContext(ctx).With("user_id", userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
