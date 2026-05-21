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

        // Wire logout button
        const logoutBtn = document.getElementById('account-logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', logout);
        }
    }

    async function logout() {
        try {
            await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
        } catch (err) {
            // Ignore errors — redirect regardless
        }
        window.location.href = '/login';
    }

    function getUser() {
        return currentUser;
    }

    // Self-initialize (loaded after app.js, so Modals.initAll() has already run)
    init();

    return { init, getUser, logout };
})();
