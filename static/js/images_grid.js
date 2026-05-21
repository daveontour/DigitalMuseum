let currentPage = 1;
const pageSize = 50;
let totalPages = 1;
let currentSortOrder = 'size';
let currentSortDirection = 'asc';
const API_BASE = window.location.origin;
const selectedImages = new Set();

if (typeof AppDialogs !== 'undefined' && AppDialogs.init) {
    AppDialogs.init();
}

async function loadImages(page) {
    try {
        document.getElementById('loading').style.display = 'block';
        document.getElementById('images-grid').style.display = 'none';
        document.getElementById('error').style.display = 'none';
        
        // Get values from top controls (they should be synced)
        const sortOrder = document.getElementById('sort-order-select').value;
        const sortDirection = document.getElementById('sort-direction-select').value;
        const showAllTypes = document.getElementById('show-all-types-checkbox').checked;
        currentSortOrder = sortOrder;
        currentSortDirection = sortDirection;
        
        // Sync bottom controls to match top
        syncTopControls();
        
        const allTypesParam = showAllTypes ? '&all_types=true' : '';
        const response = await fetch(`${API_BASE}/attachments/images?page=${page}&page_size=${pageSize}&order=${sortOrder}&direction=${sortDirection}${allTypesParam}`);
        
        if (!response.ok) {
            throw new Error(`Failed to load images: ${response.statusText}`);
        }
        
        const data = await response.json();
        
        currentPage = data.page;
        totalPages = data.total_pages;
        
        // Update pagination info (both top and bottom)
        document.getElementById('page-info').textContent = `Page ${currentPage} of ${totalPages}`;
        document.getElementById('page-info-bottom').textContent = `Page ${currentPage} of ${totalPages}`;
        document.getElementById('prev-page-btn').disabled = currentPage <= 1;
        document.getElementById('prev-page-btn-bottom').disabled = currentPage <= 1;
        document.getElementById('next-page-btn').disabled = currentPage >= totalPages;
        document.getElementById('next-page-btn-bottom').disabled = currentPage >= totalPages;
        
        // Clear previous selections for this page
        selectedImages.clear();
        updateDeleteButton();
        
        // Render images
        const grid = document.getElementById('images-grid');
        grid.innerHTML = '';
        
        if (data.images.length === 0) {
            grid.innerHTML = '<div style="grid-column: 1 / -1; text-align: center; padding: 40px; color: #666;">No images found</div>';
        } else {
            data.images.forEach(image => {
                const item = document.createElement('div');
                item.className = 'image-item';
                item.dataset.id = image.attachment_id;
                
                const checkbox = document.createElement('div');
                checkbox.className = 'image-checkbox';
                checkbox.innerHTML = `
                    <input type="checkbox" id="img-${image.attachment_id}" onchange="toggleSelection(${image.attachment_id})">
                    <label for="img-${image.attachment_id}">Select</label>
                `;
                
                const img = document.createElement('img');
                img.className = 'image-preview';
                img.src = `${API_BASE}/attachments/${image.attachment_id}?preview=true`;
                img.alt = image.filename || 'Image';
                img.onerror = function() {
                    this.src = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="200" height="150"%3E%3Crect fill="%23ddd" width="200" height="150"/%3E%3Ctext x="50%25" y="50%25" text-anchor="middle" dy=".3em" fill="%23999"%3ENo Preview%3C/text%3E%3C/svg%3E';
                };
                
                const info = document.createElement('div');
                info.className = 'image-info';
                const sizeStr = image.size ? formatBytes(image.size) : 'Unknown';
                const contentType = image.content_type || 'Unknown';
                info.innerHTML = `
                    <div class="image-id">ID: ${image.attachment_id}</div>
                    <div class="image-size">Size: ${sizeStr}</div>
                    <div style="font-size: 0.8em; color: #888; margin-top: 4px;">${contentType}</div>
                `;
                
                const viewFullBtn = document.createElement('button');
                viewFullBtn.className = 'view-full-btn';
                viewFullBtn.textContent = 'View Full Size';
                viewFullBtn.onclick = function(e) {
                    e.stopPropagation();
                    showFullImage(image.attachment_id);
                };
                
                const viewMetadataBtn = document.createElement('button');
                viewMetadataBtn.className = 'view-metadata-btn';
                viewMetadataBtn.textContent = 'View Metadata';
                viewMetadataBtn.onclick = function(e) {
                    e.stopPropagation();
                    showMetadata(image);
                };
                
                const viewEmailBtn = document.createElement('button');
                viewEmailBtn.className = 'view-email-btn';
                viewEmailBtn.textContent = 'View Email';
                viewEmailBtn.onclick = function(e) {
                    e.stopPropagation();
                    showEmail(image.email_id);
                };
                
                item.appendChild(checkbox);
                item.appendChild(img);
                item.appendChild(info);
                item.appendChild(viewFullBtn);
                item.appendChild(viewMetadataBtn);
                item.appendChild(viewEmailBtn);
                
                item.onclick = function(e) {
                    if (e.target.tagName !== 'INPUT' && e.target.tagName !== 'LABEL' && e.target.tagName !== 'BUTTON') {
                        const checkbox = this.querySelector('input[type="checkbox"]');
                        checkbox.checked = !checkbox.checked;
                        toggleSelection(image.attachment_id);
                    }
                };
                
                grid.appendChild(item);
            });
        }
        
        document.getElementById('loading').style.display = 'none';
        document.getElementById('images-grid').style.display = 'grid';
        
    } catch (error) {
        console.error('Error loading images:', error);
        document.getElementById('loading').style.display = 'none';
        document.getElementById('error').style.display = 'block';
        document.getElementById('error').textContent = `Error: ${error.message}`;
    }
}

