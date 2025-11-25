package models

import "time"

// Entry represents a filesystem entry (file or directory)
type Entry struct {
	ID          int64   `db:"id" json:"id,omitempty"`
	Path        string  `db:"path" json:"path"`
	Parent      *string `db:"parent" json:"parent"`
	Size        int64   `db:"size" json:"size"`
	Kind        string  `db:"kind" json:"kind"` // "file" or "directory"
	Ctime       int64   `db:"ctime" json:"ctime"` // Unix timestamp in seconds
	Mtime       int64   `db:"mtime" json:"mtime"` // Unix timestamp in seconds
	LastScanned int64   `db:"last_scanned" json:"last_scanned"`
	Dirty       int     `db:"dirty" json:"dirty,omitempty"`
}

// ResourceSet represents a named collection of files (DAG node)
// Pure item storage - only cares about WHAT items and WHEN they were added
// Supports DAG structure with multiple parents via ResourceSetEdge
type ResourceSet struct {
	ID          int64   `db:"id" json:"id,omitempty"`
	Name        string  `db:"name" json:"name"`
	Description *string `db:"description" json:"description,omitempty"`
	CreatedAt   int64   `db:"created_at" json:"created_at"`
	UpdatedAt   int64   `db:"updated_at" json:"updated_at"`
}

// ResourceSetEdge represents a parent-child relationship in the DAG
// Supports multiple parents per resource set
type ResourceSetEdge struct {
	ParentID int64 `db:"parent_id" json:"parent_id"`
	ChildID  int64 `db:"child_id" json:"child_id"`
	AddedAt  int64 `db:"added_at" json:"added_at"`
}

// ResourceSetEntry represents a single entry in a resource set
type ResourceSetEntry struct {
	SetID     int64  `db:"set_id" json:"set_id"`
	EntryPath string `db:"entry_path" json:"entry_path"`
	AddedAt   int64  `db:"added_at" json:"added_at"`
}

// ResourceSetInfo provides summary information about a resource set
type ResourceSetInfo struct {
	ResourceSet
	EntryCount int `json:"entry_count"`
	ChildCount int `json:"child_count"`
}

// Query represents a saved file filter query
type Query struct {
	ID                int64   `db:"id" json:"id,omitempty"`
	Name              string  `db:"name" json:"name"`
	Description       *string `db:"description" json:"description,omitempty"`
	QueryType         string  `db:"query_type" json:"query_type"` // "file_filter" or "custom_script"
	QueryJSON         string  `db:"query_json" json:"query_json"`
	TargetResourceSet *string `db:"target_resource_set" json:"target_resource_set,omitempty"`
	UpdateMode        *string `db:"update_mode" json:"update_mode,omitempty"` // "replace", "append", "merge"
	CreatedAt         int64   `db:"created_at" json:"created_at"`
	UpdatedAt         int64   `db:"updated_at" json:"updated_at"`
	LastExecuted      *int64  `db:"last_executed" json:"last_executed,omitempty"`
	ExecutionCount    int     `db:"execution_count" json:"execution_count"`
}

// FileFilter represents filtering criteria for files
type FileFilter struct {
	Path           *string  `json:"path,omitempty"`
	Pattern        *string  `json:"pattern,omitempty"`
	Extensions     []string `json:"extensions,omitempty"`
	MinSize        *int64   `json:"minSize,omitempty"`
	MaxSize        *int64   `json:"maxSize,omitempty"`
	MinDate        *string  `json:"minDate,omitempty"` // YYYY-MM-DD format
	MaxDate        *string  `json:"maxDate,omitempty"` // YYYY-MM-DD format
	NameContains   *string  `json:"nameContains,omitempty"`
	PathContains   *string  `json:"pathContains,omitempty"`
	SortBy         *string  `json:"sortBy,omitempty"` // "size", "name", "mtime"
	DescendingSort *bool    `json:"descendingSort,omitempty"`
	Limit          *int     `json:"limit,omitempty"`
}

// QueryExecution tracks query execution history
type QueryExecution struct {
	ID           int64   `db:"id" json:"id,omitempty"`
	QueryID      int64   `db:"query_id" json:"query_id"`
	ExecutedAt   int64   `db:"executed_at" json:"executed_at"`
	DurationMs   *int    `db:"duration_ms" json:"duration_ms,omitempty"`
	FilesMatched *int    `db:"files_matched" json:"files_matched,omitempty"`
	Status       string  `db:"status" json:"status"` // "success" or "error"
	ErrorMessage *string `db:"error_message" json:"error_message,omitempty"`
}

// SessionPreferences stores user preferences
type SessionPreferences struct {
	DefaultLimit        int    `json:"default_limit"`
	DefaultSortBy       string `json:"default_sort_by"`
	DefaultDescending   bool   `json:"default_descending"`
	PreferredSizeUnits  string `json:"preferred_size_units"` // "bytes", "kb", "mb", "gb"
	PreferredDateFormat string `json:"preferred_date_format"`
}

