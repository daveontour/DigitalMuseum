'use strict';

const Guide = {
    _currentTopic: null,
    _currentStep: 0,

    topics: {
        'GettingStarted': [
            {
                navigate() {
                    Guide._showGettingStartedDialog();
                }
            }
        ],
        'AskingQuestions': [
            { text: 'Type your question or comment in the chat box and press Send. The AI can search your emails, messages, photos, social media posts, and more.\nTry "Show me photos from last summer" or "Who have I emailed most?", or "Tell me about my relationship with John"',
              glow: '#user-input'
             },
             {
                text: 'You can select the personality of the AI in the voice settings. The AI will respond in the style of the selected personality.\nIf you select your own voice, then you\'ll also be able to select the mood of the AI\'s response.',
                glow: '#voice-settings-trigger',
                position: 'top-center'
             },
             {
                text: 'You can use the "Who\'s Asking?" switch in the top bar to select the person who is asking the question. If you select yourself then you can frame question from your own perspective',
                glow: '#its-me-visitor-switch',
                position: 'middle-right',
                image: '/static/images/stats.png'
             },{
                text:'The AI will respond based on the your question or comment, what you\'ve been discussing, and what information you have made availabler to it. Consider providing reference documents to the AI to help it answer your question.',
                position: 'middle-center'
             }              
        ],
        'Browsing images': [
            { text: 'Open Image Gallery or Facebook Albums from the left sidebar to browse and search your photo collection.' }
        ],
        'Managing contacts': [
            { text: 'Use the Contacts button in the right sidebar to view and manage your contact list.' }
        ],
        'Voice and AI settings': [
            { text: 'Click the voice image in the top bar to change who answers (expert, friend, etc.), set creativity, or enable Companion Mode.' }
        ],
        "Today's Thing of Interest": [
            { text: "Suggests something interesting for today based on your interests. Add interests in Settings if you haven't yet.", glow: '#todays-thing-sidebar-btn' }
        ],
        'Email catchup': [
            { text: 'Get a summary of recent emails. Use this to catch up on your inbox.' }
        ],
        'Settings and data import': [
            { text: 'Configure the app, manage subject details, import messages and images, and manage your interests.' }
        ],
        'ImportFacebookArchive': [
            {
                text: 'Importing Facebook data is a two-step process. First, request a download of your Facebook data from facebook.com/settings in JSON format and unzip the archive on your computer.',
            },
            {
                text: 'When you have the archive, go to the Settings and Data Import page.',
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            },
            {
                text:'Then select the "Manage Imported Data" tab.',
                navigate() {
                    const settingsBtn = document.getElementById('settings-data-import-sidebar-btn');
                    if (settingsBtn) settingsBtn.click();
                }
            },
            {
                text: 'Click the highlighted Facebook Archive Import tile to run the import. Select the archive directory when prompted.',
                position: 'top-right',
                glow: '.import-control-tile[data-import="facebook_all"]',
                navigate() {
                    const fbTab = document.querySelector('.config-tab-button[data-tab="manage-imported-data"]');
                    if (fbTab) fbTab.click();
                }
            }
        ],
        'ImportInstagramArchive': [
            {
                text: 'Importing Instagram data is a two-step process. First, request a download of your Facebook data from facebook.com/settings in JSON format and unzip the archive on your computer.',
            },
            {
                text: 'When you have the archive, go to the Settings and Data Import page.',
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            },
            {
                text:'Then select the "Manage Imported Data" tab.',
                navigate() {
                    const settingsBtn = document.getElementById('settings-data-import-sidebar-btn');
                    if (settingsBtn) settingsBtn.click();
                }
            },
            {
                text: 'Click the highlighted Instagram Archive Import tile to run the import. Request a download of your Instagram data from instagram.com, then select the export directory when prompted.',
                glow: '.import-control-tile[data-import="instagram"]',
                navigate() {
                    const fbTab = document.querySelector('.config-tab-button[data-tab="manage-imported-data"]');
                    if (fbTab) fbTab.click();
                }
            }
        ],
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

    _clearGlows() {
        document.querySelectorAll('.guide-glow').forEach(el => el.classList.remove('guide-glow'));
    },

    _applyGlow(selector) {
        if (!selector) return;
        const el = document.querySelector(selector);
        if (el) el.classList.add('guide-glow');
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
        const nextBtn  = document.getElementById('guide-explanation-next-btn');
        const closeBtn = document.getElementById('guide-explanation-close-btn');
        const imgEl   = document.getElementById('guide-explanation-image');
        if (overlay)  { overlay.style.display  = 'none'; overlay.onclick  = null; }
        if (dialog)   { dialog.style.display   = 'none'; dialog.classList.remove('guide-explanation-dialog-has-close'); }
        if (nextBtn)  { nextBtn.style.display  = 'none'; nextBtn.onclick  = null; }
        if (closeBtn) { closeBtn.style.display = 'none'; closeBtn.onclick = null; }
        if (imgEl)    { imgEl.style.display    = 'none'; imgEl.src        = ''; }
        this._clearGlows();
        this._currentTopic = null;
        this._currentStep  = 0;
    },

    _showStep(stepIndex) {
        const steps = this.topics[this._currentTopic];
        if (!steps || stepIndex >= steps.length) { this._closeExplanation(); return; }

        const step     = steps[stepIndex];
        const isLast   = stepIndex === steps.length - 1;
        const overlay  = document.getElementById('guide-explanation-overlay');
        const dialog   = document.getElementById('guide-explanation-dialog');
        const textEl   = document.getElementById('guide-explanation-text');
        const imgEl    = document.getElementById('guide-explanation-image');
        const nextBtn  = document.getElementById('guide-explanation-next-btn');
        const closeBtn = document.getElementById('guide-explanation-close-btn');
        if (!overlay || !dialog || !textEl) return;

        this._currentStep = stepIndex;

        const applyGlowAndShow = () => {
            this._clearGlows();
            this._applyGlow(step.glow);

            textEl.textContent = step.text;

            if (imgEl) {
                if (step.image) {
                    imgEl.src = step.image;
                    imgEl.style.display = 'block';
                } else {
                    imgEl.src = '';
                    imgEl.style.display = 'none';
                }
            }

            nextBtn.style.display  = isLast ? 'none'  : 'block';
            nextBtn.onclick        = isLast ? null     : () => this._showStep(stepIndex + 1);
            closeBtn.style.display = isLast ? 'block'  : 'none';
            closeBtn.onclick       = isLast ? () => this._closeExplanation() : null;
            dialog.classList.toggle('guide-explanation-dialog-has-close', isLast);

            this._positionDialog(dialog, step.position);
            overlay.style.display = 'block';
            dialog.style.display  = 'block';
            overlay.onclick = () => this._closeExplanation();
        };

        if (step.navigate) {
            overlay.style.display = 'none';
            dialog.style.display  = 'none';
            step.navigate();
            if (step.text) {
                setTimeout(applyGlowAndShow, 100);
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

        this._currentTopic = topic;
        this._showStep(0);
    }
};
