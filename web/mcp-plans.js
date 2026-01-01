/**
 * MCP Plans Web Component
 *
 * A standalone web component for managing plans (list, view, edit, create, delete)
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-plans api-base="http://localhost:3000"></mcp-plans>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 *   - auto-load: Automatically load plans on connect (default: true)
 *
 * Events:
 *   - plan-created: Fired when a plan is created
 *   - plan-updated: Fired when a plan is updated
 *   - plan-deleted: Fired when a plan is deleted
 *   - plans-loaded: Fired when plan list is refreshed
 */
class McpPlans extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.autoLoad = this.getAttribute('auto-load') !== 'false';
    this.requestId = 0;
    this.plans = [];
    this.selectedPlan = null;
    this.editMode = false;
    this.createMode = false;
    // Use shared session ID from localStorage for cross-component persistence
    this.sessionId = localStorage.getItem('mcp-session-id') || '';
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    if (this.autoLoad) {
      this.loadPlans();
    }
  }

  render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
          max-width: 1000px;
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
          flex-wrap: wrap;
        }

        .form-group {
          flex: 1;
          min-width: 200px;
        }

        .form-group.full-width {
          flex: 100%;
        }

        label {
          display: block;
          margin-bottom: 4px;
          font-size: 13px;
          font-weight: 500;
          color: #24292e;
        }

        input[type="text"],
        select,
        textarea {
          width: 100%;
          padding: 8px 12px;
          font-size: 14px;
          border: 1px solid #d1d5da;
          border-radius: 6px;
          box-sizing: border-box;
          font-family: inherit;
        }

        textarea {
          min-height: 120px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
          resize: vertical;
        }

        input[type="text"]:focus,
        select:focus,
        textarea:focus {
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

        .plan-list {
          display: flex;
          flex-direction: column;
          gap: 8px;
        }

        .plan-card {
          display: flex;
          justify-content: space-between;
          align-items: center;
          padding: 12px 16px;
          background: #ffffff;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          transition: border-color 0.2s, box-shadow 0.2s;
          cursor: pointer;
        }

        .plan-card:hover {
          border-color: #0969da;
        }

        .plan-card.selected {
          border-color: #2ea44f;
          background: #f6fff8;
          box-shadow: 0 0 0 1px #2ea44f;
        }

        .plan-info {
          flex: 1;
        }

        .plan-name {
          font-weight: 600;
          font-size: 15px;
          color: #24292e;
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .plan-description {
          font-size: 13px;
          color: #586069;
          margin-top: 4px;
        }

        .plan-meta {
          font-size: 12px;
          color: #8b949e;
          margin-top: 4px;
          display: flex;
          gap: 16px;
          flex-wrap: wrap;
        }

        .badge {
          display: inline-block;
          padding: 2px 8px;
          border-radius: 12px;
          font-size: 11px;
          font-weight: 500;
        }

        .badge.active {
          background: #dafbe1;
          color: #1a7f37;
        }

        .badge.paused {
          background: #fff8c5;
          color: #9a6700;
        }

        .badge.disabled {
          background: #ffebe9;
          color: #cf222e;
        }

        .badge.trigger {
          background: #ddf4ff;
          color: #0969da;
        }

        .badge.mode {
          background: #f6f8fa;
          color: #57606a;
          border: 1px solid #d0d7de;
        }

        .plan-actions {
          display: flex;
          gap: 8px;
        }

        .plan-actions button {
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

        .detail-panel {
          margin-top: 16px;
          padding: 16px;
          background: #ffffff;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          display: none;
        }

        .detail-panel.visible {
          display: block;
        }

        .detail-panel h4 {
          margin: 0 0 12px 0;
          font-size: 16px;
          font-weight: 600;
          color: #24292e;
          display: flex;
          justify-content: space-between;
          align-items: center;
        }

        .json-viewer {
          background: #f6f8fa;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          padding: 12px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
          overflow-x: auto;
          white-space: pre-wrap;
          word-break: break-word;
          max-height: 400px;
          overflow-y: auto;
        }

        .button-group {
          display: flex;
          gap: 8px;
          margin-top: 16px;
        }

        .tabs {
          display: flex;
          gap: 4px;
          margin-bottom: 12px;
          border-bottom: 1px solid #d0d7de;
          padding-bottom: 8px;
        }

        .tab {
          padding: 6px 12px;
          background: transparent;
          border: none;
          color: #586069;
          cursor: pointer;
          font-size: 13px;
          border-radius: 6px 6px 0 0;
        }

        .tab:hover {
          color: #24292e;
          background: #f6f8fa;
        }

        .tab.active {
          color: #0969da;
          background: #ddf4ff;
        }

        .help-text {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .editor-actions {
          display: flex;
          gap: 8px;
          justify-content: flex-end;
          margin-top: 16px;
        }
      </style>

      <div class="container">
        <div class="header-row">
          <div>
            <h2>Plans</h2>
            <p class="subtitle">Manage automated file processing plans with triggers and outcomes</p>
          </div>
          <div style="display: flex; gap: 8px;">
            <button id="createBtn" class="secondary">+ New Plan</button>
            <button id="refreshBtn" class="outline">Refresh</button>
          </div>
        </div>

        <!-- Create/Edit Form (hidden by default) -->
        <div id="editorSection" class="section" style="display: none;">
          <h3 id="editorTitle">Create New Plan</h3>
          <div class="form-row">
            <div class="form-group">
              <label for="planName">Name *</label>
              <input type="text" id="planName" placeholder="my-plan" pattern="[a-zA-Z0-9_-]+" />
            </div>
            <div class="form-group">
              <label for="planMode">Mode</label>
              <select id="planMode">
                <option value="oneshot">Oneshot</option>
                <option value="continuous">Continuous</option>
              </select>
            </div>
            <div class="form-group">
              <label for="planStatus">Status</label>
              <select id="planStatus">
                <option value="active">Active</option>
                <option value="paused">Paused</option>
                <option value="disabled">Disabled</option>
              </select>
            </div>
            <div class="form-group">
              <label for="planTrigger">Trigger</label>
              <select id="planTrigger">
                <option value="">Manual</option>
                <option value="on_add">On Add</option>
                <option value="on_remove">On Remove</option>
                <option value="on_refresh">On Refresh</option>
              </select>
            </div>
          </div>
          <div class="form-row" style="margin-top: 12px;">
            <div class="form-group full-width">
              <label for="planDescription">Description</label>
              <input type="text" id="planDescription" placeholder="Description of the plan" />
            </div>
          </div>
          <div class="form-row" style="margin-top: 12px;">
            <div class="form-group full-width">
              <label for="planSources">Sources (JSON array)</label>
              <textarea id="planSources" placeholder='[{"type": "entries"}]'></textarea>
              <p class="help-text">Types: entries, filesystem, selection_set, query, project</p>
            </div>
          </div>
          <div class="form-row" style="margin-top: 12px;">
            <div class="form-group full-width">
              <label for="planConditions">Conditions (JSON object, optional)</label>
              <textarea id="planConditions" placeholder='{"all": [{"field": "kind", "operator": "equals", "value": "file"}]}'></textarea>
              <p class="help-text">Optional filter conditions for entries</p>
            </div>
          </div>
          <div class="form-row" style="margin-top: 12px;">
            <div class="form-group full-width">
              <label for="planOutcomes">Outcomes (JSON array) *</label>
              <textarea id="planOutcomes" placeholder='[{"action": "add_to_set", "set_name": "my-set"}]'></textarea>
              <p class="help-text">Actions: add_to_set, remove_from_set, invoke_tool, delete</p>
            </div>
          </div>
          <div class="form-row" style="margin-top: 12px;">
            <div class="form-group full-width">
              <label for="planPreferences">Preferences (JSON object, optional)</label>
              <textarea id="planPreferences" placeholder='{"large.file.size": 524288000}'></textarea>
              <p class="help-text">Optional preferences for outcome processing</p>
            </div>
          </div>
          <div class="editor-actions">
            <button id="cancelEditBtn" class="outline">Cancel</button>
            <button id="saveBtn">Save Plan</button>
          </div>
        </div>

        <div class="section">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
            <h3 style="margin: 0;">Available Plans</h3>
          </div>
          <div id="planList" class="plan-list">
            <div class="loading-overlay">
              <span class="spinner"></span> Loading plans...
            </div>
          </div>
        </div>

        <!-- Detail Panel -->
        <div id="detailPanel" class="detail-panel">
          <h4>
            <span id="detailTitle">Plan Details</span>
            <div style="display: flex; gap: 8px;">
              <button id="editPlanBtn" class="secondary" style="padding: 4px 10px; font-size: 12px;">Edit</button>
              <button id="deletePlanBtn" class="danger" style="padding: 4px 10px; font-size: 12px;">Delete</button>
              <button id="closeDetailBtn" class="outline" style="padding: 4px 10px; font-size: 12px;">Close</button>
            </div>
          </h4>
          <div class="tabs">
            <button class="tab active" data-tab="overview">Overview</button>
            <button class="tab" data-tab="sources">Sources</button>
            <button class="tab" data-tab="conditions">Conditions</button>
            <button class="tab" data-tab="outcomes">Outcomes</button>
            <button class="tab" data-tab="preferences">Preferences</button>
            <button class="tab" data-tab="raw">Raw JSON</button>
          </div>
          <div id="detailContent" class="json-viewer"></div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    this.shadowRoot.getElementById('createBtn').addEventListener('click', () => this.showCreateForm());
    this.shadowRoot.getElementById('refreshBtn').addEventListener('click', () => this.loadPlans());
    this.shadowRoot.getElementById('cancelEditBtn').addEventListener('click', () => this.hideEditor());
    this.shadowRoot.getElementById('saveBtn').addEventListener('click', () => this.savePlan());
    this.shadowRoot.getElementById('closeDetailBtn').addEventListener('click', () => this.hideDetailPanel());
    this.shadowRoot.getElementById('editPlanBtn').addEventListener('click', () => this.editSelectedPlan());
    this.shadowRoot.getElementById('deletePlanBtn').addEventListener('click', () => this.deleteSelectedPlan());

    // Tab navigation in detail panel
    this.shadowRoot.querySelectorAll('.tab').forEach(tab => {
      tab.addEventListener('click', (e) => this.switchDetailTab(e.target.dataset.tab));
    });

    // Allow Enter key in name field
    this.shadowRoot.getElementById('planName').addEventListener('keypress', (e) => {
      if (e.key === 'Enter') {
        this.savePlan();
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

    const headers = { 'Content-Type': 'application/json' };
    // Include session ID if we have one
    if (this.sessionId) {
      headers['Mcp-Session-Id'] = this.sessionId;
    }

    const response = await fetch(`${this.apiBase}/mcp`, {
      method: 'POST',
      headers,
      credentials: 'include',
      body: JSON.stringify(request)
    });

    // Capture session ID from response and persist it
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

    // Check for tool error
    if (data.result && data.result.isError) {
      const errorText = data.result.content?.[0]?.text || 'Unknown error';
      throw new Error(errorText);
    }

    return data.result;
  }

  async loadPlans() {
    const planList = this.shadowRoot.getElementById('planList');
    planList.innerHTML = `
      <div class="loading-overlay">
        <span class="spinner"></span> Loading plans...
      </div>
    `;

    try {
      const result = await this.callMCPTool('plan-list', {});
      this.plans = JSON.parse(result.content[0].text) || [];

      this.renderPlanList();

      this.dispatchEvent(new CustomEvent('plans-loaded', {
        detail: { plans: this.plans },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      planList.innerHTML = `
        <div class="empty-state">
          <div class="icon">Error loading plans: ${this.escapeHtml(error.message)}</div>
        </div>
      `;
      this.showStatus('error', `Failed to load plans: ${error.message}`);
    }
  }

  renderPlanList() {
    const planList = this.shadowRoot.getElementById('planList');

    if (this.plans.length === 0) {
      planList.innerHTML = `
        <div class="empty-state">
          <div class="icon">&#128203;</div>
          <p>No plans yet. Create your first plan above!</p>
        </div>
      `;
      return;
    }

    planList.innerHTML = this.plans.map(plan => {
      const isSelected = this.selectedPlan && this.selectedPlan.name === plan.name;
      const triggerLabel = this.getTriggerLabel(plan.trigger);

      return `
        <div class="plan-card ${isSelected ? 'selected' : ''}" data-name="${this.escapeHtml(plan.name)}">
          <div class="plan-info">
            <div class="plan-name">
              ${this.escapeHtml(plan.name)}
              <span class="badge ${plan.status}">${plan.status}</span>
              <span class="badge mode">${plan.mode}</span>
              ${triggerLabel ? `<span class="badge trigger">${triggerLabel}</span>` : ''}
            </div>
            ${plan.description ? `<div class="plan-description">${this.escapeHtml(plan.description)}</div>` : ''}
            <div class="plan-meta">
              <span>Created: ${this.formatDate(plan.created_at)}</span>
              ${plan.last_run_at ? `<span>Last run: ${this.formatDate(plan.last_run_at)}</span>` : ''}
            </div>
          </div>
        </div>
      `;
    }).join('');

    // Attach click listeners to plan cards
    planList.querySelectorAll('.plan-card').forEach(card => {
      card.addEventListener('click', (e) => this.selectPlan(e.currentTarget.dataset.name));
    });
  }

  getTriggerLabel(trigger) {
    const labels = {
      'on_add': 'On Add',
      'on_remove': 'On Remove',
      'on_refresh': 'On Refresh',
      'manual': 'Manual'
    };
    return labels[trigger] || (trigger ? trigger : '');
  }

  async selectPlan(name) {
    try {
      const result = await this.callMCPTool('plan-get', { name });
      this.selectedPlan = JSON.parse(result.content[0].text);

      this.renderPlanList();
      this.showDetailPanel();
      this.switchDetailTab('overview');
    } catch (error) {
      this.showStatus('error', `Failed to load plan: ${error.message}`);
    }
  }

  showDetailPanel() {
    const panel = this.shadowRoot.getElementById('detailPanel');
    panel.classList.add('visible');

    const title = this.shadowRoot.getElementById('detailTitle');
    title.textContent = this.selectedPlan ? this.selectedPlan.name : 'Plan Details';
  }

  hideDetailPanel() {
    const panel = this.shadowRoot.getElementById('detailPanel');
    panel.classList.remove('visible');
    this.selectedPlan = null;
    this.renderPlanList();
  }

  switchDetailTab(tabName) {
    // Update tab styling
    this.shadowRoot.querySelectorAll('.tab').forEach(tab => {
      tab.classList.toggle('active', tab.dataset.tab === tabName);
    });

    const content = this.shadowRoot.getElementById('detailContent');
    const plan = this.selectedPlan;

    if (!plan) {
      content.textContent = 'No plan selected';
      return;
    }

    switch (tabName) {
      case 'overview':
        content.innerHTML = this.renderOverview(plan);
        break;
      case 'sources':
        content.textContent = JSON.stringify(plan.sources || [], null, 2);
        break;
      case 'conditions':
        content.textContent = JSON.stringify(plan.conditions || null, null, 2);
        break;
      case 'outcomes':
        content.textContent = JSON.stringify(plan.outcomes || [], null, 2);
        break;
      case 'preferences':
        content.textContent = JSON.stringify(plan.preferences || {}, null, 2);
        break;
      case 'raw':
        content.textContent = JSON.stringify(plan, null, 2);
        break;
      default:
        content.textContent = 'Unknown tab';
    }
  }

  renderOverview(plan) {
    return `<table style="width: 100%; border-collapse: collapse;">
      <tr><td style="padding: 4px 8px; color: #586069;">Name:</td><td style="padding: 4px 8px;">${this.escapeHtml(plan.name)}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Description:</td><td style="padding: 4px 8px;">${this.escapeHtml(plan.description || '-')}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Mode:</td><td style="padding: 4px 8px;">${plan.mode}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Status:</td><td style="padding: 4px 8px;">${plan.status}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Trigger:</td><td style="padding: 4px 8px;">${this.getTriggerLabel(plan.trigger) || 'Manual'}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Sources:</td><td style="padding: 4px 8px;">${(plan.sources || []).length} defined</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Outcomes:</td><td style="padding: 4px 8px;">${(plan.outcomes || []).length} defined</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Created:</td><td style="padding: 4px 8px;">${this.formatDate(plan.created_at)}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Updated:</td><td style="padding: 4px 8px;">${this.formatDate(plan.updated_at)}</td></tr>
      <tr><td style="padding: 4px 8px; color: #586069;">Last Run:</td><td style="padding: 4px 8px;">${plan.last_run_at ? this.formatDate(plan.last_run_at) : 'Never'}</td></tr>
    </table>`;
  }

  showCreateForm() {
    this.createMode = true;
    this.editMode = false;

    // Reset form
    this.shadowRoot.getElementById('planName').value = '';
    this.shadowRoot.getElementById('planName').disabled = false;
    this.shadowRoot.getElementById('planDescription').value = '';
    this.shadowRoot.getElementById('planMode').value = 'oneshot';
    this.shadowRoot.getElementById('planStatus').value = 'active';
    this.shadowRoot.getElementById('planTrigger').value = '';
    this.shadowRoot.getElementById('planSources').value = '[{"type": "entries"}]';
    this.shadowRoot.getElementById('planConditions').value = '';
    this.shadowRoot.getElementById('planOutcomes').value = '';
    this.shadowRoot.getElementById('planPreferences').value = '';

    this.shadowRoot.getElementById('editorTitle').textContent = 'Create New Plan';
    this.shadowRoot.getElementById('editorSection').style.display = 'block';
    this.shadowRoot.getElementById('planName').focus();
  }

  editSelectedPlan() {
    if (!this.selectedPlan) return;

    this.createMode = false;
    this.editMode = true;

    const plan = this.selectedPlan;

    this.shadowRoot.getElementById('planName').value = plan.name;
    this.shadowRoot.getElementById('planName').disabled = true; // Can't change name
    this.shadowRoot.getElementById('planDescription').value = plan.description || '';
    this.shadowRoot.getElementById('planMode').value = plan.mode || 'oneshot';
    this.shadowRoot.getElementById('planStatus').value = plan.status || 'active';
    this.shadowRoot.getElementById('planTrigger').value = plan.trigger || '';
    this.shadowRoot.getElementById('planSources').value = JSON.stringify(plan.sources || [], null, 2);
    this.shadowRoot.getElementById('planConditions').value = plan.conditions ? JSON.stringify(plan.conditions, null, 2) : '';
    this.shadowRoot.getElementById('planOutcomes').value = JSON.stringify(plan.outcomes || [], null, 2);
    this.shadowRoot.getElementById('planPreferences').value = plan.preferences ? JSON.stringify(plan.preferences, null, 2) : '';

    this.shadowRoot.getElementById('editorTitle').textContent = `Edit Plan: ${plan.name}`;
    this.shadowRoot.getElementById('editorSection').style.display = 'block';
  }

  hideEditor() {
    this.shadowRoot.getElementById('editorSection').style.display = 'none';
    this.createMode = false;
    this.editMode = false;
  }

  async savePlan() {
    const name = this.shadowRoot.getElementById('planName').value.trim();
    const description = this.shadowRoot.getElementById('planDescription').value.trim();
    const mode = this.shadowRoot.getElementById('planMode').value;
    const status = this.shadowRoot.getElementById('planStatus').value;
    const trigger = this.shadowRoot.getElementById('planTrigger').value;
    const sourcesText = this.shadowRoot.getElementById('planSources').value.trim();
    const conditionsText = this.shadowRoot.getElementById('planConditions').value.trim();
    const outcomesText = this.shadowRoot.getElementById('planOutcomes').value.trim();
    const preferencesText = this.shadowRoot.getElementById('planPreferences').value.trim();

    // Validation
    if (!name) {
      this.showStatus('error', 'Plan name is required');
      return;
    }

    if (this.createMode && !/^[a-zA-Z0-9_-]+$/.test(name)) {
      this.showStatus('error', 'Plan name can only contain letters, numbers, underscores, and hyphens');
      return;
    }

    let sources, conditions, outcomes, preferences;

    try {
      sources = JSON.parse(sourcesText || '[]');
    } catch (e) {
      this.showStatus('error', 'Invalid Sources JSON: ' + e.message);
      return;
    }

    try {
      conditions = conditionsText ? JSON.parse(conditionsText) : null;
    } catch (e) {
      this.showStatus('error', 'Invalid Conditions JSON: ' + e.message);
      return;
    }

    try {
      outcomes = JSON.parse(outcomesText || '[]');
    } catch (e) {
      this.showStatus('error', 'Invalid Outcomes JSON: ' + e.message);
      return;
    }

    try {
      preferences = preferencesText ? JSON.parse(preferencesText) : null;
    } catch (e) {
      this.showStatus('error', 'Invalid Preferences JSON: ' + e.message);
      return;
    }

    if (sources.length === 0) {
      this.showStatus('error', 'At least one source is required');
      return;
    }

    if (outcomes.length === 0) {
      this.showStatus('error', 'At least one outcome is required');
      return;
    }

    const planData = {
      name,
      description: description || undefined,
      mode,
      status,
      trigger: trigger || undefined,
      sources,
      conditions: conditions || undefined,
      outcomes,
      preferences: preferences || undefined
    };

    const saveBtn = this.shadowRoot.getElementById('saveBtn');
    saveBtn.disabled = true;

    try {
      const toolName = this.createMode ? 'plan-create' : 'plan-update';
      await this.callMCPTool(toolName, { planJson: JSON.stringify(planData) });

      this.showStatus('success', `Plan "${name}" ${this.createMode ? 'created' : 'updated'} successfully!`);
      this.hideEditor();
      await this.loadPlans();

      const eventName = this.createMode ? 'plan-created' : 'plan-updated';
      this.dispatchEvent(new CustomEvent(eventName, {
        detail: { plan: planData },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to save plan: ${error.message}`);
    } finally {
      saveBtn.disabled = false;
    }
  }

  async deleteSelectedPlan() {
    if (!this.selectedPlan) return;

    const name = this.selectedPlan.name;
    if (!confirm(`Are you sure you want to delete plan "${name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await this.callMCPTool('plan-delete', { name });

      this.showStatus('success', `Plan "${name}" deleted`);
      this.hideDetailPanel();
      await this.loadPlans();

      this.dispatchEvent(new CustomEvent('plan-deleted', {
        detail: { planName: name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Failed to delete plan: ${error.message}`);
    }
  }

  escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  formatDate(dateValue) {
    if (!dateValue) return 'Unknown';

    let date;
    if (typeof dateValue === 'string') {
      date = new Date(dateValue);
    } else if (typeof dateValue === 'number') {
      date = dateValue > 1000000000000 ? new Date(dateValue) : new Date(dateValue * 1000);
    } else {
      return 'Unknown';
    }

    if (isNaN(date.getTime())) {
      return 'Unknown';
    }

    return date.toLocaleString();
  }

  showStatus(type, message, showSpinner = false) {
    const statusDiv = this.shadowRoot.getElementById('status');
    statusDiv.className = `status visible ${type}`;
    statusDiv.innerHTML = showSpinner
      ? `<span class="spinner"></span> ${this.escapeHtml(message)}`
      : this.escapeHtml(message);

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

customElements.define('mcp-plans', McpPlans);
