package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

// registerResourceTools registers all resource-centric MCP tools
func registerResourceTools(s *server.MCPServer, db *database.DiskDB) {
	// DAG Navigation
	registerResourceChildren(s, db)
	registerResourceParent(s, db)

	// DAG Management
	registerResourceSetAddChild(s, db)
	registerResourceSetRemoveChild(s, db)

	// Metric Queries
	registerResourceSum(s, db)

	// Filter Queries
	registerResourceTimeRange(s, db)
	registerResourceMetricRange(s, db)
	registerResourceIs(s, db)
	registerResourceFuzzyMatch(s, db)
	registerResourceSearch(s, db)
}

// DAG Navigation Tools

func registerResourceChildren(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-children",
		mcp.WithDescription("Get child resource sets in the DAG (downstream navigation)"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Traversal depth: 1=immediate children only, null/0=all descendants (default: 1)"),
		),
		mcp.WithBoolean("include_entries",
			mcp.Description("Include entry count and total size for each child (default: false)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name           string `json:"name"`
			Depth          *int   `json:"depth,omitempty"`
			IncludeEntries *bool  `json:"include_entries,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		depth := getIntOrDefault(args.Depth, 1)
		includeEntries := getBoolOrDefault(args.IncludeEntries, false)

		var children interface{}

		if depth == 1 {
			// Immediate children only
			childSets, err := db.GetResourceSetChildren(args.Name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get children: %v", err)), nil
			}

			if includeEntries {
				// Enrich with stats
				enriched := make([]map[string]interface{}, len(childSets))
				for i, child := range childSets {
					stats, _ := db.GetResourceSetStats(child.Name)
					enriched[i] = map[string]interface{}{
						"id":           child.ID,
						"name":         child.Name,
						"description":  child.Description,
						"created_at":   child.CreatedAt,
						"updated_at":   child.UpdatedAt,
						"entry_count":  0,
						"total_size":   int64(0),
						"child_count":  0,
						"parent_count": 0,
					}
					if stats != nil {
						enriched[i]["entry_count"] = stats.EntryCount
						enriched[i]["total_size"] = stats.TotalSize
						enriched[i]["child_count"] = stats.ChildCount
						enriched[i]["parent_count"] = stats.ParentCount
					}
				}
				children = enriched
			} else {
				children = childSets
			}
		} else {
			// All descendants
			descendants, err := db.GetResourceSetDescendants(args.Name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get descendants: %v", err)), nil
			}
			children = descendants
		}

		response := map[string]interface{}{
			"resource_set": args.Name,
			"children":     children,
			"depth":        depth,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceParent(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-parent",
		mcp.WithDescription("Get parent resource sets in the DAG (upstream navigation, like '..')"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithNumber("depth",
			mcp.Description("Traversal depth: 1=immediate parents only, null/0=all ancestors (default: 1)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name  string `json:"name"`
			Depth *int   `json:"depth,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		depth := getIntOrDefault(args.Depth, 1)

		var parents interface{}
		var err error

		if depth == 1 {
			parents, err = db.GetResourceSetParents(args.Name)
		} else {
			parents, err = db.GetResourceSetAncestors(args.Name)
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get parents: %v", err)), nil
		}

		response := map[string]interface{}{
			"resource_set": args.Name,
			"parents":      parents,
			"depth":        depth,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// DAG Management Tools

func registerResourceSetAddChild(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-add-child",
		mcp.WithDescription("Add a child resource set to create a DAG edge"),
		mcp.WithString("parent",
			mcp.Required(),
			mcp.Description("Name of the parent resource set"),
		),
		mcp.WithString("child",
			mcp.Required(),
			mcp.Description("Name of the child resource set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Parent string `json:"parent"`
			Child  string `json:"child"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.AddResourceSetEdge(args.Parent, args.Child); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to add edge: %v", err)), nil
		}

		response := map[string]interface{}{
			"parent":  args.Parent,
			"child":   args.Child,
			"message": fmt.Sprintf("Successfully added '%s' as child of '%s'", args.Child, args.Parent),
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSetRemoveChild(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-set-remove-child",
		mcp.WithDescription("Remove a child resource set edge from the DAG"),
		mcp.WithString("parent",
			mcp.Required(),
			mcp.Description("Name of the parent resource set"),
		),
		mcp.WithString("child",
			mcp.Required(),
			mcp.Description("Name of the child resource set"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Parent string `json:"parent"`
			Child  string `json:"child"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		if err := db.RemoveResourceSetEdge(args.Parent, args.Child); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to remove edge: %v", err)), nil
		}

		response := map[string]interface{}{
			"parent":  args.Parent,
			"child":   args.Child,
			"message": fmt.Sprintf("Successfully removed '%s' as child of '%s'", args.Child, args.Parent),
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Metric Query Tools

func registerResourceSum(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-sum",
		mcp.WithDescription("Compute hierarchical aggregation of a metric across resource set (replaces disk-du)"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("metric",
			mcp.Required(),
			mcp.Description("Metric to aggregate: 'size', 'count', 'files', 'directories'"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Aggregate through DAG descendants (default: false)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string `json:"name"`
			Metric          string `json:"metric"`
			IncludeChildren *bool  `json:"include_children,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		includeChildren := getBoolOrDefault(args.IncludeChildren, false)

		result, err := db.ResourceSum(args.Name, args.Metric, includeChildren)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to compute metric: %v", err)), nil
		}

		// Add human-readable size if metric is size
		response := map[string]interface{}{
			"resource_set":     result.ResourceSet,
			"metric":           result.Metric,
			"value":            result.Value,
			"breakdown":        result.Breakdown,
			"include_children": includeChildren,
		}

		if args.Metric == "size" {
			response["human_readable"] = formatSize(result.Value)
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Filter Query Tools

func registerResourceTimeRange(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-time-range",
		mcp.WithDescription("Filter resource set entries by time field range"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("field",
			mcp.Required(),
			mcp.Description("Time field to filter: 'mtime', 'ctime', 'added_at'"),
		),
		mcp.WithString("min",
			mcp.Description("Minimum time (ISO 8601 date or datetime, e.g., '2024-01-01' or '2024-01-01T00:00:00Z')"),
		),
		mcp.WithString("max",
			mcp.Description("Maximum time (ISO 8601 date or datetime)"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Include entries from child resource sets (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 100)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string  `json:"name"`
			Field           string  `json:"field"`
			Min             *string `json:"min,omitempty"`
			Max             *string `json:"max,omitempty"`
			IncludeChildren *bool   `json:"include_children,omitempty"`
			Limit           *int    `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		// Parse time parameters
		var minTime, maxTime *time.Time

		if args.Min != nil {
			t, err := parseTime(*args.Min)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid min time: %v", err)), nil
			}
			minTime = &t
		}

		if args.Max != nil {
			t, err := parseTime(*args.Max)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid max time: %v", err)), nil
			}
			maxTime = &t
		}

		includeChildren := getBoolOrDefault(args.IncludeChildren, false)
		limit := getIntOrDefault(args.Limit, 100)

		entries, err := db.ResourceTimeRange(args.Name, args.Field, minTime, maxTime, includeChildren)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to filter: %v", err)), nil
		}

		// Apply limit
		totalCount := len(entries)
		if limit > 0 && len(entries) > limit {
			entries = entries[:limit]
		}

		response := map[string]interface{}{
			"resource_set":     args.Name,
			"field":            args.Field,
			"total_count":      totalCount,
			"returned_count":   len(entries),
			"entries":          entries,
			"include_children": includeChildren,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceMetricRange(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-metric-range",
		mcp.WithDescription("Filter resource set entries by metric value range"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("metric",
			mcp.Required(),
			mcp.Description("Metric to filter: 'size'"),
		),
		mcp.WithNumber("min",
			mcp.Description("Minimum value"),
		),
		mcp.WithNumber("max",
			mcp.Description("Maximum value"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Include entries from child resource sets (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 100)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string  `json:"name"`
			Metric          string  `json:"metric"`
			Min             *int64  `json:"min,omitempty"`
			Max             *int64  `json:"max,omitempty"`
			IncludeChildren *bool   `json:"include_children,omitempty"`
			Limit           *int    `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		includeChildren := getBoolOrDefault(args.IncludeChildren, false)
		limit := getIntOrDefault(args.Limit, 100)

		entries, err := db.ResourceMetricRange(args.Name, args.Metric, args.Min, args.Max, includeChildren)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to filter: %v", err)), nil
		}

		// Apply limit
		totalCount := len(entries)
		if limit > 0 && len(entries) > limit {
			entries = entries[:limit]
		}

		response := map[string]interface{}{
			"resource_set":     args.Name,
			"metric":           args.Metric,
			"total_count":      totalCount,
			"returned_count":   len(entries),
			"entries":          entries,
			"include_children": includeChildren,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceIs(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-is",
		mcp.WithDescription("Filter resource set entries by exact field match"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("field",
			mcp.Required(),
			mcp.Description("Field to match: 'kind' (file/directory), 'extension'"),
		),
		mcp.WithString("value",
			mcp.Required(),
			mcp.Description("Value to match (e.g., 'file', 'directory', '.jpg', 'pdf')"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Include entries from child resource sets (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 100)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string `json:"name"`
			Field           string `json:"field"`
			Value           string `json:"value"`
			IncludeChildren *bool  `json:"include_children,omitempty"`
			Limit           *int   `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		includeChildren := getBoolOrDefault(args.IncludeChildren, false)
		limit := getIntOrDefault(args.Limit, 100)

		entries, err := db.ResourceIs(args.Name, args.Field, args.Value, includeChildren)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to filter: %v", err)), nil
		}

		// Apply limit
		totalCount := len(entries)
		if limit > 0 && len(entries) > limit {
			entries = entries[:limit]
		}

		response := map[string]interface{}{
			"resource_set":     args.Name,
			"field":            args.Field,
			"value":            args.Value,
			"total_count":      totalCount,
			"returned_count":   len(entries),
			"entries":          entries,
			"include_children": includeChildren,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceFuzzyMatch(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-fuzzy-match",
		mcp.WithDescription("Filter resource set entries by fuzzy/pattern matching"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithString("field",
			mcp.Required(),
			mcp.Description("Field to match: 'path', 'name'"),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Pattern to match"),
		),
		mcp.WithString("match_type",
			mcp.Required(),
			mcp.Description("Match type: 'contains', 'prefix', 'suffix', 'regex', 'glob'"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Include entries from child resource sets (default: false)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 100)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string `json:"name"`
			Field           string `json:"field"`
			Pattern         string `json:"pattern"`
			MatchType       string `json:"match_type"`
			IncludeChildren *bool  `json:"include_children,omitempty"`
			Limit           *int   `json:"limit,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		includeChildren := getBoolOrDefault(args.IncludeChildren, false)
		limit := getIntOrDefault(args.Limit, 100)

		entries, err := db.ResourceFuzzyMatch(args.Name, args.Field, args.Pattern, args.MatchType, includeChildren)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to filter: %v", err)), nil
		}

		// Apply limit
		totalCount := len(entries)
		if limit > 0 && len(entries) > limit {
			entries = entries[:limit]
		}

		response := map[string]interface{}{
			"resource_set":     args.Name,
			"field":            args.Field,
			"pattern":          args.Pattern,
			"match_type":       args.MatchType,
			"total_count":      totalCount,
			"returned_count":   len(entries),
			"entries":          entries,
			"include_children": includeChildren,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerResourceSearch(s *server.MCPServer, db *database.DiskDB) {
	tool := mcp.NewTool("resource-search",
		mcp.WithDescription("Comprehensive search across resource set entries with multiple filters"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the resource set"),
		),
		mcp.WithBoolean("include_children",
			mcp.Description("Include entries from child resource sets (default: false)"),
		),
		mcp.WithString("kind",
			mcp.Description("Filter by kind: 'file' or 'directory'"),
		),
		mcp.WithString("extension",
			mcp.Description("Filter by file extension (e.g., '.jpg', 'pdf')"),
		),
		mcp.WithString("path_contains",
			mcp.Description("Filter paths containing this string"),
		),
		mcp.WithString("name_contains",
			mcp.Description("Filter filenames containing this string"),
		),
		mcp.WithNumber("min_size",
			mcp.Description("Minimum file size in bytes"),
		),
		mcp.WithNumber("max_size",
			mcp.Description("Maximum file size in bytes"),
		),
		mcp.WithString("min_mtime",
			mcp.Description("Minimum modification time (ISO date)"),
		),
		mcp.WithString("max_mtime",
			mcp.Description("Maximum modification time (ISO date)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results (default: 100)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Pagination offset (default: 0)"),
		),
		mcp.WithString("sort_by",
			mcp.Description("Sort by: 'size', 'name', 'mtime' (default: 'size')"),
		),
		mcp.WithBoolean("sort_desc",
			mcp.Description("Sort descending (default: true)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Name            string  `json:"name"`
			IncludeChildren *bool   `json:"include_children,omitempty"`
			Kind            *string `json:"kind,omitempty"`
			Extension       *string `json:"extension,omitempty"`
			PathContains    *string `json:"path_contains,omitempty"`
			NameContains    *string `json:"name_contains,omitempty"`
			MinSize         *int64  `json:"min_size,omitempty"`
			MaxSize         *int64  `json:"max_size,omitempty"`
			MinMtime        *string `json:"min_mtime,omitempty"`
			MaxMtime        *string `json:"max_mtime,omitempty"`
			Limit           *int    `json:"limit,omitempty"`
			Offset          *int    `json:"offset,omitempty"`
			SortBy          *string `json:"sort_by,omitempty"`
			SortDesc        *bool   `json:"sort_desc,omitempty"`
		}

		if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
		}

		params := database.ResourceSearchParams{
			Name:            args.Name,
			IncludeChildren: getBoolOrDefault(args.IncludeChildren, false),
			Kind:            args.Kind,
			Extension:       args.Extension,
			PathContains:    args.PathContains,
			NameContains:    args.NameContains,
			MinSize:         args.MinSize,
			MaxSize:         args.MaxSize,
			Limit:           getIntOrDefault(args.Limit, 100),
			Offset:          getIntOrDefault(args.Offset, 0),
			SortBy:          getStringOrDefault(args.SortBy, "size"),
			SortDesc:        getBoolOrDefault(args.SortDesc, true),
		}

		// Parse time parameters
		if args.MinMtime != nil {
			t, err := parseTime(*args.MinMtime)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid min_mtime: %v", err)), nil
			}
			params.MinMtime = &t
		}

		if args.MaxMtime != nil {
			t, err := parseTime(*args.MaxMtime)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid max_mtime: %v", err)), nil
			}
			params.MaxMtime = &t
		}

		result, err := db.ResourceSearch(params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
		}

		response := map[string]interface{}{
			"resource_set":     args.Name,
			"entries":          result.Entries,
			"total_count":      result.TotalCount,
			"returned_count":   len(result.Entries),
			"offset":           result.Offset,
			"limit":            result.Limit,
			"has_more":         result.HasMore,
			"include_children": params.IncludeChildren,
		}

		payload, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(payload)), nil
	})
}

// Helper functions

func parseTime(s string) (time.Time, error) {
	// Try various formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
