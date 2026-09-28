'use strict';

// ChatBotMode — the generic, no-persona ChatBot modality (see CLAUDE.md). Runs in place in the
// main window like InterviewerMode: while active, #chatbot-panel (history rail + messages) replaces
// .chat-box-wrapper, the shared #chat-form / #user-input is repurposed for ChatBot prompts, and the
// model comes from the top-bar LLM chooser (#llm-provider-select). No streaming (v1) — a
// "Thinking…" bubble stands in while POST /chatbot/generate is in flight.
const ChatBotMode = (() => {
    const STORAGE_KEY = 'chatbot_current_conversation_id';

    let _panel, _chatMain, _chatBoxWrapper, _sidebarBtn, _exitBtn;
    let _railList, _newConversationBtn;
    let _messagesEl, _attachTrayEl;
    let _attachBtn, _fileInput;

    let _isActive = false;
    let _currentConversationId = null;
    let _pendingAttachments = []; // [{id, filename, kind, extraction_error}], not yet sent
    let _isSending = false;

    function init() {
        _panel = document.getElementById('chatbot-panel');
        if (!_panel) return;
        _chatBoxWrapper = document.querySelector('.chat-box-wrapper');
        _chatMain = document.querySelector('.chat-main');
        _sidebarBtn = document.getElementById('chatbot-sidebar-btn');
        _exitBtn = document.getElementById('chatbot-exit-btn');
        _railList = document.getElementById('chatbot-rail-list');
        _newConversationBtn = document.getElementById('chatbot-new-conversation-btn');
        _messagesEl = document.getElementById('chatbot-messages');
        _attachTrayEl = document.getElementById('chatbot-attach-tray');
        _attachBtn = document.getElementById('chatbot-attach-btn');
        _fileInput = document.getElementById('chatbot-file-input');

        if (_sidebarBtn) _sidebarBtn.addEventListener('click', () => (_isActive ? exit() : void enter()));
        if (_exitBtn) _exitBtn.addEventListener('click', exit);
        if (_newConversationBtn) _newConversationBtn.addEventListener('click', () => void _startNewConversation());
        if (_attachBtn) _attachBtn.addEventListener('click', () => _fileInput && _fileInput.click());
        if (_fileInput) _fileInput.addEventListener('change', () => void _onFilesSelected());

        _currentConversationId = _loadStoredConversationId();
    }

    function isActive() {
        return _isActive;
    }

    function _chatForm() { return (typeof DOM !== 'undefined' && DOM.chatForm) || document.getElementById('chat-form'); }
    function _userInput() { return (typeof DOM !== 'undefined' && DOM.userInput) || document.getElementById('user-input'); }
    function _sendButton() { return (typeof DOM !== 'undefined' && DOM.sendButton) || document.getElementById('send-button'); }

    function _loadStoredConversationId() {
        try {
            const n = parseInt(localStorage.getItem(STORAGE_KEY) || '', 10);
            return Number.isFinite(n) && n > 0 ? n : null;
        } catch (_) {
            return null;
        }
    }

    function _storeConversationId(id) {
        try {
            if (id) localStorage.setItem(STORAGE_KEY, String(id));
            else localStorage.removeItem(STORAGE_KEY);
        } catch (_) { /* ignore (private window / blocked storage) */ }
    }

    // ── Mode enter / exit ──────────────────────────────────────────────────

    async function enter() {
        if (_isActive || !_panel) return;
        if (typeof InterviewerMode !== 'undefined' && InterviewerMode.isActive()) {
            window.alert('End the current interview before starting the ChatBot.');
            return;
        }
        _isActive = true;

        if (_chatBoxWrapper) _chatBoxWrapper.style.display = 'none';
        _panel.style.display = '';
        if (_chatMain) _chatMain.classList.add('chatbot-mode');
        if (_sidebarBtn) _sidebarBtn.classList.add('active');

        const form = _chatForm();
        if (form) form.addEventListener('submit', _handleFormSubmit);
        const input = _userInput();
        if (input) {
            input.placeholder = 'Message ChatBot...';
            input.focus();
        }
        const loadingEl = document.getElementById('loading-indicator');
        if (loadingEl) loadingEl.style.display = 'none';

        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('chatbot', true);
        }
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
        }

        await _refreshRail();
        const opened = _currentConversationId && await _openConversation(_currentConversationId, true);
        if (!opened) await _startNewConversation();
    }

    function exit() {
        if (!_isActive) return;
        _isActive = false;

        _panel.style.display = 'none';
        if (_chatBoxWrapper) _chatBoxWrapper.style.display = '';
        if (_chatMain) _chatMain.classList.remove('chatbot-mode');
        if (_sidebarBtn) _sidebarBtn.classList.remove('active');

        const form = _chatForm();
        if (form) form.removeEventListener('submit', _handleFormSubmit);
        const input = _userInput();
        if (input) input.placeholder = 'Enter your question or comment...';
        const sendBtn = _sendButton();
        if (sendBtn) sendBtn.disabled = false;

        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('chatbot', false);
        }
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
        }
    }

    function _handleFormSubmit(e) {
        e.preventDefault();
        if (_isActive) void _sendMessage();
    }

    // ── Rail (conversation list) ────────────────────────────────────────────

    async function _refreshRail() {
        if (!_railList) return;
        let conversations = [];
        try {
            const resp = await fetch('/chatbot/conversations', { credentials: 'same-origin' });
            if (resp.ok) conversations = await resp.json();
        } catch (e) {
            console.error('ChatBotMode: failed to load conversations', e);
        }
        _railList.innerHTML = '';
        if (!conversations || conversations.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'chatbot-rail-empty';
            empty.textContent = 'No conversations yet';
            _railList.appendChild(empty);
            return;
        }
        conversations.forEach((c) => _railList.appendChild(_buildRailItem(c)));
    }

    function _buildRailItem(conv) {
        const item = document.createElement('div');
        item.className = 'chatbot-rail-item' + (conv.id === _currentConversationId ? ' active' : '');
        item.dataset.conversationId = String(conv.id);

        const title = document.createElement('span');
        title.className = 'chatbot-rail-item-title';
        title.textContent = conv.title || 'New Chat';
        item.appendChild(title);

        const actions = document.createElement('span');
        actions.className = 'chatbot-rail-item-actions';

        const renameBtn = document.createElement('button');
        renameBtn.type = 'button';
        renameBtn.title = 'Rename';
        renameBtn.innerHTML = '<i class="fas fa-pen"></i>';
        renameBtn.addEventListener('click', (e) => { e.stopPropagation(); void _renameConversation(conv); });
        actions.appendChild(renameBtn);

        const deleteBtn = document.createElement('button');
        deleteBtn.type = 'button';
        deleteBtn.title = 'Delete';
        deleteBtn.innerHTML = '<i class="fas fa-trash"></i>';
        deleteBtn.addEventListener('click', (e) => { e.stopPropagation(); void _deleteConversation(conv); });
        actions.appendChild(deleteBtn);

        item.appendChild(actions);
        item.addEventListener('click', () => void _openConversation(conv.id, false));
        return item;
    }

    async function _renameConversation(conv) {
        const next = window.prompt('Rename conversation', conv.title || '');
        if (next === null) return;
        const title = next.trim();
        if (!title || title === conv.title) return;
        try {
            const resp = await fetch(`/chatbot/conversations/${conv.id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ title }),
            });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            await _refreshRail();
        } catch (e) {
            console.error('ChatBotMode: rename failed', e);
            window.alert('Could not rename the conversation.');
        }
    }

    async function _deleteConversation(conv) {
        if (!window.confirm(`Delete "${conv.title || 'this conversation'}"? This cannot be undone.`)) return;
        try {
            const resp = await fetch(`/chatbot/conversations/${conv.id}`, { method: 'DELETE', credentials: 'same-origin' });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const wasCurrent = conv.id === _currentConversationId;
            if (wasCurrent) {
                _currentConversationId = null;
                _storeConversationId(null);
            }
            await _refreshRail();
            if (wasCurrent) await _startNewConversation();
        } catch (e) {
            console.error('ChatBotMode: delete failed', e);
            window.alert('Could not delete the conversation.');
        }
    }

    // ── Conversation lifecycle ───────────────────────────────────────────────

    async function _startNewConversation() {
        try {
            const resp = await fetch('/chatbot/conversations', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ title: 'New Chat' }),
            });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const conv = await resp.json();
            _currentConversationId = conv.id;
            _storeConversationId(conv.id);
            _pendingAttachments = [];
            _renderAttachTray();
            _renderMessages([]);
            await _refreshRail();
        } catch (e) {
            console.error('ChatBotMode: failed to start a new conversation', e);
        }
    }

    /** Returns true if the conversation was found and opened. */
    async function _openConversation(id, silentOnMissing) {
        try {
            const resp = await fetch(`/chatbot/conversations/${id}`, { credentials: 'same-origin' });
            if (resp.status === 404) {
                if (!silentOnMissing) window.alert('That conversation no longer exists.');
                return false;
            }
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            _currentConversationId = id;
            _storeConversationId(id);
            _renderMessages(data.turns || []);
            await _loadPendingAttachments();
            _highlightActiveRailItem();
            return true;
        } catch (e) {
            console.error('ChatBotMode: failed to open conversation', e);
            return false;
        }
    }

    function _highlightActiveRailItem() {
        if (!_railList) return;
        Array.from(_railList.children).forEach((el) => {
            el.classList.toggle('active', el.dataset.conversationId === String(_currentConversationId));
        });
    }

    // ── Messages ─────────────────────────────────────────────────────────────

    function _renderMarkdownInto(el, text) {
        const escaped = String(text == null ? '' : text).replace(/</g, '&lt;').replace(/>/g, '&gt;');
        if (typeof marked !== 'undefined' && marked.parse) {
            el.innerHTML = marked.parse(escaped);
        } else {
            el.textContent = text || '';
        }
    }

    function _renderMessages(turns) {
        if (!_messagesEl) return;
        _messagesEl.innerHTML = '';
        turns.forEach((t) => {
            _appendMessage('user', t.user_input, t.attachment_ids);
            _appendMessage('assistant', t.response_text);
        });
        _scrollToBottom();
    }

    function _appendMessage(role, text, attachmentIds) {
        if (!_messagesEl) return null;
        const bubble = document.createElement('div');
        bubble.className = `chatbot-msg chatbot-msg-${role}`;
        if (role === 'assistant') {
            _renderMarkdownInto(bubble, text);
        } else {
            bubble.textContent = text || '';
        }
        if (attachmentIds && attachmentIds.length > 0) {
            const tray = document.createElement('div');
            tray.className = 'chatbot-msg-attachments';
            const chip = document.createElement('span');
            chip.className = 'chatbot-msg-attachment-chip';
            chip.innerHTML = `<i class="fas fa-paperclip"></i> ${attachmentIds.length} attachment${attachmentIds.length === 1 ? '' : 's'}`;
            tray.appendChild(chip);
            bubble.appendChild(tray);
        }
        _messagesEl.appendChild(bubble);
        return bubble;
    }

    function _scrollToBottom() {
        if (_messagesEl) _messagesEl.scrollTop = _messagesEl.scrollHeight;
    }

    // ── Sending ──────────────────────────────────────────────────────────────

    /** The top-bar LLM chooser drives the ChatBot too ("auto" → JEV picks the model). */
    function _selectedProvider() {
        const el = document.getElementById('llm-provider-select');
        return (el && el.value) || 'auto';
    }

    async function _sendMessage() {
        const input = _userInput();
        if (_isSending || !input) return;
        const prompt = input.value.trim();
        if (!prompt) return;
        if (!_currentConversationId) await _startNewConversation();

        const attachmentIds = _pendingAttachments.map((a) => a.id);
        const sendBtn = _sendButton();

        _isSending = true;
        if (sendBtn) sendBtn.disabled = true;
        input.value = '';

        const wasFirstMessage = _messagesEl && _messagesEl.children.length === 0;
        _appendMessage('user', prompt, attachmentIds);
        const typingBubble = _appendMessage('assistant', 'Thinking…');
        if (typingBubble) typingBubble.classList.add('typing');
        _scrollToBottom();

        _pendingAttachments = [];
        _renderAttachTray();

        try {
            const resp = await fetch('/chatbot/generate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    prompt,
                    conversation_id: _currentConversationId,
                    provider: _selectedProvider(),
                    attachment_ids: attachmentIds,
                }),
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
            if (typingBubble) {
                typingBubble.classList.remove('typing');
                _renderMarkdownInto(typingBubble, data.response || '');
            }
            if (wasFirstMessage) await _autoTitleConversation(prompt);
            await _refreshRail();
            if (typeof UI !== 'undefined' && UI.refreshOpenRouterCredits) void UI.refreshOpenRouterCredits();
        } catch (e) {
            console.error('ChatBotMode: generate failed', e);
            if (typingBubble) {
                typingBubble.classList.remove('typing');
                typingBubble.classList.add('chatbot-msg-error');
                typingBubble.textContent = `Error: ${e.message || 'the request failed'}`;
            }
        } finally {
            _isSending = false;
            if (sendBtn) sendBtn.disabled = false;
            _scrollToBottom();
        }
    }

    async function _autoTitleConversation(prompt) {
        const title = prompt.length > 60 ? prompt.slice(0, 57) + '…' : prompt;
        try {
            await fetch(`/chatbot/conversations/${_currentConversationId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ title }),
            });
        } catch (e) {
            console.error('ChatBotMode: auto-title failed', e);
        }
    }

    // ── Attachments ──────────────────────────────────────────────────────────

    async function _loadPendingAttachments() {
        _pendingAttachments = [];
        if (_currentConversationId) {
            try {
                const resp = await fetch(`/chatbot/conversations/${_currentConversationId}/attachments`, { credentials: 'same-origin' });
                if (resp.ok) _pendingAttachments = await resp.json();
            } catch (e) {
                console.error('ChatBotMode: failed to load pending attachments', e);
            }
        }
        _renderAttachTray();
    }

    async function _onFilesSelected() {
        if (!_fileInput || !_fileInput.files || _fileInput.files.length === 0) return;
        if (!_currentConversationId) await _startNewConversation();
        const files = Array.from(_fileInput.files);
        _fileInput.value = '';
        for (const file of files) {
            await _uploadAttachment(file);
        }
    }

    async function _uploadAttachment(file) {
        const form = new FormData();
        form.append('file', file);
        try {
            const resp = await fetch(`/chatbot/conversations/${_currentConversationId}/attachments`, {
                method: 'POST',
                credentials: 'same-origin',
                body: form,
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
            _pendingAttachments.push(data);
            _renderAttachTray();
        } catch (e) {
            console.error('ChatBotMode: attachment upload failed', e);
            window.alert(`Could not attach "${file.name}": ${e.message || 'upload failed'}`);
        }
    }

    function _renderAttachTray() {
        if (!_attachTrayEl) return;
        _attachTrayEl.innerHTML = '';
        _pendingAttachments.forEach((att) => {
            const chip = document.createElement('span');
            chip.className = 'chatbot-attach-chip' + (att.extraction_error ? ' error' : '');
            const icon = att.kind === 'image' ? 'fa-image' : 'fa-file-alt';
            chip.innerHTML = `<i class="fas ${icon}"></i> <span></span>`;
            chip.querySelector('span').textContent = att.filename;
            if (att.extraction_error) chip.title = att.extraction_error;

            const removeBtn = document.createElement('button');
            removeBtn.type = 'button';
            removeBtn.className = 'chatbot-attach-chip-remove';
            removeBtn.innerHTML = '&times;';
            removeBtn.title = 'Remove';
            removeBtn.addEventListener('click', () => void _removePendingAttachment(att.id));
            chip.appendChild(removeBtn);

            _attachTrayEl.appendChild(chip);
        });
    }

    async function _removePendingAttachment(id) {
        try {
            await fetch(`/chatbot/attachments/${id}`, { method: 'DELETE', credentials: 'same-origin' });
        } catch (e) {
            console.error('ChatBotMode: failed to remove attachment', e);
        }
        _pendingAttachments = _pendingAttachments.filter((a) => a.id !== id);
        _renderAttachTray();
    }

    return { init, enter, exit, isActive, newConversation: _startNewConversation };
})();
