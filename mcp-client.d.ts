/**
 * TypeScript definitions for MCP Space Browser JavaScript Client
 */

/**
 * Options for creating a new MCPSpaceBrowserClient
 */
export interface ClientOptions {
  /** Request timeout in milliseconds (default: 30000) */
  timeout?: number;
  /** Additional headers to include in requests */
  headers?: Record<string, string>;
}

/**
 * Index result for async operations
 */
export interface IndexAsyncResult {
  /** Job identifier */
  jobId: number;
  /** Root path being indexed */
  root: string;
  /** Current job status */
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
  /** URL to check job status */
  statusUrl: string;
  /** Hint for current working directory */
  cwdHint?: string;
  /** Error message if status is 'failed' */
  error?: string;
}

/**
 * Index result for sync operations
 */
export interface IndexSyncResult {
  /** Root path indexed */
  root: string;
  /** Completion status */
  status: 'completed';
  /** Number of files processed */
  files: number;
  /** Number of directories processed */
  directories: number;
  /** Total size in bytes */
  totalSize: number;
  /** Duration in milliseconds */
  durationMs: number;
}

/**
 * Directory entry
 */
export interface DirectoryEntry {
  /** Full path */
  path: string;
  /** Entry name (basename) */
  name: string;
  /** Entry type */
  kind: 'file' | 'directory';
  /** Size in bytes */
  size: number;
  /** Modification time in RFC3339 format */
  modifiedAt: string;
  /** Link to navigate to this entry */
  link: string;
  /** Optional metadata URI */
  metadataUri?: string;
}

/**
 * Directory summary statistics
 */
export interface DirectorySummary {
  /** Total number of child entries */
  totalChildren: number;
  /** Number of files */
  fileCount: number;
  /** Number of directories */
  directoryCount: number;
  /** Total size in bytes */
  totalSize: number;
}

/**
 * Navigation result
 */
export interface NavigationResult {
  /** Current working directory */
  cwd: string;
  /** Total count of entries */
  count: number;
  /** Array of directory entries */
  entries: DirectoryEntry[];
  /** URL for next page (if any) */
  nextPageUrl: string;
  /** Summary statistics */
  summary: DirectorySummary;
}

/**
 * Job status
 */
export interface JobStatus {
  /** Job identifier */
  jobId: number;
  /** Job status */
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
  /** Root path being indexed */
  path: string;
  /** Progress percentage (0-100) */
  progress: number;
  /** URL to check job status */
  statusUrl: string;
}

/**
 * Job metadata
 */
export interface JobMetadata {
  /** Number of files processed */
  FilesProcessed: number;
  /** Number of directories processed */
  DirectoriesProcessed: number;
  /** Total size in bytes */
  TotalSize: number;
  /** Number of errors encountered */
  ErrorCount: number;
}

/**
 * Detailed job information
 */
export interface JobInfo {
  /** Job identifier */
  jobId: number;
  /** Root path */
  path: string;
  /** Job status */
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
  /** Progress percentage (0-100) */
  progress: number;
  /** Status URL */
  statusUrl: string;
  /** Start time in RFC3339 format */
  startedAt?: string;
  /** Completion time in RFC3339 format */
  completedAt?: string;
  /** Job metadata */
  metadata?: JobMetadata;
  /** Error message if failed */
  error?: string;
  /** Current activity description */
  currentActivity?: string;
}

/**
 * List jobs result
 */
export interface ListJobsResult {
  /** Array of jobs */
  jobs: JobInfo[];
  /** Total count of jobs */
  totalCount: number;
  /** Applied filters */
  filters: {
    activeOnly?: boolean;
    status?: string;
    minProgress?: number;
    maxProgress?: number;
  };
}

/**
 * Selection set
 */
export interface SelectionSet {
  /** Selection set ID */
  id: number;
  /** Selection set name */
  name: string;
  /** Description */
  description?: string;
  /** Criteria type */
  criteriaType: 'user_selected' | 'tool_query';
  /** Creation timestamp */
  createdAt: number;
  /** Last update timestamp */
  updatedAt: number;
}

/**
 * Query definition
 */
export interface Query {
  /** Query ID */
  id: number;
  /** Query name */
  name: string;
  /** Description */
  description?: string;
  /** Query type */
  queryType: 'file_filter' | 'custom_script';
  /** Query filter as JSON string */
  queryJSON: string;
  /** Creation timestamp */
  createdAt: number;
  /** Last update timestamp */
  updatedAt: number;
}

/**
 * File operation result
 */
export interface FileOperationResult {
  /** Array of individual results */
  results: Array<{
    /** Old path (for rename/move) or path (for delete) */
    oldPath?: string;
    path?: string;
    /** New path (for rename/move) */
    newPath?: string;
    /** Source path (for move) */
    sourcePath?: string;
    /** Target path (for move) */
    targetPath?: string;
    /** Entry type */
    type?: 'file' | 'directory';
    /** Operation status */
    status: 'success' | 'error' | 'preview' | 'skipped';
    /** Error message if status is 'error' */
    error?: string;
    /** Additional message */
    message?: string;
  }>;
  /** Number of successful operations */
  successCount: number;
  /** Number of failed operations */
  errorCount: number;
  /** Whether this was a dry run */
  dryRun: boolean;
  /** Destination path (for move operations) */
  destination?: string;
}

/**
 * Session information
 */
export interface SessionInfo {
  /** Database file path */
  database: string;
  /** Version string */
  version: string;
  /** Server uptime */
  uptime: string;
  /** Current working directory */
  cwd: string;
}

/**
 * MCP tool description
 */
