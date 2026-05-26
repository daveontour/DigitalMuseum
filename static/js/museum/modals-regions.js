'use strict';

Modals.RegionsConfig = (() => {
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

    function formatBBox(bbox) {
        if (!Array.isArray(bbox) || bbox.length !== 4) return '—';
        return bbox.map((n) => Number(n).toFixed(2)).join(', ');
    }

    function parseRegionRow(row) {
        if (row.reserved) return null;
        if (row.code) {
            return {
                code: row.code,
                label: row.label || '',
                bbox: Array.isArray(row.bbox) ? row.bbox.slice() : []
            };
        }
        try {
            return JSON.parse(row.text);
        } catch (_e) {
            return null;
        }
    }

    function showStatus(message, isError) {
        const el = getEl('regions-config-status');
        if (!el) return;
        el.textContent = message || '';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-text-muted)';
    }

    async function refreshRuntimeConfig() {
        try {
            const response = await fetch('/api/regions');
            if (!response.ok) return;
            const data = await response.json();
            if (typeof Regions !== 'undefined' && Regions.setConfig) {
                Regions.setConfig(data);
            }
        } catch (err) {
            console.warn('Failed to refresh regions config:', err);
        }
    }

    function bindTableButtons() {
        const tbody = getEl('regions-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.regions-config-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.regions-config-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteRegion(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.regions-config-move-up-btn').forEach((btn) => {
            btn.addEventListener('click', () => moveRegion(parseInt(btn.dataset.id, 10), -1));
        });
        tbody.querySelectorAll('.regions-config-move-down-btn').forEach((btn) => {
            btn.addEventListener('click', () => moveRegion(parseInt(btn.dataset.id, 10), 1));
        });
    }

    function renderTable() {
        const tbody = getEl('regions-config-tbody');
        const loading = getEl('regions-config-loading');
        const tableWrap = getEl('regions-config-table-wrap');
        const emptyMsg = getEl('regions-config-empty');
        if (!tbody) return;

        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';

        const regionRows = rows.filter((r) => !r.reserved && !String(r.key || '').startsWith('__default_'));
        if (regionRows.length === 0) {
            tbody.innerHTML = '';
            if (emptyMsg) emptyMsg.style.display = 'block';
            return;
        }
        if (emptyMsg) emptyMsg.style.display = 'none';

        const sorted = regionRows.slice().sort((a, b) => {
            if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
            return a.id - b.id;
        });

        tbody.innerHTML = sorted.map((row, index) => {
            const def = parseRegionRow(row) || { code: row.key, label: '', bbox: [] };
            return `
                <tr data-id="${row.id}">
                    <td>${escapeHtml(def.code || row.key)}</td>
                    <td>${escapeHtml(def.label || '')}</td>
                    <td><code>${escapeHtml(formatBBox(def.bbox))}</code></td>
                    <td>${row.sort_order}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                            <button type="button" class="regions-config-move-up-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" ${index === 0 ? 'disabled' : ''} aria-label="Move up" title="Move up"><i class="fas fa-arrow-up" aria-hidden="true"></i></button>
                            <button type="button" class="regions-config-move-down-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" ${index === sorted.length - 1 ? 'disabled' : ''} aria-label="Move down" title="Move down"><i class="fas fa-arrow-down" aria-hidden="true"></i></button>
                            <button type="button" class="regions-config-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit region" title="Edit"><i class="fas fa-edit" aria-hidden="true"></i></button>
                            <button type="button" class="regions-config-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete region" title="Delete"><i class="fas fa-trash-alt" aria-hidden="true"></i></button>
                        </span>
                    </td>
                </tr>
            `;
        }).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('regions-config-loading');
        const tableWrap = getEl('regions-config-table-wrap');
        const emptyMsg = getEl('regions-config-empty');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';
        showStatus('', false);

        try {
            const response = await fetch('/api/regions/admin');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            rows = await response.json();
            if (!Array.isArray(rows)) rows = [];
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load regions: ${err.message}`, true);
        }
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('regions-config-edit-modal');
        const title = getEl('regions-config-edit-title');
        const code = getEl('regions-config-edit-code');
        const label = getEl('regions-config-edit-label');
        const minLon = getEl('regions-config-edit-min-lon');
        const minLat = getEl('regions-config-edit-min-lat');
        const maxLon = getEl('regions-config-edit-max-lon');
        const maxLat = getEl('regions-config-edit-max-lat');
        const errEl = getEl('regions-config-edit-error');
        if (!modal || !code || !label) return;
        if (title) title.textContent = 'Add Region';
        code.value = '';
        label.value = '';
        [minLon, minLat, maxLon, maxLat].forEach((el) => { if (el) el.value = ''; });
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        const def = parseRegionRow(row);
        if (!def) return;
        editingId = id;
        const modal = getEl('regions-config-edit-modal');
        const title = getEl('regions-config-edit-title');
        const code = getEl('regions-config-edit-code');
        const label = getEl('regions-config-edit-label');
        const minLon = getEl('regions-config-edit-min-lon');
        const minLat = getEl('regions-config-edit-min-lat');
        const maxLon = getEl('regions-config-edit-max-lon');
        const maxLat = getEl('regions-config-edit-max-lat');
        const errEl = getEl('regions-config-edit-error');
        if (!modal || !code || !label) return;
        if (title) title.textContent = 'Edit Region';
        code.value = def.code || row.key || '';
        label.value = def.label || '';
        if (minLon) minLon.value = def.bbox[0] ?? '';
        if (minLat) minLat.value = def.bbox[1] ?? '';
        if (maxLon) maxLon.value = def.bbox[2] ?? '';
        if (maxLat) maxLat.value = def.bbox[3] ?? '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('regions-config-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    function readEditForm() {
        const code = getEl('regions-config-edit-code');
        const label = getEl('regions-config-edit-label');
        const minLon = getEl('regions-config-edit-min-lon');
        const minLat = getEl('regions-config-edit-min-lat');
        const maxLon = getEl('regions-config-edit-max-lon');
        const maxLat = getEl('regions-config-edit-max-lat');
        return {
            code: (code && code.value || '').trim(),
            label: (label && label.value || '').trim(),
            bbox: [
                parseFloat(minLon && minLon.value),
                parseFloat(minLat && minLat.value),
                parseFloat(maxLon && maxLon.value),
                parseFloat(maxLat && maxLat.value)
            ]
        };
    }

    async function saveEditModal() {
        const errEl = getEl('regions-config-edit-error');
        const body = readEditForm();
        if (!body.code || !body.label || body.bbox.some((n) => Number.isNaN(n))) {
            if (errEl) {
                errEl.textContent = 'Code, label, and all four bbox numbers are required.';
                errEl.style.display = 'block';
            }
            return;
        }
        try {
            const url = editingId ? `/api/regions/${editingId}` : '/api/regions';
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
            showStatus(editingId ? 'Region updated.' : 'Region added.', false);
            await load();
            await refreshRuntimeConfig();
        } catch (err) {
            if (errEl) {
                errEl.textContent = err.message;
                errEl.style.display = 'block';
            }
        }
    }

    async function deleteRegion(id) {
        const row = rows.find((r) => r.id === id);
        const def = row ? parseRegionRow(row) : null;
        const name = def ? def.code : 'this region';
        if (!window.confirm(`Delete region "${name}"?`)) return;
        try {
            const response = await fetch(`/api/regions/${id}`, { method: 'DELETE' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('Region deleted.', false);
            await load();
            await refreshRuntimeConfig();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    async function moveRegion(id, direction) {
        const regionRows = rows.filter((r) => !r.reserved && !String(r.key || '').startsWith('__default_'));
        const sorted = regionRows.slice().sort((a, b) => {
            if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
            return a.id - b.id;
        });
        const index = sorted.findIndex((r) => r.id === id);
        const swapIndex = index + direction;
        if (index < 0 || swapIndex < 0 || swapIndex >= sorted.length) return;
        const a = sorted[index];
        const b = sorted[swapIndex];
        const items = [
            { id: a.id, sort_order: b.sort_order },
            { id: b.id, sort_order: a.sort_order }
        ];
        try {
            const response = await fetch('/api/regions/reorder', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ items })
            });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            await load();
            await refreshRuntimeConfig();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    function closeImportModal() {
        const modal = getEl('regions-config-import-modal');
        if (modal) modal.style.display = 'none';
        pendingImportFile = null;
        pendingImportPreview = null;
    }

    function renderImportConflicts() {
        const tbody = getEl('regions-config-import-conflicts-tbody');
        if (!tbody || !pendingImportPreview) return;
        const conflicts = pendingImportPreview.conflicts || [];
        if (conflicts.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3">No conflicts — new regions will be added.</td></tr>';
            return;
        }
        tbody.innerHTML = conflicts.map((c) => {
            const safeKey = String(c.key || '').replace(/"/g, '');
            return `
            <tr>
                <td><strong>${escapeHtml(c.key)}</strong><br><span style="color:var(--color-text-muted);font-size:var(--text-sm);">Uploaded: ${escapeHtml(c.uploaded && c.uploaded.label || '')}<br><code>${escapeHtml(formatBBox(c.uploaded && c.uploaded.bbox))}</code></span></td>
                <td>${escapeHtml(c.existing && c.existing.label || '')}<br><code>${escapeHtml(formatBBox(c.existing && c.existing.bbox))}</code></td>
                <td>
                    <label><input type="radio" name="regions-import-${safeKey}" value="keep" checked> Keep existing</label><br>
                    <label><input type="radio" name="regions-import-${safeKey}" value="replace"> Use uploaded</label>
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
            const response = await fetch('/api/regions/import/preview', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            pendingImportPreview = await response.json();
            renderImportConflicts();
            const modal = getEl('regions-config-import-modal');
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
            const selected = document.querySelector(`input[name="regions-import-${safeKey}"]:checked`);
            resolutions[c.key] = selected ? selected.value : 'keep';
        });
        const formData = new FormData();
        formData.append('file', pendingImportFile);
        formData.append('resolutions', JSON.stringify(resolutions));
        try {
            const response = await fetch('/api/regions/import', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            closeImportModal();
            showStatus('Import applied.', false);
            await load();
            await refreshRuntimeConfig();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    function init() {
        const addBtn = getEl('regions-config-add-btn');
        if (addBtn) addBtn.addEventListener('click', openCreateModal);

        const downloadBtn = getEl('regions-config-download-btn');
        if (downloadBtn) {
            downloadBtn.addEventListener('click', () => {
                window.location.href = '/api/regions/export';
            });
        }

        const uploadInput = getEl('regions-config-upload-input');
        if (uploadInput) {
            uploadInput.addEventListener('change', () => {
                const file = uploadInput.files && uploadInput.files[0];
                uploadInput.value = '';
                if (file) void onUploadSelected(file);
            });
        }

        const editSave = getEl('regions-config-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editCancel = getEl('regions-config-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('regions-config-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);

        const importApply = getEl('regions-config-import-apply');
        if (importApply) importApply.addEventListener('click', () => { void applyImport(); });
        const importCancel = getEl('regions-config-import-cancel');
        if (importCancel) importCancel.addEventListener('click', closeImportModal);
        const importClose = getEl('regions-config-import-close');
        if (importClose) importClose.addEventListener('click', closeImportModal);
    }

    return { init, load };
})();

Modals.RegionsConfig.init();
