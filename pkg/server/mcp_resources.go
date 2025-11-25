package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

// registerMCPResources registers all MCP resources and resource templates with the server
func registerMCPResources(s *server.MCPServer, db *database.DiskDB) {
	// Register static resources
	registerEntriesResource(s, db)
	registerResourceSetsResource(s, db)
	registerQueriesResource(s, db)
	registerPlansResource(s, db)
	registerPlanExecutionsResource(s, db)
	registerIndexJobsResource(s, db)
	registerMetadataResource(s, db) // All generated metadata (generic)

	// Type-specific metadata resources
	registerThumbnailsResource(s, db)      // All thumbnails
	registerVideoTimelinesResource(s, db)  // All video timeline frames

	// Register job queue resources
	registerJobQueuePendingResource(s, db)
	registerJobQueueRunningResource(s, db)
	registerJobQueueCompletedResource(s, db)
	registerJobQueueFailedResource(s, db)
	registerJobQueueActiveResource(s, db)

	// Register resource templates
	registerEntryTemplate(s, db)
	registerResourceSetTemplate(s, db)
	registerResourceSetEntriesTemplate(s, db)
	registerQueryTemplate(s, db)
	registerQueryExecutionsTemplate(s, db)
	registerPlanTemplate(s, db)
	registerPlanExecutionsTemplate(s, db)
	registerIndexJobTemplate(s, db)
	registerMetadataByHashTemplate(s, db)      // Metadata by hash
	registerNodeMetadataTemplate(s, db)        // All metadata for a node

	// Type-specific metadata templates for nodes
	registerNodeThumbnailTemplate(s, db)       // Thumbnail for a specific node
	registerNodeVideoTimelineTemplate(s, db)   // Video timeline for a specific node
}

// Static Resources

func registerEntriesResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://nodes",
		"All Filesystem Entries",
		mcp.WithResourceDescription("List of all indexed filesystem entries (files and directories)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		entries, err := db.All()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch entries: %w", err)
		}

		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal entries: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerResourceSetsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"resource://resource-sets",
		"All Resource Sets",
		mcp.WithResourceDescription("List of all resource sets (DAG nodes for organizing files)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		sets, err := db.ListResourceSets()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch resource sets: %w", err)
		}

		data, err := json.MarshalIndent(sets, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource sets: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerQueriesResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://queries",
		"All Saved Queries",
		mcp.WithResourceDescription("List of all saved file filter queries"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		queries, err := db.ListQueries()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch queries: %w", err)
		}

		data, err := json.MarshalIndent(queries, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal queries: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerIndexJobsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs",
		"All Index Jobs",
		mcp.WithResourceDescription("List of all filesystem indexing jobs"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		jobs, err := db.ListIndexJobs(nil, 100) // Limit to 100 recent jobs
		if err != nil {
			return nil, fmt.Errorf("failed to fetch index jobs: %w", err)
		}

		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal index jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// Job Queue Resources

func registerJobQueuePendingResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs/pending",
		"Pending Jobs",
		mcp.WithResourceDescription("List of pending indexing jobs waiting to be started"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		status := "pending"
		jobs, err := db.ListIndexJobs(&status, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch pending jobs: %w", err)
		}

		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerJobQueueRunningResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs/running",
		"Running Jobs",
		mcp.WithResourceDescription("List of currently running indexing jobs"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		status := "running"
		jobs, err := db.ListIndexJobs(&status, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch running jobs: %w", err)
		}

		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerJobQueueCompletedResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs/completed",
		"Completed Jobs",
		mcp.WithResourceDescription("List of successfully completed indexing jobs"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		status := "completed"
		jobs, err := db.ListIndexJobs(&status, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch completed jobs: %w", err)
		}

		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerJobQueueFailedResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs/failed",
		"Failed Jobs",
		mcp.WithResourceDescription("List of failed indexing jobs with error details"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		status := "failed"
		jobs, err := db.ListIndexJobs(&status, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch failed jobs: %w", err)
		}

		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerJobQueueActiveResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://jobs/active",
		"Active Jobs",
		mcp.WithResourceDescription("List of all active jobs (pending, running, and paused)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Get jobs for each active status
		allActiveJobs := make([]*database.IndexJob, 0)

		for _, status := range []string{"pending", "running", "paused"} {
			statusCopy := status
			jobs, err := db.ListIndexJobs(&statusCopy, 100)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch %s jobs: %w", status, err)
			}
			allActiveJobs = append(allActiveJobs, jobs...)
		}

		data, err := json.MarshalIndent(allActiveJobs, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jobs: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// Resource Templates

func registerEntryTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://nodes/{path}",
		"Filesystem Entry",
		mcp.WithTemplateDescription("Individual filesystem entry by path"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract path from URI: shell://nodes/{path}
		uri := request.Params.URI
		prefix := "shell://nodes/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		path := strings.TrimPrefix(uri, prefix)
		if path == "" {
			return nil, fmt.Errorf("path parameter is required")
		}

		entry, err := db.Get(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch entry: %w", err)
		}

		if entry == nil {
			return nil, fmt.Errorf("entry not found: %s", path)
		}

		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal entry: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerResourceSetTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"resource://resource-set/{name}",
		"Resource Set",
		mcp.WithTemplateDescription("Individual resource set by name"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: resource://resource-set/{name}
		uri := request.Params.URI
		prefix := "resource://resource-set/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		// Remove /entries suffix if present
		name = strings.TrimSuffix(name, "/entries")

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		set, err := db.GetResourceSet(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch resource set: %w", err)
		}

		if set == nil {
			return nil, fmt.Errorf("resource set not found: %s", name)
		}

		data, err := json.MarshalIndent(set, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource set: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerResourceSetEntriesTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"resource://resource-set/{name}/entries",
		"Resource Set Entries",
		mcp.WithTemplateDescription("All entries in a resource set"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: resource://resource-set/{name}/entries
		uri := request.Params.URI
		prefix := "resource://resource-set/"
		suffix := "/entries"

		if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		name = strings.TrimSuffix(name, suffix)

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		entries, err := db.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch resource set entries: %w", err)
		}

		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal entries: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerQueryTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://queries/{name}",
		"Query",
		mcp.WithTemplateDescription("Individual query by name"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://queries/{name}
		uri := request.Params.URI
		prefix := "shell://queries/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		// Remove /executions suffix if present
		name = strings.TrimSuffix(name, "/executions")

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		query, err := db.GetQuery(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch query: %w", err)
		}

		if query == nil {
			return nil, fmt.Errorf("query not found: %s", name)
		}

		data, err := json.MarshalIndent(query, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal query: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerQueryExecutionsTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://queries/{name}/executions",
		"Query Execution History",
		mcp.WithTemplateDescription("Execution history for a query"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://queries/{name}/executions
		uri := request.Params.URI
		prefix := "shell://queries/"
		suffix := "/executions"

		if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		name = strings.TrimSuffix(name, suffix)

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		// Get query to get its ID
		query, err := db.GetQuery(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch query: %w", err)
		}
		if query == nil {
			return nil, fmt.Errorf("query not found: %s", name)
		}

		// Get query executions
		executions, err := db.GetQueryExecutions(query.ID, 50) // Limit to 50 recent executions
		if err != nil {
			return nil, fmt.Errorf("failed to fetch query executions: %w", err)
		}

		data, err := json.MarshalIndent(executions, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal executions: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerIndexJobTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://jobs/{id}",
		"Index Job",
		mcp.WithTemplateDescription("Individual index job by ID"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract id from URI: shell://jobs/{id}
		uri := request.Params.URI
		prefix := "shell://jobs/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		idStr := strings.TrimPrefix(uri, prefix)
		if idStr == "" {
			return nil, fmt.Errorf("id parameter is required")
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id: %s", idStr)
		}

		job, err := db.GetIndexJob(id)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch index job: %w", err)
		}

		if job == nil {
			return nil, fmt.Errorf("index job not found: %d", id)
		}

		data, err := json.MarshalIndent(job, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal index job: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// Plan Resources

func registerPlansResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://plans",
		"All Plans",
		mcp.WithResourceDescription("List of all plans (automated file processing definitions)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		plans, err := db.ListPlans()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plans: %w", err)
		}

		data, err := json.MarshalIndent(plans, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal plans: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerPlanExecutionsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://plan-executions",
		"All Plan Executions",
		mcp.WithResourceDescription("List of all plan execution records across all plans"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Get all plans first
		plans, err := db.ListPlans()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plans: %w", err)
		}

		// Collect all executions from all plans
		allExecutions := make([]*models.PlanExecution, 0)
		for _, plan := range plans {
			executions, err := db.GetPlanExecutions(plan.Name, 50) // Limit to 50 per plan
			if err != nil {
				// Log error but continue with other plans
				continue
			}
			allExecutions = append(allExecutions, executions...)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"count":      len(allExecutions),
			"executions": allExecutions,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal executions: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerPlanTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://plans/{name}",
		"Plan",
		mcp.WithTemplateDescription("Individual plan by name"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://plans/{name}
		uri := request.Params.URI
		prefix := "shell://plans/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		// Remove /executions suffix if present
		name = strings.TrimSuffix(name, "/executions")

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		plan, err := db.GetPlan(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plan: %w", err)
		}

		if plan == nil {
			return nil, fmt.Errorf("plan not found: %s", name)
		}

		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal plan: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerPlanExecutionsTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://plans/{name}/executions",
		"Plan Execution History",
		mcp.WithTemplateDescription("Execution history for a specific plan"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://plans/{name}/executions
		uri := request.Params.URI
		prefix := "shell://plans/"
		suffix := "/executions"

		if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		name = strings.TrimSuffix(name, suffix)

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		// Get plan to verify it exists
		plan, err := db.GetPlan(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plan: %w", err)
		}
		if plan == nil {
			return nil, fmt.Errorf("plan not found: %s", name)
		}

		// Get execution history
		executions, err := db.GetPlanExecutions(name, 100) // Limit to 100 recent executions
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plan executions: %w", err)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"plan_name": name,
			"count":     len(executions),
			"executions": executions,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal executions: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// Generated Metadata Resources

func registerMetadataResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://metadata",
		"All Generated Metadata",
		mcp.WithResourceDescription("List of all generated file metadata (thumbnails, video timelines, etc.)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		metadataList, err := db.ListMetadata(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata: %w", err)
		}

		// Add resource URIs to metadata entries
		for _, metadata := range metadataList {
			metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"count":    len(metadataList),
			"metadata": metadataList,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerMetadataByHashTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://metadata/{hash}",
		"Metadata by Hash",
		mcp.WithTemplateDescription("Get a specific generated metadata entry by its hash"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract hash from URI: shell://metadata/{hash}
		parts := strings.Split(request.Params.URI, "/")
		if len(parts) < 4 {
			return nil, fmt.Errorf("invalid URI format: %s", request.Params.URI)
		}
		hash := parts[3]

		metadata, err := db.GetMetadata(hash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata: %w", err)
		}

		if metadata == nil {
			return nil, fmt.Errorf("metadata not found: %s", hash)
		}

		// Add resource URI
		metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)

		data, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerNodeMetadataTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://nodes/{path}/metadata",
		"Metadata for Node",
		mcp.WithTemplateDescription("Get all generated metadata for a specific filesystem node (file or directory)"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract path from URI: shell://nodes/{path}/metadata
		uri := request.Params.URI
		if !strings.HasPrefix(uri, "shell://nodes/") || !strings.HasSuffix(uri, "/metadata") {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		// Remove prefix and suffix to get path
		path := strings.TrimPrefix(uri, "shell://nodes/")
		path = strings.TrimSuffix(path, "/metadata")

		metadataList, err := db.GetMetadataByPath(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata for path: %w", err)
		}

		// Add resource URIs to metadata entries
		for _, metadata := range metadataList {
			metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"path":     path,
			"count":    len(metadataList),
			"metadata": metadataList,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

// Type-Specific Artifact Resources

func registerThumbnailsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://thumbnails",
		"All Thumbnail Artifacts",
		mcp.WithResourceDescription("List of all generated thumbnail artifacts for images and videos"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		thumbnailType := "thumbnail"
		metadataList, err := db.ListMetadata(&thumbnailType)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch thumbnails: %w", err)
		}

		// Add resource URIs to metadata
		for _, metadata := range metadataList {
			metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"count":      len(metadataList),
			"thumbnails": metadataList,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal thumbnails: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerVideoTimelinesResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://video-timelines",
		"All Video Timeline Artifacts",
		mcp.WithResourceDescription("List of all generated video timeline frame artifacts"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		timelineType := "video-timeline"
		metadataList, err := db.ListMetadata(&timelineType)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch video timelines: %w", err)
		}

		// Add resource URIs to metadata
		for _, metadata := range metadataList {
			metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
		}

		data, err := json.MarshalIndent(map[string]interface{}{
			"count":          len(metadataList),
			"video_timelines": metadataList,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal video timelines: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerNodeThumbnailTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://nodes/{path}/thumbnail",
		"Thumbnail for Node",
		mcp.WithTemplateDescription("Get the thumbnail artifact for a specific file (image or video poster)"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract path from URI: shell://nodes/{path}/thumbnail
		uri := request.Params.URI
		if !strings.HasPrefix(uri, "shell://nodes/") || !strings.HasSuffix(uri, "/thumbnail") {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		// Remove prefix and suffix to get path
		path := strings.TrimPrefix(uri, "shell://nodes/")
		path = strings.TrimSuffix(path, "/thumbnail")

		metadataList, err := db.GetMetadataByPath(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata for path: %w", err)
		}

		// Filter for thumbnail type only
		var thumbnailMetadata *models.Metadata
		for _, metadata := range metadataList {
			if metadata.MetadataType == "thumbnail" {
				metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
				thumbnailMetadata = metadata
				break
			}
		}

		if thumbnailMetadata == nil {
			return nil, fmt.Errorf("no thumbnail found for path: %s", path)
		}

		result := map[string]interface{}{
			"path":      path,
			"thumbnail": thumbnailMetadata,
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal thumbnail: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}

func registerNodeVideoTimelineTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://nodes/{path}/timeline",
		"Video Timeline for Node",
		mcp.WithTemplateDescription("Get all video timeline frame artifacts for a specific video file"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract path from URI: shell://nodes/{path}/timeline
		uri := request.Params.URI
		if !strings.HasPrefix(uri, "shell://nodes/") || !strings.HasSuffix(uri, "/timeline") {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		// Remove prefix and suffix to get path
		path := strings.TrimPrefix(uri, "shell://nodes/")
		path = strings.TrimSuffix(path, "/timeline")

		metadataList, err := db.GetMetadataByPath(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch metadata for path: %w", err)
		}

		// Filter for video-timeline type only
		var timelineFrames []*models.Metadata
		for _, metadata := range metadataList {
			if metadata.MetadataType == "video-timeline" {
				metadata.ResourceUri = fmt.Sprintf("shell://metadata/%s", metadata.Hash)
				timelineFrames = append(timelineFrames, metadata)
			}
		}

		if len(timelineFrames) == 0 {
			return nil, fmt.Errorf("no video timeline found for path: %s", path)
		}

		result := map[string]interface{}{
			"path":   path,
			"count":  len(timelineFrames),
			"frames": timelineFrames,
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal video timeline: %w", err)
		}

		return []mcp.ResourceContents{
			&mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})
}
