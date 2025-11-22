/**
 * MCP Disk Indexer Web Component
 *
 * A standalone web component that provides a UI for triggering MCP disk indexing
 * via the mcp-space-browser REST API.
 *
 * Usage:
 *   <mcp-disk-indexer api-base="http://localhost:3000"></mcp-disk-indexer>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - default-path: Default path to index (default: /tmp)
 */
class McpDiskIndexer extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.defaultPath = this.getAttribute('default-path') || '/tmp';
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
          max-width: 600px;
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

        .form-group {
          margin-bottom: 16px;
        }

        label {
          display: block;
          margin-bottom: 8px;
          font-size: 14px;
          font-weight: 500;
          color: #24292e;
        }

        input[type="text"] {
          width: 100%;
          padding: 8px 12px;
          font-size: 14px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          box-sizing: border-box;
          transition: border-color 0.2s;
        }

        input[type="text"]:focus {
          outline: none;
          border-color: #0366d6;
          box-shadow: 0 0 0 3px rgba(3, 102, 214, 0.1);
        }

        button {
          background: #2ea44f;
          color: white;
          border: none;
          padding: 10px 20px;
          font-size: 14px;
          font-weight: 500;
          border-radius: 6px;
          cursor: pointer;
          transition: background 0.2s;
        }

        button:hover:not(:disabled) {
          background: #2c974b;
        }

        button:disabled {
          background: #94d3a2;
          cursor: not-allowed;
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

        .status-icon {
          display: inline-block;
          margin-right: 8px;
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

        .api-info {
          margin-top: 16px;
          padding: 12px;
          background: #f6f8fa;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          font-size: 12px;
          color: #57606a;
        }

        .api-info code {
          background: #ffffff;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
        }
      </style>

      <div class="container">
        <h2>🗂️ MCP Disk Indexer</h2>
        <p class="subtitle">Index filesystem paths and analyze disk space usage</p>

        <form id="indexForm">
          <div class="form-group">
            <label for="path">Path to Index:</label>
            <input
              type="text"
              id="path"
              name="path"
              placeholder="/path/to/directory"
              value="${this.defaultPath}"
              required
            />
          </div>

          <button type="submit" id="submitBtn">
            Start Indexing
          </button>
        </form>

        <div id="status" class="status">
          <span class="status-icon"></span>
          <span class="status-message"></span>
        </div>

        <div class="api-info">
          <strong>API Endpoint:</strong> <code>${this.apiBase}/api/index</code>
        </div>
      </div>
    `;
  }

  attachEventListeners() {
    const form = this.shadowRoot.getElementById('indexForm');
    form.addEventListener('submit', (e) => this.handleSubmit(e));
  }

  async handleSubmit(event) {
    event.preventDefault();

    const pathInput = this.shadowRoot.getElementById('path');
    const submitBtn = this.shadowRoot.getElementById('submitBtn');
    const path = pathInput.value.trim();

    if (!path) {
      this.showStatus('error', 'Please enter a valid path');
      return;
    }

    // Disable form during submission
    submitBtn.disabled = true;
    pathInput.disabled = true;

    this.showStatus('info', 'Indexing in progress...', true);

    try {
      const response = await fetch(`${this.apiBase}/api/index?path=${encodeURIComponent(path)}`);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }

      const result = await response.text();
      this.showStatus('success', `✓ Indexing started successfully for: ${path}`);

      // Dispatch custom event for external listeners
      this.dispatchEvent(new CustomEvent('index-started', {
        detail: { path, result },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `✗ Error: ${error.message}`);

      // Dispatch error event
      this.dispatchEvent(new CustomEvent('index-error', {
        detail: { path, error: error.message },
        bubbles: true,
        composed: true
      }));
    } finally {
      // Re-enable form
      submitBtn.disabled = false;
      pathInput.disabled = false;
    }
  }

  showStatus(type, message, showSpinner = false) {
    const statusDiv = this.shadowRoot.getElementById('status');
    const iconSpan = statusDiv.querySelector('.status-icon');
    const messageSpan = statusDiv.querySelector('.status-message');

    // Update icon
    if (showSpinner) {
      iconSpan.innerHTML = '<span class="spinner"></span>';
    } else {
      iconSpan.textContent = '';
    }

    // Update message and style
    messageSpan.textContent = message;
    statusDiv.className = `status visible ${type}`;
  }

  // Allow external API base update
  static get observedAttributes() {
    return ['api-base', 'default-path'];
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'api-base') {
      this.apiBase = newValue;
      this.render();
      this.attachEventListeners();
    } else if (name === 'default-path') {
      this.defaultPath = newValue;
      this.render();
      this.attachEventListeners();
    }
  }
}

// Register the custom element
customElements.define('mcp-disk-indexer', McpDiskIndexer);
