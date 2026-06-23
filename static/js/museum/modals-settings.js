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

        function updateReferenceDocumentsUploadFileList() {
            const selectedPathInput = document.getElementById('reference-documents-upload-path');
            const listEl = document.getElementById('reference-documents-upload-file-list');
            if (!selectedPathInput || !listEl) return;
            const selectedPath = (selectedPathInput.value || '').trim();
            listEl.innerHTML = '';
            if (!selectedPath) {
                listEl.hidden = true;
                return;
            }
            listEl.hidden = false;
            const li = document.createElement('li');
            li.textContent = selectedPath;
            listEl.appendChild(li);
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

        function refreshChatContextStatus() {
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
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

        async function setAvailableForTaskOnServer(docId, checked, checkboxEl) {
            checkboxEl.disabled = true;
            try {
                const response = await fetch(`/reference-documents/${docId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ available_for_task: checked })
                });
                if (!response.ok) {
                    const errText = await response.text().catch(() => '');
                    let msg = errText || `HTTP ${response.status}`;
                    try {
                        const j = JSON.parse(errText);
                        if (j.detail) msg = typeof j.detail === 'string' ? j.detail : JSON.stringify(j.detail);
                    } catch (_) { /* plain text */ }
                    throw new Error(msg);
                }
                const updated = await response.json();
                const v = updated.available_for_task === true;
                documents.forEach((d) => {
                    if (d.id === docId) d.available_for_task = v;
                });
                checkboxEl.checked = v;
                Modals.ReferenceDocumentsNotification.reset();
                refreshChatContextStatus();
            } catch (error) {
                console.error('Failed to update available_for_task:', error);
                checkboxEl.checked = !checked;
                await AppDialogs.showAppAlert('Error', 'Could not update AI availability: ' + (error.message || 'Unknown error'));
            } finally {
                checkboxEl.disabled = false;
            }
        }

        async function setIncludeInSystemPromptOnServer(docId, checked, checkboxEl) {
            checkboxEl.disabled = true;
            try {
                const response = await fetch(`/reference-documents/${docId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ include_in_system_prompt: checked })
                });
                if (!response.ok) {
                    const errText = await response.text().catch(() => '');
                    let msg = errText || `HTTP ${response.status}`;
                    try {
                        const j = JSON.parse(errText);
                        if (j.detail) msg = typeof j.detail === 'string' ? j.detail : JSON.stringify(j.detail);
                    } catch (_) { /* plain text */ }
                    throw new Error(msg);
                }
                const updated = await response.json();
                const v = updated.include_in_system_prompt === true;
                documents.forEach((d) => {
                    if (d.id === docId) d.include_in_system_prompt = v;
                });
                checkboxEl.checked = v;
                Modals.ReferenceDocumentsNotification.reset();
                refreshChatContextStatus();
            } catch (error) {
                console.error('Failed to update include_in_system_prompt:', error);
                checkboxEl.checked = !checked;
                await AppDialogs.showAppAlert('Error', 'Could not update system prompt inclusion: ' + (error.message || 'Unknown error'));
            } finally {
                checkboxEl.disabled = false;
            }
        }

        function renderDocuments() {
            if (!DOM.referenceDocumentsList) return;

            if (filteredDocuments.length === 0) {
                DOM.referenceDocumentsList.innerHTML = '<div class="reference-documents-empty-state">No documents found</div>';
                return;
            }

            const wrap = document.createElement('div');
            wrap.className = 'reference-documents-table-wrap';

            const table = document.createElement('table');
            table.className = 'reference-documents-table';

            const thead = document.createElement('thead');
            const headTr = document.createElement('tr');
            [
                { text: 'Document', scope: 'col', className: '' },
                { text: 'Size', scope: 'col', className: 'reference-documents-col-narrow' },
                { text: 'Added', scope: 'col', className: 'reference-documents-col-date' },
                {
                    text: 'Available to the AI',
                    scope: 'col',
                    className: 'reference-documents-col-ai',
                    title: 'When enabled, AI tools may read this document where your settings allow'
                },
                {
                    text: 'System prompt',
                    scope: 'col',
                    className: 'reference-documents-col-ai',
                    title: 'Include full text in every chat system prompt; excluded from list-reference-document tools'
                },
                { text: 'Actions', scope: 'col', className: 'reference-documents-col-actions' }
            ].forEach((col) => {
                const th = document.createElement('th');
                th.scope = col.scope;
                th.textContent = col.text;
                if (col.className) th.className = col.className;
                if (col.title) th.title = col.title;
                headTr.appendChild(th);
            });
            thead.appendChild(headTr);

            const tbody = document.createElement('tbody');

            filteredDocuments.forEach((doc) => {
                const icon = getFileIcon(doc.content_type || '');
                const isImage = (doc.content_type || '').startsWith('image/');

                const tr = document.createElement('tr');
                tr.className = 'reference-document-row';

                const tdDoc = document.createElement('td');
                tdDoc.className = 'reference-document-doc-cell';
                const docInner = document.createElement('div');
                docInner.className = 'reference-document-doc-inner';
                const iconWrap = document.createElement('div');
                iconWrap.className = 'reference-document-doc-icon';
                iconWrap.style.color = icon.color;
                const iconI = document.createElement('i');
                iconI.className = icon.class;
                iconI.setAttribute('aria-hidden', 'true');
                iconWrap.appendChild(iconI);
                const textWrap = document.createElement('div');
                textWrap.className = 'reference-document-doc-text';
                const titleEl = document.createElement('div');
                titleEl.className = 'reference-document-title';
                titleEl.textContent = doc.title || doc.filename || '';
                textWrap.appendChild(titleEl);
                const subEl = document.createElement('div');
                subEl.className = 'reference-document-sub';
                subEl.textContent = doc.filename || '';
              //  textWrap.appendChild(subEl);
                if (doc.description) {
                    const descEl = document.createElement('div');
                    descEl.className = 'reference-document-desc';
                    const snip = doc.description.length > 120 ? doc.description.substring(0, 120) + '…' : doc.description;
                    descEl.textContent = snip;
                    textWrap.appendChild(descEl);
                }
                docInner.appendChild(iconWrap);
                docInner.appendChild(textWrap);
                tdDoc.appendChild(docInner);
                tr.appendChild(tdDoc);

                const tdSize = document.createElement('td');
                tdSize.className = 'reference-documents-col-narrow';
                tdSize.textContent = formatFileSize(doc.size || 0);
                tr.appendChild(tdSize);

                const tdDate = document.createElement('td');
                tdDate.className = 'reference-documents-col-date';
                tdDate.textContent = formatDateAustralian(doc.created_at);
                tr.appendChild(tdDate);

                const tdAi = document.createElement('td');
                tdAi.className = 'reference-documents-col-ai';
                const aiWrap = document.createElement('div');
                aiWrap.className = 'reference-documents-ai-cell';
                const cb = document.createElement('input');
                cb.type = 'checkbox';
                cb.className = 'reference-documents-row-ai-checkbox';
                cb.checked = !!doc.available_for_task;
                cb.title = 'Available to the AI';
                cb.setAttribute('aria-label', 'Available to the AI for ' + (doc.title || doc.filename || 'document'));
                cb.addEventListener('click', (e) => e.stopPropagation());
                cb.addEventListener('change', (e) => {
                    e.stopPropagation();
                    const want = cb.checked;
                    void setAvailableForTaskOnServer(doc.id, want, cb);
                });
                aiWrap.appendChild(cb);
                tdAi.appendChild(aiWrap);
                tr.appendChild(tdAi);

                const tdSys = document.createElement('td');
                tdSys.className = 'reference-documents-col-ai';
                const sysWrap = document.createElement('div');
                sysWrap.className = 'reference-documents-ai-cell';
                const cbSys = document.createElement('input');
                cbSys.type = 'checkbox';
                cbSys.className = 'reference-documents-row-ai-checkbox';
                cbSys.checked = !!doc.include_in_system_prompt;
                cbSys.title = 'Include in system prompt';
                cbSys.setAttribute('aria-label', 'Include in system prompt for ' + (doc.title || doc.filename || 'document'));
                cbSys.addEventListener('click', (e) => e.stopPropagation());
                cbSys.addEventListener('change', (e) => {
                    e.stopPropagation();
                    const want = cbSys.checked;
                    void setIncludeInSystemPromptOnServer(doc.id, want, cbSys);
                });
                sysWrap.appendChild(cbSys);
                tdSys.appendChild(sysWrap);
                tr.appendChild(tdSys);

                const tdAct = document.createElement('td');
                tdAct.className = 'reference-documents-col-actions';
                const actRow = document.createElement('div');
                actRow.className = 'reference-document-actions';

                if (isImage) {
                    const viewBtn = document.createElement('button');
                    viewBtn.type = 'button';
                    viewBtn.className = 'reference-document-action-btn reference-document-action-btn--view';
                    viewBtn.innerHTML = '<i class="fas fa-eye" aria-hidden="true"></i>';
                    viewBtn.title = 'View';
                    viewBtn.setAttribute('aria-label', 'View');
                    viewBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        viewDocument(doc.id);
                    });
                    actRow.appendChild(viewBtn);
                } else {
                    const downloadBtn = document.createElement('button');
                    downloadBtn.type = 'button';
                    downloadBtn.className = 'reference-document-action-btn reference-document-action-btn--download';
                    downloadBtn.innerHTML = '<i class="fas fa-download" aria-hidden="true"></i>';
                    downloadBtn.title = 'Download';
                    downloadBtn.setAttribute('aria-label', 'Download');
                    downloadBtn.addEventListener('click', (e) => {
                        e.stopPropagation();
                        downloadDocument(doc.id);
                    });
                    actRow.appendChild(downloadBtn);
                }

                const editBtn = document.createElement('button');
                editBtn.type = 'button';
                editBtn.className = 'reference-document-action-btn reference-document-action-btn--edit';
                editBtn.innerHTML = '<i class="fas fa-edit" aria-hidden="true"></i>';
                editBtn.title = 'Edit';
                editBtn.setAttribute('aria-label', 'Edit');
                editBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    editDocument(doc.id);
                });
                actRow.appendChild(editBtn);

                const delBtn = document.createElement('button');
                delBtn.type = 'button';
                delBtn.className = 'reference-document-action-btn reference-document-action-btn--delete';
                delBtn.innerHTML = '<i class="fas fa-trash" aria-hidden="true"></i>';
                delBtn.title = 'Delete';
                delBtn.setAttribute('aria-label', 'Delete');
                delBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    deleteDocument(doc.id);
                });
                actRow.appendChild(delBtn);

                tdAct.appendChild(actRow);
                tr.appendChild(tdAct);

                tbody.appendChild(tr);
            });

            table.appendChild(thead);
            table.appendChild(tbody);
            wrap.appendChild(table);

            DOM.referenceDocumentsList.innerHTML = '';
            DOM.referenceDocumentsList.appendChild(wrap);
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
            document.getElementById('reference-documents-edit-task').checked = doc.available_for_task || false;
            const sysEl = document.getElementById('reference-documents-edit-system-prompt');
            if (sysEl) sysEl.checked = !!doc.include_in_system_prompt;

            Modals._openModal(DOM.referenceDocumentsEditModal);
        }

        async function deleteDocument(documentId) {
            const ok = await AppDialogs.showAppConfirm(
                'Delete document',
                'Are you sure you want to delete this document?',
                { danger: true }
            );
            if (!ok) {
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
                refreshChatContextStatus();
            } catch (error) {
                console.error("Failed to delete document:", error);
                await AppDialogs.showAppAlert('Error', 'Failed to delete document: ' + error.message);
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
                    if (DOM.referenceDocumentsUploadForm) {
                        DOM.referenceDocumentsUploadForm.reset();
                        updateReferenceDocumentsUploadFileList();
                    }
                });
            }
            
            if (DOM.referenceDocumentsUploadCancelBtn) {
                DOM.referenceDocumentsUploadCancelBtn.addEventListener('click', () => {
                    Modals._closeModal(DOM.referenceDocumentsUploadModal);
                    DOM.referenceDocumentsUploadForm.reset();
                    updateReferenceDocumentsUploadFileList();
                });
            }

            const refDocPickBtn = document.getElementById('reference-documents-upload-pick');
            const refDocPathInput = document.getElementById('reference-documents-upload-path');
            if (refDocPickBtn && refDocPathInput) {
                const handlePickClick = async () => {
                    refDocPickBtn.disabled = true;
                    try {
                        const res = await fetch('/api/pick-file', { method: 'POST' });
                        const data = await res.json();
                        if (!res.ok) {
                            throw new Error(data.error || 'Could not open file picker.');
                        }
                        if (data.path) {
                            refDocPathInput.value = data.path;
                            updateReferenceDocumentsUploadFileList();
                        }
                    } catch (err) {
                        const message = (err && err.message) ? err.message : 'Could not open file picker.';
                        if (window.AppDialogs && typeof AppDialogs.showAppAlert === 'function') {
                            await AppDialogs.showAppAlert('Error', message);
                        } else {
                            console.error('[ReferenceDocuments] Picker error:', err);
                        }
                    } finally {
                        refDocPickBtn.disabled = false;
                    }
                };
                refDocPickBtn.addEventListener('click', handlePickClick);
            }
            
            if (DOM.referenceDocumentsUploadForm) {
                DOM.referenceDocumentsUploadForm.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    
                    const selectedPathInput = document.getElementById('reference-documents-upload-path');
                    const submitBtn = document.getElementById('reference-documents-upload-submit');
                    const selectedPath = selectedPathInput ? (selectedPathInput.value || '').trim() : '';
                    if (!selectedPath) {
                        await AppDialogs.showAppAlert('Please choose a file');
                        return;
                    }

                    const description = document.getElementById('reference-documents-upload-description').value;
                    const availableForTask = document.getElementById('reference-documents-upload-task').checked;
                    const includeInSystemPrompt = document.getElementById('reference-documents-upload-system-prompt')?.checked || false;

                    if (submitBtn) submitBtn.disabled = true;
                    try {
                        const response = await fetch('/reference-documents', {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify({
                                file_path: selectedPath,
                                description: description || '',
                                available_for_task: !!availableForTask,
                                include_in_system_prompt: !!includeInSystemPrompt
                            })
                        });
                        if (!response.ok) {
                            let detail = `HTTP ${response.status}`;
                            try {
                                const errBody = await response.json();
                                if (errBody.detail) detail = errBody.detail;
                            } catch (parseErr) { /* ignore */ }
                            throw new Error(detail);
                        }
                        Modals._closeModal(DOM.referenceDocumentsUploadModal);
                        DOM.referenceDocumentsUploadForm.reset();
                        if (selectedPathInput) selectedPathInput.value = '';
                        updateReferenceDocumentsUploadFileList();
                        await loadDocuments();
                        Modals.ReferenceDocumentsNotification.reset();
                        refreshChatContextStatus();
                    } catch (error) {
                        await AppDialogs.showAppAlert('Error', 'Failed to add reference document: ' + (error.message || String(error)));
                    } finally {
                        if (submitBtn) submitBtn.disabled = false;
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
                        available_for_task: document.getElementById('reference-documents-edit-task').checked,
                        include_in_system_prompt: document.getElementById('reference-documents-edit-system-prompt')?.checked || false
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
                        refreshChatContextStatus();
                    } catch (error) {
                        console.error("Failed to update document:", error);
                        await AppDialogs.showAppAlert('Error', 'Failed to update document: ' + error.message);
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
            const allDocs = documents
                .map(doc => `${doc.id}:${doc.available_for_task}:${doc.include_in_system_prompt ? 1 : 0}`)
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
            let shouldShow = !hasShownBefore || (storedHash !== currentHash || numberOfCalls > 15);

            // This will disable the showing of the dialog box
            shouldShow = false;

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
            const newTitle = await AppDialogs.showAppPrompt(
                'Rename conversation',
                'Enter a new title for this conversation.',
                currentTitle != null ? String(currentTitle) : '',
                { promptLabel: 'Title' }
            );
            if (newTitle === null) return;
            const trimmed = newTitle.trim();
            if (!trimmed || trimmed === currentTitle) return;
            try {
                await updateConversationTitle(conversationId, trimmed);
                showConversationList(); // Refresh list
                if (currentConversationId === conversationId) {
                    currentConversationTitle = trimmed;
                    updateConversationIndicator();
                }
            } catch (error) {
                await AppDialogs.showAppAlert('Error', `Error updating title: ${error.message}`);
            }
        }

        async function deleteConversationWithConfirm(conversationId) {
            const ok = await AppDialogs.showAppConfirm(
                'Delete conversation',
                'Are you sure you want to delete this conversation? This action cannot be undone.',
                { danger: true }
            );
            if (!ok) {
                return;
            }

            try {
                await deleteConversation(conversationId);
                if (currentConversationId === conversationId) {
                    currentConversationId = null;
                    currentConversationTitle = null;
                    localStorage.removeItem(CONVERSATION_STORAGE_KEY);
                    localStorage.removeItem(CONVERSATION_TITLE_STORAGE_KEY);
                    updateConversationIndicator();
                    Chat.clearChat();
                    void ensureChatConversationContext();
                }
                showConversationList(); // Refresh list
            } catch (error) {
                await AppDialogs.showAppAlert('Error', `Error deleting conversation: ${error.message}`);
            }
        }

        /** Clears server-side turns for the active chat (Voice Settings). Requires owner master unlock on the API. */
        async function clearCurrentConversationHistoryWithConfirm() {
            let id = getCurrentConversationId();
            if (id == null) {
                await ensureChatConversationContext();
                id = getCurrentConversationId();
            }
            if (id == null) {
                await AppDialogs.showAppAlert('No conversation', 'There is no active chat conversation to clear.');
                return;
            }
            const ok = await AppDialogs.showAppConfirm(
                'Clear conversation history',
                'Remove all saved messages for this conversation from the server? The on-screen chat will be emptied. This cannot be undone.',
                { danger: true }
            );
            if (!ok) {
                return;
            }
            try {
                const res = await fetch(`/chat/conversations/${id}/clear-history`, {
                    method: 'POST',
                    credentials: 'same-origin',
                });
                if (!res.ok) {
                    let detail = await res.text();
                    try {
                        const j = JSON.parse(detail);
                        if (j.detail) detail = j.detail;
                    } catch (_) { /* keep body as detail */ }
                    throw new Error(detail || `HTTP ${res.status}`);
                }
                Chat.clearChat();
            } catch (error) {
                await AppDialogs.showAppAlert('Error', error.message || String(error));
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
                    if (turn.user_input) {
                        Chat.addMessage('user', turn.user_input, false);
                    }
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
                await AppDialogs.showAppAlert('Error', `Error resuming conversation: ${error.message}`);
            }
        }

        async function createNewConversation() {
            if (!DOM.newConversationTitleInput || !DOM.newConversationVoiceSelect) {
                await AppDialogs.showAppAlert('New conversation form elements not found');
                return;
            }

            const title = DOM.newConversationTitleInput.value.trim();
            const voice = DOM.newConversationVoiceSelect.value;

            if (!title) {
                await AppDialogs.showAppAlert('Please enter a conversation title');
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
                await AppDialogs.showAppAlert('Error', `Error creating conversation: ${error.message}`);
            }
        }

        function updateConversationIndicator() {}

        function getCurrentConversationId() {
            return currentConversationId;
        }

        function clearCurrentConversation() {
            currentConversationId = null;
            currentConversationTitle = null;
            localStorage.removeItem(CONVERSATION_STORAGE_KEY);
            localStorage.removeItem(CONVERSATION_TITLE_STORAGE_KEY);
            updateConversationIndicator();
        }

        /** Ensures a conversation id is set so /chat/generate can load prior turns. Creates or resumes automatically. */
        async function ensureChatConversationContext() {
            try {
                const storedRaw = localStorage.getItem(CONVERSATION_STORAGE_KEY);
                if (storedRaw) {
                    const id = parseInt(storedRaw, 10);
                    if (!Number.isNaN(id) && id > 0) {
                        const res = await fetch(`/chat/conversations/${id}`, { credentials: 'same-origin' });
                        if (res.ok) {
                            const conv = await res.json();
                            currentConversationId = conv.id;
                            currentConversationTitle = conv.title != null ? conv.title : '';
                            updateConversationIndicator();
                            return;
                        }
                    }
                    localStorage.removeItem(CONVERSATION_STORAGE_KEY);
                    localStorage.removeItem(CONVERSATION_TITLE_STORAGE_KEY);
                }
                const listRes = await fetch('/chat/conversations?limit=20', { credentials: 'same-origin' });
                if (!listRes.ok) return;
                const convs = await listRes.json();
                if (Array.isArray(convs) && convs.length > 0) {
                    const c = convs[0];
                    currentConversationId = c.id;
                    currentConversationTitle = c.title != null ? c.title : '';
                    localStorage.setItem(CONVERSATION_STORAGE_KEY, String(c.id));
                    localStorage.setItem(CONVERSATION_TITLE_STORAGE_KEY, currentConversationTitle || '');
                    updateConversationIndicator();
                    return;
                }
                const voice = (typeof VoiceSelector !== 'undefined' && VoiceSelector.getSelectedVoice)
                    ? VoiceSelector.getSelectedVoice()
                    : 'expert';
                const createRes = await fetch('/chat/conversations', {
                    method: 'POST',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title: 'Chat', voice })
                });
                if (!createRes.ok) return;
                const conv = await createRes.json();
                currentConversationId = conv.id;
                currentConversationTitle = conv.title != null ? conv.title : 'Chat';
                localStorage.setItem(CONVERSATION_STORAGE_KEY, String(conv.id));
                localStorage.setItem(CONVERSATION_TITLE_STORAGE_KEY, currentConversationTitle);
                updateConversationIndicator();
            } catch (e) {
                console.warn('ensureChatConversationContext:', e);
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
            // ensureChatConversationContext() is invoked from App.init after Modals.initAll
            // so every chat request has a conversation_id and the backend can load history.

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
            clearCurrentConversation,
            clearCurrentConversationHistoryWithConfirm,
            ensureChatConversationContext
        };
})();


