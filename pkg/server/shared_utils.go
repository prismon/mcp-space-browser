package server

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
)

// PaginatedTreeResponse represents a paginated tree structure with summary statistics
type PaginatedTreeResponse struct {
	Path       string                  `json:"path"`
	Name       string                  `json:"name"`
	Size       int64                   `json:"size"`
	Kind       string                  `json:"kind"`
	ModifiedAt string                  `json:"modifiedAt"`
	Children   []*TreeChildNode        `json:"children,omitempty"`
	Pagination *PaginationMetadata     `json:"pagination"`
	Summary    *TreeStatisticsSummary  `json:"summary,omitempty"`
}

// TreeChildNode represents a child node in a paginated tree
type TreeChildNode struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Kind       string `json:"kind"`
	ModifiedAt string `json:"modifiedAt"`
	Link       string `json:"link"`
}

// PaginationMetadata contains pagination information
type PaginationMetadata struct {
	Total       int    `json:"total"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	HasMore     bool   `json:"hasMore"`
	NextPageUrl string `json:"nextPageUrl,omitempty"`
	PrevPageUrl string `json:"prevPageUrl,omitempty"`
}

// TreeStatisticsSummary provides aggregate statistics for a directory
type TreeStatisticsSummary struct {
	TotalChildren   int                  `json:"totalChildren"`
	FileCount       int                  `json:"fileCount"`
	DirectoryCount  int                  `json:"directoryCount"`
	TotalSize       int64                `json:"totalSize"`
	LargestChildren []*TreeChildNode     `json:"largestChildren,omitempty"`
}

// BuildEntrySummary creates a summary from a list of entries
func BuildEntrySummary(entries []*models.Entry, topN int) *TreeStatisticsSummary {
	if len(entries) == 0 {
		return &TreeStatisticsSummary{
			TotalChildren:   0,
			FileCount:       0,
			DirectoryCount:  0,
			TotalSize:       0,
			LargestChildren: []*TreeChildNode{},
		}
	}

	summary := &TreeStatisticsSummary{
		TotalChildren:  len(entries),
		FileCount:      0,
		DirectoryCount: 0,
		TotalSize:      0,
	}

	// Count files and directories, calculate total size
	for _, entry := range entries {
		if entry.Kind == "file" {
			summary.FileCount++
		} else {
			summary.DirectoryCount++
		}
		summary.TotalSize += entry.Size
	}

	// Get top N largest children
	sortedEntries := make([]*models.Entry, len(entries))
	copy(sortedEntries, entries)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].Size > sortedEntries[j].Size
	})

	keepCount := topN
	if len(sortedEntries) < keepCount {
		keepCount = len(sortedEntries)
	}

	summary.LargestChildren = make([]*TreeChildNode, keepCount)
	for i := 0; i < keepCount; i++ {
		summary.LargestChildren[i] = EntryToTreeChildNode(sortedEntries[i])
	}

	return summary
}

// EntryToTreeChildNode converts a database Entry to a TreeChildNode
func EntryToTreeChildNode(entry *models.Entry) *TreeChildNode {
	return &TreeChildNode{
		Path:       entry.Path,
		Name:       filepath.Base(entry.Path),
		Size:       entry.Size,
		Kind:       entry.Kind,
		ModifiedAt: time.Unix(entry.Mtime, 0).Format(time.RFC3339),
		Link:       fmt.Sprintf("synthesis://nodes/%s", entry.Path),
	}
}

// SortEntries sorts entries based on the specified field and order
func SortEntries(entries []*models.Entry, sortBy string, descending bool) {
	sort.Slice(entries, func(i, j int) bool {
		var result bool
		switch sortBy {
		case "name":
			result = filepath.Base(entries[i].Path) < filepath.Base(entries[j].Path)
		case "mtime":
			result = entries[i].Mtime < entries[j].Mtime
		case "size":
			fallthrough
		default:
			result = entries[i].Size < entries[j].Size
		}

		if descending {
			return !result
		}
		return result
	})
}

// BuildPaginationMetadata creates pagination metadata for a result set
func BuildPaginationMetadata(total, limit, offset int, baseURL string) *PaginationMetadata {
	hasMore := offset+limit < total

	metadata := &PaginationMetadata{
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: hasMore,
	}

	// Build next page URL if there are more results
	if hasMore {
		nextOffset := offset + limit
		metadata.NextPageUrl = fmt.Sprintf("%s&limit=%d&offset=%d", baseURL, limit, nextOffset)
	}

	// Build previous page URL if not on first page
	if offset > 0 {
		prevOffset := offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		metadata.PrevPageUrl = fmt.Sprintf("%s&limit=%d&offset=%d", baseURL, limit, prevOffset)
	}

	return metadata
}
