'use strict';

/**
 * Web Speech API dictation for the main chat textarea (#user-input).
 * Toggle with the floating mic button; works for normal chat and Interviewer answers.
 */
const ChatVoiceInput = (() => {
    let _btn;
    let _recognition = null;

    function _supported() {
        return !!(window.SpeechRecognition || window.webkitSpeechRecognition);
    }

    function _setStatus(msg) {
        if (DOM.chatDictationStatus) DOM.chatDictationStatus.textContent = msg || '';
    }

    function _appendAtCursor(text) {
        const el = DOM.userInput;
        if (!el || !text) return;
        const t = text.trim();
        if (!t) return;
        const start = typeof el.selectionStart === 'number' ? el.selectionStart : el.value.length;
        const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : el.value.length;
        const before = el.value.slice(0, start);
        const after = el.value.slice(end);
        const needsSpace = before.length > 0 && !/\s$/.test(before);
        const insert = (needsSpace ? ' ' : '') + t + ' ';
        el.value = before + insert + after;
        const pos = (before + insert).length;
        try {
            el.selectionStart = el.selectionEnd = pos;
        } catch (e) { /* ignore */ }
        el.dispatchEvent(new Event('input', { bubbles: true }));
    }

    function stop() {
        AppState.isChatDictationListening = false;
        if (_recognition) {
            try {
                _recognition.stop();
            } catch (e) { /* ignore */ }
            _recognition = null;
        }
        AppState.chatDictationRecognition = null;
        if (_btn) {
            _btn.classList.remove('chat-voice-input-btn--listening');
            _btn.setAttribute('aria-pressed', 'false');
        }
        _setStatus('');
    }

    function _start() {
        const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
        if (!SR || !DOM.userInput || DOM.userInput.disabled) return;

        _recognition = new SR();
        _recognition.continuous = true;
        _recognition.interimResults = true;
        _recognition.lang = navigator.language || 'en-US';

        _recognition.onresult = (event) => {
            let interim = '';
            for (let i = event.resultIndex; i < event.results.length; i++) {
                const piece = event.results[i][0].transcript;
                if (event.results[i].isFinal) {
                    _appendAtCursor(piece);
                } else {
                    interim += piece;
                }
            }
            if (AppState.isChatDictationListening) {
                _setStatus(interim ? `\u2026 ${interim}` : 'Listening\u2026 click the mic again when finished.');
            }
        };

        _recognition.onerror = (e) => {
            if (e.error === 'aborted') return;
            if (e.error === 'no-speech') {
                if (AppState.isChatDictationListening) {
                    _setStatus('Listening\u2026 click the mic again when finished.');
                }
                return;
            }
            if (e.error === 'not-allowed') {
                _setStatus('Microphone access denied. Check browser permissions.');
            } else {
                _setStatus(`Voice input: ${e.error}`);
            }
            stop();
        };

        _recognition.onend = () => {
            if (AppState.isChatDictationListening && _recognition) {
                try {
                    _recognition.start();
                } catch (err) {
                    stop();
                }
            }
        };

        try {
            _recognition.start();
            AppState.isChatDictationListening = true;
            AppState.chatDictationRecognition = _recognition;
            if (_btn) {
                _btn.classList.add('chat-voice-input-btn--listening');
                _btn.setAttribute('aria-pressed', 'true');
            }
            _setStatus('Listening\u2026 click the mic again when finished.');
        } catch (err) {
            console.warn('chat voice input start failed', err);
            _setStatus('Could not start voice input.');
            stop();
        }
    }

    function toggle() {
        if (!_supported() || !_btn || _btn.disabled) return;
        if (AppState.isChatDictationListening) {
            stop();
        } else {
            _start();
        }
    }

    function init() {
        _btn = document.getElementById('chat-voice-input-btn');
        if (typeof DOM !== 'undefined') {
            DOM.chatVoiceInputBtn = _btn;
        }
        if (!_btn) return;
        if (!_supported()) {
            _btn.classList.add('chat-voice-input-btn--unsupported');
            _btn.disabled = true;
            const hint = !window.isSecureContext
                ? 'Voice input needs a secure context. Use https:// or open as http://localhost / 127.0.0.1 (not a LAN IP like 192.168.x.x).'
                : 'Voice input is not available in this browser. Try Chrome or Edge.';
            _btn.title = hint;
            _btn.setAttribute('aria-label', hint);
            return;
        }
        _btn.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            toggle();
        });
    }

    return { init, stop, toggle, isListening: () => AppState.isChatDictationListening };
})();