// TreeNode represents a node in a file tree (for disk-tree command)
type TreeNode struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Size      int64        `json:"size"`
	Kind      string       `json:"kind"`
	Mtime     time.Time    `json:"mtime"`
	Children  []*TreeNode  `json:"children,omitempty"`
	Summary   *TreeSummary `json:"summary,omitempty"`   // Summary when children are truncated
	Truncated bool         `json:"truncated,omitempty"` // True if children were truncated
}

// TreeSummary provides aggregate statistics for truncated directories
type TreeSummary struct {
	TotalChildren   int               `json:"total_children"`
	FileCount       int               `json:"file_count"`
	DirectoryCount  int               `json:"directory_count"`
	TotalSize       int64             `json:"total_size"`
	LargestChildren []*SimplifiedNode `json:"largest_children,omitempty"` // Top N largest children
}

// SimplifiedNode represents a lightweight node for summaries
type SimplifiedNode struct {
	Name  string    `json:"name"`
	Path  string    `json:"path"`
	Size  int64     `json:"size"`
	Kind  string    `json:"kind"`
	Mtime time.Time `json:"mtime"`
}

// DiskUsageSummary represents disk usage summary (for disk-du command)
type DiskUsageSummary struct {
	Path            string `json:"path"`
	TotalSize       int64  `json:"total_size"`
	FileCount       int    `json:"file_count"`
	DirectoryCount  int    `json:"directory_count"`
	LargestFile     string `json:"largest_file,omitempty"`
	LargestFileSize int64  `json:"largest_file_size"`
	OldestFile      string `json:"oldest_file,omitempty"`
	OldestFileTime  int64  `json:"oldest_file_time"`
	NewestFile      string `json:"newest_file,omitempty"`
	NewestFileTime  int64  `json:"newest_file_time"`
}

// Metadata represents generated file metadata (thumbnail, video timeline, etc.)
type Metadata struct {
	ID           int64  `db:"id" json:"id,omitempty"`
	Hash         string `db:"hash" json:"hash"`                            // SHA256 hash for deduplication
	SourcePath   string `db:"source_path" json:"source_path"`              // Original file path
	MetadataType string `db:"metadata_type" json:"metadata_type"`          // "thumbnail", "video-timeline", etc.
	MimeType     string `db:"mime_type" json:"mime_type"`                  // "image/jpeg", etc.
	CachePath    string `db:"cache_path" json:"cache_path"`                // Path to cached metadata file
	FileSize     int64  `db:"file_size" json:"file_size"`                  // Size of metadata file in bytes
	MetadataJson string `db:"metadata_json" json:"metadata_json,omitempty"` // JSON metadata (frame number, etc.)
	CreatedAt    int64  `db:"created_at" json:"created_at"`                // Unix timestamp
	ResourceUri  string `db:"-" json:"resource_uri,omitempty"`             // MCP resource URI (computed)
}

// Rule represents a rule definition
type Rule struct {
	ID            int64   `db:"id" json:"id,omitempty"`
	Name          string  `db:"name" json:"name"`
	Description   *string `db:"description" json:"description,omitempty"`
	Enabled       bool    `db:"enabled" json:"enabled"`
	Priority      int     `db:"priority" json:"priority"`
	ConditionJSON string  `db:"condition_json" json:"condition_json"`
	OutcomeJSON   string  `db:"outcome_json" json:"outcome_json"`
	CreatedAt     int64   `db:"created_at" json:"created_at"`
	UpdatedAt     int64   `db:"updated_at" json:"updated_at"`
}

// RuleCondition represents the condition for a rule
type RuleCondition struct {
	Type       string           `json:"type"` // "all", "any", "none", "media_type", "size", "time", "path"
	Conditions []*RuleCondition `json:"conditions,omitempty"` // For composite conditions (all, any, none)

	// Media type condition
	MediaType *string `json:"mediaType,omitempty"` // "image", "video", "audio", "document"

	// Size condition
	MinSize *int64 `json:"minSize,omitempty"` // Bytes
	MaxSize *int64 `json:"maxSize,omitempty"` // Bytes

	// Time condition
	MinMtime *int64 `json:"minMtime,omitempty"` // Unix timestamp
	MaxMtime *int64 `json:"maxMtime,omitempty"` // Unix timestamp
	MinCtime *int64 `json:"minCtime,omitempty"` // Unix timestamp
	MaxCtime *int64 `json:"maxCtime,omitempty"` // Unix timestamp

	// Path condition
	PathContains *string `json:"pathContains,omitempty"`
	PathPrefix   *string `json:"pathPrefix,omitempty"`
	PathSuffix   *string `json:"pathSuffix,omitempty"`
	PathPattern  *string `json:"pathPattern,omitempty"` // Regex
}

