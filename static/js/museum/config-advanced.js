'use strict';

/**
 * Configuration → "Show advanced settings" switch.
 *
 * Everyday tabs (Appearance, API Keys, AI Models, Local AI Setup, Subject Configuration, Manage
 * Visitor Keys) are always shown; tabs marked `.config-tab-advanced` in the template (model
 * catalog, editors, developer and diagnostic tools) are hidden until the switch is on. The choice
 * is remembered per browser (localStorage — a viewer convenience only).
 *
 * Anything that opens an advanced tab directly (a link elsewhere in the app, or a guide step)
 * still works: the tabs are revealed for this visit without changing the remembered choice.
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const ConfigAdvanced = (() => {
    const STORAGE_KEY = 'dm.config.showAdvanced.v1';
    const BASIC_CLASS = 'config-tabs-sidebar--basic';

    function sidebar() { return document.querySelector('#config-modal-overlay .config-tabs-sidebar'); }
    function checkbox() { return document.getElementById('config-show-advanced-checkbox'); }

    function readPref() {
        try { return localStorage.getItem(STORAGE_KEY) === '1'; } catch (_) { return false; }
    }

    function writePref(on) {
        try { localStorage.setItem(STORAGE_KEY, on ? '1' : '0'); } catch (_) { /* not persisted */ }
    }

    function isShown() {
        const sb = sidebar();
        return !!sb && !sb.classList.contains(BASIC_CLASS);
    }

    function apply(on) {
        const sb = sidebar();
        if (!sb) return;
        sb.classList.toggle(BASIC_CLASS, !on);
        const cb = checkbox();
        if (cb) cb.checked = on;
        if (!on) {
            // Don't leave the user looking at a tab whose button just disappeared.
            const active = sb.querySelector('.config-tab-button.config-tab-advanced.active');
            if (active) sb.querySelector('.config-tab-button[data-tab="settings"]')?.click();
        }
    }

    /** Shows the advanced tabs for this visit if `target` is (or is inside) an advanced tab or
     *  its content. Used by guide.js before it highlights or clicks something. */
    function reveal(target) {
        if (!target || isShown()) return;
        const sb = sidebar();
        let advanced = !!target.closest?.('.config-tab-advanced');
        if (!advanced && sb) {
            const content = target.closest?.('.config-tab-content');
            const tab = content && content.id ? content.id.replace(/-tab$/, '') : '';
            advanced = !!(tab && sb.querySelector(`.config-tab-button.config-tab-advanced[data-tab="${tab}"]`));
        }
        if (advanced) apply(true);
    }

    function init() {
        const sb = sidebar();
        const cb = checkbox();
        if (!sb || !cb) return;
        apply(readPref());
        cb.addEventListener('change', () => {
            writePref(cb.checked);
            apply(cb.checked);
        });
        // Capture phase, so this runs before app.js's per-button handler (which ignores clicks
        // on buttons that are hidden): opening an advanced tab from elsewhere reveals the group.
        sb.addEventListener('click', (e) => {
            const btn = e.target.closest('.config-tab-button.config-tab-advanced');
            if (btn && !isShown()) apply(true);
        }, true);
    }

    return { init, reveal, isShown };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => ConfigAdvanced.init());
} else {
    ConfigAdvanced.init();
}
