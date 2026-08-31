'use strict';

window.__visitorAccess = window.__visitorAccess || { restricted: false };

function installVisitorFeatureGateCapture() {
    if (window.__visitorFeatureGateCaptureInstalled) return;
    window.__visitorFeatureGateCaptureInstalled = true;
    document.addEventListener(
        'click',
        function visitorFeatureGateCapture(e) {
            const t = e.target;
            const el = t && typeof t.closest === 'function' ? t.closest('[data-visitor-gate]') : null;
            if (!el) return;
            const va = window.__visitorAccess;
            if (!va || !va.restricted) return;
            const g = el.getAttribute('data-visitor-gate');
            let ok = true;
            if (g === 'messages_chat') ok = !!va.can_messages_chat;
            else if (g === 'emails') ok = !!va.can_emails;
            else if (g === 'contacts') ok = !!(va.can_contacts || va.can_relationships);
            else if (g === 'relationship') ok = !!va.can_relationships;
            else if (g === 'sensitive_private') ok = !!va.can_sensitive_private;
            else return;
            if (!ok) {
                e.preventDefault();
                e.stopPropagation();
                e.stopImmediatePropagation();
            }
        },
        true
    );
}

const AppActions = {
    ["showFBAlbumsOptions"]: () => Modals.FBAlbums.open(),    // showFBAlbumsOptions
    ["openGeoModal"]: () => { void Modals.Locations.open(); }, // showGeoMetadataOptions
    ["showEmailGallery"]: () => Modals.EmailGallery.open(), // showEmailGalleryOptions
    ["showImageGallery"]: () => Modals.ImageGallery.open(),
    ["listContacts"]: () => Modals.Contacts.open(),
    ["openIdentityWizard"]: () => Modals.IdentityProfileWizard && Modals.IdentityProfileWizard.open(),

};
window.customObject = AppActions; // Expose for Suggestions.json if it relies on global `customObject`


