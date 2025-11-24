// Package server provides the unified HTTP server with REST API and MCP endpoints
//
// @title MCP Space Browser API
// @version 0.1.0
// @description Disk space indexing agent with REST API and MCP tools for exploring filesystem utilization
// @description
// @description This API provides endpoints for indexing filesystems and retrieving hierarchical disk space data.
// @description The server also exposes MCP (Model Context Protocol) tools at /mcp for advanced disk space analysis.
//
// @contact.name API Support
// @contact.url https://github.com/prismon/mcp-space-browser
//
// @license.name MIT
//
// @BasePath /
// @schemes http https
//
// @tag.name Index
// @tag.description Filesystem indexing operations
//
// @tag.name Tree
// @tag.description Hierarchical disk space data retrieval
package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/prismon/mcp-space-browser/docs" // Import generated docs
)

var log *logrus.Entry

func init() {
	log = logger.WithName("server")
}

// Start starts the unified HTTP server with both REST API and MCP endpoints
func Start(config *auth.Config, db *database.DiskDB, dbPath string) error {
	// Set gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// Update contentBaseURL from config
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

		// Determine if traffic is being forwarded by an external host
		isForwarded := realIP != "" || forwardedFor != "" || forwardedHost != ""

		// Build log fields for incoming request
		logFields := logrus.Fields{
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
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

		// Log incoming request
		if isForwarded {
			log.WithFields(logFields).Info("Incoming request (forwarded by external host)")
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
			"method":     c.Request.Method,
			"path":       path,
			"status":     c.Writer.Status(),
			"duration":   duration.Milliseconds(),
			"remoteAddr": remoteAddr,
		}
		if isForwarded && realIP != "" {
			responseFields["realIP"] = realIP
		}
		log.WithFields(responseFields).Info("Request completed")
	})

	// Configure Swagger host dynamically based on config
	// This allows the Swagger UI to work correctly regardless of how the service is accessed
	configureSwaggerHost(config)

	// Swagger documentation endpoint (public, no auth required)
	// Middleware updates host dynamically based on incoming request
	router.GET("/docs/*any", swaggerHostMiddleware(), ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve web component microfrontend (public, no auth required)
	router.Static("/web", "./web")

	// REST API endpoints with optional OAuth middleware
	apiGroup := router.Group("/api")
	if validator != nil {
		apiGroup.Use(auth.AuthMiddleware(validator, &config.Auth))
	}

	apiGroup.GET("/index", func(c *gin.Context) {
		handleIndex(c, db)
	})

	apiGroup.GET("/tree", func(c *gin.Context) {
		handleTree(c, db)
	})

	apiGroup.GET("/inspect", func(c *gin.Context) {
		handleInspect(c, db)
	})

	apiGroup.GET("/content", func(c *gin.Context) {
		serveContent(c, db)
	})

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
		"host":           config.Server.Host,
		"port":           config.Server.Port,
		"externalHost":   config.Server.ExternalHost,
		"rest_api":       "/api/*",
		"mcp_endpoint":   "/mcp",
		"swagger_docs":   "/docs/index.html",
		"openapi_spec":   "/docs/swagger.json",
		"web_component":  "/web/index.html",
		"js_client":      jsClientURL,
	}
	if config.Auth.Enabled {
		logFields["auth_enabled"] = true
		logFields["auth_required"] = config.Auth.RequireAuth
		logFields["oauth_issuer"] = config.Auth.Issuer
	}
	log.WithFields(logFields).Info("Unified HTTP server starting with REST API, MCP support, and OpenAPI documentation")

	return router.Run(addr)
}

// IndexResponse represents the response from the index endpoint
type IndexResponse struct {
	Message string `json:"message" example:"OK"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"path required"`
}

