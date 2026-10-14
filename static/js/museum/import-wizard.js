'use strict';

/**
 * Source Data → Data Import: "What would you like to add?" picker.
 *
 * The tab opens on a short list of source tiles instead of every import card. Picking a tile
 * shows "How do I get this export?" steps for that source and only the existing import card(s)
 * for it — the cards themselves (buttons, counts, progress, Cancel, Clear Data) are unchanged and
 * still wired by app.js. "Show all import options" shows every card, as before.
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const ImportWizard = (() => {
    const SOURCES = {
        email:     { title: 'Email',            cards: ['[data-import="email_processing"]', '[data-import="imap_processing"]'] },
        whatsapp:  { title: 'WhatsApp',         cards: ['[data-import="upload_zip"][data-zip-archive-type="whatsapp"]'] },
        facebook:  { title: 'Facebook',         cards: ['[data-import="upload_zip"][data-zip-archive-type="facebook"]'] },
        instagram: { title: 'Instagram',        cards: ['[data-import="upload_zip"][data-zip-archive-type="instagram"]'] },
        imessage:  { title: 'iMessage & SMS',   cards: ['[data-import="upload_zip"][data-zip-archive-type="imessage"]'] },
        photos:    { title: 'Photos',           cards: ['[data-import="upload_photos"]', '[data-import="filesystem"]'] },
        documents: { title: 'Documents',        cards: ['[data-import="reference_modal"]'], howtoSummary: 'What can I add?' },
        other:     { title: 'Other data sources', cards: ['[data-import="other_data_sources"]'], howtoSummary: 'Something else?' },
    };
    const MODE_PICK = 'pick';
    const MODE_SOURCE = 'source';
    const MODE_ALL = 'all';

    let mode = MODE_PICK;
    let current = null;

    function el(id) { return document.getElementById(id); }
    function grid() { return el('data-import-import-cards-grid'); }

    function cardsFor(key) {
        const g = grid();
        const src = SOURCES[key];
        if (!g || !src) return [];
        return src.cards.flatMap(sel => Array.from(g.querySelectorAll(`.data-import-card${sel}`)));
    }

    function render() {
        const g = grid();
        const pick = el('import-wizard-pick');
        const source = el('import-wizard-source');
        if (!g || !pick || !source) return;

        pick.hidden = mode !== MODE_PICK;
        source.hidden = mode !== MODE_SOURCE;
        g.classList.toggle('import-wizard-grid--hidden', mode === MODE_PICK);
        g.classList.toggle('import-wizard-grid--filtered', mode === MODE_SOURCE);

        const matches = new Set(mode === MODE_SOURCE ? cardsFor(current) : []);
        g.querySelectorAll('.data-import-card').forEach(card => {
            card.classList.toggle('import-wizard-card--match', matches.has(card));
        });

        if (mode === MODE_SOURCE) {
            const src = SOURCES[current];
            el('import-wizard-source-title').textContent = src.title;
            el('import-wizard-howto-summary').textContent = src.howtoSummary || 'How do I get this export?';
            document.querySelectorAll('#import-wizard-howto [data-wizard-howto]').forEach(body => {
                body.hidden = body.getAttribute('data-wizard-howto') !== current;
            });
            el('import-wizard-howto').open = true;
        }

        const backBar = el('import-wizard-all-bar');
        if (backBar) backBar.hidden = mode !== MODE_ALL;
    }

    function choose(key) {
        if (!SOURCES[key]) return;
        mode = MODE_SOURCE;
        current = key;
        render();
        el('import-wizard-back-btn')?.focus();
        scrollToTop();
    }

    function showPicker() {
        mode = MODE_PICK;
        current = null;
        render();
        updateCounts();
        scrollToTop();
        el('import-wizard-tile-email')?.focus();
    }

    /** Shows every import card (the pre-wizard layout). Also used by guide.js so guide steps
     *  that highlight or click a specific card always find it visible. */
    function showAll() {
        mode = MODE_ALL;
        current = null;
        render();
    }

    /** Makes sure an element inside the import cards grid is visible (used by guide.js before it
     *  highlights or clicks something there): switches to that card's source, or to "all". */
    function reveal(target) {
        const g = grid();
        const card = target && g && g.contains(target) ? target.closest('.data-import-card') : null;
        if (!card) return;
        if (mode === MODE_ALL || (mode === MODE_SOURCE && card.classList.contains('import-wizard-card--match'))) return;
        const key = Object.keys(SOURCES).find(k => cardsFor(k).includes(card));
        if (key) {
            mode = MODE_SOURCE;
            current = key;
            render();
        } else {
            showAll();
        }
    }

    function scrollToTop() {
        const scroller = document.querySelector('#data-import-modal .data-import-panels-scroll');
        if (scroller) scroller.scrollTop = 0;
    }

    // Tiles show what's already been imported for that source, read from the cards' own counts
    // (filled in by app.js from /api/import-modal-stats).
    function updateCounts() {
        Object.keys(SOURCES).forEach(key => {
            const out = el(`import-wizard-count-${key}`);
            if (!out) return;
            let known = false;
            let total = 0;
            cardsFor(key).forEach(card => {
                card.querySelectorAll('.data-import-count').forEach(span => {
                    const n = Number(String(span.textContent).replace(/[^\d]/g, ''));
                    if (span.textContent.trim() !== '—' && span.textContent.trim() !== '' && !Number.isNaN(n)) {
                        known = true;
                        total += n;
                    }
                });
            });
            const tile = out.closest('.import-wizard-tile');
            if (!known) {
                out.textContent = '';
                tile?.classList.remove('import-wizard-tile--has-data');
                return;
            }
            out.textContent = total > 0 ? `${total.toLocaleString()} already added` : 'Not added yet';
            tile?.classList.toggle('import-wizard-tile--has-data', total > 0);
        });
    }

    function addAllModeBar() {
        // A small "back to the simple view" bar shown above the cards in "all" mode.
        const g = grid();
        if (!g || el('import-wizard-all-bar')) return;
        const bar = document.createElement('div');
        bar.id = 'import-wizard-all-bar';
        bar.className = 'import-wizard-all-bar';
        bar.hidden = true;
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.id = 'import-wizard-simple-view-btn';
        btn.className = 'import-wizard-link';
        btn.innerHTML = '<i class="fas fa-th-large" aria-hidden="true"></i> Back to the simple view';
        btn.addEventListener('click', showPicker);
        bar.appendChild(btn);
        g.parentNode.insertBefore(bar, g);
    }

    function init() {
        const wizard = el('import-wizard');
        if (!wizard || !grid()) return;
        addAllModeBar();
        wizard.querySelectorAll('[data-wizard-source]').forEach(btn => {
            btn.addEventListener('click', () => choose(btn.getAttribute('data-wizard-source')));
        });
        el('import-wizard-back-btn')?.addEventListener('click', showPicker);
        el('import-wizard-show-all-btn')?.addEventListener('click', showAll);

        let pending = false;
        const observer = new MutationObserver(() => {
            if (pending) return;
            pending = true;
            requestAnimationFrame(() => { pending = false; updateCounts(); });
        });
        grid().querySelectorAll('.data-import-count').forEach(span => {
            observer.observe(span, { childList: true, characterData: true, subtree: true });
        });

        render();
        updateCounts();
    }

    return { init, showAll, showPicker, choose, reveal };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => ImportWizard.init());
} else {
    ImportWizard.init();
}
