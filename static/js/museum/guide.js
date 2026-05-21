'use strict';

const Guide = {
    _currentTopic: null,
    _currentStep: 0,
    _lastFocusedElement: null,
    _focusTrapHandler: null,
    _escHandler: null,

    topics: {
        GettingStarted: {
            title: 'Getting started',
            description: 'Fast orientation to chat, sidebars, and key actions.',
            category: 'Getting Started',
            recommended: true,
            steps: [{ navigate() { Guide._showGettingStartedDialog(); } }]
        },
        FirstRunQuickSetup: {
            title: 'Quick setup checklist',
            description: 'A short first-run flow: import, configure, ask a question, unlock.',
            category: 'Getting Started',
            recommended: true,
            steps: [
                {
                    // text: 'Open Import & Manage Data and run at least one import.',
                    glow: '#data-import-sidebar-btn',
                    navigate() {
                        const btn = document.getElementById('data-import-sidebar-btn');
                        if (btn) btn.click();
                    }
                },
                // {
                //     text: 'Open Configuration and review Subject Configuration details.',
                //     glow: '#settings-data-import-sidebar-btn',
                //     navigate() {
                //         const cfgBtn = document.getElementById('settings-data-import-sidebar-btn');
                //         if (cfgBtn) cfgBtn.click();
                //         setTimeout(() => {
                //             const subjectTab = document.querySelector('.config-tab-button[data-tab="subject-configuration"]');
                //             if (subjectTab) subjectTab.click();
                //         }, 120);
                //     }
                // },
                // { text: 'Ask your first question in chat to verify everything is working.', glow: '#user-input' },
                // {
                //     text: 'Optional: unlock encryption for sensitive-data tools and private store access.',
                //     glow: '#master-key-unlock-submit',
                //     position: 'middle-center',
                //     navigate() {
                //         const modal = document.getElementById('master-key-unlock-modal');
                //         const input = document.getElementById('master-key-unlock-input');
                //         if (modal) modal.style.display = 'flex';
                //         if (input) input.focus();
                //     }
                // }
            ]
        },
        AskingQuestions: {
            title: 'Asking questions',
            description: 'Get better answers and steer perspective and tone.',
            category: 'Daily Use',
            steps: [
                { text: 'Type your question in chat and press Send. Try: "Show me photos from last summer" or "Who have I emailed most?"', glow: '#user-input' },
                { text: 'Use AI Personality settings to change the response style and mood.', glow: '#voice-settings-trigger', position: 'top-center' },
                { text: 'Switch "Who\'s Asking?" to frame answers from visitor or owner perspective.', glow: '#its-me-visitor-switch', position: 'middle-right' },
                { text: 'Reference documents can improve answers for specific, factual, or long-form questions.', position: 'middle-center' }
            ]
        },
        'Browsing images': {
            title: 'Browsing images',
            description: 'Find memories in photo galleries, albums, and posts.',
            category: 'Daily Use',
            steps: [
                { text: 'Open Images in the left sidebar to browse your main photo collection.', glow: '#new-image-gallery-sidebar-btn' },
                { text: 'Open Facebook Albums to browse imported album collections.', glow: '#fb-albums-sidebar-btn' },
                { text: 'Open Facebook Posts to browse imported posts that may contain photos.', glow: '#fb-posts-sidebar-btn' },
                { text: 'You can also ask AI directly: "Find photos with John" or "Show me photos from last Christmas".', glow: '#user-input' }
            ]
        },
        'Managing contacts': {
            title: 'Managing contacts',
            description: 'Use Contacts and Relationships, Profiles, and contact tools effectively.',
            category: 'Daily Use',
            steps: [
                { text: 'Open Contacts and Relationships in the right sidebar to browse people or open the Relationships tab for the connection map.', glow: '#contacts-sidebar-btn' },
                { text: 'Open Profiles to view detailed notes and relationship context.', glow: '#profiles-sidebar-btn' },
                { text: 'Use the Relationships tab for a visual network of people and connection strength.', glow: '#contacts-sidebar-btn' },
                { text: 'Use Configuration to classify contacts and tune contact recognition.', glow: '#settings-data-import-sidebar-btn', position: 'bottom-center' }
            ]
        },
        'Voice and AI settings': {
            title: 'Voice and AI settings',
            description: 'Choose provider, voice, mood, and response style.',
            category: 'Daily Use',
            steps: [
                { text: 'Open voice settings from the top bar image.', glow: '#voice-settings-trigger', position: 'top-center' },
                { text: 'Choose a voice or custom persona to control speaking style.' },
                { text: 'When using owner voice, set a mood to shape tone.' },
                { text: 'Companion Mode makes responses more conversational.' },
                { text: 'Creativity controls factual vs expressive response style.' },
                { text: 'Switch providers (Gemini, Claude, DeepSeek, Local AI) as needed.' }
            ]
        },
        "Today's Thing of Interest": {
            title: "Today's Thing of Interest",
            description: 'Generate fresh prompts based on interests.',
            category: 'Daily Use',
            steps: [
                { text: "Today's Thing suggests memory prompts based on subject interests.", glow: '#todays-thing-sidebar-btn' },
                { text: 'Click repeatedly for different suggestions.' },
                { text: 'Add more interests in Configuration for better variety.', glow: '#settings-data-import-sidebar-btn', position: 'bottom-center' }
            ]
        },
        'Email catchup': {
            title: 'Email catchup',
            description: 'Summarize and explore imported email quickly.',
            category: 'Daily Use',
            steps: [
                { text: 'Open Emails from the left sidebar.', glow: '#email-gallery-sidebar-btn' },
                { text: 'Ask for a recent summary: "Summarise my emails from this week".', glow: '#user-input' },
                { text: 'Drill into a sender or topic for deeper summaries.', glow: '#user-input' }
            ]
        },
        ArchiveSelectionLogin: {
            title: 'Archive selection at sign-in',
            description: 'Choose the right user archive before entering password.',
            category: 'Getting Started',
            steps: [
                { text: 'On the login screen, the selected user determines which archive database is active.' },
                { text: 'If multiple users exist, confirm the selected name before signing in.' },
                { text: 'If sign-in fails unexpectedly, switch away and back to the target user once, then try again.' }
            ]
        },
        LocalAISetupHealth: {
            title: 'Local AI setup and health',
            description: 'Understand Ollama readiness and required models.',
            category: 'Setup & Import',
            steps: [
                { text: 'Local AI depends on the Ollama server plus two models: gemma4 and embeddinggemma.' },
                { text: 'On login, open AI & Setup to confirm server status and model availability.' },
                { text: 'If a model is missing, use Download required models. Initial download can take several minutes.' },
                { text: 'Some AI features may be unavailable when Local AI is not ready.' }
            ]
        },
        LocalAITroubleshooting: {
            title: 'Local AI troubleshooting',
            description: 'Recover quickly from common Local AI issues.',
            category: 'Troubleshooting',
            steps: [
                { text: 'If Local AI is unavailable, verify Ollama is running and both models are installed.' },
                { text: 'Re-open AI & Setup and refresh statuses after model downloads complete.' },
                { text: 'If issues persist, use a cloud provider temporarily and retry Local AI later from AI & Setup.' }
            ]
        },
        SettingsAndDataImport: {
            title: 'Settings and data import',
            description: 'Configure app behavior and import source archives.',
            category: 'Setup & Import',
            steps: [
                { text: 'Open Configuration from the left sidebar.', glow: '#settings-data-import-sidebar-btn', position: 'bottom-center' },
                { text: 'Use chat settings, subject configuration, custom voices, and key management in this dialog.' },
                { text: 'Open Import & Manage Data for file imports and maintenance jobs.', glow: '#data-import-sidebar-btn', position: 'bottom-center' }
            ]
        },
        ImportPathsOverview: {
            title: 'Import paths overview',
            description: 'Know which import flow to use for each source.',
            category: 'Setup & Import',
            steps: [
                { text: 'Use ZIP upload imports for Facebook, Instagram, WhatsApp, and iMessage exports.', glow: '#data-import-sidebar-btn' },
                { text: 'Use email import for mailbox data and follow-up embedding jobs.' },
                { text: 'Use filesystem/photo imports when source data already exists on local disk.' }
            ]
        },
        CreateAndSwitchArchives: {
            title: 'Create and switch archives',
            description: 'Manage archive profiles safely across users.',
            category: 'Setup & Import',
            steps: [
                { text: 'Use login profile controls or admin archives to create and manage archive entries.' },
                { text: 'Switching archive changes the active SQLite database used for sign-in and chat.' },
                { text: 'Disabling an archive hides it from login selection without deleting data.' }
            ]
        },
        SharePrivacyVisitorAccess: {
            title: 'Share and privacy basics',
            description: 'Understand visitor scope and sensitive access.',
            category: 'Troubleshooting',
            steps: [
                {
                    text: 'To manage visitor access, open Configuration and switch to the "Manage Visitor Keys" tab.',
                    glow: '.config-tab-button[data-tab="manage-keys"]',
                    position: 'middle-right',
                    fallbackText: 'Click Configuration in the left sidebar, then choose Manage Visitor Keys.',
                    navigate() {
                        const configBtn = document.getElementById('settings-data-import-sidebar-btn');
                        if (configBtn) configBtn.click();
                        setTimeout(() => {
                            const manageKeysTab = document.querySelector('.config-tab-button[data-tab="manage-keys"]');
                            if (manageKeysTab) manageKeysTab.click();
                        }, 120);
                    }
                },
                { text: 'Sensitive/private data and some tools may require owner master unlock.' },
                { text: 'If a feature is missing, verify current role and access tier first.' }
            ]
        },
        ImportFacebookArchive: {
            title: 'Import Facebook archive',
            description: 'Step-by-step Facebook ZIP import flow.',
            category: 'Setup & Import',
            steps: [
                { text: 'Request a Facebook data download in JSON format and unzip it locally.' },
                { text: 'Open Import & Manage Data from the left sidebar.', glow: '#data-import-sidebar-btn', position: 'bottom-center' },
                {
                    text: 'Click Upload on the Facebook row and select the exported folder.',
                    position: 'top-right',
                    glow: '#data-import-modal .data-import-row[data-import="upload_zip"][data-zip-archive-type="facebook"]',
                    fallbackText: 'Open Import & Manage Data first, then use Upload on Facebook row.',
                    navigate() {
                        const btn = document.getElementById('data-import-sidebar-btn');
                        if (btn) btn.click();
                    }
                }
            ]
        },
        ImportInstagramArchive: {
            title: 'Import Instagram archive',
            description: 'Step-by-step Instagram ZIP import flow.',
            category: 'Setup & Import',
            steps: [
                { text: 'Request an Instagram data download and unzip it locally.' },
                { text: 'Open Import & Manage Data from the left sidebar.', glow: '#data-import-sidebar-btn', position: 'bottom-center' },
                {
                    text: 'Click Upload on the Instagram row and select the exported folder.',
                    position: 'top-right',
                    glow: '#data-import-modal .data-import-row[data-import="upload_zip"][data-zip-archive-type="instagram"]',
                    fallbackText: 'Open Import & Manage Data first, then use Upload on Instagram row.',
                    navigate() {
                        const btn = document.getElementById('data-import-sidebar-btn');
                        if (btn) btn.click();
                    }
                }
            ]
        }
    },
    topicAliases: {
        'Settings and data import': 'SettingsAndDataImport',
        Settings: 'SettingsAndDataImport',
        'Data import': 'SettingsAndDataImport'
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

    _positionDialog(dialog, position) {
        const pos = this._positions[position] || this._positions['middle-center'];
        Object.assign(dialog.style, pos);
    },
    _getTopicConfig(topicKey) {
        const key = this.topicAliases[topicKey] || topicKey;
        return { key, config: this.topics[key] };
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
        const topicConfig = this.topics[this._currentTopic];
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

        const applyGlowAndShow = () => {
            this._clearGlows();
            const glowOk = this._applyGlow(step.glow);

            textEl.textContent = step.text || '';
            if (step.glow && !glowOk && step.fallbackText) {
                textEl.textContent += '\n\n' + step.fallbackText;
            } else if (step.glow && !glowOk) {
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

            this._positionDialog(dialog, step.position);
            overlay.style.display = 'block';
            dialog.style.display  = 'block';
            this._focusTrap(dialog);
            this._bindEscape(() => this._closeExplanation());
            overlay.onclick = () => this._closeExplanation();
        };

        if (step.navigate) {
            overlay.style.display = 'none';
            dialog.style.display  = 'none';
            step.navigate();
            if (step.text) {
                setTimeout(applyGlowAndShow, 120);
            } 
        } else {
            if (step.text) {
                applyGlowAndShow()
            }
           
        }
    },

    onTopicSelected(topic) {
        const guideModal = document.getElementById('guide-modal');
        if (guideModal) guideModal.style.display = 'none';
        document.querySelectorAll('.guide-topic-btn').forEach(b => b.classList.remove('guide-topic-glow'));
        this._lastFocusedElement = document.getElementById('guide-sidebar-btn') || document.activeElement;
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
        const byCategory = {};
        this._categoryOrder.forEach((c) => { byCategory[c] = []; });
        Object.keys(this.topics).forEach((key) => {
            const t = this.topics[key];
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
            const topics = byCategory[category];
            if (!topics || topics.length === 0) return;
            html += '<section class="guide-topic-group">' +
                '<h3 class="guide-topic-group-title">' + category + '</h3>' +
                '<ul class="guide-topic-group-list">';
            topics.forEach((t) => {
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
    validateTopicBindings() {
        const btns = document.querySelectorAll('.guide-topic-btn[data-topic]');
        btns.forEach((btn) => {
            const topic = btn.dataset.topic || '';
            const resolved = this._getTopicConfig(topic);
            if (!resolved.config) {
                console.warn('Guide topic button missing config:', topic);
            }
        });
    },
    openGuideModal() {
        const guideModal = document.getElementById('guide-modal');
        if (!guideModal) return;
        const topicsContainer = document.getElementById('guide-topics-list');
        if (topicsContainer && !topicsContainer.querySelector('.guide-topic-btn')) {
            this.renderTopicList();
            this.validateTopicBindings();
        }
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
    init() {
        try {
            this.renderTopicList();
            this.validateTopicBindings();
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
