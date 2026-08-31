'use strict';

Modals.AIModelsConfig = (() => {
    let rows = [];
    let editingId = null;
    let classifierProvider = 'localai';

    /** Distinct dark row tints — stable per model key; readable with --color-text (#e2e8f3). */
    const AI_MODEL_ROW_PALETTE = [
        '#1a2540', '#1a2f4a', '#1a3a52', '#1a3548',
        '#1e2a45', '#1f2840', '#222540', '#252040',
        '#2a1f3d', '#2d2238', '#1f2d38', '#1a3238',
        '#1a3835', '#1a3530', '#243028', '#2d2820',
        '#2a2230', '#302028', '#1a2d4a', '#1d3461',
        '#1a3048', '#1c3354', '#1a3a5c', '#1e2d4f',
    ];

    function hashKeyToPaletteIndex(key) {
        const s = String(key || '').trim().toLowerCase();
        let h = 0;
        for (let i = 0; i < s.length; i++) {
            h = ((h << 5) - h + s.charCodeAt(i)) | 0;
        }
        return Math.abs(h) % AI_MODEL_ROW_PALETTE.length;
    }

    function rowBackgroundForKey(key) {
        return AI_MODEL_ROW_PALETTE[hashKeyToPaletteIndex(key)];
    }

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
        const el = getEl('ai-models-config-status');
        if (!el) return;
        el.textContent = message || '';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-text-muted)';
    }

    /** Refreshes every "AI Provider" selector in the app (top bar, Profiles, Interview,
     *  Have-a-Chat) after an AI Models tab change. App.refreshChatAvailability is
     *  loadLLMProviderAvailability() exposed by app.js's App module — the bare function name
     *  isn't reachable from this file, only the App-namespaced alias is. */
    function invalidateCaches() {
        if (typeof AIModels !== 'undefined') AIModels.invalidateCache();
        if (typeof App !== 'undefined' && App.refreshChatAvailability) void App.refreshChatAvailability();
    }

    function bindTableButtons() {
        const tbody = getEl('ai-models-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.ai-models-config-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.ai-models-config-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteModel(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.ai-models-config-move-up-btn').forEach((btn) => {
            btn.addEventListener('click', () => moveModel(parseInt(btn.dataset.id, 10), -1));
        });
        tbody.querySelectorAll('.ai-models-config-move-down-btn').forEach((btn) => {
            btn.addEventListener('click', () => moveModel(parseInt(btn.dataset.id, 10), 1));
        });
        tbody.querySelectorAll('.ai-models-config-enabled-checkbox').forEach((cb) => {
            cb.addEventListener('change', () => toggleEnabled(parseInt(cb.dataset.id, 10), cb.checked));
        });
        tbody.querySelectorAll('.ai-models-config-classifier-radio').forEach((radio) => {
            radio.addEventListener('change', () => {
                if (!radio.checked) return;
                const key = radio.value;
                classifierProvider = key;
                saveClassifierConfig({ classifier_provider: key })
                    .then(() => showStatus('Classifier updated.', false))
                    .catch((err) => {
                        showStatus(err.message, true);
                        void load();
                    });
            });
        });
    }

    function renderTable() {
        const tbody = getEl('ai-models-config-tbody');
        const loading = getEl('ai-models-config-loading');
        const tableWrap = getEl('ai-models-config-table-wrap');
        const emptyMsg = getEl('ai-models-config-empty');
        if (!tbody) return;

        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';
        const hostedCount = rows.filter((r) => r.key !== 'localai').length;
        if (emptyMsg) emptyMsg.style.display = hostedCount === 0 ? 'block' : 'none';

        const sorted = rows.slice().sort((a, b) => {
            if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
            return a.id - b.id;
        });

        tbody.innerHTML = sorted.map((row, index) => {
            const isLocal = row.key === 'localai';
            const rowBg = rowBackgroundForKey(row.key);
            const modelSlugDisplay = isLocal ? 'Ollama (local, offline)' : row.model_slug;
            const moveButtons = `
                        <button type="button" class="ai-models-config-move-up-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" ${index === 0 ? 'disabled' : ''} aria-label="Move up" title="Move up"><i class="fas fa-arrow-up" aria-hidden="true"></i></button>
                        <button type="button" class="ai-models-config-move-down-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" ${index === sorted.length - 1 ? 'disabled' : ''} aria-label="Move down" title="Move down"><i class="fas fa-arrow-down" aria-hidden="true"></i></button>`;
            const editDeleteButtons = isLocal ? '' : `
                        <button type="button" class="ai-models-config-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit model" title="Edit"><i class="fas fa-edit" aria-hidden="true"></i></button>
                        <button type="button" class="ai-models-config-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete model" title="Delete"><i class="fas fa-trash-alt" aria-hidden="true"></i></button>`;
            const actionsCell = `<span class="manage-contacts-actions-cell">${moveButtons}${editDeleteButtons}</span>`;
            return `
            <tr class="ai-models-config-row${isLocal ? ' ai-models-config-row--local' : ''}" data-id="${row.id}" data-model-key="${escapeHtml(row.key)}" style="--ai-model-row-bg:${rowBg};">
                <td><code>${escapeHtml(row.key)}</code></td>
                <td>${escapeHtml(row.display_name)}</td>
                <td><code>${escapeHtml(modelSlugDisplay)}</code></td>
                <td style="text-align:center;">
                    <input type="checkbox" class="ai-models-config-enabled-checkbox" data-id="${row.id}" ${row.enabled ? 'checked' : ''} aria-label="Enabled"${isLocal ? ' title="Also requires Local AI to be enabled and reachable in Local AI Setup"' : ''}>
                </td>
                <td style="text-align:center;">
                    <input type="radio" name="ai-models-classifier-radio" class="ai-models-config-classifier-radio" value="${escapeHtml(row.key)}" ${classifierProvider === row.key ? 'checked' : ''} aria-label="Use ${escapeHtml(row.display_name)} as the Auto-routing classifier">
                </td>
                <td>${actionsCell}</td>
            </tr>
        `;
        }).join('');
        bindTableButtons();
    }

    async function load() {
        const loading = getEl('ai-models-config-loading');
        const tableWrap = getEl('ai-models-config-table-wrap');
        const emptyMsg = getEl('ai-models-config-empty');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';
        showStatus('', false);

        try {
            const [modelsResponse, classifierConfig] = await Promise.all([
                fetch('/api/ai-models', { credentials: 'same-origin' }),
                fetchClassifierConfig(),
            ]);
            if (!modelsResponse.ok) throw new Error(`HTTP ${modelsResponse.status}`);
            const data = await modelsResponse.json();
            rows = Array.isArray(data.models) ? data.models : [];
            classifierProvider = classifierConfig.classifier_provider;
            const failoverCheckbox = getEl('ai-models-config-failover-enabled-checkbox');
            if (failoverCheckbox) failoverCheckbox.checked = classifierConfig.failover_enabled;
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load AI models: ${err.message}`, true);
        }
    }

    function openCreateModal(prefill) {
        editingId = null;
        const modal = getEl('ai-models-config-edit-modal');
        const title = getEl('ai-models-config-edit-title');
        const key = getEl('ai-models-config-edit-key');
        const name = getEl('ai-models-config-edit-display-name');
        const slug = getEl('ai-models-config-edit-model-slug');
        const enabled = getEl('ai-models-config-edit-enabled');
        const errEl = getEl('ai-models-config-edit-error');
        if (!modal || !key || !name || !slug) return;
        if (title) title.textContent = 'Add AI Model';
        key.value = (prefill && prefill.key) || '';
        key.readOnly = false;
        name.value = (prefill && prefill.display_name) || '';
        slug.value = (prefill && prefill.model_slug) || '';
        if (enabled) enabled.checked = true;
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function openEditModal(id) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        editingId = id;
        const modal = getEl('ai-models-config-edit-modal');
        const title = getEl('ai-models-config-edit-title');
        const key = getEl('ai-models-config-edit-key');
        const name = getEl('ai-models-config-edit-display-name');
        const slug = getEl('ai-models-config-edit-model-slug');
        const enabled = getEl('ai-models-config-edit-enabled');
        const errEl = getEl('ai-models-config-edit-error');
        if (!modal || !key || !name || !slug) return;
        if (title) title.textContent = 'Edit AI Model';
        key.value = row.key || '';
        key.readOnly = true;
        name.value = row.display_name || '';
        slug.value = row.model_slug || '';
        if (enabled) enabled.checked = !!row.enabled;
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    function closeEditModal() {
        const modal = getEl('ai-models-config-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveEditModal() {
        const errEl = getEl('ai-models-config-edit-error');
        const key = (getEl('ai-models-config-edit-key') || {}).value || '';
        const name = (getEl('ai-models-config-edit-display-name') || {}).value || '';
        const slug = (getEl('ai-models-config-edit-model-slug') || {}).value || '';
        const enabled = !!(getEl('ai-models-config-edit-enabled') || {}).checked;
        const body = {
            key: key.trim().toLowerCase(),
            display_name: name.trim(),
            model_slug: slug.trim(),
            enabled,
            sort_order: editingId
                ? (rows.find((r) => r.id === editingId) || {}).sort_order || 0
                : rows.length,
        };
        if (!body.key || !body.display_name || !body.model_slug) {
            if (errEl) {
                errEl.textContent = 'Key, display name, and OpenRouter model slug are required.';
                errEl.style.display = 'block';
            }
            return;
        }
        try {
            const url = editingId ? `/api/ai-models/${editingId}` : '/api/ai-models';
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
            showStatus(editingId ? 'Model updated.' : 'Model added.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            if (errEl) {
                errEl.textContent = err.message;
                errEl.style.display = 'block';
            }
        }
    }

    async function deleteModel(id) {
        const row = rows.find((r) => r.id === id);
        const name = row ? (row.display_name || row.key) : 'this model';
        if (!window.confirm(`Delete AI model "${name}"?`)) return;
        try {
            const response = await fetch(`/api/ai-models/${id}`, { method: 'DELETE', credentials: 'same-origin' });
            if (!response.ok) {
                const err = await response.json().catch(() => ({}));
                throw new Error(err.error || err.detail || `HTTP ${response.status}`);
            }
            showStatus('Model deleted.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    async function patchModel(row, overrides) {
        const body = {
            key: row.key,
            display_name: row.display_name,
            model_slug: row.model_slug,
            enabled: row.enabled,
            sort_order: row.sort_order,
            ...overrides,
        };
        const response = await fetch(`/api/ai-models/${row.id}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify(body),
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || err.detail || `HTTP ${response.status}`);
        }
    }

    async function toggleEnabled(id, enabled) {
        const row = rows.find((r) => r.id === id);
        if (!row) return;
        try {
            await patchModel(row, { enabled });
            showStatus(enabled ? 'Model enabled.' : 'Model disabled.', false);
            await load();
            invalidateCaches();
        } catch (err) {
            showStatus(err.message, true);
            await load();
        }
    }

    async function moveModel(id, direction) {
        const sorted = rows.slice().sort((a, b) => {
            if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order;
            return a.id - b.id;
        });
        const index = sorted.findIndex((r) => r.id === id);
        const swapIndex = index + direction;
        if (index < 0 || swapIndex < 0 || swapIndex >= sorted.length) return;
        const a = sorted[index];
        const b = sorted[swapIndex];
        try {
            await patchModel(a, { sort_order: b.sort_order });
            await patchModel(b, { sort_order: a.sort_order });
            await load();
            invalidateCaches();
        } catch (err) {
            showStatus(err.message, true);
        }
    }

    function openModelCatalogTab() {
        const tabBtn = document.querySelector('.config-tab-button[data-tab="model-catalog"]');
        if (tabBtn) tabBtn.click();
    }

    // The classifier radio buttons and the "Enable error failover" checkbox both live on
    // this table, but they're two independent fields of the SAME 'hosted_llm_provider_order_v1'
    // app_configuration row (also read by Modals.AutoRoutingConfig for chat failover). Every
    // write here re-reads the row first so one field is never clobbered while saving the other.
    const CLASSIFIER_CONFIG_KEY = 'hosted_llm_provider_order_v1';

    async function fetchClassifierConfig() {
        try {
            const response = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (response.ok) {
                const rowsResp = await response.json();
                const row = Array.isArray(rowsResp) ? rowsResp.find((r) => r && r.key === CLASSIFIER_CONFIG_KEY) : null;
                if (row && row.value != null && String(row.value).trim() !== '') {
                    const parsed = JSON.parse(row.value);
                    return {
                        classifier_provider: parsed.classifier_provider || 'localai',
                        failover_enabled: parsed.failover_enabled !== false,
                    };
                }
            }
        } catch (_) { /* fall through to default */ }
        return { classifier_provider: 'localai', failover_enabled: true };
    }

    async function saveClassifierConfig(overrides) {
        const current = await fetchClassifierConfig();
        const payload = { ...current, ...overrides };
        const res = await fetch('/api/configuration', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                key: CLASSIFIER_CONFIG_KEY,
                value: JSON.stringify(payload),
                description: 'Auto routing classifier provider, plus the error failover on/off toggle (provider order follows AI Models sort_order)',
            }),
        });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            throw new Error(err.error || err.detail || `HTTP ${res.status}`);
        }
        if (typeof Modals !== 'undefined' && Modals.AutoRoutingConfig && Modals.AutoRoutingConfig.ensureLoaded) {
            await Modals.AutoRoutingConfig.ensureLoaded();
        }
        return payload;
    }

    /** Called by Modals.AutoRoutingConfig.reconcileClassifierProvider() when the classifier
     *  choice changes elsewhere (e.g. Local AI gets disabled) — updates the radio selection
     *  if this table is currently rendered. */
    function reflectClassifierProvider(key) {
        classifierProvider = key;
        const tbody = getEl('ai-models-config-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.ai-models-config-classifier-radio').forEach((radio) => {
            radio.checked = radio.value === key;
        });
    }

    function init() {
        const addBtn = getEl('ai-models-config-add-btn');
        if (addBtn) addBtn.addEventListener('click', openModelCatalogTab);

        const editSave = getEl('ai-models-config-edit-save');
        if (editSave) editSave.addEventListener('click', () => { void saveEditModal(); });
        const editCancel = getEl('ai-models-config-edit-cancel');
        if (editCancel) editCancel.addEventListener('click', closeEditModal);
        const editClose = getEl('ai-models-config-edit-close');
        if (editClose) editClose.addEventListener('click', closeEditModal);

        const failoverCheckbox = getEl('ai-models-config-failover-enabled-checkbox');
        if (failoverCheckbox) {
            failoverCheckbox.addEventListener('change', () => {
                const enabled = failoverCheckbox.checked;
                saveClassifierConfig({ failover_enabled: enabled }).catch((err) => {
                    failoverCheckbox.checked = !enabled;
                    showStatus(err.message, true);
                });
            });
        }
    }

    return {
        init,
        load,
        openCreateModal,
        reflectClassifierProvider,
    };
})();

Modals.AIModelsConfig.init();
