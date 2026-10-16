package middleware

import "net/http"

// MaxBodySize returns a middleware that limits request bodies to maxBytes.
// Reads beyond the limit fail, so JSON decoding in handlers returns an error
// instead of buffering arbitrarily large payloads.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
