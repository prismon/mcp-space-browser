package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Set Gin to test mode to reduce noise
	gin.SetMode(gin.TestMode)
}

// setupTestRouter creates a test router with CORS middleware and a test endpoint
func setupTestRouter(corsConfig *CORSConfig) *gin.Engine {
	router := gin.New()
	router.Use(CORSMiddleware(corsConfig))

	// Add a test endpoint that the browser would call
	router.POST("/mcp", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"result": "success"})
	})

	router.GET("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "test"})
	})

	return router
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	router := setupTestRouter(nil) // nil uses default config (allow all origins)

	// Simulate a browser preflight request
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Preflight should return 204 No Content
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Check CORS headers
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "OPTIONS")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_PreflightRequest_WithMCPHeaders(t *testing.T) {
	router := setupTestRouter(nil)

	// MCP clients may send Mcp-Session-Id header
	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Mcp-Session-Id")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Mcp-Session-Id")
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "Mcp-Session-Id")
}

func TestCORSMiddleware_ActualRequest(t *testing.T) {
	router := setupTestRouter(nil)

	// Simulate an actual POST request with Origin header (browser making real request)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Request should succeed
	assert.Equal(t, http.StatusOK, w.Code)

	// CORS headers should be present
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	config := &CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: false,
	}
	router := setupTestRouter(config)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://any-origin.com")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Without credentials, can use literal "*"
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_WildcardOrigin_WithCredentials(t *testing.T) {
	config := &CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}
	router := setupTestRouter(config)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://any-origin.com")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// With credentials, must echo the origin (can't use "*")
	assert.Equal(t, "http://any-origin.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_SpecificOrigins_Allowed(t *testing.T) {
	config := &CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:5173", "https://app.example.com"},
		AllowCredentials: true,
	}
	router := setupTestRouter(config)

	testCases := []struct {
		name   string
		origin string
	}{
		{"localhost:3000", "http://localhost:3000"},
		{"localhost:5173", "http://localhost:5173"},
		{"production", "https://app.example.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.Header.Set("Origin", tc.origin)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.origin, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestCORSMiddleware_SpecificOrigins_Denied(t *testing.T) {
	config := &CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowCredentials: true,
	}
	router := setupTestRouter(config)

	// Request from non-allowed origin
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://evil-site.com")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Request still succeeds (CORS is browser-enforced)
	assert.Equal(t, http.StatusOK, w.Code)
	// But no CORS headers are set
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	router := setupTestRouter(nil) // Default config uses wildcard

	// Request without Origin header (non-browser client or same-origin)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Request should succeed
	assert.Equal(t, http.StatusOK, w.Code)
	// With wildcard config and no credentials requirement, "*" is returned
	// This is fine for non-browser clients
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_NoOriginHeader_SpecificOrigins(t *testing.T) {
	config := &CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowCredentials: true,
	}
	router := setupTestRouter(config)

	// Request without Origin header
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Request should succeed
	assert.Equal(t, http.StatusOK, w.Code)
	// No CORS headers when no Origin and specific origins configured
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_PreflightRequest_DifferentEndpoints(t *testing.T) {
	router := setupTestRouter(nil)

	endpoints := []string{"/mcp", "/api/test"}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, endpoint, nil)
			req.Header.Set("Origin", "http://localhost:5173")
			req.Header.Set("Access-Control-Request-Method", "GET")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestCORSMiddleware_ExposeHeaders(t *testing.T) {
	router := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check that important headers are exposed to JavaScript
	exposeHeaders := w.Header().Get("Access-Control-Expose-Headers")
	assert.Contains(t, exposeHeaders, "Content-Length")
	assert.Contains(t, exposeHeaders, "Content-Type")
	assert.Contains(t, exposeHeaders, "Mcp-Session-Id")
}

func TestDefaultCORSConfig(t *testing.T) {
	config := DefaultCORSConfig()

	require.NotNil(t, config)
	assert.Equal(t, []string{"*"}, config.AllowOrigins)
	assert.True(t, config.AllowCredentials)
}

func TestCORSMiddleware_NilConfig(t *testing.T) {
	// Passing nil should use default config
	router := gin.New()
	router.Use(CORSMiddleware(nil))
	router.POST("/mcp", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"result": "success"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should work with default config
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_AllAllowedMethods(t *testing.T) {
	router := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	expectedMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	for _, method := range expectedMethods {
		assert.Contains(t, allowMethods, method, "Allow-Methods should include %s", method)
	}
}

func TestCORSMiddleware_AllAllowedHeaders(t *testing.T) {
	router := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
	expectedHeaders := []string{
		"Origin",
		"Content-Type",
		"Accept",
		"Authorization",
		"X-Requested-With",
		"Mcp-Session-Id",
	}
	for _, header := range expectedHeaders {
		assert.Contains(t, allowHeaders, header, "Allow-Headers should include %s", header)
	}
}

func TestCORSMiddleware_MaxAge(t *testing.T) {
	router := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Max age should be 24 hours (86400 seconds)
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

// TestCORSMiddleware_ProductionOrigins tests the specific origins configured
// in config.yaml for production use
func TestCORSMiddleware_ProductionOrigins(t *testing.T) {
	// These are the origins configured in config.yaml
	config := &CORSConfig{
		AllowOrigins: []string{
			"https://mcp.technicaldetails.org",
			"https://ravening-roily-robbi.ngrok-free.dev",
		},
		AllowCredentials: true,
	}
	router := setupTestRouter(config)

	t.Run("mcp.technicaldetails.org allowed", func(t *testing.T) {
		// Test preflight
		preflightReq := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
		preflightReq.Header.Set("Origin", "https://mcp.technicaldetails.org")
		preflightReq.Header.Set("Access-Control-Request-Method", "POST")
		preflightReq.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, preflightReq)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "https://mcp.technicaldetails.org", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

		// Test actual request
		actualReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		actualReq.Header.Set("Origin", "https://mcp.technicaldetails.org")
		actualReq.Header.Set("Content-Type", "application/json")

		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, actualReq)

		assert.Equal(t, http.StatusOK, w2.Code)
		assert.Equal(t, "https://mcp.technicaldetails.org", w2.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("ngrok origin allowed", func(t *testing.T) {
		// Test preflight
		preflightReq := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
		preflightReq.Header.Set("Origin", "https://ravening-roily-robbi.ngrok-free.dev")
		preflightReq.Header.Set("Access-Control-Request-Method", "POST")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, preflightReq)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "https://ravening-roily-robbi.ngrok-free.dev", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

		// Test actual request
		actualReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		actualReq.Header.Set("Origin", "https://ravening-roily-robbi.ngrok-free.dev")

		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, actualReq)

		assert.Equal(t, http.StatusOK, w2.Code)
		assert.Equal(t, "https://ravening-roily-robbi.ngrok-free.dev", w2.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("other origins denied", func(t *testing.T) {
		deniedOrigins := []string{
			"https://evil-site.com",
			"http://mcp.technicaldetails.org", // http instead of https
			"https://other-ngrok.ngrok-free.dev",
			"https://localhost:3000",
		}

		for _, origin := range deniedOrigins {
			req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", "POST")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Preflight still returns 204, but no CORS headers
			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"),
				"Origin %s should not get CORS headers", origin)
		}
	})
}

// TestCORSMiddleware_FullPreflightDance tests the complete preflight dance:
// 1. Browser sends OPTIONS preflight
// 2. Server responds with CORS headers
// 3. Browser sends actual request
// 4. Server responds with data and CORS headers
func TestCORSMiddleware_FullPreflightDance(t *testing.T) {
	router := setupTestRouter(nil)
	origin := "http://localhost:5173"

	// Step 1: Preflight OPTIONS request
	preflightReq := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	preflightReq.Header.Set("Origin", origin)
	preflightReq.Header.Set("Access-Control-Request-Method", "POST")
	preflightReq.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	preflightResp := httptest.NewRecorder()
	router.ServeHTTP(preflightResp, preflightReq)

	// Verify preflight response
	assert.Equal(t, http.StatusNoContent, preflightResp.Code, "Preflight should return 204")
	assert.Equal(t, origin, preflightResp.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, preflightResp.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, preflightResp.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Contains(t, preflightResp.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Equal(t, "true", preflightResp.Header().Get("Access-Control-Allow-Credentials"))

	// Step 2: Actual POST request (browser would only send if preflight succeeded)
	actualReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	actualReq.Header.Set("Origin", origin)
	actualReq.Header.Set("Content-Type", "application/json")
	actualReq.Header.Set("Authorization", "Bearer test-token")

	actualResp := httptest.NewRecorder()
	router.ServeHTTP(actualResp, actualReq)

	// Verify actual response
	assert.Equal(t, http.StatusOK, actualResp.Code, "Actual request should succeed")
	assert.Equal(t, origin, actualResp.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", actualResp.Header().Get("Access-Control-Allow-Credentials"))

	// Verify response body
	assert.Contains(t, actualResp.Body.String(), "success")
}
