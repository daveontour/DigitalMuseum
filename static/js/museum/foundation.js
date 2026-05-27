'use strict';


// --- Polyfills or Global Helpers ---
function generateUUID() { // Public Domain/MIT
    var d = new Date().getTime();
    var d2 = ((typeof performance !== 'undefined') && performance.now && (performance.now() * 1000)) || 0;
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
        var r = Math.random() * 16;
        if (d > 0) {
            r = (d + r) % 16 | 0;
            d = Math.floor(d / 16);
        } else {
            r = (d2 + r) % 16 | 0;
            d2 = Math.floor(d2 / 16);
        }
        return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16);
    });
}

// --- Constants ---
const CONSTANTS = {
    VOICE_IMAGES: {
        'expert': 'expert.png', 'male_friend': 'male-friend.png', 'female_friend': 'female-friend.png',
        'psychologist': 'psychologist.png', 'after_death': 'after-death.png', 'secret_admirer': '{{admirer_image}}',
        'bar_girl': 'bar-girl.png', 'parents': 'parents.png', 'preacher': 'preacher.png',
        'owner': '{{owner_image}}', 'irish': 'irish.png', 'haiku': 'haiku.png',
        'insult': 'insult.png', 'earthchild': 'earthchild.png',
        'expert_sm': 'expert_sm.png', 'male_friend_sm': 'male-friend_sm.png', 'female_friend_sm': 'female-friend_sm.png',
        'psychologist_sm': 'psychologist_sm.png', 'after_death_sm': 'after-death_sm.png', 'secret_admirer_sm': '{{admirer_image_small}}',
        'bar_girl_sm': 'bar-girl_sm.png', 'parents_sm': 'parents_sm.png', 'preacher_sm': 'preacher_sm.png',
        'owner_sm': '{{owner_image_small}}', 'irish_sm': 'irish_sm.png', 'haiku_sm': 'haiku_sm.png',
        'insult_sm': 'insult_sm.png', 'earthchild_sm': 'earthchild_sm.png',
    },
    // FUNCTION_NAMES are no longer used, but kept for backwards compatibility. USe the function names in the suggestions.json file instead to avoid confusion.
    // FUNCTION_NAMES: Object.freeze({
    //     FirstFunction: "testFunction",
    //     SecondFunction: "showFBMessengerOptions",
    //     ThirdFunction: "showFBAlbumsOptions",
    //     FourthFunction: "openGeoModal",
    //     FifthFunction: "showLocationInfo",
    //     SixthFunction: "showImageGallery",
    //     SeventhFunction: "testEmail",
    //     EighthFunction: "showEmailGallery"
    // }),
    VOICE_DESCRIPTIONS: {
        'expert': 'a knowledgeable expert who provides accurate, factual information',
        'psychologist': 'a compassionate therapist offering psychological insights',
        'after_death': 'an uptight British professor',
        'secret_admirer': 'a romantic soul expressing deep affection who is very shy and reserved and remained annonymous',
        'bar_girl': 'a friendly Thai bar girl called Lucky offering conversation and advice',
        'parents': 'caring parental figures providing guidance and support',
        'preacher': 'a spiritual advisor who is extremely pious and judgemental',
        'owner': '{{owner}} in {{his}} own voice',
        'irish': 'a cheerful Irish comedian who is very funny and has a great sense of humour and responds in limmericks',
        'haiku': 'a poetic soul who responds in haiku form',
        'insult': 'a comedic roaster delivering playful burns',
        'earthchild': 'a free spirit sharing natural wisdom',
        'female_friend': 'a supportive female friend offering caring advice',
        'male_friend': 'a supportive male friend offering caring advice'
    },
    API_PATHS: {
        CHAT: '/chat/generate',
        RANDOM_QUESTION: '/chat/generate-random-question',
        NEW_CHAT: '/new',
        SUGGESTIONS_JSON: '/api/suggestions',
        REGIONS: '/api/regions',
        FB_CHATTERS: '/facebook-chatters/one_to_one',
        CONTACTS: '/getContacts',
        MESSAGES_BY_CONTACT: '/getMessagesByContact',
        MESSAGES_BY_CONTACT_V2: '/getConversationsByParticipant?name=',
        ALBUMS: '/getAlbums',
        EVENTS: '/events',
        HAVE_YOUR_SAY: '/HaveYourSay',
        WHAT_ARE_PEOPLE_SAYING: '/WhatArePeopleSaying',
        HAVE_A_CHAT_TURN: '/chat/have-a-chat/turn',
        HAVE_A_CHAT_SESSIONS: '/api/have-a-chat/sessions',
        INTERVIEW_START:  '/interview/start',
        INTERVIEW_TURN:   '/interview/turn',
        INTERVIEW_PAUSE:  '/interview/pause',
        INTERVIEW_RESUME: '/interview/resume',
        INTERVIEW_END:    '/interview/end',
        INTERVIEW_LIST:   '/interview/list',
        INTERVIEW_DETAIL: '/interview/'
    },
    LOCAL_STORAGE_KEYS: {
        CHAT_SETTINGS: 'chatSettings'
    },
    LLM_PROVIDERS: {
        GEMINI: "{{gemini_configured}}",
        CLAUDE: "{{claude_configured}}",
        DEEPSEEK: "{{deepseek_configured}}",
        LOCALAI: "{{localai_configured}}"
    },
    /** True when DEPLOYMENT_NATURE=local — path-based import tiles are shown; otherwise they are hidden. */
    DEPLOYMENT_NATURE_LOCAL: "{{deployment_nature_local}}",
    /** True when GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP=true — account menu can reload guide topics from seed JSON. */
    GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP: "{{guide_topics_reload_from_file_on_startup}}",
    OWNER_NAME: "{{owner}}",
    OWNER_GENDER: "{{owner_gender}}",
    RANDOM_QUESTION_PROMPT: "Generate a random question about {{owner}}'s life. It could be about any aspect of his biography, people he's known, travels, work, hobbies, relationships, psychology, interest, anything. The objective is that by answering the question it would provide insight into him or reveal hidden or understated aspects of him or amusing facts. Do not answer the question, just generate it.",
    TODAYS_THING_PROMPT: "{{todays_thing_prompt}}",
};

