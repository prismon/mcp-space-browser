package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/sirupsen/logrus"
)

var (
	log   *logrus.Entry
	db    *database.DiskDB
	dbPath = "disk.db"
)

func init() {
	log = logger.WithName("mcp")
}

func main() {
	// Initialize database
	var err error
	db, err = database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize database")
	}
	defer db.Close()

	// Create MCP server
	s := server.NewMCPServer(
		"mcp-space-browser",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// Register all tools
	registerTools(s)

	log.Info("Starting MCP server")

	// Start server with stdio transport
	if err := server.ServeStdio(s); err != nil {
		log.WithError(err).Fatal("Server failed")
	}
}

func registerTools(s *server.MCPServer) {
	// Core disk tools
	registerDiskIndexTool(s)
	registerDiskDuTool(s)
	registerDiskTreeTool(s)
	registerDiskTimeRangeTool(s)

	// Selection set tools
	registerSelectionSetCreate(s)
	registerSelectionSetList(s)
	registerSelectionSetGet(s)
	registerSelectionSetModify(s)
	registerSelectionSetDelete(s)

	// Query tools
	registerQueryCreate(s)
	registerQueryExecute(s)
	registerQueryList(s)
	registerQueryGet(s)
	registerQueryUpdate(s)
	registerQueryDelete(s)

	// Session tools
	registerSessionInfo(s)
	registerSessionSetPreferences(s)
}

// Disk Tools

func registerDiskIndexTool(s *server.MCPServer) {
	tool := mcp.NewTool("disk-index",
		mcp.WithDescription("Index the specified path"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File or directory path to index"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		log.WithField("path", args.Path).Info("Executing disk-index")

		if err := crawler.Index(args.Path, db); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Indexing failed: %v", err)), nil
		}

		return mcp.NewToolResultText("OK"), nil
	})
}

func registerDiskDuTool(s *server.MCPServer) {
	tool := mcp.NewTool("disk-du",
		mcp.WithDescription("Get disk usage for a path"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File or directory path"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		abs, err := filepath.Abs(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		entry, err := db.Get(abs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
		}

		if entry == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Path %s not found", args.Path)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("%d", entry.Size)), nil
	})
}