Modals.SubjectConfiguration = (() => {
        let currentSubjectName = null;
        /** Family name from subject_configuration; preserved for saves (name fields are read-only in UI). */
        let currentFamilyName = null;
        let currentGender = null;
        let configurationLoaded = false;
        /** Universal prompts (app_system_instructions); kept in memory when saving with Subject Configuration — edit via admin System instructions. */
        let loadedSystemInstructions = '';
        let loadedCoreSystemInstructions = null;
        let loadedQuestionSystemInstructions = '';
        /** Phone / social handles — not shown in UI; preserved from API on save (e.g. set via dashboard API). */
        let loadedPhoneNumbers = '';
        let loadedWhatsappHandle = '';
        let loadedInstagramHandle = '';
        let ownerContactSuggestions = [];
        let ownerContactAllCache = null;
        let linkedOwnerContact = null;
        /** Saved subject_contact_id from last successful GET / POST response; generate buttons require this. */
        let persistedSubjectContactId = null;

        function setPersistedOwnerContactFromConfig(config) {
            if (!config || config.subject_contact_id == null || config.subject_contact_id === undefined) {
                persistedSubjectContactId = null;
                return;
            }
            const n = Number(config.subject_contact_id, 10);
            persistedSubjectContactId = Number.isFinite(n) && n >= 0 ? n : null;
        }

        function hasPersistedOwnerContact() {
            return persistedSubjectContactId != null && Number(persistedSubjectContactId) >= 0;
        }

        const subjectConfigSubtabPanelIds = {
            details: 'subject-config-subtab-details',
            writing: 'subject-config-subtab-writing',
            psych: 'subject-config-subtab-psych'
        };

        function switchSubjectConfigSubtab(name) {
            const root = document.getElementById('subject-configuration-tab');
            if (!root || !subjectConfigSubtabPanelIds[name]) {
                return;
            }
            const btns = root.querySelectorAll('.subject-config-subtab-button');
            const panels = root.querySelectorAll('.subject-config-subtab-panel');
            btns.forEach((b) => {
                const on = b.getAttribute('data-subtab') === name;
                b.classList.toggle('active', on);
                b.setAttribute('aria-selected', on ? 'true' : 'false');
            });
            panels.forEach((p) => {
                const on = p.id === subjectConfigSubtabPanelIds[name];
                p.classList.toggle('active', on);
            });
        }

        function resetSubjectConfigSubtabs() {
            switchSubjectConfigSubtab('details');
        }

        function initSubjectConfigSubtabs() {
            const root = document.getElementById('subject-configuration-tab');
            if (!root || root.dataset.subtabWired === '1') {
                return;
            }
            root.dataset.subtabWired = '1';
            root.addEventListener('click', (e) => {
                const btn = e.target.closest('.subject-config-subtab-button');
                if (!btn || !root.contains(btn)) {
                    return;
                }
                const sub = btn.getAttribute('data-subtab');
                if (!sub) {
                    return;
                }
                switchSubjectConfigSubtab(sub);
            });
        }

        function switchSubjectAiInnerSubtab(wrap, subtab) {
            if (!wrap || !subtab) {
                return;
            }
            wrap.querySelectorAll('.subject-config-ai-inner-tab').forEach((b) => {
                const on = b.getAttribute('data-inner-subtab') === subtab;
                b.classList.toggle('active', on);
                b.setAttribute('aria-selected', on ? 'true' : 'false');
            });
            wrap.querySelectorAll('.subject-config-ai-inner-panel').forEach((p) => {
                p.classList.toggle('active', p.getAttribute('data-inner-subtab') === subtab);
            });
        }

        function showSubjectAiInnerPreview(wrapId) {
            const wrap = document.getElementById(wrapId);
            switchSubjectAiInnerSubtab(wrap, 'preview');
        }

        function resetSubjectAiInnerTabsToPreview() {
            showSubjectAiInnerPreview('writing-style-ai-inner-wrap');
            showSubjectAiInnerPreview('psychological-profile-ai-inner-wrap');
        }

        function initSubjectAiInnerTabs() {
            ['writing-style-ai-inner-wrap', 'psychological-profile-ai-inner-wrap'].forEach((id) => {
                const wrap = document.getElementById(id);
                if (!wrap || wrap.dataset.innerTabWired === '1') {
                    return;
                }
                wrap.dataset.innerTabWired = '1';
                wrap.addEventListener('click', (e) => {
                    const btn = e.target.closest('.subject-config-ai-inner-tab');
                    if (!btn || !wrap.contains(btn)) {
                        return;
                    }
                    const sub = btn.getAttribute('data-inner-subtab');
                    if (!sub) {
                        return;
                    }
                    switchSubjectAiInnerSubtab(wrap, sub);
                });
            });
        }

        function formatArchiveOwnerDisplayName(subjectName, familyName) {
            const a = (subjectName != null ? String(subjectName) : '').trim();
            const b = (familyName != null ? String(familyName) : '').trim();
            if (a && b) {
                return `${a} ${b}`;
            }
            return a || b || '';
        }

        function updateArchiveOwnerDisplay(subjectName, familyName) {
            if (!DOM.subjectArchiveOwnerName) {
                return;
            }
            DOM.subjectArchiveOwnerName.textContent = formatArchiveOwnerDisplayName(subjectName, familyName);
        }

        function syncSubjectProfileGenerateButtons() {
            const ok = hasPersistedOwnerContact();
            if (DOM.requestWritingStyleBtn) {
                DOM.requestWritingStyleBtn.disabled = !ok;
            }
            if (DOM.requestPsychologicalProfileBtn) {
                DOM.requestPsychologicalProfileBtn.disabled = !ok;
            }
            if (DOM.writingStyleRequireContactHint) {
                DOM.writingStyleRequireContactHint.style.display = ok ? 'none' : 'block';
            }
            if (DOM.psychologicalProfileRequireContactHint) {
                DOM.psychologicalProfileRequireContactHint.style.display = ok ? 'none' : 'block';
            }
            if (DOM.interestsSuggestAiBtnSc) {
                DOM.interestsSuggestAiBtnSc.disabled = !ok;
            }
            if (DOM.interestsSuggestAiRequireContactHint) {
                DOM.interestsSuggestAiRequireContactHint.style.display = ok ? 'none' : 'block';
            }
        }

        function _ownerContactLabel(c) {
            const name = (c && c.name ? String(c.name) : '').trim() || 'Unnamed';
            const em = c && c.email != null && String(c.email).trim() ? String(c.email).trim() : '';
            return em ? `${name} — ${em}` : name;
        }

        function _ensureSelectedInOwnerRows(rows, selectedId, linkedContact) {
            const id = selectedId === '' || selectedId == null ? null : Number(selectedId, 10);
            const r = Array.isArray(rows) ? rows.slice() : [];
            if (id == null || Number.isNaN(id) || id < 0) {
                return r;
            }
            const idx = r.findIndex((x) => x && Number(x.id) === id);
            if (linkedContact && linkedContact.id != null && Number(linkedContact.id) === id) {
                const row = {
                    id: linkedContact.id,
                    name: linkedContact.name || '',
                    email: linkedContact.email != null ? linkedContact.email : null
                };
                if (idx >= 0) {
                    r[idx] = row;
                } else {
                    r.push(row);
                }
                return r;
            }
            if (idx >= 0) {
                return r;
            }
            r.push({ id, name: `Contact #${id}`, email: null });
            return r;
        }

        async function _fetchAllContactsRows() {
            if (ownerContactAllCache) {
                return ownerContactAllCache;
            }
            const res = await fetch('/contacts?limit=50000&offset=0&order_by=name');
            if (!res.ok) {
                throw new Error('Could not load contacts');
            }
            const data = await res.json();
            const contacts = Array.isArray(data.contacts) ? data.contacts : [];
            ownerContactAllCache = contacts.map((row) => ({
                id: row.id,
                name: row.name || '',
                email: row.email != null ? String(row.email) : null
            }));
            return ownerContactAllCache;
        }

        async function fillOwnerContactSelect(selectedId, linkedContact) {
            const sel = DOM.subjectOwnerContactSelect;
            if (!sel) {
                return;
            }
            const showAll = !!(DOM.subjectOwnerContactShowAll && DOM.subjectOwnerContactShowAll.checked);
            let rows = ownerContactSuggestions;
            try {
                if (showAll) {
                    rows = await _fetchAllContactsRows();
                }
            } catch (e) {
                console.error('Owner contact full list:', e);
                rows = ownerContactSuggestions;
            }
            rows = _ensureSelectedInOwnerRows(rows, selectedId, linkedContact);
            rows.sort((a, b) => String(a.name || '').localeCompare(String(b.name || ''), undefined, { sensitivity: 'base' }));
            const keep = selectedId == null || selectedId === '' ? '' : String(selectedId);
            sel.innerHTML = '<option value="">None (not linked)</option>';
            for (const c of rows) {
                if (!c || c.id == null) {
                    continue;
                }
                const opt = document.createElement('option');
                opt.value = String(c.id);
                opt.textContent = _ownerContactLabel(c);
                sel.appendChild(opt);
            }
            sel.value = keep;
            if (keep !== '' && sel.value !== keep) {
                const opt = document.createElement('option');
                opt.value = keep;
                opt.textContent = `Contact #${keep}`;
                sel.appendChild(opt);
                sel.value = keep;
            }
        }

        async function loadConfiguration() {
            try {
                const response = await fetch('/api/subject-configuration');
                if (response.ok) {
                    const config = await response.json();
                    currentSubjectName = config.subject_name;
                    currentFamilyName = config.family_name != null ? String(config.family_name) : '';
                    currentGender = config.gender || 'Male';
                    configurationLoaded = true;
                    return config;
                } else if (response.status === 404) {
                    // Configuration doesn't exist yet
                    configurationLoaded = false;
                    currentSubjectName = null;
                    currentFamilyName = null;
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
            if (!hasPersistedOwnerContact()) {
                return;
            }
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
                const summary = data.summary || '';
                if (DOM.writingStyleEditTextarea) {
                    DOM.writingStyleEditTextarea.value = summary;
                }
                _renderWritingStyleMarkdown(summary);
                showSubjectAiInnerPreview('writing-style-ai-inner-wrap');
            } catch (error) {
                console.error('Error requesting writing style:', error);
                DOM.writingStyleDisplay.innerHTML = `<span style="color: #c00;">Error: ${error.message}</span>`;
            } finally {
                DOM.requestWritingStyleBtn.disabled = false;
                DOM.writingStyleLoading.style.display = 'none';
                syncSubjectProfileGenerateButtons();
            }
        }

        async function requestPsychologicalProfile() {
            if (!hasPersistedOwnerContact()) {
                return;
            }
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
                const profile = data.profile || '';
                if (DOM.psychologicalProfileEditTextarea) {
                    DOM.psychologicalProfileEditTextarea.value = profile;
                }
                _renderPsychologicalProfileMarkdown(profile);
                showSubjectAiInnerPreview('psychological-profile-ai-inner-wrap');
            } catch (error) {
                console.error('Error requesting psychological profile:', error);
                DOM.psychologicalProfileDisplay.innerHTML = `<span style="color: #c00;">Error: ${error.message}</span>`;
            } finally {
                DOM.requestPsychologicalProfileBtn.disabled = false;
                DOM.psychologicalProfileLoading.style.display = 'none';
                syncSubjectProfileGenerateButtons();
            }
        }

        async function saveWritingStyleAIText() {
            if (!DOM.writingStyleEditTextarea) {
                return;
            }
            const text = DOM.writingStyleEditTextarea.value;
            if (DOM.writingStyleSaveStatus) {
                DOM.writingStyleSaveStatus.textContent = 'Saving…';
            }
            try {
                const response = await fetch('/api/subject-configuration/writing-style-ai', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ writing_style_ai: text })
                });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.detail || 'Failed to save writing style');
                }
                await populateFormFromConfig(data);
                if (DOM.writingStyleSaveStatus) {
                    DOM.writingStyleSaveStatus.textContent = 'Saved.';
                }
                showSubjectAiInnerPreview('writing-style-ai-inner-wrap');
            } catch (err) {
                console.error('saveWritingStyleAIText:', err);
                if (DOM.writingStyleSaveStatus) {
                    DOM.writingStyleSaveStatus.textContent = err.message || 'Save failed';
                }
                await AppDialogs.showAppAlert('Error', err.message || 'Failed to save writing style');
            }
        }

        async function savePsychologicalProfileAIText() {
            if (!DOM.psychologicalProfileEditTextarea) {
                return;
            }
            const text = DOM.psychologicalProfileEditTextarea.value;
            if (DOM.psychologicalProfileSaveStatus) {
                DOM.psychologicalProfileSaveStatus.textContent = 'Saving…';
            }
            try {
                const response = await fetch('/api/subject-configuration/psychological-profile-ai', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ psychological_profile_ai: text })
                });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.detail || 'Failed to save psychological profile');
                }
                await populateFormFromConfig(data);
                if (DOM.psychologicalProfileSaveStatus) {
                    DOM.psychologicalProfileSaveStatus.textContent = 'Saved.';
                }
                showSubjectAiInnerPreview('psychological-profile-ai-inner-wrap');
            } catch (err) {
                console.error('savePsychologicalProfileAIText:', err);
                if (DOM.psychologicalProfileSaveStatus) {
                    DOM.psychologicalProfileSaveStatus.textContent = err.message || 'Save failed';
                }
                await AppDialogs.showAppAlert('Error', err.message || 'Failed to save psychological profile');
            }
        }

        async function saveConfiguration(subjectName, gender, familyName, otherNames, emailAddresses) {
            const phoneNumbers = (loadedPhoneNumbers || '').trim();
            const whatsappHandle = (loadedWhatsappHandle || '').trim();
            const instagramHandle = (loadedInstagramHandle || '').trim();
            try {
                const payload = {
                    subject_name: subjectName,
                    gender: gender || 'Male',
                    family_name: familyName || null,
                    other_names: otherNames || null,
                    email_addresses: emailAddresses || null,
                    phone_numbers: phoneNumbers || null,
                    whatsapp_handle: whatsappHandle || null,
                    instagram_handle: instagramHandle || null
                };
                let subjectContactId = null;
                if (DOM.subjectOwnerContactSelect) {
                    const raw = DOM.subjectOwnerContactSelect.value.trim();
                    if (raw !== '') {
                        const n = Number(raw, 10);
                        if (Number.isFinite(n) && n >= 0) {
                            subjectContactId = n;
                        }
                    }
                }
                payload.subject_contact_id = subjectContactId;
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
                currentFamilyName = config.family_name != null ? String(config.family_name) : '';
                currentGender = config.gender || 'Male';
                configurationLoaded = true;
                return config;
            } catch (error) {
                console.error('Error saving subject configuration:', error);
                throw error;
            }
        }

        async function saveSystemInstructions(systemInstructions, coreSystemInstructions, questionSystemInstructions) {
            const response = await fetch('/api/system-instructions', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    system_instructions: systemInstructions,
                    core_system_instructions: coreSystemInstructions,
                    question_system_instructions: questionSystemInstructions
                })
            });
            if (!response.ok) {
                const error = await response.json().catch(() => ({}));
                throw new Error(error.detail || 'Failed to save system instructions');
            }
        }

        function getSubjectName() {
            return currentSubjectName;
        }


        async function populateFormFromConfig(config) {
            if (!config) {
                currentSubjectName = null;
                currentFamilyName = null;
                updateArchiveOwnerDisplay('', '');
                if (DOM.writingStyleEditTextarea) {
                    DOM.writingStyleEditTextarea.value = '';
                }
                if (DOM.psychologicalProfileEditTextarea) {
                    DOM.psychologicalProfileEditTextarea.value = '';
                }
                if (DOM.writingStyleDisplay) {
                    _renderWritingStyleMarkdown('');
                }
                if (DOM.psychologicalProfileDisplay) {
                    _renderPsychologicalProfileMarkdown('');
                }
                if (DOM.writingStyleSaveStatus) {
                    DOM.writingStyleSaveStatus.textContent = '';
                }
                if (DOM.psychologicalProfileSaveStatus) {
                    DOM.psychologicalProfileSaveStatus.textContent = '';
                }
                setPersistedOwnerContactFromConfig(null);
                linkedOwnerContact = null;
                syncSubjectProfileGenerateButtons();
                return;
            }
            currentSubjectName = config.subject_name != null ? String(config.subject_name) : '';
            currentFamilyName = config.family_name != null ? String(config.family_name) : '';
            updateArchiveOwnerDisplay(currentSubjectName, currentFamilyName);
            if (DOM.subjectGenderSelect) DOM.subjectGenderSelect.value = config.gender || 'Male';
            if (DOM.otherNamesInput) DOM.otherNamesInput.value = config.other_names || '';
            if (DOM.emailAddressesInput) DOM.emailAddressesInput.value = config.email_addresses || '';
            ownerContactSuggestions = Array.isArray(config.owner_contact_suggestions) ? config.owner_contact_suggestions : [];
            linkedOwnerContact = config.linked_owner_contact && config.linked_owner_contact.id != null
                ? config.linked_owner_contact
                : null;
            const chosen = config.subject_contact_id != null && config.subject_contact_id !== undefined
                ? String(config.subject_contact_id)
                : '';
            await fillOwnerContactSelect(chosen, linkedOwnerContact);
            loadedPhoneNumbers = config.phone_numbers != null ? String(config.phone_numbers) : '';
            loadedWhatsappHandle = config.whatsapp_handle != null ? String(config.whatsapp_handle) : '';
            loadedInstagramHandle = config.instagram_handle != null ? String(config.instagram_handle) : '';
            const wsText = config.writing_style_ai != null ? String(config.writing_style_ai) : '';
            const ppText = config.psychological_profile_ai != null ? String(config.psychological_profile_ai) : '';
            if (DOM.writingStyleEditTextarea) {
                DOM.writingStyleEditTextarea.value = wsText;
            }
            if (DOM.psychologicalProfileEditTextarea) {
                DOM.psychologicalProfileEditTextarea.value = ppText;
            }
            if (DOM.writingStyleDisplay) _renderWritingStyleMarkdown(wsText);
            if (DOM.psychologicalProfileDisplay) _renderPsychologicalProfileMarkdown(ppText);
            if (DOM.writingStyleSaveStatus) DOM.writingStyleSaveStatus.textContent = '';
            if (DOM.psychologicalProfileSaveStatus) DOM.psychologicalProfileSaveStatus.textContent = '';
            loadedSystemInstructions = config.system_instructions || '';
            const coreVal = config.core_system_instructions || '';
            loadedCoreSystemInstructions = coreVal;
            loadedQuestionSystemInstructions = config.question_system_instructions || '';
            setPersistedOwnerContactFromConfig(config);
            syncSubjectProfileGenerateButtons();
        }

        async function loadAndPopulateForm() {
            resetSubjectConfigSubtabs();
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
            resetSubjectAiInnerTabsToPreview();
        }

        async function checkAndShow() {
            const config = await loadConfiguration();
            if (!config) {
                // No configuration exists - open config overlay and switch to Subject Configuration tab
                if (DOM.configPage) {
                    DOM.configPage.style.display = 'flex';
                    const subjectTabBtn = document.querySelector('.config-tab-button[data-tab="subject-configuration"]');
                    if (subjectTabBtn) subjectTabBtn.click();
                    if (typeof window.refreshSettingsDataImportModalLLM === 'function') {
                        window.refreshSettingsDataImportModalLLM();
                    }
                }
            }
        }

        async function loadDefaultInstructions() {
            try {
                const config = await loadConfiguration();
                if (config) {
                    loadedSystemInstructions = config.system_instructions || '';
                    const coreVal = config.core_system_instructions || '';
                    loadedCoreSystemInstructions = coreVal;
                    loadedQuestionSystemInstructions = config.question_system_instructions || '';
                    const wsText = config.writing_style_ai != null ? String(config.writing_style_ai) : '';
                    const ppText = config.psychological_profile_ai != null ? String(config.psychological_profile_ai) : '';
                    if (DOM.writingStyleEditTextarea) {
                        DOM.writingStyleEditTextarea.value = wsText;
                    }
                    if (DOM.psychologicalProfileEditTextarea) {
                        DOM.psychologicalProfileEditTextarea.value = ppText;
                    }
                    if (DOM.writingStyleDisplay) {
                        _renderWritingStyleMarkdown(wsText);
                    }
                    if (DOM.psychologicalProfileDisplay) {
                        _renderPsychologicalProfileMarkdown(ppText);
                    }
                    setPersistedOwnerContactFromConfig(config);
                    syncSubjectProfileGenerateButtons();
                    return;
                }
            } catch (err) {
                console.debug('Could not load configuration from API:', err);
            }

            if (!(loadedSystemInstructions || '').trim()) {
                try {
                    const response = await fetch('/static/data/system_instructions_chat.txt');
                    if (response.ok) {
                        loadedSystemInstructions = await response.text();
                    }
                } catch (err) {
                    console.debug('Could not load default system instructions:', err);
                }
            }

            if (loadedCoreSystemInstructions == null || loadedCoreSystemInstructions === '') {
                try {
                    const response = await fetch('/static/data/system_instructions_core.txt');
                    if (response.ok) {
                        loadedCoreSystemInstructions = await response.text();
                    }
                } catch (err) {
                    console.debug('Could not load default core system instructions:', err);
                }
            }

            if (!(loadedQuestionSystemInstructions || '').trim()) {
                try {
                    const response = await fetch('/static/data/system_instructions_question.txt');
                    if (response.ok) {
                        loadedQuestionSystemInstructions = await response.text();
                    }
                } catch (err) {
                    console.debug('Could not load default question system instructions:', err);
                }
            }
        }

        async function handleSave() {
            const subjectName = (currentSubjectName != null ? String(currentSubjectName) : '').trim();
            const gender = DOM.subjectGenderSelect ? DOM.subjectGenderSelect.value : 'Male';
            const familyName = (currentFamilyName != null ? String(currentFamilyName) : '').trim();
            const otherNames = DOM.otherNamesInput ? DOM.otherNamesInput.value.trim() : '';
            const emailAddresses = DOM.emailAddressesInput ? DOM.emailAddressesInput.value.trim() : '';
            const systemInstructions = (loadedSystemInstructions || '').trim();
            const coreSystemInstructions = loadedCoreSystemInstructions != null ? loadedCoreSystemInstructions : '';
            const questionSystemInstructions = (loadedQuestionSystemInstructions || '').trim();

            if (!subjectName) {
                await AppDialogs.showAppAlert('Archive owner name is not available. Save cannot continue until subject configuration exists for this archive.');
                return;
            }

            try {
                const config = await saveConfiguration(subjectName, gender, familyName, otherNames, emailAddresses);
                await saveSystemInstructions(systemInstructions, coreSystemInstructions, questionSystemInstructions);
                await populateFormFromConfig(config);

                await AppDialogs.showAppAlert('Success', 'Subject configuration saved successfully!');
            } catch (error) {
                await AppDialogs.showAppAlert('Error', `Error saving configuration: ${error.message}`);
            }
        }

        function init() {
            initSubjectConfigSubtabs();
            initSubjectAiInnerTabs();
            // Set up event listeners
            if (DOM.saveSubjectConfigBtn) {
                DOM.saveSubjectConfigBtn.addEventListener('click', () => { void handleSave(); });
            }

            // Writing style generate button
            if (DOM.requestWritingStyleBtn) {
                DOM.requestWritingStyleBtn.addEventListener('click', () => requestWritingStyle());
            }
            if (DOM.requestPsychologicalProfileBtn) {
                DOM.requestPsychologicalProfileBtn.addEventListener('click', () => requestPsychologicalProfile());
            }

            if (DOM.saveWritingStyleAiBtn) {
                DOM.saveWritingStyleAiBtn.addEventListener('click', () => { void saveWritingStyleAIText(); });
            }
            if (DOM.savePsychologicalProfileAiBtn) {
                DOM.savePsychologicalProfileAiBtn.addEventListener('click', () => { void savePsychologicalProfileAIText(); });
            }
            if (DOM.writingStyleEditTextarea) {
                DOM.writingStyleEditTextarea.addEventListener('input', () => {
                    _renderWritingStyleMarkdown(DOM.writingStyleEditTextarea.value);
                });
            }
            if (DOM.psychologicalProfileEditTextarea) {
                DOM.psychologicalProfileEditTextarea.addEventListener('input', () => {
                    _renderPsychologicalProfileMarkdown(DOM.psychologicalProfileEditTextarea.value);
                });
            }

            if (DOM.subjectOwnerContactShowAll) {
                DOM.subjectOwnerContactShowAll.addEventListener('change', () => {
                    const sel = DOM.subjectOwnerContactSelect;
                    const v = sel ? sel.value : '';
                    void fillOwnerContactSelect(v, linkedOwnerContact);
                });
            }

            syncSubjectProfileGenerateButtons();

            // Check and show config with Subject Configuration tab on page load if no config exists
            checkAndShow();
        }

        return {
            init,
            checkAndShow,
            loadConfiguration,
            loadAndPopulateForm,
            saveConfiguration,
            getSubjectName,
            resetSubjectConfigSubtabs,
            hasPersistedOwnerContactLinked: hasPersistedOwnerContact,
            syncProfileAiButtons: syncSubjectProfileGenerateButtons
        };
})();