// RuleOutcome represents the outcome of a rule
// IMPORTANT: All outcomes must have a ResourceSetName to ensure traceability
type RuleOutcome struct {
	Type            string `json:"type"` // "resource_set", "classifier", "chained"
	ResourceSetName string `json:"resourceSetName"` // REQUIRED for all outcome types

	// For resource_set outcome
	Operation *string `json:"operation,omitempty"` // "add", "remove"

	// For classifier outcome
	ClassifierOperation *string `json:"classifierOperation,omitempty"` // "generate_thumbnail", "extract_metadata"
	MaxWidth            *int    `json:"maxWidth,omitempty"`
	MaxHeight           *int    `json:"maxHeight,omitempty"`
	Quality             *int    `json:"quality,omitempty"`

	// For chained outcome
	Outcomes    []*RuleOutcome `json:"outcomes,omitempty"`
	StopOnError *bool          `json:"stopOnError,omitempty"`
}

// RuleExecution represents a single execution of a rule
type RuleExecution struct {
	ID               int64   `db:"id" json:"id,omitempty"`
	RuleID           int64   `db:"rule_id" json:"rule_id"`
	ResourceSetID    int64   `db:"resource_set_id" json:"resource_set_id"`
	ExecutedAt       int64   `db:"executed_at" json:"executed_at"`
	EntriesMatched   int     `db:"entries_matched" json:"entries_matched"`
	EntriesProcessed int     `db:"entries_processed" json:"entries_processed"`
	Status           string  `db:"status" json:"status"` // "success", "partial", "error"
	ErrorMessage     *string `db:"error_message" json:"error_message,omitempty"`
	DurationMs       *int    `db:"duration_ms" json:"duration_ms,omitempty"`
}

// RuleOutcomeRecord represents a specific outcome action for a file
// This ensures every action taken by a rule is tracked and associated with a resource set
type RuleOutcomeRecord struct {
	ID            int64   `db:"id" json:"id,omitempty"`
	ExecutionID   int64   `db:"execution_id" json:"execution_id"`
	ResourceSetID int64   `db:"resource_set_id" json:"resource_set_id"` // ALWAYS required
	EntryPath     string  `db:"entry_path" json:"entry_path"`
	OutcomeType   string  `db:"outcome_type" json:"outcome_type"`
	OutcomeData   *string `db:"outcome_data" json:"outcome_data,omitempty"`
	Status        string  `db:"status" json:"status"` // "success", "error"
	ErrorMessage  *string `db:"error_message" json:"error_message,omitempty"`
	CreatedAt     int64   `db:"created_at" json:"created_at"`
}

// ResourceQuery represents a unified query against resources
// Used by resource-sum, resource-time-range, resource-metric-range, resource-is, resource-fuzzy-match
type ResourceQuery struct {
	ResourceSetName string            `json:"resource_set"`
	IncludeChildren bool              `json:"include_children"`
	Filters         []QueryFilter     `json:"filters"`
	Aggregation     *QueryAggregation `json:"aggregation,omitempty"`
	Pagination      *QueryPagination  `json:"pagination,omitempty"`
}

// QueryFilter represents a single filter condition
type QueryFilter struct {
	Field         string      `json:"field"`    // "path", "size", "mtime", "kind", etc.
	Operator      string      `json:"operator"` // "eq", "ne", "gt", "gte", "lt", "lte", "in", "like", "regex", "glob"
	Value         interface{} `json:"value"`
	CaseSensitive bool        `json:"case_sensitive,omitempty"`
}

// QueryAggregation for metrics
type QueryAggregation struct {
	Function string `json:"function"` // "sum", "count", "avg", "min", "max"
	Field    string `json:"field"`    // Field to aggregate
	GroupBy  string `json:"group_by,omitempty"` // Optional grouping
}

// QueryPagination for paginated results
type QueryPagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ResourceSumResult represents the result of a resource-sum query
type ResourceSumResult struct {
	ResourceSet string                 `json:"resource_set"`
	Metric      string                 `json:"metric"`
	Value       int64                  `json:"value"`
	Breakdown   []ResourceSumBreakdown `json:"breakdown,omitempty"`
}

// ResourceSumBreakdown represents a breakdown item in resource-sum
type ResourceSumBreakdown struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// ResourceChildInfo represents a child resource-set in resource-children response
type ResourceChildInfo struct {
	Name       string `json:"name"`
	EntryCount int    `json:"entry_count"`
	ChildCount int    `json:"child_count"`
}

// ResourceParentInfo represents a parent resource-set in resource-parent response
type ResourceParentInfo struct {
	Name string `json:"name"`
}
