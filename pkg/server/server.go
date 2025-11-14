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

	// REST API endpoints
	router.GET("/api/index", func(c *gin.Context) {
		handleIndex(c, db)
	})

	router.GET("/api/tree", func(c *gin.Context) {
		handleTree(c, db)
	})

	// Create and configure MCP server
	mcpServer := server.NewMCPServer(
		"mcp-space-browser",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// Register all MCP tools
	registerMCPTools(mcpServer, db, dbPath)

	// Create streamable HTTP server with stateless mode
	mcpHTTPServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStateLess(true),
	)

	// Mount MCP endpoint at /mcp using Gin's Any method to handle all HTTP methods
	router.Any("/mcp", gin.WrapH(mcpHTTPServer))

	addr := fmt.Sprintf(":%d", port)
	log.WithFields(logrus.Fields{
		"port":          port,
		"rest_api":      "/api/*",
		"mcp_endpoint":  "/mcp",
	}).Info("Unified HTTP server starting with REST API and MCP support")

	return router.Run(addr)
}

func handleIndex(c *gin.Context, db *database.DiskDB) {
	path := c.Query("path")
	if path == "" {
		log.Warn("Missing path parameter")
		c.String(http.StatusBadRequest, "path required")
		return
	}

	log.WithField("path", path).Info("Starting filesystem index via API")

	// Run indexing asynchronously
	go func() {
		startTime := time.Now()
		if err := crawler.Index(path, db); err != nil {
			log.WithFields(logrus.Fields{
				"path":  path,
				"error": err,
			}).Error("Filesystem index failed via API")
		} else {
			duration := time.Since(startTime)
			log.WithFields(logrus.Fields{
				"path":     path,
				"duration": duration.Milliseconds(),
			}).Info("Filesystem index completed via API")
		}
	}()

	c.String(http.StatusOK, "OK")
}

type treeNode struct {
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	Children []*treeNode `json:"children"`
}

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

func buildTree(db *database.DiskDB, root string, depth int) (*treeNode, error) {
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
