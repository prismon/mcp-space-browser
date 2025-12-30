/**
 * MCP Projects Web Component
 *
 * A standalone web component for managing projects (create, list, open/switch)
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-projects api-base="http://localhost:3000"></mcp-projects>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - auto-load: Automatically load projects on connect (default: true)
 *
 * Events:
 *   - project-created: Fired when a project is created
 *   - project-opened: Fired when a project is opened/switched
 *   - project-closed: Fired when a project is closed
 *   - projects-loaded: Fired when project list is refreshed
 */
class McpProjects extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.autoLoad = this.getAttribute('auto-load') !== 'false';
    this.requestId = 0;
    this.projects = [];
    this.activeProject = null;
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    if (this.autoLoad) {
      this.loadProjects();
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
          max-width: 800px;
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

        .header-row {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 16px;
        }

        .active-project-badge {
          display: inline-flex;
          align-items: center;
          gap: 6px;
          padding: 6px 12px;
          background: #dafbe1;
          border: 1px solid #4ac26b;
          border-radius: 20px;
          font-size: 13px;
          font-weight: 500;
          color: #1a7f37;
        }

        .active-project-badge.no-project {
          background: #fff8c5;
          border-color: #d4a72c;
          color: #9a6700;
        }

        .section {
          background: #f6f8fa;
          padding: 16px;
          border-radius: 6px;
          margin-bottom: 16px;
        }

        .section h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
          font-weight: 600;
          color: #24292e;
        }

        .form-row {
          display: flex;
          gap: 12px;
          align-items: flex-end;
        }

        .form-group {
          flex: 1;
        }

        .form-group.description {
          flex: 2;
        }

        label {
          display: block;
          margin-bottom: 4px;
          font-size: 13px;
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
        }

        input[type="text"]:focus {
          outline: none;
          border-color: #0969da;
          box-shadow: 0 0 0 3px rgba(9, 105, 218, 0.1);
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
          white-space: nowrap;
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

        button.danger {
          background: #cf222e;
        }

        button.danger:hover:not(:disabled) {
          background: #a40e26;
        }

        button.outline {
          background: transparent;
          border: 1px solid #d1d5da;
          color: #24292e;
        }

        button.outline:hover:not(:disabled) {
          background: #f6f8fa;
        }

        .project-list {
          display: flex;
          flex-direction: column;
          gap: 8px;
        }

        .project-card {
          display: flex;
          justify-content: space-between;
          align-items: center;
          padding: 12px 16px;
          background: #ffffff;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          transition: border-color 0.2s, box-shadow 0.2s;
        }

        .project-card:hover {
          border-color: #0969da;
        }

        .project-card.active {
          border-color: #2ea44f;
          background: #f6fff8;
          box-shadow: 0 0 0 1px #2ea44f;
        }

        .project-info {
          flex: 1;
        }

        .project-name {
          font-weight: 600;
          font-size: 15px;
          color: #24292e;
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .project-name .active-tag {
          display: inline-block;
          padding: 2px 8px;
          background: #2ea44f;
          color: white;
          border-radius: 12px;
          font-size: 11px;
          font-weight: 500;
        }

        .project-description {
          font-size: 13px;
          color: #586069;
          margin-top: 4px;
        }

        .project-meta {
          font-size: 12px;
          color: #8b949e;
          margin-top: 4px;
        }

        .project-actions {
          display: flex;
          gap: 8px;
        }

        .project-actions button {
          padding: 6px 12px;
          font-size: 13px;
        }

        .empty-state {
          text-align: center;
          padding: 32px;
          color: #586069;
        }

        .empty-state .icon {
          font-size: 48px;
          margin-bottom: 12px;
        }

        .empty-state p {
          margin: 0;
          font-size: 14px;
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

        .loading-overlay {
          display: flex;
          justify-content: center;
          align-items: center;
          padding: 32px;
          color: #586069;
        }

        .loading-overlay .spinner {
          margin-right: 8px;
        }
      </style>

      <div class="container">
        <div class="header-row">
          <div>
            <h2>Projects</h2>
            <p class="subtitle">Manage isolated workspaces with separate databases</p>
          </div>
          <div id="activeProjectBadge" class="active-project-badge no-project">
            <span>No active project</span>
          </div>
        </div>

        <div class="section">
          <h3>Create New Project</h3>
          <div class="form-row">
            <div class="form-group">
              <label for="projectName">Project Name *</label>
              <input type="text" id="projectName" placeholder="my-project" pattern="[a-zA-Z0-9_-]+" />
            </div>
            <div class="form-group description">
              <label for="projectDescription">Description (optional)</label>
              <input type="text" id="projectDescription" placeholder="Description of the project" />
            </div>
            <button id="createBtn">Create Project</button>
          </div>
        </div>

        <div class="section">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h3 style="margin: 0;">Available Projects</h3>
            <button id="refreshBtn" class="outline">Refresh</button>
          </div>
          <div id="projectList" class="project-list">
            <div class="loading-overlay">
              <span class="spinner"></span> Loading projects...
            </div>
          </div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    this.shadowRoot.getElementById('createBtn').addEventListener('click', () => this.createProject());
    this.shadowRoot.getElementById('refreshBtn').addEventListener('click', () => this.loadProjects());

    // Allow Enter key in project name field to create
    this.shadowRoot.getElementById('projectName').addEventListener('keypress', (e) => {
      if (e.key === 'Enter') {
        this.createProject();
      }
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

  async loadProjects() {
    const projectList = this.shadowRoot.getElementById('projectList');
    projectList.innerHTML = `
      <div class="loading-overlay">
        <span class="spinner"></span> Loading projects...
      </div>
    `;

    try {
      const result = await this.callMCPTool('project-list', {});
      const data = JSON.parse(result.content[0].text);
      this.projects = data.projects || [];
      this.activeProject = data.activeProject || null;

      this.renderProjectList();
      this.updateActiveProjectBadge();

      this.dispatchEvent(new CustomEvent('projects-loaded', {
        detail: { projects: this.projects, activeProject: this.activeProject },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      projectList.innerHTML = `
        <div class="empty-state">
          <div class="icon">Error loading projects: ${error.message}</div>
        </div>
      `;
      this.showStatus('error', `Failed to load projects: ${error.message}`);
    }
  }

  renderProjectList() {
    const projectList = this.shadowRoot.getElementById('projectList');

    if (this.projects.length === 0) {
      projectList.innerHTML = `
        <div class="empty-state">
          <div class="icon">&#128193;</div>
          <p>No projects yet. Create your first project above!</p>
        </div>
      `;
      return;
    }

    projectList.innerHTML = this.projects.map(project => {
      const isActive = project.isActive || project.name === this.activeProject;
      const createdDate = this.formatDate(project.createdAt);

      return `
        <div class="project-card ${isActive ? 'active' : ''}" data-name="${this.escapeHtml(project.name)}">
          <div class="project-info">
            <div class="project-name">
              ${this.escapeHtml(project.name)}
              ${isActive ? '<span class="active-tag">Active</span>' : ''}
            </div>
            ${project.description ? `<div class="project-description">${this.escapeHtml(project.description)}</div>` : ''}
            <div class="project-meta">Created: ${createdDate}</div>
          </div>
          <div class="project-actions">
            ${isActive
              ? `<button class="outline close-btn" data-name="${this.escapeHtml(project.name)}">Close</button>`
              : `<button class="secondary open-btn" data-name="${this.escapeHtml(project.name)}">Open</button>`
            }
            <button class="danger delete-btn" data-name="${this.escapeHtml(project.name)}">Delete</button>
          </div>
        </div>
      `;
    }).join('');

    // Attach event listeners to buttons
    projectList.querySelectorAll('.open-btn').forEach(btn => {
      btn.addEventListener('click', (e) => this.openProject(e.target.dataset.name));
    });

    projectList.querySelectorAll('.close-btn').forEach(btn => {
      btn.addEventListener('click', () => this.closeProject());
    });

    projectList.querySelectorAll('.delete-btn').forEach(btn => {
      btn.addEventListener('click', (e) => this.deleteProject(e.target.dataset.name));
    });
  }

  updateActiveProjectBadge() {
    const badge = this.shadowRoot.getElementById('activeProjectBadge');
    if (this.activeProject) {
      badge.className = 'active-project-badge';
      badge.innerHTML = `<span>Active: ${this.escapeHtml(this.activeProject)}</span>`;
    } else {
      badge.className = 'active-project-badge no-project';
      badge.innerHTML = `<span>No active project</span>`;
    }
  }

  async createProject() {
    const nameInput = this.shadowRoot.getElementById('projectName');
    const descInput = this.shadowRoot.getElementById('projectDescription');
    const name = nameInput.value.trim();
    const description = descInput.value.trim();

    if (!name) {
      this.showStatus('error', 'Please enter a project name');
      nameInput.focus();
      return;
    }

    // Validate name format
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
      this.showStatus('error', 'Project name can only contain letters, numbers, underscores, and hyphens');
      nameInput.focus();
      return;
    }

    const createBtn = this.shadowRoot.getElementById('createBtn');
    createBtn.disabled = true;
    this.showStatus('info', 'Creating project...', true);

    try {
      const result = await this.callMCPTool('project-create', { name, description });
      const data = JSON.parse(result.content[0].text);

      // Clear form
      nameInput.value = '';
      descInput.value = '';

      this.showStatus('success', `Project "${name}" created successfully!`);

      // Reload projects list
      await this.loadProjects();

      this.dispatchEvent(new CustomEvent('project-created', {
        detail: { project: data },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to create project: ${error.message}`);
    } finally {
      createBtn.disabled = false;
    }
  }

  async openProject(name) {
    this.showStatus('info', `Opening project "${name}"...`, true);

    try {
      await this.callMCPTool('project-open', { name });
      this.activeProject = name;

      this.showStatus('success', `Project "${name}" is now active`);

      // Reload to update active state
      await this.loadProjects();

      this.dispatchEvent(new CustomEvent('project-opened', {
        detail: { projectName: name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to open project: ${error.message}`);
    }
  }

  async closeProject() {
    if (!this.activeProject) {
      this.showStatus('error', 'No active project to close');
      return;
    }

    const projectName = this.activeProject;
    this.showStatus('info', `Closing project "${projectName}"...`, true);

    try {
      await this.callMCPTool('project-close', {});
      this.activeProject = null;

      this.showStatus('success', `Project "${projectName}" closed`);

      // Reload to update active state
      await this.loadProjects();

      this.dispatchEvent(new CustomEvent('project-closed', {
        detail: { projectName },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to close project: ${error.message}`);
    }
  }

  async deleteProject(name) {
    if (!confirm(`Are you sure you want to delete project "${name}"? This will permanently delete all indexed data.`)) {
      return;
    }

    this.showStatus('info', `Deleting project "${name}"...`, true);

    try {
      await this.callMCPTool('project-delete', { name, confirm: true });

      if (this.activeProject === name) {
        this.activeProject = null;
      }

      this.showStatus('success', `Project "${name}" deleted`);

      // Reload projects list
      await this.loadProjects();

      this.dispatchEvent(new CustomEvent('project-deleted', {
        detail: { projectName: name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to delete project: ${error.message}`);
    }
  }

  escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  formatDate(dateValue) {
    if (!dateValue) return 'Unknown';

    let date;
    // Handle ISO 8601 string (e.g., "2024-01-15T10:30:00Z")
    if (typeof dateValue === 'string') {
      date = new Date(dateValue);
    }
    // Handle Unix timestamp (seconds)
    else if (typeof dateValue === 'number') {
      // If it looks like milliseconds (> year 2001 in ms), use directly
      // Otherwise assume seconds and multiply by 1000
      date = dateValue > 1000000000000 ? new Date(dateValue) : new Date(dateValue * 1000);
    }
    else {
      return 'Unknown';
    }

    // Check for invalid date
    if (isNaN(date.getTime())) {
      return 'Unknown';
    }

    return date.toLocaleDateString();
  }

  showStatus(type, message, showSpinner = false) {
    const statusDiv = this.shadowRoot.getElementById('status');
    statusDiv.className = `status visible ${type}`;
    statusDiv.innerHTML = showSpinner
      ? `<span class="spinner"></span> ${message}`
      : message;

    // Auto-hide success messages after 3 seconds
    if (type === 'success') {
      setTimeout(() => {
        statusDiv.classList.remove('visible');
      }, 3000);
    }
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

customElements.define('mcp-projects', McpProjects);
