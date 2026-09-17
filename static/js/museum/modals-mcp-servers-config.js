'use strict';

Modals.MCPServersConfig = (() => {
    let rows = [];
    let editingId = null;

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function showStatus(message, isError) {
        const el = getEl('mcp-servers-config-status');
        if (!el) return;
        if (!message) {
            el.style.display = 'none';
            el.textContent = '';
            return;
        }
        el.textContent = message;
        el.style.display = 'block';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-success, #1a7f37)';
    }

    /** So the AI Tool Access tab's "Server" column and the tool list itself pick up any change. */
    function invalidateCaches() {
        if (typeof Modals !== 'undefined' && Modals.LLMToolsAccess && Modals.LLMToolsAccess.load) {
            void Modals.LLMToolsAccess.load();
        }
    }

    function bindTableButtons() {
        const tbody = getEl('mcp-servers-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.mcp-servers-config-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.mcp-servers-config-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteServer(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.mcp-servers-config-enabled-checkbox').forEach((cb) => {
            cb.addEventListener('change', () => toggleEnabled(parseInt(cb.dataset.id, 10), cb.checked));
        });
        tbody.querySelectorAll('.mcp-servers-config-test-btn').forEach((btn) => {
            btn.addEventListener('click', () => testServerRow(parseInt(btn.dataset.id, 10), btn));
        });
    }

    function renderTable() {
        const tbody = getEl('mcp-servers-config-tbody');
        const loading = getEl('mcp-servers-config-loading');
        const tableWrap = getEl('mcp-servers-config-table-wrap');
        if (!tbody) return;
        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';

        const sorted = rows.slice().sort((a, b) => {
            if (a.is_builtin !== b.is_builtin) return a.is_builtin ? -1 : 1;
            if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
            return a.id - b.id;
        });

        tbody.innerHTML = sorted.map((row) => {
            const nameCell = row.is_builtin
                ? `${escapeHtml(row.name)} <span style="color:var(--color-text-muted); font-size:var(--text-sm);">(built-in)</span>`
                : escapeHtml(row.name);
            const endpointCell = row.is_builtin
                ? '<span style="color:var(--color-text-muted);">managed by the app</span>'
                : `<code>${escapeHtml(row.endpoint_url)}</code>`;
            const editDeleteButtons = row.is_builtin ? '' : `
                        <button type="button" class="mcp-servers-config-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit server" title="Edit"><i class="fas fa-edit" aria-hidden="true"></i></button>
                        <button type="button" class="mcp-servers-config-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete server" title="Delete"><i class="fas fa-trash-alt" aria-hidden="true"></i></button>`;
            const actionsCell = `<span class="manage-contacts-actions-cell">
                        <button type="button" class="mcp-servers-config-test-btn modal-btn modal-btn-secondary" data-id="${row.id}" title="Test connection">Test</button>${editDeleteButtons}</span>`;
            return `
            <tr data-id="${row.id}">
                <td>${nameCell}</td>
                <td>${endpointCell}</td>
                <td style="text-align:center;">
                    <input type="checkbox" class="mcp-servers-config-enabled-checkbox" data-id="${row.id}" ${row.enabled ? 'checked' : ''} aria-label="Enabled">
                </td>
                <td>${actionsCell}</td>
            </tr>
        `;
        }).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('mcp-servers-config-loading');
        const tableWrap = getEl('mcp-servers-config-table-wrap');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        showStatus('', false);
        try {
            const res = await fetch('/api/mcp-servers', { credentials: 'same-origin' });
            if (!res.ok) {
                const err = await res.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${res.status}`);
            }
            const data = await res.json();
            rows = Array.isArray(data.servers) ? data.servers : [];
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load MCP servers: ${err.message}`, true);
        }
    }

    function showEditTestResult(msg, isError) {
        const el = getEl('mcp-servers-config-edit-test-result');
        if (!el) return;
        el.style.display = 'block';
        el.textContent = msg;
        el.style.background = isError ? 'rgba(248,113,113,0.12)' : 'rgba(26,127,55,0.12)';
        el.style.border = `1px solid ${isError ? 'var(--color-danger)' : 'var(--color-success, #1a7f37)'}`;
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-success, #1a7f37)';
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('mcp-servers-config-edit-modal');
        const title = getEl('mcp-servers-config-edit-title');
        const name = getEl('mcp-servers-config-edit-name');
        const endpoint = getEl('mcp-servers-config-edit-endpoint');
        const token = getEl('mcp-servers-config-edit-token');
        const tokenHint = getEl('mcp-servers-config-edit-token-hint');
        const enabled = getEl('mcp-servers-config-edit-enabled');
        const errEl = getEl('mcp-servers-config-edit-error');
        const testResult = getEl('mcp-servers-config-edit-test-result');
        if (!modal || !name || !endpoint || !token) return;
        if (title) title.textContent = 'Add MCP Server';
        name.value = '';
        name.readOnly = false;
        endpoint.value = '';
        token.value = '';
        if (tokenHint) tokenHint.style.display = 'none';
        if (enabled) enabled.checked = true;
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        if (testResult) testResult.style.display = 'none';
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row || row.is_builtin) return;
        editingId = id;
        const modal = getEl('mcp-servers-config-edit-modal');
        const title = getEl('mcp-servers-config-edit-title');
        const name = getEl('mcp-servers-config-edit-name');
        const endpoint = getEl('mcp-servers-config-edit-endpoint');
        const token = getEl('mcp-servers-config-edit-token');
        const tokenHint = getEl('mcp-servers-config-edit-token-hint');
        const enabled = getEl('mcp-servers-config-edit-enabled');
        const errEl = getEl('mcp-servers-config-edit-error');
        const testResult = getEl('mcp-servers-config-edit-test-result');
        if (!modal || !name || !endpoint || !token) return;
        if (title) title.textContent = 'Edit MCP Server';
        name.value = row.name || '';
        name.readOnly = true;
        endpoint.value = row.endpoint_url || '';
        token.value = '';
        if (tokenHint) tokenHint.style.display = row.auth_token_set ? 'block' : 'none';
        if (enabled) enabled.checked = !!row.enabled;
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        if (testResult) testResult.style.display = 'none';
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('mcp-servers-config-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    function currentEditInput() {
        const name = (getEl('mcp-servers-config-edit-name') || {}).value || '';
        const endpoint = (getEl('mcp-servers-config-edit-endpoint') || {}).value || '';
        const token = (getEl('mcp-servers-config-edit-token') || {}).value || '';
        const enabled = !!(getEl('mcp-servers-config-edit-enabled') || {}).checked;
        const body = {
            name: name.trim(),
            endpoint_url: endpoint.trim(),
            enabled,
            sort_order: editingId
                ? (rows.find((r) => r.id === editingId) || {}).sort_order || 0
                : rows.length,
        };
        // Only include auth_token when the field has something typed — leaving it blank on an
        // edit means "keep the existing token" (see MCPServerInput's doc comment); omitting the
        // key entirely from the JSON body is what signals that to the backend.
        if (token.trim() !== '') {
            body.auth_token = token;
        }
        return body;
    }

    async function testEditModal() {
        const btn = getEl('mcp-servers-config-edit-test');
        const name = (getEl('mcp-servers-config-edit-name') || {}).value || '';
        const endpoint = (getEl('mcp-servers-config-edit-endpoint') || {}).value || '';
        const token = (getEl('mcp-servers-config-edit-token') || {}).value || '';
        if (!endpoint.trim()) {
            showEditTestResult('Enter an endpoint URL first.', true);
            return;
        }
        if (btn) { btn.disabled = true; btn.textContent = 'Testing…'; }
        try {
            const body = editingId && token.trim() === ''
                ? { id: editingId }
                : { endpoint_url: endpoint.trim(), auth_token: token };
            const res = await fetch('/api/mcp-servers/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify(body),
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || data.detail || `HTTP ${res.status}`);
            if (data.ok) {
                showEditTestResult(`Connected — ${data.tool_count} tool(s) discovered.`, false);
            } else {
                showEditTestResult(data.error || 'Connection failed.', true);
            }
        } catch (err) {
            showEditTestResult(err.message, true);
        } finally {
            if (btn) { btn.disabled = false; btn.textContent = 'Test'; }
            void name; // name isn't sent to /test (only endpoint/token matter) — kept for future use
        }
    }

    async function testServerRow(id, btn) {
        const original = btn ? btn.textContent : '';
        if (btn) { btn.disabled = true; btn.textContent = 'Testing…'; }
        try {
            const res = await fetch('/api/mcp-servers/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ id }),
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || data.detail || `HTTP ${res.status}`);
            if (data.ok) {
                showStatus(`Connected — ${data.tool_count} tool(s) discovered.`, false);
            } else {
                showStatus(data.error || 'Connection failed.', true);
            }
        } catch (err) {
            showStatus(err.message, true);
        } finally {
            if (btn) { btn.disabled = false; btn.textContent = original || 'Test'; }
        }
    }

    async function saveEditModal() {
        const errEl = getEl('mcp-servers-config-edit-error');
        const body = currentEditInput();
        if (!body.name || !body.endpoint_url) {
            if (errEl) {
                errEl.textContent = 'Name and endpoint URL are required.';
                errEl.style.display = 'block';
            }
            return;
        }
        try {
            const url = editingId ? `/api/mcp-servers/${editingId}` : '/api/mcp-servers';
            const method = editingId ? 'PATCH' : 'POST';
            const response = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify(body),
            });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            closeEditModal();
            showStatus(editingId ? 'Server updated.' : 'Server added.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            if (errEl) {
                errEl.textContent = err.message;
                errEl.style.display = 'block';
            }
        }
    }

    async function deleteServer(id) {
        const row = rows.find((r) => r.id === id);
        if (!row || row.is_builtin) return;
        if (!window.confirm(`Delete MCP server "${row.name}"? Its tools will no longer be available to the AI.`)) return;
        try {
            const response = await fetch(`/api/mcp-servers/${id}`, { method: 'DELETE', credentials: 'same-origin' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('Server deleted.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    async function patchEnabled(row, enabled) {
        const response = await fetch(`/api/mcp-servers/${row.id}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                name: row.name,
                endpoint_url: row.endpoint_url,
                enabled,
                sort_order: row.sort_order,
            }),
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || err.detail || `HTTP ${response.status}`);
        }
    }

    async function toggleEnabled(id, enabled) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        if (row.is_builtin && !enabled) {
            const ok = window.confirm(
                'Disabling the built-in Digital Museum server removes every built-in archive tool ' +
                '(messages, emails, photos, reference documents, etc.) from what the AI can use, ' +
                'until you re-enable it. Continue?'
            );
            if (!ok) {
                // Revert the checkbox the user just clicked.
                const cb = document.querySelector(`.mcp-servers-config-enabled-checkbox[data-id="${id}"]`);
                if (cb) cb.checked = true;
                return;
            }
        }
        try {
            await patchEnabled(row, enabled);
            showStatus(enabled ? 'Server enabled.' : 'Server disabled.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            showStatus(err.message, true);
            await load();
        }
    }

    function init() {
        const addBtn = getEl('mcp-servers-config-add-btn');
        if (addBtn) addBtn.addEventListener('click', openCreateModal);

        const editSave = getEl('mcp-servers-config-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editTest = getEl('mcp-servers-config-edit-test');
        if (editTest) editTest.addEventListener('click', () => { void testEditModal(); });
        const editCancel = getEl('mcp-servers-config-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('mcp-servers-config-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);
    }

    return { init, load, openCreateModal };
})();

Modals.MCPServersConfig.init();
