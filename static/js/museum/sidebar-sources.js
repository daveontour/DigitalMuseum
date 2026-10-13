'use strict';

/**
 * Hides left-sidebar "Explore" buttons for sources that have no data yet (e.g. no Facebook
 * import → no Facebook Albums / Posts buttons), using the cheap counts from
 * GET /api/import-modal-stats. If the counts can't be loaded every button stays visible.
 *
 * Refreshes on load, when the Source Data dialog is closed, and when post-import processing
 * finishes (the 'dm:archive-data-changed' event from post-import-status.js).
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const SidebarSources = (() => {
    // Buttons that are always useful regardless of counts are not listed here
    // (Artefacts, Multi Source Search).
    const RULES = [
        { id: 'timeline-sidebar-btn', has: c => c.messages + c.emails + c.images + c.fbPosts > 0 },
        { id: 'sms-messages-sidebar-btn', has: c => c.messages > 0 },
        { id: 'email-gallery-sidebar-btn', has: c => c.emails > 0 },
        { id: 'fb-albums-sidebar-btn', has: c => c.fbAlbums > 0 },
        { id: 'fb-posts-sidebar-btn', has: c => c.fbPosts > 0 },
        { id: 'new-image-gallery-sidebar-btn', has: c => c.images > 0 },
        { id: 'people-in-photos-sidebar-btn', has: c => c.images > 0 },
        { id: 'locations-sidebar-btn', has: c => c.images > 0 || c.locations > 0 },
    ];

    let inFlight = false;

    function sumValues(obj) {
        if (!obj || typeof obj !== 'object') return 0;
        return Object.values(obj).reduce((acc, v) => acc + (Number(v) || 0), 0);
    }

    function countsFrom(d) {
        return {
            messages: sumValues(d.message_counts),
            emails: sumValues(d.emails_by_source),
            fbAlbums: Number(d.facebook_albums_count) || 0,
            fbPosts: Number(d.facebook_posts_count) || 0,
            images: Number(d.total_images) || 0,
            locations: Number(d.locations_count) || 0,
        };
    }

    function setVisible(btn, visible) {
        btn.classList.toggle('sidebar-source-empty', !visible);
        btn.setAttribute('aria-hidden', visible ? 'false' : 'true');
    }

    function apply(counts) {
        let anyDataSource = false;
        RULES.forEach(rule => {
            const btn = document.getElementById(rule.id);
            if (!btn) return;
            const visible = counts ? !!rule.has(counts) : true;
            if (visible && counts) anyDataSource = true;
            setVisible(btn, visible);
        });
        const hint = document.getElementById('sidebar-explore-empty-hint');
        if (hint) hint.hidden = !counts || anyDataSource;
    }

    async function refresh() {
        if (inFlight) return;
        inFlight = true;
        try {
            const res = await fetch('/api/import-modal-stats', { credentials: 'same-origin' });
            if (!res.ok) {
                apply(null);
                return;
            }
            const d = await res.json();
            apply(d && typeof d === 'object' ? countsFrom(d) : null);
        } catch (_) {
            apply(null);
        } finally {
            inFlight = false;
        }
    }

    function init() {
        void refresh();
        document.getElementById('close-data-import-modal')?.addEventListener('click', () => { void refresh(); });
        document.addEventListener('dm:archive-data-changed', () => { void refresh(); });
    }

    return { init, refresh };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => SidebarSources.init());
} else {
    SidebarSources.init();
}
