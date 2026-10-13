'use strict';

/**
 * One-click archive backup (Account → Back Up Archive…, plus a top-bar reminder when the
 * archive hasn't been backed up for 30 days). Server side: internal/handler/backup_handler.go
 * and internal/service/archive_backup_service.go.
 *
 * The destination is chosen with Electron's native save dialog (window.electronAPI.showSaveDialog);
 * outside the desktop app the server reports available=false and the dialog explains why.
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const ArchiveBackup = (() => {
    const POLL_IDLE_MS = 5 * 60 * 1000;
    const POLL_RUNNING_MS = 2000;

    let status = null;      // last /api/backup/status "status" object
    let available = false;
    let isOwner = false;
    let pollTimer = null;
    let wasRunning = false;
    let lastOutcomeShownFor = null;

    function el(id) { return document.getElementById(id); }

    function formatBytes(n) {
        const v = Number(n) || 0;
        if (v <= 0) return '—';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        let i = 0;
        let x = v;
        while (x >= 1024 && i < units.length - 1) { x /= 1024; i++; }
        return `${x.toFixed(x >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
    }

    function formatWhen(iso) {
        if (!iso) return 'Never';
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return 'Never';
        const days = Math.floor((Date.now() - d.getTime()) / 86400000);
        const ago = days <= 0 ? 'today' : (days === 1 ? 'yesterday' : `${days} days ago`);
        return `${d.toLocaleString()} (${ago})`;
    }

    function schedule(ms) {
        if (pollTimer) clearTimeout(pollTimer);
        pollTimer = setTimeout(() => { void refresh(); }, ms);
    }

    async function refresh() {
        try {
            const res = await fetch('/api/backup/status', { credentials: 'same-origin' });
            if (res.ok) {
                const data = await res.json();
                available = !!data.available;
                status = data.status || null;
            }
        } catch (_) { /* keep previous */ }
        const running = !!(status && status.running);
        if (wasRunning && !running) onFinished();
        wasRunning = running;
        render();
        schedule(running ? POLL_RUNNING_MS : POLL_IDLE_MS);
    }

    function onFinished() {
        const last = status && status.last;
        if (!last || lastOutcomeShownFor === last.finished_at) return;
        lastOutcomeShownFor = last.finished_at;
        // If the dialog was closed while the backup ran, tell the owner how it went.
        const modal = el('backup-modal');
        if (modal && modal.style.display === 'none' && typeof AppDialogs !== 'undefined') {
            if (last.error) {
                void AppDialogs.showAppAlert('Backup failed', last.error);
            } else {
                void AppDialogs.showAppAlert('Backup complete', `Your archive was saved to:\n${last.path}\n(${formatBytes(last.size_bytes)})`);
            }
        }
    }

    function render() {
        const reminderBtn = el('backup-reminder-btn');
        if (reminderBtn) reminderBtn.hidden = !(isOwner && available && status && status.reminder_due);
        const section = el('account-backup-section');
        if (section) section.style.display = (isOwner && available) ? 'block' : 'none';

        const modal = el('backup-modal');
        if (!modal || modal.style.display === 'none') return;

        const st = status || {};
        el('backup-modal-last').textContent = st.last_backup_at
            ? formatWhen(st.last_backup_at) + (st.last_backup_path ? ` — ${st.last_backup_path}` : '')
            : 'Never';
        el('backup-modal-size').textContent = formatBytes(st.archive_size_bytes);
        el('backup-modal-archive-path').textContent = st.archive_path || '—';

        const running = !!st.running;
        const progress = el('backup-modal-progress');
        progress.hidden = !running;
        if (running) {
            const since = st.running_since ? new Date(st.running_since) : null;
            const secs = since ? Math.max(0, Math.round((Date.now() - since.getTime()) / 1000)) : 0;
            el('backup-modal-progress-text').textContent =
                `Backing up to ${st.running_path || 'the chosen file'}… ${secs}s`;
        }

        const result = el('backup-modal-result');
        const last = st.last;
        if (!running && last && last.finished_at) {
            result.hidden = false;
            result.classList.toggle('backup-modal-result--error', !!last.error);
            result.textContent = last.error
                ? `The last backup failed: ${last.error}`
                : `Backup saved to ${last.path} (${formatBytes(last.size_bytes)}).`;
        } else {
            result.hidden = true;
        }

        el('backup-modal-unavailable').hidden = available;
        const startBtn = el('backup-start-btn');
        startBtn.disabled = !available || running;
        el('backup-snooze-btn').hidden = !(st.reminder_due && !running);
    }

    function openModal() {
        const menu = el('account-dropdown-menu');
        if (menu) menu.style.display = 'none';
        const modal = el('backup-modal');
        if (!modal) return;
        modal.style.display = 'flex';
        render();
        void refresh();
    }

    function closeModal() {
        const modal = el('backup-modal');
        if (modal) modal.style.display = 'none';
    }

    async function chooseDestination() {
        const api = window.electronAPI;
        if (!api || typeof api.showSaveDialog !== 'function') return null;
        const result = await api.showSaveDialog({
            title: 'Save archive backup',
            defaultPath: (status && status.suggested_file_name) || 'archive-backup.sqlite',
            filters: [{ name: 'SQLite Database', extensions: ['sqlite', 'db'] }],
        });
        if (!result || result.canceled || !result.filePath) return null;
        return result.filePath;
    }

    async function start() {
        let dest = await chooseDestination();
        if (!dest) {
            if (!window.electronAPI || typeof window.electronAPI.showSaveDialog !== 'function') {
                await AppDialogs.showAppAlert('Back up your archive', 'Backups can only be made from the Digital Museum desktop app.');
            }
            return;
        }
        if (status && status.archive_path && dest.toLowerCase() === String(status.archive_path).toLowerCase()) {
            await AppDialogs.showAppAlert('Choose a different file', "The backup can't overwrite the archive itself.");
            return;
        }
        try {
            const res = await fetch('/api/backup', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ path: dest }),
            });
            const body = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(body.detail || body.error || `HTTP ${res.status}`);
            dest = body.path || dest;
        } catch (err) {
            await AppDialogs.showAppAlert('Backup could not start', err.message || 'Request failed.');
            return;
        }
        wasRunning = true;
        await refresh();
    }

    async function snooze() {
        try {
            await fetch('/api/backup/snooze', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ days: 7 }),
            });
        } catch (_) { /* ignore */ }
        closeModal();
        await refresh();
    }

    async function loadIdentity() {
        try {
            const res = await fetch('/auth/me', { credentials: 'same-origin' });
            if (!res.ok) return;
            const me = await res.json().catch(() => null);
            isOwner = !!(me && !me.is_visitor);
        } catch (_) {
            isOwner = false;
        }
    }

    async function init() {
        el('account-backup-btn')?.addEventListener('click', (e) => { e.stopPropagation(); openModal(); });
        el('backup-reminder-btn')?.addEventListener('click', openModal);
        el('close-backup-modal')?.addEventListener('click', closeModal);
        el('backup-close-btn')?.addEventListener('click', closeModal);
        el('backup-start-btn')?.addEventListener('click', () => { void start(); });
        el('backup-snooze-btn')?.addEventListener('click', () => { void snooze(); });
        const modal = el('backup-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        await loadIdentity();
        await refresh();
    }

    return { init, open: openModal, refresh };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => { void ArchiveBackup.init(); });
} else {
    void ArchiveBackup.init();
}
