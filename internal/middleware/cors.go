package middleware

import (
	"net/http"
	"strconv"
)

// corsPreflightMaxAge is how long, in seconds, a browser may cache a successful preflight.
// 10 min: fewer OPTIONS requests, while a removal from the allowlist still takes effect quickly
// (Chrome caps the value at 7200 s).
const corsPreflightMaxAge = 600

// CORS returns a middleware that restricts cross-origin requests to the given
// list of allowed origins. Origins are matched exactly (case-sensitive).
// Preflight responses for allowed origins carry Access-Control-Max-Age so the browser can cache them.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	maxAge := strconv.Itoa(corsPreflightMaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Max-Age", maxAge)
				}
			}

			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
