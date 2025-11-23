/**
 * MCP Session Web Component
 *
 * A standalone web component for viewing session info and managing preferences
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-session api-base="http://localhost:3000"></mcp-session>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - auto-load: Automatically load session info on connect (default: true)
 */
class McpSession extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.autoLoad = this.getAttribute('auto-load') !== 'false';
    this.requestId = 0;
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    if (this.autoLoad) {
      this.loadSessionInfo();
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
          max-width: 700px;
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

        .info-section {
          background: #f6f8fa;
          padding: 16px;
          border-radius: 6px;
          margin-bottom: 16px;
        }

        .info-section h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
          font-weight: 600;
        }

        .info-item {
          display: flex;
          padding: 10px 0;
          border-bottom: 1px solid #d0d7de;
        }

        .info-item:last-child {
          border-bottom: none;
        }

        .info-label {
          flex: 0 0 140px;
          font-weight: 500;
          color: #24292e;
        }

        .info-value {
          flex: 1;
          color: #586069;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 13px;
          word-break: break-all;
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

        textarea {
          width: 100%;
          padding: 8px 12px;
          font-size: 13px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          box-sizing: border-box;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          resize: vertical;
          min-height: 100px;
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
          background: #0969da;
        }

        button.secondary:hover:not(:disabled) {
          background: #0860ca;
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

        .refresh-btn-container {
          margin-bottom: 16px;
          text-align: right;
        }

        .badge {
          display: inline-block;
          padding: 2px 8px;
          background: #ddf4ff;
          border: 1px solid #54aeff;
          color: #0969da;
          border-radius: 12px;
          font-size: 11px;
          font-weight: 500;
          margin-left: 8px;
        }

        .example-prefs {
          background: #ffffff;
          padding: 12px;
          border: 1px solid #d0d7de;
          border-radius: 4px;
          margin-top: 8px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
          white-space: pre-wrap;
          color: #24292e;
        }
      </style>

      <div class="container">
        <h2>⚙️ MCP Session</h2>
        <p class="subtitle">View session information and manage preferences</p>

        <div class="refresh-btn-container">
          <button id="refreshBtn" class="secondary">🔄 Refresh Info</button>
        </div>

        <div class="info-section">
          <h3>Session Information</h3>
          <div id="sessionInfo">
            <p style="text-align: center; color: #586069;">Click "Refresh Info" to load session data</p>
          </div>
        </div>

        <div class="form-section">
          <h3>Update Preferences</h3>
          <div class="form-group">
            <label for="preferences">Preferences JSON:</label>
            <textarea id="preferences" placeholder='{"theme": "dark", "pageSize": 50}'></textarea>
            <div class="help-text">Enter your session preferences as a JSON object</div>
          </div>
          <button id="updateBtn">Update Preferences</button>
          <button id="exampleBtn" class="secondary">Load Example</button>

          <div class="example-prefs">
{
  "theme": "dark",
  "pageSize": 50,
  "sortOrder": "desc",
  "dateFormat": "YYYY-MM-DD"
}</div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    this.shadowRoot.getElementById('refreshBtn').addEventListener('click', () => this.loadSessionInfo());
    this.shadowRoot.getElementById('updateBtn').addEventListener('click', () => this.updatePreferences());
    this.shadowRoot.getElementById('exampleBtn').addEventListener('click', () => this.loadExample());
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

  async loadSessionInfo() {
    this.showStatus('info', 'Loading session info...', true);

    try {
      const result = await this.callMCPTool('session-info', {});
      const info = JSON.parse(result.content[0].text);

      this.displaySessionInfo(info);
      this.shadowRoot.getElementById('status').style.display = 'none';

      this.dispatchEvent(new CustomEvent('session-loaded', {
        detail: { info },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to load session info: ${error.message}`);
    }
  }

  async updatePreferences() {
    const preferencesText = this.shadowRoot.getElementById('preferences').value.trim();

    if (!preferencesText) {
      this.showStatus('error', 'Please enter preferences JSON');
      return;
    }

    // Validate JSON
    try {
      JSON.parse(preferencesText);
    } catch (e) {
      this.showStatus('error', 'Invalid JSON format');
      return;
    }

    this.showStatus('info', 'Updating preferences...', true);

    try {
      await this.callMCPTool('session-set-preferences', {
        preferences: preferencesText
      });

      this.showStatus('success', '✓ Preferences updated successfully');

      this.dispatchEvent(new CustomEvent('preferences-updated', {
        detail: { preferences: preferencesText },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to update preferences: ${error.message}`);
    }
  }

  loadExample() {
    const examplePrefs = {
      theme: "dark",
      pageSize: 50,
      sortOrder: "desc",
      dateFormat: "YYYY-MM-DD"
    };

    this.shadowRoot.getElementById('preferences').value = JSON.stringify(examplePrefs, null, 2);
    this.showStatus('info', 'Example preferences loaded');
  }

  displaySessionInfo(info) {
    const sessionInfoDiv = this.shadowRoot.getElementById('sessionInfo');

    const items = [
      { label: 'Database', value: info.database || 'N/A' },
      { label: 'Version', value: info.version || 'N/A' },
      { label: 'Uptime', value: info.uptime || 'N/A' },
      { label: 'Server', value: this.apiBase }
    ];

    // Add any additional fields from the response
    Object.keys(info).forEach(key => {
      if (!['database', 'version', 'uptime'].includes(key)) {
        items.push({ label: this.capitalizeFirst(key), value: info[key] });
      }
    });

    sessionInfoDiv.innerHTML = items.map(item => `
      <div class="info-item">
        <div class="info-label">${item.label}:</div>
        <div class="info-value">${item.value}</div>
      </div>
    `).join('');
  }

  capitalizeFirst(str) {
    return str.charAt(0).toUpperCase() + str.slice(1).replace(/_/g, ' ');
  }

  showStatus(type, message, showSpinner = false) {
    const statusDiv = this.shadowRoot.getElementById('status');
    statusDiv.className = `status visible ${type}`;
    statusDiv.innerHTML = showSpinner
      ? `<span class="spinner"></span> ${message}`
      : message;
  }

  static get observedAttributes() {
    return ['api-base', 'auto-load'];
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'api-base') {
      this.apiBase = newValue;
    } else if (name === 'auto-load') {
      this.autoLoad = newValue !== 'false';
    }
  }
}

customElements.define('mcp-session', McpSession);
