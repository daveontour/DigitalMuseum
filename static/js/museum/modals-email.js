'use strict';

/** GET /emails/search returns a JSON array on success, or { detail: string } on error. */
function coerceEmailSearchResponse(data) {
    if (Array.isArray(data)) return data;
    if (data && typeof data.detail === 'string') {
        console.warn('emails/search:', data.detail);
        return [];
    }
    if (data != null && typeof data === 'object') {
        console.warn('emails/search: expected array, got object with keys:', Object.keys(data));
    } else {
        console.warn('emails/search: expected array, got', typeof data);
    }
    return [];
}

async function parseEmailSearchFetchResponse(response) {
    const data = await response.json();
    if (!response.ok) {
        const detail = data && data.detail ? data.detail : response.statusText;
        throw new Error(detail);
    }
    return data;
}

// Modals.Suggestions = (() => {
//         function executeSuggestion(sugg, cat) {
//             Modals._closeModal(DOM.suggestionsModal);
//             if (sugg.function && AppActions[sugg.function]) {
//                 AppActions[sugg.function]();
//             } else {
//                 App.processFormSubmit(sugg.prompt, cat.category, sugg.title);
//             }
//         }

//         function toggleCategory(headerRow) {
//             const category = headerRow.dataset.category;
//             const expanded = headerRow.dataset.expanded !== 'false';
//             const icon = headerRow.querySelector('.suggestions-category-toggle');
//             const rows = DOM.suggestionsListContainer.querySelectorAll(`tr.suggestions-table-row[data-category="${CSS.escape(category)}"]`);

//             if (expanded) {
//                 rows.forEach(r => { r.classList.add('suggestions-row-collapsed'); });
//                 headerRow.dataset.expanded = 'false';
//                 if (icon) {
//                     icon.classList.remove('fa-chevron-down');
//                     icon.classList.add('fa-chevron-right');
//                 }
//             } else {
//                 rows.forEach(r => { r.classList.remove('suggestions-row-collapsed'); });
//                 headerRow.dataset.expanded = 'true';
//                 if (icon) {
//                     icon.classList.remove('fa-chevron-right');
//                     icon.classList.add('fa-chevron-down');
//                 }
//             }
//         }

//         function open() {
//             Modals._openModal(DOM.suggestionsModal);
//             DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">Loading...</td></tr>';
//             ApiService.fetchSuggestionsConfig()
//                 .then(data => {
//                     DOM.suggestionsListContainer.innerHTML = '';
//                     if (data.categories && Array.isArray(data.categories)) {
//                         data.categories.forEach(cat => {
//                             if (Array.isArray(cat.suggestions)) {
//                                 const categoryHeader = document.createElement('tr');
//                                 categoryHeader.className = 'suggestions-category-header';
//                                 categoryHeader.dataset.category = cat.category;
//                                 categoryHeader.dataset.expanded = 'false';
//                                 categoryHeader.innerHTML = `
//                                     <td colspan="3" class="suggestions-category-header-cell">
//                                         <i class="fas fa-chevron-right suggestions-category-toggle"></i>
//                                         <span class="suggestions-category-name">${cat.category}</span>
//                                         <span class="suggestions-category-count">(${cat.suggestions.length})</span>
//                                     </td>
//                                 `;
//                                 categoryHeader.addEventListener('click', () => toggleCategory(categoryHeader));
//                                 DOM.suggestionsListContainer.appendChild(categoryHeader);

//                                 cat.suggestions.forEach(sugg => {
//                                     const tr = document.createElement('tr');
//                                     tr.className = 'suggestions-table-row suggestions-row-collapsed';
//                                     tr.dataset.category = cat.category;

//                                     const titleCell = document.createElement('td');
//                                     titleCell.className = 'suggestions-col-title';
//                                     titleCell.colSpan = 2;
//                                     titleCell.textContent = sugg.title;

//                                     const actionCell = document.createElement('td');
//                                     actionCell.className = 'suggestions-col-action';
//                                     const btn = document.createElement('button');
//                                     btn.type = 'button';
//                                     btn.className = 'modal-btn modal-btn-primary suggestions-execute-btn';
//                                     btn.textContent = 'Execute';
//                                     btn.addEventListener('click', (e) => {
//                                         e.stopPropagation();
//                                         executeSuggestion(sugg, cat);
//                                     });

//                                     actionCell.appendChild(btn);
//                                     tr.appendChild(titleCell);
//                                     tr.appendChild(actionCell);
//                                     DOM.suggestionsListContainer.appendChild(tr);
//                                 });
//                             }
//                         });
//                         if (DOM.suggestionsListContainer.children.length === 0) {
//                             DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">No suggestions found.</td></tr>';
//                         }
//                     } else {
//                         DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">No suggestions found.</td></tr>';
//                     }
//                 })
//                 .catch(err => {
//                     console.error("Failed to load suggestions:", err);
//                     DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #c00;">Failed to load suggestions.</td></tr>';
//                     UI.displayError("Could not load suggestions: " + err.message);
//                 });
//         }

//         function close() {
//             Modals._closeModal(DOM.suggestionsModal);
//         }

//         function init() {
//             DOM.closeSuggestionsModalBtn.addEventListener('click', close);
//             DOM.suggestionsModal.addEventListener('click', (e) => { if (e.target === DOM.suggestionsModal) close(); });
//         }

//         return { init, open, close };
// })();


