'use strict';

/** Local AI / Ollama setup UI — Configuration → Local AI Setup and login advanced panel. */
const LocalAiSetup = (() => {
    const CONFIG_KEY = 'local_ai_use_enabled_v1';
    const STATUS_URL = '/api/local-ai/status';
    const SETTINGS_URL = '/api/local-ai/settings';
    const MODELS_URL = '/api/local-ai/models';
    let inited = false;
    let useEnabledCached = true;
    let saveInFlight = false;
    let applyInFlight = false;
    let lastServerStatus = null;
    let savedModelSettings = { chat_model: '', cuda_cpu_only: false };
    let settingsControlsWired = false;

    function hasElectron() {
        return !!(window.electronAPI && typeof window.electronAPI.startOllama === 'function');
    }

    function configRoot() {
        return document.getElementById('ai-setup-config-root');
    }

    function q(sel, rootEl) {
        const r = rootEl || configRoot();
        return r ? r.querySelector(sel) : null;
    }

    function loginPanel() {
        return {
            kind: 'login',
            root: document.getElementById('adv-ollama-block'),
            noElectronMsg: document.getElementById('adv-no-electron-msg'),
            serverValueEl: document.getElementById('adv-ollama-server-value'),
            embedServerValueEl: document.getElementById('adv-ollama-embed-server-value'),
            chatValueEl: document.getElementById('adv-model-gemma-value'),
            embedValueEl: document.getElementById('adv-model-embed-value'),
            chatLabelEl: document.querySelector('#adv-local-ai-rows .adv-status-row:nth-child(3) .adv-status-label'),
            embedLabelEl: document.querySelector('#adv-local-ai-rows .adv-status-row:nth-child(4) .adv-status-label'),
            downloadBtn: document.getElementById('adv-download-models-btn'),
            msgEl: document.getElementById('adv-ollama-msg'),
            progressWrap: document.getElementById('adv-ollama-progress-bar-wrap'),
            progressBar: document.getElementById('adv-ollama-progress-bar'),
            progressStatus: document.getElementById('adv-ollama-progress-status'),
            progressSize: document.getElementById('adv-ollama-progress-size'),
        };
    }

    function configPanel() {
        const root = configRoot();
        return {
            kind: 'config',
            root,
            noElectronMsg: q('[data-local-ai-no-electron]', root),
            serverValueEl: q('[data-local-ai-server-value]', root),
            embedServerValueEl: q('[data-local-ai-embed-server-value]', root),
            chatValueEl: q('[data-local-ai-gemma-value]', root),
            embedValueEl: q('[data-local-ai-embed-value]', root),
            chatLabelEl: q('[data-local-ai-chat-label]', root),
            embedLabelEl: q('[data-local-ai-embed-label]', root),
            downloadBtn: q('[data-local-ai-download]', root),
            msgEl: q('[data-local-ai-msg]', root),
            progressWrap: q('[data-local-ai-progress-wrap]', root),
            progressBar: q('[data-local-ai-progress-bar]', root),
            progressStatus: q('[data-local-ai-progress-status]', root),
            progressSize: q('[data-local-ai-progress-size]', root),
        };
    }

    function setPanelMsg(panel, text, color) {
        const el = panel && panel.msgEl;
        if (!el) return;
        el.textContent = text || '';
        if (color) el.style.color = color;
        el.style.display = text ? 'block' : 'none';
    }

    function setApplyStatus(text, color) {
        const el = document.getElementById('local-ai-settings-apply-status');
        if (!el) return;
        el.textContent = text || '';
        el.style.color = color || 'var(--color-text-muted)';
    }

    function modelSelectEl() {
        return document.getElementById('local-ai-chat-model-select');
    }

    function cudaCheckboxEl() {
        return document.getElementById('local-ai-cuda-cpu-only-checkbox');
    }

    function applyBtnEl() {
        return document.getElementById('local-ai-settings-apply-btn');
    }

    function currentDraftSettings() {
        const select = modelSelectEl();
        const cuda = cudaCheckboxEl();
        return {
            chat_model: select ? String(select.value || '').trim() : '',
            cuda_cpu_only: !!(cuda && cuda.checked),
        };
    }

    function settingsDirty() {
        const draft = currentDraftSettings();
        return draft.chat_model !== savedModelSettings.chat_model
            || draft.cuda_cpu_only !== savedModelSettings.cuda_cpu_only;
    }

    function updateApplyButtonState() {
        const btn = applyBtnEl();
        if (!btn) return;
        const dirty = settingsDirty();
        btn.disabled = applyInFlight || !dirty || !currentDraftSettings().chat_model;
    }

    function populateModelSelect(models, selectedModel) {
        const select = modelSelectEl();
        if (!select) return;
        const names = Array.isArray(models) ? models.slice() : [];
        const want = String(selectedModel || '').trim();
        if (want && !names.includes(want)) {
            names.unshift(want);
        }
        select.innerHTML = '';
        if (names.length === 0) {
            const opt = document.createElement('option');
            opt.value = want || '';
            opt.textContent = want ? `${want} (not reported by Ollama)` : 'No models available';
            select.appendChild(opt);
            return;
        }
        names.forEach((name) => {
            const opt = document.createElement('option');
            opt.value = name;
            opt.textContent = name;
            select.appendChild(opt);
        });
        if (want) select.value = want;
        else if (names.length) select.value = names[0];
    }

    async function fetchModelSettings() {
        const res = await fetch(SETTINGS_URL, { credentials: 'same-origin' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
    }

    async function fetchOllamaModels() {
        const res = await fetch(MODELS_URL, { credentials: 'same-origin' });
        if (!res.ok) {
            let detail = await res.text();
            try {
                const j = JSON.parse(detail);
                if (j.detail) detail = j.detail;
            } catch (_) { /* keep */ }
            throw new Error(detail || `HTTP ${res.status}`);
        }
        const data = await res.json();
        return Array.isArray(data.models) ? data.models : [];
    }

    function syncModelSettingsToControls(settings) {
        savedModelSettings = {
            chat_model: String(settings.chat_model || '').trim(),
            cuda_cpu_only: !!settings.cuda_cpu_only,
        };
        const cuda = cudaCheckboxEl();
        if (cuda) cuda.checked = savedModelSettings.cuda_cpu_only;
        updateApplyButtonState();
    }

    async function loadModelSettingsAndModels() {
        const select = modelSelectEl();
        if (!select) return;
        setApplyStatus('');
        if (hasElectron() && window.electronAPI.startOllama) {
            try {
                await window.electronAPI.startOllama();
            } catch (_) { /* model list may still fail */ }
        }
        try {
            const [settings, models] = await Promise.all([
                fetchModelSettings(),
                fetchOllamaModels().catch(() => []),
            ]);
            syncModelSettingsToControls(settings);
            populateModelSelect(models, savedModelSettings.chat_model);
            const cuda = cudaCheckboxEl();
            if (cuda) cuda.checked = savedModelSettings.cuda_cpu_only;
            updateApplyButtonState();
        } catch (e) {
            setApplyStatus(e.message || 'Could not load Local AI settings', 'var(--color-danger)');
            try {
                const settings = await fetchModelSettings();
                syncModelSettingsToControls(settings);
                populateModelSelect([], savedModelSettings.chat_model);
            } catch (_) { /* ignore */ }
        }
    }

    async function restartOllamaAfterApply() {
        if (hasElectron() && window.electronAPI.restartOllama) {
            const res = await window.electronAPI.restartOllama();
            if (!res || !res.ok) {
                throw new Error((res && res.error) || 'Ollama restart failed');
            }
            return 'electron';
        }
        return 'manual';
    }

    async function applyModelSettings() {
        if (applyInFlight || !settingsDirty()) return;
        const draft = currentDraftSettings();
        if (!draft.chat_model) {
            setApplyStatus('Select a chat model.', 'var(--color-danger)');
            return;
        }

        applyInFlight = true;
        const btn = applyBtnEl();
        const select = modelSelectEl();
        const cuda = cudaCheckboxEl();
        if (btn) btn.disabled = true;
        if (select) select.disabled = true;
        if (cuda) cuda.disabled = true;
        setApplyStatus('Saving…');

        try {
            const res = await fetch(SETTINGS_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    chat_model: draft.chat_model,
                    cuda_cpu_only: draft.cuda_cpu_only,
                }),
            });
            if (!res.ok) {
                let detail = await res.text();
                try {
                    const j = JSON.parse(detail);
                    if (j.detail) detail = j.detail;
                } catch (_) { /* keep */ }
                throw new Error(detail || `HTTP ${res.status}`);
            }
            const saved = await res.json();
            syncModelSettingsToControls(saved);
            if (select) select.value = savedModelSettings.chat_model;
            if (cuda) cuda.checked = savedModelSettings.cuda_cpu_only;

            setApplyStatus('Restarting Ollama…');
            const restartMode = await restartOllamaAfterApply();
            if (restartMode === 'manual') {
                setApplyStatus(
                    'Settings saved. Restart the Ollama server manually to apply GPU/CPU preference.',
                    'var(--color-warning)',
                );
            } else {
                setApplyStatus('Settings applied.', 'var(--color-success)');
            }

            const panel = configPanel();
            if (panel.root) {
                await refreshStatusFromServer(panel);
            }
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
        } catch (e) {
            setApplyStatus(e.message || 'Apply failed', 'var(--color-danger)');
        } finally {
            applyInFlight = false;
            if (select) select.disabled = false;
            if (cuda) cuda.disabled = false;
            updateApplyButtonState();
        }
    }

    function ensureModelSettingsControlsWired() {
        if (settingsControlsWired) return;
        settingsControlsWired = true;
        const select = modelSelectEl();
        const cuda = cudaCheckboxEl();
        const btn = applyBtnEl();
        if (select) {
            select.addEventListener('change', () => updateApplyButtonState());
        }
        if (cuda) {
            cuda.addEventListener('change', () => updateApplyButtonState());
        }
        if (btn) {
            btn.addEventListener('click', () => { void applyModelSettings(); });
        }
    }

    function setUseStatus(text, color) {
        const el = document.getElementById('local-ai-use-enabled-status');
        if (!el) return;
        el.textContent = text || '';
        el.style.color = color || 'var(--color-text-muted)';
    }

    function setStatusValue(el, text, color) {
        if (!el) return;
        el.textContent = text;
        if (color) el.style.color = color;
    }

    function syncElectronOnlyVisibility(panel) {
        if (!panel || !panel.root) return;
        const electronOnly = panel.root.querySelectorAll('[data-local-ai-electron-only]');
        electronOnly.forEach((el) => {
            el.style.display = hasElectron() ? '' : 'none';
        });
        if (panel.noElectronMsg) {
            panel.noElectronMsg.hidden = hasElectron();
        }
        if (panel.downloadBtn && !hasElectron()) {
            panel.downloadBtn.style.display = 'none';
        }
    }

    function parseUseEnabledFromRows(rows) {
        const row = Array.isArray(rows) ? rows.find((r) => r && r.key === CONFIG_KEY) : null;
        if (!row || row.value == null) return true;
        const v = String(row.value).trim().toLowerCase();
        return !(v === 'false' || v === '0' || v === 'no' || v === 'off');
    }

    function isUseEnabledForChat() {
        return useEnabledCached;
    }

    function syncUseEnabledForChat(enabled) {
        useEnabledCached = !!enabled;
        const checkbox = document.getElementById('local-ai-use-enabled-checkbox');
        if (checkbox) checkbox.checked = useEnabledCached;
    }

    async function loadUseEnabledSetting() {
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const rows = await res.json();
            useEnabledCached = parseUseEnabledFromRows(rows);
        } catch (_) {
            useEnabledCached = true;
        }
        const checkbox = document.getElementById('local-ai-use-enabled-checkbox');
        if (checkbox) checkbox.checked = useEnabledCached;
        setUseStatus('');
    }

    async function fetchServerStatus() {
        const res = await fetch(STATUS_URL, { credentials: 'same-origin' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
    }

    function modelAvailabilityLabel(available, modelName) {
        const name = modelName ? ` (${modelName})` : '';
        if (!available) return { text: `Not installed${name}`, color: 'var(--color-warning)' };
        return { text: `Available${name}`, color: 'var(--color-success)' };
    }

    function applyStatusToPanel(panel, status) {
        if (!panel || !panel.root || !status) return;

        syncElectronOnlyVisibility(panel);

        if (panel.chatLabelEl && status.chat_model) {
            panel.chatLabelEl.textContent = `Chat and Image Classification (${status.chat_model})`;
        }
        if (panel.embedLabelEl && status.embedding_model) {
            panel.embedLabelEl.textContent = `Vectorisation for Similarity Searches (${status.embedding_model})`;
        }

        if (!status.base_url_configured) {
            setStatusValue(panel.serverValueEl, 'Not configured', 'var(--color-danger)');
            if (panel.embedServerValueEl) {
                setStatusValue(panel.embedServerValueEl, '—', 'var(--color-text-muted)');
            }
            setStatusValue(panel.chatValueEl, '—', 'var(--color-text-muted)');
            setStatusValue(panel.embedValueEl, '—', 'var(--color-text-muted)');
            setPanelMsg(panel, 'Set LOCALAI_BASE_URL on the server to enable Local AI.', 'var(--color-danger)');
            if (panel.downloadBtn) panel.downloadBtn.style.display = 'none';
            return;
        }

        setPanelMsg(panel, '');

        if (status.server_reachable) {
            setStatusValue(panel.serverValueEl, 'Running', 'var(--color-success)');
        } else {
            setStatusValue(panel.serverValueEl, 'Not running', 'var(--color-danger)');
            if (status.server_error) {
                setPanelMsg(panel, 'Chat Ollama: ' + status.server_error, 'var(--color-danger)');
            }
        }

        if (panel.embedServerValueEl) {
            if (status.embedding_server_reachable) {
                setStatusValue(panel.embedServerValueEl, 'Running', 'var(--color-success)');
            } else {
                setStatusValue(panel.embedServerValueEl, 'Not running', 'var(--color-danger)');
                if (status.embedding_server_error) {
                    const existing = panel.msgEl && panel.msgEl.textContent ? panel.msgEl.textContent + ' ' : '';
                    setPanelMsg(panel, (existing + 'Embedding Ollama: ' + status.embedding_server_error).trim(), 'var(--color-danger)');
                }
            }
        }

        const chat = modelAvailabilityLabel(status.chat_model_available, '');
        setStatusValue(panel.chatValueEl, chat.text.replace(' ()', ''), chat.color);
        const embed = modelAvailabilityLabel(status.embedding_model_available, '');
        setStatusValue(panel.embedValueEl, embed.text.replace(' ()', ''), embed.color);

        if (hasElectron() && panel.downloadBtn) {
            const needDownload = !status.chat_model_available || !status.embedding_model_available;
            panel.downloadBtn.style.display = needDownload ? 'inline-flex' : 'none';
        }
    }

    function statusNeedsAttention(status) {
        if (!status) return true;
        if (!status.base_url_configured) return true;
        if (!status.server_reachable) return true;
        if (status.embedding_base_url !== undefined && !status.embedding_server_reachable) return true;
        if (!status.chat_model_available || !status.embedding_model_available) return true;
        return false;
    }

    async function refreshStatusFromServer(panel) {
        if (!panel || !panel.root) return null;
        syncElectronOnlyVisibility(panel);
        setStatusValue(panel.serverValueEl, 'Checking…', 'var(--color-text-muted)');
        if (panel.embedServerValueEl) {
            setStatusValue(panel.embedServerValueEl, '…', 'var(--color-text-muted)');
        }
        setStatusValue(panel.chatValueEl, '…', 'var(--color-text-muted)');
        setStatusValue(panel.embedValueEl, '…', 'var(--color-text-muted)');
        if (panel.downloadBtn) panel.downloadBtn.style.display = 'none';
        setPanelMsg(panel, '');

        if (hasElectron() && window.electronAPI.startOllama) {
            try {
                await window.electronAPI.startOllama();
            } catch (_) { /* server status reflects result */ }
        }

        try {
            const status = await fetchServerStatus();
            lastServerStatus = status;
            applyStatusToPanel(panel, status);
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
            return status;
        } catch (e) {
            setPanelMsg(panel, e.message || 'Could not load Local AI status', 'var(--color-danger)');
            return null;
        }
    }

    async function migrateClassifierOffLocalAIIfNeeded() {
        const HOSTED_KEY = 'hosted_llm_provider_order_v1';
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) return;
            const rows = await res.json();
            const row = Array.isArray(rows) ? rows.find((r) => r && r.key === HOSTED_KEY) : null;
            if (!row || row.value == null || String(row.value).trim() === '') return;
            let parsed;
            try {
                parsed = JSON.parse(row.value);
            } catch (_) {
                return;
            }
            if (String(parsed.classifier_provider || '').toLowerCase().trim() !== 'localai') return;
            parsed.classifier_provider = 'gemini';
            const saveRes = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    key: HOSTED_KEY,
                    value: JSON.stringify(parsed),
                    description: 'Hosted LLM provider try order for Auto routing and error failover, plus Auto classifier provider',
                }),
            });
            if (!saveRes.ok) return;
            if (typeof Modals !== 'undefined'
                && Modals.HostedLLMOrderConfig
                && Modals.HostedLLMOrderConfig.ensureLoaded) {
                await Modals.HostedLLMOrderConfig.ensureLoaded();
            }
            if (typeof Modals !== 'undefined'
                && Modals.HostedLLMOrderConfig
                && Modals.HostedLLMOrderConfig.reconcileClassifierProvider) {
                Modals.HostedLLMOrderConfig.reconcileClassifierProvider();
            }
        } catch (_) { /* best effort */ }
    }

    async function saveUseEnabledSetting(enabled) {
        if (saveInFlight) return;
        saveInFlight = true;
        const checkbox = document.getElementById('local-ai-use-enabled-checkbox');
        if (checkbox) checkbox.disabled = true;
        setUseStatus('Saving…');
        useEnabledCached = !!enabled;
        try {
            const res = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    key: CONFIG_KEY,
                    value: enabled ? 'true' : 'false',
                    description: 'When false, Local AI is not used for chat, Auto routing, or related LLM features',
                }),
            });
            if (!res.ok) {
                let detail = await res.text();
                try {
                    const j = JSON.parse(detail);
                    if (j.detail) detail = j.detail;
                } catch (_) { /* keep body */ }
                throw new Error(detail || `HTTP ${res.status}`);
            }
            useEnabledCached = !!enabled;
            setUseStatus(enabled ? 'Local AI enabled for chat.' : 'Local AI disabled for chat.', 'var(--color-success)');
            if (!enabled) {
                await migrateClassifierOffLocalAIIfNeeded();
            }
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
        } catch (e) {
            if (checkbox) checkbox.checked = useEnabledCached;
            setUseStatus(e.message || 'Save failed', 'var(--color-danger)');
        } finally {
            saveInFlight = false;
            if (checkbox) checkbox.disabled = false;
        }
    }

    function onUseEnabledCheckboxChanged(checkbox) {
        if (!checkbox || checkbox.id !== 'local-ai-use-enabled-checkbox') return;
        const enabled = !!checkbox.checked;
        useEnabledCached = enabled;
        setUseStatus('Saving…');
        if (typeof App !== 'undefined' && App.refreshChatAvailability) {
            void App.refreshChatAvailability();
        }
        void saveUseEnabledSetting(enabled);
    }

    let useEnabledToggleWired = false;
    function ensureUseEnabledToggleWired() {
        if (useEnabledToggleWired) return;
        useEnabledToggleWired = true;
        document.addEventListener('change', (event) => {
            const target = event.target;
            if (!target || target.id !== 'local-ai-use-enabled-checkbox') return;
            onUseEnabledCheckboxChanged(target);
        });
    }

    ensureUseEnabledToggleWired();

    async function downloadModels(panel) {
        if (!hasElectron()) return;
        const downloadBtn = panel.downloadBtn;
        const progressWrap = panel.progressWrap;
        const progressBar = panel.progressBar;
        const progressStatus = panel.progressStatus;
        const progressSize = panel.progressSize;

        if (downloadBtn) downloadBtn.disabled = true;
        if (progressWrap) progressWrap.style.display = 'block';
        if (progressBar) progressBar.value = 0;
        if (progressStatus) progressStatus.textContent = 'Preparing…';
        if (progressSize) progressSize.textContent = '';
        setPanelMsg(panel, 'Downloading AI models — this may take several minutes…', 'var(--color-text-muted)');

        const res = await window.electronAPI.pullOllamaModel();
        if (progressWrap) progressWrap.style.display = 'none';
        if (downloadBtn) downloadBtn.disabled = false;

        if (!res.ok) {
            setPanelMsg(panel, 'Download failed: ' + (res.error || 'unknown'), 'var(--color-danger)');
            await refreshStatusFromServer(panel);
            return;
        }

        setPanelMsg(panel, 'Models installed.', 'var(--color-success)');
        await refreshStatusFromServer(panel);
    }

    function onPullProgress(panel, evt) {
        const progressWrap = panel.progressWrap;
        const progressBar = panel.progressBar;
        const progressStatus = panel.progressStatus;
        const progressSize = panel.progressSize;
        if (!progressWrap || !progressBar || !progressStatus || !progressSize) return;

        const label = evt.model ? evt.model + ': ' : '';
        if (evt.type === 'progress') {
            progressWrap.style.display = 'block';
            progressBar.value = evt.percent;
            progressStatus.textContent = label + (evt.status || 'Downloading…');
            progressSize.textContent = evt.downloaded + ' / ' + evt.total;
        } else if (evt.type === 'status') {
            progressWrap.style.display = 'block';
            progressBar.value = 0;
            progressStatus.textContent = evt.status;
            progressSize.textContent = '';
        } else if (evt.type === 'done') {
            progressBar.value = 100;
            progressStatus.textContent = 'All models downloaded';
            progressSize.textContent = '';
        }
    }

    async function init() {
        ensureUseEnabledToggleWired();
        ensureModelSettingsControlsWired();
        const root = configRoot();
        if (!root) return;
        if (inited) return;
        inited = true;

        const panel = configPanel();
        syncElectronOnlyVisibility(panel);

        if (panel.downloadBtn) {
            panel.downloadBtn.addEventListener('click', () => { void downloadModels(panel); });
        }

        if (hasElectron() && window.electronAPI.onOllamaPullProgress) {
            window.electronAPI.onOllamaPullProgress((evt) => onPullProgress(panel, evt));
        }

        await loadUseEnabledSetting();
    }

    async function refresh() {
        ensureUseEnabledToggleWired();
        ensureModelSettingsControlsWired();
        await init();
        await loadUseEnabledSetting();
        const panel = configPanel();
        if (panel.root) {
            await refreshStatusFromServer(panel);
        }
        await loadModelSettingsAndModels();
    }

    async function refreshLoginAdvanced() {
        ensureUseEnabledToggleWired();
        const panel = loginPanel();
        if (!panel.root) return null;
        syncElectronOnlyVisibility(panel);
        return refreshStatusFromServer(panel);
    }

    function getLastServerStatus() {
        return lastServerStatus;
    }

    function loginStatusNeedsAttention() {
        return statusNeedsAttention(lastServerStatus);
    }

    function wireLoginDownloadButton() {
        const panel = loginPanel();
        if (!panel.downloadBtn || panel.downloadBtn.dataset.localAiLoginWired === '1') return;
        panel.downloadBtn.dataset.localAiLoginWired = '1';
        panel.downloadBtn.addEventListener('click', () => { void downloadModels(panel); });
        if (hasElectron() && window.electronAPI.onOllamaPullProgress) {
            window.electronAPI.onOllamaPullProgress((evt) => onPullProgress(panel, evt));
        }
    }

    return {
        init,
        refresh,
        refreshLoginAdvanced,
        wireLoginDownloadButton,
        isUseEnabledForChat,
        syncUseEnabledForChat,
        getLastServerStatus,
        loginStatusNeedsAttention,
        statusNeedsAttention,
    };
})();

