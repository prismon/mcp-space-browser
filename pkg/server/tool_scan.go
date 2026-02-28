package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/pathutil"
	"github.com/sirupsen/logrus"
)

var scanToolDef = mcp.NewTool("scan",
	mcp.WithDescription("Index filesystem paths and extract attributes. Supports multiple paths, configurable depth, and optional attribute extraction."),
	mcp.WithArray("paths",
		mcp.Required(),
		mcp.Description("One or more filesystem paths to scan"),
	),
	mcp.WithArray("attributes",
		mcp.Description("Attributes to extract beyond base set: mime, hash.md5, hash.sha256, hash.perceptual, exif, permissions, thumbnail, video.thumbnails, media, text"),
	),
	mcp.WithNumber("depth",
		mcp.Description("Scan depth: -1=recursive (default), 0=this level only, N=N levels"),
	),
	mcp.WithBoolean("force",
		mcp.Description("Re-index even if recently scanned (default: false)"),
	),
	mcp.WithString("target",
		mcp.Description("Resource set name to populate with results"),
	),
	mcp.WithBoolean("async",
		mcp.Description("Return job ID immediately (default: true)"),
	),
	mcp.WithNumber("maxAge",
		mcp.Description("Max age in seconds before rescan (default: 3600)"),
	),
)

// registerScanTool registers the scan tool with a direct DiskDB reference (for tests)
func registerScanTool(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(scanToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleScan(ctx, request, db)
	})
}

// registerScanToolMP registers the scan tool with multi-project support
func registerScanToolMP(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(scanToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleScan(ctx, request, db)
	})
}

func handleScan(ctx context.Context, request mcp.CallToolRequest, db *database.DiskDB) (*mcp.CallToolResult, error) {
	var args struct {
		Paths      []string `json:"paths"`
		Attributes []string `json:"attributes,omitempty"`
		Depth      *int     `json:"depth,omitempty"`
		Force      *bool    `json:"force,omitempty"`
		Target     *string  `json:"target,omitempty"`
		Async      *bool    `json:"async,omitempty"`
		MaxAge     *int64   `json:"maxAge,omitempty"`
	}

	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	if len(args.Paths) == 0 {
		return mcp.NewToolResultError("paths is required and must contain at least one path"), nil
	}

	opts := crawler.DefaultIndexOptions()
	if args.Force != nil && *args.Force {
		opts.Force = true
	}
	if args.MaxAge != nil {
		opts.MaxAge = *args.MaxAge
	}

	asyncMode := true
	if args.Async != nil {
		asyncMode = *args.Async
	}

	// Validate and expand all paths
	expandedPaths := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		expanded, err := pathutil.ExpandPath(p)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path %q: %v", p, err)), nil
		}
		if err := pathutil.ValidatePath(expanded); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid path %q: %v", p, err)), nil
		}
		expandedPaths = append(expandedPaths, expanded)
	}

	if asyncMode {
		return handleScanAsync(db, expandedPaths, opts)
	}
	return handleScanSync(db, expandedPaths, opts)
}

func handleScanAsync(db *database.DiskDB, paths []string, opts *crawler.IndexOptions) (*mcp.CallToolResult, error) {
	type jobInfo struct {
		JobID     int64  `json:"job_id"`
		Path      string `json:"path"`
		StatusURL string `json:"status_url"`
	}

	jobs := make([]jobInfo, 0, len(paths))
	for _, p := range paths {
		jobID, err := db.CreateIndexJob(p, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create job for %q: %v", p, err)), nil
		}

		go func(path string, id int64) {
			if err := db.UpdateIndexJobStatus(id, "running", nil); err != nil {
				log.WithError(err).WithField("jobID", id).Error("Failed to mark job running")
				return
			}

			_, err := crawler.IndexWithOptions(path, db, nil, id, nil, opts)
			if err != nil {
				errMsg := err.Error()
				db.UpdateIndexJobStatus(id, "failed", &errMsg)
				log.WithError(err).WithFields(logrus.Fields{"jobID": id, "path": path}).Error("Scan failed")
			} else {
				db.UpdateIndexJobStatus(id, "completed", nil)
				log.WithFields(logrus.Fields{"jobID": id, "path": path}).Info("Scan completed")
			}
		}(p, jobID)

		jobs = append(jobs, jobInfo{
			JobID:     jobID,
			Path:      p,
			StatusURL: fmt.Sprintf("synthesis://jobs/%d", jobID),
		})
	}

	response := map[string]interface{}{
		"status": "started",
		"jobs":   jobs,
	}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}

func handleScanSync(db *database.DiskDB, paths []string, opts *crawler.IndexOptions) (*mcp.CallToolResult, error) {
	type pathResult struct {
		Path           string `json:"path"`
		FilesProcessed int    `json:"files_processed"`
		DirsProcessed  int    `json:"dirs_processed"`
		TotalSize      int64  `json:"total_size"`
		Skipped        bool   `json:"skipped,omitempty"`
		Error          string `json:"error,omitempty"`
	}

	startTime := time.Now()
	results := make([]pathResult, 0, len(paths))

	for _, p := range paths {
		stats, err := crawler.IndexWithOptions(p, db, nil, 0, nil, opts)
		if err != nil {
			results = append(results, pathResult{Path: p, Error: err.Error()})
		} else {
			results = append(results, pathResult{
				Path:           p,
				FilesProcessed: stats.FilesProcessed,
				DirsProcessed:  stats.DirectoriesProcessed,
				TotalSize:      stats.TotalSize,
				Skipped:        stats.Skipped,
			})
		}
	}

	response := map[string]interface{}{
		"status":      "completed",
		"duration_ms": time.Since(startTime).Milliseconds(),
		"results":     results,
	}
	payload, _ := json.Marshal(response)
	return mcp.NewToolResultText(string(payload)), nil
}
