// Package server provides the MCP (Model Context Protocol) server
//
// This package exposes MCP tools and resources for disk space analysis and filesystem exploration.
package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry

func init() {
	log = logger.WithName("server")
}

// Start starts the MCP HTTP server
func Start(config *auth.Config, db *database.DiskDB, dbPath string) error {
	// Set gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// Update contentBaseURL from config (needed for MCP resources)
	contentBaseURL = config.Server.BaseURL

	// Set artifact cache directory from config
	SetArtifactCacheDir(config.Cache.Dir)

	// Set artifact database for persistence
	SetArtifactDB(db)

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

	// Middleware for logging with detailed traffic information
	router.Use(func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

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

	// Initialize source manager with classifier
	if err := InitializeSourceManager(db.DB(), nil); err != nil {
		log.WithError(err).Warn("Failed to initialize source manager")
	} else {
		log.Info("Source manager initialized successfully")
	}

	// Register all MCP tools
	registerMCPTools(mcpServer, db, dbPath)

	// Register all MCP resources
	registerMCPResources(mcpServer, db)

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

	// Construct full URL for JavaScript client library
	var jsClientURL string
	if config.Server.ExternalHost != "" {
		// Use external host (may include scheme)
		if len(config.Server.ExternalHost) > 8 && config.Server.ExternalHost[:8] == "https://" {
			jsClientURL = config.Server.ExternalHost + "/web/mcp-client.js"
		} else if len(config.Server.ExternalHost) > 7 && config.Server.ExternalHost[:7] == "http://" {
			jsClientURL = config.Server.ExternalHost + "/web/mcp-client.js"
		} else {
			// No scheme, default to http
			jsClientURL = "http://" + config.Server.ExternalHost + "/web/mcp-client.js"
		}
	} else if config.Server.BaseURL != "" {
		// Fall back to deprecated BaseURL
		if len(config.Server.BaseURL) > 8 && config.Server.BaseURL[:8] == "https://" {
			jsClientURL = config.Server.BaseURL + "/web/mcp-client.js"
		} else if len(config.Server.BaseURL) > 7 && config.Server.BaseURL[:7] == "http://" {
			jsClientURL = config.Server.BaseURL + "/web/mcp-client.js"
		} else {
			jsClientURL = "http://" + config.Server.BaseURL + "/web/mcp-client.js"
		}
	} else {
		// Default to listen address
		jsClientURL = fmt.Sprintf("http://%s:%d/web/mcp-client.js", config.Server.Host, config.Server.Port)
	}

	logFields := logrus.Fields{
		"host":          config.Server.Host,
		"port":          config.Server.Port,
		"externalHost":  config.Server.ExternalHost,
		"mcp_endpoint":  "/mcp",
		"web_component": "/web/index.html",
		"js_client":     jsClientURL,
	}
	if config.Auth.Enabled {
		logFields["auth_enabled"] = true
		logFields["auth_required"] = config.Auth.RequireAuth
		logFields["oauth_issuer"] = config.Auth.Issuer
	}
	log.WithFields(logFields).Info("MCP server with web components starting")

	return router.Run(addr)
}

