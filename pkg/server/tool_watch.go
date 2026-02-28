package server

import (
	"context"
	"database/sql"
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
func InitializeSourceManager(db *sql.DB, diskDB *database.DiskDB, clf classifier.Classifier) error {
	ruleEngine := rules.NewEngine(db, diskDB, clf)
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

var watchToolDef = mcp.NewTool("watch",
	mcp.WithDescription("Real-time filesystem monitoring. Start, stop, and manage filesystem watchers."),
	mcp.WithString("action",
		mcp.Required(),
		mcp.Description("Action: start, stop, status, list"),
		mcp.Enum("start", "stop", "status", "list"),
	),
	mcp.WithString("path",
		mcp.Description("Filesystem path to watch (for start)"),
	),
	mcp.WithString("name",
		mcp.Description("Watcher name (for start, stop, status)"),
	),
	mcp.WithString("target",
		mcp.Description("Resource set to populate with results"),
	),
	mcp.WithBoolean("recursive",
		mcp.Description("Watch subdirectories recursively (default: true)"),
	),
	mcp.WithNumber("debounce_ms",
		mcp.Description("Debounce delay in milliseconds (default: 500)"),
	),
)

func registerWatchTool(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(watchToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleWatch(ctx, request, db)
	})
}

func registerWatchToolMP(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(watchToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleWatch(ctx, request, db)
	})
}

func handleWatch(ctx context.Context, request mcp.CallToolRequest, db *database.DiskDB) (*mcp.CallToolResult, error) {
	var args struct {
		Action     string `json:"action"`
		Path       string `json:"path,omitempty"`
		Name       string `json:"name,omitempty"`
		Target     string `json:"target,omitempty"`
		Recursive  *bool  `json:"recursive,omitempty"`
		DebounceMs *int   `json:"debounce_ms,omitempty"`
	}

	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	switch args.Action {
	case "start":
		return handleWatchStart(ctx, db, args.Path, args.Name, args.Target, args.Recursive, args.DebounceMs)
	case "stop":
		return handleWatchStop(ctx, args.Name)
	case "status":
		return handleWatchStatus(ctx, args.Name)
	case "list":
		return handleWatchList(ctx)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown action: %q", args.Action)), nil
	}
}

func handleWatchStart(ctx context.Context, db *database.DiskDB, path, name, target string, recursive *bool, debounceMs *int) (*mcp.CallToolResult, error) {
	if path == "" {
		return mcp.NewToolResultError("path is required for start"), nil
	}
	if name == "" {
		return mcp.NewToolResultError("name is required for start"), nil
	}

	if sourceManager == nil {
		return mcp.NewToolResultError("source manager not initialized"), nil
	}

	rec := true
	if recursive != nil {
		rec = *recursive
	}
	debounce := 500
	if debounceMs != nil {
		debounce = *debounceMs
	}

	liveConfig := &sources.LiveFilesystemConfig{
		WatchRecursive: rec,
		DebounceMs:     debounce,
		BatchSize:      100,
	}

	configJSON, err := sources.MarshalLiveConfig(liveConfig)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal config: %v", err)), nil
	}

	config := &sources.SourceConfig{
		Name:       name,
		Type:       sources.SourceTypeLive,
		RootPath:   path,
		ConfigJSON: configJSON,
		Enabled:    true,
	}

	if err := sourceManager.CreateSource(ctx, config); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create watcher: %v", err)), nil
	}

	// Start it
	src, err := sourceManager.GetSource(ctx, config.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get created source: %v", err)), nil
	}

	if err := sourceManager.StartSource(ctx, src.ID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start watcher: %v", err)), nil
	}

	return jsonResult(map[string]interface{}{
		"status": "started",
		"name":   name,
		"path":   path,
		"id":     src.ID,
	})
}

func handleWatchStop(ctx context.Context, name string) (*mcp.CallToolResult, error) {
	if name == "" {
		return mcp.NewToolResultError("name is required for stop"), nil
	}

	if sourceManager == nil {
		return mcp.NewToolResultError("source manager not initialized"), nil
	}

	// Find source by name
	allSources, err := sourceManager.ListSources(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list sources: %v", err)), nil
	}

	for _, src := range allSources {
		if src.Name == name {
			if err := sourceManager.StopSource(ctx, src.ID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to stop watcher: %v", err)), nil
			}
			return jsonResult(map[string]interface{}{
				"status": "stopped",
				"name":   name,
			})
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("Watcher %q not found", name)), nil
}

func handleWatchStatus(ctx context.Context, name string) (*mcp.CallToolResult, error) {
	if name == "" {
		return mcp.NewToolResultError("name is required for status"), nil
	}

	if sourceManager == nil {
		return mcp.NewToolResultError("source manager not initialized"), nil
	}

	allSources, err := sourceManager.ListSources(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list sources: %v", err)), nil
	}

	for _, src := range allSources {
		if src.Name == name {
			result := map[string]interface{}{
				"name":   src.Name,
				"path":   src.RootPath,
				"status": string(src.Status),
				"id":     src.ID,
			}
			stats, err := sourceManager.GetSourceStats(src.ID)
			if err == nil && stats != nil {
				result["stats"] = map[string]interface{}{
					"files_indexed":  stats.FilesIndexed,
					"dirs_indexed":   stats.DirsIndexed,
					"bytes_indexed":  stats.BytesIndexed,
					"rules_executed": stats.RulesExecuted,
					"error_count":    stats.ErrorCount,
				}
			}
			return jsonResult(result)
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("Watcher %q not found", name)), nil
}

func handleWatchList(ctx context.Context) (*mcp.CallToolResult, error) {
	if sourceManager == nil {
		// No source manager = no watchers, return empty
		return jsonResult(map[string]interface{}{
			"watchers": []interface{}{},
			"total":    0,
		})
	}

	allSources, err := sourceManager.ListSources(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list sources: %v", err)), nil
	}

	// Filter to live sources only
	var watchers []map[string]interface{}
	for _, src := range allSources {
		if src.Type == sources.SourceTypeLive {
			watchers = append(watchers, map[string]interface{}{
				"name":   src.Name,
				"path":   src.RootPath,
				"status": string(src.Status),
				"id":     src.ID,
			})
		}
	}

	if watchers == nil {
		watchers = []map[string]interface{}{}
	}

	return jsonResult(map[string]interface{}{
		"watchers": watchers,
		"total":    len(watchers),
	})
}