const Regions = (() => {
    let config = typeof window !== 'undefined' ? (window.DM_REGION_CONFIG || null) : null;
    let labelMap = null;
    let loadPromise = null;

    function rebuildLabelMap() {
        labelMap = {};
        if (!config || !Array.isArray(config.regions)) {
            return;
        }
        config.regions.forEach((row) => {
            if (!row || row.code == null) return;
            const code = String(row.code).trim();
            if (!code) return;
            labelMap[code.toLowerCase()] = row.label != null ? String(row.label) : code;
        });
        if (config.default_region && config.default_label && !labelMap[String(config.default_region).toLowerCase()]) {
            labelMap[String(config.default_region).toLowerCase()] = String(config.default_label);
        }
    }

    function setConfig(next) {
        config = next || null;
        labelMap = null;
    }

    function label(code) {
        if (code == null || String(code).trim() === '') {
            return 'Unknown';
        }
        if (!labelMap) {
            rebuildLabelMap();
        }
        const k = String(code).toLowerCase().trim();
        if (labelMap && labelMap[k]) {
            return labelMap[k];
        }
        if (config && config.default_region && k === String(config.default_region).toLowerCase() && config.default_label) {
            return String(config.default_label);
        }
        return String(code);
    }

    function codesInOrder() {
        if (!config || !Array.isArray(config.regions)) {
            return [];
        }
        return config.regions
            .map((row) => (row && row.code != null ? String(row.code).trim() : ''))
            .filter(Boolean);
    }

    async function ensureLoaded() {
        if (config && Array.isArray(config.regions) && config.regions.length > 0) {
            return config;
        }
        if (loadPromise) {
            return loadPromise;
        }
        loadPromise = fetch(CONSTANTS.API_PATHS.REGIONS, { cache: 'no-store', credentials: 'same-origin' })
            .then((r) => {
                if (!r.ok) {
                    throw new Error(`Failed to fetch regions: ${r.status}`);
                }
                return r.json();
            })
            .then((data) => {
                setConfig(data);
                return config;
            })
            .catch((err) => {
                console.warn('Regions config load failed:', err);
                return config;
            })
            .finally(() => {
                loadPromise = null;
            });
        return loadPromise;
    }

    /**
     * Populate a select with region options. Preserves leading non-region options already in the select.
     * @param {HTMLSelectElement} selectEl
     * @param {{ rows?: Array<{region:string,count?:number}>, allLabel?: string }} opts
     */
    function populateSelect(selectEl, opts = {}) {
        if (!selectEl) return;
        const rows = opts.rows || codesInOrder().map((code) => ({ region: code }));
        const preserve = [];
        for (const opt of selectEl.options) {
            const v = opt.value;
            if (v === 'all' || v === 'loc' || v === 'non' || v === '') {
                preserve.push({ value: v, text: opt.textContent });
            } else {
                break;
            }
        }
        const prev = selectEl.value;
        selectEl.innerHTML = '';
        if (preserve.length === 0) {
            const allOpt = document.createElement('option');
            allOpt.value = '';
            allOpt.textContent = opts.allLabel || 'All regions';
            selectEl.appendChild(allOpt);
        } else {
            preserve.forEach(({ value, text }) => {
                const o = document.createElement('option');
                o.value = value;
                o.textContent = text;
                selectEl.appendChild(o);
            });
        }
        rows.forEach((row) => {
            const code = row && row.region != null ? String(row.region).trim() : '';
            if (!code) return;
            const o = document.createElement('option');
            o.value = code;
            const count = row.count != null ? Number(row.count) : 0;
            o.textContent = count > 0
                ? `${label(code)} (${count.toLocaleString()})`
                : label(code);
            selectEl.appendChild(o);
        });
        if (prev && Array.from(selectEl.options).some((o) => o.value === prev)) {
            selectEl.value = prev;
        }
    }

    rebuildLabelMap();

    return { label, codesInOrder, ensureLoaded, populateSelect, getConfig: () => config, setConfig };
})();