function toggleSelection(imageId) {
    const checkbox = document.getElementById(`img-${imageId}`);
    const item = document.querySelector(`[data-id="${imageId}"]`);
    
    if (checkbox.checked) {
        selectedImages.add(imageId);
        item.classList.add('selected');
    } else {
        selectedImages.delete(imageId);
        item.classList.remove('selected');
    }
    
    updateDeleteButton();
}

function selectAll() {
    const items = document.querySelectorAll('#images-grid .image-item');
    items.forEach(item => {
        const id = parseInt(item.dataset.id, 10);
        const checkbox = item.querySelector('input[type="checkbox"]');
        if (checkbox) {
            checkbox.checked = true;
            selectedImages.add(id);
            item.classList.add('selected');
        }
    });
    updateDeleteButton();
}

function updateDeleteButton() {
    const btn = document.getElementById('delete-selected-btn');
    const btnBottom = document.getElementById('delete-selected-btn-bottom');
    const btnText = selectedImages.size > 0 ? `Delete Selected (${selectedImages.size})` : 'Delete Selected';
    btn.disabled = selectedImages.size === 0;
    btn.textContent = btnText;
    btnBottom.disabled = selectedImages.size === 0;
    btnBottom.textContent = btnText;
}

async function deleteSelected() {
    if (selectedImages.size === 0) return;
    
    const ok = await AppDialogs.showAppConfirm(
        'Delete images',
        `Are you sure you want to delete ${selectedImages.size} image(s)?`,
        { danger: true }
    );
    if (!ok) {
        return;
    }
    
    const deleteBtn = document.getElementById('delete-selected-btn');
    deleteBtn.disabled = true;
    deleteBtn.textContent = 'Deleting...';
    
    const idsToDelete = Array.from(selectedImages);
    let successCount = 0;
    let failCount = 0;
    
    for (const id of idsToDelete) {
        try {
            const response = await fetch(`${API_BASE}/attachments/${id}`, {
                method: 'DELETE'
            });
            
            if (response.ok) {
                successCount++;
            } else {
                failCount++;
            }
        } catch (error) {
            console.error(`Error deleting image ${id}:`, error);
            failCount++;
        }
    }
    
    // Reload current page
    await loadImages(currentPage);
    
    if (failCount > 0) {
        await AppDialogs.showAppAlert('Delete result', `Deleted ${successCount} image(s). ${failCount} failed.`);
    }
}

function previousPage() {
    if (currentPage > 1) {
        loadImages(currentPage - 1);
    }
}

function nextPage() {
    if (currentPage < totalPages) {
        loadImages(currentPage + 1);
    }
}

function syncTopControls() {
    // Sync bottom controls to top controls
    document.getElementById('show-all-types-checkbox-bottom').checked = document.getElementById('show-all-types-checkbox').checked;
    document.getElementById('sort-order-select-bottom').value = document.getElementById('sort-order-select').value;
    document.getElementById('sort-direction-select-bottom').value = document.getElementById('sort-direction-select').value;
}

