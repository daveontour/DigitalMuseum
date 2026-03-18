let currentAttachmentId = null;
let currentOffset = 0;
let currentOrder = 'random';
const API_BASE = window.location.origin;

async function loadAttachment(maxAttempts = 50) {
    try {
        document.getElementById('loading').style.display = 'block';
        document.getElementById('attachment-view').style.display = 'none';
        document.getElementById('no-attachments').style.display = 'none';
        document.getElementById('error').style.display = 'none';
        
        const showPdf = document.getElementById('show-pdf-checkbox').checked;
        const order = document.getElementById('order-select').value;
        const minSize = parseInt(document.getElementById('min-size-select').value) || 0;
        currentOrder = order;
        
        let attempts = 0;
        let offset = currentOffset;
        
        while (attempts < maxAttempts) {
            attempts++;
            
            let response;
            if (order === 'random') {
                response = await fetch(`${API_BASE}/attachments/random`);
                offset = 0; // Reset offset for random
            } else if (order === 'id') {
                response = await fetch(`${API_BASE}/attachments/by-id?offset=${offset}`);
            } else if (order === 'size-asc') {
                response = await fetch(`${API_BASE}/attachments/by-size?order=asc&offset=${offset}`);
            } else if (order === 'size-desc') {
                response = await fetch(`${API_BASE}/attachments/by-size?order=desc&offset=${offset}`);
            }
            
            if (response.status === 404 || !response.ok) {
                // If not random and we've reached the end, try wrapping around
                if (order !== 'random' && offset > 0) {
                    offset = 0;
                    continue;
                }
                // If offset is very large (from previous wrap), reset to 0
                if (order !== 'random' && offset > 1000000) {
                    offset = 0;
                    continue;
                }
                document.getElementById('loading').style.display = 'none';
                document.getElementById('no-attachments').style.display = 'block';
                return;
            }
            
            const data = await response.json();
            
            if (!data || !data.attachment_id) {
                // If not random and we've reached the end, try wrapping around
                if (order !== 'random' && offset > 0) {
                    offset = 0;
                    continue;
                }
                // If offset is very large (from previous wrap), reset to 0
                if (order !== 'random' && offset > 1000000) {
                    offset = 0;
                    continue;
                }
                document.getElementById('loading').style.display = 'none';
                document.getElementById('no-attachments').style.display = 'block';
                return;
            }
            
            // Check file size filter first
            const fileSize = data.size || 0;
            if (minSize > 0 && fileSize < minSize) {
                // Skip this attachment if it's below the minimum size
                if (order !== 'random') {
                    offset++;
                }
                continue;
            }
            
            // Check if it's a PDF, MS Word, or octet stream and if they should be shown
            const contentType = data.content_type ? data.content_type.toLowerCase() : '';
            const isPdf = contentType === 'application/pdf';
            const isMsWord = contentType === 'application/msword' || 
                             contentType === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' ||
                             contentType === 'application/vnd.ms-word.document.macroenabled.12';
            const isOctetStream = contentType === 'application/octet-stream';
            
            if (!showPdf && (isPdf || isMsWord || isOctetStream)) {
                // Skip this attachment and try again
                if (order !== 'random') {
                    offset++;
                }
                continue;
            }
            
            currentOffset = offset;
            
            currentAttachmentId = data.attachment_id;
            
            // Display attachment info
            document.getElementById('attachment-id').textContent = data.attachment_id || 'Unknown';
            document.getElementById('filename').textContent = data.filename || 'Unknown';
            document.getElementById('content-type').textContent = data.content_type || 'Unknown';
            document.getElementById('size').textContent = data.size ? formatBytes(data.size) : 'Unknown';
            
            // Display email metadata
            document.getElementById('email-subject').textContent = data.email_subject || 'No subject';
            document.getElementById('email-from').textContent = data.email_from || 'Unknown';
            document.getElementById('email-date').textContent = data.email_date ? new Date(data.email_date).toLocaleString() : 'Unknown';
            document.getElementById('email-folder').textContent = data.email_folder || 'Unknown';
            
            // Enable/disable Delete button based on PDF/MS Word/Octet Stream status
            const deleteBtn = document.getElementById('delete-btn');
            if (isPdf || isMsWord || isOctetStream) {
                deleteBtn.disabled = true;
                deleteBtn.title = 'Delete is disabled for PDF, MS Word, and Octet Stream attachments';
            } else {
                deleteBtn.disabled = false;
                deleteBtn.title = '';
            }
            
            // Load attachment preview (full version, not thumbnail)
            const previewContainer = document.getElementById('preview-container');
            previewContainer.innerHTML = '';
            
            // Check if it's an image
            const isImage = data.content_type && data.content_type.startsWith('image/');
            
            if (isImage) {
                const img = document.createElement('img');
                img.src = `${API_BASE}/attachments/${data.attachment_id}`;
                img.alt = data.filename || 'Attachment preview';
                img.onerror = function() {
                    previewContainer.innerHTML = '<p style="color: #666;">Image not available</p>';
                };
                previewContainer.appendChild(img);
            } else if (isPdf || isMsWord || isOctetStream) {
                // For PDFs, MS Word, and Octet Streams, show thumbnail
                const img = document.createElement('img');
                img.src = `${API_BASE}/attachments/${data.attachment_id}?preview=true`;
                img.alt = data.filename || 'File preview';
                img.onerror = function() {
                    const fileType = isPdf ? 'PDF' : (isMsWord ? 'MS Word' : 'File');
                    previewContainer.innerHTML = `<p style="color: #666;">${fileType}: ${data.filename || 'Unknown'}</p><p style="color: #999; font-size: 0.9em;">Preview not available</p>`;
                };
                previewContainer.appendChild(img);
            } else {
                // For other non-image files, show thumbnail if available, otherwise show file info
                const img = document.createElement('img');
                img.src = `${API_BASE}/attachments/${data.attachment_id}?preview=true`;
                img.alt = data.filename || 'Attachment preview';
                img.onerror = function() {
                    previewContainer.innerHTML = `<p style="color: #666;">File: ${data.filename || 'Unknown'}</p><p style="color: #999; font-size: 0.9em;">${data.content_type || 'Unknown type'}</p>`;
                };
                previewContainer.appendChild(img);
            }
            
            document.getElementById('loading').style.display = 'none';
            document.getElementById('attachment-view').style.display = 'block';
            
            // Update Previous button state
            const prevBtn = document.getElementById('prev-btn');
            if (currentOrder === 'random') {
                prevBtn.disabled = true;
                prevBtn.title = 'Previous is not available in random order';
            } else {
                prevBtn.disabled = false;
                prevBtn.title = '';
            }
            
            return;
        }
        
        // If we've exhausted attempts, show no attachments message
        document.getElementById('loading').style.display = 'none';
        document.getElementById('no-attachments').style.display = 'block';
        document.getElementById('no-attachments').textContent = 'No attachments found matching your filter criteria.';
        
    } catch (error) {
        console.error('Error loading attachment:', error);
        document.getElementById('loading').style.display = 'none';
        document.getElementById('error').style.display = 'block';
        document.getElementById('error').textContent = `Error: ${error.message}`;
    }
}

