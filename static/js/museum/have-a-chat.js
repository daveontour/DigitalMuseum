'use strict';

// --- Have A Chat Module ---
// Orchestrates an autonomous Claude ↔ Gemini conversation in the main chat box.
const HaveAChat = (() => {

    // ── State ────────────────────────────────────────────────────────────────
    let _isRunning      = false;
    let _isPaused       = false;
    let _stopRequested  = false;
    let _currentSpeaker = null;   // 'claude' or 'gemini'
    let _chatHistory    = [];     // [{speaker, text}]
    let _claudeVoice    = 'expert';
    let _geminiVoice    = 'expert';
    let _topic          = '';
    let _temperature    = 0.7;
    let _pendingInjection = null; // text injected by user mid-conversation

    // ── DOM helpers ──────────────────────────────────────────────────────────
    const _el = (id) => document.getElementById(id);

    function _populateVoiceSelects() {
        ApiService.fetchVoices()
            .then(voices => {
                const claudeSel = _el('have-a-chat-claude-voice');
                const geminiSel = _el('have-a-chat-gemini-voice');
                if (!claudeSel || !geminiSel || !voices || !voices.length) return;
                claudeSel.innerHTML = '';
                geminiSel.innerHTML = '';
                voices.forEach(v => {
                    const key  = v.key  || v.value || '';
                    const name = v.name || key;
                    const o1 = document.createElement('option');
                    o1.value = key; o1.textContent = name;
                    claudeSel.appendChild(o1);
                    const o2 = document.createElement('option');
                    o2.value = key; o2.textContent = name;
                    geminiSel.appendChild(o2);
                });
                // Default Gemini to a different voice to make them distinct
                if (geminiSel.options.length > 1) geminiSel.selectedIndex = 1;
            })
            .catch(() => { /* leave the default 'expert' option */ });
    }

    // ── Setup modal ──────────────────────────────────────────────────────────
    function open() {
        _populateVoiceSelects();
        const modal = _el('have-a-chat-setup-modal');
        if (modal) modal.style.display = 'flex';
    }

    function _closeSetupModal() {
        const modal = _el('have-a-chat-setup-modal');
        if (modal) modal.style.display = 'none';
    }

    function _closeInjectModal() {
        const modal = _el('have-a-chat-inject-modal');
        if (modal) modal.style.display = 'none';
    }

    // ── Message rendering ────────────────────────────────────────────────────
    function _addHaveAChatMessage(provider, voice, text) {
        const msgEl = document.createElement('div');
        msgEl.classList.add('have-a-chat-message', `have-a-chat-${provider}`);

        // Header row: voice image + LLM label
        const header = document.createElement('div');
        header.className = 'have-a-chat-header';

        const img = document.createElement('img');
        img.className = 'have-a-chat-voice-image';
        img.src = `/static/images/${typeof VoiceSelector !== 'undefined' ? VoiceSelector.getVoiceImage(voice, true) : voice + '_sm.png'}`;
        img.alt = voice;
        img.onerror = () => { img.style.display = 'none'; };

        const label = document.createElement('span');
        label.className = 'have-a-chat-label';
        label.textContent = provider === 'claude' ? 'Claude' : 'Gemini';

        const voiceLabel = document.createElement('span');
        voiceLabel.style.cssText = 'font-size:0.75em; color:#666; margin-left:4px;';
        voiceLabel.textContent = `(${voice})`;

        header.appendChild(img);
        header.appendChild(label);
        header.appendChild(voiceLabel);
        msgEl.appendChild(header);

        // Content with markdown render
        const contentEl = document.createElement('div');
        contentEl.className = 'have-a-chat-content';
        if (typeof marked !== 'undefined') {
            const sanitized = text.replace(/</g, '&lt;').replace(/>/g, '&gt;');
            contentEl.innerHTML = marked.parse(sanitized || '');
        } else {
            contentEl.textContent = text;
        }
        msgEl.appendChild(contentEl);

        if (DOM && DOM.chatBox) {
            DOM.chatBox.appendChild(msgEl);
            if (typeof UI !== 'undefined' && UI.scrollToBottom) UI.scrollToBottom();
        }
    }

    function _addUserInjectionMessage(text) {
        const msgEl = document.createElement('div');
        msgEl.className = 'have-a-chat-user-comment';
        msgEl.textContent = `You: ${text}`;
        if (DOM && DOM.chatBox) {
            DOM.chatBox.appendChild(msgEl);
            if (typeof UI !== 'undefined' && UI.scrollToBottom) UI.scrollToBottom();
        }
    }

    // ── Control bar helpers ──────────────────────────────────────────────────
    function _showControlBar() {
        const bar = _el('have-a-chat-control-bar');
        if (bar) bar.style.display = 'flex';
    }

    function _hideControlBar() {
        const bar = _el('have-a-chat-control-bar');
        if (bar) bar.style.display = 'none';
    }

    function _setStatus(text) {
        const el = _el('have-a-chat-status-text');
        if (el) el.textContent = text;
    }

    function _setPauseButtonLabel(paused) {
        const btn = _el('have-a-chat-pause-btn');
        if (!btn) return;
        btn.innerHTML = paused
            ? '<i class="fas fa-play"></i> Resume'
            : '<i class="fas fa-pause"></i> Pause';
    }

    function _disableChatForm(disabled) {
        const sendBtn = _el('send-button');
        const input   = _el('user-input');
        if (sendBtn) sendBtn.disabled = disabled;
        if (input)   input.disabled   = disabled;
        const form = _el('chat-form');
        if (form) form.style.opacity = disabled ? '0.45' : '1';
    }

    // ── API call ─────────────────────────────────────────────────────────────
    async function _callTurnEndpoint() {
        const url = (typeof CONSTANTS !== 'undefined' && CONSTANTS.API_PATHS && CONSTANTS.API_PATHS.HAVE_A_CHAT_TURN)
            ? CONSTANTS.API_PATHS.HAVE_A_CHAT_TURN
            : '/chat/have-a-chat/turn';

        const resp = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                speaking_provider: _currentSpeaker,
                claude_voice:      _claudeVoice,
                gemini_voice:      _geminiVoice,
                topic:             _topic,
                history:           _chatHistory,
                temperature:       _temperature,
            })
        });

        if (!resp.ok) {
            let msg = `HTTP ${resp.status}`;
            try {
                const d = await resp.json();
                msg = d.detail || d.error || msg;
            } catch (_) { /* ignore */ }
            throw new Error(msg);
        }
        return resp.json();
    }

    // ── Core loop ────────────────────────────────────────────────────────────
    async function _loop() {
        while (_isRunning && !_stopRequested) {
            // Pause: spin until resumed or stopped
            while (_isPaused && !_stopRequested) {
                await _sleep(500);
            }
            if (_stopRequested) break;

            // Handle pending user injection before next turn
            if (_pendingInjection) {
                const injected = _pendingInjection;
                _pendingInjection = null;
                _addUserInjectionMessage(injected);
                _chatHistory.push({ speaker: 'user', text: injected });
            }

            const voice = _currentSpeaker === 'claude' ? _claudeVoice : _geminiVoice;
            const speakerLabel = _currentSpeaker === 'claude' ? 'Claude' : 'Gemini';
            _setStatus(`${speakerLabel} (${voice}) is thinking...`);

            try {
                const result = await _callTurnEndpoint();
                if (_stopRequested) break;

                _addHaveAChatMessage(_currentSpeaker, voice, result.response);
                _chatHistory.push({ speaker: _currentSpeaker, text: result.response });

                // Alternate speaker
                _currentSpeaker = _currentSpeaker === 'claude' ? 'gemini' : 'claude';
                _setStatus('');

                // Brief natural pause between turns (1.5 s)
                await _sleep(1500);

            } catch (err) {
                _setStatus(`Error: ${err.message} — conversation paused.`);
                _isPaused = true;
                _setPauseButtonLabel(true);
                console.error('[HaveAChat] Turn error:', err);
            }
        }

        // Clean up when loop exits
        if (!_stopRequested) stop();
    }

    // ── Public controls ──────────────────────────────────────────────────────
    function start() {
        _claudeVoice   = (_el('have-a-chat-claude-voice')?.value)  || 'expert';
        _geminiVoice   = (_el('have-a-chat-gemini-voice')?.value)  || 'expert';
        _topic         = (_el('have-a-chat-topic')?.value?.trim()) || '';
        _temperature   = parseFloat(_el('have-a-chat-temperature')?.value || '0.7');
        _chatHistory   = [];
        _isRunning     = true;
        _isPaused      = false;
        _stopRequested = false;
        _pendingInjection = null;

        // Randomly decide who goes first
        _currentSpeaker = Math.random() < 0.5 ? 'claude' : 'gemini';

        _closeSetupModal();
        _showControlBar();
        _disableChatForm(true);
        _setPauseButtonLabel(false);
        _setStatus(`Starting — ${_currentSpeaker === 'claude' ? 'Claude' : 'Gemini'} goes first...`);

        _loop();
    }

    function pause() {
        if (!_isRunning || _isPaused) return;
        _isPaused = true;
        _setPauseButtonLabel(true);
        _setStatus('Paused — click Resume to continue.');
    }

    function resume() {
        if (!_isRunning || !_isPaused) return;
        _isPaused = false;
        _setPauseButtonLabel(false);
        _setStatus('Resuming...');
    }

    function stop() {
        _isRunning     = false;
        _isPaused      = false;
        _stopRequested = true;
        _hideControlBar();
        _disableChatForm(false);
        _setStatus('');
    }

    function injectComment() {
        if (!_isRunning) return;
        const modal = _el('have-a-chat-inject-modal');
        if (modal) modal.style.display = 'flex';
        const ta = _el('have-a-chat-inject-text');
        if (ta) { ta.value = ''; ta.focus(); }
    }

    function _submitInjection() {
        const textarea = _el('have-a-chat-inject-text');
        const text = textarea?.value?.trim();
        if (!text) return;
        _pendingInjection = text;
        _closeInjectModal();
        // Auto-resume if paused so the injection is picked up
        if (_isPaused) resume();
    }

    // ── Init: wire all DOM events ────────────────────────────────────────────
    function init() {
        const sidebarBtn = _el('have-a-chat-sidebar-btn');
        if (sidebarBtn) sidebarBtn.addEventListener('click', open);

        // Setup modal
        const closeSetup = _el('close-have-a-chat-setup-modal');
        if (closeSetup) closeSetup.addEventListener('click', _closeSetupModal);
        const cancelBtn = _el('have-a-chat-cancel-btn');
        if (cancelBtn) cancelBtn.addEventListener('click', _closeSetupModal);

        // Temperature slider label
        const tempSlider  = _el('have-a-chat-temperature');
        const tempDisplay = _el('have-a-chat-temperature-display');
        if (tempSlider && tempDisplay) {
            tempSlider.addEventListener('input', () => {
                tempDisplay.textContent = parseFloat(tempSlider.value).toFixed(1);
            });
        }

        const startBtn = _el('have-a-chat-start-btn');
        if (startBtn) startBtn.addEventListener('click', start);

        // Control bar
        const pauseBtn  = _el('have-a-chat-pause-btn');
        const injectBtn = _el('have-a-chat-inject-btn');
        const stopBtn   = _el('have-a-chat-stop-btn');
        if (pauseBtn)  pauseBtn.addEventListener('click', () => _isPaused ? resume() : pause());
        if (injectBtn) injectBtn.addEventListener('click', injectComment);
        if (stopBtn)   stopBtn.addEventListener('click', stop);

        // Inject comment modal
        const closeInject  = _el('close-have-a-chat-inject-modal');
        const injectCancel = _el('have-a-chat-inject-cancel-btn');
        const injectSubmit = _el('have-a-chat-inject-submit-btn');
        if (closeInject)  closeInject.addEventListener('click', _closeInjectModal);
        if (injectCancel) injectCancel.addEventListener('click', _closeInjectModal);
        if (injectSubmit) injectSubmit.addEventListener('click', _submitInjection);

        // Allow Enter key in inject textarea to submit (Shift+Enter for newline)
        const injectText = _el('have-a-chat-inject-text');
        if (injectText) {
            injectText.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    _submitInjection();
                }
            });
        }
    }

    // ── Utility ──────────────────────────────────────────────────────────────
    function _sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    return { init, open, start, pause, resume, stop, injectComment };
})();

// have-a-chat.js is deferred after app.js, so Modals.initAll() has already run.
// Self-initialize here instead.
HaveAChat.init();