func registerDiskTreeTool(s *server.MCPServer) {
	tool := mcp.NewTool("disk-tree",
		mcp.WithDescription("Return a JSON tree of directories and file sizes"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File or directory path"),
		),
		mcp.WithNumber("maxDepth",
			mcp.Description("Maximum depth to traverse (default: unlimited)"),
		),
		mcp.WithNumber("minSize",
			mcp.Description("Minimum file size to include in bytes (default: 0)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return (default: unlimited)"),
		),
		mcp.WithString("sortBy",
			mcp.Description("Sort entries by size, name, or mtime (default: size)"),
		),
		mcp.WithBoolean("descendingSort",
			mcp.Description("Sort in descending order (default: true)"),
		),
		mcp.WithString("minDate",
			mcp.Description("Filter files modified after this date (YYYY-MM-DD)"),
		),
		mcp.WithString("maxDate",
			mcp.Description("Filter files modified before this date (YYYY-MM-DD)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path           string  `json:"path"`
			MaxDepth       *int    `json:"maxDepth,omitempty"`
			MinSize        *int64  `json:"minSize,omitempty"`
			Limit          *int    `json:"limit,omitempty"`
			SortBy         *string `json:"sortBy,omitempty"`
			DescendingSort *bool   `json:"descendingSort,omitempty"`
			MinDate        *string `json:"minDate,omitempty"`
			MaxDate        *string `json:"maxDate,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		abs, err := filepath.Abs(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		tree, err := db.GetTree(abs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get tree: %v", err)), nil
		}

		treeJSON, err := json.Marshal(tree)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal tree: %v", err)), nil
		}

		return mcp.NewToolResultText(string(treeJSON)), nil
	})
}

func registerDiskTimeRangeTool(s *server.MCPServer) {
	tool := mcp.NewTool("disk-time-range",
		mcp.WithDescription("Find files modified within a date range"),
		mcp.WithString("startDate",
			mcp.Required(),
			mcp.Description("Start date (YYYY-MM-DD)"),
		),
		mcp.WithString("endDate",
			mcp.Required(),
			mcp.Description("End date (YYYY-MM-DD)"),
		),
		mcp.WithString("path",
			mcp.Description("Root path to search (optional)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			StartDate string  `json:"startDate"`
			EndDate   string  `json:"endDate"`
			Path      *string `json:"path,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		entries, err := db.GetEntriesByTimeRange(args.StartDate, args.EndDate, args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Query failed: %v", err)), nil
		}

		result, err := json.Marshal(entries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

// Selection Set Tools

func registerSelectionSetCreate(s *server.MCPServer) {
	tool := mcp.NewTool("selection-set-create",
		mcp.WithDescription("Create a new selection set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the selection set"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the selection set"),
		),
		mcp.WithString("criteriaType",
			mcp.Required(),
			mcp.Description("Criteria type: 'user_selected' or 'tool_query'"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name         string  `json:"name"`
			Description  *string `json:"description,omitempty"`
			CriteriaType string  `json:"criteriaType"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		set := &models.SelectionSet{
			Name:         args.Name,
			Description:  args.Description,
			CriteriaType: args.CriteriaType,
			CreatedAt:    time.Now().Unix(),
			UpdatedAt:    time.Now().Unix(),
		}

		id, err := db.CreateSelectionSet(set)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create selection set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created selection set '%s' with ID %d", args.Name, id)), nil
	})
}

func registerSelectionSetList(s *server.MCPServer) {
	tool := mcp.NewTool("selection-set-list",
		mcp.WithDescription("List all selection sets"),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sets, err := db.ListSelectionSets()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list selection sets: %v", err)), nil
		}

		result, err := json.Marshal(sets)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerSelectionSetGet(s *server.MCPServer) {
	tool := mcp.NewTool("selection-set-get",
		mcp.WithDescription("Get entries in a selection set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the selection set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		entries, err := db.GetSelectionSetEntries(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get selection set entries: %v", err)), nil
		}

		result, err := json.Marshal(entries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerSelectionSetModify(s *server.MCPServer) {
	tool := mcp.NewTool("selection-set-modify",
		mcp.WithDescription("Add or remove entries from a selection set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the selection set"),
		),
		mcp.WithString("operation",
			mcp.Required(),
			mcp.Description("Operation: 'add' or 'remove'"),
		),
		// Note: paths should be an array, but mcp-go might need special handling
		mcp.WithString("paths",
			mcp.Required(),
			mcp.Description("Comma-separated list of paths to add/remove"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name      string `json:"name"`
			Operation string `json:"operation"`
			Paths     string `json:"paths"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Parse comma-separated paths
		var paths []string
		if args.Paths != "" {
			// Simple split by comma - in production, use proper JSON array
			paths = splitPaths(args.Paths)
		}

		var err error
		if args.Operation == "add" {
			err = db.AddToSelectionSet(args.Name, paths)
		} else if args.Operation == "remove" {
			err = db.RemoveFromSelectionSet(args.Name, paths)
		} else {
			return mcp.NewToolResultError("Invalid operation. Use 'add' or 'remove'"), nil
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to modify selection set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully %sed %d entries", args.Operation, len(paths))), nil
	})
}

func registerSelectionSetDelete(s *server.MCPServer) {
	tool := mcp.NewTool("selection-set-delete",
		mcp.WithDescription("Delete a selection set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the selection set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.DeleteSelectionSet(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete selection set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted selection set '%s'", args.Name)), nil
	})
}

// Query Tools

func registerQueryCreate(s *server.MCPServer) {
	tool := mcp.NewTool("query-create",
		mcp.WithDescription("Create a new saved query"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the query"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the query"),
		),
		mcp.WithString("queryType",
			mcp.Required(),
			mcp.Description("Query type: 'file_filter' or 'custom_script'"),
		),
		mcp.WithString("queryJSON",
			mcp.Required(),
			mcp.Description("JSON string of the query filter"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name        string  `json:"name"`
			Description *string `json:"description,omitempty"`
			QueryType   string  `json:"queryType"`
			QueryJSON   string  `json:"queryJSON"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		query := &models.Query{
			Name:        args.Name,
			Description: args.Description,
			QueryType:   args.QueryType,
			QueryJSON:   args.QueryJSON,
			CreatedAt:   time.Now().Unix(),
			UpdatedAt:   time.Now().Unix(),
		}

		id, err := db.CreateQuery(query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create query: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created query '%s' with ID %d", args.Name, id)), nil
	})
}

func registerQueryExecute(s *server.MCPServer) {
	tool := mcp.NewTool("query-execute",
		mcp.WithDescription("Execute a saved query"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the query to execute"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		entries, err := db.ExecuteQuery(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Query execution failed: %v", err)), nil
		}

		result, err := json.Marshal(entries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerQueryList(s *server.MCPServer) {
	tool := mcp.NewTool("query-list",
		mcp.WithDescription("List all saved queries"),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		queries, err := db.ListQueries()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list queries: %v", err)), nil
		}

		result, err := json.Marshal(queries)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerQueryGet(s *server.MCPServer) {
	tool := mcp.NewTool("query-get",
		mcp.WithDescription("Get details of a saved query"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the query"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		query, err := db.GetQuery(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get query: %v", err)), nil
		}

		if query == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Query '%s' not found", args.Name)), nil
		}

		result, err := json.Marshal(query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerQueryUpdate(s *server.MCPServer) {
	tool := mcp.NewTool("query-update",
		mcp.WithDescription("Update a saved query"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the query to update"),
		),
		mcp.WithString("queryJSON",
			mcp.Required(),
			mcp.Description("Updated JSON string of the query filter"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name      string `json:"name"`
			QueryJSON string `json:"queryJSON"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Get existing query
		query, err := db.GetQuery(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get query: %v", err)), nil
		}

		if query == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Query '%s' not found", args.Name)), nil
		}

		// Update query JSON
		query.QueryJSON = args.QueryJSON
		query.UpdatedAt = time.Now().Unix()

		// Delete and recreate (simple approach)
		if err := db.DeleteQuery(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete old query: %v", err)), nil
		}

		if _, err := db.CreateQuery(query); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create updated query: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Updated query '%s'", args.Name)), nil
	})
}

func registerQueryDelete(s *server.MCPServer) {
	tool := mcp.NewTool("query-delete",
		mcp.WithDescription("Delete a saved query"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the query to delete"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.DeleteQuery(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete query: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted query '%s'", args.Name)), nil
	})
}

// Session Tools

func registerSessionInfo(s *server.MCPServer) {
	tool := mcp.NewTool("session-info",
		mcp.WithDescription("Get session information"),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info := map[string]interface{}{
			"database": dbPath,
			"version":  "0.1.0",
			"uptime":   "N/A", // Could track this if needed
		}

		result, err := json.Marshal(info)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal info: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerSessionSetPreferences(s *server.MCPServer) {
	tool := mcp.NewTool("session-set-preferences",
		mcp.WithDescription("Set session preferences"),
		mcp.WithString("preferences",
			mcp.Required(),
			mcp.Description("JSON string of preferences"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Preferences string `json:"preferences"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// For now, just acknowledge the preferences
		// In a full implementation, you'd store these in a session store
		return mcp.NewToolResultText("Preferences updated"), nil
	})
}

// Helper functions

func unmarshalArgs(arguments interface{}, v interface{}) error {
	// Convert arguments to JSON bytes
	data, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func splitPaths(pathsStr string) []string {
	// Simple comma split - in production, parse JSON array properly
	var paths []string
	current := ""
	for _, char := range pathsStr {
		if char == ',' {
			if current != "" {
				paths = append(paths, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		paths = append(paths, current)
	}
	return paths
}