export interface MCPTool {
  /** Tool name */
  name: string;
  /** Tool description */
  description: string;
  /** Input schema */
  inputSchema: Record<string, any>;
}

/**
 * MCP Space Browser JavaScript Client
 *
 * Provides comprehensive access to all MCP tools for disk space analysis,
 * file management, queries, and session control.
 */
export declare class MCPSpaceBrowserClient {
  /**
   * Create a new MCP Space Browser client
   * @param baseUrl - Base URL of the MCP server (e.g., 'http://localhost:3000')
   * @param options - Client options
   */
  constructor(baseUrl: string, options?: ClientOptions);

  // Core Tools

  /**
   * Index a directory tree
   * @param root - Path to index
   * @param options - Indexing options
   */
  index(root: string, options?: { async?: boolean }): Promise<IndexAsyncResult | IndexSyncResult>;

  /**
   * Navigate to a directory and list its contents
   * @param path - Directory path
   * @param options - Navigation options
   */
  navigate(path: string, options?: {
    limit?: number;
    offset?: number;
    sortBy?: 'size' | 'name' | 'mtime';
    order?: 'asc' | 'desc';
  }): Promise<NavigationResult>;

  /**
   * Inspect a specific file or directory
   * @param path - Path to inspect
   * @param options - Inspection options
   */
  inspect(path: string, options?: {
    limit?: number;
    offset?: number;
  }): Promise<any>;

  /**
   * Get progress status for an indexing job
   * @param jobId - Job identifier
   */
  getJobProgress(jobId: string | number): Promise<JobStatus>;

  /**
   * List indexing jobs with optional filtering
   * @param options - Filter options
   */
  listJobs(options?: {
    activeOnly?: boolean;
    status?: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
    minProgress?: number;
    maxProgress?: number;
    limit?: number;
  }): Promise<ListJobsResult>;

  /**
   * Cancel a running or pending indexing job
   * @param jobId - Job identifier to cancel
   */
  cancelJob(jobId: string | number): Promise<{
    jobId: number;
    status: string;
    path: string;
    message: string;
    statusUrl: string;
  }>;

  // Selection Set Tools

  /**
   * Create a new selection set
   * @param name - Name of the selection set
   * @param options - Creation options
   */
  createSelectionSet(name: string, options: {
    description?: string;
    criteriaType: 'user_selected' | 'tool_query';
  }): Promise<string>;

  /**
   * List all selection sets
   */
  listSelectionSets(): Promise<SelectionSet[]>;

  /**
   * Get entries in a selection set
   * @param name - Name of the selection set
   */
  getSelectionSet(name: string): Promise<any[] | any>;

  /**
   * Modify a selection set by adding or removing entries
   * @param name - Name of the selection set
   * @param operation - Operation: 'add' or 'remove'
   * @param paths - Array of paths to add/remove
   */
  modifySelectionSet(name: string, operation: 'add' | 'remove', paths: string[] | string): Promise<string>;

  /**
   * Delete a selection set
   * @param name - Name of the selection set to delete
   */
  deleteSelectionSet(name: string): Promise<string>;

  // Query Tools

  /**
   * Create a new saved query
   * @param name - Name of the query
   * @param options - Query options
   */
  createQuery(name: string, options: {
    description?: string;
    queryType: 'file_filter' | 'custom_script';
    queryJSON: Record<string, any> | string;
  }): Promise<string>;

  /**
   * Execute a saved query
   * @param name - Name of the query to execute
   */
  executeQuery(name: string): Promise<any[] | any>;

  /**
   * List all saved queries
   */
  listQueries(): Promise<Query[]>;

  /**
   * Get details of a saved query
   * @param name - Name of the query
   */
  getQuery(name: string): Promise<Query | string>;

  /**
   * Update a saved query
   * @param name - Name of the query to update
   * @param queryJSON - Updated query filter as object or JSON string
   */
  updateQuery(name: string, queryJSON: Record<string, any> | string): Promise<string>;

  /**
   * Delete a saved query
   * @param name - Name of the query to delete
   */
  deleteQuery(name: string): Promise<string>;

  // File Action Tools

  /**
   * Rename files based on a regex pattern
   * @param paths - Array of file paths to rename
   * @param pattern - Regex pattern to match in the filename
   * @param replacement - Replacement string (can use $1, $2, etc. for captured groups)
   * @param options - Rename options
   */
  renameFiles(
    paths: string[] | string,
    pattern: string,
    replacement: string,
    options?: { dryRun?: boolean }
  ): Promise<FileOperationResult>;

  /**
   * Delete files or directories
   * @param paths - Array of paths to delete
   * @param options - Deletion options
   */
  deleteFiles(
    paths: string[] | string,
    options?: { recursive?: boolean; dryRun?: boolean }
  ): Promise<FileOperationResult>;

  /**
   * Move files or directories to a destination
   * @param sources - Array of source paths to move
   * @param destination - Destination directory path
   * @param options - Move options
   */
  moveFiles(
    sources: string[] | string,
    destination: string,
    options?: { dryRun?: boolean }
  ): Promise<FileOperationResult>;

  // Session Tools

  /**
   * Get session information
   */
  getSessionInfo(): Promise<SessionInfo>;

  /**
   * Set session preferences
   * @param preferences - Preferences object or JSON string
   */
  setSessionPreferences(preferences: Record<string, any> | string): Promise<string>;

  // Utility Methods

  /**
   * List all available MCP tools
   */
  listTools(): Promise<MCPTool[]>;

  /**
   * Check server health
   */
  healthCheck(): Promise<boolean>;
}

export default MCPSpaceBrowserClient;