Modals.ManageKeys = (() => {
    function _normKeyringPw(s) {
        return (s == null ? '' : String(s)).trim().toLowerCase();
    }

    function _visitorAccessPayload(prefix) {
        const contactsAndRelationships = !!document.getElementById(`${prefix}-can-relationships`)?.checked;
        return {
            can_messages_chat: !!document.getElementById(`${prefix}-can-messages-chat`)?.checked,
            can_emails: !!document.getElementById(`${prefix}-can-emails`)?.checked,
            can_contacts: contactsAndRelationships,
            can_relationships: contactsAndRelationships,
            can_sensitive_private: !!document.getElementById(`${prefix}-can-sensitive-private`)?.checked,
            llm_allow_owner_keys: !!document.getElementById(`${prefix}-llm-allow-owner-keys`)?.checked,
            llm_allow_server_keys: !!document.getElementById(`${prefix}-llm-allow-server-keys`)?.checked
        };
    }

    function _clearVisitorAccessCheckboxes(prefix) {
        ['can-messages-chat', 'can-emails', 'can-relationships', 'can-sensitive-private'].forEach(suf => {
            const el = document.getElementById(`${prefix}-${suf}`);
            if (el) el.checked = false;
        });
        ['llm-allow-owner-keys', 'llm-allow-server-keys'].forEach(suf => {
            const el = document.getElementById(`${prefix}-${suf}`);
            if (el) el.checked = true;
        });
    }

    function _setVisitorAccessCheckboxes(prefix, h) {
        const pairs = [
            ['can-messages-chat', 'can_messages_chat'],
            ['can-emails', 'can_emails'],
            ['can-sensitive-private', 'can_sensitive_private'],
            ['llm-allow-owner-keys', 'llm_allow_owner_keys'],
            ['llm-allow-server-keys', 'llm_allow_server_keys']
        ];
        pairs.forEach(([idSuf, key]) => {
            const el = document.getElementById(`${prefix}-${idSuf}`);
            if (!el) return;
            const v = h[key];
            if (v === undefined || v === null) {
                el.checked = key.startsWith('llm_');
            } else {
                el.checked = !!v;
            }
        });
        const relEl = document.getElementById(`${prefix}-can-relationships`);
        if (relEl) relEl.checked = !!(h.can_contacts || h.can_relationships);
    }

    const _visitorKeyAccessColTitles = {
        can_messages_chat: 'Social media messages',
        can_emails: 'Emails & IMAP/Gmail',
        can_contacts: 'Contacts',
        can_relationships: 'Contacts and Relationships',
        can_sensitive_private: 'Sensitive & private data',
        llm_allow_owner_keys: 'May use owner AI API keys',
        llm_allow_server_keys: 'May use server default AI keys'
    };

    function _visitorKeyRefDocsSetTab(which) {
        const panelRef = document.getElementById('visitor-key-refdocs-panel-ref');
        const panelSens = document.getElementById('visitor-key-refdocs-panel-sensitive');
        const btnRef = document.getElementById('visitor-key-refdocs-tab-ref');
        const btnSens = document.getElementById('visitor-key-refdocs-tab-sensitive');
        const help = document.getElementById('visitor-key-refdocs-tab-help');
        const isRef = which === 'ref';
        if (panelRef) {
            panelRef.hidden = !isRef;
        }
        if (panelSens) {
            panelSens.hidden = isRef;
        }
        if (btnRef) {
            btnRef.classList.toggle('visitor-key-refdocs-tab--active', isRef);
            btnRef.setAttribute('aria-selected', isRef ? 'true' : 'false');
            btnRef.tabIndex = isRef ? 0 : -1;
        }
        if (btnSens) {
            btnSens.classList.toggle('visitor-key-refdocs-tab--active', !isRef);
            btnSens.setAttribute('aria-selected', (!isRef).toString());
            btnSens.tabIndex = !isRef ? 0 : -1;
        }
        if (help) {
            help.textContent = isRef
                ? 'Select the documents that can be accessed by the AI on behalf of the this visitor.'
                : 'Select the private data records that can be accessed by the AI on behalf of this visitor.';
        }
    }

    function _resetVisitorKeyRefDocsUI() {
        _visitorKeyRefDocsGen++;
        _visitorKeyRefDocsLoadOk = false;
        const list = document.getElementById('visitor-key-refdocs-list');
        if (list) list.textContent = '';
        const sensList = document.getElementById('visitor-key-refdocs-sensitive-list');
        if (sensList) sensList.textContent = '';
        const loading = document.getElementById('visitor-key-refdocs-loading');
        if (loading) loading.style.display = 'none';
        _visitorKeyRefDocsSetTab('ref');
    }

    function _fillVisitorKeyRefDocCheckboxes(container, docs, emptyMessage) {
        if (!container) return;
        container.textContent = '';
        docs.forEach(d => {
            const id = d.id;
            const label = document.createElement('label');
            label.style.display = 'flex';
            label.style.alignItems = 'flex-start';
            label.style.gap = '8px';
            label.style.marginBottom = '6px';
            label.style.fontSize = '0.9rem';
            const cb = document.createElement('input');
            cb.type = 'checkbox';
            cb.dataset.docId = String(id);
            cb.checked = !!d.allowed;
            const span = document.createElement('span');
            span.textContent = d.title ? `${d.title} (id ${id})` : `Document ${id}`;
            label.appendChild(cb);
            label.appendChild(span);
            container.appendChild(label);
        });
        if (docs.length === 0) {
            const pEl = document.createElement('p');
            pEl.style.color = '#666';
            pEl.style.margin = '0';
            pEl.textContent = emptyMessage;
            container.appendChild(pEl);
        }
    }

    async function _loadVisitorKeyRefDocsForHint(hintId, loadGen) {
        const list = document.getElementById('visitor-key-refdocs-list');
        const sensList = document.getElementById('visitor-key-refdocs-sensitive-list');
        const loading = document.getElementById('visitor-key-refdocs-loading');
        const err = document.getElementById('edit-visitor-key-hint-error');
        if (!list || !hintId) return;
        if (err) { err.style.display = 'none'; err.textContent = ''; }
        list.textContent = '';
        if (sensList) sensList.textContent = '';
        if (loading) loading.style.display = 'block';
        _visitorKeyRefDocsSetTab('ref');
        try {
            const [refResp, sensResp] = await Promise.all([
                fetch(`/reference-documents/visitor-key-hints/${hintId}/reference-documents`, {
                    credentials: 'same-origin',
                    headers: { Accept: 'application/json' }
                }),
                fetch(`/reference-documents/visitor-key-hints/${hintId}/sensitive-reference-documents`, {
                    credentials: 'same-origin',
                    headers: { Accept: 'application/json' }
                })
            ]);
            const refData = await refResp.json().catch(() => ({}));
            const sensData = await sensResp.json().catch(() => ({}));
            if (!refResp.ok) throw new Error(refData.detail || `Reference docs: HTTP ${refResp.status}`);
            if (!sensResp.ok) throw new Error(sensData.detail || `Sensitive docs: HTTP ${sensResp.status}`);
            const refDocs = Array.isArray(refData.documents) ? refData.documents : [];
            const sensDocs = Array.isArray(sensData.documents) ? sensData.documents : [];
            _fillVisitorKeyRefDocCheckboxes(
                list,
                refDocs,
                'No task-available reference documents yet. Add documents under Reference Documents and mark them for AI tasks.'
            );
            _fillVisitorKeyRefDocCheckboxes(
                sensList,
                sensDocs,
                'No sensitive or private records yet. Add them under Sensitive Data.'
            );
            if (loadGen === _visitorKeyRefDocsGen) {
                _visitorKeyRefDocsLoadOk = true;
            }
        } catch (e) {
            if (loadGen === _visitorKeyRefDocsGen) {
                _visitorKeyRefDocsLoadOk = false;
            }
            console.error('[ManageKeys] load ref docs:', e);
            if (err && loadGen === _visitorKeyRefDocsGen) {
                err.textContent = e.message || 'Failed to load documents.';
                err.style.display = 'block';
            }
        } finally {
            if (loading) loading.style.display = 'none';
        }
    }

    async function _putVisitorKeyRefDocAllowlists(hintId) {
        const list = document.getElementById('visitor-key-refdocs-list');
        const sensList = document.getElementById('visitor-key-refdocs-sensitive-list');
        if (!hintId || !list) return;
        const ids = [];
        list.querySelectorAll('input[type="checkbox"][data-doc-id]').forEach(cb => {
            if (cb.checked) ids.push(parseInt(cb.dataset.docId, 10));
        });
        const sensIds = [];
        if (sensList) {
            sensList.querySelectorAll('input[type="checkbox"][data-doc-id]').forEach(cb => {
                if (cb.checked) sensIds.push(parseInt(cb.dataset.docId, 10));
            });
        }
        const [refResp, sensResp] = await Promise.all([
            fetch(`/reference-documents/visitor-key-hints/${hintId}/reference-documents`, {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ document_ids: ids })
            }),
            fetch(`/reference-documents/visitor-key-hints/${hintId}/sensitive-reference-documents`, {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ document_ids: sensIds })
            })
        ]);
        const refData = await refResp.json().catch(() => ({}));
        const sensData = await sensResp.json().catch(() => ({}));
        if (!refResp.ok) throw new Error(refData.detail || `Reference docs: HTTP ${refResp.status}`);
        if (!sensResp.ok) throw new Error(sensData.detail || `Sensitive docs: HTTP ${sensResp.status}`);
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
        _clearVisitorAccessCheckboxes('create-trusted-key');
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
            const access = _visitorAccessPayload('create-trusted-key');
            const resp = await fetch('/sensitive-data/trusted-key', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_password: userPassword,
                    master_password: masterPassword,
                    hint,
                    ...access
                })
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
    /** True only after reference/sensitive doc lists were loaded successfully for the open edit modal (avoids PUT-empty wiping allowlists). */
    let _visitorKeyRefDocsLoadOk = false;
    let _visitorKeyRefDocsGen = 0;

    async function _loadVisitorKeyHintsTable() {
        const loading = document.getElementById('manage-visitor-key-hints-loading');
        const errEl = document.getElementById('manage-visitor-key-hints-error');
        const tbody = document.getElementById('visitor-key-hints-tbody');
        const empty = document.getElementById('manage-visitor-key-hints-empty');
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
                    const tdHint = document.createElement('td');
                    tdHint.textContent = h.hint || '';
                    tdHint.style.maxWidth = '280px';
                    tdHint.style.whiteSpace = 'pre-wrap';
                    tdHint.style.wordBreak = 'break-word';
                    function mkAccessIconCell(field, on) {
                        const td = document.createElement('td');
                        td.style.textAlign = 'center';
                        const title = _visitorKeyAccessColTitles[field] || field;
                        const span = document.createElement('span');
                        span.className = 'visitor-key-table-access-icon';
                        span.title = title;
                        span.setAttribute('aria-label', `${on ? 'Allowed' : 'Not allowed'}: ${title}`);
                        const ic = document.createElement('i');
                        ic.className = on ? 'fas fa-check visitor-key-access-yes' : 'fas fa-times visitor-key-access-no';
                        ic.setAttribute('aria-hidden', 'true');
                        span.appendChild(ic);
                        td.appendChild(span);
                        return td;
                    }
                    const tdCreated = document.createElement('td');
                    tdCreated.textContent = h.created_at ? new Date(h.created_at).toLocaleString() : '';
                    const tdAct = document.createElement('td');
                    tdAct.style.textAlign = 'center';
                    const editBtn = document.createElement('button');
                    editBtn.type = 'button';
                    editBtn.className = 'visitor-key-hint-action-btn visitor-key-hint-action-btn--edit';
                    editBtn.title = 'Edit visitor key hint';
                    editBtn.setAttribute('aria-label', 'Edit visitor key hint');
                    editBtn.innerHTML = '<i class="fas fa-edit" aria-hidden="true"></i> Edit';
                    editBtn.addEventListener('click', () => _openEditVisitorKeyHint(h));
                    const delBtn = document.createElement('button');
                    delBtn.type = 'button';
                    delBtn.className = 'visitor-key-hint-action-btn visitor-key-hint-action-btn--delete';
                    delBtn.title = 'Delete visitor key hint';
                    delBtn.setAttribute('aria-label', 'Delete visitor key hint');
                    delBtn.innerHTML = '<i class="fas fa-trash-alt" aria-hidden="true"></i> Delete';
                    delBtn.addEventListener('click', () => _deleteVisitorKeyHintRow(h.id));
                    tdAct.appendChild(editBtn);
                    tdAct.appendChild(delBtn);
                    tr.appendChild(tdHint);
                    tr.appendChild(mkAccessIconCell('can_messages_chat', h.can_messages_chat));
                    tr.appendChild(mkAccessIconCell('can_emails', h.can_emails));
                    tr.appendChild(mkAccessIconCell('can_contacts', h.can_contacts));
                    tr.appendChild(mkAccessIconCell('can_relationships', h.can_relationships));
                    tr.appendChild(mkAccessIconCell('can_sensitive_private', h.can_sensitive_private));
                    tr.appendChild(mkAccessIconCell('llm_allow_owner_keys', h.llm_allow_owner_keys));
                    tr.appendChild(mkAccessIconCell('llm_allow_server_keys', h.llm_allow_server_keys));
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
        _clearVisitorAccessCheckboxes('edit-visitor-key');
        if (err) { err.textContent = ''; err.style.display = 'none'; }
        _resetVisitorKeyRefDocsUI();
    }

    function _openEditVisitorKeyHint(h) {
        const id = h && h.id;
        const hint = h && h.hint ? h.hint : '';
        const m = document.getElementById('edit-visitor-key-hint-modal');
        const idEl = document.getElementById('edit-visitor-key-hint-id');
        const ta = document.getElementById('edit-visitor-key-hint-text');
        const err = document.getElementById('edit-visitor-key-hint-error');
        if (idEl) idEl.value = String(id);
        if (ta) ta.value = hint;
        _setVisitorAccessCheckboxes('edit-visitor-key', h || {});
        if (err) { err.textContent = ''; err.style.display = 'none'; }
        _resetVisitorKeyRefDocsUI();
        const loadGen = _visitorKeyRefDocsGen;
        if (m) m.style.display = 'flex';
        _loadVisitorKeyRefDocsForHint(id, loadGen);
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
            const access = _visitorAccessPayload('edit-visitor-key');
            const resp = await fetch(`/reference-documents/visitor-key-hints/${id}`, {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                body: JSON.stringify({ hint, ...access })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.detail || `HTTP ${resp.status}`);
            if (_visitorKeyRefDocsLoadOk) {
                await _putVisitorKeyRefDocAllowlists(id);
            }
            _closeEditVisitorKeyHintModal();
            _showStatus(data.message || 'Visitor key updated.');
            _loadVisitorKeyHintsTable();
        } catch (e) {
            console.error('[ManageKeys] update visitor key error:', e);
            if (err) { err.textContent = e.message || 'Failed to save.'; err.style.display = 'block'; }
        }
    }

    async function _deleteVisitorKeyHintRow(id) {
        const ok1 = await AppDialogs.showAppConfirm(
            'Remove visitor key',
            'Remove this visitor key seat entirely? The hint and key will be deleted. This cannot be undone.',
            { danger: true }
        );
        if (!ok1) return;
        const ok2 = await AppDialogs.showAppConfirm('Final confirmation', 'Final confirmation: delete this visitor key?', { danger: true });
        if (!ok2) return;
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
        _clearVisitorAccessCheckboxes('create-visitor-key');
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
        _clearVisitorAccessCheckboxes('create-visitor-key');
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
            const access = _visitorAccessPayload('create-visitor-key');
            const resp = await fetch('/reference-documents/visitor-key-hints', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                body: JSON.stringify({ keyring_id: keyringId, hint, ...access })
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
        _clearVisitorAccessCheckboxes('add-doc-key');
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
            const access = _visitorAccessPayload('add-doc-key');
            const resp = await fetch('/reference-documents/add-user', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_password: userPassword,
                    master_password: masterPassword,
                    hint,
                    ...access
                })
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
        const ok1 = await AppDialogs.showAppConfirm(
            'Delete all visitor keys',
            'Delete ALL visitor document keys?\n\nThe owner master key will stay. Visitor keys and their unlock hints will be removed. This cannot be undone.',
            { danger: true }
        );
        if (!ok1) {
            return;
        }
        const ok2 = await AppDialogs.showAppConfirm(
            'Final confirmation',
            'Final confirmation: remove every visitor keyring seat now?',
            { danger: true }
        );
        if (!ok2) {
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

        const tabRef = document.getElementById('visitor-key-refdocs-tab-ref');
        if (tabRef) tabRef.addEventListener('click', () => { _visitorKeyRefDocsSetTab('ref'); });
        const tabSens = document.getElementById('visitor-key-refdocs-tab-sensitive');
        if (tabSens) tabSens.addEventListener('click', () => { _visitorKeyRefDocsSetTab('sensitive'); });

        // Load keyring count on page ready
        _loadDocKeyringCount();
        _loadVisitorKeyHintsTable();
    }

    return { init };
})();


