'use strict';

const Guide = {
    _currentTopic: null,
    _currentStep: 0,
    _lastFocusedElement: null,
    _focusTrapHandler: null,
    _escHandler: null,
    _topics: null, // cached from API

    _clickSidebarButton(buttonId) {
        document.getElementById(buttonId)?.click();
    },

    _openConfigurationTab(tabName) {
        Guide._clickSidebarButton('settings-data-import-sidebar-btn');
        setTimeout(() => {
            document.querySelector(`.config-tab-button[data-tab="${tabName}"]`)?.click();
        }, 120);
    },

    _openDataImportTab(tabName) {
        Guide._clickSidebarButton('data-import-sidebar-btn');
        setTimeout(() => {
            document.querySelector(`.data-import-category-tab[data-import-category-tab="${tabName}"]`)?.click();
        }, 120);
    },

    _runNavActions(navActionRaw) {
        if (!navActionRaw || typeof navActionRaw !== 'string') return Promise.resolve();
        const parts = navActionRaw.split(';').map((part) => part.trim()).filter(Boolean);
        return parts.reduce((chain, name) => chain.then(async () => {
            const fn = this.NavActions[name];
            if (!fn) {
                console.warn('Guide: unknown navigate_action:', name);
                return;
            }
            const result = fn();
            if (result && typeof result.then === 'function') {
                await result;
            }
        }), Promise.resolve());
    },

    _pauseNavActions(ms) {
        return new Promise((resolve) => setTimeout(resolve, ms));
    },

    _clickSelector(selector, options) {
        if (!selector || typeof selector !== 'string') return Promise.resolve(false);
        const opts = options || {};
        const delayMs = Number.isFinite(opts.delayMs) ? opts.delayMs : 120;
        const retries = Number.isFinite(opts.retries) ? opts.retries : 8;
        const retryDelayMs = Number.isFinite(opts.retryDelayMs) ? opts.retryDelayMs : 100;
        const trimmed = selector.trim();
        if (!trimmed) return Promise.resolve(false);

        return new Promise((resolve) => {
            const attempt = (remaining) => {
                const el = document.querySelector(trimmed);
                if (el) {
                    el.click();
                    resolve(true);
                    return;
                }
                if (remaining <= 0) {
                    console.warn('Guide: click_selector not found:', trimmed);
                    resolve(false);
                    return;
                }
                setTimeout(() => attempt(remaining - 1), retryDelayMs);
            };
            setTimeout(() => attempt(retries), delayMs);
        });
    },

    _closeOpenDialog() {
        const excludeIds = new Set([
            'guide-modal',
            'guide-explanation-overlay',
            'guide-explanation-dialog',
            'getting-started-overlay',
            'getting-started-dialog',
            'loading-indicator',
        ]);

        const isVisible = (el) => {
            if (!el || excludeIds.has(el.id)) return false;
            const style = window.getComputedStyle(el);
            if (style.display === 'none' || style.visibility === 'hidden') return false;
            return style.display === 'flex' || style.display === 'block';
        };

        const zIndexOf = (el) => {
            const z = parseInt(window.getComputedStyle(el).zIndex, 10);
            return Number.isFinite(z) ? z : 0;
        };

        const candidates = [];
        document.querySelectorAll('.modal, .config-modal-overlay').forEach((el) => {
            if (isVisible(el)) candidates.push(el);
        });
        if (!candidates.length) return;

        candidates.sort((a, b) => zIndexOf(b) - zIndexOf(a));
        const top = candidates[0];

        const closeBtn = top.querySelector('.modal-header .modal-close-btn')
            || top.querySelector('.modal-close-btn')
            || (top.id === 'config-modal-overlay' ? document.getElementById('close-config') : null);
        if (closeBtn) {
            closeBtn.click();
            return;
        }

        if (typeof Modals !== 'undefined' && Modals._closeModal) {
            Modals._closeModal(top);
        } else {
            top.style.display = 'none';
        }
    },

    // Named navigation actions referenced by guide steps stored in the database.
    NavActions: {
        showGettingStartedDialog: () => Guide._showGettingStartedDialog(),
        closeOpenDialog: () => Guide._closeOpenDialog(),
        pause0_5s: () => Guide._pauseNavActions(500),
        pause1s: () => Guide._pauseNavActions(1000),
        pause2s: () => Guide._pauseNavActions(2000),
        pause5s: () => Guide._pauseNavActions(5000),
        // Left sidebar — explore
        openSmsMessages: () => Guide._clickSidebarButton('sms-messages-sidebar-btn'),
        openEmailGallery: () => Guide._clickSidebarButton('email-gallery-sidebar-btn'),
        openImageGallery: () => Guide._clickSidebarButton('new-image-gallery-sidebar-btn'),
        openFacebookAlbums: () => Guide._clickSidebarButton('fb-albums-sidebar-btn'),
        openFacebookPosts: () => Guide._clickSidebarButton('fb-posts-sidebar-btn'),
        openMultiSourceSearch: () => Guide._clickSidebarButton('message-similarity-sidebar-btn'),
        openLocations: () => Guide._clickSidebarButton('locations-sidebar-btn'),
        openArtefacts: () => Guide._clickSidebarButton('artefacts-sidebar-btn'),
        // Left sidebar — bottom
        openIdentityProfile: () => Guide._clickSidebarButton('identity-profile-wizard-btn'),
        openDataImport: () => Guide._clickSidebarButton('data-import-sidebar-btn'),
        // Import & Manage Data — tabs
        openDataImportImport: () => Guide._openDataImportTab('import'),
        openDataImportMaintenance: () => Guide._openDataImportTab('maintenance'),
        openDataImportBackgroundJobs: () => Guide._openDataImportTab('background-jobs'),
        openConfiguration: () => Guide._clickSidebarButton('settings-data-import-sidebar-btn'),
        // Right sidebar — tools
        openPreviousResponses: () => Guide._clickSidebarButton('previous-responses-sidebar-btn'),
        openSuggestions: () => Guide._clickSidebarButton('suggestions-sidebar-btn'),
        openContacts: () => Guide._clickSidebarButton('contacts-sidebar-btn'),
        openContactsRelationships: () => {
            if (typeof Modals !== 'undefined' && Modals.Contacts?.open) {
                Modals.Contacts.open('relationships');
                return;
            }
            Guide._clickSidebarButton('contacts-sidebar-btn');
            setTimeout(() => {
                document.querySelector('#contacts-modal .manage-contacts-tab-btn[data-manage-contacts-tab="relationships"]')?.click();
            }, 120);
        },
        openProfiles: () => Guide._clickSidebarButton('profiles-sidebar-btn'),
        openSensitiveData: () => Guide._clickSidebarButton('sensitive-data-sidebar-btn'),
        openDashboard: () => Guide._clickSidebarButton('statistics-sidebar-btn'),
        // Right sidebar — bottom
        openHaveAChat: () => Guide._clickSidebarButton('have-a-chat-sidebar-btn'),
        openRandomQuestion: () => Guide._clickSidebarButton('random-question-sidebar-btn'),
        openTodaysThing: () => Guide._clickSidebarButton('todays-thing-sidebar-btn'),
        openInterviewer: () => Guide._clickSidebarButton('interviewer-sidebar-btn'),
        // Top bar — personality / voice
        openPersonalitySettings: () => Guide._clickSidebarButton('voice-settings-trigger'),
        // Configuration — tabs
        openConfigAppearance: () => Guide._openConfigurationTab('settings'),
        openConfigApiKeys: () => Guide._openConfigurationTab('api-keys'),
        openConfigAiSetup: () => Guide._openConfigurationTab('ai-setup-config'),
        openConfigSubjectConfiguration: () => Guide._openConfigurationTab('subject-configuration'),
        openConfigRegions: () => Guide._openConfigurationTab('regions-config'),
        openConfigSuggestions: () => Guide._openConfigurationTab('suggestions-config'),
        openConfigGuideTopics: () => Guide._openConfigurationTab('guide-topics-config'),
        openConfigCustomVoices: () => Guide._openConfigurationTab('custom-voices'),
        openConfigManageVisitorKeys: () => Guide._openConfigurationTab('manage-keys'),
        openConfigToolsAccess: () => Guide._openConfigurationTab('tools-access'),
        openConfigToolTest: () => Guide._openConfigurationTab('tool-test'),
        openSettingsManageKeys: () => Guide._openConfigurationTab('manage-keys'),
        // Chat area
        openReferenceDocuments: () => document.getElementById('chat-context-status-refs-seg')?.click(),
        openToolCallsDialog: () => document.getElementById('chat-context-last-toolcalls-seg')?.click(),
    },

    topicAliases: {
        'Settings and data import': 'SettingsAndDataImport',
        Settings: 'SettingsAndDataImport',
        'Data import': 'SettingsAndDataImport',
        'Browsing images': 'BrowsingImages',
        'Managing contacts': 'ManagingContacts',
        'Create a profile for a contact': 'CreateContactProfile',
        'Contact profiles': 'CreateContactProfile',
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

    _getTopicDismissNavAction(topicKey) {
        if (!topicKey) return '';
        const topics = this._topics || {};
        const topicConfig = topics[topicKey];
        if (!topicConfig) return '';
        return topicConfig.dismiss_navigate_action || topicConfig.dismissNavigateAction || '';
    },

    _getTopicQAStatus(topic) {
        if (!topic) return '';
        const raw = topic.QA !== undefined ? topic.QA : topic.qa;
        if (raw === true) return 'OK';
        const value = String(raw).trim().toUpperCase();
        if (value === 'OK') return 'OK';
        if (value === 'TEST') return 'TEST';
        return '';
    },

    _closeExplanation(options) {
        const opts = options || {};
        const topicKey = this._currentTopic;
        const dismissAction = opts.skipDismissAction ? '' : this._getTopicDismissNavAction(topicKey);

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

        if (dismissAction) {
            void this._runNavActions(dismissAction);
        }
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
        const glowSel   = step.glow || '';
        const navAction = step.navigate_action || step.navigateAction || '';
        const clickSelector = step.click_selector || step.clickSelector || step.click || '';
        const position  = step.position || 'middle-center';

        const applyGlowAndShow = () => {
            this._clearGlows();
            this._applyGlow(glowSel);

            textEl.textContent = step.text || '';
             
            //if (progressEl) progressEl.textContent = topicConfig.title + ' — Step ' + (stepIndex + 1) + ' of ' + steps.length;
            if (progressEl) progressEl.textContent = topicConfig.title 

            if (imgEl) {
                const imageUrl = step.image_url || step.imageUrl || step.image || '';
                if (imageUrl) {
                    imgEl.src = imageUrl;
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

        const runPreShowActions = async () => {
            if (navAction) {
                await this._runNavActions(navAction);
            }
            if (clickSelector) {
                await this._clickSelector(clickSelector);
            }
        };

        if (navAction || clickSelector) {
            overlay.style.display = 'none';
            dialog.style.display  = 'none';
            void runPreShowActions().then(() => {
                if (step.text) {
                    setTimeout(applyGlowAndShow, 120);
                }
            });
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
            const category = (t.category && t.category.trim()) || 'Daily Use';
            if (!byCategory[category]) byCategory[category] = [];
            byCategory[category].push({
                key: key,
                title: t.title || key,
                description: t.description || '',
                recommended: !!t.recommended,
                qaStatus: this._getTopicQAStatus(t),
            });
        });
        Object.keys(byCategory).forEach((categoryKey) => {
            const arr = byCategory[categoryKey];
            arr.sort((a, b) => {
                if (!!a.recommended !== !!b.recommended) return a.recommended ? -1 : 1;
                return String(a.title || '').localeCompare(String(b.title || ''));
            });
        });

        const knownSet = new Set(this._categoryOrder);
        const extraCategories = Object.keys(byCategory).filter((c) => !knownSet.has(c)).sort();
        const renderOrder = [...this._categoryOrder, ...extraCategories];

        let html = '';
        renderOrder.forEach((category) => {
            const items = byCategory[category];
            if (!items || items.length === 0) return;
            html += '<section class="guide-topic-group">' +
                '<h3 class="guide-topic-group-title">' + category + '</h3>' +
                '<ul class="guide-topic-group-list">';
            items.forEach((t) => {
                let qaClass = '';
                if (t.qaStatus === 'OK') qaClass = ' guide-topic-btn--qa-ok';
                else if (t.qaStatus === 'TEST') qaClass = ' guide-topic-btn--qa-test';
                html += '<li class="guide-topic-item">' +
                    '<button type="button" class="guide-topic-btn' + qaClass + '" data-topic="' + t.key.replace(/"/g, '&quot;') + '">' +
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
