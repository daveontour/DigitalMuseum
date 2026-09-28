'use strict';

// ChatHistoryPanel — collapsible, Claude-style list of saved persona-chat conversations on the
// left of .chat-box-wrapper. Every prompt/response is already persisted per conversation
// (chat_turns, via the conversation_id each /chat/generate request carries); this panel is the
// recall/manage UI over that. Conversation state itself stays owned by Modals.ConversationManager —
// this module only renders and delegates (resume / rename / delete / new chat).
const ChatHistoryPanel = (() => {
    const COLLAPSED_KEY = 'chat_history_panel_collapsed';

    let _panel, _list, _toggleBtn, _newChatBtn;
    let _refreshSeq = 0;

    function _cm() {
        return (typeof Modals !== 'undefined' && Modals.ConversationManager) ? Modals.ConversationManager : null;
    }

    function init() {
        _panel = document.getElementById('chat-history-panel');
        if (!_panel || _panel.dataset.wired === '1') return;
        _panel.dataset.wired = '1';
        _list = document.getElementById('chat-history-list');
        _toggleBtn = document.getElementById('chat-history-toggle-btn');
        _newChatBtn = document.getElementById('new-chat-btn');

        // Collapsed by default; stays expanded only once the user has explicitly opened it.
        let collapsed = true;
        try { collapsed = localStorage.getItem(COLLAPSED_KEY) !== '0'; } catch (_) { /* storage blocked */ }
        _applyCollapsed(collapsed);

        if (_toggleBtn) _toggleBtn.addEventListener('click', () => _applyCollapsed(!_panel.classList.contains('collapsed'), true));
        if (_newChatBtn) _newChatBtn.addEventListener('click', () => void _newChat());

        void refresh();
    }

    function _applyCollapsed(collapsed, persist) {
        _panel.classList.toggle('collapsed', collapsed);
        if (_toggleBtn) {
            _toggleBtn.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
            _toggleBtn.title = collapsed ? 'Show chat history' : 'Hide chat history';
            const icon = _toggleBtn.querySelector('i');
            if (icon) icon.className = collapsed ? 'fas fa-chevron-right' : 'fas fa-chevron-left';
        }
        if (persist) {
            try { localStorage.setItem(COLLAPSED_KEY, collapsed ? '1' : '0'); } catch (_) { /* storage blocked */ }
        }
    }

    /** True (after telling the user) when switching conversations now would misroute a reply or
     *  disrupt an interview. */
    async function _blockedBusy() {
        if (typeof InterviewerMode !== 'undefined' && InterviewerMode.isActive()) {
            await AppDialogs.showAppAlert('Interview in progress', 'End the current interview before switching chats.');
            return true;
        }
        const sendBtn = (typeof DOM !== 'undefined' && DOM.sendButton) || document.getElementById('send-button');
        if (sendBtn && sendBtn.disabled) {
            await AppDialogs.showAppAlert('Please wait', 'A reply is still being generated — switch chats once it has finished.');
            return true;
        }
        return false;
    }

    async function _newChat() {
        const cm = _cm();
        if (!cm || await _blockedBusy()) return;
        await cm.startNewChat();
    }

    async function _open(id) {
        const cm = _cm();
        if (!cm || id === cm.getCurrentConversationId()) return;
        if (await _blockedBusy()) return;
        await cm.resumeConversation(id);
    }

    // ── Rendering ─────────────────────────────────────────────────────────────

    async function refresh() {
        if (!_list) return;
        const seq = ++_refreshSeq;
        let conversations = [];
        try {
            const resp = await fetch('/chat/conversations', { credentials: 'same-origin' });
            if (resp.ok) conversations = await resp.json();
        } catch (e) {
            console.error('ChatHistoryPanel: failed to load conversations', e);
        }
        if (seq !== _refreshSeq) return; // a newer refresh superseded this one
        _render(Array.isArray(conversations) ? conversations : []);
    }

    function _activityDate(conv) {
        const raw = conv.last_message_at || conv.created_at;
        const d = raw ? new Date(raw) : null;
        return d && !Number.isNaN(d.getTime()) ? d : null;
    }

    function _groupLabel(date) {
        if (!date) return 'Older';
        const startOfToday = new Date();
        startOfToday.setHours(0, 0, 0, 0);
        const dayMs = 24 * 60 * 60 * 1000;
        const diffDays = (startOfToday.getTime() - date.getTime()) / dayMs;
        if (diffDays <= 0) return 'Today';
        if (diffDays <= 1) return 'Yesterday';
        if (diffDays <= 7) return 'Previous 7 days';
        if (diffDays <= 30) return 'Previous 30 days';
        return 'Older';
    }

    function _render(conversations) {
        const cm = _cm();
        const currentId = cm ? cm.getCurrentConversationId() : null;
        // Empty conversations are noise (e.g. an unused "New Chat") — except the one you're on.
        const visible = conversations.filter((c) => (c.turn_count || 0) > 0 || c.id === currentId);

        _list.innerHTML = '';
        if (visible.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'chat-history-empty';
            empty.textContent = 'No saved chats yet';
            _list.appendChild(empty);
            return;
        }

        let lastGroup = null;
        visible.forEach((conv) => {
            const group = _groupLabel(_activityDate(conv));
            if (group !== lastGroup) {
                const heading = document.createElement('div');
                heading.className = 'chat-history-group';
                heading.textContent = group;
                _list.appendChild(heading);
                lastGroup = group;
            }
            _list.appendChild(_buildItem(conv, conv.id === currentId));
        });
    }

    function _buildItem(conv, isActive) {
        const item = document.createElement('div');
        item.className = 'chat-history-item' + (isActive ? ' active' : '');
        item.setAttribute('role', 'button');
        item.tabIndex = 0;

        const title = document.createElement('span');
        title.className = 'chat-history-item-title';
        title.textContent = conv.title || 'New Chat';
        const when = _activityDate(conv);
        item.title = `${conv.title || 'New Chat'}\n${conv.turn_count || 0} messages${when ? ' • ' + when.toLocaleString() : ''}`;
        item.appendChild(title);

        const actions = document.createElement('span');
        actions.className = 'chat-history-item-actions';
        actions.appendChild(_actionBtn('fa-file-pdf', 'Download as PDF', () => _downloadPdf(conv.id)));
        actions.appendChild(_actionBtn('fa-pen', 'Rename', () => _cm() && _cm().renameConversation(conv.id, conv.title)));
        actions.appendChild(_actionBtn('fa-trash', 'Delete', async () => {
            if (conv.id === (_cm() && _cm().getCurrentConversationId()) && await _blockedBusy()) return;
            if (_cm()) await _cm().deleteConversation(conv.id);
        }));
        item.appendChild(actions);

        item.addEventListener('click', () => void _open(conv.id));
        item.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                void _open(conv.id);
            }
        });
        return item;
    }

    /** Server responds with Content-Disposition: attachment, so this downloads without leaving the
     *  page. */
    function _downloadPdf(id) {
        window.location.href = `/chat/conversations/${id}/pdf`;
    }

    function _actionBtn(icon, label, onClick) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.title = label;
        btn.setAttribute('aria-label', label);
        btn.innerHTML = `<i class="fas ${icon}" aria-hidden="true"></i>`;
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            void onClick();
        });
        return btn;
    }

    return { init, refresh };
})();
