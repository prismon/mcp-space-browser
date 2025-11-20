package auth

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
)

// CreateMCPAuthOption creates an MCP server option for OAuth authentication
// Note: The current mcp-go library doesn't expose request interceptors,
// so OAuth authentication for MCP is handled at the HTTP middleware level
// through the WrapMCPHandler function.
//
// This function returns a no-op server option for now, as authentication
// is handled by HTTP middleware before requests reach the MCP handler.
func CreateMCPAuthOption(validator *TokenValidator, config *AuthConfig) server.ServerOption {
	return func(s *server.MCPServer) {
		// No-op: Authentication is handled at the HTTP middleware level
		// The WrapMCPHandler validates tokens before they reach this handler
		// User context can be extracted from HTTP request context if needed
	}
}

// WrapMCPHandler wraps an HTTP handler to validate OAuth tokens for MCP requests
// This is used by the server to protect the MCP endpoint
func WrapMCPHandler(validator *TokenValidator, config *AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			if config.RequireAuth {
				// Return 401 with RFC 6750 compliant error
				w.Header().Set("WWW-Authenticate", createWWWAuthenticate(validator, "missing authorization header"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid_token","error_description":"missing authorization header"}`))
				return
			}
			// Auth not required, continue
			next.ServeHTTP(w, r)
			return
		}

		// Extract Bearer token
		token := extractBearerToken(authHeader)
		if token == "" {
			if config.RequireAuth {
				w.Header().Set("WWW-Authenticate", createWWWAuthenticate(validator, "invalid authorization header format"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid_token","error_description":"invalid authorization header format"}`))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Validate token
		user, err := validator.ValidateToken(token)
		if err != nil {
			log.WithError(err).Warn("Token validation failed for MCP request")
			if config.RequireAuth {
				w.Header().Set("WWW-Authenticate", createWWWAuthenticate(validator, "invalid token"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid_token","error_description":"invalid token"}`))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Add user to request context
		ctx := SetUserInContext(r.Context(), user)
		r = r.WithContext(ctx)

		log.WithField("subject", user.Subject).Debug("MCP request authenticated")
		next.ServeHTTP(w, r)
	})
}

// createWWWAuthenticate creates RFC 6750 compliant WWW-Authenticate header
func createWWWAuthenticate(validator *TokenValidator, errorDescription string) string {
	return "Bearer realm=\"" + validator.issuer + "\", error=\"invalid_token\", error_description=\"" + errorDescription + "\""
}
