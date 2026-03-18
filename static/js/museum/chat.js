'use strict';

// --- Chat Module ---
const Chat = (() => {
    function renderMarkdown(element, text) {
        const sanitizedText = text.replace(/</g, "<").replace(/>/g, ">"); // Basic sanitization
        element.innerHTML = marked.parse(sanitizedText || '');
    }

    function _createMessageElement(role, messageId) {
        const messageElement = document.createElement('div');
        messageElement.classList.add('message', role === 'suggestion' ? 'user-message' : `${role}-message`);
        if (role === 'suggestion') messageElement.style.backgroundColor = "#f4f778";
        if (messageId) messageElement.id = messageId;
        return messageElement;
    }


    function _addVoiceBranding(messageElement, role) {
        if (role === 'assistant') {
            const selectedVoice = VoiceSelector.getSelectedVoice();
            messageElement.classList.add(`voice-${selectedVoice}`);

            const brandingContainer = document.createElement('div');
            brandingContainer.className = 'message-voice-branding';

            const voiceImageSmall = document.createElement('img');
            voiceImageSmall.className = 'message-voice-image';
            voiceImageSmall.src = `/static/images/${VoiceSelector.getVoiceImage(selectedVoice, true)}`;
            voiceImageSmall.alt = `${selectedVoice} character`;
            brandingContainer.appendChild(voiceImageSmall);

            if (selectedVoice === 'owner' && DOM.ownerMood) {
                const moodSpan = document.createElement('span');
                moodSpan.className = 'message-voice-mood';
                moodSpan.textContent = DOM.ownerMood.options[DOM.ownerMood.selectedIndex]?.text || DOM.ownerMood.value || '';
                brandingContainer.appendChild(moodSpan);
            }

            messageElement.appendChild(brandingContainer);
        }
    }

    function _addSaveButton(messageElement, role, text, contentElement) {
        if (!['assistant', 'model'].includes(role)) return;
        const saveButton = document.createElement('button');
        saveButton.className = 'copy-hover-btn save-hover-btn';
        saveButton.innerHTML = '<i class="fa-regular fa-floppy-disk"></i> Save';
        saveButton.addEventListener('click', () => {
            let contentToSave = text;
            const rawMarkdown = contentElement.querySelector('.raw-markdown');
            if (rawMarkdown) contentToSave = rawMarkdown.value;
            const voice = typeof VoiceSelector !== 'undefined' && VoiceSelector.getSelectedVoice ? VoiceSelector.getSelectedVoice() : null;
            const llmProvider = DOM.llmProviderSelect?.value || null;
            Modals.SaveResponseTitle.open('', (title) => {
                _doSaveResponse(saveButton, title, contentToSave, voice, llmProvider);
            });
        });
        messageElement.appendChild(saveButton);
    }

    async function _doSaveResponse(saveButton, title, contentToSave, voice, llmProvider) {
        try {
            const res = await fetch('/api/saved-responses', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    title: title.trim(),
                    content: contentToSave,
                    voice: voice || undefined,
                    llm_provider: llmProvider || undefined
                })
            });
            if (!res.ok) throw new Error(await res.text());
            saveButton.innerHTML = '<i class="fa-solid fa-check"></i> Saved!';
            saveButton.classList.add('saved');
            setTimeout(() => {
                saveButton.innerHTML = '<i class="fa-regular fa-floppy-disk"></i> Save';
                saveButton.classList.remove('saved');
            }, 2000);
        } catch (err) {
            console.error('Failed to save:', err);
            saveButton.innerHTML = '<i class="fa-solid fa-xmark"></i> Failed';
            saveButton.classList.add('error');
            setTimeout(() => {
                saveButton.innerHTML = '<i class="fa-regular fa-floppy-disk"></i> Save';
                saveButton.classList.remove('error');
            }, 2000);
        }
    }

    function _addCopyButton(messageElement, role, text, contentElement) {
        const copyButton = document.createElement('button');
        copyButton.className = 'copy-hover-btn';
        copyButton.innerHTML = '<i class="fa-regular fa-copy"></i> Copy';
        copyButton.addEventListener('click', async () => {
            try {
                let textToCopy = text;
                if (['assistant', 'model', 'system'].includes(role)) {
                    const rawMarkdown = contentElement.querySelector('.raw-markdown');
                    textToCopy = rawMarkdown ? rawMarkdown.value : text;
                }
                await navigator.clipboard.writeText(textToCopy);
                copyButton.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
                copyButton.classList.add('copied');
                setTimeout(() => {
                    copyButton.innerHTML = '<i class="fa-regular fa-copy"></i> Copy';
                    copyButton.classList.remove('copied');
                }, 2000);
            } catch (err) {
                console.error('Failed to copy text:', err);
                copyButton.innerHTML = '<i class="fa-solid fa-xmark"></i> Failed';
                copyButton.classList.add('error');
                 setTimeout(() => {
                    copyButton.innerHTML = '<i class="fa-regular fa-copy"></i> Copy';
                    copyButton.classList.remove('error');
                }, 2000);
            }
        });
        messageElement.appendChild(copyButton);
    }

    function _renderMessageContent(contentElement, text, role, isMarkdown) {
        contentElement.dataset.role = (role === 'suggestion') ? "user" : role;
        if (role === 'suggestion') contentElement.style.backgroundColor = "#f4f778";

        if ((isMarkdown && role !== 'user') || role === 'suggestion') {
            const rawMarkdown = document.createElement('textarea');
            rawMarkdown.className = 'raw-markdown';
            rawMarkdown.style.display = 'none';
            rawMarkdown.value = text;
            contentElement.appendChild(rawMarkdown);
            renderMarkdown(contentElement, text);
        } else {
            contentElement.textContent = text;
        }
    }

    function _addImagePreviews(messageElement, embeddedJson) {
       
        if (embeddedJson && embeddedJson.attachments && Array.isArray(embeddedJson.attachments) && DOM.showImageTags.checked) {
            const imageButtonsContainer = document.createElement('div');
            imageButtonsContainer.className = 'image-buttons-container';
            embeddedJson.attachments.forEach(uri => {
                const imagePreview = document.createElement('img');
                imagePreview.src = uri+'&preview=true&resize_to=100';
                imagePreview.alt = 'Image Preview';
                imagePreview.className = 'image-preview';
                imagePreview.style.cursor = 'pointer';
                imagePreview.loading = 'lazy';
                imagePreview.onclick = () => _imageSwitcher(embeddedJson.attachments, uri); // Delegate to modal
                imageButtonsContainer.appendChild(imagePreview);
            });
            messageElement.appendChild(imageButtonsContainer);
        }
    }

    function _imageSwitcher(images, uri){

        if (images.length > 1) {
            Modals.MultiImageDisplay.showMultiImageModal(images, uri);
        } else {
            Modals.SingleImageDisplay.showSingleImageModal(images[0], uri, 0, 0, 0);
        }
            
    }

    function _addJsonViewer(messageElement, embeddedJson) {
        if (embeddedJson && DOM.showJsonTags.checked) {
            const jsonWrapper = document.createElement('div');
            jsonWrapper.className = 'json-wrapper';
            const jsonToggle = document.createElement('button');
            jsonToggle.className = 'json-toggle';
            jsonToggle.innerHTML = '<i class="fa-solid fa-chevron-down"></i>';
            const jsonContent = document.createElement('div');
            jsonContent.className = 'json-content';
            jsonContent.style.display = 'none';
            const pre = document.createElement('pre');
            pre.textContent = JSON.stringify(embeddedJson, null, 2);
            jsonContent.appendChild(pre);
            jsonToggle.onclick = () => {
                const isHidden = jsonContent.style.display === 'none';
                jsonContent.style.display = isHidden ? 'block' : 'none';
                jsonToggle.innerHTML = isHidden ? '<i class="fa-solid fa-chevron-up"></i>' : '<i class="fa-solid fa-chevron-down"></i>';
            };
            jsonWrapper.appendChild(jsonToggle);
            jsonWrapper.appendChild(jsonContent);
            messageElement.appendChild(jsonWrapper);
        }
    }

    function _addResponseActionButtons(messageElement, role, embeddedJson) {
        if (!['assistant', 'model'].includes(role)) return;
        if (!embeddedJson) return;

        const buttons = [
            {
                key: 'showImages',
                icon: 'fa-image',
                modifier: 'images',
                title: (v) => `Show images tagged ${v}`,
                action: (v) => Modals.NewImageGallery?.openTaggedImages?.(v)
            },
            {
                key: 'showLocation',
                icon: 'fa-map-marker-alt',
                modifier: 'locations',
                title: (v) => `Show locations`,
                action: () => Modals.Locations?.openMapView?.()
            },
            {
                key: 'showEmails',
                icon: 'fa-envelope',
                modifier: 'emails',
                title: (v) => `Show emails for ${v}`,
                action: (v) => Modals.EmailGallery?.openContact?.(v)
            },
            {
                key: 'showMessages',
                icon: 'fa-comments',
                modifier: 'messages',
                title: (v) => `Show messages for ${v}`,
                action: (v) => Modals.SMSMessages?.openWithFilter?.(v)
            },
            {
                key: 'showFacebookAlbum',
                icon: 'fa-images',
                modifier: 'facebook-album',
                title: () => `Open Facebook album`,
                action: (v) => Modals.FBAlbums?.openAndSelectAlbum?.(Number(v))
            },
            {
                key: 'showFacebookPost',
                icon: 'fa-newspaper',
                modifier: 'facebook-post',
                title: () => `Open Facebook posts`,
                action: (v) => Modals.FBPosts?.openAndFilterOnPosts?.(v)
            }
        ];

        const row = document.createElement('div');
        row.className = 'response-action-buttons';

        buttons.forEach(({ key, icon, modifier, title, action }) => {
            const value = embeddedJson[key];
            if (value === undefined || value === null || (typeof value !== 'string' && typeof value !== 'number' && !Array.isArray(value))) return;

            const btn = document.createElement('button');
            btn.className = `response-action-btn response-action-btn--${modifier}`;
            btn.innerHTML = `<i class="fas ${icon}"></i>`;
            btn.title = title(value);
            btn.addEventListener('click', () => action(value));
            row.appendChild(btn);
        });

        if (embeddedJson.randomQuestion === true && embeddedJson.randomQuestionText) {
            const spacer = document.createElement('span');
            spacer.className = 'response-action-spacer';
            spacer.style.flex = '1';
            row.appendChild(spacer);

            const answerBtn = document.createElement('button');
            answerBtn.className = 'response-action-btn response-action-btn--answer';
            answerBtn.textContent = 'Answer';
            answerBtn.addEventListener('click', () => {
                if (typeof App !== 'undefined' && App.processAnswerSubmit) {
                    App.processAnswerSubmit(embeddedJson.randomQuestionText);
                }
            });
            row.appendChild(answerBtn);
        }

        if (row.children.length > 0) {
            messageElement.appendChild(row);
        }
    }

    function _addSpeakButton(contentElement, role) {
        if (['assistant', 'model'].includes(role) && DOM.showAudioTags.checked) {
            const speakButton = document.createElement('button');
            speakButton.className = 'speak-button';
            speakButton.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="M14,3.23V5.29C16.89,6.15 19,8.83 19,12C19,15.17 16.89,17.84 14,18.7V20.77C18,19.86 21,16.28 21,12C21,7.72 18,4.14 14,3.23M16.5,12C16.5,10.23 15.5,8.71 14,7.97V16C15.5,15.29 16.5,13.76 16.5,12M3,9V15H7L12,20V4L7,9H3Z"/></svg>';
            
            const audioElement = document.createElement('audio');
            audioElement.controls = true;
            audioElement.style.display = 'none';

            const statusMessage = document.createElement('div');
            statusMessage.className = 'audio-status';
            statusMessage.style.display = 'none'; // CSS handles this
            
            contentElement.appendChild(speakButton);
            contentElement.appendChild(audioElement);
            contentElement.appendChild(statusMessage);

            const showAudioStatus = (message, isError = false) => {
                statusMessage.textContent = message;
                statusMessage.style.display = 'block';
                statusMessage.style.color = isError ? '#dc3545' : '#666';
            };

            audioElement.addEventListener('playing', () => { statusMessage.style.display = 'none'; });
            audioElement.addEventListener('error', () => { showAudioStatus('Error loading audio', true); });

            speakButton.onclick = async () => {
                try {
                    showAudioStatus('Loading audio...');
                    const selectedVoice = VoiceSelector.getSelectedVoice();
                    const audioBlob = await ApiService.fetchVoice({ text: contentElement.textContent, voice: selectedVoice });
                    if (!audioBlob) throw new Error("Voice conversion failed or returned no data.");

                    audioElement.src = URL.createObjectURL(audioBlob);
                    audioElement.style.display = 'block';
                    speakButton.style.display = 'none';
                    statusMessage.style.display = 'none';
                    audioElement.play();
                } catch (error) {
                    console.error('Error playing voice:', error);
                    showAudioStatus(error.message || 'Error loading audio', true);
                }
            };
        }
    }

    function addMessage(role, text, isMarkdown = true, messageId = null, embeddedJson = null) {

        if (embeddedJson && embeddedJson.showHelp) {
            Guide.onTopicSelected(embeddedJson.showHelp);
            return;
        }

        const messageElement = _createMessageElement(role, messageId);
        _addVoiceBranding(messageElement, role);

        const contentElement = document.createElement('div');
        contentElement.classList.add('message-content');
        _renderMessageContent(contentElement, text, role, isMarkdown);
        _addSaveButton(messageElement, role, text, contentElement);
        _addCopyButton(messageElement, role, text, contentElement); // Pass contentElement for raw markdown access

        messageElement.appendChild(contentElement);

        _addImagePreviews(messageElement, embeddedJson);
        _addResponseActionButtons(messageElement, role, embeddedJson);  //Add the buttons for the actions that the AI wants to perform
        _addJsonViewer(messageElement, embeddedJson);
        _addSpeakButton(contentElement, role); // Pass contentElement as it appends to it

        DOM.chatBox.appendChild(messageElement);
        UI.scrollToBottom();
        
        // Auto-voice for short responses
        if (role === 'assistant' && DOM.autoVoiceShortResponses && DOM.autoVoiceShortResponses.checked) {
            const wordCount = text.trim().split(/\s+/).length;
            if (wordCount < 60) {
                // Automatically trigger voice API for short responses
                setTimeout(() => {
                    const speakButton = messageElement.querySelector('.speak-button');
                    if (speakButton) {
                        speakButton.click();
                    }
                }, 500); // Small delay to ensure the message is fully rendered
            }
        }

        return messageElement;
    }

    function addEmail(email, messageId = null, embeddedJson = null) {
        const messageElement = _createMessageElement('email', messageId);
        
        const contentElement = document.createElement('div');
        contentElement.classList.add('message-content', 'email-content');
        
        // Create email header
        const emailHeader = document.createElement('div');
        emailHeader.classList.add('email-header');
        
        // From field
        if (email.from) {
            const fromField = document.createElement('div');
            fromField.classList.add('email-field');
            fromField.innerHTML = `<strong>From:</strong> ${email.from}`;
            emailHeader.appendChild(fromField);
        }
        
        // To field
        if (email.to) {
            const toField = document.createElement('div');
            toField.classList.add('email-field');
            toField.innerHTML = `<strong>To:</strong> ${email.to}`;
            emailHeader.appendChild(toField);
        }
        
        // CC field
        if (email.cc) {
            const ccField = document.createElement('div');
            ccField.classList.add('email-field');
            ccField.innerHTML = `<strong>CC:</strong> ${email.cc}`;
            emailHeader.appendChild(ccField);
        }
        
        // BCC field
        if (email.bcc) {
            const bccField = document.createElement('div');
            bccField.classList.add('email-field');
            bccField.innerHTML = `<strong>BCC:</strong> ${email.bcc}`;
            emailHeader.appendChild(bccField);
        }
        
        // Subject field
        if (email.subject) {
            const subjectField = document.createElement('div');
            subjectField.classList.add('email-field', 'email-subject');
            subjectField.innerHTML = `<strong>Subject:</strong> ${email.subject}`;
            emailHeader.appendChild(subjectField);
        }
        
        // Date field
        if (email.date) {
            const dateField = document.createElement('div');
            dateField.classList.add('email-field');
            dateField.innerHTML = `<strong>Date:</strong> ${email.date}`;
            emailHeader.appendChild(dateField);
        }
        
        // Reply-To field
        if (email.reply_to) {
            const replyToField = document.createElement('div');
            replyToField.classList.add('email-field');
            replyToField.innerHTML = `<strong>Reply-To:</strong> ${email.reply_to}`;
            emailHeader.appendChild(replyToField);
        }
        
        // Message-ID field
        if (email.message_id) {
            const messageIdField = document.createElement('div');
            messageIdField.classList.add('email-field');
            messageIdField.innerHTML = `<strong>Message-ID:</strong> ${email.message_id}`;
            emailHeader.appendChild(messageIdField);
        }
        
        contentElement.appendChild(emailHeader);
        
        // Add separator line
        const separator = document.createElement('hr');
        separator.classList.add('email-separator');
        contentElement.appendChild(separator);
        
        // Email body
        const emailBody = document.createElement('div');
        emailBody.classList.add('email-body');
        
        // Check if any body content exists
        if (email.body_html) {
            emailBody.innerHTML = email.body_html;
        } else if (email.body_text) {
            // Convert newlines to <br> tags for proper display
            emailBody.innerHTML = email.body_text.replace(/\n/g, '<br>');
        } else if (email.body) {
            emailBody.textContent = email.body;
        } else {
            // If no body content, show a placeholder
            emailBody.textContent = '[No content]';
        }
        
        contentElement.appendChild(emailBody);
        
        // Add copy button for the entire email
        const emailText = _formatEmailAsText(email);
        _addCopyButton(messageElement, 'email', emailText, contentElement);
        
        messageElement.appendChild(contentElement);
        
        // Add attachments if present
        if (email.attachments && email.attachments.length > 0) {
            const attachmentsContainer = document.createElement('div');
            attachmentsContainer.classList.add('email-attachments');
            
            const attachmentsTitle = document.createElement('h4');
            attachmentsTitle.textContent = 'Attachments:';
            attachmentsContainer.appendChild(attachmentsTitle);
            
            email.attachments.forEach(attachment => {
                const attachmentElement = document.createElement('div');
                attachmentElement.classList.add('email-attachment');
                attachmentElement.innerHTML = `
                    <i class="fa fa-paperclip"></i>
                    <span>${attachment.filename || attachment.name || 'Unknown file'}</span>
                    ${attachment.size ? `<span class="attachment-size">(${_formatFileSize(attachment.size)})</span>` : ''}
                `;
                attachmentsContainer.appendChild(attachmentElement);
            });
            
            contentElement.appendChild(attachmentsContainer);
        }
        
        _addJsonViewer(messageElement, embeddedJson);
        
        DOM.chatBox.appendChild(messageElement);
        UI.scrollToBottom();
        return messageElement;
    }

    function _formatEmailAsText(email) {
        let text = '';
        
        if (email.from) text += `From: ${email.from}\n`;
        if (email.to) text += `To: ${email.to}\n`;
        if (email.cc) text += `CC: ${email.cc}\n`;
        if (email.bcc) text += `BCC: ${email.bcc}\n`;
        if (email.subject) text += `Subject: ${email.subject}\n`;
        if (email.date) text += `Date: ${email.date}\n`;
        if (email.reply_to) text += `Reply-To: ${email.reply_to}\n`;
        if (email.message_id) text += `Message-ID: ${email.message_id}\n`;
        
        text += '\n';
        
        if (email.body_html) {
            // Strip HTML tags for plain text version
            text += email.body_html.replace(/<[^>]*>/g, '');
        } else if (email.body_text) {
            text += email.body_text;
        } else if (email.body) {
            text += email.body;
        }
        
        if (email.attachments && email.attachments.length > 0) {
            text += '\n\nAttachments:\n';
            email.attachments.forEach(attachment => {
                text += `- ${attachment.filename || attachment.name || 'Unknown file'}`;
                if (attachment.size) {
                    text += ` (${_formatFileSize(attachment.size)})`;
                }
                text += '\n';
            });
        }
        
        return text;
    }

    function _formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    
    function _createConversationElement() {
        const conversationElement = document.createElement('div');
       // conversationElement.classList.add('conversation');
        conversationElement.classList.add('message');
        conversationElement.classList.add('assistant-message');
        conversationElement.classList.add('voice-expert');
        return conversationElement;
    }

    function _createConversationMessageElement(contents) {

        // Create a new div element for the conversation message. The div should have a grid layout with 3 columns. 
        // The first row should have the sender_name, timestamp and source. 
        // The second row should have the one column that spans the threecontent with the  content

        try {

            // 1. Create the main container
            const myContainer = document.createElement('div');
            myContainer.classList.add('conversation-message');


            // 2. Create the top row
            const topRow = document.createElement('div');
            topRow.classList.add('conversation-row', 'conversation-top-row');

            // 3. Create items for the top row using the 'contents' object
            // Item 1
            const topItem1 = document.createElement('div');
            topItem1.classList.add('conversation-item');
            // Sanitize sender_name to only alphanumeric characters
            let safeSenderName = (contents.sender_name || 'Default Item 1 (Bold)').replace(/[^a-zA-Z0-9 ]/g, '');
            topItem1.innerHTML = `<strong>${safeSenderName}</strong>`;

            // Item 2
            const topItem2 = document.createElement('div');
            topItem2.classList.add('conversation-item');
            topItem2.textContent = contents.sent_at || '-';

            // Item 3
            const topItem3 = document.createElement('div');
            topItem3.classList.add('conversation-item');
            topItem3.textContent = contents.source || 'Default Item 3';

            // 4. Append top items to the top row
            topRow.appendChild(topItem1);
            topRow.appendChild(topItem2);
            topRow.appendChild(topItem3);

            // 5. Create the bottom row
            const bottomRow = document.createElement('div');
            bottomRow.classList.add('conversation-row', 'conversation-bottom-row');

            // 6. Create item for the bottom row using the 'contents' object
            const bottomItem = document.createElement('div');
            bottomItem.classList.add('conversation-item', 'conversation-full-width-item');
           

            // 7. Append bottom item to the bottom row
            if (contents.attachments && contents.attachments.length > 0) {
                _addImagePreviews(bottomItem, contents);
            } else {
                bottomItem.textContent = contents.content || 'Default Full Width Item';
            }

            bottomRow.appendChild(bottomItem);

            // 8. Append rows to the main container
            myContainer.appendChild(topRow);
            myContainer.appendChild(bottomRow);

            // 9. Return the created container
            return myContainer;

        } catch (error) {
            console.error('Error creating conversation message element:', error);
            return null;
        }
    }


    function addConversation(messagesData) {
        const conversationElement = _createConversationElement();

        for (const conversation of messagesData) {
            for (const message of conversation.messages) {
            const messageElement =_createConversationMessageElement(message);
            conversationElement.appendChild(messageElement);
            }
        }
        
        DOM.chatBox.appendChild(conversationElement);
        UI.scrollToBottom();
        return conversationElement;
    }

    function clearChat() {
        DOM.chatBox.innerHTML = '';
    }

    function renderExistingMessages() {
        DOM.chatBox.querySelectorAll('.message .message-content').forEach(contentElement => {
            const rawText = contentElement.textContent; // Or from raw-markdown if it exists
            const role = contentElement.dataset.role;
            if (role === 'assistant' || role === 'model') {
                renderMarkdown(contentElement, rawText);
            } // User messages are already plain text
        });
        UI.scrollToBottom();
    }

    function updateMoodInMessages() {
        if (!DOM.ownerMood) return;
        const moodText = DOM.ownerMood.options[DOM.ownerMood.selectedIndex]?.text || DOM.ownerMood.value || '';
        DOM.chatBox.querySelectorAll('.message.voice-owner .message-voice-mood').forEach(el => {
            el.textContent = moodText;
        });
    }

    return { addMessage, clearChat, addConversation, renderExistingMessages, renderMarkdown, addEmail, updateMoodInMessages };
})();

