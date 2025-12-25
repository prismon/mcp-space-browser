// Package server provides the MCP (Model Context Protocol) server
//
// This package exposes MCP tools and resources for disk space analysis and filesystem exploration.
package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/home"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("server")
}

// CORSConfig holds configuration for CORS middleware
type CORSConfig struct {
	// AllowOrigins is a list of origins that are allowed to make cross-origin requests.
	// Use "*" to allow all origins, or specify specific origins like "http://localhost:3000"
	AllowOrigins []string

	// AllowCredentials indicates whether the request can include user credentials
	AllowCredentials bool
}

// DefaultCORSConfig returns a permissive CORS configuration suitable for development
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}
}

// CORSMiddleware returns a Gin middleware that handles CORS requests
// This enables browser-based clients to directly connect to the MCP server
func CORSMiddleware(config *CORSConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Determine the allowed origin
		allowedOrigin := ""
		if len(config.AllowOrigins) == 1 && config.AllowOrigins[0] == "*" {
			// Wildcard: allow any origin
			// When credentials are enabled, we can't use "*" directly, must echo the origin
			if config.AllowCredentials && origin != "" {
				allowedOrigin = origin
			} else {
				allowedOrigin = "*"
			}
		} else {
			// Check if the request origin is in the allowed list
			for _, allowed := range config.AllowOrigins {
				if allowed == origin {
					allowedOrigin = origin
					break
				}
			}
		}

		// Set CORS headers if we have a valid origin
		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", strings.Join([]string{
				"Origin",
				"Content-Type",
				"Content-Length",
				"Accept",
				"Accept-Encoding",
				"Authorization",
				"X-Requested-With",
				"X-Request-ID",
				"Mcp-Session-Id",
			}, ", "))
			c.Header("Access-Control-Expose-Headers", strings.Join([]string{
				"Content-Length",
				"Content-Type",
				"X-Request-ID",
				"Mcp-Session-Id",
			}, ", "))
			c.Header("Access-Control-Max-Age", "86400") // 24 hours

			if config.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		// Handle preflight OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			log.WithFields(logrus.Fields{
				"origin": origin,
				"method": c.GetHeader("Access-Control-Request-Method"),
			}).Debug("CORS preflight request")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Start starts the MCP HTTP server with multi-project support
