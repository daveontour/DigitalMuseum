'use strict';

// Configuration → Model Catalog tab: browse OpenRouter's full public model
// catalog (GET /api/ai-models/catalog), search/filter/sort it client-side,
// and hand a picked model off to Modals.AIModelsConfig's create modal so it
// can be added to the app's admin-curated ai_models list.
Modals.ModelCatalog = (() => {
    let allModels = [];
    let loaded = false;
    let detailModel = null;

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
        const el = getEl('model-catalog-status');
        if (!el) return;
        el.textContent = message || '';
        el.style.color = isError ? 'var(--color-danger)' : 'var(--color-text-muted)';
    }

    // Prices arrive as decimal-string USD-per-token, e.g. "0.000005".
    function perMillion(priceStr) {
        const n = parseFloat(priceStr);
        if (!isFinite(n)) return null;
        return n * 1000000;
    }

    function formatPrice(priceStr) {
        const perM = perMillion(priceStr);
        if (perM === null) return '—';
        if (perM === 0) return 'Free';
        if (perM < 0.01) return `$${perM.toFixed(4)}`;
        return `$${perM.toFixed(2)}`;
    }

    function formatContextLength(n) {
        if (!n && n !== 0) return '—';
        if (n >= 1000000) return `${(n / 1000000).toFixed(n % 1000000 === 0 ? 0 : 1)}M`;
        if (n >= 1000) return `${(n / 1000).toFixed(n % 1000 === 0 ? 0 : 1)}K`;
        return String(n);
    }

    function isFree(m) {
        const p = parseFloat((m.pricing && m.pricing.prompt) || '0');
        const c = parseFloat((m.pricing && m.pricing.completion) || '0');
        return (!isFinite(p) || p === 0) && (!isFinite(c) || c === 0);
    }

    function matchesModalityFilter(m, filter) {
        if (!filter) return true;
        const inputs = (m.input_modalities || []);
        if (filter === 'text') return inputs.length <= 1 && inputs.every((x) => x === 'text');
        return inputs.includes(filter);
    }

    function matchesSearch(m, term) {
        if (!term) return true;
        const haystack = `${m.id} ${m.name} ${m.description || ''}`.toLowerCase();
        return haystack.includes(term);
    }

    function sortModels(models, sortKey) {
        const out = models.slice();
        switch (sortKey) {
            case 'context-desc':
                out.sort((a, b) => (b.context_length || 0) - (a.context_length || 0));
                break;
            case 'price-asc':
                out.sort((a, b) => (perMillion(a.pricing && a.pricing.prompt) || 0) - (perMillion(b.pricing && b.pricing.prompt) || 0));
                break;
            case 'price-desc':
                out.sort((a, b) => (perMillion(b.pricing && b.pricing.prompt) || 0) - (perMillion(a.pricing && a.pricing.prompt) || 0));
                break;
            default:
                out.sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));
        }
        return out;
    }

    function currentFiltered() {
        const term = ((getEl('model-catalog-search') || {}).value || '').trim().toLowerCase();
        const modality = (getEl('model-catalog-modality-filter') || {}).value || '';
        const freeOnly = !!(getEl('model-catalog-free-only') || {}).checked;
        const sortKey = (getEl('model-catalog-sort') || {}).value || 'name';

        let out = allModels.filter((m) => matchesSearch(m, term) && matchesModalityFilter(m, modality));
        if (freeOnly) out = out.filter(isFree);
        return sortModels(out, sortKey);
    }

    function bindRowButtons() {
        const tbody = getEl('model-catalog-tbody');
        if (!tbody) return;
        tbody.querySelectorAll('.model-catalog-details-btn').forEach((btn) => {
            btn.addEventListener('click', () => openDetail(btn.dataset.id));
        });
        tbody.querySelectorAll('.model-catalog-use-btn').forEach((btn) => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                useModel(btn.dataset.id);
            });
        });
        tbody.querySelectorAll('tr[data-id]').forEach((row) => {
            row.addEventListener('click', () => openDetail(row.dataset.id));
        });
    }

    function renderTable() {
        const tbody = getEl('model-catalog-tbody');
        const loading = getEl('model-catalog-loading');
        const tableWrap = getEl('model-catalog-table-wrap');
        const emptyMsg = getEl('model-catalog-empty');
        const countEl = getEl('model-catalog-count');
        if (!tbody) return;

        if (loading) loading.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';

        const filtered = currentFiltered();
        if (countEl) countEl.textContent = `${filtered.length} of ${allModels.length} models`;

        if (filtered.length === 0) {
            tbody.innerHTML = '';
            if (emptyMsg) emptyMsg.style.display = 'block';
            return;
        }
        if (emptyMsg) emptyMsg.style.display = 'none';

        tbody.innerHTML = filtered.map((m) => `
            <tr data-id="${escapeHtml(m.id)}" style="cursor:pointer;">
                <td>
                    <div style="font-weight:600;">${escapeHtml(m.name)}</div>
                    <div style="color:var(--color-text-muted); font-size:var(--text-sm);"><code>${escapeHtml(m.id)}</code></div>
                </td>
                <td>${formatPrice(m.pricing && m.pricing.prompt)} / ${formatPrice(m.pricing && m.pricing.completion)}<div style="color:var(--color-text-muted); font-size:var(--text-sm);">per 1M tokens</div></td>
                <td>${formatContextLength(m.context_length)}</td>
                <td>
                    <span class="manage-contacts-actions-cell">
                        <button type="button" class="model-catalog-details-btn modal-btn modal-btn-secondary" data-id="${escapeHtml(m.id)}">Details</button>
                        <button type="button" class="model-catalog-use-btn modal-btn modal-btn-primary" data-id="${escapeHtml(m.id)}"><i class="fas fa-plus" aria-hidden="true"></i> Use</button>
                    </span>
                </td>
            </tr>
        `).join('');
        bindRowButtons();
    }

    async function load(forceRefresh) {
        if (loaded && !forceRefresh) {
            renderTable();
            return;
        }
        const loading = getEl('model-catalog-loading');
        const tableWrap = getEl('model-catalog-table-wrap');
        if (loading) loading.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        showStatus('', false);

        try {
            const response = await fetch('/api/ai-models/catalog', { credentials: 'same-origin' });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            allModels = Array.isArray(data.models) ? data.models : [];
            loaded = true;
            renderTable();
        } catch (err) {
            if (loading) loading.style.display = 'none';
            showStatus(`Failed to load OpenRouter model catalog: ${err.message}`, true);
        }
    }

    function detailRow(label, value) {
        if (value === null || value === undefined || value === '') return '';
        return `<div style="margin-bottom:0.5rem;"><strong>${escapeHtml(label)}:</strong> ${value}</div>`;
    }

    function renderDetailBody(m) {
        const created = m.created ? new Date(m.created * 1000).toLocaleDateString() : null;
        const pricing = m.pricing || {};
        const pricingLines = [
            ['Prompt', pricing.prompt],
            ['Completion', pricing.completion],
            ['Request', pricing.request],
            ['Image', pricing.image],
            ['Web search', pricing.web_search],
            ['Cached input (read)', pricing.input_cache_read],
            ['Cached input (write)', pricing.input_cache_write],
        ].filter(([, v]) => v && parseFloat(v) !== 0)
            .map(([label, v]) => `${escapeHtml(label)}: ${formatPrice(v)} per 1M`)
            .join(' &nbsp;·&nbsp; ');

        return `
            ${detailRow('Model ID', `<code>${escapeHtml(m.id)}</code>`)}
            ${detailRow('Canonical slug', m.canonical_slug && m.canonical_slug !== m.id ? `<code>${escapeHtml(m.canonical_slug)}</code>` : '')}
            ${detailRow('Description', escapeHtml(m.description))}
            ${detailRow('Context length', formatContextLength(m.context_length))}
            ${detailRow('Max completion tokens', m.max_completion_tokens ? formatContextLength(m.max_completion_tokens) : '')}
            ${detailRow('Moderated', m.is_moderated ? 'Yes' : 'No')}
            ${detailRow('Modality', escapeHtml(m.modality))}
            ${detailRow('Input modalities', (m.input_modalities || []).map(escapeHtml).join(', '))}
            ${detailRow('Output modalities', (m.output_modalities || []).map(escapeHtml).join(', '))}
            ${detailRow('Tokenizer', escapeHtml(m.tokenizer))}
            ${detailRow('Instruct type', m.instruct_type ? escapeHtml(m.instruct_type) : '')}
            ${detailRow('Pricing', pricingLines || 'Free')}
            ${detailRow('Supported parameters', (m.supported_parameters || []).map(escapeHtml).join(', '))}
            ${detailRow('Added to OpenRouter', created)}
        `;
    }

    function openDetail(id) {
        const m = allModels.find((mm) => mm.id === id);
        if (!m) return;
        detailModel = m;
        const modal = getEl('model-catalog-detail-modal');
        const title = getEl('model-catalog-detail-title');
        const body = getEl('model-catalog-detail-body');
        if (!modal || !body) return;
        if (title) title.textContent = m.name;
        body.innerHTML = renderDetailBody(m);
        modal.style.display = 'flex';
    }

    function closeDetail() {
        const modal = getEl('model-catalog-detail-modal');
        if (modal) modal.style.display = 'none';
        detailModel = null;
    }

    function suggestKey(m) {
        const tail = (m.id.split('/').pop() || m.id).toLowerCase();
        return tail.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40) || 'model';
    }

    function useModel(id) {
        const m = allModels.find((mm) => mm.id === id) || detailModel;
        if (!m) return;
        closeDetail();
        const tabBtn = document.querySelector('.config-tab-button[data-tab="ai-models-config"]');
        if (tabBtn) tabBtn.click();
        if (Modals.AIModelsConfig && Modals.AIModelsConfig.openCreateModal) {
            Modals.AIModelsConfig.openCreateModal({
                key: suggestKey(m),
                display_name: m.name,
                model_slug: m.id,
            });
        }
    }

    function init() {
        const search = getEl('model-catalog-search');
        if (search) search.addEventListener('input', renderTable);
        const modalityFilter = getEl('model-catalog-modality-filter');
        if (modalityFilter) modalityFilter.addEventListener('change', renderTable);
        const freeOnly = getEl('model-catalog-free-only');
        if (freeOnly) freeOnly.addEventListener('change', renderTable);
        const sortSelect = getEl('model-catalog-sort');
        if (sortSelect) sortSelect.addEventListener('change', renderTable);
        const refreshBtn = getEl('model-catalog-refresh-btn');
        if (refreshBtn) refreshBtn.addEventListener('click', () => { void load(true); });

        const detailClose = getEl('model-catalog-detail-close');
        if (detailClose) detailClose.addEventListener('click', closeDetail);
        const detailCloseBtn = getEl('model-catalog-detail-close-btn');
        if (detailCloseBtn) detailCloseBtn.addEventListener('click', closeDetail);
        const detailUseBtn = getEl('model-catalog-detail-use-btn');
        if (detailUseBtn) detailUseBtn.addEventListener('click', () => {
            if (detailModel) useModel(detailModel.id);
        });
    }

    return { init, load };
})();

Modals.ModelCatalog.init();
