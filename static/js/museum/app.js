'use strict';

const AppActions = {
    // [CONSTANTS.FUNCTION_NAMES.FirstFunction]: async () => { // getFacebookChatters
    //     UI.clearError();
    //     DOM.infoBox.classList.add('hidden');
    //     UI.setControlsEnabled(false);
    //     UI.showLoadingIndicator();
    //     try {
    //         const data = await ApiService.fetchFacebookChatters();
    //         let markdownText = '# Facebook Chat Statistics\n\n|Participant|Number of Messages|\n|-----|---|\n';
    //         for (const message of Object.values(data)) { // Iterate over values if data is an object
    //             markdownText += `| ${message.participant[0].name} |${message.number_of_messages}|\n`;
    //         }
    //         Chat.addMessage('assistant', markdownText, true);
    //     } catch (error) {
    //         console.error('Error in getFacebookChatters:', error);
    //         UI.displayError("Failed to get FB chatters: " + error.message);
    //     } finally {
    //         UI.setControlsEnabled(true);
    //         UI.hideLoadingIndicator();
    //     }
    // },
    ["showFBAlbumsOptions"]: () => Modals.FBAlbums.open(),    // showFBAlbumsOptions
    ["openGeoModal"]: () => Modals.Locations.open(), // showGeoMetadataOptions
    ["showEmailGallery"]: () => Modals.EmailGallery.open(), // showEmailGalleryOptions
    //[CONSTANTS.FUNCTION_NAMES.FifthFunction]: () => SSE.browserFunctions.showLocationInfo(), // showTileAlbumOptions
    ["showImageGallery"]: () => Modals.ImageGallery.open(),
    //[CONSTANTS.FUNCTION_NAMES.SeventhFunction]: () => SSE.browserFunctions.testEmail(), // showImageGalleryOptions
 // showEmailGalleryOptions
    ["listContacts"]: () => Modals.Contacts.open(),

};
window.customObject = AppActions; // Expose for Suggestions.json if it relies on global `customObject`


