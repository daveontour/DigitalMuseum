'use strict';

/**
 * Quick search (Ctrl+K / ⌘K, or the top-bar Search button): one box that finds words across
 * messages, emails, photo titles/descriptions/tags and Facebook posts, newest first.
 * Data: GET /api/quick-search?q= (internal/handler/timeline_handler.go).
 *
 * Results open in their existing viewers via Chat.openSource (chat.js).
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const QuickSearch = (() => {
    const DEBOUNCE_MS = 250;
    const KINDS = {
        message:       { icon: 'fa-comment-dots', label: 'Messages' },
        email:         { icon: 'fa-envelope',     label: 'Emails' },
        photo:         { icon: 'fa-image',        label: 'Photos' },
        facebook_post: { icon: 'fa-newspaper',    label: 'Facebook posts' },
    };
    const ORDER = ['message', 'email', 'photo', 'facebook_post'];

    let debounceTimer = null;
    let controller = null;
    let hits = [];
    let active = -1;
    let lastFocus = null;

    function el(id) { return document.getElementById(id); }

    function isOpen() {
        const m = el('quick-search-modal');
        return !!(m && !m.hidden);
    }

    function open() {
        const m = el('quick-search-modal');
        const input = el('quick-search-input');
        if (!m || !input) return;
        lastFocus = document.activeElement;
        m.hidden = false;
        input.focus();
        input.select();
        if (!input.value.trim()) renderHint();
    }

    function close() {
        const m = el('quick-search-modal');
        if (!m) return;
        m.hidden = true;
        if (controller) controller.abort();
        if (lastFocus && typeof lastFocus.focus === 'function' && document.contains(lastFocus)) {
            lastFocus.focus();
        }
    }

    function renderHint(text) {
        const box = el('quick-search-results');
        box.innerHTML = '';
        const p = document.createElement('p');
        p.className = 'quick-search-hint';
        p.textContent = text || 'Type at least two letters — a name, a place, a word you remember.';
        box.appendChild(p);
        hits = [];
        active = -1;
    }

    function schedule() {
        if (debounceTimer) clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => { void run(); }, DEBOUNCE_MS);
    }

    async function run() {
        const q = el('quick-search-input').value.trim();
        if (q.length < 2) {
            if (controller) controller.abort();
            renderHint();
            return;
        }
        if (controller) controller.abort();
        controller = new AbortController();
        const box = el('quick-search-results');
        box.setAttribute('aria-busy', 'true');
        try {
            const res = await fetch(`/api/quick-search?q=${encodeURIComponent(q)}`, {
                credentials: 'same-origin',
                signal: controller.signal,
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            if (el('quick-search-input').value.trim() !== q) return; // stale
            render(Array.isArray(data.hits) ? data.hits : [], data.hidden || []);
        } catch (err) {
            if (err && err.name === 'AbortError') return;
            renderHint(`Search failed: ${err.message}`);
        } finally {
            box.removeAttribute('aria-busy');
        }
    }

    function render(list, hidden) {
        const box = el('quick-search-results');
        box.innerHTML = '';
        hits = [];
        active = -1;
        if (list.length === 0) {
            renderHint('No matches. Try a different word, or ask the AI in the chat — it can search by meaning.');
            return;
        }
        ORDER.forEach(kind => {
            const group = list.filter(h => h.kind === kind);
            if (!group.length) return;
            const head = document.createElement('div');
            head.className = 'quick-search-group';
            head.textContent = KINDS[kind].label;
            box.appendChild(head);
            group.forEach(h => {
                const idx = hits.length;
                hits.push(h);
                const btn = document.createElement('button');
                btn.type = 'button';
                btn.className = 'quick-search-hit';
                btn.id = `quick-search-hit-${idx}`;
                btn.setAttribute('role', 'option');
                btn.setAttribute('aria-selected', 'false');
                if (kind === 'photo') {
                    const img = document.createElement('img');
                    img.className = 'quick-search-thumb';
                    img.loading = 'lazy';
                    img.alt = '';
                    img.src = `/images/${encodeURIComponent(h.id)}?type=metadata&preview=true`;
                    img.onerror = function () { this.remove(); };
                    btn.appendChild(img);
                } else {
                    const icon = document.createElement('i');
                    icon.className = `fas ${KINDS[kind].icon} quick-search-hit-icon`;
                    icon.setAttribute('aria-hidden', 'true');
                    btn.appendChild(icon);
                }
                const text = document.createElement('span');
                text.className = 'quick-search-hit-text';
                const title = document.createElement('span');
                title.className = 'quick-search-hit-title';
                title.textContent = h.title || KINDS[kind].label;
                text.appendChild(title);
                if (h.snippet) {
                    const sn = document.createElement('span');
                    sn.className = 'quick-search-hit-snippet';
                    sn.textContent = h.snippet;
                    text.appendChild(sn);
                }
                btn.appendChild(text);
                if (h.date) {
                    const date = document.createElement('span');
                    date.className = 'quick-search-hit-date';
                    date.textContent = h.date;
                    btn.appendChild(date);
                }
                btn.addEventListener('mousemove', () => setActive(idx));
                btn.addEventListener('click', () => choose(idx));
                box.appendChild(btn);
            });
        });
        if (hidden.length) {
            const p = document.createElement('p');
            p.className = 'quick-search-hint';
            p.textContent = `Your visitor key doesn't include ${hidden.join(' or ')}, so they weren't searched.`;
            box.appendChild(p);
        }
        setActive(0);
    }

    function setActive(idx) {
        if (!hits.length) return;
        const prev = el(`quick-search-hit-${active}`);
        if (prev) {
            prev.classList.remove('quick-search-hit--active');
            prev.setAttribute('aria-selected', 'false');
        }
        active = Math.max(0, Math.min(hits.length - 1, idx));
        const cur = el(`quick-search-hit-${active}`);
        if (cur) {
            cur.classList.add('quick-search-hit--active');
            cur.setAttribute('aria-selected', 'true');
            cur.scrollIntoView({ block: 'nearest' });
            el('quick-search-input').setAttribute('aria-activedescendant', cur.id);
        }
    }

    function choose(idx) {
        const h = hits[idx];
        if (!h) return;
        close();
        if (typeof Chat !== 'undefined' && typeof Chat.openSource === 'function') {
            void Chat.openSource(h.kind, h.id);
        }
    }

    function onInputKeydown(e) {
        if (e.key === 'ArrowDown') { e.preventDefault(); setActive(active + 1); }
        else if (e.key === 'ArrowUp') { e.preventDefault(); setActive(active - 1); }
        else if (e.key === 'Enter') { e.preventDefault(); if (active >= 0) choose(active); }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); }
    }

    function onGlobalKeydown(e) {
        const isK = e.key === 'k' || e.key === 'K';
        if (isK && (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey) {
            e.preventDefault();
            if (isOpen()) close(); else open();
        } else if (e.key === 'Escape' && isOpen()) {
            e.preventDefault();
            close();
        }
    }

    function init() {
        el('quick-search-btn')?.addEventListener('click', open);
        const input = el('quick-search-input');
        input?.addEventListener('input', schedule);
        input?.addEventListener('keydown', onInputKeydown);
        const m = el('quick-search-modal');
        m?.addEventListener('mousedown', (e) => { if (e.target === m) close(); });
        document.addEventListener('keydown', onGlobalKeydown);
        if (/Mac|iPhone|iPad/.test(navigator.platform || '')) {
            document.querySelectorAll('#quick-search-btn .quick-search-kbd').forEach(k => { k.textContent = '⌘K'; });
        }
    }

    return { init, open, close };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => QuickSearch.init());
} else {
    QuickSearch.init();
}