// --- DOM Element Cache ---
const DOM = {
    chatBox: document.getElementById('chat-box'),
    chatForm: document.getElementById('chat-form'),
    chatContextStatusBar: document.getElementById('chat-context-status-bar'),
    userInput: document.getElementById('user-input'),
    chatVoiceInputBtn: document.getElementById('chat-voice-input-btn'),
    sendButton: document.getElementById('send-button'),
    suggestionsBtn: document.getElementById('suggestions-btn'),
    loadingIndicator: document.getElementById('loading-indicator'),
    loadingIndicatorProvider: document.getElementById('loading-indicator-provider'),
    loadingIndicatorCancelBtn: document.getElementById('loading-indicator-cancel-btn'),
    errorDisplay: document.getElementById('error-display'),
    infoBox: document.getElementById('info-box'),
    infoBoxModal: document.getElementById('info-box-modal'),
    infoBoxCloseBtn: document.getElementById('info-box-close-btn'),
    hamburgerMenu: document.getElementById('hamburger-menu'),
    configPage: document.getElementById('config-modal-overlay'),
    closeConfigBtn: document.getElementById('close-config'),
    voiceSettingsModal: document.getElementById('voice-settings-modal'),
    voiceSettingsTrigger: document.getElementById('voice-settings-trigger'),
    closeVoiceSettingsBtn: document.getElementById('close-voice-settings'),
    clearVoiceConversationHistoryBtn: document.getElementById('clear-voice-conversation-history-btn'),
    chatMain: document.querySelector('.chat-main'),
    messageFontSize: document.getElementById('message-font-size'),
    creativityLevel: document.getElementById('creativity-level'),
    showAudioTags: document.getElementById('show-audio-tags'),
    showImageTags: document.getElementById('show-image-tags'),
    showJsonTags: document.getElementById('show-json-tags'),
    autoVoiceShortResponses: document.getElementById('auto-voice-short-responses'),
    autoOpenSuggestions: document.getElementById('auto-open-suggestions'),
    companionModeCheckbox: document.getElementById('companion-mode'),
    allowExplicitContentCheckbox: document.getElementById('allow-explicit-content'),
    enableSnarkinessCheckbox: document.getElementById('enable-snarkiness'),
    llmProviderSelect: document.getElementById('llm-provider-select'),
    profilesLlmProviderSelect: document.getElementById('profiles-llm-provider-select'),
    voiceRadios: document.querySelectorAll('input[name="voice"]'),
    moodSelector: document.getElementById('mood-selector'),
    ownerMood: document.getElementById('owner-mood'),
    voicePreviewImg: document.querySelector('.preview-image'),
    voicePreviewDesc: document.querySelector('.preview-description'),
    loadingVoiceImage: document.querySelector('#loading-indicator .loading-voice-image'),
    voiceIcons: document.querySelectorAll('.voice-icon'),
    voiceIconWrappers: document.querySelectorAll('.voice-icon-wrapper'),
    // New voice select dropdown
    voiceSelect: document.getElementById('voice-select'),
    selectedVoiceImage: document.getElementById('selected-voice-image'),
    // Modals & Modal Elements
    suggestionsModal: document.getElementById('tile-suggestions-modal'),
    closeSuggestionsModalBtn: document.getElementById('modal-suggestions-close-btn'),
    suggestionsListContainer: document.getElementById('tile-suggestions-container'),
    suggestionsShowMatureCheckbox: document.getElementById('suggestions-show-mature-checkbox'),
    fbAlbumsModal: document.getElementById('fb-albums-modal'),
    fbAlbumsList: document.getElementById('fb-albums-list'),
    fbAlbumsSearch: document.getElementById('fb-albums-search'),
    fbAlbumsMasterPane: document.getElementById('fb-albums-master-pane'),
    fbAlbumsSlavePane: document.getElementById('fb-albums-slave-pane'),
    fbAlbumsResizeHandle: document.getElementById('fb-albums-resize-handle'),
    fbAlbumsAlbumTitle: document.getElementById('fb-albums-album-title'),
    fbAlbumsAlbumDescription: document.getElementById('fb-albums-album-description'),
    fbAlbumsImagesContainer: document.getElementById('fb-albums-images-container'),
    closeFBAlbumsModalBtn: document.getElementById('close-fb-albums-modal'),
    fbPostsModal: document.getElementById('fb-posts-modal'),
    fbPostsList: document.getElementById('fb-posts-list'),
    fbPostsSearch: document.getElementById('fb-posts-search'),
    fbPostsClearBtn: document.getElementById('fb-posts-clear-btn'),
    fbPostsMasterPane: document.getElementById('fb-posts-master-pane'),
    fbPostsSlavePane: document.getElementById('fb-posts-slave-pane'),
    fbPostsResizeHandle: document.getElementById('fb-posts-resize-handle'),
    fbPostsPostDate: document.getElementById('fb-posts-post-date'),
    fbPostsPostText: document.getElementById('fb-posts-post-text'),
    fbPostsPostLink: document.getElementById('fb-posts-post-link'),
    fbPostsImagesContainer: document.getElementById('fb-posts-images-container'),
    closeFBPostsModalBtn: document.getElementById('close-fb-posts-modal'),
    fbImageModal: document.getElementById('fb-album-modal'),
    fbImageModalTitle: document.getElementById('fb-album-modal-title'),
    fbImageModalDescription: document.getElementById('fb-album-modal-description'),
    fbImageContainer: document.getElementById('fb-album-container'),
    fbImageModalCloseBtn: document.getElementById('close-fb-album-modal'),
    haveYourSayBtn: document.getElementById('have-your-say-btn'),
    haveYourSayModal: document.getElementById('have-your-say-modal'),
    closeHaveYourSayModalBtn: document.getElementById('close-have-your-say-modal'),
    startDictationBtn: document.getElementById('start-dictation-btn'),
    stopDictationBtn: document.getElementById('stop-dictation-btn'),
    submitHaveYourSayBtn: document.getElementById('submit-have-your-say-btn'),
    cancelHaveYourSayBtn: document.getElementById('cancel-have-your-say-btn'),
    haveYourSayTextarea: document.getElementById('have-your-say-textarea'),
    dictationStatus: document.getElementById('dictation-status'),
    haveYourSayName: document.getElementById('have-your-say-name'),
    haveYourSayRelationship: document.getElementById('have-your-say-relationship'),
    // Reference Documents (manage modal opened from Data Import tile)
    contactsModal: document.getElementById('contacts-modal'),
    closeContactsModalBtn: document.getElementById('close-contacts-modal'),
    extractContactsBtn: document.getElementById('extract-contacts-btn'),
    contactsLoading: document.getElementById('contacts-loading'),
    contactsTableContainer: document.getElementById('contacts-table-container'),
    contactsTableBody: document.getElementById('contacts-table-body'),
    contactsHasMessagesOnly: document.getElementById('contacts-has-messages-only'),
    contactsEmailContainsAt: document.getElementById('contacts-email-contains-at'),
    contactsExcludePhoneNumbers: document.getElementById('contacts-exclude-phone-numbers'),
    contactsSearch: document.getElementById('contacts-search'),
    contactsSelectAll: document.getElementById('contacts-select-all'),
    contactsDeleteSelectedBtn: document.getElementById('contacts-delete-selected-btn'),
    contactsSelectedCount: document.getElementById('contacts-selected-count'),
    referenceDocumentsList: document.getElementById('reference-documents-list'),
    referenceDocumentsSearch: document.getElementById('reference-documents-search'),
    referenceDocumentsCategoryFilter: document.getElementById('reference-documents-category-filter'),
    referenceDocumentsContentTypeFilter: document.getElementById('reference-documents-content-type-filter'),
    referenceDocumentsTaskFilter: document.getElementById('reference-documents-task-filter'),
    referenceDocumentsUploadBtn: document.getElementById('reference-documents-upload-btn'),
    referenceDocumentsUploadModal: document.getElementById('reference-documents-upload-modal'),
    closeReferenceDocumentsUploadModalBtn: document.getElementById('close-reference-documents-upload-modal'),
    referenceDocumentsUploadForm: document.getElementById('reference-documents-upload-form'),
    referenceDocumentsUploadCancelBtn: document.getElementById('reference-documents-upload-cancel'),
    referenceDocumentsEditModal: document.getElementById('reference-documents-edit-modal'),
    closeReferenceDocumentsEditModalBtn: document.getElementById('close-reference-documents-edit-modal'),
    referenceDocumentsEditForm: document.getElementById('reference-documents-edit-form'),
    referenceDocumentsEditCancelBtn: document.getElementById('reference-documents-edit-cancel'),
    referenceDocumentsNotificationModal: document.getElementById('reference-documents-notification-modal'),
    closeReferenceDocumentsNotificationModalBtn: document.getElementById('close-reference-documents-notification-modal'),
    referenceDocumentsNotificationList: document.getElementById('reference-documents-notification-list'),
    referenceDocumentsNotificationCancelBtn: document.getElementById('reference-documents-notification-cancel'),
    referenceDocumentsNotificationProceedBtn: document.getElementById('reference-documents-notification-proceed'),
    // Conversation management elements
    conversationListModal: document.getElementById('conversation-list-modal'),
    closeConversationListModalBtn: document.getElementById('close-conversation-list-modal'),
    conversationListContainer: document.getElementById('conversation-list-container'),
    newConversationBtn: document.getElementById('new-conversation-btn'),
    newConversationModal: document.getElementById('new-conversation-modal'),
    // Subject Configuration elements (now in config tab)
    subjectArchiveOwnerName: document.getElementById('subject-archive-owner-name'),
    subjectGenderSelect: document.getElementById('subject-gender-select'),
    otherNamesInput: document.getElementById('other-names-input'),
    emailAddressesInput: document.getElementById('email-addresses-input'),
    subjectOwnerContactSelect: document.getElementById('subject-owner-contact-select'),
    subjectOwnerContactShowAll: document.getElementById('subject-owner-contact-show-all'),
    requestWritingStyleBtn: document.getElementById('request-writing-style-btn'),
    writingStyleRequireContactHint: document.getElementById('writing-style-require-contact-hint'),
    writingStyleLoading: document.getElementById('writing-style-loading'),
    writingStyleDisplay: document.getElementById('writing-style-display'),
    writingStyleEditTextarea: document.getElementById('writing-style-edit-textarea'),
    writingStyleSaveStatus: document.getElementById('writing-style-save-status'),
    saveWritingStyleAiBtn: document.getElementById('save-writing-style-ai-btn'),
    requestPsychologicalProfileBtn: document.getElementById('request-psychological-profile-btn'),
    psychologicalProfileRequireContactHint: document.getElementById('psychological-profile-require-contact-hint'),
    psychologicalProfileLoading: document.getElementById('psychological-profile-loading'),
    psychologicalProfileDisplay: document.getElementById('psychological-profile-display'),
    psychologicalProfileEditTextarea: document.getElementById('psychological-profile-edit-textarea'),
    psychologicalProfileSaveStatus: document.getElementById('psychological-profile-save-status'),
    savePsychologicalProfileAiBtn: document.getElementById('save-psychological-profile-ai-btn'),
    interestsSuggestAiBtnSc: document.getElementById('interests-suggest-ai-btn-sc'),
    interestsSuggestAiRequireContactHint: document.getElementById('interests-suggest-ai-require-contact-hint'),
    saveSubjectConfigBtn: document.getElementById('save-subject-config-btn'),
    cancelSubjectConfigBtn: document.getElementById('cancel-subject-config-btn'),
    closeNewConversationModalBtn: document.getElementById('close-new-conversation-modal'),
    newConversationTitleInput: document.getElementById('new-conversation-title-input'),
    newConversationVoiceSelect: document.getElementById('new-conversation-voice-select'),
    createConversationBtn: document.getElementById('create-conversation-btn'),
    // Chat dictation elements
    chatDictationStatus: document.getElementById('chat-dictation-status'),
    geoMetadataModal: document.getElementById('geo-metadata-modal'),
    geoList: document.getElementById('geo-metadata-list'),
    geoImage: document.getElementById('geo-metadata-image'),
    geoMapFixedBtn: document.getElementById('geo-map-fixed-btn'),
    closeGeoMetadataModalBtn: document.getElementById('close-geo-metadata-modal'),
    geoMetadataRegionSelect: document.getElementById('geo-metadata-region-select'),
    shufflePhotosBtn: document.getElementById('shuffle-photos-btn'),
    nearbyLocationsModal: document.getElementById('nearby-locations-modal'),
    nearbyMapView: document.getElementById('nearby-map-view'),
    nearbyLocationsCount: document.getElementById('nearby-locations-count'),
    nearbyRadiusInput: document.getElementById('nearby-radius-input'),
    nearbyRadiusUpdateBtn: document.getElementById('nearby-radius-update-btn'),
    nearbyLocationsLimitNotice: document.getElementById('nearby-locations-limit-notice'),
    closeNearbyLocationsModalBtn: document.getElementById('close-nearby-locations-modal'),
    leafletmap: document.getElementById('map'),
    tabButtons: document.querySelectorAll('.geo-tab-btn'),
    tabContents: document.querySelectorAll('.geo-metadata-tab-content'),
    geoMetaDataIframe: document.getElementById('geo-metadata-image'),
    geoMetaDataInstructions: document.getElementById('geo-metadata-instructions'),

    // Confirmation Modal
    confirmationModal: document.getElementById('confirmation-modal'),
    confirmationModalTitle: document.getElementById('confirmation-modal-title'),
    confirmationModalText: document.getElementById('confirmation-modal-text'),
    confirmationModalConfirmBtn: document.getElementById('confirmation-modal-confirm'),
    confirmationModalCancelBtn: document.getElementById('confirmation-modal-cancel'),
    confirmationModalCloseBtn: document.getElementById('close-confirmation-modal'),
    yearFilter: document.getElementById('year-filter'),
    monthFilter: document.getElementById('month-filter'),
    imageDetails: document.getElementById('image-details'),
    imageGalleryModalContent: document.getElementById('image-gallery-modal-content'),
    imageGalleryModal: document.getElementById('image-gallery-modal'),
    closeImageGalleryModalBtn: document.getElementById('close-image-gallery-modal'),
    imageGalleryModalTitle: document.getElementById('image-gallery-modal-title'),
    imageGalleryList: document.getElementById('image-gallery-list'),
    imageGalleryImage: document.getElementById('image-gallery-image'),
    imageGalleryMap: document.getElementById('image-gallery-map'),
    imageGalleryInstructions: document.getElementById('image-gallery-instructions'),
    imageGalleryFixedBtn: document.getElementById('image-gallery-fixed-btn'),
    imageGalleryYearFilter: document.getElementById('image-gallery-year-filter'),
    imageGalleryMonthFilter: document.getElementById('image-gallery-month-filter'),
    imageGallerySourceFilter: document.getElementById('image-gallery-source-filter'),
    imageGalleryLocationFilter: document.getElementById('image-gallery-location-filter'),
    imageGallerySearch: document.getElementById('image-gallery-search'),
    imageGalleryImageDetails: document.getElementById('image-gallery-image-details'),
    imageGalleryAudioContainer: document.getElementById('image-gallery-audio-container'),
    imageGalleryAudioPlayer: document.getElementById('image-gallery-audio-player'),
    imageGalleryVideoContainer: document.getElementById('image-gallery-video-container'),
    imageGalleryVideoPlayer: document.getElementById('image-gallery-video-player'),
    imageGalleryPdfContainer: document.getElementById('image-gallery-pdf-container'),
    imageGalleryPdfViewer: document.getElementById('image-gallery-pdf-viewer'),
    imageGallerySearchBtn: document.getElementById('image-gallery-search-btn'),
    imageGalleryClearBtn: document.getElementById('image-gallery-clear-btn'),
    downloadImageGalleryBtn: document.getElementById('download-image-gallery-btn'),
    // Email Gallery Modal Elements
    emailGalleryModal: document.getElementById('email-gallery-modal'),
    closeEmailGalleryModalBtn: document.getElementById('close-email-gallery-modal'),
    emailGalleryList: document.getElementById('email-gallery-list'),
    emailGalleryMasterPane: document.querySelector('.email-gallery-master-pane'),
    emailGalleryDivider: document.getElementById('email-gallery-divider'),
    emailGalleryDetailPane: document.querySelector('.email-gallery-detail-pane'),
    emailGalleryInstructions: document.getElementById('email-gallery-instructions'),
    emailGalleryEmailContent: document.getElementById('email-gallery-email-content'),
    emailGalleryIframe: document.getElementById('email-gallery-iframe'),
    emailGalleryAttachmentsSection: document.getElementById('email-gallery-attachments-section'),
    emailGalleryAttachmentsGrid: document.getElementById('email-gallery-attachments-grid'),
    emailGallerySearch: document.getElementById('email-gallery-search'),
    emailGallerySender: document.getElementById('email-gallery-sender'),
    emailGalleryRecipient: document.getElementById('email-gallery-recipient'),
    emailGalleryToFrom: document.getElementById('email-gallery-to-from'),
    emailGalleryYearFilter: document.getElementById('email-gallery-year-filter'),
    emailGalleryMonthFilter: document.getElementById('email-gallery-month-filter'),
    emailGalleryAttachmentsFilter: document.getElementById('email-gallery-attachments-filter'),
    emailGallerySourceFilter: document.getElementById('email-gallery-source-filter'),
    emailGalleryFolderFilter: document.getElementById('email-gallery-folder-filter'),
    emailGallerySearchBtn: document.getElementById('email-gallery-search-btn'),
    emailGalleryClearBtn: document.getElementById('email-gallery-clear-btn'),
    emailGallerySort: document.getElementById('email-gallery-sort'),
    emailGalleryMetadataSubject: document.getElementById('email-gallery-metadata-subject'),
    emailGalleryMetadataFrom: document.getElementById('email-gallery-metadata-from'),
    emailGalleryMetadataTo: document.getElementById('email-gallery-metadata-to'),
    emailGalleryMetadataDate: document.getElementById('email-gallery-metadata-date'),
    emailGalleryFolderCrumb: document.getElementById('email-gallery-folder-crumb'),
    emailGalleryDetailAvatarSm: document.getElementById('email-gallery-detail-avatar-sm'),
    emailGalleryEmailDetails: null, // Removed from HTML, kept for compatibility
    emailAskAIBtn: document.getElementById('email-ask-ai-btn'),
    emailDeleteBtn: document.getElementById('email-delete-btn'),
    emailAskAIModal: document.getElementById('email-ask-ai-modal'),
    // Attachment Modals
    emailAttachmentImageModal: document.getElementById('email-attachment-image-modal'),
    emailAttachmentDocumentModal: document.getElementById('email-attachment-document-modal'),
    closeEmailAttachmentImageModal: document.getElementById('close-email-attachment-image-modal'),
    closeEmailAttachmentDocumentModal: document.getElementById('close-email-attachment-document-modal'),
    emailAttachmentImageDisplay: document.getElementById('email-attachment-image-display'),
    emailAttachmentDocumentIframe: document.getElementById('email-attachment-document-iframe'),
    // Email Gallery Button
    emailGalleryBtn: document.getElementById('email-gallery-btn'),
    // Email Editor Modal Elements
    emailEditorModal: document.getElementById('email-editor-modal'),
    closeEmailEditorModalBtn: document.getElementById('close-email-editor-modal'),
    emailEditorSearch: document.getElementById('email-editor-search'),
    emailEditorSender: document.getElementById('email-editor-sender'),
    emailEditorRecipient: document.getElementById('email-editor-recipient'),
    emailEditorToFrom: document.getElementById('email-editor-to-from'),
    emailEditorYearFilter: document.getElementById('email-editor-year-filter'),
    emailEditorMonthFilter: document.getElementById('email-editor-month-filter'),
    emailEditorAttachmentsFilter: document.getElementById('email-editor-attachments-filter'),
    emailEditorSourceFilter: document.getElementById('email-editor-source-filter'),
    emailEditorSearchBtn: document.getElementById('email-editor-search-btn'),
    emailEditorClearBtn: document.getElementById('email-editor-clear-btn'),
    emailEditorTableBody: document.getElementById('email-editor-table-body'),
    emailEditorPagination: document.getElementById('email-editor-pagination'),
    emailEditorBulkDeleteBtn: document.getElementById('email-editor-bulk-delete-btn'),
    emailEditorSelectAllBtn: document.getElementById('email-editor-select-all-btn'),
    emailEditorOpenEmailsGalleryBtn: document.getElementById('email-editor-open-emails-gallery-btn'),
    emailEditorViewer: document.getElementById('email-editor-viewer'),
    emailAttachmentsModal: document.getElementById('email-attachments-modal'),
    closeEmailAttachmentsModalBtn: document.getElementById('close-email-attachments-modal'),
    emailEditorOpenAttachmentsBtn: document.getElementById('email-editor-open-attachments-btn'),
    emailAttachmentsOpenEmailManagerBtn: document.getElementById('email-attachments-open-email-manager-btn'),
    // New Image Gallery Elements
    newImageGalleryModal: document.getElementById('new-image-gallery-modal'),
    closeNewImageGalleryModalBtn: document.getElementById('close-new-image-gallery-modal'),
    newImageGalleryTitle: document.getElementById('new-image-gallery-title'),
    newImageGalleryDescription: document.getElementById('new-image-gallery-description'),
    newImageGalleryTags: document.getElementById('new-image-gallery-tags'),
    newImageGallerySimilarTags: document.getElementById('new-image-gallery-similar-tags'),
    newImageGallerySimilarBtn: document.getElementById('new-image-gallery-similar-btn'),
    newImageGallerySimilarN: document.getElementById('new-image-gallery-similar-n'),
    newImageGallerySimilarHint: document.getElementById('new-image-gallery-similar-hint'),
    newImageGalleryAuthor: document.getElementById('new-image-gallery-author'),
    newImageGallerySource: document.getElementById('new-image-gallery-source'),
    newImageGalleryRegionFilter: document.getElementById('new-image-gallery-region-filter'),
    newImageGalleryYearFilter: document.getElementById('new-image-gallery-year-filter'),
    newImageGalleryMonthFilter: document.getElementById('new-image-gallery-month-filter'),
    newImageGalleryRating: document.getElementById('new-image-gallery-rating'),
    newImageGalleryRatingMin: document.getElementById('new-image-gallery-rating-min'),
    newImageGalleryRatingMax: document.getElementById('new-image-gallery-rating-max'),
    newImageGalleryHasGps: document.getElementById('new-image-gallery-has-gps'),
    newImageGalleryHasThumbnail: document.getElementById('new-image-gallery-has-thumbnail'),
    newImageGallerySearchBtn: document.getElementById('new-image-gallery-search-btn'),
    newImageGalleryClearBtn: document.getElementById('new-image-gallery-clear-btn'),
    newImageGalleryThumbnailGrid: document.getElementById('new-image-gallery-thumbnail-grid'),
    newImageGalleryMasterPane: document.querySelector('.new-image-gallery-master-pane'),
    newImageGallerySelectedCount: document.getElementById('new-image-gallery-selected-count'),
    newImageGalleryBulkTags: document.getElementById('new-image-gallery-bulk-tags'),
    newImageGalleryApplyTagsBtn: document.getElementById('new-image-gallery-apply-tags-btn'),
    newImageGalleryAIClassificationBtn: document.getElementById('new-image-gallery-ai-classification-btn'),
    newImageGalleryDeleteSelectedBtn: document.getElementById('new-image-gallery-delete-selected-btn'),
    newImageGalleryClearSelectionBtn: document.getElementById('new-image-gallery-clear-selection-btn'),
    // New Image Gallery Detail Modal Elements
    newImageGalleryDetailModal: document.getElementById('new-image-gallery-detail-modal'),
    closeNewImageGalleryDetailModalBtn: document.getElementById('close-new-image-gallery-detail-modal'),
    newImageGalleryDetailImage: document.getElementById('new-image-gallery-detail-image'),
    newImageDetailTitle: document.getElementById('new-image-detail-title'),
    newImageDetailDescription: document.getElementById('new-image-detail-description'),
    newImageDetailDescriptionEdit: document.getElementById('new-image-detail-description-edit'),
    newImageDetailAuthor: document.getElementById('new-image-detail-author'),
    newImageDetailTags: document.getElementById('new-image-detail-tags'),
    newImageDetailTagsEdit: document.getElementById('new-image-detail-tags-edit'),
    newImageDetailCategories: document.getElementById('new-image-detail-categories'),
    newImageDetailNotes: document.getElementById('new-image-detail-notes'),
    newImageDetailDate: document.getElementById('new-image-detail-date'),
    newImageDetailRating: document.getElementById('new-image-detail-rating'),
    newImageDetailRatingContainer: document.getElementById('new-image-detail-rating-container'),
    newImageDetailRatingEdit: document.getElementById('new-image-detail-rating-edit'),
    newImageDetailFindSimilarBtn: document.getElementById('new-image-detail-find-similar-btn'),
    newImageGallerySaveBtn: document.getElementById('new-image-gallery-save-btn'),
    newImageDetailImageType: document.getElementById('new-image-detail-image-type'),
    newImageDetailSource: document.getElementById('new-image-detail-source'),
    newImageDetailSourceReference: document.getElementById('new-image-detail-source-reference'),
    newImageDetailRegion: document.getElementById('new-image-detail-region'),
    newImageDetailGpsRow: document.getElementById('new-image-detail-gps-row'),
    newImageDetailGps: document.getElementById('new-image-detail-gps'),
    newImageDetailAltitudeRow: document.getElementById('new-image-detail-altitude-row'),
    newImageDetailAltitude: document.getElementById('new-image-detail-altitude'),
    newImageDetailAvailableForTask: document.getElementById('new-image-detail-available-for-task'),
    newImageDetailProcessed: document.getElementById('new-image-detail-processed'),
    newImageDetailLocationProcessed: document.getElementById('new-image-detail-location-processed'),
    newImageDetailImageProcessed: document.getElementById('new-image-detail-image-processed'),
    newImageDetailCreatedAt: document.getElementById('new-image-detail-created-at'),
    newImageDetailUpdatedAt: document.getElementById('new-image-detail-updated-at'),
    newImageGalleryDeleteBtn: document.getElementById('new-image-gallery-delete-btn'),
    // Dropup Elements
    dropupBtn: document.getElementById('dropup-btn'),
    dropupContainer: document.querySelector('.dropup'),
    fbAlbumsDropupBtn: document.getElementById('fb-albums-dropup-btn'),
    imageGalleryDropupBtn: document.getElementById('image-gallery-dropup-btn'),
    locationsDropupBtn: document.getElementById('locations-dropup-btn'),
    emailGalleryDropupBtn: document.getElementById('email-gallery-dropup-btn'),
    suggestionsDropupBtn: document.getElementById('suggestions-dropup-btn'),
    haveYourSayDropupBtn: document.getElementById('have-your-say-dropup-btn'),
    // New sidebar buttons
    fbAlbumsSidebarBtn: document.getElementById('fb-albums-sidebar-btn'),
    fbPostsSidebarBtn: document.getElementById('fb-posts-sidebar-btn'),
    //imageGallerySidebarBtn: document.getElementById('image-gallery-sidebar-btn'),
    locationsSidebarBtn: document.getElementById('locations-sidebar-btn'),
    emailGallerySidebarBtn: document.getElementById('email-gallery-sidebar-btn'),
    newImageGallerySidebarBtn: document.getElementById('new-image-gallery-sidebar-btn'),
    suggestionsSidebarBtn: document.getElementById('suggestions-sidebar-btn'),
    statisticsSidebarBtn: document.getElementById('statistics-sidebar-btn'),
    statisticsModal: document.getElementById('statistics-modal'),
    closeStatisticsModalBtn: document.getElementById('close-statistics-modal'),
    artefactsSidebarBtn: document.getElementById('artefacts-sidebar-btn'),
    identityProfileWizardModal: document.getElementById('identity-profile-wizard-modal'),
    identityProfileWizardBtn: document.getElementById('identity-profile-wizard-btn'),
    wizardParseBtn: document.getElementById('wizard-parse-btn'),
    sensitiveSidebarBtn: document.getElementById('sensitive-data-sidebar-btn'),
    artefactsModal: document.getElementById('artefacts-modal'),
    artefactsSearch: document.getElementById('artefacts-search'),
    artefactsThumbnailGrid: document.getElementById('artefacts-thumbnail-grid'),
    artefactDetailModal: document.getElementById('artefact-detail-modal'),
    artefactDetailName: document.getElementById('artefact-detail-name'),
    artefactDetailDescription: document.getElementById('artefact-detail-description'),
    artefactDetailTags: document.getElementById('artefact-detail-tags'),
    artefactDetailStory: document.getElementById('artefact-detail-story'),
    artefactPhotosStrip: document.getElementById('artefact-photos-strip'),
   // haveYourSaySidebarBtn: document.getElementById('have-your-say-sidebar-btn'),
    // Single Image Modal Elements
    singleImageModal: document.getElementById('single-image-modal'),
    singleImageModalImg: document.getElementById('single-image-modal-img'),
    singleImageModalAudio: document.getElementById('single-image-modal-audio'),
    singleImageModalVideo: document.getElementById('single-image-modal-video'),
    singleImageModalPdf: document.getElementById('single-image-modal-pdf'),
    singleImageDetails: document.getElementById('single-image-details'),
    closeSingleImageModalBtn: document.getElementById('close-single-image-modal'),
};