func Start(config *auth.Config) error {
	// Set gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	// Initialize home manager to get projects and cache paths
	homeManager, err := home.NewManager("")
	if err != nil {
		return fmt.Errorf("failed to initialize home manager: %w", err)
	}

	// Ensure home directory is initialized
	if !homeManager.Exists() {
		if err := homeManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize home directory: %w", err)
		}
	}

	// Create server context with project and session management
	sc, err := NewServerContext(config, homeManager.ProjectsPath(), homeManager.JoinPath(home.CacheDir))
	if err != nil {
		return fmt.Errorf("failed to create server context: %w", err)
	}
	defer sc.Close()

	router := gin.Default()

	// Add CORS middleware first to handle preflight requests before any other processing
	// This enables browser-based MCP clients to connect directly without a proxy
	corsConfig := DefaultCORSConfig()
	if len(config.Server.CORSOrigins) > 0 {
		corsConfig.AllowOrigins = config.Server.CORSOrigins
	}
	router.Use(CORSMiddleware(corsConfig))
	log.WithField("allowOrigins", corsConfig.AllowOrigins).Info("CORS middleware enabled")

	// Update contentBaseURL from config (needed for MCP resources)
	contentBaseURL = config.Server.BaseURL

	// Set artifact cache directory from config
	SetArtifactCacheDir(config.Cache.Dir)

	// Initialize token validator if auth is enabled
	var validator *auth.TokenValidator
	if config.Auth.Enabled {
		var err error
		validator, err = auth.NewTokenValidator(&config.Auth)
		if err != nil {
			return fmt.Errorf("failed to initialize token validator: %w", err)
		}
		log.Info("OAuth/OIDC authentication enabled")

		// Register Protected Resource Metadata endpoint (RFC 9728)
		auth.RegisterProtectedResourceMetadataEndpoint(router, &config.Auth, config.Server.BaseURL)
		log.Info("Protected Resource Metadata endpoint registered at /.well-known/oauth-protected-resource")
	}

	// Middleware for session ID extraction and logging
	router.Use(func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Extract session ID from header and ensure session exists
		sessionID := c.GetHeader("Mcp-Session-Id")
		if sessionID != "" {
			sc.SessionManager.GetOrCreate(sessionID)
		}

		// Extract client IP and forwarding information
		remoteAddr := c.Request.RemoteAddr
		realIP := c.GetHeader("X-Real-IP")
		forwardedFor := c.GetHeader("X-Forwarded-For")
		forwardedProto := c.GetHeader("X-Forwarded-Proto")
		forwardedHost := c.GetHeader("X-Forwarded-Host")
		userAgent := c.GetHeader("User-Agent")
		referer := c.GetHeader("Referer")

		// Determine the original client IP with priority:
		// 1. X-Real-IP (set by reverse proxies like Traefik)
		// 2. First IP in X-Forwarded-For chain
		// 3. RemoteAddr as fallback
		clientIP := remoteAddr
		if realIP != "" {
			clientIP = realIP
		} else if forwardedFor != "" {
			// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
			// The first IP is typically the original client
			for i := 0; i < len(forwardedFor); i++ {
				if forwardedFor[i] == ',' {
					clientIP = forwardedFor[:i]
					break
				}
			}
			if clientIP == remoteAddr {
				// No comma found, use entire value
				clientIP = forwardedFor
			}
		}

		// Determine if traffic is being forwarded by an external host
		isForwarded := realIP != "" || forwardedFor != "" || forwardedHost != ""

		// Build log fields for incoming request
		logFields := logrus.Fields{
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
			"clientIP":   clientIP,
			"remoteAddr": remoteAddr,
			"userAgent":  userAgent,
		}

		// Add session ID if present
		if sessionID != "" {
			logFields["sessionID"] = sessionID
		}

		// Add forwarding information if present
		if isForwarded {
			logFields["forwarded"] = true
			if realIP != "" {
				logFields["realIP"] = realIP
			}
			if forwardedFor != "" {
				logFields["forwardedFor"] = forwardedFor
			}
			if forwardedProto != "" {
				logFields["forwardedProto"] = forwardedProto
			}
			if forwardedHost != "" {
				logFields["forwardedHost"] = forwardedHost
			}
		}

		// Add referer if present
		if referer != "" {
			logFields["referer"] = referer
		}

		// Log incoming request with clear indication of client IP
		if isForwarded {
			log.WithFields(logFields).Info("Incoming request (forwarded by proxy)")
		} else {
			log.WithFields(logFields).Info("Incoming request (direct)")
		}

		// Verbose logging at DEBUG level with all headers
		if logger.IsLevelEnabled(logrus.DebugLevel) {
			headerFields := logrus.Fields{
				"method": c.Request.Method,
				"path":   path,
			}
			for key, values := range c.Request.Header {
				headerFields["header_"+key] = values
			}
			log.WithFields(headerFields).Debug("Request headers (verbose)")
		}

		c.Next()

		duration := time.Since(startTime)
		responseFields := logrus.Fields{
			"method":   c.Request.Method,
			"path":     path,
			"status":   c.Writer.Status(),
			"duration": duration.Milliseconds(),
			"clientIP": clientIP,
		}
		if isForwarded {
			responseFields["forwarded"] = true
		}
		log.WithFields(responseFields).Info("Request completed")
	})

	// Serve web component microfrontend (public, no auth required)
	router.Static("/web", "./web")

	// Register API endpoints for content serving and file inspection
	// These endpoints require an active project, resolved from session
	router.GET("/api/content", func(c *gin.Context) {
		serveContentWithContext(c, sc)
	})
	router.GET("/api/inspect", func(c *gin.Context) {
		handleInspectWithContext(c, sc)
	})
	log.Info("API endpoints registered: /api/content, /api/inspect")

	// Create and configure MCP server
	mcpOptions := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true), // subscribe=false, listChanged=true
	}

	// Add OAuth option for MCP if auth is enabled
	if validator != nil {
		mcpAuthOption := auth.CreateMCPAuthOption(validator, &config.Auth)
		mcpOptions = append(mcpOptions, mcpAuthOption)
	}

	mcpServer := server.NewMCPServer(
		"mcp-space-browser",
		"0.1.0",
		mcpOptions...,
	)

	// Initialize classifier processor (used for thumbnail generation)
	classifierProcessor := classifier.NewProcessor(&classifier.ProcessorConfig{
		CacheDir:          config.Cache.Dir,
		ClassifierManager: classifier.NewManager(),
		MetadataManager:   classifier.NewMetadataManager(),
		// Note: Database is resolved per-project now, not passed here
	})
	log.Info("Classifier processor initialized")

	// Register project management tools
	registerProjectTools(mcpServer, sc)

	// Register all MCP tools with ServerContext (tools resolve DB from context)
	registerMCPToolsWithContext(mcpServer, sc, classifierProcessor)

	// Register classifier tools with ServerContext
	registerClassifierToolsWithContext(mcpServer, sc, classifierProcessor)

	// Register all MCP resources with ServerContext
	registerMCPResourcesWithContext(mcpServer, sc)

	// Create streamable HTTP server with stateless mode
	mcpHTTPServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStateLess(true),
	)

	// Mount MCP endpoint at /mcp with OAuth protection if enabled
	var mcpHandler http.Handler = mcpHTTPServer
	if validator != nil {
		// Wrap MCP handler with OAuth middleware
		mcpHandler = auth.WrapMCPHandler(validator, &config.Auth, mcpHandler)
	}
	router.Any("/mcp", gin.WrapH(mcpHandler))

	addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)

	logFields := logrus.Fields{
		"host":          config.Server.Host,
		"port":          config.Server.Port,
		"externalHost":  config.Server.ExternalHost,
		"mcp_endpoint":  "/mcp",
		"web_component": "/web/index.html",
		"projectsPath":  homeManager.ProjectsPath(),
		"cachePath":     homeManager.JoinPath(home.CacheDir),
	}
	if config.Auth.Enabled {
		logFields["auth_enabled"] = true
		logFields["auth_required"] = config.Auth.RequireAuth
		logFields["oauth_issuer"] = config.Auth.Issuer
	}
	log.WithFields(logFields).Info("MCP server with multi-project support starting")

	return router.Run(addr)
}

