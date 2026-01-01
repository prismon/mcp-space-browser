/**
 * MCP Queries Web Component
 *
 * A standalone web component for managing and executing saved queries
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-queries api-base="http://localhost:3000"></mcp-queries>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 */
class McpQueries extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.requestId = 0;
    this.currentQuery = null;
    // Use shared session ID from localStorage for cross-component persistence
    this.sessionId = localStorage.getItem('mcp-session-id') || '';
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    this.loadQueries();
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
          max-width: 900px;
          margin: 0 auto;
        }

        .container {
          background: #ffffff;
          border: 1px solid #e1e4e8;
          border-radius: 8px;
          padding: 24px;
          box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
        }

        h2 {
          margin: 0 0 8px 0;
          font-size: 20px;
          font-weight: 600;
          color: #24292e;
        }

        .subtitle {
          margin: 0 0 24px 0;
          font-size: 14px;
          color: #586069;
        }

        .form-section {
          background: #f6f8fa;
          padding: 16px;
          border-radius: 6px;
          margin-bottom: 16px;
        }

        .form-section h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
          font-weight: 600;
        }

        .form-group {
          margin-bottom: 12px;
        }

        label {
          display: block;
          margin-bottom: 4px;
          font-size: 13px;
          font-weight: 500;
          color: #24292e;
        }

        input[type="text"], textarea, select {
          width: 100%;
          padding: 8px 12px;
          font-size: 14px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          box-sizing: border-box;
          font-family: inherit;
        }

        textarea {
          resize: vertical;
          min-height: 100px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 13px;
        }

        button {
          background: #2ea44f;
          color: white;
          border: none;
          padding: 8px 16px;
          font-size: 14px;
          font-weight: 500;
          border-radius: 6px;
          cursor: pointer;
          transition: background 0.2s;
          margin-right: 8px;
        }

        button:hover:not(:disabled) {
          background: #2c974b;
        }

        button:disabled {
          background: #94d3a2;
          cursor: not-allowed;
        }

        button.primary {
          background: #0969da;
        }

        button.primary:hover:not(:disabled) {
          background: #0860ca;
        }

        button.secondary {
          background: #6e7781;
        }

        button.secondary:hover:not(:disabled) {
          background: #57606a;
        }

        button.danger {
          background: #cf222e;
        }

        button.danger:hover:not(:disabled) {
          background: #a40e26;
        }

        .queries-list {
          margin-top: 16px;
        }

        .query-item {
          display: flex;
          align-items: center;
          padding: 12px;
          border: 1px solid #e1e4e8;
          border-radius: 6px;
          margin-bottom: 8px;
          cursor: pointer;
          transition: background 0.1s;
        }

        .query-item:hover {
          background: #f6f8fa;
        }

        .query-item.active {
          background: #ddf4ff;
          border-color: #54aeff;
        }

        .query-info {
          flex: 1;
        }

        .query-name {
          font-weight: 600;
          color: #24292e;
        }

        .query-meta {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .query-actions {
          display: flex;
          gap: 8px;
        }

        .query-actions button {
          padding: 6px 12px;
          font-size: 12px;
          margin: 0;
        }

        .results-panel {
          margin-top: 16px;
          padding: 16px;
          background: #f6f8fa;
          border-radius: 6px;
          display: none;
        }

        .results-panel.visible {
          display: block;
        }

        .results-panel h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
        }

        .result-item {
          padding: 8px;
          background: white;
          border: 1px solid #d0d7de;
          border-radius: 4px;
          margin-bottom: 6px;
          font-size: 13px;
        }

        .result-path {
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-weight: 500;
        }

        .result-meta {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .status {
          margin-top: 16px;
          padding: 12px;
          border-radius: 6px;
          font-size: 14px;
          display: none;
        }

        .status.visible {
          display: block;
        }

        .status.info {
          background: #ddf4ff;
          border: 1px solid #54aeff;
          color: #0969da;
        }

        .status.success {
          background: #dafbe1;
          border: 1px solid #4ac26b;
          color: #1a7f37;
        }

        .status.error {
          background: #ffebe9;
          border: 1px solid #ff7b72;
          color: #cf222e;
        }

        .spinner {
          display: inline-block;
          width: 14px;
          height: 14px;
          border: 2px solid rgba(0, 0, 0, 0.1);
          border-top-color: #0969da;
          border-radius: 50%;
          animation: spin 0.8s linear infinite;
        }

        @keyframes spin {
          to { transform: rotate(360deg); }
        }

        .help-text {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
          font-style: italic;
        }

        code {
          background: #ffffff;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
        }

        .stats-box {
          padding: 12px;
          background: #fff3cd;
          border: 1px solid #ffc107;
          border-radius: 4px;
          margin-bottom: 12px;
        }
      </style>

      <div class="container">
        <h2>🔍 MCP Queries</h2>
        <p class="subtitle">Create, manage, and execute saved queries on indexed data</p>

        <div class="form-section">
          <h3>Create New Query</h3>
          <div class="form-group">
            <label for="queryName">Name:</label>
            <input type="text" id="queryName" placeholder="large-files-query" required />
          </div>
          <div class="form-group">
            <label for="queryDescription">Description (optional):</label>
            <textarea id="queryDescription" placeholder="Find all files larger than 100MB..." rows="2"></textarea>
          </div>
          <div class="form-group">
            <label for="queryType">Query Type:</label>
            <select id="queryType">
              <option value="file_filter">File Filter</option>
              <option value="custom_script">Custom Script</option>
            </select>
          </div>
          <div class="form-group">
            <label for="queryJSON">Query JSON:</label>
            <textarea id="queryJSON" placeholder='{"minSize": 104857600, "extensions": [".mp4", ".mkv"]}' rows="4"></textarea>
            <div class="help-text">Enter filter criteria as JSON object</div>
          </div>
          <button id="createBtn">Create Query</button>
        </div>

        <div class="form-section">
          <h3>Update Query</h3>
          <div class="form-group">
            <label for="updateQueryName">Select Query:</label>
            <select id="updateQueryName">
              <option value="">-- Select a query --</option>
            </select>
          </div>
          <div class="form-group">
            <label for="updateQueryJSON">Updated Query JSON:</label>
            <textarea id="updateQueryJSON" placeholder='{"minSize": 104857600}' rows="4"></textarea>
          </div>
          <button id="updateBtn" class="primary">Update Query</button>
        </div>

        <div class="queries-list">
          <h3>Saved Queries</h3>
          <div id="queriesList"></div>
        </div>

        <div id="resultsPanel" class="results-panel">
          <h3>Results for "<span id="currentQueryName"></span>"</h3>
          <div id="resultsList"></div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    this.shadowRoot.getElementById('createBtn').addEventListener('click', () => this.createQuery());
    this.shadowRoot.getElementById('updateBtn').addEventListener('click', () => this.updateQuery());
  }

  async callMCPTool(toolName, args) {
    const requestId = ++this.requestId;
    const request = {
      jsonrpc: '2.0',
      id: requestId,
      method: 'tools/call',
      params: {
        name: toolName,
        arguments: args
      }
    };

    const headers = { 'Content-Type': 'application/json' };
    if (this.sessionId) {
      headers['Mcp-Session-Id'] = this.sessionId;
    }

    const response = await fetch(`${this.apiBase}/mcp`, {
      method: 'POST',
      headers,
      credentials: 'include',
      body: JSON.stringify(request)
    });

    const newSessionId = response.headers.get('Mcp-Session-Id');
    if (newSessionId) {
      this.sessionId = newSessionId;
      localStorage.setItem('mcp-session-id', newSessionId);
    }

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const data = await response.json();
    if (data.error) {
      throw new Error(data.error.message || JSON.stringify(data.error));
    }

    return data.result;
  }

  async loadQueries() {
    try {
      const result = await this.callMCPTool('query-list', {});
      const queries = JSON.parse(result.content[0].text);
      this.displayQueries(queries);
      this.updateQueryDropdown(queries);
    } catch (error) {
      this.showStatus('error', `Failed to load queries: ${error.message}`);
    }
  }

  async createQuery() {
    const name = this.shadowRoot.getElementById('queryName').value.trim();
    const description = this.shadowRoot.getElementById('queryDescription').value.trim();
    const queryType = this.shadowRoot.getElementById('queryType').value;
    const queryJSON = this.shadowRoot.getElementById('queryJSON').value.trim();

    if (!name) {
      this.showStatus('error', 'Please enter a name');
      return;
    }

    if (!queryJSON) {
      this.showStatus('error', 'Please enter query JSON');
      return;
    }

    // Validate JSON
    try {
      JSON.parse(queryJSON);
    } catch (e) {
      this.showStatus('error', 'Invalid JSON format');
      return;
    }

    this.showStatus('info', 'Creating query...', true);

    try {
      const args = { name, queryType, queryJSON };
      if (description) args.description = description;

      await this.callMCPTool('query-create', args);

      this.showStatus('success', `✓ Created query "${name}"`);
      this.shadowRoot.getElementById('queryName').value = '';
      this.shadowRoot.getElementById('queryDescription').value = '';
      this.shadowRoot.getElementById('queryJSON').value = '';

      // Reload the list
      await this.loadQueries();

      this.dispatchEvent(new CustomEvent('query-created', {
        detail: { name, queryType, queryJSON, description },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async updateQuery() {
    const name = this.shadowRoot.getElementById('updateQueryName').value;
    const queryJSON = this.shadowRoot.getElementById('updateQueryJSON').value.trim();

    if (!name) {
      this.showStatus('error', 'Please select a query');
      return;
    }

    if (!queryJSON) {
      this.showStatus('error', 'Please enter updated query JSON');
      return;
    }

    // Validate JSON
    try {
      JSON.parse(queryJSON);
    } catch (e) {
      this.showStatus('error', 'Invalid JSON format');
      return;
    }

    this.showStatus('info', 'Updating query...', true);

    try {
      await this.callMCPTool('query-update', { name, queryJSON });

      this.showStatus('success', `✓ Updated query "${name}"`);
      this.shadowRoot.getElementById('updateQueryJSON').value = '';

      // Reload the list
      await this.loadQueries();

      this.dispatchEvent(new CustomEvent('query-updated', {
        detail: { name, queryJSON },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async executeQuery(name) {
    this.currentQuery = name;
    this.showStatus('info', 'Executing query...', true);

    try {
      const result = await this.callMCPTool('query-execute', { name });
      const data = JSON.parse(result.content[0].text);

      this.shadowRoot.getElementById('currentQueryName').textContent = name;
      this.shadowRoot.getElementById('resultsPanel').classList.add('visible');

      const resultsList = this.shadowRoot.getElementById('resultsList');

      // Handle compressed response
      if (data._compressed) {
        resultsList.innerHTML = `
          <div class="stats-box">
            ${data._note}
          </div>
          <div style="margin-bottom: 12px; padding: 8px; background: white; border-radius: 4px;">
            <strong>Total entries:</strong> ${data.statistics.total_entries}<br>
            <strong>Total files:</strong> ${data.statistics.total_files}<br>
            <strong>Total directories:</strong> ${data.statistics.total_directories}<br>
            <strong>Total size:</strong> ${data.statistics.total_size_mb.toFixed(2)} MB
          </div>
          ${data.top_entries.map(entry => `
            <div class="result-item">
              <div class="result-path">${entry.Path}</div>
              <div class="result-meta">${entry.Kind} • ${this.formatSize(entry.Size)}</div>
            </div>
          `).join('')}
        `;
      } else {
        // Regular response with full entries array
        const entries = Array.isArray(data) ? data : [];
        if (entries.length === 0) {
          resultsList.innerHTML = '<p style="text-align: center; color: #586069;">No results found</p>';
        } else {
          resultsList.innerHTML = entries.map(entry => `
            <div class="result-item">
              <div class="result-path">${entry.Path}</div>
              <div class="result-meta">${entry.Kind} • ${this.formatSize(entry.Size)}</div>
            </div>
          `).join('');
        }
      }

      this.shadowRoot.getElementById('status').style.display = 'none';

      this.dispatchEvent(new CustomEvent('query-executed', {
        detail: { name, data },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to execute query: ${error.message}`);
    }
  }

  async viewQuery(name) {
    this.showStatus('info', 'Loading query details...', true);

    try {
      const result = await this.callMCPTool('query-get', { name });
      const query = JSON.parse(result.content[0].text);

      // Populate the update form
      this.shadowRoot.getElementById('updateQueryName').value = name;
      this.shadowRoot.getElementById('updateQueryJSON').value = query.QueryJSON || '';

      this.showStatus('success', `✓ Loaded query "${name}"`);

    } catch (error) {
      this.showStatus('error', `Failed to load query: ${error.message}`);
    }
  }

  async deleteQuery(name) {
    if (!confirm(`Are you sure you want to delete the query "${name}"?`)) {
      return;
    }

    this.showStatus('info', 'Deleting query...', true);

    try {
      await this.callMCPTool('query-delete', { name });
      this.showStatus('success', `✓ Deleted query "${name}"`);

      // Hide results panel if this was the current query
      if (this.currentQuery === name) {
        this.shadowRoot.getElementById('resultsPanel').classList.remove('visible');
        this.currentQuery = null;
      }

      // Reload the list
      await this.loadQueries();

      this.dispatchEvent(new CustomEvent('query-deleted', {
        detail: { name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  displayQueries(queries) {
    const listDiv = this.shadowRoot.getElementById('queriesList');
    if (!queries || queries.length === 0) {
      listDiv.innerHTML = '<p style="text-align: center; color: #586069;">No queries found</p>';
      return;
    }

    listDiv.innerHTML = queries.map(query => `
      <div class="query-item ${this.currentQuery === query.Name ? 'active' : ''}" data-name="${query.Name}">
        <div class="query-info">
          <div class="query-name">${query.Name}</div>
          <div class="query-meta">
            ${query.Description || 'No description'} •
            ${query.QueryType} •
            Created: ${new Date(query.CreatedAt * 1000).toLocaleDateString()}
          </div>
        </div>
        <div class="query-actions">
          <button class="primary execute-btn" data-name="${query.Name}">Execute</button>
          <button class="secondary view-btn" data-name="${query.Name}">View</button>
          <button class="danger delete-btn" data-name="${query.Name}">Delete</button>
        </div>
      </div>
    `).join('');

    // Attach event listeners
    listDiv.querySelectorAll('.execute-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.executeQuery(btn.dataset.name);
      });
    });

    listDiv.querySelectorAll('.view-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.viewQuery(btn.dataset.name);
      });
    });

    listDiv.querySelectorAll('.delete-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteQuery(btn.dataset.name);
      });
    });
  }

  updateQueryDropdown(queries) {
    const select = this.shadowRoot.getElementById('updateQueryName');
    select.innerHTML = '<option value="">-- Select a query --</option>' +
      queries.map(query => `<option value="${query.Name}">${query.Name}</option>`).join('');
  }

  formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  }

  showStatus(type, message, showSpinner = false) {
    const statusDiv = this.shadowRoot.getElementById('status');
    statusDiv.className = `status visible ${type}`;
    statusDiv.innerHTML = showSpinner
      ? `<span class="spinner"></span> ${message}`
      : message;
  }

  static get observedAttributes() {
    return ['api-base'];
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'api-base') {
      this.apiBase = newValue;
    }
  }
}

customElements.define('mcp-queries', McpQueries);
