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
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"github.com/prismon/mcp-space-browser/pkg/plans"
	"github.com/sirupsen/logrus"
)

// registerMCPTools registers all MCP tools with the server
func registerMCPTools(s *server.MCPServer, db *database.DiskDB, dbPath string) {
	// Shell-style navigation tools
	registerIndexTool(s, db)
	registerNavigateTool(s, db)
	registerInspectTool(s, db)
	registerJobProgressTool(s, db)
	registerListJobsTool(s, db)
	registerCancelJobTool(s, db)

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
	registerPlanExecute(s, db)
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
	topN := 50 // Keep top 50 largest entries
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
		mcp.WithDescription("Index the specified path and track progress with shell://jobs/{id}."),
		mcp.WithString("root",
			mcp.Required(),
			mcp.Description("File or directory path to index"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Run asynchronously and return job ID immediately (default: true)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Root  string `json:"root"`
			Async *bool  `json:"async,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		log.WithField("root", args.Root).Info("Executing index via MCP")

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
					"statusUrl": fmt.Sprintf("shell://jobs/%d", jobID),
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

				// Run the indexing with job tracking
				stats, err := crawler.Index(expandedPath, db, nil, jobID, nil)

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

			response := map[string]any{
				"jobId":     jobID,
				"root":      expandedPath,
				"status":    "pending",
				"statusUrl": fmt.Sprintf("shell://jobs/%d", jobID),
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

		stats, err := crawler.Index(expandedPath, db, nil, 0, nil)
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

		slice := children[offset:end]
		listings := make([]map[string]any, 0, len(slice))
		for _, child := range slice {
			listing := map[string]any{
				"path":       child.Path,
				"name":       filepath.Base(child.Path),
				"kind":       child.Kind,
				"size":       child.Size,
				"modifiedAt": time.Unix(child.Mtime, 0).Format(time.RFC3339),
				"link":       fmt.Sprintf("shell://nodes/%s", child.Path),
			}

			// Check if this child has metadata/artifacts
			if hasMetadata := checkIfHasMetadata(child.Path, child.Kind, child.Mtime); hasMetadata {
				listing["metadataUri"] = fmt.Sprintf("shell://nodes/%s/metadata", child.Path)
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
			response["nextPageUrl"] = fmt.Sprintf("shell://list?path=%s&offset=%d&limit=%d&sortBy=%s&order=%s", expandedPath, end, limit, sortBy, order)
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

		// Expand path
		expandedPath, err := pathutil.ExpandPath(args.Path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path: %v", err)), nil
		}

		// Get entry from database
		entry, err := db.Get(expandedPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultError("Entry not indexed"), nil
		}

		// Build response with MCP resources
		response := map[string]interface{}{
			"path":       entry.Path,
			"kind":       entry.Kind,
			"size":       entry.Size,
			"modifiedAt": time.Unix(entry.Mtime, 0).Format(time.RFC3339),
			"createdAt":  time.Unix(entry.Ctime, 0).Format(time.RFC3339),
			"resourceUri": fmt.Sprintf("shell://nodes/%s", entry.Path),
		}

		// Check if there's metadata available
		metadataList, err := db.GetMetadataByPath(expandedPath)
		if err == nil && len(metadataList) > 0 {
			response["metadataUri"] = fmt.Sprintf("shell://nodes/%s/metadata", entry.Path)
			response["metadataCount"] = len(metadataList)

			// Check for specific metadata types
			hasThumbnail := false
			hasTimeline := false
			for _, metadata := range metadataList {
				if metadata.MetadataType == "thumbnail" {
					hasThumbnail = true
				}
				if metadata.MetadataType == "video-timeline" {
					hasTimeline = true
				}
			}

			if hasThumbnail {
				response["thumbnailUri"] = fmt.Sprintf("shell://nodes/%s/thumbnail", entry.Path)
			}
			if hasTimeline {
				response["timelineUri"] = fmt.Sprintf("shell://nodes/%s/timeline", entry.Path)
			}
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
			"statusUrl": fmt.Sprintf("shell://jobs/%d", job.ID),
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
				"statusUrl": fmt.Sprintf("shell://jobs/%d", job.ID),
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
			"statusUrl": fmt.Sprintf("shell://jobs/%d", id),
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Resource Set Tools

func registerResourceSetCreate(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-create",
		mcp.WithDescription("Create a new resource set (DAG node for organizing files)"),
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
		maxResponseSize := 512000 // 500KB
		contextInfo := fmt.Sprintf("Resource set: %s", args.Name)

		result, err := compressEntryList(entries, maxResponseSize, contextInfo)
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
		maxResponseSize := 512000 // 500KB
		contextInfo := fmt.Sprintf("Query: %s", args.Name)

		result, err := compressEntryList(entries, maxResponseSize, contextInfo)
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

func registerPlanExecute(s *server.MCPServer, db *database.DiskDB) {
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

		// Execute plan
		logger := logrus.New().WithField("tool", "plan-execute")
		executor := plans.NewExecutor(db, logger)
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