// --- API Service Module ---
const ApiService = (() => {
    async function _fetch(url, options) {
        const response = await fetch(url, options);
        if (!response.ok) {
            let errorMsg = `HTTP error! Status: ${response.status}`;
            try {
                const errorData = await response.json();
                errorMsg = errorData.detail || errorData.error || errorMsg;
            } catch (e) {
                const textError = await response.text();
                errorMsg = textError || errorMsg;
            }
            throw new Error(errorMsg);
        }
        return response;
    }

    async function fetchChat(payload) {
        return _fetch(CONSTANTS.API_PATHS.CHAT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
    }
    async function fetchRandomQuestion(payload) {
        return _fetch(CONSTANTS.API_PATHS.RANDOM_QUESTION, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
    }

    async function fetchNewChat(payload) {

            return _fetch(CONSTANTS.API_PATHS.NEW_CHAT, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            }); // Returns the raw response for streaming/json handling by caller
        
    }

    async function fetchVoice(payload) {
        const response = await _fetch(CONSTANTS.API_PATHS.VOICE, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        return response.blob();
    }
    
    async function fetchSuggestionsConfig() {
        const response = await _fetch(CONSTANTS.API_PATHS.SUGGESTIONS_JSON);
        return response.json();
    }

    async function fetchFacebookChatters() {
        const response = await _fetch(CONSTANTS.API_PATHS.FB_CHATTERS);
        return response.json();
    }

    async function fetchContacts() {
        const response = await _fetch(CONSTANTS.API_PATHS.CONTACTS);
        return response.json();
    }
    
    async function fetchMessagesByContact(name, one_to_one_only) {
         const response = await _fetch(CONSTANTS.API_PATHS.MESSAGES_BY_CONTACT, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, one_to_one_only })
        });
        return response.json();
    }
    async function fetchMessagesByContactV2(name, one_to_one_only) {
        const url = CONSTANTS.API_PATHS.MESSAGES_BY_CONTACT_V2 + name;
        const response = await _fetch(url, {
           method: 'GET',
           headers: { 'Content-Type': 'application/json' }
       });
       return response.json();
   }

    async function fetchAlbums() {
        const response = await _fetch(CONSTANTS.API_PATHS.ALBUMS);
        return response.json();
    }

    async function fetchVoices() {
        const response = await _fetch('/api/voices');
        return response.json();
    }

    return {
        fetchChat, fetchRandomQuestion, fetchNewChat, fetchVoice, fetchSuggestionsConfig,
        fetchFacebookChatters, fetchContacts, fetchMessagesByContact, fetchAlbums, fetchMessagesByContactV2,
        fetchVoices
    };
})();

