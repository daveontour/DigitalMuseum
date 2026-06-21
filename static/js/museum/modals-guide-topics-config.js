'use strict';

Modals.GuideTopicsConfig = (() => {
    let rows = [];
    let editingId = null;
    let editingSteps = [];
    let pendingImportFile = null;
    let pendingImportPreview = null;

    const NAV_ACTIONS = [
        '',
        'showGettingStartedDialog',
        'openSmsMessages',
        'openEmailGallery',
        'openImageGallery',
        'openFacebookAlbums',
        'openFacebookPosts',
        'openMultiSourceSearch',
        'openLocations',
        'openArtefacts',
        'openIdentityProfile',
        'openDataImport',
        'openDataSourcesImport',
        'openDataMaintenance',
        'openDataImportImport',
        'openDataImportMaintenance',
        'openDataImportBackgroundJobs',
        'openConfiguration',
        'openPreviousResponses',
        'openSuggestions',
        'openContacts',
        'openContactsRelationships',
        'openProfiles',
        'openSensitiveData',
        'openDashboard',
        'openHaveAChat',
        'openRandomQuestion',
        'openTodaysThing',
        'openInterviewer',
        'openPersonalitySettings',
        'openConfigAppearance',
        'openConfigApiKeys',
        'openConfigAiSetup',
        'openConfigSubjectConfiguration',
        'openConfigRegions',
        'openConfigSuggestions',
        'openConfigGuideTopics',
        'openConfigCustomVoices',
        'openConfigManageVisitorKeys',
        'openConfigToolsAccess',
        'openSettingsManageKeys',
        'openReferenceDocuments',
        'openToolCallsDialog',
        'closeOpenDialog',
        'pause0_5s',
        'pause1s',
        'pause2s',
        'pause5s',
    ];

    const POSITIONS = [
        'middle-center',
        'top-left', 'top-center', 'top-right',
        'middle-left', 'middle-right',
        'bottom-left', 'bottom-center', 'bottom-right',
    ];

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
        const el = getEl('guide-topics-config-status');
        if (!el) return;
        el.textContent = message || '';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-text-muted)';
    }

    function invalidateGuideCache() {
        if (typeof Guide !== 'undefined' && Guide.invalidateCache) {
            Guide.invalidateCache();
        }
    }

    // ── Table ──────────────────────────────────────────────────────────────────

    function bindTableButtons() {
        const tbody = getEl('guide-topics-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.guide-topics-config-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.guide-topics-config-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteTopic(parseInt(btn.dataset.id, 10)));
        });
    }

    function renderTable() {
        const tbody = getEl('guide-topics-config-tbody');
        const loading = getEl('guide-topics-config-loading');
        const tableWrap = getEl('guide-topics-config-table-wrap');
        const emptyMsg = getEl('guide-topics-config-empty');
        if (!tbody) return;

        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';

        if (!rows || rows.length === 0) {
            tbody.innerHTML = '';
            if (emptyMsg) emptyMsg.style.display = 'block';
            return;
        }
        if (emptyMsg) emptyMsg.style.display = 'none';

        tbody.innerHTML = rows.map((row) => {
            const steps = Array.isArray(row.steps) ? row.steps : [];
            return `
                <tr data-id="${row.id}">
                    <td style="font-family:ui-monospace,monospace;font-size:var(--text-sm);">${escapeHtml(row.key || '')}</td>
                    <td>${escapeHtml(row.title || '')}</td>
                    <td>${escapeHtml(row.category || '')}</td>
                    <td style="text-align:center;">${steps.length}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                            <button type="button" class="guide-topics-config-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit topic" title="Edit">
                                <i class="fas fa-edit" aria-hidden="true"></i>
                            </button>
                            <button type="button" class="guide-topics-config-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete topic" title="Delete">
                                <i class="fas fa-trash-alt" aria-hidden="true"></i>
                            </button>
                        </span>
                    </td>
                </tr>
            `;
        }).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('guide-topics-config-loading');
        const tableWrap = getEl('guide-topics-config-table-wrap');
        const emptyMsg = getEl('guide-topics-config-empty');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';
        showStatus('', false);

        try {
            const response = await fetch('/api/guide-topics/admin');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            rows = await response.json();
            if (!Array.isArray(rows)) rows = [];
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load guide topics: ${err.message}`, true);
        }
    }

    // ── Step editor ────────────────────────────────────────────────────────────

    function navActionOptions(selected) {
        return NAV_ACTIONS.map((v) => {
            const label = v === '' ? '— none —' : v;
            const sel = v === (selected || '') ? ' selected' : '';
            return `<option value="${escapeHtml(v)}"${sel}>${escapeHtml(label)}</option>`;
        }).join('');
    }

    function positionOptions(selected) {
        return POSITIONS.map((v) => {
            const sel = v === (selected || 'middle-center') ? ' selected' : '';
            return `<option value="${escapeHtml(v)}"${sel}>${escapeHtml(v)}</option>`;
        }).join('');
    }

    function renderStepCard(step, index) {
        return `
            <div class="guide-step-card" data-index="${index}" style="border:1px solid var(--color-border);border-radius:6px;padding:0.75rem;margin-bottom:0.5rem;background:var(--color-bg-elevated);">
                <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:0.5rem;">
                    <strong style="font-size:var(--text-sm);color:var(--color-text-muted);">Step ${index + 1}</strong>
                    <button type="button" class="guide-step-remove-btn modal-btn" data-index="${index}" style="padding:2px 8px;font-size:var(--text-sm);" title="Remove step"><i class="fas fa-times"></i></button>
                </div>
                <div class="setting-group" style="margin-bottom:0.4rem;">
                    <label class="artefact-field-label">Text</label>
                    <textarea class="form-control guide-step-text" rows="2" data-index="${index}" placeholder="Instruction text shown to user">${escapeHtml(step.text || '')}</textarea>
                </div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.5rem;margin-bottom:0.4rem;">
                    <div class="setting-group" style="margin-bottom:0;">
                        <label class="artefact-field-label">Glow selector (optional)</label>
                        <input type="text" class="form-control guide-step-glow" data-index="${index}" value="${escapeHtml(step.glow || '')}" placeholder="#element-id">
                    </div>
                    <div class="setting-group" style="margin-bottom:0;">
                        <label class="artefact-field-label">Position</label>
                        <select class="form-control guide-step-position" data-index="${index}">${positionOptions(step.position)}</select>
                    </div>
                </div>
                <div class="setting-group" style="margin-bottom:0.4rem;">
                    <label class="artefact-field-label">Navigate action (optional)</label>
                    <select class="form-control guide-step-nav" data-index="${index}">${navActionOptions(step.navigate_action || step.navigateAction || '')}</select>
                </div>
                <div class="setting-group" style="margin-bottom:0.4rem;">
                    <label class="artefact-field-label">Click selector (optional)</label>
                    <input type="text" class="form-control guide-step-click-selector" data-index="${index}" value="${escapeHtml(step.click_selector || step.clickSelector || step.click || '')}" placeholder="#element-id or .css-selector">
                </div>
                <div class="setting-group" style="margin-bottom:0;">
                    <label class="artefact-field-label">Image URL (optional)</label>
                    <input type="text" class="form-control guide-step-image-url" data-index="${index}" value="${escapeHtml(step.image_url || step.imageUrl || step.image || '')}" placeholder="/static/images/…">
                </div>
            </div>
        `;
    }

    function renderStepsList() {
        const container = getEl('guide-topics-steps-list');
        if (!container) return;
        if (editingSteps.length === 0) {
            container.innerHTML = '<p style="color:var(--color-text-muted);font-size:var(--text-sm);margin:0;">No steps yet. Click "Add step" to add one.</p>';
        } else {
            container.innerHTML = editingSteps.map((s, i) => renderStepCard(s, i)).join('');
        }
        container.querySelectorAll('.guide-step-remove-btn').forEach((btn) => {
            btn.addEventListener('click', () => {
                const idx = parseInt(btn.dataset.index, 10);
                syncStepsFromDOM();
                editingSteps.splice(idx, 1);
                renderStepsList();
            });
        });
    }

    function syncStepsFromDOM() {
        const container = getEl('guide-topics-steps-list');
        if (!container) return;
        editingSteps.forEach((step, index) => {
            const text = container.querySelector(`.guide-step-text[data-index="${index}"]`);
            const glow = container.querySelector(`.guide-step-glow[data-index="${index}"]`);
            const position = container.querySelector(`.guide-step-position[data-index="${index}"]`);
            const nav = container.querySelector(`.guide-step-nav[data-index="${index}"]`);
            const clickSelector = container.querySelector(`.guide-step-click-selector[data-index="${index}"]`);
            const imageUrl = container.querySelector(`.guide-step-image-url[data-index="${index}"]`);
            if (text) step.text = text.value;
            if (glow) step.glow = glow.value;
            if (position) step.position = position.value;
            if (nav) step.navigate_action = nav.value;
            if (clickSelector) step.click_selector = clickSelector.value;
            if (imageUrl) step.image_url = imageUrl.value;
        });
    }

    function readStepsFromDOM() {
        syncStepsFromDOM();
        return editingSteps.map((step) => {
            const s = {};
            if ((step.text || '').trim()) s.text = step.text.trim();
            if ((step.glow || '').trim()) s.glow = step.glow.trim();
            if (step.position && step.position !== 'middle-center') s.position = step.position;
            if ((step.navigate_action || '').trim()) s.navigate_action = step.navigate_action.trim();
            if ((step.click_selector || '').trim()) s.click_selector = step.click_selector.trim();
            if ((step.image_url || '').trim()) s.image_url = step.image_url.trim();
            return s;
        });
    }

    // ── Edit modal ─────────────────────────────────────────────────────────────

    function clearEditForm() {
        ['guide-topics-config-edit-key', 'guide-topics-config-edit-title-input',
         'guide-topics-config-edit-description', 'guide-topics-config-edit-category',
         'guide-topics-config-edit-dismiss-nav'].forEach((id) => {
            const el = getEl(id);
            if (el) el.value = '';
        });
        const recEl = getEl('guide-topics-config-edit-recommended');
        if (recEl) recEl.checked = false;
        editingSteps = [];
        renderStepsList();
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('guide-topics-config-edit-modal');
        const title = getEl('guide-topics-config-edit-title');
        const errEl = getEl('guide-topics-config-edit-error');
        const keyInput = getEl('guide-topics-config-edit-key');
        if (!modal) return;
        if (title) title.textContent = 'Add Guide Topic';
        clearEditForm();
        if (keyInput) keyInput.readOnly = false;
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        editingId = id;
        const modal = getEl('guide-topics-config-edit-modal');
        const title = getEl('guide-topics-config-edit-title');
        const errEl = getEl('guide-topics-config-edit-error');
        const keyInput = getEl('guide-topics-config-edit-key');
        if (!modal) return;
        if (title) title.textContent = 'Edit Guide Topic';
        if (keyInput) { keyInput.value = row.key || ''; keyInput.readOnly = true; }
        const titleInput = getEl('guide-topics-config-edit-title-input');
        if (titleInput) titleInput.value = row.title || '';
        const descInput = getEl('guide-topics-config-edit-description');
        if (descInput) descInput.value = row.description || '';
        const catInput = getEl('guide-topics-config-edit-category');
        if (catInput) catInput.value = row.category || '';
        const recEl = getEl('guide-topics-config-edit-recommended');
        if (recEl) recEl.checked = !!row.recommended;
        const dismissNavInput = getEl('guide-topics-config-edit-dismiss-nav');
        if (dismissNavInput) dismissNavInput.value = row.dismiss_navigate_action || row.dismissNavigateAction || '';
        editingSteps = Array.isArray(row.steps) ? row.steps.map((s) => Object.assign({}, s)) : [];
        renderStepsList();
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('guide-topics-config-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
        editingSteps = [];
    }

    async function saveEditModal() {
        const errEl = getEl('guide-topics-config-edit-error');
        const key = (getEl('guide-topics-config-edit-key') && getEl('guide-topics-config-edit-key').value || '').trim();
        const title = (getEl('guide-topics-config-edit-title-input') && getEl('guide-topics-config-edit-title-input').value || '').trim();
        const description = (getEl('guide-topics-config-edit-description') && getEl('guide-topics-config-edit-description').value || '').trim();
        const category = (getEl('guide-topics-config-edit-category') && getEl('guide-topics-config-edit-category').value || '').trim();
        const recommended = !!(getEl('guide-topics-config-edit-recommended') && getEl('guide-topics-config-edit-recommended').checked);
        const dismissNavigateAction = (getEl('guide-topics-config-edit-dismiss-nav') && getEl('guide-topics-config-edit-dismiss-nav').value || '').trim();
        const steps = readStepsFromDOM();

        if (!key) {
            if (errEl) { errEl.textContent = 'Key is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!title) {
            if (errEl) { errEl.textContent = 'Title is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!category) {
            if (errEl) { errEl.textContent = 'Category is required.'; errEl.style.display = 'block'; }
            return;
        }

        const body = { key, title, description, category, recommended, steps };
        if (dismissNavigateAction) body.dismiss_navigate_action = dismissNavigateAction;
        try {
            const url = editingId ? `/api/guide-topics/${editingId}` : '/api/guide-topics';
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
            showStatus(editingId ? 'Guide topic updated.' : 'Guide topic added.', false);
            await load();
            invalidateGuideCache();
        } catch (err) {
            if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
        }
    }

    async function clearAllTopics() {
        if (!rows || rows.length === 0) {
            showStatus('No guide topics to clear.', false);
            return;
        }
        const count = rows.length;
        const confirmFn = (typeof AppDialogs !== 'undefined' && AppDialogs.showAppConfirm)
            ? AppDialogs.showAppConfirm.bind(AppDialogs)
            : null;
        const message = `Delete all ${count} guide topic${count === 1 ? '' : 's'} from the database?\n\nThe Guide will be empty until you add topics again. On the next application restart, guide topics will be reloaded from the guide_topics.json file on the filesystem (inserting any keys that are still missing).`;
        let confirmed = false;
        if (confirmFn) {
            confirmed = await confirmFn('Clear all guide topics?', message, { danger: true, confirmLabel: 'Clear All' });
        } else {
            confirmed = window.confirm(`Clear all guide topics?\n\n${message}`);
        }
        if (!confirmed) return;
        try {
            const response = await fetch('/api/guide-topics/all', { method: 'DELETE' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('All guide topics cleared.', false);
            await load();
            invalidateGuideCache();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    async function deleteTopic(id) {
        const row = rows.find((r) => r.id === id);
        const name = row ? `${row.key} — ${row.title}` : 'this topic';
        if (!window.confirm(`Delete guide topic "${name}"?`)) return;
        try {
            const response = await fetch(`/api/guide-topics/${id}`, { method: 'DELETE' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('Guide topic deleted.', false);
            await load();
            invalidateGuideCache();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    // ── Import / Export ────────────────────────────────────────────────────────

    function closeImportModal() {
        const modal = getEl('guide-topics-config-import-modal');
        if (modal) modal.style.display = 'none';
        pendingImportFile = null;
        pendingImportPreview = null;
    }

    function renderImportConflicts() {
        const tbody = getEl('guide-topics-config-import-conflicts-tbody');
        if (!tbody || !pendingImportPreview) return;
        const conflicts = pendingImportPreview.conflicts || [];
        if (conflicts.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3">No conflicts — new topics will be added.</td></tr>';
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
                        <label><input type="radio" name="guide-topics-import-${safeKey}" value="keep" checked> Keep existing</label><br>
                        <label><input type="radio" name="guide-topics-import-${safeKey}" value="replace"> Use uploaded</label>
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
            const response = await fetch('/api/guide-topics/import/preview', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            pendingImportPreview = await response.json();
            renderImportConflicts();
            const modal = getEl('guide-topics-config-import-modal');
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
            const selected = document.querySelector(`input[name="guide-topics-import-${safeKey}"]:checked`);
            resolutions[c.key] = selected ? selected.value : 'keep';
        });
        const formData = new FormData();
        formData.append('file', pendingImportFile);
        formData.append('resolutions', JSON.stringify(resolutions));
        try {
            const response = await fetch('/api/guide-topics/import', { method: 'POST', body: formData });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            closeImportModal();
            showStatus('Import applied.', false);
            await load();
            invalidateGuideCache();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    // ── Init ───────────────────────────────────────────────────────────────────

    function init() {
        const addBtn = getEl('guide-topics-config-add-btn');
        if (addBtn) addBtn.addEventListener('click', openCreateModal);

        const downloadBtn = getEl('guide-topics-config-download-btn');
        if (downloadBtn) {
            downloadBtn.addEventListener('click', () => { window.location.href = '/api/guide-topics/export'; });
        }

        const clearAllBtn = getEl('guide-topics-config-clear-all-btn');
        if (clearAllBtn) clearAllBtn.addEventListener('click', () => { void clearAllTopics(); });

        const uploadInput = getEl('guide-topics-config-upload-input');
        if (uploadInput) {
            uploadInput.addEventListener('change', () => {
                const file = uploadInput.files && uploadInput.files[0];
                uploadInput.value = '';
                if (file) void onUploadSelected(file);
            });
        }

        const addStepBtn = getEl('guide-topics-config-add-step-btn');
        if (addStepBtn) {
            addStepBtn.addEventListener('click', () => {
                syncStepsFromDOM();
                editingSteps.push({ text: '', glow: '', position: 'middle-center', navigate_action: '', click_selector: '', image_url: '' });
                renderStepsList();
            });
        }

        const editSave = getEl('guide-topics-config-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editCancel = getEl('guide-topics-config-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('guide-topics-config-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);

        const importApply = getEl('guide-topics-config-import-apply');
        if (importApply) importApply.addEventListener('click', () => { void applyImport(); });
        const importCancel = getEl('guide-topics-config-import-cancel');
        if (importCancel) importCancel.addEventListener('click', closeImportModal);
        const importClose = getEl('guide-topics-config-import-close');
        if (importClose) importClose.addEventListener('click', closeImportModal);
    }

    return { init, load };
})();

Modals.GuideTopicsConfig.init();