Modals.InactivityPromptConfig = (() => {
    const CONFIG_KEY = 'chat_inactivity_nudge';
    const DEFAULTS = { enabled: false, min_seconds: 120, max_seconds: 300 };
    const MIN_ALLOWED = 30;

    function getEl(id) { return document.getElementById(id); }

    function setStatus(msg, isErr) {
        const el = getEl('inactivity-prompt-status');
        if (!el) return;
        el.textContent = msg || '';
        el.style.color = isErr ? '#dc3545' : (msg ? '#1a7f37' : 'var(--color-text-muted)');
    }

    function populateForm(settings) {
        const enabledEl = getEl('inactivity-prompt-enabled');
        const minEl = getEl('inactivity-prompt-min-seconds');
        const maxEl = getEl('inactivity-prompt-max-seconds');
        if (enabledEl) enabledEl.checked = !!settings.enabled;
        if (minEl) minEl.value = String(settings.min_seconds);
        if (maxEl) maxEl.value = String(settings.max_seconds);
    }

    function readFormSettings() {
        const enabledEl = getEl('inactivity-prompt-enabled');
        const minEl = getEl('inactivity-prompt-min-seconds');
        const maxEl = getEl('inactivity-prompt-max-seconds');
        let minSeconds = parseInt(minEl?.value, 10);
        let maxSeconds = parseInt(maxEl?.value, 10);
        if (!Number.isFinite(minSeconds) || minSeconds < MIN_ALLOWED) {
            throw new Error(`Minimum seconds must be at least ${MIN_ALLOWED}.`);
        }
        if (!Number.isFinite(maxSeconds) || maxSeconds < MIN_ALLOWED) {
            throw new Error(`Maximum seconds must be at least ${MIN_ALLOWED}.`);
        }
        if (minSeconds > maxSeconds) {
            throw new Error('Minimum seconds cannot exceed maximum seconds.');
        }
        return {
            enabled: !!(enabledEl && enabledEl.checked),
            min_seconds: minSeconds,
            max_seconds: maxSeconds,
        };
    }

    function parseSettingsFromRows(rows) {
        const row = Array.isArray(rows) ? rows.find((r) => r && r.key === CONFIG_KEY) : null;
        if (!row || row.value == null || String(row.value).trim() === '') return { ...DEFAULTS };
        try {
            const parsed = JSON.parse(row.value);
            return {
                enabled: !!parsed.enabled,
                min_seconds: Math.max(MIN_ALLOWED, parseInt(parsed.min_seconds, 10) || DEFAULTS.min_seconds),
                max_seconds: Math.max(MIN_ALLOWED, parseInt(parsed.max_seconds, 10) || DEFAULTS.max_seconds),
            };
        } catch (_) {
            return { ...DEFAULTS };
        }
    }

    async function load() {
        setStatus('Loading…');
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const rows = await res.json();
            const settings = parseSettingsFromRows(rows);
            if (settings.min_seconds > settings.max_seconds) {
                const swap = settings.min_seconds;
                settings.min_seconds = settings.max_seconds;
                settings.max_seconds = swap;
            }
            populateForm(settings);
            setStatus('');
        } catch (e) {
            setStatus(`Error loading settings: ${e.message}`, true);
        }
    }

    async function save() {
        let settings;
        try {
            settings = readFormSettings();
        } catch (e) {
            setStatus(e.message, true);
            return;
        }
        const saveBtn = getEl('inactivity-prompt-save-btn');
        if (saveBtn) saveBtn.disabled = true;
        setStatus('Saving…');
        try {
            const res = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    key: CONFIG_KEY,
                    value: JSON.stringify(settings),
                    description: 'Inactivity chat prompt timer settings',
                }),
            });
            if (!res.ok) {
                let detail = await res.text();
                try {
                    const j = JSON.parse(detail);
                    if (j.detail) detail = j.detail;
                } catch (_) { /* keep body */ }
                throw new Error(detail || `HTTP ${res.status}`);
            }
            setStatus('Saved.');
            if (typeof ChatInactivityNudge !== 'undefined' && ChatInactivityNudge.reloadSettings) {
                await ChatInactivityNudge.reloadSettings();
            }
        } catch (e) {
            setStatus(`Error saving: ${e.message}`, true);
        } finally {
            if (saveBtn) saveBtn.disabled = false;
        }
    }

    function init() {
        const saveBtn = getEl('inactivity-prompt-save-btn');
        if (saveBtn) saveBtn.addEventListener('click', () => { void save(); });
    }

    return { init, load, save };
})();


