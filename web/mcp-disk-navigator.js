/**
 * MCP Disk Navigator Web Component
 *
 * A standalone web component for navigating indexed directories and inspecting files
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-disk-navigator api-base="http://localhost:3000"></mcp-disk-navigator>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - default-path: Default path to navigate to (default: /tmp)
 */
class McpDiskNavigator extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.defaultPath = this.getAttribute('default-path') || '/tmp';
    this.requestId = 0;
    this.currentPath = null;
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
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

        .nav-controls {
          display: flex;
          gap: 8px;
          margin-bottom: 16px;
        }

        input[type="text"] {
          flex: 1;
          padding: 8px 12px;
          font-size: 14px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          box-sizing: border-box;
        }

        button {
          background: #0969da;
          color: white;
          border: none;
          padding: 8px 16px;
          font-size: 14px;
          font-weight: 500;
          border-radius: 6px;
          cursor: pointer;
          transition: background 0.2s;
        }

        button:hover:not(:disabled) {
          background: #0860ca;
        }

        button:disabled {
          background: #94d3a2;
          cursor: not-allowed;
        }

        .current-path {
          padding: 12px;
          background: #f6f8fa;
          border-radius: 6px;
          margin-bottom: 16px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 13px;
        }

        .summary {
          display: grid;
          grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
          gap: 12px;
          margin-bottom: 16px;
        }

        .summary-item {
          padding: 12px;
          background: #f6f8fa;
          border-radius: 6px;
          text-align: center;
        }

        .summary-value {
          font-size: 20px;
          font-weight: 600;
          color: #24292e;
        }

        .summary-label {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .entries {
          margin-top: 16px;
        }

        .entry {
          display: flex;
          align-items: center;
          padding: 12px;
          border-bottom: 1px solid #e1e4e8;
          cursor: pointer;
          transition: background 0.1s;
        }

        .entry:hover {
          background: #f6f8fa;
        }

        .entry-icon {
          margin-right: 12px;
          font-size: 18px;
        }

        .entry-info {
          flex: 1;
        }

        .entry-name {
          font-weight: 500;
          color: #24292e;
        }

        .entry-size {
          font-size: 12px;
          color: #586069;
          margin-left: 12px;
        }

        .entry-meta {
          font-size: 12px;
          color: #586069;
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

        .inspect-panel {
          margin-top: 16px;
          padding: 16px;
          background: #f6f8fa;
          border-radius: 6px;
          display: none;
        }

        .inspect-panel.visible {
          display: block;
        }

        .inspect-title {
          font-weight: 600;
          margin-bottom: 12px;
        }

        .inspect-detail {
          margin: 8px 0;
          font-size: 13px;
        }

        .inspect-detail strong {
          color: #24292e;
        }

        .inspect-detail code {
          background: #ffffff;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
        }
      </style>

      <div class="container">
        <h2>📁 MCP Disk Navigator</h2>
        <p class="subtitle">Navigate indexed directories via MCP protocol</p>

        <div class="nav-controls">
          <input
            type="text"
            id="pathInput"
            placeholder="/path/to/directory"
            value="${this.defaultPath}"
          />
          <button id="navigateBtn">Navigate</button>
          <button id="inspectBtn">Inspect</button>
        </div>

        <div id="currentPath" class="current-path" style="display: none;"></div>

        <div id="summary" class="summary" style="display: none;"></div>

        <div id="entries" class="entries"></div>

        <div id="inspectPanel" class="inspect-panel"></div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    const navigateBtn = this.shadowRoot.getElementById('navigateBtn');
    const inspectBtn = this.shadowRoot.getElementById('inspectBtn');
    const pathInput = this.shadowRoot.getElementById('pathInput');

    navigateBtn.addEventListener('click', () => this.navigate());
    inspectBtn.addEventListener('click', () => this.inspect());
    pathInput.addEventListener('keypress', (e) => {
      if (e.key === 'Enter') this.navigate();
    });
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

    const response = await fetch(`${this.apiBase}/mcp`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request)
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    const data = await response.json();
    if (data.error) {
      throw new Error(data.error.message || JSON.stringify(data.error));
    }

    return data.result;
  }

  async navigate() {
    const pathInput = this.shadowRoot.getElementById('pathInput');
    const path = pathInput.value.trim();

    if (!path) {
      this.showStatus('error', 'Please enter a valid path');
      return;
    }

    this.showStatus('info', 'Loading directory...', true);
    this.shadowRoot.getElementById('inspectPanel').classList.remove('visible');

    try {
      const result = await this.callMCPTool('cd', {
        path: path,
        limit: 50,
        offset: 0,
        sortBy: 'size',
        order: 'desc'
      });

      const data = JSON.parse(result.content[0].text);
      this.currentPath = data.cwd;

      // Display current path
      const currentPathDiv = this.shadowRoot.getElementById('currentPath');
      currentPathDiv.textContent = `Current: ${data.cwd}`;
      currentPathDiv.style.display = 'block';

      // Display summary
      this.displaySummary(data.summary);

      // Display entries
      this.displayEntries(data.entries);

      this.shadowRoot.getElementById('status').style.display = 'none';

      this.dispatchEvent(new CustomEvent('navigate', {
        detail: { path: data.cwd, data },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async inspect() {
    const pathInput = this.shadowRoot.getElementById('pathInput');
    const path = pathInput.value.trim();

    if (!path) {
      this.showStatus('error', 'Please enter a valid path');
      return;
    }

    this.showStatus('info', 'Inspecting...', true);

    try {
      const result = await this.callMCPTool('inspect', {
        path: path,
        limit: 20,
        offset: 0
      });

      const data = JSON.parse(result.content[0].text);
      this.displayInspect(data);

      this.shadowRoot.getElementById('status').style.display = 'none';

      this.dispatchEvent(new CustomEvent('inspect', {
        detail: { path, data },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  displaySummary(summary) {
    const summaryDiv = this.shadowRoot.getElementById('summary');
    if (!summary) {
      summaryDiv.style.display = 'none';
      return;
    }

    summaryDiv.style.display = 'grid';
    summaryDiv.innerHTML = `
      <div class="summary-item">
        <div class="summary-value">${summary.totalChildren || 0}</div>
        <div class="summary-label">Total Items</div>
      </div>
      <div class="summary-item">
        <div class="summary-value">${summary.fileCount || 0}</div>
        <div class="summary-label">Files</div>
      </div>
      <div class="summary-item">
        <div class="summary-value">${summary.directoryCount || 0}</div>
        <div class="summary-label">Directories</div>
      </div>
      <div class="summary-item">
        <div class="summary-value">${this.formatSize(summary.totalSize || 0)}</div>
        <div class="summary-label">Total Size</div>
      </div>
    `;
  }

  displayEntries(entries) {
    const entriesDiv = this.shadowRoot.getElementById('entries');
    if (!entries || entries.length === 0) {
      entriesDiv.innerHTML = '<p style="text-align: center; color: #586069;">No entries found</p>';
      return;
    }

    entriesDiv.innerHTML = entries.map(entry => `
      <div class="entry" data-path="${entry.path}">
        <div class="entry-icon">${entry.kind === 'directory' ? '📁' : '📄'}</div>
        <div class="entry-info">
          <div class="entry-name">
            ${entry.name}
            <span class="entry-size">${this.formatSize(entry.size)}</span>
          </div>
          <div class="entry-meta">${entry.kind} • ${entry.modifiedAt}</div>
        </div>
      </div>
    `).join('');

    // Add click handlers
    entriesDiv.querySelectorAll('.entry').forEach(el => {
      el.addEventListener('click', () => {
        const path = el.dataset.path;
        this.shadowRoot.getElementById('pathInput').value = path;
        this.navigate();
      });
    });
  }

  displayInspect(data) {
    const panel = this.shadowRoot.getElementById('inspectPanel');
    panel.classList.add('visible');

    panel.innerHTML = `
      <div class="inspect-title">📋 File/Directory Details</div>
      <div class="inspect-detail"><strong>Path:</strong> <code>${data.path}</code></div>
      <div class="inspect-detail"><strong>Kind:</strong> ${data.kind}</div>
      <div class="inspect-detail"><strong>Size:</strong> ${this.formatSize(data.size)}</div>
      <div class="inspect-detail"><strong>Modified:</strong> ${data.modifiedAt}</div>
      ${data.parent ? `<div class="inspect-detail"><strong>Parent:</strong> <code>${data.parent}</code></div>` : ''}
      ${data.childCount ? `<div class="inspect-detail"><strong>Children:</strong> ${data.childCount}</div>` : ''}
    `;
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
    return ['api-base', 'default-path'];
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'api-base') {
      this.apiBase = newValue;
    } else if (name === 'default-path') {
      this.defaultPath = newValue;
    }
  }
}

customElements.define('mcp-disk-navigator', McpDiskNavigator);
