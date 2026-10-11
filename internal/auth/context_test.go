package auth

import (
	"context"
	"testing"
)

func TestUserIDFromContext_Absent(t *testing.T) {
	if id, ok := UserIDFromContext(context.Background()); ok || id != 0 {
		t.Errorf("UserIDFromContext() = (%d, %t), want (0, false)", id, ok)
	}
}

func TestWithUserID_RoundTrip(t *testing.T) {
	ctx := WithUserID(context.Background(), 42)

	id, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("UserIDFromContext() did not find user ID")
	}
	if id != 42 {
		t.Errorf("UserIDFromContext() ID = %d, want 42", id)
	}
}
