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

// registerResources registers 8 resource templates with the MCP server
func registerResources(s *server.MCPServer, db *database.DiskDB) {
	registerEntryResource(s, db)
	registerEntryAttributesResource(s, db)
	registerSetsListResource(s, db)
	registerSetResource(s, db)
	registerSetEntriesResource(s, db)
	registerJobsListResource(s, db)
	registerJobResource(s, db)
	registerProjectsResource(s, db)
}

// registerResourcesMP registers resources with multi-project support
func registerResourcesMP(s *server.MCPServer, sc *ServerContext) {
	// For multi-project, each resource handler resolves the DB from context.
	// For simplicity, we register the same templates and resolve DB in handlers.
	// The resource template URIs remain the same.
	// Note: In multi-project mode, resources that need a project DB
	// will need the context to resolve it. For now, we wire them up
	// with the assumption that project selection happens at the tool level.
}

func resourceJSON(data interface{}, uri string) ([]mcp.ResourceContents, error) {
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return []mcp.ResourceContents{
		&mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(payload),
		},
	}, nil
}

// 1. synthesis://entries/{path} — entry + attributes
func registerEntryResource(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"synthesis://entries/{path}",
		"Filesystem Entry",
		mcp.WithTemplateDescription("Individual filesystem entry with attributes"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		path := extractURIParam(request.Params.URI, "synthesis://entries/")
		if path == "" {
			return nil, fmt.Errorf("path parameter is required")
		}

		// Check if this is actually an attributes sub-resource
		if strings.HasSuffix(path, "/attributes") {
			path = strings.TrimSuffix(path, "/attributes")
		}

		entry, err := db.Get(path)
		if err != nil || entry == nil {
			return nil, fmt.Errorf("entry not found: %s", path)
		}

		attrs, err := db.GetAttributes(path)
		if err != nil {
			attrs = nil
		}

		result := map[string]interface{}{
			"entry":      entry,
			"attributes": attrs,
		}

		return resourceJSON(result, request.Params.URI)
	})
}

// 2. synthesis://entries/{path}/attributes — just attributes
func registerEntryAttributesResource(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"synthesis://entries/{path}/attributes",
		"Entry Attributes",
		mcp.WithTemplateDescription("Attributes for a filesystem entry"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract path: remove prefix and /attributes suffix
		uri := request.Params.URI
		path := extractURIParam(uri, "synthesis://entries/")
		path = strings.TrimSuffix(path, "/attributes")
		if path == "" {
			return nil, fmt.Errorf("path parameter is required")
		}

		attrs, err := db.GetAttributes(path)
		if err != nil {
			return nil, fmt.Errorf("failed to get attributes: %w", err)
		}

		return resourceJSON(attrs, uri)
	})
}

// 3. synthesis://sets — resource set list
func registerSetsListResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"synthesis://sets",
		"Resource Sets",
		mcp.WithResourceDescription("List of all resource sets"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		sets, err := db.ListResourceSets()
		if err != nil {
			return nil, fmt.Errorf("failed to list resource sets: %w", err)
		}
		return resourceJSON(sets, request.Params.URI)
	})
}

// 4. synthesis://sets/{name} — set details
func registerSetResource(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"synthesis://sets/{name}",
		"Resource Set",
		mcp.WithTemplateDescription("Resource set details"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		name := extractURIParam(request.Params.URI, "synthesis://sets/")
		// Don't match sub-resources
		if strings.Contains(name, "/") {
			name = strings.Split(name, "/")[0]
		}
		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		set, err := db.GetResourceSet(name)
		if err != nil || set == nil {
			return nil, fmt.Errorf("resource set not found: %s", name)
		}

		return resourceJSON(set, request.Params.URI)
	})
}

// 5. synthesis://sets/{name}/entries — entries in set
func registerSetEntriesResource(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"synthesis://sets/{name}/entries",
		"Resource Set Entries",
		mcp.WithTemplateDescription("Entries in a resource set"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		name := extractURIParam(request.Params.URI, "synthesis://sets/")
		name = strings.TrimSuffix(name, "/entries")
		if name == "" {
			return nil, fmt.Errorf("name parameter is required")
		}

		entries, err := db.GetResourceSetEntries(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}

		return resourceJSON(entries, request.Params.URI)
	})
}

// 6. synthesis://jobs — job list
func registerJobsListResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"synthesis://jobs",
		"Index Jobs",
		mcp.WithResourceDescription("List of all indexing jobs"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		jobs, err := db.ListIndexJobs(nil, 100)
		if err != nil {
			return nil, fmt.Errorf("failed to list jobs: %w", err)
		}
		return resourceJSON(jobs, request.Params.URI)
	})
}

// 7. synthesis://jobs/{id} — job details
func registerJobResource(s *server.MCPServer, db *database.DiskDB) {
	template := mcp.NewResourceTemplate(
		"synthesis://jobs/{id}",
		"Index Job",
		mcp.WithTemplateDescription("Individual indexing job details"),
		mcp.WithTemplateMIMEType("application/json"),
	)

	s.AddResourceTemplate(template, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		idStr := extractURIParam(request.Params.URI, "synthesis://jobs/")
		if idStr == "" {
			return nil, fmt.Errorf("id parameter is required")
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid job ID: %s", idStr)
		}

		job, err := db.GetIndexJob(id)
		if err != nil {
			return nil, fmt.Errorf("job not found: %d", id)
		}

		return resourceJSON(job, request.Params.URI)
	})
}

// 8. synthesis://projects — project list
func registerProjectsResource(s *server.MCPServer, db *database.DiskDB) {
	resource := mcp.NewResource(
		"synthesis://projects",
		"Projects",
		mcp.WithResourceDescription("List of all projects"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Projects are managed at the ServerContext level, not per-DB.
		// Return empty for single-DB mode.
		return resourceJSON([]interface{}{}, request.Params.URI)
	})
}

func extractURIParam(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return strings.TrimPrefix(uri, prefix)
}