/** Crux + pointers: all use largest star size (chat-star--4), yellow via CSS .chat-star-crux. */
function appendSouthernCrossToChatStarfield(host) {
    const wrap = document.createElement('div');
    wrap.className = 'chat-starfield-crux';
    wrap.setAttribute('aria-hidden', 'true');

    const stars = [
        { x: 11.2, y: 92.4, a: 1 },
        { x: 24.2, y: 69.7, a: 0.93 },
        { x: 61.7, y: 37.6, a: 1 },
        { x: 46.0, y: 52.6, a: 1 },
        { x: 49.1, y: 23.4, a: 0.93 },
        { x: 58.6, y: 83.2, a: 0.88 },
        { x: 60.4, y: 56.6, a: 0.85 },
    ];

    stars.forEach((o) => {
        const s = document.createElement('span');
        s.className = 'chat-star chat-star-crux chat-star--4';
        s.style.left = `${o.x}%`;
        s.style.top = `${o.y}%`;
        s.style.setProperty('--chat-star-alpha', String(o.a));
        wrap.appendChild(s);
    });

    host.appendChild(wrap);
}

function appendSunTattooToChatStarfield(host) {
    const container = document.createElement('div');
   container.className = 'chat-starfield-sun-tattoo';
    container.setAttribute('aria-hidden', 'true');
    
        // 1. Setup the Canvas and Context
       // const container = document.getElementById('sun-container');
        const canvas = document.createElement('canvas');
        
        canvas.width = 300;
        canvas.height = 300;
        // Ensure the canvas itself remains completely transparent
        canvas.style.backgroundColor = 'transparent';
        container.appendChild(canvas);

        const ctx = canvas.getContext('2d');

        // 2. Configure the "Marker" Style
        // Using a dark charcoal color, rounded caps, and thick lines to mimic the drawing
        ctx.strokeStyle = 'rgba(255, 220, 100, 0.80)';
        ctx.lineWidth = 2;
        ctx.lineCap = 'round';
        ctx.lineJoin = 'round';

        const centerX = 150;
        const centerY = 150;
        const radius = 15;

        // 3. Draw the Central Circle
        // Slight organic variation can be added by making it slightly elliptical, 
        // but a standard arc matches the mathematical layout best for now
        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
        ctx.stroke();

        // 4. Map Out the Rays
        // The distance between the circle and where the line starts
        const gap = 5; 
        
        // Custom array defining the specific lengths based on the reference image,
        // but now with perfectly equal angular spacing (0.2 * Math.PI increments)
        const rays = [
            { angle: Math.PI, length: 10 },       // Left horizontal (Short)
            { angle: Math.PI * 1.2, length: 20 }, // Up-left (Long)
            { angle: Math.PI * 1.4, length: 10 }, // Top-left (Short)
            { angle: Math.PI * 1.6, length: 20 }, // Top-right (Long)
            { angle: Math.PI * 1.8, length: 10 }, // Up-right (Short)
            { angle: 0, length: 20 },             // Right horizontal (Long)
            { angle: Math.PI * 0.2, length: 10 }, // Down-right (Short)
            { angle: Math.PI * 0.4, length: 20 }, // Bottom-right (Long)
            { angle: Math.PI * 0.6, length: 10 }, // Bottom-left (Short)
            { angle: Math.PI * 0.8, length: 20 }  // Down-left (Long)
        ];

        // 5. Draw the Rays
        rays.forEach(ray => {
            // Calculate starting coordinates (circle edge + gap)
            const startX = centerX + Math.cos(ray.angle) * (radius + gap);
            const startY = centerY + Math.sin(ray.angle) * (radius + gap);
            
            // Calculate ending coordinates (starting point + length)
            const endX = centerX + Math.cos(ray.angle) * (radius + gap + ray.length);
            const endY = centerY + Math.sin(ray.angle) * (radius + gap + ray.length);

            // Draw the line
            ctx.beginPath();
            ctx.moveTo(startX, startY);
            ctx.lineTo(endX, endY);
            ctx.stroke();
        });


        host.appendChild(container);
}


