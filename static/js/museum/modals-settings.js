'use strict';

Modals.ReferenceDocuments = (() => {
        let documents = [];
        let filteredDocuments = [];
        let currentFilters = {
            search: '',
            category: '',
            contentType: '',
            availableForTask: null
        };

        function formatFileSize(bytes) {
            if (bytes === 0) return '0 Bytes';
            const k = 1024;
            const sizes = ['Bytes', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
        }

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

        function getFileIcon(contentType) {
            if (!contentType) return { class: 'fas fa-file', color: '#666' };
            
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
            
            if (contentType.startsWith('image/')) {
                return { class: 'fas fa-file-image', color: '#17a2b8' };
            }
            
            if (contentType === 'application/json') {
                return { class: 'fas fa-file-code', color: '#f39c12' };
            }
            
            if (contentType.includes('text') || contentType === 'text/csv') {
                return { class: 'fas fa-file-alt', color: '#17a2b8' };
            }
            
            return { class: 'fas fa-file', color: '#666' };
        }

        async function loadDocuments() {
            if (!DOM.referenceDocumentsList) return;
            
            DOM.referenceDocumentsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">Loading documents...</div>';
            
            try {
                const params = new URLSearchParams();
                if (currentFilters.search) params.append('search', currentFilters.search);
                if (currentFilters.category) params.append('category', currentFilters.category);
                if (currentFilters.contentType) {
                    if (currentFilters.contentType === 'image') {
                        params.append('content_type', 'image/');
                    } else if (currentFilters.contentType === 'text') {
                        params.append('content_type', 'text/');
                    } else {
                        params.append('content_type', currentFilters.contentType);
                    }
                }
                if (currentFilters.availableForTask !== null) {
                    params.append('available_for_task', currentFilters.availableForTask.toString());
                }
                
                const response = await fetch(`/reference-documents?${params.toString()}`);
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                documents = await response.json();
                filteredDocuments = documents;
                renderDocuments();
            } catch (error) {
                console.error("Failed to load reference documents:", error);
                DOM.referenceDocumentsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #dc3545;">Failed to load documents: ' + error.message + '</div>';
            }
        }

        function renderDocuments() {
            if (!DOM.referenceDocumentsList) return;
            
            if (filteredDocuments.length === 0) {
                DOM.referenceDocumentsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">No documents found</div>';
                return;
            }
            
            DOM.referenceDocumentsList.innerHTML = '';
            
            filteredDocuments.forEach(doc => {
                const docCard = document.createElement('div');
                docCard.className = 'reference-document-item';
                docCard.style.cssText = 'padding: 1em; margin-bottom: 0.75em; border: 1px solid #e9ecef; border-radius: 6px; background: #ffffff; cursor: pointer; transition: all 0.2s ease;';
                
                const icon = getFileIcon(doc.content_type);
                
                docCard.innerHTML = `
                    <div style="display: flex; align-items: flex-start; gap: 1em;">
                        <div style="font-size: 2em; color: ${icon.color}; flex-shrink: 0;">
                            <i class="${icon.class}"></i>
                        </div>
                        <div style="flex: 1; min-width: 0;">
                            <div style="font-weight: 600; color: #233366; margin-bottom: 0.25em; font-size: 1em;">
                                ${doc.title || doc.filename}
                            </div>
                            <div style="font-size: 0.85em; color: #666; margin-bottom: 0.25em;">
                                ${doc.filename} • ${formatFileSize(doc.size)} • ${formatDateAustralian(doc.created_at)}
                            </div>
                            ${doc.description ? `<div style="font-size: 0.85em; color: #888; margin-bottom: 0.25em;">${doc.description.substring(0, 100)}${doc.description.length > 100 ? '...' : ''}</div>` : ''}
                            ${doc.author ? `<div style="font-size: 0.8em; color: #999;">Author: ${doc.author}</div>` : ''}
                            ${doc.available_for_task ? '<div style="font-size: 0.8em; color: #28a745; margin-top: 0.25em;"><i class="fas fa-check-circle"></i> Available for Task</div>' : ''}
                        </div>
                        <div style="display: flex; flex-direction: row; gap: 0.5em; flex-shrink: 0; align-items: center;">
                            ${doc.content_type.startsWith('image/') ? 
                                `<button class="reference-document-view-btn" data-doc-id="${doc.id}" style="padding: 0.4em 0.8em; font-size: 0.85em; background: #4a90e2; color: white; border: none; border-radius: 4px; cursor: pointer;">
                                    <i class="fas fa-eye"></i> View
                                </button>` :
                                `<button class="reference-document-download-btn" data-doc-id="${doc.id}" style="padding: 0.4em 0.8em; font-size: 0.85em; background: #28a745; color: white; border: none; border-radius: 4px; cursor: pointer;">
                                    <i class="fas fa-download"></i> Download
                                </button>`
                            }
                            <button class="reference-document-edit-btn" data-doc-id="${doc.id}" style="padding: 0.4em 0.8em; font-size: 0.85em; background: #6c757d; color: white; border: none; border-radius: 4px; cursor: pointer;">
                                <i class="fas fa-edit"></i> Edit
                            </button>
                            <button class="reference-document-delete-btn" data-doc-id="${doc.id}" style="padding: 0.4em 0.8em; font-size: 0.85em; background: #dc3545; color: white; border: none; border-radius: 4px; cursor: pointer;">
                                <i class="fas fa-trash"></i> Delete
                            </button>
                        </div>
                    </div>
                `;
                
                // Add event listeners
                const viewBtn = docCard.querySelector('.reference-document-view-btn');
                if (viewBtn) {
                    viewBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        viewDocument(parseInt(viewBtn.dataset.docId));
                    });
                }
                
                const downloadBtn = docCard.querySelector('.reference-document-download-btn');
                if (downloadBtn) {
                    downloadBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        downloadDocument(parseInt(downloadBtn.dataset.docId));
                    });
                }
                
                const editBtn = docCard.querySelector('.reference-document-edit-btn');
                if (editBtn) {
                    editBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        editDocument(parseInt(editBtn.dataset.docId));
                    });
                }
                
                const deleteBtn = docCard.querySelector('.reference-document-delete-btn');
                if (deleteBtn) {
                    deleteBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        deleteDocument(parseInt(deleteBtn.dataset.docId));
                    });
                }
                
                DOM.referenceDocumentsList.appendChild(docCard);
            });
        }

        function viewDocument(documentId) {
            const doc = documents.find(d => d.id === documentId);
            if (!doc) return;
            
            if (doc.content_type.startsWith('image/')) {
                // Show image in modal
                if (DOM.singleImageModal && DOM.singleImageModalImg) {
                    if (DOM.singleImageModalAudio) DOM.singleImageModalAudio.style.display = 'none';
                    if (DOM.singleImageModalVideo) DOM.singleImageModalVideo.style.display = 'none';
                    if (DOM.singleImageModalPdf) DOM.singleImageModalPdf.style.display = 'none';
                    
                    DOM.singleImageModalImg.src = `/reference-documents/${documentId}/download`;
                    DOM.singleImageModalImg.alt = doc.title || doc.filename;
                    DOM.singleImageModalImg.style.display = 'block';
                    
                    if (DOM.singleImageDetails) {
                        const details = [];
                        if (doc.title) details.push(`<strong>Title:</strong> ${doc.title}`);
                        if (doc.description) details.push(`<strong>Description:</strong> ${doc.description}`);
                        if (doc.author) details.push(`<strong>Author:</strong> ${doc.author}`);
                        if (doc.filename) details.push(`<strong>Filename:</strong> ${doc.filename}`);
                        if (doc.created_at) details.push(`<strong>Date:</strong> ${formatDateAustralian(doc.created_at)}`);
                        DOM.singleImageDetails.innerHTML = details.length > 0 ? details.join('<br>') : '';
                    }
                    
                    Modals._openModal(DOM.singleImageModal);
                }
            } else {
                // Download document
                downloadDocument(documentId);
            }
        }

        function downloadDocument(documentId) {
            window.open(`/reference-documents/${documentId}/download`, '_blank');
        }

        async function editDocument(documentId) {
            const doc = documents.find(d => d.id === documentId);
            if (!doc) return;
            
            // Populate edit form
            document.getElementById('reference-documents-edit-id').value = doc.id;
            document.getElementById('reference-documents-edit-title').value = doc.title || '';
            document.getElementById('reference-documents-edit-description').value = doc.description || '';
            document.getElementById('reference-documents-edit-author').value = doc.author || '';
            document.getElementById('reference-documents-edit-tags').value = doc.tags || '';
            document.getElementById('reference-documents-edit-categories').value = doc.categories || '';
            document.getElementById('reference-documents-edit-notes').value = doc.notes || '';
            document.getElementById('reference-documents-edit-task').checked = doc.available_for_task || false;
            
            Modals._openModal(DOM.referenceDocumentsEditModal);
        }

        async function deleteDocument(documentId) {
            if (!confirm('Are you sure you want to delete this document?')) {
                return;
            }
            
            try {
                const response = await fetch(`/reference-documents/${documentId}`, {
                    method: 'DELETE'
                });
                
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                
                await loadDocuments();
                // Reset notification flag when document is deleted
                Modals.ReferenceDocumentsNotification.reset();
            } catch (error) {
                console.error("Failed to delete document:", error);
                alert('Failed to delete document: ' + error.message);
            }
        }

        function applyFilters() {
            currentFilters.search = DOM.referenceDocumentsSearch.value.trim();
            currentFilters.category = DOM.referenceDocumentsCategoryFilter.value;
            currentFilters.contentType = DOM.referenceDocumentsContentTypeFilter.value;
            currentFilters.availableForTask = DOM.referenceDocumentsTaskFilter.checked ? true : null;
            
            loadDocuments();
        }

        function init() {
            if (DOM.referenceDocumentsSearch) {
                let searchTimeout;
                DOM.referenceDocumentsSearch.addEventListener('input', () => {
                    clearTimeout(searchTimeout);
                    searchTimeout = setTimeout(() => {
                        applyFilters();
                    }, 300);
                });
            }
            
            if (DOM.referenceDocumentsCategoryFilter) {
                DOM.referenceDocumentsCategoryFilter.addEventListener('change', applyFilters);
            }
            
            if (DOM.referenceDocumentsContentTypeFilter) {
                DOM.referenceDocumentsContentTypeFilter.addEventListener('change', applyFilters);
            }
            
            if (DOM.referenceDocumentsTaskFilter) {
                DOM.referenceDocumentsTaskFilter.addEventListener('change', applyFilters);
            }
            
            if (DOM.referenceDocumentsUploadBtn) {
                DOM.referenceDocumentsUploadBtn.addEventListener('click', () => {
                    Modals._openModal(DOM.referenceDocumentsUploadModal);
                });
            }
            
            if (DOM.closeReferenceDocumentsUploadModalBtn) {
                DOM.closeReferenceDocumentsUploadModalBtn.addEventListener('click', () => {
                    Modals._closeModal(DOM.referenceDocumentsUploadModal);
                });
            }
            
            if (DOM.referenceDocumentsUploadCancelBtn) {
                DOM.referenceDocumentsUploadCancelBtn.addEventListener('click', () => {
                    Modals._closeModal(DOM.referenceDocumentsUploadModal);
                    DOM.referenceDocumentsUploadForm.reset();
                });
            }
            
            if (DOM.referenceDocumentsUploadForm) {
                DOM.referenceDocumentsUploadForm.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    
                    const formData = new FormData();
                    const fileInput = document.getElementById('reference-documents-upload-file');
                    if (!fileInput.files[0]) {
                        alert('Please select a file');
                        return;
                    }
                    
                    formData.append('file', fileInput.files[0]);
                    formData.append('title', document.getElementById('reference-documents-upload-title').value);
                    formData.append('description', document.getElementById('reference-documents-upload-description').value);
                    formData.append('author', document.getElementById('reference-documents-upload-author').value);
                    formData.append('tags', document.getElementById('reference-documents-upload-tags').value);
                    formData.append('categories', document.getElementById('reference-documents-upload-categories').value);
                    formData.append('notes', document.getElementById('reference-documents-upload-notes').value);
                    formData.append('available_for_task', document.getElementById('reference-documents-upload-task').checked);
                    
                    try {
                        const response = await fetch('/reference-documents', {
                            method: 'POST',
                            body: formData
                        });
                        
                        if (!response.ok) {
                            const error = await response.json();
                            throw new Error(error.detail || `HTTP error! status: ${response.status}`);
                        }
                        
                        Modals._closeModal(DOM.referenceDocumentsUploadModal);
                        DOM.referenceDocumentsUploadForm.reset();
                        await loadDocuments();
                        // Reset notification flag when document is added
                        Modals.ReferenceDocumentsNotification.reset();
                    } catch (error) {
                        console.error("Failed to upload document:", error);
                        alert('Failed to upload document: ' + error.message);
                    }
                });
            }
            
            if (DOM.closeReferenceDocumentsEditModalBtn) {
                DOM.closeReferenceDocumentsEditModalBtn.addEventListener('click', () => {
                    Modals._closeModal(DOM.referenceDocumentsEditModal);
                });
            }
            
            if (DOM.referenceDocumentsEditCancelBtn) {
                DOM.referenceDocumentsEditCancelBtn.addEventListener('click', () => {
                    Modals._closeModal(DOM.referenceDocumentsEditModal);
                });
            }
            
            if (DOM.referenceDocumentsEditForm) {
                DOM.referenceDocumentsEditForm.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    
                    const documentId = parseInt(document.getElementById('reference-documents-edit-id').value);
                    const updateData = {
                        title: document.getElementById('reference-documents-edit-title').value || null,
                        description: document.getElementById('reference-documents-edit-description').value || null,
                        author: document.getElementById('reference-documents-edit-author').value || null,
                        tags: document.getElementById('reference-documents-edit-tags').value || null,
                        categories: document.getElementById('reference-documents-edit-categories').value || null,
                        notes: document.getElementById('reference-documents-edit-notes').value || null,
                        available_for_task: document.getElementById('reference-documents-edit-task').checked
                    };
                    
                    try {
                        const response = await fetch(`/reference-documents/${documentId}`, {
                            method: 'PUT',
                            headers: {
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify(updateData)
                        });
                        
                        if (!response.ok) {
                            const error = await response.json();
                            throw new Error(error.detail || `HTTP error! status: ${response.status}`);
                        }
                        
                        Modals._closeModal(DOM.referenceDocumentsEditModal);
                        await loadDocuments();
                        // Reset notification flag when document is edited
                        Modals.ReferenceDocumentsNotification.reset();
                    } catch (error) {
                        console.error("Failed to update document:", error);
                        alert('Failed to update document: ' + error.message);
                    }
                });
            }
        }

        return { init, loadDocuments };
})();


