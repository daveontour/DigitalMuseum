'use strict';

/**
 * Post-import processing status ("Getting your archive ready").
 *
 * Polls GET /api/post-import/status (internal/handler/post_import_pipeline.go) and shows a
 * top-bar indicator while the automatic after-import steps run, with a details dialog listing
 * each step. Also wires the "Automatically prepare new data after each import" checkbox on the
 * Source Data → Data Import tab (app_configuration key post_import_processing_enabled_v1).
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const PostImportStatus = (() => {
    const CONFIG_KEY = 'post_import_processing_enabled_v1';
    const POLL_IDLE_MS = 20000;
    const POLL_ACTIVE_MS = 3000;
    const SHOW_FINISHED_MS = 3 * 60 * 1000;

    const STEP_ICONS = {
        pending:   { icon: 'fa-circle', cls: 'post-import-step--pending', label: 'Waiting' },
        running:   { icon: 'fa-circle-notch fa-spin', cls: 'post-import-step--running', label: 'Working' },
        done:      { icon: 'fa-check-circle', cls: 'post-import-step--done', label: 'Done' },
        skipped:   { icon: 'fa-minus-circle', cls: 'post-import-step--skipped', label: 'Skipped' },
        error:     { icon: 'fa-exclamation-circle', cls: 'post-import-step--error', label: 'Problem' },
        cancelled: { icon: 'fa-stop-circle', cls: 'post-import-step--skipped', label: 'Stopped' },
    };

    let pollTimer = null;
    let lastRun = null;
    let lastQueued = false;
    let firstPoll = true;
    let knownFinishedAt = null;
    let finishedSeenAt = 0;
    let dismissedFinishedAt = null;
    let initialised = false;

    function el(id) { return document.getElementById(id); }

    function isActive(run, queued) {
        return !!queued || !!(run && run.running);
    }

    function schedule(ms) {
        if (pollTimer) clearTimeout(pollTimer);
        pollTimer = setTimeout(() => { void poll(); }, ms);
    }

    async function poll() {
        let data = null;
        try {
            const res = await fetch('/api/post-import/status', { credentials: 'same-origin' });
            if (res.ok) data = await res.json();
        } catch (_) {
            data = null;
        }
        if (data) {
            lastRun = data.run || null;
            lastQueued = !!data.queued;
            const active = isActive(lastRun, lastQueued);
            // Announce a pass that finished since this page loaded — keyed on finished_at so a
            // quick pass that starts and ends between two idle polls is still noticed.
            const finishedAt = (!active && lastRun && lastRun.finished_at) || null;
            if (firstPoll) {
                knownFinishedAt = finishedAt;
            } else if (finishedAt && finishedAt !== knownFinishedAt) {
                knownFinishedAt = finishedAt;
                finishedSeenAt = Date.now();
                document.dispatchEvent(new CustomEvent('dm:archive-data-changed'));
            }
            firstPoll = false;
            render();
        }
        schedule(isActive(lastRun, lastQueued) ? POLL_ACTIVE_MS : POLL_IDLE_MS);
    }

    function counts(run) {
        const steps = (run && Array.isArray(run.steps)) ? run.steps : [];
        const finished = steps.filter(s => s.status !== 'pending' && s.status !== 'running').length;
        const problems = steps.filter(s => s.status === 'error').length;
        const skipped = steps.filter(s => s.status === 'skipped').length;
        return { total: steps.length, finished, problems, skipped };
    }

    function renderButton() {
        const btn = el('post-import-status-btn');
        const icon = el('post-import-status-icon');
        const label = el('post-import-status-label');
        if (!btn || !icon || !label) return;

        const active = isActive(lastRun, lastQueued);
        const finishedKey = lastRun && lastRun.finished_at;
        const recentlyFinished = !active && finishedSeenAt > 0 &&
            (Date.now() - finishedSeenAt) < SHOW_FINISHED_MS &&
            finishedKey && finishedKey !== dismissedFinishedAt;

        if (active) {
            const c = counts(lastRun);
            icon.className = 'fas fa-circle-notch fa-spin';
            label.textContent = c.total > 0
                ? `Preparing your archive… ${Math.min(c.finished + 1, c.total)} of ${c.total}`
                : 'Preparing your archive…';
            btn.classList.remove('post-import-status-btn--done', 'post-import-status-btn--warn');
            btn.hidden = false;
        } else if (recentlyFinished) {
            const c = counts(lastRun);
            const warn = c.problems > 0 || c.skipped > 0 || (lastRun && lastRun.cancelled);
            icon.className = warn ? 'fas fa-exclamation-circle' : 'fas fa-check-circle';
            label.textContent = lastRun && lastRun.cancelled
                ? 'Archive preparation stopped'
                : (warn ? 'Archive ready — some steps need attention' : 'Archive ready');
            btn.classList.toggle('post-import-status-btn--done', !warn);
            btn.classList.toggle('post-import-status-btn--warn', !!warn);
            btn.hidden = false;
        } else {
            btn.hidden = true;
        }
    }

    function renderModal() {
        const list = el('post-import-steps-list');
        const stopBtn = el('post-import-stop-btn');
        const summary = el('post-import-modal-summary');
        if (!list) return;
        list.innerHTML = '';
        const steps = (lastRun && Array.isArray(lastRun.steps)) ? lastRun.steps : [];
        if (steps.length === 0) {
            const li = document.createElement('li');
            li.className = 'post-import-step post-import-step--pending';
            li.textContent = lastQueued ? 'Starting shortly…' : 'Nothing has been imported since the app started.';
            list.appendChild(li);
        }
        steps.forEach(step => {
            const meta = STEP_ICONS[step.status] || STEP_ICONS.pending;
            const li = document.createElement('li');
            li.className = `post-import-step ${meta.cls}`;
            const icon = document.createElement('i');
            icon.className = `fas ${meta.icon} post-import-step-icon`;
            icon.setAttribute('aria-hidden', 'true');
            const body = document.createElement('div');
            body.className = 'post-import-step-body';
            const title = document.createElement('div');
            title.className = 'post-import-step-title';
            title.textContent = step.title || step.key;
            const state = document.createElement('span');
            state.className = 'post-import-step-state';
            state.textContent = meta.label;
            title.appendChild(state);
            body.appendChild(title);
            if (step.message) {
                const msg = document.createElement('div');
                msg.className = 'post-import-step-message';
                msg.textContent = step.message;
                body.appendChild(msg);
            }
            li.appendChild(icon);
            li.appendChild(body);
            list.appendChild(li);
        });
        const active = isActive(lastRun, lastQueued);
        if (stopBtn) stopBtn.hidden = !active;
        const statusBtn = el('post-import-open-status-btn');
        if (statusBtn) {
            const c = counts(lastRun);
            statusBtn.hidden = active || (c.problems === 0 && c.skipped === 0) || typeof SystemStatus === 'undefined';
        }
        if (summary) {
            summary.textContent = active
                ? 'Digital Museum is preparing your newly imported data so you can search it, browse photos, and ask the AI about it. You can keep using the app while this runs.'
                : 'After each import, Digital Museum prepares the new data so you can search it, browse photos, and ask the AI about it. Skipped steps can be run any time from Scheduled jobs.';
        }
    }

    function render() {
        renderButton();
        const modal = el('post-import-modal');
        if (modal && modal.style.display !== 'none') renderModal();
    }

    function openModal() {
        const modal = el('post-import-modal');
        if (!modal) return;
        renderModal();
        modal.style.display = 'flex';
    }

    function closeModal() {
        const modal = el('post-import-modal');
        if (modal) modal.style.display = 'none';
        // Once the owner has looked at a finished run, stop advertising it in the top bar.
        if (lastRun && !isActive(lastRun, lastQueued) && lastRun.finished_at) {
            dismissedFinishedAt = lastRun.finished_at;
            renderButton();
        }
    }

    async function stop() {
        const ok = await AppDialogs.showAppConfirm(
            'Stop preparing your archive?',
            'The current step will be stopped and the remaining steps skipped. Your imported data is kept, and you can run any step later from Scheduled jobs.'
        );
        if (!ok) return;
        try {
            const res = await fetch('/api/post-import/cancel', { method: 'POST', credentials: 'same-origin' });
            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                throw new Error(body.detail || body.error || `HTTP ${res.status}`);
            }
        } catch (err) {
            await AppDialogs.showAppAlert('Could not stop', err.message || 'Request failed.');
        }
        void poll();
    }

    function openScheduledJobs() {
        closeModal();
        if (typeof Guide !== 'undefined' && typeof Guide._openDataImportTab === 'function') {
            Guide._openDataImportTab('background-jobs');
        } else {
            el('data-import-sidebar-btn')?.click();
        }
    }

    // ── Auto-processing setting ─────────────────────────────────────────────
    function parseEnabled(raw) {
        if (raw == null) return true;
        const v = String(raw).trim().toLowerCase();
        return !(v === 'false' || v === '0' || v === 'no' || v === 'off');
    }

    async function loadSetting() {
        const cb = el('post-import-auto-checkbox');
        if (!cb) return;
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) return;
            const rows = await res.json();
            const row = Array.isArray(rows) ? rows.find(r => r && r.key === CONFIG_KEY) : null;
            cb.checked = parseEnabled(row ? row.value : null);
        } catch (_) { /* keep default */ }
    }

    async function saveSetting() {
        const cb = el('post-import-auto-checkbox');
        const status = el('post-import-auto-status');
        if (!cb) return;
        const value = cb.checked ? 'true' : 'false';
        try {
            const res = await fetch('/api/configuration', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    key: CONFIG_KEY,
                    value,
                    is_mandatory: false,
                    description: 'Automatically run contact refresh, thumbnails, search indexing and face detection after each import',
                }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            if (status) {
                status.textContent = 'Saved';
                setTimeout(() => { if (status.textContent === 'Saved') status.textContent = ''; }, 2000);
            }
        } catch (err) {
            cb.checked = !cb.checked;
            if (status) status.textContent = 'Could not save';
        }
    }

    function init() {
        if (initialised) return;
        initialised = true;
        el('post-import-status-btn')?.addEventListener('click', openModal);
        el('close-post-import-modal')?.addEventListener('click', closeModal);
        el('post-import-close-btn')?.addEventListener('click', closeModal);
        el('post-import-stop-btn')?.addEventListener('click', () => { void stop(); });
        el('post-import-open-jobs-btn')?.addEventListener('click', openScheduledJobs);
        el('post-import-open-status-btn')?.addEventListener('click', () => {
            closeModal();
            if (typeof SystemStatus !== 'undefined') SystemStatus.open('health');
        });
        const modal = el('post-import-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        el('post-import-auto-checkbox')?.addEventListener('change', () => { void saveSetting(); });
        el('data-import-sidebar-btn')?.addEventListener('click', () => { void loadSetting(); });
        el('close-data-import-modal')?.addEventListener('click', () => { void poll(); });
        void loadSetting();
        void poll();
        // Import dialogs finish in the background; check promptly when the user comes back.
        document.addEventListener('visibilitychange', () => {
            if (document.visibilityState === 'visible') void poll();
        });
    }

    return { init, refresh: poll, open: openModal };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => PostImportStatus.init());
} else {
    PostImportStatus.init();
}
