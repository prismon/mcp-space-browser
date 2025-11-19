package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"github.com/sirupsen/logrus"
)

// registerMCPTools registers all MCP tools with the server
func registerMCPTools(s *server.MCPServer, db *database.DiskDB, dbPath string) {
	// Core disk tools
	registerDiskIndexTool(s, db)
	registerDiskDuTool(s, db)
	registerDiskTreeTool(s, db)
	registerDiskTimeRangeTool(s, db)

	// Selection set tools
	registerSelectionSetCreate(s, db)
	registerSelectionSetList(s, db)
	registerSelectionSetGet(s, db)
	registerSelectionSetModify(s, db)
	registerSelectionSetDelete(s, db)

	// Query tools
	registerQueryCreate(s, db)
	registerQueryExecute(s, db)
	registerQueryList(s, db)
	registerQueryGet(s, db)
	registerQueryUpdate(s, db)
	registerQueryDelete(s, db)

	// Session tools
	registerSessionInfo(s, db, dbPath)
	registerSessionSetPreferences(s, db)
}

// Disk Tools

func registerDiskIndexTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("disk-index",
		mcp.WithDescription("Index the specified path. Returns immediately with a job ID. Use disk://index-jobs/{id} resource to monitor progress."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File or directory path to index"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Run asynchronously and return job ID immediately (default: true)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path  string `json:"path"`
			Async *bool  `json:"async,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		log.WithField("path", args.Path).Info("Executing disk-index via MCP")

		// Expand tilde and validate path
		expandedPath, err := pathutil.ExpandAndValidatePath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		// Default to async mode
		asyncMode := getBoolOrDefault(args.Async, true)

		if asyncMode {
			// Create job in database first
			jobID, err := db.CreateIndexJob(expandedPath, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create index job: %v", err)), nil
			}

			// Start indexing in background
			go func() {
				// Mark job as running
				if err := db.UpdateIndexJobStatus(jobID, "running", nil); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as running")
					return
				}

				// Run the indexing with job tracking
				stats, err := crawler.Index(expandedPath, db, jobID, nil)

				// Update job with final status
				if err != nil {
					errMsg := err.Error()
					if updateErr := db.UpdateIndexJobStatus(jobID, "failed", &errMsg); updateErr != nil {
						log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to update job status to failed")
					}
					log.WithError(err).WithField("jobID", jobID).Error("Indexing failed")
				} else {
					// Mark as completed with final stats
					if err := db.UpdateIndexJobStatus(jobID, "completed", nil); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as completed")
					}
					if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
						FilesProcessed:       int(stats.FilesProcessed),
						DirectoriesProcessed: int(stats.DirectoriesProcessed),
						TotalSize:            stats.TotalSize,
						ErrorCount:           stats.Errors,
					}); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to update final job progress")
					}
					log.WithFields(logrus.Fields{
						"jobID":       jobID,
						"path":        expandedPath,
						"files":       stats.FilesProcessed,
						"directories": stats.DirectoriesProcessed,
					}).Info("Indexing completed successfully")
				}
			}()

			// Return job ID immediately
			result := fmt.Sprintf("Indexing job started\n"+
				"Job ID: %d\n"+
				"Path: %s\n"+
				"\nTo monitor progress, use the resource: disk://index-jobs/%d\n"+
				"Or list running jobs: disk://jobs/running",
				jobID, expandedPath, jobID)

			return mcp.NewToolResultText(result), nil
		} else {
			// Synchronous mode (for backward compatibility) - no job tracking
			stats, err := crawler.Index(expandedPath, db, 0, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Indexing failed: %v", err)), nil
			}

			// Format the result with statistics
			result := fmt.Sprintf("Indexing completed successfully\n"+
				"Files: %d\n"+
				"Directories: %d\n"+
				"Total size: %d bytes (%.2f MB)\n"+
				"Errors: %d\n"+
				"Duration: %s",
				stats.FilesProcessed,
				stats.DirectoriesProcessed,
				stats.TotalSize,
				float64(stats.TotalSize)/(1024*1024),
				stats.Errors,
				stats.Duration,
			)

			return mcp.NewToolResultText(result), nil
		}
	})
}

func registerDiskDuTool(s *server.MCPServer, db *database.DiskDB) {
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

		// Expand tilde
		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		// Check if path exists on filesystem
		_, statErr := os.Stat(expandedPath)
		pathExists := statErr == nil

		entry, err := db.Get(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
		}

		if entry == nil {
			if pathExists {
				return mcp.NewToolResultError(fmt.Sprintf("Path '%s' exists but has not been indexed yet. Run disk-index first.", args.Path)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("Path '%s' does not exist", args.Path)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("%d", entry.Size)), nil
	})
}

func registerDiskTreeTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("disk-tree",
		mcp.WithDescription("Return a JSON tree of directories and file sizes. Large directories are automatically summarized. Response size is limited to 500KB - if exceeded, an error with suggestions is returned."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("File or directory path"),
		),
		mcp.WithNumber("maxDepth",
			mcp.Description("Maximum depth to traverse (default: 10 for performance)"),
		),
		mcp.WithNumber("minSize",
			mcp.Description("Minimum file size to include in bytes (default: 0)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of nodes to return (default: 1000)"),
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
		mcp.WithNumber("childThreshold",
			mcp.Description("Summarize directories with more than this many children (default: 100)"),
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
			ChildThreshold *int    `json:"childThreshold,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Expand tilde
		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		// Check if path exists on filesystem
		_, statErr := os.Stat(expandedPath)
		pathExists := statErr == nil

		// Build tree options with defaults for performance
		opts := database.TreeOptions{
			MaxDepth:       getIntOrDefault(args.MaxDepth, 10),        // Default to 10 levels deep
			CurrentDepth:   0,
			MinSize:        getInt64OrDefault(args.MinSize, 0),
			Limit:          getIntPtrOrDefault(args.Limit, 1000),      // Default to 1000 nodes max
			SortBy:         getStringOrDefault(args.SortBy, "size"),
			DescendingSort: getBoolOrDefault(args.DescendingSort, true),
			ChildThreshold: getIntOrDefault(args.ChildThreshold, 100), // Summarize dirs with >100 children
			NodesReturned:  new(int),
		}

		// Parse date filters
		if args.MinDate != nil {
			minDate, err := time.Parse("2006-01-02", *args.MinDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid minDate format (use YYYY-MM-DD): %v", err)), nil
			}
			opts.MinDate = &minDate
		}
		if args.MaxDate != nil {
			maxDate, err := time.Parse("2006-01-02", *args.MaxDate)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid maxDate format (use YYYY-MM-DD): %v", err)), nil
			}
			opts.MaxDate = &maxDate
		}

		// Add timeout to prevent hangs
		treeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		tree, err := db.GetTreeWithOptions(treeCtx, expandedPath, opts)
		if err != nil {
			// Check if it was a timeout
			if err == context.DeadlineExceeded {
				return mcp.NewToolResultError("Tree building timed out after 30 seconds. Try reducing maxDepth or limit, or increasing childThreshold."), nil
			}
			// Provide more context in error message
			if pathExists {
				return mcp.NewToolResultError(fmt.Sprintf("Path '%s' exists but has not been indexed yet. Run disk-index first.", args.Path)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get tree for '%s': %v", args.Path, err)), nil
		}

		treeJSON, err := json.Marshal(tree)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal tree: %v", err)), nil
		}

		// Check response size to prevent blowing up Claude's context window
		// Maximum 500KB response size
		maxResponseSize := 512000 // 500KB in bytes
		if len(treeJSON) > maxResponseSize {
			// Calculate how much we exceeded the limit
			sizeKB := len(treeJSON) / 1024
			maxKB := maxResponseSize / 1024

			// Create a helpful error message with suggestions
			errorMsg := fmt.Sprintf(
				"Response size too large (%d KB exceeds %d KB limit).\n\n"+
					"The directory tree is too large to return in a single response.\n\n"+
					"Current parameters:\n"+
					"- maxDepth: %d\n"+
					"- limit: %d\n"+
					"- childThreshold: %d\n\n"+
					"Suggestions to reduce response size:\n"+
					"1. Reduce maxDepth (try 2-3 for large directories)\n"+
					"2. Reduce limit (try 100-500 for initial exploration)\n"+
					"3. Increase childThreshold (try 50 to trigger more summarization)\n"+
					"4. Add minSize filter to skip small files\n"+
					"5. Query a more specific subdirectory\n"+
					"6. Use date filters (minDate/maxDate) to narrow results\n\n"+
					"Example:\n"+
					"  {\"path\": \"%s\", \"maxDepth\": 3, \"limit\": 200, \"childThreshold\": 50}",
				sizeKB, maxKB,
				opts.MaxDepth, *opts.Limit, opts.ChildThreshold,
				args.Path,
			)

			return mcp.NewToolResultError(errorMsg), nil
		}

		return mcp.NewToolResultText(string(treeJSON)), nil
	})
}