function syncBottomControls() {
    // Sync top controls to bottom controls
    document.getElementById('show-all-types-checkbox').checked = document.getElementById('show-all-types-checkbox-bottom').checked;
    document.getElementById('sort-order-select').value = document.getElementById('sort-order-select-bottom').value;
    document.getElementById('sort-direction-select').value = document.getElementById('sort-direction-select-bottom').value;
}

function changeSortOrder() {
    currentPage = 1;
    loadImages(1);
}

function showMetadata(imageData) {
    const modal = document.getElementById('metadata-modal');
    const content = document.getElementById('metadata-content');
    
    const formatDate = (dateStr) => {
        if (!dateStr) return 'Unknown';
        try {
            return new Date(dateStr).toLocaleString();
        } catch {
            return dateStr;
        }
    };
    
    const formatBytesHelper = (bytes) => {
        if (!bytes) return 'Unknown';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    };
    
    content.innerHTML = `
        <div class="metadata-section">
            <h3>Attachment Information</h3>
            <div class="metadata-row">
                <span class="metadata-label">ID:</span>
                <span class="metadata-value">${imageData.attachment_id || 'Unknown'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Filename:</span>
                <span class="metadata-value">${imageData.filename || 'Unknown'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Content Type:</span>
                <span class="metadata-value">${imageData.content_type || 'Unknown'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Size:</span>
                <span class="metadata-value">${formatBytesHelper(imageData.size)}</span>
            </div>
        </div>
        
        <div class="metadata-section">
            <h3>Email Metadata</h3>
            <div class="metadata-row">
                <span class="metadata-label">Email ID:</span>
                <span class="metadata-value">${imageData.email_id || 'Unknown'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Subject:</span>
                <span class="metadata-value">${imageData.email_subject || 'No subject'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">From:</span>
                <span class="metadata-value">${imageData.email_from || 'Unknown'}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Date:</span>
                <span class="metadata-value">${formatDate(imageData.email_date)}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Folder:</span>
                <span class="metadata-value">${imageData.email_folder || 'Unknown'}</span>
            </div>
        </div>
    `;
    
    modal.style.display = 'block';
}

function closeMetadataModal(event) {
    if (event) {
        event.stopPropagation();
    }
    const modal = document.getElementById('metadata-modal');
    modal.style.display = 'none';
}

