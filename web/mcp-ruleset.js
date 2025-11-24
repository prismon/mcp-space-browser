/**
 * MCP Ruleset Web Component
 *
 * A standalone web component for creating, editing, and deleting rules
 * via the mcp-space-browser MCP JSON-RPC protocol.
 *
 * Usage:
 *   <mcp-ruleset api-base="http://localhost:3000"></mcp-ruleset>
 *
 * Attributes:
 *   - api-base: Base URL for the API (default: window.location.origin)
 */
class McpRuleset extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.apiBase = this.getAttribute('api-base') || window.location.origin;
    this.requestId = 0;
    this.currentRule = null;
    this.editingRule = null;
  }

  connectedCallback() {
    this.render();
    this.attachEventListeners();
    this.loadRules();
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
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .form-group {
          margin-bottom: 12px;
        }

        .form-row {
          display: grid;
          grid-template-columns: 1fr 1fr;
          gap: 12px;
        }

        label {
          display: block;
          margin-bottom: 4px;
          font-size: 13px;
          font-weight: 500;
          color: #24292e;
        }

        input[type="text"], input[type="number"], textarea, select {
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
          min-height: 80px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
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

        button.small {
          padding: 6px 12px;
          font-size: 12px;
          margin: 0;
        }

        .checkbox-group {
          display: flex;
          align-items: center;
          gap: 8px;
        }

        .checkbox-group input[type="checkbox"] {
          width: auto;
        }

        .rules-list {
          margin-top: 16px;
        }

        .rule-item {
          display: flex;
          align-items: flex-start;
          padding: 16px;
          border: 1px solid #e1e4e8;
          border-radius: 6px;
          margin-bottom: 8px;
          background: white;
          transition: background 0.1s;
        }

        .rule-item:hover {
          background: #f6f8fa;
        }

        .rule-item.active {
          background: #ddf4ff;
          border-color: #54aeff;
        }

        .rule-item.disabled {
          opacity: 0.6;
        }

        .rule-info {
          flex: 1;
        }

        .rule-header {
          display: flex;
          align-items: center;
          gap: 8px;
          margin-bottom: 4px;
        }

        .rule-name {
          font-weight: 600;
          color: #24292e;
        }

        .rule-badge {
          padding: 2px 8px;
          border-radius: 12px;
          font-size: 11px;
          font-weight: 500;
        }

        .rule-badge.enabled {
          background: #dafbe1;
          color: #1a7f37;
        }

        .rule-badge.disabled {
          background: #ffebe9;
          color: #cf222e;
        }

        .rule-badge.priority {
          background: #ddf4ff;
          color: #0969da;
        }

        .rule-meta {
          font-size: 12px;
          color: #586069;
          margin-top: 4px;
        }

        .rule-description {
          font-size: 13px;
          color: #57606a;
          margin-top: 4px;
        }

        .rule-details {
          font-size: 12px;
          color: #57606a;
          margin-top: 8px;
          padding-top: 8px;
          border-top: 1px solid #e1e4e8;
        }

        .rule-details code {
          background: #f6f8fa;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 11px;
        }

        .rule-actions {
          display: flex;
          flex-direction: column;
          gap: 6px;
        }

        .details-panel {
          margin-top: 16px;
          padding: 16px;
          background: #f6f8fa;
          border-radius: 6px;
          display: none;
        }

        .details-panel.visible {
          display: block;
        }

        .details-panel h3 {
          margin: 0 0 12px 0;
          font-size: 16px;
        }

        .json-view {
          background: #24292e;
          color: #e6edf3;
          padding: 12px;
          border-radius: 6px;
          font-family: 'SF Mono', Monaco, Consolas, monospace;
          font-size: 12px;
          overflow-x: auto;
          white-space: pre-wrap;
          max-height: 300px;
          overflow-y: auto;
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

        .collapsible {
          cursor: pointer;
        }

        .collapsible::before {
          content: '\\25B6';
          display: inline-block;
          margin-right: 6px;
          transition: transform 0.2s;
          font-size: 10px;
        }

        .collapsible.expanded::before {
          transform: rotate(90deg);
        }

        .collapse-content {
          display: none;
          margin-top: 8px;
        }

        .collapse-content.visible {
          display: block;
        }

        .tabs {
          display: flex;
          gap: 4px;
          margin-bottom: 12px;
          border-bottom: 1px solid #e1e4e8;
        }

        .tab {
          padding: 8px 16px;
          cursor: pointer;
          border: none;
          background: none;
          color: #57606a;
          font-weight: 500;
          border-bottom: 2px solid transparent;
          margin-bottom: -1px;
        }

        .tab:hover {
          color: #24292e;
        }

        .tab.active {
          color: #0969da;
          border-bottom-color: #0969da;
        }

        .tab-content {
          display: none;
        }

        .tab-content.active {
          display: block;
        }

        .help-text {
          font-size: 11px;
          color: #6e7781;
          margin-top: 4px;
        }

        .condition-builder, .outcome-builder {
          background: white;
          border: 1px solid #d0d7de;
          border-radius: 6px;
          padding: 12px;
          margin-top: 8px;
        }
      </style>

      <div class="container">
        <h2>Rule Management</h2>
        <p class="subtitle">Create and manage rules for automatic file classification and processing</p>

        <div class="tabs">
          <button class="tab active" data-tab="create">Create Rule</button>
          <button class="tab" data-tab="list">Rules List</button>
        </div>

        <!-- Create/Edit Rule Tab -->
        <div id="createTab" class="tab-content active">
          <div class="form-section">
            <h3 id="formTitle">Create New Rule</h3>

            <div class="form-row">
              <div class="form-group">
                <label for="ruleName">Name:</label>
                <input type="text" id="ruleName" placeholder="my-image-classifier" required />
              </div>
              <div class="form-group">
                <label for="rulePriority">Priority:</label>
                <input type="number" id="rulePriority" value="0" min="0" />
                <div class="help-text">Higher priority rules execute first</div>
              </div>
            </div>

            <div class="form-group">
              <label for="ruleDescription">Description:</label>
              <input type="text" id="ruleDescription" placeholder="Classify image files and add to selection set" />
            </div>

            <div class="form-group checkbox-group">
              <input type="checkbox" id="ruleEnabled" checked />
              <label for="ruleEnabled">Enabled</label>
            </div>

            <div class="form-group">
              <label>Condition (when to match):</label>
              <div class="condition-builder">
                <div class="form-row">
                  <div class="form-group">
                    <label for="conditionType">Condition Type:</label>
                    <select id="conditionType">
                      <option value="media_type">Media Type</option>
                      <option value="size">File Size</option>
                      <option value="path">Path Pattern</option>
                      <option value="time">Time Range</option>
                      <option value="all">All (AND)</option>
                      <option value="any">Any (OR)</option>
                      <option value="custom">Custom JSON</option>
                    </select>
                  </div>
                  <div class="form-group" id="mediaTypeGroup">
                    <label for="mediaType">Media Type:</label>
                    <select id="mediaType">
                      <option value="image">Image</option>
                      <option value="video">Video</option>
                      <option value="audio">Audio</option>
                      <option value="document">Document</option>
                    </select>
                  </div>
                </div>
                <div id="sizeGroup" style="display:none;">
                  <div class="form-row">
                    <div class="form-group">
                      <label for="minSize">Min Size (bytes):</label>
                      <input type="number" id="minSize" min="0" />
                    </div>
                    <div class="form-group">
                      <label for="maxSize">Max Size (bytes):</label>
                      <input type="number" id="maxSize" min="0" />
                    </div>
                  </div>
                </div>
                <div id="pathGroup" style="display:none;">
                  <div class="form-group">
                    <label for="pathPattern">Path Pattern (regex):</label>
                    <input type="text" id="pathPattern" placeholder=".*\\.jpg$" />
                  </div>
                </div>
                <div id="customConditionGroup" style="display:none;">
                  <div class="form-group">
                    <label for="customCondition">Custom Condition JSON:</label>
                    <textarea id="customCondition" placeholder='{"type": "all", "conditions": [...]}'></textarea>
                  </div>
                </div>
              </div>
            </div>

            <div class="form-group">
              <label>Outcome (what to do when matched):</label>
              <div class="outcome-builder">
                <div class="form-row">
                  <div class="form-group">
                    <label for="outcomeType">Outcome Type:</label>
                    <select id="outcomeType">
                      <option value="selection_set">Add to Selection Set</option>
                      <option value="classifier">Run Classifier</option>
                      <option value="custom">Custom JSON</option>
                    </select>
                  </div>
                  <div class="form-group">
                    <label for="selectionSetName">Selection Set Name:</label>
                    <input type="text" id="selectionSetName" placeholder="classified-images" required />
                    <div class="help-text">Required for all outcomes</div>
                  </div>
                </div>
                <div id="classifierGroup" style="display:none;">
                  <div class="form-row">
                    <div class="form-group">
                      <label for="classifierOperation">Operation:</label>
                      <select id="classifierOperation">
                        <option value="generate_thumbnail">Generate Thumbnail</option>
                        <option value="extract_metadata">Extract Metadata</option>
                      </select>
                    </div>
                    <div class="form-group">
                      <label for="thumbnailSize">Max Size (px):</label>
                      <input type="number" id="thumbnailSize" value="256" min="32" max="1024" />
                    </div>
                  </div>
                </div>
                <div id="customOutcomeGroup" style="display:none;">
                  <div class="form-group">
                    <label for="customOutcome">Custom Outcome JSON:</label>
                    <textarea id="customOutcome" placeholder='{"type": "selection_set", "selectionSetName": "...", "operation": "add"}'></textarea>
                  </div>
                </div>
              </div>
            </div>

            <div style="margin-top: 16px;">
              <button id="createBtn">Create Rule</button>
              <button id="updateBtn" class="secondary" style="display:none;">Update Rule</button>
              <button id="cancelEditBtn" class="secondary" style="display:none;">Cancel</button>
            </div>
          </div>
        </div>

        <!-- Rules List Tab -->
        <div id="listTab" class="tab-content">
          <div class="form-section">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <h3>Rules</h3>
              <button id="refreshBtn" class="small secondary">Refresh</button>
            </div>
            <div id="rulesList"></div>
          </div>

          <div id="detailsPanel" class="details-panel">
            <h3>Rule Details: <span id="detailsRuleName"></span></h3>
            <div class="json-view" id="ruleJson"></div>
            <div style="margin-top: 12px;">
              <button id="executeBtn" class="secondary small">Execute Rule</button>
            </div>
          </div>
        </div>

        <div id="status" class="status"></div>
      </div>
    `;
  }

  attachEventListeners() {
    // Tab switching
    this.shadowRoot.querySelectorAll('.tab').forEach(tab => {
      tab.addEventListener('click', () => this.switchTab(tab.dataset.tab));
    });

    // Form buttons
    this.shadowRoot.getElementById('createBtn').addEventListener('click', () => this.createRule());
    this.shadowRoot.getElementById('updateBtn').addEventListener('click', () => this.updateRule());
    this.shadowRoot.getElementById('cancelEditBtn').addEventListener('click', () => this.cancelEdit());
    this.shadowRoot.getElementById('refreshBtn').addEventListener('click', () => this.loadRules());
    this.shadowRoot.getElementById('executeBtn').addEventListener('click', () => this.executeRule());

    // Condition type changes
    this.shadowRoot.getElementById('conditionType').addEventListener('change', (e) => this.updateConditionUI(e.target.value));

    // Outcome type changes
    this.shadowRoot.getElementById('outcomeType').addEventListener('change', (e) => this.updateOutcomeUI(e.target.value));
  }

  switchTab(tabId) {
    this.shadowRoot.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    this.shadowRoot.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

    this.shadowRoot.querySelector(`.tab[data-tab="${tabId}"]`).classList.add('active');
    this.shadowRoot.getElementById(`${tabId}Tab`).classList.add('active');

    if (tabId === 'list') {
      this.loadRules();
    }
  }

  updateConditionUI(type) {
    const mediaTypeGroup = this.shadowRoot.getElementById('mediaTypeGroup');
    const sizeGroup = this.shadowRoot.getElementById('sizeGroup');
    const pathGroup = this.shadowRoot.getElementById('pathGroup');
    const customConditionGroup = this.shadowRoot.getElementById('customConditionGroup');

    mediaTypeGroup.style.display = 'none';
    sizeGroup.style.display = 'none';
    pathGroup.style.display = 'none';
    customConditionGroup.style.display = 'none';

    switch (type) {
      case 'media_type':
        mediaTypeGroup.style.display = 'block';
        break;
      case 'size':
        sizeGroup.style.display = 'block';
        break;
      case 'path':
        pathGroup.style.display = 'block';
        break;
      case 'custom':
      case 'all':
      case 'any':
        customConditionGroup.style.display = 'block';
        break;
    }
  }

  updateOutcomeUI(type) {
    const classifierGroup = this.shadowRoot.getElementById('classifierGroup');
    const customOutcomeGroup = this.shadowRoot.getElementById('customOutcomeGroup');

    classifierGroup.style.display = 'none';
    customOutcomeGroup.style.display = 'none';

    switch (type) {
      case 'classifier':
        classifierGroup.style.display = 'block';
        break;
      case 'custom':
        customOutcomeGroup.style.display = 'block';
        break;
    }
  }

  buildCondition() {
    const type = this.shadowRoot.getElementById('conditionType').value;

    switch (type) {
      case 'media_type':
        return {
          type: 'media_type',
          mediaType: this.shadowRoot.getElementById('mediaType').value
        };

      case 'size':
        const condition = { type: 'size' };
        const minSize = this.shadowRoot.getElementById('minSize').value;
        const maxSize = this.shadowRoot.getElementById('maxSize').value;
        if (minSize) condition.minSize = parseInt(minSize);
        if (maxSize) condition.maxSize = parseInt(maxSize);
        return condition;

      case 'path':
        return {
          type: 'path',
          pathPattern: this.shadowRoot.getElementById('pathPattern').value
        };

      case 'custom':
      case 'all':
      case 'any':
        try {
          return JSON.parse(this.shadowRoot.getElementById('customCondition').value || '{}');
        } catch (e) {
          throw new Error(`Invalid condition JSON: ${e.message}`);
        }

      default:
        return { type: type };
    }
  }

  buildOutcome() {
    const type = this.shadowRoot.getElementById('outcomeType').value;
    const selectionSetName = this.shadowRoot.getElementById('selectionSetName').value.trim();

    if (!selectionSetName) {
      throw new Error('Selection Set Name is required');
    }

    switch (type) {
      case 'selection_set':
        return {
          type: 'selection_set',
          selectionSetName: selectionSetName,
          operation: 'add'
        };

      case 'classifier':
        const size = parseInt(this.shadowRoot.getElementById('thumbnailSize').value) || 256;
        return {
          type: 'classifier',
          selectionSetName: selectionSetName,
          classifierOperation: this.shadowRoot.getElementById('classifierOperation').value,
          maxWidth: size,
          maxHeight: size
        };

      case 'custom':
        try {
          const customOutcome = JSON.parse(this.shadowRoot.getElementById('customOutcome').value || '{}');
          // Ensure selectionSetName is set
          customOutcome.selectionSetName = selectionSetName;
          return customOutcome;
        } catch (e) {
          throw new Error(`Invalid outcome JSON: ${e.message}`);
        }

      default:
        return { type: type, selectionSetName: selectionSetName };
    }
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

  async loadRules() {
    this.showStatus('info', 'Loading rules...', true);

    try {
      const result = await this.callMCPTool('rule-list', {});
      const data = JSON.parse(result.content[0].text);
      this.displayRules(data.rules || []);
      this.shadowRoot.getElementById('status').style.display = 'none';
    } catch (error) {
      this.showStatus('error', `Failed to load rules: ${error.message}`);
    }
  }

  async createRule() {
    const name = this.shadowRoot.getElementById('ruleName').value.trim();
    const description = this.shadowRoot.getElementById('ruleDescription').value.trim();
    const priority = parseInt(this.shadowRoot.getElementById('rulePriority').value) || 0;
    const enabled = this.shadowRoot.getElementById('ruleEnabled').checked;

    if (!name) {
      this.showStatus('error', 'Please enter a rule name');
      return;
    }

    try {
      const condition = this.buildCondition();
      const outcome = this.buildOutcome();

      this.showStatus('info', 'Creating rule...', true);

      const ruleJson = JSON.stringify({
        name,
        description: description || null,
        enabled,
        priority,
        condition,
        outcome
      });

      await this.callMCPTool('rule-create', { ruleJson });

      this.showStatus('success', `Rule "${name}" created successfully`);
      this.clearForm();
      this.loadRules();

      this.dispatchEvent(new CustomEvent('rule-created', {
        detail: { name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async updateRule() {
    if (!this.editingRule) return;

    const name = this.editingRule;
    const description = this.shadowRoot.getElementById('ruleDescription').value.trim();
    const priority = parseInt(this.shadowRoot.getElementById('rulePriority').value) || 0;
    const enabled = this.shadowRoot.getElementById('ruleEnabled').checked;

    try {
      const condition = this.buildCondition();
      const outcome = this.buildOutcome();

      this.showStatus('info', 'Updating rule...', true);

      const ruleJson = JSON.stringify({
        name,
        description: description || null,
        enabled,
        priority,
        condition,
        outcome
      });

      await this.callMCPTool('rule-update', { ruleJson });

      this.showStatus('success', `Rule "${name}" updated successfully`);
      this.cancelEdit();
      this.loadRules();

      this.dispatchEvent(new CustomEvent('rule-updated', {
        detail: { name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async deleteRule(name) {
    if (!confirm(`Are you sure you want to delete the rule "${name}"?`)) {
      return;
    }

    this.showStatus('info', 'Deleting rule...', true);

    try {
      await this.callMCPTool('rule-delete', { name });

      this.showStatus('success', `Rule "${name}" deleted successfully`);

      // Hide details panel if this was the current rule
      if (this.currentRule === name) {
        this.shadowRoot.getElementById('detailsPanel').classList.remove('visible');
        this.currentRule = null;
      }

      this.loadRules();

      this.dispatchEvent(new CustomEvent('rule-deleted', {
        detail: { name },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Error: ${error.message}`);
    }
  }

  async viewRuleDetails(name) {
    this.showStatus('info', 'Loading rule details...', true);

    try {
      const result = await this.callMCPTool('rule-get', { name });
      const rule = JSON.parse(result.content[0].text);

      this.currentRule = name;
      this.shadowRoot.getElementById('detailsRuleName').textContent = name;
      this.shadowRoot.getElementById('ruleJson').textContent = JSON.stringify(rule, null, 2);
      this.shadowRoot.getElementById('detailsPanel').classList.add('visible');
      this.shadowRoot.getElementById('status').style.display = 'none';

    } catch (error) {
      this.showStatus('error', `Failed to load rule details: ${error.message}`);
    }
  }

  async editRule(name) {
    this.showStatus('info', 'Loading rule for editing...', true);

    try {
      const result = await this.callMCPTool('rule-get', { name });
      const rule = JSON.parse(result.content[0].text);

      // Switch to create tab
      this.switchTab('create');

      // Fill in the form
      this.shadowRoot.getElementById('ruleName').value = rule.name;
      this.shadowRoot.getElementById('ruleName').disabled = true;
      this.shadowRoot.getElementById('ruleDescription').value = rule.description || '';
      this.shadowRoot.getElementById('rulePriority').value = rule.priority || 0;
      this.shadowRoot.getElementById('ruleEnabled').checked = rule.enabled;

      // Handle condition
      if (rule.condition) {
        const condType = rule.condition.type || 'custom';
        this.shadowRoot.getElementById('conditionType').value = condType;
        this.updateConditionUI(condType);

        if (condType === 'media_type' && rule.condition.mediaType) {
          this.shadowRoot.getElementById('mediaType').value = rule.condition.mediaType;
        } else if (condType === 'size') {
          if (rule.condition.minSize) this.shadowRoot.getElementById('minSize').value = rule.condition.minSize;
          if (rule.condition.maxSize) this.shadowRoot.getElementById('maxSize').value = rule.condition.maxSize;
        } else if (condType === 'path' && rule.condition.pathPattern) {
          this.shadowRoot.getElementById('pathPattern').value = rule.condition.pathPattern;
        } else {
          this.shadowRoot.getElementById('customCondition').value = JSON.stringify(rule.condition, null, 2);
        }
      }

      // Handle outcome
      if (rule.outcome) {
        const outcomeType = rule.outcome.type || 'custom';
        this.shadowRoot.getElementById('outcomeType').value = outcomeType;
        this.updateOutcomeUI(outcomeType);
        this.shadowRoot.getElementById('selectionSetName').value = rule.outcome.selectionSetName || '';

        if (outcomeType === 'classifier') {
          if (rule.outcome.classifierOperation) {
            this.shadowRoot.getElementById('classifierOperation').value = rule.outcome.classifierOperation;
          }
          if (rule.outcome.maxWidth) {
            this.shadowRoot.getElementById('thumbnailSize').value = rule.outcome.maxWidth;
          }
        } else if (outcomeType === 'custom') {
          this.shadowRoot.getElementById('customOutcome').value = JSON.stringify(rule.outcome, null, 2);
        }
      }

      // Update UI for edit mode
      this.editingRule = name;
      this.shadowRoot.getElementById('formTitle').textContent = `Edit Rule: ${name}`;
      this.shadowRoot.getElementById('createBtn').style.display = 'none';
      this.shadowRoot.getElementById('updateBtn').style.display = 'inline-block';
      this.shadowRoot.getElementById('cancelEditBtn').style.display = 'inline-block';

      this.shadowRoot.getElementById('status').style.display = 'none';

    } catch (error) {
      this.showStatus('error', `Failed to load rule for editing: ${error.message}`);
    }
  }

  cancelEdit() {
    this.editingRule = null;
    this.clearForm();
    this.shadowRoot.getElementById('formTitle').textContent = 'Create New Rule';
    this.shadowRoot.getElementById('ruleName').disabled = false;
    this.shadowRoot.getElementById('createBtn').style.display = 'inline-block';
    this.shadowRoot.getElementById('updateBtn').style.display = 'none';
    this.shadowRoot.getElementById('cancelEditBtn').style.display = 'none';
  }

  clearForm() {
    this.shadowRoot.getElementById('ruleName').value = '';
    this.shadowRoot.getElementById('ruleDescription').value = '';
    this.shadowRoot.getElementById('rulePriority').value = '0';
    this.shadowRoot.getElementById('ruleEnabled').checked = true;
    this.shadowRoot.getElementById('conditionType').value = 'media_type';
    this.shadowRoot.getElementById('mediaType').value = 'image';
    this.shadowRoot.getElementById('minSize').value = '';
    this.shadowRoot.getElementById('maxSize').value = '';
    this.shadowRoot.getElementById('pathPattern').value = '';
    this.shadowRoot.getElementById('customCondition').value = '';
    this.shadowRoot.getElementById('outcomeType').value = 'selection_set';
    this.shadowRoot.getElementById('selectionSetName').value = '';
    this.shadowRoot.getElementById('classifierOperation').value = 'generate_thumbnail';
    this.shadowRoot.getElementById('thumbnailSize').value = '256';
    this.shadowRoot.getElementById('customOutcome').value = '';

    this.updateConditionUI('media_type');
    this.updateOutcomeUI('selection_set');
  }

  async executeRule() {
    if (!this.currentRule) return;

    this.showStatus('info', `Executing rule "${this.currentRule}"...`, true);

    try {
      const result = await this.callMCPTool('rule-execute', { name: this.currentRule });
      const data = JSON.parse(result.content[0].text);

      this.showStatus('success', `Rule executed: ${data.entries_matched} entries matched, ${data.entries_processed} processed in ${data.duration_ms}ms`);

      this.dispatchEvent(new CustomEvent('rule-executed', {
        detail: { name: this.currentRule, ...data },
        bubbles: true,
        composed: true
      }));

    } catch (error) {
      this.showStatus('error', `Execution failed: ${error.message}`);
    }
  }

  displayRules(rules) {
    const listDiv = this.shadowRoot.getElementById('rulesList');

    if (!rules || rules.length === 0) {
      listDiv.innerHTML = '<p style="text-align: center; color: #586069;">No rules found. Create one to get started!</p>';
      return;
    }

    listDiv.innerHTML = rules.map(rule => `
      <div class="rule-item ${rule.enabled ? '' : 'disabled'} ${this.currentRule === rule.name ? 'active' : ''}" data-name="${rule.name}">
        <div class="rule-info">
          <div class="rule-header">
            <span class="rule-name">${rule.name}</span>
            <span class="rule-badge ${rule.enabled ? 'enabled' : 'disabled'}">${rule.enabled ? 'Enabled' : 'Disabled'}</span>
            <span class="rule-badge priority">Priority: ${rule.priority || 0}</span>
          </div>
          ${rule.description ? `<div class="rule-description">${rule.description}</div>` : ''}
          <div class="rule-meta">
            Created: ${new Date(rule.created_at * 1000).toLocaleDateString()} |
            Updated: ${new Date(rule.updated_at * 1000).toLocaleDateString()}
          </div>
        </div>
        <div class="rule-actions">
          <button class="small secondary view-btn" data-name="${rule.name}">View</button>
          <button class="small secondary edit-btn" data-name="${rule.name}">Edit</button>
          <button class="small danger delete-btn" data-name="${rule.name}">Delete</button>
        </div>
      </div>
    `).join('');

    // Attach event listeners
    listDiv.querySelectorAll('.view-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.viewRuleDetails(btn.dataset.name);
      });
    });

    listDiv.querySelectorAll('.edit-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.editRule(btn.dataset.name);
      });
    });

    listDiv.querySelectorAll('.delete-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteRule(btn.dataset.name);
      });
    });
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

customElements.define('mcp-ruleset', McpRuleset);