Modals.EmailGallery = (() => {
        let emailData = [];
        let selectedEmailIndex = -1;
        let currentEmailId = null;
        let currentPage = 0;
        let itemsPerPage = 20;
        let isLoading = false;
        let hasMoreData = true;
        let searchTimeout = null;
        let viewMode = 'list'; // 'list' or 'grid'

        function formatDateAustralian(dateString) {
          
            if (!dateString) return 'No Date';
            try {
                const date = new Date(dateString);
                if (isNaN(date.getTime())) return 'Invalid Date';
                
                const day = String(date.getDate()).padStart(2, '0');
                const month = String(date.getMonth() + 1).padStart(2, '0');
                const year = date.getFullYear();
                const hours = String(date.getHours()).padStart(2, '0');
                const minutes = String(date.getMinutes()).padStart(2, '0');
                
                return `${day}/${month}/${year} ${hours}:${minutes}`;
            } catch (error) {
                return 'Invalid Date';
            }
        }

        function init() {
            DOM.closeEmailGalleryModalBtn.addEventListener('click', close);
            DOM.emailGallerySearchBtn.addEventListener('click', _handleSearch);
            DOM.emailGalleryClearBtn.addEventListener('click', _handleClear);
            
            // Delete button handler
            if (DOM.emailDeleteBtn) {
                DOM.emailDeleteBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    deleteEmail();
                });
            }

            // Ask AI button and modal handlers
            const emailAskAIBtn = DOM.emailAskAIBtn;
            const emailAskAIModal = DOM.emailAskAIModal;
            const emailAskAICloseBtn = document.getElementById('email-ask-ai-close');
            const emailAskAICancelBtn = document.getElementById('email-ask-ai-cancel');
            const emailAskAISubmitBtn = document.getElementById('email-ask-ai-submit');
            const emailAskAIRadioButtons = document.querySelectorAll('input[name="email-ask-ai-option"]');
            const emailAskAIOtherTextarea = document.getElementById('email-ask-ai-other-text');
            const emailAskAIOtherInput = document.getElementById('email-ask-ai-other-input');

            if (emailAskAIBtn && emailAskAIModal) {
                emailAskAIBtn.addEventListener('click', () => {
                    // Update email subject in modal
                    const emailSubjectEl = document.getElementById('email-ask-ai-email-subject');
                    const emailSubject = DOM.emailGalleryMetadataSubject?.textContent || 'Unknown Email';
                    if (emailSubjectEl) {
                        emailSubjectEl.textContent = emailSubject;
                    }
                    emailAskAIModal.style.display = 'flex';
                });
            }

            if (emailAskAICloseBtn && emailAskAIModal) {
                emailAskAICloseBtn.addEventListener('click', () => {
                    emailAskAIModal.style.display = 'none';
                });
            }

            if (emailAskAICancelBtn && emailAskAIModal) {
                emailAskAICancelBtn.addEventListener('click', () => {
                    emailAskAIModal.style.display = 'none';
                });
            }

            if (emailAskAISubmitBtn) {
                emailAskAISubmitBtn.addEventListener('click', () => {
                    // Functionality will be added later
                    const selectedOption = document.querySelector('input[name="email-ask-ai-option"]:checked')?.value;
                    const otherText = emailAskAIOtherInput?.value || '';


                    if (selectedOption === 'summarise') {
                        if (!emailData[selectedEmailIndex].sender) {
                            alert('No conversation selected');
                            return;
                        }

                        try {
                            // Open the conversation summary modal
                            // currentSession is already the chat session name (string)
                            Modals.ConversationSummary.openForEmailThread(emailData[selectedEmailIndex].sender);
                        } catch (error) {
                            console.error('Error opening conversation summary:', error);
                            alert('Failed to start conversation summarization. Please try again.');
                        }
                    }


                    // TODO: Implement AI functionality
                    emailAskAIModal.style.display = 'none';
                });
            }

            // Toggle textarea visibility based on radio selection
            if (emailAskAIRadioButtons.length > 0 && emailAskAIOtherTextarea) {
                emailAskAIRadioButtons.forEach(radio => {
                    radio.addEventListener('change', () => {
                        if (radio.value === 'other') {
                            emailAskAIOtherTextarea.style.display = 'block';
                        } else {
                            emailAskAIOtherTextarea.style.display = 'none';
                        }
                    });
                });
            }

            // Close modal when clicking outside
            if (emailAskAIModal) {
                emailAskAIModal.addEventListener('click', (e) => {
                    if (e.target === emailAskAIModal) {
                        emailAskAIModal.style.display = 'none';
                    }
                });
            }
            
            // Attachment modal close handlers
            if (DOM.closeEmailAttachmentImageModal && DOM.emailAttachmentImageModal) {
                DOM.closeEmailAttachmentImageModal.addEventListener('click', () => {
                    DOM.emailAttachmentImageModal.style.display = 'none';
                });
                DOM.emailAttachmentImageModal.addEventListener('click', (e) => {
                    if (e.target === DOM.emailAttachmentImageModal) {
                        DOM.emailAttachmentImageModal.style.display = 'none';
                    }
                });
            }
            if (DOM.closeEmailAttachmentDocumentModal && DOM.emailAttachmentDocumentModal) {
                DOM.closeEmailAttachmentDocumentModal.addEventListener('click', () => {
                    DOM.emailAttachmentDocumentModal.style.display = 'none';
                });
                DOM.emailAttachmentDocumentModal.addEventListener('click', (e) => {
                    if (e.target === DOM.emailAttachmentDocumentModal) {
                        DOM.emailAttachmentDocumentModal.style.display = 'none';
                    }
                });
            }
            
            // Add event listeners for filter changes with debouncing
            DOM.emailGallerySearch.addEventListener('input', (e) => {
                // Clear any existing timeout
                if (searchTimeout) clearTimeout(searchTimeout);
                
                const query = e.target.value.trim();
                
                // Only search if query is at least 3 characters or empty
                if (query.length >= 3 || query.length === 0) {
                    // Set a new timeout to debounce the search
                    searchTimeout = setTimeout(() => {
                        _handleSearch();
                    }, 300); // 300ms debounce
                }
            });
            
            DOM.emailGallerySender.addEventListener('input', (e) => {
                if (searchTimeout) clearTimeout(searchTimeout);
                searchTimeout = setTimeout(() => {
                    _handleSearch();
                }, 300);
            });
            
            DOM.emailGalleryRecipient.addEventListener('input', (e) => {
                if (searchTimeout) clearTimeout(searchTimeout);
                searchTimeout = setTimeout(() => {
                    _handleSearch();
                }, 300);
            });
            
            DOM.emailGalleryYearFilter.addEventListener('change', _handleSearch);
            DOM.emailGalleryMonthFilter.addEventListener('change', _handleSearch);
            DOM.emailGalleryAttachmentsFilter.addEventListener('change', _handleAttachmentsFilter);
           // DOM.emailGalleryFolderFilter.addEventListener('change', _handleSearch);

            // Add scroll event listener for lazy loading
            DOM.emailGalleryList.addEventListener('scroll', _handleEmailListScroll);

            // Add keyboard navigation
            document.addEventListener('keydown', _handleKeydown);
            
            // View toggle handler
            if (DOM.emailGalleryViewToggle) {
                DOM.emailGalleryViewToggle.addEventListener('click', _toggleViewMode);
            }
            
            // Initialize resizable panes
            _initResizablePanes();
            
            // Initialize view mode
            if (DOM.emailGalleryList) {
                DOM.emailGalleryList.classList.add('email-gallery-list-view');
            }
        }
        
        function _toggleViewMode() {
            viewMode = viewMode === 'list' ? 'grid' : 'list';
            const listContainer = DOM.emailGalleryList;
            if (listContainer) {
                listContainer.classList.toggle('email-gallery-grid-view', viewMode === 'grid');
                listContainer.classList.toggle('email-gallery-list-view', viewMode === 'list');
            }
            if (DOM.emailGalleryViewToggle) {
                const icon = DOM.emailGalleryViewToggle.querySelector('i');
                if (icon) {
                    icon.className = viewMode === 'grid' ? 'fas fa-list' : 'fas fa-th';
                }
            }
            // Re-render the list
            _renderEmailList();
        }
        
        function _initResizablePanes() {
            if (!DOM.emailGalleryDivider || !DOM.emailGalleryMasterPane || !DOM.emailGalleryDetailPane) {
                return;
            }
            
            // Load saved divider position from localStorage
            const savedPosition = localStorage.getItem('emailGalleryDividerPosition');
            const defaultPosition = 35; // 35% for master pane
            const masterPaneWidth = savedPosition ? parseFloat(savedPosition) : defaultPosition;
            
            _setPaneWidths(masterPaneWidth);
            
            let isResizing = false;
            let startX = 0;
            let startMasterWidth = 0;
            
            DOM.emailGalleryDivider.addEventListener('mousedown', (e) => {
                isResizing = true;
                startX = e.clientX;
                startMasterWidth = parseFloat(getComputedStyle(DOM.emailGalleryMasterPane).width);
                document.body.style.cursor = 'col-resize';
                document.body.style.userSelect = 'none';
                e.preventDefault();
            });
            
            document.addEventListener('mousemove', (e) => {
                if (!isResizing) return;
                
                const deltaX = e.clientX - startX;
                const modalWidth = DOM.emailGalleryModal.offsetWidth;
                const newMasterWidth = ((startMasterWidth + deltaX) / modalWidth) * 100;
                
                // Constrain between min and max
                const minWidth = 20; // 20% minimum
                const maxWidth = 70; // 70% maximum
                const constrainedWidth = Math.max(minWidth, Math.min(maxWidth, newMasterWidth));
                
                _setPaneWidths(constrainedWidth);
            });
            
            document.addEventListener('mouseup', () => {
                if (isResizing) {
                    isResizing = false;
                    document.body.style.cursor = '';
                    document.body.style.userSelect = '';
                    
                    // Save position to localStorage
                    const currentWidth = parseFloat(getComputedStyle(DOM.emailGalleryMasterPane).width);
                    const modalWidth = DOM.emailGalleryModal.offsetWidth;
                    const percentage = (currentWidth / modalWidth) * 100;
                    localStorage.setItem('emailGalleryDividerPosition', percentage.toString());
                }
            });
        }
        
        function _setPaneWidths(masterPanePercentage) {
            if (!DOM.emailGalleryMasterPane || !DOM.emailGalleryDetailPane) {
                return;
            }
            
            DOM.emailGalleryMasterPane.style.width = `${masterPanePercentage}%`;
            DOM.emailGalleryDetailPane.style.width = `${100 - masterPanePercentage}%`;
        }

        async function open() {
            DOM.emailGalleryModal.style.display = 'flex';
            _loadEmailData().catch(error => {
                console.error('Error loading email data in open():', error);
            });
        }

        function close() {
            DOM.emailGalleryModal.style.display = 'none';
            selectedEmailIndex = -1;
            _clearEmailContent();
        }

        function _loadEmailData() {
            return new Promise((resolve, reject) => {
                _setupFilters();

                //month equals the current month
                // const currentMonth = new Date().getMonth() + 1;
                // DOM.emailGalleryMonthFilter.value = currentMonth;
                DOM.emailGalleryMonthFilter.value = 0;
                const currentMonth = 0;
                const currentYear = new Date().getFullYear();
                DOM.emailGalleryYearFilter.value = currentYear;

                try {
                    const params = new URLSearchParams();
                    params.append('year', currentYear);
                    // API rejects month=0 ("All months"); omit month to search the whole year.
                    if (currentMonth >= 1 && currentMonth <= 12) {
                        params.append('month', String(currentMonth));
                    }

                    fetch('/emails/search?' + params.toString())
                    .then(r => parseEmailSearchFetchResponse(r))
                    .then(data => {
                        const rows = coerceEmailSearchResponse(data);
                        emailData = rows.map(email => ({
                            id: email.id,
                            subject: email.subject || 'No Subject',
                            sender: email.from_address || 'Unknown Sender',
                            recipient: email.to_addresses || 'Unknown Recipient',
                            date: email.date ? formatDateAustralian(email.date) : 'No Date',
                            folder: email.folder || 'Unknown Folder',
                            body: email.snippet || 'No content',
                            preview: email.snippet || 'No preview',
                            attachments: (email.attachment_ids || []).map(id => `/attachments/${id}`),
                            emailId: email.id // Store email ID for fetching full content
                        }));
                        _renderEmailList();
                        _showInstructions();
                        resolve(emailData);
                    })
                    .catch(error => {
                        console.error('Error in fetch:', error);
                        emailData = [];
                        reject(error);
                    });

                } catch (error) {
                    console.error('Error loading email data:', error);
                    emailData = [];
                    reject(error);
                }
            });
        }

        function _setupFilters() {
            // Setup year filter
           // const years = [...new Set(emailData.map(email => email.year))].sort((a, b) => b - a);

            const years = [
                { value: 0, text: 'All Years' },
                { value: 2032, text: '2032' },
                { value: 2031, text: '2031' },
                { value: 2030, text: '2030' },
                { value: 2029, text: '2029' },
                { value: 2028, text: '2028' },
                { value: 2027, text: '2027' },
                { value: 2026, text: '2026' },
                { value: 2025, text: '2025' },
                { value: 2024, text: '2024' },
                { value: 2023, text: '2023' },
                { value: 2022, text: '2022' },
                { value: 2021, text: '2021' },
                { value: 2020, text: '2020' },  
                { value: 2019, text: '2019' },
                { value: 2018, text: '2018' },
                { value: 2017, text: '2017' },
                { value: 2016, text: '2016' },
                { value: 2015, text: '2015' },
                { value: 2014, text: '2014' },
                { value: 2013, text: '2013' },  
                { value: 2012, text: '2012' },
                { value: 2011, text: '2011' },
                { value: 2010, text: '2010' },
                { value: 2009, text: '2009' },
                { value: 2008, text: '2008' },
                { value: 2007, text: '2007' },
                { value: 2006, text: '2006' },
                { value: 2005, text: '2005' },
                { value: 2004, text: '2004' },
                { value: 2003, text: '2003' },
                { value: 2002, text: '2002' },
                { value: 2001, text: '2001' },
                { value: 2000, text: '2000' },
                { value: 1999, text: '1999' },
                { value: 1998, text: '1998' },
                { value: 1997, text: '1997' },
                { value: 1996, text: '1996' },
                { value: 1995, text: '1995' },
                { value: 1994, text: '1994' },
                { value: 1993, text: '1993' },
                { value: 1992, text: '1992' }
            ]
            //DOM.emailGalleryYearFilter.innerHTML = '<option value="0" selected>All Years</option>';
            years.forEach(year => {
                const option = document.createElement('option');
                option.value = year.value;
                option.textContent = year.text;
                DOM.emailGalleryYearFilter.appendChild(option);
            });

            // Setup month filter
            const months = [
                { value: 0, text: 'All Months' },
                { value: 1, text: 'January' },
                { value: 2, text: 'February' },
                { value: 3, text: 'March' },
                { value: 4, text: 'April' },
                { value: 5, text: 'May' },
                { value: 6, text: 'June' },
                { value: 7, text: 'July' },
                { value: 8, text: 'August' },
                { value: 9, text: 'September' },
                { value: 10, text: 'October' },
                { value: 11, text: 'November' },
                { value: 12, text: 'December' }
            ];
            DOM.emailGalleryMonthFilter.innerHTML = '';
            months.forEach(month => {
                const option = document.createElement('option');
                option.value = month.value;
                option.textContent = month.text;
                DOM.emailGalleryMonthFilter.appendChild(option);
            });

            // Setup folder filter
            // const folders = [...new Set(emailData.map(email => email.metadata.source_file))].sort();
            // DOM.emailGalleryFolderFilter.innerHTML = '<option value="all" selected>All Folders</option>';
            // folders.forEach(folder => {
            //     const option = document.createElement('option');
            //     option.value = folder;
            //     option.textContent = folder;
            //     DOM.emailGalleryFolderFilter.appendChild(option);
            // });
        }

        function _handleSearch() {
            const searchTerm = DOM.emailGallerySearch.value.trim();
            const senderFilter = DOM.emailGallerySender.value.trim();
            const recipientFilter = DOM.emailGalleryRecipient.value.trim();
            const toFromFilter = DOM.emailGalleryToFrom.value.trim();
            const yearFilter = DOM.emailGalleryYearFilter.value;
            const monthFilter = DOM.emailGalleryMonthFilter.value;
            const attachmentsFilter = DOM.emailGalleryAttachmentsFilter.checked;
            //const folderFilter = DOM.emailGalleryFolderFilter.value;

            // Build query parameters for /emails/search endpoint
            const params = new URLSearchParams();
            
            if (searchTerm) {
                params.append('subject', searchTerm);
            }
            if (senderFilter) {
                params.append('from_address', senderFilter);
            }
            if (recipientFilter) {
                params.append('to_address', recipientFilter);
            }
            if (toFromFilter) {
                params.append('to_from', toFromFilter);
            }
            if (yearFilter && yearFilter !== '0' && yearFilter !== '') {
                params.append('year', yearFilter);
            }
            if (monthFilter && monthFilter !== '0' && monthFilter !== '') {
                params.append('month', monthFilter);
            }
            if (attachmentsFilter) {
                params.append('has_attachments', 'true');
            }

            // Reset pagination for new search
            currentPage = 0;
            hasMoreData = true;
            
            fetch('/emails/search?' + params.toString())
            .then(r => parseEmailSearchFetchResponse(r))
            .then(data => {
                const rows = coerceEmailSearchResponse(data);
                emailData = rows.map(email => ({
                    id: email.id,
                    subject: email.subject || 'No Subject',
                    sender: email.from_address || 'Unknown Sender',
                    recipient: email.to_addresses || 'Unknown Recipient',
                    date: email.date ? formatDateAustralian(email.date) : 'No Date',
                    folder: email.folder || 'Unknown Folder',
                    body: email.snippet || 'No content',
                    preview: email.snippet || 'No preview',
                    attachments: (email.attachment_ids || []).map(id => `/attachments/${id}`),
                    emailId: email.id // Store email ID for fetching full content
                }));
                selectedEmailIndex = -1;
                _renderEmailList();
                _showInstructions();
                _updateEmailDetails();
                _selectFirstEmail();
         })
         .catch(error => {
             console.error('Error searching emails:', error);
             emailData = [];
             _renderEmailList();
             _showInstructions();
         });
        }

        function _selectFirstEmail() {
            if (emailData.length > 0) {
                _selectEmail(0);
            }
        }

        function _handleClear() {
            DOM.emailGallerySearch.value = '';
            DOM.emailGallerySender.value = '';
            DOM.emailGalleryRecipient.value = '';
            DOM.emailGalleryToFrom.value = '';
            DOM.emailGalleryYearFilter.value = 0;
            DOM.emailGalleryMonthFilter.value = 0;
            DOM.emailGalleryAttachmentsFilter.checked = false;
            DOM.emailGalleryList.innerHTML = '';
            DOM.emailGalleryEmailContent.style.display = 'none';

            // Repopulate the year filter select with all the years from 1990 to 2040
            if (DOM.emailGalleryYearFilter) {
                DOM.emailGalleryYearFilter.innerHTML = '';
                
                // Add "All Years" option
                const allYearsOption = document.createElement('option');
                allYearsOption.value = '0';
                allYearsOption.textContent = 'All Years';
                DOM.emailGalleryYearFilter.appendChild(allYearsOption);
                
                // Add years from 2040 down to 1990 (descending order)
                for (let year = 2040; year >= 1990; year--) {
                    const option = document.createElement('option');
                    option.value = year.toString();
                    option.textContent = year.toString();
                    DOM.emailGalleryYearFilter.appendChild(option);
                }
            }
            
            // Repopulate the month filter select with all months
            if (DOM.emailGalleryMonthFilter) {
                DOM.emailGalleryMonthFilter.innerHTML = '';
                
                const months = [
                    { value: 0, text: 'All Months' },
                    { value: 1, text: 'January' },
                    { value: 2, text: 'February' },
                    { value: 3, text: 'March' },
                    { value: 4, text: 'April' },
                    { value: 5, text: 'May' },
                    { value: 6, text: 'June' },
                    { value: 7, text: 'July' },
                    { value: 8, text: 'August' },
                    { value: 9, text: 'September' },
                    { value: 10, text: 'October' },
                    { value: 11, text: 'November' },
                    { value: 12, text: 'December' }
                ];
                
                months.forEach(month => {
                    const option = document.createElement('option');
                    option.value = month.value.toString();
                    option.textContent = month.text;
                    DOM.emailGalleryMonthFilter.appendChild(option);
                });
            }
            
            //DOM.emailGalleryFolderFilter.value = 'all';
            
            // Reset pagination for clear
            currentPage = 0;
            hasMoreData = true;
            //_handleSearch();
        }

        function _handleAttachmentsFilter() {
            // Trigger new search with attachments filter
            _handleSearch();
        }

        function _renderEmailList() {
            // Reset pagination when rendering new list
            currentPage = 0;
            hasMoreData = true;
            DOM.emailGalleryList.innerHTML = '';

            if (emailData.length === 0) {
                const noResults = document.createElement('div');
                noResults.style.textAlign = 'center';
                noResults.style.padding = '2em';
                noResults.style.color = '#666';
                noResults.textContent = 'No emails found matching your criteria';
                DOM.emailGalleryList.appendChild(noResults);
                return;
            }

            // Create table structure for grid/compact view
            if (viewMode === 'grid') {
                const table = document.createElement('table');
                table.className = 'email-gallery-table';
                const thead = document.createElement('thead');
                thead.innerHTML = `
                    <tr>
                        <th class="email-table-col-subject">Subject</th>
                        <th class="email-table-col-sender">From</th>
                        <th class="email-table-col-date">Date</th>
                        <th class="email-table-col-attachments"></th>
                    </tr>
                `;
                table.appendChild(thead);
                const tbody = document.createElement('tbody');
                tbody.id = 'email-gallery-table-body';
                table.appendChild(tbody);
                DOM.emailGalleryList.appendChild(table);
            }

            _loadMoreEmails();
        }

        function _loadMoreEmails() {
            if (isLoading || !hasMoreData) return;

            isLoading = true;
            
            // Note: Attachments filter is now handled server-side via has_attachments parameter
            const startIndex = currentPage * itemsPerPage;
            const endIndex = startIndex + itemsPerPage;
            const emailsToRender = emailData.slice(startIndex, endIndex);

            if (emailsToRender.length === 0) {
                hasMoreData = false;
                isLoading = false;
                return;
            }
            
            // Get the correct container for appending items
            let container;
            if (viewMode === 'grid') {
                container = document.getElementById('email-gallery-table-body');
                if (!container) {
                    // Table not created yet, create it
                    const table = document.createElement('table');
                    table.className = 'email-gallery-table';
                    const thead = document.createElement('thead');
                    thead.innerHTML = `
                        <tr>
                            <th class="email-table-col-subject">Subject</th>
                            <th class="email-table-col-sender">From</th>
                            <th class="email-table-col-date">Date</th>
                            <th class="email-table-col-attachments"></th>
                        </tr>
                    `;
                    table.appendChild(thead);
                    const tbody = document.createElement('tbody');
                    tbody.id = 'email-gallery-table-body';
                    table.appendChild(tbody);
                    DOM.emailGalleryList.appendChild(table);
                    container = tbody;
                }
            } else {
                container = DOM.emailGalleryList;
            }

            emailsToRender.forEach((email, localIndex) => {
             
                const actualIndex = startIndex + localIndex;

                if (viewMode === 'grid') {
                    // Compact table view - single line
                    if (!container) return;
                    
                    const row = document.createElement('tr');
                    row.className = 'email-table-row';
                    row.dataset.index = actualIndex;
                    
                    // Add has-attachments class if email has attachments
                    if (email.attachments && email.attachments.length > 0) {
                        row.classList.add('has-attachments');
                    }

                    row.innerHTML = `
                        <td class="email-table-col-subject">
                            <div class="email-table-subject">${email.subject || 'No Subject'}</div>
                        </td>
                        <td class="email-table-col-sender">
                            <div class="email-table-sender">${email.sender || 'Unknown Sender'}</div>
                        </td>
                        <td class="email-table-col-date">
                            <div class="email-table-date">${email.date || 'No Date'}</div>
                        </td>
                        <td class="email-table-col-attachments">
                            ${email.attachments && email.attachments.length > 0 ? '<span class="email-attachment-indicator">📎</span>' : ''}
                        </td>
                    `;

                    row.addEventListener('click', () => _selectEmail(actualIndex));
                    container.appendChild(row);
                } else {
                    // List view - cards
                    if (!container) return;
                    
                    const emailItem = document.createElement('div');
                    emailItem.className = 'email-list-item';
                    emailItem.dataset.index = actualIndex;

                    // Add has-attachments class if email has attachments
                    if (email.attachments && email.attachments.length > 0) {
                        emailItem.classList.add('has-attachments');
                    }

                    emailItem.innerHTML = `
                        <div class="email-subject">${email.subject || 'No Subject'}</div>
                        <div class="email-sender">From: ${email.sender || 'Unknown Sender'}</div>
                        <div class="email-date">${email.date || 'No Date'}</div>
                        <div class="email-preview">${email.preview || 'No Preview'}</div>
                    `;

                    emailItem.addEventListener('click', () => _selectEmail(actualIndex));
                    container.appendChild(emailItem);
                }
            });

            currentPage++;
            hasMoreData = endIndex < emailData.length;
            isLoading = false;

            // Add loading indicator if there's more data
            if (hasMoreData) {
                _addLoadingIndicator();
            }
        }

        function _addLoadingIndicator() {
            // Remove existing loading indicator
            const existingIndicator = DOM.emailGalleryList.querySelector('.loading-indicator');
            if (existingIndicator) {
                existingIndicator.remove();
            }

            const loadingIndicator = document.createElement('div');
            loadingIndicator.className = 'loading-indicator';
            loadingIndicator.innerHTML = `
                <div style="text-align: center; padding: 1em; color: #666;">
                    <div style="display: inline-block; width: 20px; height: 20px; border: 2px solid #f3f3f3; border-top: 2px solid #4a90e2; border-radius: 50%; animation: spin 1s linear infinite;"></div>
                    <div style="margin-top: 0.5em;">Loading more emails...</div>
                </div>
            `;
            DOM.emailGalleryList.appendChild(loadingIndicator);
        }

        function _handleEmailListScroll() {
            const { scrollTop, scrollHeight, clientHeight } = DOM.emailGalleryList;
            
            // Load more emails when user scrolls to within 100px of the bottom
            if (scrollTop + clientHeight >= scrollHeight - 100) {
                _loadMoreEmails();
            }
        }

        function _selectEmail(index) {
            selectedEmailIndex = index;
            
            // Update visual selection using dataset.index to match actual emailData index
            document.querySelectorAll('.email-list-item, .email-table-row').forEach((item) => {
                const itemIndex = parseInt(item.dataset.index, 10);
                item.classList.toggle('selected', itemIndex === index);
            });

            // Display email directly (filtering is now handled server-side)
            if (emailData[index]) {
                _displayEmail(emailData[index]);
            } else {
                console.error(`Email at index ${index} not found in emailData`);
            }
            _updateEmailDetails();
        }

        async function _displayEmail(email) {
            // Show email content, hide instructions
            DOM.emailGalleryInstructions.style.display = 'none';
            DOM.emailGalleryEmailContent.style.display = 'flex';
            
            // Store current email ID for delete/ask AI operations
            currentEmailId = email.emailId || email.id || null;
            
            // Show buttons
            if (DOM.emailAskAIBtn) {
                DOM.emailAskAIBtn.style.display = 'flex';
            }
            if (DOM.emailDeleteBtn) {
                DOM.emailDeleteBtn.style.display = 'flex';
            }
            
            // Update metadata
            if (DOM.emailGalleryMetadataSubject) {
                DOM.emailGalleryMetadataSubject.textContent = email.subject || 'No Subject';
            }
            if (DOM.emailGalleryMetadataFrom) {
                DOM.emailGalleryMetadataFrom.textContent = email.sender || 'Unknown Sender';
            }
            if (DOM.emailGalleryMetadataDate) {
                DOM.emailGalleryMetadataDate.textContent = email.date || 'No Date';
            }
            if (DOM.emailGalleryMetadataFolder) {
                DOM.emailGalleryMetadataFolder.textContent = email.folder || 'Unknown Folder';
            }
            
            // Show loading state
            if (DOM.emailGalleryIframe) {
                DOM.emailGalleryIframe.style.display = 'none';
            }
            if (DOM.emailGalleryAttachmentsGrid) {
                DOM.emailGalleryAttachmentsGrid.innerHTML = '<div class="email-attachment-loading">Loading attachments...</div>';
            }
            
            // Load email HTML into iframe
            if (email.emailId && DOM.emailGalleryIframe) {
                try {
                    const response = await fetch(`/emails/${email.emailId}/html`);
                    if (response.ok) {
                        const htmlContent = await response.text();
                        DOM.emailGalleryIframe.srcdoc = htmlContent;
                        DOM.emailGalleryIframe.style.display = 'block';
                    } else {
                        // Fallback to plain text
                        const textResponse = await fetch(`/emails/${email.emailId}/text`);
                        if (textResponse.ok) {
                            const textContent = await textResponse.text();
                            const wrappedHtml = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
    body {
        font-family: Arial, sans-serif;
        line-height: 1.6;
        max-width: 800px;
        margin: 20px auto;
        padding: 20px;
        white-space: pre-wrap;
        word-wrap: break-word;
    }
</style>
</head>
<body>
${textContent}
</body>
</html>`;
                            DOM.emailGalleryIframe.srcdoc = wrappedHtml;
                            DOM.emailGalleryIframe.style.display = 'block';
                        } else {
                            throw new Error('Failed to fetch email content');
                        }
                    }
                } catch (error) {
                    console.error('Error fetching email content:', error);
                    if (DOM.emailGalleryIframe) {
                        DOM.emailGalleryIframe.srcdoc = `<html><body style="padding: 20px; color: #c33; text-align: center;">Error loading email: ${error.message}</body></html>`;
                        DOM.emailGalleryIframe.style.display = 'block';
                    }
                }
            }
            
            // Fetch and display attachments
            if (email.emailId && email.attachments && email.attachments.length > 0) {
                await _loadAttachments(email.emailId, email.attachments);
            } else {
                if (DOM.emailGalleryAttachmentsGrid) {
                    DOM.emailGalleryAttachmentsGrid.innerHTML = '<div class="email-attachment-empty">No attachments</div>';
                }
            }
        }

        async function _loadAttachments(emailId, attachmentUrls) {
            if (!DOM.emailGalleryAttachmentsGrid) {
                return;
            }
            
            DOM.emailGalleryAttachmentsGrid.innerHTML = '';
            
            // Extract attachment IDs from URLs
            const attachmentIds = attachmentUrls.map(url => {
                const match = url.match(/\/attachments\/(\d+)/);
                return match ? parseInt(match[1]) : null;
            }).filter(id => id !== null);
            
            // Fetch attachment info for each
            const attachmentPromises = attachmentIds.map(id => 
                fetch(`/attachments/${id}/info`)
                    .then(r => r.json())
                    .catch(err => {
                        console.error(`Error fetching attachment ${id} info:`, err);
                        return null;
                    })
            );
            
            const attachmentInfos = await Promise.all(attachmentPromises);
            
            // Render each attachment
            attachmentInfos.forEach((info, index) => {
                if (!info) return;
                
                const attachmentElement = _createAttachmentElement(info, attachmentIds[index]);
                DOM.emailGalleryAttachmentsGrid.appendChild(attachmentElement);
            });
        }
        
        function _createAttachmentElement(attachmentInfo, attachmentId) {
            const container = document.createElement('div');
            container.className = 'email-attachment-item';
            container.dataset.attachmentId = attachmentId;
            
            const isImage = attachmentInfo.content_type && attachmentInfo.content_type.startsWith('image/');
            
            if (isImage) {
                // Image attachment - show thumbnail preview
                const img = document.createElement('img');
                img.className = 'email-attachment-thumbnail';
                img.src = `/attachments/${attachmentId}?preview=true`;
                img.alt = attachmentInfo.filename || 'Attachment';
                img.loading = 'lazy';
                container.appendChild(img);
            } else {
                // Non-image attachment - show icon
                const iconContainer = document.createElement('div');
                iconContainer.className = 'email-attachment-icon-container';
                const icon = _getAttachmentIcon(attachmentInfo.content_type);
                iconContainer.innerHTML = `<i class="${icon.class}" style="font-size: 3em; color: ${icon.color};"></i>`;
                container.appendChild(iconContainer);
            }
            
            // Add filename label
            const filenameLabel = document.createElement('div');
            filenameLabel.className = 'email-attachment-filename';
            filenameLabel.textContent = attachmentInfo.filename || `Attachment ${attachmentId}`;
            filenameLabel.title = attachmentInfo.filename || `Attachment ${attachmentId}`;
            container.appendChild(filenameLabel);
            
            // Add click handler
            container.addEventListener('click', () => {
                _viewAttachment(attachmentId, attachmentInfo, isImage);
            });
            
            return container;
        }
        
        function _getAttachmentIcon(contentType) {
            if (!contentType) {
                return { class: 'fas fa-file', color: '#666' };
            }
            
            if (contentType === 'application/pdf') {
                return { class: 'fas fa-file-pdf', color: '#dc3545' };
            }
            
            if (contentType.includes('word') || contentType.includes('msword') || contentType.includes('document')) {
                return { class: 'fas fa-file-word', color: '#2b579a' };
            }
            
            if (contentType.includes('excel') || contentType.includes('spreadsheet')) {
                return { class: 'fas fa-file-excel', color: '#1d6f42' };
            }
            
            if (contentType.includes('powerpoint') || contentType.includes('presentation')) {
                return { class: 'fas fa-file-powerpoint', color: '#d04423' };
            }
            
            if (contentType.includes('zip') || contentType.includes('archive')) {
                return { class: 'fas fa-file-archive', color: '#ffc107' };
            }
            
            if (contentType.includes('text')) {
                return { class: 'fas fa-file-alt', color: '#17a2b8' };
            }
            
            return { class: 'fas fa-file', color: '#666' };
        }
        
        function _viewAttachment(attachmentId, attachmentInfo, isImage) {
            if (isImage) {
                // Show image in image modal
                if (DOM.emailAttachmentImageDisplay && DOM.emailAttachmentImageModal) {
                    DOM.emailAttachmentImageDisplay.src = `/attachments/${attachmentId}`;
                    DOM.emailAttachmentImageDisplay.alt = attachmentInfo.filename || 'Attachment';
                    DOM.emailAttachmentImageModal.style.display = 'flex';
                }
            } else {
                // Show document in iframe modal
                if (DOM.emailAttachmentDocumentIframe && DOM.emailAttachmentDocumentModal) {
                    DOM.emailAttachmentDocumentIframe.src = `/attachments/${attachmentId}`;
                    DOM.emailAttachmentDocumentModal.style.display = 'flex';
                }
            }
        }

        function _showInstructions() {
            DOM.emailGalleryInstructions.style.display = 'flex';
            DOM.emailGalleryEmailContent.style.display = 'none';
            
            // Hide buttons
            if (DOM.emailAskAIBtn) {
                DOM.emailAskAIBtn.style.display = 'none';
            }
            if (DOM.emailDeleteBtn) {
                DOM.emailDeleteBtn.style.display = 'none';
            }
            
            // Clear current email ID
            currentEmailId = null;
        }

        function _clearEmailContent() {
            if (DOM.emailGalleryIframe) {
                DOM.emailGalleryIframe.srcdoc = '';
            }
            if (DOM.emailGalleryAttachmentsGrid) {
                DOM.emailGalleryAttachmentsGrid.innerHTML = '';
            }
            if (DOM.emailGalleryMetadataSubject) {
                DOM.emailGalleryMetadataSubject.textContent = '';
            }
            if (DOM.emailGalleryMetadataFrom) {
                DOM.emailGalleryMetadataFrom.textContent = '';
            }
            if (DOM.emailGalleryMetadataDate) {
                DOM.emailGalleryMetadataDate.textContent = '';
            }
            if (DOM.emailGalleryMetadataFolder) {
                DOM.emailGalleryMetadataFolder.textContent = '';
            }
            
            // Hide buttons
            if (DOM.emailAskAIBtn) {
                DOM.emailAskAIBtn.style.display = 'none';
            }
            if (DOM.emailDeleteBtn) {
                DOM.emailDeleteBtn.style.display = 'none';
            }
            
            // Clear current email ID
            currentEmailId = null;
        }

        async function deleteEmail() {
            if (!currentEmailId) return;

            // Get email subject for confirmation message
            const emailSubject = DOM.emailGalleryMetadataSubject?.textContent || 'this email';
            const emailIdToDelete = currentEmailId;
            
            // Show confirmation dialog
            const confirmed = confirm(`Are you sure you want to delete "${emailSubject}"?\n\nThis action cannot be undone.`);
            if (!confirmed) {
                return;
            }

            try {
                const response = await fetch(`/emails/${emailIdToDelete}`, {
                    method: 'DELETE'
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.detail || 'Failed to delete email');
                }

                const result = await response.json();
                
                // Remove from emailData array before clearing view
                const emailIndex = emailData.findIndex(e => (e.emailId || e.id) === emailIdToDelete);
                if (emailIndex !== -1) {
                    emailData.splice(emailIndex, 1);
                }

                // Clear the email view
                currentEmailId = null;
                selectedEmailIndex = -1;
                _showInstructions();
                _clearEmailContent();

                // Remove active state from all items
                const items = document.querySelectorAll('.email-list-item, .email-table-row');
                items.forEach(item => item.classList.remove('selected'));

                // Reload email list
                await _handleSearch();
                
                alert(`Successfully deleted email: ${emailSubject}`);
            } catch (error) {
                console.error('Error deleting email:', error);
                alert(`Error deleting email: ${error.message}`);
            }
        }

        function _updateEmailDetails() {
            // Email details are now displayed in the iframe and attachment grid
            // This function is kept for potential future use
        }

        function _handleKeydown(event) {
            if (DOM.emailGalleryModal.style.display !== 'flex') return;
            

            switch(event.key) {
                case 'Escape':
                    close();
                    break;
                case 'ArrowDown':
                    event.preventDefault();
                    if (selectedEmailIndex < emailData.length - 1) {
                        _selectEmail(selectedEmailIndex + 1);
                        _scrollToSelectedEmail();
                    }
                    break;
                case 'ArrowUp':
                    event.preventDefault();
                    if (selectedEmailIndex > 0) {
                        _selectEmail(selectedEmailIndex - 1);
                        _scrollToSelectedEmail();
                    }
                    break;
            }
        }

        function openContact(contactName) {
            _setupFilters();
            DOM.emailGalleryToFrom.value = contactName || '';
            DOM.emailGalleryYearFilter.value = '0';
            DOM.emailGalleryMonthFilter.value = '0';
            DOM.emailGalleryModal.style.display = 'flex';
            _handleSearch();
        }

        function _scrollToSelectedEmail() {
            const selectedItem = document.querySelector('.email-list-item.selected, .email-table-row.selected');
            if (selectedItem) {
                selectedItem.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            }
        }

        async function openAndSelectEmail(emailId) {
            // Open the Email Gallery modal
            DOM.emailGalleryModal.style.display = 'flex';
            
            try {
                // First, fetch the email metadata to get its properties
                const metadataResponse = await fetch(`/emails/${emailId}/metadata`);
                if (!metadataResponse.ok) {
                    throw new Error(`Failed to fetch email metadata: ${metadataResponse.status}`);
                }
                
                const emailMetadata = await metadataResponse.json();
                
                // Extract date information
                let year = 0;
                let month = 0;
                if (emailMetadata.date) {
                    const emailDate = new Date(emailMetadata.date);
                    if (!isNaN(emailDate.getTime())) {
                        year = emailDate.getFullYear();
                        month = emailDate.getMonth() + 1; // JavaScript months are 0-indexed
                    }
                }
                
                // Set filters to match the email's properties
                DOM.emailGallerySearch.value = '';
                DOM.emailGallerySender.value = emailMetadata.from_address || '';
                DOM.emailGalleryRecipient.value = '';
                DOM.emailGalleryToFrom.value = emailMetadata.from_address || '';
                DOM.emailGalleryYearFilter.value = year > 0 ? year.toString() : '0';
                DOM.emailGalleryMonthFilter.value = month > 0 ? month.toString() : '0';
                DOM.emailGalleryAttachmentsFilter.checked = false;
                
                // Build query parameters based on the email's properties
                const params = new URLSearchParams();
                if (emailMetadata.from_address) {
                    params.append('from_address', emailMetadata.from_address);
                }
                if (year > 0) {
                    params.append('year', year.toString());
                }
                if (month > 0) {
                    params.append('month', month.toString());
                }
                
                // Load emails with filters matching the email
                const searchResponse = await fetch('/emails/search?' + params.toString());
                const data = await parseEmailSearchFetchResponse(searchResponse);
                const rows = coerceEmailSearchResponse(data);
                emailData = rows.map(email => ({
                    id: email.id,
                    subject: email.subject || 'No Subject',
                    sender: email.from_address || 'Unknown Sender',
                    recipient: email.to_addresses || 'Unknown Recipient',
                    date: email.date ? formatDateAustralian(email.date) : 'No Date',
                    folder: email.folder || 'Unknown Folder',
                    body: email.snippet || 'No content',
                    preview: email.snippet || 'No preview',
                    attachments: (email.attachment_ids || []).map(id => `/attachments/${id}`),
                    emailId: email.id // Store email ID for fetching full content
                }));
                
                _renderEmailList();
                _showInstructions();
                
                // Find email index by ID
                const emailIndex = emailData.findIndex(email => 
                    (email.emailId === emailId) || (email.id === emailId)
                );
                
                if (emailIndex !== -1) {
                    _selectEmail(emailIndex);
                    _scrollToSelectedEmail();
                } else {
                    console.warn(`Email with ID ${emailId} not found in filtered results`);
                    // Show instructions since email wasn't found
                    _showInstructions();
                }
            } catch (error) {
                console.error('Error loading email:', error);
                alert('Failed to load email. Please try again.');
                _showInstructions();
            }
        }

        return { init, open, close, openContact, openAndSelectEmail };
})();


Modals.EmailEditor = (() => {
        let emailData = [];
        let currentPage = 1;
        let pageSize = 50;
        let selectedRowIndex = -1;
        let selectedEmailIds = new Set();

        function formatDateAustralian(dateString) {
            if (!dateString) return 'No Date';
            try {
                const date = new Date(dateString);
                if (isNaN(date.getTime())) return 'Invalid Date';
                
                const day = String(date.getDate()).padStart(2, '0');
                const month = String(date.getMonth() + 1).padStart(2, '0');
                const year = date.getFullYear();
                const hours = String(date.getHours()).padStart(2, '0');
                const minutes = String(date.getMinutes()).padStart(2, '0');
                
                return `${day}/${month}/${year} ${hours}:${minutes}`;
            } catch (error) {
                return 'Invalid Date';
            }
        }

        function _truncateText(text, maxLength) {
            if (!text) return '';
            if (text.length <= maxLength) return text;
            return text.substring(0, maxLength - 3) + '...';
        }

        function _setupFilters() {
            // Setup year filter
            const years = [
                { value: 0, text: 'All Years' },
                { value: 2032, text: '2032' },
                { value: 2031, text: '2031' },
                { value: 2030, text: '2030' },
                { value: 2029, text: '2029' },
                { value: 2028, text: '2028' },
                { value: 2027, text: '2027' },
                { value: 2026, text: '2026' },
                { value: 2025, text: '2025' },
                { value: 2024, text: '2024' },
                { value: 2023, text: '2023' },
                { value: 2022, text: '2022' },
                { value: 2021, text: '2021' },
                { value: 2020, text: '2020' },  
                { value: 2019, text: '2019' },
                { value: 2018, text: '2018' },
                { value: 2017, text: '2017' },
                { value: 2016, text: '2016' },
                { value: 2015, text: '2015' },
                { value: 2014, text: '2014' },
                { value: 2013, text: '2013' },  
                { value: 2012, text: '2012' },
                { value: 2011, text: '2011' },
                { value: 2010, text: '2010' },
                { value: 2009, text: '2009' },
                { value: 2008, text: '2008' },
                { value: 2007, text: '2007' },
                { value: 2006, text: '2006' },
                { value: 2005, text: '2005' },
                { value: 2004, text: '2004' },
                { value: 2003, text: '2003' },
                { value: 2002, text: '2002' },
                { value: 2001, text: '2001' },
                { value: 2000, text: '2000' },
                { value: 1999, text: '1999' },
                { value: 1998, text: '1998' },
                { value: 1997, text: '1997' },
                { value: 1996, text: '1996' },
                { value: 1995, text: '1995' },
                { value: 1994, text: '1994' },
                { value: 1993, text: '1993' },
                { value: 1992, text: '1992' }
            ];
            
            if (DOM.emailEditorYearFilter) {
                DOM.emailEditorYearFilter.innerHTML = '';
                years.forEach(year => {
                    const option = document.createElement('option');
                    option.value = year.value;
                    option.textContent = year.text;
                    DOM.emailEditorYearFilter.appendChild(option);
                });
            }

            // Setup month filter
            const months = [
                { value: 0, text: 'All Months' },
                { value: 1, text: 'January' },
                { value: 2, text: 'February' },
                { value: 3, text: 'March' },
                { value: 4, text: 'April' },
                { value: 5, text: 'May' },
                { value: 6, text: 'June' },
                { value: 7, text: 'July' },
                { value: 8, text: 'August' },
                { value: 9, text: 'September' },
                { value: 10, text: 'October' },
                { value: 11, text: 'November' },
                { value: 12, text: 'December' }
            ];
            
            if (DOM.emailEditorMonthFilter) {
                DOM.emailEditorMonthFilter.innerHTML = '';
                months.forEach(month => {
                    const option = document.createElement('option');
                    option.value = month.value;
                    option.textContent = month.text;
                    DOM.emailEditorMonthFilter.appendChild(option);
                });
            }
        }

        function init() {
            if (DOM.emailEditorSearchBtn) {
                DOM.emailEditorSearchBtn.addEventListener('click', _handleSearch);
            }
            if (DOM.emailEditorClearBtn) {
                DOM.emailEditorClearBtn.addEventListener('click', _handleClear);
            }
            if (DOM.emailEditorBulkDeleteBtn) {
                DOM.emailEditorBulkDeleteBtn.addEventListener('click', _handleBulkDelete);
            }
            if (DOM.emailEditorSelectAllBtn) {
                DOM.emailEditorSelectAllBtn.addEventListener('click', _handleSelectAll);
            }
            if (DOM.emailEditorOpenEmailsGalleryBtn) {
                DOM.emailEditorOpenEmailsGalleryBtn.addEventListener('click', () => {
                    if (typeof Modals !== 'undefined' && Modals.EmailEditor && Modals.EmailEditor.close) {
                        Modals.EmailEditor.close();
                    }
                    if (typeof Modals !== 'undefined' && Modals.EmailGallery && Modals.EmailGallery.open) {
                        void Modals.EmailGallery.open();
                    }
                });
            }
            if (DOM.closeEmailEditorModalBtn) {
                DOM.closeEmailEditorModalBtn.addEventListener('click', () => close());
            }
            if (DOM.emailEditorModal) {
                DOM.emailEditorModal.addEventListener('click', (e) => {
                    if (e.target === DOM.emailEditorModal) close();
                });
            }
            _setupFilters();
        }

        let _emailEditorLoadingCount = 0;
        function _setEmailEditorLoading(loading) {
            if (!DOM.emailEditorViewer) return;
            _emailEditorLoadingCount += loading ? 1 : -1;
            _emailEditorLoadingCount = Math.max(0, _emailEditorLoadingCount);
            DOM.emailEditorViewer.classList.toggle('loading', _emailEditorLoadingCount > 0);
        }

        function _handleSearch() {
            currentPage = 1;
            _loadEmails();
        }

        function _handleClear() {
            DOM.emailEditorSearch.value = '';
            DOM.emailEditorSender.value = '';
            DOM.emailEditorRecipient.value = '';
            DOM.emailEditorToFrom.value = '';
            DOM.emailEditorYearFilter.value = '0';
            DOM.emailEditorMonthFilter.value = '0';
            DOM.emailEditorAttachmentsFilter.checked = false;
            emailData = [];
            selectedEmailIds.clear();
            currentPage = 1;
            _renderTable();
            _renderPagination();
            _updateBulkDeleteButton();
        }

        function _loadEmails() {
            const params = new URLSearchParams();
            const searchTerm = DOM.emailEditorSearch.value.trim();
            const senderFilter = DOM.emailEditorSender.value.trim();
            const recipientFilter = DOM.emailEditorRecipient.value.trim();
            const toFromFilter = DOM.emailEditorToFrom.value.trim();
            const yearFilter = DOM.emailEditorYearFilter.value;
            const monthFilter = DOM.emailEditorMonthFilter.value;
            const attachmentsFilter = DOM.emailEditorAttachmentsFilter.checked;

            if (searchTerm) {
                params.append('subject', searchTerm);
            }
            if (senderFilter) {
                params.append('from_address', senderFilter);
            }
            if (recipientFilter) {
                params.append('to_address', recipientFilter);
            }
            if (toFromFilter) {
                params.append('to_from', toFromFilter);
            }
            if (yearFilter && yearFilter !== '0' && yearFilter !== '') {
                params.append('year', yearFilter);
            }
            if (monthFilter && monthFilter !== '0' && monthFilter !== '') {
                params.append('month', monthFilter);
            }
            if (attachmentsFilter) {
                params.append('has_attachments', 'true');
            }

            _setEmailEditorLoading(true);
            fetch('/emails/search?' + params.toString())
                .then(r => parseEmailSearchFetchResponse(r))
                .then(data => {
                    const rows = coerceEmailSearchResponse(data);
                    emailData = rows.map(email => ({
                        id: email.id,
                        subject: email.subject || 'No Subject',
                        sender: email.from_address || 'Unknown Sender',
                        recipient: email.to_addresses || 'Unknown Recipient',
                        date: email.date ? formatDateAustralian(email.date) : 'No Date',
                        emailId: email.id,
                        is_personal: email.is_personal || false,
                        is_business: email.is_business || false,
                        is_important: email.is_important || false,
                        use_by_ai: email.use_by_ai !== undefined ? email.use_by_ai : true
                    }));
                    _renderTable();
                    _renderPagination();
                })
                .catch(error => {
                    console.error('Error loading emails:', error);
                    emailData = [];
                    _renderTable();
                    _renderPagination();
                })
                .finally(() => {
                    _setEmailEditorLoading(false);
                });
        }

        function _renderTable() {
            if (!DOM.emailEditorTableBody) return;

            const startIndex = (currentPage - 1) * pageSize;
            const endIndex = startIndex + pageSize;
            const emailsToShow = emailData.slice(startIndex, endIndex);

            DOM.emailEditorTableBody.innerHTML = '';

            if (emailsToShow.length === 0) {
                const row = document.createElement('tr');
                const cell = document.createElement('td');
                cell.colSpan = 9;
                cell.textContent = 'No emails found';
                cell.style.textAlign = 'center';
                cell.style.padding = '2em';
                cell.style.color = '#666';
                row.appendChild(cell);
                DOM.emailEditorTableBody.appendChild(row);
                return;
            }

            emailsToShow.forEach((email, index) => {
                const row = document.createElement('tr');
                row.className = 'email-editor-table-row';
                if (selectedRowIndex === startIndex + index) {
                    row.classList.add('selected');
                }
                row.dataset.index = startIndex + index;
                row.dataset.emailId = email.id;

                // Subject column
                const subjectCell = document.createElement('td');
                subjectCell.className = 'email-editor-col-subject';
                subjectCell.textContent = _truncateText(email.subject, 60);
                subjectCell.title = email.subject;
                subjectCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _selectRow(startIndex + index);
                });
                row.appendChild(subjectCell);

                // From column
                const fromCell = document.createElement('td');
                fromCell.className = 'email-editor-col-from';
                fromCell.textContent = _truncateText(email.sender, 40);
                fromCell.title = email.sender;
                fromCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _selectRow(startIndex + index);
                });
                row.appendChild(fromCell);

                // To column
                const toCell = document.createElement('td');
                toCell.className = 'email-editor-col-to';
                toCell.textContent = _truncateText(email.recipient, 40);
                toCell.title = email.recipient;
                toCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _selectRow(startIndex + index);
                });
                row.appendChild(toCell);

                // Date column
                const dateCell = document.createElement('td');
                dateCell.className = 'email-editor-col-date';
                dateCell.textContent = email.date;
                dateCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _selectRow(startIndex + index);
                });
                row.appendChild(dateCell);

                // Personal column (editable)
                const personalCell = document.createElement('td');
                personalCell.className = 'email-editor-col-personal';
                personalCell.innerHTML = email.is_personal ? '✓' : '';
                personalCell.style.cursor = 'pointer';
                personalCell.style.textAlign = 'center';
                personalCell.style.fontWeight = 'bold';
                personalCell.style.color = email.is_personal ? '#28a745' : '#ccc';
                personalCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _toggleField(email.id, 'is_personal', !email.is_personal);
                });
                row.appendChild(personalCell);

                // Business column (editable)
                const businessCell = document.createElement('td');
                businessCell.className = 'email-editor-col-business';
                businessCell.innerHTML = email.is_business ? '✓' : '';
                businessCell.style.cursor = 'pointer';
                businessCell.style.textAlign = 'center';
                businessCell.style.fontWeight = 'bold';
                businessCell.style.color = email.is_business ? '#28a745' : '#ccc';
                businessCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _toggleField(email.id, 'is_business', !email.is_business);
                });
                row.appendChild(businessCell);

                // Important column (editable)
                const importantCell = document.createElement('td');
                importantCell.className = 'email-editor-col-important';
                importantCell.innerHTML = email.is_important ? '✓' : '';
                importantCell.style.cursor = 'pointer';
                importantCell.style.textAlign = 'center';
                importantCell.style.fontWeight = 'bold';
                importantCell.style.color = email.is_important ? '#ffc107' : '#ccc';
                importantCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    _toggleField(email.id, 'is_important', !email.is_important);
                });
                row.appendChild(importantCell);

                // Use by AI column (editable)
                const useByAiCell = document.createElement('td');
                useByAiCell.className = 'email-editor-col-use-by-ai';
                if (email.use_by_ai === true) {
                    useByAiCell.innerHTML = '✓';
                    useByAiCell.style.color = '#17a2b8';
                } else {
                    useByAiCell.innerHTML = '✗';
                    useByAiCell.style.color = '#dc3545';
                }
                useByAiCell.style.cursor = 'pointer';
                useByAiCell.style.textAlign = 'center';
                useByAiCell.style.fontWeight = 'bold';
                useByAiCell.addEventListener('click', (e) => {
                    e.stopPropagation();
                    // Toggle between true and false
                    _toggleField(email.id, 'use_by_ai', !email.use_by_ai);
                });
                row.appendChild(useByAiCell);

                // Delete column (checkbox)
                const deleteCell = document.createElement('td');
                deleteCell.className = 'email-editor-col-delete';
                deleteCell.style.textAlign = 'center';
                const checkbox = document.createElement('input');
                checkbox.type = 'checkbox';
                checkbox.className = 'email-delete-checkbox';
                checkbox.dataset.emailId = email.id;
                checkbox.checked = selectedEmailIds.has(email.id);
                checkbox.addEventListener('click', (e) => {
                    e.stopPropagation();
                    if (checkbox.checked) {
                        selectedEmailIds.add(email.id);
                    } else {
                        selectedEmailIds.delete(email.id);
                    }
                    _updateBulkDeleteButton();
                });
                deleteCell.appendChild(checkbox);
                row.appendChild(deleteCell);

                DOM.emailEditorTableBody.appendChild(row);
            });
        }

        function _selectRow(index) {
            selectedRowIndex = index;
            _renderTable();
        }

        function _toggleField(emailId, fieldName, newValue) {
            // Update local data immediately for responsive UI
            const email = emailData.find(e => e.id === emailId);
            if (email) {
                email[fieldName] = newValue;
            }

            // Update on server
            _setEmailEditorLoading(true);
            fetch(`/emails/${emailId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ [fieldName]: newValue })
            })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to update email');
                }
                return response.json();
            })
            .then(updatedEmail => {
                // Update local data with server response
                const emailIndex = emailData.findIndex(e => e.id === emailId);
                if (emailIndex !== -1) {
                    emailData[emailIndex] = {
                        ...emailData[emailIndex],
                        is_personal: updatedEmail.is_personal,
                        is_business: updatedEmail.is_business,
                        is_important: updatedEmail.is_important,
                        use_by_ai: updatedEmail.use_by_ai
                    };
                }
                _renderTable();
            })
            .catch(error => {
                console.error('Error updating email:', error);
                // Revert local change on error
                if (email) {
                    email[fieldName] = !newValue;
                }
                _renderTable();
                alert('Failed to update email. Please try again.');
            })
            .finally(() => {
                _setEmailEditorLoading(false);
            });
        }

        function _handleSelectAll() {
            const startIndex = (currentPage - 1) * pageSize;
            const endIndex = startIndex + pageSize;
            const emailsOnPage = emailData.slice(startIndex, endIndex);
            emailsOnPage.forEach(email => selectedEmailIds.add(email.id));
            _renderTable();
            _updateBulkDeleteButton();
        }

        function _updateBulkDeleteButton() {
            if (DOM.emailEditorBulkDeleteBtn) {
                if (selectedEmailIds.size > 0) {
                    DOM.emailEditorBulkDeleteBtn.style.display = 'inline-block';
                    DOM.emailEditorBulkDeleteBtn.textContent = `Delete Selected (${selectedEmailIds.size})`;
                } else {
                    DOM.emailEditorBulkDeleteBtn.style.display = 'none';
                }
            }
        }

        function _handleBulkDelete() {
            if (selectedEmailIds.size === 0) {
                return;
            }

            const emailIdsArray = Array.from(selectedEmailIds);
            const count = emailIdsArray.length;
            
            // Use the existing confirmation modal
            if (window.Modals && window.Modals.Confirmation) {
                window.Modals.Confirmation.show(
                    'Delete Emails',
                    `Are you sure you want to delete ${count} email(s)? This action cannot be undone.`,
                    () => {
                        _bulkDeleteEmails(emailIdsArray);
                    }
                );
            } else {
                // Fallback to browser confirm
                if (confirm(`Are you sure you want to delete ${count} email(s)? This action cannot be undone.`)) {
                    _bulkDeleteEmails(emailIdsArray);
                }
            }
        }

        function _bulkDeleteEmails(emailIds) {
            _setEmailEditorLoading(true);
            fetch('/emails/bulk-delete', {
                method: 'DELETE',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ email_ids: emailIds })
            })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to delete emails');
                }
                return response.json();
            })
            .then(result => {
                // Remove deleted emails from local data
                emailIds.forEach(id => {
                    emailData = emailData.filter(e => e.id !== id);
                    selectedEmailIds.delete(id);
                });
                
                // Reset to first page if current page is empty
                const totalPages = Math.ceil(emailData.length / pageSize);
                if (currentPage > totalPages && totalPages > 0) {
                    currentPage = totalPages;
                } else if (totalPages === 0) {
                    currentPage = 1;
                }
                
                _updateBulkDeleteButton();
                _renderTable();
                _renderPagination();
                
                if (result.errors && result.errors.length > 0) {
                    alert(`Deleted ${result.deleted_count} email(s). Some errors occurred:\n${result.errors.join('\n')}`);
                } else {
                    alert(`Successfully deleted ${result.deleted_count} email(s).`);
                }
            })
            .catch(error => {
                console.error('Error bulk deleting emails:', error);
                alert('Failed to delete emails. Please try again.');
            })
            .finally(() => {
                _setEmailEditorLoading(false);
            });
        }

        function _confirmDelete(emailId, emailSubject) {
            // Use the existing confirmation modal
            if (window.Modals && window.Modals.Confirmation) {
                window.Modals.Confirmation.show(
                    'Delete Email',
                    `Are you sure you want to delete "${emailSubject}"? This action cannot be undone.`,
                    () => {
                        _deleteEmail(emailId);
                    }
                );
            } else {
                // Fallback to browser confirm
                if (confirm(`Are you sure you want to delete "${emailSubject}"? This action cannot be undone.`)) {
                    _deleteEmail(emailId);
                }
            }
        }

        function _deleteEmail(emailId) {
            _setEmailEditorLoading(true);
            fetch(`/emails/${emailId}`, {
                method: 'DELETE'
            })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to delete email');
                }
                return response.json();
            })
            .then(() => {
                // Remove from local data
                emailData = emailData.filter(e => e.id !== emailId);
                selectedEmailIds.delete(emailId);
                // Reset to first page if current page is empty
                const totalPages = Math.ceil(emailData.length / pageSize);
                if (currentPage > totalPages && totalPages > 0) {
                    currentPage = totalPages;
                } else if (totalPages === 0) {
                    currentPage = 1;
                }
                _updateBulkDeleteButton();
                _renderTable();
                _renderPagination();
            })
            .catch(error => {
                console.error('Error deleting email:', error);
                alert('Failed to delete email. Please try again.');
            })
            .finally(() => {
                _setEmailEditorLoading(false);
            });
        }

        function _renderPagination() {
            if (!DOM.emailEditorPagination) return;

            const totalPages = Math.ceil(emailData.length / pageSize);
            
            if (totalPages <= 1) {
                DOM.emailEditorPagination.innerHTML = '';
                return;
            }

            let paginationHTML = '';

            // Previous button
            paginationHTML += `<button ${currentPage === 1 ? 'disabled' : ''} class="email-editor-prev-btn">Previous</button>`;

            // Page info
            paginationHTML += `<span class="email-editor-page-info">Page ${currentPage} of ${totalPages}</span>`;

            // Next button
            paginationHTML += `<button ${currentPage === totalPages ? 'disabled' : ''} class="email-editor-next-btn">Next</button>`;

            DOM.emailEditorPagination.innerHTML = paginationHTML;

            // Add event listeners
            const prevBtn = DOM.emailEditorPagination.querySelector('.email-editor-prev-btn');
            const nextBtn = DOM.emailEditorPagination.querySelector('.email-editor-next-btn');

            if (prevBtn) {
                prevBtn.addEventListener('click', () => {
                    if (currentPage > 1) {
                        currentPage--;
                        _renderTable();
                        _renderPagination();
                    }
                });
            }

            if (nextBtn) {
                nextBtn.addEventListener('click', () => {
                    if (currentPage < totalPages) {
                        currentPage++;
                        _renderTable();
                        _renderPagination();
                    }
                });
            }
        }

        function open() {
            if (DOM.emailEditorModal) {
                DOM.emailEditorModal.style.display = 'flex';
            }
            currentPage = 1;
            selectedRowIndex = -1;
            selectedEmailIds.clear();
            _updateBulkDeleteButton();
        }

        function close() {
            if (DOM.emailEditorModal) {
                DOM.emailEditorModal.style.display = 'none';
            }
        }

        return { init, open, close };
})();


Modals.EmailAttachments = (() => {
        function _triggerGridLoad() {
            if (typeof loadImages !== 'function') return;
            setTimeout(() => {
                const imagesGrid = document.getElementById('images-grid');
                if (imagesGrid && (imagesGrid.innerHTML === '' || imagesGrid.style.display === 'none')) {
                    loadImages(1);
                }
            }, 100);
        }

        function open() {
            if (DOM.emailAttachmentsModal) {
                DOM.emailAttachmentsModal.style.display = 'flex';
            }
            _triggerGridLoad();
        }

        function close() {
            if (DOM.emailAttachmentsModal) {
                DOM.emailAttachmentsModal.style.display = 'none';
            }
        }

        function init() {
            if (DOM.closeEmailAttachmentsModalBtn) {
                DOM.closeEmailAttachmentsModalBtn.addEventListener('click', () => close());
            }
            if (DOM.emailAttachmentsModal) {
                DOM.emailAttachmentsModal.addEventListener('click', (e) => {
                    if (e.target === DOM.emailAttachmentsModal) close();
                });
            }
        }

        return { init, open, close };
})();


