'use strict';

/**
 * Promise-based app dialogs (replaces alert / confirm / prompt).
 * Single #confirmation-modal queue; init() is idempotent.
 */
const AppDialogs = (function () {
    let inited = false;
    let isOpen = false;
    /** @type {Array<{ mode: string, resolve: function, opts: object }>} */
    const queue = [];

    let modal;
    let titleEl;
    let textEl;
    let promptWrap;
    let promptLabel;
    let promptInput;
    let confirmBtn;
    let cancelBtn;
    let closeBtn;

    let currentMode = 'alert';
    let keyHandlerBound = false;

    function bindEls() {
        modal = document.getElementById('confirmation-modal');
        titleEl = document.getElementById('confirmation-modal-title');
        textEl = document.getElementById('confirmation-modal-text');
        promptWrap = document.getElementById('confirmation-modal-prompt-wrap');
        promptLabel = document.getElementById('confirmation-modal-prompt-label');
        promptInput = document.getElementById('confirmation-modal-prompt-input');
        confirmBtn = document.getElementById('confirmation-modal-confirm');
        cancelBtn = document.getElementById('confirmation-modal-cancel');
        closeBtn = document.getElementById('close-confirmation-modal');
    }

    function cancelResult(mode) {
        if (mode === 'confirm') return false;
        if (mode === 'prompt') return null;
        return undefined;
    }

    function hideUi() {
        if (modal) modal.style.display = 'none';
        if (promptWrap) promptWrap.style.display = 'none';
        if (promptInput) promptInput.value = '';
        if (confirmBtn) {
            confirmBtn.classList.remove('modal-btn-danger');
            confirmBtn.classList.add('modal-btn-primary');
        }
    }

    function finish(value) {
        if (!isOpen || queue.length === 0) return;
        hideUi();
        isOpen = false;
        const job = queue.shift();
        job.resolve(value);
        pump();
    }

    function onConfirmClick() {
        if (currentMode === 'prompt') {
            const v = (promptInput && promptInput.value) || '';
            finish(v);
            return;
        }
        if (currentMode === 'confirm') {
            finish(true);
            return;
        }
        finish(undefined);
    }

    function onCancelClick() {
        finish(cancelResult(currentMode));
    }

    function onKeyDown(e) {
        if (!isOpen || !modal || modal.style.display !== 'flex') return;
        if (e.key === 'Escape') {
            e.preventDefault();
            onCancelClick();
            return;
        }
        if (e.key === 'Enter' && e.target === promptInput) {
            e.preventDefault();
            onConfirmClick();
        }
    }

    function applyJob(job) {
        const { mode, opts } = job;
        currentMode = mode;
        const title = opts.title != null ? opts.title : mode === 'alert' ? 'Notice' : 'Confirm';
        const text = opts.text != null ? String(opts.text) : '';

        titleEl.textContent = title;
        textEl.textContent = text;

        if (mode === 'prompt') {
            promptWrap.style.display = 'block';
            promptLabel.textContent = opts.promptLabel || '';
            promptInput.value = opts.defaultValue != null ? String(opts.defaultValue) : '';
            cancelBtn.style.display = '';
            closeBtn.style.display = '';
            confirmBtn.textContent = opts.confirmLabel || 'OK';
            cancelBtn.textContent = opts.cancelLabel || 'Cancel';
            confirmBtn.classList.toggle('modal-btn-danger', !!opts.danger);
            confirmBtn.classList.toggle('modal-btn-primary', !opts.danger);
        } else if (mode === 'confirm') {
            promptWrap.style.display = 'none';
            cancelBtn.style.display = '';
            closeBtn.style.display = '';
            confirmBtn.textContent = opts.confirmLabel || 'Confirm';
            cancelBtn.textContent = opts.cancelLabel || 'Cancel';
            if (opts.danger) {
                confirmBtn.classList.add('modal-btn-danger');
                confirmBtn.classList.remove('modal-btn-primary');
            } else {
                confirmBtn.classList.remove('modal-btn-danger');
                confirmBtn.classList.add('modal-btn-primary');
            }
        } else {
            /* alert */
            promptWrap.style.display = 'none';
            cancelBtn.style.display = 'none';
            closeBtn.style.display = '';
            confirmBtn.textContent = opts.confirmLabel || 'OK';
            confirmBtn.classList.remove('modal-btn-danger');
            confirmBtn.classList.add('modal-btn-primary');
        }

        modal.style.display = 'flex';
        isOpen = true;

        if (mode === 'prompt') {
            setTimeout(() => {
                try {
                    promptInput.focus();
                    promptInput.select();
                } catch (err) { /* ignore */ }
            }, 0);
        } else {
            setTimeout(() => {
                try {
                    confirmBtn.focus();
                } catch (err) { /* ignore */ }
            }, 0);
        }
    }

    function pump() {
        if (isOpen || queue.length === 0) return;
        if (!modal) bindEls();
        if (!modal || !confirmBtn) return;
        applyJob(queue[0]);
    }

    function enqueue(mode, opts) {
        return new Promise((resolve) => {
            queue.push({ mode, opts, resolve });
            pump();
        });
    }

    function init() {
        if (inited) return;
        bindEls();
        if (!modal || !confirmBtn || !cancelBtn) {
            console.warn('AppDialogs: #confirmation-modal elements missing');
            return;
        }

        confirmBtn.addEventListener('click', onConfirmClick);
        cancelBtn.addEventListener('click', onCancelClick);
        closeBtn.addEventListener('click', onCancelClick);
        modal.addEventListener('click', (e) => {
            if (e.target === modal) onCancelClick();
        });

        if (!keyHandlerBound) {
            document.addEventListener('keydown', onKeyDown, true);
            keyHandlerBound = true;
        }

        inited = true;
    }

    /**
     * @param {string} titleOrMessage
     * @param {string} [message]
     */
    function showAppAlert(titleOrMessage, message) {
        init();
        let title;
        let text;
        if (message === undefined) {
            title = 'Notice';
            text = titleOrMessage != null ? String(titleOrMessage) : '';
        } else {
            title = titleOrMessage != null ? String(titleOrMessage) : 'Notice';
            text = message != null ? String(message) : '';
        }
        return enqueue('alert', { title, text });
    }

    /**
     * @param {string} titleOrMessage
     * @param {string} [message]
     * @param {{ confirmLabel?: string, cancelLabel?: string, danger?: boolean }} [options]
     */
    function showAppConfirm(titleOrMessage, message, options) {
        init();
        const opts = typeof options === 'object' && options !== null ? options : {};
        let title;
        let text;
        if (arguments.length === 1) {
            title = 'Confirm';
            text = titleOrMessage != null ? String(titleOrMessage) : '';
        } else {
            title = titleOrMessage != null ? String(titleOrMessage) : 'Confirm';
            text = message != null ? String(message) : '';
        }
        return enqueue('confirm', {
            title,
            text,
            confirmLabel: opts.confirmLabel,
            cancelLabel: opts.cancelLabel,
            danger: opts.danger,
        });
    }

    /**
     * @param {string} title
     * @param {string} message
     * @param {string} [defaultValue]
     * @param {{ promptLabel?: string, confirmLabel?: string, cancelLabel?: string }} [options]
     */
    function showAppPrompt(title, message, defaultValue, options) {
        init();
        const opts = options || {};
        return enqueue('prompt', {
            title: title != null ? String(title) : 'Input',
            text: message != null ? String(message) : '',
            defaultValue: defaultValue != null ? defaultValue : '',
            promptLabel: opts.promptLabel || 'Value',
            confirmLabel: opts.confirmLabel || 'OK',
            cancelLabel: opts.cancelLabel || 'Cancel',
            danger: !!opts.danger,
        });
    }

    /** Close visible dialog and cancel all queued; used by Modals.closeAll */
    function close() {
        hideUi();
        isOpen = false;
        while (queue.length) {
            const job = queue.shift();
            try {
                job.resolve(cancelResult(job.mode));
            } catch (e) { /* ignore */ }
        }
    }

    /**
     * Legacy: Modals.ConfirmationModal.open(title, text, onConfirmFn)
     * @param {function|undefined} onConfirmFn
     */
    function openLegacy(title, text, onConfirmFn) {
        init();
        if (typeof onConfirmFn === 'function') {
            showAppConfirm(title, text).then((ok) => {
                if (ok) onConfirmFn();
            });
        } else {
            void showAppAlert(title, text != null ? text : '');
        }
    }

    return {
        init,
        showAppAlert,
        showAppConfirm,
        showAppPrompt,
        close,
        openLegacy,
    };
})();

window.AppDialogs = AppDialogs;
