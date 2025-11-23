/**
 * MCP Job Manager Web Component
 *
 * A standalone web component that provides a UI for managing indexing jobs
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-job-manager api-base="http://localhost:3000"></mcp-job-manager>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - auto-refresh: Enable automatic refresh (default: true)
 *   - refresh-interval: Refresh interval in seconds (default: 3)
 */
class McpJobManager extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.autoRefresh = this.getAttribute('auto-refresh') !== 'false';
    this.refreshInterval = parseInt(this.getAttribute('refresh-interval')) || 3;
    this.requestId = 0;
    this.refreshTimer = null;
    this.jobs = [];
    this.filters = {
      activeOnly: false,
      status: null,
      minProgress: null,
      maxProgress: null,
    };
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    this.loadJobs();
    if (this.autoRefresh) {
      this.startAutoRefresh();
    }
  }

  disconnectedCallback() {
    this.stopAutoRefresh();
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

        .controls {
          display: flex;
          gap: 12px;
          margin-bottom: 20px;
          flex-wrap: wrap;
          align-items: center;
        }

        .filter-group {
          display: flex;
          align-items: center;
          gap: 8px;
        }

        label {
          font-size: 14px;
          font-weight: 500;
          color: #24292e;
        }

        select, input[type="number"] {
          padding: 6px 10px;
          font-size: 13px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          background: white;
          transition: border-color 0.2s;
        }

        select:focus, input[type="number"]:focus {
          outline: none;
          border-color: #0366d6;
        }

        input[type="checkbox"] {
          cursor: pointer;
        }

        button {
          padding: 8px 16px;
          font-size: 13px;
          font-weight: 500;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          cursor: pointer;
          transition: all 0.2s;
          background: white;
          color: #24292e;
        }

        button:hover:not(:disabled) {
          background: #f6f8fa;
          border-color: #0969da;
        }

        button:disabled {
          cursor: not-allowed;
          opacity: 0.5;
        }

        button.primary {
          background: #2ea44f;
          color: white;
          border-color: #2ea44f;
        }

        button.primary:hover:not(:disabled) {
          background: #2c974b;
        }

        button.danger {
          background: #cf222e;
          color: white;
          border-color: #cf222e;
        }

        button.danger:hover:not(:disabled) {
          background: #a40e26;
        }

        .jobs-list {
          margin-top: 16px;
        }

        .job-card {
          background: #f6f8fa;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          padding: 16px;
          margin-bottom: 12px;
          transition: border-color 0.2s;
        }

        .job-card:hover {
          border-color: #0969da;
        }

        .job-header {
          display: flex;
          justify-content: space-between;
          align-items: flex-start;
          margin-bottom: 12px;
        }

        .job-info {
          flex: 1;
        }

        .job-path {
          font-size: 14px;
          font-weight: 600;
          color: #24292e;
          margin-bottom: 4px;
          word-break: break-all;
        }

        .job-meta {
          font-size: 12px;
          color: #586069;
        }

        .job-meta code {
          background: #ffffff;
          padding: 2px 4px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
        }

        .job-actions {
          display: flex;
          gap: 8px;
        }

        .job-actions button {
          padding: 6px 12px;
          font-size: 12px;
        }

        .status-badge {
          display: inline-block;
          padding: 3px 8px;
          border-radius: 12px;
          font-size: 11px;
          font-weight: 500;
          margin-right: 8px;
        }

        .status-badge.running {
          background: #ddf4ff;
          border: 1px solid #54aeff;
          color: #0969da;
        }

        .status-badge.pending {
          background: #fff8c5;
          border: 1px solid #d4a72c;
          color: #9a6700;
        }

        .status-badge.completed {
          background: #dafbe1;
          border: 1px solid #4ac26b;
          color: #1a7f37;
        }

        .status-badge.failed {
          background: #ffebe9;
          border: 1px solid #ff7b72;
          color: #cf222e;
        }

        .status-badge.cancelled {
          background: #e1e4e8;
          border: 1px solid #d0d7de;
          color: #57606a;
        }

        .progress-bar {
          height: 6px;
          background: #e1e4e8;
          border-radius: 3px;
          overflow: hidden;
          margin-top: 8px;
        }

        .progress-fill {
          height: 100%;
          transition: width 0.3s ease;
          border-radius: 3px;
        }

        .progress-fill.running {
          background: #0969da;
        }

        .progress-fill.completed {
          background: #2ea44f;
        }

        .progress-fill.failed {
          background: #cf222e;
        }

        .progress-text {
          font-size: 11px;
          color: #586069;
          margin-top: 4px;
        }

        .activity-text {
          margin-top: 8px;
          font-size: 12px;
          color: #0969da;
          font-style: italic;
        }

        .error-text {
          margin-top: 8px;
          font-size: 12px;
          color: #cf222e;
          background: #ffebe9;
          padding: 8px;
          border-radius: 4px;
          border: 1px solid #ff7b72;
        }

        .empty-state {
          text-align: center;
          padding: 40px 20px;
          color: #586069;
        }

        .empty-state-icon {
          font-size: 48px;
          margin-bottom: 16px;
        }

        .spinner {
          display: inline-block;
          width: 14px;
          height: 14px;
          border: 2px solid rgba(0, 0, 0, 0.1);
          border-top-color: #0969da;
          border-radius: 50%;
          animation: spin 0.8s linear infinite;
          margin-right: 8px;
          vertical-align: middle;
        }

        @keyframes spin {
          to { transform: rotate(360deg); }
        }

        .loading {
          text-align: center;
          padding: 20px;
          color: #586069;
        }

        .stats {
          display: flex;
          gap: 20px;
          margin-bottom: 20px;
          padding: 12px;
          background: #f6f8fa;
          border-radius: 6px;
        }

        .stat {
          display: flex;
          flex-direction: column;
          gap: 4px;
        }

        .stat-label {
          font-size: 11px;
          color: #586069;
          text-transform: uppercase;
          font-weight: 500;
        }

        .stat-value {
          font-size: 18px;
          font-weight: 600;
          color: #24292e;
        }
      </style>

      <div class="container">
        <h2>🎯 Job Manager</h2>
        <p class="subtitle">Monitor and manage indexing jobs</p>

        <div class="controls">
          <div class="filter-group">
            <label>
              <input type="checkbox" id="activeOnly" />
              Active only
            </label>
          </div>

          <div class="filter-group">
            <label for="statusFilter">Status:</label>
            <select id="statusFilter">
              <option value="">All</option>
              <option value="pending">Pending</option>
              <option value="running">Running</option>
              <option value="paused">Paused</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="cancelled">Cancelled</option>
            </select>
          </div>

          <button id="refreshBtn" class="primary">
            🔄 Refresh
          </button>

          <button id="autoRefreshBtn">
            <span id="autoRefreshIcon">⏸️</span>
            <span id="autoRefreshText">Pause Auto-refresh</span>
          </button>
        </div>

        <div id="stats" class="stats" style="display: none;">
          <div class="stat">
            <div class="stat-label">Total Jobs</div>
            <div class="stat-value" id="totalJobs">0</div>
          </div>
          <div class="stat">
            <div class="stat-label">Active</div>
            <div class="stat-value" id="activeJobs">0</div>
          </div>
          <div class="stat">
            <div class="stat-label">Completed</div>
            <div class="stat-value" id="completedJobs">0</div>
          </div>
          <div class="stat">
            <div class="stat-label">Failed</div>
            <div class="stat-value" id="failedJobs">0</div>
          </div>
        </div>

        <div id="jobsList" class="jobs-list">
          <div class="loading">
            <span class="spinner"></span>
            Loading jobs...
          </div>
        </div>
      </div>
    `;
  }

  attachEventListeners() {
    const activeOnlyCheckbox = this.shadowRoot.getElementById('activeOnly');
    const statusFilter = this.shadowRoot.getElementById('statusFilter');
    const refreshBtn = this.shadowRoot.getElementById('refreshBtn');
    const autoRefreshBtn = this.shadowRoot.getElementById('autoRefreshBtn');

    activeOnlyCheckbox.addEventListener('change', () => {
      this.filters.activeOnly = activeOnlyCheckbox.checked;
      this.loadJobs();
    });

    statusFilter.addEventListener('change', () => {
      this.filters.status = statusFilter.value || null;
      this.loadJobs();
    });

    refreshBtn.addEventListener('click', () => {
      this.loadJobs();
    });

    autoRefreshBtn.addEventListener('click', () => {
      if (this.autoRefresh) {
        this.stopAutoRefresh();
        this.shadowRoot.getElementById('autoRefreshIcon').textContent = '▶️';
        this.shadowRoot.getElementById('autoRefreshText').textContent = 'Start Auto-refresh';
      } else {
        this.startAutoRefresh();
        this.shadowRoot.getElementById('autoRefreshIcon').textContent = '⏸️';
        this.shadowRoot.getElementById('autoRefreshText').textContent = 'Pause Auto-refresh';
      }
    });
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

  async loadJobs() {
    const jobsList = this.shadowRoot.getElementById('jobsList');

    try {
      // Build filter arguments
      const args = {};
      if (this.filters.activeOnly) {
        args.activeOnly = true;
      }
      if (this.filters.status) {
        args.status = this.filters.status;
      }

      // Call the MCP list-jobs tool
      const result = await this.callMCPTool('list-jobs', args);

      // Validate response structure
      if (!result || !result.content || !Array.isArray(result.content) || result.content.length === 0) {
        throw new Error('Invalid MCP response structure: missing content array');
      }

      if (!result.content[0].text) {
        throw new Error('Invalid MCP response: missing text content');
      }

      // Parse the response
      const response = JSON.parse(result.content[0].text);
      this.jobs = response.jobs || [];

      // Update stats
      this.updateStats();

      // Render jobs
      this.renderJobs();

      // Dispatch event
      this.dispatchEvent(new CustomEvent('jobs-loaded', {
        detail: { jobs: this.jobs },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      jobsList.innerHTML = `
        <div class="empty-state">
          <div class="empty-state-icon">❌</div>
          <div>Failed to load jobs: ${error.message}</div>
        </div>
      `;
    }
  }

  updateStats() {
    const statsDiv = this.shadowRoot.getElementById('stats');
    statsDiv.style.display = 'flex';

    const totalJobs = this.jobs.length;
    const activeJobs = this.jobs.filter(j => j.status === 'running' || j.status === 'pending').length;
    const completedJobs = this.jobs.filter(j => j.status === 'completed').length;
    const failedJobs = this.jobs.filter(j => j.status === 'failed').length;

    this.shadowRoot.getElementById('totalJobs').textContent = totalJobs;
    this.shadowRoot.getElementById('activeJobs').textContent = activeJobs;
    this.shadowRoot.getElementById('completedJobs').textContent = completedJobs;
    this.shadowRoot.getElementById('failedJobs').textContent = failedJobs;
  }

  renderJobs() {
    const jobsList = this.shadowRoot.getElementById('jobsList');

    if (this.jobs.length === 0) {
      jobsList.innerHTML = `
        <div class="empty-state">
          <div class="empty-state-icon">📭</div>
          <div>No jobs found</div>
        </div>
      `;
      return;
    }

    jobsList.innerHTML = this.jobs.map(job => this.renderJobCard(job)).join('');

    // Attach cancel button listeners
    this.jobs.forEach(job => {
      const cancelBtn = this.shadowRoot.getElementById(`cancel-${job.jobId}`);
      if (cancelBtn) {
        cancelBtn.addEventListener('click', () => this.cancelJob(job.jobId));
      }
    });
  }

  renderJobCard(job) {
    const canCancel = job.status === 'running' || job.status === 'pending';
    const progressClass = job.status === 'completed' ? 'completed' :
                          job.status === 'failed' ? 'failed' : 'running';

    let activityHTML = '';
    if (job.currentActivity) {
      activityHTML = `<div class="activity-text">📊 ${job.currentActivity}</div>`;
    }

    let errorHTML = '';
    if (job.error) {
      errorHTML = `<div class="error-text">⚠️ Error: ${job.error}</div>`;
    }

    let metadataHTML = '';
    if (job.metadata) {
      const { filesProcessed, directoriesProcessed, totalSize } = job.metadata;
      const totalSizeMB = (totalSize / (1024 * 1024)).toFixed(2);
      metadataHTML = `
        <div class="job-meta">
          📁 ${directoriesProcessed || 0} directories |
          📄 ${filesProcessed || 0} files |
          💾 ${totalSizeMB} MB
        </div>
      `;
    }

    let timestampHTML = '';
    if (job.startedAt) {
      const startTime = new Date(job.startedAt).toLocaleString();
      timestampHTML = `<div class="job-meta">Started: ${startTime}</div>`;
    }
    if (job.completedAt) {
      const endTime = new Date(job.completedAt).toLocaleString();
      timestampHTML += `<div class="job-meta">Completed: ${endTime}</div>`;
    }

    return `
      <div class="job-card">
        <div class="job-header">
          <div class="job-info">
            <div class="job-path">
              <span class="status-badge ${job.status}">${job.status.toUpperCase()}</span>
              ${job.path}
            </div>
            <div class="job-meta">Job ID: <code>${job.jobId}</code></div>
            ${timestampHTML}
            ${metadataHTML}
          </div>
          <div class="job-actions">
            ${canCancel ? `<button id="cancel-${job.jobId}" class="danger">Cancel</button>` : ''}
          </div>
        </div>

        <div class="progress-bar">
          <div class="progress-fill ${progressClass}" style="width: ${job.progress}%"></div>
        </div>
        <div class="progress-text">Progress: ${job.progress}%</div>

        ${activityHTML}
        ${errorHTML}
      </div>
    `;
  }

  async cancelJob(jobId) {
    try {
      const result = await this.callMCPTool('cancel-job', { jobId: String(jobId) });

      // Parse response
      if (!result || !result.content || !Array.isArray(result.content) || result.content.length === 0) {
        throw new Error('Invalid MCP response structure');
      }

      const response = JSON.parse(result.content[0].text);

      // Dispatch event
      this.dispatchEvent(new CustomEvent('job-cancelled', {
        detail: { jobId, response },
        bubbles: true,
        composed: true
      }));

      // Reload jobs
      await this.loadJobs();

    } catch (error) {
      alert(`Failed to cancel job: ${error.message}`);
    }
  }

  startAutoRefresh() {
    this.autoRefresh = true;
    this.refreshTimer = setInterval(() => {
      this.loadJobs();
    }, this.refreshInterval * 1000);
  }

  stopAutoRefresh() {
    this.autoRefresh = false;
    if (this.refreshTimer) {
      clearInterval(this.refreshTimer);
      this.refreshTimer = null;
    }
  }

  static get observedAttributes() {
    return ['api-base', 'auto-refresh', 'refresh-interval'];
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'api-base') {
      this.apiBase = newValue;
    } else if (name === 'auto-refresh') {
      const shouldAutoRefresh = newValue !== 'false';
      if (shouldAutoRefresh !== this.autoRefresh) {
        if (shouldAutoRefresh) {
          this.startAutoRefresh();
        } else {
          this.stopAutoRefresh();
        }
      }
    } else if (name === 'refresh-interval') {
      this.refreshInterval = parseInt(newValue) || 3;
      if (this.autoRefresh) {
        this.stopAutoRefresh();
        this.startAutoRefresh();
      }
    }
  }
}

// Register the custom element
customElements.define('mcp-job-manager', McpJobManager);
