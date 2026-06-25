'use strict';

const InterviewerMode = (() => {
    let _activeInterviewID = null;
    let _isActive = false;
    let _turnNumber = 0;

    // ── DOM references (resolved once on init) ─────────────────────────────
    let _setupModal, _closeSetupBtn, _startBtn, _cancelBtn;
    let _styleGrid, _purposeGrid, _detailInput, _providerSelect;
    let _resumeSection, _resumeSelect, _resumeBtn;
    let _finishedSection, _finishedSelect, _viewBtn;
    let _controlBar, _statusText, _turnBadge, _pauseBtn, _endBtn;

    function init() {
        _setupModal      = document.getElementById('interview-setup-modal');
        _closeSetupBtn   = document.getElementById('close-interview-setup-modal');
        _startBtn        = document.getElementById('interview-start-btn');
        _cancelBtn       = document.getElementById('interview-cancel-btn');
        _styleGrid       = document.getElementById('interview-style-grid');
        _purposeGrid     = document.getElementById('interview-purpose-grid');
        _detailInput     = document.getElementById('interview-purpose-detail');
        _providerSelect  = document.getElementById('interview-provider');
        _resumeSection   = document.getElementById('interview-resume-section');
        _resumeSelect    = document.getElementById('interview-resume-select');
        _resumeBtn       = document.getElementById('interview-resume-btn');
        _finishedSection = document.getElementById('interview-finished-section');
        _finishedSelect  = document.getElementById('interview-finished-select');
        _viewBtn         = document.getElementById('interview-view-btn');
        _controlBar      = document.getElementById('interview-control-bar');
        _statusText      = document.getElementById('interview-status-text');
        _turnBadge       = document.getElementById('interview-turn-badge');
        _pauseBtn        = document.getElementById('interview-pause-btn');
        _endBtn          = document.getElementById('interview-end-btn');

        if (!_setupModal) return;

        const sidebarBtn = document.getElementById('interviewer-sidebar-btn');
        if (sidebarBtn) sidebarBtn.addEventListener('click', openSetup);

        _closeSetupBtn.addEventListener('click', _closeSetup);
        _cancelBtn.addEventListener('click', _closeSetup);
        _startBtn.addEventListener('click', _startInterview);
        _pauseBtn.addEventListener('click', _pauseInterview);
        _endBtn.addEventListener('click', _endInterview);
        if (_resumeBtn) _resumeBtn.addEventListener('click', _resumeInterview);
        if (_viewBtn) _viewBtn.addEventListener('click', _viewFinishedInterview);

        _setupModal.addEventListener('click', (e) => {
            if (e.target === _setupModal) _closeSetup();
        });
    }

    // ── Setup modal ────────────────────────────────────────────────────────
    async function openSetup() {
        if (!_setupModal) return;
        await _loadPausedInterviews();
        Modals._openModal(_setupModal);
    }

    function _closeSetup() {
        if (_setupModal) Modals._closeModal(_setupModal);
    }

    function _getSelectedRadio(name) {
        const el = document.querySelector(`input[name="${name}"]:checked`);
        return el ? el.value : '';
    }

    // ── Load paused + finished interviews for resume / view ────────────────
    async function _loadPausedInterviews() {
        try {
            const resp = await fetch('/interview/list');
            const data = await resp.json();
            const all = data.interviews || [];

            const paused = all.filter(iv => iv.state === 'paused');
            if (_resumeSection && _resumeSelect) {
                _resumeSelect.innerHTML = '<option value="">— select —</option>';
                paused.forEach(iv => {
                    const opt = document.createElement('option');
                    opt.value = iv.id;
                    opt.textContent = `${iv.title} (${iv.turn_count || 0} turns)`;
                    _resumeSelect.appendChild(opt);
                });
                _resumeSection.style.display = paused.length > 0 ? '' : 'none';
            }

            const finished = all.filter(iv => iv.state === 'finished');
            if (_finishedSection && _finishedSelect) {
                _finishedSelect.innerHTML = '<option value="">— select —</option>';
                finished.forEach(iv => {
                    const opt = document.createElement('option');
                    opt.value = iv.id;
                    const date = iv.updated_at ? new Date(iv.updated_at).toLocaleDateString() : '';
                    const w = iv.has_writeup ? ' · writeup saved' : '';
                    opt.textContent = `${iv.title} — ${date} (${iv.turn_count || 0} turns)${w}`;
                    _finishedSelect.appendChild(opt);
                });
                _finishedSection.style.display = finished.length > 0 ? '' : 'none';
            }
        } catch (err) {
            console.error('Failed to load interviews:', err);
            if (_resumeSection) _resumeSection.style.display = 'none';
            if (_finishedSection) _finishedSection.style.display = 'none';
        }
    }

    async function _viewFinishedInterview() {
        const id = _finishedSelect ? parseInt(_finishedSelect.value, 10) : 0;
        if (!id) return;

        _viewBtn.disabled = true;
        _viewBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Loading...';

        try {
            const resp = await fetch(`/interview/${id}`);
            if (!resp.ok) throw new Error('Failed to load interview');
            const data = await resp.json();
            const iv = data.interview;
            const turns = data.turns || [];

            _closeSetup();
            _clearChatForInterview();

            // Show the Q&A transcript
            turns.forEach(t => {
                _addQuestionMessage(t.question, t.turn_number);
                if (t.answer) _addAnswerMessage(t.answer);
            });

            // Show the writeup if available
            if (iv.writeup) {
                _displayWriteup(iv.writeup);
            } else {
                _displayError('No saved writeup was found for this interview.');
            }
        } catch (err) {
            console.error('View interview error:', err);
            if (typeof AppDialogs !== 'undefined') {
                AppDialogs.showAppAlert('Error', err.message);
            }
        } finally {
            _viewBtn.disabled = false;
            _viewBtn.innerHTML = '<i class="fas fa-eye"></i> Load in chat';
        }
    }

    // ── Start a new interview ──────────────────────────────────────────────
    async function _startInterview() {
        const style   = _getSelectedRadio('interview-style');
        const purpose = _getSelectedRadio('interview-purpose');
        const detail  = _detailInput ? _detailInput.value.trim() : '';
        const provider = _providerSelect ? _providerSelect.value : 'gemini';

        if (!style || !purpose) return;

        _startBtn.disabled = true;
        _startBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Starting...';

        try {
            const resp = await fetch('/interview/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    style: style,
                    purpose: purpose,
                    purpose_detail: detail,
                    provider: provider
                })
            });
            if (!resp.ok) {
                const err = await resp.json();
                throw new Error(err.detail || 'Failed to start interview');
            }
            const data = await resp.json();
            _activeInterviewID = data.interview_id;
            _turnNumber = data.turn_number;
            _isActive = true;

            _closeSetup();
            _enterInterviewMode();
            _clearChatForInterview();
            _addQuestionMessage(data.question, data.turn_number);
            _enableAnswerInput();
        } catch (err) {
            console.error('Start interview error:', err);
            if (typeof AppDialogs !== 'undefined') {
                AppDialogs.showAppAlert('Error', err.message);
            }
        } finally {
            _startBtn.disabled = false;
            _startBtn.innerHTML = '<i class="fas fa-microphone-alt"></i> Start Interview';
        }
    }

    // ── Resume a paused interview ──────────────────────────────────────────
    async function _resumeInterview() {
        const id = _resumeSelect ? parseInt(_resumeSelect.value, 10) : 0;
        if (!id) return;

        _resumeBtn.disabled = true;
        _resumeBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Resuming...';

        try {
            // Load existing turns first
            const turnsResp = await fetch(`/interview/${id}/turns`);
            const turnsData = await turnsResp.json();

            const resp = await fetch('/interview/resume', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ interview_id: id })
            });
            if (!resp.ok) {
                const err = await resp.json();
                throw new Error(err.detail || 'Failed to resume interview');
            }
            const data = await resp.json();
            _activeInterviewID = data.interview_id;
            _turnNumber = data.turn_number;
            _isActive = true;

            _closeSetup();
            _enterInterviewMode();
            _clearChatForInterview();

            // Replay existing turns
            const turns = turnsData.turns || [];
            turns.forEach(t => {
                _addQuestionMessage(t.question, t.turn_number);
                if (t.answer) {
                    _addAnswerMessage(t.answer);
                }
            });

            // Show the resume question
            _addQuestionMessage(data.question, data.turn_number);
            _enableAnswerInput();
        } catch (err) {
            console.error('Resume interview error:', err);
            if (typeof AppDialogs !== 'undefined') {
                AppDialogs.showAppAlert('Error', err.message);
            }
        } finally {
            _resumeBtn.disabled = false;
            _resumeBtn.innerHTML = '<i class="fas fa-play"></i> Resume Selected';
        }
    }

    // ── Submit answer and get next question ────────────────────────────────
    async function _submitAnswer() {
        if (!_activeInterviewID || !_isActive) return;

        const input = DOM.userInput;
        const answer = input.value.trim();
        if (!answer) return;

        _addAnswerMessage(answer);
        input.value = '';
        _showLoading();
        _setControlsEnabled(false);

        try {
            const resp = await fetch('/interview/turn', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    interview_id: _activeInterviewID,
                    answer: answer
                })
            });
            if (!resp.ok) {
                const err = await resp.json();
                throw new Error(err.detail || 'Failed to get next question');
            }
            const data = await resp.json();
            _turnNumber = data.turn_number;
            _hideLoading();
            _addQuestionMessage(data.question, data.turn_number);
            _updateControlBar();
            _setControlsEnabled(true);
        } catch (err) {
            console.error('Interview turn error:', err);
            _hideLoading();
            _displayError(err.message);
            _setControlsEnabled(true);
        }
    }

    // ── Pause / End ────────────────────────────────────────────────────────
    async function _pauseInterview() {
        if (!_activeInterviewID) return;
        try {
            await fetch('/interview/pause', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ interview_id: _activeInterviewID })
            });
            _exitInterviewMode();
        } catch (err) {
            console.error('Pause interview error:', err);
        }
    }

    async function _endInterview() {
        if (!_activeInterviewID) return;

        const confirmed = confirm(
            'End this interview?\n\nThe AI will generate a written piece based on your responses. This may take a moment.'
        );
        if (!confirmed) return;

        _setControlsEnabled(false);
        _endBtn.disabled = true;
        _pauseBtn.disabled = true;
        if (_statusText) _statusText.textContent = 'Generating writeup…';

        _showWriteupLoading();

        try {
            const resp = await fetch('/interview/end', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ interview_id: _activeInterviewID })
            });
            if (!resp.ok) {
                const err = await resp.json();
                throw new Error(err.detail || 'Failed to end interview');
            }
            const data = await resp.json();
            _hideWriteupLoading();
            if (data.writeup) {
                _displayWriteup(data.writeup);
            } else {
                _displayError('Interview ended, but no writeup was returned.');
            }
            _exitInterviewMode();
        } catch (err) {
            console.error('End interview error:', err);
            _hideWriteupLoading();
            _displayError(err.message);
            _setControlsEnabled(true);
            if (_endBtn) _endBtn.disabled = false;
            if (_pauseBtn) _pauseBtn.disabled = false;
        }
    }

    function _showWriteupLoading() {
        const el = document.createElement('div');
        el.className = 'interview-loading';
        el.id = 'interview-writeup-loading';
        el.innerHTML = '<i class="fas fa-pen-fancy" style="color:#4a90e2;"></i> Generating writeup — this may take a minute <span class="interview-dots"><span></span><span></span><span></span></span>';
        _appendToChat(el);
    }

    function _hideWriteupLoading() {
        const el = document.getElementById('interview-writeup-loading');
        if (el) el.remove();
    }

    function _displayWriteup(text) {
        const wrapper = document.createElement('div');
        wrapper.className = 'interview-writeup';

        const header = document.createElement('div');
        header.className = 'interview-writeup-header';
        header.innerHTML = '<i class="fas fa-file-alt"></i> Interview Writeup';
        wrapper.appendChild(header);

        const saved = document.createElement('div');
        saved.className = 'interview-writeup-saved-note';
        saved.innerHTML =
            '<i class="fas fa-database" aria-hidden="true"></i> Saved to your archive. Open <strong>Interviewer</strong> → <strong>Saved interviews</strong> anytime to load this writeup again.';
        wrapper.appendChild(saved);

        const body = document.createElement('div');
        body.className = 'interview-writeup-body';
        if (typeof marked !== 'undefined') {
            body.innerHTML = marked.parse(text);
        } else {
            body.textContent = text;
        }
        wrapper.appendChild(body);

        const actions = document.createElement('div');
        actions.className = 'interview-writeup-actions';
        const copyBtn = document.createElement('button');
        copyBtn.type = 'button';
        copyBtn.className = 'interview-writeup-copy-btn';
        copyBtn.innerHTML = '<i class="fas fa-copy"></i> Copy to clipboard';
        copyBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(text).then(() => {
                copyBtn.innerHTML = '<i class="fas fa-check"></i> Copied!';
                setTimeout(() => { copyBtn.innerHTML = '<i class="fas fa-copy"></i> Copy to clipboard'; }, 2000);
            });
        });
        actions.appendChild(copyBtn);

        const dlBtn = document.createElement('button');
        dlBtn.type = 'button';
        dlBtn.className = 'interview-writeup-copy-btn';
        dlBtn.innerHTML = '<i class="fas fa-download"></i> Download .md';
        dlBtn.addEventListener('click', () => {
            const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `interview-writeup-${new Date().toISOString().slice(0, 10)}.md`;
            a.click();
            URL.revokeObjectURL(url);
        });
        actions.appendChild(dlBtn);

        wrapper.appendChild(actions);

        _appendToChat(wrapper);
    }

    // ── Mode enter / exit ──────────────────────────────────────────────────
    function _enterInterviewMode() {
        _isActive = true;
        if (_controlBar) _controlBar.style.display = '';
        _updateControlBar();
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
        }

        // Disable normal chat form and repurpose for interview answers
        const chatForm = DOM.chatForm || document.getElementById('chat-form');
        const sendBtn = DOM.sendButton || document.getElementById('send-button');
        if (chatForm) {
            chatForm._originalSubmitHandler = chatForm.onsubmit;
            chatForm.onsubmit = null;
            chatForm.addEventListener('submit', _handleFormSubmit);
        }
        if (sendBtn) {
            sendBtn._originalText = sendBtn.textContent;
            sendBtn.textContent = 'Answer';
        }

        // Hide the loading indicator if visible
        const loadingEl = document.getElementById('loading-indicator');
        if (loadingEl) loadingEl.style.display = 'none';

        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('interviewer', true);
        }
    }

    function _exitInterviewMode() {
        _isActive = false;
        _activeInterviewID = null;
        _turnNumber = 0;
        if (_controlBar) _controlBar.style.display = 'none';
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
        }

        const chatForm = DOM.chatForm || document.getElementById('chat-form');
        const sendBtn = DOM.sendButton || document.getElementById('send-button');
        const input = DOM.userInput || document.getElementById('user-input');
        if (chatForm) {
            chatForm.removeEventListener('submit', _handleFormSubmit);
            if (chatForm._originalSubmitHandler) {
                chatForm.onsubmit = chatForm._originalSubmitHandler;
            }
        }
        if (sendBtn && sendBtn._originalText) {
            sendBtn.textContent = sendBtn._originalText;
        }
        if (input) {
            input.placeholder = 'Enter your question or comment...';
        }
        _setControlsEnabled(true);

        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.setSuppressed) {
            ChatInactivityNudge.setSuppressed('interviewer', false);
        }
    }

    function _handleFormSubmit(e) {
        e.preventDefault();
        if (_isActive) _submitAnswer();
    }

    // ── Message rendering ──────────────────────────────────────────────────
    function _addQuestionMessage(text, turnNum) {
        const el = document.createElement('div');
        el.className = 'interview-question';

        const label = document.createElement('span');
        label.className = 'interview-msg-label';
        label.textContent = `Question ${turnNum || _turnNumber}`;
        el.appendChild(label);

        const body = document.createElement('div');
        body.className = 'interview-msg-body';
        if (typeof marked !== 'undefined') {
            body.innerHTML = marked.parse(text.replace(/</g, '&lt;').replace(/>/g, '&gt;'));
        } else {
            body.textContent = text;
        }
        el.appendChild(body);

        _appendToChat(el);
    }

    function _addAnswerMessage(text) {
        const el = document.createElement('div');
        el.className = 'interview-answer';

        const label = document.createElement('span');
        label.className = 'interview-msg-label';
        label.textContent = 'Your Answer';
        el.appendChild(label);

        const body = document.createElement('div');
        body.className = 'interview-msg-body';
        body.textContent = text;
        el.appendChild(body);

        _appendToChat(el);
    }

    function _showLoading() {
        const el = document.createElement('div');
        el.className = 'interview-loading';
        el.id = 'interview-loading-el';
        el.innerHTML = '<i class="fas fa-microphone-alt" style="color:#4a90e2;"></i> Preparing next question <span class="interview-dots"><span></span><span></span><span></span></span>';
        _appendToChat(el);
    }

    function _hideLoading() {
        const el = document.getElementById('interview-loading-el');
        if (el) el.remove();
    }

    function _displayError(msg) {
        const el = document.createElement('div');
        el.className = 'interview-loading';
        el.style.borderColor = '#e57373';
        el.style.color = '#c62828';
        el.innerHTML = `<i class="fas fa-exclamation-triangle"></i> ${msg}`;
        _appendToChat(el);
        setTimeout(() => el.remove(), 8000);
    }

    function _appendToChat(el) {
        if (DOM && DOM.chatBox) {
            DOM.chatBox.appendChild(el);
            if (typeof UI !== 'undefined' && UI.scrollToBottom) UI.scrollToBottom();
        }
    }

    function _clearChatForInterview() {
        if (DOM && DOM.chatBox) {
            DOM.chatBox.innerHTML = '';
        }
    }

    function _enableAnswerInput() {
        const input = DOM.userInput || document.getElementById('user-input');
        if (input) {
            input.focus();
            input.placeholder = 'Type your answer...';
        }
    }

    function _setControlsEnabled(enabled) {
        const sendBtn = DOM.sendButton || document.getElementById('send-button');
        const input = DOM.userInput || document.getElementById('user-input');
        const micBtn = DOM.chatVoiceInputBtn || document.getElementById('chat-voice-input-btn');
        if (sendBtn) sendBtn.disabled = !enabled;
        if (input) input.disabled = !enabled;
        if (micBtn) micBtn.disabled = !enabled;
        if (!enabled && typeof ChatVoiceInput !== 'undefined' && ChatVoiceInput.stop) {
            ChatVoiceInput.stop();
        }
    }

    function _updateControlBar() {
        if (_statusText) _statusText.textContent = 'Interview in progress';
        if (_turnBadge) _turnBadge.textContent = `Q${_turnNumber}`;
    }

    function isActive() {
        return _isActive;
    }

    return {
        init,
        openSetup,
        isActive,
    };
})();
