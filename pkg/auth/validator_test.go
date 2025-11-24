package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    string
	}{
		{
			name:  "simple token",
			token: "test-token-123",
			want:  "5f8c1f8d8f5e5c5f0f1c2f3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b",
		},
		{
			name:  "empty token",
			token: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashToken(tt.token)
			hash2 := hashToken(tt.token)

			// Hash should be consistent
			if hash1 != hash2 {
				t.Errorf("Hash should be consistent for same token")
			}

			// Hash should be 64 characters (256 bits in hex)
			if len(hash1) != 64 {
				t.Errorf("Expected hash length 64, got %d", len(hash1))
			}

			// Different tokens should have different hashes
			if tt.token != "" {
				differentHash := hashToken(tt.token + "different")
				if hash1 == differentHash {
					t.Error("Different tokens should produce different hashes")
				}
			}
		})
	}
}

func TestTokenCache_SetAndGet(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     5 * time.Minute,
	}

	user := &UserClaims{
		Subject: "test-user",
		Email:   "test@example.com",
		Name:    "Test User",
	}

	tokenHash := hashToken("test-token")

	// Set user in cache
	cache.set(tokenHash, user)

	// Get user from cache
	cachedUser := cache.get(tokenHash)
	if cachedUser == nil {
		t.Fatal("Expected to retrieve user from cache")
	}

	if cachedUser.Subject != user.Subject {
		t.Errorf("Expected subject %s, got %s", user.Subject, cachedUser.Subject)
	}
	if cachedUser.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, cachedUser.Email)
	}
}

func TestTokenCache_GetNonexistent(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     5 * time.Minute,
	}

	cachedUser := cache.get("nonexistent-hash")
	if cachedUser != nil {
		t.Error("Expected nil for nonexistent cache entry")
	}
}

func TestTokenCache_Expiry(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     10 * time.Millisecond, // Very short TTL for testing
	}

	user := &UserClaims{
		Subject: "test-user",
	}

	tokenHash := hashToken("test-token")

	// Set user in cache
	cache.set(tokenHash, user)

	// Should be retrievable immediately
	cachedUser := cache.get(tokenHash)
	if cachedUser == nil {
		t.Fatal("Expected to retrieve user from cache immediately")
	}

	// Wait for expiry
	time.Sleep(50 * time.Millisecond)

	// Should be expired now
	cachedUser = cache.get(tokenHash)
	if cachedUser != nil {
		t.Error("Expected cache entry to be expired")
	}
}

func TestTokenCache_Cleanup(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     1 * time.Millisecond,
	}

	// Add some entries that will expire
	for i := 0; i < 5; i++ {
		user := &UserClaims{
			Subject: string(rune('A' + i)),
		}
		cache.set(hashToken(string(rune('A'+i))), user)
	}

	// Wait for entries to expire
	time.Sleep(10 * time.Millisecond)

	// Run cleanup
	cache.cleanup()

	// All entries should be cleaned up
	cache.mu.RLock()
	entryCount := len(cache.entries)
	cache.mu.RUnlock()

	if entryCount != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", entryCount)
	}
}

func TestTokenCache_CleanupKeepsValid(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     5 * time.Minute,
	}

	// Add some valid entries
	for i := 0; i < 5; i++ {
		user := &UserClaims{
			Subject: string(rune('A' + i)),
		}
		cache.set(hashToken(string(rune('A'+i))), user)
	}

	// Run cleanup (should not remove valid entries)
	cache.cleanup()

	// All entries should still be there
	cache.mu.RLock()
	entryCount := len(cache.entries)
	cache.mu.RUnlock()

	if entryCount != 5 {
		t.Errorf("Expected 5 entries after cleanup, got %d", entryCount)
	}
}

func TestNewTokenValidator_MissingIssuer(t *testing.T) {
	config := &AuthConfig{
		Audience:        "test-audience",
		CacheTTLMinutes: 5,
	}

	_, err := NewTokenValidator(config)
	if err == nil {
		t.Error("Expected error for missing issuer")
	}
}