async function nextAttachment() {
    if (!currentAttachmentId) return;
    
    // Disable all buttons during loading
    const buttons = ['prev-btn', 'next-btn', 'keep-btn', 'delete-btn'];
    buttons.forEach(id => document.getElementById(id).disabled = true);
    
    // Increment offset for non-random orders
    if (currentOrder !== 'random') {
        currentOffset++;
    }
    
    // Move to next attachment
    await loadAttachment();
    
    // Re-enable buttons (loadAttachment will handle Previous button state)
    buttons.forEach(id => {
        const btn = document.getElementById(id);
        if (id !== 'prev-btn') {
            btn.disabled = false;
        }
    });
    // Delete button state is set by loadAttachment based on PDF status
}

async function previousAttachment() {
    if (!currentAttachmentId) return;
    
    // Disable Previous button for random order
    if (currentOrder === 'random') {
        return;
    }
    
    // Disable all buttons during loading
    const buttons = ['prev-btn', 'next-btn', 'keep-btn', 'delete-btn'];
    buttons.forEach(id => document.getElementById(id).disabled = true);
    
    // Decrement offset for ordered views
    if (currentOffset > 0) {
        currentOffset--;
    } else {
        // Wrap around to end - set to a large number and let loadAttachment handle it
        currentOffset = 999999;
    }
    
    // Move to previous attachment
    await loadAttachment();
    
    // Re-enable buttons (loadAttachment will handle Previous button state)
    buttons.forEach(id => {
        const btn = document.getElementById(id);
        if (id !== 'prev-btn') {
            btn.disabled = false;
        }
    });
    // Delete button state is set by loadAttachment based on PDF status
}

async function keepAttachment() {
    if (!currentAttachmentId) return;
    
    // Use nextAttachment logic
    await nextAttachment();
}

async function deleteAttachment() {
    if (!currentAttachmentId) return;
    
    const confirmDelete = document.getElementById('confirm-delete-checkbox').checked;
    if (confirmDelete) {
        if (!confirm('Are you sure you want to delete this attachment?')) {
            return;
        }
    }
    
    // Disable all buttons during deletion
    const buttons = ['prev-btn', 'next-btn', 'keep-btn', 'delete-btn'];
    buttons.forEach(id => document.getElementById(id).disabled = true);
    
    try {
        const response = await fetch(`${API_BASE}/attachments/${currentAttachmentId}`, {
            method: 'DELETE'
        });
        
        if (!response.ok) {
            throw new Error('Failed to delete attachment');
        }
        
        // Increment offset for non-random orders
        if (currentOrder !== 'random') {
            currentOffset++;
        }
        
        // Move to next attachment
        await loadAttachment();
        
        // Re-enable buttons (loadAttachment will handle Previous button state)
        const buttons = ['prev-btn', 'next-btn', 'keep-btn', 'delete-btn'];
        buttons.forEach(id => {
            const btn = document.getElementById(id);
            if (id !== 'prev-btn') {
                btn.disabled = false;
            }
        });
        
    } catch (error) {
        console.error('Error deleting attachment:', error);
        document.getElementById('error').style.display = 'block';
        document.getElementById('error').textContent = `Error deleting attachment: ${error.message}`;
        
        // Re-enable buttons on error
        const buttons = ['prev-btn', 'next-btn', 'keep-btn', 'delete-btn'];
        buttons.forEach(id => {
            const btn = document.getElementById(id);
            if (id !== 'prev-btn' || currentOrder !== 'random') {
                btn.disabled = false;
            }
        });
    }
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

// Load initial attachment
loadAttachment();

// Reload when PDF checkbox changes
document.getElementById('show-pdf-checkbox').addEventListener('change', function() {
    if (currentOrder === 'random') {
        currentOffset = 0;
    }
    loadAttachment();
});

// Reload when order changes
document.getElementById('order-select').addEventListener('change', function() {
    currentOffset = 0;
    loadAttachment();
});

// Reload when min size changes
document.getElementById('min-size-select').addEventListener('change', function() {
    currentOffset = 0;
    loadAttachment();
});
