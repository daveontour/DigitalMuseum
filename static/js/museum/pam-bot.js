'use strict';

/**
 * PamBot — Memory Companion for archive owners with dementia.
 *
 * Accessibility design:
 *  - Full-screen overlay, warm background, 48px+ text
 *  - Three large buttons: Tell me more / Something else / Goodbye
 *  - Large text input + Send button for typed responses
 *  - Auto reads each message aloud via Web Speech API
 *
 * Self-initialises because app.js / Modals.initAll() has already run
 * by the time this deferred script executes.
 */
const PamBot = (() => {
    // ── State ────────────────────────────────────────────────────────────────
    let _speechEnabled = true;
    let _busy = false;
    let _subjectName = '';
    let _recognition = null;
    let _isListening = false;
    let _photoDragActive = false;
    let _photoDragStartX = 0;
    let _photoDragStartY = 0;
    let _photoOffsetX = 0;
    let _photoOffsetY = 0;

    // ── DOM helpers ──────────────────────────────────────────────────────────
    const _el = id => document.getElementById(id);

    // ── Speech ───────────────────────────────────────────────────────────────
    function _speak(text) {
        if (!_speechEnabled || !window.speechSynthesis) return;
        window.speechSynthesis.cancel();
        const utt = new SpeechSynthesisUtterance(text);
        utt.rate = 0.80;
        utt.pitch = 1.0;
        utt.volume = 1.0;
        window.speechSynthesis.speak(utt);
    }

    function _stopSpeech() {
        if (window.speechSynthesis) window.speechSynthesis.cancel();
    }

    // ── UI helpers ───────────────────────────────────────────────────────────
    function _setLoading() {
        const msg = _el('pam-bot-message');
        if (msg) {
            msg.className = 'pam-bot-loading-text';
            msg.textContent = 'One moment\u2026';
        }
        _setControlsDisabled(true);
    }

    function _showMessage(text) {
        const msg = _el('pam-bot-message');
        if (msg) {
            msg.className = '';
            msg.textContent = text;
        }
        _setControlsDisabled(false);
        _speak(text);
    }

    function _showError(text) {
        const msg = _el('pam-bot-message');
        if (msg) {
            msg.className = '';
            msg.textContent = text;
        }
        _setControlsDisabled(false);
    }

    function _showPhoto(url) {
        const dialog = _el('pam-bot-photo-dialog');
        const img    = _el('pam-bot-photo-img');
        if (!dialog || !img) return;
        _photoOffsetX = 0;
        _photoOffsetY = 0;
        dialog.style.transform = '';
        img.src = url;
        dialog.setAttribute('aria-hidden', 'false');
        dialog.classList.add('pam-bot-photo-visible');
    }

    function _hidePhoto() {
        const dialog = _el('pam-bot-photo-dialog');
        if (!dialog) return;
        dialog.classList.remove('pam-bot-photo-visible');
        dialog.setAttribute('aria-hidden', 'true');
        const img = _el('pam-bot-photo-img');
        if (img) img.src = '';
    }

    // Disable/enable all interactive controls while a request is in flight.
    function _setControlsDisabled(disabled) {
        ['pam-bot-more-btn', 'pam-bot-diff-btn', 'pam-bot-send-btn'].forEach(id => {
            const el = _el(id);
            if (el) el.disabled = disabled;
        });
        const input = _el('pam-bot-input');
        if (input) input.disabled = disabled;
    }

    // ── Fetch subject name from the API ──────────────────────────────────────
    async function _loadSubjectName() {
        try {
            const resp = await fetch('/api/subject-configuration');
            if (!resp.ok) return;
            const data = await resp.json();
            _subjectName = data.subject_name || '';
        } catch (_) {
            // Non-fatal; leave blank
        }
        const nameEl = _el('pam-bot-subject-name');
        if (nameEl) nameEl.textContent = _subjectName || 'you';
    }

    // ── API calls ────────────────────────────────────────────────────────────
    async function _sendAction(action, typedText) {
        if (_busy) return;
        _busy = true;
        _stopSpeech();
        _setLoading();

        try {
            const body = { action };
            if (typedText) body.typed_text = typedText;

            const resp = await fetch('/api/pambot/message', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                _showError(err.error || 'Something went wrong. Please try again.');
                return;
            }
            const data = await resp.json();
            _showMessage(data.message || 'I could not find a memory right now. Shall we try something else?');
            if (data.photo_url) {
                _showPhoto(data.photo_url);
            } else {
                _hidePhoto();
            }
        } catch (_) {
            _showError('There was a problem reaching the server. Please try again.');
        } finally {
            _busy = false;
        }
    }

    function _sendTyped() {
        const input = _el('pam-bot-input');
        if (!input) return;
        const text = input.value.trim();
        if (!text) return;
        _stopDictation();
        input.value = '';
        _el('pam-bot-send-btn') && (_el('pam-bot-send-btn').disabled = true);
        _sendAction('typed', text);
    }

    // ── Voice dictation ───────────────────────────────────────────────────────
    function _dictationSupported() {
        return !!(window.SpeechRecognition || window.webkitSpeechRecognition);
    }

    function _setDictationStatus(msg) {
        const el = _el('pam-bot-dictation-status');
        if (el) el.textContent = msg || '';
    }

    function _appendToInput(text) {
        const el = _el('pam-bot-input');
        if (!el || !text) return;
        const t = text.trim();
        if (!t) return;
        const start = typeof el.selectionStart === 'number' ? el.selectionStart : el.value.length;
        const end   = typeof el.selectionEnd   === 'number' ? el.selectionEnd   : el.value.length;
        const before = el.value.slice(0, start);
        const after  = el.value.slice(end);
        const needsSpace = before.length > 0 && !/\s$/.test(before);
        const insert = (needsSpace ? ' ' : '') + t + ' ';
        el.value = before + insert + after;
        const pos = (before + insert).length;
        try { el.selectionStart = el.selectionEnd = pos; } catch (_) { /* ignore */ }
        el.dispatchEvent(new Event('input', { bubbles: true }));
    }

    function _stopDictation() {
        _isListening = false;
        if (_recognition) {
            try { _recognition.stop(); } catch (_) { /* ignore */ }
            _recognition = null;
        }
        const btn = _el('pam-bot-mic-btn');
        if (btn) {
            btn.classList.remove('pam-bot-mic-btn--listening');
            btn.setAttribute('aria-pressed', 'false');
        }
        _setDictationStatus('');
    }

    function _startDictation() {
        const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
        const input = _el('pam-bot-input');
        if (!SR || !input || input.disabled) return;

        _recognition = new SR();
        _recognition.continuous     = true;
        _recognition.interimResults = true;
        _recognition.lang           = navigator.language || 'en-US';

        _recognition.onresult = (event) => {
            let interim = '';
            for (let i = event.resultIndex; i < event.results.length; i++) {
                const piece = event.results[i][0].transcript;
                if (event.results[i].isFinal) {
                    _appendToInput(piece);
                } else {
                    interim += piece;
                }
            }
            if (_isListening) {
                _setDictationStatus(interim ? `\u2026 ${interim}` : 'Listening\u2026 tap the mic again when finished.');
            }
        };

        _recognition.onerror = (e) => {
            if (e.error === 'aborted') return;
            if (e.error === 'no-speech') {
                if (_isListening) _setDictationStatus('Listening\u2026 tap the mic again when finished.');
                return;
            }
            _setDictationStatus(e.error === 'not-allowed'
                ? 'Microphone access denied. Check browser permissions.'
                : `Voice input error: ${e.error}`);
            _stopDictation();
        };

        _recognition.onend = () => {
            if (_isListening && _recognition) {
                try { _recognition.start(); } catch (_) { _stopDictation(); }
            }
        };

        try {
            _recognition.start();
            _isListening = true;
            const btn = _el('pam-bot-mic-btn');
            if (btn) {
                btn.classList.add('pam-bot-mic-btn--listening');
                btn.setAttribute('aria-pressed', 'true');
            }
            _setDictationStatus('Listening\u2026 tap the mic again when finished.');
        } catch (_) {
            _setDictationStatus('Could not start voice input.');
            _stopDictation();
        }
    }

    function _toggleDictation() {
        if (_isListening) { _stopDictation(); } else { _startDictation(); }
    }

    // ── Open / close ─────────────────────────────────────────────────────────
    function open() {
        const overlay = _el('pam-bot-overlay');
        if (!overlay) return;
        overlay.classList.add('pam-bot-visible');
        document.body.style.overflow = 'hidden';

        if (_subjectName) {
            const nameEl = _el('pam-bot-subject-name');
            if (nameEl) nameEl.textContent = _subjectName;
        }

        // Clear any previous input
        const input = _el('pam-bot-input');
        if (input) input.value = '';

        _sendAction('start');
    }

    function close() {
        _stopSpeech();
        _stopDictation();
        _hidePhoto();
        const overlay = _el('pam-bot-overlay');
        if (overlay) overlay.classList.remove('pam-bot-visible');
        document.body.style.overflow = '';
        _busy = false;
    }

    // ── Speech toggle ────────────────────────────────────────────────────────
    function _toggleSpeech() {
        _speechEnabled = !_speechEnabled;
        const btn = _el('pam-bot-speech-btn');
        if (btn) {
            if (_speechEnabled) {
                btn.classList.remove('pam-bot-speech-off');
                btn.title = 'Turn off voice';
                btn.innerHTML = '<i class="fas fa-volume-up"></i>';
            } else {
                btn.classList.add('pam-bot-speech-off');
                btn.title = 'Turn on voice';
                btn.innerHTML = '<i class="fas fa-volume-mute"></i>';
                _stopSpeech();
            }
        }
    }

    // ── Init ─────────────────────────────────────────────────────────────────
    function init() {
        // Sidebar trigger
        const triggerBtn = _el('pam-bot-trigger-btn');
        if (triggerBtn) triggerBtn.addEventListener('click', open);

        // Action buttons
        const moreBtn   = _el('pam-bot-more-btn');
        const diffBtn   = _el('pam-bot-diff-btn');
        const byeBtn    = _el('pam-bot-bye-btn');
        const speechBtn = _el('pam-bot-speech-btn');
        const sendBtn   = _el('pam-bot-send-btn');
        const input     = _el('pam-bot-input');

        if (moreBtn)   moreBtn.addEventListener('click',   () => _sendAction('more'));
        if (diffBtn)   diffBtn.addEventListener('click',   () => _sendAction('different'));
        if (byeBtn)    byeBtn.addEventListener('click',    close);
        if (speechBtn) speechBtn.addEventListener('click', _toggleSpeech);
        if (sendBtn)   sendBtn.addEventListener('click',   _sendTyped);

        // Mic button
        const micBtn = _el('pam-bot-mic-btn');
        if (micBtn) {
            if (!_dictationSupported()) {
                micBtn.disabled = true;
                micBtn.title = window.isSecureContext
                    ? 'Voice input is not available in this browser. Try Chrome or Edge.'
                    : 'Voice input needs a secure context (https:// or localhost).';
            } else {
                micBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    _toggleDictation();
                });
            }
        }

        // Enter (without Shift) submits; Shift+Enter adds a newline
        if (input) {
            input.addEventListener('keydown', e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    _sendTyped();
                }
            });
            // Enable/disable Send button based on whether there is content
            input.addEventListener('input', () => {
                if (sendBtn) sendBtn.disabled = _busy || !input.value.trim();
            });
        }

        // Escape key closes
        document.addEventListener('keydown', e => {
            if (e.key === 'Escape') {
                const overlay = _el('pam-bot-overlay');
                if (overlay && overlay.classList.contains('pam-bot-visible')) close();
            }
        });

        // Photo dialog drag + close
        const photoClose    = _el('pam-bot-photo-close');
        const photoTitlebar = _el('pam-bot-photo-titlebar');
        if (photoClose) photoClose.addEventListener('click', _hidePhoto);
        if (photoTitlebar) {
            photoTitlebar.addEventListener('mousedown', e => {
                _photoDragActive = true;
                _photoDragStartX = e.clientX - _photoOffsetX;
                _photoDragStartY = e.clientY - _photoOffsetY;
                e.preventDefault();
            });
        }
        document.addEventListener('mousemove', e => {
            if (!_photoDragActive) return;
            _photoOffsetX = e.clientX - _photoDragStartX;
            _photoOffsetY = e.clientY - _photoDragStartY;
            const d = _el('pam-bot-photo-dialog');
            if (d) d.style.transform = `translate(${_photoOffsetX}px, ${_photoOffsetY}px)`;
        });
        document.addEventListener('mouseup', () => { _photoDragActive = false; });

        _loadSubjectName();
    }

    return { init, open, close };
})();

// Self-initialise (app.js has already run by this point)
PamBot.init();
