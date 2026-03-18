'use strict';



// --- Global Interviewee Management Functions ---
async function loadInterviewees() {
    try {
        const response = await fetch('/interviewer/interviewees');
        const data = await response.json();
        
        if (data.interviewees) {
            // Clear existing options
            DOM.intervieweeSelect.innerHTML = '';
            
            // Add options for each interviewee
            data.interviewees.forEach(interviewee => {
                const option = document.createElement('option');
                option.value = interviewee;
                option.textContent = interviewee;
                if (interviewee === data.current_interviewee) {
                    option.selected = true;
                }
                DOM.intervieweeSelect.appendChild(option);
            });
        }
    } catch (error) {
        console.error('Error loading interviewees:', error);
        // Use a more generic error display since we're not in interviewer mode yet
        console.error('Failed to load interviewees');
    }
}

async function switchInterviewee(subjectName) {
    try {
        const response = await fetch('/interviewer/switchinterviewee', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ subject_name: subjectName })
        });
        
        const data = await response.json();
        
        if (data.success) {
            // Update the UI to reflect the new interviewee
            await loadInterviewees();
            
            // Update the page title
            document.title = `Interviewer - ${subjectName}`;
            
            // Update the header title
            const interviewerHeaderTitle = document.querySelector('.interviewer-header-left h2');
            if (interviewerHeaderTitle) {
                interviewerHeaderTitle.textContent = `Interview Mode - ${subjectName}`;
            }
            
            // Clear the interviewer chat window
            clearInterviewerChat();
            
            // Fetch the new interview state and update button controls
            try {
                const interviewData = await fetchInterviewState();
                AppState.interviewState = interviewData.interview_state;
                updateInterviewControlButtons();
            } catch (error) {
                console.error('Failed to fetch interview state after switch:', error);
                AppState.interviewState = 'initial';
                updateInterviewControlButtons();
            }
            
            // Add a system message indicating the switch
            addInterviewerMessage('system', `Switched to interviewee: ${subjectName}`, false);
            

        } else {
            console.error(data.error || 'Failed to switch interviewee');
        }
    } catch (error) {
        console.error('Error switching interviewee:', error);
    }
}

async function addNewInterviewee(subjectName) {
    try {
        const response = await fetch('/interviewer/addinterviewee', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ subject_name: subjectName })
        });
        
        const data = await response.json();
        
        if (data.success) {
            // Update the interviewee list
            await loadInterviewees();
            
            // Update the page title
            document.title = `Interviewer - ${subjectName}`;
            
            // Update the header title
            const interviewerHeaderTitle = document.querySelector('.interviewer-header-left h2');
            if (interviewerHeaderTitle) {
                interviewerHeaderTitle.textContent = `Interview Mode - ${subjectName}`;
            }
            
            // Clear the interviewer chat window
            clearInterviewerChat();
            
            // Fetch the new interview state and update button controls
            try {
                const interviewData = await fetchInterviewState();
                AppState.interviewState = interviewData.interview_state;
                updateInterviewControlButtons();
            } catch (error) {
                console.error('Failed to fetch interview state after adding interviewee:', error);
                AppState.interviewState = 'initial';
                updateInterviewControlButtons();
            }
            
            // Add a system message indicating the new interviewee
            addInterviewerMessage('system', `Added new interviewee: ${subjectName}`, false);
            
            
            // Close the modal
            Modals.AddInterviewee.close();
        } else {
            console.error(data.error || 'Failed to add interviewee');
        }
    } catch (error) {
        console.error('Error adding interviewee:', error);
    }
}

function handleIntervieweeSelectChange() {
    const selectedInterviewee = DOM.intervieweeSelect.value;
    if (selectedInterviewee) {
        switchInterviewee(selectedInterviewee);
    }
}

function handleAddIntervieweeClick() {
    Modals.AddInterviewee.open();
}

function handleAddIntervieweeSubmit() {
    const newName = DOM.newIntervieweeName.value.trim();
    if (newName) {
        addNewInterviewee(newName);
    } else {
        console.error('Please enter a valid interviewee name');
    }
}

function handleAddIntervieweeCancel() {
    Modals.AddInterviewee.close();
}

