package auth

import (
	"context"
)

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	userContextKey contextKey = "user"
)

// SetUserInContext stores user claims in context (for MCP tool handlers)
func SetUserInContext(ctx context.Context, user *UserClaims) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// GetUserFromContext extracts user claims from context (for MCP tool handlers)
func GetUserFromContext(ctx context.Context) (*UserClaims, bool) {
	user, ok := ctx.Value(userContextKey).(*UserClaims)
	return user, ok
}
