'use strict';

/**
 * Path Equivalences — manage filesystem-import dedup rules (e.g. "these two
 * folders are the same location, just on a different drive") so re-importing
 * from a moved photo tree doesn't duplicate what's already been imported.
 * Opened on demand from the "Upload Photos from Device" import modal.
 */
Modals.PathEquivalences = (() => {
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

    /** Open a native Electron folder picker. Falls back to null if unavailable. */
    async function showOpenDialog(options) {
        if (window.electronAPI && window.electronAPI.showOpenDialog) {
            const result = await window.electronAPI.showOpenDialog(options);
            if (result && !result.canceled && result.filePaths && result.filePaths.length > 0) {
                return result.filePaths[0];
            }
        }
        return null;
    }

    function bindTableButtons() {
        const tbody = getEl('path-equivalences-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.path-equivalence-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.path-equivalence-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => { void deleteRule(parseInt(btn.dataset.id, 10)); });
        });
    }

    function renderTable() {
        const tbody = getEl('path-equivalences-tbody');
        const loading = getEl('path-equivalences-loading');
        const empty = getEl('path-equivalences-empty');
        const table = getEl('path-equivalences-table');
        if (!tbody) return;
        if (loading) loading.style.display = 'none';

        if (rows.length === 0) {
            if (empty) empty.style.display = 'block';
            if (table) table.style.display = 'none';
            return;
        }
        if (empty) empty.style.display = 'none';
        if (table) table.style.display = 'table';

        tbody.innerHTML = rows.map((row) => `
            <tr data-id="${row.id}">
                <td><code>${escapeHtml(row.path_a)}</code></td>
                <td><code>${escapeHtml(row.path_b)}</code></td>
                <td>
                    <span class="manage-contacts-actions-cell">
                        <button type="button" class="path-equivalence-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit rule" title="Edit"><i class="fas fa-edit" aria-hidden="true"></i></button>
                        <button type="button" class="path-equivalence-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete rule" title="Delete"><i class="fas fa-trash-alt" aria-hidden="true"></i></button>
                    </span>
                </td>
            </tr>
        `).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('path-equivalences-loading');
        const table = getEl('path-equivalences-table');
        const empty = getEl('path-equivalences-empty');
        if (loading) loading.style.display = 'block';
        if (table) table.style.display = 'none';
        if (empty) empty.style.display = 'none';
        try {
            const res = await fetch('/api/path-equivalences', { credentials: 'same-origin' });
            if (!res.ok) {
                const err = await res.json().catch(() => ({}));
                throw new Error(err.detail || err.error || `HTTP ${res.status}`);
            }
            const data = await res.json();
            rows = Array.isArray(data.rules) ? data.rules : [];
            renderTable();
        } catch (err) {
            if (loading) {
                loading.textContent = `Failed to load path equivalences: ${err.message}`;
                loading.style.display = 'block';
            }
        }
    }

    function open() {
        const modal = getEl('path-equivalences-modal');
        if (modal) modal.style.display = 'flex';
        void load();
    }

    function close() {
        const modal = getEl('path-equivalences-modal');
        if (modal) modal.style.display = 'none';
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('path-equivalence-edit-modal');
        const title = getEl('path-equivalence-edit-title');
        const pathA = getEl('path-equivalence-edit-path-a');
        const pathB = getEl('path-equivalence-edit-path-b');
        const errEl = getEl('path-equivalence-edit-error');
        if (!modal || !pathA || !pathB) return;
        if (title) title.textContent = 'Add Rule';
        pathA.value = '';
        pathB.value = '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        editingId = id;
        const modal = getEl('path-equivalence-edit-modal');
        const title = getEl('path-equivalence-edit-title');
        const pathA = getEl('path-equivalence-edit-path-a');
        const pathB = getEl('path-equivalence-edit-path-b');
        const errEl = getEl('path-equivalence-edit-error');
        if (!modal || !pathA || !pathB) return;
        if (title) title.textContent = 'Edit Rule';
        pathA.value = row.path_a || '';
        pathB.value = row.path_b || '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('path-equivalence-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveEditModal() {
        const errEl = getEl('path-equivalence-edit-error');
        const pathA = (getEl('path-equivalence-edit-path-a') || {}).value || '';
        const pathB = (getEl('path-equivalence-edit-path-b') || {}).value || '';
        const body = { path_a: pathA.trim(), path_b: pathB.trim() };
        if (!body.path_a || !body.path_b) {
            if (errEl) {
                errEl.textContent = 'Both paths are required.';
                errEl.style.display = 'block';
            }
            return;
        }
        try {
            const url = editingId ? `/api/path-equivalences/${editingId}` : '/api/path-equivalences';
            const method = editingId ? 'PUT' : 'POST';
            const response = await fetch(url, {
                method,
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify(body),
            });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.detail || err.error || `HTTP ${response.status}`);
            }
            closeEditModal();
            await load();
        } catch (err) {
            if (errEl) {
                errEl.textContent = err.message;
                errEl.style.display = 'block';
            }
        }
    }

    async function deleteRule(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        if (!window.confirm(`Delete this path equivalence rule?\n\n${row.path_a}\n${row.path_b}`)) return;
        try {
            const response = await fetch(`/api/path-equivalences/${id}`, { method: 'DELETE', credentials: 'same-origin' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.detail || err.error || `HTTP ${response.status}`);
            }
            await load();
        } catch (err) {
            console.error('Error deleting path equivalence:', err);
        }
    }

    function init() {
        const openBtn = getEl('upload-photos-manage-path-equivalences-btn');
        if (openBtn) openBtn.addEventListener('click', open);

        const closeBtn = getEl('close-path-equivalences-modal');
        if (closeBtn) closeBtn.addEventListener('click', close);
        const modal = getEl('path-equivalences-modal');
        if (modal) {
            modal.addEventListener('click', (evt) => {
                if (evt.target === modal) close();
            });
        }

        const addBtn = getEl('path-equivalences-add-btn');
        if (addBtn) addBtn.addEventListener('click', openCreateModal);

        const editSave = getEl('path-equivalence-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editCancel = getEl('path-equivalence-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('path-equivalence-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);
        const editModal = getEl('path-equivalence-edit-modal');
        if (editModal) {
            editModal.addEventListener('click', (evt) => {
                if (evt.target === editModal) closeEditModal();
            });
        }

        const browseA = getEl('path-equivalence-edit-browse-a');
        if (browseA) {
            browseA.addEventListener('click', async () => {
                const p = await showOpenDialog({ title: 'Select folder for Path A', properties: ['openDirectory'] });
                if (p) getEl('path-equivalence-edit-path-a').value = p;
            });
        }
        const browseB = getEl('path-equivalence-edit-browse-b');
        if (browseB) {
            browseB.addEventListener('click', async () => {
                const p = await showOpenDialog({ title: 'Select folder for Path B', properties: ['openDirectory'] });
                if (p) getEl('path-equivalence-edit-path-b').value = p;
            });
        }
    }

    return { init, open, close };
})();

Modals.PathEquivalences.init();