Modals.ReferenceDocumentsNotification = (() => {
        let proceedCallback = null;
        let hasShownBefore = false;
        let numberOfCalls = 0;
        const STORAGE_KEY = 'reference_documents_notification_shown';
        const STORAGE_KEY_DOCS_HASH = 'reference_documents_hash';

        async function fetchReferenceDocuments() {
            try {
                // Fetch all reference documents (not just those with available_for_task=true)
                const response = await fetch('/reference-documents');
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                return await response.json();
            } catch (error) {
                console.error('Error fetching reference documents:', error);
                return [];
            }
        }

        function getDocumentsHash(documents) {
            // Create a hash of all document IDs and their available_for_task status
            const allDocs = documents
                .map(doc => `${doc.id}:${doc.available_for_task}`)
                .sort()
                .join(',');
            return allDocs;
        }

        function renderDocumentsList(documents) {
            if (!DOM.referenceDocumentsNotificationList) return;

            if (documents.length === 0) {
                DOM.referenceDocumentsNotificationList.innerHTML = 
                    '<div style="text-align: center; padding: 1rem; color: #666;">No reference documents found.</div>';
                return;
            }

            // Separate selected and non-selected documents
            const selectedDocs = documents.filter(doc => doc.available_for_task === true);
            const nonSelectedDocs = documents.filter(doc => doc.available_for_task !== true);

            DOM.referenceDocumentsNotificationList.innerHTML = '';

            // Show selected documents section
            if (selectedDocs.length > 0) {
                const sectionHeader = document.createElement('div');
                sectionHeader.style.cssText = 'font-weight: 600; color: #28a745; margin-bottom: 0.75rem; margin-top: 0.5rem; font-size: 0.95em;';
                sectionHeader.textContent = `Selected for Chat (${selectedDocs.length}):`;
                DOM.referenceDocumentsNotificationList.appendChild(sectionHeader);

                selectedDocs.forEach(doc => {
                    const docItem = document.createElement('div');
                    docItem.style.cssText = 'padding: 0.75rem; margin-bottom: 0.5rem; border: 1px solid #28a745; border-radius: 6px; background: #f0f9f4;';
                    
                    const title = document.createElement('div');
                    title.style.cssText = 'font-weight: 600; color: #233366; margin-bottom: 0.25rem;';
                    title.textContent = doc.title || doc.filename;
                    
                    const details = document.createElement('div');
                    details.style.cssText = 'font-size: 0.85em; color: #666;';
                    details.textContent = `${doc.filename}${doc.author ? ` • ${doc.author}` : ''}`;
                    
                    docItem.appendChild(title);
                    docItem.appendChild(details);
                    DOM.referenceDocumentsNotificationList.appendChild(docItem);
                });
            }

            // Show non-selected documents section
            if (nonSelectedDocs.length > 0) {
                const sectionHeader = document.createElement('div');
                sectionHeader.style.cssText = 'font-weight: 600; color: #6c757d; margin-bottom: 0.75rem; margin-top: 1rem; font-size: 0.95em;';
                sectionHeader.textContent = `Not Selected (${nonSelectedDocs.length}):`;
                DOM.referenceDocumentsNotificationList.appendChild(sectionHeader);

                nonSelectedDocs.forEach(doc => {
                    const docItem = document.createElement('div');
                    docItem.style.cssText = 'padding: 0.75rem; margin-bottom: 0.5rem; border: 1px solid #e9ecef; border-radius: 6px; background: #f8f9fa; opacity: 0.7;';
                    
                    const title = document.createElement('div');
                    title.style.cssText = 'font-weight: 600; color: #6c757d; margin-bottom: 0.25rem;';
                    title.textContent = doc.title || doc.filename;
                    
                    const details = document.createElement('div');
                    details.style.cssText = 'font-size: 0.85em; color: #999;';
                    details.textContent = `${doc.filename}${doc.author ? ` • ${doc.author}` : ''}`;
                    
                    docItem.appendChild(title);
                    docItem.appendChild(details);
                    DOM.referenceDocumentsNotificationList.appendChild(docItem);
                });
            }

            // Show message if no documents are selected
            if (selectedDocs.length === 0) {
                const noSelectionMsg = document.createElement('div');
                noSelectionMsg.style.cssText = 'text-align: center; padding: 1rem; color: #dc3545; font-style: italic; margin-top: 0.5rem;';
                noSelectionMsg.textContent = 'No documents are currently set to be included in chat.';
                DOM.referenceDocumentsNotificationList.appendChild(noSelectionMsg);
            }
        }

        async function checkAndShow(callback) {

           // if (callback) callback();
         
            proceedCallback = callback;
            
            // Check if we should show the notification
            const documents = await fetchReferenceDocuments();
            
            const currentHash = getDocumentsHash(documents);
            const storedHash = localStorage.getItem(STORAGE_KEY_DOCS_HASH);
            //const hasShownBefore = localStorage.getItem(STORAGE_KEY) === 'true';
            
            // Show if:
            // 1. User hasn't seen it before, OR
            // 2. Documents have changed (hash differs)
            const shouldShow = !hasShownBefore || (storedHash !== currentHash || numberOfCalls > 15);
            
            if (shouldShow) {
                renderDocumentsList(documents);
                open();
                // Update hash after showing
                localStorage.setItem(STORAGE_KEY_DOCS_HASH, currentHash);
                hasShownBefore = true;
                numberOfCalls = 0;
            } else {
                numberOfCalls++;
                // No need to show, proceed directly
                if (callback) callback();
            }
        }

        function open() {
            if (DOM.referenceDocumentsNotificationModal) {
                DOM.referenceDocumentsNotificationModal.style.display = 'flex';
            }
        }

        function close() {
            if (DOM.referenceDocumentsNotificationModal) {
                DOM.referenceDocumentsNotificationModal.style.display = 'none';
            }
            proceedCallback = null;
        }

        function proceed() {
            // Mark as shown
            localStorage.setItem(STORAGE_KEY, 'true');
            
            if (proceedCallback) {
                proceedCallback();
            }
            close();
        }

        function cancel() {
            close();
            // Don't mark as shown if user cancels
        }

        function reset() {
            // Reset the flag when documents change
            localStorage.removeItem(STORAGE_KEY);
        }

        function init() {
            if (DOM.closeReferenceDocumentsNotificationModalBtn) {
                DOM.closeReferenceDocumentsNotificationModalBtn.addEventListener('click', cancel);
            }
            
            if (DOM.referenceDocumentsNotificationCancelBtn) {
                DOM.referenceDocumentsNotificationCancelBtn.addEventListener('click', cancel);
            }
            
            if (DOM.referenceDocumentsNotificationProceedBtn) {
                DOM.referenceDocumentsNotificationProceedBtn.addEventListener('click', proceed);
            }
            
            if (DOM.referenceDocumentsNotificationModal) {
                DOM.referenceDocumentsNotificationModal.addEventListener('click', (e) => {
                    if (e.target === DOM.referenceDocumentsNotificationModal) {
                        cancel();
                    }
                });
            }
        }

        return { init, checkAndShow, reset };
})();


