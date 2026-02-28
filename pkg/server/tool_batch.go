package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/database"
)

var batchToolDef = mcp.NewTool("batch",
	mcp.WithDescription("Multi-file operations: bulk attribute extraction, duplicate detection, move, and delete."),
	mcp.WithString("operation",
		mcp.Required(),
		mcp.Description("Operation: attributes, duplicates, move, delete"),
		mcp.Enum("attributes", "duplicates", "move", "delete"),
	),
	mcp.WithString("from",
		mcp.Description("Resource set name to operate on"),
	),
	mcp.WithArray("paths",
		mcp.Description("Explicit file paths (alternative to from)"),
	),
	mcp.WithArray("keys",
		mcp.Description("Attribute keys to extract (for attributes operation)"),
	),
	mcp.WithString("method",
		mcp.Description("For duplicates: exact (hash.md5) or perceptual (hash.perceptual)"),
	),
	mcp.WithNumber("threshold",
		mcp.Description("For perceptual duplicates: hamming distance threshold (default 8)"),
	),
	mcp.WithString("destination",
		mcp.Description("Destination directory for move operation"),
	),
	mcp.WithBoolean("async",
		mcp.Description("Return job ID for long operations (default: false)"),
	),
)

func registerBatchTool(s *server.MCPServer, db *database.DiskDB) {
	s.AddTool(batchToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleBatch(ctx, request, db)
	})
}

func registerBatchToolMP(s *server.MCPServer, sc *ServerContext) {
	s.AddTool(batchToolDef, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		db, errResult := requireProjectDB(ctx, sc)
		if errResult != nil {
			return errResult, nil
		}
		return handleBatch(ctx, request, db)
	})
}

func handleBatch(ctx context.Context, request mcp.CallToolRequest, db *database.DiskDB) (*mcp.CallToolResult, error) {
	var args struct {
		Operation   string   `json:"operation"`
		From        string   `json:"from,omitempty"`
		Paths       StringOrStrings `json:"paths,omitempty"`
		Keys        StringOrStrings `json:"keys,omitempty"`
		Method      string   `json:"method,omitempty"`
		Threshold   *int     `json:"threshold,omitempty"`
		Destination string   `json:"destination,omitempty"`
		Async       *bool    `json:"async,omitempty"`
	}

	if err := unmarshalArgs(request.Params.Arguments, &args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid arguments: %v", err)), nil
	}

	// Resolve paths from resource set or explicit paths
	paths, err := resolveBatchPaths(db, args.From, args.Paths)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve paths: %v", err)), nil
	}

	switch args.Operation {
	case "attributes":
		return handleBatchAttributes(db, paths, args.Keys)
	case "duplicates":
		method := "exact"
		if args.Method != "" {
			method = args.Method
		}
		threshold := 8
		if args.Threshold != nil {
			threshold = *args.Threshold
		}
		return handleBatchDuplicates(db, paths, method, threshold)
	case "move":
		if args.Destination == "" {
			return mcp.NewToolResultError("destination is required for move operation"), nil
		}
		return handleBatchMove(db, paths, args.Destination)
	case "delete":
		return handleBatchDelete(db, paths)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown operation: %q", args.Operation)), nil
	}
}

func resolveBatchPaths(db *database.DiskDB, from string, paths []string) ([]string, error) {
	if from != "" {
		entries, err := db.GetResourceSetEntries(from)
		if err != nil {
			return nil, fmt.Errorf("failed to get entries from set %q: %w", from, err)
		}
		result := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.Kind == "file" {
				result = append(result, e.Path)
			}
		}
		return result, nil
	}
	if len(paths) > 0 {
		return paths, nil
	}
	return nil, fmt.Errorf("either 'from' (resource set) or 'paths' is required")
}

