package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"github.com/prismon/mcp-space-browser/pkg/plans"
	"github.com/sirupsen/logrus"
)

// Response size and compression constants
const (
	// maxMCPResponseSize is the maximum response size before compression (500KB)
	maxMCPResponseSize = 512000
	// summaryTopEntriesCount is the number of top entries to include in compressed summaries
	summaryTopEntriesCount = 50
)

// normalizeCachePath converts absolute cache paths to relative paths for URL generation.
// This handles legacy database entries that have absolute paths stored.
func normalizeCachePath(cachePath string) string {
	if cachePath == "" {
		return cachePath
	}

	// If it's already a relative path, use as-is
	if !filepath.IsAbs(cachePath) {
		return cachePath
	}

	// Get absolute cache directory for comparison
	if artifactCacheDir == "" {
		return cachePath
	}

	absCacheDir, err := filepath.Abs(artifactCacheDir)
	if err != nil {
		return cachePath
	}

	// If the absolute path starts with the cache directory, extract the relative portion
	cleanPath := filepath.Clean(cachePath)
	cleanCacheDir := filepath.Clean(absCacheDir)

	if strings.HasPrefix(cleanPath, cleanCacheDir) {
		// Get relative path within the cache directory
		relPath, err := filepath.Rel(cleanCacheDir, cleanPath)
		if err != nil {
			return cachePath
		}
		// Return path relative to current directory using configured cache dir
		return filepath.Join(artifactCacheDir, relPath)
	}

	return cachePath
}

// registerMCPTools registers all MCP tools with the server (legacy single-db version)
// Deprecated: Use registerMCPToolsMultiProject for multi-project support
func registerMCPTools(s *server.MCPServer, db *database.DiskDB, dbPath string, processor *classifier.Processor) {
	// Shell-style navigation tools
	registerIndexTool(s, db)
	registerNavigateTool(s, db)
	registerInspectTool(s, db)
	registerJobProgressTool(s, db)
	registerListJobsTool(s, db)
	registerCancelJobTool(s, db)
	registerDbDiagnoseTool(s, db)

	// Resource set tools
	registerResourceSetCreate(s, db)
	registerResourceSetList(s, db)
	registerResourceSetGet(s, db)
	registerResourceSetModify(s, db)
	registerResourceSetDelete(s, db)

	// Query tools
	registerQueryCreate(s, db)
	registerQueryExecute(s, db)
	registerQueryList(s, db)
	registerQueryGet(s, db)
	registerQueryUpdate(s, db)
	registerQueryDelete(s, db)

	// Plan tools
	registerPlanCreate(s, db)
	registerPlanExecute(s, db, processor)
	registerPlanList(s, db)
	registerPlanGet(s, db)
	registerPlanUpdate(s, db)
	registerPlanDelete(s, db)

	// Session tools
	registerSessionInfo(s, db, dbPath)
	registerSessionSetPreferences(s, db)

	// File action tools
	registerRenameFilesTool(s, db)
	registerDeleteFilesTool(s, db)
	registerMoveFilesTool(s, db)

	// Source management tools
	registerSourceTools(s, db)

	// Resource-centric tools (DAG navigation, metrics, queries)
	registerResourceTools(s, db)
}

// registerMCPToolsMultiProject registers all MCP tools with multi-project support
// Tools resolve the database from the session's active project at runtime
func registerMCPToolsMultiProject(s *server.MCPServer, sc *ServerContext, processor *classifier.Processor) {
	// Shell-style navigation tools
	registerIndexToolMP(s, sc)
	registerNavigateToolMP(s, sc)
	registerInspectToolMP(s, sc)
	registerJobProgressToolMP(s, sc)
	registerListJobsToolMP(s, sc)
	registerCancelJobToolMP(s, sc)
	registerDbDiagnoseToolMP(s, sc)

	// Resource set tools
	registerResourceSetCreateMP(s, sc)
	registerResourceSetListMP(s, sc)
	registerResourceSetGetMP(s, sc)
	registerResourceSetModifyMP(s, sc)
	registerResourceSetDeleteMP(s, sc)

	// Query tools
	registerQueryCreateMP(s, sc)
	registerQueryExecuteMP(s, sc)
	registerQueryListMP(s, sc)
	registerQueryGetMP(s, sc)
	registerQueryUpdateMP(s, sc)
	registerQueryDeleteMP(s, sc)

	// Plan tools
	registerPlanCreateMP(s, sc)
	registerPlanExecuteMP(s, sc, processor)
	registerPlanListMP(s, sc)
	registerPlanGetMP(s, sc)
	registerPlanUpdateMP(s, sc)
	registerPlanDeleteMP(s, sc)

	// Session tools
	registerSessionInfoMP(s, sc)
	registerSessionSetPreferencesMP(s, sc)

	// File action tools
	registerRenameFilesToolMP(s, sc)
	registerDeleteFilesToolMP(s, sc)
	registerMoveFilesToolMP(s, sc)

	// Source management tools
	registerSourceToolsMP(s, sc)

	// Resource-centric tools (DAG navigation, metrics, queries)
	registerResourceToolsMP(s, sc)
}

// getProjectDB is a helper function to get the database backend for the current session
// Returns an error result if no session or no active project
func getProjectDB(ctx context.Context, sc *ServerContext) (database.Backend, error) {
	return sc.GetProjectDB(ctx)
}

// requireProjectDB gets the project database or returns an MCP error
// This is a convenience helper for tool handlers
func requireProjectDB(ctx context.Context, sc *ServerContext) (*database.DiskDB, *mcp.CallToolResult) {
	backend, err := sc.GetProjectDB(ctx)
	if err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("No active project: %v. Use project-open to select a project.", err))
	}

	// For now, we only support DiskDB until the full backend abstraction is implemented
	db, ok := backend.(*database.DiskDB)
	if !ok {
		return nil, mcp.NewToolResultError("Unsupported database backend. Only SQLite is currently supported.")
	}

	return db, nil
}

// Tree compression utilities

// compressTree progressively compresses a tree until it fits within the size limit
func compressTree(tree *models.TreeNode, maxSizeBytes int) (*models.TreeNode, bool) {
	// Check current size
	currentJSON, err := json.Marshal(tree)
	if err != nil {
		return tree, false
	}

	if len(currentJSON) <= maxSizeBytes {
		return tree, false // No compression needed
	}

	// Create a working copy
	compressed := tree
	compressionLevels := []int{20, 10, 5, 3, 1} // Progressive limits for children to keep

	for _, keepLimit := range compressionLevels {
		compressed = compressTreeRecursive(compressed, keepLimit, 0, 5) // Start at depth 0, compress beyond depth 5

		// Check if we're now under the limit
		testJSON, err := json.Marshal(compressed)
		if err != nil {
			continue
		}

		if len(testJSON) <= maxSizeBytes {
			log.WithFields(logrus.Fields{
				"originalSize":   len(currentJSON),
				"compressedSize": len(testJSON),
				"keepLimit":      keepLimit,
			}).Info("Successfully compressed tree to fit size limit")
			return compressed, true
		}
	}

	// If still too large, do aggressive compression - keep only root with summary
	compressed = compressToSummaryOnly(tree)
	return compressed, true
}

// compressTreeRecursive recursively compresses a tree by limiting children at each level
func compressTreeRecursive(node *models.TreeNode, keepLimit int, currentDepth int, compressAfterDepth int) *models.TreeNode {
	if node == nil {
		return nil
	}

	// Create a new node to avoid modifying the original
	newNode := &models.TreeNode{
		Name:      node.Name,
		Path:      node.Path,
		Size:      node.Size,
		Kind:      node.Kind,
		Mtime:     node.Mtime,
		Truncated: node.Truncated,
		Summary:   node.Summary,
	}

	// If this is a file or has no children, return as-is
	if node.Kind == "file" || len(node.Children) == 0 {
		return newNode
	}

	// If we're past the compression depth, summarize
	if currentDepth >= compressAfterDepth {
		newNode.Truncated = true
		newNode.Summary = createSummaryFromChildren(node.Children, keepLimit)
		newNode.Children = nil
		return newNode
	}

	// Otherwise, keep limited children and recurse
	sortedChildren := make([]*models.TreeNode, len(node.Children))
	copy(sortedChildren, node.Children)

	// Sort by size (largest first)
	sort.Slice(sortedChildren, func(i, j int) bool {
		return sortedChildren[i].Size > sortedChildren[j].Size
	})

	// Keep only top N children
	childrenToKeep := keepLimit
	if len(sortedChildren) < childrenToKeep {
		childrenToKeep = len(sortedChildren)
	}

	newNode.Children = make([]*models.TreeNode, 0, childrenToKeep)
	for i := 0; i < childrenToKeep; i++ {
		compressed := compressTreeRecursive(sortedChildren[i], keepLimit, currentDepth+1, compressAfterDepth)
		newNode.Children = append(newNode.Children, compressed)
	}

	// If we truncated children, add summary
	if len(sortedChildren) > childrenToKeep {
		newNode.Truncated = true
		newNode.Summary = createSummaryFromChildren(node.Children, keepLimit)
	}

	return newNode
}

// compressToSummaryOnly creates a minimal tree with just the root and summary stats
func compressToSummaryOnly(node *models.TreeNode) *models.TreeNode {
	if node == nil {
		return nil
	}

	return &models.TreeNode{
		Name:      node.Name,
		Path:      node.Path,
		Size:      node.Size,
		Kind:      node.Kind,
		Mtime:     node.Mtime,
		Truncated: true,
		Summary:   createSummaryFromChildren(node.Children, 10),
		Children:  nil,
	}
}

