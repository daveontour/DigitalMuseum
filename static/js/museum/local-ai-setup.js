'use strict';

/** Local AI / Ollama setup UI — shared by login advanced panel and Configuration → AI & Setup. */
const LocalAiSetup = (() => {
    let inited = false;

    function root() {
        return document.getElementById('ai-setup-config-root');
    }

    function q(sel) {
        const r = root();
        return r ? r.querySelector(sel) : null;
    }

    function hasElectron() {
        return !!(window.electronAPI && typeof window.electronAPI.startOllama === 'function');
    }

    function setMsg(text, color) {
        const el = q('[data-local-ai-msg]');
        if (!el) return;
        el.textContent = text || '';
        el.style.color = color || 'var(--color-text-muted)';
        el.style.display = text ? 'block' : 'none';
    }

    function setStatusValue(el, text, color) {
        if (!el) return;
        el.textContent = text;
        if (color) el.style.color = color;
    }

    async function refresh() {
        init();

        const noElectronMsg = q('[data-local-ai-no-electron]');
        const ollamaBlock = q('[data-local-ai-ollama-block]');
        const downloadBtn = q('[data-local-ai-download]');

        if (!hasElectron()) {
            if (noElectronMsg) noElectronMsg.hidden = false;
            if (ollamaBlock) ollamaBlock.hidden = true;
            return;
        }

        if (noElectronMsg) noElectronMsg.hidden = true;
        if (ollamaBlock) ollamaBlock.hidden = false;

        const serverValueEl = q('[data-local-ai-server-value]');
        const gemmaValueEl = q('[data-local-ai-gemma-value]');
        const embedValueEl = q('[data-local-ai-embed-value]');
        const progressWrap = q('[data-local-ai-progress-wrap]');

        setMsg('');
        if (progressWrap) progressWrap.style.display = 'none';
        setStatusValue(serverValueEl, 'Checking…', 'var(--color-text-muted)');
        setStatusValue(gemmaValueEl, '…', 'var(--color-text-muted)');
        setStatusValue(embedValueEl, '…', 'var(--color-text-muted)');
        if (downloadBtn) downloadBtn.style.display = 'none';

        const startRes = await window.electronAPI.startOllama();
        if (startRes.ok) {
            setStatusValue(serverValueEl, 'Running', 'var(--color-success)');
        } else {
            setStatusValue(serverValueEl, 'Not running', 'var(--color-danger)');
            setMsg('Ollama: ' + (startRes.error || 'failed to start'), 'var(--color-danger)');
        }

        const check = await window.electronAPI.checkOllamaModel();
        if (!check.ok) {
            setStatusValue(gemmaValueEl, '—', 'var(--color-text-muted)');
            setStatusValue(embedValueEl, '—', 'var(--color-text-muted)');
            setMsg(check.error, 'var(--color-danger)');
            return;
        }

        setStatusValue(
            gemmaValueEl,
            check.hasGemma4 ? 'Available' : 'Not installed',
            check.hasGemma4 ? 'var(--color-success)' : 'var(--color-warning)'
        );
        setStatusValue(
            embedValueEl,
            check.hasEmbeddingModel ? 'Available' : 'Not installed',
            check.hasEmbeddingModel ? 'var(--color-success)' : 'var(--color-warning)'
        );

        const needDownload = !check.hasGemma4 || !check.hasEmbeddingModel;
        if (downloadBtn) downloadBtn.style.display = needDownload ? 'inline-flex' : 'none';

        if (check.hasGemma4 && check.hasEmbeddingModel && startRes.ok) {
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
        }
    }

    async function downloadModels() {
        const downloadBtn = q('[data-local-ai-download]');
        const progressWrap = q('[data-local-ai-progress-wrap]');
        const progressBar = q('[data-local-ai-progress-bar]');
        const progressStatus = q('[data-local-ai-progress-status]');
        const progressSize = q('[data-local-ai-progress-size]');

        if (!hasElectron()) return;

        if (downloadBtn) downloadBtn.disabled = true;
        if (progressWrap) progressWrap.style.display = 'block';
        if (progressBar) progressBar.value = 0;
        if (progressStatus) progressStatus.textContent = 'Preparing…';
        if (progressSize) progressSize.textContent = '';
        setMsg('Downloading AI models — this may take several minutes…', 'var(--color-text-muted)');

        const res = await window.electronAPI.pullOllamaModel();
        if (progressWrap) progressWrap.style.display = 'none';
        if (downloadBtn) downloadBtn.disabled = false;

        if (!res.ok) {
            setMsg('Download failed: ' + (res.error || 'unknown'), 'var(--color-danger)');
            await refresh();
            return;
        }

        setMsg('Models installed.', 'var(--color-success)');
        await refresh();
    }

    function onPullProgress(evt) {
        const progressWrap = q('[data-local-ai-progress-wrap]');
        const progressBar = q('[data-local-ai-progress-bar]');
        const progressStatus = q('[data-local-ai-progress-status]');
        const progressSize = q('[data-local-ai-progress-size]');
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

    function init() {
        if (inited) return;
        const r = root();
        if (!r) return;
        inited = true;

        const downloadBtn = q('[data-local-ai-download]');
        if (downloadBtn) {
            downloadBtn.addEventListener('click', () => { void downloadModels(); });
        }

        if (hasElectron() && window.electronAPI.onOllamaPullProgress) {
            window.electronAPI.onOllamaPullProgress(onPullProgress);
        }
    }

    return { init, refresh };
})();
