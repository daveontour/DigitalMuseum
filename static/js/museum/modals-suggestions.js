'use strict';

Modals.Suggestions = (() => {
    const PROMPT_LONG_THRESHOLD = 500;

    let cachedPayload = null;

    function escapeHtml(s) {
        if (s == null) return '';
        const d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }

    function isDeploymentLocal() {
        return typeof CONSTANTS !== 'undefined' && CONSTANTS.DEPLOYMENT_NATURE_LOCAL === 'True';
    }

    function suggestionPassesRequires(sugg) {
        const req = sugg.requires;
        if (!Array.isArray(req)) return true;
        if (req.indexOf('local_only') >= 0 && !isDeploymentLocal()) return false;
        return true;
    }

    function suggestionPassesMature(sugg, showMature) {
        if (sugg.sensitivity === 'mature' && !showMature) return false;
        return true;
    }

    function filterSuggestionsForCategory(cat, showMature) {
        if (!Array.isArray(cat.suggestions)) return [];
        return cat.suggestions.filter((s) => suggestionPassesRequires(s) && suggestionPassesMature(s, showMature));
    }

    function defaultActionLabel(sugg) {
        if (sugg.action_label && String(sugg.action_label).trim()) return String(sugg.action_label).trim();
        if (sugg.function) return 'Open';
        return 'Ask in chat';
    }

    function runChatSuggestion(sugg, cat) {
        Modals._closeModal(DOM.suggestionsModal);
        if (sugg.function) {
            if (typeof AppActions === 'undefined' || typeof AppActions[sugg.function] !== 'function') {
                if (typeof UI !== 'undefined' && UI.displayError) {
                    UI.displayError('This suggestion is no longer available (missing action: ' + String(sugg.function) + ').');
                }
                return;
            }
            AppActions[sugg.function]();
            return;
        }
        const prompt = (sugg.prompt || '').trim();
        if (prompt) {
            if (typeof App !== 'undefined' && App.processFormSubmit) {
                App.processFormSubmit(prompt, cat.category, sugg.title);
            }
            return;
        }
        if (typeof UI !== 'undefined' && UI.displayError) {
            UI.displayError('This suggestion has no runnable action.');
        }
    }

    function maybeConfirmAndRun(sugg, cat) {
        const hasFn = !!(sugg.function && typeof AppActions !== 'undefined' && typeof AppActions[sugg.function] === 'function');
        const prompt = (sugg.prompt || '').trim();
        const long = prompt.length > PROMPT_LONG_THRESHOLD;
        const mature = sugg.sensitivity === 'mature';
        if (hasFn && !mature) {
            runChatSuggestion(sugg, cat);
            return;
        }
        if (!hasFn && !long && !mature) {
            runChatSuggestion(sugg, cat);
            return;
        }
        if (typeof Modals !== 'undefined' && Modals.ConfirmationModal && Modals.ConfirmationModal.open) {
            const bits = [];
            if (long) bits.push('This sends a long prompt to the chat.');
            if (mature) bits.push('It is marked as a mature or long-form example.');
            Modals.ConfirmationModal.open('Run this suggestion?', bits.join(' '), () => runChatSuggestion(sugg, cat));
        } else {
            runChatSuggestion(sugg, cat);
        }
    }

    function setLoadingState() {
        if (!DOM.suggestionsListContainer) return;
        DOM.suggestionsListContainer.innerHTML = '';
        const wrap = document.createElement('div');
        wrap.className = 'suggestions-state-msg';
        wrap.innerHTML = '<i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Loading suggestions…';
        DOM.suggestionsListContainer.appendChild(wrap);
    }

    function setEmptyOrError(msg, isError) {
        if (!DOM.suggestionsListContainer) return;
        DOM.suggestionsListContainer.innerHTML = '';
        const wrap = document.createElement('div');
        wrap.className = 'suggestions-state-msg' + (isError ? ' suggestions-state-msg--error' : '');
        wrap.textContent = msg;
        DOM.suggestionsListContainer.appendChild(wrap);
    }

    function buildSuggestionCard(sugg, cat) {
        const card = document.createElement('article');
        card.className = 'suggestions-item-card';

        const promptText = (sugg.prompt || '').trim();
        const showPreview = promptText.length > 0 && promptText !== '-Initiates LocalProcessing-';

        // Header row: title left, buttons right
        const header = document.createElement('div');
        header.className = 'suggestions-item-header';

        const titleEl = document.createElement('h3');
        titleEl.className = 'suggestions-item-title';
        titleEl.textContent = sugg.title || 'Untitled';
        header.appendChild(titleEl);

        const actions = document.createElement('div');
        actions.className = 'suggestions-item-actions';

        let toggle = null;
        if (showPreview) {
            toggle = document.createElement('button');
            toggle.type = 'button';
            toggle.className = 'suggestions-preview-toggle';
            toggle.textContent = 'Show prompt';
            toggle.addEventListener('click', (e) => {
                e.stopPropagation();
                const open = card.classList.toggle('suggestions-item-card--preview-open');
                toggle.textContent = open ? 'Hide prompt' : 'Show prompt';
            });
            actions.appendChild(toggle);
        }

        const runBtn = document.createElement('button');
        runBtn.type = 'button';
        runBtn.className = 'modal-btn suggestions-run-btn';
        runBtn.textContent = defaultActionLabel(sugg);
        runBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            maybeConfirmAndRun(sugg, cat);
        });
        actions.appendChild(runBtn);

        header.appendChild(actions);
        card.appendChild(header);

        if (sugg.description && String(sugg.description).trim()) {
            const desc = document.createElement('p');
            desc.className = 'suggestions-item-desc';
            desc.textContent = String(sugg.description).trim();
            card.appendChild(desc);
        }

        if (showPreview) {
            const previewWrap = document.createElement('div');
            previewWrap.className = 'suggestions-preview-wrap';
            const body = document.createElement('div');
            body.className = 'suggestions-preview-body';
            body.textContent = promptText;
            previewWrap.appendChild(body);
            card.appendChild(previewWrap);
        }

        return card;
    }

    function buildCategoryBlock(cat, filteredSuggestions, startCollapsed) {
        const block = document.createElement('section');
        block.className = 'suggestions-category-block';
        if (startCollapsed) block.classList.add('is-collapsed');

        const head = document.createElement('button');
        head.type = 'button';
        head.className = 'suggestions-category-head';
        head.innerHTML = `
            <i class="fas fa-chevron-down suggestions-category-chevron" aria-hidden="true"></i>
            <span class="suggestions-category-title-text">${escapeHtml(cat.category)}</span>
            <span class="suggestions-category-count">(${filteredSuggestions.length})</span>
        `;
        head.addEventListener('click', () => {
            block.classList.toggle('is-collapsed');
        });

        const body = document.createElement('div');
        body.className = 'suggestions-category-body';
        filteredSuggestions.forEach((sugg) => {
            body.appendChild(buildSuggestionCard(sugg, cat));
        });

        block.appendChild(head);
        block.appendChild(body);
        return block;
    }

    function renderList(data) {
        if (!DOM.suggestionsListContainer) return;
        DOM.suggestionsListContainer.innerHTML = '';
        const showMature = !!(DOM.suggestionsShowMatureCheckbox && DOM.suggestionsShowMatureCheckbox.checked);

        if (!data || !Array.isArray(data.categories)) {
            setEmptyOrError('No suggestions found.', false);
            return;
        }

        let any = false;
        let firstRendered = true;
        data.categories.forEach((cat) => {
            if (!cat || typeof cat !== 'object' || !cat.category) return;
            const filtered = filterSuggestionsForCategory(cat, showMature);
            if (filtered.length === 0) return;
            any = true;
            const startCollapsed = !firstRendered;
            firstRendered = false;
            DOM.suggestionsListContainer.appendChild(buildCategoryBlock(cat, filtered, startCollapsed));
        });

        if (!any) {
            setEmptyOrError('No suggestions match the current filters. Enable “Show mature / long-form examples” or check deployment (some items need local desktop mode).', false);
        }
    }

    function expandAll() {
        if (!DOM.suggestionsListContainer) return;
        DOM.suggestionsListContainer.querySelectorAll('.suggestions-category-block.is-collapsed').forEach((el) => {
            el.classList.remove('is-collapsed');
        });
    }

    function collapseAll() {
        if (!DOM.suggestionsListContainer) return;
        DOM.suggestionsListContainer.querySelectorAll('.suggestions-category-block').forEach((el) => {
            el.classList.add('is-collapsed');
        });
    }

    function open() {
        Modals._openModal(DOM.suggestionsModal);
        setLoadingState();
        ApiService.fetchSuggestionsConfig()
            .then((data) => {
                cachedPayload = data;
                renderList(cachedPayload);
            })
            .catch((err) => {
                console.error('Failed to load suggestions:', err);
                cachedPayload = null;
                setEmptyOrError('Failed to load suggestions.', true);
                if (typeof UI !== 'undefined' && UI.displayError) {
                    UI.displayError('Could not load suggestions: ' + (err && err.message ? err.message : String(err)));
                }
            });
    }

    function close() {
        Modals._closeModal(DOM.suggestionsModal);
    }

    function init() {
        if (DOM.closeSuggestionsModalBtn) {
            DOM.closeSuggestionsModalBtn.addEventListener('click', close);
        }
        if (DOM.suggestionsModal) {
            DOM.suggestionsModal.addEventListener('click', (e) => {
                if (e.target === DOM.suggestionsModal) close();
            });
        }
        const expandAllBtn = document.getElementById('suggestions-expand-all-btn');
        const collapseAllBtn = document.getElementById('suggestions-collapse-all-btn');
        if (expandAllBtn) expandAllBtn.addEventListener('click', expandAll);
        if (collapseAllBtn) collapseAllBtn.addEventListener('click', collapseAll);
        if (DOM.suggestionsShowMatureCheckbox) {
            DOM.suggestionsShowMatureCheckbox.addEventListener('change', () => {
                if (cachedPayload) renderList(cachedPayload);
            });
        }
    }

    function invalidateCache() {
        cachedPayload = null;
    }

    return { init, open, close, invalidateCache };
})();
