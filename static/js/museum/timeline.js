'use strict';

/**
 * Timeline — browse the whole archive by year → month → day, mixing conversations, emails,
 * photos and Facebook posts. Data: GET /api/timeline (per-month counts) and
 * GET /api/timeline/{year}/{month} (internal/handler/timeline_handler.go).
 *
 * Items open in their existing viewers via Chat.openSource (chat.js).
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const Timeline = (() => {
    const MONTH_NAMES = ['January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'];
    const MONTH_SHORT = MONTH_NAMES.map(m => m.slice(0, 3));

    let months = [];          // [{month:"YYYY-MM", messages, emails, photos, posts}]
    let byMonth = new Map();
    let selectedYear = null;
    let selectedMonth = null; // "YYYY-MM"
    let loadGen = 0;

    function el(id) { return document.getElementById(id); }

    function total(m) {
        return m ? (m.messages || 0) + (m.emails || 0) + (m.photos || 0) + (m.posts || 0) : 0;
    }

    function plural(n, one, many) {
        return `${n.toLocaleString()} ${n === 1 ? one : many}`;
    }

    function openItem(kind, id) {
        if (typeof Chat !== 'undefined' && typeof Chat.openSource === 'function') {
            void Chat.openSource(kind, id);
        }
    }

    async function open(targetMonth) {
        const modal = el('timeline-modal');
        if (!modal) return;
        modal.style.display = 'flex';
        el('timeline-days').innerHTML = '<p class="timeline-empty"><i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Loading your timeline…</p>';
        try {
            const res = await fetch('/api/timeline', { credentials: 'same-origin' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            months = Array.isArray(data.months) ? data.months.filter(m => total(m) > 0) : [];
        } catch (err) {
            el('timeline-days').innerHTML = '';
            el('timeline-days').appendChild(emptyMessage(`Couldn't load the timeline: ${err.message}`));
            return;
        }
        byMonth = new Map(months.map(m => [m.month, m]));
        if (months.length === 0) {
            el('timeline-years').innerHTML = '';
            el('timeline-months').innerHTML = '';
            el('timeline-month-title').textContent = '';
            el('timeline-month-summary').textContent = '';
            el('timeline-prev-btn').disabled = true;
            el('timeline-next-btn').disabled = true;
            el('timeline-days').innerHTML = '';
            el('timeline-days').appendChild(emptyMessage('Nothing with a date has been imported yet. Use Source Data to add messages, emails or photos.'));
            return;
        }
        const target = (targetMonth && byMonth.has(targetMonth)) ? targetMonth
            : (selectedMonth && byMonth.has(selectedMonth)) ? selectedMonth
                : months[months.length - 1].month;
        selectMonth(target);
    }

    function close() {
        const modal = el('timeline-modal');
        if (modal) modal.style.display = 'none';
    }

    function emptyMessage(text) {
        const p = document.createElement('p');
        p.className = 'timeline-empty';
        p.textContent = text;
        return p;
    }

    function years() {
        return [...new Set(months.map(m => m.month.slice(0, 4)))];
    }

    function renderYears() {
        const wrap = el('timeline-years');
        wrap.innerHTML = '';
        years().forEach(y => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.id = `timeline-year-${y}`;
            btn.className = 'timeline-year-btn' + (y === selectedYear ? ' timeline-year-btn--active' : '');
            btn.setAttribute('role', 'tab');
            btn.setAttribute('aria-selected', y === selectedYear ? 'true' : 'false');
            btn.textContent = y;
            btn.addEventListener('click', () => {
                const first = months.find(m => m.month.startsWith(y + '-'));
                if (first) selectMonth(first.month);
            });
            wrap.appendChild(btn);
        });
        const active = wrap.querySelector('.timeline-year-btn--active');
        if (active && typeof active.scrollIntoView === 'function') {
            active.scrollIntoView({ block: 'nearest', inline: 'center' });
        }
    }

    function renderMonths() {
        const wrap = el('timeline-months');
        wrap.innerHTML = '';
        const yearTotals = [];
        for (let i = 1; i <= 12; i++) {
            yearTotals.push(total(byMonth.get(`${selectedYear}-${String(i).padStart(2, '0')}`)));
        }
        const max = Math.max(1, ...yearTotals);
        for (let i = 1; i <= 12; i++) {
            const key = `${selectedYear}-${String(i).padStart(2, '0')}`;
            const m = byMonth.get(key);
            const n = total(m);
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.id = `timeline-month-${key}`;
            btn.className = 'timeline-month-btn' + (key === selectedMonth ? ' timeline-month-btn--active' : '');
            btn.disabled = n === 0;
            const bar = document.createElement('span');
            bar.className = 'timeline-month-bar';
            bar.style.height = n > 0 ? `${Math.max(8, Math.round((n / max) * 100))}%` : '0';
            const label = document.createElement('span');
            label.className = 'timeline-month-label';
            label.textContent = MONTH_SHORT[i - 1];
            btn.appendChild(bar);
            btn.appendChild(label);
            if (m) {
                const parts = [];
                if (m.messages) parts.push(plural(m.messages, 'message', 'messages'));
                if (m.emails) parts.push(plural(m.emails, 'email', 'emails'));
                if (m.photos) parts.push(plural(m.photos, 'photo', 'photos'));
                if (m.posts) parts.push(plural(m.posts, 'post', 'posts'));
                btn.title = `${MONTH_NAMES[i - 1]} ${selectedYear}: ${parts.join(', ')}`;
            } else {
                btn.title = `Nothing in ${MONTH_NAMES[i - 1]} ${selectedYear}`;
            }
            btn.addEventListener('click', () => selectMonth(key));
            wrap.appendChild(btn);
        }
    }

    function selectMonth(key) {
        selectedMonth = key;
        selectedYear = key.slice(0, 4);
        renderYears();
        renderMonths();
        const idx = months.findIndex(m => m.month === key);
        el('timeline-prev-btn').disabled = idx <= 0;
        el('timeline-next-btn').disabled = idx < 0 || idx >= months.length - 1;
        const [y, mo] = key.split('-').map(Number);
        el('timeline-month-title').textContent = `${MONTH_NAMES[mo - 1]} ${y}`;
        void loadMonth(y, mo);
    }

    function step(delta) {
        const idx = months.findIndex(m => m.month === selectedMonth);
        const next = months[idx + delta];
        if (next) selectMonth(next.month);
    }

    async function loadMonth(year, month) {
        const gen = ++loadGen;
        const days = el('timeline-days');
        days.innerHTML = '<p class="timeline-empty"><i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Loading…</p>';
        el('timeline-month-summary').textContent = '';
        let d;
        try {
            const res = await fetch(`/api/timeline/${year}/${month}`, { credentials: 'same-origin' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            d = await res.json();
        } catch (err) {
            if (gen !== loadGen) return;
            days.innerHTML = '';
            days.appendChild(emptyMessage(`Couldn't load this month: ${err.message}`));
            return;
        }
        if (gen !== loadGen) return; // a newer month was selected meanwhile
        renderMonth(d, year, month);
    }

    function dayKeyFromDate(s) { return (s || '').slice(0, 10); }

    function renderMonth(d, year, month) {
        const days = el('timeline-days');
        days.innerHTML = '';
        const t = d.totals || {};
        const parts = [];
        if (t.messages) parts.push(plural(t.messages, 'message', 'messages'));
        if (t.emails) parts.push(plural(t.emails, 'email', 'emails'));
        if (t.photos) parts.push(plural(t.photos, 'photo', 'photos'));
        if (t.posts) parts.push(plural(t.posts, 'post', 'posts'));
        el('timeline-month-summary').textContent = parts.join(' · ');

        const prefix = `${year}-${String(month).padStart(2, '0')}`;
        const groups = new Map(); // "YYYY-MM-DD" or "undated" → {conversations, emails, photos, posts}
        const group = (k) => {
            if (!groups.has(k)) groups.set(k, { conversations: [], emails: [], photos: [], posts: [] });
            return groups.get(k);
        };
        (d.conversations || []).forEach(c => group(dayKeyFromDate(c.date)).conversations.push(c));
        (d.emails || []).forEach(e => group(dayKeyFromDate(e.date)).emails.push(e));
        (d.photos || []).forEach(p => group(p.day > 0 ? `${prefix}-${String(p.day).padStart(2, '0')}` : 'undated').photos.push(p));
        (d.posts || []).forEach(p => group(dayKeyFromDate(p.date)).posts.push(p));

        const keys = [...groups.keys()].sort((a, b) => {
            if (a === 'undated') return 1;
            if (b === 'undated') return -1;
            return a < b ? -1 : a > b ? 1 : 0;
        });
        if (keys.length === 0) {
            days.appendChild(emptyMessage('Nothing to show for this month.'));
        }
        keys.forEach(k => days.appendChild(renderDay(k, groups.get(k))));

        const limits = d.limits || {};
        const capped = [];
        if ((d.emails || []).length >= (limits.emails || Infinity) && t.emails > d.emails.length) capped.push(`the first ${d.emails.length} of ${t.emails.toLocaleString()} emails`);
        if ((d.photos || []).length >= (limits.photos || Infinity) && t.photos > d.photos.length) capped.push(`the first ${d.photos.length} of ${t.photos.toLocaleString()} photos`);
        if ((d.conversations || []).length >= (limits.conversations || Infinity)) capped.push('the busiest conversations');
        if (capped.length) {
            days.appendChild(emptyMessage(`Showing ${capped.join(', ')} for this month. Use the Emails, Images or Messages views to see everything.`));
        }
        if (Array.isArray(d.hidden) && d.hidden.length) {
            days.appendChild(emptyMessage(`Your visitor key doesn't include ${d.hidden.join(' or ')}, so they aren't shown.`));
        }
        days.scrollTop = 0;
    }

    function dayTitle(key) {
        if (key === 'undated') return 'Sometime this month';
        const [y, m, dd] = key.split('-').map(Number);
        const date = new Date(y, m - 1, dd);
        if (Number.isNaN(date.getTime())) return key;
        return date.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
    }

    function itemButton(kind, id, iconCls, label, detail) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'timeline-item';
        btn.id = `timeline-item-${kind}-${id}`;
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

    function renderDay(key, g) {
        const section = document.createElement('section');
        section.className = 'timeline-day';
        const h = document.createElement('h4');
        h.className = 'timeline-day-title';
        h.textContent = dayTitle(key);
        section.appendChild(h);

        const list = document.createElement('div');
        list.className = 'timeline-day-items';
        g.conversations.forEach(c => {
            list.appendChild(itemButton('message', c.first_message_id, 'fa-comment-dots',
                c.chat_session || 'Conversation', plural(c.count, 'message', 'messages')));
        });
        g.emails.forEach(e => {
            list.appendChild(itemButton('email', e.id, 'fa-envelope',
                e.subject || '(no subject)', [e.from, e.time].filter(Boolean).join(' · ')));
        });
        g.posts.forEach(p => {
            list.appendChild(itemButton('facebook_post', p.id, 'fa-newspaper',
                p.title || 'Facebook post', p.text || ''));
        });
        if (list.childElementCount) section.appendChild(list);

        if (g.photos.length) {
            const strip = document.createElement('div');
            strip.className = 'timeline-photos';
            g.photos.forEach(p => {
                const btn = document.createElement('button');
                btn.type = 'button';
                btn.className = 'timeline-photo';
                btn.id = `timeline-photo-${p.id}`;
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

    function onKeydown(e) {
        const modal = el('timeline-modal');
        if (!modal || modal.style.display === 'none') return;
        const tag = (e.target && e.target.tagName) || '';
        if (tag === 'INPUT' || tag === 'TEXTAREA') return;
        // Only when the timeline is the topmost thing (no record viewer opened over it).
        const top = document.elementFromPoint(window.innerWidth / 2, 10);
        if (top && !modal.contains(top) && top !== modal) return;
        if (e.key === 'ArrowLeft') { e.preventDefault(); step(-1); }
        if (e.key === 'ArrowRight') { e.preventDefault(); step(1); }
    }

    function init() {
        el('timeline-sidebar-btn')?.addEventListener('click', () => { void open(); });
        el('close-timeline-modal')?.addEventListener('click', close);
        el('timeline-prev-btn')?.addEventListener('click', () => step(-1));
        el('timeline-next-btn')?.addEventListener('click', () => step(1));
        const modal = el('timeline-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) close(); });
        document.addEventListener('keydown', onKeydown);
    }

    return { init, open, close };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => Timeline.init());
} else {
    Timeline.init();
}
