package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates a Gin middleware for OAuth/OIDC authentication
func AuthMiddleware(validator *TokenValidator, config *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			// No token provided
			if config.RequireAuth {
				// Reject request with 401 and RFC 6750 compliant error
				respondUnauthorized(c, validator, "missing authorization header")
				c.Abort()
				return
			}
			// Auth not required, continue without user context
			c.Next()
			return
		}

		// Extract Bearer token
		token := extractBearerToken(authHeader)
		if token == "" {
			if config.RequireAuth {
				respondUnauthorized(c, validator, "invalid authorization header format")
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Validate token
		user, err := validator.ValidateToken(token)
		if err != nil {
			log.WithError(err).Warn("Token validation failed")
			if config.RequireAuth {
				respondUnauthorized(c, validator, "invalid token")
				c.Abort()
				return
			}
			// Auth not required, continue without user context
			c.Next()
			return
		}

		// Store user in context
		c.Set("user", user)
		c.Set("authenticated", true)

		log.WithField("subject", user.Subject).Debug("Request authenticated")
		c.Next()
	}
}

// extractBearerToken extracts the token from "Bearer <token>" format
func extractBearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

// respondUnauthorized sends an RFC 6750 compliant 401 response
func respondUnauthorized(c *gin.Context, validator *TokenValidator, errorDescription string) {
	// RFC 6750 Section 3: WWW-Authenticate header format
	wwwAuthenticate := fmt.Sprintf(
		`Bearer realm="%s", error="invalid_token", error_description="%s"`,
		validator.issuer,
		errorDescription,
	)

	c.Header("WWW-Authenticate", wwwAuthenticate)
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":             "invalid_token",
		"error_description": errorDescription,
	})
}

// GetUser extracts the authenticated user from Gin context
func GetUser(c *gin.Context) (*UserClaims, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}

	userClaims, ok := user.(*UserClaims)
	return userClaims, ok
}

// IsAuthenticated checks if the request is authenticated
func IsAuthenticated(c *gin.Context) bool {
	authenticated, exists := c.Get("authenticated")
	if !exists {
		return false
	}

	isAuth, ok := authenticated.(bool)
	return ok && isAuth
}

// RequireAuth is a middleware that requires authentication
// Use this for specific routes that always need auth, even if global config allows optional auth
func RequireAuth(validator *TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsAuthenticated(c) {
			respondUnauthorized(c, validator, "authentication required")
			c.Abort()
			return
		}
		c.Next()
	}
}