async function showEmail(emailId) {
    const modal = document.getElementById('email-modal');
    const content = document.getElementById('email-content');
    const metadataDisplay = document.getElementById('email-metadata-display');
    
    modal.style.display = 'block';
    metadataDisplay.innerHTML = '<div style="text-align: center; padding: 20px; color: #666;">Loading metadata...</div>';
    content.innerHTML = '<div style="text-align: center; padding: 40px; color: #666;">Loading email...</div>';
    
    try {
        // Fetch email metadata first
        const metadataResponse = await fetch(`${API_BASE}/emails/${emailId}/metadata`);
        let metadataHtml = '';
        
        if (metadataResponse.ok) {
            const metadata = await metadataResponse.json();
            
            const formatDate = (dateStr) => {
                if (!dateStr) return 'Unknown';
                try {
                    return new Date(dateStr).toLocaleString();
                } catch {
                    return dateStr;
                }
            };
            
            metadataHtml = `
                <div style="background: #f8f9fa; border-radius: 8px; padding: 15px; margin-bottom: 15px;">
                    <h3 style="color: #667eea; margin-bottom: 12px; font-size: 1.2em; border-bottom: 2px solid #667eea; padding-bottom: 8px;"></h3>
                    <div style="display: grid; grid-template-columns: 150px 1fr; gap: 8px; font-size: 0.9em;">
                        <div style="font-weight: 600; color: #555;">Subject:</div>
                        <div style="color: #333;">${metadata.subject || 'No subject'}</div>
                        <div style="font-weight: 600; color: #555;">From:</div>
                        <div style="color: #333;">${metadata.from_address || 'Unknown'}</div>
                        <div style="font-weight: 600; color: #555;">To:</div>
                        <div style="color: #333;">${metadata.to_addresses || 'Unknown'}</div>
                        ${metadata.cc_addresses ? `
                        <div style="font-weight: 600; color: #555;">CC:</div>
                        <div style="color: #333;">${metadata.cc_addresses}</div>
                        ` : ''}
                        ${metadata.bcc_addresses ? `
                        <div style="font-weight: 600; color: #555;">BCC:</div>
                        <div style="color: #333;">${metadata.bcc_addresses}</div>
                        ` : ''}
                        <div style="font-weight: 600; color: #555;">Date:</div>
                        <div style="color: #333;">${formatDate(metadata.date)}</div>
                        <div style="font-weight: 600; color: #555;">Folder:</div>
                        <div style="color: #333;">${metadata.folder || 'Unknown'}</div>
                        <div style="font-weight: 600; color: #555;">UID:</div>
                        <div style="color: #333;">${metadata.uid || 'Unknown'}</div>
                    </div>
                </div>
            `;
        }
        
        metadataDisplay.innerHTML = metadataHtml;
        
        // Fetch email HTML content (endpoint will fall back to plain text if HTML not available)
        const response = await fetch(`${API_BASE}/emails/${emailId}/html`);
        
        if (!response.ok) {
            // If HTML endpoint fails, try plain text as fallback
            const textResponse = await fetch(`${API_BASE}/emails/${emailId}/text`);
            if (textResponse.ok) {
                const textContent = await textResponse.text();
                // Wrap plain text in HTML for display
                const htmlContent = `<!DOCTYPE html>
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
                content.innerHTML = '';
                const iframe = document.createElement('iframe');
                iframe.srcdoc = htmlContent;
                iframe.style.width = '100%';
                iframe.style.minHeight = '600px';
                iframe.style.border = 'none';
                iframe.style.borderRadius = '6px';
                content.appendChild(iframe);
            } else {
                throw new Error(`Failed to load email: ${response.statusText}`);
            }
        } else {
            const htmlContent = await response.text();
            
            // Create an iframe to display the HTML email content
            content.innerHTML = '';
            const iframe = document.createElement('iframe');
            iframe.srcdoc = htmlContent;
            iframe.style.width = '100%';
            iframe.style.minHeight = '600px';
            iframe.style.border = 'none';
            iframe.style.borderRadius = '6px';
            content.appendChild(iframe);
        }
        
    } catch (error) {
        console.error('Error loading email:', error);
        content.innerHTML = `<div style="color: #c33; padding: 20px; text-align: center;">Error loading email: ${error.message}</div>`;
    }
}

function closeEmailModal(event) {
    if (event) {
        event.stopPropagation();
    }
    const modal = document.getElementById('email-modal');
    const content = document.getElementById('email-content');
    const metadataDisplay = document.getElementById('email-metadata-display');
    modal.style.display = 'none';
    // Clear content to stop loading
    content.innerHTML = '';
    metadataDisplay.innerHTML = '';
}

function showFullImage(imageId) {
    const modal = document.getElementById('image-modal');
    const modalImg = document.getElementById('modal-image');
    const modalPdf = document.getElementById('modal-pdf');
    
    // Get attachment info to check content type
    fetch(`${API_BASE}/attachments/${imageId}/info`)
        .then(response => response.json())
        .then(data => {
            const contentType = data.content_type ? data.content_type.toLowerCase() : '';
            const isPdf = contentType === 'application/pdf';
            
            modal.style.display = 'block';
            
            if (isPdf) {
                // Show PDF in iframe
                modalImg.style.display = 'none';
                modalPdf.style.display = 'block';
                modalPdf.src = `${API_BASE}/attachments/${imageId}`;
            } else {
                // Show image normally
                modalPdf.style.display = 'none';
                modalImg.style.display = 'block';
                modalImg.src = `${API_BASE}/attachments/${imageId}`;
            }
        })
        .catch(error => {
            console.error('Error fetching attachment info:', error);
            // Fallback: try as image
            modalImg.style.display = 'block';
            modalPdf.style.display = 'none';
            modalImg.src = `${API_BASE}/attachments/${imageId}`;
        });
}

function closeModal() {
    const modal = document.getElementById('image-modal');
    const modalImg = document.getElementById('modal-image');
    const modalPdf = document.getElementById('modal-pdf');
    modal.style.display = 'none';
    // Clear sources to stop loading
    modalImg.src = '';
    modalPdf.src = '';
}

// Close modal when clicking the X
document.addEventListener('DOMContentLoaded', function() {
    const closeBtn = document.querySelector('.close-modal');
    if (closeBtn) {
        closeBtn.onclick = closeModal;
    }
    
    // Close modal on Escape key
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            closeModal();
            closeMetadataModal();
            closeEmailModal();
        }
    });
});

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