Modals.HostedLLMOrderConfig = (() => {
    const CONFIG_KEY = 'hosted_llm_provider_order_v1';
    const DEFAULT_ORDER = ['gemini', 'claude', 'deepseek', 'openai'];
    const DEFAULT_CLASSIFIER_PROVIDER = 'localai';
    const PROVIDER_LABELS = {
        localai: 'Local AI',
        gemini: 'Gemini',
        claude: 'Claude',
        deepseek: 'DeepSeek',
        openai: 'ChatGPT',
    };

    let cached = {
        auto_order: DEFAULT_ORDER.slice(),
        failover_order: DEFAULT_ORDER.slice(),
        classifier_provider: DEFAULT_CLASSIFIER_PROVIDER,
        failover_enabled: true,
    };
    let working = null;

    function getEl(id) { return document.getElementById(id); }

    function cloneOrder(order) {
        return Array.isArray(order) ? order.slice() : DEFAULT_ORDER.slice();
    }

    function isLocalAIAllowedForChat() {
        if (typeof App !== 'undefined' && typeof App.isLocalAIUseEnabledForChat === 'function') {
            return App.isLocalAIUseEnabledForChat({});
        }
        const checkbox = document.getElementById('local-ai-use-enabled-checkbox');
        if (checkbox) return !!checkbox.checked;
        if (typeof LocalAiSetup !== 'undefined'
            && typeof LocalAiSetup.isUseEnabledForChat === 'function') {
            return LocalAiSetup.isUseEnabledForChat();
        }
        return true;
    }

    function defaultClassifierProvider() {
        return isLocalAIAllowedForChat() ? DEFAULT_CLASSIFIER_PROVIDER : 'gemini';
    }

    function normalizeClassifierProvider(value) {
        const key = String(value || '').toLowerCase().trim();
        if (key === 'localai' && !isLocalAIAllowedForChat()) {
            return 'gemini';
        }
        return PROVIDER_LABELS[key] ? key : defaultClassifierProvider();
    }

    function normalizeOrder(order) {
        const valid = { gemini: true, claude: true, deepseek: true, openai: true };
        const out = [];
        const seen = {};
        (order || []).forEach((p) => {
            const key = String(p || '').toLowerCase().trim();
            if (!valid[key] || seen[key]) return;
            seen[key] = true;
            out.push(key);
        });
        DEFAULT_ORDER.forEach((p) => {
            if (!seen[p]) out.push(p);
        });
        return out;
    }

    function parseSettingsFromRows(rows) {
        const row = Array.isArray(rows) ? rows.find((r) => r && r.key === CONFIG_KEY) : null;
        if (!row || row.value == null || String(row.value).trim() === '') {
            return {
                auto_order: DEFAULT_ORDER.slice(),
                failover_order: DEFAULT_ORDER.slice(),
                classifier_provider: DEFAULT_CLASSIFIER_PROVIDER,
                failover_enabled: true,
            };
        }
        try {
            const parsed = JSON.parse(row.value);
            return {
                auto_order: normalizeOrder(parsed.auto_order),
                failover_order: normalizeOrder(parsed.failover_order),
                classifier_provider: normalizeClassifierProvider(parsed.classifier_provider),
                failover_enabled: parsed.failover_enabled !== false,
            };
        } catch (_) {
            return {
                auto_order: DEFAULT_ORDER.slice(),
                failover_order: DEFAULT_ORDER.slice(),
                classifier_provider: DEFAULT_CLASSIFIER_PROVIDER,
                failover_enabled: true,
            };
        }
    }

    function setStatus(msg, isErr) {
        const el = getEl('hosted-llm-order-status');
        if (!el) return;
        el.textContent = msg || '';
        el.style.color = isErr ? '#dc3545' : (msg ? '#1a7f37' : 'var(--color-text-muted)');
    }

    function renderOrderList(listId, orderKey) {
        const listEl = getEl(listId);
        if (!listEl || !working) return;
        const order = working[orderKey] || DEFAULT_ORDER.slice();
        listEl.innerHTML = '';
        order.forEach((provider, idx) => {
            const li = document.createElement('li');
            li.className = 'hosted-llm-order-item';
            li.dataset.provider = provider;
            li.innerHTML = `
                <span class="hosted-llm-order-rank" aria-hidden="true">${idx + 1}.</span>
                <span class="hosted-llm-order-label">${PROVIDER_LABELS[provider] || provider}</span>
                <button type="button" class="modal-btn modal-btn-secondary hosted-llm-order-move-btn" data-move="up" data-order-key="${orderKey}" data-provider="${provider}" aria-label="Move ${PROVIDER_LABELS[provider] || provider} up" ${idx === 0 ? 'disabled' : ''}><i class="fas fa-arrow-up" aria-hidden="true"></i></button>
                <button type="button" class="modal-btn modal-btn-secondary hosted-llm-order-move-btn" data-move="down" data-order-key="${orderKey}" data-provider="${provider}" aria-label="Move ${PROVIDER_LABELS[provider] || provider} down" ${idx === order.length - 1 ? 'disabled' : ''}><i class="fas fa-arrow-down" aria-hidden="true"></i></button>
            `;
            listEl.appendChild(li);
        });
    }

    function syncFailoverOrderListState() {
        const enabled = working ? working.failover_enabled !== false : cached.failover_enabled !== false;
        const checkbox = getEl('hosted-llm-failover-enabled-checkbox');
        if (checkbox) checkbox.checked = enabled;
        const listEl = getEl('hosted-llm-failover-order-list');
        if (listEl) listEl.style.opacity = enabled ? '' : '0.5';
        if (listEl) {
            listEl.querySelectorAll('.hosted-llm-order-move-btn').forEach((btn) => {
                btn.disabled = !enabled;
            });
        }
    }

    function renderAllLists() {
        renderOrderList('hosted-llm-auto-order-list', 'auto_order');
        renderOrderList('hosted-llm-failover-order-list', 'failover_order');
        syncFailoverOrderListState();
    }

    function moveProvider(orderKey, provider, direction) {
        if (!working || !working[orderKey]) return;
        const order = working[orderKey].slice();
        const idx = order.indexOf(provider);
        if (idx < 0) return;
        const swapWith = direction === 'up' ? idx - 1 : idx + 1;
        if (swapWith < 0 || swapWith >= order.length) return;
        const tmp = order[idx];
        order[idx] = order[swapWith];
        order[swapWith] = tmp;
        working[orderKey] = order;
        renderAllLists();
    }

    function populateForm(settings) {
        working = {
            auto_order: cloneOrder(settings.auto_order),
            failover_order: cloneOrder(settings.failover_order),
            classifier_provider: normalizeClassifierProvider(settings.classifier_provider),
            failover_enabled: settings.failover_enabled !== false,
        };
        const classifierSelect = getEl('hosted-llm-classifier-provider');
        if (classifierSelect) {
            classifierSelect.value = working.classifier_provider;
        }
        renderAllLists();
    }

    async function load() {
        setStatus('Loading…');
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const rows = await res.json();
            const settings = parseSettingsFromRows(rows);
            cached = settings;
            populateForm(settings);
            setStatus('');
        } catch (e) {
            populateForm(cached);
            setStatus(`Error loading settings: ${e.message}`, true);
        }
    }

    async function ensureLoaded() {
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) return;
            const rows = await res.json();
            cached = parseSettingsFromRows(rows);
        } catch (_) { /* keep defaults */ }
    }

    async function save() {
        if (!working) return;
        const classifierSelect = getEl('hosted-llm-classifier-provider');
        if (classifierSelect) {
            working.classifier_provider = normalizeClassifierProvider(classifierSelect.value);
        }
        const failoverCheckbox = getEl('hosted-llm-failover-enabled-checkbox');
        if (failoverCheckbox) {
            working.failover_enabled = !!failoverCheckbox.checked;
        }
        const payload = {
            auto_order: normalizeOrder(working.auto_order),
            failover_order: normalizeOrder(working.failover_order),
            classifier_provider: normalizeClassifierProvider(working.classifier_provider),
            failover_enabled: working.failover_enabled !== false,
        };
        const saveBtn = getEl('hosted-llm-order-save-btn');
        if (saveBtn) saveBtn.disabled = true;
        setStatus('Saving…');
        try {
            const res = await fetch('/api/configuration', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    key: CONFIG_KEY,
                    value: JSON.stringify(payload),
                    description: 'Hosted LLM provider try order for Auto routing and error failover, plus Auto classifier provider',
                }),
            });
            if (!res.ok) {
                let detail = await res.text();
                try {
                    const j = JSON.parse(detail);
                    if (j.detail) detail = j.detail;
                } catch (_) { /* keep body */ }
                throw new Error(detail || `HTTP ${res.status}`);
            }
            cached = payload;
            working = {
                auto_order: cloneOrder(payload.auto_order),
                failover_order: cloneOrder(payload.failover_order),
                classifier_provider: normalizeClassifierProvider(payload.classifier_provider),
                failover_enabled: payload.failover_enabled !== false,
            };
            renderAllLists();
            setStatus('Saved.');
        } catch (e) {
            setStatus(`Error saving: ${e.message}`, true);
        } finally {
            if (saveBtn) saveBtn.disabled = false;
        }
    }

    function resetToDefault() {
        working = {
            auto_order: DEFAULT_ORDER.slice(),
            failover_order: DEFAULT_ORDER.slice(),
            classifier_provider: DEFAULT_CLASSIFIER_PROVIDER,
            failover_enabled: true,
        };
        const classifierSelect = getEl('hosted-llm-classifier-provider');
        if (classifierSelect) {
            classifierSelect.value = DEFAULT_CLASSIFIER_PROVIDER;
        }
        renderAllLists();
        setStatus('Reset to defaults (not saved yet).');
    }

    function getClassifierProvider() {
        return normalizeClassifierProvider(cached.classifier_provider);
    }

    function reconcileClassifierProvider() {
        const classifierSelect = getEl('hosted-llm-classifier-provider');
        if (!classifierSelect) return;
        const resolved = normalizeClassifierProvider(
            (working && working.classifier_provider) || cached.classifier_provider || classifierSelect.value
        );
        if (working) working.classifier_provider = resolved;
        cached.classifier_provider = resolved;
        if (classifierSelect.querySelector(`option[value="${resolved}"]`)) {
            classifierSelect.value = resolved;
        }
    }

    function getAutoOrder() {
        return cloneOrder(cached.auto_order);
    }

    function getFailoverOrder() {
        return cloneOrder(cached.failover_order);
    }

    function getFailoverCandidates(primary) {
        if (!isFailoverEnabled()) return [];
        const p = String(primary || '').toLowerCase().trim();
        return getFailoverOrder().filter((name) => name !== p);
    }

    function isFailoverEnabled() {
        return cached.failover_enabled !== false;
    }

    function init() {
        const saveBtn = getEl('hosted-llm-order-save-btn');
        const resetBtn = getEl('hosted-llm-order-reset-btn');
        const classifierSelect = getEl('hosted-llm-classifier-provider');
        const failoverCheckbox = getEl('hosted-llm-failover-enabled-checkbox');
        if (saveBtn) saveBtn.addEventListener('click', () => { void save(); });
        if (resetBtn) resetBtn.addEventListener('click', resetToDefault);
        if (failoverCheckbox) {
            failoverCheckbox.addEventListener('change', () => {
                if (working) working.failover_enabled = !!failoverCheckbox.checked;
                syncFailoverOrderListState();
            });
        }
        if (classifierSelect) {
            classifierSelect.addEventListener('change', () => {
                if (working) {
                    working.classifier_provider = normalizeClassifierProvider(classifierSelect.value);
                }
            });
        }
        document.addEventListener('click', (e) => {
            const btn = e.target && e.target.closest ? e.target.closest('.hosted-llm-order-move-btn') : null;
            if (!btn) return;
            const orderKey = btn.getAttribute('data-order-key');
            const provider = btn.getAttribute('data-provider');
            const direction = btn.getAttribute('data-move');
            if (!orderKey || !provider || !direction) return;
            moveProvider(orderKey, provider, direction);
        });
    }

    return {
        init,
        load,
        save,
        ensureLoaded,
        getAutoOrder,
        getFailoverOrder,
        getFailoverCandidates,
        isFailoverEnabled,
        getClassifierProvider,
        reconcileClassifierProvider,
    };
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
                _status(data.detail || 'Could not load policy. Unlock the keyring first.', true);
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
                tdDesc.textContent = t.description || '';
                tr.appendChild(tdName);
                tr.appendChild(tdDesc);
                const tdMaster = document.createElement('td');
                tdMaster.style.textAlign = 'center';
                const masterChk = document.createElement('input');
                masterChk.type = 'checkbox';
                masterChk.className = 'llm-tool-chk';
                masterChk.dataset.name = name;
                masterChk.dataset.flag = 'enabled';
                masterChk.checked = !!t.enabled;
                tdMaster.appendChild(masterChk);
                tr.appendChild(tdMaster);
                const tdVisitor = document.createElement('td');
                tdVisitor.style.textAlign = 'center';
                const visitorChk = document.createElement('input');
                visitorChk.type = 'checkbox';
                visitorChk.className = 'llm-tool-chk';
                visitorChk.dataset.name = name;
                visitorChk.dataset.flag = 'visitor_enabled';
                visitorChk.checked = !!t.visitor_enabled;
                tdVisitor.appendChild(visitorChk);
                tr.appendChild(tdVisitor);
                tbody.appendChild(tr);
            }
        } catch (e) {
            _status(e.message || 'Load failed', true);
        }
    }

    async function save() {
        const saveBtn = document.getElementById('llm-tools-access-save');
        if (saveBtn) {
            saveBtn.disabled = true;
            saveBtn.textContent = 'Saving...';
        }
        const rows = {};
        document.querySelectorAll('#llm-tools-access-tbody .llm-tool-chk').forEach((chk) => {
            const name = chk.dataset.name;
            const flag = chk.dataset.flag;
            if (!name || !flag) return;
            if (!rows[name]) {
                rows[name] = { name: name, enabled: false, visitor_enabled: false };
            }
            if (flag === 'enabled') rows[name].enabled = chk.checked;
            if (flag === 'visitor_enabled') rows[name].visitor_enabled = chk.checked;
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
                if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
                    await AppDialogs.showAppAlert('Tools Access', data.detail || 'Save failed');
                }
                return;
            }
            if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
                await AppDialogs.showAppAlert('Tools Access', data.message || 'Tool access policy saved successfully.');
            }
            if (typeof App !== 'undefined' && App.refreshChatAvailability) {
                void App.refreshChatAvailability();
            }
        } catch (e) {
            if (typeof AppDialogs !== 'undefined' && AppDialogs.showAppAlert) {
                await AppDialogs.showAppAlert('Tools Access', e.message || 'Save failed');
            }
        } finally {
            if (saveBtn) {
                saveBtn.disabled = false;
                saveBtn.textContent = 'Save';
            }
        }
    }

    function init() {
        const btn = document.getElementById('llm-tools-access-save');
        if (btn) btn.addEventListener('click', () => void save());
    }

    return { init, load, save };
})();