// handleIndex godoc
//
// @Summary Index a filesystem path
// @Description Triggers asynchronous indexing of the specified filesystem path. The indexing process crawls the directory tree and stores metadata in the database.
// @Tags Index
// @Accept json
// @Produce plain
// @Param path query string true "Filesystem path to index" example("/home/user/Documents")
// @Success 200 {string} string "OK"
// @Failure 400 {string} string "path required"
// @Router /api/index [get]
func handleIndex(c *gin.Context, db *database.DiskDB) {
	path := c.Query("path")
	if path == "" {
		log.Warn("Missing path parameter")
		c.String(http.StatusBadRequest, "path required")
		return
	}

	log.WithField("path", path).Info("Starting filesystem index via API")

	// Create job for tracking
	jobID, err := db.CreateIndexJob(path, nil)
	if err != nil {
		log.WithError(err).Error("Failed to create index job")
		c.String(http.StatusInternalServerError, "failed to create index job")
		return
	}

	// Mark job as running
	if err := db.UpdateIndexJobStatus(jobID, "running", nil); err != nil {
		log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as running")
	}

	// Run indexing asynchronously with job tracking
	go func() {
		stats, err := crawler.Index(path, db, nil, jobID, nil)
		if err != nil {
			errMsg := err.Error()
			db.UpdateIndexJobStatus(jobID, "failed", &errMsg)
			log.WithFields(logrus.Fields{
				"path":  path,
				"jobID": jobID,
				"error": err,
			}).Error("Filesystem index failed via API")
		} else {
			db.UpdateIndexJobStatus(jobID, "completed", nil)
			db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
				FilesProcessed:       stats.FilesProcessed,
				DirectoriesProcessed: stats.DirectoriesProcessed,
				TotalSize:            stats.TotalSize,
				ErrorCount:           stats.Errors,
			})
			log.WithFields(logrus.Fields{
				"path":                 path,
				"duration":             stats.Duration.Milliseconds(),
				"filesProcessed":       stats.FilesProcessed,
				"directoriesProcessed": stats.DirectoriesProcessed,
				"totalSize":            stats.TotalSize,
			}).Info("Filesystem index completed via API")
		}
	}()

	c.String(http.StatusOK, "OK")
}

// TreeNode represents a node in the hierarchical directory tree (legacy, for backwards compatibility)
type TreeNode struct {
	Path     string      `json:"path" example:"/home/user/Documents"`
	Size     int64       `json:"size" example:"1048576"`
	Children []*TreeNode `json:"children"`
}

// Legacy type alias for backwards compatibility
type treeNode = TreeNode

// handleTree godoc
//
// @Summary Get hierarchical directory tree with pagination
// @Description Returns a paginated directory listing with statistics and summary information
// @Tags Tree
// @Accept json
// @Produce json
// @Param path query string false "Filesystem path to analyze (defaults to current directory)" example("/home/user/Documents")
// @Param limit query int false "Maximum number of children to return per page (default: 100, max: 1000)" example(100)
// @Param offset query int false "Pagination offset (default: 0)" example(0)
// @Param sortBy query string false "Sort children by: size, name, mtime (default: size)" example("size")
// @Param order query string false "Sort order: asc or desc (default: desc)" example("desc")
// @Param includeChildren query boolean false "Include children in response (default: true)" example(true)
// @Success 200 {object} PaginatedTreeResponse
// @Failure 400 {string} string "invalid path"
// @Failure 500 {string} string "failed to build tree"
// @Router /api/tree [get]
func handleTree(c *gin.Context, db *database.DiskDB) {
	path := c.Query("path")
	if path == "" {
		path = "."
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		log.WithError(err).Error("Failed to resolve absolute path")
		c.String(http.StatusBadRequest, "invalid path")
		return
	}

	// Parse pagination parameters
	limit, offset := parsePaginationParams(c.Query("limit"), c.Query("offset"), 100, 1000)
	sortBy := c.DefaultQuery("sortBy", "size")
	order := c.DefaultQuery("order", "desc")
	includeChildren := c.DefaultQuery("includeChildren", "true") == "true"

	log.WithFields(logrus.Fields{
		"path":            abs,
		"limit":           limit,
		"offset":          offset,
		"sortBy":          sortBy,
		"order":           order,
		"includeChildren": includeChildren,
	}).Debug("Building paginated tree structure")

	response, err := buildPaginatedTree(db, abs, limit, offset, sortBy, order == "desc", includeChildren)
	if err != nil {
		log.WithError(err).Error("Failed to build tree")
		c.String(http.StatusInternalServerError, "failed to build tree")
		return
	}

	log.WithField("path", abs).Info("Paginated tree structure built successfully")

	c.JSON(http.StatusOK, response)
}

const maxTreeDepth = 100

// buildTree is kept for backwards compatibility but may be deprecated in favor of buildPaginatedTree
func buildTree(db *database.DiskDB, root string, depth int) (*treeNode, error) {
	// Prevent stack overflow with depth limit
	if depth > maxTreeDepth {
		return nil, fmt.Errorf("maximum tree depth (%d) exceeded", maxTreeDepth)
	}

	entry, err := db.Get(root)
	if err != nil {
		return nil, err
	}

	if entry == nil {
		log.WithField("path", root).Trace("Entry not found while building tree")
		return nil, nil
	}

	node := &treeNode{
		Path:     root,
		Size:     entry.Size,
		Children: []*treeNode{},
	}

	children, err := db.Children(root)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		childNode, err := buildTree(db, child.Path, depth+1)
		if err != nil {
			return nil, err
		}
		if childNode != nil {
			node.Children = append(node.Children, childNode)
		}
	}

	return node, nil
}

