package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/sirupsen/logrus"
)

// registerClassifierTools registers all classifier-related MCP tools
func registerClassifierTools(s *server.MCPServer, db *database.DiskDB, processor *classifier.Processor) {
	registerRerunClassifierTool(s, db, processor)
	registerClassifierJobProgressTool(s, db)
	registerListClassifierJobsTool(s, db)
}

// registerRerunClassifierTool registers the rerun-classifier tool
func registerRerunClassifierTool(s *server.MCPServer, db *database.DiskDB, processor *classifier.Processor) {
	tool := mcp.NewTool("rerun-classifier",
		mcp.WithDescription("Rerun classifiers on a resource (file://, http://, https://, or synthesis:// URL) to generate thumbnails, timelines, and metadata."),
		mcp.WithString("resource",
			mcp.Required(),
			mcp.Description("Resource URL to process. Supports file://, http://, https://, and synthesis:// (e.g., synthesis://nodes/<path>)"),
		),
		mcp.WithBoolean("async",
			mcp.Description("Run asynchronously and return job ID immediately (default: true)"),
		),
		mcp.WithString("artifactTypes",
			mcp.Description("Comma-separated list of artifact types to generate: thumbnail, timeline, metadata. If not specified, all applicable types are generated."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Resource      string `json:"resource"`
			Async         *bool  `json:"async,omitempty"`
			ArtifactTypes string `json:"artifactTypes,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		log.WithFields(logrus.Fields{
			"resource":      args.Resource,
			"artifactTypes": args.ArtifactTypes,
		}).Info("Executing rerun-classifier via MCP")

		// Parse artifact types
		var artifactTypes []string
		if args.ArtifactTypes != "" {
			artifactTypes = splitPaths(args.ArtifactTypes) // Reuse the splitPaths helper
		}

		// Default to async mode
		asyncMode := getBoolOrDefault(args.Async, true)

		if asyncMode {
			// Create job in database FIRST
			// Try to resolve the resource to get local path (for display purposes)
			localPath := args.Resource // Default to resource URL

			jobID, err := db.CreateClassifierJob(args.Resource, localPath, artifactTypes)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create classifier job: %v", err)), nil
			}

			// Start processing in background
			go func() {
				// Mark job as running
				if err := db.StartClassifierJob(jobID); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as running")
					return
				}

				// Update progress to 10%
				db.UpdateClassifierJobProgress(jobID, 10)

				// Process the resource
				req := &classifier.ProcessRequest{
					ResourceURL:   args.Resource,
					ArtifactTypes: artifactTypes,
				}

				result, err := processor.ProcessResource(req)

				// Update progress to 50%
				db.UpdateClassifierJobProgress(jobID, 50)

				if err != nil {
					errMsg := err.Error()
					if updateErr := db.UpdateClassifierJobStatus(jobID, "failed", &errMsg); updateErr != nil {
						log.WithError(updateErr).WithField("jobID", jobID).Error("Failed to update job status to failed")
					}
					log.WithError(err).WithField("jobID", jobID).Error("Classifier processing failed")
					return
				}

				// Convert ProcessResult to ClassifierJobResult
				jobResult := &database.ClassifierJobResult{
					Artifacts: result.Artifacts,
					Errors:    result.Errors,
				}

				// Update job result
				if err := db.UpdateClassifierJobResult(jobID, jobResult); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to update job result")
				}

				// Update progress to 90%
				db.UpdateClassifierJobProgress(jobID, 90)

				// Mark as completed
				if err := db.UpdateClassifierJobStatus(jobID, "completed", nil); err != nil {
					log.WithError(err).WithField("jobID", jobID).Error("Failed to mark job as completed")
				}

				// Final progress update
				db.UpdateClassifierJobProgress(jobID, 100)

				log.WithFields(logrus.Fields{
					"jobID":         jobID,
					"resource":      args.Resource,
					"artifactCount": len(result.Artifacts),
					"errorCount":    len(result.Errors),
				}).Info("Classifier processing completed successfully")
			}()

			response := map[string]any{
				"jobId":     jobID,
				"resource":  args.Resource,
				"status":    "pending",
				"statusUrl": fmt.Sprintf("synthesis://classifier-jobs/%d", jobID),
			}

			payload, err := json.Marshal(response)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(payload)), nil
		}

		// Synchronous mode
		req := &classifier.ProcessRequest{
			ResourceURL:   args.Resource,
			ArtifactTypes: artifactTypes,
		}

		result, err := processor.ProcessResource(req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Classifier processing failed: %v", err)), nil
		}

		response := map[string]any{
			"resource":      args.Resource,
			"status":        "completed",
			"artifactCount": len(result.Artifacts),
			"artifacts":     result.Artifacts,
			"errors":        result.Errors,
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

// registerClassifierJobProgressTool registers the classifier-job-progress tool
func registerClassifierJobProgressTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("classifier-job-progress",
		mcp.WithDescription("Retrieve status and results for a classifier job."),
		mcp.WithString("jobId",
			mcp.Required(),
			mcp.Description("Job identifier returned from rerun-classifier"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			JobID string `json:"jobId"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		id, err := parseJobID(args.JobID)
		if err != nil {
			return mcp.NewToolResultError("jobId must be an integer"), nil
		}

		job, err := db.GetClassifierJob(id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load job: %v", err)), nil
		}
		if job == nil {
			return mcp.NewToolResultError("Job not found"), nil
		}

		response := map[string]any{
			"jobId":     job.ID,
			"status":    job.Status,
			"resource":  job.ResourceURL,
			"progress":  job.Progress,
			"statusUrl": fmt.Sprintf("synthesis://classifier-jobs/%d", job.ID),
		}

		// Add result if completed
		if job.Result != nil {
			var result database.ClassifierJobResult
			if err := json.Unmarshal([]byte(*job.Result), &result); err == nil {
				response["result"] = result
			}
		}

		// Add error if failed
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

// registerListClassifierJobsTool registers the list-classifier-jobs tool
func registerListClassifierJobsTool(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("list-classifier-jobs",
		mcp.WithDescription("List classifier jobs with optional filtering by status."),
		mcp.WithBoolean("activeOnly",
			mcp.Description("Show only active jobs (running or pending). Default: false"),
		),
		mcp.WithString("status",
			mcp.Description("Filter by specific status: pending, running, completed, failed, or cancelled"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of jobs to return (default: 50)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ActiveOnly *bool   `json:"activeOnly,omitempty"`
			Status     *string `json:"status,omitempty"`
			Limit      *int    `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Set default limit
		limit := getIntOrDefault(args.Limit, 50)

		// Determine status filter
		var statusFilter *string
		if args.ActiveOnly != nil && *args.ActiveOnly {
			statusFilter = nil // Will filter in-memory
		} else if args.Status != nil {
			statusFilter = args.Status
		}

		// Get jobs from database
		jobs, err := db.ListClassifierJobs(statusFilter, limit*2)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
		}

		// Apply filters
		var filteredJobs []*database.ClassifierJob
		for _, job := range jobs {
			// Filter by activeOnly
			if args.ActiveOnly != nil && *args.ActiveOnly {
				if job.Status != "running" && job.Status != "pending" {
					continue
				}
			}

			filteredJobs = append(filteredJobs, job)

			// Respect the limit
			if len(filteredJobs) >= limit {
				break
			}
		}

		// Build response
		var jobList []map[string]any
		for _, job := range filteredJobs {
			jobInfo := map[string]any{
				"jobId":     job.ID,
				"resource":  job.ResourceURL,
				"status":    job.Status,
				"progress":  job.Progress,
				"statusUrl": fmt.Sprintf("synthesis://classifier-jobs/%d", job.ID),
			}

			// Add result if completed
			if job.Result != nil {
				var result database.ClassifierJobResult
				if err := json.Unmarshal([]byte(*job.Result), &result); err == nil {
					jobInfo["artifactCount"] = len(result.Artifacts)
					jobInfo["errorCount"] = len(result.Errors)
				}
			}

			// Add error if failed
			if job.Error != nil {
				jobInfo["error"] = *job.Error
			}

			jobList = append(jobList, jobInfo)
		}

		response := map[string]any{
			"jobs":       jobList,
			"totalCount": len(jobList),
			"filters": map[string]any{
				"activeOnly": args.ActiveOnly,
				"status":     args.Status,
			},
		}

		payload, err := json.Marshal(response)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}
