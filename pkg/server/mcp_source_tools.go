package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/rules"
	"github.com/prismon/mcp-space-browser/pkg/sources"
)

// Global source manager (initialized when server starts)
var sourceManager *sources.Manager

// InitializeSourceManager initializes the global source manager
func InitializeSourceManager(db *sql.DB, clf classifier.Classifier) error {
	ruleEngine := rules.NewEngine(db, clf)
	sourceManager = sources.NewManager(db, ruleEngine)

	// Restore active sources
	ctx := context.Background()
	if err := sourceManager.RestoreActiveSources(ctx); err != nil {
		log.WithError(err).Warn("Failed to restore some active sources")
	}

	return nil
}

// StopSourceManager stops all running sources
func StopSourceManager() error {
	if sourceManager == nil {
		return nil
	}

	ctx := context.Background()
	return sourceManager.StopAll(ctx)
}

// registerSourceTools registers all source management MCP tools
func registerSourceTools(s *server.MCPServer, db *database.DiskDB) {
	registerSourceCreate(s, db)
	registerSourceStart(s, db)
	registerSourceStop(s, db)
	registerSourceList(s, db)
	registerSourceGet(s, db)
	registerSourceDelete(s, db)
	registerSourceStats(s, db)
}

// source-create: Create a new filesystem source
func registerSourceCreate(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-create",
		Description: "Create a new filesystem source for monitoring. Sources can be 'live' (real-time monitoring with fsnotify) or 'manual' (one-time scans).",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Unique name for this source",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Source type: 'live' for real-time monitoring, 'manual' for one-time scans",
					"enum":        []string{"live", "manual"},
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Root path to watch/index",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether this source should start automatically (default: true)",
					"default":     true,
				},
				"watch_recursive": map[string]interface{}{
					"type":        "boolean",
					"description": "For live sources: watch subdirectories recursively (default: true)",
					"default":     true,
				},
				"ignore_patterns": map[string]interface{}{
					"type":        "array",
					"description": "For live sources: glob patterns to ignore (e.g., ['*.tmp', '*.log'])",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"debounce_ms": map[string]interface{}{
					"type":        "integer",
					"description": "For live sources: debounce delay in milliseconds (default: 500)",
					"default":     500,
				},
			},
			Required: []string{"name", "type", "path"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			Name            string   `json:"name"`
			Type            string   `json:"type"`
			Path            string   `json:"path"`
			Enabled         *bool    `json:"enabled"`
			WatchRecursive  *bool    `json:"watch_recursive"`
			IgnorePatterns  []string `json:"ignore_patterns"`
			DebounceMs      *int     `json:"debounce_ms"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Build config
		config := &sources.SourceConfig{
			Name:     args.Name,
			Type:     sources.SourceType(args.Type),
			RootPath: args.Path,
			Enabled:  true,
			Status:   sources.SourceStatusStopped,
		}

		if args.Enabled != nil {
			config.Enabled = *args.Enabled
		}

		// Build type-specific config
		if args.Type == "live" {
			liveConfig := &sources.LiveFilesystemConfig{
				WatchRecursive: true,
				IgnorePatterns: args.IgnorePatterns,
				DebounceMs:     500,
				BatchSize:      100,
			}

			if args.WatchRecursive != nil {
				liveConfig.WatchRecursive = *args.WatchRecursive
			}
			if args.DebounceMs != nil {
				liveConfig.DebounceMs = *args.DebounceMs
			}

			configJSON, err := sources.MarshalLiveConfig(liveConfig)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal config: %v", err)), nil
			}
			config.ConfigJSON = configJSON
		}

		// Create source
		if err := sourceManager.CreateSource(ctx, config); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create source: %v", err)), nil
		}

		result := map[string]interface{}{
			"success": true,
			"source": map[string]interface{}{
				"id":      config.ID,
				"name":    config.Name,
				"type":    config.Type,
				"path":    config.RootPath,
				"enabled": config.Enabled,
				"status":  config.Status,
			},
		}

		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// source-start: Start a filesystem source
func registerSourceStart(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-start",
		Description: "Start a filesystem source. For live sources, this begins real-time monitoring.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "Source ID to start",
				},
			},
			Required: []string{"id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			ID int64 `json:"id"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := sourceManager.StartSource(ctx, args.ID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to start source: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Source %d started successfully", args.ID)), nil
	})
}

// source-stop: Stop a filesystem source
func registerSourceStop(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-stop",
		Description: "Stop a running filesystem source",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "Source ID to stop",
				},
			},
			Required: []string{"id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			ID int64 `json:"id"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := sourceManager.StopSource(ctx, args.ID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to stop source: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Source %d stopped successfully", args.ID)), nil
	})
}

// source-list: List all filesystem sources
func registerSourceList(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-list",
		Description: "List all configured filesystem sources",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		sources, err := sourceManager.ListSources(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list sources: %v", err)), nil
		}

		data, _ := json.Marshal(sources)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// source-get: Get detailed information about a source
func registerSourceGet(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-get",
		Description: "Get detailed information about a specific source",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "Source ID",
				},
			},
			Required: []string{"id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			ID int64 `json:"id"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		config, err := sourceManager.GetSource(ctx, args.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get source: %v", err)), nil
		}

		data, _ := json.Marshal(config)
		return mcp.NewToolResultText(string(data)), nil
	})
}

// source-delete: Delete a filesystem source
func registerSourceDelete(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-delete",
		Description: "Delete a filesystem source (stops it first if running)",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "Source ID to delete",
				},
			},
			Required: []string{"id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			ID int64 `json:"id"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := sourceManager.DeleteSource(ctx, args.ID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete source: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Source %d deleted successfully", args.ID)), nil
	})
}

// source-stats: Get statistics for a running source
func registerSourceStats(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(mcp.Tool{
		Name:        "source-stats",
		Description: "Get statistics for a running filesystem source",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "integer",
					"description": "Source ID",
				},
			},
			Required: []string{"id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if sourceManager == nil {
			return mcp.NewToolResultError("Source manager not initialized"), nil
		}

		var args struct {
			ID int64 `json:"id"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		stats, err := sourceManager.GetSourceStats(args.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get stats: %v", err)), nil
		}

		data, _ := json.Marshal(stats)
		return mcp.NewToolResultText(string(data)), nil
	})
}
