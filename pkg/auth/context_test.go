package auth

import (
	"context"
	"testing"
)

func TestSetUserInContext(t *testing.T) {
	ctx := context.Background()
	user := &UserClaims{
		Subject: "test-user",
		Email:   "test@example.com",
		Name:    "Test User",
		Issuer:  "https://auth.example.com",
		Audience: "test-audience",
	}

	newCtx := SetUserInContext(ctx, user)

	// Verify context is different
	if newCtx == ctx {
		t.Error("Expected new context to be created")
	}

	// Verify user can be retrieved
	retrievedUser, ok := GetUserFromContext(newCtx)
	if !ok {
		t.Fatal("Failed to retrieve user from context")
	}

	if retrievedUser.Subject != user.Subject {
		t.Errorf("Expected subject %s, got %s", user.Subject, retrievedUser.Subject)
	}
	if retrievedUser.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, retrievedUser.Email)
	}
	if retrievedUser.Name != user.Name {
		t.Errorf("Expected name %s, got %s", user.Name, retrievedUser.Name)
	}
}

func TestGetUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()

	user, ok := GetUserFromContext(ctx)
	if ok {
		t.Error("Expected ok to be false when no user in context")
	}
	if user != nil {
		t.Error("Expected user to be nil when not in context")
	}
}

func TestGetUserFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), userContextKey, "not a user")

	user, ok := GetUserFromContext(ctx)
	if ok {
		t.Error("Expected ok to be false when wrong type in context")
	}
	if user != nil {
		t.Error("Expected user to be nil when wrong type in context")
	}
}

func TestSetUserInContext_Nil(t *testing.T) {
	ctx := context.Background()

	newCtx := SetUserInContext(ctx, nil)

	retrievedUser, ok := GetUserFromContext(newCtx)
	// Setting nil pointer still succeeds with type assertion
	if !ok {
		t.Error("Expected ok to be true even for nil *UserClaims")
	}
	if retrievedUser != nil {
		t.Error("Expected user to be nil")
	}
}

func TestSetUserInContext_Multiple(t *testing.T) {
	ctx := context.Background()

	user1 := &UserClaims{
		Subject: "user1",
		Email:   "user1@example.com",
	}

	user2 := &UserClaims{
		Subject: "user2",
		Email:   "user2@example.com",
	}

	// Set first user
	ctx1 := SetUserInContext(ctx, user1)

	// Set second user (overwrites)
	ctx2 := SetUserInContext(ctx1, user2)

	// Verify first context still has user1
	retrievedUser1, ok := GetUserFromContext(ctx1)
	if !ok {
		t.Fatal("Failed to retrieve user1 from ctx1")
	}
	if retrievedUser1.Subject != "user1" {
		t.Errorf("Expected user1, got %s", retrievedUser1.Subject)
	}

	// Verify second context has user2
	retrievedUser2, ok := GetUserFromContext(ctx2)
	if !ok {
		t.Fatal("Failed to retrieve user2 from ctx2")
	}
	if retrievedUser2.Subject != "user2" {
		t.Errorf("Expected user2, got %s", retrievedUser2.Subject)
	}
}