// registerMCPToolsWithContext registers all MCP tools using ServerContext for database resolution
func registerMCPToolsWithContext(s *server.MCPServer, sc *ServerContext, processor *classifier.Processor) {
	log.Info("Registering MCP tools with multi-project support")
	registerMCPToolsMultiProject(s, sc, processor)
}

// registerClassifierToolsWithContext registers classifier tools with ServerContext
func registerClassifierToolsWithContext(s *server.MCPServer, sc *ServerContext, processor *classifier.Processor) {
	// TODO: Refactor classifier tools to use ServerContext
	log.Info("Registering classifier tools with multi-project support")
}

// registerMCPResourcesWithContext registers MCP resources with ServerContext
func registerMCPResourcesWithContext(s *server.MCPServer, sc *ServerContext) {
	// TODO: Refactor resources to use ServerContext
	log.Info("Registering MCP resources with multi-project support")
}

// serveContentWithContext handles content serving with project context
func serveContentWithContext(c *gin.Context, sc *ServerContext) {
	// For content serving, we can serve from the shared cache without needing a project
	// The cache directory is shared across all projects
	serveContentFromCache(c, sc.CacheDir)
}

// handleInspectWithContext handles file inspection with project context
func handleInspectWithContext(c *gin.Context, sc *ServerContext) {
	// Get session ID from request
	sessionID := c.GetHeader("Mcp-Session-Id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Mcp-Session-Id header required"})
		return
	}

	// Get active project for this session
	projectName, err := sc.SessionManager.GetActiveProject(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active project. Use project-open to select a project."})
		return
	}

	// Get database for active project
	db, err := sc.ProjectManager.GetProjectDB(projectName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get project database: %v", err)})
		return
	}

	// Call original inspect handler with the resolved database
	handleInspectWithDB(c, db)
}