/** Random static star positions (four size classes); call once after DOM is ready. */
function initChatStarfield() {
    const host = document.getElementById('chat-starfield');
    if (!host || host.getAttribute('data-initialized') === '1') return;
    host.setAttribute('data-initialized', '1');
    const count = 160;
    const frag = document.createDocumentFragment();
    for (let i = 0; i < count; i++) {
        const s = document.createElement('span');
        const sz = 1 + Math.floor(Math.random() * 4);
        s.className = `chat-star chat-star--${sz}`;
        s.style.left = `${Math.random() * 100}%`;
        s.style.top = `${Math.random() * 100}%`;
        s.style.setProperty('--chat-star-alpha', String(0.4 + Math.random() * 0.6));
        frag.appendChild(s);
    }
    host.appendChild(frag);
    appendSouthernCrossToChatStarfield(host);
    appendSunTattooToChatStarfield(host);
}

// Debug DOM elements

// --- Configure Marked ---
marked.setOptions({
    highlight: function(code, lang) {
        const language = hljs.getLanguage(lang) ? lang : 'plaintext';
        return hljs.highlight(code, { language }).value;
    },
    langPrefix: 'hljs language-', breaks: true, gfm: true
});

// --- State ---
const AppState = {
    clientId: generateUUID(),
    sseEventSource: null,
    dictationRecognition: null,
    isDictationListening: false,
    finalDictationTranscript: '',
    // Chat dictation state
    chatDictationRecognition: null,
    isChatDictationListening: false,
    finalChatDictationTranscript: '',
};