const App = (() => {
    async function processFormSubmit(userPrompt, category = null, title = null, supplementary_prompt = null) {
        if (!userPrompt && !category && !title) return;

        // Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();

            const selectedVoice = VoiceSelector.getSelectedVoice();
            const selectedMood = (selectedVoice === 'owner' && DOM.ownerMood) ? DOM.ownerMood.value : null;
            
            try {
                const finalMessage = UI.getWorkModePrefix() + userPrompt;

                if (category && title) Chat.addMessage('suggestion', `**${category}:** ${title}`, true);
                else Chat.addMessage('user', userPrompt, false); // User messages are not markdown by default
                
                DOM.userInput.value = '';

                const currentUserId = localStorage.getItem('userId') || 'default';
                // Get current conversation ID if available
                const conversationId = Modals.ConversationManager ? Modals.ConversationManager.getCurrentConversationId() : null;
                
                const itsMeVisitorSwitch = document.getElementById('its-me-visitor-switch');
                const whosAsking = itsMeVisitorSwitch?.querySelector('.its-me-visitor-option.active')?.dataset?.value || 'visitor';

                const response = await ApiService.fetchChat({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: DOM.companionModeCheckbox ? DOM.companionModeCheckbox.checked : false,
                    supplementary_prompt: supplementary_prompt,
                    temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
                    conversation_id: conversationId,
                    clientId: AppState.clientId,
                    userId:currentUserId,
                    provider: DOM.llmProviderSelect ? DOM.llmProviderSelect.value : 'gemini',
                    whos_asking: whosAsking,
                });
                
                // Non-streaming JSON response handling (original code commented out streaming)
                const data = await response.json();
                UI.hideLoadingIndicator(); // Hide after getting response, before adding message
                if (data.error) UI.displayError(data.error);
                else Chat.addMessage('assistant', data.response, true, null, data.embedded_json);

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }
    async function processQuestionSubmit() {
        
        var userPrompt = CONSTANTS.RANDOM_QUESTION_PROMPT;
        var title = "Generate a random question about " + CONSTANTS.OWNER_NAME + "'s life.";

        if (!userPrompt && !category && !title) return;

        // Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();

            const selectedVoice = VoiceSelector.getSelectedVoice();
            const selectedMood = (selectedVoice === 'owner' && DOM.ownerMood) ? DOM.ownerMood.value : null;
            
            try {
                const finalMessage = UI.getWorkModePrefix() + userPrompt;

                Chat.addMessage('suggestion', `**Random Question:** ${title}`, true); // User messages are not markdown by default
                
                DOM.userInput.value = '';

                const currentUserId = localStorage.getItem('userId') || 'default';
                // Get current conversation ID if available
                const conversationId = Modals.ConversationManager ? Modals.ConversationManager.getCurrentConversationId() : null;
                
                const itsMeVisitorSwitch = document.getElementById('its-me-visitor-switch');
                const whosAsking = itsMeVisitorSwitch?.querySelector('.its-me-visitor-option.active')?.dataset?.value || 'visitor';

                const response = await ApiService.fetchRandomQuestion({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: DOM.companionModeCheckbox ? DOM.companionModeCheckbox.checked : false,
                    provider: DOM.llmProviderSelect ? DOM.llmProviderSelect.value : 'gemini',
                    whos_asking: whosAsking,
                });
                
                // Non-streaming JSON response handling (original code commented out streaming)
                const data = await response.json();
                UI.hideLoadingIndicator(); // Hide after getting response, before adding message
                if (data.error) UI.displayError(data.error);
                else Chat.addMessage('assistant', data.response, true, null, data.embedded_json);

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }
    async function processAnswerSubmit(userPrompt) {


        // Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();

            const selectedVoice = VoiceSelector.getSelectedVoice();
            const selectedMood = (selectedVoice === 'owner' && DOM.ownerMood) ? DOM.ownerMood.value : null;
            
            try {
                const finalMessage = UI.getWorkModePrefix() + userPrompt;

                const category = "Answer";
                const title = "Answer the question";

                if (category && title) Chat.addMessage('suggestion', `**${category}:** ${title}`, true);
                else Chat.addMessage('user', userPrompt, false); // User messages are not markdown by default
                
                DOM.userInput.value = '';

                const currentUserId = localStorage.getItem('userId') || 'default';
                // Get current conversation ID if available
                const conversationId = Modals.ConversationManager ? Modals.ConversationManager.getCurrentConversationId() : null;
                
                const itsMeVisitorSwitch = document.getElementById('its-me-visitor-switch');
                const whosAsking = itsMeVisitorSwitch?.querySelector('.its-me-visitor-option.active')?.dataset?.value || 'visitor';

                const response = await ApiService.fetchChat({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: DOM.companionModeCheckbox ? DOM.companionModeCheckbox.checked : false,
                    temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
                    conversation_id: conversationId,
                    clientId: AppState.clientId,
                    userId:currentUserId,
                    provider: DOM.llmProviderSelect ? DOM.llmProviderSelect.value : 'gemini',
                    whos_asking: whosAsking,
                    repeat_question: true,
                });
                
                // Non-streaming JSON response handling (original code commented out streaming)
                const data = await response.json();
                UI.hideLoadingIndicator(); // Hide after getting response, before adding message
                if (data.error) UI.displayError(data.error);
                else Chat.addMessage('assistant', data.response, true, null, data.embedded_json);

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }

    /** True only when the owner master key was used for this session (visitor unlock does not count). */
    async function fetchMasterUnlockedForDataImport() {
        try {
            const st = await fetch('/api/session/master-key/status', { credentials: 'same-origin' });
            if (!st.ok) return false;
            const sj = await st.json();
            return !!sj.master_unlocked;
        } catch (e) {
            return false;
        }
    }

    /** True when the session has any keyring unlock (master or visitor). */
    async function fetchSessionKeyringUnlocked() {
        try {
            const st = await fetch('/api/session/master-key/status', { credentials: 'same-origin' });
            if (!st.ok) return false;
            const sj = await st.json();
            return !!sj.unlocked;
        } catch (e) {
            return false;
        }
    }

    async function ensureMasterKeyForDataImport() {
        if (await fetchMasterUnlockedForDataImport()) return true;
        if (typeof Modals !== 'undefined' && Modals.ConfirmationModal && Modals.ConfirmationModal.open) {
            Modals.ConfirmationModal.open(
                'Owner master key required',
                'Data Import requires the owner master key. Use Unlock Encryption and choose Unlock with Master Key (a visitor key is not enough).',
                () => {
                    const m = document.getElementById('master-key-unlock-modal');
                    if (m) {
                        m.style.display = 'flex';
                        const inp = document.getElementById('master-key-unlock-input');
                        if (inp) inp.focus();
                    }
                }
            );
        } else {
            window.alert('Unlock with the owner master key before opening Data Import (visitor key is not sufficient).');
        }
        return false;
    }

    function refreshDataImportMasterKeyAccessUI() {
        void (async () => {
            const [masterOk, keyringUnlocked] = await Promise.all([
                fetchMasterUnlockedForDataImport(),
                fetchSessionKeyringUnlocked(),
            ]);
            const sidebarBtn = document.getElementById('data-import-sidebar-btn');
            const sensitiveSidebarBtn = document.getElementById('sensitive-data-sidebar-btn');
            const tiles = document.querySelectorAll('.import-data-dialog-tile[data-open-modal="data-import-modal"]');
            if (sidebarBtn) {
                sidebarBtn.style.display = masterOk ? '' : 'none';
                sidebarBtn.disabled = false;
                sidebarBtn.title = masterOk ? '' : 'Owner master key required — use Unlock with Master Key (visitor key is not enough).';
                sidebarBtn.style.opacity = '';
                sidebarBtn.style.cursor = '';
            }
            if (sensitiveSidebarBtn) {
                sensitiveSidebarBtn.style.display = keyringUnlocked ? '' : 'none';
            }
            tiles.forEach((tile) => {
                if (masterOk) {
                    tile.classList.remove('data-import-entry-blocked');
                    tile.removeAttribute('aria-disabled');
                    if (tile.hasAttribute('data-title-when-unlocked')) {
                        const orig = tile.getAttribute('data-title-when-unlocked');
                        tile.title = orig || 'Click to open';
                    }
                } else {
                    tile.classList.add('data-import-entry-blocked');
                    tile.setAttribute('aria-disabled', 'true');
                    if (!tile.hasAttribute('data-title-when-unlocked')) {
                        tile.setAttribute('data-title-when-unlocked', tile.getAttribute('title') || 'Click to open');
                    }
                    tile.title = 'Owner master key required for Data Import (visitor key is not enough).';
                }
            });

            // Settings & Data Import modal: without owner master unlock, only Settings tab is available.
            const configOverlay = document.getElementById('config-modal-overlay');
            if (configOverlay) {
                configOverlay.classList.toggle('config-modal-master-unlock-required', !masterOk);
            }
            document.querySelectorAll('.config-sidebar-requires-master').forEach((el) => {
                el.style.display = masterOk ? '' : 'none';
            });
            if (!masterOk) {
                document.querySelectorAll('.config-tab-button').forEach((btn) => btn.classList.remove('active'));
                document.querySelectorAll('.config-tab-content').forEach((c) => c.classList.remove('active'));
                const settingsBtn = document.querySelector('.config-tab-button[data-tab="settings"]');
                const settingsTab = document.getElementById('settings-tab');
                if (settingsBtn) settingsBtn.classList.add('active');
                if (settingsTab) settingsTab.classList.add('active');
            }
        })();
    }

    function initEventListeners() {
        DOM.chatForm.addEventListener('submit', (event) => {
            event.preventDefault();
            const userPrompt = DOM.userInput.value.trim();
            if (!userPrompt) return;
            processFormSubmit(userPrompt);
        });
        
        DOM.userInput.addEventListener('keydown', (event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                // dispatchEvent on form seems more robust than calling submit() directly
                DOM.chatForm.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }));
            }
        });

        // It's Me / Visitor switch - visual toggle only (no mode-change handler yet)
        const itsMeVisitorSwitch = document.getElementById('its-me-visitor-switch');
        if (itsMeVisitorSwitch) {
            itsMeVisitorSwitch.querySelectorAll('.its-me-visitor-option').forEach(opt => {
                opt.addEventListener('click', () => {
                    itsMeVisitorSwitch.querySelectorAll('.its-me-visitor-option').forEach(o => o.classList.remove('active'));
                    opt.classList.add('active');
                    itsMeVisitorSwitch.setAttribute('aria-checked', opt.dataset.value === 'its-me');
                });
            });
        }

        // Hamburger menu for config page (guard — if these elements are missing, do not skip the rest of init)
        if (DOM.hamburgerMenu && DOM.configPage) {
            DOM.hamburgerMenu.addEventListener('click', () => {
                DOM.configPage.style.display = 'flex';
                loadControlDefaults();
                if (Modals.AppConfig && Modals.AppConfig.load) void Modals.AppConfig.load();
            });
        }
        if (DOM.closeConfigBtn && DOM.configPage) {
            DOM.closeConfigBtn.addEventListener('click', () => {
                DOM.configPage.style.display = 'none';
            });
        }
        if (DOM.configPage) {
            DOM.configPage.addEventListener('click', (e) => {
                if (e.target === DOM.configPage) DOM.configPage.style.display = 'none';
            });
        }

        // Voice settings modal (opened by clicking voice image)
        if (DOM.voiceSettingsTrigger) {
            DOM.voiceSettingsTrigger.addEventListener('click', () => {
                if (DOM.voiceSettingsModal) DOM.voiceSettingsModal.style.display = 'flex';
            });
        }
        if (DOM.closeVoiceSettingsBtn) {
            DOM.closeVoiceSettingsBtn.addEventListener('click', () => {
                if (DOM.voiceSettingsModal) DOM.voiceSettingsModal.style.display = 'none';
            });
        }
        if (DOM.voiceSettingsModal) {
            DOM.voiceSettingsModal.addEventListener('click', (e) => {
                if (e.target === DOM.voiceSettingsModal) DOM.voiceSettingsModal.style.display = 'none';
            });
        }

        // Guide modal
        const guideModal = document.getElementById('guide-modal');
        const guideSidebarBtn = document.getElementById('guide-sidebar-btn');
        const closeGuideModalBtn = document.getElementById('close-guide-modal');
        if (guideSidebarBtn && guideModal) {
            guideSidebarBtn.addEventListener('click', () => {
                guideModal.style.display = 'flex';
            });
        }
        const closeGuideModal = () => {
            guideModal.style.display = 'none';
            const gsOverlay = document.getElementById('getting-started-overlay');
            const gsDialog = document.getElementById('getting-started-dialog');
            if (gsOverlay) gsOverlay.style.display = 'none';
            if (gsDialog) gsDialog.style.display = 'none';
            document.getElementById('guide-explanation-overlay').style.display = 'none';
            document.getElementById('guide-explanation-dialog').style.display = 'none';
            document.querySelectorAll('.guide-topic-btn').forEach(b => b.classList.remove('guide-topic-glow'));
            document.querySelectorAll('.guide-glow').forEach(el => el.classList.remove('guide-glow'));
            const nextBtn = document.getElementById('guide-explanation-next-btn');
            if (nextBtn) nextBtn.style.display = 'none';
            const tileCloseBtn = document.getElementById('guide-explanation-close-btn');
            if (tileCloseBtn) tileCloseBtn.style.display = 'none';
        };
        if (closeGuideModalBtn && guideModal) {
            closeGuideModalBtn.addEventListener('click', closeGuideModal);
        }
        if (guideModal) {
            guideModal.addEventListener('click', (e) => {
                if (e.target === guideModal) closeGuideModal();
            });
        }
        document.querySelectorAll('.guide-topic-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const topic = btn.dataset.topic || '';
                Guide.onTopicSelected(topic, btn);
            });
        });

        // Load control defaults from API
        let controlDefaults = {};
        let controlDefaultsListenersSetup = false;
        
        async function loadControlDefaults() {
            try {
                const response = await fetch('/api/control-defaults');
                if (response.ok) {
                    controlDefaults = await response.json();
                    populateControlDefaults();
                    if (!controlDefaultsListenersSetup) {
                        setupControlDefaultsListeners();
                        controlDefaultsListenersSetup = true;
                    }
                }
            } catch (error) {
                console.error('Error loading control defaults:', error);
            }
        }

        // Helper function to get value from localStorage or defaults
        function getControlValue(key, defaultValue) {
            const stored = localStorage.getItem(`control_defaults_${key}`);
            if (stored !== null) {
                // Try to parse as boolean or number, otherwise return as string
                if (stored === 'true') return true;
                if (stored === 'false') return false;
                if (!isNaN(stored) && stored !== '') return stored; // Return as string for numbers
                return stored;
            }
            return defaultValue;
        }

        // Helper function to save value to localStorage
        function saveControlValue(key, value) {
            if (value === null || value === undefined) {
                localStorage.removeItem(`control_defaults_${key}`);
            } else {
                localStorage.setItem(`control_defaults_${key}`, String(value));
            }
        }

        // Populate control inputs with localStorage values (preferred) or defaults
        function populateControlDefaults() {
            // Email Controls
            const processAllFoldersCheckbox = document.getElementById('process-all-folders');
            const newOnlyOption = document.getElementById('new-only-option');
            if (processAllFoldersCheckbox) {
                const value = getControlValue('process_all_folders', controlDefaults.process_all_folders);
                if (value !== undefined && value !== null) {
                    processAllFoldersCheckbox.checked = value === true || value === 'true';
                }
            }
            if (newOnlyOption) {
                const value = getControlValue('new_only_option', controlDefaults.new_only_option);
                if (value !== undefined && value !== null) {
                    newOnlyOption.checked = value === true || value === 'true';
                }
            }

            // WhatsApp Import
            const whatsappImportDirectory = document.getElementById('whatsapp-import-directory');
            if (whatsappImportDirectory) {
                const value = getControlValue('whatsapp_import_directory', controlDefaults.whatsapp_import_directory);
                if (value) {
                    whatsappImportDirectory.value = value;
                }
            }

            // Facebook Messenger Import
            const facebookImportDirectory = document.getElementById('facebook-import-directory');
            const facebookUserName = document.getElementById('facebook-user-name');
            if (facebookImportDirectory) {
                const value = getControlValue('facebook_import_directory', controlDefaults.facebook_import_directory);
                if (value) {
                    facebookImportDirectory.value = value;
                }
            }
            if (facebookUserName) {
                const value = getControlValue('facebook_user_name', controlDefaults.facebook_user_name);
                if (value) {
                    facebookUserName.value = value;
                }
            }

            // Instagram Import
            const instagramImportDirectory = document.getElementById('instagram-import-directory');
            const instagramUserName = document.getElementById('instagram-user-name');
            if (instagramImportDirectory) {
                const value = getControlValue('instagram_import_directory', controlDefaults.instagram_import_directory);
                if (value) {
                    instagramImportDirectory.value = value;
                }
            }
            if (instagramUserName) {
                const value = getControlValue('instagram_user_name', controlDefaults.instagram_user_name);
                if (value) {
                    instagramUserName.value = value;
                }
            }

            // iMessage Import
            const imessageDirectoryPath = document.getElementById('imessage-directory-path');
            if (imessageDirectoryPath) {
                const value = getControlValue('imessage_directory_path', controlDefaults.imessage_directory_path);
                if (value) {
                    imessageDirectoryPath.value = value;
                }
            }

            // Facebook Albums Import
            const facebookAlbumsImportDirectory = document.getElementById('facebook-albums-import-directory');
            if (facebookAlbumsImportDirectory) {
                const value = getControlValue('facebook_albums_import_directory', controlDefaults.facebook_albums_import_directory);
                if (value) {
                    facebookAlbumsImportDirectory.value = value;
                }
            }

            // Filesystem Image Import
            const filesystemImportDirectory = document.getElementById('filesystem-import-directory');
            const filesystemImportMaxImages = document.getElementById('filesystem-import-max-images');
            if (filesystemImportDirectory) {
                const value = getControlValue('filesystem_import_directory', controlDefaults.filesystem_import_directory);
                if (value) {
                    filesystemImportDirectory.value = value;
                }
            }
            if (filesystemImportMaxImages) {
                const value = getControlValue('filesystem_import_max_images', controlDefaults.filesystem_import_max_images);
                if (value) {
                    filesystemImportMaxImages.value = value;
                }
            }
        }

        // Setup event listeners to save changes to localStorage
        function setupControlDefaultsListeners() {
            // Email Controls
            const processAllFoldersCheckbox = document.getElementById('process-all-folders');
            const newOnlyOption = document.getElementById('new-only-option');
            if (processAllFoldersCheckbox) {
                processAllFoldersCheckbox.addEventListener('change', (e) => {
                    saveControlValue('process_all_folders', e.target.checked);
                });
            }
            if (newOnlyOption) {
                newOnlyOption.addEventListener('change', (e) => {
                    saveControlValue('new_only_option', e.target.checked);
                });
            }

            // WhatsApp Import
            const whatsappImportDirectory = document.getElementById('whatsapp-import-directory');
            if (whatsappImportDirectory) {
                whatsappImportDirectory.addEventListener('change', (e) => {
                    saveControlValue('whatsapp_import_directory', e.target.value);
                });
                whatsappImportDirectory.addEventListener('blur', (e) => {
                    saveControlValue('whatsapp_import_directory', e.target.value);
                });
            }

            // Facebook Messenger Import
            const facebookImportDirectory = document.getElementById('facebook-import-directory');
            const facebookUserName = document.getElementById('facebook-user-name');
            if (facebookImportDirectory) {
                facebookImportDirectory.addEventListener('change', (e) => {
                    saveControlValue('facebook_import_directory', e.target.value);
                });
                facebookImportDirectory.addEventListener('blur', (e) => {
                    saveControlValue('facebook_import_directory', e.target.value);
                });
            }
            if (facebookUserName) {
                facebookUserName.addEventListener('change', (e) => {
                    saveControlValue('facebook_user_name', e.target.value);
                });
                facebookUserName.addEventListener('blur', (e) => {
                    saveControlValue('facebook_user_name', e.target.value);
                });
            }

            // Instagram Import
            const instagramImportDirectory = document.getElementById('instagram-import-directory');
            const instagramUserName = document.getElementById('instagram-user-name');
            if (instagramImportDirectory) {
                instagramImportDirectory.addEventListener('change', (e) => {
                    saveControlValue('instagram_import_directory', e.target.value);
                });
                instagramImportDirectory.addEventListener('blur', (e) => {
                    saveControlValue('instagram_import_directory', e.target.value);
                });
            }
            if (instagramUserName) {
                instagramUserName.addEventListener('change', (e) => {
                    saveControlValue('instagram_user_name', e.target.value);
                });
                instagramUserName.addEventListener('blur', (e) => {
                    saveControlValue('instagram_user_name', e.target.value);
                });
            }

            // iMessage Import
            const imessageDirectoryPath = document.getElementById('imessage-directory-path');
            if (imessageDirectoryPath) {
                imessageDirectoryPath.addEventListener('change', (e) => {
                    saveControlValue('imessage_directory_path', e.target.value);
                });
                imessageDirectoryPath.addEventListener('blur', (e) => {
                    saveControlValue('imessage_directory_path', e.target.value);
                });
            }

            // Facebook Albums Import
            const facebookAlbumsImportDirectory = document.getElementById('facebook-albums-import-directory');
            if (facebookAlbumsImportDirectory) {
                facebookAlbumsImportDirectory.addEventListener('change', (e) => {
                    saveControlValue('facebook_albums_import_directory', e.target.value);
                });
                facebookAlbumsImportDirectory.addEventListener('blur', (e) => {
                    saveControlValue('facebook_albums_import_directory', e.target.value);
                });
            }

            // Filesystem Image Import
            const filesystemImportDirectory = document.getElementById('filesystem-import-directory');
            const filesystemImportMaxImages = document.getElementById('filesystem-import-max-images');
            if (filesystemImportDirectory) {
                filesystemImportDirectory.addEventListener('change', (e) => {
                    saveControlValue('filesystem_import_directory', e.target.value);
                });
                filesystemImportDirectory.addEventListener('blur', (e) => {
                    saveControlValue('filesystem_import_directory', e.target.value);
                });
            }
            if (filesystemImportMaxImages) {
                filesystemImportMaxImages.addEventListener('change', (e) => {
                    saveControlValue('filesystem_import_max_images', e.target.value);
                });
                filesystemImportMaxImages.addEventListener('blur', (e) => {
                    saveControlValue('filesystem_import_max_images', e.target.value);
                });
            }
        }

        // Load defaults when config page opens
        const configBtn = document.getElementById('config-btn');
        if (configBtn) {
            configBtn.addEventListener('click', () => {
                loadControlDefaults();
            });
        }

        // Config tab switching
        const configTabButtons = document.querySelectorAll('.config-tab-button');
        const configTabContents = document.querySelectorAll('.config-tab-content');
        
        configTabButtons.forEach(button => {
            button.addEventListener('click', () => {
                if (button.classList.contains('config-sidebar-requires-master')) {
                    const st = window.getComputedStyle(button);
                    if (st.display === 'none' || st.visibility === 'hidden') return;
                }
                const targetTab = button.getAttribute('data-tab');
                
                // Remove active class from all buttons and contents
                configTabButtons.forEach(btn => btn.classList.remove('active'));
                configTabContents.forEach(content => content.classList.remove('active'));
                
                // Add active class to clicked button and corresponding content
                button.classList.add('active');
                const targetContent = document.getElementById(`${targetTab}-tab`);
                    if (targetContent) {
                    targetContent.classList.add('active');
                }
                
                // Load control defaults when any control tab is opened (if not already loaded)
                const controlTabs = ['manage-imported-data'];
                if (controlTabs.includes(targetTab) && Object.keys(controlDefaults).length === 0) {
                    loadControlDefaults();
                } else if (controlTabs.includes(targetTab)) {
                    // If defaults already loaded, just populate (in case elements weren't ready before)
                    populateControlDefaults();
                }
                // Load last run times when Import Controls tab is opened
                if (controlTabs.includes(targetTab)) {
                    loadImportControlLastRun();
                }
                // Load email matches and classifications when Manage Contacts tab is opened
                if (targetTab === 'manage-contacts') {
                    if (Modals.EmailMatches && Modals.EmailMatches.load) Modals.EmailMatches.load();
                    if (Modals.EmailClassifications && Modals.EmailClassifications.load) Modals.EmailClassifications.load();
                    if (Modals.EmailExclusions && Modals.EmailExclusions.load) Modals.EmailExclusions.load();
                }
                // Load configuration when Settings tab is opened (Application Configuration is in Settings)
                if (targetTab === 'settings') {
                    if (Modals.AppConfig && Modals.AppConfig.load) Modals.AppConfig.load();
                }
                // Load subject configuration when Subject Configuration tab is opened
                if (targetTab === 'subject-configuration') {
                    if (Modals.SubjectConfiguration && Modals.SubjectConfiguration.loadAndPopulateForm) {
                        Modals.SubjectConfiguration.loadAndPopulateForm();
                    }
                }
                if (targetTab === 'interests') {
                    if (Modals.Interests && Modals.Interests.load) Modals.Interests.load();
                }
                if (targetTab === 'custom-voices') {
                    if (Modals.CustomVoices && Modals.CustomVoices.load) Modals.CustomVoices.load();
                }
                if (targetTab === 'tools-access') {
                    if (Modals.LLMToolsAccess && Modals.LLMToolsAccess.load) void Modals.LLMToolsAccess.load();
                }
                // Load system instructions when System Instructions tab is opened
                if (targetTab === 'system-instructions') {
                    if (Modals.SubjectConfiguration && Modals.SubjectConfiguration.loadAndPopulateForm) {
                        Modals.SubjectConfiguration.loadAndPopulateForm();
                    }
                }
            });
        });

        // Manage Contacts inner tabs: switch table (Email Matches / Exclusions / Classifications)
        document.querySelectorAll('.manage-contacts-tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const tabName = btn.getAttribute('data-manage-contacts-tab');
                if (!tabName) return;
                const container = document.getElementById('manage-contacts-tab');
                if (!container) return;
                container.querySelectorAll('.manage-contacts-tab-btn').forEach(b => b.classList.remove('active'));
                container.querySelectorAll('.manage-contacts-tab-content').forEach(c => c.classList.remove('active'));
                btn.classList.add('active');
                const content = document.getElementById(`${tabName}-tab-content`);
                if (content) content.classList.add('active');
            });
        });

        // Dashboard: load stats and render. prefix: '' for config modal, 'stats-' for Statistics modal
        async function loadDashboard(prefix) {
            prefix = prefix || '';
            const container = document.getElementById(prefix + 'dashboard-stats');
            const loadingEl = document.getElementById(prefix + 'dashboard-loading');
            const errorEl = document.getElementById(prefix + 'dashboard-load-error');
            if (!container) return;
            if (loadingEl) loadingEl.style.display = 'inline';
            if (errorEl) { errorEl.style.display = 'none'; errorEl.textContent = ''; }
            try {
                const response = await fetch('/api/dashboard');
                if (!response.ok) throw new Error(`HTTP ${response.status}`);
                const data = await response.json();
                if (loadingEl) loadingEl.style.display = 'none';
                renderDashboardStats(container, data);
                renderDashboardMessagesByYearChart(data, prefix);
                renderDashboardEmailsByYearChart(data, prefix);
                renderDashboardContactsByCategoryChart(data, prefix);
                renderDashboardImagesByRegionChart(data, prefix);
                renderDashboardMessagesByContactChart(data, prefix);
            } catch (err) {
                if (loadingEl) loadingEl.style.display = 'none';
                if (errorEl) {
                    errorEl.style.display = 'block';
                    errorEl.textContent = err.message || 'Failed to load dashboard';
                }
                container.innerHTML = '';
                const yearChart = document.getElementById(prefix + 'dashboard-messages-by-year-chart');
                const emailsYearChart = document.getElementById(prefix + 'dashboard-emails-by-year-chart');
                const contactsCatChart = document.getElementById(prefix + 'dashboard-contacts-by-category-chart');
                const imagesRegionChart = document.getElementById(prefix + 'dashboard-images-by-region-chart');
                const contactChart = document.getElementById(prefix + 'dashboard-messages-by-contact-chart');
                if (yearChart) yearChart.innerHTML = '';
                if (emailsYearChart) emailsYearChart.innerHTML = '';
                if (contactsCatChart) contactsCatChart.innerHTML = '';
                if (imagesRegionChart) imagesRegionChart.innerHTML = '';
                if (contactChart) contactChart.innerHTML = '';
            }
        }

        function renderDashboardMessagesByYearChart(data, prefix) {
            prefix = prefix || '';
            const chartEl = document.getElementById(prefix + 'dashboard-messages-by-year-chart');
            if (!chartEl) return;
            const byYear = data.messages_by_year || {};
            const years = Object.keys(byYear).map(Number).sort((a, b) => a - b);
            if (years.length === 0) {
                chartEl.innerHTML = '<h4 style="margin:0 0 8px 0; font-size:14px; color:#64748b;">Messages by Year</h4><p style="color:#94a3b8; font-size:13px;">No message data by year</p>';
                return;
            }
            const maxCount = Math.max(...years.map(y => byYear[y]), 1);
            const barHeight = 28;
            const bars = years.map(y => {
                const count = byYear[y];
                const pct = Math.round((count / maxCount) * 100);
                return `<div style="display:flex; align-items:center; gap:12px; margin-bottom:8px;">
                    <span style="width:48px; font-weight:600; color:#334155;">${y}</span>
                    <div style="flex:1; height:${barHeight}px; background:#e2e8f0; border-radius:4px; overflow:hidden; position:relative;">
                        <div style="height:100%; width:${pct}%; background:#3b82f6; border-radius:4px; transition:width 0.3s;"></div>
                    </div>
                    <span style="width:72px; text-align:right; font-size:13px; color:#64748b;">${count.toLocaleString()}</span>
                </div>`;
            }).join('');
            chartEl.innerHTML = '<h4 style="margin:0 0 12px 0; font-size:14px; color:#64748b;">Messages by Year</h4>' + bars;
        }

        function renderDashboardEmailsByYearChart(data, prefix) {
            prefix = prefix || '';
            const chartEl = document.getElementById(prefix + 'dashboard-emails-by-year-chart');
            if (!chartEl) return;
            const byYear = data.emails_by_year || {};
            const years = Object.keys(byYear).map(Number).sort((a, b) => a - b);
            if (years.length === 0) {
                chartEl.innerHTML = '<h4 style="margin:0 0 8px 0; font-size:14px; color:#64748b;">Emails by Year</h4><p style="color:#94a3b8; font-size:13px;">No email data by year</p>';
                return;
            }
            const maxCount = Math.max(...years.map(y => byYear[y]), 1);
            const barHeight = 28;
            const barColor = '#60a5fa';
            const bars = years.map(y => {
                const count = byYear[y];
                const pct = Math.round((count / maxCount) * 100);
                return `<div style="display:flex; align-items:center; gap:12px; margin-bottom:8px;">
                    <span style="width:48px; font-weight:600; color:#334155;">${y}</span>
                    <div style="flex:1; height:${barHeight}px; background:#e2e8f0; border-radius:4px; overflow:hidden; position:relative;">
                        <div style="height:100%; width:${pct}%; background:${barColor}; border-radius:4px; transition:width 0.3s;"></div>
                    </div>
                    <span style="width:72px; text-align:right; font-size:13px; color:#64748b;">${count.toLocaleString()}</span>
                </div>`;
            }).join('');
            chartEl.innerHTML = '<h4 style="margin:0 0 12px 0; font-size:14px; color:#64748b;">Emails by Year</h4>' + bars;
        }

        function renderDashboardMessagesByContactChart(data, prefix) {
            prefix = prefix || '';
            const chartEl = document.getElementById(prefix + 'dashboard-messages-by-contact-chart');
            if (!chartEl) return;
            const items = data.messages_by_contact || [];
            if (items.length === 0) {
                chartEl.innerHTML = '<h4 style="margin:0 0 8px 0; font-size:14px; color:#64748b;">Messages by Contact (Top 20)</h4><p style="color:#94a3b8; font-size:13px;">No message data by contact</p>';
                return;
            }
            const maxCount = Math.max(...items.map(i => i.count), 1);
            const barHeight = 28;
            const barColor = '#22c55e';
            const bars = items.map(i => {
                const pct = Math.round((i.count / maxCount) * 100);
                const name = (i.name || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                return `<div style="display:flex; align-items:center; gap:12px; margin-bottom:8px;">
                    <span style="min-width:0; flex:1; max-width:200px; font-weight:600; color:#334155; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${name}">${name}</span>
                    <div style="flex:1; min-width:80px; height:${barHeight}px; background:#e2e8f0; border-radius:4px; overflow:hidden; position:relative;">
                        <div style="height:100%; width:${pct}%; background:${barColor}; border-radius:4px; transition:width 0.3s;"></div>
                    </div>
                    <span style="width:72px; text-align:right; font-size:13px; color:#64748b;">${i.count.toLocaleString()}</span>
                </div>`;
            }).join('');
            chartEl.innerHTML = '<h4 style="margin:0 0 12px 0; font-size:14px; color:#64748b;">Messages by Contact (Top 20)</h4>' + bars;
        }

        function renderDashboardContactsByCategoryChart(data, prefix) {
            prefix = prefix || '';
            const chartEl = document.getElementById(prefix + 'dashboard-contacts-by-category-chart');
            if (!chartEl) return;
            const byCat = data.contacts_by_category || {};
            const unknownCount = byCat['unknown'] ?? byCat['Unknown'] ?? 0;
            const categories = Object.entries(byCat)
                .filter(([k]) => (k || '').toLowerCase() !== 'unknown')
                .sort((a, b) => b[1] - a[1]);
            const barColor = '#8b5cf6';
            let html = '<h4 style="margin:0 0 12px 0; font-size:14px; color:#64748b;">Contacts by Category</h4>';
            if (categories.length === 0 && unknownCount === 0) {
                html += '<p style="color:#94a3b8; font-size:13px;">No contact data by category</p>';
            } else {
                const maxCount = categories.length ? Math.max(...categories.map(([, n]) => n), 1) : 1;
                const barHeight = 28;
                const bars = categories.map(([name, count]) => {
                    const pct = Math.round((count / maxCount) * 100);
                    const safeName = (name || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    return `<div style="display:flex; align-items:center; gap:12px; margin-bottom:8px;">
                        <span style="min-width:0; flex:1; max-width:180px; font-weight:600; color:#334155; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${safeName}">${safeName}</span>
                        <div style="flex:1; min-width:80px; height:${barHeight}px; background:#e2e8f0; border-radius:4px; overflow:hidden; position:relative;">
                            <div style="height:100%; width:${pct}%; background:${barColor}; border-radius:4px; transition:width 0.3s;"></div>
                        </div>
                        <span style="width:72px; text-align:right; font-size:13px; color:#64748b;">${count.toLocaleString()}</span>
                    </div>`;
                }).join('');
                html += bars;
                if (unknownCount > 0) {
                    html += `<p style="margin:12px 0 0 0; font-size:13px; color:#64748b;">Unknown: ${unknownCount.toLocaleString()} contacts</p>`;
                }
            }
            chartEl.innerHTML = html;
        }

        const REGION_NAME_MAP = {
            eur: 'Europe', dxb: 'Dubai', af: 'Africa', me: 'Middle East',
            aus: 'Australia', asia: 'Asia', usa: 'USA', south_america: 'South America',
            oth: 'Other', carribean: 'Caribbean', nz: 'New Zealand'
        };
        function regionDisplayName(key) {
            if (key == null || key === '') return 'Unknown';
            const k = String(key).toLowerCase().trim();
            return REGION_NAME_MAP[k] ?? key;
        }

        function renderDashboardImagesByRegionChart(data, prefix) {
            prefix = prefix || '';
            const chartEl = document.getElementById(prefix + 'dashboard-images-by-region-chart');
            if (!chartEl) return;
            const byRegion = data.images_by_region || {};
            const unknownCount = byRegion['Unknown'] ?? byRegion['unknown'] ?? 0;
            const regions = Object.entries(byRegion)
                .filter(([k]) => (k || '').toLowerCase() !== 'unknown')
                .sort((a, b) => b[1] - a[1]);
            const barColor = '#0ea5e9';
            let html = '<h4 style="margin:0 0 12px 0; font-size:14px; color:#64748b;">Images by Region</h4>';
            if (regions.length === 0 && unknownCount === 0) {
                html += '<p style="color:#94a3b8; font-size:13px;">No image data by region</p>';
            } else {
                const maxCount = regions.length ? Math.max(...regions.map(([, n]) => n), 1) : 1;
                const barHeight = 28;
                const bars = regions.map(([name, count]) => {
                    const pct = Math.round((count / maxCount) * 100);
                    const displayName = regionDisplayName(name);
                    const safeName = displayName.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    return `<div style="display:flex; align-items:center; gap:12px; margin-bottom:8px;">
                        <span style="min-width:0; flex:1; max-width:180px; font-weight:600; color:#334155; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;" title="${safeName}">${safeName}</span>
                        <div style="flex:1; min-width:80px; height:${barHeight}px; background:#e2e8f0; border-radius:4px; overflow:hidden; position:relative;">
                            <div style="height:100%; width:${pct}%; background:${barColor}; border-radius:4px; transition:width 0.3s;"></div>
                        </div>
                        <span style="width:72px; text-align:right; font-size:13px; color:#64748b;">${count.toLocaleString()}</span>
                    </div>`;
                }).join('');
                html += bars;
                if (unknownCount > 0) {
                    html += `<p style="margin:12px 0 0 0; font-size:13px; color:#64748b;">Unknown: ${unknownCount.toLocaleString()} images</p>`;
                }
            }
            chartEl.innerHTML = html;
        }

        function renderDashboardStats(container, data) {
            const cards = [];
            const add = (label, value, num) => cards.push({ label, value: value ?? 0, num: num ?? (typeof value === 'number' ? value : null) });
            add('Total Messages', data.total_messages);
            add('Total Emails', data.emails_count);
            const counts = data.message_counts || {};
            Object.entries(counts).sort((a,b)=>b[1]-a[1]).forEach(([service, n]) => {
                add(`Messages (${service})`, n);
            });
            add('Contacts', data.contacts_count);
            add('Images (total)', data.total_images);
            add('Images (imported)', data.imported_images);
            add('Images (reference)', data.reference_images);
            const thumbVal = `${data.thumbnail_count || 0} (${data.thumbnail_percentage ?? 0}%)`;
            const thumbPct = data.thumbnail_percentage ?? 0;
            const thumbBorder = thumbPct >= 100 ? '#22c55e' : (thumbPct < 80 ? '#dc3545' : (thumbPct < 95 ? '#eab308' : null));
            cards.push({ label: 'Images with thumbnails', value: thumbVal, num: data.thumbnail_count ?? 0, borderColor: thumbBorder });
            add('Facebook albums', data.facebook_albums_count);
            add('Facebook posts', data.facebook_posts_count);
            add('Locations', data.locations_count);
            add('Places', data.places_count);
            add('Artefacts', data.artefacts_count);
            add('Reference docs (enabled)', data.reference_docs_enabled);
            add('Reference docs (disabled)', data.reference_docs_disabled);
            add('Complete profiles', data.complete_profiles_count);
            const subjName = data.subject_full_name || 'Subject';
            const subjHasProfile = data.subject_has_complete_profile === true;
            cards.push({
                label: subjName,
                value: subjHasProfile ? 'Complete profile: Yes' : 'Complete profile: No',
                num: null,
                borderColor: subjHasProfile ? '#22c55e' : '#dc3545'
            });
            container.innerHTML = cards.map(c => {
                let borderColor = c.borderColor;
                if (borderColor == null) borderColor = (c.num !== null && c.num === 0) ? '#dc3545' : '#3b82f6';
                const displayVal = typeof c.value === 'number' ? c.value.toLocaleString() : c.value;
                return `<div style="padding:12px; background:#f5f7fa; border-radius:8px; border-left:4px solid ${borderColor};">
                    <div style="font-size:12px; color:#64748b; text-transform:uppercase;">${c.label}</div>
                    <div style="font-size:18px; font-weight:600; color:#1e293b; margin-top:4px;">${displayVal}</div>
                </div>`;
            }).join('');
        }


        // Format date/time in local timezone, 24-hour format (dd/mm/yyyy HH:mm)
        function formatImportLastRunLocal(isoString) {
            if (!isoString) return '';
            try {
                const date = new Date(isoString);
                if (isNaN(date.getTime())) return '';
                return new Intl.DateTimeFormat('en-AU', {
                    day: '2-digit',
                    month: '2-digit',
                    year: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit',
                    hour12: false
                }).format(date);
            } catch (e) {
                return '';
            }
        }

        async function loadImportControlLastRun() {
            try {
                const response = await fetch('/api/import-control-last-run');
                if (!response.ok) return;
                const data = await response.json();
                const importTypes = ['email_processing', 'imap_processing', 'whatsapp', 'facebook', 'instagram', 'imessage', 'facebook_albums', 'facebook_posts', 'facebook_all', 'facebook_places', 'filesystem', 'reference_import', 'image_export', 'thumbnails', 'contacts'];
                for (const importType of importTypes) {
                    const els = document.querySelectorAll(`[data-import-last-run="${importType}"]`);
                    const info = data[importType];
                    const formatted = (info && info.last_run_at) ? formatImportLastRunLocal(info.last_run_at) : null;
                    const resultLabel = (info && info.result) ? ((info.result === 'success' || info.result === 'completed') ? 'success' : (info.result === 'cancelled' ? 'cancelled' : 'error')) : null;
                    const text = (formatted && resultLabel) ? `Last run: ${formatted} (${resultLabel})` : '';
                    const title = (info && info.result_message) ? info.result_message : '';
                    els.forEach(el => {
                        el.textContent = text;
                        el.title = title;
                    });
                }
            } catch (e) {
                console.warn('Failed to load import control last run:', e);
            }
        }

        // Empty Media Tables Button
        const emptyMediaTablesBtn = document.getElementById('empty-media-tables-btn');
        const emptyTablesStatus = document.getElementById('empty-tables-status');
        
        if (emptyMediaTablesBtn) {
            emptyMediaTablesBtn.addEventListener('click', async () => {
                // Show confirmation dialog
                const confirmed = confirm(
                    'WARNING: This will permanently delete ALL data from:\n\n' +
                    '- attachments\n' +
                    '- media_blob\n' +
                    '- media_items\n' +
                    '- messages\n' +
                    '- message_attachments\n\n' +
                    'This action cannot be undone!\n\n' +
                    'Are you absolutely sure you want to continue?'
                );
                
                if (!confirmed) {
                    return;
                }
                
                //Double confirmation
                const doubleConfirmed = confirm(
                    'FINAL WARNING: This will DELETE ALL messages and media data.\n\n' 
                );
                
                if (!doubleConfirmed) {
                    return;
                }
                const userInput = "DELETE";
               // const userInput = prompt('Type "DELETE" to confirm:');
                if (userInput !== 'DELETE') {
                    if (emptyTablesStatus) {
                        emptyTablesStatus.style.display = 'block';
                        emptyTablesStatus.style.backgroundColor = '#fff3cd';
                        emptyTablesStatus.style.color = '#856404';
                        emptyTablesStatus.style.border = '1px solid #ffc107';
                        emptyTablesStatus.textContent = 'Operation cancelled. Tables were not emptied.';
                    }
                    return;
                }
                
                // Disable button and show loading
                emptyMediaTablesBtn.disabled = true;
                emptyMediaTablesBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Emptying tables...';
                
                if (emptyTablesStatus) {
                    emptyTablesStatus.style.display = 'block';
                    emptyTablesStatus.style.backgroundColor = '#d1ecf1';
                    emptyTablesStatus.style.color = '#0c5460';
                    emptyTablesStatus.style.border = '1px solid #bee5eb';
                    emptyTablesStatus.textContent = 'Emptying tables...';
                }
                
                try {
                    const response = await fetch('/admin/empty-media-tables', {
                        method: 'DELETE'
                    });
                    
                    if (!response.ok) {
                        const errorData = await response.json().catch(() => ({ detail: 'Unknown error' }));
                        throw new Error(errorData.detail || `HTTP ${response.status}`);
                    }
                    
                    const result = await response.json();
                    
                    // Show success message
                    if (emptyTablesStatus) {
                        emptyTablesStatus.style.backgroundColor = '#d4edda';
                        emptyTablesStatus.style.color = '#155724';
                        emptyTablesStatus.style.border = '1px solid #c3e6cb';
                        
                        const counts = result.deleted_counts || {};
                        emptyTablesStatus.innerHTML = `
                            <strong>Tables emptied successfully!</strong><br>
                            Deleted counts:<br>
                            • Messages: ${counts.messages || 0}<br>
                            • Message Attachments: ${counts.message_attachments || 0}<br>
                            • Media Items: ${counts.media_items || 0}<br>
                            • Media Blobs: ${counts.media_blob || 0}<br>
                            • Attachments: ${counts.attachments || 0}<br>
                            • Facebook Album Images: ${counts.facebook_album_images || 0}<br>
                            • Facebook Albums: ${counts.facebook_albums || 0}<br>
                        `;
                    }
                    
                    // Re-enable button
                    emptyMediaTablesBtn.disabled = false;
                    emptyMediaTablesBtn.innerHTML = '<i class="fas fa-trash-alt"></i> Empty Media and Message Tables';
                    
                } catch (error) {
                    console.error('Error emptying tables:', error);
                    
                    if (emptyTablesStatus) {
                        emptyTablesStatus.style.backgroundColor = '#f8d7da';
                        emptyTablesStatus.style.color = '#721c24';
                        emptyTablesStatus.style.border = '1px solid #f5c6cb';
                        emptyTablesStatus.textContent = `Error: ${error.message}`;
                    }
                    
                    // Re-enable button
                    emptyMediaTablesBtn.disabled = false;
                    emptyMediaTablesBtn.innerHTML = '<i class="fas fa-trash-alt"></i> Empty Media and Message Tables';
                }
            });
        }

        // Email Processing Controls
        const processAllFoldersCheckbox = document.getElementById('process-all-folders');
        const folderSelect = document.getElementById('folder-select');
        const folderSelectionGroup = document.getElementById('folder-selection-group');
        const newOnlyOption = document.getElementById('new-only-option');
        const startProcessingBtn = document.getElementById('start-processing-btn');
        const cancelProcessingBtn = document.getElementById('cancel-processing-btn');
        const processingStatus = document.getElementById('processing-status');
        const processingStatusMessage = document.getElementById('processing-status-message');
        const processingStatusDetails = document.getElementById('processing-status-details');
        const processingProgressContainer = document.getElementById('processing-progress-container');
        const currentLabelName = document.getElementById('current-label-name');
        const labelProgressText = document.getElementById('label-progress-text');
        const processingProgressBar = document.getElementById('processing-progress-bar');
        const progressBarText = document.getElementById('progress-bar-text');
        const emailsProcessedCount = document.getElementById('emails-processed-count');
        let eventSource = null;

        // Toggle folder selection based on "Process All" checkbox
        if (processAllFoldersCheckbox) {
            // Set initial state
            if (processAllFoldersCheckbox.checked) {
                folderSelectionGroup.style.display = 'none';
            }
            
            processAllFoldersCheckbox.addEventListener('change', (e) => {
                if (e.target.checked) {
                    folderSelectionGroup.style.display = 'none';
                } else {
                    folderSelectionGroup.style.display = 'block';
                }
            });
        }

        // Load folders from API
        async function loadFolders() {
            if (!folderSelect) return;
            
            try {
                const response = await fetch('/emails/folders');
                if (!response.ok) {
                    throw new Error(`Failed to load folders: ${response.statusText}`);
                }
                const folders = await response.json();
                
                folderSelect.innerHTML = '';
                folders.forEach(folder => {
                    const option = document.createElement('option');
                    option.value = folder.name;
                    option.textContent = folder.name;
                    folderSelect.appendChild(option);
                });
            } catch (error) {
                folderSelect.innerHTML = '<option value="">Error loading folders</option>';
                showProcessingStatus('error', 'Failed to load folders', error.message);
            }
        }

        // Close SSE connection if open
        function closeEventSource() {
            if (eventSource) {
                eventSource.close();
                eventSource = null;
            }
        }

        // Request browser notification permission
        async function requestNotificationPermission() {
            if ('Notification' in window && Notification.permission === 'default') {
                await Notification.requestPermission();
            }
        }

        // Show browser notification
        function showNotification(title, body, icon = null) {
            if ('Notification' in window && Notification.permission === 'granted') {
                new Notification(title, {
                    body: body,
                    icon: icon || '/static/images/expert.png',
                    tag: 'email-processing'
                });
            }
        }

        // Update progress display
        function updateProgressDisplay(progressData) {
            if (!processingProgressContainer) return;

            const {
                current_label,
                current_label_index,
                total_labels,
                emails_processed,
                status
            } = progressData;

            // Show progress container when processing starts
            if (status === 'in_progress') {
                processingProgressContainer.style.display = 'block';
            }

            // Update current label
            if (currentLabelName) {
                currentLabelName.textContent = current_label || 'Waiting...';
            }

            // Update label progress
            if (labelProgressText) {
                labelProgressText.textContent = `${current_label_index} / ${total_labels}`;
            }

            // Update progress bar
            if (total_labels > 0 && processingProgressBar && progressBarText) {
                const percentage = Math.round((current_label_index / total_labels) * 100);
                processingProgressBar.style.width = `${percentage}%`;
                progressBarText.textContent = `${percentage}%`;
            }

            // Update emails processed count
            if (emailsProcessedCount) {
                emailsProcessedCount.textContent = emails_processed || 0;
            }
        }

        // Connect to SSE stream
        function connectToProgressStream() {
            // Close existing connection if any
            closeEventSource();

            // Request notification permission
            requestNotificationPermission();

            // Create EventSource connection
            eventSource = new EventSource('/emails/process/stream');

            eventSource.onmessage = (event) => {
                try {
                    const eventData = JSON.parse(event.data);
                    handleProgressEvent(eventData);
                } catch (error) {
                    console.error('Error parsing SSE event:', error);
                }
            };

            eventSource.onerror = (error) => {
                console.error('SSE connection error:', error);
                // Don't close on error - EventSource will attempt to reconnect
            };

            // Clean up on page unload
            window.addEventListener('beforeunload', () => {
                closeEventSource();
            });
        }

        // Handle progress events from SSE
        function handleProgressEvent(eventData) {
            const { type, data } = eventData;

            switch (type) {
                case 'progress':
                    updateProgressDisplay(data);
                    if (data.status === 'in_progress') {
                        cancelProcessingBtn.style.display = 'inline-block';
                        startProcessingBtn.disabled = true;
                        showProcessingStatus('info', 'Processing in progress...', `Processing label ${data.current_label_index} of ${data.total_labels}`);
                    }
                    break;

                case 'completed':
                    updateProgressDisplay(data);
                    cancelProcessingBtn.style.display = 'none';
                    startProcessingBtn.disabled = false;
                    showProcessingStatus('success', 'Processing completed', `Successfully processed ${data.emails_processed} emails from ${data.total_labels} label(s).`);
                    showNotification('Email Processing Complete', `Processed ${data.emails_processed} emails from ${data.total_labels} label(s).`);
                    closeEventSource();
                    break;

                case 'error':
                    updateProgressDisplay(data);
                    cancelProcessingBtn.style.display = 'none';
                    startProcessingBtn.disabled = false;
                    showProcessingStatus('error', 'Processing error', data.error_message || 'An error occurred during processing.');
                    showNotification('Email Processing Error', data.error_message || 'An error occurred during processing.');
                    closeEventSource();
                    break;

                case 'cancelled':
                    updateProgressDisplay(data);
                    cancelProcessingBtn.style.display = 'none';
                    startProcessingBtn.disabled = false;
                    showProcessingStatus('info', 'Processing cancelled', data.error_message || 'Processing was cancelled.');
                    showNotification('Email Processing Cancelled', 'Processing was cancelled by user.');
                    closeEventSource();
                    break;

                case 'heartbeat':
                    // Keep connection alive - no UI update needed
                    break;

                default:
                    console.log('Unknown event type:', type);
            }
        }

        // Check initial processing status
        async function checkInitialStatus() {
            if (!processingStatus) return;
            
            try {
                const response = await fetch('/emails/process/status');
                if (!response.ok) {
                    return;
                }
                const status = await response.json();
                
                if (status.in_progress) {
                    cancelProcessingBtn.style.display = 'inline-block';
                    startProcessingBtn.disabled = true;
                    // Connect to stream to get updates
                    connectToProgressStream();
                } else {
                    cancelProcessingBtn.style.display = 'none';
                    startProcessingBtn.disabled = false;
                }
            } catch (error) {
                console.error('Error checking initial status:', error);
            }
        }

        // Show processing status message
        function showProcessingStatus(type, message, details = '') {
            if (!processingStatus) return;
            
            // Remove all status classes
            processingStatus.classList.remove('success', 'error', 'info');
            // Add the new status class
            processingStatus.classList.add(type);
            processingStatus.style.display = 'block';
            processingStatusMessage.textContent = message;
            processingStatusDetails.textContent = details;
        }

        // Start processing
        if (startProcessingBtn) {
            startProcessingBtn.addEventListener('click', async () => {
                const allFolders = processAllFoldersCheckbox?.checked || false;
                const newOnly = newOnlyOption?.checked || false;
                let labels = null;
                
                if (!allFolders) {
                    const selectedOptions = Array.from(folderSelect?.selectedOptions || []);
                    labels = selectedOptions.map(opt => opt.value);
                    
                    if (labels.length === 0) {
                        showProcessingStatus('error', 'No folders selected', 'Please select at least one folder or check "Process All Folders".');
                        return;
                    }
                }
                
                const requestBody = {
                    all_folders: allFolders,
                    new_only: newOnly,
                    labels: labels
                };
                
                try {
                    startProcessingBtn.disabled = true;
                    showProcessingStatus('info', 'Starting processing...', 'Sending request to server...');
                    
                    const response = await fetch('/emails/process', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify(requestBody)
                    });
                    
                    const result = await response.json();
                    
                    if (response.ok) {
                        showProcessingStatus('info', 'Processing started', result.message || 'Email processing has been initiated.');
                        cancelProcessingBtn.style.display = 'inline-block';
                        
                        // Connect to SSE stream for real-time updates
                        connectToProgressStream();
                    } else {
                        showProcessingStatus('error', 'Failed to start processing', result.detail || 'An error occurred while starting processing.');
                        startProcessingBtn.disabled = false;
                    }
                } catch (error) {
                    showProcessingStatus('error', 'Error starting processing', error.message);
                    startProcessingBtn.disabled = false;
                }
            });
        }

        // Cancel processing
        if (cancelProcessingBtn) {
            cancelProcessingBtn.addEventListener('click', async () => {
                try {
                    cancelProcessingBtn.disabled = true;
                    showProcessingStatus('info', 'Cancelling processing...', 'Sending cancellation request...');
                    
                    const response = await fetch('/emails/process/cancel', {
                        method: 'POST'
                    });
                    
                    const result = await response.json();
                    
                    if (result.cancelled) {
                        showProcessingStatus('info', 'Cancellation requested', result.message || 'Processing cancellation has been requested.');
                        // The SSE stream will send the cancelled event
                    } else {
                        showProcessingStatus('info', 'No processing in progress', result.message || 'No email processing is currently in progress.');
                        closeEventSource();
                    }
                } catch (error) {
                    showProcessingStatus('error', 'Error cancelling processing', error.message);
                } finally {
                    cancelProcessingBtn.disabled = false;
                }
            });
        }

        // Unified Import Controls (table layout, modal inputs, shared status box in both tabs)
        const importStatusTextEls = () => document.querySelectorAll('.import-controls-status-text');
        const importCancelBtns = () => document.querySelectorAll('.import-controls-cancel-btn');
        const importInputModal = document.getElementById('import-input-modal');
        const importInputModalTitle = document.getElementById('import-input-modal-title');
        const importInputModalBody = document.getElementById('import-input-modal-body');
        const importInputModalCancel = document.getElementById('import-input-modal-cancel');
        const importInputModalSubmit = document.getElementById('import-input-modal-submit');

        let importInProgress = false;
        let currentImportType = null;
        let currentEventSource = null;
        let importCompletionWaiter = null;
        let masterImportQueueRunning = false;
        let masterImportBatchAbort = false;
        let lastMasterBatchImportType = null;
        let masterImportConfirmPendingJobs = null;

        const MASTER_IMPORT_TYPES = [
            'email_processing',
            'imap_processing',
            'imessage',
            'whatsapp',
            'facebook_all',
            'instagram',
            'filesystem',
            'thumbnails'
        ];

        const MASTER_IMPORT_META = {
            email_processing: { title: 'Emails from Gmail', icon: 'fas fa-envelope' },
            imap_processing: { title: 'Emails from other IMAP', icon: 'fas fa-inbox' },
            imessage: { title: 'iMessage and SMS', icon: 'fas fa-comment-dots' },
            whatsapp: { title: 'WhatsApp Messages', icon: 'fab fa-whatsapp' },
            facebook_all: { title: 'Facebook Archive', icon: 'fab fa-facebook' },
            instagram: { title: 'Instagram Archive', icon: 'fab fa-instagram' },
            filesystem: { title: 'Picture and Images', icon: 'fas fa-images' },
            thumbnails: { title: 'Generate Thumbnails and Location', icon: 'fas fa-image' }
        };

        const GMAIL_FORM_IDS_STANDALONE = {
            authWrap: 'gmail-auth-status-wrap',
            authText: 'gmail-auth-status-text',
            authLink: 'gmail-auth-link',
            importControls: 'gmail-import-controls',
            allLabels: 'import-modal-email-all-labels',
            excludeLabelsWrap: 'import-modal-email-exclude-labels-wrap',
            excludeLabels: 'import-modal-email-exclude-labels',
            labelsWrap: 'import-modal-email-labels-wrap',
            labelsSelect: 'import-modal-email-labels',
            newOnly: 'import-modal-email-new-only'
        };

        const GMAIL_FORM_IDS_MASTER = {
            authWrap: 'mi-gmail-auth-wrap',
            authText: 'mi-gmail-auth-text',
            authLink: 'mi-gmail-auth-link',
            importControls: 'mi-gmail-import-controls',
            allLabels: 'mi-gmail-all-labels',
            excludeLabelsWrap: 'mi-gmail-exclude-labels-wrap',
            excludeLabels: 'mi-gmail-exclude-labels',
            labelsWrap: 'mi-gmail-labels-wrap',
            labelsSelect: 'mi-gmail-labels',
            newOnly: 'mi-gmail-new-only'
        };

        function gmailImportFormHTML(ids) {
            return `
                <div id="${ids.authWrap}" style="margin-bottom: 15px; padding: 10px; border-radius: 6px; background: #f0f4ff; border: 1px solid #c7d4f0; display: flex; align-items: center; justify-content: space-between; gap: 10px;">
                    <span id="${ids.authText}" style="font-size: 0.95em;">Checking Gmail authentication…</span>
                    <a id="${ids.authLink}" href="/gmail/auth/start" style="display: none; padding: 5px 12px; background: #4285f4; color: #fff; border-radius: 4px; text-decoration: none; font-size: 0.9em; font-weight: 500;">Connect Gmail</a>
                </div>
                <div id="${ids.importControls}" style="display: none;">
                    <div class="setting-group" style="margin-bottom: 15px;">
                        <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                            <input type="checkbox" id="${ids.allLabels}" style="cursor: pointer;">
                            <span>Process All Labels</span>
                        </label>
                    </div>
                    <div id="${ids.excludeLabelsWrap}" class="setting-group" style="margin-bottom: 15px; display: none;">
                        <label for="${ids.excludeLabels}" style="display: block; margin-bottom: 5px; font-weight: 500;">Exclude Labels (one regex per line)</label>
                        <textarea id="${ids.excludeLabels}" rows="4" placeholder="e.g.&#10;^Spam$&#10;^Trash.*&#10;.*Promotions.*" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da; font-family: monospace; font-size: 0.9em; resize: vertical; box-sizing: border-box;"></textarea>
                        <small style="color: #666; margin-top: 4px; display: block;">Labels matching any pattern will be skipped.</small>
                    </div>
                    <div id="${ids.labelsWrap}" class="setting-group" style="margin-bottom: 15px;">
                        <label for="${ids.labelsSelect}" style="display: block; margin-bottom: 5px; font-weight: 500;">Select Labels</label>
                        <select id="${ids.labelsSelect}" multiple style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da; min-height: 120px;">
                            <option value="">Loading labels…</option>
                        </select>
                        <small style="color: #666; margin-top: 4px; display: block;">Hold Ctrl/Cmd to select multiple</small>
                    </div>
                    <div class="setting-group" style="margin-bottom: 15px;">
                        <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                            <input type="checkbox" id="${ids.newOnly}" style="cursor: pointer;">
                            <span>New Only (skip already imported emails)</span>
                        </label>
                    </div>
                </div>`;
        }

        function imapImportFormHTML(ids) {
            return `
                <div class="setting-group" style="margin-bottom: 15px;">
                    <label for="${ids.host}" style="display: block; margin-bottom: 5px; font-weight: 500;">IMAP Host</label>
                    <input type="text" id="${ids.host}" placeholder="e.g., imap.outlook.com" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;">
                </div>
                <div class="setting-group" style="margin-bottom: 15px; display: flex; gap: 10px; align-items: flex-end;">
                    <div style="flex: 1;">
                        <label for="${ids.port}" style="display: block; margin-bottom: 5px; font-weight: 500;">Port</label>
                        <input type="number" id="${ids.port}" placeholder="993" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;">
                    </div>
                    <div style="margin-bottom: 9px;">
                        <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                            <input type="checkbox" id="${ids.useSsl}" style="cursor: pointer;">
                            <span>SSL/TLS</span>
                        </label>
                    </div>
                </div>
                <div class="setting-group" style="margin-bottom: 15px;">
                    <label for="${ids.username}" style="display: block; margin-bottom: 5px; font-weight: 500;">Username</label>
                    <input type="text" id="${ids.username}" placeholder="your@email.com" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;">
                </div>
                <div class="setting-group" style="margin-bottom: 15px;">
                    <label for="${ids.password}" style="display: block; margin-bottom: 5px; font-weight: 500;">Password</label>
                    <input type="password" id="${ids.password}" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;">
                </div>
                <div class="setting-group" style="margin-bottom: 15px; display: flex; align-items: center; gap: 10px;">
                    <button type="button" id="${ids.fetchBtn}" style="padding: 6px 14px; border-radius: 4px; border: 1px solid #bfc9da; cursor: pointer; background: #e8f0fe; font-weight: 500;">Fetch Folders</button>
                    <span id="${ids.fetchStatus}" style="font-size: 0.9em; color: #666;"></span>
                </div>
                <div class="setting-group" style="margin-bottom: 15px;">
                    <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                        <input type="checkbox" id="${ids.allFolders}" style="cursor: pointer;">
                        <span>Process All Folders</span>
                    </label>
                </div>
                <div id="${ids.excludeFoldersWrap}" class="setting-group" style="margin-bottom: 15px; display: none;">
                    <label for="${ids.excludeFolders}" style="display: block; margin-bottom: 5px; font-weight: 500;">Exclude Folders (one regex per line)</label>
                    <textarea id="${ids.excludeFolders}" rows="4" placeholder="e.g.&#10;^Spam$&#10;^Trash&#10;.*Junk.*" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da; font-family: monospace; font-size: 0.9em; resize: vertical; box-sizing: border-box;"></textarea>
                    <small style="color: #666; margin-top: 4px; display: block;">Folders matching any pattern will be skipped.</small>
                </div>
                <div id="${ids.foldersWrap}" class="setting-group" style="margin-bottom: 15px; display: none;">
                    <label for="${ids.foldersSelect}" style="display: block; margin-bottom: 5px; font-weight: 500;">Select Folders</label>
                    <select id="${ids.foldersSelect}" multiple style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da; min-height: 120px;"></select>
                    <small style="color: #666; margin-top: 4px; display: block;">Hold Ctrl/Cmd to select multiple</small>
                </div>
                <div class="setting-group" style="margin-bottom: 15px;">
                    <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
                        <input type="checkbox" id="${ids.newOnly}" style="cursor: pointer;">
                        <span>New Only (skip already imported emails)</span>
                    </label>
                </div>`;
        }

        const IMAP_FORM_IDS_STANDALONE = {
            host: 'imap-modal-host',
            port: 'imap-modal-port',
            useSsl: 'imap-modal-use-ssl',
            username: 'imap-modal-username',
            password: 'imap-modal-password',
            fetchBtn: 'imap-modal-fetch-folders',
            fetchStatus: 'imap-modal-fetch-status',
            allFolders: 'imap-modal-all-folders',
            excludeFoldersWrap: 'imap-modal-exclude-folders-wrap',
            excludeFolders: 'imap-modal-exclude-folders',
            foldersWrap: 'imap-modal-folders-wrap',
            foldersSelect: 'imap-modal-folders',
            newOnly: 'imap-modal-new-only'
        };

        const IMAP_FORM_IDS_MASTER = {
            host: 'mi-imap-host',
            port: 'mi-imap-port',
            useSsl: 'mi-imap-use-ssl',
            username: 'mi-imap-username',
            password: 'mi-imap-password',
            fetchBtn: 'mi-imap-fetch-folders',
            fetchStatus: 'mi-imap-fetch-status',
            allFolders: 'mi-imap-all-folders',
            excludeFoldersWrap: 'mi-imap-exclude-folders-wrap',
            excludeFolders: 'mi-imap-exclude-folders',
            foldersWrap: 'mi-imap-folders-wrap',
            foldersSelect: 'mi-imap-folders',
            newOnly: 'mi-imap-new-only'
        };
        const cancelEndpoints = {
            email_processing: '/emails/process/cancel',
            imap_processing: '/imap/process/cancel',
            whatsapp: '/whatsapp/import/cancel',
            facebook: '/facebook/import/cancel',
            instagram: '/instagram/import/cancel',
            imessage: '/imessages/import/cancel',
            facebook_albums: '/facebook/albums/import/cancel',
            facebook_places: '/facebook/import-places/cancel',
            facebook_posts: '/facebook/posts/import/cancel',
            facebook_all: '/facebook/all/import/cancel',
            filesystem: '/images/import/cancel',
            reference_import: '/images/import-reference/cancel',
            image_export: '/images/export/cancel',
            thumbnails: '/images/process-thumbnails/cancel',
            contacts: '/contacts/extract/cancel'
        };

        function setImportStatus(text, isError = false) {
            importStatusTextEls().forEach(el => {
                el.textContent = text || 'Idle';
                el.style.color = isError ? '#dc3545' : '#666';
            });
        }

        function setExecuting(importType, executing) {
            const btns = document.querySelectorAll('.import-execute-btn');
            btns.forEach(btn => {
                const type = btn.getAttribute('data-import');
                if (type === importType) {
                    btn.disabled = executing;
                    btn.innerHTML = executing ? '<i class="fas fa-spinner fa-spin"></i> Executing' : '<i class="fas fa-play"></i> Execute';
                    btn.style.backgroundColor = executing ? '#ffc107' : '';
                    btn.classList.toggle('import-executing', executing);
                } else {
                    btn.disabled = executing;
                    btn.classList.remove('import-executing');
                }
            });
            const tiles = document.querySelectorAll('.import-control-tile');
            tiles.forEach(tile => {
                const type = tile.getAttribute('data-import');
                tile.classList.toggle('import-executing', type === importType && executing);
            });
            importCancelBtns().forEach(btn => { btn.disabled = !executing; });
        }

        function formatProgressLine(importType, data) {
            if (!data) return '';
            if (data.status_line) return data.status_line;
            if (data.error_message) return data.error_message;
            switch (importType) {
                case 'email_processing':
                    return `Label: ${data.current_label || '-'} | ${data.current_label_index || 0}/${data.total_labels || 0} | ${data.emails_processed || 0} emails processed`;
                case 'imap_processing':
                    return `Folder: ${data.current_folder || '-'} | ${data.current_folder_index || 0}/${data.total_folders || 0} | ${data.emails_processed || 0} emails processed`;
                case 'whatsapp':
                case 'facebook':
                case 'instagram':
                case 'imessage':
                    return `Conversation: ${data.current_conversation || '-'} | ${data.conversations_processed || 0}/${data.total_conversations || 0} | ${data.messages_imported || 0} msg (${data.messages_created || 0} new, ${data.messages_updated || 0} updated) | ${data.attachments_found || 0} attachments, ${data.attachments_missing || 0} missing | ${data.errors || 0} errors`;
                case 'facebook_albums':
                    return `Album: ${data.current_album || '-'} | ${data.albums_processed || 0}/${data.total_albums || 0} | ${data.images_imported || 0} imported, ${data.images_found || 0} found, ${data.images_missing || 0} missing | ${data.errors || 0} errors`;
                case 'facebook_places':
                    return data.status_line || `Places: ${data.places_imported || 0} imported`;
                case 'facebook_posts':
                    return `Post: ${data.current_post || '-'} | ${data.posts_processed || 0}/${data.total_posts || 0} | ${data.posts_imported || 0} new, ${data.posts_updated || 0} updated | ${data.with_media || 0} with media, ${data.images_imported || 0} images | ${data.errors || 0} errors`;
                case 'facebook_all':
                    return data.status_line || 'Running...';
                case 'filesystem':
                    return `File: ${data.current_file || '-'} | ${data.files_processed || 0}/${data.total_files || 0} | ${data.images_imported || 0} imported, ${data.images_referenced || 0} referenced, ${data.images_updated || 0} updated | ${data.errors || 0} errors`;
                case 'reference_import':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.imported || 0} imported, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'image_export':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.exported || 0} exported, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'thumbnails':
                    const p1 = `Phase 1: ${data.phase1_scanned || 0} scanned, ${data.phase1_updated || 0} updated`;
                    const p2 = `Phase 2: ${data.phase2_scanned || 0}/${data.phase2_total || 0} scanned, ${data.phase2_processed || 0} processed, ${data.phase2_errors || 0} errors`;
                    return data.phase === '2' ? `${p1} | ${p2}` : p1;
                case 'contacts':
                    return data.status_line || 'Processing contacts...';
                default:
                    return JSON.stringify(data).substring(0, 100);
            }
        }

        function closeCurrentEventSource() {
            if (currentEventSource) {
                currentEventSource.close();
                currentEventSource = null;
            }
        }

        const autoContactsExtractImports = new Set([
            'whatsapp',
            'imessage',
            'instagram',
            'facebook_all',
            'facebook',
            'email_processing',
            'imap_processing'
        ]);

        async function maybeAutoRunContactsExtract(finishedImportType) {
            if (!autoContactsExtractImports.has(finishedImportType)) return;
            if (finishedImportType === 'contacts') return;
            if (importInProgress) return;

            try {
                const res = await fetch('/contacts/extract/status');
                if (!res.ok) return;
                const status = await res.json();
                if (status && status.in_progress) return;
            } catch (e) {
                console.warn('Auto contacts extract status check failed:', e);
                return;
            }

            try {
                await runImport('contacts', {});
            } catch (e) {
                console.warn('Auto contacts extract start failed:', e);
            }
        }

        function finishImport(importType, success, message) {
            importInProgress = false;
            currentImportType = null;
            setExecuting(importType, false);
            closeCurrentEventSource();
            setImportStatus(message, !success);
            if (typeof loadImportControlLastRun === 'function') loadImportControlLastRun();

            const waiter = importCompletionWaiter;
            if (waiter && waiter.type === importType) {
                importCompletionWaiter = null;
                waiter.resolve({ ok: success, message: message || '' });
            }

            if (!masterImportQueueRunning) {
                void maybeAutoRunContactsExtract(importType);
            }
        }

        const importConfigs = {
            email_processing: { needsInput: true, title: 'Email Processing (Gmail)', run: async (vals) => { const body = { all_labels: vals.all_folders || false, label_ids: vals.all_folders ? [] : (vals.label_ids || []), new_only: vals.new_only || false, exclude_labels: vals.exclude_labels || [] }; const r = await fetch('/gmail/process', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/gmail/process/stream' },
            whatsapp: { needsInput: true, title: 'WhatsApp Import', fields: [{ id: 'directory_path', key: 'whatsapp_import_directory', label: 'WhatsApp Export Directory', placeholder: 'e.g., C:\\iMazingBackup\\WhatsApp', required: true }], run: async (vals) => { const r = await fetch('/whatsapp/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path }) }); return r; }, stream: '/whatsapp/import/stream' },
            facebook: { needsInput: true, title: 'Facebook Messenger Import', fields: [{ id: 'directory_path', key: 'facebook_import_directory', label: 'Export Directory', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11\\your_facebook_activity\\messages\\e2ee_cutover', required: true }, { id: 'user_name', key: 'facebook_user_name', label: 'Your Name (Optional)', placeholder: 'e.g., Dave Burton', required: false }], run: async (vals) => { const r = await fetch('/facebook/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path, user_name: vals.user_name || null }) }); return r; }, stream: '/facebook/import/stream' },
            instagram: { needsInput: true, title: 'Instagram Import', fields: [{ id: 'directory_path', key: 'instagram_import_directory', label: 'Export Directory', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11\\your_instagram_activity\\messages\\inbox', required: true }, { id: 'user_name', key: 'instagram_user_name', label: 'Your Name (Optional)', placeholder: 'e.g., Dave Burton', required: false }], run: async (vals) => { const r = await fetch('/instagram/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path, user_name: vals.user_name || null }) }); return r; }, stream: '/instagram/import/stream' },
            imessage: { needsInput: true, title: 'iMessage Import', fields: [{ id: 'directory_path', key: 'imessage_directory_path', label: 'Directory Path', placeholder: 'Path to iMessage conversation subdirectories', required: true }], run: async (vals) => { const r = await fetch('/imessages/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path }) }); return r; }, stream: '/imessages/import/stream' },
            facebook_albums: { needsInput: true, title: 'Facebook Albums Import', fields: [{ id: 'directory_path', key: 'facebook_albums_import_directory', label: 'Export Directory', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11\\your_facebook_activity\\posts', required: true }], run: async (vals) => { const r = await fetch('/facebook/albums/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path }) }); return r; }, stream: '/facebook/albums/import/stream' },
            facebook_places: { needsInput: true, title: 'Facebook Places Import', fields: [{ id: 'file_path', key: 'facebook_places_import_file', label: 'Facebook Posts JSON File', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11\\your_posts__check_ins__photos_and_videos_1.json', required: true }], run: async (vals) => { const r = await fetch('/facebook/import-places', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ file_path: vals.file_path }) }); return r; }, stream: '/facebook/import-places/stream' },
            facebook_posts: { needsInput: true, title: 'Facebook Posts Import', fields: [{ id: 'file_path', key: 'facebook_posts_import_path', label: 'Posts JSON File or Directory', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11\\your_posts__check_ins__photos_and_videos_1.json', required: true }, { id: 'export_root', key: 'facebook_posts_export_root', label: 'Export Root (Optional — auto-detected if blank)', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11', required: false }], run: async (vals) => { const r = await fetch('/facebook/posts/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ file_path: vals.file_path, export_root: vals.export_root || null }) }); return r; }, stream: '/facebook/posts/import/stream' },
            facebook_all: { needsInput: true, title: 'Facebook Full Import', fields: [{ id: 'directory_path', key: 'facebook_all_directory_path', label: 'Facebook Export Root Directory', placeholder: 'e.g., G:\\My Drive\\meta-2026-Jan-11', required: true }, { id: 'user_name', key: 'facebook_all_user_name', label: 'Your Name (for Messenger classification, optional)', placeholder: 'e.g., Dave', required: false }], run: async (vals) => { const r = await fetch('/facebook/all/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ directory_path: vals.directory_path, user_name: vals.user_name || null }) }); return r; }, stream: '/facebook/all/import/stream' },
            filesystem: { needsInput: true, title: 'Filesystem Image Import', fields: [{ id: 'root_directory', key: 'filesystem_import_directory', label: 'Root Directory(ies)', placeholder: 'e.g., C:\\Users\\Dave\\Pictures; D:\\Photos', required: true }, { id: 'max_images', key: 'filesystem_import_max_images', label: 'Max Images (Optional)', placeholder: 'Leave empty for all', required: false, type: 'number' }, { id: 'reference_mode', key: 'filesystem_import_reference_mode', label: 'Reference only — leave images on filesystem', required: false, type: 'checkbox' }], run: async (vals) => { const body = { root_directory: vals.root_directory, create_thumb_and_get_exif: false, reference_mode: !!vals.reference_mode }; if (vals.max_images) body.max_images = parseInt(vals.max_images, 10); const r = await fetch('/images/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/images/import/stream' },
            reference_import: { needsInput: false, title: 'Import Reference Images to Database', run: async () => { const r = await fetch('/images/import-reference', { method: 'POST' }); return r; }, stream: '/images/import-reference/stream' },
            image_export: { needsInput: true, title: 'Export Images to Filesystem', fields: [{ id: 'target_directory', key: 'image_export_directory', label: 'Target Directory', placeholder: 'e.g., C:\\Users\\Dave\\Exports\\images', required: true }], run: async (vals) => { const r = await fetch('/images/export', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ target_directory: vals.target_directory }) }); return r; }, stream: '/images/export/stream' },
            thumbnails: { needsInput: false, title: 'Image Processing', run: async () => { const r = await fetch('/images/process-thumbnails', { method: 'POST' }); return r; }, stream: '/images/process-thumbnails/stream' },
            thumbnails_async: { needsInput: false, title: 'Image Processing (Async)', run: async () => { const r = await fetch('/images/process-thumbnails/async', { method: 'POST' }); return r; }, stream: null },
            contacts: { needsInput: false, title: 'Contacts Merge', run: async () => { const r = await fetch('/contacts/extract', { method: 'POST' }); return r; }, stream: '/contacts/extract/stream' },
            imap_processing: { needsInput: true, title: 'IMAP Import', run: async (vals) => { const body = { host: vals.host, port: vals.port, username: vals.username, password: vals.password, use_ssl: vals.use_ssl !== false, all_folders: vals.all_folders || false, folders: vals.folders && vals.folders.length ? vals.folders : (vals.all_folders ? [] : ['INBOX']), new_only: vals.new_only || false, exclude_folders: vals.exclude_folders || [] }; return await fetch('/imap/process', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); }, stream: '/imap/process/stream' }
        };

        async function wireGmailImportForm(ids, submitButtonHook) {
            const authStatusText = document.getElementById(ids.authText);
            const authLink = document.getElementById(ids.authLink);
            const importControls = document.getElementById(ids.importControls);
            const allLabelsCb = document.getElementById(ids.allLabels);
            const excludeLabelsWrap = document.getElementById(ids.excludeLabelsWrap);
            const excludeLabelsEl = document.getElementById(ids.excludeLabels);
            const labelsWrap = document.getElementById(ids.labelsWrap);
            const labelsSelect = document.getElementById(ids.labelsSelect);
            const newOnlyCb = document.getElementById(ids.newOnly);

            function collectGmailValues() {
                const all_folders = allLabelsCb ? allLabelsCb.checked : false;
                const label_ids = all_folders ? [] : Array.from(labelsSelect.selectedOptions).map(o => o.value).filter(Boolean);
                const new_only = newOnlyCb ? newOnlyCb.checked : true;
                const exclude_labels = (all_folders && excludeLabelsEl) ? excludeLabelsEl.value.split('\n').map(s => s.trim()).filter(Boolean) : [];
                return { all_folders, label_ids, new_only, exclude_labels };
            }

            function validateGmail() {
                const { all_folders, label_ids } = collectGmailValues();
                if (!all_folders && label_ids.length === 0) {
                    return 'Select at least one Gmail label or enable Process All Labels.';
                }
                return null;
            }

            try {
                const authResp = await fetch('/gmail/auth/status');
                const authData = await authResp.json();
                if (!authData.authenticated) {
                    authStatusText.textContent = 'Not connected to Gmail.';
                    authLink.style.display = 'inline-block';
                    if (submitButtonHook) submitButtonHook.disabled = true;
                } else {
                    const expiry = authData.expiry ? ` (token expires ${new Date(authData.expiry).toLocaleDateString('en-GB', { day: '2-digit', month: 'numeric', year: 'numeric' })})` : '';
                    authStatusText.textContent = `Connected to Gmail${expiry}`;
                    authStatusText.style.color = '#27ae60';
                    importControls.style.display = 'block';
                    if (submitButtonHook) submitButtonHook.disabled = false;

                    const allLabelsVal = typeof getControlValue === 'function' ? getControlValue('gmail_all_labels', false) : false;
                    const newOnlyVal = typeof getControlValue === 'function' ? getControlValue('gmail_new_only', true) : true;
                    allLabelsCb.checked = !!allLabelsVal;
                    newOnlyCb.checked = !!newOnlyVal;
                    labelsWrap.style.display = allLabelsCb.checked ? 'none' : 'block';
                    if (excludeLabelsWrap) excludeLabelsWrap.style.display = allLabelsCb.checked ? 'block' : 'none';
                    allLabelsCb.addEventListener('change', () => {
                        labelsWrap.style.display = allLabelsCb.checked ? 'none' : 'block';
                        if (excludeLabelsWrap) excludeLabelsWrap.style.display = allLabelsCb.checked ? 'block' : 'none';
                    });

                    try {
                        const labelsResp = await fetch('/gmail/labels');
                        if (!labelsResp.ok) throw new Error('Failed to load labels');
                        const labelsData = await labelsResp.json();
                        labelsSelect.innerHTML = '';
                        (labelsData.labels || []).forEach(l => {
                            const opt = document.createElement('option');
                            opt.value = l.id;
                            opt.textContent = l.name;
                            if (l.name === 'INBOX') opt.selected = true;
                            labelsSelect.appendChild(opt);
                        });
                    } catch (e) {
                        labelsSelect.innerHTML = '<option value="">Error loading labels</option>';
                    }
                }
            } catch (e) {
                authStatusText.textContent = 'Could not check Gmail status.';
                authStatusText.style.color = '#c0392b';
            }

            return { collectGmailValues, validateGmail };
        }

        async function showEmailProcessingModal(onSubmit) {
            importInputModalTitle.textContent = 'Email Processing (Gmail)';
            importInputModalBody.innerHTML = gmailImportFormHTML(GMAIL_FORM_IDS_STANDALONE);
            importInputModal.style.display = 'flex';
            importInputModal.style.alignItems = 'center';
            importInputModal.style.justifyContent = 'center';
            const gmailApi = await wireGmailImportForm(GMAIL_FORM_IDS_STANDALONE, importInputModalSubmit);
            const doSubmit = () => {
                const err = gmailApi.validateGmail();
                if (err) return;
                const vals = gmailApi.collectGmailValues();
                if (typeof saveControlValue === 'function') {
                    saveControlValue('gmail_all_labels', vals.all_folders);
                    saveControlValue('gmail_new_only', vals.new_only);
                }
                importInputModal.style.display = 'none';
                importInputModalSubmit.disabled = false;
                onSubmit(vals);
            };
            importInputModalSubmit.onclick = doSubmit;
            importInputModalCancel.onclick = () => { importInputModal.style.display = 'none'; importInputModalSubmit.disabled = false; };
            importInputModal.onclick = (e) => { if (e.target === importInputModal) { importInputModal.style.display = 'none'; importInputModalSubmit.disabled = false; } };
        }

        async function wireImapImportForm(ids) {
            const hostEl = document.getElementById(ids.host);
            const portEl = document.getElementById(ids.port);
            const useSslEl = document.getElementById(ids.useSsl);
            const usernameEl = document.getElementById(ids.username);
            const passwordEl = document.getElementById(ids.password);
            const fetchBtn = document.getElementById(ids.fetchBtn);
            const fetchStatus = document.getElementById(ids.fetchStatus);
            const allFoldersCb = document.getElementById(ids.allFolders);
            const excludeFoldersWrap = document.getElementById(ids.excludeFoldersWrap);
            const excludeFoldersEl = document.getElementById(ids.excludeFolders);
            const foldersWrap = document.getElementById(ids.foldersWrap);
            const foldersSelect = document.getElementById(ids.foldersSelect);
            const newOnlyCb = document.getElementById(ids.newOnly);

            function imapCollectPayload() {
                const host = hostEl.value.trim();
                const port = parseInt(portEl.value, 10) || 993;
                const username = usernameEl.value.trim();
                const password = passwordEl.value;
                const use_ssl = useSslEl.checked;
                const all_folders = allFoldersCb.checked;
                const new_only = newOnlyCb.checked;
                const folder_options = Array.from(foldersSelect.options).map(function (o) { return o.value; });
                const folders = all_folders ? [] : Array.from(foldersSelect.selectedOptions).map(function (o) { return o.value; }).filter(Boolean);
                const exclude_folders = excludeFoldersEl ? excludeFoldersEl.value.split('\n').map(s => s.trim()).filter(Boolean) : [];
                return { host: host, port: port, username: username, password: password, use_ssl: use_ssl, all_folders: all_folders, new_only: new_only, folders: folders, folder_options: folder_options, exclude_folders: exclude_folders };
            }

            async function imapSaveToPrivateStore() {
                try {
                    const body = imapCollectPayload();
                    const res = await fetch('/api/import-saved-settings/imap', {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(body)
                    });
                    if (!res.ok && res.status !== 403) {
                        console.warn('IMAP settings not saved to private store:', res.status);
                    }
                } catch (e) {
                    console.warn('IMAP settings private store save failed:', e);
                }
            }

            async function imapLoadFromPrivateStore() {
                let appliedServer = false;
                try {
                    const res = await fetch('/api/import-saved-settings/imap');
                    if (!res.ok) return;
                    const data = await res.json();
                    if (data.ok && data.saved && data.settings) {
                        var s = data.settings;
                        hostEl.value = s.host || '';
                        portEl.value = String(s.port != null ? s.port : 993);
                        useSslEl.checked = s.use_ssl !== false;
                        usernameEl.value = s.username || '';
                        passwordEl.value = s.password || '';
                        allFoldersCb.checked = !!s.all_folders;
                        newOnlyCb.checked = s.new_only !== false;
                        if (excludeFoldersEl && Array.isArray(s.exclude_folders)) {
                            excludeFoldersEl.value = s.exclude_folders.join('\n');
                        }
                        if (excludeFoldersWrap) excludeFoldersWrap.style.display = allFoldersCb.checked ? 'block' : 'none';
                        if (Array.isArray(s.folder_options) && s.folder_options.length) {
                            foldersSelect.innerHTML = '';
                            var selectedSet = {};
                            (Array.isArray(s.folders) ? s.folders : []).forEach(function (n) { selectedSet[n] = true; });
                            s.folder_options.forEach(function (f) {
                                var opt = document.createElement('option');
                                opt.value = f;
                                opt.textContent = f;
                                if (selectedSet[f]) opt.selected = true;
                                foldersSelect.appendChild(opt);
                            });
                            if (!allFoldersCb.checked) foldersWrap.style.display = 'block';
                        } else if (Array.isArray(s.folders) && s.folders.length) {
                            foldersSelect.innerHTML = '';
                            s.folders.forEach(function (f) {
                                var opt = document.createElement('option');
                                opt.value = f;
                                opt.textContent = f;
                                opt.selected = true;
                                foldersSelect.appendChild(opt);
                            });
                            if (!allFoldersCb.checked) foldersWrap.style.display = 'block';
                        }
                        appliedServer = true;
                    }
                } catch (e) {
                    console.debug('IMAP saved-settings load skipped:', e);
                }
                if (!appliedServer && typeof getControlValue === 'function') {
                    var defs = typeof controlDefaults !== 'undefined' ? controlDefaults : {};
                    hostEl.value = getControlValue('imap_host', defs.imap_host || '') || '';
                    portEl.value = getControlValue('imap_port', defs.imap_port || '993') || '993';
                    useSslEl.checked = getControlValue('imap_use_ssl', defs.imap_use_ssl !== undefined ? defs.imap_use_ssl : true) !== false;
                    usernameEl.value = getControlValue('imap_username', defs.imap_username || '') || '';
                    allFoldersCb.checked = !!(getControlValue('imap_all_folders', false));
                    newOnlyCb.checked = !!(getControlValue('imap_new_only', defs.imap_new_only !== undefined ? defs.imap_new_only : true));
                }
            }

            await imapLoadFromPrivateStore();

            allFoldersCb.addEventListener('change', () => {
                foldersWrap.style.display = (!allFoldersCb.checked && foldersSelect.options.length > 0) ? 'block' : 'none';
                if (excludeFoldersWrap) excludeFoldersWrap.style.display = allFoldersCb.checked ? 'block' : 'none';
            });

            fetchBtn.addEventListener('click', async () => {
                const host = hostEl.value.trim();
                const port = parseInt(portEl.value, 10) || 993;
                const username = usernameEl.value.trim();
                const password = passwordEl.value;
                const use_ssl = useSslEl.checked;
                if (!host || !username || !password) {
                    fetchStatus.textContent = 'Fill in host, username, and password first.';
                    fetchStatus.style.color = '#c0392b';
                    return;
                }
                fetchBtn.disabled = true;
                fetchStatus.textContent = 'Connecting...';
                fetchStatus.style.color = '#666';
                try {
                    const resp = await fetch('/imap/folders', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ host, port, username, password, use_ssl })
                    });
                    if (!resp.ok) {
                        const err = await resp.json();
                        fetchStatus.textContent = err.detail || 'Failed to fetch folders';
                        fetchStatus.style.color = '#c0392b';
                        return;
                    }
                    const data = await resp.json();
                    const folders = data.folders || [];
                    foldersSelect.innerHTML = '';
                    folders.forEach(f => {
                        const opt = document.createElement('option');
                        opt.value = f;
                        opt.textContent = f;
                        foldersSelect.appendChild(opt);
                    });
                    const inboxOpt = Array.from(foldersSelect.options).find(o => o.value.toUpperCase() === 'INBOX');
                    if (inboxOpt) inboxOpt.selected = true;
                    fetchStatus.textContent = `${folders.length} folder(s) loaded`;
                    fetchStatus.style.color = '#27ae60';
                    if (!allFoldersCb.checked) foldersWrap.style.display = 'block';
                    if (excludeFoldersWrap) excludeFoldersWrap.style.display = allFoldersCb.checked ? 'block' : 'none';
                    void imapSaveToPrivateStore();
                } catch (e) {
                    fetchStatus.textContent = 'Connection error: ' + e.message;
                    fetchStatus.style.color = '#c0392b';
                } finally {
                    fetchBtn.disabled = false;
                }
            });

            function collectImapValues() {
                const host = hostEl.value.trim();
                const port = parseInt(portEl.value, 10) || 993;
                const username = usernameEl.value.trim();
                const password = passwordEl.value;
                const use_ssl = useSslEl.checked;
                const all_folders = allFoldersCb.checked;
                const folders = all_folders ? [] : Array.from(foldersSelect.selectedOptions).map(o => o.value).filter(Boolean);
                const new_only = newOnlyCb.checked;
                const exclude_folders = excludeFoldersEl ? excludeFoldersEl.value.split('\n').map(s => s.trim()).filter(Boolean) : [];
                return { host, port, username, password, use_ssl, all_folders, folders, new_only, exclude_folders };
            }

            function validateImap() {
                const { host, username, password, all_folders, folders } = collectImapValues();
                if (!host || !username || !password) {
                    return 'Fill in IMAP host, username, and password.';
                }
                if (!all_folders && folders.length === 0 && foldersSelect.options.length > 0) {
                    return 'Select at least one folder or enable Process All Folders.';
                }
                return null;
            }

            return { collectImapValues, validateImap, imapSaveToPrivateStore };
        }

        async function showImapModal(onSubmit) {
            importInputModalTitle.textContent = 'IMAP Import';
            importInputModalBody.innerHTML = imapImportFormHTML(IMAP_FORM_IDS_STANDALONE);
            importInputModal.style.display = 'flex';
            importInputModal.style.alignItems = 'center';
            importInputModal.style.justifyContent = 'center';
            const imapApi = await wireImapImportForm(IMAP_FORM_IDS_STANDALONE);
            const doSubmit = () => {
                const err = imapApi.validateImap();
                if (err) return;
                const vals = imapApi.collectImapValues();
                if (typeof saveControlValue === 'function') {
                    saveControlValue('imap_host', vals.host);
                    saveControlValue('imap_port', String(vals.port));
                    saveControlValue('imap_use_ssl', vals.use_ssl);
                    saveControlValue('imap_username', vals.username);
                    saveControlValue('imap_all_folders', vals.all_folders);
                    saveControlValue('imap_new_only', vals.new_only);
                }
                void imapApi.imapSaveToPrivateStore();
                importInputModal.style.display = 'none';
                onSubmit(vals);
            };
            importInputModalSubmit.onclick = doSubmit;
            importInputModalCancel.onclick = () => { importInputModal.style.display = 'none'; };
            importInputModal.onclick = (e) => { if (e.target === importInputModal) importInputModal.style.display = 'none'; };
        }

        const IMPORT_SAVED_SETTINGS_BASE = '/api/import-saved-settings';
        const IMPORT_DIALOG_PRIVATE_KINDS = {
            imessage: 'imessage',
            whatsapp: 'whatsapp',
            facebook_all: 'facebook_all',
            instagram: 'instagram',
            filesystem: 'filesystem'
        };

        async function importDialogLoadPrivateStore(kind) {
            try {
                const res = await fetch(`${IMPORT_SAVED_SETTINGS_BASE}/${encodeURIComponent(kind)}`);
                if (!res.ok) return null;
                const data = await res.json();
                if (data.ok && data.saved && data.settings && typeof data.settings === 'object') {
                    return data.settings;
                }
            } catch (e) {
                console.debug('import saved-settings load skipped:', e);
            }
            return null;
        }

        async function importDialogSavePrivateStore(kind, settingsObj) {
            try {
                const res = await fetch(`${IMPORT_SAVED_SETTINGS_BASE}/${encodeURIComponent(kind)}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(settingsObj)
                });
                if (!res.ok && res.status !== 403) {
                    console.warn('Import dialog settings not saved to private store:', kind, res.status);
                }
            } catch (e) {
                console.warn('Import dialog private store save failed:', e);
            }
        }

        function masterImportFieldRowHtml(importType, f) {
            const eid = `mi-${importType}-${f.id}`;
            if (f.type === 'checkbox') {
                return `<div class="setting-group" style="margin-bottom: 12px; display: flex; align-items: center; gap: 8px;"><input type="checkbox" id="${eid}" style="width: 16px; height: 16px;"><label for="${eid}" style="font-weight: 500; cursor: pointer;">${f.label}</label></div>`;
            }
            return `<div class="setting-group" style="margin-bottom: 12px;"><label for="${eid}" style="display: block; margin-bottom: 5px; font-weight: 500;">${f.label}</label><input type="${f.type || 'text'}" id="${eid}" placeholder="${f.placeholder || ''}" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;"></div>`;
        }

        async function applyMasterImportFieldDefaults(importType, container) {
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.fields) return;
            cfg.fields.forEach(f => {
                const el = container.querySelector(`#mi-${importType}-${f.id}`);
                if (el && typeof getControlValue === 'function') {
                    const val = getControlValue(f.key, typeof controlDefaults !== 'undefined' ? controlDefaults[f.key] : null);
                    if (f.type === 'checkbox') {
                        el.checked = val === true || val === 'true';
                    } else {
                        el.value = (val !== undefined && val !== null ? String(val) : '') || '';
                    }
                }
            });
            const privKind = IMPORT_DIALOG_PRIVATE_KINDS[importType];
            if (privKind) {
                const s = await importDialogLoadPrivateStore(privKind);
                if (s) {
                    cfg.fields.forEach(f => {
                        const el = container.querySelector(`#mi-${importType}-${f.id}`);
                        if (!el || !Object.prototype.hasOwnProperty.call(s, f.id)) return;
                        const v = s[f.id];
                        if (f.type === 'checkbox') {
                            el.checked = !!v;
                        } else {
                            el.value = v != null && v !== undefined ? String(v) : '';
                        }
                    });
                }
            }
        }

        function collectMasterImportFieldValues(importType, container) {
            const cfg = importConfigs[importType];
            const vals = {};
            if (!cfg || !cfg.fields) return vals;
            cfg.fields.forEach(f => {
                const el = container.querySelector(`#mi-${importType}-${f.id}`);
                if (f.type === 'checkbox') {
                    vals[f.id] = el ? el.checked : false;
                } else {
                    vals[f.id] = el ? el.value.trim() : '';
                }
            });
            return vals;
        }

        function validateMasterImportFieldValues(importType, container) {
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.fields) return null;
            const vals = collectMasterImportFieldValues(importType, container);
            for (const f of cfg.fields) {
                if (f.required && f.type !== 'checkbox' && !String(vals[f.id] || '').trim()) {
                    return `${cfg.title}: ${f.label} is required.`;
                }
            }
            return null;
        }

        function updateMasterImportReorderButtonStates() {
            const root = document.getElementById('master-import-sections');
            if (!root) return;
            const sections = root.querySelectorAll('.master-import-section');
            sections.forEach((sec, i) => {
                const up = sec.querySelector('.master-import-move-up-btn');
                const down = sec.querySelector('.master-import-move-down-btn');
                if (up) up.disabled = i === 0;
                if (down) down.disabled = i === sections.length - 1;
            });
        }

        async function populateMasterImportModal() {
            const root = document.getElementById('master-import-sections');
            if (!root) return;
            root.innerHTML = '';
            for (const importType of MASTER_IMPORT_TYPES) {
                const meta = MASTER_IMPORT_META[importType];
                const section = document.createElement('section');
                section.className = 'master-import-section';
                section.dataset.masterImportType = importType;
                section.innerHTML = `
                    <div class="master-import-section-head">
                        <div class="master-import-section-head-top">
                            <label class="master-import-run-label"><input type="checkbox" class="master-import-run-cb"> <span>Run this import</span></label>
                            <div class="master-import-reorder-buttons" role="group" aria-label="Run order for this import">
                                <button type="button" class="master-import-move-up-btn master-import-reorder-btn modal-btn modal-btn-secondary" title="Run earlier (move up)" aria-label="Run earlier (move up)"><i class="fas fa-arrow-up" aria-hidden="true"></i></button>
                                <button type="button" class="master-import-move-down-btn master-import-reorder-btn modal-btn modal-btn-secondary" title="Run later (move down)" aria-label="Run later (move down)"><i class="fas fa-arrow-down" aria-hidden="true"></i></button>
                            </div>
                        </div>
                        <h3 class="master-import-section-title"><i class="${meta.icon}"></i> ${meta.title}</h3>
                    </div>
                    <div class="master-import-section-body"></div>
                `;
                const body = section.querySelector('.master-import-section-body');
                if (importType === 'email_processing') {
                    body.innerHTML = gmailImportFormHTML(GMAIL_FORM_IDS_MASTER);
                } else if (importType === 'imap_processing') {
                    body.innerHTML = imapImportFormHTML(IMAP_FORM_IDS_MASTER);
                } else if (importType === 'thumbnails') {
                    body.innerHTML = '<p style="margin:0;font-size:0.9em;color:#64748b;">No extra parameters.</p>';
                } else {
                    const cfg = importConfigs[importType];
                    if (cfg && cfg.fields && cfg.fields.length) {
                        body.innerHTML = cfg.fields.map(f => masterImportFieldRowHtml(importType, f)).join('');
                    }
                }
                // Must attach before wireGmailImportForm / wireImapImportForm: they use document.getElementById,
                // which does not resolve IDs on detached subtrees.
                root.appendChild(section);
                if (importType === 'email_processing') {
                    section._masterGmailApi = await wireGmailImportForm(GMAIL_FORM_IDS_MASTER, null);
                } else if (importType === 'imap_processing') {
                    section._masterImapApi = await wireImapImportForm(IMAP_FORM_IDS_MASTER);
                } else if (importType !== 'thumbnails') {
                    const cfg = importConfigs[importType];
                    if (cfg && cfg.fields && cfg.fields.length) {
                        await applyMasterImportFieldDefaults(importType, body);
                    }
                }
            }
            updateMasterImportReorderButtonStates();
        }

        function gatherMasterImportJobs() {
            const errors = [];
            const jobs = [];
            document.querySelectorAll('#master-import-sections .master-import-section').forEach(section => {
                const cb = section.querySelector('.master-import-run-cb');
                if (!cb || !cb.checked) return;
                const type = section.dataset.masterImportType;
                const body = section.querySelector('.master-import-section-body');
                if (type === 'email_processing') {
                    const api = section._masterGmailApi;
                    if (!api) return;
                    const err = api.validateGmail();
                    if (err) errors.push(err);
                    else jobs.push({ type, values: api.collectGmailValues() });
                } else if (type === 'imap_processing') {
                    const api = section._masterImapApi;
                    if (!api) return;
                    const err = api.validateImap();
                    if (err) errors.push(err);
                    else jobs.push({ type, values: api.collectImapValues() });
                } else if (type === 'thumbnails') {
                    jobs.push({ type, values: {} });
                } else {
                    const err = validateMasterImportFieldValues(type, body);
                    if (err) errors.push(err);
                    else {
                        const vals = collectMasterImportFieldValues(type, body);
                        const cfg = importConfigs[type];
                        if (cfg && cfg.fields) {
                            cfg.fields.forEach(f => {
                                if (typeof saveControlValue === 'function') saveControlValue(f.key, vals[f.id]);
                            });
                            const privKind = IMPORT_DIALOG_PRIVATE_KINDS[type];
                            if (privKind) void importDialogSavePrivateStore(privKind, vals);
                        }
                        jobs.push({ type, values: vals });
                    }
                }
            });
            return { errors, jobs };
        }

        function showMasterImportConfirmDialog(jobs) {
            const willRunEl = document.getElementById('master-import-confirm-will-run');
            const notRunEl = document.getElementById('master-import-confirm-not-run');
            const modal = document.getElementById('master-import-confirm-modal');
            if (!willRunEl || !notRunEl || !modal) {
                void startMasterImportQueue(jobs);
                return;
            }
            masterImportConfirmPendingJobs = jobs;
            willRunEl.innerHTML = '';
            notRunEl.innerHTML = '';
            jobs.forEach((job, i) => {
                const meta = MASTER_IMPORT_META[job.type];
                const title = (meta && meta.title) || job.type;
                const label = document.createElement('label');
                label.className = 'master-import-confirm-row';
                label.setAttribute('role', 'listitem');
                const cb = document.createElement('input');
                cb.type = 'checkbox';
                cb.className = 'master-import-confirm-cb';
                cb.dataset.jobIndex = String(i);
                cb.checked = true;
                const span = document.createElement('span');
                span.textContent = title;
                label.appendChild(cb);
                label.appendChild(span);
                willRunEl.appendChild(label);
            });
            const runningTypes = new Set(jobs.map(j => j.type));
            const notRunningTypes = MASTER_IMPORT_TYPES.filter(t => !runningTypes.has(t));
            if (notRunningTypes.length === 0) {
                const note = document.createElement('p');
                note.className = 'master-import-confirm-empty-note';
                note.textContent = 'All master import types are included in the batch above.';
                notRunEl.appendChild(note);
            } else {
                notRunningTypes.forEach(t => {
                    const meta = MASTER_IMPORT_META[t];
                    const title = (meta && meta.title) || t;
                    const row = document.createElement('div');
                    row.className = 'master-import-confirm-row-static';
                    row.setAttribute('role', 'listitem');
                    const span = document.createElement('span');
                    span.textContent = title;
                    row.appendChild(span);
                    notRunEl.appendChild(row);
                });
            }
            modal.style.display = 'flex';
            modal.style.alignItems = 'center';
            modal.style.justifyContent = 'center';
        }

        function closeMasterImportConfirmModal() {
            const modal = document.getElementById('master-import-confirm-modal');
            if (modal) modal.style.display = 'none';
            masterImportConfirmPendingJobs = null;
        }

        function confirmMasterImportExecute() {
            const jobs = masterImportConfirmPendingJobs;
            if (!jobs || !jobs.length) {
                closeMasterImportConfirmModal();
                return;
            }
            const willRunEl = document.getElementById('master-import-confirm-will-run');
            const confirmed = [];
            if (willRunEl) {
                willRunEl.querySelectorAll('.master-import-confirm-cb').forEach(cb => {
                    if (!cb.checked) return;
                    const idx = parseInt(cb.dataset.jobIndex, 10);
                    if (!Number.isNaN(idx) && jobs[idx]) confirmed.push(jobs[idx]);
                });
            }
            if (confirmed.length === 0) {
                window.alert('Select at least one job to run, or choose Back.');
                return;
            }
            closeMasterImportConfirmModal();
            void startMasterImportQueue(confirmed);
        }

        async function startMasterImportQueue(jobs) {
            if (!jobs || !jobs.length) return;
            if (importInProgress) {
                window.alert('An import is already running. Wait for it to finish.');
                return;
            }
            masterImportQueueRunning = true;
            masterImportBatchAbort = false;
            lastMasterBatchImportType = null;
            let lastContactsTriggerType = null;
            const mim = document.getElementById('master-import-modal');
            if (mim) mim.style.display = 'none';
            const dim = document.getElementById('data-import-modal');
            if (dim) dim.style.display = 'flex';
            for (let i = 0; i < jobs.length; i++) {
                if (masterImportBatchAbort) {
                    setImportStatus('Master import queue stopped (cancelled).');
                    break;
                }
                const job = jobs[i];
                lastMasterBatchImportType = job.type;
                if (autoContactsExtractImports.has(job.type)) {
                    lastContactsTriggerType = job.type;
                }
                setImportStatus(`[Master ${i + 1}/${jobs.length}] Starting ${job.type}…`);
                const r = await runImportUntilComplete(job.type, job.values);
                setImportStatus(`[Master ${i + 1}/${jobs.length}] ${job.type}: ${r.ok ? 'finished' : 'failed'} — ${r.message || ''}`, !r.ok);
            }
            masterImportQueueRunning = false;
            if (lastContactsTriggerType) {
                void maybeAutoRunContactsExtract(lastContactsTriggerType);
            }
        }

        async function executeMasterImportRun() {
            const { errors, jobs } = gatherMasterImportJobs();
            if (errors.length > 0) {
                window.alert(errors.join('\n'));
                return;
            }
            if (jobs.length === 0) {
                window.alert('Select at least one import to run.');
                return;
            }
            if (importInProgress) {
                window.alert('An import is already running. Wait for it to finish.');
                return;
            }
            showMasterImportConfirmDialog(jobs);
        }

        async function showImportModal(importType, onSubmit) {
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.needsInput) { onSubmit({}); return; }
            importInputModalTitle.textContent = cfg.title;
            importInputModalBody.innerHTML = cfg.fields.map(f => {
                if (f.type === 'checkbox') {
                    return `<div class="setting-group" style="margin-bottom: 15px; display: flex; align-items: center; gap: 8px;"><input type="checkbox" id="import-modal-${f.id}" style="width: 16px; height: 16px;"><label for="import-modal-${f.id}" style="font-weight: 500; cursor: pointer;">${f.label}</label></div>`;
                }
                return `<div class="setting-group" style="margin-bottom: 15px;"><label for="import-modal-${f.id}" style="display: block; margin-bottom: 5px; font-weight: 500;">${f.label}</label><input type="${f.type || 'text'}" id="import-modal-${f.id}" placeholder="${f.placeholder || ''}" style="width: 100%; padding: 8px; border-radius: 4px; border: 1px solid #bfc9da;"></div>`;
            }).join('');
            cfg.fields.forEach(f => {
                const el = document.getElementById(`import-modal-${f.id}`);
                if (el && typeof getControlValue === 'function') {
                    const val = getControlValue(f.key, typeof controlDefaults !== 'undefined' ? controlDefaults[f.key] : null);
                    if (f.type === 'checkbox') {
                        el.checked = val === true || val === 'true';
                    } else {
                        el.value = (val !== undefined && val !== null ? String(val) : '') || '';
                    }
                }
            });

            const privKind = IMPORT_DIALOG_PRIVATE_KINDS[importType];
            if (privKind) {
                const s = await importDialogLoadPrivateStore(privKind);
                if (s) {
                    cfg.fields.forEach(f => {
                        const el = document.getElementById(`import-modal-${f.id}`);
                        if (!el || !Object.prototype.hasOwnProperty.call(s, f.id)) return;
                        const v = s[f.id];
                        if (f.type === 'checkbox') {
                            el.checked = !!v;
                        } else {
                            el.value = v != null && v !== undefined ? String(v) : '';
                        }
                    });
                }
            }

            importInputModal.style.display = 'flex';
            importInputModal.style.alignItems = 'center';
            importInputModal.style.justifyContent = 'center';

            const doSubmit = () => {
                const vals = {};
                let valid = true;
                cfg.fields.forEach(f => {
                    const el = document.getElementById(`import-modal-${f.id}`);
                    if (f.type === 'checkbox') {
                        vals[f.id] = el ? el.checked : false;
                        if (typeof saveControlValue === 'function') saveControlValue(f.key, el ? el.checked : false);
                    } else {
                        const v = el ? el.value.trim() : '';
                        if (f.required && !v) valid = false;
                        vals[f.id] = v;
                        if (typeof saveControlValue === 'function') saveControlValue(f.key, v);
                    }
                });
                if (!valid && cfg.fields.some(f => f.required)) return;
                if (privKind) void importDialogSavePrivateStore(privKind, vals);
                importInputModal.style.display = 'none';
                onSubmit(vals);
            };

            importInputModalSubmit.onclick = doSubmit;
            importInputModalCancel.onclick = () => { importInputModal.style.display = 'none'; };
            importInputModal.onclick = (e) => { if (e.target === importInputModal) importInputModal.style.display = 'none'; };
            importInputModalBody.querySelectorAll('input').forEach(inp => {
                inp.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); doSubmit(); } });
            });
        }

        function connectToImportStream(importType) {
            closeCurrentEventSource();
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.stream) return;
            currentEventSource = new EventSource(cfg.stream);
            currentEventSource.onmessage = (event) => {
                try {
                    const ed = JSON.parse(event.data);
                    const type = ed.type;
                    const data = ed.data || {};
                    if (type === 'progress' || type === 'status') {
                        const line = data.status_line || formatProgressLine(importType, data);
                        setImportStatus(line);
                    } else if (type === 'completed') {
                        const line = data.status_line || formatProgressLine(importType, data) || 'Completed successfully';
                        finishImport(importType, true, line);
                    } else if (type === 'error') {
                        finishImport(importType, false, data.error_message || data.status_line || 'Error');
                    } else if (type === 'cancelled') {
                        finishImport(importType, false, data.status_line || 'Cancelled');
                    }
                } catch (e) { console.warn('Import SSE parse error:', e); }
            };
            currentEventSource.onerror = () => {};
        }

        function rejectImportCompletionWaiter(importType, message) {
            if (importCompletionWaiter && importCompletionWaiter.type === importType) {
                importCompletionWaiter.resolve({ ok: false, message: message || 'Import did not start.' });
                importCompletionWaiter = null;
            }
        }

        function runImportUntilComplete(importType, values) {
            if (importInProgress) {
                return Promise.resolve({ ok: false, message: 'Another import is already running.' });
            }
            return new Promise((resolve) => {
                importCompletionWaiter = { type: importType, resolve };
                void runImport(importType, values);
            });
        }

        async function runImport(importType, values) {
            if (importInProgress) {
                rejectImportCompletionWaiter(importType, 'Import already running.');
                return;
            }
            const cfg = importConfigs[importType];
            if (!cfg) {
                rejectImportCompletionWaiter(importType, 'Unknown import type.');
                return;
            }
            importInProgress = true;
            currentImportType = importType;
            setExecuting(importType, true);
            setImportStatus('Starting...');

            try {
                const res = await cfg.run(values);
                let result = {};
                try {
                    const text = await res.text();
                    if (text) result = JSON.parse(text);
                } catch (_) {}
                if (!res.ok) {
                    finishImport(importType, false, result.detail || 'Failed to start');
                    return;
                }
                if (cfg.stream) {
                    connectToImportStream(importType);
                } else {
                    finishImport(importType, true, 'Accepted');
                }
            } catch (e) {
                finishImport(importType, false, e.message || 'Error');
            }
        }

        async function triggerImport(importType) {
            if (importInProgress) return;
            if (importType === 'email_processing') {
                showEmailProcessingModal((vals) => runImport(importType, vals));
            } else if (importType === 'imap_processing') {
                showImapModal((vals) => runImport(importType, vals));
            } else {
                await showImportModal(importType, (vals) => runImport(importType, vals));
            }
        }

        document.querySelectorAll('.import-execute-btn').forEach(btn => {
            btn.addEventListener('click', () => { void triggerImport(btn.getAttribute('data-import')); });
        });

        document.querySelectorAll('.import-control-tile').forEach(tile => {
            tile.addEventListener('click', (e) => {
                const openModal = tile.getAttribute('data-open-modal');
                if (openModal) {
                    if (openModal === 'data-import-modal') {
                        void (async () => {
                            if (!(await ensureMasterKeyForDataImport())) return;
                            const modal = document.getElementById(openModal);
                            if (modal) {
                                modal.style.display = 'flex';
                                if (DOM.configPage) DOM.configPage.style.display = 'none';
                                if (typeof loadControlDefaults === 'function') loadControlDefaults();
                            }
                        })();
                        return;
                    }
                    if (openModal === 'master-import-modal') {
                        void (async () => {
                            if (!(await ensureMasterKeyForDataImport())) return;
                            const dim = document.getElementById('data-import-modal');
                            const mim = document.getElementById('master-import-modal');
                            if (dim) dim.style.display = 'none';
                            if (mim) {
                                mim.style.display = 'flex';
                                await populateMasterImportModal();
                            }
                        })();
                        return;
                    }
                    const modal = document.getElementById(openModal);
                    if (modal) {
                        modal.style.display = 'flex';
                        if (openModal === 'reference-documents-manage-modal') {
                            const dim = document.getElementById('data-import-modal');
                            if (dim) dim.style.display = 'none';
                            if (Modals.ReferenceDocuments && Modals.ReferenceDocuments.loadDocuments) {
                                Modals.ReferenceDocuments.loadDocuments();
                            }
                        }
                    }
                    return;
                }
                const openTab = tile.getAttribute('data-open-tab');
                if (openTab) {
                    const dataImportModal = document.getElementById('data-import-modal');
                    if (dataImportModal) dataImportModal.style.display = 'none';
                    if (DOM.configPage) {
                        DOM.configPage.style.display = 'flex';
                        loadControlDefaults();
                        const tabBtn = document.querySelector(`.config-tab-button[data-tab="${openTab}"]`);
                        if (tabBtn) {
                            tabBtn.click();
                        }
                        if (openTab === 'settings' && Modals.AppConfig && Modals.AppConfig.load) {
                            void Modals.AppConfig.load();
                        }
                    }
                    return;
                }
                void triggerImport(tile.getAttribute('data-import'));
            });
        });

        importCancelBtns().forEach(btn => {
            btn.addEventListener('click', async () => {
                if (!currentImportType) return;
                if (masterImportQueueRunning) {
                    masterImportBatchAbort = true;
                }
                const endpoint = cancelEndpoints[currentImportType];
                if (!endpoint) return;
                try {
                    await fetch(endpoint, { method: 'POST' });
                } catch (e) { console.warn('Cancel error:', e); }
            });
        });

        function resetImportControls() {
            closeCurrentEventSource();
            importInProgress = false;
            currentImportType = null;
            setImportStatus('Idle');
            importCancelBtns().forEach(btn => { btn.disabled = true; });
            document.querySelectorAll('.import-execute-btn').forEach(btn => {
                btn.disabled = false;
                btn.innerHTML = '<i class="fas fa-play"></i> Execute';
                btn.style.backgroundColor = '';
                btn.classList.remove('import-executing');
            });
            document.querySelectorAll('.import-control-tile').forEach(tile => tile.classList.remove('import-executing'));
            if (typeof loadImportControlLastRun === 'function') loadImportControlLastRun();
        }

        document.querySelectorAll('.import-controls-reset-btn').forEach(btn => {
            btn.addEventListener('click', () => resetImportControls());
        });

        async function checkInitialImportStatus() {
            const types = ['email_processing','imap_processing','imessage','whatsapp','facebook','instagram','facebook_albums','facebook_places','filesystem','reference_import','image_export','thumbnails','contacts'];
            const statusEndpoints = { email_processing: '/emails/process/status', imap_processing: '/imap/process/status', imessage: '/imessages/import/status', whatsapp: '/whatsapp/import/status', facebook: '/facebook/import/status', instagram: '/instagram/import/status', facebook_albums: '/facebook/albums/import/status', facebook_places: '/facebook/import-places/status', filesystem: '/images/import/status', reference_import: '/images/import-reference/status', image_export: '/images/export/status', thumbnails: '/images/process-thumbnails/status', contacts: '/contacts/extract/status' };
            for (const t of types) {
                try {
                    const r = await fetch(statusEndpoints[t]);
                    const s = await r.json();
                    if (s.in_progress) { importInProgress = true; currentImportType = t; setExecuting(t, true); connectToImportStream(t); return; }
                } catch (_) {}
            }
        }
        checkInitialImportStatus();


        
        // Sidebar button event listeners
        if (DOM.fbAlbumsSidebarBtn) {
            DOM.fbAlbumsSidebarBtn.addEventListener('click', () => {
                Modals.FBAlbums.open();
            });
        }
        if (DOM.fbPostsSidebarBtn) {
            DOM.fbPostsSidebarBtn.addEventListener('click', () => {
                Modals.FBPosts.open();
            });
        }

        // if (DOM.imageGallerySidebarBtn) {
        //     DOM.imageGallerySidebarBtn.addEventListener('click', () => {
        //         Modals.ImageGallery.open();
        //     });
        // }

        if (DOM.locationsSidebarBtn) {
            DOM.locationsSidebarBtn.addEventListener('click', () => {
                Modals.Locations.open();
            });
        }

        if (DOM.emailGallerySidebarBtn) {
            DOM.emailGallerySidebarBtn.addEventListener('click', () => {
                Modals.EmailGallery.open();
            });
        }

        if (DOM.newImageGallerySidebarBtn) {
            DOM.newImageGallerySidebarBtn.addEventListener('click', () => {
                Modals.NewImageGallery.open();
            });
        }

        const smsMessagesSidebarBtn = document.getElementById('sms-messages-sidebar-btn');
        if (smsMessagesSidebarBtn) {
            smsMessagesSidebarBtn.addEventListener('click', () => {
                Modals.SMSMessages.open();
            });
        }

        if (DOM.suggestionsSidebarBtn) {
            DOM.suggestionsSidebarBtn.addEventListener('click', () => {
                Modals.Suggestions.open();
            });
        }

        if (DOM.statisticsSidebarBtn) {
            DOM.statisticsSidebarBtn.addEventListener('click', () => {
                if (DOM.statisticsModal) {
                    DOM.statisticsModal.style.display = 'flex';
                    loadDashboard('stats-');
                }
            });
        }
        if (DOM.closeStatisticsModalBtn) {
            DOM.closeStatisticsModalBtn.addEventListener('click', () => {
                if (DOM.statisticsModal) DOM.statisticsModal.style.display = 'none';
            });
        }
        if (DOM.statisticsModal) {
            DOM.statisticsModal.addEventListener('click', (e) => {
                if (e.target === DOM.statisticsModal) DOM.statisticsModal.style.display = 'none';
            });
        }
        const statsRefreshBtn = document.getElementById('stats-dashboard-refresh-btn');
        if (statsRefreshBtn) {
            statsRefreshBtn.addEventListener('click', () => loadDashboard('stats-'));
        }

        // if (DOM.haveYourSaySidebarBtn) {
        //     DOM.haveYourSaySidebarBtn.addEventListener('click', () => {
        //         Modals.HaveYourSay.open();
        //     });
        // }

        const contactsSidebarBtn = document.getElementById('contacts-sidebar-btn');
        if (contactsSidebarBtn) {
            contactsSidebarBtn.addEventListener('click', () => {
                Modals.Contacts.open();
            });
        }

        const profilesSidebarBtn = document.getElementById('profiles-sidebar-btn');
        if (profilesSidebarBtn) {
            profilesSidebarBtn.addEventListener('click', () => {
                Modals.Profiles.open();
            });
        }

        const relationshipsBtn = document.getElementById('relationships-btn');
        if (relationshipsBtn) {
            relationshipsBtn.addEventListener('click', () => {
                Modals.Relationships.open();
            });
        }

        if (DOM.artefactsSidebarBtn) {
            DOM.artefactsSidebarBtn.addEventListener('click', () => {
                Modals.Artefacts.open();
            });
        }

        if (DOM.sensitiveSidebarBtn) {
            DOM.sensitiveSidebarBtn.addEventListener('click', () => {
                Modals.SensitiveData.open();
            });
        }

        const settingsDataImportSidebarBtn = document.getElementById('settings-data-import-sidebar-btn');
        if (settingsDataImportSidebarBtn && DOM.configPage) {
            settingsDataImportSidebarBtn.addEventListener('click', () => {
                DOM.configPage.style.display = 'flex';
                loadControlDefaults();
                if (Modals.AppConfig && Modals.AppConfig.load) Modals.AppConfig.load();
            });
        }

        const emailEditorOpenAttachmentsBtn = document.getElementById('email-editor-open-attachments-btn');
        if (emailEditorOpenAttachmentsBtn) {
            emailEditorOpenAttachmentsBtn.addEventListener('click', async () => {
                if (!(await ensureMasterKeyForDataImport())) return;
                if (Modals.EmailEditor && Modals.EmailEditor.close) Modals.EmailEditor.close();
                if (Modals.EmailAttachments && Modals.EmailAttachments.open) Modals.EmailAttachments.open();
            });
        }

        const emailAttachmentsOpenEmailManagerBtn = document.getElementById('email-attachments-open-email-manager-btn');
        if (emailAttachmentsOpenEmailManagerBtn) {
            emailAttachmentsOpenEmailManagerBtn.addEventListener('click', async () => {
                if (!(await ensureMasterKeyForDataImport())) return;
                if (Modals.EmailAttachments && Modals.EmailAttachments.close) Modals.EmailAttachments.close();
                if (Modals.EmailEditor && Modals.EmailEditor.open) Modals.EmailEditor.open();
            });
        }

        const emailGalleryManageEmailsBtn = document.getElementById('email-gallery-manage-emails-btn');
        if (emailGalleryManageEmailsBtn) {
            emailGalleryManageEmailsBtn.addEventListener('click', async () => {
                if (!(await ensureMasterKeyForDataImport())) return;
                if (Modals.EmailGallery && Modals.EmailGallery.close) Modals.EmailGallery.close();
                if (Modals.EmailEditor && Modals.EmailEditor.open) Modals.EmailEditor.open();
            });
        }

        const dataImportSidebarBtn = document.getElementById('data-import-sidebar-btn');
        const dataImportModal = document.getElementById('data-import-modal');
        const closeDataImportModalBtn = document.getElementById('close-data-import-modal');
        if (dataImportSidebarBtn && dataImportModal) {
            dataImportSidebarBtn.addEventListener('click', () => {
                void (async () => {
                    if (!(await ensureMasterKeyForDataImport())) return;
                    dataImportModal.style.display = 'flex';
                    loadControlDefaults();
                })();
            });
        }
        const closeDataImportModal = () => {
            if (dataImportModal) dataImportModal.style.display = 'none';
        };
        if (closeDataImportModalBtn && dataImportModal) {
            closeDataImportModalBtn.addEventListener('click', closeDataImportModal);
        }
        if (dataImportModal) {
            dataImportModal.addEventListener('click', (e) => {
                if (e.target === dataImportModal) closeDataImportModal();
            });
        }

        const referenceDocumentsManageModal = document.getElementById('reference-documents-manage-modal');
        const closeReferenceDocumentsManageBtn = document.getElementById('close-reference-documents-manage-modal');
        const closeReferenceDocumentsManageModal = () => {
            if (referenceDocumentsManageModal) referenceDocumentsManageModal.style.display = 'none';
        };
        if (closeReferenceDocumentsManageBtn && referenceDocumentsManageModal) {
            closeReferenceDocumentsManageBtn.addEventListener('click', closeReferenceDocumentsManageModal);
        }
        if (referenceDocumentsManageModal) {
            referenceDocumentsManageModal.addEventListener('click', (e) => {
                if (e.target === referenceDocumentsManageModal) closeReferenceDocumentsManageModal();
            });
        }

        const masterImportModal = document.getElementById('master-import-modal');
        const closeMasterImportModalBtn = document.getElementById('close-master-import-modal');
        const masterImportCancelFooterBtn = document.getElementById('master-import-cancel-footer-btn');
        const masterImportRunSelectedBtn = document.getElementById('master-import-run-selected-btn');
        const masterImportSelectAllBtn = document.getElementById('master-import-select-all-btn');
        const masterImportClearAllBtn = document.getElementById('master-import-clear-all-btn');

        function closeMasterImportModal() {
            if (masterImportQueueRunning) {
                window.alert('Wait for the master import queue to finish, or use Cancel in the Data Import dialog to stop the current job.');
                return;
            }
            closeMasterImportConfirmModal();
            if (masterImportModal) masterImportModal.style.display = 'none';
            if (dataImportModal) dataImportModal.style.display = 'flex';
        }

        if (closeMasterImportModalBtn) {
            closeMasterImportModalBtn.addEventListener('click', closeMasterImportModal);
        }
        if (masterImportCancelFooterBtn) {
            masterImportCancelFooterBtn.addEventListener('click', closeMasterImportModal);
        }
        if (masterImportModal) {
            masterImportModal.addEventListener('click', (e) => {
                if (e.target === masterImportModal) closeMasterImportModal();
            });
        }
        if (masterImportRunSelectedBtn) {
            masterImportRunSelectedBtn.addEventListener('click', () => { void executeMasterImportRun(); });
        }

        const masterImportConfirmModal = document.getElementById('master-import-confirm-modal');
        const closeMasterImportConfirmBtn = document.getElementById('close-master-import-confirm-modal');
        const masterImportConfirmCancelBtn = document.getElementById('master-import-confirm-cancel-btn');
        const masterImportConfirmExecuteBtn = document.getElementById('master-import-confirm-execute-btn');
        if (closeMasterImportConfirmBtn) {
            closeMasterImportConfirmBtn.addEventListener('click', closeMasterImportConfirmModal);
        }
        if (masterImportConfirmCancelBtn) {
            masterImportConfirmCancelBtn.addEventListener('click', closeMasterImportConfirmModal);
        }
        if (masterImportConfirmExecuteBtn) {
            masterImportConfirmExecuteBtn.addEventListener('click', () => { confirmMasterImportExecute(); });
        }
        if (masterImportConfirmModal) {
            masterImportConfirmModal.addEventListener('click', (e) => {
                if (e.target === masterImportConfirmModal) closeMasterImportConfirmModal();
            });
        }
        if (masterImportSelectAllBtn) {
            masterImportSelectAllBtn.addEventListener('click', () => {
                document.querySelectorAll('#master-import-sections .master-import-run-cb').forEach(c => { c.checked = true; });
            });
        }
        if (masterImportClearAllBtn) {
            masterImportClearAllBtn.addEventListener('click', () => {
                document.querySelectorAll('#master-import-sections .master-import-run-cb').forEach(c => { c.checked = false; });
            });
        }

        const masterImportSectionsRoot = document.getElementById('master-import-sections');
        if (masterImportSectionsRoot) {
            masterImportSectionsRoot.addEventListener('click', (e) => {
                const upBtn = e.target.closest('.master-import-move-up-btn');
                const downBtn = e.target.closest('.master-import-move-down-btn');
                if (!upBtn && !downBtn) return;
                const section = e.target.closest('.master-import-section');
                if (!section || !masterImportSectionsRoot.contains(section)) return;
                e.preventDefault();
                if (upBtn && !upBtn.disabled) {
                    const prev = section.previousElementSibling;
                    if (prev && prev.classList.contains('master-import-section')) {
                        masterImportSectionsRoot.insertBefore(section, prev);
                    }
                } else if (downBtn && !downBtn.disabled) {
                    const next = section.nextElementSibling;
                    if (next && next.classList.contains('master-import-section')) {
                        masterImportSectionsRoot.insertBefore(next, section);
                    }
                }
                updateMasterImportReorderButtonStates();
            });
        }

        const randomQuestionSidebarBtn = document.getElementById('random-question-sidebar-btn');
        if (randomQuestionSidebarBtn) {
            randomQuestionSidebarBtn.addEventListener('click', () => {
                App.processQuestionSubmit();
            });
        }

        const todaysThingSidebarBtn = document.getElementById('todays-thing-sidebar-btn');
        const todaysThingAddInterestModal = document.getElementById('todays-thing-add-interest-modal');
        const todaysThingAddInterestInput = document.getElementById('todays-thing-add-interest-input');
        const todaysThingAddInterestError = document.getElementById('todays-thing-add-interest-error');
        const todaysThingAddInterestSave = document.getElementById('todays-thing-add-interest-save');
        const todaysThingAddInterestCancel = document.getElementById('todays-thing-add-interest-cancel');
        const closeTodaysThingAddInterest = document.getElementById('close-todays-thing-add-interest-modal');

        const runTodaysThing = () => App.processFormSubmit(
            "What's today's things of interest? Suggest something interesting for today based on my interests.",
            "Today's Things of Interest",
            "What's going on today?"
        );

        if (todaysThingAddInterestModal && todaysThingAddInterestSave) {
            const closeTodaysThingModal = () => { todaysThingAddInterestModal.style.display = 'none'; };
            todaysThingAddInterestSave.addEventListener('click', async () => {
                const name = (todaysThingAddInterestInput?.value || '').trim();
                if (!name) {
                    if (todaysThingAddInterestError) {
                        todaysThingAddInterestError.textContent = 'Please enter at least one topic.';
                        todaysThingAddInterestError.style.display = 'block';
                    }
                    return;
                }
                if (todaysThingAddInterestError) todaysThingAddInterestError.style.display = 'none';
                try {
                    const postRes = await fetch('/api/interests', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ name })
                    });
                    if (!postRes.ok) {
                        const d = await postRes.json().catch(() => ({}));
                        const msg = typeof d.detail === 'string' ? d.detail : 'Failed to save';
                        if (todaysThingAddInterestError) {
                            todaysThingAddInterestError.textContent = msg;
                            todaysThingAddInterestError.style.display = 'block';
                        }
                        return;
                    }
                    closeTodaysThingModal();
                    runTodaysThing();
                } catch (e) {
                    if (todaysThingAddInterestError) {
                        todaysThingAddInterestError.textContent = 'Failed to save: ' + e.message;
                        todaysThingAddInterestError.style.display = 'block';
                    }
                }
            });
            if (todaysThingAddInterestCancel) todaysThingAddInterestCancel.addEventListener('click', closeTodaysThingModal);
            if (closeTodaysThingAddInterest) closeTodaysThingAddInterest.addEventListener('click', closeTodaysThingModal);
            todaysThingAddInterestModal.addEventListener('click', (e) => { if (e.target === todaysThingAddInterestModal) closeTodaysThingModal(); });
        }

        if (todaysThingSidebarBtn) {
            todaysThingSidebarBtn.addEventListener('click', async () => {
                try {
                    const res = await fetch('/api/interests');
                    if (!res.ok) { runTodaysThing(); return; }
                    const data = await res.json();
                    if (data && data.length > 0) {
                        runTodaysThing();
                        return;
                    }
                    if (todaysThingAddInterestModal && todaysThingAddInterestInput) {
                        todaysThingAddInterestInput.value = '';
                        if (todaysThingAddInterestError) { todaysThingAddInterestError.style.display = 'none'; todaysThingAddInterestError.textContent = ''; }
                        todaysThingAddInterestModal.style.display = 'flex';
                    } else {
                        runTodaysThing();
                    }
                } catch (e) {
                    runTodaysThing();
                }
            });
        }

    }

    async function loadLLMProviderAvailability() {
        const select = DOM.llmProviderSelect;
        if (!select) return;
        try {
            const res = await fetch('/chat/availability');
            if (!res.ok) return;
            const { gemini_available, claude_available } = await res.json();
            const geminiOpt = select.querySelector('option[value="gemini"]');
            const claudeOpt = select.querySelector('option[value="claude"]');
            if (geminiOpt) geminiOpt.disabled = !gemini_available;
            if (claudeOpt) claudeOpt.disabled = !claude_available;
            const current = select.value;
            if ((current === 'gemini' && !gemini_available) || (current === 'claude' && !claude_available)) {
                select.value = gemini_available ? 'gemini' : (claude_available ? 'claude' : 'gemini');
            }
        } catch (e) {
            console.error('Failed to load LLM availability:', e);
        }
    }

    function closeMasterKeyUnlockModal() {
        const modal = document.getElementById('master-key-unlock-modal');
        const input = document.getElementById('master-key-unlock-input');
        const errEl = document.getElementById('master-key-unlock-error');
        const submitBtn = document.getElementById('master-key-unlock-submit');
        const visitorBtn = document.getElementById('master-key-unlock-visitor-submit');
        if (modal) modal.style.display = 'none';
        if (input) input.value = '';
        if (errEl) {
            errEl.style.display = 'none';
            errEl.textContent = '';
        }
        if (submitBtn) {
            submitBtn.disabled = false;
            submitBtn.textContent = 'Unlock with Master Key';
        }
        if (visitorBtn) {
            visitorBtn.disabled = false;
            visitorBtn.textContent = 'Unlock as visitor';
        }
    }

    /** After welcome info box: offer master key unlock if keyring exists and server RAM has no key yet. */
    function maybePromptMasterKeyUnlock() {
        (async () => {
            try {
                const kc = await fetch('/sensitive-data/key-count', { credentials: 'same-origin' });
                if (!kc.ok) return;
                const kj = await kc.json();
                if (!kj.count) return;
                const st = await fetch('/api/session/master-key/status', { credentials: 'same-origin' });
                if (!st.ok) return;
                const sj = await st.json();
                if (sj.unlocked) return;
                const modal = document.getElementById('master-key-unlock-modal');
                const input = document.getElementById('master-key-unlock-input');
                if (!modal || !input) return;
                closeMasterKeyUnlockModal();
                modal.style.display = 'flex';
                input.focus();
            } catch (e) {
                console.warn('Master key prompt check failed:', e);
            }
        })();
    }

    function init() {
        // Info box modal: set up close first (before other inits that might throw)
        window.closeInfoBoxModal = function() {
            const modal = document.getElementById('info-box-modal');
            if (modal) {
                modal.classList.add('info-box-modal-closed');
                if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(true);
            }
            const geminiOk = CONSTANTS.LLM_PROVIDERS && CONSTANTS.LLM_PROVIDERS.GEMINI === 'True';
            const claudeOk = CONSTANTS.LLM_PROVIDERS && CONSTANTS.LLM_PROVIDERS.CLAUDE === 'True';
            if (!geminiOk && !claudeOk && Modals.ConfirmationModal) {
                Modals.ConfirmationModal.open(
                    'No AI Provider Available',
                    'No LLM provider is available. AI functions will not be available until at least one API key (Gemini or Anthropic) is configured in the Application Configuration.',
                    undefined
                );
            }
            maybePromptMasterKeyUnlock();
        };
        const infoBoxModal = document.getElementById('info-box-modal');
        if (infoBoxModal) {
            infoBoxModal.addEventListener('click', (e) => {
                if (e.target === infoBoxModal) window.closeInfoBoxModal();
            });
            document.getElementById('info-box-close-btn')?.addEventListener('click', window.closeInfoBoxModal);
            if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(false);
        }

        Config.init(); // Loads and applies settings, sets up its listeners
        Chat.renderExistingMessages();
        VoiceSelector.init(); // Sets initial voice state, creativity lock, listeners
        Modals.initAll();
        //SSE.init();
        //InterviewerMode.init(); // Initialize interviewer mode
        try {
            initEventListeners(); // Attach main app event listeners
        } catch (e) {
            console.error('initEventListeners failed (some UI may be broken):', e);
        }
        refreshDataImportMasterKeyAccessUI();
        (function observeConfigModalMasterUnlockRefresh() {
            const el = document.getElementById('config-modal-overlay');
            if (!el) return;
            const run = () => {
                try {
                    if (window.getComputedStyle(el).display !== 'none') refreshDataImportMasterKeyAccessUI();
                } catch (e) { /* ignore */ }
            };
            const obs = new MutationObserver(run);
            obs.observe(el, { attributes: true, attributeFilter: ['style'] });
        })();
        loadLLMProviderAvailability();
        try {
            if (Modals.AppConfig && Modals.AppConfig.load) void Modals.AppConfig.load();
        } catch (e) {
            console.warn('Application configuration preload failed:', e);
        }

        const mkSkip = document.getElementById('master-key-unlock-skip');
        const mkSubmit = document.getElementById('master-key-unlock-submit');
        const mkVisitorSubmit = document.getElementById('master-key-unlock-visitor-submit');
        if (mkSkip) mkSkip.addEventListener('click', closeMasterKeyUnlockModal);

        async function runKeyUnlock(endpoint) {
            const input = document.getElementById('master-key-unlock-input');
            const errEl = document.getElementById('master-key-unlock-error');
            const raw = (input && input.value || '').trim();
            const pw = raw.toLowerCase();
            if (!pw) {
                if (errEl) {
                    errEl.textContent = 'Enter a key or choose Skip for now.';
                    errEl.style.display = 'block';
                }
                return;
            }
            if (errEl) errEl.style.display = 'none';
            if (mkSubmit) {
                mkSubmit.disabled = true;
                mkSubmit.textContent = 'Checking\u2026';
            }
            if (mkVisitorSubmit) {
                mkVisitorSubmit.disabled = true;
                mkVisitorSubmit.textContent = 'Checking\u2026';
            }
            try {
                const res = await fetch(endpoint, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ password: pw })
                });
                const data = await res.json().catch(() => ({}));
                if (!res.ok || !data.valid) {
                    if (errEl) {
                        errEl.textContent = data.detail || 'That key does not match. Try again or skip.';
                        errEl.style.display = 'block';
                    }
                    if (mkSubmit) {
                        mkSubmit.disabled = false;
                        mkSubmit.textContent = 'Unlock with Master Key';
                    }
                    if (mkVisitorSubmit) {
                        mkVisitorSubmit.disabled = false;
                        mkVisitorSubmit.textContent = 'Unlock as visitor';
                    }
                    if (input) {
                        input.value = '';
                        input.focus();
                    }
                    return;
                }
                closeMasterKeyUnlockModal();
                refreshDataImportMasterKeyAccessUI();
            } catch (e) {
                if (errEl) {
                    errEl.textContent = e.message || 'Request failed';
                    errEl.style.display = 'block';
                }
                if (mkSubmit) {
                    mkSubmit.disabled = false;
                    mkSubmit.textContent = 'Unlock with Master Key';
                }
                if (mkVisitorSubmit) {
                    mkVisitorSubmit.disabled = false;
                    mkVisitorSubmit.textContent = 'Unlock as visitor';
                }
            }
        }

        if (mkSubmit) {
            mkSubmit.addEventListener('click', () => void runKeyUnlock('/api/session/master-key/unlock'));
        }
        if (mkVisitorSubmit) {
            mkVisitorSubmit.addEventListener('click', () => void runKeyUnlock('/api/session/master-key/unlock-visitor'));
        }

        const visitorHintsModal = document.getElementById('visitor-key-hints-modal');
        const visitorHintsBtn = document.getElementById('master-key-unlock-show-hints');
        const visitorHintsClose = document.getElementById('visitor-key-hints-close');
        const visitorHintsList = document.getElementById('visitor-key-hints-list');
        const visitorHintsEmpty = document.getElementById('visitor-key-hints-empty');
        const visitorHintsErr = document.getElementById('visitor-key-hints-error');
        const visitorHintsLoading = document.getElementById('visitor-key-hints-loading');

        function closeVisitorHintsModal() {
            if (visitorHintsModal) visitorHintsModal.style.display = 'none';
        }

        async function openVisitorHintsModal() {
            if (!visitorHintsModal || !visitorHintsList) return;
            visitorHintsModal.style.display = 'flex';
            if (visitorHintsErr) {
                visitorHintsErr.style.display = 'none';
                visitorHintsErr.textContent = '';
            }
            if (visitorHintsEmpty) visitorHintsEmpty.style.display = 'none';
            visitorHintsList.innerHTML = '';
            if (visitorHintsLoading) visitorHintsLoading.style.display = 'block';
            try {
                const res = await fetch('/api/session/visitor-key-hints');
                const data = await res.json().catch(() => ({}));
                if (!res.ok) {
                    throw new Error(data.detail || `HTTP ${res.status}`);
                }
                const hints = Array.isArray(data.hints) ? data.hints : [];
                if (visitorHintsLoading) visitorHintsLoading.style.display = 'none';
                if (hints.length === 0) {
                    if (visitorHintsEmpty) visitorHintsEmpty.style.display = 'block';
                    return;
                }
                hints.forEach((row) => {
                    const li = document.createElement('li');
                    li.style.marginBottom = '10px';
                    li.style.lineHeight = '1.4';
                    li.textContent = row.hint != null ? String(row.hint) : '';
                    visitorHintsList.appendChild(li);
                });
            } catch (e) {
                if (visitorHintsLoading) visitorHintsLoading.style.display = 'none';
                if (visitorHintsErr) {
                    visitorHintsErr.textContent = e.message || 'Failed to load hints';
                    visitorHintsErr.style.display = 'block';
                }
            }
        }

        if (visitorHintsBtn) visitorHintsBtn.addEventListener('click', () => { void openVisitorHintsModal(); });
        if (visitorHintsClose) visitorHintsClose.addEventListener('click', closeVisitorHintsModal);
        if (visitorHintsModal) {
            visitorHintsModal.addEventListener('click', (e) => {
                if (e.target === visitorHintsModal) closeVisitorHintsModal();
            });
        }

        window.onbeforeunload = () => { SSE.close(); };
    }
    return { init, processFormSubmit, processQuestionSubmit, processAnswerSubmit };
})();

App.init();
