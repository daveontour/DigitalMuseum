'use strict';

/**
 * Setup checklist + System status (owner only).
 *
 * - "Getting started": GET /api/setup-checklist — first-run steps worked out from the archive's
 *   real data (internal/handler/system_status_handler.go), each with a button that takes you there.
 * - "System status": GET /api/system-status — one row per part of the app (AI connection, AI tools
 *   server, tools turned on, local search indexing, helper programs, backup, failed jobs…).
 * - Top-bar pill: "Needs attention" when a status row is an error, otherwise "Finish setup n/m"
 *   until the checklist is complete or dismissed.
 * - Chat warning when no AI tools are turned on, with a one-click "Turn on recommended tools"
 *   (POST /api/settings/llm-tools-access/enable-recommended).
 *
 * Rows carry a fix {label, action}; FIX_ACTIONS maps each action key to the UI that fixes it.
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const SystemStatus = (() => {
    const POLL_MS = 10 * 60 * 1000;
    const BANNER_HIDDEN_KEY = 'dm.chatToolsOffBanner.hidden.v1';
    const STATUS_META = {
        ok:    { icon: 'fa-check-circle',        cls: 'system-status-row--ok',    label: 'OK' },
        info:  { icon: 'fa-info-circle',         cls: 'system-status-row--info',  label: 'Note' },
        warn:  { icon: 'fa-exclamation-triangle', cls: 'system-status-row--warn',  label: 'Check' },
        error: { icon: 'fa-times-circle',        cls: 'system-status-row--error', label: 'Problem' },
    };

    let isOwner = false;
    let checklist = null;
    let health = null;
    let toolSummary = null;
    let pollTimer = null;
    let activeTab = 'setup';

    function el(id) { return document.getElementById(id); }

    async function getJSON(url) {
        const res = await fetch(url, { credentials: 'same-origin' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
    }

    // ── Fix actions ─────────────────────────────────────────────────────────
    function openConfigTab(tab) {
        if (typeof window.openConfigurationToTab === 'function') {
            void window.openConfigurationToTab(tab);
        } else if (typeof Guide !== 'undefined' && Guide._openConfigurationTab) {
            Guide._openConfigurationTab(tab);
        }
    }

    function openDataImportTab(tab) {
        if (typeof Guide !== 'undefined' && typeof Guide._openDataImportTab === 'function') {
            Guide._openDataImportTab(tab);
        } else {
            el('data-import-sidebar-btn')?.click();
        }
    }

    const FIX_ACTIONS = {
        openConfigApiKeys:              { close: true, run: () => openConfigTab('api-keys') },
        openConfigAiSetup:              { close: true, run: () => openConfigTab('ai-setup-config') },
        openConfigSubjectConfiguration: { close: true, run: () => openConfigTab('subject-configuration') },
        openConfigManageVisitorKeys:    { close: true, run: () => openConfigTab('manage-keys') },
        openConfigToolsAccess:          { close: true, run: () => openConfigTab('tools-access') },
        openDataImport:                 { close: true, run: () => openDataImportTab('import') },
        openScheduledJobs:              { close: true, run: () => openDataImportTab('background-jobs') },
        openPostImport:                 { close: true, run: () => { if (typeof PostImportStatus !== 'undefined') PostImportStatus.open(); } },
        openBackup:                     { close: true, run: () => { if (typeof ArchiveBackup !== 'undefined') ArchiveBackup.open(); } },
        openPeopleInPhotos:             { close: true, run: () => el('people-in-photos-sidebar-btn')?.click() },
        enableRecommendedTools:         { close: false, run: () => enableRecommendedTools() },
        refreshToolCatalog:             { close: false, run: () => refreshToolCatalog() },
    };

    function runFix(action) {
        const fix = FIX_ACTIONS[action];
        if (!fix) return;
        if (fix.close) closeModal();
        return fix.run();
    }

    async function enableRecommendedTools() {
        let body = {};
        try {
            const res = await fetch('/api/settings/llm-tools-access/enable-recommended', {
                method: 'POST',
                credentials: 'same-origin',
            });
            body = await res.json().catch(() => ({}));
            if (res.status === 403) {
                await AppDialogs.showAppAlert(
                    'Unlock needed',
                    'Changing which tools the AI may use needs your master password to be unlocked in this session. ' +
                    'Unlock it (Sensitive & Private Data), then try again — or choose tools yourself in Configuration → Tools Access.'
                );
                return;
            }
            if (!res.ok) throw new Error(body.detail || body.error || `HTTP ${res.status}`);
        } catch (err) {
            await AppDialogs.showAppAlert('Could not turn on tools', err.message || 'Request failed.');
            return;
        }
        const n = Array.isArray(body.turned_on) ? body.turned_on.length : 0;
        await AppDialogs.showAppAlert(
            'AI tools turned on',
            n > 0
                ? `${n} recommended tools are now on, so the AI can search your messages, emails and photos when you ask it something. ` +
                  'You can review them any time in Configuration → Tools Access.'
                : 'The recommended tools were already on.'
        );
        await refreshChatAvailability();
        await refreshAll();
    }

    async function refreshToolCatalog() {
        try {
            const res = await fetch('/api/settings/llm-tools-access/refresh-catalog', { method: 'POST', credentials: 'same-origin' });
            const body = await res.json().catch(() => ({}));
            if (body.error) {
                await AppDialogs.showAppAlert('AI tools server still not responding', body.error);
            }
        } catch (err) {
            await AppDialogs.showAppAlert('AI tools server still not responding', err.message || 'Request failed.');
        }
        await refreshAll();
    }

    async function refreshChatAvailability() {
        try {
            const av = await getJSON('/chat/availability');
            if (typeof UI !== 'undefined' && UI.updateChatContextStatusBarFromAvailability) {
                UI.updateChatContextStatusBarFromAvailability(av);
            }
        } catch (_) { /* best effort */ }
    }

    // ── Data ────────────────────────────────────────────────────────────────
    async function loadIdentity() {
        try {
            const me = await getJSON('/auth/me');
            isOwner = !!(me && !me.is_visitor);
        } catch (_) {
            isOwner = false;
        }
    }

    async function refreshAll() {
        if (!isOwner) { render(); return; }
        const [c, h, t] = await Promise.allSettled([
            getJSON('/api/setup-checklist'),
            getJSON('/api/system-status'),
            getJSON('/api/settings/llm-tools-access/summary'),
        ]);
        if (c.status === 'fulfilled') checklist = c.value;
        if (h.status === 'fulfilled') health = h.value;
        if (t.status === 'fulfilled') toolSummary = t.value;
        render();
        if (pollTimer) clearTimeout(pollTimer);
        pollTimer = setTimeout(() => { void refreshAll(); }, POLL_MS);
    }

    async function refreshHealthOnly() {
        const btn = el('system-status-recheck-btn');
        if (btn) btn.disabled = true;
        try {
            health = await getJSON('/api/system-status');
        } catch (_) { /* keep previous */ }
        if (btn) btn.disabled = false;
        render();
    }

    // ── Rendering ───────────────────────────────────────────────────────────
    function render() {
        renderAccountEntry();
        renderPill();
        renderWelcome();
        renderBanner();
        const modal = el('system-status-modal');
        if (modal && modal.style.display !== 'none') {
            renderChecklist();
            renderHealth();
        }
    }

    function renderAccountEntry() {
        const section = el('account-system-status-section');
        if (section) section.style.display = isOwner ? 'block' : 'none';
    }

    function problemCount() {
        const checks = (health && Array.isArray(health.checks)) ? health.checks : [];
        return checks.filter(c => c.status === 'error').length;
    }

    function renderPill() {
        const btn = el('museum-health-btn');
        const icon = el('museum-health-icon');
        const label = el('museum-health-label');
        if (!btn || !icon || !label) return;
        btn.classList.remove('museum-health-btn--error');
        if (!isOwner) { btn.hidden = true; return; }
        const problems = problemCount();
        if (problems > 0) {
            icon.className = 'fas fa-exclamation-circle';
            label.textContent = problems === 1 ? '1 problem needs attention' : `${problems} problems need attention`;
            btn.title = 'Something isn’t working — open System status';
            btn.dataset.target = 'health';
            btn.classList.add('museum-health-btn--error');
            btn.hidden = false;
            return;
        }
        if (checklist && !checklist.complete && !checklist.dismissed && checklist.total > 0) {
            icon.className = 'fas fa-list-check';
            label.textContent = `Finish setup ${checklist.done}/${checklist.total}`;
            btn.title = 'A few steps left to get your museum ready';
            btn.dataset.target = 'setup';
            btn.hidden = false;
            return;
        }
        btn.hidden = true;
    }

    function renderWelcome() {
        const btn = el('welcome-setup-checklist-btn');
        const label = el('welcome-setup-checklist-label');
        if (!btn) return;
        const show = isOwner && checklist && !checklist.complete && checklist.total > 0;
        btn.hidden = !show;
        if (show && label) label.textContent = `Continue setup (${checklist.done} of ${checklist.total} done)`;
    }

    function bannerHiddenThisSession() {
        try { return sessionStorage.getItem(BANNER_HIDDEN_KEY) === '1'; } catch (_) { return false; }
    }

    function renderBanner() {
        const banner = el('chat-tools-off-banner');
        if (!banner) return;
        const show = isOwner && toolSummary && toolSummary.available > 0
            && toolSummary.enabled_for_owner === 0 && !bannerHiddenThisSession();
        banner.hidden = !show;
    }

    function fixButton(fix, idPrefix) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'modal-btn modal-btn-secondary system-status-fix-btn';
        b.id = `${idPrefix}-fix-btn`;
        b.textContent = fix.label;
        b.addEventListener('click', () => { void runFix(fix.action); });
        return b;
    }

    function renderChecklist() {
        const list = el('system-status-setup-list');
        const lead = el('system-status-setup-lead');
        const bar = el('system-status-progress-bar');
        const count = el('system-status-setup-count');
        const dismiss = el('system-status-setup-dismiss-checkbox');
        if (!list) return;
        list.innerHTML = '';
        if (!checklist) {
            lead.textContent = 'Loading…';
            return;
        }
        const pct = checklist.total > 0 ? Math.round((checklist.done / checklist.total) * 100) : 100;
        if (bar) bar.style.width = `${pct}%`;
        if (count) count.textContent = `${checklist.done}/${checklist.total}`;
        if (dismiss) dismiss.checked = !!checklist.dismissed;
        lead.textContent = checklist.complete
            ? 'Your museum is set up. The optional steps below are still worth a look.'
            : `${checklist.done} of ${checklist.total} steps done. Each step has a button that takes you straight there.`;

        (checklist.items || []).forEach((it) => {
            const li = document.createElement('li');
            li.className = 'system-status-row';
            li.id = `system-status-setup-${it.key}`;
            const state = it.done ? 'done' : (it.not_yet ? 'later' : 'todo');
            li.classList.add(`system-status-step--${state}`);
            const icon = document.createElement('i');
            icon.className = `fas ${it.done ? 'fa-check-circle' : (it.not_yet ? 'fa-hourglass-half' : 'fa-circle')} system-status-row-icon`;
            icon.setAttribute('aria-hidden', 'true');
            const body = document.createElement('div');
            body.className = 'system-status-row-body';
            const title = document.createElement('div');
            title.className = 'system-status-row-title';
            title.textContent = it.title;
            if (it.optional) {
                const tag = document.createElement('span');
                tag.className = 'system-status-optional';
                tag.textContent = 'Optional';
                title.appendChild(tag);
            }
            const detail = document.createElement('div');
            detail.className = 'system-status-row-message';
            detail.textContent = it.done ? 'Done.' : (it.not_yet || it.detail);
            body.appendChild(title);
            body.appendChild(detail);
            li.appendChild(icon);
            li.appendChild(body);
            if (!it.done && !it.not_yet && it.fix) li.appendChild(fixButton(it.fix, `system-status-setup-${it.key}`));
            list.appendChild(li);
        });
    }

    function renderHealth() {
        const list = el('system-status-health-list');
        const summary = el('system-status-health-summary');
        const badge = el('system-status-health-badge');
        if (!list) return;
        list.innerHTML = '';
        if (!health) {
            summary.textContent = 'Checking…';
            return;
        }
        const checks = Array.isArray(health.checks) ? health.checks : [];
        const problems = checks.filter(c => c.status === 'error').length;
        const warnings = checks.filter(c => c.status === 'warn').length;
        if (badge) {
            badge.textContent = problems ? String(problems) : (warnings ? String(warnings) : '');
            badge.className = `system-status-tab-badge${problems ? ' system-status-tab-badge--error' : (warnings ? ' system-status-tab-badge--warn' : '')}`;
        }
        summary.textContent = problems
            ? `${problems === 1 ? 'One thing isn’t' : `${problems} things aren’t`} working. Start with the red rows.`
            : (warnings ? 'Everything essential is working. A few things are worth checking.' : 'Everything is working.');
        const order = { error: 0, warn: 1, info: 2, ok: 3 };
        checks.slice().sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9)).forEach((c) => {
            const meta = STATUS_META[c.status] || STATUS_META.info;
            const li = document.createElement('li');
            li.className = `system-status-row ${meta.cls}`;
            li.id = `system-status-check-${c.key}`;
            const icon = document.createElement('i');
            icon.className = `fas ${meta.icon} system-status-row-icon`;
            icon.setAttribute('aria-label', meta.label);
            const body = document.createElement('div');
            body.className = 'system-status-row-body';
            const title = document.createElement('div');
            title.className = 'system-status-row-title';
            title.textContent = c.title;
            const msg = document.createElement('div');
            msg.className = 'system-status-row-message';
            msg.textContent = c.message;
            body.appendChild(title);
            body.appendChild(msg);
            li.appendChild(icon);
            li.appendChild(body);
            if (c.fix && c.status !== 'ok') li.appendChild(fixButton(c.fix, `system-status-check-${c.key}`));
            list.appendChild(li);
        });
    }

    // ── Modal ───────────────────────────────────────────────────────────────
    function selectTab(tab) {
        activeTab = tab === 'health' ? 'health' : 'setup';
        document.querySelectorAll('#system-status-modal .system-status-tab').forEach((b) => {
            const on = b.getAttribute('data-sys-tab') === activeTab;
            b.classList.toggle('system-status-tab--active', on);
            b.setAttribute('aria-selected', on ? 'true' : 'false');
        });
        el('system-status-panel-setup').hidden = activeTab !== 'setup';
        el('system-status-panel-health').hidden = activeTab !== 'health';
    }

    function openModal(tab) {
        const menu = el('account-dropdown-menu');
        if (menu) menu.style.display = 'none';
        const modal = el('system-status-modal');
        if (!modal) return;
        selectTab(tab || activeTab);
        modal.style.display = 'flex';
        render();
        void refreshAll();
    }

    function closeModal() {
        const modal = el('system-status-modal');
        if (modal) modal.style.display = 'none';
    }

    async function setDismissed(dismissed) {
        try {
            await fetch('/api/setup-checklist/dismiss', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ dismissed }),
            });
        } catch (_) { /* ignore */ }
        if (checklist) checklist.dismissed = dismissed;
        render();
    }

    async function init() {
        el('museum-health-btn')?.addEventListener('click', () => openModal(el('museum-health-btn').dataset.target || 'setup'));
        el('account-system-status-btn')?.addEventListener('click', (e) => { e.stopPropagation(); openModal('setup'); });
        el('welcome-setup-checklist-btn')?.addEventListener('click', () => {
            if (window.closeInfoBoxModal) window.closeInfoBoxModal({ skipMasterPrompt: true });
            openModal('setup');
        });
        el('close-system-status-modal')?.addEventListener('click', closeModal);
        const modal = el('system-status-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        document.querySelectorAll('#system-status-modal .system-status-tab').forEach((b) => {
            b.addEventListener('click', () => selectTab(b.getAttribute('data-sys-tab')));
        });
        el('system-status-recheck-btn')?.addEventListener('click', () => { void refreshHealthOnly(); });
        el('system-status-setup-dismiss-checkbox')?.addEventListener('change', (e) => { void setDismissed(!!e.target.checked); });
        el('chat-tools-off-enable-btn')?.addEventListener('click', () => { void enableRecommendedTools(); });
        el('chat-tools-off-dismiss-btn')?.addEventListener('click', () => {
            try { sessionStorage.setItem(BANNER_HIDDEN_KEY, '1'); } catch (_) { /* ignore */ }
            renderBanner();
        });
        document.addEventListener('dm:archive-data-changed', () => { void refreshAll(); });
        // Configuration and Source Data are where most steps get done — re-check when they close.
        ['close-config', 'close-data-import-modal', 'close-backup-modal', 'backup-close-btn'].forEach((id) => {
            el(id)?.addEventListener('click', () => { setTimeout(() => { void refreshAll(); }, 300); });
        });
        await loadIdentity();
        await refreshAll();
    }

    return { init, open: openModal, refresh: refreshAll, enableRecommendedTools };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => { void SystemStatus.init(); });
} else {
    void SystemStatus.init();
}