let mapViewInitialized = false;
 // Track the current marker on the main map

// --- UI Helper Module ---
const UI = (() => {
    let chatFailoverNoticeTimer = null;

    function clearChatProviderFailoverNotice() {
        const wrap = document.getElementById('chat-context-failover-wrap');
        const el = document.getElementById('chat-context-failover-msg');
        if (el) el.textContent = '';
        if (wrap) wrap.style.display = 'none';
        if (chatFailoverNoticeTimer) {
            clearTimeout(chatFailoverNoticeTimer);
            chatFailoverNoticeTimer = null;
        }
        syncChatContextStatusBarVisibility();
    }

    function setChatProviderFailoverNotice(fromProvider, toProvider) {
        const wrap = document.getElementById('chat-context-failover-wrap');
        const el = document.getElementById('chat-context-failover-msg');
        if (!el || !wrap) return;
        const providerLabel = (p) => {
            if (p === 'claude') return 'Claude';
            if (p === 'deepseek') return 'DeepSeek';
            if (p === 'localai') return 'Local AI';
            return 'Gemini';
        };
        const fromN = providerLabel(fromProvider);
        const toN = providerLabel(toProvider);
        el.textContent = `Switched to ${toN} after ${fromN} returned an error.`;
        wrap.style.display = 'inline-flex';
        syncChatContextStatusBarVisibility();
        if (chatFailoverNoticeTimer) clearTimeout(chatFailoverNoticeTimer);
        chatFailoverNoticeTimer = setTimeout(() => clearChatProviderFailoverNotice(), 120000);
    }

    function clearError() {
        DOM.errorDisplay.textContent = '';
        DOM.errorDisplay.style.display = 'none';
        clearChatProviderFailoverNotice();
    }

    function displayError(message) {
        console.error("Error displayed:", message);
        DOM.errorDisplay.textContent = `Error: ${message}`;
        DOM.errorDisplay.style.display = 'block';
        DOM.loadingIndicator.style.display = 'none'; // Hide loading indicator on error
        clearChatProviderFailoverNotice();
    }

    function scrollToBottom() {
        setTimeout(() => { DOM.chatBox.scrollTop = DOM.chatBox.scrollHeight; }, 50);
    }

    function setControlsEnabled(enabled) {
        DOM.userInput.disabled = !enabled;
        DOM.sendButton.disabled = !enabled;
        if (DOM.chatVoiceInputBtn) DOM.chatVoiceInputBtn.disabled = !enabled;
        if (!enabled && typeof ChatVoiceInput !== 'undefined' && ChatVoiceInput.stop) {
            ChatVoiceInput.stop();
        }
        // DOM.suggestionsBtn.disabled = !enabled;
    }

    /** Label from the main chat AI Provider select (Gemini, Claude, …). */
    function syncLoadingIndicatorProvider() {
        const el = DOM.loadingIndicatorProvider;
        if (!el) return;
        const sel = DOM.llmProviderSelect;
        let label = '';
        if (sel && sel.options && sel.selectedIndex >= 0) {
            const opt = sel.options[sel.selectedIndex];
            if (opt && opt.textContent) label = opt.textContent.trim();
        }
        el.textContent = label;
    }

    function showLoadingIndicator() {
        VoiceSelector.updateLoadingIndicatorImage(); // Update image first
        syncLoadingIndicatorProvider();
        DOM.loadingIndicator.style.display = 'flex';
    }

    function hideLoadingIndicator() {
        DOM.loadingIndicator.style.display = 'none';
        if (DOM.loadingIndicatorProvider) DOM.loadingIndicatorProvider.textContent = '';
    }
    
    function getWorkModePrefix() {
        // Since workModeCheckbox was replaced with interviewer mode button,
        // we'll use the interviewer mode state instead
       // return AppState.isInterviewerMode ? "Do not respond with any sexual related material. " : "";
       return ""
    }

    const DASH = '\u2014';

    let lastChatFunctionCalls = null;

    function formatChatToolCallsLog(calls) {
        if (!Array.isArray(calls) || calls.length === 0) {
            return 'No tool calls in the last chat request.';
        }
        return calls.map((call, idx) => {
            const name = call && call.name ? String(call.name) : '(unknown)';
            const iter = call && call.iteration != null ? call.iteration : '?';
            const args = call && call.arguments != null ? call.arguments : {};
            let argsText;
            try {
                argsText = JSON.stringify(args, null, 2);
            } catch (_) {
                argsText = String(args);
            }
            return `#${idx + 1} — ${name} (round ${iter})\n${argsText}`;
        }).join('\n\n');
    }

    function openChatToolCallsLogModal() {
        const modal = document.getElementById('chat-tool-calls-log-modal');
        const body = document.getElementById('chat-tool-calls-log-body');
        if (!modal || !body) return;
        body.textContent = formatChatToolCallsLog(lastChatFunctionCalls);
        modal.style.display = 'flex';
    }

    function closeChatToolCallsLogModal() {
        const modal = document.getElementById('chat-tool-calls-log-modal');
        if (modal) modal.style.display = 'none';
    }

    function initChatToolCallsLogModal() {
        const seg = document.getElementById('chat-context-last-toolcalls-seg');
        const modal = document.getElementById('chat-tool-calls-log-modal');
        const closeBtn = document.getElementById('close-chat-tool-calls-log-modal');
        const closeBtn2 = document.getElementById('chat-tool-calls-log-close-btn');
        if (seg) {
            seg.addEventListener('click', (e) => {
                e.preventDefault();
                openChatToolCallsLogModal();
            });
            seg.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    openChatToolCallsLogModal();
                }
            });
        }
        if (closeBtn) closeBtn.addEventListener('click', closeChatToolCallsLogModal);
        if (closeBtn2) closeBtn2.addEventListener('click', closeChatToolCallsLogModal);
        if (modal) {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) closeChatToolCallsLogModal();
            });
        }
    }

    function syncChatContextStatusBarVisibility() {
        const bar = DOM.chatContextStatusBar;
        if (!bar) return;
        const hac = document.getElementById('have-a-chat-control-bar');
        const hacRound = document.getElementById('have-a-chat-round-prompt');
        const iv = document.getElementById('interview-control-bar');
        function visible(el) {
            if (!el) return false;
            return window.getComputedStyle(el).display !== 'none';
        }
        const hideBar = visible(hac) || visible(hacRound) || visible(iv);
        bar.style.display = hideBar ? 'none' : 'flex';
    }

    function updateChatContextStatusBarFromAvailability(av) {
        if (!av || typeof av !== 'object') return;
        const tEl = document.getElementById('chat-context-status-tools');
        const rEl = document.getElementById('chat-context-status-refs');
        const wT = document.getElementById('chat-context-status-warn-tools');
        const wR = document.getElementById('chat-context-status-warn-refs');
        const tn = av.llm_tools_count;
        const rn = av.reference_documents_available_count;
        if (tEl) tEl.textContent = typeof tn === 'number' && Number.isFinite(tn) ? String(tn) : DASH;
        if (rEl) rEl.textContent = typeof rn === 'number' && Number.isFinite(rn) ? String(rn) : DASH;
        if (wT) wT.style.display = (typeof tn === 'number' && tn === 0) ? 'inline' : 'none';
        if (wR) wR.style.display = (typeof rn === 'number' && rn === 0) ? 'inline' : 'none';
        syncChatContextStatusBarVisibility();
    }

    function setChatLastRequestStatsFromEmbedded(embeddedJson) {
        const inEl = document.getElementById('chat-context-last-in');
        const outEl = document.getElementById('chat-context-last-out');
        const fcEl = document.getElementById('chat-context-last-toolcalls');
        if (!inEl && !outEl && !fcEl) return;
        const em = embeddedJson && typeof embeddedJson === 'object' ? embeddedJson : null;
        let inT = null;
        let outT = null;
        let nFc = null;
        if (em) {
            if (typeof em.input_tokens === 'number') inT = em.input_tokens;
            if (typeof em.output_tokens === 'number') outT = em.output_tokens;
            if (Array.isArray(em.function_calls)) {
                nFc = em.function_calls.length;
                lastChatFunctionCalls = em.function_calls;
            } else {
                lastChatFunctionCalls = null;
            }
        } else {
            lastChatFunctionCalls = null;
        }
        if (inEl) inEl.textContent = inT != null ? String(inT) : DASH;
        if (outEl) outEl.textContent = outT != null ? String(outT) : DASH;
        if (fcEl) fcEl.textContent = nFc != null ? String(nFc) : DASH;
        syncChatContextStatusBarVisibility();
    }

    return {
        clearError, displayError, scrollToBottom, setControlsEnabled,
        showLoadingIndicator, hideLoadingIndicator, syncLoadingIndicatorProvider, getWorkModePrefix,
        syncChatContextStatusBarVisibility,
        updateChatContextStatusBarFromAvailability,
        setChatLastRequestStatsFromEmbedded,
        setChatProviderFailoverNotice,
        clearChatProviderFailoverNotice,
        initChatToolCallsLogModal,
    };
})();