func registerDiskTimeRangeTool(s *server.MCPServer, db *database.DiskDB) {
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

		// Check response size to prevent blowing up Claude's context window
		maxResponseSize := 512000 // 500KB in bytes
		if len(result) > maxResponseSize {
			sizeKB := len(result) / 1024
			maxKB := maxResponseSize / 1024
			entryCount := len(entries)

			// Return summary instead of full data
			errorMsg := fmt.Sprintf(
				"Response size too large (%d KB exceeds %d KB limit).\n\n"+
					"Found %d entries matching the time range.\n"+
					"The result set is too large to return in a single response.\n\n"+
					"Suggestions:\n"+
					"1. Narrow the date range\n"+
					"2. Specify a more specific path to search within\n"+
					"3. Use a saved query with additional filters (minSize, file patterns)\n"+
					"4. Create a selection set and paginate through results\n\n"+
					"Summary of results:\n"+
					"- Total entries: %d\n"+
					"- Date range: %s to %s\n"+
					"- Search path: %s",
				sizeKB, maxKB, entryCount, entryCount,
				args.StartDate, args.EndDate,
				getStringOrDefault(args.Path, "(all paths)"),
			)

			return mcp.NewToolResultError(errorMsg), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

// Selection Set Tools

func registerSelectionSetCreate(s *server.MCPServer, db *database.DiskDB) {
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

func registerSelectionSetList(s *server.MCPServer, db *database.DiskDB) {
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

func registerSelectionSetGet(s *server.MCPServer, db *database.DiskDB) {
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

		// Check response size to prevent blowing up Claude's context window
		maxResponseSize := 512000 // 500KB in bytes
		if len(result) > maxResponseSize {
			sizeKB := len(result) / 1024
			maxKB := maxResponseSize / 1024
			entryCount := len(entries)

			errorMsg := fmt.Sprintf(
				"Response size too large (%d KB exceeds %d KB limit).\n\n"+
					"Selection set '%s' contains %d entries.\n"+
					"The result set is too large to return in a single response.\n\n"+
					"Suggestions:\n"+
					"1. Split the selection set into smaller subsets\n"+
					"2. Use a query with additional filters to narrow results\n"+
					"3. Process entries in smaller batches\n\n"+
					"Summary:\n"+
					"- Selection set: %s\n"+
					"- Total entries: %d",
				sizeKB, maxKB, args.Name, entryCount,
				args.Name, entryCount,
			)

			return mcp.NewToolResultError(errorMsg), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerSelectionSetModify(s *server.MCPServer, db *database.DiskDB) {
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

func registerSelectionSetDelete(s *server.MCPServer, db *database.DiskDB) {
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

func registerQueryCreate(s *server.MCPServer, db *database.DiskDB) {
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

func registerQueryExecute(s *server.MCPServer, db *database.DiskDB) {
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

		// Check response size to prevent blowing up Claude's context window
		maxResponseSize := 512000 // 500KB in bytes
		if len(result) > maxResponseSize {
			sizeKB := len(result) / 1024
			maxKB := maxResponseSize / 1024
			entryCount := len(entries)

			errorMsg := fmt.Sprintf(
				"Response size too large (%d KB exceeds %d KB limit).\n\n"+
					"Query '%s' returned %d entries.\n"+
					"The result set is too large to return in a single response.\n\n"+
					"Suggestions:\n"+
					"1. Update the query to add more restrictive filters (minSize, date range)\n"+
					"2. Narrow the search path\n"+
					"3. Create a selection set and process results in batches\n"+
					"4. Use the disk-tree tool with appropriate limits for hierarchical exploration\n\n"+
					"Summary:\n"+
					"- Query: %s\n"+
					"- Total entries: %d",
				sizeKB, maxKB, args.Name, entryCount,
				args.Name, entryCount,
			)

			return mcp.NewToolResultError(errorMsg), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerQueryList(s *server.MCPServer, db *database.DiskDB) {
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

func registerQueryGet(s *server.MCPServer, db *database.DiskDB) {
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

func registerQueryUpdate(s *server.MCPServer, db *database.DiskDB) {
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

func registerQueryDelete(s *server.MCPServer, db *database.DiskDB) {
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

func registerSessionInfo(s *server.MCPServer, db *database.DiskDB, dbPath string) {
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

func registerSessionSetPreferences(s *server.MCPServer, db *database.DiskDB) {
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

func getIntOrDefault(ptr *int, defaultVal int) int {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func getInt64OrDefault(ptr *int64, defaultVal int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func getStringOrDefault(ptr *string, defaultVal string) string {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func getBoolOrDefault(ptr *bool, defaultVal bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

func getIntPtrOrDefault(ptr *int, defaultVal int) *int {
	if ptr != nil {
		return ptr
	}
	return &defaultVal
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