Modals.BackgroundJobs = (() => {
    let pollTimer = null;
    let saveTimer = null;
    let cachedJobs = [];

    function _status(msg, isErr) {
        const el = document.getElementById('background-jobs-status');
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

    function _setLoading(loading) {
        const loadingEl = document.getElementById('background-jobs-loading');
        const tableWrap = document.getElementById('background-jobs-table-wrap');
        const empty = document.getElementById('background-jobs-empty');
        const panel = document.getElementById('data-import-panel-background-jobs');
        if (loadingEl) loadingEl.style.display = loading ? 'flex' : 'none';
        if (tableWrap) tableWrap.style.display = loading ? 'none' : '';
        if (loading && empty) empty.style.display = 'none';
        if (panel) panel.setAttribute('aria-busy', loading ? 'true' : 'false');
    }

    function _formatLocal(ts) {
        if (!ts) return '—';
        try {
            const d = new Date(ts);
            if (isNaN(d.getTime())) return '—';
            return d.toLocaleString();
        } catch (e) {
            return String(ts);
        }
    }

    function _renderRow(job) {
        const tr = document.createElement('tr');
        tr.dataset.name = job.name;

        const tdName = document.createElement('td');
        tdName.style.maxWidth = '320px';
        const strong = document.createElement('strong');
        strong.textContent = job.title || job.name;
        tdName.appendChild(strong);
        if (job.description) {
            const small = document.createElement('div');
            small.style.fontSize = '0.85em';
            small.style.marginTop = '2px';
            small.textContent = job.description;
            tdName.appendChild(small);
        }
        tr.appendChild(tdName);

        const tdAuto = document.createElement('td');
        tdAuto.style.textAlign = 'center';
        const autoChk = document.createElement('input');
        autoChk.type = 'checkbox';
        autoChk.className = 'bg-job-chk';
        autoChk.dataset.field = 'auto_start';
        autoChk.checked = !!job.auto_start;
        autoChk.addEventListener('change', () => _saveRow(tr));
        tdAuto.appendChild(autoChk);
        tr.appendChild(tdAuto);

        const tdRestart = document.createElement('td');
        tdRestart.style.textAlign = 'center';
        const restartChk = document.createElement('input');
        restartChk.type = 'checkbox';
        restartChk.className = 'bg-job-chk';
        restartChk.dataset.field = 'restart_on_complete';
        restartChk.checked = !!job.restart_on_complete;
        restartChk.addEventListener('change', () => _saveRow(tr));
        tdRestart.appendChild(restartChk);
        tr.appendChild(tdRestart);

        const tdInterval = document.createElement('td');
        tdInterval.style.textAlign = 'center';
        const intervalInp = document.createElement('input');
        intervalInp.type = 'number';
        intervalInp.min = '30';
        intervalInp.step = '30';
        intervalInp.style.width = '7rem';
        intervalInp.dataset.field = 'interval_seconds';
        intervalInp.value = String(job.interval_seconds || job.default_interval_seconds || 3600);
        intervalInp.addEventListener('change', () => _saveRow(tr));
        intervalInp.addEventListener('blur', () => _saveRow(tr));
        tdInterval.appendChild(intervalInp);
        tr.appendChild(tdInterval);

        const tdLast = document.createElement('td');
        tdLast.style.fontSize = '0.85em';
        const lastLine = document.createElement('div');
        lastLine.textContent = _formatLocal(job.last_run_at);
        tdLast.appendChild(lastLine);
        if (job.last_run_result) {
            const showResult = job.in_progress || job.last_run_result !== 'running';
            if (showResult) {
            const resLine = document.createElement('div');
            resLine.style.color = job.last_run_result === 'error' ? '#dc3545' :
                                  job.last_run_result === 'cancelled' ? '#856404' :
                                  job.last_run_result === 'running' ? '#0c63e4' : '#1a7f37';
            resLine.textContent = job.last_run_result;
            tdLast.appendChild(resLine);
            }
        }
        tr.appendChild(tdLast);

        const tdStatus = document.createElement('td');
        tdStatus.style.fontSize = '0.85em';
        if (job.in_progress) {
            const dot = document.createElement('span');
            dot.style.display = 'inline-block';
            dot.style.width = '8px';
            dot.style.height = '8px';
            dot.style.borderRadius = '50%';
            dot.style.background = '#0c63e4';
            dot.style.marginRight = '6px';
            tdStatus.appendChild(dot);
        }
        const statusText = document.createElement('span');
        statusText.textContent = job.status_line || (job.in_progress ? 'running' : 'idle');
        tdStatus.appendChild(statusText);
        tr.appendChild(tdStatus);

        const tdActions = document.createElement('td');
        tdActions.style.textAlign = 'center';
        const startBtn = document.createElement('button');
        startBtn.type = 'button';
        startBtn.className = 'modal-btn modal-btn-primary';
        startBtn.style.marginRight = '4px';
        startBtn.textContent = 'Start';
        startBtn.disabled = !!job.in_progress;
        startBtn.addEventListener('click', () => _startJob(job.name));
        tdActions.appendChild(startBtn);
        const stopBtn = document.createElement('button');
        stopBtn.type = 'button';
        stopBtn.className = 'modal-btn modal-btn-secondary';
        stopBtn.textContent = 'Stop';
        stopBtn.disabled = !job.in_progress;
        stopBtn.addEventListener('click', () => _stopJob(job.name));
        tdActions.appendChild(stopBtn);
        tr.appendChild(tdActions);

        return tr;
    }

    function _render(jobs) {
        cachedJobs = Array.isArray(jobs) ? jobs : [];
        const tbody = document.getElementById('background-jobs-tbody');
        const empty = document.getElementById('background-jobs-empty');
        if (!tbody) return;
        tbody.replaceChildren();
        if (!cachedJobs.length) {
            if (empty) empty.style.display = 'block';
            return;
        }
        if (empty) empty.style.display = 'none';
        for (const job of cachedJobs) {
            tbody.appendChild(_renderRow(job));
        }
    }

    async function load() {
        _status('', false);
        _setLoading(true);
        try {
            const res = await fetch('/api/background-jobs', { credentials: 'same-origin' });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                _status(data.detail || 'Could not load background jobs.', true);
                return;
            }
            _render(data.jobs || []);
            _ensurePolling();
        } catch (e) {
            _status(e.message || 'Load failed', true);
        } finally {
            _setLoading(false);
        }
    }

    function _ensurePolling() {
        const modal = document.getElementById('data-import-modal');
        const tab = document.getElementById('data-import-panel-background-jobs');
        if (!modal || !tab) return;
        if (pollTimer) return;
        pollTimer = setInterval(() => {
            const visible = modal.style.display !== 'none' && tab.classList.contains('data-import-category-panel--active');
            if (!visible) {
                clearInterval(pollTimer);
                pollTimer = null;
                return;
            }
            void _refreshOnly();
        }, 5000);
    }

    async function _refreshOnly() {
        try {
            const res = await fetch('/api/background-jobs', { credentials: 'same-origin' });
            if (!res.ok) return;
            const data = await res.json();
            _render(data.jobs || []);
        } catch (_) { /* ignore polling errors */ }
    }

    function _saveRow(tr) {
        if (!tr) return;
        const name = tr.dataset.name;
        if (!name) return;
        const auto = tr.querySelector('input[data-field="auto_start"]');
        const restart = tr.querySelector('input[data-field="restart_on_complete"]');
        const interval = tr.querySelector('input[data-field="interval_seconds"]');
        const body = {
            auto_start: !!(auto && auto.checked),
            restart_on_complete: !!(restart && restart.checked),
            interval_seconds: Math.max(30, parseInt(interval && interval.value, 10) || 3600)
        };
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(async () => {
            try {
                const res = await fetch('/api/background-jobs/' + encodeURIComponent(name), {
                    method: 'PUT',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body)
                });
                if (!res.ok) {
                    const data = await res.json().catch(() => ({}));
                    _status(data.detail || 'Save failed for ' + name, true);
                    return;
                }
                _status('Saved ' + name, false);
                setTimeout(() => _status('', false), 2000);
            } catch (e) {
                _status(e.message || 'Save failed', true);
            }
        }, 250);
    }

    async function _startJob(name) {
        _status('', false);
        try {
            const res = await fetch('/api/background-jobs/' + encodeURIComponent(name) + '/start', {
                method: 'POST',
                credentials: 'same-origin'
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                _status(data.detail || 'Could not start ' + name, true);
                return;
            }
            await _refreshOnly();
        } catch (e) {
            _status(e.message || 'Start failed', true);
        }
    }

    async function _stopJob(name) {
        _status('', false);
        try {
            const res = await fetch('/api/background-jobs/' + encodeURIComponent(name) + '/stop', {
                method: 'POST',
                credentials: 'same-origin'
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                _status(data.detail || 'Could not stop ' + name, true);
                return;
            }
            await _refreshOnly();
        } catch (e) {
            _status(e.message || 'Stop failed', true);
        }
    }

    function init() {
        // Nothing to wire eagerly — load() handles row creation and listeners.
    }

    return { init, load };
})();


Modals.initAll = () => {
        Modals.Suggestions.init();
        Modals.FBAlbums.init();
        Modals.FBPosts.init();
        Modals.ImageDetailModal.init();
        Modals.MultiImageDisplay.init();
        //Modals.HaveYourSay.init();
        Modals.Locations.init();
        Modals.NearbyLocations.init();
        if (Modals.BulkGpsPicker && Modals.BulkGpsPicker.init) Modals.BulkGpsPicker.init();
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
        Modals.ReferenceDocumentsNotification.init();
        Modals.ConversationManager.init();
        Modals.SubjectConfiguration.init();
        Modals.Artefacts.init();
        Modals.SensitiveData.init();
        Modals.ManageKeys.init();
        if (Modals.LLMToolsAccess && Modals.LLMToolsAccess.init) Modals.LLMToolsAccess.init();
        if (Modals.InactivityPromptConfig && Modals.InactivityPromptConfig.init) Modals.InactivityPromptConfig.init();
        if (Modals.HostedLLMOrderConfig && Modals.HostedLLMOrderConfig.init) Modals.HostedLLMOrderConfig.init();
        if (Modals.ToolTest && Modals.ToolTest.init) Modals.ToolTest.init();
        if (Modals.BackgroundJobs && Modals.BackgroundJobs.init) Modals.BackgroundJobs.init();
        Modals.Profiles.init();
        if (Modals.EmailMatches && Modals.EmailMatches.init) Modals.EmailMatches.init();
        if (Modals.EmailClassifications && Modals.EmailClassifications.init) Modals.EmailClassifications.init();
        if (Modals.Interests && Modals.Interests.init) Modals.Interests.init();
        if (Modals.CustomVoices && Modals.CustomVoices.init) Modals.CustomVoices.init();
        if (Modals.EmailExclusions && Modals.EmailExclusions.init) Modals.EmailExclusions.init();
        if (Modals.PreviousResponses && Modals.PreviousResponses.init) Modals.PreviousResponses.init();
        if (Modals.SaveResponseTitle && Modals.SaveResponseTitle.init) Modals.SaveResponseTitle.init();
        if (Modals.UserLLMSettings && Modals.UserLLMSettings.init) Modals.UserLLMSettings.init();
        if (typeof LocalAiSetup !== 'undefined' && LocalAiSetup.init) void LocalAiSetup.init();
        if (typeof HaveAChat !== 'undefined' && HaveAChat.init) HaveAChat.init();
        if (Modals.IdentityProfileWizard && Modals.IdentityProfileWizard.init) Modals.IdentityProfileWizard.init();
};

Modals.closeAll = (options) => {
        const keepOpen = options && options.keepOpen ? options.keepOpen : null;

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
            if (Modals.Contacts && Modals.Contacts.close) Modals.Contacts.close();
        } catch (e) { console.debug('Error closing Contacts modal:', e); }
        
        try {
            if (Modals.Profiles && Modals.Profiles.close) Modals.Profiles.close();
        } catch (e) { console.debug('Error closing Profiles modal:', e); }
        
        try {
            if (Modals.Locations && Modals.Locations.close) Modals.Locations.close();
        } catch (e) { console.debug('Error closing Locations modal:', e); }

        try {
            if (Modals.NearbyLocations && Modals.NearbyLocations.close && keepOpen !== DOM.nearbyLocationsModal) {
                Modals.NearbyLocations.close();
            }
        } catch (e) { console.debug('Error closing NearbyLocations modal:', e); }

        try {
            if (Modals.BulkGpsPicker && Modals.BulkGpsPicker.close && keepOpen !== DOM.bulkGpsPickModal) {
                Modals.BulkGpsPicker.close();
            }
        } catch (e) { console.debug('Error closing BulkGpsPicker modal:', e); }
        
        try {
            if (Modals.ConfirmationModal && Modals.ConfirmationModal.close) Modals.ConfirmationModal.close();
        } catch (e) { console.debug('Error closing ConfirmationModal:', e); }
        
        try {
            if (Modals.PreviousResponses && Modals.PreviousResponses.close) Modals.PreviousResponses.close();
        } catch (e) { console.debug('Error closing PreviousResponses:', e); }
        
        try {
            if (Modals.SaveResponseTitle && Modals.SaveResponseTitle.close) Modals.SaveResponseTitle.close();
        } catch (e) { console.debug('Error closing SaveResponseTitle:', e); }

        try {
            if (Modals.UserLLMSettings && Modals.UserLLMSettings.closeInfoPopover) Modals.UserLLMSettings.closeInfoPopover();
        } catch (e) { console.debug('Error closing UserLLM info popover:', e); }

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
        
        // Close any other top-level modal shells still visible
        try {
            const allModals = document.querySelectorAll('.modal');
            allModals.forEach(modal => {
                if (keepOpen && modal === keepOpen) return;
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
                <tr>
                    <td>${escapeHtml(row.primary_name)}</td>
                    <td>${escapeHtml(row.email)}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                        <button type="button" class="email-match-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit email match" title="Edit">
                            <i class="fas fa-edit" aria-hidden="true"></i>
                        </button>
                        <button type="button" class="email-match-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete email match" title="Delete">
                            <i class="fas fa-trash-alt" aria-hidden="true"></i>
                        </button>
                        </span>
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
        const ok = await AppDialogs.showAppConfirm('Delete email match', msg, { danger: true });
        if (!ok) return;
        try {
            const res = await fetch(`/email-matches/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailMatches();
        } catch (err) {
            await AppDialogs.showAppAlert('Failed to delete: ' + err.message);
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

        const exportBtn = getEl('email-matches-export-btn');
        const importBtn = getEl('email-matches-import-btn');
        const importFile = getEl('email-matches-import-file');
        if (exportBtn) exportBtn.addEventListener('click', () => { window.location.href = '/email-matches/export'; });
        if (importBtn) importBtn.addEventListener('click', () => importFile && importFile.click());
        if (importFile) importFile.addEventListener('change', async () => {
            if (!importFile.files || !importFile.files[0]) return;
            const file = importFile.files[0];
            importFile.value = '';
            try {
                const text = await file.text();
                const res = await fetch('/email-matches/import', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: text,
                    credentials: 'same-origin',
                });
                const data = await res.json();
                if (!res.ok) throw new Error(data.error || res.statusText);
                await AppDialogs.showAppAlert(`Imported ${data.imported} email match(es).`);
                loadEmailMatches();
            } catch (err) {
                await AppDialogs.showAppAlert('Import failed: ' + err.message);
            }
        });
    }

    return {
        load: loadEmailMatches,
        init: init
    };
})();


// --- Interests (Settings and Data Import) ---
Modals.Interests = (() => {
    let editingId = null;

    const INTEREST_PANEL_IDS = [
        { tbody: 'interests-tbody-sc', loading: 'interests-loading-sc', tableContainer: 'interests-table-container-sc', emptyMsg: 'interests-empty-msg-sc' }
    ];

    function getEl(id) {
        return document.getElementById(id);
    }

    function getInterestPanels() {
        return INTEREST_PANEL_IDS.map((ids) => {
            const tbody = getEl(ids.tbody);
            const loading = getEl(ids.loading);
            if (!tbody || !loading) return null;
            return {
                tbody,
                loading,
                tableContainer: getEl(ids.tableContainer),
                emptyMsg: getEl(ids.emptyMsg)
            };
        }).filter(Boolean);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    function bindInterestRowButtons(tbody) {
        tbody.querySelectorAll('.interest-edit-btn').forEach((btn) => {
            btn.addEventListener('click', () => openEditModal(parseInt(btn.dataset.id, 10)));
        });
        tbody.querySelectorAll('.interest-delete-btn').forEach((btn) => {
            btn.addEventListener('click', () => deleteInterest(parseInt(btn.dataset.id, 10)));
        });
    }

    async function loadInterests() {
        const panels = getInterestPanels();
        if (panels.length === 0) return;

        panels.forEach((p) => {
            p.loading.style.display = 'block';
            if (p.tableContainer) p.tableContainer.style.display = 'none';
            if (p.emptyMsg) p.emptyMsg.style.display = 'none';
        });

        try {
            const response = await fetch('/api/interests');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();

            panels.forEach((p) => {
                p.loading.style.display = 'none';
                if (p.tableContainer) p.tableContainer.style.display = 'block';
            });

            if (!data || data.length === 0) {
                panels.forEach((p) => {
                    p.tbody.innerHTML = '';
                    if (p.emptyMsg) p.emptyMsg.style.display = 'block';
                });
                return;
            }
            panels.forEach((p) => {
                if (p.emptyMsg) p.emptyMsg.style.display = 'none';
            });

            const rowsHtml = data.map((row) => `
                <tr>
                    <td>${escapeHtml(row.name)}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                        <button type="button" class="interest-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit interest" title="Edit">
                            <i class="fas fa-edit" aria-hidden="true"></i>
                        </button>
                        <button type="button" class="interest-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete interest" title="Delete">
                            <i class="fas fa-trash-alt" aria-hidden="true"></i>
                        </button>
                        </span>
                    </td>
                </tr>
            `).join('');

            panels.forEach((p) => {
                p.tbody.innerHTML = rowsHtml;
                bindInterestRowButtons(p.tbody);
            });
        } catch (err) {
            panels.forEach((p) => {
                p.loading.style.display = 'none';
                if (p.tableContainer) p.tableContainer.style.display = 'block';
                p.tbody.innerHTML = `<tr><td colspan="2" style="padding: 1em; color: #c00;">Failed to load: ${escapeHtml(err.message)}</td></tr>`;
            });
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
        const ok = await AppDialogs.showAppConfirm(
            'Delete interest',
            'Are you sure you want to delete this interest?',
            { danger: true }
        );
        if (!ok) return;
        try {
            const res = await fetch(`/api/interests/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadInterests();
        } catch (err) {
            await AppDialogs.showAppAlert('Failed to delete: ' + err.message);
        }
    }

    async function suggestInterestsFromAI() {
        if (Modals.SubjectConfiguration && typeof Modals.SubjectConfiguration.hasPersistedOwnerContactLinked === 'function'
            && !Modals.SubjectConfiguration.hasPersistedOwnerContactLinked()) {
            return;
        }
        const btn = getEl('interests-suggest-ai-btn-sc');
        const loading = getEl('interests-loading-sc');
        const tableContainer = getEl('interests-table-container-sc');
        if (!btn) return;
        btn.disabled = true;
        if (loading) loading.style.display = 'block';
        if (tableContainer) tableContainer.style.display = 'none';
        try {
            const response = await fetch('/interests/generate-suggested', { method: 'POST' });
            const data = await response.json().catch(() => ({}));
            if (!response.ok) {
                const msg = typeof data.detail === 'string' ? data.detail : `HTTP ${response.status}`;
                throw new Error(msg);
            }
            const skipped = Array.isArray(data.skipped) ? data.skipped : [];
            const added = Array.isArray(data.added) ? data.added : [];
            await loadInterests();
            if (added.length === 0 && skipped.length > 0) {
                await AppDialogs.showAppAlert('Interests', 'No new interests were added; those suggestions were already in your list.');
            }
        } catch (err) {
            await AppDialogs.showAppAlert('Suggest interests', err.message || 'Request failed');
        } finally {
            if (loading) loading.style.display = 'none';
            if (tableContainer) tableContainer.style.display = 'block';
            if (Modals.SubjectConfiguration && typeof Modals.SubjectConfiguration.syncProfileAiButtons === 'function') {
                Modals.SubjectConfiguration.syncProfileAiButtons();
            }
        }
    }

    function init() {
        const createBtnSc = getEl('interests-create-btn-sc');
        const suggestAiBtnSc = getEl('interests-suggest-ai-btn-sc');
        const refreshBtnSc = getEl('interests-refresh-btn-sc');
        const closeBtn = getEl('close-interest-modal');
        const cancelBtn = getEl('interest-modal-cancel');
        const saveBtn = getEl('interest-modal-save');

        if (createBtnSc) createBtnSc.addEventListener('click', openCreateModal);
        if (suggestAiBtnSc) suggestAiBtnSc.addEventListener('click', () => { void suggestInterestsFromAI(); });
        if (refreshBtnSc) refreshBtnSc.addEventListener('click', () => loadInterests());
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
                <tr>
                    <td>${escapeHtml(row.name)}</td>
                    <td>${escapeHtml(row.classification)}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                        <button type="button" class="email-classification-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit classification" title="Edit">
                            <i class="fas fa-edit" aria-hidden="true"></i>
                        </button>
                        <button type="button" class="email-classification-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete classification" title="Delete">
                            <i class="fas fa-trash-alt" aria-hidden="true"></i>
                        </button>
                        </span>
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
        const ok = await AppDialogs.showAppConfirm(
            'Delete classification',
            'Are you sure you want to delete this classification?',
            { danger: true }
        );
        if (!ok) return;
        try {
            const res = await fetch(`/email-classifications/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailClassifications();
        } catch (err) {
            await AppDialogs.showAppAlert('Failed to delete: ' + err.message);
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

        const exportBtn = getEl('email-classifications-export-btn');
        const importBtn = getEl('email-classifications-import-btn');
        const importFile = getEl('email-classifications-import-file');
        if (exportBtn) exportBtn.addEventListener('click', () => { window.location.href = '/email-classifications/export'; });
        if (importBtn) importBtn.addEventListener('click', () => importFile && importFile.click());
        if (importFile) importFile.addEventListener('change', async () => {
            if (!importFile.files || !importFile.files[0]) return;
            const file = importFile.files[0];
            importFile.value = '';
            try {
                const text = await file.text();
                const res = await fetch('/email-classifications/import', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: text,
                    credentials: 'same-origin',
                });
                const data = await res.json();
                if (!res.ok) throw new Error(data.error || res.statusText);
                await AppDialogs.showAppAlert(`Imported ${data.imported} classification(s).`);
                loadEmailClassifications();
            } catch (err) {
                await AppDialogs.showAppAlert('Import failed: ' + err.message);
            }
        });
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
                <tr>
                    <td>${escapeHtml(row.email)}</td>
                    <td>${escapeHtml(row.name)}</td>
                    <td>${escapeHtml(getTypeLabel(row))}</td>
                    <td>
                        <span class="manage-contacts-actions-cell">
                        <button type="button" class="email-exclusion-edit-btn manage-contacts-icon-btn modal-btn modal-btn-secondary" data-id="${row.id}" aria-label="Edit exclusion" title="Edit">
                            <i class="fas fa-edit" aria-hidden="true"></i>
                        </button>
                        <button type="button" class="email-exclusion-delete-btn manage-contacts-icon-btn manage-contacts-icon-btn--delete modal-btn" data-id="${row.id}" aria-label="Delete exclusion" title="Delete">
                            <i class="fas fa-trash-alt" aria-hidden="true"></i>
                        </button>
                        </span>
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
        const ok = await AppDialogs.showAppConfirm('Delete exclusion', msg, { danger: true });
        if (!ok) return;
        try {
            const res = await fetch(`/email-exclusions/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadEmailExclusions();
        } catch (err) {
            await AppDialogs.showAppAlert('Failed to delete: ' + err.message);
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

        const exportBtn = getEl('email-exclusions-export-btn');
        const importBtn = getEl('email-exclusions-import-btn');
        const importFile = getEl('email-exclusions-import-file');
        if (exportBtn) exportBtn.addEventListener('click', () => { window.location.href = '/email-exclusions/export'; });
        if (importBtn) importBtn.addEventListener('click', () => importFile && importFile.click());
        if (importFile) importFile.addEventListener('change', async () => {
            if (!importFile.files || !importFile.files[0]) return;
            const file = importFile.files[0];
            importFile.value = '';
            try {
                const text = await file.text();
                const res = await fetch('/email-exclusions/import', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: text,
                    credentials: 'same-origin',
                });
                const data = await res.json();
                if (!res.ok) throw new Error(data.error || res.statusText);
                await AppDialogs.showAppAlert(`Imported ${data.imported} email exclusion(s).`);
                loadEmailExclusions();
            } catch (err) {
                await AppDialogs.showAppAlert('Import failed: ' + err.message);
            }
        });
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
            if (!currentId) return;
            const okDel = await AppDialogs.showAppConfirm(
                'Delete saved response',
                'Delete this saved response?',
                { danger: true }
            );
            if (!okDel) return;
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
        const tbody = document.getElementById('cfg-table-body');
        if (!tbody) return;
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
        const ok = await AppDialogs.showAppConfirm(
            'Delete configuration key',
            `Delete '${key}' from database? (Will revert to .env value.)`,
            { danger: true }
        );
        if (!ok) return;
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

// Expose for console / future UI if Application Configuration is re-added.
window.AppConfig = Modals.AppConfig;

// --- Per-user LLM / Tavily overrides (Configuration → API Keys tab) ---
Modals.UserLLMSettings = (() => {
    function getEl(id) {
        return document.getElementById(id);
    }

    function setHint(elId, saved, sessionScoped) {
        const el = getEl(elId);
        if (!el) return;
        if (!saved) {
            el.textContent = '';
            return;
        }
        el.textContent = sessionScoped ? '(set for this visit only)' : '(saved on your account)';
    }

    let userLLMStatusClearTimer = null;
    /** Last-loaded user override vs effective display (user → owner → server) for save semantics. */
    let userLLMModelSaveCtx = {
        geminiUser: '',
        geminiEffective: '',
        claudeUser: '',
        claudeEffective: '',
        deepseekUser: '',
        deepseekEffective: '',
        openaiUser: '',
        openaiEffective: '',
    };

    const USER_LLM_MODEL_FIELD_INFO =
        'This field shows your saved model name if you set one; otherwise the archive owner’s default (when you are a visitor), otherwise this server’s default from .env. Save with an empty field to clear your personal override.';

    function setStatus(msg, color) {
        const el = getEl('user-llm-status');
        if (el) {
            el.textContent = msg || '';
            el.style.color = color || '#666';
        }
        if (userLLMStatusClearTimer) {
            clearTimeout(userLLMStatusClearTimer);
            userLLMStatusClearTimer = null;
        }
    }

    /** Success / info messages fade after a few seconds so the line does not stay forever. */
    function setStatusThenFade(msg, color, ms) {
        setStatus(msg, color);
        const el = getEl('user-llm-status');
        if (!el || !msg) return;
        userLLMStatusClearTimer = setTimeout(() => {
            userLLMStatusClearTimer = null;
            if (el.textContent === msg) {
                el.textContent = '';
                el.style.color = '#666';
            }
        }, ms || 10000);
    }

    function applyLLMInputState(el, isOk) {
        if (!el) return;
        el.classList.remove('user-llm-field-input--state-ok', 'user-llm-field-input--state-warn');
        el.classList.add(isOk ? 'user-llm-field-input--state-ok' : 'user-llm-field-input--state-warn');
    }

    /**
     * Empty API key field: saved user key is never returned from the server — show a mask plus “(user supplied)”.
     * If the service is still usable (server .env and/or archive owner key on visitor sessions), prompt to paste an override.
     * Otherwise prompt to add a key. serviceNameForEmptyPrompt is the product string, e.g. “Google Gemini”.
     */
    function apiKeyPlaceholder(userKeySet, serviceConfiguredForUser, serviceNameForEmptyPrompt) {
        if (userKeySet) {
            return 'User supplied key in use. Paste another key to overwrite';
        }
        if (serviceConfiguredForUser) {
            return 'Server key in use. Paste your own key to overwrite';
        }
        return `Enter API Key to access ${serviceNameForEmptyPrompt}`;
    }

    /** Persisted override: empty string clears. If the user only saw an inherited default and did not change it, send "" so no spurious override is stored. */
    function formatModelForSave(raw, userStored, effectiveAtLoad) {
        const r = String(raw || '').trim();
        const u = String(userStored || '').trim();
        const e = String(effectiveAtLoad || '').trim();
        if (!r) return '';
        if (!u && r === e) return '';
        return r;
    }

    function setLLMInfoButton(id, title, bodyText) {
        const el = getEl(id);
        if (!el) return;
        el.setAttribute('data-llm-info-title', title);
        el.setAttribute('data-llm-info-body', bodyText);
    }

    let llmInfoPopoverKeyHandler = null;

    const LLM_INFO_BTN_IDS = [
        'user-llm-info-gemini-key',
        'user-llm-info-gemini-model',
        'user-llm-info-anthropic-key',
        'user-llm-info-claude-model',
        'user-llm-info-deepseek-key',
        'user-llm-info-deepseek-model',
        'user-llm-info-openai-key',
        'user-llm-info-openai-model',
        'user-llm-info-tavily-key',
        'user-llm-info-runpod-key',
        'user-llm-info-elevenlabs-key',
    ];

    let llmInfoPopoverTeardownLayout = null;

    function closeLLMInfoPopover() {
        const wrap = getEl('user-llm-info-popover');
        if (wrap) {
            wrap.setAttribute('hidden', '');
        }
        const card = wrap && wrap.querySelector('.user-llm-info-popover-card');
        if (card) {
            card.style.left = '';
            card.style.top = '';
            card.style.right = '';
            card.style.bottom = '';
        }
        if (llmInfoPopoverTeardownLayout) {
            llmInfoPopoverTeardownLayout();
            llmInfoPopoverTeardownLayout = null;
        }
        if (llmInfoPopoverKeyHandler) {
            document.removeEventListener('keydown', llmInfoPopoverKeyHandler, true);
            llmInfoPopoverKeyHandler = null;
        }
    }

    /** Nearest scrollable ancestor (for repositioning the anchored popover). */
    function llmInfoPopoverScrollParent(el) {
        let p = el && el.parentElement;
        while (p && p !== document.body && p !== document.documentElement) {
            const cs = window.getComputedStyle(p);
            const oy = cs.overflowY;
            const ox = cs.overflowX;
            const scrollableY = (oy === 'auto' || oy === 'scroll') && p.scrollHeight > p.clientHeight;
            const scrollableX = (ox === 'auto' || ox === 'scroll') && p.scrollWidth > p.clientWidth;
            if (scrollableY || scrollableX) return p;
            p = p.parentElement;
        }
        return null;
    }

    function positionLLMInfoPopoverCard(anchorEl) {
        const wrap = getEl('user-llm-info-popover');
        const card = wrap && wrap.querySelector('.user-llm-info-popover-card');
        if (!card || !anchorEl || typeof anchorEl.getBoundingClientRect !== 'function') return;
        const gap = 8;
        const margin = 10;
        const rect = anchorEl.getBoundingClientRect();
        let left = rect.right + gap;
        let top = rect.top;
        const vw = window.innerWidth;
        const vh = window.innerHeight;
        const cw = card.offsetWidth || 280;
        const ch = card.offsetHeight || 120;
        if (left + cw + margin > vw) {
            left = rect.left - gap - cw;
        }
        if (left < margin) left = margin;
        if (left + cw > vw - margin) {
            left = Math.max(margin, vw - margin - cw);
        }
        if (top + ch + margin > vh) {
            top = Math.max(margin, vh - ch - margin);
        }
        if (top < margin) top = margin;
        card.style.left = `${Math.round(left)}px`;
        card.style.top = `${Math.round(top)}px`;
    }

    function attachLLMInfoPopoverLayoutListeners(anchorEl) {
        const reposition = () => positionLLMInfoPopoverCard(anchorEl);
        window.addEventListener('resize', reposition);
        const scrollEl = llmInfoPopoverScrollParent(anchorEl);
        if (scrollEl) {
            scrollEl.addEventListener('scroll', reposition, { passive: true });
        }
        return () => {
            window.removeEventListener('resize', reposition);
            if (scrollEl) scrollEl.removeEventListener('scroll', reposition);
        };
    }

    function openLLMInfoPopover(title, bodyText, anchorEl) {
        const wrap = getEl('user-llm-info-popover');
        const titleEl = getEl('user-llm-info-popover-title');
        const bodyEl = getEl('user-llm-info-popover-body');
        if (!wrap || !titleEl || !bodyEl) return;
        if (llmInfoPopoverTeardownLayout) {
            llmInfoPopoverTeardownLayout();
            llmInfoPopoverTeardownLayout = null;
        }
        if (llmInfoPopoverKeyHandler) {
            document.removeEventListener('keydown', llmInfoPopoverKeyHandler, true);
            llmInfoPopoverKeyHandler = null;
        }
        titleEl.textContent = title || 'Information';
        bodyEl.textContent = bodyText || '';
        wrap.removeAttribute('hidden');
        if (anchorEl) {
            llmInfoPopoverTeardownLayout = attachLLMInfoPopoverLayoutListeners(anchorEl);
            positionLLMInfoPopoverCard(anchorEl);
            requestAnimationFrame(() => {
                positionLLMInfoPopoverCard(anchorEl);
                requestAnimationFrame(() => positionLLMInfoPopoverCard(anchorEl));
            });
        }
        llmInfoPopoverKeyHandler = (e) => {
            if (e.key === 'Escape') {
                e.preventDefault();
                closeLLMInfoPopover();
            }
        };
        document.addEventListener('keydown', llmInfoPopoverKeyHandler, true);
    }

    /** Bind once per button (init may run before tab is first shown; load() runs when API Keys is used). */
    function wireLLMInfoPopover() {
        const wrap = getEl('user-llm-info-popover');
        if (wrap && wrap.dataset.llmPopoverBackdropWired !== '1') {
            wrap.dataset.llmPopoverBackdropWired = '1';
            wrap.addEventListener('click', (ev) => {
                if (ev.target.closest('.user-llm-info-popover-close') || ev.target.classList.contains('user-llm-info-popover-backdrop')) {
                    closeLLMInfoPopover();
                }
            });
        }
        LLM_INFO_BTN_IDS.forEach((id) => {
            const el = getEl(id);
            if (!el || el.dataset.llmInfoClickBound === '1') return;
            el.dataset.llmInfoClickBound = '1';
            el.addEventListener('click', (ev) => {
                ev.preventDefault();
                ev.stopPropagation();
                const title = el.getAttribute('data-llm-info-title') || 'Information';
                const body = el.getAttribute('data-llm-info-body') || '';
                openLLMInfoPopover(title, body, el);
            });
        });
    }

    function refreshProviderAvailabilityAfterKeysChange() {
        if (typeof App !== 'undefined' && App.refreshChatAvailability) {
            void App.refreshChatAvailability();
        }
    }

    async function patchLLM(body) {
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
                else if (j && j.detail) detail = j.detail;
            } catch (_) { /* keep text */ }
            throw new Error(detail || res.statusText);
        }
    }

    /**
     * @param {{ preserveStatus?: boolean }} [options]
     * @returns {Promise<{ sessionScoped: boolean }>}
     */
    async function load(options = {}) {
        const preserveStatus = options.preserveStatus === true;
        const section = getEl('user-llm-settings-section');
        if (!section) return { sessionScoped: false };
        if (!preserveStatus) {
            setStatus('', '#666');
        }
        let sessionScoped = false;
        try {
            const res = await fetch('/auth/me', { credentials: 'same-origin' });
            if (!res.ok) {
                section.style.display = 'none';
                return { sessionScoped: false };
            }
            const data = await res.json();
            section.style.display = 'block';
            let av = {};
            try {
                const avRes = await fetch('/chat/availability', { credentials: 'same-origin' });
                if (avRes.ok) av = await avRes.json();
            } catch (_) { /* leave av partial */ }
            const geminiRoute = !!av.gemini_available;
            const claudeRoute = !!av.claude_available;
            const deepseekRoute = !!av.deepseek_available;
            const openaiRoute = !!av.openai_available;
            const tavilyEnv = !!av.tavily_env_configured;
            const runpodEnv = !!av.runpod_env_configured;
            const runpodEndpointEnv = String(av.runpod_endpoint_id_env || '').trim();
            const runpodWorkersEnv = parseInt(av.runpod_workers_env, 10) || 0;
            const elevenlabsEnv = !!av.elevenlabs_env_configured;
            const serverGeminiModelStr = String(av.server_gemini_model_default || '').trim();
            const serverClaudeModelStr = String(av.server_claude_model_default || '').trim();
            const serverDeepseekModelStr = String(av.server_deepseek_model_default || '').trim();
            const serverOpenaiModelStr = String(av.server_openai_model_default || '').trim();
            const serverGeminiModel = !!av.server_gemini_model_default_set || serverGeminiModelStr !== '';
            const serverClaudeModel = !!av.server_claude_model_default_set || serverClaudeModelStr !== '';
            const serverDeepseekModel = !!av.server_deepseek_model_default_set || serverDeepseekModelStr !== '';
            const serverOpenaiModel = !!av.server_openai_model_default_set || serverOpenaiModelStr !== '';
            const llm = data.llm_settings || {};
            sessionScoped = !!llm.session_scoped;
            const intro = getEl('user-llm-intro');
            if (intro) {
                if (sessionScoped) {
                    intro.textContent = 'Optional overrides for this visit only. Values are stored on your session and cleared when you sign out or the session expires. Leave a field blank to use the archive owner’s saved keys and models where available, then server defaults.';
                } else {
                    intro.textContent = 'API Keys are required to access cloud hosted AIs and hosted tools like web search and speech generation. Digital Museum will work fine without these services, but the responses may not be as sophisticated or as complete. Keys are stored on your account and are not shown again after saving — leave a key field blank to keep your current saved key, or use Clear to remove your override.';
                }
            }
            const subEl = getEl('user-llm-subject-hints');
            if (subEl) {
                if (sessionScoped) {
                    const parts = [];
                    if (llm.subject_gemini_api_key_set) parts.push('Gemini API key');
                    if (llm.subject_anthropic_key_set) parts.push('Anthropic API key');
                    if (llm.subject_deepseek_key_set) parts.push('DeepSeek API key');
                    if (llm.subject_openai_key_set) parts.push('OpenAI API key');
                    if (llm.subject_tavily_key_set) parts.push('Tavily API key');
                    if (llm.subject_runpod_key_set) parts.push('RunPod API key');
                    if (llm.subject_elevenlabs_key_set) parts.push('ElevenLabs API key');
                    if (llm.subject_gemini_model) parts.push('Gemini model "' + llm.subject_gemini_model + '"');
                    if (llm.subject_claude_model) parts.push('Claude model "' + llm.subject_claude_model + '"');
                    if (llm.subject_deepseek_model) parts.push('DeepSeek model "' + llm.subject_deepseek_model + '"');
                    if (llm.subject_openai_model) parts.push('ChatGPT model "' + llm.subject_openai_model + '"');
                    subEl.textContent = parts.length
                        ? ('When you leave a field blank, the owner’s saved settings apply where available, then server defaults. Owner has: ' + parts.join('; ') + '.')
                        : 'The archive owner has not saved personal API keys or models in Settings — blank fields use server defaults only.';
                    subEl.style.display = 'block';
                } else {
                    subEl.textContent = '';
                    subEl.style.display = 'none';
                }
            }
            setHint('user-llm-gemini-key-hint', !!llm.gemini_api_key_set, sessionScoped);
            setHint('user-llm-anthropic-key-hint', !!llm.anthropic_api_key_set, sessionScoped);
            setHint('user-llm-deepseek-key-hint', !!llm.deepseek_api_key_set, sessionScoped);
            setHint('user-llm-openai-key-hint', !!llm.openai_api_key_set, sessionScoped);
            setHint('user-llm-tavily-key-hint', !!llm.tavily_api_key_set, sessionScoped);
            setHint('user-llm-runpod-key-hint', !!llm.runpod_api_key_set, sessionScoped);
            setHint('user-llm-elevenlabs-key-hint', !!llm.elevenlabs_api_key_set, sessionScoped);
            const gm = getEl('user-llm-gemini-model');
            const cm = getEl('user-llm-claude-model');
            const dm = getEl('user-llm-deepseek-model');
            const om = getEl('user-llm-openai-model');
            const geminiStored = String(llm.gemini_model || '').trim();
            const claudeStored = String(llm.claude_model || '').trim();
            const deepseekStored = String(llm.deepseek_model || '').trim();
            const openaiStored = String(llm.openai_model || '').trim();
            const ownerGeminiModel = sessionScoped ? String(llm.subject_gemini_model || '').trim() : '';
            const ownerClaudeModel = sessionScoped ? String(llm.subject_claude_model || '').trim() : '';
            const ownerDeepseekModel = sessionScoped ? String(llm.subject_deepseek_model || '').trim() : '';
            const ownerOpenaiModel = sessionScoped ? String(llm.subject_openai_model || '').trim() : '';
            const ownerGeminiKey = !!(sessionScoped && llm.subject_gemini_api_key_set);
            const ownerAnthropicKey = !!(sessionScoped && llm.subject_anthropic_key_set);
            const ownerDeepseekKey = !!(sessionScoped && llm.subject_deepseek_key_set);
            const ownerOpenaiKey = !!(sessionScoped && llm.subject_openai_key_set);
            const ownerTavilyKey = !!(sessionScoped && llm.subject_tavily_key_set);
            const ownerRunpodKey = !!(sessionScoped && llm.subject_runpod_key_set);
            const ownerElevenlabsKey = !!(sessionScoped && llm.subject_elevenlabs_key_set);
            const geminiEffective = geminiStored || ownerGeminiModel || serverGeminiModelStr;
            const claudeEffective = claudeStored || ownerClaudeModel || serverClaudeModelStr;
            const deepseekEffective = deepseekStored || ownerDeepseekModel || serverDeepseekModelStr;
            const openaiEffective = openaiStored || ownerOpenaiModel || serverOpenaiModelStr;
            const gemModelPh = geminiEffective ? '' : 'Enter model name';
            const claudeModelPh = claudeEffective ? '' : 'Enter model name';
            const deepseekModelPh = deepseekEffective ? '' : 'Enter model name';
            const openaiModelPh = openaiEffective ? '' : 'Enter model name';
            userLLMModelSaveCtx = {
                geminiUser: geminiStored,
                geminiEffective: geminiEffective,
                claudeUser: claudeStored,
                claudeEffective: claudeEffective,
                deepseekUser: deepseekStored,
                deepseekEffective: deepseekEffective,
                openaiUser: openaiStored,
                openaiEffective: openaiEffective,
            };
            const gemKeyOk = !!(llm.gemini_api_key_set || ownerGeminiKey || geminiRoute);
            const claudeKeyOk = !!(llm.anthropic_api_key_set || ownerAnthropicKey || claudeRoute);
            const deepKeyOk = !!(llm.deepseek_api_key_set || ownerDeepseekKey || deepseekRoute);
            const openaiKeyOk = !!(llm.openai_api_key_set || ownerOpenaiKey || openaiRoute);
            const tavKeyOk = !!(llm.tavily_api_key_set || ownerTavilyKey || tavilyEnv);
            const runpodKeyOk = !!(llm.runpod_api_key_set || ownerRunpodKey || runpodEnv);
            const elevenlabsKeyOk = !!(llm.elevenlabs_api_key_set || ownerElevenlabsKey || elevenlabsEnv);
            const gk = getEl('user-llm-gemini-key');
            const ak = getEl('user-llm-anthropic-key');
            const dk = getEl('user-llm-deepseek-key');
            const ok = getEl('user-llm-openai-key');
            const tk = getEl('user-llm-tavily-key');
            const rk = getEl('user-llm-runpod-key');
            const el11 = getEl('user-llm-elevenlabs-key');
            const gemKeyPh = apiKeyPlaceholder(!!llm.gemini_api_key_set, gemKeyOk, 'Google Gemini');
            const anthropicKeyPh = apiKeyPlaceholder(!!llm.anthropic_api_key_set, claudeKeyOk, 'Anthropic Claude');
            const deepseekKeyPh = apiKeyPlaceholder(!!llm.deepseek_api_key_set, deepKeyOk, 'DeepSeek');
            const openaiKeyPh = apiKeyPlaceholder(!!llm.openai_api_key_set, openaiKeyOk, 'ChatGPT');
            const tavilyKeyPh = apiKeyPlaceholder(!!llm.tavily_api_key_set, tavKeyOk, 'Tavily (web search)');
            const runpodKeyPh = apiKeyPlaceholder(!!llm.runpod_api_key_set, runpodKeyOk, 'RunPod (image AI classification)');
            const elevenlabsKeyPh = apiKeyPlaceholder(!!llm.elevenlabs_api_key_set, elevenlabsKeyOk, 'ElevenLabs (speech / voice)');
            if (gm) {
                gm.value = geminiEffective;
                gm.placeholder = gemModelPh;
            }
            if (cm) {
                cm.value = claudeEffective;
                cm.placeholder = claudeModelPh;
            }
            if (dm) {
                dm.value = deepseekEffective;
                dm.placeholder = deepseekModelPh;
            }
            if (om) {
                om.value = openaiEffective;
                om.placeholder = openaiModelPh;
            }
            if (gk) {
                gk.value = '';
                gk.placeholder = gemKeyPh;
            }
            if (ak) {
                ak.value = '';
                ak.placeholder = anthropicKeyPh;
            }
            if (dk) {
                dk.value = '';
                dk.placeholder = deepseekKeyPh;
            }
            if (ok) {
                ok.value = '';
                ok.placeholder = openaiKeyPh;
            }
            if (tk) {
                tk.value = '';
                tk.placeholder = tavilyKeyPh;
            }
            if (rk) {
                rk.value = '';
                rk.placeholder = runpodKeyPh;
            }
            const rpEpEl = getEl('user-llm-runpod-endpoint');
            const rpWkEl = getEl('user-llm-runpod-workers');
            const storedEndpoint = String(llm.runpod_endpoint_id || (sessionScoped ? llm.subject_runpod_endpoint_id : '') || '').trim();
            const storedWorkers = parseInt(llm.runpod_workers || (sessionScoped ? llm.subject_runpod_workers : 0), 10) || 0;
            if (rpEpEl) {
                rpEpEl.value = storedEndpoint;
                rpEpEl.placeholder = runpodEndpointEnv ? `Server: ${runpodEndpointEnv}` : 'Enter endpoint ID';
            }
            if (rpWkEl) {
                rpWkEl.value = String(storedWorkers > 0 ? storedWorkers : 0);
                if (storedWorkers === 0) {
                    rpWkEl.options[0].text = runpodWorkersEnv > 0
                        ? `Server default (${runpodWorkersEnv})`
                        : 'Server default';
                }
            }
            if (el11) {
                el11.value = '';
                el11.placeholder = elevenlabsKeyPh;
            }
            setLLMInfoButton('user-llm-info-gemini-key', 'Gemini API key', gemKeyPh);
            setLLMInfoButton('user-llm-info-gemini-model', 'Gemini model name', USER_LLM_MODEL_FIELD_INFO);
            setLLMInfoButton('user-llm-info-anthropic-key', 'Anthropic Claude API key', anthropicKeyPh);
            setLLMInfoButton('user-llm-info-claude-model', 'Claude model name', USER_LLM_MODEL_FIELD_INFO);
            setLLMInfoButton('user-llm-info-deepseek-key', 'DeepSeek API key', deepseekKeyPh);
            setLLMInfoButton('user-llm-info-deepseek-model', 'DeepSeek model name', USER_LLM_MODEL_FIELD_INFO);
            setLLMInfoButton('user-llm-info-openai-key', 'OpenAI API key', openaiKeyPh);
            setLLMInfoButton('user-llm-info-openai-model', 'ChatGPT model name', USER_LLM_MODEL_FIELD_INFO);
            setLLMInfoButton('user-llm-info-tavily-key', 'Tavily API key (web search)', tavilyKeyPh);
            setLLMInfoButton('user-llm-info-runpod-key', 'RunPod API key (image AI classification)', runpodKeyPh);
            setLLMInfoButton('user-llm-info-elevenlabs-key', 'ElevenLabs API key (speech / voice)', elevenlabsKeyPh);
            wireLLMInfoPopover();
            const gemModelOk = !!(geminiStored || ownerGeminiModel || serverGeminiModelStr || serverGeminiModel);
            applyLLMInputState(gk, gemKeyOk);
            applyLLMInputState(gm, gemModelOk);
            const claudeModelOk = !!(claudeStored || ownerClaudeModel || serverClaudeModelStr || serverClaudeModel);
            applyLLMInputState(ak, claudeKeyOk);
            applyLLMInputState(cm, claudeModelOk);
            const deepModelOk = !!(deepseekStored || ownerDeepseekModel || serverDeepseekModelStr || serverDeepseekModel);
            applyLLMInputState(dk, deepKeyOk);
            applyLLMInputState(dm, deepModelOk);
            const openaiModelOk = !!(openaiStored || ownerOpenaiModel || serverOpenaiModelStr || serverOpenaiModel);
            applyLLMInputState(ok, openaiKeyOk);
            applyLLMInputState(om, openaiModelOk);
            applyLLMInputState(tk, tavKeyOk);
            applyLLMInputState(rk, runpodKeyOk);
            applyLLMInputState(el11, elevenlabsKeyOk);
            const saveBtn = getEl('user-llm-save-btn');
            if (saveBtn) saveBtn.textContent = sessionScoped ? 'Save for this session' : 'Save AI API settings';
        } catch (e) {
            section.style.display = 'none';
            return { sessionScoped: false };
        }
        return { sessionScoped };
    }

    async function save() {
        setStatus('Saving…', '#666');
        const body = {};
        const gk = (getEl('user-llm-gemini-key') && getEl('user-llm-gemini-key').value) || '';
        const ak = (getEl('user-llm-anthropic-key') && getEl('user-llm-anthropic-key').value) || '';
        const dk = (getEl('user-llm-deepseek-key') && getEl('user-llm-deepseek-key').value) || '';
        const ok = (getEl('user-llm-openai-key') && getEl('user-llm-openai-key').value) || '';
        const tk = (getEl('user-llm-tavily-key') && getEl('user-llm-tavily-key').value) || '';
        const rpK = (getEl('user-llm-runpod-key') && getEl('user-llm-runpod-key').value) || '';
        const elK = (getEl('user-llm-elevenlabs-key') && getEl('user-llm-elevenlabs-key').value) || '';
        if (gk.trim()) body.gemini_api_key = gk.trim();
        if (ak.trim()) body.anthropic_api_key = ak.trim();
        if (dk.trim()) body.deepseek_api_key = dk.trim();
        if (ok.trim()) body.openai_api_key = ok.trim();
        if (tk.trim()) body.tavily_api_key = tk.trim();
        if (rpK.trim()) body.runpod_api_key = rpK.trim();
        if (elK.trim()) body.elevenlabs_api_key = elK.trim();
        const rpEpVal = ((getEl('user-llm-runpod-endpoint') && getEl('user-llm-runpod-endpoint').value) || '').trim();
        body.runpod_endpoint_id = rpEpVal;
        const rpWkVal = parseInt((getEl('user-llm-runpod-workers') && getEl('user-llm-runpod-workers').value) || '0', 10);
        body.runpod_workers = isNaN(rpWkVal) ? 0 : rpWkVal;
        const gmEl = getEl('user-llm-gemini-model');
        const cmEl = getEl('user-llm-claude-model');
        const dmEl = getEl('user-llm-deepseek-model');
        const omEl = getEl('user-llm-openai-model');
        body.gemini_model = formatModelForSave(
            gmEl && gmEl.value,
            userLLMModelSaveCtx.geminiUser,
            userLLMModelSaveCtx.geminiEffective
        );
        body.claude_model = formatModelForSave(
            cmEl && cmEl.value,
            userLLMModelSaveCtx.claudeUser,
            userLLMModelSaveCtx.claudeEffective
        );
        body.deepseek_model = formatModelForSave(
            dmEl && dmEl.value,
            userLLMModelSaveCtx.deepseekUser,
            userLLMModelSaveCtx.deepseekEffective
        );
        body.openai_model = formatModelForSave(
            omEl && omEl.value,
            userLLMModelSaveCtx.openaiUser,
            userLLMModelSaveCtx.openaiEffective
        );
        try {
            await patchLLM(body);
            const { sessionScoped } = await load({ preserveStatus: true });
            const msg = sessionScoped
                ? 'Saved. Session overrides apply until you sign out or this visit ends. API keys are not shown again after saving.'
                : 'Saved. Your API keys and model names are stored on your account. Keys are not shown again after saving for security.';
            setStatusThenFade(msg, '#15803d', 12000);
            refreshProviderAvailabilityAfterKeysChange();
        } catch (e) {
            setStatus(e.message || 'Save failed', '#c00');
        }
    }

    async function clearLLMPatch(body) {
        setStatus('Clearing…', '#666');
        try {
            await patchLLM(body);
            await load({ preserveStatus: true });
            setStatusThenFade('Cleared. Those overrides were removed; empty fields use the owner’s saved values or server defaults where applicable.', '#15803d', 12000);
        } catch (e) {
            setStatus(e.message || 'Clear failed', '#c00');
        }
    }

    function init() {
        wireLLMInfoPopover();
        const saveBtn = getEl('user-llm-save-btn');
        if (saveBtn) saveBtn.addEventListener('click', () => void save());
        const cg = getEl('user-llm-clear-gemini');
        if (cg) cg.addEventListener('click', () => void clearLLMPatch({ gemini_api_key: '', gemini_model: '' }));
        const ca = getEl('user-llm-clear-anthropic');
        if (ca) ca.addEventListener('click', () => void clearLLMPatch({ anthropic_api_key: '', claude_model: '' }));
        const cd = getEl('user-llm-clear-deepseek');
        if (cd) cd.addEventListener('click', () => void clearLLMPatch({ deepseek_api_key: '', deepseek_model: '' }));
        const co = getEl('user-llm-clear-openai');
        if (co) co.addEventListener('click', () => void clearLLMPatch({ openai_api_key: '', openai_model: '' }));
        const ct = getEl('user-llm-clear-tavily');
        if (ct) ct.addEventListener('click', () => void clearLLMPatch({ tavily_api_key: '' }));
        const crp = getEl('user-llm-clear-runpod');
        if (crp) crp.addEventListener('click', () => void clearLLMPatch({ runpod_api_key: '', runpod_endpoint_id: '', runpod_workers: 0 }));
        const cel = getEl('user-llm-clear-elevenlabs');
        if (cel) cel.addEventListener('click', () => void clearLLMPatch({ elevenlabs_api_key: '' }));
    }

    return { load, init, closeInfoPopover: closeLLMInfoPopover };
})();

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
            await AppDialogs.showAppAlert('Failed to load voice: ' + err.message);
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
        const ok = await AppDialogs.showAppConfirm(
            'Delete custom voice',
            'Delete this custom voice? This cannot be undone.',
            { danger: true }
        );
        if (!ok) return;
        try {
            const res = await fetch(`/api/voices/${id}`, { method: 'DELETE' });
            if (!res.ok) throw new Error(res.statusText);
            loadCustomVoices();
            if (typeof VoiceSelector !== 'undefined' && VoiceSelector.loadVoices) {
                VoiceSelector.loadVoices();
            }
        } catch (err) {
            await AppDialogs.showAppAlert('Failed to delete: ' + err.message);
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

