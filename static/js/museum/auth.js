const AuthModule = (() => {
    let currentUser = null; // { id, email, display_name }

    async function init() {
        try {
            const res = await fetch('/auth/me', { credentials: 'same-origin' });
            if (res.ok) {
                currentUser = await res.json();
                renderAccountUI();
            } else if (res.status === 401) {
                // Not authenticated — redirect to login page.
                window.location.href = '/login';
            } else {
                // Other error — hide account UI silently.
                const dropdown = document.getElementById('account-dropdown');
                if (dropdown) dropdown.style.display = 'none';
            }
        } catch (err) {
            // Network error — silently ignore, hide account UI
            const dropdown = document.getElementById('account-dropdown');
            if (dropdown) dropdown.style.display = 'none';
        }
    }

    function renderAccountUI() {
        const dropdown = document.getElementById('account-dropdown');
        if (!dropdown) return;

        // Show the account dropdown
        dropdown.style.display = '';

        // Set display name — visitors see "Visitor" rather than the archive owner's name
        const nameEl = document.getElementById('account-display-name');
        if (nameEl && currentUser) {
            nameEl.textContent = currentUser.is_visitor ? 'Visitor' : (currentUser.display_name || currentUser.email || '');
        }

        // Wire dropdown trigger toggle
        const trigger = document.getElementById('account-dropdown-trigger');
        if (trigger) {
            trigger.addEventListener('click', (e) => {
                e.stopPropagation();
                const menu = document.getElementById('account-dropdown-menu');
                if (menu) menu.style.display = menu.style.display === 'none' ? 'block' : 'none';
            });
            document.addEventListener('click', () => {
                const menu = document.getElementById('account-dropdown-menu');
                if (menu) menu.style.display = 'none';
            });
        }

        const billSec = document.getElementById('account-billing-section');
        if (billSec) {
            billSec.style.display = (currentUser && !currentUser.is_visitor) ? 'block' : 'none';
        }

        const curBill = document.getElementById('account-billing-current-btn');
        const prevBill = document.getElementById('account-billing-previous-btn');
        if (curBill && !curBill.dataset.wired) {
            curBill.dataset.wired = '1';
            curBill.addEventListener('click', (e) => {
                e.stopPropagation();
                window.location.href = '/api/llm-usage/me/bill.pdf?period=current';
            });
        }
        if (prevBill && !prevBill.dataset.wired) {
            prevBill.dataset.wired = '1';
            prevBill.addEventListener('click', (e) => {
                e.stopPropagation();
                window.location.href = '/api/llm-usage/me/bill.pdf?period=previous';
            });
        }

        const adminSec = document.getElementById('account-admin-section');
        if (adminSec) {
            // Same scope as billing PDFs: signed-in archive owner, not a visitor session.
            // /admin uses its own cookie + credentials — DB is_admin is not required to open it.
            adminSec.style.display = (currentUser && !currentUser.is_visitor) ? 'block' : 'none';
        }
        const adminBtn = document.getElementById('account-admin-btn');
        if (adminBtn && !adminBtn.dataset.wired) {
            adminBtn.dataset.wired = '1';
            adminBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                window.location.href = '/admin';
            });
        }

        const guideReloadSec = document.getElementById('account-guide-reload-section');
        const guideReloadEnabled = typeof CONSTANTS !== 'undefined'
            && CONSTANTS.GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP === 'True';
        if (guideReloadSec) {
            guideReloadSec.style.display = (guideReloadEnabled && currentUser && !currentUser.is_visitor) ? 'block' : 'none';
        }
        const guideReloadBtn = document.getElementById('account-reload-guide-topics-btn');
        if (guideReloadBtn && !guideReloadBtn.dataset.wired) {
            guideReloadBtn.dataset.wired = '1';
            guideReloadBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                void reloadGuideTopicsFromFile();
            });
        }

        // Wire about button
        const aboutBtn = document.getElementById('account-about-btn');
        if (aboutBtn && !aboutBtn.dataset.wired) {
            aboutBtn.dataset.wired = '1';
            aboutBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                void openAboutModal();
            });
        }

        // Wire logout button
        const logoutBtn = document.getElementById('account-logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', logout);
        }
    }

    async function resolveAppInfo() {
        if (window.electronAPI && typeof window.electronAPI.getAppInfo === 'function') {
            try {
                const info = await window.electronAPI.getAppInfo();
                if (info) return info;
            } catch (err) {
                console.debug('getAppInfo failed', err);
            }
        }
        return {
            name: 'Digital Museum',
            version: 'dev',
            description: 'AI-powered personal digital archive. Import emails, messages, photos, and documents, then explore them through an AI chat interface.',
        };
    }

    function closeAboutModal() {
        const modal = document.getElementById('about-modal');
        if (modal) {
            modal.style.display = 'none';
            modal.classList.remove('about-modal-open');
        }
        const menu = document.getElementById('account-dropdown-menu');
        if (menu) menu.style.display = 'none';
    }

    async function openAboutModal() {
        const modal = document.getElementById('about-modal');
        const nameEl = document.getElementById('about-app-name');
        const versionEl = document.getElementById('about-app-version');
        const descEl = document.getElementById('about-app-description');
        if (!modal || !nameEl || !versionEl || !descEl) return;

        const menu = document.getElementById('account-dropdown-menu');
        if (menu) menu.style.display = 'none';

        nameEl.textContent = 'Digital Museum';
        versionEl.textContent = 'Version …';
        descEl.textContent = 'Loading…';
        modal.classList.add('about-modal-open');
        modal.style.display = 'flex';

        const info = await resolveAppInfo();
        const rawName = (info && info.name) ? String(info.name) : 'Digital Museum';
        const appName = rawName.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
        const version = (info && info.version) ? String(info.version) : 'dev';
        const description = (info && info.description)
            ? String(info.description)
            : 'AI-powered personal digital archive.';

        nameEl.textContent = appName;
        versionEl.textContent = `Version ${version}`;
        descEl.textContent = description;
    }

    function initAboutModal() {
        const modal = document.getElementById('about-modal');
        if (!modal || modal.dataset.wired) return;
        modal.dataset.wired = '1';

        modal.addEventListener('click', (e) => {
            if (e.target === modal) closeAboutModal();
        });
        document.getElementById('about-modal-close-btn')?.addEventListener('click', closeAboutModal);
        document.getElementById('about-modal-ok-btn')?.addEventListener('click', closeAboutModal);
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && modal.style.display === 'flex') {
                closeAboutModal();
            }
        });
    }

    async function logout() {
        try {
            await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
        } catch (err) {
            // Ignore errors — redirect regardless
        }
        window.location.href = '/login';
    }

    async function reloadGuideTopicsFromFile() {
        const message = 'Delete all guide topics in the database and reload from guide_topics.json?\n\nAny topics edited in Configuration will be replaced with the seed file contents.';
        let confirmed = false;
        if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppConfirm) {
            confirmed = await AppDialogs.showAppConfirm(
                'Reload guide topics?',
                message,
                { danger: true, confirmLabel: 'Reload' }
            );
        } else {
            confirmed = window.confirm(`Reload guide topics?\n\n${message}`);
        }
        if (!confirmed) return;

        const menu = document.getElementById('account-dropdown-menu');
        if (menu) menu.style.display = 'none';

        try {
            const response = await fetch('/api/guide-topics/reload-from-file', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Accept': 'application/json' },
            });
            const data = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(data.error || data.detail || `HTTP ${response.status}`);
            }
            if (typeof Guide !== 'undefined' && Guide.invalidateCache) {
                Guide.invalidateCache();
            }
            if (typeof Modals !== 'undefined' && Modals.GuideTopicsConfig && Modals.GuideTopicsConfig.load) {
                void Modals.GuideTopicsConfig.load();
            }
            if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
                await AppDialogs.showAppAlert('Guide topics reloaded from guide_topics.json.');
            }
        } catch (err) {
            if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
                await AppDialogs.showAppAlert('Failed to reload guide topics: ' + err.message);
            } else {
                window.alert('Failed to reload guide topics: ' + err.message);
            }
        }
    }

    function getUser() {
        return currentUser;
    }

    // Self-initialize (loaded after app.js, so Modals.initAll() has already run)
    initAboutModal();
    init();

    return { init, getUser, logout };
})();