// parsePaginationParams parses limit and offset with defaults and max limit
func parsePaginationParams(limitStr, offsetStr string, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	offset := 0

	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	if offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}

	return limit, offset
}

// buildPaginatedTree builds a paginated tree structure with summary statistics
func buildPaginatedTree(db *database.DiskDB, root string, limit, offset int, sortBy string, descending, includeChildren bool) (*PaginatedTreeResponse, error) {
	// Get the root entry
	entry, err := db.Get(root)
	if err != nil {
		return nil, err
	}

	if entry == nil {
		return nil, fmt.Errorf("path not found in index")
	}

	response := &PaginatedTreeResponse{
		Path:       entry.Path,
		Name:       filepath.Base(entry.Path),
		Size:       entry.Size,
		Kind:       entry.Kind,
		ModifiedAt: time.Unix(entry.Mtime, 0).Format(time.RFC3339),
	}

	// If it's a file, no children to fetch
	if entry.Kind == "file" {
		response.Pagination = &PaginationMetadata{
			Total:   0,
			Limit:   limit,
			Offset:  0,
			HasMore: false,
		}
		return response, nil
	}

	// Get all children for this directory
	children, err := db.Children(root)
	if err != nil {
		return nil, err
	}

	total := len(children)

	// Sort children
	SortEntries(children, sortBy, descending)

	// Build summary statistics from all children
	response.Summary = BuildEntrySummary(children, 10)

	// Apply pagination
	start := offset
	if start > len(children) {
		start = len(children)
	}
	end := offset + limit
	if end > len(children) {
		end = len(children)
	}

	// Build pagination metadata
	baseURL := fmt.Sprintf("/api/tree?path=%s&sortBy=%s&order=%s", root, sortBy, map[bool]string{true: "desc", false: "asc"}[descending])
	response.Pagination = BuildPaginationMetadata(total, limit, offset, baseURL)

	// Optionally include children in response
	if includeChildren {
		paginatedChildren := children[start:end]
		response.Children = make([]*TreeChildNode, len(paginatedChildren))
		for i, child := range paginatedChildren {
			response.Children[i] = EntryToTreeChildNode(child)
		}
	}

	return response, nil
}

// configureSwaggerHost sets the initial Swagger host configuration based on server config
func configureSwaggerHost(config *auth.Config) {
	var host string
	var schemes []string

	// Prefer ExternalHost over deprecated BaseURL
	if config.Server.ExternalHost != "" {
		host = config.Server.ExternalHost
		// Determine scheme from ExternalHost
		if len(host) > 8 && host[:8] == "https://" {
			host = host[8:]
			schemes = []string{"https"}
		} else if len(host) > 7 && host[:7] == "http://" {
			host = host[7:]
			schemes = []string{"http"}
		} else {
			// No scheme specified, support both
			schemes = []string{"http", "https"}
		}
	} else if config.Server.BaseURL != "" {
		// Fall back to deprecated BaseURL
		baseURL := config.Server.BaseURL
		if len(baseURL) > 8 && baseURL[:8] == "https://" {
			host = baseURL[8:]
			schemes = []string{"https"}
		} else if len(baseURL) > 7 && baseURL[:7] == "http://" {
			host = baseURL[7:]
			schemes = []string{"http"}
		} else {
			host = baseURL
			schemes = []string{"http", "https"}
		}
	} else {
		// Default to listen address
		host = fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
		schemes = []string{"http"}
	}

	// Update Swagger info
	docs.SwaggerInfo.Host = host
	docs.SwaggerInfo.Schemes = schemes

	log.WithFields(logrus.Fields{
		"swagger_host":    host,
		"swagger_schemes": schemes,
	}).Info("Swagger documentation configured")
}

// swaggerHostMiddleware returns middleware that dynamically updates Swagger host based on request
func swaggerHostMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine scheme from request
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		// Check X-Forwarded-Proto header (common in reverse proxy setups)
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}

		// Get host from request (includes port if non-standard)
		host := c.Request.Host
		// Check X-Forwarded-Host header (common in reverse proxy setups)
		if forwarded := c.GetHeader("X-Forwarded-Host"); forwarded != "" {
			host = forwarded
		}

		// Update Swagger info for this request
		docs.SwaggerInfo.Host = host
		docs.SwaggerInfo.Schemes = []string{scheme}

		c.Next()
	}
}