func TestNewTokenValidator_MissingAudience(t *testing.T) {
	config := &AuthConfig{
		Issuer:          "https://auth.example.com",
		CacheTTLMinutes: 5,
	}

	_, err := NewTokenValidator(config)
	if err == nil {
		t.Error("Expected error for missing audience")
	}
}

func TestValidateAudience_String(t *testing.T) {
	validator := &TokenValidator{
		audience: "expected-audience",
	}

	tests := []struct {
		name     string
		claims   jwt.MapClaims
		expected bool
	}{
		{
			name: "matching string audience",
			claims: jwt.MapClaims{
				"aud": "expected-audience",
			},
			expected: true,
		},
		{
			name: "non-matching string audience",
			claims: jwt.MapClaims{
				"aud": "wrong-audience",
			},
			expected: false,
		},
		{
			name: "matching in array",
			claims: jwt.MapClaims{
				"aud": []interface{}{"other-audience", "expected-audience", "another-audience"},
			},
			expected: true,
		},
		{
			name: "not matching in array",
			claims: jwt.MapClaims{
				"aud": []interface{}{"other-audience", "another-audience"},
			},
			expected: false,
		},
		{
			name:     "missing audience",
			claims:   jwt.MapClaims{},
			expected: false,
		},
		{
			name: "invalid type",
			claims: jwt.MapClaims{
				"aud": 123,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.validateAudience(tt.claims)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for claims %v", tt.expected, result, tt.claims)
			}
		})
	}
}

func TestUserClaims_ExtraFields(t *testing.T) {
	user := &UserClaims{
		Subject:  "test-user",
		Email:    "test@example.com",
		Name:     "Test User",
		Issuer:   "https://auth.example.com",
		Audience: "test-audience",
		IssuedAt: 1234567890,
		Expiry:   1234567990,
		Extra: map[string]interface{}{
			"custom_claim": "custom_value",
			"role":         "admin",
		},
	}

	// Verify extra fields are stored
	if user.Extra["custom_claim"] != "custom_value" {
		t.Error("Expected custom_claim to be stored in Extra")
	}
	if user.Extra["role"] != "admin" {
		t.Error("Expected role to be stored in Extra")
	}
}

func TestAuthConfig_GetCacheTTLFromValidator(t *testing.T) {
	config := &AuthConfig{
		CacheTTLMinutes: 10,
	}

	expectedTTL := 10 * time.Minute
	actualTTL := config.GetCacheTTL()

	if actualTTL != expectedTTL {
		t.Errorf("Expected TTL %v, got %v", expectedTTL, actualTTL)
	}
}

func TestTokenCache_ConcurrentAccess(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     5 * time.Minute,
	}

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			user := &UserClaims{
				Subject: string(rune('A' + id)),
			}
			cache.set(hashToken(string(rune('A'+id))), user)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			cache.get(hashToken(string(rune('A' + id))))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all entries are there
	cache.mu.RLock()
	entryCount := len(cache.entries)
	cache.mu.RUnlock()

	if entryCount != 10 {
		t.Errorf("Expected 10 entries, got %d", entryCount)
	}
}

func TestTokenCache_OverwriteEntry(t *testing.T) {
	cache := &tokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     5 * time.Minute,
	}

	tokenHash := hashToken("test-token")

	// Set first user
	user1 := &UserClaims{
		Subject: "user1",
		Email:   "user1@example.com",
	}
	cache.set(tokenHash, user1)

	// Set second user with same hash (overwrite)
	user2 := &UserClaims{
		Subject: "user2",
		Email:   "user2@example.com",
	}
	cache.set(tokenHash, user2)

	// Should retrieve second user
	cachedUser := cache.get(tokenHash)
	if cachedUser == nil {
		t.Fatal("Expected to retrieve user from cache")
	}

	if cachedUser.Subject != "user2" {
		t.Errorf("Expected subject user2, got %s", cachedUser.Subject)
	}
}