// --- Voice Selector Module ---
const VoiceSelector = (() => {
    // Module-level voice map populated from /api/voices
    let voiceMap = {};

    function _getVoiceImage(voiceName, small = false) {
        const suffix = small ? '_sm' : '';
        return CONSTANTS.VOICE_IMAGES[voiceName + suffix]
            || (small ? 'custom_sm.png' : 'custom.png');
    }

    function getSelectedVoice() {
        const voiceSelect = document.getElementById('voice-select');
        return voiceSelect ? voiceSelect.value : 'expert'; // Default to expert
    }

    function updateLoadingIndicatorImage() {
        const selectedVoice = getSelectedVoice();
        if (DOM.loadingVoiceImage) {
            DOM.loadingVoiceImage.src = `/static/images/${_getVoiceImage(selectedVoice, true)}`;
            DOM.loadingVoiceImage.alt = `${selectedVoice} character`;
        }
    }

    function updateVoicePreview(voiceName) {
        // Re-query DOM elements to ensure we have fresh references
        const voicePreviewImg = document.querySelector('.preview-image');
        const voicePreviewDesc = document.querySelector('.preview-description');

        if (voicePreviewImg) {
            const imageName = _getVoiceImage(voiceName);
            const imagePath = `/static/images/${imageName}`;
            voicePreviewImg.src = imagePath;
            voicePreviewImg.alt = `${voiceName} character`;
            voicePreviewImg.onerror = () => {
                voicePreviewImg.src = `/static/images/custom.png`;
            };
        } else {
            console.error('voicePreviewImg element not found!');
        }

        if (voicePreviewDesc) {
            let description = CONSTANTS.VOICE_DESCRIPTIONS[voiceName] || '';

            // Fall back to voiceMap description for custom voices
            if (!description && voiceMap[voiceName]) {
                description = voiceMap[voiceName].description || '';
            }

            // Try to get description from voice icon wrapper's data-description attribute first
            const voiceWrapper = document.querySelector(`.voice-icon-wrapper[data-voice="${voiceName}"]`);
            if (voiceWrapper && voiceWrapper.dataset.description) {
                description = voiceWrapper.dataset.description;
            }

            voicePreviewDesc.textContent = description;
        } else {
            console.error('voicePreviewDesc element not found!');
        }
    }

    function updateSelectedVoiceImage(voiceName) {
        if (DOM.selectedVoiceImage) {
            DOM.selectedVoiceImage.src = `/static/images/${_getVoiceImage(voiceName)}`;
            DOM.selectedVoiceImage.alt = `${voiceName} character`;
        }
    }

    async function loadVoices() {
        try {
            const voices = await ApiService.fetchVoices();
            voiceMap = {};
            voices.forEach(v => { voiceMap[v.key] = v; });

            const selects = [
                document.getElementById('voice-select'),
                document.getElementById('new-conversation-voice-select'),
            ];
            selects.forEach(sel => {
                if (!sel) return;
                const currentVal = sel.value || 'expert';
                sel.innerHTML = '';
                voices.forEach(v => {
                    const opt = document.createElement('option');
                    opt.value = v.key;
                    const label = v.description ? `${v.name} - ${v.description}` : v.name;
                    opt.textContent = label + (v.is_custom ? ' *' : '');
                    if (v.key === currentVal) opt.selected = true;
                    sel.appendChild(opt);
                });
                // Default to expert if previous value no longer exists
                if (!sel.value) {
                    const expertOpt = sel.querySelector('option[value="expert"]');
                    if (expertOpt) expertOpt.selected = true;
                }
            });
        } catch (e) {
            console.error('[VoiceSelector] Failed to load voices from API:', e);
        }
    }

    function _closeVoiceSettingsModal() {
        if (DOM.voiceSettingsModal) DOM.voiceSettingsModal.style.display = 'none';
    }

    function _handleVoiceChange(event) {
        const newVoice = event.target.value;
        updateLoadingIndicatorImage();
        updateVoicePreview(newVoice);
        updateSelectedVoiceImage(newVoice);

        if (newVoice === 'expert') {
            if (DOM.creativityLevel) {
                DOM.creativityLevel.value = 0;
                DOM.creativityLevel.disabled = true;
            }
        } else if (voiceMap[newVoice] && voiceMap[newVoice].creativity !== undefined) {
            if (DOM.creativityLevel) {
                DOM.creativityLevel.value = voiceMap[newVoice].creativity;
                DOM.creativityLevel.disabled = false;
            }
        } else if (['irish', 'earthchild', 'haiku', 'insult', 'secret_admirer'].includes(newVoice)) {
            if (DOM.creativityLevel) {
                DOM.creativityLevel.value = 2.0;
                DOM.creativityLevel.disabled = false;
            }
        } else {
            if (DOM.creativityLevel) {
                DOM.creativityLevel.value = 0.5;
                DOM.creativityLevel.disabled = false;
            }
        }
        if(DOM.creativityLevel && DOM.creativityLevel.nextElementSibling) DOM.creativityLevel.nextElementSibling.textContent = DOM.creativityLevel.value;

        Config.saveSettings(); // Save new creativity level

        if (DOM.moodSelector) {
            DOM.moodSelector.style.display = newVoice === 'owner' ? 'block' : 'none';
        }

        if (newVoice !== 'owner') {
            _closeVoiceSettingsModal();
        }

        // Chat.clearChat();
        // UI.clearError();
        UI.hideLoadingIndicator();
        if (DOM.userInput) DOM.userInput.value = '';
        const voiceDesc = CONSTANTS.VOICE_DESCRIPTIONS[newVoice]
            || (voiceMap[newVoice] && voiceMap[newVoice].description)
            || newVoice;
        Chat.addMessage('assistant', "Voice changed to " + voiceDesc, true, null, null);
    }

    function highlightSelectedVoiceIcon() {
        const selectedVoice = getSelectedVoice();
        
        // Re-query voiceIconWrappers to ensure we have fresh references (in case DOM was updated)
        const voiceIconWrappers = document.querySelectorAll('.voice-icon-wrapper');
        
        voiceIconWrappers.forEach(wrapper => {
            const wrapperVoice = wrapper.dataset.voice;
            const img = wrapper.querySelector('.voice-icon');
            if (img) {
                if(wrapperVoice === selectedVoice) {
                    // Add 'selected' class for highlighting
                    img.classList.add('selected');
                } else {
                    // Remove 'selected' class
                    img.classList.remove('selected');
                }
            }
        });
    }
    
    function setInitialState() {
        const initialVoice = getSelectedVoice();
        updateVoicePreview(initialVoice); // Update preview for initial voice
        updateSelectedVoiceImage(initialVoice); // Update selected voice image
        if (initialVoice === 'expert') {
            if (DOM.creativityLevel) {
                DOM.creativityLevel.value = 0;
                DOM.creativityLevel.disabled = true;
            }
        } else {
            // Ensure creativity is enabled if not expert, using its current value or a default
            if (DOM.creativityLevel) {
                DOM.creativityLevel.disabled = false; 
            }
        }
        if(DOM.creativityLevel && DOM.creativityLevel.nextElementSibling) DOM.creativityLevel.nextElementSibling.textContent = DOM.creativityLevel.value;
        highlightSelectedVoiceIcon();
        updateLoadingIndicatorImage(); // Also set initial loading indicator image
    }

    function setVoice(voiceName) {
        if (!voiceName) return;
        
        if (DOM.voiceSelect) {
            DOM.voiceSelect.value = voiceName;
            // Manually dispatch change event to trigger all handlers
            DOM.voiceSelect.dispatchEvent(new Event('change', { bubbles: true }));
            highlightSelectedVoiceIcon(); // Update highlight
        }
    }

    function init() {
        // Load voices from API first, then set initial state
        loadVoices().then(() => setInitialState());
        // Add event listener for the voice select dropdown
        if (DOM.voiceSelect) {
            DOM.voiceSelect.addEventListener('change', (e) => {
                _handleVoiceChange(e);
                highlightSelectedVoiceIcon(); // Also update highlight on change
            });
        }
        if (DOM.ownerMood) {
            DOM.ownerMood.addEventListener('change', () => {
                if (Chat.updateMoodInMessages) Chat.updateMoodInMessages();
                _closeVoiceSettingsModal();
            });
        }
        DOM.voiceIconWrappers.forEach(wrapper => {
            wrapper.addEventListener('click', () => {
                const voice = wrapper.dataset.voice;
                if (DOM.voiceSelect) {
                    DOM.voiceSelect.value = voice;
                    // Manually dispatch change event to trigger all handlers
                    DOM.voiceSelect.dispatchEvent(new Event('change', { bubbles: true })); 
                }
            });
        });
    }
    return { init, getSelectedVoice, setVoice, updateLoadingIndicatorImage, updateVoicePreview, updateSelectedVoiceImage, loadVoices, getVoiceImage: _getVoiceImage };
})();

