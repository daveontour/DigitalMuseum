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
                text: 'You can use the "Who\'s Asking?" switch in the top bar to select the person who is asking the question. If you select yourself then you can frame questions from your own perspective.',
                glow: '#its-me-visitor-switch',
                position: 'middle-right',
                image: '/static/images/stats.png'
             },
             {
                text: 'The AI responds based on your question, what you\'ve been discussing, and the information available to it. Consider providing reference documents to help it answer specific questions.',
                position: 'middle-center'
             }
        ],
        'Browsing images': [
            {
                text: 'Open Images from the left sidebar to browse and search your personal photo collection.',
                glow: '#new-image-gallery-sidebar-btn'
            },
            {
                text: 'Open Facebook Albums to browse albums imported from your Facebook archive.',
                glow: '#fb-albums-sidebar-btn'
            },
            {
                text: 'Open Facebook Posts to browse your imported Facebook posts, which may also include photos.',
                glow: '#fb-posts-sidebar-btn'
            },
            {
                text: 'You can also ask the AI to find photos for you — try "Show me photos from last Christmas" or "Find photos with John".',
                glow: '#user-input'
            }
        ],
        'Managing contacts': [
            {
                text: 'Open Contacts from the right sidebar to see everyone who appears in your emails, messages, and social data.',
                glow: '#contacts-sidebar-btn'
            },
            {
                text: 'Open Profiles to view and manage detailed profiles for your contacts, including notes and relationship context.',
                glow: '#profiles-sidebar-btn'
            },
            {
                text: 'Open Relationships to see a visual network of the people in your life and how they connect to each other.',
                glow: '#relationships-btn'
            },
            {
                text: 'In Settings you can classify contacts, exclude people from AI responses, and adjust how contacts are recognised across your data.',
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            }
        ],
        'Voice and AI settings': [
            {
                text: 'Click the voice image in the top bar to open the voice and AI settings.',
                glow: '#voice-settings-trigger',
                position: 'top-center'
            },
            {
                text: 'Choose who answers your questions — a built-in voice like Expert or Friend, the subject\'s own voice, or a custom voice you have created in Settings.'
            },
            {
                text: 'When the subject\'s own voice is selected, you can also set a mood to shape the tone of responses: Neutral, Reflective, Excited, Melancholic, and more.'
            },
            {
                text: 'Companion Mode turns the AI into a conversational partner that responds like a friend rather than an assistant — ideal for casual, ongoing dialogue.'
            },
            {
                text: 'The Creativity slider controls how imaginative the AI\'s responses are. Lower values stay closer to the facts; higher values allow more flair and interpretation.'
            },
            {
                text: 'You can switch between AI providers — Gemini and Claude — to use your preferred model or compare responses.'
            }
        ],
        "Today's Thing of Interest": [
            {
                text: "Today's Thing suggests something interesting to explore — a question, a memory, or a topic — based on the subject's personal interests.",
                glow: '#todays-thing-sidebar-btn'
            },
            {
                text: "Click the button to generate a suggestion. Each click produces something different drawn from the subject's interests and their data."
            },
            {
                text: "To get richer suggestions, add interests in the Interests tab in Settings. The more interests you add, the more varied and personal the suggestions.",
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            }
        ],
        'Email catchup': [
            {
                text: 'Open Emails from the left sidebar to browse and search your imported email archive.',
                glow: '#email-gallery-sidebar-btn'
            },
            {
                text: 'You can ask the AI to summarise your recent emails — try "Summarise my emails from this week" or "What has John been writing to me about?"',
                glow: '#user-input'
            },
            {
                text: 'For a deeper dive, ask the AI to summarise all emails from a specific person or about a particular topic.',
                glow: '#user-input'
            }
        ],
        'Settings': [
            {
                text: 'Open Settings from the left sidebar to configure the app, manage the subject\'s data, and personalise your experience.',
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            },
            {
                text: 'The Chat Settings tab lets you adjust font size, control whether audio and image links are shown in responses, and enable auto-voice for short replies.'
            },
            {
                text: 'Subject Configuration (requires a master key) is where you set the subject\'s name, contact details, and have the AI generate a writing style analysis or psychological profile.'
            },
            {
                text: 'Custom Voices lets you create new AI personas with their own name, instructions, and creativity level — useful for tailoring how the AI presents itself.'
            },
            {
                text: 'Manage Keys lets you create master keys for secure access to private settings, and configure hints shown to visitors who need to unlock the app.'
            }
        ],
        'Data import': [
            {
                text: 'You can import data from Facebook, Instagram, WhatsApp, email, and more to build up the subject\'s digital archive.',
                glow: '#settings-data-import-sidebar-btn',
                position: 'bottom-center'
            },
            {
                text: 'Go to Settings and open the "Manage Imported Data" tab to see all available import options.',
                navigate() {
                    const settingsBtn = document.getElementById('settings-data-import-sidebar-btn');
                    if (settingsBtn) settingsBtn.click();
                }
            },
            {
                text: 'Click any import tile to run that import. Facebook and Instagram imports require you to first download your data archive from their respective settings pages.',
                glow: '.config-tab-button[data-tab="manage-imported-data"]',
                navigate() {
                    const tab = document.querySelector('.config-tab-button[data-tab="manage-imported-data"]');
                    if (tab) tab.click();
                }
            }
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