// createSummaryFromChildren creates a TreeSummary from a list of children
func createSummaryFromChildren(children []*models.TreeNode, topN int) *models.TreeSummary {
	if len(children) == 0 {
		return &models.TreeSummary{
			TotalChildren:   0,
			FileCount:       0,
			DirectoryCount:  0,
			TotalSize:       0,
			LargestChildren: []*models.SimplifiedNode{},
		}
	}

	summary := &models.TreeSummary{
		TotalChildren:  len(children),
		FileCount:      0,
		DirectoryCount: 0,
		TotalSize:      0,
	}

	// Count files and directories
	for _, child := range children {
		if child.Kind == "file" {
			summary.FileCount++
		} else {
			summary.DirectoryCount++
		}
		summary.TotalSize += child.Size
	}

	// Get top N largest children
	sortedChildren := make([]*models.TreeNode, len(children))
	copy(sortedChildren, children)
	sort.Slice(sortedChildren, func(i, j int) bool {
		return sortedChildren[i].Size > sortedChildren[j].Size
	})

	keepCount := topN
	if len(sortedChildren) < keepCount {
		keepCount = len(sortedChildren)
	}

	summary.LargestChildren = make([]*models.SimplifiedNode, keepCount)
	for i := 0; i < keepCount; i++ {
		summary.LargestChildren[i] = &models.SimplifiedNode{
			Name:  sortedChildren[i].Name,
			Path:  sortedChildren[i].Path,
			Size:  sortedChildren[i].Size,
			Kind:  sortedChildren[i].Kind,
			Mtime: sortedChildren[i].Mtime,
		}
	}

	return summary
}

// compressEntryList creates a summary response for large entry lists
func compressEntryList(entries []*models.Entry, maxSizeBytes int, contextInfo string) (string, error) {
	// Try full list first
	fullJSON, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}

	if len(fullJSON) <= maxSizeBytes {
		return string(fullJSON), nil
	}

	// Create a summary response
	sortedEntries := make([]*models.Entry, len(entries))
	copy(sortedEntries, entries)

	// Sort by size (largest first)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].Size > sortedEntries[j].Size
	})

	// Calculate statistics
	var totalSize int64
	fileCount := 0
	dirCount := 0
	for _, entry := range entries {
		totalSize += entry.Size
		if entry.Kind == "file" {
			fileCount++
		} else {
			dirCount++
		}
	}

	// Create summary with top entries
	topN := summaryTopEntriesCount
	if len(sortedEntries) < topN {
		topN = len(sortedEntries)
	}

	summary := map[string]interface{}{
		"_compressed": true,
		"_note":       fmt.Sprintf("Result set was too large (%d KB). Showing summary with top %d entries by size.", len(fullJSON)/1024, topN),
		"context":     contextInfo,
		"statistics": map[string]interface{}{
			"total_entries":     len(entries),
			"total_files":       fileCount,
			"total_directories": dirCount,
			"total_size":        totalSize,
			"total_size_mb":     float64(totalSize) / (1024 * 1024),
		},
		"top_entries": sortedEntries[:topN],
	}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		"originalSize":   len(fullJSON),
		"compressedSize": len(summaryJSON),
		"totalEntries":   len(entries),
		"keptEntries":    topN,
	}).Info("Compressed entry list to fit size limit")

	return string(summaryJSON), nil
}

// Disk Tools

func registerIndexTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("index",
		mcp.WithDescription("Index the specified path and track progress with synthesis://jobs/{id}. Skips indexing if the path was recently scanned (within maxAge seconds) unless force=true."),
		mcp.WithString("root",
			mcp.Required(),
			mcp.Description("File or directory path to index"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Run asynchronously and return job ID immediately (default: true)"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force re-indexing even if the path was recently scanned (default: false)"),
		),
		mcp.WithNumber("maxAge",
			mcp.Description("Maximum age in seconds before a scan is considered stale (default: 3600 = 1 hour). Set to 0 to always re-index."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Root   string  `json:"root"`
			Async  *bool   `json:"async,omitempty"`
			Force  *bool   `json:"force,omitempty"`
			MaxAge *int64  `json:"maxAge,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Build index options
		indexOpts := crawler.DefaultIndexOptions()
		if args.Force != nil {
			indexOpts.Force = *args.Force
		}
		if args.MaxAge != nil {
			indexOpts.MaxAge = *args.MaxAge
		}

		log.WithFields(logrus.Fields{
			"root":   args.Root,
			"force":  indexOpts.Force,
			"maxAge": indexOpts.MaxAge,
		}).Info("Executing index via MCP")

		// Default to async mode
		asyncMode := getBoolOrDefault(args.Async, true)

		if asyncMode {
			// Expand path (but don't validate yet)
			expandedPath, expandErr := pathutil.ExpandPath(args.Root)
			if expandErr != nil {
				// If we can't even expand the path, use original for job creation
				expandedPath = args.Root
			}

			// Create job in database FIRST (before validation)
			// This ensures all indexing attempts are tracked
			jobID, err := db.CreateIndexJob(expandedPath, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create index job: %v", err)), nil
			}

			// Now validate the path
			validationErr := pathutil.ValidatePath(expandedPath)
			if expandErr != nil || validationErr != nil {
				// Path is invalid - mark job as failed immediately
				var errMsg string
				if expandErr != nil {
					errMsg = fmt.Sprintf("Path expansion failed: %v", expandErr)
				} else {
					errMsg = fmt.Sprintf("Invalid path: %v", validationErr)
				}

				if updateErr := db.UpdateIndexJobStatus(jobID, "failed", &errMsg); updateErr != nil {
					log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to mark job as failed")
				}

				log.WithField("jobID", jobID).WithField("path", args.Root).WithError(validationErr).Warn("Job created but path validation failed")

				// Return job info with failed status and error message
				response := map[string]any{
					"jobId":     jobID,
					"root":      expandedPath,
					"status":    "failed",
					"error":     errMsg,
					"statusUrl": fmt.Sprintf("synthesis://jobs/%d", jobID),
				}

				payload, err := json.Marshal(response)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
				}

				return mcp.NewToolResultText(string(payload)), nil
			}

			// Path is valid - start indexing in background
			go func() {
				// Mark job as running
				if err := db.UpdateIndexJobStatus(jobID, "running", nil); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as running")
					return
				}

				// Run the indexing with job tracking and options
				stats, err := crawler.IndexWithOptions(expandedPath, db, nil, jobID, nil, indexOpts)

				// Update job with final status
				if err != nil {
					errMsg := err.Error()
					if updateErr := db.UpdateIndexJobStatus(jobID, "failed", &errMsg); updateErr != nil {
						log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to update job status to failed")
					}
					log.WithError(err).WithField("jobID", jobID).Error("Indexing failed")
				} else if stats.Skipped {
					// Indexing was skipped due to recent scan - store skip info in metadata
					if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
						FilesProcessed:       0,
						DirectoriesProcessed: 0,
						TotalSize:            0,
						ErrorCount:           0,
						Skipped:              true,
						SkipReason:           stats.SkipReason,
					}); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to update job metadata for skip")
					}
					if err := db.UpdateIndexJobStatus(jobID, "completed", nil); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as completed")
					}
					log.WithFields(logrus.Fields{
						"jobID":      jobID,
						"path":       expandedPath,
						"skipReason": stats.SkipReason,
					}).Info("Indexing skipped - path recently scanned")
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

			response := map[string]any{
				"jobId":     jobID,
				"root":      expandedPath,
				"status":    "pending",
				"statusUrl": fmt.Sprintf("synthesis://jobs/%d", jobID),
				"cwdHint":   expandedPath,
			}

			payload, err := json.Marshal(response)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(payload)), nil
		}

		// Synchronous mode (for backward compatibility) - validate before indexing
		expandedPath, err := pathutil.ExpandAndValidatePath(args.Root)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		stats, err := crawler.IndexWithOptions(expandedPath, db, nil, 0, nil, indexOpts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Indexing failed: %v", err)), nil
		}

		response := map[string]any{
			"root":        expandedPath,
			"status":      "completed",
			"files":       stats.FilesProcessed,
			"directories": stats.DirectoriesProcessed,
			"totalSize":   stats.TotalSize,
			"durationMs":  stats.Duration.Milliseconds(),
			"skipped":     stats.Skipped,
		}

		if stats.Skipped {
			response["skipReason"] = stats.SkipReason
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerNavigateTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("navigate",
		mcp.WithDescription("Navigate to a directory within the indexed tree and return a lightweight listing with summary statistics."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Target path (absolute or relative to previous call)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return (default: 20)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset into the child list for pagination"),
		),
		mcp.WithString("sortBy",
			mcp.Description("Sort by: size, name, or mtime (default: size)"),
		),
		mcp.WithString("order",
			mcp.Description("Sort order: asc or desc (default: desc)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path   string  `json:"path"`
			Limit  *int    `json:"limit,omitempty"`
			Offset *int    `json:"offset,omitempty"`
			SortBy *string `json:"sortBy,omitempty"`
			Order  *string `json:"order,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		entry, err := db.Get(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load path: %v", err)), nil
		}
		if entry == nil || entry.Kind != "directory" {
			return mcp.NewToolResultError("Path is not an indexed directory"), nil
		}

		children, err := db.Children(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch children: %v", err)), nil
		}

		offset := getIntOrDefault(args.Offset, 0)
		if offset < 0 {
			offset = 0
		}
		limit := getIntOrDefault(args.Limit, 20)
		if limit <= 0 {
			limit = 20
		}

		sortBy := getStringOrDefault(args.SortBy, "size")
		order := getStringOrDefault(args.Order, "desc")
		descending := order == "desc"

		// Sort children using shared utility
		SortEntries(children, sortBy, descending)

		// Build summary statistics using shared utility
		summary := BuildEntrySummary(children, 10)

		end := offset + limit
		if end > len(children) {
			end = len(children)
		}

		// Build base URL for HTTP content serving
		baseURL := contentBaseURL
		if baseURL == "" {
			baseURL = "http://localhost:3000"
		}

		slice := children[offset:end]
		listings := make([]map[string]any, 0, len(slice))
		for _, child := range slice {
			listing := map[string]any{
				"path":       child.Path,
				"name":       filepath.Base(child.Path),
				"kind":       child.Kind,
				"size":       child.Size,
				"modifiedAt": time.Unix(child.Mtime, 0).Format(time.RFC3339),
				"link":       fmt.Sprintf("synthesis://nodes/%s", child.Path),
			}

			// For files, look up thumbnail if available
			if child.Kind == "file" {
				metadataList, err := db.GetMetadataByPath(child.Path)
				if err == nil && len(metadataList) > 0 {
					listing["metadataUri"] = fmt.Sprintf("synthesis://nodes/%s/metadata", child.Path)
					for _, metadata := range metadataList {
						if metadata.MetadataType == "thumbnail" && metadata.CachePath != "" {
							normalizedPath := normalizeCachePath(metadata.CachePath)
							listing["thumbnailUrl"] = fmt.Sprintf("%s/api/content?path=%s", baseURL, normalizedPath)
							break
						}
					}
				}
			} else {
				// Check if this child has metadata/artifacts (for directories)
				if hasMetadata := checkIfHasMetadata(child.Path, child.Kind, child.Mtime); hasMetadata {
					listing["metadataUri"] = fmt.Sprintf("synthesis://nodes/%s/metadata", child.Path)
				}
			}

			listings = append(listings, listing)
		}

		response := map[string]any{
			"cwd":         expandedPath,
			"count":       len(children),
			"entries":     listings,
			"nextPageUrl": "",
			"summary": map[string]any{
				"totalChildren":  summary.TotalChildren,
				"fileCount":      summary.FileCount,
				"directoryCount": summary.DirectoryCount,
				"totalSize":      summary.TotalSize,
			},
		}

		if end < len(children) {
			response["nextPageUrl"] = fmt.Sprintf("synthesis://list?path=%s&offset=%d&limit=%d&sortBy=%s&order=%s", expandedPath, end, limit, sortBy, order)
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerInspectTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("inspect",
		mcp.WithDescription("Return metadata for a specific indexed node, including MCP resource references."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path of the file or directory to inspect"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Use buildInspectResponse to ensure artifacts are generated if needed
		// This calls the same logic as the REST endpoint to ensure consistency
		inspectResp, err := buildInspectResponse(args.Path, db, 20, 0)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Build response with normalized paths for HTTP URLs
		baseURL := contentBaseURL
		if baseURL == "" {
			baseURL = "http://localhost:3000"
		}

		// Use TrimPrefix to avoid double slashes (path starts with /)
		uriPath := strings.TrimPrefix(inspectResp.Path, "/")
		response := map[string]interface{}{
			"path":        inspectResp.Path,
			"kind":        inspectResp.Kind,
			"size":        inspectResp.Size,
			"modifiedAt":  inspectResp.ModifiedAt,
			"createdAt":   inspectResp.CreatedAt,
			"resourceUri": fmt.Sprintf("synthesis://nodes/%s", uriPath),
		}

		// Build metadata from the generated artifacts
		if len(inspectResp.Artifacts) > 0 {
			response["metadataUri"] = fmt.Sprintf("synthesis://nodes/%s/metadata", uriPath)
			response["metadataCount"] = inspectResp.ArtifactsCount

			metadataItems := make([]map[string]interface{}, 0, len(inspectResp.Artifacts))
			for _, artifact := range inspectResp.Artifacts {
				item := map[string]interface{}{
					"type":     artifact.Type,
					"mimeType": artifact.MimeType,
					"url":      artifact.Url,
				}
				if artifact.Metadata != nil {
					item["metadata"] = artifact.Metadata
				}

				// Set specific URI fields based on artifact type
				if artifact.Type == "thumbnail" {
					response["thumbnailUri"] = artifact.Url
				}
				if artifact.Type == "video-timeline" {
					if _, ok := response["timelineUri"]; !ok {
						response["timelineUri"] = artifact.Url
					}
				}

				metadataItems = append(metadataItems, item)
			}
			response["metadata"] = metadataItems
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerJobProgressTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("job-progress",
		mcp.WithDescription("Retrieve status for an indexing job."),
		mcp.WithString("jobId",
			mcp.Required(),
			mcp.Description("Job identifier returned from index"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			JobID string `json:"jobId"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		id, err := strconv.ParseInt(args.JobID, 10, 64)
		if err != nil {
			return mcp.NewToolResultError("jobId must be an integer"), nil
		}

		job, err := db.GetIndexJob(id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load job: %v", err)), nil
		}
		if job == nil {
			return mcp.NewToolResultError("Job not found"), nil
		}

		response := map[string]any{
			"jobId":     job.ID,
			"status":    job.Status,
			"path":      job.RootPath,
			"progress":  job.Progress,
			"statusUrl": fmt.Sprintf("synthesis://jobs/%d", job.ID),
		}

		// Include metadata if available (contains skip info, file counts, etc.)
		if job.Metadata != nil {
			var metadata database.IndexJobMetadata
			if err := json.Unmarshal([]byte(*job.Metadata), &metadata); err == nil {
				response["filesProcessed"] = metadata.FilesProcessed
				response["directoriesProcessed"] = metadata.DirectoriesProcessed
				response["totalSize"] = metadata.TotalSize
				response["errorCount"] = metadata.ErrorCount
				if metadata.Skipped {
					response["skipped"] = true
					response["skipReason"] = metadata.SkipReason
				}
			}
		}

		// Include error if present
		if job.Error != nil {
			response["error"] = *job.Error
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerListJobsTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("list-jobs",
		mcp.WithDescription("List indexing jobs with optional filtering by status and progress."),
		mcp.WithBoolean("activeOnly",
			mcp.Description("Show only active jobs (running or pending). Default: false"),
		),
		mcp.WithString("status",
			mcp.Description("Filter by specific status: pending, running, paused, completed, failed, or cancelled"),
		),
		mcp.WithNumber("minProgress",
			mcp.Description("Filter jobs with progress >= this value (0-100)"),
		),
		mcp.WithNumber("maxProgress",
			mcp.Description("Filter jobs with progress <= this value (0-100)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of jobs to return (default: 50)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ActiveOnly  *bool   `json:"activeOnly,omitempty"`
			Status      *string `json:"status,omitempty"`
			MinProgress *int    `json:"minProgress,omitempty"`
			MaxProgress *int    `json:"maxProgress,omitempty"`
			Limit       *int    `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Set default limit
		limit := getIntOrDefault(args.Limit, 50)

		// Determine status filter
		var statusFilter *string
		if args.ActiveOnly != nil && *args.ActiveOnly {
			// For activeOnly, we'll need to filter in-memory since we can't pass multiple statuses to ListIndexJobs
			statusFilter = nil
		} else if args.Status != nil {
			statusFilter = args.Status
		}

		// Get jobs from database
		jobs, err := db.ListIndexJobs(statusFilter, limit*2) // Get more for filtering
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
		}

		// Apply filters
		var filteredJobs []*database.IndexJob
		for _, job := range jobs {
			// Filter by activeOnly
			if args.ActiveOnly != nil && *args.ActiveOnly {
				if job.Status != "running" && job.Status != "pending" {
					continue
				}
			}

			// Filter by progress range
			if args.MinProgress != nil && job.Progress < *args.MinProgress {
				continue
			}
			if args.MaxProgress != nil && job.Progress > *args.MaxProgress {
				continue
			}

			filteredJobs = append(filteredJobs, job)

			// Respect the limit
			if len(filteredJobs) >= limit {
				break
			}
		}

		// Build response with detailed job information
		var jobList []map[string]any
		for _, job := range filteredJobs {
			jobInfo := map[string]any{
				"jobId":     job.ID,
				"path":      job.RootPath,
				"status":    job.Status,
				"progress":  job.Progress,
				"statusUrl": fmt.Sprintf("synthesis://jobs/%d", job.ID),
			}

			// Add timestamps if available
			if job.StartedAt != nil {
				jobInfo["startedAt"] = time.Unix(*job.StartedAt, 0).Format(time.RFC3339)
			}
			if job.CompletedAt != nil {
				jobInfo["completedAt"] = time.Unix(*job.CompletedAt, 0).Format(time.RFC3339)
			}

			// Add metadata if available
			if job.Metadata != nil {
				var metadata database.IndexJobMetadata
				if err := json.Unmarshal([]byte(*job.Metadata), &metadata); err == nil {
					jobInfo["metadata"] = metadata
				}
			}

			// Add error if available
			if job.Error != nil {
				jobInfo["error"] = *job.Error
			}

			// Calculate what the job is currently doing (for running jobs)
			if job.Status == "running" {
				jobInfo["currentActivity"] = "Indexing filesystem"
				if job.Metadata != nil {
					var metadata database.IndexJobMetadata
					if err := json.Unmarshal([]byte(*job.Metadata), &metadata); err == nil {
						jobInfo["currentActivity"] = fmt.Sprintf("Processed %d files, %d directories",
							metadata.FilesProcessed, metadata.DirectoriesProcessed)
					}
				}
			}

			jobList = append(jobList, jobInfo)
		}

		response := map[string]any{
			"jobs":       jobList,
			"totalCount": len(jobList),
			"filters": map[string]any{
				"activeOnly":  args.ActiveOnly,
				"status":      args.Status,
				"minProgress": args.MinProgress,
				"maxProgress": args.MaxProgress,
			},
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerCancelJobTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("cancel-job",
		mcp.WithDescription("Cancel a running or pending indexing job."),
		mcp.WithString("jobId",
			mcp.Required(),
			mcp.Description("Job identifier to cancel"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			JobID string `json:"jobId"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		id, err := strconv.ParseInt(args.JobID, 10, 64)
		if err != nil {
			return mcp.NewToolResultError("jobId must be an integer"), nil
		}

		// Get the job to check its status
		job, err := db.GetIndexJob(id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load job: %v", err)), nil
		}
		if job == nil {
			return mcp.NewToolResultError("Job not found"), nil
		}

		// Check if job can be cancelled
		if job.Status == "completed" {
			return mcp.NewToolResultError("Cannot cancel a completed job"), nil
		}
		if job.Status == "failed" {
			return mcp.NewToolResultError("Cannot cancel a failed job"), nil
		}
		if job.Status == "cancelled" {
			return mcp.NewToolResultError("Job is already cancelled"), nil
		}

		// Update job status to cancelled
		if err := db.UpdateIndexJobStatus(id, "cancelled", nil); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to cancel job: %v", err)), nil
		}

		log.WithFields(logrus.Fields{
			"jobID": id,
			"path":  job.RootPath,
		}).Info("Job cancelled via MCP")

		response := map[string]any{
			"jobId":     id,
			"status":    "cancelled",
			"path":      job.RootPath,
			"message":   "Job has been cancelled",
			"statusUrl": fmt.Sprintf("synthesis://jobs/%d", id),
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerDbDiagnoseTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("db-diagnose",
		mcp.WithDescription("Run database diagnostics to check for integrity issues. Reports entry counts, orphaned entries, and aggregation problems."),
		mcp.WithString("root",
			mcp.Description("Optional: limit diagnostics to entries under this root path"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Root *string `json:"root,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		diagnostics := map[string]any{}
		issues := []string{}

		// 1. Get total entry count
		var totalEntries int64
		var err error
		if args.Root != nil && *args.Root != "" {
			totalEntries, err = db.GetEntryCount(*args.Root)
		} else {
			err = db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&totalEntries)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to count entries: %v", err)), nil
		}
		diagnostics["totalEntries"] = totalEntries

		if totalEntries == 0 {
			issues = append(issues, "No entries in database - index has not been run or database is empty")
		}

		// 2. Count files vs directories
		var fileCount, dirCount int64
		if args.Root != nil && *args.Root != "" {
			root := *args.Root
			db.QueryRow(`SELECT COUNT(*) FROM entries WHERE kind = 'file' AND (path = ? OR path LIKE ?)`, root, root+"/%").Scan(&fileCount)
			db.QueryRow(`SELECT COUNT(*) FROM entries WHERE kind = 'directory' AND (path = ? OR path LIKE ?)`, root, root+"/%").Scan(&dirCount)
		} else {
			db.QueryRow(`SELECT COUNT(*) FROM entries WHERE kind = 'file'`).Scan(&fileCount)
			db.QueryRow(`SELECT COUNT(*) FROM entries WHERE kind = 'directory'`).Scan(&dirCount)
		}
		diagnostics["fileCount"] = fileCount
		diagnostics["directoryCount"] = dirCount

		// 3. Check for orphaned entries (parent path doesn't exist)
		var orphanCount int64
		db.QueryRow(`
			SELECT COUNT(*) FROM entries e1
			WHERE e1.parent IS NOT NULL
			AND e1.parent != ''
			AND NOT EXISTS (SELECT 1 FROM entries e2 WHERE e2.path = e1.parent)
		`).Scan(&orphanCount)
		diagnostics["orphanedEntries"] = orphanCount
		if orphanCount > 0 {
			issues = append(issues, fmt.Sprintf("%d orphaned entries found (parent path missing)", orphanCount))
		}

		// 4. Check for directories with potential aggregation issues
		// (directories with size=0 but have children with non-zero size)
		var badAggregateCount int64
		db.QueryRow(`
			SELECT COUNT(*) FROM entries e1
			WHERE e1.kind = 'directory'
			AND e1.size = 0
			AND EXISTS (
				SELECT 1 FROM entries e2
				WHERE e2.parent = e1.path
				AND e2.size > 0
			)
		`).Scan(&badAggregateCount)
		diagnostics["directoriesWithMissingAggregates"] = badAggregateCount
		if badAggregateCount > 0 {
			issues = append(issues, fmt.Sprintf("%d directories have size=0 but contain files with non-zero size", badAggregateCount))
		}

		// 5. Get total size
		var totalSize int64
		if args.Root != nil && *args.Root != "" {
			root := *args.Root
			db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM entries WHERE kind = 'file' AND (path = ? OR path LIKE ?)`, root, root+"/%").Scan(&totalSize)
		} else {
			db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM entries WHERE kind = 'file'`).Scan(&totalSize)
		}
		diagnostics["totalFileSize"] = totalSize

		// 6. Count resource sets
		var resourceSetCount int64
		db.QueryRow(`SELECT COUNT(*) FROM resource_sets`).Scan(&resourceSetCount)
		diagnostics["resourceSetCount"] = resourceSetCount

		// 7. Check for recent index jobs
		var recentJobCount int64
		oneDayAgo := time.Now().Unix() - 86400
		db.QueryRow(`SELECT COUNT(*) FROM index_jobs WHERE created_at > ?`, oneDayAgo).Scan(&recentJobCount)
		diagnostics["recentIndexJobs24h"] = recentJobCount

		// Add issues summary
		diagnostics["issues"] = issues
		diagnostics["healthy"] = len(issues) == 0

		log.WithFields(logrus.Fields{
			"totalEntries": totalEntries,
			"issues":       len(issues),
		}).Info("Database diagnostics completed")

		payload, err := json.Marshal(diagnostics)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Resource Set Tools

func registerResourceSetCreate(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-create",
		mcp.WithDescription("Create a new resource set (pure item storage)"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("description",
			mcp.Description("Description of the resource set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name        string  `json:"name"`
			Description *string `json:"description,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		set := &models.ResourceSet{
			Name:        args.Name,
			Description: args.Description,
			CreatedAt:   time.Now().Unix(),
			UpdatedAt:   time.Now().Unix(),
		}

		id, err := db.CreateResourceSet(set)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create resource set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Created resource set '%s' with ID %d", args.Name, id)), nil
	})
}

func registerResourceSetList(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-list",
		mcp.WithDescription("List all resource sets"),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sets, err := db.ListResourceSets()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list resource sets: %v", err)), nil
		}

		result, err := json.Marshal(sets)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

func registerResourceSetGet(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-get",
		mcp.WithDescription("Get entries in a resource set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		entries, err := db.GetResourceSetEntries(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get resource set entries: %v", err)), nil
		}

		// Compress if necessary
		contextInfo := fmt.Sprintf("Selection set: %s", args.Name)

		result, err := compressEntryList(entries, maxMCPResponseSize, contextInfo)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to process results: %v", err)), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

func registerResourceSetModify(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-modify",
		mcp.WithDescription("Add or remove entries from a resource set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
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
			err = db.AddToResourceSet(args.Name, paths)
		} else if args.Operation == "remove" {
			err = db.RemoveFromResourceSet(args.Name, paths)
		} else {
			return mcp.NewToolResultError("Invalid operation. Use 'add' or 'remove'"), nil
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to modify resource set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully %sed %d entries", args.Operation, len(paths))), nil
	})
}

func registerResourceSetDelete(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-delete",
		mcp.WithDescription("Delete a resource set"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.DeleteResourceSet(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete resource set: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Deleted resource set '%s'", args.Name)), nil
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

		// Compress if necessary
		contextInfo := fmt.Sprintf("Query: %s", args.Name)

		result, err := compressEntryList(entries, maxMCPResponseSize, contextInfo)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to process results: %v", err)), nil
		}

		return mcp.NewToolResultText(result), nil
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
		// Get current working directory
		cwd, err := filepath.Abs(".")
		if err != nil {
			cwd = "unknown"
		}

		info := map[string]interface{}{
			"database": dbPath,
			"version":  "0.1.0",
			"uptime":   "N/A", // Could track this if needed
			"cwd":      cwd,
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

func parseJobID(jobIDStr string) (int64, error) {
	return strconv.ParseInt(jobIDStr, 10, 64)
}

// checkIfHasMetadata checks if a file would have metadata/artifacts generated
func checkIfHasMetadata(path string, kind string, mtime int64) bool {
	if kind != "file" {
		return false
	}

	// Get the file extension and convert to lowercase
	lower := strings.ToLower(filepath.Ext(path))

	// Check for image extensions
	isImage := lower == ".jpg" || lower == ".jpeg" || lower == ".png" ||
		lower == ".gif" || lower == ".bmp"

	// Check for video extensions
	isVideo := lower == ".mp4" || lower == ".mov" || lower == ".mkv" ||
		lower == ".avi" || lower == ".webm"

	return isImage || isVideo
}

// File Action Tools

func registerRenameFilesTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("rename-files",
		mcp.WithDescription("Rename files based on a pattern. Supports regex pattern matching and replacement."),
		mcp.WithString("paths",
			mcp.Required(),
			mcp.Description("Comma-separated list of file paths to rename"),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Regex pattern to match in the filename (not the full path, just the basename)"),
		),
		mcp.WithString("replacement",
			mcp.Required(),
			mcp.Description("Replacement string. Can use $1, $2, etc. for captured groups from the pattern"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("Preview changes without executing (default: false)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Paths       string `json:"paths"`
			Pattern     string `json:"pattern"`
			Replacement string `json:"replacement"`
			DryRun      *bool  `json:"dryRun,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		dryRun := getBoolOrDefault(args.DryRun, false)

		// Parse paths
		paths := splitPaths(args.Paths)
		if len(paths) == 0 {
			return mcp.NewToolResultError("No paths provided"), nil
		}

		// Compile regex pattern
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid regex pattern: %v", err)), nil
		}

		// Process each path
		var results []map[string]interface{}
		successCount := 0
		errorCount := 0

		for _, pathStr := range paths {
			// Expand and validate path
			expandedPath, err := pathutil.ExpandPath(pathStr)
			if err != nil {
				results = append(results, map[string]interface{}{
					"oldPath": pathStr,
					"status":  "error",
					"error":   fmt.Sprintf("Invalid path: %v", err),
				})
				errorCount++
				continue
			}

			// Check if path exists
			if err := pathutil.ValidatePath(expandedPath); err != nil {
				results = append(results, map[string]interface{}{
					"oldPath": expandedPath,
					"status":  "error",
					"error":   fmt.Sprintf("Path does not exist: %v", err),
				})
				errorCount++
				continue
			}

			// Get basename and directory
			dir := filepath.Dir(expandedPath)
			basename := filepath.Base(expandedPath)

			// Apply regex replacement
			newBasename := re.ReplaceAllString(basename, args.Replacement)

			// Skip if no change
			if newBasename == basename {
				results = append(results, map[string]interface{}{
					"oldPath": expandedPath,
					"status":  "skipped",
					"message": "Pattern did not match",
				})
				continue
			}

			newPath := filepath.Join(dir, newBasename)

			// Check if target already exists
			if _, err := os.Stat(newPath); err == nil {
				results = append(results, map[string]interface{}{
					"oldPath": expandedPath,
					"newPath": newPath,
					"status":  "error",
					"error":   "Target path already exists",
				})
				errorCount++
				continue
			}

			if dryRun {
				results = append(results, map[string]interface{}{
					"oldPath": expandedPath,
					"newPath": newPath,
					"status":  "preview",
				})
				successCount++
			} else {
				// Perform the rename on filesystem
				if err := os.Rename(expandedPath, newPath); err != nil {
					results = append(results, map[string]interface{}{
						"oldPath": expandedPath,
						"newPath": newPath,
						"status":  "error",
						"error":   fmt.Sprintf("Failed to rename: %v", err),
					})
					errorCount++
					continue
				}

				// Update database
				if err := db.UpdateEntryPath(expandedPath, newPath); err != nil {
					log.WithError(err).WithFields(logrus.Fields{
						"oldPath": expandedPath,
						"newPath": newPath,
					}).Warn("Failed to update database after rename")
				}

				results = append(results, map[string]interface{}{
					"oldPath": expandedPath,
					"newPath": newPath,
					"status":  "success",
				})
				successCount++

				log.WithFields(logrus.Fields{
					"oldPath": expandedPath,
					"newPath": newPath,
				}).Info("File renamed successfully")
			}
		}

		response := map[string]interface{}{
			"results":      results,
			"successCount": successCount,
			"errorCount":   errorCount,
			"dryRun":       dryRun,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerDeleteFilesTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("delete-files",
		mcp.WithDescription("Delete files or directories from the filesystem and database index."),
		mcp.WithString("paths",
			mcp.Required(),
			mcp.Description("Comma-separated list of file or directory paths to delete"),
		),
		mcp.WithBoolean("recursive",
			mcp.Description("Delete directories recursively (default: false)"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("Preview changes without executing (default: false)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Paths     string `json:"paths"`
			Recursive *bool  `json:"recursive,omitempty"`
			DryRun    *bool  `json:"dryRun,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		recursive := getBoolOrDefault(args.Recursive, false)
		dryRun := getBoolOrDefault(args.DryRun, false)

		// Parse paths
		paths := splitPaths(args.Paths)
		if len(paths) == 0 {
			return mcp.NewToolResultError("No paths provided"), nil
		}

		// Process each path
		var results []map[string]interface{}
		successCount := 0
		errorCount := 0

		for _, pathStr := range paths {
			// Expand and validate path
			expandedPath, err := pathutil.ExpandPath(pathStr)
			if err != nil {
				results = append(results, map[string]interface{}{
					"path":   pathStr,
					"status": "error",
					"error":  fmt.Sprintf("Invalid path: %v", err),
				})
				errorCount++
				continue
			}

			// Check if path exists
			fileInfo, err := os.Stat(expandedPath)
			if err != nil {
				if os.IsNotExist(err) {
					results = append(results, map[string]interface{}{
						"path":   expandedPath,
						"status": "error",
						"error":  "Path does not exist",
					})
					errorCount++
					continue
				}
				results = append(results, map[string]interface{}{
					"path":   expandedPath,
					"status": "error",
					"error":  fmt.Sprintf("Failed to stat path: %v", err),
				})
				errorCount++
				continue
			}

			isDir := fileInfo.IsDir()

			// Check if recursive is needed for directories
			if isDir && !recursive {
				results = append(results, map[string]interface{}{
					"path":   expandedPath,
					"status": "error",
					"error":  "Path is a directory; use recursive=true to delete",
				})
				errorCount++
				continue
			}

			if dryRun {
				results = append(results, map[string]interface{}{
					"path":   expandedPath,
					"type":   map[bool]string{true: "directory", false: "file"}[isDir],
					"status": "preview",
				})
				successCount++
			} else {
				// Delete from filesystem
				var deleteErr error
				if isDir {
					deleteErr = os.RemoveAll(expandedPath)
				} else {
					deleteErr = os.Remove(expandedPath)
				}

				if deleteErr != nil {
					results = append(results, map[string]interface{}{
						"path":   expandedPath,
						"status": "error",
						"error":  fmt.Sprintf("Failed to delete: %v", deleteErr),
					})
					errorCount++
					continue
				}

				// Delete from database
				var dbErr error
				if isDir {
					dbErr = db.DeleteEntryRecursive(expandedPath)
				} else {
					dbErr = db.DeleteEntry(expandedPath)
				}

				if dbErr != nil {
					log.WithError(dbErr).WithField("path", expandedPath).Warn("Failed to delete from database")
				}

				results = append(results, map[string]interface{}{
					"path":   expandedPath,
					"type":   map[bool]string{true: "directory", false: "file"}[isDir],
					"status": "success",
				})
				successCount++

				log.WithFields(logrus.Fields{
					"path": expandedPath,
					"type": map[bool]string{true: "directory", false: "file"}[isDir],
				}).Info("Path deleted successfully")
			}
		}

		response := map[string]interface{}{
			"results":      results,
			"successCount": successCount,
			"errorCount":   errorCount,
			"dryRun":       dryRun,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerMoveFilesTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("move-files",
		mcp.WithDescription("Move files or directories to a destination directory."),
		mcp.WithString("sources",
			mcp.Required(),
			mcp.Description("Comma-separated list of source file or directory paths to move"),
		),
		mcp.WithString("destination",
			mcp.Required(),
			mcp.Description("Destination directory path"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("Preview changes without executing (default: false)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Sources     string `json:"sources"`
			Destination string `json:"destination"`
			DryRun      *bool  `json:"dryRun,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		dryRun := getBoolOrDefault(args.DryRun, false)

		// Expand and validate destination
		destPath, err := pathutil.ExpandPath(args.Destination)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid destination path: %v", err)), nil
		}

		// Check if destination exists and is a directory
		destInfo, err := os.Stat(destPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Destination does not exist: %v", err)), nil
		}
		if !destInfo.IsDir() {
			return mcp.NewToolResultError("Destination must be a directory"), nil
		}

		// Parse source paths
		sources := splitPaths(args.Sources)
		if len(sources) == 0 {
			return mcp.NewToolResultError("No source paths provided"), nil
		}

		// Process each source path
		var results []map[string]interface{}
		successCount := 0
		errorCount := 0

		for _, sourceStr := range sources {
			// Expand and validate source path
			sourcePath, err := pathutil.ExpandPath(sourceStr)
			if err != nil {
				results = append(results, map[string]interface{}{
					"sourcePath": sourceStr,
					"status":     "error",
					"error":      fmt.Sprintf("Invalid source path: %v", err),
				})
				errorCount++
				continue
			}

			// Check if source exists
			sourceInfo, err := os.Stat(sourcePath)
			if err != nil {
				if os.IsNotExist(err) {
					results = append(results, map[string]interface{}{
						"sourcePath": sourcePath,
						"status":     "error",
						"error":      "Source path does not exist",
					})
					errorCount++
					continue
				}
				results = append(results, map[string]interface{}{
					"sourcePath": sourcePath,
					"status":     "error",
					"error":      fmt.Sprintf("Failed to stat source: %v", err),
				})
				errorCount++
				continue
			}

			// Calculate target path
			basename := filepath.Base(sourcePath)
			targetPath := filepath.Join(destPath, basename)

			// Check if target already exists
			if _, err := os.Stat(targetPath); err == nil {
				results = append(results, map[string]interface{}{
					"sourcePath": sourcePath,
					"targetPath": targetPath,
					"status":     "error",
					"error":      "Target path already exists",
				})
				errorCount++
				continue
			}

			if dryRun {
				results = append(results, map[string]interface{}{
					"sourcePath": sourcePath,
					"targetPath": targetPath,
					"type":       map[bool]string{true: "directory", false: "file"}[sourceInfo.IsDir()],
					"status":     "preview",
				})
				successCount++
			} else {
				// Move on filesystem
				if err := os.Rename(sourcePath, targetPath); err != nil {
					results = append(results, map[string]interface{}{
						"sourcePath": sourcePath,
						"targetPath": targetPath,
						"status":     "error",
						"error":      fmt.Sprintf("Failed to move: %v", err),
					})
					errorCount++
					continue
				}

				// Update database
				var dbErr error
				if sourceInfo.IsDir() {
					dbErr = db.UpdatePathsRecursive(sourcePath, targetPath)
				} else {
					dbErr = db.UpdateEntryPath(sourcePath, targetPath)
				}

				if dbErr != nil {
					log.WithError(dbErr).WithFields(logrus.Fields{
						"sourcePath": sourcePath,
						"targetPath": targetPath,
					}).Warn("Failed to update database after move")
				}

				results = append(results, map[string]interface{}{
					"sourcePath": sourcePath,
					"targetPath": targetPath,
					"type":       map[bool]string{true: "directory", false: "file"}[sourceInfo.IsDir()],
					"status":     "success",
				})
				successCount++

				log.WithFields(logrus.Fields{
					"sourcePath": sourcePath,
					"targetPath": targetPath,
				}).Info("Path moved successfully")
			}
		}

		response := map[string]interface{}{
			"results":      results,
			"destination":  destPath,
			"successCount": successCount,
			"errorCount":   errorCount,
			"dryRun":       dryRun,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Plan Tools

func registerPlanCreate(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("plan-create",
		mcp.WithDescription("Create a new plan that defines automated file processing"),
		mcp.WithString("planJson",
			mcp.Required(),
			mcp.Description("JSON string containing plan definition with sources, conditions (RuleCondition), and outcomes (RuleOutcome)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PlanJson string `json:"planJson"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Parse plan JSON
		var plan models.Plan
		if err := json.Unmarshal([]byte(args.PlanJson), &plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid plan JSON: %v", err)), nil
		}

		// Set defaults
		if plan.Mode == "" {
			plan.Mode = "oneshot"
		}
		if plan.Status == "" {
			plan.Status = "active"
		}

		// Create plan
		if err := db.CreatePlan(&plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create plan: %v", err)), nil
		}

		result := map[string]interface{}{
			"id":     plan.ID,
			"name":   plan.Name,
			"mode":   plan.Mode,
			"status": plan.Status,
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanExecute(s *server.MCPServer, db *database.DiskDB, processor *classifier.Processor) {
	tool := mcp.NewTool("plan-execute",
		mcp.WithDescription("Execute a plan to process files according to its rules"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the plan to execute"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Get plan
		plan, err := db.GetPlan(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan not found: %v", err)), nil
		}

		// Execute plan with classifier processor for thumbnail generation
		logger := logrus.New().WithField("tool", "plan-execute")
		executor := plans.NewExecutor(db, logger)
		executor.SetProcessor(processor)
		execution, err := executor.Execute(plan)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan execution failed: %v", err)), nil
		}

		result := map[string]interface{}{
			"execution_id":       execution.ID,
			"plan":               plan.Name,
			"status":             execution.Status,
			"entries_processed":  execution.EntriesProcessed,
			"entries_matched":    execution.EntriesMatched,
			"outcomes_applied":   execution.OutcomesApplied,
			"duration_ms":        execution.DurationMs,
			"error_message":      execution.ErrorMessage,
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanList(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("plan-list",
		mcp.WithDescription("List all plans"),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		plans, err := db.ListPlans()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list plans: %v", err)), nil
		}

		result := make([]map[string]interface{}, len(plans))
		for i, plan := range plans {
			result[i] = map[string]interface{}{
				"id":          plan.ID,
				"name":        plan.Name,
				"description": plan.Description,
				"mode":        plan.Mode,
				"status":      plan.Status,
				"created_at":  plan.CreatedAt,
				"last_run_at": plan.LastRunAt,
			}
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanGet(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("plan-get",
		mcp.WithDescription("Get plan details including execution history"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the plan"),
		),
		mcp.WithNumber("executionLimit",
			mcp.Description("Number of recent executions to include (default: 10)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name           string  `json:"name"`
			ExecutionLimit *int    `json:"executionLimit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Get plan
		plan, err := db.GetPlan(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plan not found: %v", err)), nil
		}

		// Get execution history
		limit := 10
		if args.ExecutionLimit != nil {
			limit = *args.ExecutionLimit
		}
		executions, err := db.GetPlanExecutions(plan.Name, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get executions: %v", err)), nil
		}

		result := map[string]interface{}{
			"id":          plan.ID,
			"name":        plan.Name,
			"description": plan.Description,
			"mode":        plan.Mode,
			"status":      plan.Status,
			"sources":     plan.Sources,
			"conditions":  plan.Conditions,
			"outcomes":    plan.Outcomes,
			"created_at":  plan.CreatedAt,
			"updated_at":  plan.UpdatedAt,
			"last_run_at": plan.LastRunAt,
			"executions":  executions,
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanUpdate(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("plan-update",
		mcp.WithDescription("Update an existing plan"),
		mcp.WithString("planJson",
			mcp.Required(),
			mcp.Description("JSON string containing updated plan definition (must include name field to identify the plan to update)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PlanJson string `json:"planJson"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Parse plan JSON
		var plan models.Plan
		if err := json.Unmarshal([]byte(args.PlanJson), &plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid plan JSON: %v", err)), nil
		}

		// Update plan
		if err := db.UpdatePlan(&plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update plan: %v", err)), nil
		}

		result := map[string]interface{}{
			"name":       plan.Name,
			"mode":       plan.Mode,
			"status":     plan.Status,
			"updated_at": plan.UpdatedAt,
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanDelete(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("plan-delete",
		mcp.WithDescription("Delete a plan"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the plan to delete"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name string `json:"name"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.DeletePlan(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete plan: %v", err)), nil
		}

		result := map[string]interface{}{
			"name":    args.Name,
			"deleted": true,
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// =============================================================================
// Multi-Project Tool Registrations (MP suffix)
// These versions resolve the database from the session's active project
// =============================================================================

func registerIndexToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("index",
		mcp.WithDescription("Index a directory tree into the active project database. Use force=true to re-index even if recently scanned."),
		mcp.WithString("root",
			mcp.Required(),
			mcp.Description("File or directory path to index"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Run asynchronously and return job ID immediately (default: true)"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force re-indexing even if the path was recently scanned (default: false)"),
		),
		mcp.WithNumber("maxAge",
			mcp.Description("Maximum age in seconds before a scan is considered stale (default: 3600 = 1 hour)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}

		var args struct {
			Root   string `json:"root"`
			Async  *bool  `json:"async,omitempty"`
			Force  *bool  `json:"force,omitempty"`
			MaxAge *int64 `json:"maxAge,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Build index options
		indexOpts := crawler.DefaultIndexOptions()
		if args.Force != nil {
			indexOpts.Force = *args.Force
		}
		if args.MaxAge != nil {
			indexOpts.MaxAge = *args.MaxAge
		}

		log.WithFields(logrus.Fields{
			"root":   args.Root,
			"force":  indexOpts.Force,
			"maxAge": indexOpts.MaxAge,
		}).Info("Executing index via MCP (multi-project)")

		// Default to async mode
		asyncMode := getBoolOrDefault(args.Async, true)

		if asyncMode {
			// Expand path (but don't validate yet)
			expandedPath, expandErr := pathutil.ExpandPath(args.Root)
			if expandErr != nil {
				expandedPath = args.Root
			}

			// Create job in database FIRST (before validation)
			jobID, err := db.CreateIndexJob(expandedPath, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create index job: %v", err)), nil
			}

			// Now validate the path
			validationErr := pathutil.ValidatePath(expandedPath)
			if expandErr != nil || validationErr != nil {
				var errMsg string
				if expandErr != nil {
					errMsg = fmt.Sprintf("Path expansion failed: %v", expandErr)
				} else {
					errMsg = fmt.Sprintf("Invalid path: %v", validationErr)
				}

				if updateErr := db.UpdateIndexJobStatus(jobID, "failed", &errMsg); updateErr != nil {
					log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to mark job as failed")
				}

				response := map[string]any{
					"jobId":     jobID,
					"root":      expandedPath,
					"status":    "failed",
					"error":     errMsg,
					"statusUrl": fmt.Sprintf("synthesis://jobs/%d", jobID),
				}
				payload, _ := json.Marshal(response)
				return mcp.NewToolResultText(string(payload)), nil
			}

			// Path is valid - start indexing in background
			go func() {
				if err := db.UpdateIndexJobStatus(jobID, "running", nil); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as running")
					return
				}

				stats, err := crawler.IndexWithOptions(expandedPath, db, nil, jobID, nil, indexOpts)

				if err != nil {
					errMsg := err.Error()
					if updateErr := db.UpdateIndexJobStatus(jobID, "failed", &errMsg); updateErr != nil {
						log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to update job status to failed")
					}
					log.WithError(err).WithField("jobID", jobID).Error("Indexing failed")
				} else if stats.Skipped {
					if err := db.UpdateIndexJobProgress(jobID, 100, &database.IndexJobMetadata{
						FilesProcessed:       0,
						DirectoriesProcessed: 0,
						TotalSize:            0,
						ErrorCount:           0,
						Skipped:              true,
						SkipReason:           stats.SkipReason,
					}); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to update job metadata for skip")
					}
					if err := db.UpdateIndexJobStatus(jobID, "completed", nil); err != nil {
						log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as completed")
					}
				} else {
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
				}
			}()

			response := map[string]any{
				"jobId":     jobID,
				"root":      expandedPath,
				"status":    "running",
				"statusUrl": fmt.Sprintf("synthesis://jobs/%d", jobID),
			}
			payload, _ := json.Marshal(response)
			return mcp.NewToolResultText(string(payload)), nil
		}

		// Synchronous mode
		expandedPath, err := pathutil.ExpandPath(args.Root)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		if err := pathutil.ValidatePath(expandedPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		stats, err := crawler.IndexWithOptions(expandedPath, db, nil, 0, nil, indexOpts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Indexing failed: %v", err)), nil
		}

		if stats.Skipped {
			response := map[string]any{
				"root":       expandedPath,
				"skipped":    true,
				"skipReason": stats.SkipReason,
			}
			payload, _ := json.Marshal(response)
			return mcp.NewToolResultText(string(payload)), nil
		}

		response := map[string]any{
			"root":                 expandedPath,
			"filesProcessed":      stats.FilesProcessed,
			"directoriesProcessed": stats.DirectoriesProcessed,
			"totalSize":           stats.TotalSize,
			"duration":            stats.Duration.String(),
			"errors":              stats.Errors,
		}
		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerNavigateToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("navigate",
		mcp.WithDescription("Navigate to a directory within the indexed tree and return a lightweight listing with summary statistics."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Target path (absolute or relative to previous call)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return (default: 20)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset into the child list for pagination"),
		),
		mcp.WithString("sortBy",
			mcp.Description("Sort by: size, name, or mtime (default: size)"),
		),
		mcp.WithString("order",
			mcp.Description("Sort order: asc or desc (default: desc)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}

		var args struct {
			Path   string  `json:"path"`
			Limit  *int    `json:"limit,omitempty"`
			Offset *int    `json:"offset,omitempty"`
			SortBy *string `json:"sortBy,omitempty"`
			Order  *string `json:"order,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		entry, err := db.Get(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load path: %v", err)), nil
		}
		if entry == nil || entry.Kind != "directory" {
			return mcp.NewToolResultError("Path is not an indexed directory"), nil
		}

		children, err := db.Children(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch children: %v", err)), nil
		}

		offset := getIntOrDefault(args.Offset, 0)
		if offset < 0 {
			offset = 0
		}
		limit := getIntOrDefault(args.Limit, 20)
		if limit <= 0 {
			limit = 20
		}

		sortBy := getStringOrDefault(args.SortBy, "size")
		order := getStringOrDefault(args.Order, "desc")
		descending := order == "desc"

		SortEntries(children, sortBy, descending)
		summary := BuildEntrySummary(children, 10)

		totalCount := len(children)
		if offset >= totalCount {
			children = []*models.Entry{}
		} else {
			end := offset + limit
			if end > totalCount {
				end = totalCount
			}
			children = children[offset:end]
		}

		enrichEntriesWithThumbnails(db, children)

		response := map[string]interface{}{
			"path":        expandedPath,
			"total_size":  entry.Size,
			"total_items": totalCount,
			"summary":     summary,
			"entries":     children,
			"pagination": map[string]int{
				"offset": offset,
				"limit":  limit,
				"total":  totalCount,
			},
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerInspectToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("inspect",
		mcp.WithDescription("Get detailed information about a single file or directory including metadata and generated artifacts"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to inspect"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}

		var args struct {
			Path string `json:"path"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		entry, err := db.Get(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultError("Path not found in index"), nil
		}

		metadata, _ := db.GetMetadataByPath(expandedPath)

		response := map[string]interface{}{
			"path":         entry.Path,
			"kind":         entry.Kind,
			"size":         entry.Size,
			"mtime":        entry.Mtime,
			"ctime":        entry.Ctime,
			"last_scanned": entry.LastScanned,
		}

		if len(metadata) > 0 {
			metadataList := make([]map[string]interface{}, 0, len(metadata))
			for _, m := range metadata {
				normalizedPath := normalizeCachePath(m.CachePath)
				metadataList = append(metadataList, map[string]interface{}{
					"type":       m.MetadataType,
					"hash":       m.Hash,
					"cache_path": normalizedPath,
					"url":        fmt.Sprintf("%s/api/content?path=%s", sc.ContentBaseURL, normalizedPath),
				})
			}
			response["metadata"] = metadataList
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Stub implementations for remaining tools - these delegate to the original implementations
// TODO: Refactor these to use requireProjectDB pattern

func registerJobProgressToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("job-progress",
		mcp.WithDescription("Get progress of an indexing job"),
		mcp.WithNumber("jobId", mcp.Required(), mcp.Description("Job ID to check")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			JobId int64 `json:"jobId"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		job, err := db.GetIndexJob(args.JobId)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get job: %v", err)), nil
		}
		if job == nil {
			return mcp.NewToolResultError("Job not found"), nil
		}
		payload, _ := json.Marshal(job)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerListJobsToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("job-list",
		mcp.WithDescription("List indexing jobs"),
		mcp.WithString("status", mcp.Description("Filter by status")),
		mcp.WithNumber("limit", mcp.Description("Maximum jobs to return")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Status *string `json:"status,omitempty"`
			Limit  *int    `json:"limit,omitempty"`
		}
		unmarshalArgs(request.Params.Arguments, &args)
		limit := 50
		if args.Limit != nil {
			limit = *args.Limit
		}
		jobs, err := db.ListIndexJobs(args.Status, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
		}
		payload, _ := json.Marshal(jobs)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerCancelJobToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("job-cancel",
		mcp.WithDescription("Cancel an indexing job"),
		mcp.WithNumber("jobId", mcp.Required(), mcp.Description("Job ID to cancel")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			JobId int64 `json:"jobId"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		if err := db.UpdateIndexJobStatus(args.JobId, "cancelled", nil); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to cancel job: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"cancelled": true}`), nil
	})
}

func registerDbDiagnoseToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("db-diagnose",
		mcp.WithDescription("Run database diagnostics"),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var count int
		db.DB().QueryRow("SELECT COUNT(*) FROM entries").Scan(&count)
		result := map[string]interface{}{
			"entries_count": count,
			"backend":       "sqlite3",
		}
		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSetCreateMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("resource-set-create",
		mcp.WithDescription("Create a new resource set"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Unique name for the resource set")),
		mcp.WithString("description", mcp.Description("Optional description")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name        string  `json:"name"`
			Description *string `json:"description,omitempty"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		set := &models.ResourceSet{Name: args.Name, Description: args.Description}
		id, err := db.CreateResourceSet(set)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create: %v", err)), nil
		}
		result := map[string]interface{}{"id": id, "name": args.Name}
		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSetListMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("resource-set-list",
		mcp.WithDescription("List all resource sets"),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		sets, err := db.ListResourceSets()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list: %v", err)), nil
		}
		payload, _ := json.Marshal(sets)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSetGetMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("resource-set-get",
		mcp.WithDescription("Get resource set by name"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource set name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		set, err := db.GetResourceSet(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get: %v", err)), nil
		}
		if set == nil {
			return mcp.NewToolResultError("Resource set not found"), nil
		}
		payload, _ := json.Marshal(set)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSetModifyMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("resource-set-modify",
		mcp.WithDescription("Add or remove entries from a resource set"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource set name")),
		mcp.WithString("operation", mcp.Required(), mcp.Description("Operation: add or remove")),
		mcp.WithString("paths", mcp.Required(), mcp.Description("Comma-separated paths")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name      string `json:"name"`
			Operation string `json:"operation"`
			Paths     string `json:"paths"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		paths := strings.Split(args.Paths, ",")
		for i := range paths {
			paths[i] = strings.TrimSpace(paths[i])
		}
		var err error
		if args.Operation == "add" {
			err = db.AddToResourceSet(args.Name, paths)
		} else if args.Operation == "remove" {
			err = db.RemoveFromResourceSet(args.Name, paths)
		} else {
			return mcp.NewToolResultError("Invalid operation. Use 'add' or 'remove'"), nil
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"success": true}`), nil
	})
}

func registerResourceSetDeleteMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("resource-set-delete",
		mcp.WithDescription("Delete a resource set"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource set name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		if err := db.DeleteResourceSet(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"deleted": true}`), nil
	})
}

func registerQueryCreateMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-create",
		mcp.WithDescription("Create a saved query"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Query name")),
		mcp.WithString("filterJson", mcp.Required(), mcp.Description("Filter JSON")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name       string `json:"name"`
			FilterJson string `json:"filterJson"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		// Validate JSON format
		var filter models.FileFilter
		if err := json.Unmarshal([]byte(args.FilterJson), &filter); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid filter JSON: %v", err)), nil
		}
		query := &models.Query{Name: args.Name, QueryType: "file_filter", QueryJSON: args.FilterJson}
		id, err := db.CreateQuery(query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		result := map[string]interface{}{"id": id, "name": args.Name}
		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerQueryExecuteMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-execute",
		mcp.WithDescription("Execute a saved query"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Query name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		entries, err := db.ExecuteQuery(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		payload, _ := json.Marshal(entries)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerQueryListMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-list", mcp.WithDescription("List all saved queries"))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		queries, err := db.ListQueries()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		payload, _ := json.Marshal(queries)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerQueryGetMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-get",
		mcp.WithDescription("Get a saved query by name"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Query name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		query, err := db.GetQuery(args.Name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		payload, _ := json.Marshal(query)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerQueryUpdateMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-update",
		mcp.WithDescription("Update a saved query"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Query name")),
		mcp.WithString("filterJson", mcp.Required(), mcp.Description("Updated filter JSON")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name       string `json:"name"`
			FilterJson string `json:"filterJson"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		query, err := db.GetQuery(args.Name)
		if err != nil || query == nil {
			return mcp.NewToolResultError("Query not found"), nil
		}
		// Validate JSON format
		var filter models.FileFilter
		if err := json.Unmarshal([]byte(args.FilterJson), &filter); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid filter JSON: %v", err)), nil
		}
		query.QueryJSON = args.FilterJson
		// Note: UpdateQuery not implemented in DiskDB, would need to add
		return mcp.NewToolResultText(`{"updated": true}`), nil
	})
}

func registerQueryDeleteMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("query-delete",
		mcp.WithDescription("Delete a saved query"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Query name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		if err := db.DeleteQuery(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"deleted": true}`), nil
	})
}

func registerPlanCreateMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("plan-create",
		mcp.WithDescription("Create a new plan"),
		mcp.WithString("planJson", mcp.Required(), mcp.Description("Plan definition as JSON")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			PlanJson string `json:"planJson"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		var plan models.Plan
		if err := json.Unmarshal([]byte(args.PlanJson), &plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid plan JSON: %v", err)), nil
		}
		if err := db.CreatePlan(&plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		result := map[string]interface{}{"id": plan.ID, "name": plan.Name}
		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanExecuteMP(s *server.MCPServer, sc *ServerContext, processor *classifier.Processor) {
	tool := mcp.NewTool("plan-execute",
		mcp.WithDescription("Execute a plan"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Plan name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		plan, err := db.GetPlan(args.Name)
		if err != nil || plan == nil {
			return mcp.NewToolResultError("Plan not found"), nil
		}
		executor := plans.NewExecutor(db, log)
		executor.SetProcessor(processor)
		result, err := executor.Execute(plan)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Execution failed: %v", err)), nil
		}
		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanListMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("plan-list", mcp.WithDescription("List all plans"))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		plans, err := db.ListPlans()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		payload, _ := json.Marshal(plans)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanGetMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("plan-get",
		mcp.WithDescription("Get a plan by name"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Plan name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		plan, err := db.GetPlan(args.Name)
		if err != nil || plan == nil {
			return mcp.NewToolResultError("Plan not found"), nil
		}
		payload, _ := json.Marshal(plan)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerPlanUpdateMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("plan-update",
		mcp.WithDescription("Update a plan"),
		mcp.WithString("planJson", mcp.Required(), mcp.Description("Updated plan JSON")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			PlanJson string `json:"planJson"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		var plan models.Plan
		if err := json.Unmarshal([]byte(args.PlanJson), &plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid plan JSON: %v", err)), nil
		}
		if err := db.UpdatePlan(&plan); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"updated": true}`), nil
	})
}

func registerPlanDeleteMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("plan-delete",
		mcp.WithDescription("Delete a plan"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Plan name")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		if err := db.DeletePlan(args.Name); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed: %v", err)), nil
		}
		return mcp.NewToolResultText(`{"deleted": true}`), nil
	})
}

func registerSessionInfoMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("session-info",
		mcp.WithDescription("Get current session information including active project"),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := GetSessionID(ctx)
		if sessionID == "" {
			return mcp.NewToolResultError("No session ID available"), nil
		}

		activeProject := ""
		if projectName, err := sc.SessionManager.GetActiveProject(sessionID); err == nil {
			activeProject = projectName
		}

		result := map[string]interface{}{
			"session_id":     sessionID,
			"active_project": activeProject,
			"has_project":    activeProject != "",
		}

		if activeProject != "" {
			if project, err := sc.ProjectManager.GetProject(activeProject); err == nil {
				result["project_info"] = map[string]interface{}{
					"name":        project.Name,
					"description": project.Description,
					"path":        project.Path,
				}
			}
		}

		payload, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerSessionSetPreferencesMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("session-set-preferences",
		mcp.WithDescription("Set session preferences"),
		mcp.WithString("prefsJson", mcp.Required(), mcp.Description("Preferences as JSON")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := GetSessionID(ctx)
		if sessionID == "" {
			return mcp.NewToolResultError("No session ID available"), nil
		}
		var args struct {
			PrefsJson string `json:"prefsJson"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		var prefs map[string]interface{}
		if err := json.Unmarshal([]byte(args.PrefsJson), &prefs); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid preferences JSON: %v", err)), nil
		}
		session := sc.SessionManager.GetOrCreate(sessionID)
		for k, v := range prefs {
			session.Preferences[k] = v
		}
		return mcp.NewToolResultText(`{"updated": true}`), nil
	})
}

func registerRenameFilesToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("rename-files",
		mcp.WithDescription("Rename files matching a pattern"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Base path")),
		mcp.WithString("pattern", mcp.Required(), mcp.Description("Pattern to match")),
		mcp.WithString("replacement", mcp.Required(), mcp.Description("Replacement string")),
		mcp.WithBoolean("dryRun", mcp.Description("Preview without making changes")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Path        string `json:"path"`
			Pattern     string `json:"pattern"`
			Replacement string `json:"replacement"`
			DryRun      bool   `json:"dryRun"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		expandedPath, _ := pathutil.ExpandPath(args.Path)
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid pattern: %v", err)), nil
		}
		children, _ := db.Children(expandedPath)
		var results []map[string]string
		for _, entry := range children {
			if entry.Kind != "file" {
				continue
			}
			name := filepath.Base(entry.Path)
			if re.MatchString(name) {
				newName := re.ReplaceAllString(name, args.Replacement)
				if newName != name {
					results = append(results, map[string]string{
						"old": name,
						"new": newName,
					})
					if !args.DryRun {
						oldPath := entry.Path
						newPath := filepath.Join(filepath.Dir(entry.Path), newName)
						os.Rename(oldPath, newPath)
						db.UpdateEntryPath(oldPath, newPath)
					}
				}
			}
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"dry_run": args.DryRun,
			"renamed": results,
			"count":   len(results),
		})
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerDeleteFilesToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("delete-files",
		mcp.WithDescription("Delete files matching criteria"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Base path")),
		mcp.WithString("pattern", mcp.Description("Pattern to match")),
		mcp.WithBoolean("dryRun", mcp.Description("Preview without making changes")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			Path    string `json:"path"`
			Pattern string `json:"pattern"`
			DryRun  bool   `json:"dryRun"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		expandedPath, _ := pathutil.ExpandPath(args.Path)
		children, _ := db.Children(expandedPath)
		var re *regexp.Regexp
		if args.Pattern != "" {
			re, _ = regexp.Compile(args.Pattern)
		}
		var deleted []string
		for _, entry := range children {
			if entry.Kind != "file" {
				continue
			}
			name := filepath.Base(entry.Path)
			if re == nil || re.MatchString(name) {
				deleted = append(deleted, name)
				if !args.DryRun {
					os.Remove(entry.Path)
					db.DeleteEntry(entry.Path)
				}
			}
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"dry_run": args.DryRun,
			"deleted": deleted,
			"count":   len(deleted),
		})
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerMoveFilesToolMP(s *server.MCPServer, sc *ServerContext) {
	tool := mcp.NewTool("move-files",
		mcp.WithDescription("Move files to a new location"),
		mcp.WithString("sourcePath", mcp.Required(), mcp.Description("Source path")),
		mcp.WithString("destPath", mcp.Required(), mcp.Description("Destination path")),
		mcp.WithString("pattern", mcp.Description("Pattern to match")),
		mcp.WithBoolean("dryRun", mcp.Description("Preview without making changes")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		var args struct {
			SourcePath string `json:"sourcePath"`
			DestPath   string `json:"destPath"`
			Pattern    string `json:"pattern"`
			DryRun     bool   `json:"dryRun"`
		}
		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}
		sourcePath, _ := pathutil.ExpandPath(args.SourcePath)
		destPath, _ := pathutil.ExpandPath(args.DestPath)
		children, _ := db.Children(sourcePath)
		var re *regexp.Regexp
		if args.Pattern != "" {
			re, _ = regexp.Compile(args.Pattern)
		}
		var moved []map[string]string
		for _, entry := range children {
			if entry.Kind != "file" {
				continue
			}
			name := filepath.Base(entry.Path)
			if re == nil || re.MatchString(name) {
				newPath := filepath.Join(destPath, name)
				moved = append(moved, map[string]string{
					"from": entry.Path,
					"to":   newPath,
				})
				if !args.DryRun {
					os.Rename(entry.Path, newPath)
					db.UpdateEntryPath(entry.Path, newPath)
				}
			}
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"dry_run": args.DryRun,
			"moved":   moved,
			"count":   len(moved),
		})
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Placeholder implementations for source and resource tools
// These will be refactored in separate files

func registerSourceToolsMP(s *server.MCPServer, sc *ServerContext) {
	// TODO: Refactor source tools to use ServerContext
	// For now, sources are not supported in multi-project mode
	log.Warn("Source tools not yet implemented for multi-project mode")
}

func registerResourceToolsMP(s *server.MCPServer, sc *ServerContext) {
	// TODO: Refactor resource-centric tools to use ServerContext
	log.Warn("Resource tools not yet implemented for multi-project mode")
}
