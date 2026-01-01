/**
 * MCP Selection Sets Web Component
 *
 * A standalone web component for managing selection sets
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-selection-sets api-base="http://localhost:3000"></mcp-selection-sets>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 */
class McpSelectionSets extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.requestId = 0;
    this.currentSet = null;
    // Use shared session ID from localStorage for cross-component persistence
    this.sessionId = localStorage.getItem('mcp-session-id') || '';
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    this.loadSelectionSets();
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
          min-height: 60px;
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

        .sets-list {
          margin-top: 16px;
        }

        .set-item {
          display: flex;
          align-items: center;
          padding: 12px;
          border: 1px solid #e1e4e8;
          border-radius: 6px;
          margin-bottom: 8px;
          cursor: pointer;
          transition: background 0.1s;
        }

        .set-item:hover {
          background: #f6f8fa;
        }

        .set-item.active {
          background: #ddf4ff;
          border-color: #54aeff;
        }

        .set-info {
          flex: 1;
        }

        .set-name {
          font-weight: 600;
          color: #24292e;
        }

        .set-meta {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .set-actions {
          display: flex;
          gap: 8px;
        }

        .set-actions button {
          padding: 6px 12px;
          font-size: 12px;
          margin: 0;
        }

        .entries-panel {
          margin-top: 16px;
          padding: 16px;
          background: #f6f8fa;
          border-radius: 6px;
          display: none;
        }

        .entries-panel.visible {
          display: block;
        }

        .entries-panel h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
        }

        .entry-item {
          padding: 8px;
          background: white;
          border: 1px solid #d0d7de;
          border-radius: 4px;
          margin-bottom: 6px;
          font-size: 13px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
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
      </style>

      <div class="container">
        <h2>📦 MCP Selection Sets</h2>
        <p class="subtitle">Manage selection sets for organizing indexed files and directories</p>

        <div class="form-section">
          <h3>Create New Selection Set</h3>
          <div class="form-group">
            <label for="setName">Name:</label>
            <input type="text" id="setName" placeholder="my-selection-set" required />
          </div>
          <div class="form-group">
            <label for="setDescription">Description (optional):</label>
            <textarea id="setDescription" placeholder="A collection of important files..."></textarea>
          </div>
          <div class="form-group">
            <label for="criteriaType">Criteria Type:</label>
            <select id="criteriaType">
              <option value="user_selected">User Selected</option>
              <option value="tool_query">Tool Query</option>
            </select>
          </div>
          <button id="createBtn">Create Selection Set</button>
        </div>

        <div class="form-section">
          <h3>Modify Selection Set</h3>
          <div class="form-group">
            <label for="modifySetName">Selection Set:</label>
            <select id="modifySetName">
              <option value="">-- Select a set --</option>
            </select>
          </div>
          <div class="form-group">
            <label for="modifyPaths">Paths (comma-separated):</label>
            <input type="text" id="modifyPaths" placeholder="/path/one, /path/two" />
          </div>
          <button id="addBtn">Add Entries</button>
          <button id="removeBtn" class="danger">Remove Entries</button>
        </div>

        <div class="sets-list">
          <h3>Selection Sets</h3>
          <div id="setsList"></div>
        </div>

        <div id="entriesPanel" class="entries-panel">
          <h3>Entries in "<span id="currentSetName"></span>"</h3>
          <div id="entriesList"></div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    this.shadowRoot.getElementById('createBtn').addEventListener('click', () => this.createSet());
    this.shadowRoot.getElementById('addBtn').addEventListener('click', () => this.modifySet('add'));
    this.shadowRoot.getElementById('removeBtn').addEventListener('click', () => this.modifySet('remove'));
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

  async loadSelectionSets() {
    try {
      const result = await this.callMCPTool('selection-set-list', {});
      const sets = JSON.parse(result.content[0].text);
      this.displaySets(sets);
      this.updateModifyDropdown(sets);
    } catch (error) {
      this.showStatus('error', `Failed to load selection sets: ${error.message}`);
    }
  }

  async createSet() {
    const name = this.shadowRoot.getElementById('setName').value.trim();
    const description = this.shadowRoot.getElementById('setDescription').value.trim();
    const criteriaType = this.shadowRoot.getElementById('criteriaType').value;

    if (!name) {
      this.showStatus('error', 'Please enter a name');
      return;
    }

    this.showStatus('info', 'Creating selection set...', true);

    try {
      const args = { name, criteriaType };
      if (description) args.description = description;

      await this.callMCPTool('selection-set-create', args);

      this.showStatus('success', `✓ Created selection set "${name}"`);
      this.shadowRoot.getElementById('setName').value = '';
      this.shadowRoot.getElementById('setDescription').value = '';

      // Reload the list
      await this.loadSelectionSets();

      this.dispatchEvent(new CustomEvent('set-created', {
        detail: { name, criteriaType, description },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async modifySet(operation) {
    const name = this.shadowRoot.getElementById('modifySetName').value;
    const paths = this.shadowRoot.getElementById('modifyPaths').value.trim();

    if (!name) {
      this.showStatus('error', 'Please select a selection set');
      return;
    }

    if (!paths) {
      this.showStatus('error', 'Please enter paths');
      return;
    }

    this.showStatus('info', `${operation === 'add' ? 'Adding' : 'Removing'} entries...`, true);

    try {
      await this.callMCPTool('selection-set-modify', {
        name,
        operation,
        paths
      });

      this.showStatus('success', `✓ Successfully ${operation === 'add' ? 'added' : 'removed'} entries`);
      this.shadowRoot.getElementById('modifyPaths').value = '';

      // Reload entries if this is the current set
      if (this.currentSet === name) {
        await this.loadSetEntries(name);
      }

      this.dispatchEvent(new CustomEvent('set-modified', {
        detail: { name, operation, paths },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async loadSetEntries(name) {
    this.currentSet = name;
    this.showStatus('info', 'Loading entries...', true);

    try {
      const result = await this.callMCPTool('selection-set-get', { name });
      const data = JSON.parse(result.content[0].text);

      this.shadowRoot.getElementById('currentSetName').textContent = name;
      this.shadowRoot.getElementById('entriesPanel').classList.add('visible');

      const entriesList = this.shadowRoot.getElementById('entriesList');

      // Handle compressed response
      if (data._compressed) {
        entriesList.innerHTML = `
          <div style="padding: 12px; background: #fff3cd; border: 1px solid #ffc107; border-radius: 4px; margin-bottom: 12px;">
            ${data._note}
          </div>
          <div style="margin-bottom: 12px; padding: 8px; background: white; border-radius: 4px;">
            <strong>Total entries:</strong> ${data.statistics.total_entries}<br>
            <strong>Total files:</strong> ${data.statistics.total_files}<br>
            <strong>Total directories:</strong> ${data.statistics.total_directories}<br>
            <strong>Total size:</strong> ${data.statistics.total_size_mb.toFixed(2)} MB
          </div>
          ${data.top_entries.map(entry => `
            <div class="entry-item">${entry.Path} (${this.formatSize(entry.Size)})</div>
          `).join('')}
        `;
      } else {
        // Regular response with full entries array
        const entries = Array.isArray(data) ? data : [];
        if (entries.length === 0) {
          entriesList.innerHTML = '<p style="text-align: center; color: #586069;">No entries in this set</p>';
        } else {
          entriesList.innerHTML = entries.map(entry => `
            <div class="entry-item">${entry.Path} (${this.formatSize(entry.Size)})</div>
          `).join('');
        }
      }

      this.shadowRoot.getElementById('status').style.display = 'none';

    } catch (error) {
      this.showStatus('error', `Failed to load entries: ${error.message}`);
    }
  }

  async deleteSet(name) {
    if (!confirm(`Are you sure you want to delete the selection set "${name}"?`)) {
      return;
    }

    this.showStatus('info', 'Deleting selection set...', true);

    try {
      await this.callMCPTool('selection-set-delete', { name });
      this.showStatus('success', `✓ Deleted selection set "${name}"`);

      // Hide entries panel if this was the current set
      if (this.currentSet === name) {
        this.shadowRoot.getElementById('entriesPanel').classList.remove('visible');
        this.currentSet = null;
      }

      // Reload the list
      await this.loadSelectionSets();

      this.dispatchEvent(new CustomEvent('set-deleted', {
        detail: { name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  displaySets(sets) {
    const listDiv = this.shadowRoot.getElementById('setsList');
    if (!sets || sets.length === 0) {
      listDiv.innerHTML = '<p style="text-align: center; color: #586069;">No selection sets found</p>';
      return;
    }

    listDiv.innerHTML = sets.map(set => `
      <div class="set-item ${this.currentSet === set.name ? 'active' : ''}" data-name="${set.name}">
        <div class="set-info">
          <div class="set-name">${set.name}</div>
          <div class="set-meta">
            ${set.description || 'No description'} •
            ${set.criteria_type} •
            Created: ${new Date(set.created_at * 1000).toLocaleDateString()}
          </div>
        </div>
        <div class="set-actions">
          <button class="secondary view-btn" data-name="${set.name}">View</button>
          <button class="danger delete-btn" data-name="${set.name}">Delete</button>
        </div>
      </div>
    `).join('');

    // Attach event listeners
    listDiv.querySelectorAll('.view-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.loadSetEntries(btn.dataset.name);
      });
    });

    listDiv.querySelectorAll('.delete-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteSet(btn.dataset.name);
      });
    });
  }

  updateModifyDropdown(sets) {
    const select = this.shadowRoot.getElementById('modifySetName');
    select.innerHTML = '<option value="">-- Select a set --</option>' +
      sets.map(set => `<option value="${set.name}">${set.name}</option>`).join('');
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

customElements.define('mcp-selection-sets', McpSelectionSets);
