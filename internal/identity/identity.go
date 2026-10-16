// Package identity carries the authenticated user identity through request contexts.
package identity

import "context"

type userIDKey struct{}

// WithUserID returns a context that carries the authenticated user's ID.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserIDFromContext returns the authenticated user's ID when it is present.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}
