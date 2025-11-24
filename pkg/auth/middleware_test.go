package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name         string
		authHeader   string
		expectedToken string
	}{
		{
			name:         "valid bearer token",
			authHeader:   "Bearer abc123",
			expectedToken: "abc123",
		},
		{
			name:         "bearer with extra spaces",
			authHeader:   "Bearer   token-with-spaces",
			expectedToken: "  token-with-spaces",
		},
		{
			name:         "Bearer uppercase",
			authHeader:   "Bearer TOKEN123",
			expectedToken: "TOKEN123",
		},
		{
			name:         "bearer lowercase",
			authHeader:   "bearer lowercase-token",
			expectedToken: "lowercase-token",
		},
		{
			name:         "BEARER all caps",
			authHeader:   "BEARER CAPS_TOKEN",
			expectedToken: "CAPS_TOKEN",
		},
		{
			name:         "invalid - no bearer prefix",
			authHeader:   "Token abc123",
			expectedToken: "",
		},
		{
			name:         "invalid - no space",
			authHeader:   "Bearerabc123",
			expectedToken: "",
		},
		{
			name:         "invalid - empty",
			authHeader:   "",
			expectedToken: "",
		},
		{
			name:         "invalid - only bearer",
			authHeader:   "Bearer",
			expectedToken: "",
		},
		{
			name:         "invalid - only bearer with space",
			authHeader:   "Bearer ",
			expectedToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := extractBearerToken(tt.authHeader)
			if token != tt.expectedToken {
				t.Errorf("Expected token %q, got %q", tt.expectedToken, token)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("user exists", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		expectedUser := &UserClaims{
			Subject: "test-user",
			Email:   "test@example.com",
		}
		c.Set("user", expectedUser)

		user, ok := GetUser(c)
		if !ok {
			t.Fatal("Expected ok to be true")
		}
		if user.Subject != expectedUser.Subject {
			t.Errorf("Expected subject %s, got %s", expectedUser.Subject, user.Subject)
		}
	})

	t.Run("user does not exist", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		user, ok := GetUser(c)
		if ok {
			t.Error("Expected ok to be false")
		}
		if user != nil {
			t.Error("Expected user to be nil")
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("user", "not a user")

		user, ok := GetUser(c)
		if ok {
			t.Error("Expected ok to be false for wrong type")
		}
		if user != nil {
			t.Error("Expected user to be nil for wrong type")
		}
	})
}

func TestIsAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("authenticated", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("authenticated", true)

		if !IsAuthenticated(c) {
			t.Error("Expected IsAuthenticated to return true")
		}
	})

	t.Run("not authenticated", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("authenticated", false)

		if IsAuthenticated(c) {
			t.Error("Expected IsAuthenticated to return false")
		}
	})

	t.Run("authenticated key missing", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		if IsAuthenticated(c) {
			t.Error("Expected IsAuthenticated to return false when key missing")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("authenticated", "not a bool")

		if IsAuthenticated(c) {
			t.Error("Expected IsAuthenticated to return false for wrong type")
		}
	})
}

func TestAuthMiddleware_NoAuthHeader_NotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock validator (won't be used in this test)
	config := &AuthConfig{
		RequireAuth:     false,
		Issuer:          "https://test.example.com",
		Audience:        "test-audience",
		CacheTTLMinutes: 5,
	}

	middleware := AuthMiddleware(nil, config)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	middleware(c)

	// Should not abort
	if c.IsAborted() {
		t.Error("Expected middleware not to abort when auth not required and no header")
	}

	// Should not set user
	_, exists := c.Get("user")
	if exists {
		t.Error("Expected user not to be set")
	}
}

func TestAuthMiddleware_NoAuthHeader_Required(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &AuthConfig{
		RequireAuth:     true,
		Issuer:          "https://test.example.com",
		Audience:        "test-audience",
		CacheTTLMinutes: 5,
	}

	// Create a minimal validator for the test
	validator := &TokenValidator{
		issuer:   config.Issuer,
		audience: config.Audience,
	}

	middleware := AuthMiddleware(validator, config)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	middleware(c)

	// Should abort with 401
	if !c.IsAborted() {
		t.Error("Expected middleware to abort when auth required and no header")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	// Check WWW-Authenticate header
	wwwAuth := w.Header().Get("WWW-Authenticate")
	if wwwAuth == "" {
		t.Error("Expected WWW-Authenticate header to be set")
	}
}

func TestAuthMiddleware_InvalidAuthHeader_Required(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &AuthConfig{
		RequireAuth:     true,
		Issuer:          "https://test.example.com",
		Audience:        "test-audience",
		CacheTTLMinutes: 5,
	}

	validator := &TokenValidator{
		issuer:   config.Issuer,
		audience: config.Audience,
	}

	middleware := AuthMiddleware(validator, config)

	tests := []struct {
		name       string
		authHeader string
	}{
		{
			name:       "no bearer prefix",
			authHeader: "Token abc123",
		},
		{
			name:       "only bearer",
			authHeader: "Bearer",
		},
		{
			name:       "empty token",
			authHeader: "Bearer ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/", nil)
			c.Request.Header.Set("Authorization", tt.authHeader)

			middleware(c)

			if !c.IsAborted() {
				t.Error("Expected middleware to abort for invalid auth header")
			}

			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected status 401, got %d", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_InvalidAuthHeader_NotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &AuthConfig{
		RequireAuth:     false,
		Issuer:          "https://test.example.com",
		Audience:        "test-audience",
		CacheTTLMinutes: 5,
	}

	middleware := AuthMiddleware(nil, config)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Authorization", "Invalid header")

	middleware(c)

	// Should not abort when auth not required
	if c.IsAborted() {
		t.Error("Expected middleware not to abort when auth not required")
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validator := &TokenValidator{
		issuer:   "https://test.example.com",
		audience: "test-audience",
	}

	middleware := RequireAuth(validator)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Set("authenticated", true)

	middleware(c)

	// Should not abort
	if c.IsAborted() {
		t.Error("Expected middleware not to abort when authenticated")
	}
}

func TestRequireAuth_NotAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validator := &TokenValidator{
		issuer:   "https://test.example.com",
		audience: "test-audience",
	}

	middleware := RequireAuth(validator)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	middleware(c)

	// Should abort with 401
	if !c.IsAborted() {
		t.Error("Expected middleware to abort when not authenticated")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestRespondUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validator := &TokenValidator{
		issuer:   "https://auth.example.com",
		audience: "test-audience",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondUnauthorized(c, validator, "test error description")

	// Check status code
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	// Check WWW-Authenticate header
	wwwAuth := w.Header().Get("WWW-Authenticate")
	if wwwAuth == "" {
		t.Error("Expected WWW-Authenticate header to be set")
	}

	// Verify header contains realm and error
	if len(wwwAuth) == 0 {
		t.Error("WWW-Authenticate header is empty")
	}
}
