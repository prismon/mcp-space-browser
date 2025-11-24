/**
 * MCP Space Browser JavaScript Client
 *
 * A comprehensive client library for interacting with the mcp-space-browser MCP server.
 * Provides methods for all 21 MCP tools including disk indexing, navigation, queries,
 * selection sets, file actions, and session management.
 *
 * @example
 * const client = new MCPSpaceBrowserClient('http://localhost:3000');
 *
 * // Index a directory
 * const result = await client.index('/home/user/projects', { async: true });
 * console.log(result.jobId);
 *
 * // Navigate a directory
 * const listing = await client.navigate('/home/user/projects', { limit: 50 });
 * console.log(listing.entries);
 */

class MCPSpaceBrowserClient {
  /**
   * Create a new MCP Space Browser client
   * @param {string} baseUrl - Base URL of the MCP server (e.g., 'http://localhost:3000')
   * @param {Object} options - Client options
   * @param {number} options.timeout - Request timeout in milliseconds (default: 30000)
   * @param {Object} options.headers - Additional headers to include in requests
   */
  constructor(baseUrl, options = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, ''); // Remove trailing slash
    this.mcpEndpoint = `${this.baseUrl}/mcp`;
    this.timeout = options.timeout || 30000;
    this.headers = options.headers || {};
    this._requestId = 0;
  }

  /**
   * Make an MCP JSON-RPC request
   * @private
   * @param {string} toolName - Name of the MCP tool
   * @param {Object} args - Tool arguments
   * @returns {Promise<any>} Tool result
   */
  async _callTool(toolName, args = {}) {
    const requestId = ++this._requestId;

    const payload = {
      jsonrpc: '2.0',
      method: 'tools/call',
      params: {
        name: toolName,
        arguments: args
      },
      id: requestId
    };

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(this.mcpEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...this.headers
        },
        body: JSON.stringify(payload),
        signal: controller.signal
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      const data = await response.json();

      if (data.error) {
        throw new Error(data.error.message || 'MCP tool error');
      }

      // Parse the result if it's a JSON string
      if (data.result && data.result.content && data.result.content[0]) {
        const textContent = data.result.content[0].text;
        try {
          return JSON.parse(textContent);
        } catch {
          return textContent;
        }
      }

      return data.result;
    } catch (error) {
      if (error.name === 'AbortError') {
        throw new Error(`Request timeout after ${this.timeout}ms`);
      }
      throw error;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  // ==================== Core Tools ====================

  /**
   * Index a directory tree
   * @param {string} root - Path to index
   * @param {Object} options - Indexing options
   * @param {boolean} options.async - Run asynchronously (default: true)
   * @returns {Promise<Object>} Index result with jobId (async) or statistics (sync)
   * @example
   * const result = await client.index('/home/user/projects', { async: true });
   * console.log(`Job ID: ${result.jobId}, Status URL: ${result.statusUrl}`);
   */
  async index(root, options = {}) {
    return this._callTool('index', {
      root,
      async: options.async !== undefined ? options.async : true
    });
  }

  /**
   * Navigate to a directory and list its contents
   * @param {string} path - Directory path
   * @param {Object} options - Navigation options
   * @param {number} options.limit - Maximum entries to return (default: 20)
   * @param {number} options.offset - Pagination offset
   * @param {string} options.sortBy - Sort by: 'size', 'name', or 'mtime' (default: 'size')
   * @param {string} options.order - Sort order: 'asc' or 'desc' (default: 'desc')
   * @returns {Promise<Object>} Directory listing with entries and summary
   * @example
   * const listing = await client.navigate('/home/user', { limit: 50, sortBy: 'size' });
   * console.log(`Found ${listing.count} entries`);
   * listing.entries.forEach(entry => console.log(entry.name, entry.size));
   */
  async navigate(path, options = {}) {
    return this._callTool('navigate', {
      path,
      limit: options.limit,
      offset: options.offset,
      sortBy: options.sortBy,
      order: options.order
    });
  }

  /**
   * Inspect a specific file or directory
   * @param {string} path - Path to inspect
   * @param {Object} options - Inspection options
   * @param {number} options.limit - Maximum artifacts to return (default: 20)
   * @param {number} options.offset - Pagination offset
   * @returns {Promise<Object>} Detailed metadata for the path
   * @example
   * const details = await client.inspect('/home/user/photo.jpg');
   * console.log(details.path, details.size, details.modifiedAt);
   */
  async inspect(path, options = {}) {
    return this._callTool('inspect', {
      path,
      limit: options.limit,
      offset: options.offset
    });
  }

  /**
   * Get progress status for an indexing job
   * @param {string|number} jobId - Job identifier
   * @returns {Promise<Object>} Job status and progress
   * @example
   * const status = await client.getJobProgress('123');
   * console.log(`Progress: ${status.progress}%, Status: ${status.status}`);
   */
  async getJobProgress(jobId) {
    return this._callTool('job-progress', {
      jobId: String(jobId)
    });
  }

  /**
   * List indexing jobs with optional filtering
   * @param {Object} options - Filter options
   * @param {boolean} options.activeOnly - Show only active jobs (default: false)
   * @param {string} options.status - Filter by status: 'pending', 'running', 'paused', 'completed', 'failed', or 'cancelled'
   * @param {number} options.minProgress - Filter jobs with progress >= this value (0-100)
   * @param {number} options.maxProgress - Filter jobs with progress <= this value (0-100)
   * @param {number} options.limit - Maximum jobs to return (default: 50)
   * @returns {Promise<Object>} List of jobs matching the filters
   * @example
   * const jobs = await client.listJobs({ activeOnly: true });
   * jobs.jobs.forEach(job => console.log(`Job ${job.jobId}: ${job.status}`));
   */
  async listJobs(options = {}) {
    return this._callTool('list-jobs', {
      activeOnly: options.activeOnly,
      status: options.status,
      minProgress: options.minProgress,
      maxProgress: options.maxProgress,
      limit: options.limit
    });
  }

  /**
   * Cancel a running or pending indexing job
   * @param {string|number} jobId - Job identifier to cancel
   * @returns {Promise<Object>} Cancellation result
   * @example
   * const result = await client.cancelJob('123');
   * console.log(result.message);
   */
  async cancelJob(jobId) {
    return this._callTool('cancel-job', {
      jobId: String(jobId)
    });
  }

  // ==================== Selection Set Tools ====================

  /**
   * Create a new selection set
   * @param {string} name - Name of the selection set
   * @param {Object} options - Creation options
   * @param {string} options.description - Description of the selection set
   * @param {string} options.criteriaType - Criteria type: 'user_selected' or 'tool_query' (required)
   * @returns {Promise<string>} Creation confirmation message
   * @example
   * await client.createSelectionSet('large-files', {
   *   description: 'Files larger than 100MB',
   *   criteriaType: 'tool_query'
   * });
   */
  async createSelectionSet(name, options = {}) {
    if (!options.criteriaType) {
      throw new Error('criteriaType is required');
    }
    return this._callTool('selection-set-create', {
      name,
      description: options.description,
      criteriaType: options.criteriaType
    });
  }

  /**
   * List all selection sets
   * @returns {Promise<Array>} Array of selection sets
   * @example
   * const sets = await client.listSelectionSets();
   * sets.forEach(set => console.log(set.name, set.description));
   */
  async listSelectionSets() {
    return this._callTool('selection-set-list');
  }

  /**
   * Get entries in a selection set
   * @param {string} name - Name of the selection set
   * @returns {Promise<Array|Object>} Array of entries or compressed summary
   * @example
   * const entries = await client.getSelectionSet('large-files');
   * console.log(`Found ${entries.length} files`);
   */
  async getSelectionSet(name) {
    return this._callTool('selection-set-get', { name });
  }

  /**
   * Modify a selection set by adding or removing entries
   * @param {string} name - Name of the selection set
   * @param {string} operation - Operation: 'add' or 'remove'
   * @param {string[]} paths - Array of paths to add/remove
   * @returns {Promise<string>} Modification result message
   * @example
   * await client.modifySelectionSet('large-files', 'add', ['/home/user/file1.bin', '/home/user/file2.iso']);
   * await client.modifySelectionSet('large-files', 'remove', ['/home/user/file1.bin']);
   */
  async modifySelectionSet(name, operation, paths) {
    if (!['add', 'remove'].includes(operation)) {
      throw new Error("Operation must be 'add' or 'remove'");
    }
    return this._callTool('selection-set-modify', {
      name,
      operation,
      paths: Array.isArray(paths) ? paths.join(',') : paths
    });
  }

  /**
   * Delete a selection set
   * @param {string} name - Name of the selection set to delete
   * @returns {Promise<string>} Deletion confirmation message
   * @example
   * await client.deleteSelectionSet('large-files');
   */
  async deleteSelectionSet(name) {
    return this._callTool('selection-set-delete', { name });
  }

  // ==================== Query Tools ====================

  /**
   * Create a new saved query
   * @param {string} name - Name of the query
   * @param {Object} options - Query options
   * @param {string} options.description - Description of the query
   * @param {string} options.queryType - Query type: 'file_filter' or 'custom_script' (required)
   * @param {Object|string} options.queryJSON - Query filter as object or JSON string (required)
   * @returns {Promise<string>} Creation confirmation message
   * @example
   * await client.createQuery('large-videos', {
   *   description: 'Video files larger than 100MB',
   *   queryType: 'file_filter',
   *   queryJSON: { minSize: 104857600, extensions: ['.mp4', '.mov'] }
   * });
   */
  async createQuery(name, options = {}) {
    if (!options.queryType) {
      throw new Error('queryType is required');
    }
    if (!options.queryJSON) {
      throw new Error('queryJSON is required');
    }

    const queryJSON = typeof options.queryJSON === 'string'
      ? options.queryJSON
      : JSON.stringify(options.queryJSON);

    return this._callTool('query-create', {
      name,
      description: options.description,
      queryType: options.queryType,
      queryJSON
    });
  }

  /**
   * Execute a saved query
   * @param {string} name - Name of the query to execute
   * @returns {Promise<Array|Object>} Query results or compressed summary
   * @example
   * const results = await client.executeQuery('large-videos');
   * console.log(`Found ${results.length} matching files`);
   */
  async executeQuery(name) {
    return this._callTool('query-execute', { name });
  }

  /**
   * List all saved queries
   * @returns {Promise<Array>} Array of queries
   * @example
   * const queries = await client.listQueries();
   * queries.forEach(q => console.log(q.name, q.queryType));
   */
  async listQueries() {
    return this._callTool('query-list');
  }

  /**
   * Get details of a saved query
   * @param {string} name - Name of the query
   * @returns {Promise<Object>} Query details
   * @example
   * const query = await client.getQuery('large-videos');
   * console.log(query.queryJSON);
   */
  async getQuery(name) {
    return this._callTool('query-get', { name });
  }

  /**
   * Update a saved query
   * @param {string} name - Name of the query to update
   * @param {Object|string} queryJSON - Updated query filter as object or JSON string
   * @returns {Promise<string>} Update confirmation message
   * @example
   * await client.updateQuery('large-videos', {
   *   minSize: 209715200,
   *   extensions: ['.mp4', '.mov', '.mkv']
   * });
   */
  async updateQuery(name, queryJSON) {
    const queryJSONStr = typeof queryJSON === 'string'
      ? queryJSON
      : JSON.stringify(queryJSON);

    return this._callTool('query-update', {
      name,
      queryJSON: queryJSONStr
    });
  }

  /**
   * Delete a saved query
   * @param {string} name - Name of the query to delete
   * @returns {Promise<string>} Deletion confirmation message
   * @example
   * await client.deleteQuery('large-videos');
   */
  async deleteQuery(name) {
    return this._callTool('query-delete', { name });
  }

  // ==================== File Action Tools ====================

  /**
   * Rename files based on a regex pattern
   * @param {string[]} paths - Array of file paths to rename
   * @param {string} pattern - Regex pattern to match in the filename
   * @param {string} replacement - Replacement string (can use $1, $2, etc. for captured groups)
   * @param {Object} options - Rename options
   * @param {boolean} options.dryRun - Preview changes without executing (default: false)
   * @returns {Promise<Object>} Rename results with successCount and errorCount
   * @example
   * const result = await client.renameFiles(
   *   ['/home/user/photo_001.jpg', '/home/user/photo_002.jpg'],
   *   'photo_(\\d+)',
   *   'image_$1',
   *   { dryRun: true }
   * );
   * console.log(`Would rename ${result.successCount} files`);
   */
  async renameFiles(paths, pattern, replacement, options = {}) {
    return this._callTool('rename-files', {
      paths: Array.isArray(paths) ? paths.join(',') : paths,
      pattern,
      replacement,
      dryRun: options.dryRun
    });
  }

  /**
   * Delete files or directories
   * @param {string[]} paths - Array of paths to delete
   * @param {Object} options - Deletion options
   * @param {boolean} options.recursive - Delete directories recursively (default: false)
   * @param {boolean} options.dryRun - Preview changes without executing (default: false)
   * @returns {Promise<Object>} Deletion results with successCount and errorCount
   * @example
   * const result = await client.deleteFiles(
   *   ['/home/user/temp/file1.txt', '/home/user/temp/old_dir'],
   *   { recursive: true, dryRun: true }
   * );
   * console.log(`Would delete ${result.successCount} items`);
   */
  async deleteFiles(paths, options = {}) {
    return this._callTool('delete-files', {
      paths: Array.isArray(paths) ? paths.join(',') : paths,
      recursive: options.recursive,
      dryRun: options.dryRun
    });
  }

  /**
   * Move files or directories to a destination
   * @param {string[]} sources - Array of source paths to move
   * @param {string} destination - Destination directory path
   * @param {Object} options - Move options
   * @param {boolean} options.dryRun - Preview changes without executing (default: false)
   * @returns {Promise<Object>} Move results with successCount and errorCount
   * @example
   * const result = await client.moveFiles(
   *   ['/home/user/file1.txt', '/home/user/file2.txt'],
   *   '/home/user/archive',
   *   { dryRun: false }
   * );
   * console.log(`Moved ${result.successCount} files`);
   */
  async moveFiles(sources, destination, options = {}) {
    return this._callTool('move-files', {
      sources: Array.isArray(sources) ? sources.join(',') : sources,
      destination,
      dryRun: options.dryRun
    });
  }

  // ==================== Session Tools ====================

  /**
   * Get session information
   * @returns {Promise<Object>} Session info including database path, version, and cwd
   * @example
   * const info = await client.getSessionInfo();
   * console.log(`Database: ${info.database}, Version: ${info.version}`);
   */
  async getSessionInfo() {
    return this._callTool('session-info');
  }

  /**
   * Set session preferences
   * @param {Object|string} preferences - Preferences object or JSON string
   * @returns {Promise<string>} Update confirmation message
   * @example
   * await client.setSessionPreferences({ theme: 'dark', pageSize: 50 });
   */
  async setSessionPreferences(preferences) {
    const preferencesStr = typeof preferences === 'string'
      ? preferences
      : JSON.stringify(preferences);

    return this._callTool('session-set-preferences', {
      preferences: preferencesStr
    });
  }

  // ==================== Utility Methods ====================

  /**
   * List all available MCP tools
   * @returns {Promise<Array>} Array of available tools with descriptions
   * @example
   * const tools = await client.listTools();
   * tools.forEach(tool => console.log(tool.name, tool.description));
   */
  async listTools() {
    const requestId = ++this._requestId;

    const payload = {
      jsonrpc: '2.0',
      method: 'tools/list',
      params: {},
      id: requestId
    };

    const response = await fetch(this.mcpEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...this.headers
      },
      body: JSON.stringify(payload)
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const data = await response.json();
    return data.result?.tools || [];
  }

  /**
   * Check server health
   * @returns {Promise<boolean>} True if server is responding
   * @example
   * const healthy = await client.healthCheck();
   * console.log(healthy ? 'Server is up' : 'Server is down');
   */
  async healthCheck() {
    try {
      await this.listTools();
      return true;
    } catch {
      return false;
    }
  }
}

// Export for different module systems
if (typeof module !== 'undefined' && module.exports) {
  module.exports = MCPSpaceBrowserClient;
}

if (typeof window !== 'undefined') {
  window.MCPSpaceBrowserClient = MCPSpaceBrowserClient;
}
