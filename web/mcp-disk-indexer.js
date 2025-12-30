/**
 * MCP Disk Indexer Web Component
 *
 * A standalone web component that provides a UI for triggering MCP disk indexing
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-disk-indexer api-base="http://localhost:3000"></mcp-disk-indexer>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - default-path: Default path to index (default: /tmp)
 *   - poll-progress: Enable job progress polling (default: true)
 */
class McpDiskIndexer extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.defaultPath = this.getAttribute('default-path') || '/tmp';
    this.pollProgress = this.getAttribute('poll-progress') !== 'false';
    this.requestId = 0;
    this.currentJobId = null;
    this.pollInterval = null;
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
  }

  disconnectedCallback() {
    // Clean up polling when component is removed
    if (this.pollInterval) {
      clearInterval(this.pollInterval);
      this.pollInterval = null;
    }
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

        .progress-bar {
          margin-top: 8px;
          height: 8px;
          background: #e1e4e8;
          border-radius: 4px;
          overflow: hidden;
          display: none;
        }

        .progress-bar.visible {
          display: block;
        }

        .progress-fill {
          height: 100%;
          background: #2ea44f;
          transition: width 0.3s ease;
          width: 0%;
        }

        .checkbox-group {
          margin-bottom: 16px;
        }

        .checkbox-group label {
          display: flex;
          align-items: center;
          gap: 8px;
          cursor: pointer;
          font-size: 14px;
          color: #586069;
        }

        .checkbox-group input[type="checkbox"] {
          width: 16px;
          height: 16px;
          cursor: pointer;
        }

        .progress-text {
          margin-top: 4px;
          font-size: 12px;
          color: #586069;
          display: none;
        }

        .progress-text.visible {
          display: block;
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

        .job-info {
          margin-top: 8px;
          font-size: 12px;
          color: #586069;
        }

        .job-info code {
          background: #f6f8fa;
          padding: 2px 4px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
        }
      </style>

      <div class="container">
        <h2>🗂️ MCP Disk Indexer</h2>
        <p class="subtitle">Index filesystem paths via MCP protocol</p>

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

          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" id="autoExecutePlans" checked />
              Auto-execute lifecycle plans (classify files, generate thumbnails)
            </label>
          </div>

          <button type="submit" id="submitBtn">
            Start Indexing
          </button>
        </form>

        <div id="status" class="status">
          <span class="status-icon"></span>
          <span class="status-message"></span>
          <div id="jobInfo" class="job-info"></div>
        </div>

        <div id="progressBar" class="progress-bar">
          <div id="progressFill" class="progress-fill"></div>
        </div>
        <div id="progressText" class="progress-text"></div>

        <div class="api-info">
          <strong>MCP Endpoint:</strong> <code>${this.apiBase}/mcp</code><br>
          <strong>Protocol:</strong> JSON-RPC 2.0<br>
          <strong>Tool:</strong> <code>index</code>
        </div>
      </div>
    `;
  }

  attachEventListeners() {
    const form = this.shadowRoot.getElementById('indexForm');
    form.addEventListener('submit', (e) => this.handleSubmit(e));
  }

  /**
   * Call an MCP tool using JSON-RPC 2.0 protocol
   */
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
      headers: {
        'Content-Type': 'application/json',
      },
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

    // Clear any existing polling
    if (this.pollInterval) {
      clearInterval(this.pollInterval);
      this.pollInterval = null;
    }

    this.showStatus('info', 'Starting indexing via MCP...', true);
    this.hideProgress();

    // Get checkbox value
    const autoExecutePlans = this.shadowRoot.getElementById('autoExecutePlans').checked;

    try {
      // Call the MCP index tool
      const result = await this.callMCPTool('index', {
        root: path,
        async: true,
        autoExecutePlans: autoExecutePlans
      });

      // Validate response structure
      if (!result || !result.content || !Array.isArray(result.content) || result.content.length === 0) {
        throw new Error('Invalid MCP response structure: missing content array');
      }

      // Check for tool error (isError flag)
      if (result.isError) {
        const errorText = result.content[0]?.text || 'Unknown error';
        throw new Error(errorText);
      }

      if (!result.content[0].text) {
        throw new Error('Invalid MCP response: missing text content');
      }

      // Parse the response
      const response = JSON.parse(result.content[0].text);

      this.currentJobId = response.jobId;

      const jobInfo = this.shadowRoot.getElementById('jobInfo');
      jobInfo.innerHTML = `Job ID: <code>${response.jobId}</code> | Status: <code>${response.status}</code>`;

      // Check if job failed immediately (e.g., invalid path)
      if (response.status === 'failed') {
        const errorMsg = response.error || 'Unknown error';
        this.showStatus('error', `✗ Job ${response.jobId} failed: ${errorMsg}`);

        // Dispatch error event for failed job
        this.dispatchEvent(new CustomEvent('index-error', {
          detail: {
            path,
            jobId: response.jobId,
            error: errorMsg
          },
          bubbles: true,
          composed: true
        }));

        return; // Don't start polling for a failed job
      }

      this.showStatus('success', `✓ Indexing job created for: ${path}`);

      // Start polling for progress if enabled
      if (this.pollProgress) {
        this.startProgressPolling(response.jobId);
      }

      // Dispatch custom event
      this.dispatchEvent(new CustomEvent('index-started', {
        detail: {
          path,
          jobId: response.jobId,
          status: response.status,
          response
        },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      let errorMsg = error.message;

      // Check for common errors and provide helpful hints
      if (errorMsg.includes('No active project')) {
        errorMsg = 'No active project. Please go to the Projects tab and open a project first.';
      }

      this.showStatus('error', `✗ Error: ${errorMsg}`);

      // Dispatch error event
      this.dispatchEvent(new CustomEvent('index-error', {
        detail: { path, error: errorMsg },
        bubbles: true,
        composed: true
      }));
    } finally {
      // Re-enable form
      submitBtn.disabled = false;
      pathInput.disabled = false;
    }
  }

  async startProgressPolling(jobId) {
    // Initial poll
    await this.pollJobProgress(jobId);

    // Poll every 2 seconds
    this.pollInterval = setInterval(async () => {
      await this.pollJobProgress(jobId);
    }, 2000);
  }

  async pollJobProgress(jobId) {
    try {
      const result = await this.callMCPTool('job-progress', {
        jobId: String(jobId)
      });

      // Validate response structure
      if (!result || !result.content || !Array.isArray(result.content) || result.content.length === 0) {
        throw new Error('Invalid MCP response structure: missing content array');
      }

      if (!result.content[0].text) {
        throw new Error('Invalid MCP response: missing text content');
      }

      const progress = JSON.parse(result.content[0].text);

      // Update job info
      const jobInfo = this.shadowRoot.getElementById('jobInfo');
      jobInfo.innerHTML = `Job ID: <code>${progress.jobId}</code> | Status: <code>${progress.status}</code>`;

      // Update progress bar if available
      if (progress.progress !== undefined) {
        this.showProgress(progress.progress);
      }

      // Update status message based on job status
      if (progress.status === 'running') {
        this.showStatus('info', `⏳ Indexing in progress... (${progress.progress || 0}%)`, true);
      } else if (progress.status === 'completed') {
        this.showStatus('success', `✓ Indexing completed successfully!`);
        this.showProgress(100);
        this.stopProgressPolling();

        // Dispatch completion event
        this.dispatchEvent(new CustomEvent('index-completed', {
          detail: { jobId: progress.jobId, progress },
          bubbles: true,
          composed: true
        }));
      } else if (progress.status === 'failed') {
        this.showStatus('error', `✗ Indexing failed`);
        this.hideProgress();
        this.stopProgressPolling();

        // Dispatch failure event
        this.dispatchEvent(new CustomEvent('index-failed', {
          detail: { jobId: progress.jobId, progress },
          bubbles: true,
          composed: true
        }));
      }

    } catch (error) {
      console.error('Failed to poll job progress:', error);
      // Don't stop polling on transient errors, but log them
    }
  }

  stopProgressPolling() {
    if (this.pollInterval) {
      clearInterval(this.pollInterval);
      this.pollInterval = null;
    }
  }

  showProgress(percentage) {
    const progressBar = this.shadowRoot.getElementById('progressBar');
    const progressFill = this.shadowRoot.getElementById('progressFill');
    const progressText = this.shadowRoot.getElementById('progressText');

    progressBar.classList.add('visible');
    progressText.classList.add('visible');

    progressFill.style.width = `${percentage}%`;
    progressText.textContent = `Progress: ${percentage}%`;
  }

  hideProgress() {
    const progressBar = this.shadowRoot.getElementById('progressBar');
    const progressText = this.shadowRoot.getElementById('progressText');

    progressBar.classList.remove('visible');
    progressText.classList.remove('visible');
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
    return ['api-base', 'default-path', 'poll-progress'];
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
    } else if (name === 'poll-progress') {
      this.pollProgress = newValue !== 'false';
    }
  }
}

// Register the custom element
customElements.define('mcp-disk-indexer', McpDiskIndexer);
