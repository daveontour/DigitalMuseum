'use strict';

// Shared cache of the admin-managed AI model list (Configuration → AI Models tab).
// Mirrors Guide.fetchTopics()'s simple memoize-until-invalidated pattern: the first
// call to ensureLoaded() fetches from the server and caches; invalidateCache() (called
// by modals-ai-models-config.js after every create/update/delete) clears the cache so
// the next ensureLoaded() call re-fetches.
const AIModels = (() => {
    let _models = null; // [{key, display_name, model_slug, enabled, sort_order}, ...], enabled-only, sort_order-ordered
    let _defaultKey = '';
    let _loading = null;

    async function ensureLoaded() {
        if (_models !== null) return _models;
        if (_loading) return _loading;
        _loading = (async () => {
            try {
                const res = await fetch('/api/ai-models/available', { credentials: 'same-origin' });
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                const data = await res.json();
                _models = Array.isArray(data.models) ? data.models : [];
                _defaultKey = data.default_model_key || (_models[0] && _models[0].key) || '';
                if (typeof AIModelLabels !== 'undefined') AIModelLabels.set(_models);
            } catch (e) {
                console.error('AIModels: failed to load models', e);
                _models = [];
                _defaultKey = '';
            } finally {
                _loading = null;
            }
            return _models;
        })();
        return _loading;
    }

    function invalidateCache() {
        _models = null;
        _defaultKey = '';
    }

    /** Synchronous accessor — returns [] until ensureLoaded() has resolved at least once. */
    function cached() {
        return _models || [];
    }

    function defaultKey() {
        return _defaultKey;
    }

    function isValidKey(key) {
        return cached().some((m) => m.key === key);
    }

    function label(key, fallback) {
        const m = cached().find((mm) => mm.key === key);
        return (m && m.display_name) || fallback || key;
    }

    return { ensureLoaded, invalidateCache, cached, defaultKey, isValidKey, label };
})();
