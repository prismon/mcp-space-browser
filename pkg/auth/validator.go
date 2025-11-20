package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("auth")
}

// UserClaims represents the authenticated user extracted from JWT token
type UserClaims struct {
	Subject  string                 `json:"sub"`
	Email    string                 `json:"email,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Issuer   string                 `json:"iss"`
	Audience string                 `json:"aud"`
	IssuedAt int64                  `json:"iat"`
	Expiry   int64                  `json:"exp"`
	Extra    map[string]interface{} `json:"-"`
}

// TokenValidator validates JWT access tokens
type TokenValidator struct {
	issuer   string
	audience string
	jwksURL  string
	keySet   jwk.Set
	cache    *tokenCache
	mu       sync.RWMutex
}

// tokenCache stores validated tokens with TTL
type tokenCache struct {
	entries map[string]*cacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

type cacheEntry struct {
	user      *UserClaims
	expiresAt time.Time
}

// NewTokenValidator creates a new token validator
func NewTokenValidator(config *AuthConfig) (*TokenValidator, error) {
	if config.Issuer == "" {
		return nil, fmt.Errorf("issuer is required")
	}
	if config.Audience == "" {
		return nil, fmt.Errorf("audience is required")
	}

	// Auto-discover JWKS URL if not provided
	jwksURL := config.JWKSURL
	if jwksURL == "" {
		jwksURL = fmt.Sprintf("%s/.well-known/jwks.json", config.Issuer)
		log.WithField("jwks_url", jwksURL).Debug("Auto-discovered JWKS URL")
	}

	validator := &TokenValidator{
		issuer:   config.Issuer,
		audience: config.Audience,
		jwksURL:  jwksURL,
		cache: &tokenCache{
			entries: make(map[string]*cacheEntry),
			ttl:     config.GetCacheTTL(),
		},
	}

	// Fetch JWKS
	if err := validator.refreshJWKS(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	log.WithFields(logrus.Fields{
		"issuer":   validator.issuer,
		"audience": validator.audience,
		"jwks_url": validator.jwksURL,
	}).Info("Token validator initialized")

	return validator, nil
}

// ValidateToken validates a JWT access token and returns user claims
func (v *TokenValidator) ValidateToken(tokenString string) (*UserClaims, error) {
	// Check cache first (use hash to avoid storing full tokens)
	tokenHash := hashToken(tokenString)
	if cached := v.cache.get(tokenHash); cached != nil {
		log.WithField("subject", cached.Subject).Debug("Token validation cache hit")
		return cached, nil
	}

	// Parse and validate JWT
	token, err := jwt.Parse(tokenString, v.keyFunc)
	if err != nil {
		log.WithError(err).Warn("Failed to parse JWT token")
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer
	iss, ok := claims["iss"].(string)
	if !ok || iss != v.issuer {
		log.WithFields(logrus.Fields{
			"expected": v.issuer,
			"actual":   iss,
		}).Warn("Token issuer mismatch")
		return nil, fmt.Errorf("invalid issuer")
	}

	// Validate audience
	if !v.validateAudience(claims) {
		log.WithField("audience", v.audience).Warn("Token audience mismatch")
		return nil, fmt.Errorf("invalid audience")
	}

	// Extract user info
	user := &UserClaims{
		Issuer:   iss,
		Audience: v.audience,
		Extra:    make(map[string]interface{}),
	}

	if sub, ok := claims["sub"].(string); ok {
		user.Subject = sub
	}
	if email, ok := claims["email"].(string); ok {
		user.Email = email
	}
	if name, ok := claims["name"].(string); ok {
		user.Name = name
	}
	if iat, ok := claims["iat"].(float64); ok {
		user.IssuedAt = int64(iat)
	}
	if exp, ok := claims["exp"].(float64); ok {
		user.Expiry = int64(exp)
	}

	// Store extra claims
	for k, v := range claims {
		if k != "sub" && k != "email" && k != "name" && k != "iss" && k != "aud" && k != "iat" && k != "exp" {
			user.Extra[k] = v
		}
	}

	// Cache validated token
	v.cache.set(tokenHash, user)

	log.WithFields(logrus.Fields{
		"subject": user.Subject,
		"email":   user.Email,
	}).Debug("Token validated successfully")

	return user, nil
}

// validateAudience checks if the token audience matches
func (v *TokenValidator) validateAudience(claims jwt.MapClaims) bool {
	aud, ok := claims["aud"]
	if !ok {
		return false
	}

	switch audVal := aud.(type) {
	case string:
		return audVal == v.audience
	case []interface{}:
		for _, a := range audVal {
			if audStr, ok := a.(string); ok && audStr == v.audience {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// keyFunc returns the key for JWT validation
func (v *TokenValidator) keyFunc(token *jwt.Token) (interface{}, error) {
	// Get the key ID from token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing kid header")
	}

	// Lookup key in JWKS
	v.mu.RLock()
	key, ok := v.keySet.LookupKeyID(kid)
	v.mu.RUnlock()

	if !ok {
		// Try refreshing JWKS and lookup again
		log.WithField("kid", kid).Debug("Key not found in JWKS, refreshing...")
		if err := v.refreshJWKS(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
		}

		v.mu.RLock()
		key, ok = v.keySet.LookupKeyID(kid)
		v.mu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("key with kid %s not found in JWKS", kid)
		}
	}

	// Convert JWK to public key
	var pubKey interface{}
	if err := key.Raw(&pubKey); err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	return pubKey, nil
}

// refreshJWKS fetches the latest JWKS from the provider
func (v *TokenValidator) refreshJWKS(ctx context.Context) error {
	keySet, err := jwk.Fetch(ctx, v.jwksURL)
	if err != nil {
		return err
	}

	v.mu.Lock()
	v.keySet = keySet
	v.mu.Unlock()

	log.WithField("key_count", keySet.Len()).Debug("JWKS refreshed")
	return nil
}

// hashToken creates a SHA256 hash of the token for cache key
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// tokenCache methods

func (c *tokenCache) get(tokenHash string) *UserClaims {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[tokenHash]
	if !ok {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.user
}

func (c *tokenCache) set(tokenHash string, user *UserClaims) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[tokenHash] = &cacheEntry{
		user:      user,
		expiresAt: time.Now().Add(c.ttl),
	}

	// Clean up expired entries periodically
	if len(c.entries) > 1000 {
		go c.cleanup()
	}
}

func (c *tokenCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for hash, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, hash)
		}
	}
}