func handleBatchAttributes(db *database.DiskDB, paths []string, keys []string) (*mcp.CallToolResult, error) {
	if len(keys) == 0 {
		return mcp.NewToolResultError("keys is required for attributes operation"), nil
	}

	now := time.Now().Unix()
	results := make([]map[string]interface{}, 0, len(paths))

	for _, path := range paths {
		entry, err := db.Get(path)
		if err != nil || entry == nil {
			results = append(results, map[string]interface{}{
				"path":  path,
				"error": "entry not found",
			})
			continue
		}

		attrs := make(map[string]string)
		for _, key := range keys {
			// For now, set a placeholder value. In production this would
			// call actual extractors (mime detection, hashing, etc.)
			value := computeAttribute(entry, key)
			if value != "" {
				attr := &models.Attribute{
					EntryPath:  path,
					Key:        key,
					Value:      value,
					Source:     "enrichment",
					ComputedAt: now,
				}
				if err := db.SetAttribute(attr); err != nil {
					results = append(results, map[string]interface{}{
						"path":  path,
						"error": fmt.Sprintf("failed to set %s: %v", key, err),
					})
					continue
				}
				attrs[key] = value
			}
		}

		results = append(results, map[string]interface{}{
			"path":       path,
			"attributes": attrs,
		})
	}

	return jsonResult(map[string]interface{}{
		"operation":      "attributes",
		"processed":      len(results),
		"results":        results,
	})
}

// computeAttribute derives an attribute value for an entry.
// For now this handles basic attributes; full enrichment would
// integrate with the classifier pipeline.
func computeAttribute(entry *models.Entry, key string) string {
	switch key {
	case "kind":
		return entry.Kind
	case "size":
		return fmt.Sprintf("%d", entry.Size)
	default:
		// Unknown keys return empty - would be handled by enrichment pipeline
		return ""
	}
}

func handleBatchDuplicates(db *database.DiskDB, paths []string, method string, threshold int) (*mcp.CallToolResult, error) {
	hashKey := "hash.md5"
	if method == "perceptual" {
		hashKey = "hash.perceptual"
	}

	// Build hash -> paths map
	hashGroups := make(map[string][]string)
	for _, path := range paths {
		attr, err := db.GetAttribute(path, hashKey)
		if err != nil || attr == nil {
			continue
		}
		hashGroups[attr.Value] = append(hashGroups[attr.Value], path)
	}

	// Filter to groups with duplicates
	var groups []map[string]interface{}
	for hash, members := range hashGroups {
		if len(members) > 1 {
			groups = append(groups, map[string]interface{}{
				"hash":  hash,
				"count": len(members),
				"paths": members,
			})
		}
	}

	if groups == nil {
		groups = []map[string]interface{}{}
	}

	return jsonResult(map[string]interface{}{
		"operation":       "duplicates",
		"method":          method,
		"duplicate_groups": len(groups),
		"groups":          groups,
	})
}

func handleBatchMove(db *database.DiskDB, paths []string, destination string) (*mcp.CallToolResult, error) {
	results := make([]map[string]interface{}, 0, len(paths))
	moved := 0

	for _, path := range paths {
		baseName := path[len(path)-len(pathBase(path)):]
		newPath := destination + "/" + baseName

		if err := os.Rename(path, newPath); err != nil {
			results = append(results, map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			continue
		}

		// Update the database entry
		entry, err := db.Get(path)
		if err == nil && entry != nil {
			entry.Path = newPath
			parent := destination
			entry.Parent = &parent
			// Delete old and insert new
			db.DeleteEntry(path)
			db.InsertOrUpdate(entry)
		}

		results = append(results, map[string]interface{}{
			"path":     path,
			"new_path": newPath,
			"status":   "moved",
		})
		moved++
	}

	return jsonResult(map[string]interface{}{
		"operation": "move",
		"moved":     moved,
		"total":     len(paths),
		"results":   results,
	})
}

func handleBatchDelete(db *database.DiskDB, paths []string) (*mcp.CallToolResult, error) {
	results := make([]map[string]interface{}, 0, len(paths))
	deleted := 0

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			results = append(results, map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			continue
		}

		// Remove from database
		if err := db.DeleteEntry(path); err != nil {
			results = append(results, map[string]interface{}{
				"path":  path,
				"error": fmt.Sprintf("file deleted but db cleanup failed: %v", err),
			})
			continue
		}

		results = append(results, map[string]interface{}{
			"path":   path,
			"status": "deleted",
		})
		deleted++
	}

	return jsonResult(map[string]interface{}{
		"operation": "delete",
		"deleted":   deleted,
		"total":     len(paths),
		"results":   results,
	})
}

func pathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