// --- Interviewer Mode Module ---
const InterviewerMode = (() => {
    async function toggleInterviewerMode() {

        AppState.isInterviewerMode = !AppState.isInterviewerMode;
        
        if (AppState.isInterviewerMode) {

            
            // Fetch current interview state from API
            try {
                const interviewData = await fetchInterviewState();
                AppState.interviewState = interviewData.interview_state;
                AppState.interviewSubjectName = interviewData.subject_name;
                
                // Update the interviewer header title with subject name
                const interviewerHeaderTitle = document.querySelector('.interviewer-header-left h2');
                if (interviewerHeaderTitle) {
                    interviewerHeaderTitle.textContent = `Interview Mode - ${interviewData.subject_name}`;
                }
                
                // Update the page title
                document.title = `Interviewer - ${interviewData.subject_name}`;
                

            } catch (error) {
                console.error('Failed to fetch interview state:', error);
                AppState.interviewState = 'initial';
                AppState.interviewSubjectName = 'Dave';
            }
            
            // Switch to interviewer mode
            DOM.chatMain.style.display = 'none';
            DOM.interviewerMain.style.display = 'flex';
            DOM.interviewerModeBtn.classList.add('active');
            document.body.classList.add('interviewer-mode');
            
            // Hide voice selector in interviewer mode
            const voiceSelector = document.querySelector('.voice-selector');
            if (voiceSelector) {
                voiceSelector.style.display = 'none';
            }
            
            // Show interview control section
            const interviewControlSection = document.querySelector('.interview-control-section');
            if (interviewControlSection) {
                interviewControlSection.style.display = 'block';
            }
            
            // Update button controls based on actual interview state
            updateInterviewControlButtons(true);
            
            // Clear any existing chat
            Chat.clearChat();
            
            // Focus on interviewer input
            DOM.interviewerUserInput.focus();
        } else {

            // Switch back to normal mode
            DOM.interviewerMain.style.display = 'none';
            DOM.chatMain.style.display = 'flex';
            DOM.interviewerModeBtn.classList.remove('active');
            document.body.classList.remove('interviewer-mode');
            
            // Reset the page title
            document.title = 'Talk About Dave';
            
            // Show voice selector again
            const voiceSelector = document.querySelector('.voice-selector');
            if (voiceSelector) {
                voiceSelector.style.display = 'block';
            }
            
            // Hide interview control section
            const interviewControlSection = document.querySelector('.interview-control-section');
            if (interviewControlSection) {
                interviewControlSection.style.display = 'none';
            }
            
            // Clear interviewer chat
            DOM.interviewerChatBox.innerHTML = '';
            
            // Reset interview state
            AppState.interviewState = 'initial';
            AppState.interviewSubjectName = 'Dave';
        }
    }

    function addInterviewerMessage(role, text, isMarkdown = true, messageId = null, embeddedJson = null) {
        const messageElement = document.createElement('div');
        messageElement.classList.add('message', role === 'suggestion' ? 'user-message' : `${role}-message`);
        if (role === 'suggestion') messageElement.style.backgroundColor = "#f4f778";
        if (messageId) messageElement.id = messageId;

        const contentElement = document.createElement('div');
        contentElement.classList.add('message-content');
        contentElement.dataset.role = (role === 'suggestion') ? "user" : role;
        if (role === 'suggestion') contentElement.style.backgroundColor = "#f4f778";

        if ((isMarkdown && role !== 'user') || role === 'suggestion') {
            const rawMarkdown = document.createElement('textarea');
            rawMarkdown.className = 'raw-markdown';
            rawMarkdown.style.display = 'none';
            rawMarkdown.value = text;
            contentElement.appendChild(rawMarkdown);
            Chat.renderMarkdown(contentElement, text);
        } else {
            contentElement.textContent = text;
        }

        // Add copy button
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

        messageElement.appendChild(contentElement);
        DOM.interviewerChatBox.appendChild(messageElement);
        
        // Scroll to bottom
        setTimeout(() => { 
            DOM.interviewerChatBox.scrollTop = DOM.interviewerChatBox.scrollHeight; 
        }, 50);
        
        return messageElement;
    }

    function clearInterviewerChat() {
        DOM.interviewerChatBox.innerHTML = '';
        // Re-add info box
        const infoBox = document.getElementById('interviewer-info-box');
        if (infoBox) {
            DOM.interviewerChatBox.appendChild(infoBox);
        }
    }

    function setInterviewerControlsEnabled(enabled) {
        DOM.interviewerUserInput.disabled = !enabled;
        DOM.interviewerSendButton.disabled = !enabled;
    }

    function showInterviewerLoadingIndicator() {
        DOM.interviewerLoadingIndicator.style.display = 'flex';
    }

    function hideInterviewerLoadingIndicator() {
        DOM.interviewerLoadingIndicator.style.display = 'none';
    }

    function displayInterviewerError(message) {
        console.error("Interviewer Error displayed:", message);
        DOM.interviewerErrorDisplay.textContent = `Error: ${message}`;
        DOM.interviewerErrorDisplay.style.display = 'block';
        DOM.interviewerLoadingIndicator.style.display = 'none';
    }

    function clearInterviewerError() {
        DOM.interviewerErrorDisplay.textContent = '';
        DOM.interviewerErrorDisplay.style.display = 'none';
    }

    async function fetchInterviewState() {
        try {
            const response = await fetch('/interviewer/interviewstate');
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            return data;
        } catch (error) {
            console.error('Error fetching interview state:', error);
            // Return default state if API call fails
            return {
                interview_state: 'initial',
                subject_name: 'Dave',
                session_turns_count: 0,
                session_tool_outputs_count: 0,
                uploaded_files_count: 0
            };
        }
    }

    function updateInterviewControlButtons(just_switched_to_interviewer_mode = false) {
        // Disable all buttons first
        DOM.startInterviewBtn.disabled = true;
        DOM.resumeInterviewBtn.disabled = true;
        DOM.pauseInterviewBtn.disabled = true;
        DOM.writeInterimBioBtn.disabled = true;
        DOM.finishInterviewBtn.disabled = true;
        DOM.writeFinalBioBtn.disabled = true;
        DOM.resetInterviewBtn.disabled = true;

        // Enable buttons based on current interview state
        switch (AppState.interviewState) {
            case 'initial':
                // Only enable Start Interview when interview state is initial
                DOM.startInterviewBtn.disabled = false;
                break;
            case 'active':
                // Enable Pause Interview when interview is active
                DOM.pauseInterviewBtn.disabled = false;
                if (just_switched_to_interviewer_mode) {
                    DOM.resumeInterviewBtn.disabled = false;
                }
                DOM.resetInterviewBtn.disabled = false;
                break;
            case 'paused':
                // Enable Resume Interview, Write Interim Biography, and Finish Interview when paused
                DOM.resumeInterviewBtn.disabled = false;
                DOM.writeInterimBioBtn.disabled = false;
                DOM.finishInterviewBtn.disabled = false;
                DOM.resetInterviewBtn.disabled = false;
                break;
            case 'finished':
                // Enable Write Final Biography, Resume Interview, and Start Interview when interview is finished
                DOM.writeFinalBioBtn.disabled = false;
                DOM.resumeInterviewBtn.disabled = false;
                DOM.startInterviewBtn.disabled = false;
                DOM.resetInterviewBtn.disabled = false;
                break;
        }
    }

    // Interviewer dictation functions
    function startInterviewerDictation() {
        if (!AppState.interviewerDictationRecognition) return;
        if (AppState.isInterviewerDictationListening) {
            stopInterviewerDictation();
            return;
        }
        AppState.finalInterviewerDictationTranscript = '';
        DOM.interviewerUserInput.value = '';
        AppState.interviewerDictationRecognition.start();
        DOM.interviewerStartDictationBtn.style.display = 'none';
        DOM.interviewerStopDictationBtn.style.display = 'block';
    }

    function stopInterviewerDictation() {
        if (!AppState.isInterviewerDictationListening) {
            return;
        }
        
        if (AppState.interviewerDictationRecognition) {
            AppState.interviewerDictationRecognition.stop();
        }
        DOM.interviewerStopDictationBtn.style.display = 'none';
        DOM.interviewerStartDictationBtn.style.display = 'block';
        AppState.isInterviewerDictationListening = false;
        
        // Auto-submit the entered text if there's content
        const userInput = DOM.interviewerUserInput.value.trim();
        if (userInput) {
            processInterviewerFormSubmit(userInput);
        }
    }

    function initInterviewerSpeechRecognition() {
        const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
        if (SpeechRecognition) {
            AppState.interviewerDictationRecognition = new SpeechRecognition();
            AppState.interviewerDictationRecognition.lang = 'en-US';
            AppState.interviewerDictationRecognition.interimResults = true;
            AppState.interviewerDictationRecognition.continuous = true;

            AppState.interviewerDictationRecognition.onstart = () => { 
                AppState.isInterviewerDictationListening = true; 
            };
            AppState.interviewerDictationRecognition.onerror = (event) => { 
                DOM.interviewerDictationStatus.textContent = 'Dictation error: ' + event.error; 
                AppState.isInterviewerDictationListening = false; 
                stopInterviewerDictation(); 
            };
            AppState.interviewerDictationRecognition.onend = () => { 
                AppState.isInterviewerDictationListening = false; 
                stopInterviewerDictation(); 
            };
            AppState.interviewerDictationRecognition.onresult = (event) => {
                let interimTranscript = '';
                for (let i = event.resultIndex; i < event.results.length; ++i) {
                    if (event.results[i].isFinal) {
                        AppState.finalInterviewerDictationTranscript += event.results[i][0].transcript;
                    } else {
                        interimTranscript += event.results[i][0].transcript;
                    }
                }
                DOM.interviewerUserInput.value = AppState.finalInterviewerDictationTranscript + interimTranscript;
            };
        } else {
            if (DOM.interviewerStartDictationBtn) {
                DOM.interviewerStartDictationBtn.disabled = true;
                DOM.interviewerDictationStatus.textContent = 'Speech recognition not supported.';
            }
        }
    }

    // ==================== INTERVIEWEE MANAGEMENT ====================
    
    // These functions are now in global scope above

    async function processInterviewerFormSubmit(userPrompt, category = null, title = null, supplementary_prompt = null) {
        if (!userPrompt && !category && !title) return;

        clearInterviewerError();
        setInterviewerControlsEnabled(false);
        showInterviewerLoadingIndicator();

        const selectedVoice = VoiceSelector.getSelectedVoice();
        const selectedMood = (selectedVoice === 'owner' && DOM.ownerMood) ? DOM.ownerMood.value : null;
        
        // Get selected interview purpose
        const selectedPurpose = DOM.interviewPurposeSelect ? DOM.interviewPurposeSelect.value : '';
        let purposeContext = '';
        
        if (selectedPurpose && !userPrompt.includes('biography')) {
            const purposeMap = {
                'biography': 'biography writing',
                'research': 'academic research',
                'journalism': 'journalism/article writing',
                'therapy': 'therapeutic session',
                'oral-history': 'oral history documentation',
                'memoir': 'memoir writing',
                'documentary': 'documentary film',
                'podcast': 'podcast interview',
                'book': 'book research',
                'personal': 'personal interest',
                'family': 'family history',
                'professional': 'professional development',
                'creative': 'creative writing',
                'other': 'the selected purpose'
            };
            purposeContext = ` (Context: This interview is for ${purposeMap[selectedPurpose] || 'the selected purpose'})`;
        }
        
        try {
            const finalMessage = UI.getWorkModePrefix() + userPrompt + purposeContext;

            if (category && title) {
                addInterviewerMessage('suggestion', `**${category}:** ${title}`, true);
            } else {
                addInterviewerMessage('user', userPrompt, false);
            }
            
            DOM.interviewerUserInput.value = '';

            const response = await ApiService.fetchChat({
                prompt: finalMessage,
                voice: selectedVoice,
                mood: selectedMood,
                interviewMode: true, // Always use interview mode for interviewer
                companionMode: false,
                supplementary_prompt: supplementary_prompt,
                temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
                clientId: AppState.clientId
            });
            
            const data = await response.json();
            hideInterviewerLoadingIndicator();
            if (data.error) displayInterviewerError(data.error);
            else addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);

        } catch (error) {
            console.error('Interviewer form submit error:', error);
            displayInterviewerError(error.message || 'An unknown error occurred.');
        } finally {
            setInterviewerControlsEnabled(true);
            hideInterviewerLoadingIndicator();
        }
    }

    function init() {
        // Initialize interviewer speech recognition
        initInterviewerSpeechRecognition();
        
        // Add event listeners for interviewer mode
        if (DOM.interviewerModeBtn) {

            DOM.interviewerModeBtn.addEventListener('click', async () => {
                await toggleInterviewerMode();
            });
            
            // Add a simple test click handler

        } else {
            console.error('Interviewer mode button not found!');
        }
        
        if (DOM.interviewerChatForm) {
            DOM.interviewerChatForm.addEventListener('submit', (event) => {
                event.preventDefault();
                const userPrompt = DOM.interviewerUserInput.value.trim();
                if (!userPrompt) return;
                processInterviewerFormSubmit(userPrompt);
            });
        }
        
        if (DOM.interviewerUserInput) {
            DOM.interviewerUserInput.addEventListener('keydown', (event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault();
                    DOM.interviewerChatForm.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }));
                }
            });
        }
        
        if (DOM.interviewerStartDictationBtn) {
            DOM.interviewerStartDictationBtn.addEventListener('click', startInterviewerDictation);
        }
        
        if (DOM.interviewerStopDictationBtn) {
            DOM.interviewerStopDictationBtn.addEventListener('click', stopInterviewerDictation);
        }
        
        if (DOM.exitInterviewerModeBtn) {
            DOM.exitInterviewerModeBtn.addEventListener('click', async () => {
                await toggleInterviewerMode();
            });
        }
        
        // Add event listeners for interview control buttons
        if (DOM.startInterviewBtn) {
            DOM.startInterviewBtn.addEventListener('click', handleStartInterview);
        }
        
        if (DOM.resumeInterviewBtn) {
            DOM.resumeInterviewBtn.addEventListener('click', handleResumeInterview);
        }
        
        if (DOM.pauseInterviewBtn) {
            DOM.pauseInterviewBtn.addEventListener('click', handlePauseInterview);
        }
        
        if (DOM.writeInterimBioBtn) {
            DOM.writeInterimBioBtn.addEventListener('click', handleWriteInterimBio);
        }
        
        if (DOM.finishInterviewBtn) {
            DOM.finishInterviewBtn.addEventListener('click', handleFinishInterview);
        }
        
        if (DOM.writeFinalBioBtn) {
            DOM.writeFinalBioBtn.addEventListener('click', handleWriteFinalBio);
        }
        
        if (DOM.resetInterviewBtn) {
            DOM.resetInterviewBtn.addEventListener('click', handleResetInterview);
        }
        
        // Add event listener for interview purpose selector
        if (DOM.interviewPurposeSelect) {
            DOM.interviewPurposeSelect.addEventListener('change', handleInterviewPurposeChange);
        }

        // Initialize interviewee list
        loadInterviewees();
    }

    // Interview control button handlers
    async function handleStartInterview() {

        
        try {
            // Check if there's existing interview data
            const checkResponse = await ApiService.checkInterviewData();
            const checkData = await checkResponse.json();
            
            if (checkData.has_data) {
                // Show confirmation popup
                Modals.ConfirmationModal.open(
                    'Start New Interview', 
                    `There is existing interview data (${checkData.turn_count} conversation turns). Starting a new interview will clear all existing data. Are you sure you want to continue?`,
                    async () => {
                        // User confirmed - proceed with starting new interview
                        await _performStartInterview();
                    }
                );
            } else {
                // No existing data - proceed directly
                await _performStartInterview();
            }
        } catch (error) {
            console.error('Error checking interview data:', error);
            // If check fails, proceed anyway
            await _performStartInterview();
        }
    }

    async function _performStartInterview() {
        addInterviewerMessage('system', 'Starting new interview session. First thing I\'ll do is check your biography and decide how to proceed.', false);
        clearInterviewerError();
        setInterviewerControlsEnabled(false);
        showInterviewerLoadingIndicator();

        const response = await ApiService.startInterview();
        const data = await response.json();
        hideInterviewerLoadingIndicator();
        if (data.error) {
            displayInterviewerError(data.error);
        } else {
            addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);
            // Update interview state to 'active' and enable appropriate buttons
            AppState.interviewState = 'active';
            updateInterviewControlButtons();
            setInterviewerControlsEnabled(true);
        }
    }

    async function handleResumeInterview() {

        addInterviewerMessage('system', 'Interview resumed.', false);
        clearInterviewerError();
        setInterviewerControlsEnabled(false);
        showInterviewerLoadingIndicator();

        const response = await ApiService.resumeInterview();
        const data = await response.json();
        hideInterviewerLoadingIndicator();
        if (data.error) {
            displayInterviewerError(data.error);
        } else {
            addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);
            // Update interview state to 'active' and enable appropriate buttons
            AppState.interviewState = 'active';
            updateInterviewControlButtons();
            setInterviewerControlsEnabled(true);
        }
    }

    async function handlePauseInterview() {
  
        addInterviewerMessage('system', 'Interview paused. You can resume it later.', false);
        showInterviewerLoadingIndicator();
        const response = await ApiService.pauseInterview();
        const data = await response.json();
        hideInterviewerLoadingIndicator();
        if (data.error) {
            displayInterviewerError(data.error);
        } else {
            addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);
            // Update interview state to 'paused' and enable appropriate buttons
            AppState.interviewState = 'paused';
            updateInterviewControlButtons();
            setInterviewerControlsEnabled(false);
        }
    }

    async function handleWriteInterimBio() {

        showInterviewerLoadingIndicator();
        const response = await ApiService.writeInterimBio();
        const data = await response.json();
        hideInterviewerLoadingIndicator();
        if (data.error) {
            displayInterviewerError(data.error);
        } else {
            addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);
        }
    }

    function handleFinishInterview() {

        addInterviewerMessage('system', 'Interview finished. You can now write the final biography or exit interview mode.', false);
        // Update interview state to 'finished' and enable appropriate buttons
        AppState.interviewState = 'finished';
        updateInterviewControlButtons();
        // Disable input controls
        setInterviewerControlsEnabled(false);
    }

    async function handleWriteFinalBio() {
 
        showInterviewerLoadingIndicator();
        const response = await ApiService.writeFinalBio();
        const data = await response.json();
        hideInterviewerLoadingIndicator();
        if (data.error) {
            displayInterviewerError(data.error);
        } else {
            addInterviewerMessage('assistant', data.text, true, null, data.embedded_json);
        }
    }

    async function handleResetInterview() {
   
        
        // Show confirmation dialog
        Modals.ConfirmationModal.open(
            'Reset Interview', 
            'This will permanently delete all interview data including conversation history, biography brief, and vector embeddings. This action cannot be undone. Are you sure you want to reset the interview?',
            async () => {
                // User confirmed - proceed with reset
                addInterviewerMessage('system', 'Resetting interview...', false);
                clearInterviewerError();
                setInterviewerControlsEnabled(false);
                showInterviewerLoadingIndicator();

                try {
                    const response = await ApiService.resetInterview();
                    const data = await response.json();
                    hideInterviewerLoadingIndicator();
                    
                    if (data.error) {
                        displayInterviewerError(data.error);
                        setInterviewerControlsEnabled(true);
                    } else {
                        // Clear the interviewer chat
                        clearInterviewerChat();
                        
                        // Update interview state to 'initial'
                        AppState.interviewState = 'initial';
                        updateInterviewControlButtons();
                        
                        // Show success message
                        addInterviewerMessage('system', 'Interview reset successfully. All data has been cleared. You can now start a new interview.', false);
                        setInterviewerControlsEnabled(true);
                    }
                } catch (error) {
                    console.error('Error resetting interview:', error);
                    hideInterviewerLoadingIndicator();
                    displayInterviewerError('Failed to reset interview. Please try again.');
                    setInterviewerControlsEnabled(true);
                }
            }
        );
    }

    function handleInterviewPurposeChange() {
        const selectedPurpose = DOM.interviewPurposeSelect.value;

        
        if (selectedPurpose) {
            // Add a system message to indicate the interview purpose
            addInterviewerMessage('system', `Interview purpose set to: ${DOM.interviewPurposeSelect.options[DOM.interviewPurposeSelect.selectedIndex].text}`, false);
        }
    }

    return { 
        init, 
        toggleInterviewerMode, 
        addInterviewerMessage, 
        clearInterviewerChat, 
        processInterviewerFormSubmit,
        setInterviewerControlsEnabled,
        showInterviewerLoadingIndicator,
        hideInterviewerLoadingIndicator,
        displayInterviewerError,
        clearInterviewerError,
        updateInterviewControlButtons
    };
})();