// --- Configuration Module ---
const Config = (() => {
    function applySettings() {
        if (DOM.messageFontSize) {
            document.documentElement.style.setProperty('--message-font-size', `${DOM.messageFontSize.value}px`);
            if (DOM.messageFontSize.nextElementSibling) {
                DOM.messageFontSize.nextElementSibling.textContent = `${DOM.messageFontSize.value}px`;
            }
        }
        // Apply audio tag visibility directly (CSS might be better for this)
        if (DOM.showAudioTags) {
            document.body.classList.toggle('hide-audio-tags', !DOM.showAudioTags.checked);
        }

        // Apply creativity level text
        if (DOM.creativityLevel && DOM.creativityLevel.nextElementSibling) {
            DOM.creativityLevel.nextElementSibling.textContent = DOM.creativityLevel.value;
        }
    }

    function loadSettings() {
        const settings = JSON.parse(localStorage.getItem(CONSTANTS.LOCAL_STORAGE_KEYS.CHAT_SETTINGS) || '{}');
        if (settings.messageFontSize && DOM.messageFontSize) DOM.messageFontSize.value = settings.messageFontSize;
        if (settings.creativityLevel && DOM.creativityLevel) DOM.creativityLevel.value = settings.creativityLevel;
        if (settings.showAudioTags !== undefined && DOM.showAudioTags) DOM.showAudioTags.checked = settings.showAudioTags;
        if (settings.showImageTags !== undefined && DOM.showImageTags) DOM.showImageTags.checked = settings.showImageTags;
        if (settings.showJsonTags !== undefined && DOM.showJsonTags) DOM.showJsonTags.checked = settings.showJsonTags;
        if (settings.autoVoiceShortResponses !== undefined && DOM.autoVoiceShortResponses) DOM.autoVoiceShortResponses.checked = settings.autoVoiceShortResponses;
        if (settings.autoOpenSuggestions !== undefined && DOM.autoOpenSuggestions) DOM.autoOpenSuggestions.checked = settings.autoOpenSuggestions;
        if (settings.allowExplicitContent !== undefined && DOM.allowExplicitContentCheckbox) {
            DOM.allowExplicitContentCheckbox.checked = settings.allowExplicitContent;
        }
        if (settings.enableSnarkiness !== undefined && DOM.enableSnarkinessCheckbox) {
            DOM.enableSnarkinessCheckbox.checked = settings.enableSnarkiness;
        }

        applySettings();
    }

    function saveSettings() {
        const settings = {
            messageFontSize: DOM.messageFontSize ? DOM.messageFontSize.value : '',
            creativityLevel: DOM.creativityLevel ? DOM.creativityLevel.value : '',
            showAudioTags: DOM.showAudioTags ? DOM.showAudioTags.checked : false,
            showImageTags: DOM.showImageTags ? DOM.showImageTags.checked : false,
            showJsonTags: DOM.showJsonTags ? DOM.showJsonTags.checked : false,
            autoVoiceShortResponses: DOM.autoVoiceShortResponses ? DOM.autoVoiceShortResponses.checked : false,
            autoOpenSuggestions: DOM.autoOpenSuggestions ? DOM.autoOpenSuggestions.checked : true,
            allowExplicitContent: DOM.allowExplicitContentCheckbox ? DOM.allowExplicitContentCheckbox.checked : false,
            enableSnarkiness: DOM.enableSnarkinessCheckbox ? DOM.enableSnarkinessCheckbox.checked : false,
        };
        localStorage.setItem(CONSTANTS.LOCAL_STORAGE_KEYS.CHAT_SETTINGS, JSON.stringify(settings));
        applySettings();
    }

    function init() {
        loadSettings();
        [DOM.messageFontSize, DOM.creativityLevel, DOM.showAudioTags, DOM.showImageTags, DOM.showJsonTags, DOM.companionModeCheckbox, DOM.allowExplicitContentCheckbox, DOM.enableSnarkinessCheckbox, DOM.autoVoiceShortResponses, DOM.autoOpenSuggestions].forEach(el => {
            if (el && el.type === 'checkbox') {
                el.addEventListener('change', saveSettings);
            } else if (el) {
                el.addEventListener('input', saveSettings);
            }
        });
        // Special handling for creativityLevel label update on input
        if (DOM.creativityLevel) {
            DOM.creativityLevel.addEventListener('input', (e) => {
                if (e.target.nextElementSibling) {
                    e.target.nextElementSibling.textContent = e.target.value;
                }
            });
        }
    }
    return { init, saveSettings, loadSettings }; // Expose saveSettings for voice selector
})();

