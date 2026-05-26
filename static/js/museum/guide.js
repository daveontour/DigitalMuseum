'use strict';

const Guide = {
    _currentTopic: null,
    _currentStep: 0,
    _lastFocusedElement: null,
    _focusTrapHandler: null,
    _escHandler: null,
    _topics: null, // cached from API

    // Named navigation actions referenced by guide steps stored in the database.
    NavActions: {
        showGettingStartedDialog: () => Guide._showGettingStartedDialog(),
        openDataImport: () => document.getElementById('data-import-sidebar-btn')?.click(),
        openSettingsManageKeys: () => {
            document.getElementById('settings-data-import-sidebar-btn')?.click();
            setTimeout(() => {
                document.querySelector('.config-tab-button[data-tab="manage-keys"]')?.click();
            }, 120);
        },
    },

    topicAliases: {
        'Settings and data import': 'SettingsAndDataImport',
        Settings: 'SettingsAndDataImport',
        'Data import': 'SettingsAndDataImport',
        'Browsing images': 'BrowsingImages',
        'Managing contacts': 'ManagingContacts',
        'Voice and AI settings': 'VoiceAndAISettings',
        "Today's Thing of Interest": 'TodaysThingOfInterest',
        'Email catchup': 'EmailCatchup',
    },

    _positions: {
        'top-left':      { top: '5%',   bottom: 'auto', left: '5%',   right: 'auto', transform: 'none' },
        'top-center':    { top: '5%',   bottom: 'auto', left: '50%',  right: 'auto', transform: 'translateX(-50%)' },
        'top-right':     { top: '5%',   bottom: 'auto', left: 'auto', right: '5%',   transform: 'none' },
        'middle-left':   { top: '50%',  bottom: 'auto', left: '5%',   right: 'auto', transform: 'translateY(-50%)' },
        'middle-center': { top: '50%',  bottom: 'auto', left: '50%',  right: 'auto', transform: 'translate(-50%, -50%)' },
        'middle-right':  { top: '50%',  bottom: 'auto', left: 'auto', right: '5%',   transform: 'translateY(-50%)' },
        'bottom-left':   { top: 'auto', bottom: '5%',   left: '5%',   right: 'auto', transform: 'none' },
        'bottom-center': { top: 'auto', bottom: '5%',   left: '50%',  right: 'auto', transform: 'translateX(-50%)' },
        'bottom-right':  { top: 'auto', bottom: '5%',   left: 'auto', right: '5%',   transform: 'none' },
    },

    invalidateCache() {
        this._topics = null;
    },

    async fetchTopics() {
        if (this._topics !== null) return this._topics;
        try {
            const resp = await fetch('/api/guide-topics');
            if (!resp.ok) throw new Error('HTTP ' + resp.status);
            const data = await resp.json();
            this._topics = (data && data.topics) ? data.topics : {};
        } catch (err) {
            console.error('Guide: failed to load topics', err);
            this._topics = {};
        }
        return this._topics;
    },

    _positionDialog(dialog, position) {
        const pos = this._positions[position] || this._positions['middle-center'];
        Object.assign(dialog.style, pos);
    },
    _getTopicConfig(topicKey) {
        const topics = this._topics || {};
        const key = this.topicAliases[topicKey] || topicKey;
        return { key, config: topics[key] };
    },

    _clearGlows() {
        document.querySelectorAll('.guide-glow').forEach(el => el.classList.remove('guide-glow'));
    },

    _applyGlow(selector) {
        if (!selector) return true;
        const el = document.querySelector(selector);
        if (el) {
            el.classList.add('guide-glow');
            return true;
        }
        return false;
    },
    _bindEscape(handler) {
        this._unbindEscape();
        this._escHandler = (evt) => {
            if (evt.key === 'Escape') {
                evt.preventDefault();
                handler();
            }
        };
        document.addEventListener('keydown', this._escHandler);
    },
    _unbindEscape() {
        if (this._escHandler) {
            document.removeEventListener('keydown', this._escHandler);
            this._escHandler = null;
        }
    },
    _focusTrap(container) {
        if (!container) return;
        if (this._focusTrapHandler) {
            container.removeEventListener('keydown', this._focusTrapHandler);
        }
        this._focusTrapHandler = (evt) => {
            if (evt.key !== 'Tab') return;
            const focusables = container.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
            if (!focusables.length) return;
            const first = focusables[0];
            const last = focusables[focusables.length - 1];
            if (evt.shiftKey && document.activeElement === first) {
                evt.preventDefault();
                last.focus();
            } else if (!evt.shiftKey && document.activeElement === last) {
                evt.preventDefault();
                first.focus();
            }
        };
        container.addEventListener('keydown', this._focusTrapHandler);
    },
    _restoreFocus() {
        if (this._lastFocusedElement && typeof this._lastFocusedElement.focus === 'function') {
            this._lastFocusedElement.focus();
        }
        this._lastFocusedElement = null;
    },

    _showGettingStartedDialog() {
        const overlay  = document.getElementById('getting-started-overlay');
        const dialog   = document.getElementById('getting-started-dialog');
        const closeBtn = document.getElementById('getting-started-close-btn');
        if (!overlay || !dialog) return;

        const close = () => {
            overlay.style.display = 'none';
            dialog.style.display = 'none';
            overlay.onclick = null;
            if (closeBtn) closeBtn.onclick = null;
            this._closeExplanation();
        };

        overlay.style.display = 'block';
        dialog.style.display = 'flex';
        this._focusTrap(dialog);
        this._bindEscape(close);
        overlay.onclick = close;
        if (closeBtn) closeBtn.onclick = close;
    },

    _closeExplanation() {
        const gsOverlay  = document.getElementById('getting-started-overlay');
        const gsDialog   = document.getElementById('getting-started-dialog');
        if (gsOverlay)  { gsOverlay.style.display = 'none'; gsOverlay.onclick = null; }
        if (gsDialog)   { gsDialog.style.display = 'none'; }

        const overlay  = document.getElementById('guide-explanation-overlay');
        const dialog   = document.getElementById('guide-explanation-dialog');
        const backBtn  = document.getElementById('guide-explanation-back-btn');
        const nextBtn  = document.getElementById('guide-explanation-next-btn');
        const doneBtn  = document.getElementById('guide-explanation-done-btn');
        const closeBtn = document.getElementById('guide-explanation-close-btn');
        const imgEl   = document.getElementById('guide-explanation-image');
        const progressEl = document.getElementById('guide-explanation-progress');
        if (overlay)  { overlay.style.display  = 'none'; overlay.onclick  = null; }
        if (dialog)   { dialog.style.display   = 'none'; dialog.classList.remove('guide-explanation-dialog-has-close'); }
        if (backBtn)  { backBtn.style.display  = 'none'; backBtn.onclick  = null; }
        if (nextBtn)  { nextBtn.style.display  = 'none'; nextBtn.onclick  = null; }
        if (doneBtn)  { doneBtn.style.display  = 'none'; doneBtn.onclick  = null; }
        if (closeBtn) { closeBtn.style.display = 'none'; closeBtn.onclick = null; }
        if (imgEl)    { imgEl.style.display    = 'none'; imgEl.src        = ''; }
        if (progressEl) progressEl.textContent = '';
        this._clearGlows();
        this._currentTopic = null;
        this._currentStep  = 0;
        this._unbindEscape();
        this._restoreFocus();
    },

    _showStep(stepIndex) {
        const topics = this._topics || {};
        const topicConfig = topics[this._currentTopic];
        const steps = topicConfig && topicConfig.steps;
        if (!steps || stepIndex >= steps.length) { this._closeExplanation(); return; }

        const step     = steps[stepIndex];
        const isLast   = stepIndex === steps.length - 1;
        const overlay  = document.getElementById('guide-explanation-overlay');
        const dialog   = document.getElementById('guide-explanation-dialog');
        const progressEl = document.getElementById('guide-explanation-progress');
        const textEl   = document.getElementById('guide-explanation-text');
        const imgEl    = document.getElementById('guide-explanation-image');
        const backBtn  = document.getElementById('guide-explanation-back-btn');
        const nextBtn  = document.getElementById('guide-explanation-next-btn');
        const doneBtn  = document.getElementById('guide-explanation-done-btn');
        const closeBtn = document.getElementById('guide-explanation-close-btn');
        if (!overlay || !dialog || !textEl) return;

        this._currentStep = stepIndex;

        // Support both camelCase (JS) and snake_case (DB-stored JSON) step fields.
        const glowSel     = step.glow || '';
        const fallbackTxt = step.fallback_text || step.fallbackText || '';
        const navAction   = step.navigate_action || step.navigateAction || '';
        const position    = step.position || 'middle-center';

        const applyGlowAndShow = () => {
            this._clearGlows();
            const glowOk = this._applyGlow(glowSel);

            textEl.textContent = step.text || '';
            if (glowSel && !glowOk && fallbackTxt) {
                textEl.textContent += '\n\n' + fallbackTxt;
            } else if (glowSel && !glowOk) {
                textEl.textContent += '\n\nOpen the related section first, then continue this guide step.';
            }
            if (progressEl) progressEl.textContent = topicConfig.title + ' — Step ' + (stepIndex + 1) + ' of ' + steps.length;

            if (imgEl) {
                if (step.image) {
                    imgEl.src = step.image;
                    imgEl.style.display = 'block';
                } else {
                    imgEl.src = '';
                    imgEl.style.display = 'none';
                }
            }

            if (backBtn) {
                backBtn.style.display = stepIndex > 0 ? 'inline-flex' : 'none';
                backBtn.onclick = stepIndex > 0 ? () => this._showStep(stepIndex - 1) : null;
            }
            nextBtn.style.display  = isLast ? 'none' : 'inline-flex';
            nextBtn.onclick        = isLast ? null     : () => this._showStep(stepIndex + 1);
            if (doneBtn) {
                doneBtn.style.display = isLast ? 'inline-flex' : 'none';
                doneBtn.onclick = isLast ? () => this._closeExplanation() : null;
            }
            closeBtn.style.display = 'block';
            closeBtn.onclick       = () => this._closeExplanation();
            dialog.classList.toggle('guide-explanation-dialog-has-close', true);

            this._positionDialog(dialog, position);
            overlay.style.display = 'block';
            dialog.style.display  = 'block';
            this._focusTrap(dialog);
            this._bindEscape(() => this._closeExplanation());
            overlay.onclick = () => this._closeExplanation();
        };

        if (navAction) {
            const fn = this.NavActions[navAction];
            overlay.style.display = 'none';
            dialog.style.display  = 'none';
            if (fn) fn();
            if (step.text) {
                setTimeout(applyGlowAndShow, 120);
            }
        } else {
            if (step.text) {
                applyGlowAndShow();
            }
        }
    },

    async onTopicSelected(topic) {
        const guideModal = document.getElementById('guide-modal');
        if (guideModal) guideModal.style.display = 'none';
        document.querySelectorAll('.guide-topic-btn').forEach(b => b.classList.remove('guide-topic-glow'));
        this._lastFocusedElement = document.getElementById('guide-sidebar-btn') || document.activeElement;
        await this.fetchTopics();
        const resolved = this._getTopicConfig(topic);
        if (!resolved.config) {
            this._showUnknownTopic(resolved.key || topic);
            return;
        }
        this._currentTopic = resolved.key;
        this._showStep(0);
    },
    _showUnknownTopic(topic) {
        const overlay = document.getElementById('guide-explanation-overlay');
        const dialog = document.getElementById('guide-explanation-dialog');
        const progressEl = document.getElementById('guide-explanation-progress');
        const textEl = document.getElementById('guide-explanation-text');
        const doneBtn = document.getElementById('guide-explanation-done-btn');
        const closeBtn = document.getElementById('guide-explanation-close-btn');
        const nextBtn = document.getElementById('guide-explanation-next-btn');
        const backBtn = document.getElementById('guide-explanation-back-btn');
        if (!overlay || !dialog || !textEl) return;
        if (progressEl) progressEl.textContent = 'Guide topic unavailable';
        textEl.textContent = 'This help topic is unavailable right now. Please reopen Guide and choose another topic.';
        if (nextBtn) nextBtn.style.display = 'none';
        if (backBtn) backBtn.style.display = 'none';
        if (doneBtn) { doneBtn.style.display = 'inline-flex'; doneBtn.onclick = () => this._closeExplanation(); }
        if (closeBtn) { closeBtn.style.display = 'block'; closeBtn.onclick = () => this._closeExplanation(); }
        overlay.style.display = 'block';
        dialog.style.display = 'block';
        this._positionDialog(dialog, 'middle-center');
        this._focusTrap(dialog);
        this._bindEscape(() => this._closeExplanation());
    },
    _categoryOrder: ['Getting Started', 'Daily Use', 'Setup & Import', 'Troubleshooting'],
    renderTopicList() {
        const container = document.getElementById('guide-topics-list');
        if (!container) return;
        const topics = this._topics || {};
        const byCategory = {};
        this._categoryOrder.forEach((c) => { byCategory[c] = []; });
        Object.keys(topics).forEach((key) => {
            const t = topics[key];
            const category = byCategory[t.category] ? t.category : 'Daily Use';
            byCategory[category].push({
                key: key,
                title: t.title || key,
                description: t.description || '',
                recommended: !!t.recommended
            });
        });
        Object.keys(byCategory).forEach((categoryKey) => {
            const arr = byCategory[categoryKey];
            arr.sort((a, b) => {
                if (!!a.recommended !== !!b.recommended) return a.recommended ? -1 : 1;
                return String(a.title || '').localeCompare(String(b.title || ''));
            });
        });

        let html = '';
        this._categoryOrder.forEach((category) => {
            const items = byCategory[category];
            if (!items || items.length === 0) return;
            html += '<section class="guide-topic-group">' +
                '<h3 class="guide-topic-group-title">' + category + '</h3>' +
                '<ul class="guide-topic-group-list">';
            items.forEach((t) => {
                html += '<li class="guide-topic-item">' +
                    '<button type="button" class="guide-topic-btn" data-topic="' + t.key.replace(/"/g, '&quot;') + '">' +
                    '<span class="guide-topic-btn-title" style="display:block;">' + t.title + (t.recommended ? ' <span class="guide-topic-badge">Recommended first</span>' : '') + '</span>' +
                    '<span class="guide-topic-btn-desc" style="display:block;margin-top:4px;">' + t.description + '</span>' +
                    '</button></li>';
            });
            html += '</ul></section>';
        });
        container.innerHTML = html;
        const buttons = container.querySelectorAll('.guide-topic-btn');
        buttons.forEach((btn) => {
            btn.addEventListener('click', (evt) => {
                evt.preventDefault();
                evt.stopPropagation();
                const topic = btn.dataset.topic || '';
                this.onTopicSelected(topic);
            });
        });
    },
    async openGuideModal() {
        await this.fetchTopics();
        const guideModal = document.getElementById('guide-modal');
        if (!guideModal) return;
        this.renderTopicList();
        this._lastFocusedElement = document.activeElement;
        guideModal.style.display = 'flex';
        const dialog = guideModal.querySelector('.guide-modal-content');
        this._focusTrap(dialog);
        this._bindEscape(() => this.closeAll());
    },
    closeAll() {
        const guideModal = document.getElementById('guide-modal');
        if (guideModal) guideModal.style.display = 'none';
        this._closeExplanation();
    },
    async init() {
        try {
            await this.fetchTopics();
            this.renderTopicList();
        } catch (err) {
            console.error('Guide init failed:', err);
        }
    }
};

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => Guide.init());
} else {
    Guide.init();
}
