'use strict';

/**
 * On This Day — browse memories across previous years that happened on a specific calendar day.
 * Data: GET /api/on-this-day?date=YYYY-MM-DD (internal/handler/memories_handler.go).
 *
 * Items open in their existing viewers via Chat.openSource (chat.js).
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const OnThisDay = (() => {
    let currentDate = new Date();
    let loadGen = 0;

    const MONTH_NAMES = ['January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'];

    function el(id) { return document.getElementById(id); }

    function formatISODate(d) {
        const y = d.getFullYear();
        const m = String(d.getMonth() + 1).padStart(2, '0');
        const day = String(d.getDate()).padStart(2, '0');
        return `${y}-${m}-${day}`;
    }

    function plural(n, one, many) {
        return `${n.toLocaleString()} ${n === 1 ? one : many}`;
    }

    function openItem(kind, id) {
        if (typeof Chat !== 'undefined' && typeof Chat.openSource === 'function') {
            void Chat.openSource(kind, id);
        }
    }

    async function errorText(res) {
        try {
            const body = await res.json();
            if (body && (body.detail || body.error)) return body.detail || body.error;
        } catch (_) { /* not JSON */ }
        return `HTTP ${res.status}`;
    }

    function emptyMessage(text) {
        const p = document.createElement('p');
        p.className = 'timeline-empty';
        p.textContent = text;
        return p;
    }

    function setDate(d) {
        currentDate = new Date(d);
        const picker = el('on-this-day-date-picker');
        if (picker) {
            picker.value = formatISODate(currentDate);
        }
        void load();
    }

    function stepDay(delta) {
        const d = new Date(currentDate);
        d.setDate(d.getDate() + delta);
        setDate(d);
    }

    function goToday() {
        setDate(new Date());
    }

    async function open(optDate) {
        const modal = el('on-this-day-modal');
        if (!modal) return;
        modal.style.display = 'flex';
        if (optDate) {
            const parsed = new Date(optDate);
            if (!Number.isNaN(parsed.getTime())) {
                currentDate = parsed;
            }
        }
        const picker = el('on-this-day-date-picker');
        if (picker) {
            picker.value = formatISODate(currentDate);
        }
        void load();
    }

    function close() {
        const modal = el('on-this-day-modal');
        if (modal) modal.style.display = 'none';
    }

    async function load() {
        const gen = ++loadGen;
        const container = el('on-this-day-years');
        const titleEl = el('on-this-day-title');
        const summaryEl = el('on-this-day-summary');

        const m = currentDate.getMonth();
        const day = currentDate.getDate();
        const dateStr = `${MONTH_NAMES[m]} ${day}`;

        if (titleEl) titleEl.textContent = `${dateStr} across the years`;
        if (summaryEl) summaryEl.textContent = 'Searching your archive…';
        if (container) {
            container.innerHTML = '<p class="timeline-empty"><i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Loading memories…</p>';
        }

        const iso = formatISODate(currentDate);
        let data;
        try {
            const res = await fetch(`/api/on-this-day?date=${encodeURIComponent(iso)}`, { credentials: 'same-origin' });
            if (!res.ok) throw new Error(await errorText(res));
            data = await res.json();
        } catch (err) {
            if (gen !== loadGen) return;
            if (container) {
                container.innerHTML = '';
                container.appendChild(emptyMessage(`Couldn't load memories: ${err.message}`));
            }
            if (summaryEl) summaryEl.textContent = '';
            return;
        }

        if (gen !== loadGen) return;
        render(data, dateStr);
    }

    function render(data, dateStr) {
        const container = el('on-this-day-years');
        const summaryEl = el('on-this-day-summary');
        if (!container) return;
        container.innerHTML = '';

        const years = Array.isArray(data.years) ? data.years : [];
        const totalItems = data.total || 0;

        if (summaryEl) {
            if (totalItems > 0) {
                summaryEl.textContent = `${plural(totalItems, 'memory', 'memories')} across ${plural(years.length, 'year', 'years')}`;
            } else {
                summaryEl.textContent = 'No memories found for this day';
            }
        }

        if (years.length === 0) {
            const emptyWrap = document.createElement('div');
            emptyWrap.className = 'on-this-day-empty-wrap';
            emptyWrap.innerHTML = `
                <div class="on-this-day-empty-icon"><i class="fas fa-calendar-times"></i></div>
                <h3>No memories recorded on ${dateStr}</h3>
                <p>Nothing with this calendar date was found in previous years. Try stepping to yesterday or tomorrow, or pick another date above!</p>
            `;
            container.appendChild(emptyWrap);
            return;
        }

        years.forEach(y => {
            const block = renderYearBlock(y);
            container.appendChild(block);
        });

        if (Array.isArray(data.hidden) && data.hidden.length) {
            container.appendChild(emptyMessage(`Your visitor key doesn't include ${data.hidden.join(' or ')}, so they aren't shown.`));
        }
    }

    function renderYearBlock(y) {
        const section = document.createElement('section');
        section.className = 'on-this-day-year-block';

        const header = document.createElement('div');
        header.className = 'on-this-day-year-header';

        const badge = document.createElement('div');
        badge.className = 'on-this-day-year-badge';
        badge.textContent = y.year;

        const ago = document.createElement('div');
        ago.className = 'on-this-day-year-ago';
        ago.textContent = y.years_ago === 1 ? '1 year ago' : `${y.years_ago} years ago`;

        const stats = document.createElement('div');
        stats.className = 'on-this-day-year-stats';
        const parts = [];
        const t = y.totals || {};
        if (t.photos) parts.push(plural(t.photos, 'photo', 'photos'));
        if (t.messages) parts.push(plural(t.messages, 'message', 'messages'));
        if (t.emails) parts.push(plural(t.emails, 'email', 'emails'));
        if (t.posts) parts.push(plural(t.posts, 'post', 'posts'));
        stats.textContent = parts.join(' · ');

        header.appendChild(badge);
        header.appendChild(ago);
        header.appendChild(stats);
        section.appendChild(header);

        // Content items
        const list = document.createElement('div');
        list.className = 'timeline-day-items';

        (y.conversations || []).forEach(c => {
            list.appendChild(itemButton('message', c.first_message_id, 'fa-comment-dots',
                c.chat_session || 'Conversation', plural(c.count, 'message', 'messages')));
        });
        (y.emails || []).forEach(e => {
            list.appendChild(itemButton('email', e.id, 'fa-envelope',
                e.subject || '(no subject)', [e.from, e.time].filter(Boolean).join(' · ')));
        });
        (y.posts || []).forEach(p => {
            list.appendChild(itemButton('facebook_post', p.id, 'fa-newspaper',
                p.title || 'Facebook post', p.text || ''));
        });
        if (list.childElementCount) section.appendChild(list);

        // Photos
        if (Array.isArray(y.photos) && y.photos.length) {
            const strip = document.createElement('div');
            strip.className = 'timeline-photos';
            y.photos.forEach(p => {
                const btn = document.createElement('button');
                btn.type = 'button';
                btn.className = 'timeline-photo';
                btn.id = `on-this-day-photo-${p.id}`;
                btn.title = p.title || 'Photo';
                const img = document.createElement('img');
                img.loading = 'lazy';
                img.alt = p.title || 'Photo';
                img.src = `/images/${encodeURIComponent(p.id)}?type=metadata&preview=true`;
                img.onerror = function () { this.style.visibility = 'hidden'; };
                btn.appendChild(img);
                btn.addEventListener('click', () => openItem('photo', p.id));
                strip.appendChild(btn);
            });
            section.appendChild(strip);
        }

        return section;
    }

    function itemButton(kind, id, iconCls, label, detail) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'timeline-item';
        btn.id = `on-this-day-item-${kind}-${id}`;
        const icon = document.createElement('i');
        icon.className = `fas ${iconCls} timeline-item-icon`;
        icon.setAttribute('aria-hidden', 'true');
        const text = document.createElement('span');
        text.className = 'timeline-item-text';
        const l = document.createElement('span');
        l.className = 'timeline-item-label';
        l.textContent = label;
        text.appendChild(l);
        if (detail) {
            const dt = document.createElement('span');
            dt.className = 'timeline-item-detail';
            dt.textContent = detail;
            text.appendChild(dt);
        }
        btn.appendChild(icon);
        btn.appendChild(text);
        btn.addEventListener('click', () => openItem(kind, id));
        return btn;
    }

    function onKeydown(e) {
        const modal = el('on-this-day-modal');
        if (!modal || modal.style.display === 'none') return;
        const tag = (e.target && e.target.tagName) || '';
        if (tag === 'INPUT' || tag === 'TEXTAREA') return;
        const top = document.elementFromPoint(window.innerWidth / 2, 10);
        if (top && !modal.contains(top) && top !== modal) return;
        if (e.key === 'ArrowLeft') { e.preventDefault(); stepDay(-1); }
        if (e.key === 'ArrowRight') { e.preventDefault(); stepDay(1); }
    }

    function init() {
        el('on-this-day-sidebar-btn')?.addEventListener('click', () => { void open(); });
        el('close-on-this-day-modal')?.addEventListener('click', close);
        el('on-this-day-prev-btn')?.addEventListener('click', () => stepDay(-1));
        el('on-this-day-next-btn')?.addEventListener('click', () => stepDay(1));
        el('on-this-day-today-btn')?.addEventListener('click', goToday);
        el('on-this-day-date-picker')?.addEventListener('change', (e) => {
            if (e.target.value) setDate(new Date(e.target.value + 'T00:00:00'));
        });
        const modal = el('on-this-day-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) close(); });
        document.addEventListener('keydown', onKeydown);
    }

    return { init, open, close };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => OnThisDay.init());
} else {
    OnThisDay.init();
}
