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
// @host localhost:3000
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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/prismon/mcp-space-browser/docs" // Import generated docs
)

var log *logrus.Entry

func init() {
	log = logger.WithName("server")
}

// Start starts the unified HTTP server with both REST API and MCP endpoints
func Start(port int, db *database.DiskDB, dbPath string) error {
	// Set gin to release mode in production
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	contentBaseURL = fmt.Sprintf("http://localhost:%d", port)
	initContentTokenSecret()

	// Middleware for logging
	router.Use(func(c *gin.Context) {
		startTime := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		log.WithFields(logrus.Fields{
			"method": c.Request.Method,
			"path":   path,
			"query":  query,
		}).Info("Incoming request")

		c.Next()

		duration := time.Since(startTime)
		log.WithFields(logrus.Fields{
			"method":   c.Request.Method,
			"path":     path,
			"status":   c.Writer.Status(),
			"duration": duration.Milliseconds(),
		}).Info("Request completed")
	})

	// Swagger documentation endpoint
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// REST API endpoints
	router.GET("/api/index", func(c *gin.Context) {
		handleIndex(c, db)
	})

	router.GET("/api/tree", func(c *gin.Context) {
		handleTree(c, db)
	})

	router.GET("/api/inspect", func(c *gin.Context) {
		handleInspect(c, db)
	})

	router.GET("/api/content", func(c *gin.Context) {
		serveContent(c, db)
	})

	// Create and configure MCP server
	mcpServer := server.NewMCPServer(
		"mcp-space-browser",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true), // subscribe=false, listChanged=true
	)

	// Register all MCP tools
	registerMCPTools(mcpServer, db, dbPath)

	// Register all MCP resources
	registerMCPResources(mcpServer, db)

	// Create streamable HTTP server with stateless mode
	mcpHTTPServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStateLess(true),
	)

	// Mount MCP endpoint at /mcp using Gin's Any method to handle all HTTP methods
	router.Any("/mcp", gin.WrapH(mcpHTTPServer))

	addr := fmt.Sprintf(":%d", port)
	log.WithFields(logrus.Fields{
		"port":         port,
		"rest_api":     "/api/*",
		"mcp_endpoint": "/mcp",
		"swagger_docs": "/docs/index.html",
		"openapi_spec": "/docs/swagger.json",
	}).Info("Unified HTTP server starting with REST API, MCP support, and OpenAPI documentation")

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
		stats, err := crawler.Index(path, db, jobID, nil)
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

// TreeNode represents a node in the hierarchical directory tree
type TreeNode struct {
	Path     string      `json:"path" example:"/home/user/Documents"`
	Size     int64       `json:"size" example:"1048576"`
	Children []*TreeNode `json:"children"`
}

// Legacy type alias for backwards compatibility
type treeNode = TreeNode

// handleTree godoc
//
// @Summary Get hierarchical directory tree
// @Description Returns a hierarchical JSON tree structure showing disk space usage for the specified path and all subdirectories
// @Tags Tree
// @Accept json
// @Produce json
// @Param path query string false "Filesystem path to analyze (defaults to current directory)" example("/home/user/Documents")
// @Success 200 {object} TreeNode
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

	log.WithField("path", abs).Debug("Building tree structure")

	tree, err := buildTree(db, abs, 0)
	if err != nil {
		log.WithError(err).Error("Failed to build tree")
		c.String(http.StatusInternalServerError, "failed to build tree")
		return
	}

	log.WithField("path", abs).Info("Tree structure built successfully")

	c.JSON(http.StatusOK, tree)
}

const maxTreeDepth = 100

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
