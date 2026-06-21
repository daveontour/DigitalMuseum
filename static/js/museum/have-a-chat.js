'use strict';

// --- Have A Chat Module ---
// Orchestrates an autonomous voice-to-voice conversation. Each voice can be powered
// independently by Claude or Gemini, including both voices using the same LLM.
const HaveAChat = (() => {

    // ── State ────────────────────────────────────────────────────────────────
    let _isRunning      = false;
    let _isPaused       = false;
    let _stopRequested  = false;
    let _currentSlot    = null;   // 'a' or 'b'
    let _chatHistory    = [];     // [{speaker: 'a'|'b'|'user', text}]
    let _voiceA         = 'expert';
    let _voiceB         = 'expert';
    let _providerA      = 'claude';
    let _providerB      = 'gemini';
    let _banterMode     = false;
    let _allowExplicit  = false;
    let _topic          = '';
    let _temperature    = 0.7;
    let _pendingInjection = null;
    let _voiceNames     = {};  // { key: displayName } populated when voices load
    let _turnCount      = 0;   // incremented after each completed turn; prompt shown every 2 turns
    /** Full rounds (A+B) to auto-continue when user clicks Free Run; read from setup at start(). */
    let _freeRunRoundsPreset = 0;
    /** Decremented after each full round while > 0 to skip the round-end pause. */
    let _freeRunRoundsRemaining = 0;
    /** True while a Free Run burst is in progress (also reflected in the status text). */
    let _freeRunActive = false;
    /** Snapshot for post-stop save prompt (null when not showing save dialog). */
    let _pendingSavePayload = null;
    /** Current in-chat thinking bubble element (while waiting on active turn). */
    let _thinkingBubbleEl = null;

    // ── DOM helpers ──────────────────────────────────────────────────────────
    const _el = (id) => document.getElementById(id);

    const FREE_RUN_ROUNDS_MAX = 50;

    function _readFreeRunPresetRounds() {
        const el = _el('have-a-chat-free-run-rounds');
        const raw = el && el.value != null ? String(el.value).trim() : '';
        const n = parseInt(raw, 10);
        if (!Number.isFinite(n) || n < 1) return 0;
        return Math.min(n, FREE_RUN_ROUNDS_MAX);
    }

    /** Rounds to use for Free Run (never 0 when field + preset are broken; default 3). */
    function _resolveFreeRunRoundCountForAction() {
        let n = _readFreeRunPresetRounds();
        if (n < 1) n = _freeRunRoundsPreset;
        if (n < 1) n = 3;
        return Math.min(Math.max(n, 1), FREE_RUN_ROUNDS_MAX);
    }

    /** Leave round-end UI and unblock the async loop (do not gate on _isPaused — it can desync). */
    function _exitRoundPromptUnpause(statusText) {
        _hideRoundPrompt();
        if (!_isRunning) return;
        _isPaused = false;
        _setPauseButtonLabel(false);
        _setStatus(statusText || 'Resuming…');
    }

    /** Shorter delay between AI turns while a free-run burst is active. */
    function _betweenTurnDelayMs() {
        if (_freeRunActive) return 0;
        return _banterMode ? 900 : 1500;
    }

    function _cancelFreeRun(reason) {
        if (_freeRunActive || _freeRunRoundsRemaining > 0) {
            try { console.info('[HaveAChat] Free Run cancelled', reason || ''); } catch (_) { /* ignore */ }
        }
        _freeRunActive = false;
        _freeRunRoundsRemaining = 0;
    }

    function _freeRunStatusText() {
        if (!_freeRunActive || _freeRunRoundsRemaining < 1) return '';
        const r = _freeRunRoundsRemaining;
        return r === 1 ? 'Free Run — 1 round remaining' : `Free Run — ${r} rounds remaining`;
    }

    function _llmDisplayName(provider) {
        if (provider === 'claude') return 'Claude';
        if (provider === 'deepseek') return 'DeepSeek';
        if (provider === 'openai') return 'ChatGPT';
        if (provider === 'localai') return 'Local AI';
        return 'Gemini';
    }

    function _populateVoiceSelects() {
        ApiService.fetchVoices()
            .then(voices => {
                const voiceASel = _el('have-a-chat-voice-a');
                const voiceBSel = _el('have-a-chat-voice-b');
                if (!voiceASel || !voiceBSel || !voices || !voices.length) return;
                voiceASel.innerHTML = '';
                voiceBSel.innerHTML = '';
                voices.forEach(v => {
                    const key  = v.key  || v.value || '';
                    const name = v.name || key;
                    _voiceNames[key] = name;
                    const label = v.description ? `${name} - ${v.description}` : name;
                    const displayText = label + (v.is_custom ? ' *' : '');
                    const o1 = document.createElement('option');
                    o1.value = key;
                    o1.textContent = displayText;
                    voiceASel.appendChild(o1);
                    const o2 = document.createElement('option');
                    o2.value = key;
                    o2.textContent = displayText;
                    voiceBSel.appendChild(o2);
                });
                // Default voice B to a different voice to keep them distinct
                if (voiceBSel.options.length > 1) voiceBSel.selectedIndex = 1;
            })
            .catch(() => { /* leave the default option */ });
    }

    async function _getLLMAvailability() {
        try {
            const res = await fetch('/chat/availability', { credentials: 'same-origin' });
            if (!res.ok) return { localai: false };
            const av = await res.json();
            return { localai: !!av.localai_available };
        } catch (_) {
            return { localai: false };
        }
    }

    async function _applyHaveAChatProviderDefaults() {
        const providerAEl = _el('have-a-chat-provider-a');
        const providerBEl = _el('have-a-chat-provider-b');
        if (!providerAEl || !providerBEl) return;

        const av = await _getLLMAvailability();
        if (!av.localai) return;

        const userSelectedA = providerAEl.dataset.userSelectedProvider === 'true';
        const userSelectedB = providerBEl.dataset.userSelectedProvider === 'true';
        if (!userSelectedA) providerAEl.value = 'localai';
        if (!userSelectedB) providerBEl.value = 'localai';
    }

    // ── Setup modal ──────────────────────────────────────────────────────────
    async function open() {
        _populateVoiceSelects();
        await _applyHaveAChatProviderDefaults();
        const haveAChatExplicit = _el('have-a-chat-allow-explicit');
        const globalExplicit = _el('allow-explicit-content');
        if (haveAChatExplicit && globalExplicit) {
            haveAChatExplicit.checked = !!globalExplicit.checked;
        }
        const modal = _el('have-a-chat-setup-modal');
        if (modal) modal.style.display = 'flex';
    }

    function _closeSetupModal() {
        const modal = _el('have-a-chat-setup-modal');
        if (modal) modal.style.display = 'none';
    }

    function _closeSessionsModal() {
        const modal = _el('have-a-chat-sessions-modal');
        if (modal) modal.style.display = 'none';
    }

    function _formatSessionStamp(iso) {
        if (!iso) return '—';
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return iso;
        return d.toLocaleString();
    }

    function _speakerLabel(s) {
        if (s === 'a') return 'Voice A';
        if (s === 'b') return 'Voice B';
        if (s === 'user') return 'You';
        return s || '?';
    }

    async function openSessionsHistory() {
        const modal = _el('have-a-chat-sessions-modal');
        const listEl = _el('have-a-chat-sessions-list');
        const detailEl = _el('have-a-chat-sessions-detail');
        const errEl = _el('have-a-chat-sessions-error');
        const dlBtn = _el('have-a-chat-sessions-download-btn');
        if (!modal || !listEl || !detailEl) return;
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        if (dlBtn) dlBtn.style.display = 'none';
        detailEl.innerHTML = '<p style="margin:0;color:var(--color-text-muted);font-size:var(--text-sm);">Select a session to view the transcript.</p>';
        listEl.innerHTML = '<p style="margin:0;color:var(--color-text-muted);">Loading…</p>';
        modal.style.display = 'flex';

        const base = _sessionsApiBase();
        let res;
        try {
            res = await fetch(base + '?limit=50', { credentials: 'same-origin' });
        } catch (e) {
            listEl.innerHTML = '';
            if (errEl) {
                errEl.textContent = 'Could not load sessions (network error).';
                errEl.style.display = 'block';
            }
            return;
        }
        if (!res.ok) {
            listEl.innerHTML = '';
            let msg = `Could not load sessions (HTTP ${res.status}).`;
            if (res.status === 403) msg = 'Unlock the archive keyring to view saved Have a Chat sessions.';
            if (errEl) {
                errEl.textContent = msg;
                errEl.style.display = 'block';
            }
            return;
        }
        const data = await res.json().catch(() => ({}));
        const sessions = (data && data.sessions) || [];
        listEl.innerHTML = '';
        if (!sessions.length) {
            listEl.innerHTML = '<p style="margin:0;color:var(--color-text-muted);">No saved conversations yet. When you stop a run, choose <strong>Save to archive</strong> to keep the transcript here.</p>';
            return;
        }
        const ul = document.createElement('ul');
        ul.style.cssText = 'list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:6px;max-height:220px;overflow-y:auto;';
        sessions.forEach((s) => {
            const li = document.createElement('li');
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'modal-btn modal-btn-secondary';
            btn.style.cssText = 'width:100%;text-align:left;justify-content:flex-start;font-size:var(--text-sm);';
            const when = _formatSessionStamp(s.stopped_at || s.created_at);
            const topic = (s.topic && String(s.topic).trim()) ? String(s.topic).trim().slice(0, 80) : '(no topic)';
            const turns = typeof s.turn_count === 'number' ? s.turn_count : '';
            btn.innerHTML = `<span style="font-weight:600;">${when}</span><span style="opacity:0.85;"> — ${topic}${turns !== '' ? ` (${turns} turns)` : ''}</span>`;
            btn.addEventListener('click', () => {
                void _loadSessionDetail(s.id, detailEl, errEl, dlBtn);
            });
            li.appendChild(btn);
            ul.appendChild(li);
        });
        listEl.appendChild(ul);
    }

    async function _loadSessionDetail(id, detailEl, errEl, dlBtn) {
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        if (dlBtn) dlBtn.style.display = 'none';
        detailEl.innerHTML = '<p style="margin:0;color:var(--color-text-muted);">Loading…</p>';
        const base = _sessionsApiBase();
        const res = await fetch(`${base}/${id}`, { credentials: 'same-origin' });
        if (!res.ok) {
            detailEl.innerHTML = '';
            if (errEl) {
                errEl.textContent = res.status === 404 ? 'Session not found.' : `Could not load session (HTTP ${res.status}).`;
                errEl.style.display = 'block';
            }
            return;
        }
        const d = await res.json();
        if (dlBtn) {
            dlBtn.style.display = 'inline-flex';
            dlBtn.dataset.sessionId = String(id);
        }
        const metaBits = [
            `Stopped: ${_formatSessionStamp(d.stopped_at || d.created_at)}`,
            `Voice A: ${d.voice_a} (${d.provider_a})`,
            `Voice B: ${d.voice_b} (${d.provider_b})`,
            `Banter: ${d.banter_mode ? 'on' : 'off'} · Temp: ${d.temperature} · Explicit: ${d.allow_explicit ? 'allowed' : 'off'}`,
        ];
        const topicHtml = (d.topic && String(d.topic).trim())
            ? `<p style="margin:0 0 10px 0;"><strong>Topic:</strong> ${String(d.topic).replace(/</g, '&lt;').replace(/>/g, '&gt;')}</p>`
            : '';
        const parts = [];
        parts.push(`<div style="font-size:var(--text-sm);color:var(--color-text-muted);margin-bottom:12px;line-height:1.5;">${metaBits.map((x) => x.replace(/</g, '&lt;')).join('<br>')}</div>`);
        parts.push(topicHtml);
        parts.push('<div style="border-top:1px solid var(--color-border);padding-top:12px;max-height:280px;overflow-y:auto;font-size:var(--text-md);line-height:1.45;">');
        (d.history || []).forEach((turn, i) => {
            const lab = _speakerLabel(turn.speaker);
            const body = String(turn.text || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
            parts.push(`<p style="margin:0 0 14px 0;"><strong>${i + 1}. ${lab}</strong><br><span style="white-space:pre-wrap;">${body}</span></p>`);
        });
        parts.push('</div>');
        detailEl.innerHTML = parts.join('');
    }

    async function _downloadSessionMarkdown() {
        const dlBtn = _el('have-a-chat-sessions-download-btn');
        const id = dlBtn && dlBtn.dataset.sessionId;
        if (!id) return;
        const base = _sessionsApiBase();
        const res = await fetch(`${base}/${id}/markdown`, { credentials: 'same-origin' });
        if (!res.ok) {
            try { console.warn('[HaveAChat] Markdown export failed', res.status); } catch (_) { /* ignore */ }
            return;
        }
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `have-a-chat-${id}.md`;
        a.rel = 'noopener';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
    }

    function _closeInjectModal() {
        const modal = _el('have-a-chat-inject-modal');
        if (modal) modal.style.display = 'none';
    }

    // ── Message rendering ────────────────────────────────────────────────────
    // slot: 'a' or 'b'; provider: which LLM actually responded;
    // voiceKey: voice personality key; text: markdown string
    function _addHaveAChatMessage(slot, provider, voiceKey, text) {
        const msgEl = document.createElement('div');
        // Slot A always renders with blue styling, slot B with purple —
        // regardless of which LLM is powering the voice.
        const slotClass = slot === 'a' ? 'have-a-chat-claude' : 'have-a-chat-gemini';
        msgEl.classList.add('have-a-chat-message', slotClass);

        // Header: voice image + voice name + LLM badge
        const header = document.createElement('div');
        header.className = 'have-a-chat-header';

        const img = document.createElement('img');
        img.className = 'have-a-chat-voice-image';
        img.src = `/static/images/${typeof VoiceSelector !== 'undefined' ? VoiceSelector.getVoiceImage(voiceKey, true) : voiceKey + '_sm.png'}`;
        img.alt = voiceKey;
        img.onerror = () => { img.style.display = 'none'; };

        // Voice name (primary identity)
        const nameLabel = document.createElement('span');
        nameLabel.className = 'have-a-chat-label';
        nameLabel.textContent = _voiceNames[voiceKey] || voiceKey;

        // LLM badge (secondary — shows which model is speaking)
        const llmBadge = document.createElement('span');
        llmBadge.className = 'have-a-chat-llm-badge have-a-chat-llm-' + provider;
        llmBadge.textContent = _llmDisplayName(provider);

        header.appendChild(img);
        header.appendChild(nameLabel);
        header.appendChild(llmBadge);
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

    function _removeThinkingBubble() {
        if (_thinkingBubbleEl) {
            _thinkingBubbleEl.remove();
            _thinkingBubbleEl = null;
        }
    }

    function _showThinkingBubble(slot, provider, voiceKey) {
        _removeThinkingBubble();
        if (!DOM || !DOM.chatBox) return;

        const msgEl = document.createElement('div');
        const slotClass = slot === 'a' ? 'have-a-chat-claude' : 'have-a-chat-gemini';
        msgEl.classList.add('have-a-chat-message', slotClass, 'have-a-chat-thinking-message');

        const header = document.createElement('div');
        header.className = 'have-a-chat-header';

        const img = document.createElement('img');
        img.className = 'have-a-chat-voice-image';
        img.src = `/static/images/${typeof VoiceSelector !== 'undefined' ? VoiceSelector.getVoiceImage(voiceKey, true) : voiceKey + '_sm.png'}`;
        img.alt = voiceKey;
        img.onerror = () => { img.style.display = 'none'; };

        const nameLabel = document.createElement('span');
        nameLabel.className = 'have-a-chat-label';
        nameLabel.textContent = _voiceNames[voiceKey] || voiceKey;

        const llmBadge = document.createElement('span');
        llmBadge.className = 'have-a-chat-llm-badge have-a-chat-llm-' + provider;
        llmBadge.textContent = _llmDisplayName(provider);

        header.appendChild(img);
        header.appendChild(nameLabel);
        header.appendChild(llmBadge);
        msgEl.appendChild(header);

        const contentEl = document.createElement('div');
        contentEl.className = 'have-a-chat-thinking-content';
        contentEl.innerHTML = `
            <span class="have-a-chat-thinking-text">Thinking</span>
            <span class="have-a-chat-thinking-dots" aria-hidden="true">
                <span></span><span></span><span></span>
            </span>
        `;
        msgEl.appendChild(contentEl);

        DOM.chatBox.appendChild(msgEl);
        _thinkingBubbleEl = msgEl;
        if (typeof UI !== 'undefined' && UI.scrollToBottom) UI.scrollToBottom();
    }

    // ── Control bar helpers ──────────────────────────────────────────────────
    function _syncContextStatusBar() {
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
        }
    }

    function _showControlBar() {
        const bar = _el('have-a-chat-control-bar');
        if (bar) bar.style.display = 'flex';
        _syncContextStatusBar();
    }

    function _hideControlBar() {
        const bar = _el('have-a-chat-control-bar');
        if (bar) bar.style.display = 'none';
        _syncContextStatusBar();
    }

    function _showRoundPrompt() {
        const bar    = _el('have-a-chat-control-bar');
        const prompt = _el('have-a-chat-round-prompt');
        if (bar)    bar.style.display    = 'none';
        if (prompt) prompt.style.display = 'flex';
        _syncContextStatusBar();
    }

    function _hideRoundPrompt() {
        const bar    = _el('have-a-chat-control-bar');
        const prompt = _el('have-a-chat-round-prompt');
        if (prompt) prompt.style.display = 'none';
        if (bar)    bar.style.display    = 'flex';
        _syncContextStatusBar();
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
        const micBtn  = _el('chat-voice-input-btn');
        if (sendBtn) sendBtn.disabled = disabled;
        if (input)   input.disabled   = disabled;
        if (micBtn)  micBtn.disabled  = disabled;
        if (disabled && typeof ChatVoiceInput !== 'undefined' && ChatVoiceInput.stop) {
            ChatVoiceInput.stop();
        }
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
                speaking_slot: _currentSlot,
                voice_a:       _voiceA,
                voice_b:       _voiceB,
                provider_a:    _providerA,
                provider_b:    _providerB,
                topic:         _topic,
                history:       _chatHistory,
                temperature:   _temperature,
                banter_mode:   _banterMode,
                allowExplicitContent: _allowExplicit,
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

            const voiceKey  = _currentSlot === 'a' ? _voiceA : _voiceB;
            const provider  = _currentSlot === 'a' ? _providerA : _providerB;
            const voiceName = _voiceNames[voiceKey] || voiceKey;
            const llmLabel  = _llmDisplayName(provider);
            _setStatus(`${voiceName} (${llmLabel}) is thinking…`);
            _showThinkingBubble(_currentSlot, provider, voiceKey);

            try {
                const result = await _callTurnEndpoint();
                _removeThinkingBubble();
                if (_stopRequested) break;

                // Use provider from response (handles fallback cases)
                const actualProvider = result.provider || provider;
                _addHaveAChatMessage(_currentSlot, actualProvider, voiceKey, result.response);
                _chatHistory.push({ speaker: _currentSlot, text: result.response });

                _turnCount++;

                // Alternate slot
                _currentSlot = _currentSlot === 'a' ? 'b' : 'a';

                // After every complete round (both voices have spoken), pause and prompt — unless Free Run still has rounds left
                if (_turnCount % 2 === 0) {
                    if (_freeRunActive && _freeRunRoundsRemaining > 0) {
                        _freeRunRoundsRemaining--;
                        try { console.info('[HaveAChat] Free Run round consumed; remaining =', _freeRunRoundsRemaining); } catch (_) { /* ignore */ }
                        if (_freeRunRoundsRemaining <= 0) {
                            _freeRunActive = false;
                            _setStatus('Free Run complete — pausing at next round end.');
                        } else {
                            _setStatus(_freeRunStatusText());
                        }
                        // No artificial inter-round delay during or at the tail of a burst.
                    } else {
                        _setStatus('');
                        _freeRunActive = false;
                        _isPaused = true;
                        _showRoundPrompt();
                        while (_isPaused && !_stopRequested) {
                            await _sleep(150);
                        }
                        if (_stopRequested) break;
                    }
                } else {
                    if (_freeRunActive) {
                        _setStatus(_freeRunStatusText());
                    } else {
                        _setStatus('');
                    }
                    await _sleep(_betweenTurnDelayMs());
                }

            } catch (err) {
                _removeThinkingBubble();
                _setStatus(`Error: ${err.message} — conversation paused.`);
                _isPaused = true;
                _setPauseButtonLabel(true);
                console.error('[HaveAChat] Turn error:', err);
            }
        }

        if (!_stopRequested) stop();
    }

    // ── Public controls ──────────────────────────────────────────────────────
    function start() {
        _voiceA      = (_el('have-a-chat-voice-a')?.value)     || 'expert';
        _voiceB      = (_el('have-a-chat-voice-b')?.value)     || 'expert';
        _providerA   = (_el('have-a-chat-provider-a')?.value)  || 'claude';
        _providerB   = (_el('have-a-chat-provider-b')?.value)  || 'gemini';
        _banterMode  = (_el('have-a-chat-banter-mode')?.checked) || false;
        _allowExplicit = (_el('have-a-chat-allow-explicit')?.checked) || false;
        _topic       = (_el('have-a-chat-topic')?.value?.trim()) || '';
        _temperature = parseFloat(_el('have-a-chat-temperature')?.value || '0.7');
        _chatHistory   = [];
        _turnCount     = 0;
        _freeRunRoundsPreset = _readFreeRunPresetRounds();
        if (_freeRunRoundsPreset < 1) _freeRunRoundsPreset = 3;
        _freeRunRoundsRemaining = 0;
        _freeRunActive = false;
        _isRunning     = true;
        _isPaused      = false;
        _stopRequested = false;
        _pendingInjection = null;
        try { console.info('[HaveAChat] start; freeRunPreset =', _freeRunRoundsPreset); } catch (_) { /* ignore */ }

        // Randomly decide who goes first
        _currentSlot = Math.random() < 0.5 ? 'a' : 'b';

        _closeSetupModal();
        _showControlBar();
        _disableChatForm(true);
        _setPauseButtonLabel(false);

        const firstVoice = _currentSlot === 'a' ? (_voiceNames[_voiceA] || _voiceA) : (_voiceNames[_voiceB] || _voiceB);
        const firstLLM   = _currentSlot === 'a' ? _providerA : _providerB;
        const firstLLMLabel = _llmDisplayName(firstLLM);
        _setStatus(`Starting — ${firstVoice} (${firstLLMLabel}) goes first…`);

        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('have_a_chat', true);
        }

        _loop();
    }

    function pause() {
        if (!_isRunning || _isPaused) return;
        _cancelFreeRun('pause');
        _isPaused = true;
        _setPauseButtonLabel(true);
        _setStatus('Paused — click Resume to continue.');
    }

    function resume() {
        if (!_isRunning || !_isPaused) return;
        _hideRoundPrompt();
        _isPaused = false;
        _setPauseButtonLabel(false);
        _setStatus('Resuming…');
    }

    function _closeSavePromptModal() {
        const modal = _el('have-a-chat-save-prompt-modal');
        if (modal) modal.style.display = 'none';
        const err = _el('have-a-chat-save-prompt-error');
        if (err) {
            err.textContent = '';
            err.style.display = 'none';
        }
        const saveBtn = _el('have-a-chat-save-prompt-save-btn');
        if (saveBtn) saveBtn.disabled = false;
        _pendingSavePayload = null;
    }

    function _openSavePromptAfterStop() {
        const history = _chatHistory.slice();
        if (!history.length) return;
        _pendingSavePayload = {
            topic: _topic,
            voice_a: _voiceA,
            voice_b: _voiceB,
            provider_a: _providerA,
            provider_b: _providerB,
            banter_mode: _banterMode,
            temperature: _temperature,
            allow_explicit: _allowExplicit,
            turn_count: _turnCount > 0 ? _turnCount : history.length,
            history,
        };
        const err = _el('have-a-chat-save-prompt-error');
        if (err) {
            err.textContent = '';
            err.style.display = 'none';
        }
        const tc = _el('have-a-chat-save-prompt-turn-count');
        if (tc) tc.textContent = String(history.length);
        const saveBtn = _el('have-a-chat-save-prompt-save-btn');
        if (saveBtn) saveBtn.disabled = false;
        const modal = _el('have-a-chat-save-prompt-modal');
        if (modal) modal.style.display = 'flex';
    }

    async function _submitPendingSave() {
        if (!_pendingSavePayload) return;
        const err = _el('have-a-chat-save-prompt-error');
        const saveBtn = _el('have-a-chat-save-prompt-save-btn');
        if (saveBtn) saveBtn.disabled = true;
        const base = _sessionsApiBase();
        try {
            const res = await fetch(base, {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(_pendingSavePayload),
            });
            if (!res.ok) {
                const d = await res.json().catch(() => ({}));
                let msg = (d && (d.error || d.detail)) || `HTTP ${res.status}`;
                if (res.status === 403) msg = 'Unlock the archive keyring to save this conversation.';
                if (err) {
                    err.textContent = msg;
                    err.style.display = 'block';
                }
                if (saveBtn) saveBtn.disabled = false;
                return;
            }
            try { console.info('[HaveAChat] Session saved to archive'); } catch (_) { /* ignore */ }
            _closeSavePromptModal();
        } catch (e) {
            if (err) {
                err.textContent = 'Could not reach the server. Try again.';
                err.style.display = 'block';
            }
            if (saveBtn) saveBtn.disabled = false;
        }
    }

    function _sessionsApiBase() {
        return (typeof CONSTANTS !== 'undefined' && CONSTANTS.API_PATHS && CONSTANTS.API_PATHS.HAVE_A_CHAT_SESSIONS)
            ? CONSTANTS.API_PATHS.HAVE_A_CHAT_SESSIONS
            : '/api/have-a-chat/sessions';
    }

    function stop() {
        _isRunning     = false;
        _isPaused      = false;
        _stopRequested = true;
        _removeThinkingBubble();
        _cancelFreeRun('stop');
        const hadHistory = _chatHistory.length > 0;
        _hideControlBar();
        const prompt = _el('have-a-chat-round-prompt');
        if (prompt) prompt.style.display = 'none';
        _disableChatForm(false);
        _setStatus('');
        _syncContextStatusBar();
        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('have_a_chat', false);
        }
        if (hadHistory) _openSavePromptAfterStop();
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
        if (_isPaused) resume();
    }

    // ── Init: wire all DOM events ────────────────────────────────────────────
    function init() {
        if (document.body.dataset.haveAChatInit === '1') return;
        document.body.dataset.haveAChatInit = '1';

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

        const prevBtn = _el('have-a-chat-previous-sessions-btn');
        if (prevBtn) prevBtn.addEventListener('click', () => { void openSessionsHistory(); });

        const closeSess = _el('close-have-a-chat-sessions-modal');
        const closeSess2 = _el('have-a-chat-sessions-close-btn');
        if (closeSess) closeSess.addEventListener('click', _closeSessionsModal);
        if (closeSess2) closeSess2.addEventListener('click', _closeSessionsModal);

        const dlMd = _el('have-a-chat-sessions-download-btn');
        if (dlMd) dlMd.addEventListener('click', () => { void _downloadSessionMarkdown(); });

        const closeSavePrompt = _el('close-have-a-chat-save-prompt-modal');
        const discardSavePrompt = _el('have-a-chat-save-prompt-discard-btn');
        const confirmSavePrompt = _el('have-a-chat-save-prompt-save-btn');
        if (closeSavePrompt) closeSavePrompt.addEventListener('click', _closeSavePromptModal);
        if (discardSavePrompt) discardSavePrompt.addEventListener('click', _closeSavePromptModal);
        if (confirmSavePrompt) confirmSavePrompt.addEventListener('click', () => { void _submitPendingSave(); });

        const providerAEl = _el('have-a-chat-provider-a');
        const providerBEl = _el('have-a-chat-provider-b');
        if (providerAEl) {
            providerAEl.addEventListener('change', () => {
                providerAEl.dataset.userSelectedProvider = 'true';
            });
        }
        if (providerBEl) {
            providerBEl.addEventListener('change', () => {
                providerBEl.dataset.userSelectedProvider = 'true';
            });
        }

        // Control bar
        const pauseBtn  = _el('have-a-chat-pause-btn');
        const injectBtn = _el('have-a-chat-inject-btn');
        const stopBtn   = _el('have-a-chat-stop-btn');
        if (pauseBtn)  pauseBtn.addEventListener('click', () => _isPaused ? resume() : pause());
        if (injectBtn) injectBtn.addEventListener('click', () => {
            _cancelFreeRun('inject-mid-round');
            injectComment();
        });
        if (stopBtn)   stopBtn.addEventListener('click', stop);

        // Round-end prompt
        const continueBtn       = _el('have-a-chat-continue-btn');
        const freeRunBtn        = _el('have-a-chat-free-run-btn');
        const injectRoundBtn    = _el('have-a-chat-inject-round-btn');
        const stopRoundBtn      = _el('have-a-chat-stop-round-btn');
        if (continueBtn)    continueBtn.addEventListener('click', () => {
            _cancelFreeRun('continue');
            _exitRoundPromptUnpause();
        });
        if (freeRunBtn) freeRunBtn.addEventListener('click', () => {
            const n = _resolveFreeRunRoundCountForAction();
            _freeRunRoundsRemaining = n;
            _freeRunActive = n > 0;
            try { console.info('[HaveAChat] Free Run requested; rounds =', n); } catch (_) { /* ignore */ }
            _exitRoundPromptUnpause(_freeRunActive ? _freeRunStatusText() : 'Resuming…');
        });
        if (injectRoundBtn) injectRoundBtn.addEventListener('click', () => {
            _cancelFreeRun('inject-round');
            injectComment();
            // injectComment resumes automatically when the injection is submitted
        });
        if (stopRoundBtn)   stopRoundBtn.addEventListener('click', stop);

        // Inject comment modal
        const closeInject  = _el('close-have-a-chat-inject-modal');
        const injectCancel = _el('have-a-chat-inject-cancel-btn');
        const injectSubmit = _el('have-a-chat-inject-submit-btn');
        if (closeInject)  closeInject.addEventListener('click', _closeInjectModal);
        if (injectCancel) injectCancel.addEventListener('click', _closeInjectModal);
        if (injectSubmit) injectSubmit.addEventListener('click', _submitInjection);

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

    return { init, open, start, pause, resume, stop, injectComment, openSessionsHistory };
})();

// have-a-chat.js is deferred after app.js, so Modals.initAll() has already run.
// Self-initialize here instead.
HaveAChat.init();
