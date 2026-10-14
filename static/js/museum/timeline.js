'use strict';

/**
 * Timeline — browse the whole archive by year → month → day, mixing conversations, emails,
 * photos and Facebook posts. Data: GET /api/timeline (per-month counts) and
 * GET /api/timeline/{year}/{month} (internal/handler/timeline_handler.go).
 *
 * Optional filter (person / keywords / photo tag) is passed to both as ?person=&q=&tag=; the
 * year/month bars then count only matching items, so navigation skips months with no matches.
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
    let filter = { personId: null, personLabel: '', q: '', tag: '' };
    let personIds = null;     // Map label → contact id (null until loaded)
    let suggestionsPromise = null;

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

    function filterActive() {
        return !!(filter.personId || filter.q || filter.tag);
    }

    function filterQuery() {
        const p = new URLSearchParams();
        if (filter.personId) p.set('person', String(filter.personId));
        if (filter.q) p.set('q', filter.q);
        if (filter.tag) p.set('tag', filter.tag);
        const s = p.toString();
        return s ? `?${s}` : '';
    }

    async function errorText(res) {
        try {
            const body = await res.json();
            if (body && (body.detail || body.error)) return body.detail || body.error;
        } catch (_) { /* not JSON */ }
        return `HTTP ${res.status}`;
    }

    function setStatus(text, isError) {
        const st = el('timeline-filter-status');
        if (!st) return;
        st.textContent = text || '';
        st.hidden = !text;
        st.classList.toggle('timeline-filter-status--error', !!isError);
    }

    function describeFilter(excluded) {
        if (!filterActive()) return '';
        const parts = [];
        if (filter.personId) parts.push(`involving ${filter.personLabel}`);
        if (filter.q) parts.push(`containing ${filter.q}`);
        if (filter.tag) parts.push(`tagged “${filter.tag}”`);
        const n = months.reduce((acc, m) => acc + total(m), 0);
        let text = `Showing ${plural(n, 'item', 'items')} ${parts.join(', ')}` +
            (months.length ? ` across ${plural(months.length, 'month', 'months')}.` : '.');
        if (Array.isArray(excluded) && excluded.length) {
            text += ` A photo tag filter leaves out ${excluded.join(', ')}.`;
        }
        if (filter.personId) {
            text += ' Photos match people named in People in Photos; posts match mentions of the name.';
        }
        return text;
    }

    function loadSuggestions() {
        if (!suggestionsPromise) suggestionsPromise = fetchSuggestions();
        return suggestionsPromise;
    }

    async function fetchSuggestions() {
        const fill = (listId, labels) => {
            const list = el(listId);
            if (!list) return;
            list.innerHTML = '';
            labels.forEach(l => {
                const o = document.createElement('option');
                o.value = l;
                list.appendChild(o);
            });
        };
        try {
            const res = await fetch('/contacts/names', { credentials: 'same-origin' });
            if (res.status === 403) {
                // Visitor key without contacts — no person filter.
                const field = el('timeline-filter-person-field');
                if (field) field.hidden = true;
            } else if (res.ok) {
                const data = await res.json();
                const contacts = (Array.isArray(data.contacts) ? data.contacts : [])
                    .filter(c => c && c.id && (c.name || '').trim());
                const counts = new Map();
                contacts.forEach(c => counts.set(c.name.trim(), (counts.get(c.name.trim()) || 0) + 1));
                personIds = new Map();
                contacts.forEach(c => {
                    const name = c.name.trim();
                    personIds.set(counts.get(name) > 1 ? `${name} (#${c.id})` : name, c.id);
                });
                fill('timeline-filter-person-list', [...personIds.keys()].sort((a, b) => a.localeCompare(b)));
            }
        } catch (_) { /* suggestions are optional */ }
        try {
            const res = await fetch('/images/tags', { credentials: 'same-origin' });
            if (res.ok) {
                const data = await res.json();
                fill('timeline-filter-tag-list', Array.isArray(data.tags) ? data.tags : []);
            }
        } catch (_) { /* suggestions are optional */ }
    }

    // Returns the contact id for a label, null for an empty box, undefined when it isn't a contact.
    function resolvePerson(label) {
        if (!label) return null;
        if (!personIds) return undefined;
        if (personIds.has(label)) return personIds.get(label);
        const lower = label.toLowerCase();
        for (const [k, id] of personIds) {
            if (k.toLowerCase() === lower) return id;
        }
        return undefined;
    }

    async function applyFilter() {
        const personLabel = (el('timeline-filter-person')?.value || '').trim();
        if (personLabel) await loadSuggestions();
        const personId = resolvePerson(personLabel);
        if (personId === undefined) {
            setStatus(`No contact is called “${personLabel}”. Pick a name from the list.`, true);
            return;
        }
        filter = {
            personId,
            personLabel,
            q: (el('timeline-filter-keywords')?.value || '').trim(),
            tag: (el('timeline-filter-tag')?.value || '').trim(),
        };
        void open();
    }

    function clearFilter() {
        ['timeline-filter-person', 'timeline-filter-keywords', 'timeline-filter-tag'].forEach(id => {
            const input = el(id);
            if (input) input.value = '';
        });
        filter = { personId: null, personLabel: '', q: '', tag: '' };
        void open();
    }

    async function open(targetMonth) {
        const modal = el('timeline-modal');
        if (!modal) return;
        modal.style.display = 'flex';
        void loadSuggestions();
        const clearBtn = el('timeline-filter-clear-btn');
        if (clearBtn) clearBtn.hidden = !filterActive();
        setStatus('');
        el('timeline-days').innerHTML = '<p class="timeline-empty"><i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Loading your timeline…</p>';
        const gen = ++loadGen;
        let excluded = [];
        try {
            const res = await fetch(`/api/timeline${filterQuery()}`, { credentials: 'same-origin' });
            if (!res.ok) throw new Error(await errorText(res));
            const data = await res.json();
            if (gen !== loadGen) return; // the filter changed meanwhile
            months = Array.isArray(data.months) ? data.months.filter(m => total(m) > 0) : [];
            excluded = data.excluded || [];
        } catch (err) {
            if (gen !== loadGen) return;
            el('timeline-days').innerHTML = '';
            el('timeline-days').appendChild(emptyMessage(`Couldn't load the timeline: ${err.message}`));
            return;
        }
        byMonth = new Map(months.map(m => [m.month, m]));
        setStatus(describeFilter(excluded));
        if (months.length === 0) {
            el('timeline-years').innerHTML = '';
            el('timeline-months').innerHTML = '';
            el('timeline-month-title').textContent = '';
            el('timeline-month-summary').textContent = '';
            el('timeline-prev-btn').disabled = true;
            el('timeline-next-btn').disabled = true;
            el('timeline-days').innerHTML = '';
            el('timeline-days').appendChild(emptyMessage(filterActive()
                ? 'Nothing matches this filter. Try fewer keywords, or Clear to see everything.'
                : 'Nothing with a date has been imported yet. Use Source Data to add messages, emails or photos.'));
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
            const res = await fetch(`/api/timeline/${year}/${month}${filterQuery()}`, { credentials: 'same-origin' });
            if (!res.ok) throw new Error(await errorText(res));
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
        el('timeline-filter-form')?.addEventListener('submit', (e) => { e.preventDefault(); void applyFilter(); });
        el('timeline-filter-clear-btn')?.addEventListener('click', clearFilter);
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
