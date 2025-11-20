package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

// registerMCPResources registers all MCP resources and resource templates with the server
func registerMCPResources(s *server.MCPServer, db *database.DiskDB) {
	// Register static resources
	registerEntriesResource(s, db)
	registerSelectionSetsResource(s, db)
	registerQueriesResource(s, db)
	registerIndexJobsResource(s, db)

	// Register job queue resources
	registerJobQueuePendingResource(s, db)
	registerJobQueueRunningResource(s, db)
	registerJobQueueCompletedResource(s, db)
	registerJobQueueFailedResource(s, db)
	registerJobQueueActiveResource(s, db)

	// Register resource templates
	registerEntryTemplate(s, db)
	registerSelectionSetTemplate(s, db)
	registerSelectionSetEntriesTemplate(s, db)
	registerQueryTemplate(s, db)
	registerQueryExecutionsTemplate(s, db)
	registerIndexJobTemplate(s, db)
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

func registerSelectionSetsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"shell://selection-sets",
		"All Selection Sets",
		mcp.WithResourceDescription("List of all selection sets (named groups of files)"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		sets, err := db.ListSelectionSets()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch selection sets: %w", err)
		}

		data, err := json.MarshalIndent(sets, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal selection sets: %w", err)
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

func registerSelectionSetTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://selection-sets/{name}",
		"Selection Set",
		mcp.WithTemplateDescription("Individual selection set by name"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://selection-sets/{name}
		uri := request.Params.URI
		prefix := "shell://selection-sets/"
		if !strings.HasPrefix(uri, prefix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		// Remove /entries suffix if present
		name = strings.TrimSuffix(name, "/entries")

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		set, err := db.GetSelectionSet(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch selection set: %w", err)
		}

		if set == nil {
			return nil, fmt.Errorf("selection set not found: %s", name)
		}

		data, err := json.MarshalIndent(set, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal selection set: %w", err)
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

func registerSelectionSetEntriesTemplate(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"shell://selection-sets/{name}/entries",
		"Selection Set Entries",
		mcp.WithTemplateDescription("All entries in a selection set"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract name from URI: shell://selection-sets/{name}/entries
		uri := request.Params.URI
		prefix := "shell://selection-sets/"
		suffix := "/entries"

		if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
			return nil, fmt.Errorf("invalid URI format: %s", uri)
		}

		name := strings.TrimPrefix(uri, prefix)
		name = strings.TrimSuffix(name, suffix)

		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		entries, err := db.GetSelectionSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch selection set entries: %w", err)
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
