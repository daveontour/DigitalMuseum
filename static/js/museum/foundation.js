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
        VOICE: '/voice',
        SUGGESTIONS_JSON: '/api/suggestions',
        FB_CHATTERS: '/facebook-chatters/one_to_one',
        CONTACTS: '/getContacts',
        MESSAGES_BY_CONTACT: '/getMessagesByContact',
        MESSAGES_BY_CONTACT_V2: '/getConversationsByParticipant?name=',
        ALBUMS: '/getAlbums',
        EVENTS: '/events',
        HAVE_YOUR_SAY: '/HaveYourSay',
        WHAT_ARE_PEOPLE_SAYING: '/WhatArePeopleSaying',
        HAVE_A_CHAT_TURN: '/chat/have-a-chat/turn'
    },
    LOCAL_STORAGE_KEYS: {
        CHAT_SETTINGS: 'chatSettings'
    },
    LLM_PROVIDERS: {
        GEMINI: "{{gemini_configured}}",
        CLAUDE: "{{claude_configured}}"
    },
    OWNER_NAME: "{{owner}}",
    OWNER_GENDER: "{{owner_gender}}",
    RANDOM_QUESTION_PROMPT: "Generate a random question about {{owner}}'s life. It could be about any aspect of his biography, people he's known, travels, work, hobbies, relationships, psychology, interest, anything. The objective is that by answering the question it would provide insight into him or reveal hidden or understated aspects of him or amusing facts. Do not answer the question, just generate it.",
    TODAYS_THING_PROMPT: "{{todays_thing_prompt}}",
};

