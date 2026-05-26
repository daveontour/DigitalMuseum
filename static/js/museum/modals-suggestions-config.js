'use strict';

Modals.SuggestionsConfig = (() => {
    let rows = [];
    let editingId = null;
    let pendingImportFile = null;
    let pendingImportPreview = null;

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function previewText(text, maxLen) {
        const s = String(text || '').trim();
        if (s.length <= maxLen) return s;
        return s.slice(0, maxLen) + '…';
    }

    function showStatus(message, isError) {
        const el = getEl('suggestions-config-status');
        if (!el) return;
        el.textContent = message || '';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-text-muted)';
    }

    async function refreshRuntimeSuggestions() {
        if (typeof Modals !== 'undefined' && Modals.Suggestions && Modals.Suggestions.invalidateCache) {
            Modals.Suggestions.invalidateCache();
        }
    }

    function bindTableButtons() {
        const tbody = getEl('suggestions-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.suggestions-config-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.suggestions-config-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteSuggestion(parseInt(btn.dataset.id, 10)));
        });
    }

    function renderTable() {
        const tbody = getEl('suggestions-config-tbody');
        const loading = getEl('suggestions-config-loading');
        const tableWrap = getEl('suggestions-config-table-wrap');
        const emptyMsg = getEl('suggestions-config-empty');
        if (!tbody) return;

        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';

        if (!rows || rows.length === 0) {
            tbody.innerHTML = '';
            if (emptyMsg) emptyMsg.style.display = 'block';
            return;
        }
        if (emptyMsg) emptyMsg.style.display = 'none';

        tbody.innerHTML = rows.map((row) => `
            <tr data-id="${row.id}">
                <td>${escapeHtml(row.category || '')}</td>
                <td>${escapeHtml(row.title || '')}</td>
                <td><span style="color:var(--color-text-muted);">${escapeHtml(previewText(row.prompt, 80))}</span></td>
                <td>
                    <span class="manage-contacts-actions-cell">
                        <button type="button" class="suggestions-config-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit suggestion" title="Edit">
                            <i class="fas fa-edit" aria-hidden="true"></i>
                        </button>
                        <button type="button" class="suggestions-config-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete suggestion" title="Delete">
                            <i class="fas fa-trash-alt" aria-hidden="true"></i>
                        </button>
                    </span>
                </td>
            </tr>
        `).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('suggestions-config-loading');
        const tableWrap = getEl('suggestions-config-table-wrap');
        const emptyMsg = getEl('suggestions-config-empty');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';
        showStatus('', false);

        try {
            const response = await fetch('/api/suggestions/admin');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            rows = await response.json();
            if (!Array.isArray(rows)) rows = [];
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load suggestions: ${err.message}`, true);
        }
    }

    function clearEditForm() {
        ['suggestions-config-edit-category', 'suggestions-config-edit-title-input', 'suggestions-config-edit-prompt',
            'suggestions-config-edit-description', 'suggestions-config-edit-action-label', 'suggestions-config-edit-function',
            'suggestions-config-edit-requires', 'suggestions-config-edit-sensitivity'].forEach((id) => {
            const el = getEl(id);
            if (el) el.value = '';
        });
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('suggestions-config-edit-modal');
        const title = getEl('suggestions-config-edit-title');
        const errEl = getEl('suggestions-config-edit-error');
        if (!modal) return;
        if (title) title.textContent = 'Add Suggestion';
        clearEditForm();
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        editingId = id;
        const modal = getEl('suggestions-config-edit-modal');
        const title = getEl('suggestions-config-edit-title');
        const errEl = getEl('suggestions-config-edit-error');
        if (!modal) return;
        if (title) title.textContent = 'Edit Suggestion';
        getEl('suggestions-config-edit-category').value = row.category || '';
        getEl('suggestions-config-edit-title-input').value = row.title || '';
        getEl('suggestions-config-edit-prompt').value = row.prompt || '';
        getEl('suggestions-config-edit-description').value = row.description || '';
        getEl('suggestions-config-edit-action-label').value = row.action_label || '';
        getEl('suggestions-config-edit-function').value = row.function || '';
        getEl('suggestions-config-edit-requires').value = Array.isArray(row.requires) ? row.requires.join(', ') : '';
        getEl('suggestions-config-edit-sensitivity').value = row.sensitivity || '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('suggestions-config-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    function readEditForm() {
        const requiresRaw = (getEl('suggestions-config-edit-requires') && getEl('suggestions-config-edit-requires').value || '').trim();
        const requires = requiresRaw ? requiresRaw.split(',').map((s) => s.trim()).filter(Boolean) : [];
        const body = {
            category: (getEl('suggestions-config-edit-category') && getEl('suggestions-config-edit-category').value || '').trim(),
            title: (getEl('suggestions-config-edit-title-input') && getEl('suggestions-config-edit-title-input').value || '').trim(),
            prompt: (getEl('suggestions-config-edit-prompt') && getEl('suggestions-config-edit-prompt').value || '').trim(),
            description: (getEl('suggestions-config-edit-description') && getEl('suggestions-config-edit-description').value || '').trim(),
            action_label: (getEl('suggestions-config-edit-action-label') && getEl('suggestions-config-edit-action-label').value || '').trim(),
            function: (getEl('suggestions-config-edit-function') && getEl('suggestions-config-edit-function').value || '').trim(),
            sensitivity: (getEl('suggestions-config-edit-sensitivity') && getEl('suggestions-config-edit-sensitivity').value || '').trim()
        };
        if (requires.length > 0) body.requires = requires;
        return body;
    }

    async function saveEditModal() {
        const errEl = getEl('suggestions-config-edit-error');
        const body = readEditForm();
        if (!body.category || !body.title) {
            if (errEl) {
                errEl.textContent = 'Category and title are required.';
                errEl.style.display = 'block';
            }
            return;
        }
        if (!body.prompt && !body.function) {
            if (errEl) {
                errEl.textContent = 'Prompt or function is required.';
                errEl.style.display = 'block';
            }
            return;
        }
        try {
            const url = editingId ? `/api/suggestions/${editingId}` : '/api/suggestions';
            const method = editingId ? 'PATCH' : 'POST';
            const response = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            closeEditModal();
            showStatus(editingId ? 'Suggestion updated.' : 'Suggestion added.', false);
            await load();
            await refreshRuntimeSuggestions();
        } catch (err) {
            if (errEl) {
                errEl.textContent = err.message;
                errEl.style.display = 'block';
            }
        }
    }

    async function deleteSuggestion(id) {
        const row = rows.find((r) => r.id === id);
        const name = row ? `${row.category} — ${row.title}` : 'this suggestion';
        if (!window.confirm(`Delete suggestion "${name}"?`)) return;
        try {
            const response = await fetch(`/api/suggestions/${id}`, { method: 'DELETE' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('Suggestion deleted.', false);
            await load();
            await refreshRuntimeSuggestions();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    function closeImportModal() {
        const modal = getEl('suggestions-config-import-modal');
        if (modal) modal.style.display = 'none';
        pendingImportFile = null;
        pendingImportPreview = null;
    }

    function renderImportConflicts() {
        const tbody = getEl('suggestions-config-import-conflicts-tbody');
        if (!tbody || !pendingImportPreview) return;
        const conflicts = pendingImportPreview.conflicts || [];
        if (conflicts.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3">No conflicts — new suggestions will be added.</td></tr>';
            return;
        }
        tbody.innerHTML = conflicts.map((c) => {
            const safeKey = String(c.key || '').replace(/"/g, '');
            const existingTitle = c.existing && c.existing.title ? c.existing.title : '';
            const uploadedTitle = c.uploaded && c.uploaded.title ? c.uploaded.title : '';
            return `
            <tr>
                <td><strong>${escapeHtml(c.key)}</strong><br><span style="color:var(--color-text-muted);font-size:var(--text-sm);">Uploaded: ${escapeHtml(uploadedTitle)}</span></td>
                <td>${escapeHtml(existingTitle)}</td>
                <td>
                    <label><input type="radio" name="suggestions-import-${safeKey}" value="keep" checked> Keep existing</label><br>
                    <label><input type="radio" name="suggestions-import-${safeKey}" value="replace"> Use uploaded</label>
                </td>
            </tr>
        `;
        }).join('');
    }

    async function onUploadSelected(file) {
        if (!file) return;
        pendingImportFile = file;
        const formData = new FormData();
        formData.append('file', file);
        showStatus('Checking upload…', false);
        try {
            const response = await fetch('/api/suggestions/import/preview', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            pendingImportPreview = await response.json();
            renderImportConflicts();
            const modal = getEl('suggestions-config-import-modal');
            if (modal) modal.style.display = 'flex';
            showStatus('', false);
        } catch (err) {
            showStatus(err.message, true);
            pendingImportFile = null;
        }
    }

    async function applyImport() {
        if (!pendingImportFile) return;
        const resolutions = {};
        (pendingImportPreview && pendingImportPreview.conflicts || []).forEach((c) => {
            const safeKey = String(c.key || '').replace(/"/g, '');
            const selected = document.querySelector(`input[name="suggestions-import-${safeKey}"]:checked`);
            resolutions[c.key] = selected ? selected.value : 'keep';
        });
        const formData = new FormData();
        formData.append('file', pendingImportFile);
        formData.append('resolutions', JSON.stringify(resolutions));
        try {
            const response = await fetch('/api/suggestions/import', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            closeImportModal();
            showStatus('Import applied.', false);
            await load();
            await refreshRuntimeSuggestions();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    function init() {
        const addBtn = getEl('suggestions-config-add-btn');
        if (addBtn) addBtn.addEventListener('click', openCreateModal);

        const downloadBtn = getEl('suggestions-config-download-btn');
        if (downloadBtn) {
            downloadBtn.addEventListener('click', () => {
                window.location.href = '/api/suggestions/export';
            });
        }

        const uploadInput = getEl('suggestions-config-upload-input');
        if (uploadInput) {
            uploadInput.addEventListener('change', () => {
                const file = uploadInput.files && uploadInput.files[0];
                uploadInput.value = '';
                if (file) void onUploadSelected(file);
            });
        }

        const editSave = getEl('suggestions-config-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editCancel = getEl('suggestions-config-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('suggestions-config-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);

        const importApply = getEl('suggestions-config-import-apply');
        if (importApply) importApply.addEventListener('click', () => { void applyImport(); });
        const importCancel = getEl('suggestions-config-import-cancel');
        if (importCancel) importCancel.addEventListener('click', closeImportModal);
        const importClose = getEl('suggestions-config-import-close');
        if (importClose) importClose.addEventListener('click', closeImportModal);
    }

    return { init, load };
})();

Modals.SuggestionsConfig.init();