Modals.ConversationManager = (() => {
        let currentConversationId = null;
        let currentConversationTitle = null;

        // Store conversation state
        const CONVERSATION_STORAGE_KEY = 'current_conversation_id';
        const CONVERSATION_TITLE_STORAGE_KEY = 'current_conversation_title';

        async function fetchConversations() {
            try {
                const response = await fetch('/chat/conversations');
                if (!response.ok) {
                    const errorText = await response.text();
                    console.error(`HTTP error! status: ${response.status}, body: ${errorText}`);
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const data = await response.json();
                console.log('Fetched conversations:', data);
                return data;
            } catch (error) {
                console.error('Error fetching conversations:', error);
                return [];
            }
        }

        async function createConversation(title, voice) {
            try {
                const response = await fetch('/chat/conversations', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title, voice })
                });
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                return await response.json();
            } catch (error) {
                console.error('Error creating conversation:', error);
                throw error;
            }
        }

        async function deleteConversation(conversationId) {
            try {
                const response = await fetch(`/chat/conversations/${conversationId}`, {
                    method: 'DELETE'
                });
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                return await response.json();
            } catch (error) {
                console.error('Error deleting conversation:', error);
                throw error;
            }
        }

        async function updateConversationTitle(conversationId, newTitle) {
            try {
                const response = await fetch(`/chat/conversations/${conversationId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title: newTitle })
                });
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                return await response.json();
            } catch (error) {
                console.error('Error updating conversation title:', error);
                throw error;
            }
        }

        async function getConversation(conversationId) {
            try {
                const response = await fetch(`/chat/conversations/${conversationId}`);
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                return await response.json();
            } catch (error) {
                console.error('Error getting conversation:', error);
                throw error;
            }
        }

        function renderConversationList(conversations) {
            if (!DOM.conversationListContainer) {
                console.error('conversationListContainer not found in DOM');
                return;
            }

            console.log('Rendering conversations:', conversations);
            DOM.conversationListContainer.innerHTML = '';

            if (!conversations || conversations.length === 0) {
                const noConvsMsg = document.createElement('div');
                noConvsMsg.style.cssText = 'text-align: center; padding: 2rem; color: #666;';
                noConvsMsg.textContent = 'No conversations found. Create a new one to get started!';
                DOM.conversationListContainer.appendChild(noConvsMsg);
                return;
            }

            conversations.forEach(conv => {
                const convItem = document.createElement('div');
                convItem.style.cssText = 'padding: 1rem; margin-bottom: 0.75rem; border: 1px solid #ddd; border-radius: 8px; background: #fff; cursor: pointer; transition: background 0.2s;';
                convItem.style.cursor = 'pointer';
                convItem.onmouseover = () => convItem.style.background = '#f5f5f5';
                convItem.onmouseout = () => convItem.style.background = '#fff';

                const titleRow = document.createElement('div');
                titleRow.style.cssText = 'display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;';

                const titleDiv = document.createElement('div');
                titleDiv.style.cssText = 'font-weight: 600; color: #233366; font-size: 1.05em;';
                titleDiv.textContent = conv.title;
                titleRow.appendChild(titleDiv);

                const actionsDiv = document.createElement('div');
                actionsDiv.style.cssText = 'display: flex; gap: 0.5rem;';

                // Edit title button
                const editBtn = document.createElement('button');
                editBtn.innerHTML = '<i class="fa-solid fa-pencil"></i>';
                editBtn.style.cssText = 'padding: 0.25rem 0.5rem; border: 1px solid #ddd; border-radius: 4px; background: #fff; cursor: pointer;';
                editBtn.title = 'Edit title';
                editBtn.onclick = (e) => {
                    e.stopPropagation();
                    editConversationTitle(conv.id, conv.title);
                };
                actionsDiv.appendChild(editBtn);

                // Delete button
                const deleteBtn = document.createElement('button');
                deleteBtn.innerHTML = '<i class="fa-solid fa-trash"></i>';
                deleteBtn.style.cssText = 'padding: 0.25rem 0.5rem; border: 1px solid #dc3545; border-radius: 4px; background: #fff; color: #dc3545; cursor: pointer;';
                deleteBtn.title = 'Delete conversation';
                deleteBtn.onclick = (e) => {
                    e.stopPropagation();
                    deleteConversationWithConfirm(conv.id);
                };
                actionsDiv.appendChild(deleteBtn);

                titleRow.appendChild(actionsDiv);
                convItem.appendChild(titleRow);

                const detailsDiv = document.createElement('div');
                detailsDiv.style.cssText = 'font-size: 0.85em; color: #666; margin-top: 0.25rem;';
                const lastMsgDate = conv.last_message_at ? new Date(conv.last_message_at).toLocaleString() : 'No messages yet';
                detailsDiv.textContent = `${conv.turn_count} messages • ${lastMsgDate}`;
                convItem.appendChild(detailsDiv);

                // Resume conversation on click
                convItem.onclick = () => {
                    resumeConversation(conv.id);
                };

                DOM.conversationListContainer.appendChild(convItem);
            });
        }

        async function editConversationTitle(conversationId, currentTitle) {
            const newTitle = prompt('Enter new conversation title:', currentTitle);
            if (newTitle && newTitle.trim() && newTitle !== currentTitle) {
                try {
                    await updateConversationTitle(conversationId, newTitle.trim());
                    showConversationList(); // Refresh list
                    if (currentConversationId === conversationId) {
                        currentConversationTitle = newTitle.trim();
                        updateConversationIndicator();
                    }
                } catch (error) {
                    alert(`Error updating title: ${error.message}`);
                }
            }
        }

        async function deleteConversationWithConfirm(conversationId) {
            if (!confirm('Are you sure you want to delete this conversation? This action cannot be undone.')) {
                return;
            }

            try {
                await deleteConversation(conversationId);
                if (currentConversationId === conversationId) {
                    // If deleting current conversation, clear it
                    currentConversationId = null;
                    currentConversationTitle = null;
                    updateConversationIndicator();
                    Chat.clearChat();
                }
                showConversationList(); // Refresh list
            } catch (error) {
                alert(`Error deleting conversation: ${error.message}`);
            }
        }

        async function resumeConversation(conversationId) {
            try {
                // Get conversation details with turns
                const conversation = await getConversation(conversationId);
                
                // Set current conversation
                currentConversationId = conversationId;
                currentConversationTitle = conversation.title;
                localStorage.setItem(CONVERSATION_STORAGE_KEY, conversationId.toString());
                localStorage.setItem(CONVERSATION_TITLE_STORAGE_KEY, conversation.title);
                
                // Clear chat display
                Chat.clearChat();
                
                // Load and display up to 30 messages
                const turns = conversation.turns || [];
                const displayTurns = turns.slice(-30); // Get last 30 turns
                
                displayTurns.forEach(turn => {
                    Chat.addMessage('user', turn.user_input, false);
                    Chat.addMessage('assistant', turn.response_text, true);
                });
                
                // Set voice if different
                if (conversation.voice && VoiceSelector) {
                    VoiceSelector.setVoice(conversation.voice);
                }
                
                // Update conversation indicator
                updateConversationIndicator();
                
                // Close modal
                close();
                
                // Scroll to bottom
                UI.scrollToBottom();
            } catch (error) {
                console.error('Error resuming conversation:', error);
                alert(`Error resuming conversation: ${error.message}`);
            }
        }

        async function createNewConversation() {
            if (!DOM.newConversationTitleInput || !DOM.newConversationVoiceSelect) {
                alert('New conversation form elements not found');
                return;
            }

            const title = DOM.newConversationTitleInput.value.trim();
            const voice = DOM.newConversationVoiceSelect.value;

            if (!title) {
                alert('Please enter a conversation title');
                return;
            }

            try {
                const conversation = await createConversation(title, voice);
                
                // Set as current conversation
                currentConversationId = conversation.id;
                currentConversationTitle = conversation.title;
                localStorage.setItem(CONVERSATION_STORAGE_KEY, conversation.id.toString());
                localStorage.setItem(CONVERSATION_TITLE_STORAGE_KEY, conversation.title);
                
                // Clear chat and update indicator
                Chat.clearChat();
                updateConversationIndicator();
                
                // Close modals
                close();
                if (DOM.newConversationModal) {
                    Modals._closeModal(DOM.newConversationModal);
                }
                
                // Clear form
                DOM.newConversationTitleInput.value = '';
                
                // Refresh conversation list
                showConversationList();
            } catch (error) {
                alert(`Error creating conversation: ${error.message}`);
            }
        }

        function updateConversationIndicator() {
            if (DOM.conversationIndicator) {
                if (currentConversationTitle) {
                    DOM.conversationIndicator.textContent = `Conversation: ${currentConversationTitle}`;
                    DOM.conversationIndicator.style.display = 'block';
                } else {
                    DOM.conversationIndicator.style.display = 'none';
                }
            }
        }

        function getCurrentConversationId() {
            return currentConversationId;
        }

        function clearCurrentConversation() {
            currentConversationId = null;
            currentConversationTitle = null;
            updateConversationIndicator();
        }

        function loadStoredConversation() {
            const storedId = localStorage.getItem(CONVERSATION_STORAGE_KEY);
            const storedTitle = localStorage.getItem(CONVERSATION_TITLE_STORAGE_KEY);
            if (storedId) {
                currentConversationId = parseInt(storedId);
                currentConversationTitle = storedTitle;
                updateConversationIndicator();
            }
        }

        async function showConversationList() {
            if (!DOM.conversationListModal) {
                console.error('Conversation list modal not found');
                return;
            }

            Modals._openModal(DOM.conversationListModal);
            
            // Show loading
            if (DOM.conversationListContainer) {
                DOM.conversationListContainer.innerHTML = '<div style="text-align: center; padding: 2rem;">Loading conversations...</div>';
            }

            try {
                const conversations = await fetchConversations();
                renderConversationList(conversations);
            } catch (error) {
                console.error('Error loading conversations:', error);
                if (DOM.conversationListContainer) {
                    DOM.conversationListContainer.innerHTML = `<div style="text-align: center; padding: 2rem; color: #dc3545;">Error loading conversations: ${error.message}</div>`;
                }
            }
        }

        function showNewConversationModal(e) {
            if (e) {
                e.preventDefault();
                e.stopPropagation();
            }
            if (!DOM.newConversationModal) {
                console.error('New conversation modal not found');
                return;
            }
            Modals._openModal(DOM.newConversationModal);
            
            // Set default voice if available
            if (DOM.newConversationVoiceSelect && VoiceSelector) {
                DOM.newConversationVoiceSelect.value = VoiceSelector.getSelectedVoice();
            }
        }

        function close(e) {
            if (e) {
                e.preventDefault();
                e.stopPropagation();
            }
            if (DOM.conversationListModal) {
                Modals._closeModal(DOM.conversationListModal);
            }
        }

        function init() {
            // Do not load stored conversation on page reload
            // loadStoredConversation();

            // Set up event listeners
            if (DOM.closeConversationListModalBtn) {
                DOM.closeConversationListModalBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    close(e);
                });
            }

            if (DOM.newConversationBtn) {
                DOM.newConversationBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    showNewConversationModal(e);
                });
            }

            if (DOM.createConversationBtn) {
                DOM.createConversationBtn.addEventListener('click', createNewConversation);
            }

            if (DOM.closeNewConversationModalBtn) {
                DOM.closeNewConversationModalBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (DOM.newConversationModal) {
                        Modals._closeModal(DOM.newConversationModal);
                    }
                });
            }

            // Cancel button in new conversation modal footer
            const cancelNewConversationBtn = document.getElementById('cancel-new-conversation-btn');
            if (cancelNewConversationBtn) {
                cancelNewConversationBtn.addEventListener('click', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    if (DOM.newConversationModal) {
                        Modals._closeModal(DOM.newConversationModal);
                    }
                });
            }

            if (DOM.conversationListModal) {
                DOM.conversationListModal.addEventListener('click', (e) => {
                    if (e.target === DOM.conversationListModal) {
                        close();
                    }
                });
            }

            if (DOM.newConversationModal) {
                DOM.newConversationModal.addEventListener('click', (e) => {
                    if (e.target === DOM.newConversationModal) {
                        Modals._closeModal(DOM.newConversationModal);
                    }
                });
            }
        }

        return { 
            init, 
            showConversationList, 
            resumeConversation, 
            createNewConversation,
            getCurrentConversationId,
            updateConversationIndicator,
            clearCurrentConversation
        };
})();


Modals.SubjectConfiguration = (() => {
        let currentSubjectName = null;
        let currentGender = null;
        let configurationLoaded = false;

        async function loadConfiguration() {
            try {
                const response = await fetch('/api/subject-configuration');
                if (response.ok) {
                    const config = await response.json();
                    currentSubjectName = config.subject_name;
                    currentGender = config.gender || 'Male';
                    configurationLoaded = true;
                    return config;
                } else if (response.status === 404) {
                    // Configuration doesn't exist yet
                    configurationLoaded = false;
                    return null;
                } else {
                    throw new Error(`Failed to load configuration: ${response.statusText}`);
                }
            } catch (error) {
                console.error('Error loading subject configuration:', error);
                configurationLoaded = false;
                return null;
            }
        }

        function switchTab(tabName) {
            // Hide all tab contents
            const tabContents = document.querySelectorAll('.subject-config-tab-content');
            tabContents.forEach(content => {
                content.style.display = 'none';
            });

            // Remove active class from all tabs
            const tabs = document.querySelectorAll('.subject-config-tab');
            tabs.forEach(tab => {
                tab.classList.remove('active');
            });

            // Show selected tab content (use flex for system-instructions tab so textarea fills space)
            const selectedContent = document.getElementById(`${tabName}-tab-content`);
            if (selectedContent) {
                const parent = selectedContent.closest('#system-instructions-tab');
                selectedContent.style.display = parent ? 'flex' : 'block';
            }

            // Add active class to selected tab
            const selectedTab = document.querySelector(`.subject-config-tab[data-tab="${tabName}"]`);
            if (selectedTab) {
                selectedTab.classList.add('active');
            }
        }

        function _renderWritingStyleMarkdown(text) {
            if (!DOM.writingStyleDisplay) return;
            if (!text || !text.trim()) {
                DOM.writingStyleDisplay.innerHTML = '<span style="color: #999;">No writing style summary yet. Click "Generate Writing Style" to analyze messages.</span>';
                return;
            }
            try {
                if (typeof marked !== 'undefined') {
                    DOM.writingStyleDisplay.innerHTML = marked.parse(text);
                } else {
                    DOM.writingStyleDisplay.textContent = text;
                }
            } catch (e) {
                console.error('Error rendering writing style markdown:', e);
                DOM.writingStyleDisplay.textContent = text;
            }
        }

        function _renderPsychologicalProfileMarkdown(text) {
            if (!DOM.psychologicalProfileDisplay) return;
            if (!text || !text.trim()) {
                DOM.psychologicalProfileDisplay.innerHTML = '<span style="color: #999;">No psychological profile yet. Click "Generate Psychological Profile" to analyze messages.</span>';
                return;
            }
            try {
                if (typeof marked !== 'undefined') {
                    DOM.psychologicalProfileDisplay.innerHTML = marked.parse(text);
                } else {
                    DOM.psychologicalProfileDisplay.textContent = text;
                }
            } catch (e) {
                console.error('Error rendering psychological profile markdown:', e);
                DOM.psychologicalProfileDisplay.textContent = text;
            }
        }

        async function requestWritingStyle() {
            if (!DOM.requestWritingStyleBtn || !DOM.writingStyleLoading || !DOM.writingStyleDisplay) return;
            DOM.requestWritingStyleBtn.disabled = true;
            DOM.writingStyleLoading.style.display = 'block';
            DOM.writingStyleDisplay.innerHTML = '';
            try {
                const response = await fetch('/writing-style/summarize', { method: 'POST' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.detail || 'Failed to generate writing style');
                }
                _renderWritingStyleMarkdown(data.summary || '');
            } catch (error) {
                console.error('Error requesting writing style:', error);
                DOM.writingStyleDisplay.innerHTML = `<span style="color: #c00;">Error: ${error.message}</span>`;
            } finally {
                DOM.requestWritingStyleBtn.disabled = false;
                DOM.writingStyleLoading.style.display = 'none';
            }
        }

        async function requestPsychologicalProfile() {
            if (!DOM.requestPsychologicalProfileBtn || !DOM.psychologicalProfileLoading || !DOM.psychologicalProfileDisplay) return;
            DOM.requestPsychologicalProfileBtn.disabled = true;
            DOM.psychologicalProfileLoading.style.display = 'block';
            DOM.psychologicalProfileDisplay.innerHTML = '';
            try {
                const response = await fetch('/psychological-profile/summarize', { method: 'POST' });
                const data = await response.json();
                if (!response.ok) {
                    throw new Error(data.detail || 'Failed to generate psychological profile');
                }
                _renderPsychologicalProfileMarkdown(data.profile || '');
            } catch (error) {
                console.error('Error requesting psychological profile:', error);
                DOM.psychologicalProfileDisplay.innerHTML = `<span style="color: #c00;">Error: ${error.message}</span>`;
            } finally {
                DOM.requestPsychologicalProfileBtn.disabled = false;
                DOM.psychologicalProfileLoading.style.display = 'none';
            }
        }

        async function saveConfiguration(subjectName, systemInstructions, coreSystemInstructions, gender, familyName, otherNames, emailAddresses, phoneNumbers, whatsappHandle, instagramHandle) {
            try {
                const payload = {
                    subject_name: subjectName,
                    system_instructions: systemInstructions,
                    gender: gender || 'Male',
                    family_name: familyName || null,
                    other_names: otherNames || null,
                    email_addresses: emailAddresses || null,
                    phone_numbers: phoneNumbers || null,
                    whatsapp_handle: whatsappHandle || null,
                    instagram_handle: instagramHandle || null
                };
                if (coreSystemInstructions !== undefined) {
                    payload.core_system_instructions = coreSystemInstructions;
                }
                const response = await fetch('/api/subject-configuration', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(payload)
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.detail || 'Failed to save configuration');
                }

                const config = await response.json();
                currentSubjectName = config.subject_name;
                currentGender = config.gender || 'Male';
                configurationLoaded = true;
                return config;
            } catch (error) {
                console.error('Error saving subject configuration:', error);
                throw error;
            }
        }

        function getSubjectName() {
            return currentSubjectName;
        }


        async function populateFormFromConfig(config) {
            if (!config) return;
            if (DOM.subjectNameInput) DOM.subjectNameInput.value = config.subject_name || '';
            if (DOM.subjectGenderSelect) DOM.subjectGenderSelect.value = config.gender || 'Male';
            if (DOM.familyNameInput) DOM.familyNameInput.value = config.family_name || '';
            if (DOM.otherNamesInput) DOM.otherNamesInput.value = config.other_names || '';
            if (DOM.emailAddressesInput) DOM.emailAddressesInput.value = config.email_addresses || '';
            if (DOM.phoneNumbersInput) DOM.phoneNumbersInput.value = config.phone_numbers || '';
            if (DOM.whatsappHandleInput) DOM.whatsappHandleInput.value = config.whatsapp_handle || '';
            if (DOM.instagramHandleInput) DOM.instagramHandleInput.value = config.instagram_handle || '';
            if (DOM.writingStyleDisplay) _renderWritingStyleMarkdown(config.writing_style_ai || '');
            if (DOM.psychologicalProfileDisplay) _renderPsychologicalProfileMarkdown(config.psychological_profile_ai || '');
            if (DOM.systemInstructionsTextarea) DOM.systemInstructionsTextarea.value = config.system_instructions || '';
            if (DOM.coreSystemInstructionsTextarea) {
                const coreVal = config.core_system_instructions || '';
                DOM.coreSystemInstructionsTextarea.value = coreVal;
                loadedCoreSystemInstructions = coreVal;
            }
        }

        async function loadAndPopulateForm() {
            try {
                const config = await loadConfiguration();
                if (config) {
                    await populateFormFromConfig(config);
                } else {
                    populateFormFromConfig({});
                    await loadDefaultInstructions();
                }
            } catch (error) {
                console.error('Error loading configuration:', error);
                populateFormFromConfig({});
                await loadDefaultInstructions();
            }
            switchTab('system-instructions');
        }

        async function checkAndShow() {
            const config = await loadConfiguration();
            if (!config) {
                // No configuration exists - open config overlay and switch to Subject Configuration tab
                if (DOM.configPage) {
                    DOM.configPage.style.display = 'flex';
                    const subjectTabBtn = document.querySelector('.config-tab-button[data-tab="subject-configuration"]');
                    if (subjectTabBtn) subjectTabBtn.click();
                }
            }
        }

        async function loadDefaultInstructions() {
            // Try to load from API first (in case initialization already happened)
            try {
                const config = await loadConfiguration();
                    if (config) {
                        if (DOM.systemInstructionsTextarea) {
                            DOM.systemInstructionsTextarea.value = config.system_instructions || '';
                        }
                        if (DOM.coreSystemInstructionsTextarea) {
                            const coreVal = config.core_system_instructions || '';
                            DOM.coreSystemInstructionsTextarea.value = coreVal;
                            loadedCoreSystemInstructions = coreVal;
                        }
                        if (DOM.writingStyleDisplay) {
                            _renderWritingStyleMarkdown(config.writing_style_ai || '');
                        }
                        if (DOM.psychologicalProfileDisplay) {
                            _renderPsychologicalProfileMarkdown(config.psychological_profile_ai || '');
                        }
                        return;
                    }
            } catch (err) {
                console.debug('Could not load configuration from API:', err);
            }

            // Fallback to loading from files
            if (DOM.systemInstructionsTextarea && !DOM.systemInstructionsTextarea.value) {
                try {
                    const response = await fetch('/static/data/system_instructions_chat.txt');
                    if (response.ok) {
                        const text = await response.text();
                        if (DOM.systemInstructionsTextarea) {
                            DOM.systemInstructionsTextarea.value = text;
                        }
                    }
                } catch (err) {
                    console.debug('Could not load default system instructions:', err);
                }
            }

            if (DOM.coreSystemInstructionsTextarea && !DOM.coreSystemInstructionsTextarea.value) {
                try {
                    const response = await fetch('/static/data/system_instructions_core.txt');
                    if (response.ok) {
                        const text = await response.text();
                        if (DOM.coreSystemInstructionsTextarea) {
                            DOM.coreSystemInstructionsTextarea.value = text;
                            loadedCoreSystemInstructions = text;
                        }
                    }
                } catch (err) {
                    console.debug('Could not load default core system instructions:', err);
                }
            }
        }

        let loadedCoreSystemInstructions = null;

        async function handleSave() {
            const subjectName = DOM.subjectNameInput ? DOM.subjectNameInput.value.trim() : '';
            const gender = DOM.subjectGenderSelect ? DOM.subjectGenderSelect.value : 'Male';
            const familyName = DOM.familyNameInput ? DOM.familyNameInput.value.trim() : '';
            const otherNames = DOM.otherNamesInput ? DOM.otherNamesInput.value.trim() : '';
            const emailAddresses = DOM.emailAddressesInput ? DOM.emailAddressesInput.value.trim() : '';
            const phoneNumbers = DOM.phoneNumbersInput ? DOM.phoneNumbersInput.value.trim() : '';
            const whatsappHandle = DOM.whatsappHandleInput ? DOM.whatsappHandleInput.value.trim() : '';
            const instagramHandle = DOM.instagramHandleInput ? DOM.instagramHandleInput.value.trim() : '';
            const systemInstructions = DOM.systemInstructionsTextarea ? DOM.systemInstructionsTextarea.value.trim() : '';
            const coreSystemInstructions = DOM.coreSystemInstructionsTextarea ? DOM.coreSystemInstructionsTextarea.value : '';

            if (!subjectName) {
                alert('Please enter a subject name');
                return;
            }

            if (!systemInstructions) {
                alert('Please enter system instructions');
                return;
            }

            const coreInstructionsChanged = loadedCoreSystemInstructions !== null &&
                coreSystemInstructions !== loadedCoreSystemInstructions;

            if (coreInstructionsChanged) {
                const firstConfirm = confirm(
                    'WARNING: You are about to change the Core System Instructions.\n\n' +
                    'Incorrect changes can destroy chat functionality. The system may stop working correctly.\n\n' +
                    'Are you sure you want to proceed?'
                );
                if (!firstConfirm) return;

                const secondConfirm = confirm(
                    'Final confirmation: This will overwrite the Core System Instructions.\n\n' +
                    'Are you absolutely sure you want to save these changes?'
                );
                if (!secondConfirm) return;
            }

            try {
                await saveConfiguration(subjectName, systemInstructions, coreSystemInstructions, gender, familyName, otherNames, emailAddresses, phoneNumbers, whatsappHandle, instagramHandle);

                alert('Subject configuration saved successfully!');

                window.location.reload();
            } catch (error) {
                alert(`Error saving configuration: ${error.message}`);
            }
        }

        function init() {
            // Set up event listeners
            if (DOM.saveSubjectConfigBtn) {
                DOM.saveSubjectConfigBtn.addEventListener('click', handleSave);
            }

            if (DOM.cancelSubjectConfigBtn) {
                DOM.cancelSubjectConfigBtn.addEventListener('click', () => {
                    loadAndPopulateForm();
                });
            }

            if (DOM.saveSystemInstructionsBtn) {
                DOM.saveSystemInstructionsBtn.addEventListener('click', handleSave);
            }
            if (DOM.cancelSystemInstructionsBtn) {
                DOM.cancelSystemInstructionsBtn.addEventListener('click', () => {
                    loadAndPopulateForm();
                });
            }

            // Tab switching logic
            if (DOM.subjectConfigTabs && DOM.subjectConfigTabs.length > 0) {
                DOM.subjectConfigTabs.forEach(tab => {
                    tab.addEventListener('click', () => {
                        const tabName = tab.getAttribute('data-tab');
                        if (tabName) {
                            switchTab(tabName);
                        }
                    });
                });
            }

            // Writing style generate button
            if (DOM.requestWritingStyleBtn) {
                DOM.requestWritingStyleBtn.addEventListener('click', () => requestWritingStyle());
            }
            if (DOM.requestPsychologicalProfileBtn) {
                DOM.requestPsychologicalProfileBtn.addEventListener('click', () => requestPsychologicalProfile());
            }

            // Check and show config with Subject Configuration tab on page load if no config exists
            checkAndShow();
        }

        return {
            init,
            checkAndShow,
            loadConfiguration,
            loadAndPopulateForm,
            saveConfiguration,
            getSubjectName
        };
})();


Modals.ManageKeys = (() => {
    function _normKeyringPw(s) {
        return (s == null ? '' : String(s)).trim().toLowerCase();
    }

    function _showStatus(msg, isError = false) {
        const el = document.getElementById('manage-keys-status');
        if (!el) return;
        el.textContent = msg;
        el.style.display = 'block';
        el.style.color = isError ? '#dc3545' : '#28a745';
        el.style.backgroundColor = isError ? 'rgba(220,53,69,0.1)' : 'rgba(40,167,69,0.1)';
    }

    function _closeCreateModal() {
        const modal = document.getElementById('create-trusted-key-modal');
        if (modal) modal.style.display = 'none';
        const userPw = document.getElementById('create-trusted-key-user-password');
        const masterPw = document.getElementById('create-trusted-key-master-password');
        const hintEl = document.getElementById('create-trusted-key-hint');
        const err = document.getElementById('create-trusted-key-error');
        if (userPw) userPw.value = '';
        if (masterPw) masterPw.value = '';
        if (hintEl) hintEl.value = '';
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    function _closeDeleteModal() {
        const modal = document.getElementById('delete-trusted-key-modal');
        if (modal) modal.style.display = 'none';
        const userPw = document.getElementById('delete-trusted-key-user-password');
        const masterPw = document.getElementById('delete-trusted-key-master-password');
        const err = document.getElementById('delete-trusted-key-error');
        if (userPw) userPw.value = '';
        if (masterPw) masterPw.value = '';
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    function _openCreateNewMasterKeyModal() {
        const modal = document.getElementById('create-new-master-key-modal');
        const step1 = document.getElementById('create-new-master-key-step1');
        const step2 = document.getElementById('create-new-master-key-step2');
        const cb1 = document.getElementById('create-master-key-understand-keys');
        const cb2 = document.getElementById('create-master-key-understand-data');
        const continueBtn = document.getElementById('create-new-master-key-continue');
        const pwInput = document.getElementById('create-new-master-key-password');
        const confirmInput = document.getElementById('create-new-master-key-confirm');
        const errEl = document.getElementById('create-new-master-key-error');
        if (modal) modal.style.display = 'flex';
        if (step1) step1.style.display = 'block';
        if (step2) step2.style.display = 'none';
        if (cb1) cb1.checked = false;
        if (cb2) cb2.checked = false;
        if (continueBtn) continueBtn.disabled = true;
        if (pwInput) pwInput.value = '';
        if (confirmInput) confirmInput.value = '';
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
    }

    function _closeCreateNewMasterKeyModal() {
        const modal = document.getElementById('create-new-master-key-modal');
        if (modal) modal.style.display = 'none';
    }

    function _createNewMasterKeyToStep2() {
        const step1 = document.getElementById('create-new-master-key-step1');
        const step2 = document.getElementById('create-new-master-key-step2');
        if (step1) step1.style.display = 'none';
        if (step2) step2.style.display = 'block';
    }

    function _createNewMasterKeyToStep1() {
        const step1 = document.getElementById('create-new-master-key-step1');
        const step2 = document.getElementById('create-new-master-key-step2');
        if (step1) step1.style.display = 'block';
        if (step2) step2.style.display = 'none';
        const errEl = document.getElementById('create-new-master-key-error');
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
    }

    async function _submitCreateNewMasterKey() {
        const pwInput = document.getElementById('create-new-master-key-password');
        const confirmInput = document.getElementById('create-new-master-key-confirm');
        const errEl = document.getElementById('create-new-master-key-error');
        const password = _normKeyringPw(pwInput ? pwInput.value : '');
        const confirm = _normKeyringPw(confirmInput ? confirmInput.value : '');
        if (!password) {
            if (errEl) { errEl.textContent = 'Please enter a password.'; errEl.style.display = 'block'; }
            return;
        }
        if (password !== confirm) {
            if (errEl) { errEl.textContent = 'Passwords do not match.'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        try {
            const resp = await fetch('/sensitive-data/master-key', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || `HTTP ${resp.status}`);
            }
            _closeCreateNewMasterKeyModal();
            _showStatus(data.message || 'New master key created successfully.');
        } catch (e) {
            console.error('[ManageKeys] create new master key error:', e);
            if (errEl) { errEl.textContent = e.message || 'Failed to create master key.'; errEl.style.display = 'block'; }
        }
    }

    async function _createTrustedKey() {
        const userPw = document.getElementById('create-trusted-key-user-password');
        const masterPw = document.getElementById('create-trusted-key-master-password');
        const hintEl = document.getElementById('create-trusted-key-hint');
        const errEl = document.getElementById('create-trusted-key-error');
        const userPassword = _normKeyringPw(userPw ? userPw.value : '');
        const masterPassword = _normKeyringPw(masterPw ? masterPw.value : '');
        const hint = hintEl ? hintEl.value.trim() : '';
        if (!userPassword) {
            if (errEl) { errEl.textContent = 'User password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!masterPassword) {
            if (errEl) { errEl.textContent = 'Master password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!hint) {
            if (errEl) { errEl.textContent = 'A plain-text visitor hint is required (shown on the unlock screen).'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        try {
            const resp = await fetch('/sensitive-data/trusted-key', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_password: userPassword, master_password: masterPassword, hint })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || data.error || `HTTP ${resp.status}`);
            }
            _closeCreateModal();
            _showStatus(data.message || 'Trusted key created successfully.');
        } catch (e) {
            console.error('[ManageKeys] create error:', e);
            if (errEl) { errEl.textContent = e.message || 'Failed to create trusted key.'; errEl.style.display = 'block'; }
        }
    }

    async function _loadDocKeyringCount() {
        try {
            const resp = await fetch('/reference-documents/keyring-count');
            if (!resp.ok) return;
            const data = await resp.json().catch(() => ({}));
            const display = document.getElementById('doc-keyring-count-display');
            const value = document.getElementById('doc-keyring-count-value');
            if (display) display.style.display = 'block';
            if (value) value.textContent = data.count !== undefined ? data.count : '—';
        } catch (e) {
            console.error('[ManageKeys] keyring count error:', e);
        }
    }

    let _visitorHintOrphanIds = [];

    async function _loadVisitorKeyHintsTable() {
        const loading = document.getElementById('visitor-key-hints-loading');
        const errEl = document.getElementById('visitor-key-hints-error');
        const tbody = document.getElementById('visitor-key-hints-tbody');
        const empty = document.getElementById('visitor-key-hints-empty');
        const createBtn = document.getElementById('create-visitor-key-hint-btn');
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        if (loading) loading.style.display = 'block';
        try {
            const resp = await fetch('/reference-documents/visitor-key-hints', {
                credentials: 'same-origin',
                headers: { 'Accept': 'application/json' }
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || `HTTP ${resp.status}`);
            }
            _visitorHintOrphanIds = Array.isArray(data.orphan_keyring_ids) ? data.orphan_keyring_ids : [];
            const hints = Array.isArray(data.hints) ? data.hints : [];
            if (tbody) {
                tbody.textContent = '';
                hints.forEach(h => {
                    const tr = document.createElement('tr');
                    const tdId = document.createElement('td');
                    tdId.textContent = String(h.id);
                    const tdKr = document.createElement('td');
                    tdKr.textContent = h.keyring_id != null ? String(h.keyring_id) : '';
                    const tdHint = document.createElement('td');
                    tdHint.textContent = h.hint || '';
                    tdHint.style.maxWidth = '280px';
                    tdHint.style.whiteSpace = 'pre-wrap';
                    tdHint.style.wordBreak = 'break-word';
                    const tdCreated = document.createElement('td');
                    tdCreated.textContent = h.created_at ? new Date(h.created_at).toLocaleString() : '';
                    const tdAct = document.createElement('td');
                    tdAct.style.textAlign = 'center';
                    const editBtn = document.createElement('button');
                    editBtn.type = 'button';
                    editBtn.className = 'modal-btn modal-btn-secondary';
                    editBtn.style.padding = '4px 8px';
                    editBtn.style.fontSize = '0.85em';
                    editBtn.textContent = 'Edit';
                    editBtn.addEventListener('click', () => _openEditVisitorKeyHint(h.id, h.hint || ''));
                    const delBtn = document.createElement('button');
                    delBtn.type = 'button';
                    delBtn.className = 'modal-btn';
                    delBtn.style.padding = '4px 8px';
                    delBtn.style.fontSize = '0.85em';
                    delBtn.style.marginLeft = '6px';
                    delBtn.style.backgroundColor = '#dc3545';
                    delBtn.style.color = '#fff';
                    delBtn.textContent = 'Delete';
                    delBtn.addEventListener('click', () => _deleteVisitorKeyHintRow(h.id));
                    tdAct.appendChild(editBtn);
                    tdAct.appendChild(delBtn);
                    tr.appendChild(tdId);
                    tr.appendChild(tdKr);
                    tr.appendChild(tdHint);
                    tr.appendChild(tdCreated);
                    tr.appendChild(tdAct);
                    tbody.appendChild(tr);
                });
            }
            if (empty) empty.style.display = hints.length === 0 ? 'block' : 'none';
            if (createBtn) {
                createBtn.disabled = _visitorHintOrphanIds.length === 0;
                createBtn.style.opacity = _visitorHintOrphanIds.length === 0 ? '0.55' : '1';
            }
        } catch (e) {
            console.error('[ManageKeys] visitor hints load error:', e);
            if (errEl) {
                errEl.textContent = e.message || 'Failed to load visitor hints.';
                errEl.style.display = 'block';
            }
            if (tbody) tbody.textContent = '';
            if (empty) empty.style.display = 'none';
        } finally {
            if (loading) loading.style.display = 'none';
        }
    }

    function _closeEditVisitorKeyHintModal() {
        const m = document.getElementById('edit-visitor-key-hint-modal');
        if (m) m.style.display = 'none';
        const idEl = document.getElementById('edit-visitor-key-hint-id');
        const ta = document.getElementById('edit-visitor-key-hint-text');
        const err = document.getElementById('edit-visitor-key-hint-error');
        if (idEl) idEl.value = '';
        if (ta) ta.value = '';
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    function _openEditVisitorKeyHint(id, hint) {
        const m = document.getElementById('edit-visitor-key-hint-modal');
        const idEl = document.getElementById('edit-visitor-key-hint-id');
        const ta = document.getElementById('edit-visitor-key-hint-text');
        if (idEl) idEl.value = String(id);
        if (ta) ta.value = hint;
        if (m) m.style.display = 'flex';
    }

    async function _saveEditVisitorKeyHint() {
        const idEl = document.getElementById('edit-visitor-key-hint-id');
        const ta = document.getElementById('edit-visitor-key-hint-text');
        const err = document.getElementById('edit-visitor-key-hint-error');
        const id = idEl ? parseInt(idEl.value, 10) : 0;
        const hint = ta ? ta.value.trim() : '';
        if (!id || !hint) {
            if (err) { err.textContent = 'Hint is required.'; err.style.display = 'block'; }
            return;
        }
        if (err) { err.textContent = ''; err.style.display = 'none'; }
        try {
            const resp = await fetch(`/reference-documents/visitor-key-hints/${id}`, {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                body: JSON.stringify({ hint })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.detail || `HTTP ${resp.status}`);
            _closeEditVisitorKeyHintModal();
            _showStatus(data.message || 'Hint updated.');
            _loadVisitorKeyHintsTable();
        } catch (e) {
            console.error('[ManageKeys] update hint error:', e);
            if (err) { err.textContent = e.message || 'Failed to update.'; err.style.display = 'block'; }
        }
    }

    async function _deleteVisitorKeyHintRow(id) {
        if (!window.confirm('Remove this visitor key seat entirely? The hint and key will be deleted. This cannot be undone.')) return;
        if (!window.confirm('Final confirmation: delete this visitor key?')) return;
        try {
            const resp = await fetch(`/reference-documents/visitor-key-hints/${id}`, {
                method: 'DELETE',
                credentials: 'same-origin',
                headers: { 'Accept': 'application/json' }
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.detail || `HTTP ${resp.status}`);
            _showStatus(data.message || 'Visitor key removed.');
            _loadVisitorKeyHintsTable();
            _loadDocKeyringCount();
        } catch (e) {
            console.error('[ManageKeys] delete hint row error:', e);
            _showStatus(e.message || 'Failed to delete.', true);
        }
    }

    function _closeCreateVisitorKeyHintModal() {
        const m = document.getElementById('create-visitor-key-hint-modal');
        if (m) m.style.display = 'none';
        const ta = document.getElementById('create-visitor-key-hint-text');
        const err = document.getElementById('create-visitor-key-hint-error');
        if (ta) ta.value = '';
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    function _openCreateVisitorKeyHintModal() {
        if (_visitorHintOrphanIds.length === 0) return;
        const sel = document.getElementById('create-visitor-key-hint-keyring-select');
        const m = document.getElementById('create-visitor-key-hint-modal');
        if (sel) {
            sel.textContent = '';
            _visitorHintOrphanIds.forEach(kid => {
                const opt = document.createElement('option');
                opt.value = String(kid);
                opt.textContent = `Seat ${kid}`;
                sel.appendChild(opt);
            });
        }
        const err = document.getElementById('create-visitor-key-hint-error');
        const ta = document.getElementById('create-visitor-key-hint-text');
        if (ta) ta.value = '';
        if (err) { err.textContent = ''; err.style.display = 'none'; }
        if (m) m.style.display = 'flex';
    }

    async function _saveCreateVisitorKeyHint() {
        const sel = document.getElementById('create-visitor-key-hint-keyring-select');
        const ta = document.getElementById('create-visitor-key-hint-text');
        const err = document.getElementById('create-visitor-key-hint-error');
        const keyringId = sel ? parseInt(sel.value, 10) : 0;
        const hint = ta ? ta.value.trim() : '';
        if (!keyringId || !hint) {
            if (err) { err.textContent = 'Keyring seat and hint are required.'; err.style.display = 'block'; }
            return;
        }
        if (err) { err.textContent = ''; err.style.display = 'none'; }
        try {
            const resp = await fetch('/reference-documents/visitor-key-hints', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                body: JSON.stringify({ keyring_id: keyringId, hint })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.detail || `HTTP ${resp.status}`);
            _closeCreateVisitorKeyHintModal();
            _showStatus(data.message || 'Hint created.');
            _loadVisitorKeyHintsTable();
            _loadDocKeyringCount();
        } catch (e) {
            console.error('[ManageKeys] create hint error:', e);
            if (err) { err.textContent = e.message || 'Failed to create hint.'; err.style.display = 'block'; }
        }
    }

    function _closeAddDocKeyModal() {
        const modal = document.getElementById('add-doc-key-modal');
        if (modal) modal.style.display = 'none';
        ['add-doc-key-user-password', 'add-doc-key-master-password', 'add-doc-key-hint'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.value = '';
        });
        const err = document.getElementById('add-doc-key-error');
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    function _closeRemoveDocKeyModal() {
        const modal = document.getElementById('remove-doc-key-modal');
        if (modal) modal.style.display = 'none';
        ['remove-doc-key-user-password', 'remove-doc-key-master-password'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.value = '';
        });
        const err = document.getElementById('remove-doc-key-error');
        if (err) { err.textContent = ''; err.style.display = 'none'; }
    }

    async function _addDocKey() {
        const userPw = document.getElementById('add-doc-key-user-password');
        const masterPw = document.getElementById('add-doc-key-master-password');
        const hintEl = document.getElementById('add-doc-key-hint');
        const errEl = document.getElementById('add-doc-key-error');
        const userPassword = _normKeyringPw(userPw ? userPw.value : '');
        const masterPassword = _normKeyringPw(masterPw ? masterPw.value : '');
        const hint = hintEl ? hintEl.value.trim() : '';
        if (!userPassword) {
            if (errEl) { errEl.textContent = 'User password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!masterPassword) {
            if (errEl) { errEl.textContent = 'Master password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!hint) {
            if (errEl) { errEl.textContent = 'A plain-text visitor hint is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        try {
            const resp = await fetch('/reference-documents/add-user', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_password: userPassword, master_password: masterPassword, hint })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || data.error || `HTTP ${resp.status}`);
            }
            _closeAddDocKeyModal();
            _showStatus(data.message || 'Document key added successfully.');
            _loadDocKeyringCount();
            _loadVisitorKeyHintsTable();
        } catch (e) {
            console.error('[ManageKeys] add doc key error:', e);
            if (errEl) { errEl.textContent = e.message || 'Failed to add document key.'; errEl.style.display = 'block'; }
        }
    }

    async function _removeDocKey() {
        const userPw = document.getElementById('remove-doc-key-user-password');
        const masterPw = document.getElementById('remove-doc-key-master-password');
        const errEl = document.getElementById('remove-doc-key-error');
        const userPassword = _normKeyringPw(userPw ? userPw.value : '');
        const masterPassword = _normKeyringPw(masterPw ? masterPw.value : '');
        if (!userPassword) {
            if (errEl) { errEl.textContent = 'User password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!masterPassword) {
            if (errEl) { errEl.textContent = 'Master password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        try {
            const resp = await fetch('/reference-documents/remove-user', {
                method: 'DELETE',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_password: userPassword, master_password: masterPassword })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.error || `HTTP ${resp.status}`);
            }
            _closeRemoveDocKeyModal();
            _showStatus(data.message || 'Document key removed successfully.');
            _loadDocKeyringCount();
            _loadVisitorKeyHintsTable();
        } catch (e) {
            console.error('[ManageKeys] remove doc key error:', e);
            if (errEl) { errEl.textContent = e.message || 'Failed to remove document key.'; errEl.style.display = 'block'; }
        }
    }

    async function _deleteAllVisitorDocKeys() {
        if (!window.confirm('Delete ALL visitor document keys?\n\nThe owner master key will stay. Visitor keys and their unlock hints will be removed. This cannot be undone.')) {
            return;
        }
        if (!window.confirm('Final confirmation: remove every visitor keyring seat now?')) {
            return;
        }
        try {
            const resp = await fetch('/reference-documents/visitor-keys', {
                method: 'DELETE',
                credentials: 'same-origin',
                headers: { 'Accept': 'application/json' }
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || data.error || `HTTP ${resp.status}`);
            }
            _showStatus(data.message || 'Visitor keys removed.');
            _loadDocKeyringCount();
            _loadVisitorKeyHintsTable();
        } catch (e) {
            console.error('[ManageKeys] delete all visitor keys error:', e);
            _showStatus(e.message || 'Failed to remove visitor keys.', true);
        }
    }

    async function _deleteTrustedKey() {
        const userPw = document.getElementById('delete-trusted-key-user-password');
        const masterPw = document.getElementById('delete-trusted-key-master-password');
        const errEl = document.getElementById('delete-trusted-key-error');
        const userPassword = _normKeyringPw(userPw ? userPw.value : '');
        const masterPassword = _normKeyringPw(masterPw ? masterPw.value : '');
        if (!userPassword) {
            if (errEl) { errEl.textContent = 'User password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (!masterPassword) {
            if (errEl) { errEl.textContent = 'Master password is required.'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) { errEl.textContent = ''; errEl.style.display = 'none'; }
        try {
            const resp = await fetch('/sensitive-data/trusted-key', {
                method: 'DELETE',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ user_password: userPassword, master_password: masterPassword })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.detail || `HTTP ${resp.status}`);
            }
            _closeDeleteModal();
            _showStatus(data.message || 'Trusted key deleted successfully.');
        } catch (e) {
            console.error('[ManageKeys] delete error:', e);
            if (errEl) { errEl.textContent = e.message || 'Failed to delete trusted key.'; errEl.style.display = 'block'; }
        }
    }

    function init() {
        const createBtn = document.getElementById('create-trusted-key-btn');
        if (createBtn) createBtn.addEventListener('click', () => {
            const modal = document.getElementById('create-trusted-key-modal');
            if (modal) modal.style.display = 'flex';
        });

        const deleteBtn = document.getElementById('delete-trusted-key-btn');
        if (deleteBtn) deleteBtn.addEventListener('click', () => {
            const modal = document.getElementById('delete-trusted-key-modal');
            if (modal) modal.style.display = 'flex';
        });

        const closeCreate = document.getElementById('close-create-trusted-key-modal');
        if (closeCreate) closeCreate.addEventListener('click', _closeCreateModal);

        const closeDelete = document.getElementById('close-delete-trusted-key-modal');
        if (closeDelete) closeDelete.addEventListener('click', _closeDeleteModal);

        const cancelCreate = document.getElementById('create-trusted-key-cancel');
        if (cancelCreate) cancelCreate.addEventListener('click', _closeCreateModal);

        const cancelDelete = document.getElementById('delete-trusted-key-cancel');
        if (cancelDelete) cancelDelete.addEventListener('click', _closeDeleteModal);

        const submitCreate = document.getElementById('create-trusted-key-submit');
        if (submitCreate) submitCreate.addEventListener('click', _createTrustedKey);

        const submitDelete = document.getElementById('delete-trusted-key-submit');
        if (submitDelete) submitDelete.addEventListener('click', _deleteTrustedKey);

        const createModal = document.getElementById('create-trusted-key-modal');
        if (createModal) {
            createModal.addEventListener('click', e => {
                if (e.target === createModal) _closeCreateModal();
            });
        }

        const deleteModal = document.getElementById('delete-trusted-key-modal');
        if (deleteModal) {
            deleteModal.addEventListener('click', e => {
                if (e.target === deleteModal) _closeDeleteModal();
            });
        }

        // Create New Master Key
        const createNewMasterKeyBtn = document.getElementById('create-new-master-key-btn');
        if (createNewMasterKeyBtn) createNewMasterKeyBtn.addEventListener('click', _openCreateNewMasterKeyModal);

        const closeCreateNewMasterKeyBtn = document.getElementById('close-create-new-master-key-modal');
        if (closeCreateNewMasterKeyBtn) closeCreateNewMasterKeyBtn.addEventListener('click', _closeCreateNewMasterKeyModal);

        const cancelCreateNewMasterKeyBtn = document.getElementById('create-new-master-key-cancel');
        if (cancelCreateNewMasterKeyBtn) cancelCreateNewMasterKeyBtn.addEventListener('click', _closeCreateNewMasterKeyModal);

        const cbUnderstandKeys = document.getElementById('create-master-key-understand-keys');
        const cbUnderstandData = document.getElementById('create-master-key-understand-data');
        const continueNewMasterKeyBtn = document.getElementById('create-new-master-key-continue');
        function _updateContinueEnabled() {
            if (continueNewMasterKeyBtn) {
                continueNewMasterKeyBtn.disabled = !(cbUnderstandKeys && cbUnderstandKeys.checked && cbUnderstandData && cbUnderstandData.checked);
            }
        }
        if (cbUnderstandKeys) cbUnderstandKeys.addEventListener('change', _updateContinueEnabled);
        if (cbUnderstandData) cbUnderstandData.addEventListener('change', _updateContinueEnabled);

        if (continueNewMasterKeyBtn) continueNewMasterKeyBtn.addEventListener('click', _createNewMasterKeyToStep2);

        const backNewMasterKeyBtn = document.getElementById('create-new-master-key-back');
        if (backNewMasterKeyBtn) backNewMasterKeyBtn.addEventListener('click', _createNewMasterKeyToStep1);

        const submitNewMasterKeyBtn = document.getElementById('create-new-master-key-submit');
        if (submitNewMasterKeyBtn) submitNewMasterKeyBtn.addEventListener('click', _submitCreateNewMasterKey);

        const createNewMasterKeyModal = document.getElementById('create-new-master-key-modal');
        if (createNewMasterKeyModal) {
            createNewMasterKeyModal.addEventListener('click', e => {
                if (e.target === createNewMasterKeyModal) _closeCreateNewMasterKeyModal();
            });
        }

        const pwToggleNew = document.getElementById('create-new-master-key-password-toggle');
        if (pwToggleNew) {
            pwToggleNew.addEventListener('click', () => {
                const inp = document.getElementById('create-new-master-key-password');
                if (!inp) return;
                const isPassword = inp.type === 'password';
                inp.type = isPassword ? 'text' : 'password';
                pwToggleNew.innerHTML = isPassword ? '<i class="fas fa-eye-slash"></i>' : '<i class="fas fa-eye"></i>';
                pwToggleNew.title = isPassword ? 'Hide password' : 'Show password';
            });
        }
        const confirmToggleNew = document.getElementById('create-new-master-key-confirm-toggle');
        if (confirmToggleNew) {
            confirmToggleNew.addEventListener('click', () => {
                const inp = document.getElementById('create-new-master-key-confirm');
                if (!inp) return;
                const isPassword = inp.type === 'password';
                inp.type = isPassword ? 'text' : 'password';
                confirmToggleNew.innerHTML = isPassword ? '<i class="fas fa-eye-slash"></i>' : '<i class="fas fa-eye"></i>';
                confirmToggleNew.title = isPassword ? 'Hide password' : 'Show password';
            });
        }

        const pwInputNew = document.getElementById('create-new-master-key-password');
        const confirmInputNew = document.getElementById('create-new-master-key-confirm');
        const enterHandler = e => { if (e.key === 'Enter') _submitCreateNewMasterKey(); };
        if (pwInputNew) pwInputNew.addEventListener('keydown', enterHandler);
        if (confirmInputNew) confirmInputNew.addEventListener('keydown', enterHandler);

        // ── Document Encryption Keys ──────────────────────────────────────────
        const addDocKeyBtn = document.getElementById('add-doc-key-btn');
        if (addDocKeyBtn) addDocKeyBtn.addEventListener('click', () => {
            const modal = document.getElementById('add-doc-key-modal');
            if (modal) modal.style.display = 'flex';
        });

        const removeDocKeyBtn = document.getElementById('remove-doc-key-btn');
        if (removeDocKeyBtn) removeDocKeyBtn.addEventListener('click', () => {
            const modal = document.getElementById('remove-doc-key-modal');
            if (modal) modal.style.display = 'flex';
        });

        const deleteAllVisitorKeysBtn = document.getElementById('delete-all-visitor-keys-btn');
        if (deleteAllVisitorKeysBtn) deleteAllVisitorKeysBtn.addEventListener('click', () => { _deleteAllVisitorDocKeys(); });

        const closeAddDocKey = document.getElementById('close-add-doc-key-modal');
        if (closeAddDocKey) closeAddDocKey.addEventListener('click', _closeAddDocKeyModal);

        const closeRemoveDocKey = document.getElementById('close-remove-doc-key-modal');
        if (closeRemoveDocKey) closeRemoveDocKey.addEventListener('click', _closeRemoveDocKeyModal);

        const cancelAddDocKey = document.getElementById('add-doc-key-cancel');
        if (cancelAddDocKey) cancelAddDocKey.addEventListener('click', _closeAddDocKeyModal);

        const cancelRemoveDocKey = document.getElementById('remove-doc-key-cancel');
        if (cancelRemoveDocKey) cancelRemoveDocKey.addEventListener('click', _closeRemoveDocKeyModal);

        const submitAddDocKey = document.getElementById('add-doc-key-submit');
        if (submitAddDocKey) submitAddDocKey.addEventListener('click', _addDocKey);

        const submitRemoveDocKey = document.getElementById('remove-doc-key-submit');
        if (submitRemoveDocKey) submitRemoveDocKey.addEventListener('click', _removeDocKey);

        const addDocKeyModal = document.getElementById('add-doc-key-modal');
        if (addDocKeyModal) addDocKeyModal.addEventListener('click', e => { if (e.target === addDocKeyModal) _closeAddDocKeyModal(); });

        const removeDocKeyModal = document.getElementById('remove-doc-key-modal');
        if (removeDocKeyModal) removeDocKeyModal.addEventListener('click', e => { if (e.target === removeDocKeyModal) _closeRemoveDocKeyModal(); });

        // Enter key shortcuts for doc key modals
        const addDocKeyEnter = e => { if (e.key === 'Enter') _addDocKey(); };
        const addDocKeyPw = document.getElementById('add-doc-key-user-password');
        const addDocKeyMaster = document.getElementById('add-doc-key-master-password');
        if (addDocKeyPw) addDocKeyPw.addEventListener('keydown', addDocKeyEnter);
        if (addDocKeyMaster) addDocKeyMaster.addEventListener('keydown', addDocKeyEnter);

        const removeDocKeyEnter = e => { if (e.key === 'Enter') _removeDocKey(); };
        const removeDocKeyPw = document.getElementById('remove-doc-key-user-password');
        const removeDocKeyMaster = document.getElementById('remove-doc-key-master-password');
        if (removeDocKeyPw) removeDocKeyPw.addEventListener('keydown', removeDocKeyEnter);
        if (removeDocKeyMaster) removeDocKeyMaster.addEventListener('keydown', removeDocKeyEnter);

        const visitorHintsRefresh = document.getElementById('visitor-key-hints-refresh-btn');
        if (visitorHintsRefresh) visitorHintsRefresh.addEventListener('click', () => { _loadVisitorKeyHintsTable(); });

        const createVisitorHintBtn = document.getElementById('create-visitor-key-hint-btn');
        if (createVisitorHintBtn) createVisitorHintBtn.addEventListener('click', () => { _openCreateVisitorKeyHintModal(); });

        const closeEditHint = document.getElementById('close-edit-visitor-key-hint-modal');
        if (closeEditHint) closeEditHint.addEventListener('click', _closeEditVisitorKeyHintModal);
        const cancelEditHint = document.getElementById('edit-visitor-key-hint-cancel');
        if (cancelEditHint) cancelEditHint.addEventListener('click', _closeEditVisitorKeyHintModal);
        const saveEditHint = document.getElementById('edit-visitor-key-hint-save');
        if (saveEditHint) saveEditHint.addEventListener('click', () => { _saveEditVisitorKeyHint(); });

        const editHintModal = document.getElementById('edit-visitor-key-hint-modal');
        if (editHintModal) {
            editHintModal.addEventListener('click', e => { if (e.target === editHintModal) _closeEditVisitorKeyHintModal(); });
        }

        const closeCreateHint = document.getElementById('close-create-visitor-key-hint-modal');
        if (closeCreateHint) closeCreateHint.addEventListener('click', _closeCreateVisitorKeyHintModal);
        const cancelCreateHint = document.getElementById('create-visitor-key-hint-cancel');
        if (cancelCreateHint) cancelCreateHint.addEventListener('click', _closeCreateVisitorKeyHintModal);
        const saveCreateHint = document.getElementById('create-visitor-key-hint-save');
        if (saveCreateHint) saveCreateHint.addEventListener('click', () => { _saveCreateVisitorKeyHint(); });

        const createHintModal = document.getElementById('create-visitor-key-hint-modal');
        if (createHintModal) {
            createHintModal.addEventListener('click', e => { if (e.target === createHintModal) _closeCreateVisitorKeyHintModal(); });
        }

        // Load keyring count on page ready
        _loadDocKeyringCount();
        _loadVisitorKeyHintsTable();
    }

    return { init };
})();


Modals.LLMToolsAccess = (() => {
    function _status(msg, isErr) {
        const el = document.getElementById('llm-tools-access-status');
        if (!el) return;
        if (!msg) {
            el.style.display = 'none';
            el.textContent = '';
            return;
        }
        el.textContent = msg;
        el.style.display = 'block';
        el.style.color = isErr ? '#dc3545' : '#1a7f37';
        el.style.backgroundColor = isErr ? 'rgba(220,53,69,0.1)' : 'rgba(26,127,55,0.1)';
    }

    async function load() {
        const tbody = document.getElementById('llm-tools-access-tbody');
        if (!tbody) return;
        tbody.replaceChildren();
        _status('', false);
        try {
            const res = await fetch('/api/settings/llm-tools-access', { credentials: 'same-origin' });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                _status(data.detail || 'Could not load policy. Unlock the keyring (master or visitor) first.', true);
                return;
            }
            const tools = data.tools || [];
            for (const t of tools) {
                const name = String(t.name || '');
                const tr = document.createElement('tr');
                const tdName = document.createElement('td');
                const code = document.createElement('code');
                code.textContent = name;
                tdName.appendChild(code);
                const tdDesc = document.createElement('td');
                tdDesc.style.maxWidth = '320px';
                tdDesc.style.fontSize = '0.9em';
                tdDesc.style.color = '#555';
                tdDesc.textContent = t.description || '';
                tr.appendChild(tdName);
                tr.appendChild(tdDesc);
                const flags = [
                    { key: 'no_key', val: !!t.no_key },
                    { key: 'visitor', val: !!t.visitor },
                    { key: 'master', val: !!t.master }
                ];
                for (const f of flags) {
                    const td = document.createElement('td');
                    const inp = document.createElement('input');
                    inp.type = 'checkbox';
                    inp.className = 'llm-tool-chk';
                    inp.dataset.name = name;
                    inp.dataset.flag = f.key;
                    inp.checked = f.val;
                    td.appendChild(inp);
                    tr.appendChild(td);
                }
                tbody.appendChild(tr);
            }
        } catch (e) {
            _status(e.message || 'Load failed', true);
        }
    }

    async function save() {
        const rows = {};
        document.querySelectorAll('#llm-tools-access-tbody .llm-tool-chk').forEach((chk) => {
            const name = chk.dataset.name;
            const flag = chk.dataset.flag;
            if (!name || !flag) return;
            if (!rows[name]) {
                rows[name] = { name: name, no_key: false, visitor: false, master: false };
            }
            if (flag === 'no_key') rows[name].no_key = chk.checked;
            if (flag === 'visitor') rows[name].visitor = chk.checked;
            if (flag === 'master') rows[name].master = chk.checked;
        });
        const tools = Object.values(rows);
        try {
            const res = await fetch('/api/settings/llm-tools-access', {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ tools })
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                _status(data.detail || 'Save failed', true);
                return;
            }
            _status(data.message || 'Saved.', false);
        } catch (e) {
            _status(e.message || 'Save failed', true);
        }
    }

    function init() {
        const btn = document.getElementById('llm-tools-access-save');
        if (btn) btn.addEventListener('click', () => void save());
    }

    return { init, load, save };
})();


Modals.initAll = () => {
        Modals.Suggestions.init();
        Modals.FBAlbums.init();
        Modals.FBPosts.init();
        Modals.ImageDetailModal.init();
        Modals.MultiImageDisplay.init();
        //Modals.HaveYourSay.init();
        Modals.Locations.init();
        // Modals.ImageGallery.init();
        Modals.EmailGallery.init();
        Modals.EmailEditor.init();
        if (Modals.EmailAttachments && Modals.EmailAttachments.init) Modals.EmailAttachments.init();
        Modals.NewImageGallery.init();
        Modals.SMSMessages.init();
        Modals.SingleImageDisplay.init();
        Modals.ReferenceDocuments.init();
        Modals.Contacts.init();
        Modals.Relationships.init();
        Modals.ConfirmationModal.init();
        Modals.ConversationSummary.init();
        Modals.AddInterviewee.init();
        Modals.ReferenceDocumentsNotification.init();
        Modals.ConversationManager.init();
        Modals.SubjectConfiguration.init();
        Modals.Artefacts.init();
        Modals.SensitiveData.init();
        Modals.ManageKeys.init();
        if (Modals.LLMToolsAccess && Modals.LLMToolsAccess.init) Modals.LLMToolsAccess.init();
        Modals.Profiles.init();
        if (Modals.EmailMatches && Modals.EmailMatches.init) Modals.EmailMatches.init();
        if (Modals.EmailClassifications && Modals.EmailClassifications.init) Modals.EmailClassifications.init();
        if (Modals.Interests && Modals.Interests.init) Modals.Interests.init();
        if (Modals.CustomVoices && Modals.CustomVoices.init) Modals.CustomVoices.init();
        if (Modals.EmailExclusions && Modals.EmailExclusions.init) Modals.EmailExclusions.init();
        if (Modals.PreviousResponses && Modals.PreviousResponses.init) Modals.PreviousResponses.init();
        if (Modals.SaveResponseTitle && Modals.SaveResponseTitle.init) Modals.SaveResponseTitle.init();
        if (typeof HaveAChat !== 'undefined' && HaveAChat.init) HaveAChat.init();
};

Modals.closeAll = () => {
        // Close all modals that have a close function
        try {
            if (Modals.Suggestions && Modals.Suggestions.close) Modals.Suggestions.close();
        } catch (e) { console.debug('Error closing Suggestions modal:', e); }
        
        try {
            if (Modals.FBAlbums && Modals.FBAlbums.close) Modals.FBAlbums.close();
        } catch (e) { console.debug('Error closing FBAlbums modal:', e); }
        
        try {
            if (Modals.EmailGallery && Modals.EmailGallery.close) Modals.EmailGallery.close();
        } catch (e) { console.debug('Error closing EmailGallery modal:', e); }
        
        try {
            if (Modals.EmailEditor && Modals.EmailEditor.close) Modals.EmailEditor.close();
        } catch (e) { console.debug('Error closing EmailEditor modal:', e); }
        
        try {
            if (Modals.EmailAttachments && Modals.EmailAttachments.close) Modals.EmailAttachments.close();
        } catch (e) { console.debug('Error closing EmailAttachments modal:', e); }
        
        try {
            if (Modals.NewImageGallery && Modals.NewImageGallery.close) Modals.NewImageGallery.close();
        } catch (e) { console.debug('Error closing NewImageGallery modal:', e); }
        
        try {
            if (Modals.ImageDetailModal && Modals.ImageDetailModal.close) Modals.ImageDetailModal.close();
        } catch (e) { console.debug('Error closing ImageDetailModal:', e); }
        
        try {
            if (Modals.ConversationSummary && Modals.ConversationSummary.close) Modals.ConversationSummary.close();
        } catch (e) { console.debug('Error closing ConversationSummary modal:', e); }
        
        try {
            if (Modals.SMSMessages && Modals.SMSMessages.close) Modals.SMSMessages.close();
        } catch (e) { console.debug('Error closing SMSMessages modal:', e); }
        
        try {
            if (Modals.AddInterviewee && Modals.AddInterviewee.close) Modals.AddInterviewee.close();
        } catch (e) { console.debug('Error closing AddInterviewee modal:', e); }
        
        try {
            if (Modals.Contacts && Modals.Contacts.close) Modals.Contacts.close();
        } catch (e) { console.debug('Error closing Contacts modal:', e); }
        
        try {
            if (Modals.Profiles && Modals.Profiles.close) Modals.Profiles.close();
        } catch (e) { console.debug('Error closing Profiles modal:', e); }
        
        try {
            if (Modals.Relationships && Modals.Relationships.close) Modals.Relationships.close();
        } catch (e) { console.debug('Error closing Relationships modal:', e); }
        
        try {
            if (Modals.Locations && Modals.Locations.close) Modals.Locations.close();
        } catch (e) { console.debug('Error closing Locations modal:', e); }
        
        try {
            if (Modals.ConfirmationModal && Modals.ConfirmationModal.close) Modals.ConfirmationModal.close();
        } catch (e) { console.debug('Error closing ConfirmationModal:', e); }
        
        try {
            if (Modals.PreviousResponses && Modals.PreviousResponses.close) Modals.PreviousResponses.close();
        } catch (e) { console.debug('Error closing PreviousResponses:', e); }
        
        try {
            if (Modals.SaveResponseTitle && Modals.SaveResponseTitle.close) Modals.SaveResponseTitle.close();
        } catch (e) { console.debug('Error closing SaveResponseTitle:', e); }
        
        // Close SingleImageDisplay modal directly via DOM
        try {
            if (DOM.singleImageModal) {
                Modals._closeModal(DOM.singleImageModal);
            }
        } catch (e) { console.debug('Error closing SingleImageDisplay modal:', e); }
        
        // Close MultiImageDisplay modal if it exists
        try {
            const multiImageModal = document.getElementById('multi-image-modal');
            if (multiImageModal) {
                Modals._closeModal(multiImageModal);
            }
        } catch (e) { console.debug('Error closing MultiImageDisplay modal:', e); }
        
        // Also close any other modals by checking DOM elements with modal class
        try {
            const allModals = document.querySelectorAll('.modal, [class*="modal"], [id*="modal"], [id*="Modal"]');
            allModals.forEach(modal => {
                if (modal && modal.style) {
                    const display = window.getComputedStyle(modal).display;
                    if (display === 'flex' || display === 'block') {
                        modal.style.display = 'none';
                    }
                }
            });
        } catch (e) { console.debug('Error closing modals via DOM query:', e); }
    };


// --- Email Matches (Manage Contacts) ---
Modals.EmailMatches = (() => {
    let editingId = null;

    function getEl(id) {
        return document.getElementById(id);
    }

    async function loadEmailMatches() {
        const tbody = getEl('email-matches-tbody');
        const loading = getEl('email-matches-loading');
        const tableContainer = getEl('email-matches-table-container');
        const emptyMsg = getEl('email-matches-empty-msg');
        const filterInput = getEl('email-matches-filter');
        if (!tbody || !loading) return;

        loading.style.display = 'block';
        if (tableContainer) tableContainer.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';

        try {
            const params = new URLSearchParams();
            if (filterInput && filterInput.value.trim()) {
                params.append('primary_name', filterInput.value.trim());
            }
            const response = await fetch(`/email-matches?${params.toString()}`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();

            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';

            if (!data || data.length === 0) {
                tbody.innerHTML = '';
                if (emptyMsg) emptyMsg.style.display = 'block';
                return;
            }
            if (emptyMsg) emptyMsg.style.display = 'none';

            tbody.innerHTML = data.map(row => `
                <tr style="border-bottom: 1px solid #e9ecef;">
                    <td style="padding: 8px;">${escapeHtml(row.primary_name)}</td>
                    <td style="padding: 8px;">${escapeHtml(row.email)}</td>
                    <td style="padding: 8px; text-align: center;">
                        <button type="button" class="email-match-edit-btn modal-btn modal-btn-secondary" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em;">
                            <i class="fas fa-edit"></i> Edit
                        </button>
                        <button type="button" class="email-match-delete-btn modal-btn" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em; background: #dc3545; color: white; margin-left: 4px;">
                            <i class="fas fa-trash-alt"></i> Delete
                        </button>
                    </td>
                </tr>
            `).join('');

            tbody.querySelectorAll('.email-match-edit-btn').forEach(btn => {
                btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
            });
            tbody.querySelectorAll('.email-match-delete-btn').forEach(btn => {
                btn.addEventListener('click', () => deleteMatch(parseInt(btn.dataset.id, 10)));
            });
        } catch (err) {
            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';
            tbody.innerHTML = `<tr><td colspan="3" style="padding: 1em; color: #c00;">Failed to load: ${escapeHtml(err.message)}</td></tr>`;
        }
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('email-match-modal');
        const title = getEl('email-match-modal-title');
        const primaryName = getEl('email-match-primary-name');
        const email = getEl('email-match-email');
        const errEl = getEl('email-match-modal-error');
        if (!modal || !title || !primaryName || !email) return;
        title.textContent = 'Add Email Match';
        primaryName.value = '';
        email.value = '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    async function openEditModal(id) {
        editingId = id;
        const modal = getEl('email-match-modal');
        const title = getEl('email-match-modal-title');
        const primaryName = getEl('email-match-primary-name');
        const email = getEl('email-match-email');
        const errEl = getEl('email-match-modal-error');
        if (!modal || !title || !primaryName || !email) return;

        try {
            const res = await fetch(`/email-matches/${id}`);
            if (!res.ok) throw new Error(res.statusText);
            const row = await res.json();
            title.textContent = 'Edit Email Match';
            primaryName.value = row.primary_name;
            email.value = row.email;
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            modal.style.display = 'flex';
        } catch (err) {
            if (errEl) { errEl.textContent = 'Failed to load: ' + err.message; errEl.style.display = 'block'; }
        }
    }

    function closeModal() {
        const modal = getEl('email-match-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveMatch() {
        const primaryName = getEl('email-match-primary-name');
        const email = getEl('email-match-email');
        const errEl = getEl('email-match-modal-error');
        if (!primaryName || !email) return;

        const pn = primaryName.value.trim();
        const em = email.value.trim();
        if (!pn) {
            if (errEl) { errEl.textContent = 'Primary name is required'; errEl.style.display = 'block'; }
            return;
        }
        if (!em) {
            if (errEl) { errEl.textContent = 'Email is required'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) errEl.style.display = 'none';

        try {
            if (editingId !== null) {
                const res = await fetch(`/email-matches/${editingId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ primary_name: pn, email: em })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            } else {
                const res = await fetch('/email-matches', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ primary_name: pn, email: em })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            }
            closeModal();
            loadEmailMatches();
        } catch (err) {
            if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
        }
    }

    async function deleteMatch(id) {
        const msg = 'Are you sure you want to delete this email match?';
        if (!confirm(msg)) return;
        try {
            const res = await fetch(`/email-matches/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailMatches();
        } catch (err) {
            alert('Failed to delete: ' + err.message);
        }
    }

    function init() {
        const createBtn = getEl('email-matches-create-btn');
        const refreshBtn = getEl('email-matches-refresh-btn');
        const filterInput = getEl('email-matches-filter');
        const closeBtn = getEl('close-email-match-modal');
        const cancelBtn = getEl('email-match-modal-cancel');
        const saveBtn = getEl('email-match-modal-save');

        if (createBtn) createBtn.addEventListener('click', openCreateModal);
        if (refreshBtn) refreshBtn.addEventListener('click', () => loadEmailMatches());
        if (filterInput) {
            filterInput.addEventListener('keyup', (e) => { if (e.key === 'Enter') loadEmailMatches(); });
        }
        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        if (saveBtn) saveBtn.addEventListener('click', saveMatch);

        const modal = getEl('email-match-modal');
        if (modal) {
            modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        }
    }

    return {
        load: loadEmailMatches,
        init: init
    };
})();


// --- Interests (Settings and Data Import) ---
Modals.Interests = (() => {
    let editingId = null;

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    async function loadInterests() {
        const tbody = getEl('interests-tbody');
        const loading = getEl('interests-loading');
        const tableContainer = getEl('interests-table-container');
        const emptyMsg = getEl('interests-empty-msg');
        if (!tbody || !loading) return;

        loading.style.display = 'block';
        if (tableContainer) tableContainer.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';

        try {
            const response = await fetch('/api/interests');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();

            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';

            if (!data || data.length === 0) {
                tbody.innerHTML = '';
                if (emptyMsg) emptyMsg.style.display = 'block';
                return;
            }
            if (emptyMsg) emptyMsg.style.display = 'none';

            tbody.innerHTML = data.map(row => `
                <tr style="border-bottom: 1px solid #e9ecef;">
                    <td style="padding: 8px;">${escapeHtml(row.name)}</td>
                    <td style="padding: 8px; text-align: center;">
                        <button type="button" class="interest-edit-btn modal-btn modal-btn-secondary" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em;">
                            <i class="fas fa-edit"></i> Edit
                        </button>
                        <button type="button" class="interest-delete-btn modal-btn" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em; background: #dc3545; color: white; margin-left: 4px;">
                            <i class="fas fa-trash-alt"></i> Delete
                        </button>
                    </td>
                </tr>
            `).join('');

            tbody.querySelectorAll('.interest-edit-btn').forEach(btn => {
                btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
            });
            tbody.querySelectorAll('.interest-delete-btn').forEach(btn => {
                btn.addEventListener('click', () => deleteInterest(parseInt(btn.dataset.id, 10)));
            });
        } catch (err) {
            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';
            tbody.innerHTML = `<tr><td colspan="2" style="padding: 1em; color: #c00;">Failed to load: ${escapeHtml(err.message)}</td></tr>`;
        }
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('interest-modal');
        const title = getEl('interest-modal-title');
        const nameInput = getEl('interest-name');
        const errEl = getEl('interest-modal-error');
        if (!modal || !title || !nameInput) return;
        title.textContent = 'Add Interest';
        nameInput.value = '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    async function openEditModal(id) {
        editingId = id;
        const modal = getEl('interest-modal');
        const title = getEl('interest-modal-title');
        const nameInput = getEl('interest-name');
        const errEl = getEl('interest-modal-error');
        if (!modal || !title || !nameInput) return;

        try {
            const res = await fetch(`/api/interests/${id}`);
            if (!res.ok) throw new Error(res.statusText);
            const row = await res.json();
            title.textContent = 'Edit Interest';
            nameInput.value = row.name;
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            modal.style.display = 'flex';
        } catch (err) {
            if (errEl) { errEl.textContent = 'Failed to load: ' + err.message; errEl.style.display = 'block'; }
        }
    }

    function closeModal() {
        const modal = getEl('interest-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveInterest() {
        const nameInput = getEl('interest-name');
        const errEl = getEl('interest-modal-error');
        if (!nameInput || !errEl) return;

        const name = nameInput.value.trim();
        if (!name) {
            errEl.textContent = 'Name is required';
            errEl.style.display = 'block';
            return;
        }
        errEl.style.display = 'none';

        try {
            if (editingId !== null) {
                const res = await fetch(`/api/interests/${editingId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            } else {
                const res = await fetch('/api/interests', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            }
            closeModal();
            loadInterests();
        } catch (err) {
            errEl.textContent = err.message;
            errEl.style.display = 'block';
        }
    }

    async function deleteInterest(id) {
        if (!confirm('Are you sure you want to delete this interest?')) return;
        try {
            const res = await fetch(`/api/interests/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadInterests();
        } catch (err) {
            alert('Failed to delete: ' + err.message);
        }
    }

    function init() {
        const createBtn = getEl('interests-create-btn');
        const refreshBtn = getEl('interests-refresh-btn');
        const closeBtn = getEl('close-interest-modal');
        const cancelBtn = getEl('interest-modal-cancel');
        const saveBtn = getEl('interest-modal-save');

        if (createBtn) createBtn.addEventListener('click', openCreateModal);
        if (refreshBtn) refreshBtn.addEventListener('click', () => loadInterests());
        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        if (saveBtn) saveBtn.addEventListener('click', saveInterest);

        const modal = getEl('interest-modal');
        if (modal) {
            modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        }
    }

    return {
        load: loadInterests,
        init: init
    };
})();


// --- Email Classifications (Manage Contacts) ---
Modals.EmailClassifications = (() => {
    let editingId = null;
    let classificationOptions = [];

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    async function loadClassificationOptions() {
        try {
            const res = await fetch('/email-classifications/options');
            if (!res.ok) return;
            const data = await res.json();
            classificationOptions = data.classifications || [];
        } catch (e) {
            console.error('Failed to load classification options:', e);
        }
    }

    function populateTypeFilter() {
        const sel = getEl('email-classifications-type-filter');
        if (!sel) return;
        const currentVal = sel.value;
        sel.innerHTML = '<option value="">All types</option>';
        classificationOptions.forEach(opt => {
            const o = document.createElement('option');
            o.value = opt;
            o.textContent = opt.charAt(0).toUpperCase() + opt.slice(1);
            sel.appendChild(o);
        });
        if (currentVal) sel.value = currentVal;
    }

    function populateModalDropdown() {
        const sel = getEl('email-classification-type');
        if (!sel) return;
        const currentVal = sel.value;
        sel.innerHTML = '<option value="">Select classification...</option>';
        classificationOptions.forEach(opt => {
            const o = document.createElement('option');
            o.value = opt;
            o.textContent = opt.charAt(0).toUpperCase() + opt.slice(1);
            sel.appendChild(o);
        });
        if (currentVal) sel.value = currentVal;
    }

    async function loadEmailClassifications() {
        const tbody = getEl('email-classifications-tbody');
        const loading = getEl('email-classifications-loading');
        const tableContainer = getEl('email-classifications-table-container');
        const emptyMsg = getEl('email-classifications-empty-msg');
        const filterInput = getEl('email-classifications-filter');
        const typeFilter = getEl('email-classifications-type-filter');
        if (!tbody || !loading) return;

        if (classificationOptions.length === 0) {
            await loadClassificationOptions();
            populateTypeFilter();
        }
        loading.style.display = 'block';
        if (tableContainer) tableContainer.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';

        try {
            const params = new URLSearchParams();
            if (filterInput && filterInput.value.trim()) params.append('name', filterInput.value.trim());
            if (typeFilter && typeFilter.value) params.append('classification', typeFilter.value);
            const response = await fetch(`/email-classifications?${params.toString()}`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();

            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';

            if (!data || data.length === 0) {
                tbody.innerHTML = '';
                if (emptyMsg) emptyMsg.style.display = 'block';
                return;
            }
            if (emptyMsg) emptyMsg.style.display = 'none';

            tbody.innerHTML = data.map(row => `
                <tr style="border-bottom: 1px solid #e9ecef;">
                    <td style="padding: 8px;">${escapeHtml(row.name)}</td>
                    <td style="padding: 8px;">${escapeHtml(row.classification)}</td>
                    <td style="padding: 8px; text-align: center;">
                        <button type="button" class="email-classification-edit-btn modal-btn modal-btn-secondary" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em;">
                            <i class="fas fa-edit"></i> Edit
                        </button>
                        <button type="button" class="email-classification-delete-btn modal-btn" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em; background: #dc3545; color: white; margin-left: 4px;">
                            <i class="fas fa-trash-alt"></i> Delete
                        </button>
                    </td>
                </tr>
            `).join('');

            tbody.querySelectorAll('.email-classification-edit-btn').forEach(btn => {
                btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
            });
            tbody.querySelectorAll('.email-classification-delete-btn').forEach(btn => {
                btn.addEventListener('click', () => deleteClassification(parseInt(btn.dataset.id, 10)));
            });
        } catch (err) {
            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';
            tbody.innerHTML = `<tr><td colspan="3" style="padding: 1em; color: #c00;">Failed to load: ${escapeHtml(err.message)}</td></tr>`;
        }
    }

    async function openCreateModal() {
        editingId = null;
        await loadClassificationOptions();
        populateModalDropdown();
        const modal = getEl('email-classification-modal');
        const title = getEl('email-classification-modal-title');
        const nameInput = getEl('email-classification-name');
        const typeSel = getEl('email-classification-type');
        const errEl = getEl('email-classification-modal-error');
        if (!modal || !title || !nameInput || !typeSel) return;
        title.textContent = 'Add Classification';
        nameInput.value = '';
        typeSel.value = '';
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    async function openEditModal(id) {
        editingId = id;
        await loadClassificationOptions();
        populateModalDropdown();
        const modal = getEl('email-classification-modal');
        const title = getEl('email-classification-modal-title');
        const nameInput = getEl('email-classification-name');
        const typeSel = getEl('email-classification-type');
        const errEl = getEl('email-classification-modal-error');
        if (!modal || !title || !nameInput || !typeSel) return;

        try {
            const res = await fetch(`/email-classifications/${id}`);
            if (!res.ok) throw new Error(res.statusText);
            const row = await res.json();
            title.textContent = 'Edit Classification';
            nameInput.value = row.name;
            typeSel.value = row.classification;
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            modal.style.display = 'flex';
        } catch (err) {
            if (errEl) { errEl.textContent = 'Failed to load: ' + err.message; errEl.style.display = 'block'; }
        }
    }

    function closeModal() {
        const modal = getEl('email-classification-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveClassification() {
        const nameInput = getEl('email-classification-name');
        const typeSel = getEl('email-classification-type');
        const errEl = getEl('email-classification-modal-error');
        if (!nameInput || !typeSel) return;

        const nm = nameInput.value.trim();
        const cl = typeSel.value ? typeSel.value.trim().toLowerCase() : '';
        if (!nm) {
            if (errEl) { errEl.textContent = 'Name is required'; errEl.style.display = 'block'; }
            return;
        }
        if (!cl) {
            if (errEl) { errEl.textContent = 'Please select a classification'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) errEl.style.display = 'none';

        try {
            if (editingId !== null) {
                const res = await fetch(`/email-classifications/${editingId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: nm, classification: cl })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            } else {
                const res = await fetch('/email-classifications', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: nm, classification: cl })
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            }
            closeModal();
            loadEmailClassifications();
        } catch (err) {
            if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
        }
    }

    async function deleteClassification(id) {
        if (!confirm('Are you sure you want to delete this classification?')) return;
        try {
            const res = await fetch(`/email-classifications/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailClassifications();
        } catch (err) {
            alert('Failed to delete: ' + err.message);
        }
    }

    function init() {
        const createBtn = getEl('email-classifications-create-btn');
        const refreshBtn = getEl('email-classifications-refresh-btn');
        const filterInput = getEl('email-classifications-filter');
        const typeFilter = getEl('email-classifications-type-filter');
        const closeBtn = getEl('close-email-classification-modal');
        const cancelBtn = getEl('email-classification-modal-cancel');
        const saveBtn = getEl('email-classification-modal-save');

        if (createBtn) createBtn.addEventListener('click', () => openCreateModal());
        if (refreshBtn) refreshBtn.addEventListener('click', () => loadEmailClassifications());
        if (filterInput) filterInput.addEventListener('keyup', (e) => { if (e.key === 'Enter') loadEmailClassifications(); });
        if (typeFilter) typeFilter.addEventListener('change', () => loadEmailClassifications());
        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        if (saveBtn) saveBtn.addEventListener('click', saveClassification);

        const modal = getEl('email-classification-modal');
        if (modal) modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
    }

    return {
        load: loadEmailClassifications,
        init: init,
        loadOptions: loadClassificationOptions,
        populateTypeFilter: populateTypeFilter
    };
})();


// --- Email Exclusions (Manage Contacts) ---
Modals.EmailExclusions = (() => {
    let editingId = null;

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function getTypeLabel(row) {
        if (row.name_email) return 'Name+Email pair';
        if (row.email) return 'Email pattern';
        if (row.name) return 'Name pattern';
        return '';
    }

    function toggleModalFields() {
        const typeSel = getEl('email-exclusion-type');
        const emailGroup = getEl('email-exclusion-email-group');
        const nameGroup = getEl('email-exclusion-name-group');
        if (!typeSel || !emailGroup || !nameGroup) return;
        const val = typeSel.value;
        if (val === 'email') {
            emailGroup.style.display = 'block';
            nameGroup.style.display = 'none';
        } else if (val === 'name') {
            emailGroup.style.display = 'none';
            nameGroup.style.display = 'block';
        } else {
            emailGroup.style.display = 'block';
            nameGroup.style.display = 'block';
        }
    }

    async function loadEmailExclusions() {
        const tbody = getEl('email-exclusions-tbody');
        const loading = getEl('email-exclusions-loading');
        const tableContainer = getEl('email-exclusions-table-container');
        const emptyMsg = getEl('email-exclusions-empty-msg');
        const filterInput = getEl('email-exclusions-filter');
        const typeFilter = getEl('email-exclusions-type-filter');
        if (!tbody || !loading) return;

        loading.style.display = 'block';
        if (tableContainer) tableContainer.style.display = 'none';
        if (emptyMsg) emptyMsg.style.display = 'none';

        try {
            const params = new URLSearchParams();
            if (filterInput && filterInput.value.trim()) {
                params.append('search', filterInput.value.trim());
            }
            if (typeFilter && typeFilter.value === 'name_email') {
                params.append('name_email', 'true');
            } else if (typeFilter && (typeFilter.value === 'email' || typeFilter.value === 'name')) {
                params.append('name_email', 'false');
            }
            const response = await fetch(`/email-exclusions?${params.toString()}`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            let data = await response.json();
            if (typeFilter && typeFilter.value === 'email') {
                data = data.filter(r => r.email && !r.name_email);
            } else if (typeFilter && typeFilter.value === 'name') {
                data = data.filter(r => r.name && !r.name_email);
            }

            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';

            if (!data || data.length === 0) {
                tbody.innerHTML = '';
                if (emptyMsg) emptyMsg.style.display = 'block';
                return;
            }
            if (emptyMsg) emptyMsg.style.display = 'none';

            tbody.innerHTML = data.map(row => `
                <tr style="border-bottom: 1px solid #e9ecef;">
                    <td style="padding: 8px;">${escapeHtml(row.email)}</td>
                    <td style="padding: 8px;">${escapeHtml(row.name)}</td>
                    <td style="padding: 8px;">${escapeHtml(getTypeLabel(row))}</td>
                    <td style="padding: 8px; text-align: center;">
                        <button type="button" class="email-exclusion-edit-btn modal-btn modal-btn-secondary" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em;">
                            <i class="fas fa-edit"></i> Edit
                        </button>
                        <button type="button" class="email-exclusion-delete-btn modal-btn" data-id="${row.id}" style="padding: 4px 8px; font-size: 0.85em; background: #dc3545; color: white; margin-left: 4px;">
                            <i class="fas fa-trash-alt"></i> Delete
                        </button>
                    </td>
                </tr>
            `).join('');

            tbody.querySelectorAll('.email-exclusion-edit-btn').forEach(btn => {
                btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
            });
            tbody.querySelectorAll('.email-exclusion-delete-btn').forEach(btn => {
                btn.addEventListener('click', () => deleteExclusion(parseInt(btn.dataset.id, 10)));
            });
        } catch (err) {
            loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';
            tbody.innerHTML = `<tr><td colspan="4" style="padding: 1em; color: #c00;">Failed to load: ${escapeHtml(err.message)}</td></tr>`;
        }
    }

    function openCreateModal() {
        editingId = null;
        const modal = getEl('email-exclusion-modal');
        const title = getEl('email-exclusion-modal-title');
        const typeSel = getEl('email-exclusion-type');
        const email = getEl('email-exclusion-email');
        const name = getEl('email-exclusion-name');
        const errEl = getEl('email-exclusion-modal-error');
        if (!modal || !title || !typeSel || !email || !name) return;
        title.textContent = 'Add Email Exclusion';
        typeSel.value = 'email';
        email.value = '';
        name.value = '';
        toggleModalFields();
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
        modal.style.display = 'flex';
    }

    async function openEditModal(id) {
        editingId = id;
        const modal = getEl('email-exclusion-modal');
        const title = getEl('email-exclusion-modal-title');
        const typeSel = getEl('email-exclusion-type');
        const email = getEl('email-exclusion-email');
        const name = getEl('email-exclusion-name');
        const errEl = getEl('email-exclusion-modal-error');
        if (!modal || !title || !typeSel || !email || !name) return;

        try {
            const res = await fetch(`/email-exclusions/${id}`);
            if (!res.ok) throw new Error(res.statusText);
            const row = await res.json();
            title.textContent = 'Edit Email Exclusion';
            if (row.name_email) {
                typeSel.value = 'name_email';
                email.value = row.email || '';
                name.value = row.name || '';
            } else if (row.email) {
                typeSel.value = 'email';
                email.value = row.email || '';
                name.value = '';
            } else {
                typeSel.value = 'name';
                email.value = '';
                name.value = row.name || '';
            }
            toggleModalFields();
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            modal.style.display = 'flex';
        } catch (err) {
            if (errEl) { errEl.textContent = 'Failed to load: ' + err.message; errEl.style.display = 'block'; }
        }
    }

    function closeModal() {
        const modal = getEl('email-exclusion-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    async function saveExclusion() {
        const typeSel = getEl('email-exclusion-type');
        const email = getEl('email-exclusion-email');
        const name = getEl('email-exclusion-name');
        const errEl = getEl('email-exclusion-modal-error');
        if (!typeSel || !email || !name) return;

        const typeVal = typeSel.value;
        const em = email.value.trim();
        const nm = name.value.trim();
        const name_email = typeVal === 'name_email';

        if (name_email) {
            if (!em || !nm) {
                if (errEl) { errEl.textContent = 'Both email and name are required for name+email pair'; errEl.style.display = 'block'; }
                return;
            }
        } else {
            if (typeVal === 'email') {
                if (!em) {
                    if (errEl) { errEl.textContent = 'Email is required'; errEl.style.display = 'block'; }
                    return;
                }
            } else {
                if (!nm) {
                    if (errEl) { errEl.textContent = 'Name is required'; errEl.style.display = 'block'; }
                    return;
                }
            }
        }
        if (errEl) errEl.style.display = 'none';

        const payload = {
            email: name_email || typeVal === 'email' ? em : '',
            name: name_email || typeVal === 'name' ? nm : '',
            name_email: name_email
        };

        try {
            if (editingId !== null) {
                const res = await fetch(`/email-exclusions/${editingId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            } else {
                const res = await fetch('/email-exclusions', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                });
                if (!res.ok) {
                    const d = await res.json().catch(() => ({}));
                    const msg = typeof d.detail === 'string' ? d.detail : (Array.isArray(d.detail) && d.detail[0]?.msg ? d.detail[0].msg : JSON.stringify(d.detail || res.statusText));
                    throw new Error(msg);
                }
            }
            closeModal();
            loadEmailExclusions();
        } catch (err) {
            if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
        }
    }

    async function deleteExclusion(id) {
        const msg = 'Are you sure you want to delete this email exclusion?';
        if (!confirm(msg)) return;
        try {
            const res = await fetch(`/email-exclusions/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailExclusions();
        } catch (err) {
            alert('Failed to delete: ' + err.message);
        }
    }

    function init() {
        const createBtn = getEl('email-exclusions-create-btn');
        const refreshBtn = getEl('email-exclusions-refresh-btn');
        const filterInput = getEl('email-exclusions-filter');
        const typeFilter = getEl('email-exclusions-type-filter');
        const closeBtn = getEl('close-email-exclusion-modal');
        const cancelBtn = getEl('email-exclusion-modal-cancel');
        const saveBtn = getEl('email-exclusion-modal-save');
        const typeSel = getEl('email-exclusion-type');

        if (createBtn) createBtn.addEventListener('click', openCreateModal);
        if (refreshBtn) refreshBtn.addEventListener('click', () => loadEmailExclusions());
        if (filterInput) {
            filterInput.addEventListener('keyup', (e) => { if (e.key === 'Enter') loadEmailExclusions(); });
        }
        if (typeFilter) typeFilter.addEventListener('change', () => loadEmailExclusions());
        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        if (saveBtn) saveBtn.addEventListener('click', saveExclusion);
        if (typeSel) typeSel.addEventListener('change', toggleModalFields);

        const modal = getEl('email-exclusion-modal');
        if (modal) {
            modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });
        }
    }

    return {
        load: loadEmailExclusions,
        init: init
    };
})();

Modals.SaveResponseTitle = (() => {
    let onConfirm = null;

    function close() {
        const modal = document.getElementById('save-response-title-modal');
        const input = document.getElementById('save-response-title-input');
        if (modal) modal.style.display = 'none';
        if (input) input.value = '';
        onConfirm = null;
    }

    function open(defaultTitle, onConfirmFn) {
        const modal = document.getElementById('save-response-title-modal');
        const input = document.getElementById('save-response-title-input');
        if (!modal || !input) return;
        onConfirm = onConfirmFn;
        input.value = defaultTitle || '';
        modal.style.display = 'flex';
        input.focus();
    }

    function init() {
        const modal = document.getElementById('save-response-title-modal');
        const input = document.getElementById('save-response-title-input');
        const saveBtn = document.getElementById('save-response-title-save');
        const cancelBtn = document.getElementById('save-response-title-cancel');
        const closeBtn = document.getElementById('close-save-response-title-modal');

        const handleSave = () => {
            const title = input?.value?.trim() || '';
            if (!title) return;
            const callback = onConfirm;
            close();
            if (typeof callback === 'function') callback(title);
        };

        if (saveBtn) saveBtn.addEventListener('click', handleSave);
        if (cancelBtn) cancelBtn.addEventListener('click', close);
        if (closeBtn) closeBtn.addEventListener('click', close);
        if (input) input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') handleSave();
            if (e.key === 'Escape') close();
        });
        if (modal) modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
    }

    return { open, close, init };
})();

Modals.PreviousResponses = (() => {
    let currentId = null;

    function _formatDateDMY(dateString) {
        if (!dateString) return '';
        try {
            const d = new Date(dateString);
            if (isNaN(d.getTime())) return '';
            return d.toLocaleString('en-GB', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });
        } catch (e) { return ''; }
    }

    function _esc(s) {
        return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
    }

    function showListView() {
        document.getElementById('previous-responses-list-view').style.display = 'block';
        document.getElementById('previous-responses-detail-view').style.display = 'none';
    }

    function showDetailView() {
        document.getElementById('previous-responses-list-view').style.display = 'none';
        document.getElementById('previous-responses-detail-view').style.display = 'flex';
    }

    async function loadList() {
        const listEl = document.getElementById('previous-responses-list');
        const emptyEl = document.getElementById('previous-responses-empty');
        listEl.innerHTML = '';
        try {
            const res = await fetch('/api/saved-responses');
            if (!res.ok) throw new Error(await res.text());
            const items = await res.json();
            if (items.length === 0) {
                emptyEl.style.display = 'block';
                return;
            }
            emptyEl.style.display = 'none';
            items.forEach(item => {
                const li = document.createElement('li');
                li.style.cssText = 'padding: 12px 16px; border-bottom: 1px solid #dee2e6; cursor: pointer;';
                li.onmouseover = () => { li.style.backgroundColor = '#f0f0f0'; };
                li.onmouseout = () => { li.style.backgroundColor = ''; };
                const topRow = document.createElement('div');
                topRow.style.cssText = 'display: flex; justify-content: space-between; align-items: center;';
                const titleSpan = document.createElement('span');
                titleSpan.textContent = item.title;
                titleSpan.style.fontWeight = '500';
                const dateSpan = document.createElement('span');
                dateSpan.textContent = _formatDateDMY(item.created_at);
                dateSpan.style.color = '#666'; dateSpan.style.fontSize = '0.9em';
                topRow.appendChild(titleSpan);
                topRow.appendChild(dateSpan);
                li.appendChild(topRow);
                const metaRow = document.createElement('div');
                metaRow.style.cssText = 'font-size: 0.85em; color: #888; margin-top: 4px;';
                const metaParts = [];
                if (item.voice) metaParts.push('Voice: ' + item.voice);
                if (item.llm_provider) metaParts.push('LLM: ' + item.llm_provider);
                metaRow.textContent = metaParts.length ? metaParts.join(' · ') : '';
                li.appendChild(metaRow);
                li.addEventListener('click', () => openDetail(item.id));
                listEl.appendChild(li);
            });
        } catch (e) {
            emptyEl.textContent = 'Error loading: ' + e.message;
            emptyEl.style.display = 'block';
        }
    }

    async function openDetail(id) {
        currentId = id;
        try {
            const res = await fetch(`/api/saved-responses/${id}`);
            if (!res.ok) throw new Error(await res.text());
            const item = await res.json();
            document.getElementById('previous-responses-detail-title').textContent = item.title;
            const metaEl = document.getElementById('previous-responses-detail-meta');
            const metaParts = [];
            if (item.created_at) metaParts.push('Saved: ' + _formatDateDMY(item.created_at));
            if (item.voice) metaParts.push('Voice: ' + item.voice);
            if (item.llm_provider) metaParts.push('LLM: ' + item.llm_provider);
            metaEl.textContent = metaParts.length ? metaParts.join(' · ') : '';
            const contentEl = document.getElementById('previous-responses-detail-content');
            contentEl.innerHTML = marked.parse(item.content || '');
            showDetailView();
        } catch (e) {
            console.error('Failed to load saved response:', e);
        }
    }

    function close() {
        const modal = document.getElementById('previous-responses-modal');
        if (modal) modal.style.display = 'none';
        showListView();
    }

    async function open() {
        const modal = document.getElementById('previous-responses-modal');
        if (!modal) return;
        await loadList();
        showListView();
        modal.style.display = 'flex';
    }

    function init() {
        const sidebarBtn = document.getElementById('previous-responses-sidebar-btn');
        const closeBtn = document.getElementById('close-previous-responses-modal');
        const backBtn = document.getElementById('previous-responses-back-btn');
        const deleteBtn = document.getElementById('previous-responses-delete-btn');
        const modal = document.getElementById('previous-responses-modal');

        if (sidebarBtn) sidebarBtn.addEventListener('click', () => open());
        if (closeBtn) closeBtn.addEventListener('click', close);
        if (backBtn) backBtn.addEventListener('click', () => { showListView(); });
        if (deleteBtn) deleteBtn.addEventListener('click', async () => {
            if (!currentId || !confirm('Delete this saved response?')) return;
            try {
                const res = await fetch(`/api/saved-responses/${currentId}`, { method: 'DELETE' });
                if (!res.ok) throw new Error(await res.text());
                showListView();
                await loadList();
            } catch (e) {
                console.error('Delete failed:', e);
            }
        });
        if (modal) modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
    }

    return { open, close, init };
})();

Modals.AppConfig = (() => {
    let loaded = false;

    function setStatus(msg, color) {
        const el = document.getElementById('cfg-status');
        if (el) { el.textContent = msg; el.style.color = color || '#666'; }
    }

    function _renderRow(cfg) {
        const tr = document.createElement('tr');
        tr.style.borderBottom = '1px solid #dee2e6';
        if (cfg.is_mandatory) tr.style.backgroundColor = '#fff3e0';
        tr.dataset.key = cfg.key;

        const valueId = `cfg-val-${cfg.key.replace(/[^a-zA-Z0-9]/g, '_')}`;

        tr.innerHTML = `
            <td style="padding:8px; font-family:monospace; word-break:break-all">${_esc(cfg.key)}</td>
            <td style="padding:8px">
                <input class="modal-input" id="${valueId}" value="${_esc(cfg.value || '')}" style="width:100%">
            </td>
            <td style="padding:8px; text-align:center">
                <input type="checkbox" ${cfg.is_mandatory ? 'checked disabled' : 'disabled'}>
            </td>
            <td style="padding:8px; color:#666; font-size:12px">${_esc(cfg.description || '')}</td>
            <td style="padding:8px; white-space:nowrap">
                <button class="modal-btn modal-btn-primary" style="padding:4px 10px; font-size:12px"
                    onclick="Modals.AppConfig.update('${_esc(cfg.key)}', '${valueId}')">Save</button>
                <button class="modal-btn modal-btn-secondary" style="padding:4px 10px; font-size:12px; margin-left:4px"
                    onclick="Modals.AppConfig.delete('${_esc(cfg.key)}')">Delete</button>
            </td>`;
        return tr;
    }

    function _esc(s) {
        return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
    }

    async function load() {
        try {
            setStatus('Loading…');
            const res = await fetch('/api/configuration');
            const rawText = await res.text();
            if (!res.ok) {
                let detail = rawText;
                try {
                    const j = JSON.parse(rawText);
                    if (j && j.detail) detail = j.detail;
                } catch (_) { /* use raw */ }
                throw new Error(detail || `HTTP ${res.status}`);
            }
            let rows;
            try {
                rows = JSON.parse(rawText);
            } catch (e) {
                throw new Error('Invalid JSON from /api/configuration');
            }
            if (!Array.isArray(rows)) {
                console.error('AppConfig.load: expected array, got', typeof rows, rows);
                throw new Error('Server returned non-array JSON');
            }
            rows.sort((a, b) => (b.is_mandatory ? 1 : 0) - (a.is_mandatory ? 1 : 0));
            const tbody = document.getElementById('cfg-table-body');
            if (!tbody) {
                setStatus('Configuration table not found in page.', '#c00');
                return;
            }
            tbody.innerHTML = '';
            rows.forEach(cfg => tbody.appendChild(_renderRow(cfg)));
            if (rows.length === 0) {
                setStatus('No keys in database yet — use “Seed from .env” to add known keys, or add a row above.', '#666');
            } else {
                setStatus(`${rows.length} key(s) loaded.`);
            }
            loaded = true;
        } catch (e) {
            setStatus('Error loading configuration: ' + e.message, '#c00');
        }
    }

    async function save() {
        const key = (document.getElementById('cfg-new-key')?.value || '').trim();
        const value = document.getElementById('cfg-new-value')?.value || '';
        const is_mandatory = document.getElementById('cfg-new-mandatory')?.checked || false;
        if (!key) { setStatus('Key must not be empty.', '#c00'); return; }
        try {
            setStatus('Saving…');
            const res = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ key, value, is_mandatory })
            });
            if (!res.ok) throw new Error(await res.text());
            document.getElementById('cfg-new-key').value = '';
            document.getElementById('cfg-new-value').value = '';
            document.getElementById('cfg-new-mandatory').checked = false;
            setStatus(`Saved '${key}'.`, '#2a7a2a');
            await load();
        } catch (e) {
            setStatus('Error saving: ' + e.message, '#c00');
        }
    }

    async function update(key, inputId) {
        const value = document.getElementById(inputId)?.value ?? '';
        try {
            setStatus('Saving…');
            const res = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ key, value })
            });
            if (!res.ok) throw new Error(await res.text());
            setStatus(`Saved '${key}'.`, '#2a7a2a');
            await load();
        } catch (e) {
            setStatus('Error saving: ' + e.message, '#c00');
        }
    }

    async function deleteKey(key) {
        if (!confirm(`Delete '${key}' from database? (Will revert to .env value.)`)) return;
        try {
            setStatus('Deleting…');
            const res = await fetch(`/api/configuration/${encodeURIComponent(key)}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(await res.text());
            setStatus(`Deleted '${key}'.`, '#2a7a2a');
            await load();
        } catch (e) {
            setStatus('Error deleting: ' + e.message, '#c00');
        }
    }

    async function seed() {
        try {
            setStatus('Seeding from .env…');
            const res = await fetch('/api/configuration/seed', { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            const data = await res.json();
            setStatus(data.message, '#2a7a2a');
            await load();
        } catch (e) {
            setStatus('Error seeding: ' + e.message, '#c00');
        }
    }

    return {
        load,
        save,
        update,
        delete: deleteKey,
        seed,
    };
})();

// Inline onclick in index.template.html uses AppConfig.* — must be on window.
window.AppConfig = Modals.AppConfig;

// --- Custom Voices (Settings Tab) ---
Modals.CustomVoices = (() => {
    const getEl = id => document.getElementById(id);
    let editingId = null;

    async function loadCustomVoices() {
        const loadingEl = getEl('custom-voices-loading');
        const tbody = getEl('custom-voices-tbody');
        const emptyMsg = getEl('custom-voices-empty-msg');
        if (loadingEl) loadingEl.style.display = 'block';
        if (tbody) tbody.innerHTML = '';
        try {
            const res = await fetch('/api/voices/custom');
            if (!res.ok) throw new Error(res.statusText);
            const rows = await res.json();
            if (tbody) {
                rows.forEach(row => {
                    const tr = document.createElement('tr');
                    tr.innerHTML = `
                        <td style="padding: 8px;">${_esc(row.name)}</td>
                        <td style="padding: 8px; color: #666;">${_esc(row.description || '')}</td>
                        <td style="padding: 8px; text-align: center;">${row.creativity.toFixed(1)}</td>
                        <td style="padding: 8px; text-align: center;">
                            <button class="modal-btn modal-btn-secondary" style="padding: 4px 10px; font-size: 0.85em;" onclick="Modals.CustomVoices.openEdit(${row.id})">Edit</button>
                            <button class="modal-btn" style="padding: 4px 10px; font-size: 0.85em; background: #e74c3c; color: #fff; border: none; border-radius: 4px; cursor: pointer;" onclick="Modals.CustomVoices.deleteVoice(${row.id})">Delete</button>
                        </td>`;
                    tbody.appendChild(tr);
                });
            }
            if (emptyMsg) emptyMsg.style.display = rows.length === 0 ? 'block' : 'none';
        } catch (err) {
            if (tbody) tbody.innerHTML = `<tr><td colspan="4" style="padding:1em;color:#c0392b;">Failed to load: ${_esc(err.message)}</td></tr>`;
        } finally {
            if (loadingEl) loadingEl.style.display = 'none';
        }
    }

    function _esc(str) {
        return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
    }

    function openCreate() {
        editingId = null;
        const title = getEl('custom-voice-edit-modal-title');
        if (title) title.textContent = 'New Custom Voice';
        _clearForm();
        const modal = getEl('custom-voice-edit-modal');
        if (modal) modal.style.display = 'flex';
    }

    async function openEdit(id) {
        editingId = id;
        const modal = getEl('custom-voice-edit-modal');
        const errEl = getEl('custom-voice-edit-error');
        try {
            const res = await fetch(`/api/voices/custom`);
            if (!res.ok) throw new Error(res.statusText);
            const rows = await res.json();
            const row = rows.find(r => r.id === id);
            if (!row) throw new Error('Voice not found');
            getEl('custom-voice-edit-modal-title').textContent = 'Edit Custom Voice';
            getEl('custom-voice-edit-id').value = id;
            getEl('custom-voice-edit-name').value = row.name;
            getEl('custom-voice-edit-description').value = row.description || '';
            getEl('custom-voice-edit-instructions').value = row.instructions;
            const creativityInput = getEl('custom-voice-edit-creativity');
            if (creativityInput) { creativityInput.value = row.creativity; }
            const display = getEl('custom-voice-edit-creativity-display');
            if (display) display.textContent = row.creativity.toFixed(1);
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            if (modal) modal.style.display = 'flex';
        } catch (err) {
            alert('Failed to load voice: ' + err.message);
        }
    }

    function closeModal() {
        const modal = getEl('custom-voice-edit-modal');
        if (modal) modal.style.display = 'none';
        editingId = null;
    }

    function _clearForm() {
        const fields = ['custom-voice-edit-id','custom-voice-edit-name','custom-voice-edit-description','custom-voice-edit-instructions'];
        fields.forEach(f => { const el = getEl(f); if (el) el.value = ''; });
        const c = getEl('custom-voice-edit-creativity');
        if (c) c.value = 0.5;
        const d = getEl('custom-voice-edit-creativity-display');
        if (d) d.textContent = '0.5';
        const errEl = getEl('custom-voice-edit-error');
        if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
    }

    async function saveVoice() {
        const errEl = getEl('custom-voice-edit-error');
        const name = (getEl('custom-voice-edit-name')?.value || '').trim();
        const description = (getEl('custom-voice-edit-description')?.value || '').trim();
        const instructions = (getEl('custom-voice-edit-instructions')?.value || '').trim();
        const creativity = parseFloat(getEl('custom-voice-edit-creativity')?.value || '0.5');
        if (!name) {
            if (errEl) { errEl.textContent = 'Name is required'; errEl.style.display = 'block'; }
            return;
        }
        if (!instructions) {
            if (errEl) { errEl.textContent = 'Instructions are required'; errEl.style.display = 'block'; }
            return;
        }
        if (errEl) errEl.style.display = 'none';
        try {
            let res;
            if (editingId !== null) {
                res = await fetch(`/api/voices/${editingId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name, description, instructions, creativity })
                });
            } else {
                res = await fetch('/api/voices', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name, description, instructions, creativity })
                });
            }
            if (!res.ok) {
                const d = await res.json().catch(() => ({}));
                const msg = typeof d.detail === 'string' ? d.detail : JSON.stringify(d.detail || res.statusText);
                throw new Error(msg);
            }
            closeModal();
            loadCustomVoices();
            // Refresh the voice dropdowns to include the new/updated voice
            if (typeof VoiceSelector !== 'undefined' && VoiceSelector.loadVoices) {
                VoiceSelector.loadVoices();
            }
        } catch (err) {
            if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
        }
    }

    async function deleteVoice(id) {
        if (!confirm('Delete this custom voice? This cannot be undone.')) return;
        try {
            const res = await fetch(`/api/voices/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadCustomVoices();
            if (typeof VoiceSelector !== 'undefined' && VoiceSelector.loadVoices) {
                VoiceSelector.loadVoices();
            }
        } catch (err) {
            alert('Failed to delete: ' + err.message);
        }
    }

    function init() {
        const createBtn = getEl('custom-voices-create-btn');
        const refreshBtn = getEl('custom-voices-refresh-btn');
        const closeBtn = getEl('close-custom-voice-edit-modal');
        const cancelBtn = getEl('custom-voice-edit-cancel-btn');
        const saveBtn = getEl('custom-voice-edit-save-btn');

        if (createBtn) createBtn.addEventListener('click', openCreate);
        if (refreshBtn) refreshBtn.addEventListener('click', () => loadCustomVoices());
        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        if (saveBtn) saveBtn.addEventListener('click', saveVoice);

        const modal = getEl('custom-voice-edit-modal');
        if (modal) modal.addEventListener('click', e => { if (e.target === modal) closeModal(); });
    }

    return { init, load: loadCustomVoices, openEdit, deleteVoice };
})();

