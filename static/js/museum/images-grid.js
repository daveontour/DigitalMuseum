let currentPage = 1;
const pageSize = 100;
let totalPages = 1;
let currentSortOrder = 'size';
let currentSortDirection = 'asc';
const API_BASE = window.location.origin;
const selectedImages = new Set();

if (typeof AppDialogs !== 'undefined' && AppDialogs.init) {
    AppDialogs.init();
}

function escapeHtml(s) {
    return String(s ?? '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function wrapAttachmentEmailHtmlForIframe(html) {
    if (typeof wrapEmailHtmlForIframeDisplay === 'function') {
        return wrapEmailHtmlForIframeDisplay(html);
    }

    const readabilityStyle = '<style data-dm-email-readability>html{color-scheme:light;}html,body{background-color:#f0f8ff!important;color:#1a1a1a!important;}body{padding:1rem 1.25rem;line-height:1.6;word-wrap:break-word;}a{color:#2563eb;}</style>';
    const trimmed = String(html || '').trim();
    if (!trimmed) {
        return '<!DOCTYPE html><html><head><meta charset="UTF-8">' + readabilityStyle + '</head><body></body></html>';
    }
    if (/<head[\s>]/i.test(trimmed)) {
        return trimmed.replace(/<head([^>]*)>/i, '<head$1>' + readabilityStyle);
    }
    if (/<html[\s>]/i.test(trimmed)) {
        return trimmed.replace(/<html([^>]*)>/i, '<html$1><head><meta charset="UTF-8">' + readabilityStyle + '</head>');
    }
    return '<!DOCTYPE html><html><head><meta charset="UTF-8">' + readabilityStyle + '</head><body>' + trimmed + '</body></html>';
}

function getTableBody() {
    return document.getElementById('attachments-table-body');
}

function getTableWrap() {
    return document.getElementById('attachments-table-wrap');
}

function registerAttachmentRowData(image) {
    if (!window.__attachmentRowData) {
        window.__attachmentRowData = {};
    }
    window.__attachmentRowData[image.attachment_id] = image;
}

function buildAttachmentEntryCells(image) {
    const sizeStr = image.size ? formatBytes(image.size) : 'Unknown';
    const contentType = image.content_type || 'Unknown';
    const previewUrl = `${API_BASE}/attachments/${image.attachment_id}?preview=true`;
    const noPreviewSvg = 'data:image/svg+xml,' + encodeURIComponent(
        '<svg xmlns="http://www.w3.org/2000/svg" width="50" height="50"><rect fill="#334155" width="50" height="50"/><text x="50%" y="50%" text-anchor="middle" dy=".35em" fill="#94a3b8" font-size="8">—</text></svg>'
    );

    registerAttachmentRowData(image);

    const cells = [];
    const classNames = [
        'attachments-td-select',
        'attachments-td-preview',
        'attachments-td-size',
        'attachments-td-type',
        'attachments-td-actions'
    ];
    const htmlParts = [
        `<label class="attachments-select-label">
            <input type="checkbox" id="img-${image.attachment_id}" aria-label="Select attachment ${image.attachment_id}" onchange="toggleSelection(${image.attachment_id})">
        </label>`,
        `<img class="attachments-preview-img" src="${escapeHtml(previewUrl)}" alt="" width="50" height="50" loading="lazy" onerror="this.src='${noPreviewSvg}'">`,
        escapeHtml(sizeStr),
        escapeHtml(contentType),
        `<div class="attachments-actions-cell">
            <button type="button" class="modal-btn modal-btn-secondary attachments-icon-btn" title="View full size" aria-label="View full size" onclick="event.stopPropagation(); showFullImage(${image.attachment_id})">
                <i class="fas fa-expand" aria-hidden="true"></i>
            </button>
            <button type="button" class="modal-btn modal-btn-secondary attachments-icon-btn" title="View metadata" aria-label="View metadata" onclick="event.stopPropagation(); showMetadata(window.__attachmentRowData[${image.attachment_id}])">
                <i class="fas fa-circle-info" aria-hidden="true"></i>
            </button>
            <button type="button" class="modal-btn modal-btn-secondary attachments-icon-btn" title="View email" aria-label="View email" onclick="event.stopPropagation(); showEmail(${image.email_id})">
                <i class="fas fa-envelope" aria-hidden="true"></i>
            </button>
        </div>`
    ];

    htmlParts.forEach((html, index) => {
        const td = document.createElement('td');
        td.className = classNames[index];
        td.classList.add('attachments-entry');
        td.dataset.entryId = String(image.attachment_id);
        td.innerHTML = html;
        cells.push(td);
    });

    return cells;
}

function wireAttachmentEntryClick(cells, image) {
    cells.forEach((cell) => {
        cell.addEventListener('click', (e) => {
            if (e.target.closest('button') || e.target.closest('input') || e.target.closest('label')) {
                return;
            }
            const checkbox = document.getElementById(`img-${image.attachment_id}`);
            if (checkbox) {
                checkbox.checked = !checkbox.checked;
                toggleSelection(image.attachment_id);
            }
        });
    });
}

function appendEmptyAttachmentEntry(tr) {
    const classNames = [
        'attachments-td-select',
        'attachments-td-preview',
        'attachments-td-size',
        'attachments-td-type',
        'attachments-td-actions'
    ];
    classNames.forEach((className) => {
        const td = document.createElement('td');
        td.className = `${className} attachments-entry attachments-entry-empty`;
        tr.appendChild(td);
    });
}

function createEntryGapCell() {
    const td = document.createElement('td');
    td.className = 'attachments-td-gap';
    td.setAttribute('aria-hidden', 'true');
    return td;
}

function buildAttachmentPairRow(leftImage, rightImage) {
    const tr = document.createElement('tr');
    tr.className = 'attachments-table-row';

    const leftCells = buildAttachmentEntryCells(leftImage);
    leftCells.forEach((cell) => tr.appendChild(cell));
    wireAttachmentEntryClick(leftCells, leftImage);

    tr.appendChild(createEntryGapCell());

    if (rightImage) {
        const rightCells = buildAttachmentEntryCells(rightImage);
        rightCells.forEach((cell) => tr.appendChild(cell));
        wireAttachmentEntryClick(rightCells, rightImage);
    } else {
        appendEmptyAttachmentEntry(tr);
    }

    return tr;
}

async function loadImages(page) {
    const tableBody = getTableBody();
    const tableWrap = getTableWrap();
    const loadingEl = document.getElementById('loading');
    const errorEl = document.getElementById('error');

    try {
        if (loadingEl) loadingEl.style.display = 'block';
        if (tableWrap) tableWrap.style.display = 'none';
        if (errorEl) errorEl.style.display = 'none';

        const sortOrder = document.getElementById('sort-order-select').value;
        const sortDirection = document.getElementById('sort-direction-select').value;
        const showAllTypes = document.getElementById('show-all-types-checkbox').checked;
        currentSortOrder = sortOrder;
        currentSortDirection = sortDirection;

        const allTypesParam = showAllTypes ? '&all_types=true' : '';
        const response = await fetch(`${API_BASE}/attachments/images?page=${page}&page_size=${pageSize}&order=${sortOrder}&direction=${sortDirection}${allTypesParam}`);

        if (!response.ok) {
            throw new Error(`Failed to load images: ${response.statusText}`);
        }

        const data = await response.json();

        currentPage = data.page;
        totalPages = data.total_pages;

        document.getElementById('page-info').textContent = `Page ${currentPage} of ${totalPages}`;
        document.getElementById('prev-page-btn').disabled = currentPage <= 1;
        document.getElementById('next-page-btn').disabled = currentPage >= totalPages;

        selectedImages.clear();
        updateDeleteButton();

        window.__attachmentRowData = {};

        if (!tableBody) {
            throw new Error('Attachments table not found');
        }
        tableBody.innerHTML = '';

        if (!data.images || data.images.length === 0) {
            const tr = document.createElement('tr');
            tr.className = 'attachments-table-empty';
            tr.innerHTML = '<td colspan="11">No attachments found</td>';
            tableBody.appendChild(tr);
        } else {
            for (let i = 0; i < data.images.length; i += 2) {
                tableBody.appendChild(buildAttachmentPairRow(data.images[i], data.images[i + 1]));
            }
        }

        if (loadingEl) loadingEl.style.display = 'none';
        if (tableWrap) tableWrap.style.display = 'block';
    } catch (error) {
        console.error('Error loading images:', error);
        if (loadingEl) loadingEl.style.display = 'none';
        if (errorEl) {
            errorEl.style.display = 'block';
            errorEl.textContent = `Error: ${error.message}`;
        }
    }
}

function toggleSelection(imageId) {
    const checkbox = document.getElementById(`img-${imageId}`);
    const cells = document.querySelectorAll(`.attachments-entry[data-entry-id="${imageId}"]`);

    if (checkbox && checkbox.checked) {
        selectedImages.add(imageId);
        cells.forEach((cell) => cell.classList.add('selected'));
    } else {
        selectedImages.delete(imageId);
        cells.forEach((cell) => cell.classList.remove('selected'));
    }

    updateDeleteButton();
}

function selectAll() {
    document.querySelectorAll('#attachments-table-body input[type="checkbox"]').forEach((checkbox) => {
        checkbox.checked = true;
        const id = parseInt(checkbox.id.replace('img-', ''), 10);
        if (!Number.isNaN(id)) {
            selectedImages.add(id);
            document.querySelectorAll(`.attachments-entry[data-entry-id="${id}"]`).forEach((cell) => {
                cell.classList.add('selected');
            });
        }
    });
    updateDeleteButton();
}

function updateDeleteButton() {
    const btn = document.getElementById('delete-selected-btn');
    const btnText = selectedImages.size > 0 ? `Delete Selected (${selectedImages.size})` : 'Delete Selected';
    if (btn) {
        btn.disabled = selectedImages.size === 0;
        btn.textContent = btnText;
    }
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
    if (deleteBtn) {
        deleteBtn.disabled = true;
        deleteBtn.textContent = 'Deleting...';
    }

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
                <span class="metadata-value">${escapeHtml(imageData.attachment_id || 'Unknown')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Filename:</span>
                <span class="metadata-value">${escapeHtml(imageData.filename || 'Unknown')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Content Type:</span>
                <span class="metadata-value">${escapeHtml(imageData.content_type || 'Unknown')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Size:</span>
                <span class="metadata-value">${escapeHtml(formatBytesHelper(imageData.size))}</span>
            </div>
        </div>

        <div class="metadata-section">
            <h3>Email Metadata</h3>
            <div class="metadata-row">
                <span class="metadata-label">Email ID:</span>
                <span class="metadata-value">${escapeHtml(imageData.email_id || 'Unknown')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Subject:</span>
                <span class="metadata-value">${escapeHtml(imageData.email_subject || 'No subject')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">From:</span>
                <span class="metadata-value">${escapeHtml(imageData.email_from || 'Unknown')}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Date:</span>
                <span class="metadata-value">${escapeHtml(formatDate(imageData.email_date))}</span>
            </div>
            <div class="metadata-row">
                <span class="metadata-label">Folder:</span>
                <span class="metadata-value">${escapeHtml(imageData.email_folder || 'Unknown')}</span>
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
    if (modal) {
        modal.style.display = 'none';
    }
}

async function showEmail(emailId) {
    const modal = document.getElementById('email-modal');
    const content = document.getElementById('email-content');
    const metadataDisplay = document.getElementById('email-metadata-display');

    modal.style.display = 'block';
    metadataDisplay.innerHTML = '<div class="email-modal-loading">Loading metadata...</div>';
    content.innerHTML = '<div class="email-modal-loading">Loading email...</div>';

    try {
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
                        <div style="color: #333;">${escapeHtml(metadata.subject || 'No subject')}</div>
                        <div style="font-weight: 600; color: #555;">From:</div>
                        <div style="color: #333;">${escapeHtml(metadata.from_address || 'Unknown')}</div>
                        <div style="font-weight: 600; color: #555;">To:</div>
                        <div style="color: #333;">${escapeHtml(metadata.to_addresses || 'Unknown')}</div>
                        ${metadata.cc_addresses ? `
                        <div style="font-weight: 600; color: #555;">CC:</div>
                        <div style="color: #333;">${escapeHtml(metadata.cc_addresses)}</div>
                        ` : ''}
                        ${metadata.bcc_addresses ? `
                        <div style="font-weight: 600; color: #555;">BCC:</div>
                        <div style="color: #333;">${escapeHtml(metadata.bcc_addresses)}</div>
                        ` : ''}
                        <div style="font-weight: 600; color: #555;">Date:</div>
                        <div style="color: #333;">${escapeHtml(formatDate(metadata.date))}</div>
                        <div style="font-weight: 600; color: #555;">Folder:</div>
                        <div style="color: #333;">${escapeHtml(metadata.folder || 'Unknown')}</div>
                        <div style="font-weight: 600; color: #555;">UID:</div>
                        <div style="color: #333;">${escapeHtml(metadata.uid || 'Unknown')}</div>
                    </div>
                </div>
            `;
        }

        metadataDisplay.innerHTML = metadataHtml;

        const response = await fetch(`${API_BASE}/emails/${emailId}/html`);

        if (!response.ok) {
            const textResponse = await fetch(`${API_BASE}/emails/${emailId}/text`);
            if (textResponse.ok) {
                const textContent = await textResponse.text();
                const escaped = escapeHtml(textContent);
                const htmlContent = wrapAttachmentEmailHtmlForIframe(
                    '<div style="white-space:pre-wrap;">' + escaped + '</div>'
                );
                content.innerHTML = '';
                const iframe = document.createElement('iframe');
                iframe.srcdoc = htmlContent;
                iframe.className = 'email-viewer-iframe';
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
            content.innerHTML = '';
            const iframe = document.createElement('iframe');
            iframe.srcdoc = wrapAttachmentEmailHtmlForIframe(htmlContent);
            iframe.className = 'email-viewer-iframe';
            iframe.style.width = '100%';
            iframe.style.minHeight = '600px';
            iframe.style.border = 'none';
            iframe.style.borderRadius = '6px';
            content.appendChild(iframe);
        }
    } catch (error) {
        console.error('Error loading email:', error);
        content.innerHTML = `<div style="color: #c33; padding: 20px; text-align: center;">Error loading email: ${escapeHtml(error.message)}</div>`;
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
    content.innerHTML = '';
    metadataDisplay.innerHTML = '';
}

function showFullImage(imageId) {
    const modal = document.getElementById('image-modal');
    const modalImg = document.getElementById('modal-image');
    const modalPdf = document.getElementById('modal-pdf');

    fetch(`${API_BASE}/attachments/${imageId}/info`)
        .then(response => response.json())
        .then(data => {
            const contentType = data.content_type ? data.content_type.toLowerCase() : '';
            const isPdf = contentType === 'application/pdf';

            modal.style.display = 'block';

            if (isPdf) {
                modalImg.style.display = 'none';
                modalPdf.style.display = 'block';
                modalPdf.src = `${API_BASE}/attachments/${imageId}`;
            } else {
                modalPdf.style.display = 'none';
                modalImg.style.display = 'block';
                modalImg.src = `${API_BASE}/attachments/${imageId}`;
            }
        })
        .catch(error => {
            console.error('Error fetching attachment info:', error);
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
    modalImg.src = '';
    modalPdf.src = '';
}

document.addEventListener('DOMContentLoaded', function() {
    const closeBtn = document.querySelector('.close-modal');
    if (closeBtn) {
        closeBtn.onclick = closeModal;
    }

    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            closeModal();
            closeMetadataModal();
            closeEmailModal();
        }
    });

    if (getTableBody() && document.getElementById('sort-order-select')) {
        loadImages(1);
    }
});

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}