// --- DOM Element Cache ---
const DOM = {
    chatBox: document.getElementById('chat-box'),
    chatForm: document.getElementById('chat-form'),
    userInput: document.getElementById('user-input'),
    sendButton: document.getElementById('send-button'),
    suggestionsBtn: document.getElementById('suggestions-btn'),
    loadingIndicator: document.getElementById('loading-indicator'),
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
    chatMain: document.querySelector('.chat-main'),
    messageFontSize: document.getElementById('message-font-size'),
    creativityLevel: document.getElementById('creativity-level'),
    showAudioTags: document.getElementById('show-audio-tags'),
    showImageTags: document.getElementById('show-image-tags'),
    showJsonTags: document.getElementById('show-json-tags'),
    autoVoiceShortResponses: document.getElementById('auto-voice-short-responses'),
    companionModeCheckbox: document.getElementById('companion-mode'),
    llmProviderSelect: document.getElementById('llm-provider-select'),
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
    relationshipsModal: document.getElementById('relationships-modal'),
    closeRelationshipsModalBtn: document.getElementById('close-relationships-modal'),
    contactsModal: document.getElementById('contacts-modal'),
    closeContactsModalBtn: document.getElementById('close-contacts-modal'),
    extractContactsBtn: document.getElementById('extract-contacts-btn'),
    contactsLoading: document.getElementById('contacts-loading'),
    contactsTableContainer: document.getElementById('contacts-table-container'),
    contactsTableBody: document.getElementById('contacts-table-body'),
    contactsPaginationInfoTop: document.getElementById('contacts-pagination-info-top'),
    contactsPaginationInfoBottom: document.getElementById('contacts-pagination-info-bottom'),
    contactsPageSize: document.getElementById('contacts-page-size'),
    contactsPageSizeBottom: document.getElementById('contacts-page-size-bottom'),
    contactsPrevBtn: document.getElementById('contacts-prev-btn'),
    contactsNextBtn: document.getElementById('contacts-next-btn'),
    contactsPrevBtnTop: document.getElementById('contacts-prev-btn-top'),
    contactsNextBtnTop: document.getElementById('contacts-next-btn-top'),
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
    conversationIndicator: document.getElementById('conversation-indicator'),
    newConversationModal: document.getElementById('new-conversation-modal'),
    // Subject Configuration elements (now in config tab)
    subjectNameInput: document.getElementById('subject-name-input'),
    subjectGenderSelect: document.getElementById('subject-gender-select'),
    familyNameInput: document.getElementById('family-name-input'),
    otherNamesInput: document.getElementById('other-names-input'),
    emailAddressesInput: document.getElementById('email-addresses-input'),
    phoneNumbersInput: document.getElementById('phone-numbers-input'),
    whatsappHandleInput: document.getElementById('whatsapp-handle-input'),
    instagramHandleInput: document.getElementById('instagram-handle-input'),
    requestWritingStyleBtn: document.getElementById('request-writing-style-btn'),
    writingStyleLoading: document.getElementById('writing-style-loading'),
    writingStyleDisplay: document.getElementById('writing-style-display'),
    requestPsychologicalProfileBtn: document.getElementById('request-psychological-profile-btn'),
    psychologicalProfileLoading: document.getElementById('psychological-profile-loading'),
    psychologicalProfileDisplay: document.getElementById('psychological-profile-display'),
    systemInstructionsTextarea: document.getElementById('system-instructions-textarea'),
    coreSystemInstructionsTextarea: document.getElementById('core-system-instructions-textarea'),
    subjectConfigTabs: document.querySelectorAll('.subject-config-tab'),
    saveSubjectConfigBtn: document.getElementById('save-subject-config-btn'),
    cancelSubjectConfigBtn: document.getElementById('cancel-subject-config-btn'),
    saveSystemInstructionsBtn: document.getElementById('save-system-instructions-btn'),
    cancelSystemInstructionsBtn: document.getElementById('cancel-system-instructions-btn'),
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
    shufflePhotosBtn: document.getElementById('shuffle-photos-btn'),
    refreshLocationsBtn: document.getElementById('refresh-locations-btn'),
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
    emailGalleryAttachmentsGrid: document.getElementById('email-gallery-attachments-grid'),
    emailGallerySearch: document.getElementById('email-gallery-search'),
    emailGallerySender: document.getElementById('email-gallery-sender'),
    emailGalleryRecipient: document.getElementById('email-gallery-recipient'),
    emailGalleryToFrom: document.getElementById('email-gallery-to-from'),
    emailGalleryYearFilter: document.getElementById('email-gallery-year-filter'),
    emailGalleryMonthFilter: document.getElementById('email-gallery-month-filter'),
    emailGalleryAttachmentsFilter: document.getElementById('email-gallery-attachments-filter'),
    emailGalleryFolderFilter: document.getElementById('email-gallery-folder-filter'),
    emailGallerySearchBtn: document.getElementById('email-gallery-search-btn'),
    emailGalleryClearBtn: document.getElementById('email-gallery-clear-btn'),
    emailGalleryViewToggle: document.getElementById('email-gallery-view-toggle'),
    emailGalleryMetadataSubject: document.getElementById('email-gallery-metadata-subject'),
    emailGalleryMetadataFrom: document.getElementById('email-gallery-metadata-from'),
    emailGalleryMetadataDate: document.getElementById('email-gallery-metadata-date'),
    emailGalleryMetadataFolder: document.getElementById('email-gallery-metadata-folder'),
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
    newImageGalleryAuthor: document.getElementById('new-image-gallery-author'),
    newImageGallerySource: document.getElementById('new-image-gallery-source'),
    newImageGalleryYearFilter: document.getElementById('new-image-gallery-year-filter'),
    newImageGalleryMonthFilter: document.getElementById('new-image-gallery-month-filter'),
    newImageGalleryRating: document.getElementById('new-image-gallery-rating'),
    newImageGalleryRatingMin: document.getElementById('new-image-gallery-rating-min'),
    newImageGalleryRatingMax: document.getElementById('new-image-gallery-rating-max'),
    newImageGalleryHasGps: document.getElementById('new-image-gallery-has-gps'),
    newImageGallerySearchBtn: document.getElementById('new-image-gallery-search-btn'),
    newImageGalleryClearBtn: document.getElementById('new-image-gallery-clear-btn'),
    newImageGalleryThumbnailGrid: document.getElementById('new-image-gallery-thumbnail-grid'),
    newImageGalleryMasterPane: document.querySelector('.new-image-gallery-master-pane'),
    newImageGallerySelectMode: document.getElementById('new-image-gallery-select-mode'),
    newImageGallerySelectedCount: document.getElementById('new-image-gallery-selected-count'),
    newImageGalleryBulkTags: document.getElementById('new-image-gallery-bulk-tags'),
    newImageGalleryApplyTagsBtn: document.getElementById('new-image-gallery-apply-tags-btn'),
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
    function clearError() {
        DOM.errorDisplay.textContent = '';
        DOM.errorDisplay.style.display = 'none';
    }

    function displayError(message) {
        console.error("Error displayed:", message);
        DOM.errorDisplay.textContent = `Error: ${message}`;
        DOM.errorDisplay.style.display = 'block';
        DOM.loadingIndicator.style.display = 'none'; // Hide loading indicator on error
    }

    function scrollToBottom() {
        setTimeout(() => { DOM.chatBox.scrollTop = DOM.chatBox.scrollHeight; }, 50);
    }

    function setControlsEnabled(enabled) {
        DOM.userInput.disabled = !enabled;
        DOM.sendButton.disabled = !enabled;
        // DOM.suggestionsBtn.disabled = !enabled;
    }

    function showLoadingIndicator() {
        VoiceSelector.updateLoadingIndicatorImage(); // Update image first
        DOM.loadingIndicator.style.display = 'flex';
    }

    function hideLoadingIndicator() {
        DOM.loadingIndicator.style.display = 'none';
    }
    
    function getWorkModePrefix() {
        // Since workModeCheckbox was replaced with interviewer mode button,
        // we'll use the interviewer mode state instead
       // return AppState.isInterviewerMode ? "Do not respond with any sexual related material. " : "";
       return ""
    }

    return {
        clearError, displayError, scrollToBottom, setControlsEnabled,
        showLoadingIndicator, hideLoadingIndicator, getWorkModePrefix
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

        };
        localStorage.setItem(CONSTANTS.LOCAL_STORAGE_KEYS.CHAT_SETTINGS, JSON.stringify(settings));
        applySettings();
    }

    function init() {
        loadSettings();
        [DOM.messageFontSize, DOM.creativityLevel, DOM.showAudioTags, DOM.showImageTags, DOM.showJsonTags, DOM.companionModeCheckbox, DOM.autoVoiceShortResponses].forEach(el => {
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