const App = (() => {
    const LOCAL_AI_USE_CONFIG_KEY = 'local_ai_use_enabled_v1';

    function parseLocalAIUseEnabledValue(raw) {
        if (raw == null) return true;
        const v = String(raw).trim().toLowerCase();
        return !(v === 'false' || v === '0' || v === 'no' || v === 'off');
    }

    async function fetchLocalAIUseEnabledFromServer() {
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) return true;
            const rows = await res.json();
            const row = Array.isArray(rows)
                ? rows.find((r) => r && r.key === LOCAL_AI_USE_CONFIG_KEY)
                : null;
            if (!row || row.value == null) return true;
            return parseLocalAIUseEnabledValue(row.value);
        } catch (_) {
            return true;
        }
    }

    function resolveLocalAIUseEnabled(serverValue) {
        const checkbox = document.getElementById('local-ai-use-enabled-checkbox');
        if (checkbox) return !!checkbox.checked;
        if (typeof LocalAiSetup !== 'undefined'
            && typeof LocalAiSetup.isUseEnabledForChat === 'function') {
            return LocalAiSetup.isUseEnabledForChat();
        }
        return parseLocalAIUseEnabledValue(serverValue);
    }

    function isLocalAIUseEnabledForChat(availability) {
        return resolveLocalAIUseEnabled(availability && availability.localai_use_enabled);
    }

    function effectiveLocalAIAvailable(availability) {
        if (!availability) return false;
        if (!isLocalAIUseEnabledForChat(availability)) return false;
        if (availability.localai_infrastructure_available === false) return false;
        return !!(availability.localai_infrastructure_available || availability.localai_available);
    }

    // availMap shape: { auto, localai, _order: [key,...], [key]: available }
    // _order lists hosted AI model keys in server sort_order (the dynamic replacement
    // for the old fixed gemini/claude/deepseek/openai set).
    function buildProviderAvailabilityMap(availability) {
        const models = Array.isArray(availability.models) ? availability.models : [];
        const map = {
            auto: !!availability.auto_available,
            localai: effectiveLocalAIAvailable(availability),
            _order: [],
        };
        models.forEach((m) => {
            if (!m || !m.key) return;
            map[m.key] = !!m.available;
            map._order.push(m.key);
        });
        return map;
    }

    function isHostedProviderKey(availMap, key) {
        return !!(availMap && availMap._order && availMap._order.indexOf(key) !== -1);
    }

    const LLM_PROVIDER_SELECT_IDS = [
        'llm-provider-select',
        'profiles-llm-provider-select',
        'interview-provider',
        'have-a-chat-provider-a',
        'have-a-chat-provider-b',
    ];

    function firstAvailableProvider(availMap, preferAuto) {
        if (preferAuto && availMap.auto) return 'auto';
        const order = availMap._order || [];
        for (const key of order) {
            if (availMap[key]) return key;
        }
        if (availMap.localai) return 'localai';
        return preferAuto ? 'auto' : (order[0] || 'localai');
    }

    async function getLLMProviderAvailabilityPair() {
        try {
            const res = await fetch('/chat/availability', { credentials: 'same-origin' });
            if (!res.ok) return { _order: [], localai: false };
            const av = await res.json();
            if (typeof AIModelLabels !== 'undefined' && Array.isArray(av.models)) AIModelLabels.set(av.models);
            return buildProviderAvailabilityMap(av);
        } catch (e) {
            return { _order: [], localai: false };
        }
    }

    function getHostedFailoverCandidates(primary) {
        if (typeof Modals !== 'undefined'
            && Modals.AutoRoutingConfig
            && Modals.AutoRoutingConfig.getFailoverCandidates) {
            return Modals.AutoRoutingConfig.getFailoverCandidates(primary);
        }
        return [];
    }

    function isHostedProviderAvailable(name, av) {
        return !!(av && av[name]);
    }

    const LAST_HOSTED_LLM_PROVIDER_KEY = 'dm_last_hosted_llm_provider';

    function getLastManualHostedProvider() {
        try {
            const v = localStorage.getItem(LAST_HOSTED_LLM_PROVIDER_KEY);
            if (v && v !== 'auto' && v !== 'localai') return v;
        } catch (_) { /* ignore */ }
        return null;
    }

    function saveLastManualHostedProviderIfHosted(value) {
        if (value && value !== 'auto' && value !== 'localai') {
            try {
                localStorage.setItem(LAST_HOSTED_LLM_PROVIDER_KEY, value);
            } catch (_) { /* ignore */ }
        }
    }

    /** In-flight LLM chat request; Cancel button calls abortCurrentChatRequest(). */
    let chatRequestAbortController = null;

    function abortCurrentChatRequest() {
        if (chatRequestAbortController) {
            chatRequestAbortController.abort();
        }
    }

    /** Abort any stale request, then return a new controller for this chat round. */
    function beginChatRequestAbortable() {
        abortCurrentChatRequest();
        const ctrl = new AbortController();
        chatRequestAbortController = ctrl;
        return ctrl;
    }

    function endChatRequestAbortable(ctrl) {
        if (chatRequestAbortController === ctrl) {
            chatRequestAbortController = null;
        }
    }

    function isChatRequestInFlight() {
        return chatRequestAbortController !== null;
    }

    function getChatProviderDisplayLabel() {
        const sel = typeof DOM !== 'undefined' ? DOM.llmProviderSelect : null;
        if (!sel || sel.selectedIndex < 0) return '';
        const opt = sel.options[sel.selectedIndex];
        return opt && opt.textContent ? opt.textContent.trim() : '';
    }

    function notifyChatRequestCancelled() {
        if (typeof Chat === 'undefined' || !Chat.addMessage) return;
        const name = getChatProviderDisplayLabel();
        const msg = name ? `**Request cancelled** (${name}).` : '**Request cancelled.**';
        Chat.addMessage('cancelled', msg, true, null, null);
    }

    async function postJsonChatEndpoint(url, payload, signal) {
        let res;
        try {
            res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
                credentials: 'same-origin',
                signal,
            });
        } catch (e) {
            if (e && e.name === 'AbortError') {
                return { ok: false, aborted: true, error: 'Cancelled' };
            }
            return { ok: false, error: e && e.message ? String(e.message) : 'Network error' };
        }
        if (signal && signal.aborted) {
            return { ok: false, aborted: true, error: 'Cancelled' };
        }
        let data = null;
        try {
            data = await res.json();
        } catch (e) {
            if (e && e.name === 'AbortError') {
                return { ok: false, aborted: true, error: 'Cancelled' };
            }
            return { ok: false, error: 'Could not read response', httpStatus: res.status };
        }
        const detail = data && (data.detail || data.error);
        if (!res.ok) {
            return { ok: false, data, error: detail ? String(detail) : `HTTP ${res.status}`, httpStatus: res.status };
        }
        if (data && data.error) {
            return { ok: false, data, error: String(data.error) };
        }
        return { ok: true, data };
    }

    /** On LLM error, retry once with the other provider if it is available; update AI Provider selector on success.
     *  LocalAI is never used as an automatic failover target, and selecting it disables cloud failover
     *  (the user explicitly chose a local model and should not be silently routed to a cloud provider). */
    async function runChatWithProviderFailover(url, buildPayload, signal) {
        const select = typeof DOM !== 'undefined' ? DOM.llmProviderSelect : null;
        const primary = (select && select.value) ? select.value : '';
        if (typeof UI !== 'undefined' && UI.clearChatAutoRoutingNotice) {
            UI.clearChatAutoRoutingNotice();
        }
        const first = await postJsonChatEndpoint(url, buildPayload(primary), signal);
        if (first.aborted) {
            return { ok: false, aborted: true, error: 'Cancelled' };
        }
        if (first.ok) {
            return { ok: true, data: first.data, switched: false };
        }
        // Auto routing is handled server-side; do not client-failover away from Auto.
        if (primary === 'auto') {
            return { ok: false, error: first.error || 'Auto routing request failed', data: first.data };
        }
        // Never failover away from an explicitly chosen local provider.
        if (primary === 'localai') {
            return { ok: false, error: first.error || 'Local AI request failed', data: first.data };
        }
        const failoverEnabled = (typeof Modals !== 'undefined'
            && Modals.AutoRoutingConfig
            && Modals.AutoRoutingConfig.isFailoverEnabled)
            ? Modals.AutoRoutingConfig.isFailoverEnabled()
            : true;
        if (!failoverEnabled) {
            return { ok: false, error: first.error || 'Request failed', data: first.data };
        }
        const av = await getLLMProviderAvailabilityPair();
        if (signal && signal.aborted) {
            return { ok: false, aborted: true, error: 'Cancelled' };
        }
        const candidates = getHostedFailoverCandidates(primary);
        let lastError = first.error || 'Request failed';
        for (const alt of candidates) {
            if (alt === 'localai') continue;
            if (!isHostedProviderAvailable(alt, av)) continue;
            if (typeof UI !== 'undefined' && UI.setChatProviderFailoverNotice) {
                UI.setChatProviderFailoverNotice(primary, alt);
            }
            if (select) select.value = alt;
            if (typeof UI !== 'undefined' && UI.syncLoadingIndicatorProvider) {
                UI.syncLoadingIndicatorProvider();
            }
            if (signal && signal.aborted) {
                if (select) select.value = primary;
                if (typeof UI !== 'undefined' && UI.clearChatProviderFailoverNotice) {
                    UI.clearChatProviderFailoverNotice();
                }
                return { ok: false, aborted: true, error: 'Cancelled' };
            }
            const attempt = await postJsonChatEndpoint(url, buildPayload(alt), signal);
            if (attempt.aborted) {
                if (select) select.value = primary;
                if (typeof UI !== 'undefined' && UI.clearChatProviderFailoverNotice) {
                    UI.clearChatProviderFailoverNotice();
                }
                return { ok: false, aborted: true, error: 'Cancelled' };
            }
            if (attempt.ok) {
                return { ok: true, data: attempt.data, switched: true };
            }
            lastError = attempt.error || lastError;
        }
        if (select) select.value = primary;
        if (typeof UI !== 'undefined' && UI.clearChatProviderFailoverNotice) {
            UI.clearChatProviderFailoverNotice();
        }
        return { ok: false, error: lastError, data: first.data };
    }

    /** Shows the "API Key Required" warning modal; resolves true if the user clicked
     *  "Go to Configuration" (which also opens it there), false on Cancel/close. */
    function showApiKeyRequiredDialog(message) {
        return new Promise((resolve) => {
            const modal = document.getElementById('api-key-required-modal');
            const msgEl = document.getElementById('api-key-required-modal-message');
            const goBtn = document.getElementById('api-key-required-goto-btn');
            const cancelBtn = document.getElementById('api-key-required-cancel-btn');
            const closeBtn = document.getElementById('api-key-required-close-btn');
            if (!modal || !goBtn || !cancelBtn) { resolve(false); return; }
            if (msgEl) {
                msgEl.textContent = message
                    || 'This action needs an OpenRouter API key, which hasn’t been configured yet.';
            }
            modal.style.display = 'flex';
            function cleanup() {
                modal.style.display = 'none';
                goBtn.removeEventListener('click', onGo);
                cancelBtn.removeEventListener('click', onCancel);
                if (closeBtn) closeBtn.removeEventListener('click', onCancel);
            }
            function onGo() {
                cleanup();
                if (typeof window.openConfigurationToTab === 'function') window.openConfigurationToTab('api-keys');
                resolve(true);
            }
            function onCancel() {
                cleanup();
                resolve(false);
            }
            goBtn.addEventListener('click', onGo);
            cancelBtn.addEventListener('click', onCancel);
            if (closeBtn) closeBtn.addEventListener('click', onCancel);
        });
    }

    /** Returns true (and shows the warning dialog) when the currently selected chat
     *  provider needs an OpenRouter key that has not been configured — callers should
     *  bail out immediately without sending anything. Local AI never requires a key. */
    async function blockIfChatProviderNotReady() {
        const select = typeof DOM !== 'undefined' ? DOM.llmProviderSelect : null;
        const provider = (select && select.value) ? select.value : 'auto';
        if (provider === 'localai') return false;
        const av = await getLLMProviderAvailabilityPair();
        const ready = provider === 'auto'
            ? !!(av.auto || av.localai)
            : isHostedProviderAvailable(provider, av);
        if (ready) return false;
        await showApiKeyRequiredDialog(
            'This action needs an OpenRouter API key, which hasn’t been configured yet. '
            + 'Add one in Configuration → API Keys, or switch to Local AI in the provider list.'
        );
        return true;
    }

    async function processFormSubmit(userPrompt, category = null, title = null, supplementary_prompt = null) {
        if (!userPrompt && !category && !title) return;
        if (await blockIfChatProviderNotReady()) return;

 //       Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();
            const chatCtrl = beginChatRequestAbortable();

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

                const result = await runChatWithProviderFailover(CONSTANTS.API_PATHS.CHAT, (provider) => ({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: DOM.companionModeCheckbox ? DOM.companionModeCheckbox.checked : false,
                    allowExplicitContent: DOM.allowExplicitContentCheckbox ? DOM.allowExplicitContentCheckbox.checked : false,
                    enableSnarkiness: DOM.enableSnarkinessCheckbox ? DOM.enableSnarkinessCheckbox.checked : false,
                    supplementary_prompt: supplementary_prompt,
                    temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
                    conversation_id: conversationId,
                    clientId: AppState.clientId,
                    userId: currentUserId,
                    provider,
                    last_manual_hosted_provider: getLastManualHostedProvider(),
                    whos_asking: whosAsking,
                    request_only: DOM.requestOnlyCheckbox ? DOM.requestOnlyCheckbox.checked : false,
                }), chatCtrl.signal);

                if (!result.ok) {
                    if (!result.aborted) {
                        UI.displayError(result.error || 'Request failed.');
                    } else {
                        if (DOM.userInput) DOM.userInput.value = userPrompt;
                        notifyChatRequestCancelled();
                    }
                } else {
                    const data = result.data;
                    Chat.addMessage('assistant', data.response, true, null, data.embedded_json);
                    if (typeof UI !== 'undefined' && UI.setChatLastRequestStatsFromEmbedded) {
                        UI.setChatLastRequestStatsFromEmbedded(data.embedded_json);
                    }
                    if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.onUserMessageSent) {
                        ChatInactivityNudge.onUserMessageSent();
                    }
                }

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                endChatRequestAbortable(chatCtrl);
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }
    async function processQuestionSubmit() {
        
        var userPrompt = CONSTANTS.RANDOM_QUESTION_PROMPT;
        var title = "Generate a random question about " + CONSTANTS.OWNER_NAME + "'s life.";

        if (!userPrompt && !category && !title) return;
        if (await blockIfChatProviderNotReady()) return;

        // Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();
            const chatCtrl = beginChatRequestAbortable();

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

                const result = await runChatWithProviderFailover(CONSTANTS.API_PATHS.RANDOM_QUESTION, (provider) => ({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: false,
                    allowExplicitContent: DOM.allowExplicitContentCheckbox ? DOM.allowExplicitContentCheckbox.checked : false,
                    enableSnarkiness: DOM.enableSnarkinessCheckbox ? DOM.enableSnarkinessCheckbox.checked : false,
                    provider,
                    whos_asking: whosAsking,
                }), chatCtrl.signal);

                if (!result.ok) {
                    if (!result.aborted) {
                        UI.displayError(result.error || 'Request failed.');
                    } else {
                        notifyChatRequestCancelled();
                    }
                } else {
                    const data = result.data;
                    Chat.addMessage('assistant', data.response, true, null, data.embedded_json);
                    if (typeof UI !== 'undefined' && UI.setChatLastRequestStatsFromEmbedded) {
                        UI.setChatLastRequestStatsFromEmbedded(data.embedded_json);
                    }
                }

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                endChatRequestAbortable(chatCtrl);
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }
    async function processAnswerSubmit(userPrompt) {
        if (await blockIfChatProviderNotReady()) return;

        // Check and show reference documents notification before proceeding
        await Modals.ReferenceDocumentsNotification.checkAndShow(async () => {
            // This callback is called when user proceeds
            UI.clearError();
            UI.setControlsEnabled(false);
            UI.showLoadingIndicator();
            const chatCtrl = beginChatRequestAbortable();

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

                const result = await runChatWithProviderFailover(CONSTANTS.API_PATHS.CHAT, (provider) => ({
                    prompt: finalMessage,
                    voice: selectedVoice,
                    mood: selectedMood,
                    companionMode: false,
                    allowExplicitContent: DOM.allowExplicitContentCheckbox ? DOM.allowExplicitContentCheckbox.checked : false,
                    enableSnarkiness: DOM.enableSnarkinessCheckbox ? DOM.enableSnarkinessCheckbox.checked : false,
                    temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
                    conversation_id: conversationId,
                    clientId: AppState.clientId,
                    userId: currentUserId,
                    provider,
                    whos_asking: whosAsking,
                    repeat_question: true,
                }), chatCtrl.signal);

                if (!result.ok) {
                    if (!result.aborted) {
                        UI.displayError(result.error || 'Request failed.');
                    } else {
                        if (DOM.userInput) DOM.userInput.value = userPrompt;
                        notifyChatRequestCancelled();
                    }
                } else {
                    const data = result.data;
                    Chat.addMessage('assistant', data.response, true, null, data.embedded_json);
                    if (typeof UI !== 'undefined' && UI.setChatLastRequestStatsFromEmbedded) {
                        UI.setChatLastRequestStatsFromEmbedded(data.embedded_json);
                    }
                }

            } catch (error) {
                console.error('Form submit error:', error);
                UI.displayError(error.message || 'An unknown error occurred.');
                // UI.hideLoadingIndicator(); // Already handled in displayError or finally
            } finally {
                endChatRequestAbortable(chatCtrl);
                UI.setControlsEnabled(true);
                UI.hideLoadingIndicator(); // Ensure it's hidden
            }
        });
    }

    /** Cached owner master unlock; updated by refreshDataImportMasterKeyAccessUI. */
    let dataImportMasterUnlockedCached = null;

    /** Sidebar Import button is only shown after refreshDataImportMasterKeyAccessUI confirms owner master unlock. */
    function isDataImportSidebarMasterUnlockVisible() {
        const btns = [
            document.getElementById('data-sources-import-sidebar-btn'),
            document.getElementById('data-import-sidebar-btn'),
        ].filter(Boolean);
        if (!btns.length) return false;
        return btns.some((btn) => window.getComputedStyle(btn).display !== 'none');
    }

    function dataImportModalRoots() {
        return ['data-import-modal', 'data-sources-import-modal']
            .map((id) => document.getElementById(id))
            .filter(Boolean);
    }

    function hideAllDataImportModals() {
        dataImportModalRoots().forEach((m) => { m.style.display = 'none'; });
    }

    /** True only when the owner master key was used for this session (visitor unlock does not count). */
    async function fetchMasterUnlockedForDataImport() {
        try {
            const st = await fetch('/api/session/master-key/status', { credentials: 'same-origin' });
            if (!st.ok) {
                dataImportMasterUnlockedCached = false;
                return false;
            }
            const sj = await st.json();
            dataImportMasterUnlockedCached = !!sj.master_unlocked;
            return dataImportMasterUnlockedCached;
        } catch (e) {
            dataImportMasterUnlockedCached = false;
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
        if (dataImportMasterUnlockedCached === true) return true;
        if (isDataImportSidebarMasterUnlockVisible()) {
            dataImportMasterUnlockedCached = true;
            return true;
        }
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
        } else if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
            await AppDialogs.showAppAlert('Unlock with the owner master key before opening Data Import (visitor key is not sufficient).');
        }
        return false;
    }

    function refreshDataImportMasterKeyAccessUI() {
        void (async () => {
            const [masterOk, keyringUnlocked] = await Promise.all([
                fetchMasterUnlockedForDataImport(),
                fetchSessionKeyringUnlocked(),
            ]);
            dataImportMasterUnlockedCached = masterOk;
            const sidebarBtns = [
                document.getElementById('data-sources-import-sidebar-btn'),
                document.getElementById('data-import-sidebar-btn'),
            ].filter(Boolean);
            const sensitiveSidebarBtn = document.getElementById('sensitive-data-sidebar-btn');
            const tiles = document.querySelectorAll('.import-data-dialog-tile[data-open-modal="data-import-modal"]');
            sidebarBtns.forEach((sidebarBtn) => {
                sidebarBtn.style.display = masterOk ? '' : 'none';
                sidebarBtn.disabled = false;
                sidebarBtn.title = masterOk ? '' : 'Owner master key required — use Unlock with Master Key (visitor key is not enough).';
                sidebarBtn.style.opacity = '';
                sidebarBtn.style.cursor = '';
            });
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

            // Settings & Data Import modal: without owner master unlock, only Appearance and API Keys tabs are available.
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
        if (typeof CONSTANTS !== 'undefined' && CONSTANTS.DEPLOYMENT_NATURE_LOCAL !== 'True') {
            document.body.classList.add('deployment-hide-path-import-tiles');
        }

        function refreshSettingsDataImportModalLLM() {
            if (typeof Modals !== 'undefined' && Modals.UserLLMSettings && Modals.UserLLMSettings.load) {
                void Modals.UserLLMSettings.load();
            }
            void loadLLMProviderAvailability();
        }
        window.refreshSettingsDataImportModalLLM = refreshSettingsDataImportModalLLM;

        /** Set by loadArchiveOverviewLLMStatus when /auth/me succeeded and no LLM provider is available. */
        let archiveOverviewMeForKeys = null;

        DOM.chatForm.addEventListener('submit', (event) => {
            event.preventDefault();
            if (typeof InterviewerMode !== 'undefined' && InterviewerMode.isActive()) return;
            const userPrompt = DOM.userInput.value.trim();
            if (!userPrompt) return;
            processFormSubmit(userPrompt);
        });

        if (DOM.loadingIndicatorCancelBtn) {
            DOM.loadingIndicatorCancelBtn.addEventListener('click', (e) => {
                e.preventDefault();
                abortCurrentChatRequest();
            });
        }
        
        DOM.userInput.addEventListener('keydown', (event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                // dispatchEvent on form seems more robust than calling submit() directly
                DOM.chatForm.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }));
            }
        });

        const chatMessagesVisToggle = document.getElementById('chat-messages-visibility-toggle');
        const chatBoxWrapper = document.querySelector('.chat-box-wrapper');
        if (chatMessagesVisToggle && chatBoxWrapper && DOM.chatBox) {
            const icon = chatMessagesVisToggle.querySelector('i');
            const applyMessagesHidden = (hidden) => {
                chatBoxWrapper.classList.toggle('chat-pane-messages-hidden', hidden);
                chatMessagesVisToggle.setAttribute('aria-pressed', hidden ? 'true' : 'false');
                chatMessagesVisToggle.title = hidden ? 'Show messages' : 'Hide messages to see background';
                chatMessagesVisToggle.setAttribute('aria-label', hidden ? 'Show chat messages' : 'Hide chat messages');
                if (icon) {
                    icon.className = hidden ? 'fas fa-eye' : 'fas fa-eye-slash';
                }
            };
            chatMessagesVisToggle.addEventListener('click', () => {
                applyMessagesHidden(!chatBoxWrapper.classList.contains('chat-pane-messages-hidden'));
            });
            applyMessagesHidden(false);
        }

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
                refreshSettingsDataImportModalLLM();
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
        if (DOM.clearVoiceConversationHistoryBtn) {
            DOM.clearVoiceConversationHistoryBtn.addEventListener('click', async () => {
                if (Modals.ConversationManager && Modals.ConversationManager.clearCurrentConversationHistoryWithConfirm) {
                    await Modals.ConversationManager.clearCurrentConversationHistoryWithConfirm();
                }
            });
        }

        // Guide modal
        const guideModal = document.getElementById('guide-modal');
        const guideSidebarBtn = document.getElementById('guide-sidebar-btn');
        const closeGuideModalBtn = document.getElementById('close-guide-modal');
        if (guideSidebarBtn && guideModal) {
            guideSidebarBtn.addEventListener('click', () => {
                const topicsContainer = document.getElementById('guide-topics-list');
                if (typeof Guide !== 'undefined' && Guide.renderTopicList && topicsContainer && !topicsContainer.querySelector('.guide-topic-btn')) {
                    try { Guide.renderTopicList(); } catch (_) {}
                }
                if (typeof Guide !== 'undefined' && Guide.openGuideModal) Guide.openGuideModal();
                else guideModal.style.display = 'flex';
            });
        }
        const closeGuideModal = () => {
            if (typeof Guide !== 'undefined' && Guide.closeAll) Guide.closeAll();
            else guideModal.style.display = 'none';
        };
        if (closeGuideModalBtn && guideModal) {
            closeGuideModalBtn.addEventListener('click', closeGuideModal);
        }
        if (guideModal) {
            guideModal.addEventListener('click', (e) => {
                if (e.target === guideModal) closeGuideModal();
            });
        }
        if (guideModal) {
            guideModal.addEventListener('click', (e) => {
                const btn = e.target && e.target.closest ? e.target.closest('.guide-topic-btn') : null;
                if (!btn) return;
                e.stopPropagation();
                const topic = btn.dataset.topic || '';
                if (typeof Guide !== 'undefined' && Guide.onTopicSelected) {
                    Guide.onTopicSelected(topic, btn);
                }
            });
        }

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
                
                // Load subject configuration when Subject Configuration tab is opened
                if (targetTab === 'subject-configuration') {
                    if (Modals.SubjectConfiguration && Modals.SubjectConfiguration.loadAndPopulateForm) {
                        Modals.SubjectConfiguration.loadAndPopulateForm();
                    }
                    if (Modals.Interests && Modals.Interests.load) Modals.Interests.load();
                }
                if (targetTab === 'custom-voices') {
                    if (Modals.CustomVoices && Modals.CustomVoices.load) Modals.CustomVoices.load();
                }
                if (targetTab === 'tools-access') {
                    if (Modals.LLMToolsAccess && Modals.LLMToolsAccess.load) void Modals.LLMToolsAccess.load();
                }
                if (targetTab === 'tool-test') {
                    if (Modals.ToolTest && Modals.ToolTest.load) void Modals.ToolTest.load();
                }
                if (targetTab === 'regions-config') {
                    if (Modals.RegionsConfig && Modals.RegionsConfig.load) void Modals.RegionsConfig.load();
                }
                if (targetTab === 'suggestions-config') {
                    if (Modals.SuggestionsConfig && Modals.SuggestionsConfig.load) void Modals.SuggestionsConfig.load();
                }
                if (targetTab === 'guide-topics-config') {
                    if (Modals.GuideTopicsConfig && Modals.GuideTopicsConfig.load) void Modals.GuideTopicsConfig.load();
                }
                if (targetTab === 'ai-setup-config') {
                    if (typeof LocalAiSetup !== 'undefined' && LocalAiSetup.refresh) void LocalAiSetup.refresh();
                }
                if (targetTab === 'settings' || targetTab === 'api-keys') {
                    if (Modals.UserLLMSettings && Modals.UserLLMSettings.load) void Modals.UserLLMSettings.load();
                    if (targetTab === 'settings') {
                        void loadLLMProviderAvailability();
                        if (Modals.InactivityPromptConfig && Modals.InactivityPromptConfig.load) {
                            void Modals.InactivityPromptConfig.load();
                        }
                    }
                }
                if (targetTab === 'ai-models-config') {
                    if (Modals.AIModelsConfig && Modals.AIModelsConfig.load) {
                        void Modals.AIModelsConfig.load();
                    }
                }
                if (targetTab === 'model-catalog') {
                    if (Modals.ModelCatalog && Modals.ModelCatalog.load) {
                        void Modals.ModelCatalog.load();
                    }
                }
            });
        });

        function openReferenceDocumentsManageModalFromUI() {
            const modal = document.getElementById('reference-documents-manage-modal');
            if (!modal) return;
            modal.style.display = 'flex';
            hideAllDataImportModals();
            if (Modals.ReferenceDocuments && Modals.ReferenceDocuments.loadDocuments) {
                Modals.ReferenceDocuments.loadDocuments();
            }
        }

        async function openConfigurationToTab(tabName) {
            if (!DOM.configPage) return;
            DOM.configPage.style.display = 'flex';
            if (typeof loadControlDefaults === 'function') {
                await loadControlDefaults();
            }
            const tabBtn = document.querySelector(`.config-tab-button[data-tab="${tabName}"]`);
            if (tabBtn) tabBtn.click();
            if (typeof refreshSettingsDataImportModalLLM === 'function') refreshSettingsDataImportModalLLM();
        }
        window.openConfigurationToTab = openConfigurationToTab;

        async function onChatStatusBarConfigClick(tabName) {
            if (!(await fetchMasterUnlockedForDataImport())) {
                await ensureMasterKeyForDataImport();
                return;
            }
            await openConfigurationToTab(tabName);
        }

        async function onChatStatusBarRefsClick() {
            if (!(await fetchMasterUnlockedForDataImport())) {
                await ensureMasterKeyForDataImport();
                return;
            }
            openReferenceDocumentsManageModalFromUI();
        }

        const chatStatusToolsSeg = document.getElementById('chat-context-status-tools-seg');
        const chatStatusRefsSeg = document.getElementById('chat-context-status-refs-seg');
        if (chatStatusToolsSeg) {
            chatStatusToolsSeg.addEventListener('click', (e) => {
                e.preventDefault();
                void onChatStatusBarConfigClick('tools-access');
            });
            chatStatusToolsSeg.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    void onChatStatusBarConfigClick('tools-access');
                }
            });
        }
        if (chatStatusRefsSeg) {
            chatStatusRefsSeg.addEventListener('click', (e) => {
                e.preventDefault();
                void onChatStatusBarRefsClick();
            });
            chatStatusRefsSeg.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    void onChatStatusBarRefsClick();
                }
            });
        }
        if (typeof UI !== 'undefined' && UI.initChatToolCallsLogModal) {
            UI.initChatToolCallsLogModal();
        }
        if (typeof UI !== 'undefined' && UI.initChatAutoRoutingReasonModal) {
            UI.initChatAutoRoutingReasonModal();
        }
        if (typeof UI !== 'undefined' && UI.initOpenRouterCreditsClick) {
            UI.initOpenRouterCreditsClick();
        }

        // Dashboard: load stats and render. prefix e.g. 'stats-' for Dashboard modal (ids: prefix + 'dashboard-stats', …).
        async function loadDashboard(prefix) {
            prefix = prefix || '';
            const container = document.getElementById(prefix + 'dashboard-stats');
            const loadingEl = document.getElementById(prefix + 'dashboard-loading');
            const errorEl = document.getElementById(prefix + 'dashboard-load-error');
            if (!container) return;
            if (loadingEl) loadingEl.style.display = 'inline';
            if (errorEl) { errorEl.style.display = 'none'; errorEl.textContent = ''; }
            try {
                const response = await fetch('/api/dashboard', { credentials: 'same-origin' });
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

        function openDashboardModal() {
            if (!DOM.statisticsModal) return;
            DOM.statisticsModal.style.display = 'flex';
            void loadDashboard('stats-');
        }
        window.openDashboardModal = openDashboardModal;

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

        function regionDisplayName(key) {
            return Regions.label(key);
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
            const ebs = data.emails_by_source && typeof data.emails_by_source === 'object' ? data.emails_by_source : {};
            const gmailN = ebs.gmail != null ? Number(ebs.gmail) : 0;
            let otherN = 0;
            Object.entries(ebs).forEach(([k, v]) => {
                if (k !== 'gmail') otherN += v != null ? Number(v) : 0;
            });
            const emailLabelExtra = Object.keys(ebs).length
                ? ` — Gmail ${gmailN.toLocaleString()}, other ${otherN.toLocaleString()}`
                : '';
            add('Total Emails' + emailLabelExtra, data.emails_count);
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

        async function loadArchiveOverviewLLMStatus() {
            const body = document.getElementById('overview-llm-status-body');
            if (!body) return;
            function esc(s) {
                return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
            }
            function row2(leftHtml, rightHtml) {
                return `<div style="display:grid;grid-template-columns:minmax(160px,auto) 1fr;gap:8px 16px;margin-bottom:8px;align-items:start;">${leftHtml}${rightHtml}</div>`;
            }
            body.innerHTML = '<span style="color:#64748b;">Loading…</span>';
            try {
                const [avRes, meRes] = await Promise.all([
                    fetch('/chat/availability', { credentials: 'same-origin' }),
                    fetch('/auth/me', { credentials: 'same-origin' })
                ]);
                let av = {};
                if (avRes.ok) {
                    try { av = await avRes.json(); } catch (_) { /* ignore */ }
                }
                let me = null;
                if (meRes.ok) {
                    try { me = await meRes.json(); } catch (_) { /* ignore */ }
                }
                const models = Array.isArray(av.models) ? av.models : [];
                const lOk = effectiveLocalAIAvailable(av);
                const anyHostedOk = models.some((m) => m && m.available);
                const localVal = lOk
                    ? '<strong style="color:#15803d;">Ready</strong>'
                    : '<strong style="color:#b91c1c;">Not Available</strong>';
                const parts = [];
                models.forEach((m) => {
                    if (!m || !m.key) return;
                    const val = m.available
                        ? '<strong style="color:#15803d;">Ready</strong>'
                        : '<strong style="color:#b91c1c;">Not available</strong> — configure an OpenRouter API key';
                    parts.push(row2(`<span style="color:#64748b;">${esc(m.display_name || m.key)}</span>`, `<span>${val}</span>`));
                });
                parts.push(row2('<span style="color:#64748b;">Local AI</span>', `<span>${localVal}</span>`));
                if (me && me.llm_settings) {
                    const ls = me.llm_settings;
                    const sess = !!ls.session_scoped;
                    const tavVal = ls.tavily_api_key_set
                        ? '<strong style="color:#15803d;">Key set</strong> — search tool can use your Tavily key' + (sess ? ' <span style="color:#64748b;font-weight:normal;">(this session)</span>' : '')
                        : '<span style="color:#64748b;">No personal Tavily key in Settings — server default or none</span>';
                    parts.push(row2('<span style="color:#64748b;">Tavily (web search)</span>', `<span>${tavVal}</span>`));
                    const runpodVal = ls.runpod_api_key_set
                        ? '<strong style="color:#15803d;">Key set</strong> — RunPod image classification can use your API key' + (sess ? ' <span style="color:#64748b;font-weight:normal;">(this session)</span>' : '')
                        : '<span style="color:#64748b;">No personal RunPod key in Settings — server default or none</span>';
                    parts.push(row2('<span style="color:#64748b;">RunPod</span>', `<span>${runpodVal}</span>`));
                    const elVal = ls.elevenlabs_api_key_set
                        ? '<strong style="color:#15803d;">Key set</strong> — ElevenLabs integrations can use your API key' + (sess ? ' <span style="color:#64748b;font-weight:normal;">(this session)</span>' : '')
                        : '<span style="color:#64748b;">No personal ElevenLabs key in Settings — server default or none</span>';
                    parts.push(row2('<span style="color:#64748b;">ElevenLabs</span>', `<span>${elVal}</span>`));
                } else if (meRes.status === 401 || !meRes.ok) {
                    parts.push('<p style="margin:12px 0 0;font-size:0.85rem;color:#64748b;">Sign in to save personal API keys under <strong>Settings → Your API keys &amp; models</strong>.</p>');
                }
                body.innerHTML = parts.join('');
                const wrap = document.getElementById('overview-llm-keys-button-wrap');
                const lbl = document.getElementById('overview-llm-keys-open-btn-label');
                archiveOverviewMeForKeys = null;
                if (wrap) {
                    if (!anyHostedOk && !lOk && me) {
                        wrap.style.display = 'block';
                        archiveOverviewMeForKeys = me;
                        if (lbl) {
                            lbl.textContent = (me.llm_settings && me.llm_settings.session_scoped)
                                ? 'Add keys for this session'
                                : 'Add your API keys';
                        }
                    } else {
                        wrap.style.display = 'none';
                    }
                }
            } catch (e) {
                body.textContent = 'Could not load AI configuration status.';
                archiveOverviewMeForKeys = null;
                const wrap = document.getElementById('overview-llm-keys-button-wrap');
                if (wrap) wrap.style.display = 'none';
            }
        }

        function setupArchiveOverviewLLMKeysModal() {
            const modal = document.getElementById('archive-overview-llm-modal');
            const openBtn = document.getElementById('overview-llm-keys-open-btn');
            const cancelBtn = document.getElementById('archive-overview-llm-cancel-btn');
            const saveBtn = document.getElementById('archive-overview-llm-save-btn');
            if (!modal || !openBtn) return;

            function closeArchiveOverviewLLMModal() {
                modal.style.display = 'none';
                const st = document.getElementById('archive-overview-llm-save-status');
                if (st) st.textContent = '';
            }

            function openArchiveOverviewLLMModal() {
                const me = archiveOverviewMeForKeys;
                if (!me) return;
                const ls = me.llm_settings || {};
                const sess = !!ls.session_scoped;
                const introEl = document.getElementById('archive-overview-llm-modal-intro');
                const titleEl = document.getElementById('archive-overview-llm-modal-title');
                if (titleEl) titleEl.textContent = sess ? 'API keys for this visit' : 'Add your API keys';
                if (introEl) {
                    introEl.textContent = sess
                        ? 'Keys are stored on this browser session only and clear when the session ends. Enter an OpenRouter API key to use chat.'
                        : 'Keys are saved to your account. Enter an OpenRouter API key when no server key is available for you.';
                }
                const ork = document.getElementById('overview-llm-openrouter-key');
                const tk = document.getElementById('overview-llm-tavily-key');
                const rk = document.getElementById('overview-llm-runpod-key');
                const elK = document.getElementById('overview-llm-elevenlabs-key');
                if (ork) {
                    ork.value = '';
                    ork.placeholder = sess ? 'Leave blank to use owner or server default' : 'Paste key to save';
                }
                if (tk) {
                    tk.value = '';
                    tk.placeholder = 'Optional — Tavily web search';
                }
                if (rk) {
                    rk.value = '';
                    rk.placeholder = 'Optional — RunPod image AI';
                }
                if (elK) {
                    elK.value = '';
                    elK.placeholder = 'Optional — ElevenLabs speech';
                }
                const st = document.getElementById('archive-overview-llm-save-status');
                if (st) st.textContent = '';
                modal.style.display = 'flex';
            }

            openBtn.addEventListener('click', openArchiveOverviewLLMModal);
            if (cancelBtn) cancelBtn.addEventListener('click', closeArchiveOverviewLLMModal);
            modal.addEventListener('click', (e) => {
                if (e.target === modal) closeArchiveOverviewLLMModal();
            });

            if (saveBtn) {
                saveBtn.addEventListener('click', async () => {
                    const st = document.getElementById('archive-overview-llm-save-status');
                    const orkEl = document.getElementById('overview-llm-openrouter-key');
                    const tkEl = document.getElementById('overview-llm-tavily-key');
                    const rkEl = document.getElementById('overview-llm-runpod-key');
                    const elKEl = document.getElementById('overview-llm-elevenlabs-key');
                    const ork = (orkEl && orkEl.value.trim()) || '';
                    const tk = (tkEl && tkEl.value.trim()) || '';
                    const rp = (rkEl && rkEl.value.trim()) || '';
                    const elK = (elKEl && elKEl.value.trim()) || '';
                    if (!ork) {
                        if (st) {
                            st.textContent = 'Enter an OpenRouter API key.';
                            st.style.color = '#b91c1c';
                        }
                        return;
                    }
                    const body = {};
                    body.openrouter_api_key = ork;
                    if (tk) body.tavily_api_key = tk;
                    if (rp) body.runpod_api_key = rp;
                    if (elK) body.elevenlabs_api_key = elK;
                    saveBtn.disabled = true;
                    if (st) {
                        st.textContent = 'Saving…';
                        st.style.color = '#64748b';
                    }
                    try {
                        const res = await fetch('/auth/me/llm-settings', {
                            method: 'PATCH',
                            headers: { 'Content-Type': 'application/json' },
                            credentials: 'same-origin',
                            body: JSON.stringify(body),
                        });
                        if (!res.ok) {
                            let detail = await res.text();
                            try {
                                const j = JSON.parse(detail);
                                if (j && j.error) detail = j.error;
                            } catch (_) { /* keep */ }
                            throw new Error(detail || res.statusText);
                        }
                        closeArchiveOverviewLLMModal();
                        void loadArchiveOverviewLLMStatus();
                        void loadLLMProviderAvailability();
                        refreshSettingsDataImportModalLLM();
                    } catch (err) {
                        if (st) {
                            st.textContent = err.message || 'Save failed';
                            st.style.color = '#b91c1c';
                        }
                    } finally {
                        saveBtn.disabled = false;
                    }
                });
            }
        }
        setupArchiveOverviewLLMKeysModal();

        let importModalContentLoadGen = 0;

        function showDataImportModalLoading(show) {
            ['data-import-modal-loading', 'data-sources-import-modal-loading'].forEach((id) => {
                const loadingEl = document.getElementById(id);
                if (loadingEl) {
                    loadingEl.hidden = !show;
                    loadingEl.style.display = show ? 'flex' : 'none';
                }
            });
            dataImportModalRoots().forEach((modal) => {
                modal.setAttribute('aria-busy', show ? 'true' : 'false');
            });
        }

        function applyImportModalCounts(modalRoot, d) {
            if (!modalRoot || !d) return;
            const mc = d.message_counts || {};
            const n = (k) => {
                const v = mc[k];
                return v != null ? Number(v) : 0;
            };
            const imessageMsg = n('iMessage') + n('SMS') + n('MMS');
            const fbZipTotal = n('Facebook Messenger') + (d.facebook_albums_count || 0) + (d.facebook_posts_count || 0) + (d.locations_count || 0);
            const setCell = (key, value) => {
                const num = typeof value === 'number' && !Number.isNaN(value) ? value : Number(value);
                const safe = Number.isNaN(num) ? 0 : num;
                const isZero = safe === 0;
                modalRoot.querySelectorAll(`[data-import-count-key="${key}"]`).forEach((el) => {
                    el.textContent = String(safe);
                    const row = el.closest('.data-import-row');
                    if (row) {
                        row.classList.toggle('data-import-row-zero-count', isZero);
                    }
                });
            };
            const ebs = d.emails_by_source && typeof d.emails_by_source === 'object' ? d.emails_by_source : {};
            const gmailOnly = ebs.gmail != null ? Number(ebs.gmail) : 0;
            let imapLike = 0;
            Object.entries(ebs).forEach(([k, v]) => {
                if (k !== 'gmail') imapLike += v != null ? Number(v) : 0;
            });
            setCell('email_processing', gmailOnly);
            setCell('imap_processing', imapLike);
            setCell('zip_whatsapp', n('WhatsApp'));
            setCell('zip_instagram', n('Instagram'));
            setCell('zip_imessage', imessageMsg);
            setCell('zip_facebook', fbZipTotal);
            const hasFsSplit = Object.prototype.hasOwnProperty.call(d, 'filesystem_images_embedded_count')
                && Object.prototype.hasOwnProperty.call(d, 'filesystem_images_referenced_count');
            let uploadPhotosCount;
            let folderScanCount;
            if (hasFsSplit) {
                uploadPhotosCount = Number(d.filesystem_images_embedded_count) || 0;
                folderScanCount = Number(d.filesystem_images_referenced_count) || 0;
            } else {
                uploadPhotosCount = d.imported_images != null ? Number(d.imported_images) : 0;
                folderScanCount = d.reference_images != null ? Number(d.reference_images) : 0;
            }
            setCell('upload_photos', uploadPhotosCount);
            setCell('filesystem', folderScanCount);
            setCell('filesystem_reference', folderScanCount);
            setCell('imported_images', uploadPhotosCount);
            setCell('thumbnails', d.thumbnail_count != null ? Number(d.thumbnail_count) : 0);
            setCell('reference_docs', d.reference_docs_count != null ? Number(d.reference_docs_count) : 0);
            setCell('contacts', d.contacts_count != null ? Number(d.contacts_count) : 0);
            setCell('reference_import_entries', d.reference_images != null ? Number(d.reference_images) : 0);
            setCell('image_export_entries', d.total_images != null ? Number(d.total_images) : 0);
            setCell('gps_images', d.gps_images_count != null ? Number(d.gps_images_count) : 0);
        }

        function clearEmbeddingProgress(modalRoot) {
            if (!modalRoot) return;
            modalRoot.querySelectorAll('[data-progress-key]').forEach((line) => {
                line.textContent = '';
                line.removeAttribute('data-progress-state');
            });
            modalRoot.querySelectorAll('.data-import-nudge-target[data-import-start]').forEach((btn) => {
                btn.classList.remove('data-import-nudge');
            });
        }

        function applyImportModalStats(d, options) {
            if (!d) return;
            const opts = options && typeof options === 'object' ? options : {};
            const modalId = opts.modalId || null;
            const includeEmbeddingProgress = opts.includeEmbeddingProgress !== false;
            const targets = modalId
                ? [document.getElementById(modalId)].filter(Boolean)
                : dataImportModalRoots();
            targets.forEach((modalRoot) => {
                applyImportModalCounts(modalRoot, d);
                const isMaintenanceModal = modalRoot.id === 'data-import-modal';
                if (includeEmbeddingProgress && isMaintenanceModal) {
                    applyEmbeddingProgress(modalRoot, d);
                } else if (modalRoot.id === 'data-sources-import-modal') {
                    clearEmbeddingProgress(modalRoot);
                }
            });
        }

        const EMBEDDING_PROGRESS_LABELS = {
            message_context_embeddings: 'messages',
            email_embeddings: 'emails',
            facebook_post_text_embeddings: 'posts',
            facebook_album_description_embeddings: 'albums',
            image_tag_embeddings: 'tagged photos',
        };

        function applyEmbeddingProgress(modalRoot, d) {
            const progress = d && d.embedding_progress && typeof d.embedding_progress === 'object' ? d.embedding_progress : {};
            Object.keys(EMBEDDING_PROGRESS_LABELS).forEach((key) => {
                const entry = progress[key] || { total: 0, pending: 0 };
                const total = Number(entry.total) || 0;
                const pending = Number(entry.pending) || 0;
                const label = EMBEDDING_PROGRESS_LABELS[key];
                const lines = modalRoot.querySelectorAll(`[data-progress-key="${key}"]`);
                lines.forEach((line) => {
                    if (total === 0) {
                        line.textContent = '';
                        line.removeAttribute('data-progress-state');
                        return;
                    }
                    if (pending === 0) {
                        line.textContent = `Searchable: all ${total} ${label}`;
                        line.setAttribute('data-progress-state', 'done');
                    } else {
                        line.textContent = `Searchable: ${total - pending} of ${total} ${label}`;
                        line.setAttribute('data-progress-state', 'pending');
                    }
                });
                const nudgeBtn = modalRoot.querySelector(`.data-import-nudge-target[data-import-start="${key}"]`);
                if (nudgeBtn) {
                    nudgeBtn.classList.toggle('data-import-nudge', total > 0 && pending > 0);
                }
            });
        }

        async function loadDataImportModalContent(options) {
            const opts = options && typeof options === 'object' ? options : {};
            const modalId = opts.modalId || null;
            const includeEmbeddingProgress = opts.includeEmbeddingProgress !== false;
            const gen = ++importModalContentLoadGen;
            showDataImportModalLoading(true);
            try {
                const url = includeEmbeddingProgress
                    ? '/api/import-modal-stats'
                    : '/api/import-modal-stats?embedding_progress=0';
                const statsRes = await fetch(url, { credentials: 'same-origin' });
                if (gen !== importModalContentLoadGen) return;
                if (statsRes.ok) {
                    applyImportModalStats(await statsRes.json(), { modalId, includeEmbeddingProgress });
                }
            } catch (e) {
                console.warn('Failed to load import modal content:', e);
            } finally {
                if (gen === importModalContentLoadGen) showDataImportModalLoading(false);
            }
        }

        function reloadVisibleImportModalStats() {
            const sources = document.getElementById('data-sources-import-modal');
            const maintenance = document.getElementById('data-import-modal');
            if (sources && sources.style.display !== 'none') {
                void loadDataImportModalContent({ modalId: 'data-sources-import-modal', includeEmbeddingProgress: false });
            }
            if (maintenance && maintenance.style.display !== 'none') {
                void loadDataImportModalContent({ modalId: 'data-import-modal', includeEmbeddingProgress: true });
            }
        }

        function openDataMaintenanceModalUI() {
            const modal = document.getElementById('data-import-modal');
            if (!modal) return;
            modal.style.display = 'flex';
            if (typeof loadControlDefaults === 'function') void loadControlDefaults();
            if (typeof resetDataImportDetailSidebar === 'function') resetDataImportDetailSidebar();
            void loadDataImportModalContent({ modalId: 'data-import-modal', includeEmbeddingProgress: true });
        }

        function openDataSourcesImportModalUI() {
            const modal = document.getElementById('data-sources-import-modal');
            if (!modal) return;
            modal.style.display = 'flex';
            if (typeof markOnboardingChecklistStepDone === 'function') markOnboardingChecklistStepDone('import_data');
            if (typeof loadControlDefaults === 'function') void loadControlDefaults();
            if (typeof resetDataImportDetailSidebar === 'function') resetDataImportDetailSidebar();
            void loadDataImportModalContent({ modalId: 'data-sources-import-modal', includeEmbeddingProgress: false });
        }

        function openDataImportModalUI() {
            openDataMaintenanceModalUI();
        }

        async function openDataSourcesImportModalAfterKeyCheck() {
            if (!(await ensureMasterKeyForDataImport())) return false;
            openDataSourcesImportModalUI();
            return true;
        }

        async function openDataImportModalAfterKeyCheck() {
            if (!(await ensureMasterKeyForDataImport())) return false;
            openDataImportModalUI();
            return true;
        }

        async function loadDataImportModalCounts() {
            reloadVisibleImportModalStats();
        }

        // Empty Media Tables Button
        const emptyMediaTablesBtn = document.getElementById('empty-media-tables-btn');
        const emptyTablesStatus = document.getElementById('empty-tables-status');
        
        if (emptyMediaTablesBtn) {
            emptyMediaTablesBtn.addEventListener('click', async () => {
                const confirmed = await AppDialogs.showAppConfirm(
                    'Empty media tables',
                    'WARNING: This will permanently delete ALL data from:\n\n' +
                    '- attachments\n' +
                    '- media_blob\n' +
                    '- media_items\n' +
                    '- messages\n' +
                    '- message_attachments\n\n' +
                    'This action cannot be undone!\n\n' +
                    'Are you absolutely sure you want to continue?',
                    { danger: true }
                );
                
                if (!confirmed) {
                    return;
                }
                
                const doubleConfirmed = await AppDialogs.showAppConfirm(
                    'Final warning',
                    'FINAL WARNING: This will DELETE ALL messages and media data.\n\n',
                    { danger: true }
                );
                
                if (!doubleConfirmed) {
                    return;
                }
                const userInput = "DELETE";
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

        const importInputModal = document.getElementById('import-input-modal');
        const importInputModalTitle = document.getElementById('import-input-modal-title');
        const importInputModalBody = document.getElementById('import-input-modal-body');
        const importInputModalCancel = document.getElementById('import-input-modal-cancel');
        const importInputModalSubmit = document.getElementById('import-input-modal-submit');

        /** @type {Map<string, { importType: string, zipArchiveType: string|null, label: string, lines: string[], eventSource: EventSource|null, cancelPending?: boolean }>} */
        const runningJobs = new Map();
        let selectedJobKey = null;
        let sidebarBackgroundJobsRunning = false;
        let sidebarBackgroundJobsPollTimer = null;

        function syncOneDataImportSidebarJobIndicator(btnId, idleIconClass) {
            const btn = document.getElementById(btnId);
            if (!btn) return;
            const icon = btn.querySelector('.sidebar-data-import-btn-icon');
            const active = runningJobs.size > 0 || sidebarBackgroundJobsRunning;
            btn.classList.toggle('sidebar-jobs-running', active);
            btn.title = active ? 'Import or maintenance jobs running' : '';
            if (!icon) return;
            icon.classList.toggle(idleIconClass, !active);
            icon.classList.toggle('fa-cog', active);
            icon.classList.toggle('fa-spin', active);
        }

        function syncDataImportSidebarJobIndicator() {
            syncOneDataImportSidebarJobIndicator('data-sources-import-sidebar-btn', 'fa-file-import');
            syncOneDataImportSidebarJobIndicator('data-import-sidebar-btn', 'fa-tools');
        }

        async function refreshSidebarBackgroundJobsState() {
            try {
                const res = await fetch('/api/background-jobs', { credentials: 'same-origin' });
                if (!res.ok) return;
                const data = await res.json();
                const jobs = Array.isArray(data.jobs) ? data.jobs : [];
                sidebarBackgroundJobsRunning = jobs.some((j) => j && j.in_progress);
            } catch (_) {
                /* ignore polling errors */
            }
            syncDataImportSidebarJobIndicator();
        }

        function ensureSidebarBackgroundJobsPoll() {
            if (sidebarBackgroundJobsPollTimer) return;
            sidebarBackgroundJobsPollTimer = setInterval(() => {
                void refreshSidebarBackgroundJobsState();
            }, 15000);
        }

        function makeJobKey(importType, extra = {}) {
            if (importType === 'upload_zip' && extra.zipArchiveType) {
                return 'upload_zip:' + extra.zipArchiveType;
            }
            return importType;
        }

        function jobTabLabel(importType, extra = {}) {
            if (importType === 'upload_zip' && extra.zipArchiveType) {
                const z = extra.zipArchiveType;
                const names = { whatsapp: 'WhatsApp', instagram: 'Instagram', imessage: 'iMessage & SMS', facebook: 'Facebook' };
                return names[z] || ('ZIP · ' + z);
            }
            const SHORT = {
                email_processing: 'Gmail',
                imap_processing: 'IMAP',
                upload_zip: 'ZIP',
                filesystem: 'Filesystem Upload',
                filesystem_reference: 'Filesystem References',
                thumbnails: 'Thumbnails',
                thumbnails_async: 'Thumbnails',
                reference_import: 'Reference Images',
                image_regions_recalc: 'Image Regions',
                media_classification_tag_qa: 'Classification Tag QA',
                media_gps_duplicate_spread: 'Spread Duplicate GPS',
                image_metadata_json_export: 'Export Metadata JSON',
                email_embeddings: 'Optimise Emails',
                message_embeddings: 'Message Embeddings',
                message_context_embeddings: 'Message Context Embeddings',
                image_export: 'Export Images',
                contacts: 'ProcessContacts',
                image_ai_gemma_unclassified: 'Image AI Classification',
                image_tag_embeddings: 'Image Tag Embeddings',
                artefact_embeddings: 'Artefact Embeddings',
                facebook_post_text_embeddings: 'Facebook Post Text Embeddings',
                facebook_album_description_embeddings: 'Facebook Album Description Embeddings'
            };
            return SHORT[importType] || importType;
        }

        function dataImportStatusEls() {
            const modal = document.getElementById('data-import-modal')
                || document.getElementById('data-sources-import-modal');
            if (!modal) return { tabsStrip: null, logPre: null, idleWrap: null };
            return {
                tabsStrip: modal.querySelector('.import-job-tabs-strip'),
                logPre: modal.querySelector('.import-job-log-display'),
                idleWrap: modal.querySelector('.import-job-status-idle-wrap')
            };
        }

        function forEachDataImportJobStatusModal(fn) {
            dataImportModalRoots().forEach((modal) => {
                fn({
                    modal,
                    tabsStrip: modal.querySelector('.import-job-tabs-strip'),
                    logPre: modal.querySelector('.import-job-log-display'),
                    idleWrap: modal.querySelector('.import-job-status-idle-wrap'),
                    logOuter: modal.querySelector('.import-job-log-outer'),
                });
            });
        }

        function syncLegacyConfigStatusLine() {
            const el = document.querySelector('#config-modal-overlay .import-controls-status-text');
            if (!el) return;
            if (runningJobs.size === 0) return;
            const job = selectedJobKey && runningJobs.get(selectedJobKey);
            const last = job && job.lines && job.lines.length ? job.lines[job.lines.length - 1] : '';
            el.textContent = last ? String(last).slice(0, 140) : (runningJobs.size + ' job(s) running');
        }

        function setImportStatusAllIdle(text, isError = false) {
            importStatusTextEls().forEach(el => {
                el.textContent = text || 'Idle';
                el.style.color = isError ? '#ff6b6b' : '';
            });
            forEachDataImportJobStatusModal(({ logPre, idleWrap, tabsStrip }) => {
                if (logPre) {
                    logPre.hidden = true;
                    logPre.textContent = '';
                    logPre.style.color = '';
                }
                if (idleWrap) idleWrap.hidden = false;
                if (tabsStrip) {
                    tabsStrip.innerHTML = '';
                    tabsStrip.hidden = true;
                }
            });
        }

        function appendJobLine(jobKey, text) {
            const job = runningJobs.get(jobKey);
            if (!job) return;
            const line = (text || '').trim();
            if (!line) return;
            if (!job.lines) job.lines = [];
            job.lines.push(line);
            if (job.lines.length > 100) job.lines = job.lines.slice(-100);
            if (selectedJobKey === jobKey) {
                renderJobLogDisplay();
            }
            syncLegacyConfigStatusLine();
        }

        function scrollImportJobLogToBottom() {
            forEachDataImportJobStatusModal(({ logOuter }) => {
                if (!logOuter) return;
                requestAnimationFrame(() => {
                    logOuter.scrollTop = logOuter.scrollHeight;
                });
            });
        }

        function renderJobLogDisplay() {
            if (runningJobs.size === 0) {
                forEachDataImportJobStatusModal(({ logPre, idleWrap }) => {
                    if (logPre) {
                        logPre.hidden = true;
                        logPre.textContent = '';
                    }
                    if (idleWrap) idleWrap.hidden = false;
                });
                return;
            }
            const job = selectedJobKey && runningJobs.get(selectedJobKey);
            const logText = job && job.lines && job.lines.length ? job.lines.join('\n') : '';
            forEachDataImportJobStatusModal(({ logPre, idleWrap }) => {
                if (!logPre || !idleWrap) return;
                idleWrap.hidden = true;
                logPre.hidden = false;
                logPre.style.color = '';
                logPre.textContent = logText;
            });
            scrollImportJobLogToBottom();
        }

        function selectJobTab(jobKey) {
            if (!runningJobs.has(jobKey)) return;
            selectedJobKey = jobKey;
            forEachDataImportJobStatusModal(({ tabsStrip }) => {
                if (!tabsStrip) return;
                tabsStrip.querySelectorAll('.import-job-tab').forEach(btn => {
                    const active = btn.getAttribute('data-job-key') === jobKey;
                    btn.classList.toggle('import-job-tab--active', active);
                    btn.setAttribute('aria-selected', active ? 'true' : 'false');
                });
            });
            renderJobLogDisplay();
            syncLegacyConfigStatusLine();
        }

        function addJobTab(jobKey, label) {
            forEachDataImportJobStatusModal(({ tabsStrip }) => {
                if (!tabsStrip) return;
                const btn = document.createElement('button');
                btn.type = 'button';
                btn.className = 'import-job-tab';
                btn.setAttribute('data-job-key', jobKey);
                btn.setAttribute('role', 'tab');
                btn.textContent = label;
                btn.addEventListener('click', () => selectJobTab(jobKey));
                tabsStrip.appendChild(btn);
                tabsStrip.hidden = false;
            });
        }

        function removeJobTab(jobKey) {
            forEachDataImportJobStatusModal(({ tabsStrip }) => {
                if (!tabsStrip) return;
                const btn = tabsStrip.querySelector('[data-job-key="' + String(jobKey).replace(/"/g, '\\"') + '"]');
                if (btn) btn.remove();
                if (!tabsStrip.querySelector('.import-job-tab')) tabsStrip.hidden = true;
            });
        }

        function registerRunningJob(jobKey, importType, extra, labelText) {
            const ex = extra || {};
            runningJobs.set(jobKey, {
                importType,
                zipArchiveType: ex.zipArchiveType || null,
                runReferenceImportAfterLink: !!ex.runReferenceImportAfterLink,
                label: labelText || jobTabLabel(importType, ex),
                lines: [],
                eventSource: null,
                cancelPending: false
            });
            addJobTab(jobKey, runningJobs.get(jobKey).label);
            if (!selectedJobKey || !runningJobs.has(selectedJobKey)) {
                selectedJobKey = jobKey;
            }
            selectJobTab(selectedJobKey);
            syncLegacyConfigStatusLine();
            syncDataImportSidebarJobIndicator();
        }

        function closeEventSourceForJob(jobKey) {
            const job = runningJobs.get(jobKey);
            if (job && job.eventSource) {
                try {
                    job.eventSource.close();
                } catch (_) { /* ignore */ }
                job.eventSource = null;
            }
        }

        /** Per-user suffix for email import exclusion cache (AuthModule is available when modals open). */
        function emailImportCurrentUserSuffix() {
            try {
                if (typeof AuthModule !== 'undefined' && AuthModule.getUser) {
                    const u = AuthModule.getUser();
                    if (u && u.id != null) return String(u.id);
                }
            } catch (_) { /* ignore */ }
            return '0';
        }

        function emailImportLocalExcludeKey(kind) {
            return `dm_email_import_exclude_${kind}_u${emailImportCurrentUserSuffix()}`;
        }

        function loadEmailExcludeLinesFromLocal(kind) {
            try {
                const raw = localStorage.getItem(emailImportLocalExcludeKey(kind));
                if (!raw) return null;
                const arr = JSON.parse(raw);
                return Array.isArray(arr) ? arr : null;
            } catch (_) {
                return null;
            }
        }

        function saveEmailExcludeLinesToLocal(kind, lines) {
            try {
                if (!Array.isArray(lines)) return;
                localStorage.setItem(emailImportLocalExcludeKey(kind), JSON.stringify(lines));
            } catch (_) { /* ignore quota */ }
        }

        async function gmailLoadSavedExcludeLabels(excludeLabelsEl) {
            if (!excludeLabelsEl) return;
            let lines = null;
            try {
                const res = await fetch('/api/import-saved-settings/gmail');
                if (res.ok) {
                    const data = await res.json();
                    if (data.ok && data.saved && data.settings && Array.isArray(data.settings.exclude_labels)) {
                        lines = data.settings.exclude_labels;
                    }
                }
            } catch (e) {
                console.debug('Gmail saved exclude labels load skipped:', e);
            }
            if (lines == null) {
                lines = loadEmailExcludeLinesFromLocal('gmail');
            }
            if (lines && lines.length) {
                excludeLabelsEl.value = lines.join('\n');
            }
        }

        async function gmailPersistExcludeLabels(excludeLabelsEl) {
            const lines = excludeLabelsEl
                ? excludeLabelsEl.value.split('\n').map((s) => s.trim()).filter(Boolean)
                : [];
            saveEmailExcludeLinesToLocal('gmail', lines);
            try {
                const res = await fetch('/api/import-saved-settings/gmail', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ exclude_labels: lines })
                });
                if (!res.ok && res.status !== 403) {
                    console.warn('Gmail exclude labels not saved to private store:', res.status);
                }
            } catch (e) {
                console.warn('Gmail exclude labels private store save failed:', e);
            }
        }

        const GMAIL_FORM_IDS_STANDALONE = {
            authWrap: 'gmail-auth-status-wrap',
            authText: 'gmail-auth-status-text',
            authActions: 'gmail-auth-actions',
            authLink: 'gmail-auth-link',
            authDisconnect: 'gmail-auth-disconnect',
            importControls: 'gmail-import-controls',
            allLabels: 'import-modal-email-all-labels',
            excludeLabelsWrap: 'import-modal-email-exclude-labels-wrap',
            excludeLabels: 'import-modal-email-exclude-labels',
            labelsWrap: 'import-modal-email-labels-wrap',
            labelsSelect: 'import-modal-email-labels',
            newOnly: 'import-modal-email-new-only'
        };

        function gmailImportFormHTML(ids) {
            return `
                <div class="import-input-form">
                <div id="${ids.authWrap}" class="import-input-gmail-auth">
                    <span id="${ids.authText}" class="import-input-gmail-auth-text">Checking Gmail authentication…</span>
                    <div id="${ids.authActions}" class="import-input-gmail-auth-actions">
                        <button type="button" id="${ids.authDisconnect}" class="modal-btn modal-btn-secondary" style="display: none;">Disconnect</button>
                        <button type="button" id="${ids.authLink}" class="modal-btn modal-btn-primary" style="display: none;">Connect Gmail</button>
                    </div>
                </div>
                <div id="${ids.importControls}" style="display: none;">
                    <div class="setting-group">
                        <label class="import-input-check-row">
                            <input type="checkbox" id="${ids.allLabels}">
                            <span>Process All Labels</span>
                        </label>
                    </div>
                    <div id="${ids.excludeLabelsWrap}" class="setting-group" style="display: none;">
                        <label class="artefact-field-label" for="${ids.excludeLabels}">Exclude Labels (one regex per line)</label>
                        <textarea id="${ids.excludeLabels}" class="form-control import-input-regex-textarea" rows="4" placeholder="e.g.&#10;^Spam$&#10;^Trash.*&#10;.*Promotions.*"></textarea>
                        <small class="import-input-hint">Labels matching any pattern will be skipped.</small>
                    </div>
                    <div id="${ids.labelsWrap}" class="setting-group import-dialog-labels-folders-section">
                        <label class="artefact-field-label" for="${ids.labelsSelect}">Select Labels</label>
                        <select id="${ids.labelsSelect}" class="form-control import-dialog-folders-pane" multiple>
                            <option value="">Loading labels…</option>
                        </select>
                        <small class="import-input-hint">Hold Ctrl/Cmd to select multiple</small>
                    </div>
                    <div class="setting-group">
                        <label class="import-input-check-row">
                            <input type="checkbox" id="${ids.newOnly}">
                            <span>New Only (skip already imported emails)</span>
                        </label>
                    </div>
                </div>
                </div>`;
        }

        function imapImportFormHTML(ids) {
            return `
                <div class="import-input-form">
                <div class="setting-group">
                    <label class="artefact-field-label" for="${ids.host}">IMAP Host</label>
                    <input type="text" class="form-control" id="${ids.host}" placeholder="e.g., imap.outlook.com">
                </div>
                <div class="import-input-port-row">
                    <div class="setting-group import-input-port-field">
                        <label class="artefact-field-label" for="${ids.port}">Port</label>
                        <input type="number" class="form-control" id="${ids.port}" placeholder="993">
                    </div>
                    <div class="import-input-ssl-field">
                        <label class="import-input-check-row">
                            <input type="checkbox" id="${ids.useSsl}">
                            <span>SSL/TLS</span>
                        </label>
                    </div>
                </div>
                <div class="setting-group">
                    <label class="artefact-field-label" for="${ids.username}">Username</label>
                    <input type="text" class="form-control" id="${ids.username}" placeholder="your@email.com">
                </div>
                <div class="setting-group">
                    <label class="artefact-field-label" for="${ids.password}">Password</label>
                    <input type="password" class="form-control" id="${ids.password}">
                </div>
                <div class="setting-group import-input-fetch-row">
                    <button type="button" id="${ids.fetchBtn}" class="modal-btn modal-btn-secondary">Fetch Folders</button>
                    <span id="${ids.fetchStatus}" class="import-input-status" aria-live="polite"></span>
                </div>
                <div class="setting-group">
                    <label class="import-input-check-row">
                        <input type="checkbox" id="${ids.allFolders}">
                        <span>Process All Folders</span>
                    </label>
                </div>
                <div id="${ids.excludeFoldersWrap}" class="setting-group" style="display: none;">
                    <label class="artefact-field-label" for="${ids.excludeFolders}">Exclude Folders (one regex per line)</label>
                    <textarea id="${ids.excludeFolders}" class="form-control import-input-regex-textarea" rows="4" placeholder="e.g.&#10;^Spam$&#10;^Trash&#10;.*Junk.*"></textarea>
                    <small class="import-input-hint">Folders matching any pattern will be skipped.</small>
                </div>
                <div id="${ids.foldersWrap}" class="setting-group import-dialog-labels-folders-section" style="display: none;">
                    <label class="artefact-field-label" for="${ids.foldersSelect}">Select Folders</label>
                    <select id="${ids.foldersSelect}" class="form-control import-dialog-folders-pane" multiple></select>
                    <small class="import-input-hint">Hold Ctrl/Cmd to select multiple</small>
                </div>
                <div class="setting-group">
                    <label class="import-input-check-row">
                        <input type="checkbox" id="${ids.newOnly}">
                        <span>New Only (skip already imported emails)</span>
                    </label>
                </div>
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

        const cancelEndpoints = {
            /* ZIP/TUS archive import (single shared uploadJob on server) */
            upload_zip: '/import/upload/cancel',
            /* Gmail uses gmail_handler job — not the /emails/process stub */
            email_processing: '/gmail/process/cancel',
            imap_processing: '/imap/process/cancel',
            filesystem: '/images/import/cancel',
            reference_import: '/images/import-reference/cancel',
            image_regions_recalc: '/images/recalculate-regions/cancel',
            media_classification_tag_qa: '/images/classification-tag-qa/cancel',
            media_gps_duplicate_spread: '/images/gps-duplicate-spread/cancel',
            image_metadata_json_export: '/images/metadata-json-export/cancel',
            email_embeddings: '/emails/embeddings/backfill/cancel',
            message_embeddings: '/messages/embeddings/backfill/cancel',
            message_context_embeddings: '/messages/context-embeddings/backfill/cancel',
            image_export: '/images/export/cancel',
            thumbnails: '/images/process-thumbnails/cancel',
            contacts: '/contacts/extract/cancel',
            image_ai_gemma_unclassified: '/image/ai-classification/cancel',
            image_tag_embeddings: '/images/tag-embeddings/backfill/cancel',
            artefact_embeddings: '/artefacts/embeddings/backfill/cancel',
            facebook_post_text_embeddings: '/facebook/posts/embeddings/backfill/cancel',
            facebook_album_description_embeddings: '/facebook/albums/embeddings/backfill/cancel'
        };

        function setImportStatus(text, isError = false) {
            if (runningJobs.size === 0) {
                setImportStatusAllIdle(text, isError);
            } else if (selectedJobKey && runningJobs.has(selectedJobKey)) {
                appendJobLine(selectedJobKey, text);
                const { logPre } = dataImportStatusEls();
                if (logPre) logPre.style.color = isError ? '#ff6b6b' : '';
            }
        }

        /** Resolve running job key + cancel endpoint from a Data Import table row. */
        function rowToJobCancelTarget(row) {
            if (!row) return null;
            const t = row.getAttribute('data-import');
            if (t === 'upload_zip') {
                const z = row.getAttribute('data-zip-archive-type');
                const extra = z ? { zipArchiveType: z } : {};
                return { jobKey: makeJobKey('upload_zip', extra), importType: 'upload_zip' };
            }
            // Reference documents row only opens the manage modal — not the /images/import-reference job.
            if (t === 'reference_modal') {
                return null;
            }
            if (t === 'filesystem_reference' || t === 'upload_photos') {
                return { jobKey: 'filesystem', importType: 'filesystem' };
            }
            if (t === 'thumbnails') {
                if (runningJobs.has('thumbnails_async')) {
                    return { jobKey: 'thumbnails_async', importType: 'thumbnails_async' };
                }
            }
            const importType = t || '';
            return { jobKey: makeJobKey(importType, {}), importType };
        }

        function markJobCancelPending(jobKey) {
            const job = runningJobs.get(jobKey);
            if (!job) return;
            job.cancelPending = true;
            syncDataImportModalRowsFromRunningJobs();
            document.querySelectorAll('.import-execute-btn').forEach(btn => {
                const type = btn.getAttribute('data-import');
                if (type !== job.importType) return;
                btn.classList.add('import-cancel-pending');
                btn.classList.remove('import-executing');
                btn.style.backgroundColor = '';
                btn.innerHTML = '<i class="fas fa-hourglass-half"></i> Stopping';
            });
        }

        async function cancelImportJobForDataImportRow(row) {
            if (!row || !row.classList.contains('import-executing')) return;
            const target = rowToJobCancelTarget(row);
            if (!target) return;
            const job = runningJobs.get(target.jobKey);
            if (!job) return;
            const endpoint = cancelEndpoints[job.importType];
            if (!endpoint) return;
            try {
                const res = await fetch(endpoint, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: '{}' });
                if (!res.ok) {
                    console.warn('Row cancel HTTP', res.status, await res.text().catch(() => ''));
                    return;
                }
                markJobCancelPending(target.jobKey);
            } catch (e) {
                console.warn('Row cancel error:', e);
            }
        }

        /** Drive Data Import table rows (Processing / per-row Cancel) from runningJobs — required for parallel jobs. */
        function syncDataImportModalRowsFromRunningJobs() {
            dataImportModalRoots().forEach((dim) => {
            dim.querySelectorAll('.data-import-row').forEach(row => {
                const target = rowToJobCancelTarget(row);
                const job = target ? runningJobs.get(target.jobKey) : null;
                const busy = !!job;

                row.classList.toggle('import-executing', busy);

                const rowCancel = row.querySelector('.data-import-row-cancel-btn');
                if (rowCancel) {
                    const showCancel = !!(target && job && cancelEndpoints[job.importType] && !job.cancelPending);
                    rowCancel.hidden = !showCancel;
                    rowCancel.disabled = !showCancel;
                    rowCancel.setAttribute('aria-hidden', showCancel ? 'false' : 'true');
                }

                row.querySelectorAll('.data-import-start-btn, .data-import-zip-start-btn').forEach(b => {
                    b.classList.remove('data-import-start-btn--cancel-pending', 'data-import-btn--processing');
                    if (busy) {
                        if (!b.dataset.importOriginalHtml) b.dataset.importOriginalHtml = b.innerHTML;
                        if (job.cancelPending) {
                            b.innerHTML = '<i class="fas fa-hourglass-half"></i> Stopping';
                            b.classList.add('data-import-start-btn--cancel-pending');
                            b.style.backgroundColor = '';
                            b.disabled = true;
                        } else {
                            b.classList.add('data-import-btn--processing');
                            b.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Processing';
                            b.disabled = true;
                        }
                    } else {
                        b.innerHTML = b.dataset.importOriginalHtml || b.innerHTML;
                        delete b.dataset.importOriginalHtml;
                        b.style.backgroundColor = '';
                        b.disabled = false;
                    }
                });

                row.querySelectorAll('.data-import-open-modal-btn').forEach(b => {
                    const isUploadPhotos = b.getAttribute('data-open-modal') === 'upload-photos-modal';
                    if (isUploadPhotos) {
                        b.disabled = false;
                        return;
                    }
                    b.classList.remove('data-import-start-btn--cancel-pending', 'data-import-btn--processing');
                    if (busy) {
                        if (!b.dataset.importOriginalHtml) b.dataset.importOriginalHtml = b.innerHTML;
                        if (job.cancelPending) {
                            b.innerHTML = '<i class="fas fa-hourglass-half"></i> Stopping';
                            b.classList.add('data-import-start-btn--cancel-pending');
                            b.style.backgroundColor = '';
                            b.disabled = true;
                        } else {
                            b.classList.add('data-import-btn--processing');
                            b.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Processing';
                            b.disabled = true;
                        }
                    } else {
                        b.innerHTML = b.dataset.importOriginalHtml || b.innerHTML;
                        delete b.dataset.importOriginalHtml;
                        b.style.backgroundColor = '';
                        b.disabled = false;
                    }
                });
            });
            });
        }

        function setExecuting(importType, executing, extra = {}) {
            document.querySelectorAll('.import-execute-btn').forEach(btn => {
                const type = btn.getAttribute('data-import');
                if (type === importType) {
                    btn.disabled = executing;
                    if (executing) {
                        btn.classList.remove('import-cancel-pending');
                        if (!btn.dataset.importOriginalHtml) btn.dataset.importOriginalHtml = btn.innerHTML;
                        btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Processing';
                        btn.style.backgroundColor = '#ffc107';
                    } else {
                        btn.innerHTML = btn.dataset.importOriginalHtml || '<i class="fas fa-play"></i> Execute';
                        delete btn.dataset.importOriginalHtml;
                        btn.style.backgroundColor = '';
                        btn.classList.remove('import-cancel-pending');
                    }
                    btn.classList.toggle('import-executing', executing);
                }
            });
            document.querySelectorAll('.import-control-tile').forEach(tile => {
                const type = tile.getAttribute('data-import');
                tile.classList.toggle('import-executing', type === importType && executing);
            });
            syncDataImportModalRowsFromRunningJobs();
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
                case 'filesystem':
                    return `File: ${data.current_file || '-'} | ${data.files_processed || 0}/${data.total_files || 0} | ${data.images_imported || 0} imported, ${data.images_referenced || 0} referenced, ${data.images_updated || 0} updated | ${data.errors || 0} errors`;
                case 'reference_import':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.imported || 0} imported, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'image_regions_recalc':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.updated || 0} updated`;
                case 'media_classification_tag_qa':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.reflagged || 0} re-flagged | ${data.errors || 0} errors`;
                case 'media_gps_duplicate_spread':
                    return `Group: ${data.processed || 0}/${data.total || 0} | ${data.updated || 0} images | ${data.errors || 0} errors`;
                case 'image_metadata_json_export':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.exported || 0} exported | ${data.errors || 0} errors`;
                case 'email_embeddings':
                    return `Email: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'message_embeddings':
                    return `Message: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'message_context_embeddings':
                    return `Message: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'image_export':
                    return `Item: ${data.processed || 0}/${data.total || 0} | ${data.exported || 0} exported, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'thumbnails':
                    const p1 = `Phase 1: ${data.phase1_scanned || 0} scanned, ${data.phase1_updated || 0} updated`;
                    const p2 = `Phase 2: ${data.phase2_scanned || 0}/${data.phase2_total || 0} scanned, ${data.phase2_processed || 0} processed, ${data.phase2_errors || 0} errors`;
                    return data.phase === '2' ? `${p1} | ${p2}` : p1;
                case 'contacts':
                    return data.status_line || 'Processing contacts...';
                case 'image_ai_gemma_unclassified':
                    return data.status_line || 'Classifying untagged images...';
                case 'image_tag_embeddings':
                    if (data.status_line) return data.status_line;
                    return `Image tag embeddings: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped_unchanged || 0} skipped (unchanged), ${data.skipped || 0} skipped (empty), ${data.errors || 0} errors`;
                case 'artefact_embeddings':
                    if (data.status_line) return data.status_line;
                    return `Artefact embeddings: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped (empty), ${data.errors || 0} errors`;
                case 'facebook_post_text_embeddings':
                    if (data.status_line) return data.status_line;
                    return `Facebook post embeddings: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                case 'facebook_album_description_embeddings':
                    if (data.status_line) return data.status_line;
                    return `Facebook album embeddings: ${data.processed || 0}/${data.total || 0} | ${data.embedded || 0} embedded, ${data.skipped || 0} skipped | ${data.errors || 0} errors`;
                default:
                    return JSON.stringify(data).substring(0, 100);
            }
        }

        function closeAllImportEventSources() {
            runningJobs.forEach((job, jobKey) => {
                closeEventSourceForJob(jobKey);
            });
        }

        const autoContactsExtractImports = new Set([
            'upload_zip',
            'email_processing',
            'imap_processing'
        ]);

        async function maybeAutoRunContactsExtract(finishedImportType) {
            if (!autoContactsExtractImports.has(finishedImportType)) return;
            if (finishedImportType === 'contacts') return;
            if (runningJobs.has('contacts')) return;

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

        function finishImport(jobKey, success, message) {
            const rec = runningJobs.get(jobKey);
            const importType = rec ? rec.importType : null;
            const runRefAfterLink = !!(rec && rec.runReferenceImportAfterLink);
            const extra = rec && rec.zipArchiveType ? { zipArchiveType: rec.zipArchiveType } : {};
            if (message && !success) {
                appendJobLine(jobKey, message);
            }
            closeEventSourceForJob(jobKey);
            runningJobs.delete(jobKey);
            removeJobTab(jobKey);
            if (selectedJobKey === jobKey) {
                selectedJobKey = runningJobs.size ? Array.from(runningJobs.keys())[0] : null;
                if (selectedJobKey) selectJobTab(selectedJobKey);
            }
            if (importType) {
                setExecuting(importType, false, extra);
            }
            if (runningJobs.size === 0) {
                selectedJobKey = null;
                setImportStatusAllIdle(message || 'Idle', !success);
            } else {
                renderJobLogDisplay();
                syncLegacyConfigStatusLine();
            }
            if (dataImportModalRoots().some((dim) => dim.style.display !== 'none')) reloadVisibleImportModalStats();

            void maybeAutoRunContactsExtract(importType || '');

            if (success && importType === 'filesystem' && runRefAfterLink && !runningJobs.has('reference_import')) {
                void runImport('reference_import', {}).catch((e) => {
                    console.warn('Import Linked Images to Database after filesystem import failed:', e);
                });
            }
            syncDataImportSidebarJobIndicator();
            void refreshSidebarBackgroundJobsState();
        }

        const importConfigs = {
            upload_zip: { needsInput: false, title: 'Upload Archive Import', stream: '/import/upload/stream' },
            email_processing: { needsInput: true, title: 'Email Processing (Gmail)', run: async (vals) => { const body = { all_labels: vals.all_folders || false, label_ids: vals.all_folders ? [] : (vals.label_ids || []), new_only: vals.new_only || false, exclude_labels: vals.exclude_labels || [] }; const r = await fetch('/gmail/process', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/gmail/process/stream' },
            filesystem: { needsInput: true, title: 'Filesystem Image Import', fields: [{ id: 'root_directory', key: 'filesystem_import_directory', label: 'Folder to scan', placeholder: 'Choose a folder or type a full path', required: true, type: 'folder', folderDialogTitle: 'Select folder to import (reference only)' }, { id: 'max_images', key: 'filesystem_import_max_images', label: 'Max Images (Optional)', placeholder: 'Leave empty for all', required: false, type: 'number' }, { id: 'import_db_after_link', key: 'filesystem_import_db_after_link', label: 'Import to Database After Linking', required: false, type: 'checkbox' }], run: async (vals) => { const body = { root_directory: vals.root_directory, create_thumb_and_get_exif: false, reference_mode: true }; if (vals.max_images) body.max_images = parseInt(vals.max_images, 10); const r = await fetch('/images/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/images/import/stream' },
            filesystem_reference: { needsInput: true, title: 'Reference images on filesystem (local)', fields: [{ id: 'root_directory', key: 'filesystem_reference_import_directory', label: 'Folder(s) on this machine — separate with semicolons. Each folder is scanned recursively; only paths are stored in the archive (images stay on disk).', placeholder: 'e.g., C:\\Photos\\Vacation; D:\\Pictures', required: true }, { id: 'max_images', key: 'filesystem_reference_import_max_images', label: 'Max images (optional)', placeholder: 'Leave empty for all', required: false, type: 'number' }], run: async (vals) => { const body = { root_directory: vals.root_directory, create_thumb_and_get_exif: false, reference_mode: true }; if (vals.max_images) body.max_images = parseInt(vals.max_images, 10); const r = await fetch('/images/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/images/import/stream' },
            reference_import: { needsInput: false, title: 'Import Reference Images to Database', run: async () => { const r = await fetch('/images/import-reference', { method: 'POST' }); return r; }, stream: '/images/import-reference/stream' },
            image_regions_recalc: { needsInput: false, title: 'Recalculate Image Regions', run: async () => { const r = await fetch('/images/recalculate-regions', { method: 'POST' }); return r; }, stream: '/images/recalculate-regions/stream' },
            media_classification_tag_qa: { needsInput: false, title: 'QA Check Image Classification Tags', run: async () => { const r = await fetch('/images/classification-tag-qa', { method: 'POST' }); return r; }, stream: '/images/classification-tag-qa/stream' },
            media_gps_duplicate_spread: { needsInput: false, title: 'Spread Duplicate GPS Coordinates', run: async () => { const r = await fetch('/images/gps-duplicate-spread', { method: 'POST' }); return r; }, stream: '/images/gps-duplicate-spread/stream' },
            image_metadata_json_export: { needsInput: true, title: 'Export Image Metadata JSON', fields: [{ id: 'output_path', key: 'image_metadata_export_path', label: 'Output JSON file path', placeholder: 'e.g., C:\\Users\\Dave\\Exports\\image-metadata.json', required: true }], run: async (vals) => { const r = await fetch('/images/metadata-json-export', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ output_path: vals.output_path }) }); return r; }, stream: '/images/metadata-json-export/stream' },
            email_embeddings: { needsInput: false, title: 'Optimise Emails for Searching', run: async () => { const r = await fetch('/emails/embeddings/backfill', { method: 'POST' }); return r; }, stream: '/emails/embeddings/backfill/stream' },
            message_embeddings: { needsInput: false, title: 'Generate missing message embedding vectors', run: async () => { const r = await fetch('/messages/embeddings/backfill', { method: 'POST' }); return r; }, stream: '/messages/embeddings/backfill/stream' },
            message_context_embeddings: { needsInput: false, title: 'Build message context embeddings', run: async () => { const r = await fetch('/messages/context-embeddings/backfill', { method: 'POST' }); return r; }, stream: '/messages/context-embeddings/backfill/stream' },
            image_export: { needsInput: true, title: 'Export Images to Filesystem', fields: [{ id: 'target_directory', key: 'image_export_directory', label: 'Target Directory', placeholder: 'e.g., C:\\Users\\Dave\\Exports\\images', required: true }], run: async (vals) => { const r = await fetch('/images/export', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ target_directory: vals.target_directory }) }); return r; }, stream: '/images/export/stream' },
            thumbnails: { needsInput: false, title: 'Image Processing', run: async () => { const r = await fetch('/images/process-thumbnails', { method: 'POST' }); return r; }, stream: '/images/process-thumbnails/stream' },
            thumbnails_async: { needsInput: false, title: 'Image Processing (Async)', run: async () => { const r = await fetch('/images/process-thumbnails/async', { method: 'POST' }); return r; }, stream: null },
            contacts: { needsInput: false, title: 'Contacts Merge', run: async () => { const r = await fetch('/contacts/extract', { method: 'POST' }); return r; }, stream: '/contacts/extract/stream' },
            image_ai_gemma_unclassified: { needsInput: false, title: 'Classify images with unclassified content', run: async () => { const r = await fetch('/image/ai-classification/missing-gemma-classified', { method: 'POST' }); return r; }, stream: '/image/ai-classification/stream' },
            image_tag_embeddings: { needsInput: false, title: 'Build image tag similarity embeddings', run: async (vals) => { const body = { reprocess_all: !!(vals && vals.reprocess_all) }; const r = await fetch('/images/tag-embeddings/backfill', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/images/tag-embeddings/backfill/stream' },
            artefact_embeddings: { needsInput: false, title: 'Build artefact similarity embeddings', run: async (vals) => { const body = { reprocess_all: !!(vals && vals.reprocess_all) }; const r = await fetch('/artefacts/embeddings/backfill', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/artefacts/embeddings/backfill/stream' },
            facebook_post_text_embeddings: { needsInput: false, title: 'Build Facebook post text embeddings', run: async (vals) => { const body = { reprocess_all: !!(vals && vals.reprocess_all) }; const r = await fetch('/facebook/posts/embeddings/backfill', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/facebook/posts/embeddings/backfill/stream' },
            facebook_album_description_embeddings: { needsInput: false, title: 'Build Facebook album description embeddings', run: async (vals) => { const body = { reprocess_all: !!(vals && vals.reprocess_all) }; const r = await fetch('/facebook/albums/embeddings/backfill', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return r; }, stream: '/facebook/albums/embeddings/backfill/stream' },
            imap_processing: { needsInput: true, title: 'IMAP Import', run: async (vals) => { const body = { host: vals.host, port: vals.port, username: vals.username, password: vals.password, use_ssl: vals.use_ssl !== false, all_folders: vals.all_folders || false, folders: vals.folders && vals.folders.length ? vals.folders : (vals.all_folders ? [] : ['INBOX']), new_only: vals.new_only || false, exclude_folders: vals.exclude_folders || [] }; return await fetch('/imap/process', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); }, stream: '/imap/process/stream' }
        };

        const GMAIL_OAUTH_MESSAGE_TYPE = 'gmail-oauth-complete';
        let gmailOAuthPopupWindow = null;
        let gmailOAuthMessageListener = null;
        let gmailOAuthPopupPollTimer = null;

        function teardownGmailOAuthPopupWatchers() {
            if (gmailOAuthMessageListener) {
                window.removeEventListener('message', gmailOAuthMessageListener);
                gmailOAuthMessageListener = null;
            }
            if (gmailOAuthPopupPollTimer) {
                clearInterval(gmailOAuthPopupPollTimer);
                gmailOAuthPopupPollTimer = null;
            }
            gmailOAuthPopupWindow = null;
        }

        async function wireGmailImportForm(ids, submitButtonHook) {
            const authStatusText = document.getElementById(ids.authText);
            const authLink = document.getElementById(ids.authLink);
            const authDisconnect = document.getElementById(ids.authDisconnect);
            const importControls = document.getElementById(ids.importControls);
            const allLabelsCb = document.getElementById(ids.allLabels);
            const excludeLabelsWrap = document.getElementById(ids.excludeLabelsWrap);
            const excludeLabelsEl = document.getElementById(ids.excludeLabels);
            const labelsWrap = document.getElementById(ids.labelsWrap);
            const labelsSelect = document.getElementById(ids.labelsSelect);
            const newOnlyCb = document.getElementById(ids.newOnly);

            function showGmailDisconnected() {
                authStatusText.textContent = 'Not connected to Gmail.';
                authStatusText.style.color = '';
                if (authLink) authLink.style.display = 'inline-block';
                if (authDisconnect) authDisconnect.style.display = 'none';
                if (importControls) importControls.style.display = 'none';
                if (submitButtonHook) submitButtonHook.disabled = true;
            }

            function showGmailConnected(expiryIso) {
                const expiry = expiryIso
                    ? ` (token expires ${new Date(expiryIso).toLocaleDateString('en-GB', { day: '2-digit', month: 'numeric', year: 'numeric' })})`
                    : '';
                authStatusText.textContent = `Connected to Gmail${expiry}`;
                authStatusText.style.color = '#27ae60';
                if (authLink) authLink.style.display = 'none';
                if (authDisconnect) authDisconnect.style.display = 'inline-block';
                if (importControls) importControls.style.display = 'block';
                if (submitButtonHook) submitButtonHook.disabled = false;
            }

            async function disconnectGmail() {
                if (!authDisconnect) return;
                if (!window.confirm('Disconnect Gmail? The stored OAuth token will be removed from this archive.')) {
                    return;
                }
                const prevLabel = authDisconnect.textContent;
                authDisconnect.disabled = true;
                authDisconnect.textContent = 'Disconnecting…';
                try {
                    const res = await fetch('/gmail/auth', {
                        method: 'DELETE',
                        credentials: 'same-origin',
                    });
                    if (!res.ok) {
                        let detail = 'Disconnect failed';
                        try {
                            const d = await res.json();
                            if (d && (d.detail || d.error)) detail = d.detail || d.error;
                        } catch (_) { /* ignore */ }
                        throw new Error(detail);
                    }
                    showGmailDisconnected();
                } catch (e) {
                    authStatusText.textContent = e.message || 'Could not disconnect Gmail.';
                    authStatusText.style.color = '#c0392b';
                } finally {
                    authDisconnect.disabled = false;
                    authDisconnect.textContent = prevLabel;
                }
            }

            if (authDisconnect && authDisconnect.dataset.gmailDisconnectWired !== '1') {
                authDisconnect.dataset.gmailDisconnectWired = '1';
                authDisconnect.addEventListener('click', () => void disconnectGmail());
            }

            async function loadGmailLabelsAndOptions() {
                const allLabelsVal = typeof getControlValue === 'function' ? getControlValue('gmail_all_labels', false) : false;
                const newOnlyVal = typeof getControlValue === 'function' ? getControlValue('gmail_new_only', true) : true;
                if (allLabelsCb) allLabelsCb.checked = !!allLabelsVal;
                if (newOnlyCb) newOnlyCb.checked = !!newOnlyVal;
                if (labelsWrap) labelsWrap.style.display = allLabelsCb && allLabelsCb.checked ? 'none' : 'block';
                if (excludeLabelsWrap) excludeLabelsWrap.style.display = allLabelsCb && allLabelsCb.checked ? 'block' : 'none';
                if (allLabelsCb && allLabelsCb.dataset.gmailAllLabelsWired !== '1') {
                    allLabelsCb.dataset.gmailAllLabelsWired = '1';
                    allLabelsCb.addEventListener('change', () => {
                        if (labelsWrap) labelsWrap.style.display = allLabelsCb.checked ? 'none' : 'block';
                        if (excludeLabelsWrap) excludeLabelsWrap.style.display = allLabelsCb.checked ? 'block' : 'none';
                    });
                }
                if (!labelsSelect) return;
                try {
                    const labelsResp = await fetch('/gmail/labels', { credentials: 'same-origin' });
                    if (!labelsResp.ok) throw new Error('Failed to load labels');
                    const labelsData = await labelsResp.json();
                    labelsSelect.innerHTML = '';
                    (labelsData.labels || []).forEach((l) => {
                        const opt = document.createElement('option');
                        opt.value = l.id;
                        opt.textContent = l.name;
                        if (l.name === 'INBOX') opt.selected = true;
                        labelsSelect.appendChild(opt);
                    });
                } catch (_) {
                    labelsSelect.innerHTML = '<option value="">Error loading labels</option>';
                }
                await gmailLoadSavedExcludeLabels(excludeLabelsEl);
            }

            async function refreshGmailAuthStatus() {
                try {
                    const authResp = await fetch('/gmail/auth/status', { credentials: 'same-origin' });
                    const authData = await authResp.json();
                    if (!authData.authenticated) {
                        showGmailDisconnected();
                        return;
                    }
                    showGmailConnected(authData.expiry);
                    await loadGmailLabelsAndOptions();
                } catch (_) {
                    authStatusText.textContent = 'Could not check Gmail status.';
                    authStatusText.style.color = '#c0392b';
                }
            }

            function openGmailOAuthPopup() {
                teardownGmailOAuthPopupWatchers();
                const prevLabel = authLink ? authLink.textContent : '';
                if (authLink) {
                    authLink.disabled = true;
                    authLink.textContent = 'Connecting…';
                }
                authStatusText.textContent = 'Complete sign-in in the popup window…';
                authStatusText.style.color = '#555';

                const popupUrl = '/gmail/auth/start?popup=1';
                const popupW = 520;
                const popupH = 700;
                const screenLeft = window.screenLeft ?? window.screenX ?? 0;
                const screenTop = window.screenTop ?? window.screenY ?? 0;
                const availW = window.screen.availWidth ?? window.screen.width;
                const availH = window.screen.availHeight ?? window.screen.height;
                const popupLeft = Math.round(screenLeft + Math.max(0, (availW - popupW) / 2));
                const popupTop = Math.round(screenTop + Math.max(0, (availH - popupH) / 2));
                const features = `width=${popupW},height=${popupH},left=${popupLeft},top=${popupTop},menubar=no,toolbar=no,location=yes,status=no,resizable=yes,scrollbars=yes`;
                gmailOAuthPopupWindow = window.open(popupUrl, 'gmail_oauth', features);
                if (!gmailOAuthPopupWindow) {
                    if (authLink) {
                        authLink.disabled = false;
                        authLink.textContent = prevLabel || 'Connect Gmail';
                    }
                    authStatusText.textContent = 'Popup blocked. Allow popups for this site and try again.';
                    authStatusText.style.color = '#c0392b';
                    return;
                }

                let finished = false;
                const finishConnect = async (ok, errMsg) => {
                    if (finished) return;
                    finished = true;
                    teardownGmailOAuthPopupWatchers();
                    if (authLink) {
                        authLink.disabled = false;
                        authLink.textContent = prevLabel || 'Connect Gmail';
                    }
                    if (ok) {
                        await refreshGmailAuthStatus();
                    } else {
                        authStatusText.textContent = errMsg || 'Gmail connection failed.';
                        authStatusText.style.color = '#c0392b';
                        if (authLink) authLink.style.display = 'inline-block';
                        if (authDisconnect) authDisconnect.style.display = 'none';
                        if (importControls) importControls.style.display = 'none';
                        if (submitButtonHook) submitButtonHook.disabled = true;
                    }
                };

                gmailOAuthMessageListener = (ev) => {
                    if (ev.origin !== window.location.origin) return;
                    const data = ev.data;
                    if (!data || data.type !== GMAIL_OAUTH_MESSAGE_TYPE) return;
                    void finishConnect(!!data.ok, data.error || '');
                };
                window.addEventListener('message', gmailOAuthMessageListener);

                gmailOAuthPopupPollTimer = setInterval(() => {
                    if (!gmailOAuthPopupWindow || gmailOAuthPopupWindow.closed) {
                        void (async () => {
                            if (finished) return;
                            try {
                                const authResp = await fetch('/gmail/auth/status', { credentials: 'same-origin' });
                                const authData = authResp.ok ? await authResp.json() : {};
                                if (authData.authenticated) {
                                    await finishConnect(true, '');
                                } else {
                                    await finishConnect(false, 'Sign-in window closed before Gmail was connected.');
                                }
                            } catch (_) {
                                await finishConnect(false, 'Could not verify Gmail connection.');
                            }
                        })();
                    }
                }, 500);
            }

            if (authLink && authLink.dataset.gmailConnectWired !== '1') {
                authLink.dataset.gmailConnectWired = '1';
                authLink.addEventListener('click', (e) => {
                    e.preventDefault();
                    openGmailOAuthPopup();
                });
            }

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

            await refreshGmailAuthStatus();

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
                void gmailPersistExcludeLabels(document.getElementById(GMAIL_FORM_IDS_STANDALONE.excludeLabels));
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
                    saveEmailExcludeLinesToLocal('imap', body.exclude_folders || []);
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
                let data = null;
                try {
                    const res = await fetch('/api/import-saved-settings/imap');
                    if (res.ok) {
                        data = await res.json();
                    }
                } catch (e) {
                    console.debug('IMAP saved-settings load skipped:', e);
                }
                try {
                    if (data && data.ok && data.saved && data.settings) {
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
                    console.debug('IMAP saved-settings apply skipped:', e);
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
                if (excludeFoldersEl && !excludeFoldersEl.value.trim()) {
                    const loc = loadEmailExcludeLinesFromLocal('imap');
                    if (loc && loc.length) excludeFoldersEl.value = loc.join('\n');
                }
                if (excludeFoldersWrap) excludeFoldersWrap.style.display = allFoldersCb.checked ? 'block' : 'none';
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
                    fetchStatus.classList.add('import-input-status--error');
                    fetchStatus.classList.remove('import-input-status--success');
                    return;
                }
                fetchBtn.disabled = true;
                fetchStatus.textContent = 'Connecting...';
                fetchStatus.classList.remove('import-input-status--error', 'import-input-status--success');
                try {
                    const resp = await fetch('/imap/folders', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ host, port, username, password, use_ssl })
                    });
                    if (!resp.ok) {
                        const err = await resp.json();
                        fetchStatus.textContent = err.detail || 'Failed to fetch folders';
                        fetchStatus.classList.add('import-input-status--error');
                        fetchStatus.classList.remove('import-input-status--success');
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
                    fetchStatus.classList.add('import-input-status--success');
                    fetchStatus.classList.remove('import-input-status--error');
                    if (!allFoldersCb.checked) foldersWrap.style.display = 'block';
                    if (excludeFoldersWrap) excludeFoldersWrap.style.display = allFoldersCb.checked ? 'block' : 'none';
                    void imapSaveToPrivateStore();
                } catch (e) {
                    fetchStatus.textContent = 'Connection error: ' + e.message;
                    fetchStatus.classList.add('import-input-status--error');
                    fetchStatus.classList.remove('import-input-status--success');
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
            filesystem: 'filesystem',
            filesystem_reference: 'filesystem_reference'
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

        async function showImportModal(importType, onSubmit) {
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.needsInput) { onSubmit({}); return; }
            importInputModalTitle.textContent = cfg.title;
            importInputModalBody.innerHTML = '<div class="import-input-form">' + cfg.fields.map(f => {
                if (f.type === 'checkbox') {
                    return `<div class="setting-group"><label class="import-input-check-row"><input type="checkbox" id="import-modal-${f.id}"><span>${f.label}</span></label></div>`;
                }
                if (f.type === 'folder') {
                    return `<div class="setting-group"><label class="artefact-field-label" for="import-modal-${f.id}">${f.label}</label><div style="display: flex; gap: 8px; align-items: stretch;"><input type="text" class="form-control" id="import-modal-${f.id}" placeholder="${f.placeholder || ''}" style="flex: 1; min-width: 0;"><button type="button" class="modal-btn modal-btn-secondary" id="import-modal-${f.id}-browse" style="white-space: nowrap;">Choose folder…</button></div><p class="import-input-hint" style="margin-top: 0.5rem;">Paths only are stored in the archive; image files remain on disk (reference import).</p></div>`;
                }
                return `<div class="setting-group"><label class="artefact-field-label" for="import-modal-${f.id}">${f.label}</label><input type="${f.type || 'text'}" class="form-control" id="import-modal-${f.id}" placeholder="${f.placeholder || ''}"></div>`;
            }).join('') + '</div>';
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

            cfg.fields.forEach(f => {
                if (f.type !== 'folder') return;
                const input = document.getElementById(`import-modal-${f.id}`);
                const browse = document.getElementById(`import-modal-${f.id}-browse`);
                if (!input || !browse) return;
                browse.addEventListener('click', async () => {
                    if (!window.electronAPI || typeof window.electronAPI.showOpenDialog !== 'function') {
                        window.alert('Folder picker is only available in the desktop app. You can type the full path in the box instead.');
                        input.focus();
                        return;
                    }
                    try {
                        const picked = await window.electronAPI.showOpenDialog({
                            title: f.folderDialogTitle || 'Select folder',
                            properties: ['openDirectory']
                        });
                        const p = picked && !picked.canceled && picked.filePaths && picked.filePaths.length ? picked.filePaths[0] : '';
                        if (p) {
                            input.value = p;
                            if (typeof saveControlValue === 'function') saveControlValue(f.key, p);
                        }
                    } catch (err) {
                        console.warn('showOpenDialog failed:', err);
                    }
                });
            });

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

        function connectToImportStream(importType, jobKey) {
            const cfg = importConfigs[importType];
            if (!cfg || !cfg.stream) return;
            closeEventSourceForJob(jobKey);
            const es = new EventSource(cfg.stream);
            const job = runningJobs.get(jobKey);
            if (job) job.eventSource = es;
            es.onmessage = (event) => {
                try {
                    const ed = JSON.parse(event.data);
                    const type = ed.type;
                    const data = ed.data || {};
                    if (type === 'progress' || type === 'status') {
                        const line = data.status_line || formatProgressLine(importType, data);
                        appendJobLine(jobKey, line);
                    } else if (type === 'completed') {
                        const line = data.status_line || formatProgressLine(importType, data) || 'Completed successfully';
                        appendJobLine(jobKey, line);
                        finishImport(jobKey, true, line);
                    } else if (type === 'error') {
                        finishImport(jobKey, false, data.error_message || data.status_line || 'Error');
                    } else if (type === 'cancelled') {
                        finishImport(jobKey, false, data.status_line || 'Cancelled');
                    }
                } catch (e) { console.warn('Import SSE parse error:', e); }
            };
            es.onerror = () => {};
        }

        async function runImport(importType, values, extra = {}) {
            const serverKind = importType === 'filesystem_reference' ? 'filesystem' : importType;
            const jobKey = makeJobKey(serverKind, extra);
            if (runningJobs.has(jobKey)) {
                return;
            }
            const cfg = importConfigs[importType];
            if (!cfg) {
                return;
            }
            const jobExtra = Object.assign({}, extra, {
                runReferenceImportAfterLink: importType === 'filesystem' && !!(values && values.import_db_after_link)
            });
            registerRunningJob(jobKey, serverKind, jobExtra);
            setExecuting(serverKind, true, extra);
            appendJobLine(jobKey, 'Starting…');

            try {
                const res = await cfg.run(values);
                let result = {};
                let rawText = '';
                try {
                    rawText = await res.text();
                    if (rawText) result = JSON.parse(rawText);
                } catch (_) {}
                if (!res.ok) {
                    const fallback = (rawText || '').trim();
                    finishImport(jobKey, false, result.detail || fallback || 'Failed to start');
                    return;
                }
                if (cfg.stream) {
                    connectToImportStream(serverKind, jobKey);
                } else {
                    finishImport(jobKey, true, 'Accepted');
                }
            } catch (e) {
                finishImport(jobKey, false, e.message || 'Error');
            }
        }

        async function triggerImport(importType) {
            if (!(await ensureMasterKeyForDataImport())) return;
            if (importType === 'thumbnails_prompt') {
                if (runningJobs.has('thumbnails') || runningJobs.has('thumbnails_async')) return;
                void runImport('thumbnails', {});
                return;
            }
            const jobKey = makeJobKey(importType, {});
            if (runningJobs.has(jobKey)) return;
            if (importType === 'email_processing') {
                showEmailProcessingModal((vals) => runImport(importType, vals));
            } else if (importType === 'imap_processing') {
                showImapModal((vals) => runImport(importType, vals));
            } else if (importType === 'image_tag_embeddings') {
                let reprocessAll = false;
                if (window.AppDialogs && typeof window.AppDialogs.showAppConfirm === 'function') {
                    reprocessAll = await window.AppDialogs.showAppConfirm(
                        'Image tag embeddings',
                        'Do you want to reprocess all tagged images?\n\n' +
                            'Reprocess all: refresh every embedding via local AI (slower).\n' +
                            'Skip unchanged: only embed images where tags changed or no embedding exists yet.',
                        { confirmLabel: 'Reprocess all', cancelLabel: 'Skip unchanged' }
                    );
                }
                await runImport(importType, { reprocess_all: !!reprocessAll });
            } else if (importType === 'artefact_embeddings') {
                let reprocessAll = false;
                if (window.AppDialogs && typeof window.AppDialogs.showAppConfirm === 'function') {
                    reprocessAll = await window.AppDialogs.showAppConfirm(
                        'Artefact embeddings',
                        'Do you want to reprocess all artefacts?\n\n' +
                            'Reprocess all: refresh every embedding via local AI (slower).\n' +
                            'Fill missing only: only embed artefacts that do not already have vectors.',
                        { confirmLabel: 'Reprocess all', cancelLabel: 'Fill missing only' }
                    );
                }
                await runImport(importType, { reprocess_all: !!reprocessAll });
            } else if (importType === 'facebook_post_text_embeddings' || importType === 'facebook_album_description_embeddings') {
                let reprocessAll = false;
                if (window.AppDialogs && typeof window.AppDialogs.showAppConfirm === 'function') {
                    const label = importType === 'facebook_post_text_embeddings'
                        ? 'Facebook post text embeddings'
                        : 'Facebook album description embeddings';
                    reprocessAll = await window.AppDialogs.showAppConfirm(
                        label,
                        'Do you want to reprocess all rows?\n\n' +
                            'Reprocess all: refresh every embedding via local AI (slower).\n' +
                            'Fill missing only: only embed rows that do not already have vectors.',
                        { confirmLabel: 'Reprocess all', cancelLabel: 'Fill missing only' }
                    );
                }
                await runImport(importType, { reprocess_all: !!reprocessAll });
            } else {
                await showImportModal(importType, (vals) => runImport(importType, vals));
            }
        }

        document.querySelectorAll('.import-execute-btn').forEach(btn => {
            btn.addEventListener('click', () => { void triggerImport(btn.getAttribute('data-import')); });
        });

        let resetDataImportDetailSidebar = () => {};

        (function setupDataImportModalCategoryTabs() {
            const modal = document.getElementById('data-import-modal');
            if (!modal) return;
            const strip = modal.querySelector('.data-import-category-tabstrip');
            if (!strip) return;
            strip.addEventListener('click', (e) => {
                const btn = e.target.closest('.data-import-category-tab');
                if (!btn || !strip.contains(btn)) return;
                const tab = btn.getAttribute('data-import-category-tab');
                if (!tab) return;
                e.preventDefault();
                strip.querySelectorAll('.data-import-category-tab').forEach((b) => {
                    const on = b.getAttribute('data-import-category-tab') === tab;
                    b.classList.toggle('data-import-category-tab--active', on);
                    b.setAttribute('aria-selected', on ? 'true' : 'false');
                });
                modal.querySelectorAll('.data-import-category-panel').forEach((p) => {
                    const on = p.getAttribute('data-import-category-panel') === tab;
                    p.classList.toggle('data-import-category-panel--active', on);
                });
                const scrollRegion = modal.querySelector('.data-import-table-scroll-region');
                if (scrollRegion) {
                    scrollRegion.classList.toggle('data-import-table-scroll-region--background-jobs', tab === 'background-jobs');
                }
                if (tab === 'background-jobs') {
                    if (Modals.BackgroundJobs && Modals.BackgroundJobs.load) void Modals.BackgroundJobs.load();
                }
                resetDataImportDetailSidebar();
            });
        })();

        (function setupDataImportDetailSidebar() {
            function wireDetailSidebar(modal) {
                if (!modal) return;
                const ph = modal.querySelector('.data-import-detail-placeholder');
                const content = modal.querySelector('.data-import-detail-content');
                const titleEl = modal.querySelector('.data-import-detail-title');
                const descEl = modal.querySelector('.data-import-detail-desc');
                const entriesEl = modal.querySelector('.data-import-detail-entries');

                function showCard(card) {
                    if (!card || !content || !ph || !titleEl || !descEl || !entriesEl) return;
                    const titleSrc = card.querySelector('.data-import-card-title');
                    const detailBody = card.querySelector('.data-import-card-detail-body');
                    const metrics = card.querySelector('.data-import-card-metrics-source');
                    const cnt = metrics && metrics.querySelector('.data-import-count');
                    titleEl.innerHTML = titleSrc ? titleSrc.innerHTML : '';
                    descEl.innerHTML = detailBody ? detailBody.innerHTML : '';
                    entriesEl.textContent = cnt ? cnt.textContent.trim() : '—';
                    ph.hidden = true;
                    content.hidden = false;
                }

                modal.querySelectorAll('.data-import-card').forEach((card) => {
                    card.addEventListener('mouseenter', () => showCard(card));
                    card.addEventListener('focusin', () => showCard(card));
                });
            }

            resetDataImportDetailSidebar = () => {
                dataImportModalRoots().forEach((modal) => {
                    const ph = modal.querySelector('.data-import-detail-placeholder');
                    const content = modal.querySelector('.data-import-detail-content');
                    if (ph) ph.hidden = false;
                    if (content) content.hidden = true;
                });
            };

            dataImportModalRoots().forEach(wireDetailSidebar);
        })();

        document.querySelectorAll('.import-control-tile').forEach(tile => {
            tile.addEventListener('click', (e) => {
                const openModal = tile.getAttribute('data-open-modal');
                if (openModal) {
                    if (openModal === 'data-import-modal') {
                        void (async () => {
                            if (DOM.configPage) DOM.configPage.style.display = 'none';
                            await openDataImportModalAfterKeyCheck();
                        })();
                        return;
                    }
                    if (openModal === 'data-sources-import-modal') {
                        void (async () => {
                            if (DOM.configPage) DOM.configPage.style.display = 'none';
                            await openDataSourcesImportModalAfterKeyCheck();
                        })();
                        return;
                    }
                    const modal = document.getElementById(openModal);
                    if (modal) {
                        modal.style.display = 'flex';
                        if (openModal === 'reference-documents-manage-modal') {
                            hideAllDataImportModals();
                            if (Modals.ReferenceDocuments && Modals.ReferenceDocuments.loadDocuments) {
                                Modals.ReferenceDocuments.loadDocuments();
                            }
                        }
                    }
                    return;
                }
                const openTab = tile.getAttribute('data-open-tab');
                if (openTab) {
                    hideAllDataImportModals();
                    if (DOM.configPage) {
                        DOM.configPage.style.display = 'flex';
                        loadControlDefaults();
                        const tabBtn = document.querySelector(`.config-tab-button[data-tab="${openTab}"]`);
                        if (tabBtn) {
                            tabBtn.click();
                        }
                        refreshSettingsDataImportModalLLM();
                    }
                    return;
                }
                void triggerImport(tile.getAttribute('data-import'));
            });
        });

        (function setupDataImportTableDelegations() {
            // Capture phase: inner .modal-content uses onclick="event.stopPropagation()" which blocks
            // bubbling to the overlay; delegated handlers must run on capture.
            function bindDataImportTableClicks(container, opts) {
                if (!container) return;
                const hideDataImportWhenOpeningRefModal = !!(opts && opts.hideDataImportWhenOpeningRefModal);
                container.addEventListener('click', (e) => {
                    const rowCancelBtn = e.target.closest('.data-import-row-cancel-btn');
                    if (rowCancelBtn && container.contains(rowCancelBtn) && !rowCancelBtn.disabled) {
                        e.preventDefault();
                        e.stopPropagation();
                        const row = rowCancelBtn.closest('.data-import-row');
                        if (row) void cancelImportJobForDataImportRow(row);
                        return;
                    }
                    const startBtn = e.target.closest('.data-import-start-btn');
                    if (startBtn && container.contains(startBtn)) {
                        e.preventDefault();
                        e.stopPropagation();
                        const t = startBtn.getAttribute('data-import-start');
                        if (t) void triggerImport(t);
                        return;
                    }
                    const zipBtn = e.target.closest('.data-import-zip-start-btn');
                    if (zipBtn && container.contains(zipBtn)) {
                        e.preventDefault();
                        e.stopPropagation();
                        const arch = zipBtn.getAttribute('data-zip-archive-type');
                        if (arch && window.UploadImport && typeof window.UploadImport.openZipModalWithArchiveType === 'function') {
                            void (async () => {
                                if (!(await ensureMasterKeyForDataImport())) return;
                                window.UploadImport.openZipModalWithArchiveType(arch);
                            })();
                        }
                        return;
                    }
                    const openM = e.target.closest('.data-import-open-modal-btn');
                    if (openM && container.contains(openM)) {
                        e.preventDefault();
                        e.stopPropagation();
                        void (async () => {
                            if (!(await ensureMasterKeyForDataImport())) return;
                            const mid = openM.getAttribute('data-open-modal');
                            const m = mid ? document.getElementById(mid) : null;
                            if (m) {
                                m.style.display = 'flex';
                                if (mid === 'reference-documents-manage-modal' && hideDataImportWhenOpeningRefModal) {
                                    hideAllDataImportModals();
                                    if (Modals.ReferenceDocuments && Modals.ReferenceDocuments.loadDocuments) {
                                        Modals.ReferenceDocuments.loadDocuments();
                                    }
                                }
                            }
                        })();
                        return;
                    }
                    const delBtn = e.target.closest('.data-import-delete-btn');
                    if (delBtn && container.contains(delBtn) && !delBtn.disabled) {
                        e.preventDefault();
                        e.stopPropagation();
                        const kind = delBtn.getAttribute('data-import-purge-kind');
                        if (!kind) return;
                        void (async () => {
                            if (!(await ensureMasterKeyForDataImport())) return;
                            const ok1 = await AppDialogs.showAppConfirm(
                                'Delete imported data',
                                'This permanently removes archive data for this type. This cannot be undone.\n\nAre you sure you want to continue?',
                                { danger: true }
                            );
                            if (!ok1) return;
                            const ok2 = await AppDialogs.showAppConfirm(
                                'Final confirmation',
                                'Confirm again: delete all data for this import type for your archive.',
                                { danger: true }
                            );
                            if (!ok2) return;
                            try {
                                const res = await fetch('/api/import-data/purge', {
                                    method: 'POST',
                                    headers: { 'Content-Type': 'application/json' },
                                    body: JSON.stringify({ kind }),
                                    credentials: 'same-origin'
                                });
                                const body = await res.json().catch(() => ({}));
                                if (!res.ok) {
                                    throw new Error(body.detail || body.error || `HTTP ${res.status}`);
                                }
                                reloadVisibleImportModalStats();
                            } catch (err) {
                                console.error('Purge failed:', err);
                                if (typeof UI !== 'undefined' && UI.displayError) UI.displayError(err.message || 'Delete failed');
                                else alert(err.message || 'Delete failed');
                            }
                        })();
                    }
                }, true);
            }
            bindDataImportTableClicks(document.getElementById('data-import-modal'), { hideDataImportWhenOpeningRefModal: true });
            bindDataImportTableClicks(document.getElementById('data-sources-import-modal'), { hideDataImportWhenOpeningRefModal: true });
        })();

        async function checkInitialImportStatus() {
            const types = ['upload_zip','email_processing','imap_processing','filesystem','reference_import','image_regions_recalc','media_classification_tag_qa','media_gps_duplicate_spread','image_metadata_json_export','email_embeddings','message_embeddings','message_context_embeddings','image_tag_embeddings','artefact_embeddings','facebook_post_text_embeddings','facebook_album_description_embeddings','image_export','thumbnails','contacts','image_ai_gemma_unclassified'];
            const statusEndpoints = {
                upload_zip: '/import/upload/status',
                email_processing: '/gmail/process/status',
                imap_processing: '/imap/process/status',
                filesystem: '/images/import/status',
                reference_import: '/images/import-reference/status',
                image_regions_recalc: '/images/recalculate-regions/status',
                media_classification_tag_qa: '/images/classification-tag-qa/status',
                media_gps_duplicate_spread: '/images/gps-duplicate-spread/status',
                image_metadata_json_export: '/images/metadata-json-export/status',
                email_embeddings: '/emails/embeddings/backfill/status',
                message_embeddings: '/messages/embeddings/backfill/status',
                message_context_embeddings: '/messages/context-embeddings/backfill/status',
                image_export: '/images/export/status',
                thumbnails: '/images/process-thumbnails/status',
                contacts: '/contacts/extract/status',
                image_ai_gemma_unclassified: '/image/ai-classification/status',
                image_tag_embeddings: '/images/tag-embeddings/backfill/status',
                artefact_embeddings: '/artefacts/embeddings/backfill/status',
                facebook_post_text_embeddings: '/facebook/posts/embeddings/backfill/status',
                facebook_album_description_embeddings: '/facebook/albums/embeddings/backfill/status'
            };
            for (const t of types) {
                try {
                    const r = await fetch(statusEndpoints[t]);
                    const s = await r.json();
                    if (!s.in_progress) continue;
                    const extra = (t === 'upload_zip' && s.import_type) ? { zipArchiveType: String(s.import_type) } : {};
                    const jobKey = makeJobKey(t, extra);
                    if (runningJobs.has(jobKey)) continue;
                    registerRunningJob(jobKey, t, extra);
                    setExecuting(t, true, extra);
                    if (s.status_line) appendJobLine(jobKey, String(s.status_line));
                    const cfg = importConfigs[t];
                    if (cfg && cfg.stream) {
                        connectToImportStream(t, jobKey);
                    }
                } catch (_) {}
            }
            syncDataImportSidebarJobIndicator();
        }
        void checkInitialImportStatus();
        void refreshSidebarBackgroundJobsState();
        ensureSidebarBackgroundJobsPoll();

        // ── Public hook for upload-import.js ────────────────────────────────
        // Called after the upload ZIP modal closes to wire the background import
        // job into the main import status system without going through runImport().
        window.ImportControls = {
            /** @param {string} label Human-readable archive label @param {string} [archiveTypeValue] e.g. whatsapp, facebook — disambiguates parallel ZIP jobs */
            attachUploadStream: function (label, archiveTypeValue) {
                const extra = archiveTypeValue ? { zipArchiveType: String(archiveTypeValue) } : {};
                const jobKey = makeJobKey('upload_zip', extra);
                if (runningJobs.has(jobKey)) return;
                registerRunningJob(jobKey, 'upload_zip', extra);
                setExecuting('upload_zip', true, extra);
                appendJobLine(jobKey, 'Starting ' + (label || 'upload') + ' import…');
                closeEventSourceForJob(jobKey);
                const es = new EventSource('/import/upload/stream');
                const job = runningJobs.get(jobKey);
                if (job) job.eventSource = es;
                es.onmessage = function (event) {
                    try {
                        const ed = JSON.parse(event.data);
                        const type = ed.type;
                        const data = ed.data || {};
                        if (type === 'progress' || type === 'status') {
                            appendJobLine(jobKey, data.status_line || '');
                        } else if (type === 'completed') {
                            finishImport(jobKey, true, data.status_line || 'Upload import completed');
                        } else if (type === 'error') {
                            finishImport(jobKey, false, data.error_message || data.status_line || 'Import error');
                        } else if (type === 'cancelled') {
                            finishImport(jobKey, false, 'Import cancelled');
                        }
                    } catch (e) { /* ignore */ }
                };
                es.onerror = function () { /* SSE auto-reconnects */ };
            },
            /** Wire a running “Upload Photos from Device” job into the Import Status panel. */
            attachFilesystemStream: function () {
                const jobKey = makeJobKey('filesystem', {});
                if (runningJobs.has(jobKey)) return;
                registerRunningJob(jobKey, 'filesystem', {});
                setExecuting('filesystem', true, {});
                appendJobLine(jobKey, 'Starting photo import…');
                closeEventSourceForJob(jobKey);
                const es = new EventSource('/images/import/stream');
                const job = runningJobs.get(jobKey);
                if (job) job.eventSource = es;
                let logLinesShown = 0;
                es.onmessage = function (event) {
                    try {
                        const ed = JSON.parse(event.data);
                        const type = ed.type;
                        const data = ed.data || {};
                        // Replay any server-buffered log lines not yet shown
                        if (Array.isArray(data.log_lines) && data.log_lines.length > logLinesShown) {
                            data.log_lines.slice(logLinesShown).forEach(function (line) {
                                if (line) appendJobLine(jobKey, line);
                            });
                            logLinesShown = data.log_lines.length;
                        } else if ((type === 'progress' || type === 'status') && data.status_line) {
                            appendJobLine(jobKey, data.status_line);
                        }
                        if (type === 'completed') {
                            if (data.status_line) appendJobLine(jobKey, data.status_line);
                            finishImport(jobKey, true, data.status_line || 'Photo import completed');
                            if (window.ImportControls && typeof window.ImportControls.startThumbnailsAfterPhotoImport === 'function') {
                                void window.ImportControls.startThumbnailsAfterPhotoImport();
                            }
                        } else if (type === 'error') {
                            finishImport(jobKey, false, data.error_message || data.status_line || 'Import error');
                        } else if (type === 'cancelled') {
                            finishImport(jobKey, false, 'Import cancelled');
                        }
                    } catch (e) { /* ignore */ }
                };
                es.onerror = function () { /* SSE auto-reconnects */ };
            },
            /** After “Upload Photos from Device” finishes: start async thumbnail/EXIF job and mirror progress in Import status. */
            startThumbnailsAfterPhotoImport: async function () {
                try {
                    const jobKey = makeJobKey('thumbnails', {});
                    if (runningJobs.has(jobKey)) return;
                    const res = await fetch('/images/process-thumbnails/async', { method: 'POST', credentials: 'same-origin' });
                    if (res.status !== 202) return;

                    registerRunningJob(jobKey, 'thumbnails', {});
                    setExecuting('thumbnails', true, {});
                    appendJobLine(jobKey, 'Processing thumbnails for uploaded photos…');
                    closeEventSourceForJob(jobKey);
                    const es = new EventSource('/images/process-thumbnails/stream');
                    const job = runningJobs.get(jobKey);
                    if (job) job.eventSource = es;
                    es.onmessage = function (event) {
                        try {
                            const ed = JSON.parse(event.data);
                            const type = ed.type;
                            const data = ed.data || {};
                            if (type === 'progress' || type === 'status') {
                                const line = data.status_line || formatProgressLine('thumbnails', data);
                                appendJobLine(jobKey, line);
                            } else if (type === 'completed') {
                                finishImport(jobKey, true, data.status_line || formatProgressLine('thumbnails', data) || 'Thumbnail processing completed');
                            } else if (type === 'error') {
                                finishImport(jobKey, false, data.error_message || data.status_line || 'Thumbnail error');
                            } else if (type === 'cancelled') {
                                finishImport(jobKey, false, data.status_line || 'Cancelled');
                            }
                        } catch (e) { /* ignore */ }
                    };
                    es.onerror = function () { /* SSE reconnects */ };
                } catch (e) {
                    console.warn('startThumbnailsAfterPhotoImport:', e);
                }
            }
        };


        
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

        if (DOM.locationsSidebarBtn) {
            DOM.locationsSidebarBtn.addEventListener('click', () => {
                void Modals.Locations.open();
            });
        }

        if (DOM.emailGallerySidebarBtn) {
            DOM.emailGallerySidebarBtn.addEventListener('click', () => {
                Modals.EmailGallery.open();
            });
        }

        if (DOM.newImageGallerySidebarBtn) {
            DOM.newImageGallerySidebarBtn.addEventListener('click', () => {
                void Modals.NewImageGallery.open({ resetCriteriaAndSearch: true });
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
                openDashboardModal();
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

        const messageSimilaritySidebarBtn = document.getElementById('message-similarity-sidebar-btn');
        const messageSimilarityModal = document.getElementById('message-similarity-modal');
        const closeMessageSimilarityModalBtn = document.getElementById('close-message-similarity-modal');
        const messageSimilarityCancelBtn = document.getElementById('message-similarity-cancel-btn');
        const messageSimilarityRunBtn = document.getElementById('message-similarity-run-btn');
        const messageSimilarityText = document.getElementById('message-similarity-text');
        const messageSimilarityN = document.getElementById('message-similarity-n');
        const messageSimilarityStatus = document.getElementById('message-similarity-status');
        const messageSimilarityTabs = document.getElementById('message-similarity-tabs');
        const messageSimilarityResults = document.getElementById('message-similarity-results');
        const messageSimilarityClearBtn = document.getElementById('message-similarity-clear-btn');
        const messageSimilaritySensitivity = document.getElementById('message-similarity-sensitivity');
        const messageSimilaritySensitivityLabel = document.getElementById('message-similarity-sensitivity-label');
        const MESSAGE_SIMILARITY_SENSITIVITY_DEFAULT = 50;

        const sensitivityToMaxDistance = (sensitivity) => {
            const s = Math.max(0, Math.min(100, Number(sensitivity)));
            if (!Number.isFinite(s)) return 1.1;
            const minDist = 0.35;
            const maxDist = 1.85;
            return maxDist - (s / 100) * (maxDist - minDist);
        };

        const sensitivityDisplayLabel = (sensitivity) => {
            const s = Math.max(0, Math.min(100, Number(sensitivity)));
            if (s >= 85) return 'Very strict';
            if (s >= 65) return 'Strict';
            if (s >= 35) return 'Balanced';
            if (s >= 15) return 'Relaxed';
            return 'Broad';
        };

        const itemPassesSimilaritySensitivity = (item, maxDistance) => {
            const raw = item?.distance;
            if (raw == null || raw === '') {
                return maxDistance >= 1.05;
            }
            const d = Number(raw);
            return Number.isFinite(d) && d <= maxDistance;
        };

        const filterGroupedBySensitivity = (grouped, maxDistance) => {
            const out = {};
            Object.keys(grouped).forEach((key) => {
                const items = Array.isArray(grouped[key]) ? grouped[key] : [];
                out[key] = items.filter((item) => itemPassesSimilaritySensitivity(item, maxDistance));
            });
            return out;
        };

        const formatSimilarityDistance = (distance) => {
            const d = Number(distance);
            if (Number.isFinite(d)) return d.toFixed(3);
            return '—';
        };

        const updateMessageSimilaritySensitivityLabel = () => {
            if (!messageSimilaritySensitivity || !messageSimilaritySensitivityLabel) return;
            const val = parseInt(messageSimilaritySensitivity.value, 10);
            messageSimilaritySensitivityLabel.textContent = sensitivityDisplayLabel(val);
            messageSimilaritySensitivity.setAttribute('aria-valuenow', String(val));
        };

        const closeMessageSimilarityModal = () => {
            if (!messageSimilarityModal) return;
            messageSimilarityModal.style.display = 'none';
        };

        const setMessageSimilarityStatus = (msg, isError = false) => {
            if (!messageSimilarityStatus) return;
            if (!msg) {
                messageSimilarityStatus.style.display = 'none';
                messageSimilarityStatus.textContent = '';
                return;
            }
            messageSimilarityStatus.style.display = 'block';
            messageSimilarityStatus.textContent = msg;
            messageSimilarityStatus.style.color = isError ? '#b91c1c' : '';
        };

        const messageSimilarityMediaMeta = {
            messages: { label: 'Messages' },
            emails: { label: 'Emails' },
            images: { label: 'Images' },
            artefacts: { label: 'Artefacts' },
            facebook_posts: { label: 'Facebook Posts' },
            facebook_albums: { label: 'Facebook Albums' }
        };

        const escapeHtml = (value) => String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');

        const normalizeMessageSimilarityResults = (data) => {
            const results = Array.isArray(data?.results) ? data.results : [];
            const uniqueMessages = Array.isArray(data?.unique_messages) ? data.unique_messages : [];
            const distanceById = new Map();
            results.forEach((row) => {
                const distance = row?.distance;
                const ids = Array.isArray(row?.int_ids) ? row.int_ids : [];
                ids.forEach((id) => {
                    const key = String(id);
                    if (!distanceById.has(key)) distanceById.set(key, distance);
                });
            });
            return uniqueMessages.map((m) => ({
                mediaType: 'messages',
                id: m?.id,
                title: m?.chat_session || 'Conversation',
                primaryText: m?.text || '',
                secondaryText: '',
                distance: distanceById.get(String(m?.id)),
                preview: m?.text || '',
                raw: m
            }));
        };

        const normalizeImageSimilarityResults = (data) => {
            const rows = Array.isArray(data?.results) ? data.results : [];
            return rows.map((r) => ({
                mediaType: 'images',
                id: r?.id,
                title: r?.title || `Image ${r?.id ?? ''}`.trim(),
                primaryText: r?.description || r?.tags || r?.notes || 'No description',
                secondaryText: r?.media_type || '',
                distance: r?.distance,
                previewImageUrl: r?.id ? `/images/${r.id}/thumbnail` : '',
                raw: r
            }));
        };

        const normalizeFacebookPostSimilarityResults = (data) => {
            const rows = Array.isArray(data?.results) ? data.results : [];
            return rows.map((r) => ({
                mediaType: 'facebook_posts',
                id: r?.id,
                title: r?.title || `Post ${r?.id ?? ''}`.trim(),
                primaryText: r?.post_text || '(no post text)',
                secondaryText: r?.timestamp || '',
                distance: r?.distance,
                raw: r
            }));
        };

        const normalizeFacebookAlbumSimilarityResults = (data) => {
            const rows = Array.isArray(data?.results) ? data.results : [];
            return rows.map((r) => ({
                mediaType: 'facebook_albums',
                id: r?.id,
                title: r?.name || `Album ${r?.id ?? ''}`.trim(),
                primaryText: r?.description || '(no album description)',
                secondaryText: r?.cover_photo_uri || '',
                distance: r?.distance,
                raw: r
            }));
        };

        const normalizeEmailSimilarityResults = (data) => {
            const rows = Array.isArray(data?.results) ? data.results : [];
            return rows.map((r) => ({
                mediaType: 'emails',
                id: r?.id,
                title: r?.subject || `Email ${r?.id ?? ''}`.trim(),
                primaryText: r?.snippet || r?.plain_text || '(no preview)',
                secondaryText: r?.from_address || '',
                distance: r?.distance,
                raw: r
            }));
        };

        const normalizeArtefactSimilarityResults = (data) => {
            const rows = Array.isArray(data?.results) ? data.results : [];
            return rows.map((r) => ({
                mediaType: 'artefacts',
                id: r?.id,
                title: r?.name || `Artefact ${r?.id ?? ''}`.trim(),
                primaryText: r?.description || r?.story || r?.tags || 'No description',
                secondaryText: r?.tags || '',
                distance: r?.distance,
                previewImageUrl: r?.id ? `/artefacts/${r.id}/thumbnail` : '',
                raw: r
            }));
        };

        const openMessageSimilarityResult = (item) => {
            if (!item) return;
            switch (item.mediaType) {
                case 'messages':
                    if (item.id && Modals.SMSMessages && typeof Modals.SMSMessages.openAndSelectConversation === 'function') {
                        Modals.SMSMessages.openAndSelectConversation(item.id);
                    }
                    return;
                case 'emails':
                    if (item.id != null && item.id !== '') {
                        if (Modals.EmailGallery && typeof Modals.EmailGallery.openAndSelectEmail === 'function') {
                            void Modals.EmailGallery.openAndSelectEmail(item.id);
                        }
                    }
                    return;
                case 'images':
                    if (item.raw && Modals.ImageDetailModal && typeof Modals.ImageDetailModal.open === 'function') {
                        Modals.ImageDetailModal.open(item.raw, { allowRedirects: true });
                        return;
                    }
                    if (Modals.NewImageGallery && typeof Modals.NewImageGallery.open === 'function') {
                        void Modals.NewImageGallery.open({ resetCriteriaAndSearch: true });
                    }
                    return;
                case 'facebook_posts':
                    if (item.id && Modals.FBPosts && typeof Modals.FBPosts.openAndFilterOnPosts === 'function') {
                        void Modals.FBPosts.openAndFilterOnPosts([item.id]);
                    }
                    return;
                case 'facebook_albums':
                    if (item.id && Modals.FBAlbums && typeof Modals.FBAlbums.openAndSelectAlbum === 'function') {
                        void Modals.FBAlbums.openAndSelectAlbum(item.id);
                    }
                    return;
                case 'artefacts':
                    if (item.id != null && item.id !== '') {
                        if (Modals.Artefacts && typeof Modals.Artefacts.openAndSelectArtefact === 'function') {
                            void Modals.Artefacts.openAndSelectArtefact(item.id);
                        }
                    }
                    return;
                default:
                    return;
            }
        };

        const renderMessageSimilarityResults = (payload) => {
            if (!messageSimilarityResults) return;
            messageSimilarityResults.innerHTML = '';
            messageSimilarityResults.style.display = 'block';
            if (messageSimilarityTabs) { messageSimilarityTabs.innerHTML = ''; messageSimilarityTabs.style.display = 'flex'; }
            const grouped = payload?.grouped || {};
            const errors = Array.isArray(payload?.errors) ? payload.errors : [];
            const mediaOrder = ['messages', 'emails', 'images', 'artefacts', 'facebook_posts', 'facebook_albums'];
            const totalCount = mediaOrder.reduce((acc, key) => acc + ((grouped[key] || []).length), 0);
            if (totalCount === 0) {
                const empty = document.createElement('div');
                empty.className = 'app-card';
                empty.textContent = errors.length ? 'No matches returned from available sources.' : 'No matches found.';
                messageSimilarityResults.appendChild(empty);
                if (errors.length) {
                    const err = document.createElement('div');
                    err.className = 'app-card';
                    err.style.border = '1px solid #fecaca';
                    err.style.background = '#fff1f2';
                    err.style.padding = '10px';
                    err.innerHTML = `<strong>Sources with errors:</strong><br>${errors.map(e => escapeHtml(e)).join('<br>')}`;
                    messageSimilarityResults.appendChild(err);
                }
                return;
            }
            const tabsBar = document.createElement('div');
            tabsBar.style.display = 'flex';
            tabsBar.style.flexWrap = 'wrap';
            tabsBar.style.gap = '8px';
            tabsBar.style.marginBottom = '10px';

            const panelsWrap = document.createElement('div');
            const panelByType = {};
            const tabByType = {};
            let firstType = '';

            mediaOrder.forEach((mediaType) => {
                const items = grouped[mediaType] || [];
                const label = (messageSimilarityMediaMeta[mediaType] && messageSimilarityMediaMeta[mediaType].label) || mediaType;
                if (!firstType) firstType = mediaType;

                const tabBtn = document.createElement('button');
                tabBtn.type = 'button';
                tabBtn.className = 'modal-btn modal-btn-secondary';
                tabBtn.textContent = `${label} (${items.length})`;
                tabBtn.style.padding = '6px 10px';
                tabBtn.style.fontSize = '0.9em';
                tabByType[mediaType] = tabBtn;
                tabsBar.appendChild(tabBtn);

                const section = document.createElement('div');
                section.className = 'app-card';
                section.style.padding = '10px';
                section.style.border = '1px solid var(--color-border)';
                section.style.borderRadius = '8px';
                section.style.display = 'none';
                panelByType[mediaType] = section;

                if (!items.length) {
                    const empty = document.createElement('div');
                    empty.style.color = 'var(--color-text-muted)';
                    empty.style.fontSize = '0.9em';
                    empty.textContent = 'No matches in this media type.';
                    section.appendChild(empty);
                    panelsWrap.appendChild(section);
                    return;
                }

                items.forEach((item) => {
                    const row = document.createElement('div');
                    row.className = 'message-similarity-result-row';

                    const left = document.createElement('div');
                    left.className = 'message-similarity-result-main';
                    if (item.previewImageUrl) {
                        const thumb = document.createElement('img');
                        thumb.src = item.previewImageUrl;
                        thumb.alt = item.title || 'Image preview';
                        thumb.className = 'message-similarity-result-thumb';
                        left.appendChild(thumb);
                    }
                    const title = document.createElement('div');
                    title.className = 'message-similarity-result-title';
                    title.textContent = item.title || `${label} item`;
                    left.appendChild(title);

                    const preview = document.createElement('div');
                    preview.className = 'message-similarity-result-preview';
                    preview.textContent = item.primaryText || '';
                    left.appendChild(preview);

                    const meta = document.createElement('div');
                    meta.className = 'message-similarity-result-meta';
                    const metaParts = [`Distance: ${formatSimilarityDistance(item.distance)}`];
                    if (item.secondaryText) metaParts.push(item.secondaryText);
                    meta.textContent = metaParts.join(' · ');
                    left.appendChild(meta);

                    const right = document.createElement('div');
                    right.className = 'message-similarity-result-actions';
                    const openBtn = document.createElement('button');
                    openBtn.type = 'button';
                    openBtn.className = 'modal-btn modal-btn-secondary';
                    openBtn.textContent = 'Open';
                    openBtn.addEventListener('click', () => openMessageSimilarityResult(item));
                    right.appendChild(openBtn);

                    row.appendChild(left);
                    row.appendChild(right);
                    section.appendChild(row);
                });

                panelsWrap.appendChild(section);
            });

            const activateTab = (mediaType) => {
                mediaOrder.forEach((key) => {
                    const panel = panelByType[key];
                    const tab = tabByType[key];
                    const active = key === mediaType;
                    if (panel) panel.style.display = active ? 'block' : 'none';
                    if (tab) {
                        tab.style.background = active ? 'var(--color-bg-alt)' : '';
                        tab.style.borderColor = active ? 'var(--color-blue)' : '';
                    }
                });
            };

            mediaOrder.forEach((mediaType) => {
                const tab = tabByType[mediaType];
                if (tab) tab.addEventListener('click', () => activateTab(mediaType));
            });
            if (firstType) activateTab(firstType);

            if (messageSimilarityTabs) messageSimilarityTabs.appendChild(tabsBar);
            messageSimilarityResults.appendChild(panelsWrap);

            if (errors.length) {
                const err = document.createElement('div');
                err.className = 'app-card';
                err.style.border = '1px solid #fecaca';
                err.style.background = '#fff1f2';
                err.style.padding = '10px';
                err.innerHTML = `<strong>Some sources failed:</strong><br>${errors.map(e => escapeHtml(e)).join('<br>')}`;
                messageSimilarityResults.appendChild(err);
            }
        };

        if (messageSimilaritySidebarBtn && messageSimilarityModal) {
            messageSimilaritySidebarBtn.addEventListener('click', () => {
                messageSimilarityModal.style.display = 'flex';
                if (messageSimilarityText) messageSimilarityText.focus();
            });
        }
        if (closeMessageSimilarityModalBtn) closeMessageSimilarityModalBtn.addEventListener('click', closeMessageSimilarityModal);
        if (messageSimilarityCancelBtn) messageSimilarityCancelBtn.addEventListener('click', closeMessageSimilarityModal);
        if (messageSimilaritySensitivity) {
            messageSimilaritySensitivity.addEventListener('input', updateMessageSimilaritySensitivityLabel);
            updateMessageSimilaritySensitivityLabel();
        }
        if (messageSimilarityClearBtn) {
            messageSimilarityClearBtn.addEventListener('click', () => {
                if (messageSimilarityText) messageSimilarityText.value = '';
                if (messageSimilarityN) messageSimilarityN.value = '25';
                if (messageSimilaritySensitivity) {
                    messageSimilaritySensitivity.value = String(MESSAGE_SIMILARITY_SENSITIVITY_DEFAULT);
                    updateMessageSimilaritySensitivityLabel();
                }
                setMessageSimilarityStatus('');
                if (messageSimilarityTabs) { messageSimilarityTabs.innerHTML = ''; messageSimilarityTabs.style.display = 'none'; }
                if (messageSimilarityResults) { messageSimilarityResults.innerHTML = ''; messageSimilarityResults.style.display = 'none'; }
            });
        }
        if (messageSimilarityModal) {
            messageSimilarityModal.addEventListener('click', (e) => {
                if (e.target === messageSimilarityModal) closeMessageSimilarityModal();
            });
        }
        if (messageSimilarityRunBtn) {
            messageSimilarityRunBtn.addEventListener('click', async () => {
                const text = (messageSimilarityText?.value || '').trim();
                const nVal = parseInt(messageSimilarityN?.value || '10', 10);
                const n = Number.isFinite(nVal) && nVal > 0 ? nVal : 10;
                if (!text) {
                    setMessageSimilarityStatus('Please enter text to search.', true);
                    return;
                }
                setMessageSimilarityStatus('');
                if (messageSimilarityResults) messageSimilarityResults.innerHTML = '';
                if (messageSimilarityTabs) messageSimilarityTabs.innerHTML = '';
                const originalBtnHTML = messageSimilarityRunBtn.innerHTML;
                messageSimilarityRunBtn.disabled = true;
                messageSimilarityRunBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Searching…';
                try {
                    const sources = [
                        {
                            id: 'messages',
                            label: 'Messages',
                            endpoint: '/imessages/similarity-search/unique',
                            normalize: normalizeMessageSimilarityResults
                        },
                        {
                            id: 'emails',
                            label: 'Emails',
                            endpoint: '/emails/similar-by-text',
                            normalize: normalizeEmailSimilarityResults
                        },
                        {
                            id: 'images',
                            label: 'Images',
                            endpoint: '/images/similar-by-tags',
                            normalize: normalizeImageSimilarityResults
                        },
                        {
                            id: 'artefacts',
                            label: 'Artefacts',
                            endpoint: '/artefacts/similar-by-text',
                            normalize: normalizeArtefactSimilarityResults
                        },
                        {
                            id: 'facebook_posts',
                            label: 'Facebook Posts',
                            endpoint: '/facebook/posts/similar-by-text',
                            normalize: normalizeFacebookPostSimilarityResults
                        },
                        {
                            id: 'facebook_albums',
                            label: 'Facebook Albums',
                            endpoint: '/facebook/albums/similar-by-description',
                            normalize: normalizeFacebookAlbumSimilarityResults
                        }
                    ];

                    const settled = await Promise.allSettled(sources.map(async (src) => {
                        const res = await fetch(src.endpoint, {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            credentials: 'same-origin',
                            body: JSON.stringify({ text, n })
                        });
                        const data = await res.json().catch(() => ({}));
                        if (!res.ok) {
                            throw new Error(data.detail || data.error || `${src.label} request failed (${res.status})`);
                        }
                        const items = src.normalize(data || {});
                        return { source: src, items };
                    }));

                    const grouped = {
                        messages: [],
                        emails: [],
                        images: [],
                        artefacts: [],
                        facebook_posts: [],
                        facebook_albums: []
                    };
                    const errors = [];
                    settled.forEach((entry, idx) => {
                        const src = sources[idx];
                        if (entry.status === 'fulfilled') {
                            const items = Array.isArray(entry.value.items) ? entry.value.items.slice() : [];
                            items.sort((a, b) => {
                                const da = Number(a?.distance);
                                const db = Number(b?.distance);
                                const fa = Number.isFinite(da);
                                const fb = Number.isFinite(db);
                                if (fa && fb) return da - db;
                                if (fa) return -1;
                                if (fb) return 1;
                                return String(a?.title || '').localeCompare(String(b?.title || ''));
                            });
                            grouped[src.id] = items;
                        } else {
                            grouped[src.id] = [];
                            errors.push(`${src.label}: ${entry.reason?.message || entry.reason || 'Unknown error'}`);
                        }
                    });

                    const rawTotal = Object.values(grouped).reduce((acc, arr) => acc + (Array.isArray(arr) ? arr.length : 0), 0);
                    const sensitivityVal = parseInt(
                        messageSimilaritySensitivity?.value || String(MESSAGE_SIMILARITY_SENSITIVITY_DEFAULT),
                        10
                    );
                    const maxDistance = sensitivityToMaxDistance(sensitivityVal);
                    const groupedFiltered = filterGroupedBySensitivity(grouped, maxDistance);
                    const total = Object.values(groupedFiltered).reduce((acc, arr) => acc + (Array.isArray(arr) ? arr.length : 0), 0);
                    const okSourceCount = sources.length - errors.length;
                    const sensitivityNote = total < rawTotal
                        ? ` (${total} after ${sensitivityDisplayLabel(sensitivityVal).toLowerCase()} sensitivity filter)`
                        : '';
                    if (errors.length > 0) {
                        setMessageSimilarityStatus(`Found ${rawTotal} match(es) across ${okSourceCount}/${sources.length} sources${sensitivityNote}.`, false);
                    } else {
                        setMessageSimilarityStatus(`Found ${rawTotal} match(es) across ${sources.length} sources${sensitivityNote}.`, false);
                    }
                    renderMessageSimilarityResults({ grouped: groupedFiltered, errors });
                } catch (err) {
                    setMessageSimilarityStatus(`Search failed: ${err.message || err}`, true);
                } finally {
                    messageSimilarityRunBtn.innerHTML = originalBtnHTML;
                    messageSimilarityRunBtn.disabled = false;
                }
            });
        }

        const profilesSidebarBtn = document.getElementById('profiles-sidebar-btn');
        if (profilesSidebarBtn) {
            profilesSidebarBtn.addEventListener('click', () => {
                Modals.Profiles.open();
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
                refreshSettingsDataImportModalLLM();
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

        const dataSourcesImportSidebarBtn = document.getElementById('data-sources-import-sidebar-btn');
        const dataSourcesImportModal = document.getElementById('data-sources-import-modal');
        const closeDataSourcesImportModalBtn = document.getElementById('close-data-sources-import-modal');
        if (dataSourcesImportSidebarBtn && dataSourcesImportModal) {
            dataSourcesImportSidebarBtn.addEventListener('click', () => {
                if (isDataImportSidebarMasterUnlockVisible()) {
                    dataImportMasterUnlockedCached = true;
                    openDataSourcesImportModalUI();
                    return;
                }
                void openDataSourcesImportModalAfterKeyCheck();
            });
        }
        const closeDataSourcesImportModal = () => {
            if (dataSourcesImportModal) dataSourcesImportModal.style.display = 'none';
        };
        if (closeDataSourcesImportModalBtn && dataSourcesImportModal) {
            closeDataSourcesImportModalBtn.addEventListener('click', closeDataSourcesImportModal);
        }
        if (dataSourcesImportModal) {
            dataSourcesImportModal.addEventListener('click', (e) => {
                if (e.target === dataSourcesImportModal) closeDataSourcesImportModal();
            });
        }

        const dataImportSidebarBtn = document.getElementById('data-import-sidebar-btn');
        const dataImportModal = document.getElementById('data-import-modal');
        const closeDataImportModalBtn = document.getElementById('close-data-import-modal');
        if (dataImportSidebarBtn && dataImportModal) {
            dataImportSidebarBtn.addEventListener('click', () => {
                if (isDataImportSidebarMasterUnlockVisible()) {
                    dataImportMasterUnlockedCached = true;
                    openDataImportModalUI();
                    return;
                }
                void openDataImportModalAfterKeyCheck();
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

        /** @param {Array<{name?: string}>|null|undefined} interestRows rows from GET /api/interests */
        const runTodaysThing = (interestRows) => {
            const rows = Array.isArray(interestRows) ? interestRows : [];
            const names = rows
                .map((r) => (r && r.name != null ? String(r.name).trim() : ''))
                .filter((s) => s.length > 0);
            let userPrompt = "What's today's things of interest? Suggest something interesting for today based on my interests.";
            if (names.length > 0) {
                userPrompt += "\n\nThe archive owner's saved interests (use these as the main inspiration; you may combine topics or focus on one):\n";
                userPrompt += names.map((n) => `- ${n.replace(/\r?\n/g, ' ')}`).join('\n');
            }
            App.processFormSubmit(
                userPrompt,
                "Today's Things of Interest",
                "What's going on today?"
            );
        };

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
                    try {
                        const listRes = await fetch('/api/interests');
                        const listData = listRes.ok ? await listRes.json() : [];
                        runTodaysThing(Array.isArray(listData) ? listData : []);
                    } catch {
                        runTodaysThing([{ name }]);
                    }
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
                    if (!res.ok) { runTodaysThing([]); return; }
                    const data = await res.json();
                    if (data && data.length > 0) {
                        runTodaysThing(data);
                        return;
                    }
                    if (todaysThingAddInterestModal && todaysThingAddInterestInput) {
                        todaysThingAddInterestInput.value = '';
                        if (todaysThingAddInterestError) { todaysThingAddInterestError.style.display = 'none'; todaysThingAddInterestError.textContent = ''; }
                        todaysThingAddInterestModal.style.display = 'flex';
                    } else {
                        runTodaysThing([]);
                    }
                } catch (e) {
                    runTodaysThing([]);
                }
            });
        }

        setupWelcomeOnboardingUI();
        void syncOnboardingUnlockStatus();

    }

    async function loadLLMProviderAvailability() {
        const selects = LLM_PROVIDER_SELECT_IDS
            .map((id) => document.getElementById(id))
            .filter(Boolean);
        if (!selects.length) return;

        function applyProviderAvailabilityToSelect(sel, availMap, models) {
            if (!sel) return;
            const includeAuto = sel.querySelector('option[value="auto"]') != null;
            const providers = [
                ...(includeAuto ? [{ value: 'auto', label: 'Auto' }] : []),
                ...models.map((m) => ({ value: m.key, label: m.display_name || m.key })),
                { value: 'localai', label: 'Local AI' },
            ];

            const previous = sel.value;
            const userSelected = sel.dataset.userSelectedProvider === 'true';

            while (sel.options.length > 0) {
                sel.remove(0);
            }

            providers.forEach((p) => {
                if (!availMap[p.value]) return;
                const opt = document.createElement('option');
                opt.value = p.value;
                opt.textContent = `${p.label} \u2705`;
                sel.appendChild(opt);
            });

            if (sel.options.length === 0 && providers.length > 0) {
                const fallback = document.createElement('option');
                fallback.value = providers[providers.length > 1 ? 1 : 0].value;
                fallback.textContent = providers[providers.length > 1 ? 1 : 0].label;
                sel.appendChild(fallback);
            }

            let next = previous;
            if (!availMap[previous]) {
                next = firstAvailableProvider(availMap, includeAuto);
            } else if (!userSelected && sel.id === 'llm-provider-select') {
                next = firstAvailableProvider(availMap, true);
            }
            if (availMap[next]) {
                sel.value = next;
            } else if (sel.options.length > 0) {
                sel.selectedIndex = 0;
            }
        }

        try {
            const [avRes, useEnabled] = await Promise.all([
                fetch('/chat/availability', { credentials: 'same-origin' }),
                fetchLocalAIUseEnabledFromServer(),
            ]);
            if (!avRes.ok) return;
            const av = await avRes.json();
            av.localai_use_enabled = resolveLocalAIUseEnabled(useEnabled);
            const availMap = buildProviderAvailabilityMap(av);
            const models = Array.isArray(av.models) ? av.models : [];
            if (typeof AIModelLabels !== 'undefined') AIModelLabels.set(models);
            selects.forEach((sel) => applyProviderAvailabilityToSelect(sel, availMap, models));
            if (typeof Modals !== 'undefined'
                && Modals.AutoRoutingConfig
                && Modals.AutoRoutingConfig.reconcileClassifierProvider) {
                Modals.AutoRoutingConfig.reconcileClassifierProvider();
            }
            if (typeof UI !== 'undefined' && UI.updateChatContextStatusBarFromAvailability) {
                UI.updateChatContextStatusBarFromAvailability(av);
            }
            if (typeof UI !== 'undefined' && UI.refreshOpenRouterCredits) {
                void UI.refreshOpenRouterCredits();
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

    // Phase 1 onboarding persistence is localStorage-based.
    // Phase 2 (optional): persist this state server-side per user for multi-device continuity.
    // Suggested contract:
    //   GET  /auth/me/ui-preferences -> { onboarding: { dont_show_again, checklist, version } }
    //   PATCH /auth/me/ui-preferences (same shape) with localStorage as fallback cache.
    const ONBOARDING_SCHEMA_VERSION = 1;
    const ONBOARDING_STATE_PREFIX = 'dm_onboarding_state_v1';
    const ONBOARDING_STEPS = ['import_data', 'subject_config', 'identity_profile', 'ask_first_question', 'unlock_sensitive'];

    function onboardingDefaultState() {
        return {
            version: ONBOARDING_SCHEMA_VERSION,
            dontShowAgain: false,
            checklist: {
                import_data: false,
                subject_config: false,
                identity_profile: false,
                ask_first_question: false,
                unlock_sensitive: false
            }
        };
    }

    function getOnboardingIdentityKey() {
        try {
            const authUser = (typeof AuthModule !== 'undefined' && AuthModule.getUser) ? AuthModule.getUser() : null;
            if (authUser && authUser.id != null) return `uid_${String(authUser.id)}`;
            const legacy = localStorage.getItem('userId');
            if (legacy && legacy.trim()) return `legacy_${legacy.trim()}`;
        } catch (e) {
            // ignore storage/auth lookup issues
        }
        return 'anonymous';
    }

    function getOnboardingStorageKey() {
        return `${ONBOARDING_STATE_PREFIX}_${getOnboardingIdentityKey()}`;
    }

    function getOnboardingStorageKeys() {
        const keys = [getOnboardingStorageKey(), `${ONBOARDING_STATE_PREFIX}_anonymous`];
        const seen = new Set();
        return keys.filter((k) => {
            if (!k || seen.has(k)) return false;
            seen.add(k);
            return true;
        });
    }

    function readOnboardingState() {
        const base = onboardingDefaultState();
        try {
            const out = {
                version: ONBOARDING_SCHEMA_VERSION,
                dontShowAgain: false,
                checklist: { ...base.checklist }
            };
            let foundAny = false;
            getOnboardingStorageKeys().forEach((key) => {
                const raw = localStorage.getItem(key);
                if (!raw) return;
                const parsed = JSON.parse(raw);
                if (!parsed || typeof parsed !== 'object') return;
                foundAny = true;
                out.dontShowAgain = out.dontShowAgain || !!parsed.dontShowAgain;
                if (parsed.checklist && typeof parsed.checklist === 'object') {
                    ONBOARDING_STEPS.forEach((step) => {
                        out.checklist[step] = out.checklist[step] || !!parsed.checklist[step];
                    });
                }
            });
            return foundAny ? out : base;
        } catch (e) {
            return base;
        }
    }

    function writeOnboardingState(state) {
        try {
            const serialized = JSON.stringify(state);
            getOnboardingStorageKeys().forEach((key) => {
                localStorage.setItem(key, serialized);
            });
        } catch (e) {
            // ignore storage failures
        }
    }

    function setOnboardingDontShowAgain(flag) {
        const state = readOnboardingState();
        state.dontShowAgain = !!flag;
        writeOnboardingState(state);
    }

    function resetOnboardingState() {
        writeOnboardingState(onboardingDefaultState());
    }

    function markOnboardingChecklistStepDone(stepKey) {
        if (!ONBOARDING_STEPS.includes(stepKey)) return;
        const state = readOnboardingState();
        state.checklist[stepKey] = true;
        writeOnboardingState(state);
        renderOnboardingChecklistState();
    }
    window.markOnboardingChecklistStepDone = markOnboardingChecklistStepDone;

    function renderOnboardingChecklistState() {
        const state = readOnboardingState();
        ONBOARDING_STEPS.forEach((step) => {
            const row = document.querySelector(`.onboarding-checklist-item[data-onboarding-step="${step}"]`);
            const status = document.getElementById(`onboarding-step-status-${step}`);
            const done = !!(state.checklist && state.checklist[step]);
            if (row) row.classList.toggle('onboarding-checklist-item--done', done);
            if (status) {
                status.textContent = done ? 'Done' : (step === 'unlock_sensitive' ? 'Optional' : 'Pending');
            }
        });
        const dontShow = document.getElementById('welcome-dont-show-again-checkbox');
        if (dontShow) dontShow.checked = !!state.dontShowAgain;
    }

    /** Shows/hides the welcome dialog's "no OpenRouter API key configured" warning banner. */
    async function checkWelcomeOpenRouterKeyWarning() {
        const warn = document.getElementById('welcome-openrouter-key-warning');
        if (!warn) return;
        try {
            const res = await fetch('/chat/availability', { credentials: 'same-origin' });
            if (!res.ok) { warn.style.display = 'none'; return; }
            const av = await res.json();
            const models = Array.isArray(av.models) ? av.models : [];
            const anyHostedOk = models.some((m) => m && m.available);
            warn.style.display = anyHostedOk ? 'none' : 'flex';
        } catch (_) {
            warn.style.display = 'none';
        }
    }

    function showWelcomeOnboardingModal() {
        const modal = document.getElementById('info-box-modal');
        if (!modal) return;
        modal.classList.remove('info-box-modal-closed');
        modal.style.display = 'flex';
        if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(false);
        renderOnboardingChecklistState();
        void loadArchiveOverviewLLMStatus();
        void checkWelcomeOpenRouterKeyWarning();
    }

    function restartOnboardingFlow() {
        resetOnboardingState();
        showWelcomeOnboardingModal();
    }

    function setupWelcomeOnboardingUI() {
        renderOnboardingChecklistState();
        const dontShow = document.getElementById('welcome-dont-show-again-checkbox');
        if (dontShow && !dontShow.dataset.onboardingWired) {
            dontShow.dataset.onboardingWired = '1';
            dontShow.addEventListener('change', () => {
                setOnboardingDontShowAgain(dontShow.checked);
                if (dontShow.checked && window.AppDialogs && typeof window.AppDialogs.showAppAlert === 'function') {
                    void window.AppDialogs.showAppAlert(
                        'Welcome dialog',
                        'You can restart onboarding anytime from Guide → Reeanable Welcome Dialog.'
                    );
                }
            });
        }

        const openRouterWarnBtn = document.getElementById('welcome-openrouter-key-warning-btn');
        if (openRouterWarnBtn && !openRouterWarnBtn.dataset.onboardingWired) {
            openRouterWarnBtn.dataset.onboardingWired = '1';
            openRouterWarnBtn.addEventListener('click', () => {
                if (window.closeInfoBoxModal) window.closeInfoBoxModal({ skipMasterPrompt: true });
                if (typeof window.openConfigurationToTab === 'function') window.openConfigurationToTab('api-keys');
            });
        }

        const skipBtn = document.getElementById('welcome-skip-for-now-btn');
        if (skipBtn && !skipBtn.dataset.onboardingWired) {
            skipBtn.dataset.onboardingWired = '1';
            skipBtn.addEventListener('click', () => {
                if (window.closeInfoBoxModal) window.closeInfoBoxModal();
            });
        }

        const openGuideBtn = document.getElementById('welcome-open-guide-btn');
        if (openGuideBtn && !openGuideBtn.dataset.onboardingWired) {
            openGuideBtn.dataset.onboardingWired = '1';
            openGuideBtn.addEventListener('click', () => {
                if (window.closeInfoBoxModal) window.closeInfoBoxModal({ skipMasterPrompt: true });
                if (typeof Guide !== 'undefined' && Guide.openGuideModal) Guide.openGuideModal();
            });
        }

        const welcomeDashboardBtn = document.getElementById('welcome-open-dashboard-btn');
        if (welcomeDashboardBtn && !welcomeDashboardBtn.dataset.onboardingWired) {
            welcomeDashboardBtn.dataset.onboardingWired = '1';
            welcomeDashboardBtn.addEventListener('click', () => {
                if (typeof window.openDashboardModal === 'function') window.openDashboardModal();
            });
        }

        const identityProfileWelcomeBtn = document.getElementById('welcome-identity-profile-btn');
        if (identityProfileWelcomeBtn && !identityProfileWelcomeBtn.dataset.onboardingWired) {
            identityProfileWelcomeBtn.dataset.onboardingWired = '1';
            identityProfileWelcomeBtn.addEventListener('click', () => {
                if (window.closeInfoBoxModal) window.closeInfoBoxModal({ skipMasterPrompt: true });
                if (typeof Modals !== 'undefined' && Modals.IdentityProfileWizard) Modals.IdentityProfileWizard.open();
            });
        }

        const quickSetupBtn = document.getElementById('welcome-start-quick-setup-btn');
        if (quickSetupBtn && !quickSetupBtn.dataset.onboardingWired) {
            quickSetupBtn.dataset.onboardingWired = '1';
            quickSetupBtn.addEventListener('click', () => {
                if (window.closeInfoBoxModal) window.closeInfoBoxModal({ skipMasterPrompt: true });
                if (typeof Guide !== 'undefined' && Guide.onTopicSelected) Guide.onTopicSelected('FirstRunQuickSetup');
            });
        }

        const restartBtn = document.getElementById('guide-restart-onboarding-btn');
        if (restartBtn && !restartBtn.dataset.onboardingWired) {
            restartBtn.dataset.onboardingWired = '1';
            restartBtn.addEventListener('click', () => {
                if (typeof Guide !== 'undefined' && Guide.closeAll) Guide.closeAll();
                restartOnboardingFlow();
            });
        }

        const importBtn = document.getElementById('onboarding-checklist-import-btn');
        if (importBtn && !importBtn.dataset.onboardingWired) {
            importBtn.dataset.onboardingWired = '1';
            importBtn.addEventListener('click', () => {
                const dataImportBtn = document.getElementById('data-sources-import-sidebar-btn');
                if (dataImportBtn) dataImportBtn.click();
            });
        }

        const subjectBtn = document.getElementById('onboarding-checklist-subject-btn');
        if (subjectBtn && !subjectBtn.dataset.onboardingWired) {
            subjectBtn.dataset.onboardingWired = '1';
            subjectBtn.addEventListener('click', () => {
                const configBtn = document.getElementById('settings-data-import-sidebar-btn');
                if (configBtn) configBtn.click();
                setTimeout(() => {
                    const subjectTab = document.querySelector('.config-tab-button[data-tab="subject-configuration"]');
                    if (subjectTab) subjectTab.click();
                }, 120);
            });
        }

        const questionBtn = document.getElementById('onboarding-checklist-question-btn');
        if (questionBtn && !questionBtn.dataset.onboardingWired) {
            questionBtn.dataset.onboardingWired = '1';
            questionBtn.addEventListener('click', () => {
                if (DOM.userInput) DOM.userInput.focus();
            });
        }

        const unlockBtn = document.getElementById('onboarding-checklist-unlock-btn');
        if (unlockBtn && !unlockBtn.dataset.onboardingWired) {
            unlockBtn.dataset.onboardingWired = '1';
            unlockBtn.addEventListener('click', () => {
                const modal = document.getElementById('master-key-unlock-modal');
                const input = document.getElementById('master-key-unlock-input');
                if (modal) modal.style.display = 'flex';
                if (input) input.focus();
            });
        }

        if (DOM.chatForm && !DOM.chatForm.dataset.onboardingWired) {
            DOM.chatForm.dataset.onboardingWired = '1';
            DOM.chatForm.addEventListener('submit', () => {
                const userPrompt = DOM.userInput ? DOM.userInput.value.trim() : '';
                if (userPrompt) markOnboardingChecklistStepDone('ask_first_question');
            });
        }

        const saveSubject = document.getElementById('save-subject-config-btn');
        if (saveSubject && !saveSubject.dataset.onboardingWired) {
            saveSubject.dataset.onboardingWired = '1';
            saveSubject.addEventListener('click', () => {
                markOnboardingChecklistStepDone('subject_config');
            });
        }
    }

    function shouldShowOnboardingWelcome() {
        const state = readOnboardingState();
        return !state.dontShowAgain;
    }

    async function syncOnboardingUnlockStatus() {
        try {
            const st = await fetch('/api/session/master-key/status', { credentials: 'same-origin' });
            if (!st.ok) return;
            const sj = await st.json();
            if (sj && sj.unlocked) markOnboardingChecklistStepDone('unlock_sensitive');
        } catch (e) {
            // ignore
        }
    }

    /** After welcome info box: offer master key unlock if keyring exists and server RAM has no key yet. */
    function maybePromptMasterKeyUnlock() {
        (async () => {
            try {
                // If the user is authenticated, the login flow auto-unlocks the keyring — no prompt needed.
                if (typeof AuthModule !== 'undefined' && AuthModule.getUser && AuthModule.getUser()) return;
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

    /** Visitor-key login (not share-link visitors): hide owner-only onboarding and profile UI. */
    function applyVisitorKeyOwnerOnlyUI(isVisitorKeySession) {
        const hidden = !!isVisitorKeySession;
        const profileBtn = document.getElementById('identity-profile-wizard-btn');
        if (profileBtn) {
            profileBtn.hidden = hidden;
            profileBtn.style.display = hidden ? 'none' : '';
        }
        const welcomeOverview = document.querySelector('.info-box-archive-overview');
        if (welcomeOverview) welcomeOverview.hidden = hidden;
    }

    async function applyVisitorFeatureGatingFromSession() {
        const deniedTitle = 'Not available for your visitor key. The owner can grant access under Settings → Manage Keys.';
        try {
            const res = await fetch('/auth/me', { credentials: 'same-origin' });
            if (!res.ok) return;
            const me = await res.json().catch(() => null);
            const isVisitorKeySession = !!(me && me.is_visitor && me.visitor_access && me.visitor_access.restricted);
            applyVisitorKeyOwnerOnlyUI(isVisitorKeySession);
            if (!me || !me.is_visitor) {
                window.__visitorAccess = { restricted: false };
                return;
            }
            const va = me.visitor_access;
            if (!va) {
                window.__visitorAccess = { restricted: false };
                return;
            }
            window.__visitorAccess = {
                restricted: !!va.restricted,
                can_messages_chat: !!va.can_messages_chat,
                can_emails: !!va.can_emails,
                can_contacts: !!va.can_contacts,
                can_relationships: !!va.can_relationships,
                can_sensitive_private: !!va.can_sensitive_private,
            };
            if (!va.restricted) return;

            function gate(id, allowed, gateKey) {
                const el = document.getElementById(id);
                if (!el) return;
                el.disabled = !allowed;
                if (allowed) {
                    el.title = '';
                    el.removeAttribute('data-visitor-gate');
                } else {
                    el.title = deniedTitle;
                    el.setAttribute('data-visitor-gate', gateKey);
                }
            }
            gate('sms-messages-sidebar-btn', !!va.can_messages_chat, 'messages_chat');
            gate('email-gallery-sidebar-btn', !!va.can_emails, 'emails');
            gate('contacts-sidebar-btn', !!(va.can_contacts || va.can_relationships), 'contacts');
            gate('sensitive-data-sidebar-btn', !!va.can_sensitive_private, 'sensitive_private');
            gate('profiles-sidebar-btn', !!va.can_relationships, 'relationship');
        } catch (e) {
            console.warn('Visitor feature gating: /auth/me failed', e);
        }
    }

    async function init() {
        if (window.electronAPI && typeof window.electronAPI.getAppInfo === 'function') {
            window.electronAPI.getAppInfo().then(info => {
                const el = document.getElementById('electron-titlebar-text');
                if (!el || !info) return;
                const name = String(info.name || 'Digital Museum').replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
                const version = String(info.version || '').trim();
                el.textContent = version ? `${name} · v${version}` : name;
            }).catch(() => {});
        }

        // Info box modal: set up close first (before other inits that might throw)
        window.closeInfoBoxModal = function(options = {}) {
            const modal = document.getElementById('info-box-modal');
            const dontShow = document.getElementById('welcome-dont-show-again-checkbox');
            if (dontShow) {
                setOnboardingDontShowAgain(!!dontShow.checked);
            }
            if (modal) {
                modal.classList.add('info-box-modal-closed');
                modal.style.display = '';
                if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(true);
            }
            getLLMProviderAvailabilityPair().then((av) => {
                const anyHostedOk = (av._order || []).some((key) => av[key]);
                if (!anyHostedOk && !av.localai && typeof Modals !== 'undefined' && Modals.ConfirmationModal) {
                    Modals.ConfirmationModal.open(
                        'No AI Provider Available',
                        'No LLM provider is available. AI functions will not be available until an OpenRouter API key or a Local AI endpoint is set in the server environment (for example in the .env file).',
                        undefined
                    );
                }
            }).catch(() => {});
            if (!options || !options.skipMasterPrompt) {
                maybePromptMasterKeyUnlock();
            }
        };
        const infoBoxModal = document.getElementById('info-box-modal');
        if (infoBoxModal) {
            infoBoxModal.addEventListener('click', (e) => {
                if (e.target === infoBoxModal) window.closeInfoBoxModal();
            });
            document.getElementById('info-box-close-btn')?.addEventListener('click', window.closeInfoBoxModal);
            if (shouldShowOnboardingWelcome()) {
                infoBoxModal.classList.remove('info-box-modal-closed');
                if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(false);
            } else {
                infoBoxModal.classList.add('info-box-modal-closed');
                if (typeof UI !== 'undefined' && UI.setControlsEnabled) UI.setControlsEnabled(true);
            }
        }

        installVisitorFeatureGateCapture();
        Config.init(); // Loads and applies settings, sets up its listeners
        await Regions.ensureLoaded();
        if (DOM.imageGalleryLocationFilter) {
            Regions.populateSelect(DOM.imageGalleryLocationFilter, { allLabel: 'All regions' });
        }
        initChatStarfield();
        Chat.renderExistingMessages();
        VoiceSelector.init(); // Sets initial voice state, creativity lock, listeners
        Modals.initAll();
        if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.init) {
            void ChatInactivityNudge.init();
        }
        if (typeof Modals.ConversationManager !== 'undefined' && Modals.ConversationManager.ensureChatConversationContext) {
            await Modals.ConversationManager.ensureChatConversationContext();
        }

        //SSE.init();
        InterviewerMode.init();
        if (typeof ChatVoiceInput !== 'undefined' && ChatVoiceInput.init) ChatVoiceInput.init();
        try {
            initEventListeners(); // Attach main app event listeners
        } catch (e) {
            console.error('initEventListeners failed (some UI may be broken):', e);
        }
        if (DOM.llmProviderSelect) {
            DOM.llmProviderSelect.addEventListener('change', () => {
                const val = DOM.llmProviderSelect.value;
                if (val === 'gemini' || val === 'claude' || val === 'deepseek' || val === 'openai') {
                    saveLastManualHostedProviderIfHosted(val);
                    DOM.llmProviderSelect.dataset.userSelectedProvider = 'true';
                } else if (val === 'auto' || val === 'localai') {
                    DOM.llmProviderSelect.dataset.userSelectedProvider = 'true';
                }
            });
        }
        void applyVisitorFeatureGatingFromSession();
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
        void (async () => {
            if (typeof LocalAiSetup !== 'undefined' && LocalAiSetup.init) {
                await LocalAiSetup.init();
            }
            await loadLLMProviderAvailability();
        })();
        if (Modals.AutoRoutingConfig && Modals.AutoRoutingConfig.ensureLoaded) {
            void Modals.AutoRoutingConfig.ensureLoaded();
        }
        if (typeof UI !== 'undefined' && UI.syncChatContextStatusBarVisibility) {
            UI.syncChatContextStatusBarVisibility();
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
                markOnboardingChecklistStepDone('unlock_sensitive');
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
    return {
        init,
        processFormSubmit,
        processQuestionSubmit,
        processAnswerSubmit,
        refreshChatAvailability: loadLLMProviderAvailability,
        effectiveLocalAIAvailable,
        isLocalAIUseEnabledForChat,
        resolveLocalAIUseEnabled,
        buildProviderAvailabilityMap,
        abortCurrentChatRequest,
        runChatWithProviderFailover,
        isChatRequestInFlight,
    };
})();

void App.init();
