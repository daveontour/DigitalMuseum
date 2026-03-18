/**
 * Artefacts Gallery Modal
 * Allows browsing, creating, editing and deleting artefacts with associated photos.
 */

Modals.Artefacts = (() => {
    let artefacts = [];
    let currentArtefact = null;   // The artefact currently shown in the detail modal
    let isCreating = false;

    // -------------------------------------------------------------------------
    // Gallery Modal
    // -------------------------------------------------------------------------

    function open() {
        const modal = document.getElementById('artefacts-modal');
        if (modal) modal.style.display = 'flex';
        _loadArtefacts();
    }

    function close() {
        const modal = document.getElementById('artefacts-modal');
        if (modal) modal.style.display = 'none';
    }

    async function _loadArtefacts() {
        const searchEl = document.getElementById('artefacts-search');
        const term = searchEl ? searchEl.value.trim() : '';
        const params = new URLSearchParams();
        if (term) params.set('search', term);
        try {
            const resp = await fetch('/artefacts?' + params.toString());
            if (!resp.ok) throw new Error('Failed to load artefacts');
            artefacts = await resp.json();
        } catch (err) {
            console.error('Error loading artefacts:', err);
            artefacts = [];
        }
        _renderGallery();
    }

    function _renderGallery() {
        const grid = document.getElementById('artefacts-thumbnail-grid');
        if (!grid) return;
        grid.innerHTML = '';

        if (artefacts.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'artefacts-empty-state';
            empty.innerHTML = '<i class="fas fa-box-open"></i>No artefacts found. Click "+ New Artefact" to add one.';
            grid.appendChild(empty);
            return;
        }

        artefacts.forEach(artefact => {
            const card = document.createElement('div');
            card.className = 'artefact-card';
            card.addEventListener('click', () => _openDetail(artefact.id));

            // Thumbnail or placeholder
            if (artefact.primary_thumbnail_url) {
                const img = document.createElement('img');
                img.className = 'artefact-card-thumb';
                img.src = artefact.primary_thumbnail_url;
                img.alt = artefact.name;
                img.onerror = function() {
                    this.parentNode.replaceChild(_makePlaceholder(), this);
                };
                card.appendChild(img);
            } else {
                card.appendChild(_makePlaceholder());
            }

            // Card body
            const body = document.createElement('div');
            body.className = 'artefact-card-body';

            const name = document.createElement('div');
            name.className = 'artefact-card-name';
            name.textContent = artefact.name;
            body.appendChild(name);

            if (artefact.description) {
                const desc = document.createElement('div');
                desc.className = 'artefact-card-desc';
                desc.textContent = artefact.description;
                body.appendChild(desc);
            }

            if (artefact.tags) {
                const tagRow = document.createElement('div');
                tagRow.className = 'artefact-card-tags';
                artefact.tags.split(',').forEach(t => {
                    const tag = t.trim();
                    if (!tag) return;
                    const badge = document.createElement('span');
                    badge.className = 'artefact-tag-badge';
                    badge.textContent = tag;
                    tagRow.appendChild(badge);
                });
                body.appendChild(tagRow);
            }

            card.appendChild(body);
            grid.appendChild(card);
        });
    }

    function _makePlaceholder() {
        const ph = document.createElement('div');
        ph.className = 'artefact-card-placeholder';
        ph.innerHTML = '<i class="fas fa-box-open"></i>';
        return ph;
    }

    // -------------------------------------------------------------------------
    // Detail Modal
    // -------------------------------------------------------------------------

    async function _openDetail(artefactId) {
        try {
            const resp = await fetch(`/artefacts/${artefactId}`);
            if (!resp.ok) throw new Error('Failed to load artefact');
            currentArtefact = await resp.json();
        } catch (err) {
            console.error('Error loading artefact:', err);
            return;
        }
        isCreating = false;
        _populateDetailModal(currentArtefact);
        _showDetailModal();
    }

    function _openCreate() {
        currentArtefact = null;
        isCreating = true;
        // Clear fields
        _setField('artefact-detail-name', '');
        _setField('artefact-detail-description', '');
        _setField('artefact-detail-tags', '');
        _setField('artefact-detail-story', '');
        const titleBar = document.getElementById('artefact-detail-title-text');
        if (titleBar) titleBar.textContent = 'New Artefact';
        const strip = document.getElementById('artefact-photos-strip');
        if (strip) strip.innerHTML = '';
        const saveBtn = document.getElementById('artefact-save-btn');
        if (saveBtn) saveBtn.textContent = 'Create';
        const deleteBtn = document.getElementById('artefact-delete-btn');
        if (deleteBtn) deleteBtn.style.display = 'none';
        // Disable photo buttons until artefact is saved
        _setPhotoButtonsEnabled(false);
        _showDetailModal();
    }

    function _populateDetailModal(artefact) {
        _setField('artefact-detail-name', artefact.name || '');
        _setField('artefact-detail-description', artefact.description || '');
        _setField('artefact-detail-tags', artefact.tags || '');
        _setField('artefact-detail-story', artefact.story || '');
        const titleBar = document.getElementById('artefact-detail-title-text');
        if (titleBar) titleBar.textContent = artefact.name || 'Artefact';
        const saveBtn = document.getElementById('artefact-save-btn');
        if (saveBtn) saveBtn.textContent = 'Save';
        const deleteBtn = document.getElementById('artefact-delete-btn');
        if (deleteBtn) deleteBtn.style.display = '';
        _setPhotoButtonsEnabled(true);
        _renderPhotoStrip(artefact.media_items || []);
    }

    function _renderPhotoStrip(mediaItems) {
        const strip = document.getElementById('artefact-photos-strip');
        if (!strip) return;
        strip.innerHTML = '';

        if (mediaItems.length === 0) {
            const msg = document.createElement('div');
            msg.style.cssText = 'color:#aaa; font-size:0.85rem; text-align:center; padding:16px;';
            msg.textContent = 'No photos yet. Add photos using the buttons below.';
            strip.appendChild(msg);
            return;
        }

        mediaItems.forEach(item => {
            const wrap = document.createElement('div');
            wrap.className = 'artefact-photo-thumb-wrap';
            wrap.dataset.mediaItemId = item.media_item_id;

            const img = document.createElement('img');
            img.src = item.thumbnail_url || '';
            img.alt = item.title || 'Photo';
            img.loading = 'lazy';
            img.onerror = function() {
                this.src = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="100" height="80"%3E%3Crect fill="%23ddd" width="100" height="80"/%3E%3C/svg%3E';
            };
            // Click to view full-size
            img.addEventListener('click', () => {
                if (Modals.SingleImageDisplay && Modals.SingleImageDisplay.showSingleImageModal) {
                    const blobId = item.media_blob_id;
                    if (blobId) {
                        Modals.SingleImageDisplay.showSingleImageModal(
                            item.title || 'Photo',
                            `/images/${blobId}?type=blob`,
                            null, null, null
                        );
                    }
                }
            });

            const removeBtn = document.createElement('button');
            removeBtn.className = 'artefact-photo-remove-btn';
            removeBtn.title = 'Remove photo';
            removeBtn.innerHTML = '&times;';
            removeBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                Modals.ConfirmationModal.open(
                    'Remove Photo',
                    'Remove this photo from the artefact?',
                    () => _removePhoto(item.media_item_id)
                );
            });

            wrap.appendChild(img);
            wrap.appendChild(removeBtn);
            strip.appendChild(wrap);
        });
    }

    function _showDetailModal() {
        const modal = document.getElementById('artefact-detail-modal');
        if (modal) modal.style.display = 'flex';
    }

    function _closeDetailModal() {
        const modal = document.getElementById('artefact-detail-modal');
        if (modal) modal.style.display = 'none';
        currentArtefact = null;
        isCreating = false;
    }

    // -------------------------------------------------------------------------
    // CRUD operations
    // -------------------------------------------------------------------------

    async function _handleSave() {
        const name = _getField('artefact-detail-name').trim();
        if (!name) {
            alert('Please enter an artefact name.');
            return;
        }
        const payload = {
            name,
            description: _getField('artefact-detail-description') || null,
            tags: _getField('artefact-detail-tags') || null,
            story: _getField('artefact-detail-story') || null,
        };

        try {
            let resp;
            if (isCreating) {
                resp = await fetch('/artefacts', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload),
                });
            } else {
                resp = await fetch(`/artefacts/${currentArtefact.id}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload),
                });
            }
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || 'Save failed');
            }
            const saved = await resp.json();
            currentArtefact = saved;
            isCreating = false;

            // Enable photo buttons now that artefact exists
            _setPhotoButtonsEnabled(true);
            _populateDetailModal(saved);

            // Refresh gallery in background
            _loadArtefacts();
        } catch (err) {
            console.error('Error saving artefact:', err);
            alert(`Error saving artefact: ${err.message}`);
        }
    }

    function _handleDelete() {
        if (!currentArtefact) return;
        Modals.ConfirmationModal.open(
            'Delete Artefact',
            `Are you sure you want to delete "${currentArtefact.name}"? This cannot be undone.`,
            async () => {
                try {
                    const resp = await fetch(`/artefacts/${currentArtefact.id}`, { method: 'DELETE' });
                    if (!resp.ok) throw new Error('Delete failed');
                    _closeDetailModal();
                    _loadArtefacts();
                } catch (err) {
                    console.error('Error deleting artefact:', err);
                    alert(`Error deleting artefact: ${err.message}`);
                }
            }
        );
    }

    // -------------------------------------------------------------------------
    // Photo management
    // -------------------------------------------------------------------------

    async function _handleFileUpload(file) {
        if (!currentArtefact) return;
        const formData = new FormData();
        formData.append('file', file);
        formData.append('title', file.name || 'photo');
        try {
            const resp = await fetch(`/artefacts/${currentArtefact.id}/media/upload`, {
                method: 'POST',
                body: formData,
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || 'Upload failed');
            }
            const updated = await resp.json();
            currentArtefact = updated;
            _renderPhotoStrip(updated.media_items || []);
            _loadArtefacts();
        } catch (err) {
            console.error('Error uploading photo:', err);
            alert(`Error uploading photo: ${err.message}`);
        }
    }

    async function _handlePickFromGallery(mediaItemId) {
        if (!currentArtefact) return;
        try {
            const resp = await fetch(`/artefacts/${currentArtefact.id}/media/${mediaItemId}`, {
                method: 'POST',
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || 'Link failed');
            }
            const updated = await resp.json();
            currentArtefact = updated;
            _renderPhotoStrip(updated.media_items || []);
            // Re-show detail modal (gallery pick mode closed it briefly)
            _showDetailModal();
            _loadArtefacts();
        } catch (err) {
            console.error('Error linking photo:', err);
            alert(`Error linking photo: ${err.message}`);
        }
    }

    async function _removePhoto(mediaItemId) {
        if (!currentArtefact) return;
        try {
            const resp = await fetch(`/artefacts/${currentArtefact.id}/media/${mediaItemId}`, {
                method: 'DELETE',
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || 'Remove failed');
            }
            const updated = await resp.json();
            currentArtefact = updated;
            _renderPhotoStrip(updated.media_items || []);
            _loadArtefacts();
        } catch (err) {
            console.error('Error removing photo:', err);
            alert(`Error removing photo: ${err.message}`);
        }
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    function _setField(id, value) {
        const el = document.getElementById(id);
        if (el) el.value = value;
    }

    function _getField(id) {
        const el = document.getElementById(id);
        return el ? el.value : '';
    }

    function _setPhotoButtonsEnabled(enabled) {
        const fileBtn = document.getElementById('artefact-add-photo-file-btn');
        const galBtn = document.getElementById('artefact-add-photo-gallery-btn');
        if (fileBtn) fileBtn.disabled = !enabled;
        if (galBtn) galBtn.disabled = !enabled;
    }

    // -------------------------------------------------------------------------
    // Populate tags autocomplete
    // -------------------------------------------------------------------------

    // -------------------------------------------------------------------------
    // Export / Import
    // -------------------------------------------------------------------------

    function _handleExport() {
        // Trigger a direct download via a temporary anchor pointing at the export endpoint
        const a = document.createElement('a');
        a.href = '/artefacts/export';
        a.download = 'artefacts_export.json';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
    }

    async function _handleImport(file) {
        const importBtn = document.getElementById('artefacts-import-btn');
        if (importBtn) {
            importBtn.disabled = true;
            importBtn.textContent = 'Importing…';
        }
        try {
            const formData = new FormData();
            formData.append('file', file);
            const resp = await fetch('/artefacts/import', { method: 'POST', body: formData });
            const result = await resp.json();
            if (!resp.ok) {
                alert(`Import failed: ${result.detail || 'Unknown error'}`);
            } else {
                alert(result.message);
                _loadArtefacts();
            }
        } catch (err) {
            console.error('Import error:', err);
            alert(`Import error: ${err.message}`);
        } finally {
            if (importBtn) {
                importBtn.disabled = false;
                importBtn.innerHTML = '<i class="fas fa-upload"></i> Import';
            }
        }
    }

    async function _loadTagSuggestions() {
        try {
            const resp = await fetch('/images/tags');
            if (!resp.ok) return;
            const tags = await resp.json();
            const datalist = document.getElementById('artefact-tags-list');
            if (!datalist) return;
            datalist.innerHTML = '';
            tags.forEach(tag => {
                const opt = document.createElement('option');
                opt.value = tag;
                datalist.appendChild(opt);
            });
        } catch (err) {
            // Non-critical, ignore
        }
    }

    // -------------------------------------------------------------------------
    // Init – wire up event listeners
    // -------------------------------------------------------------------------

    function init() {
        // Gallery modal close
        const closeGalleryBtn = document.getElementById('close-artefacts-modal');
        if (closeGalleryBtn) closeGalleryBtn.addEventListener('click', close);

        // Search
        const searchBtn = document.getElementById('artefacts-search-btn');
        if (searchBtn) searchBtn.addEventListener('click', _loadArtefacts);

        const searchInput = document.getElementById('artefacts-search');
        if (searchInput) {
            searchInput.addEventListener('keydown', e => {
                if (e.key === 'Enter') _loadArtefacts();
            });
        }

        // Clear
        const clearBtn = document.getElementById('artefacts-clear-btn');
        if (clearBtn) {
            clearBtn.addEventListener('click', () => {
                const inp = document.getElementById('artefacts-search');
                if (inp) inp.value = '';
                _loadArtefacts();
            });
        }

        // Export
        const exportBtn = document.getElementById('artefacts-export-btn');
        if (exportBtn) {
            exportBtn.addEventListener('click', _handleExport);
        }

        // Import
        const importBtn = document.getElementById('artefacts-import-btn');
        const importFile = document.getElementById('artefacts-import-file');
        if (importBtn && importFile) {
            importBtn.addEventListener('click', () => importFile.click());
            importFile.addEventListener('change', async e => {
                const file = e.target.files[0];
                if (file) {
                    await _handleImport(file);
                    importFile.value = '';
                }
            });
        }

        // New artefact button
        const newBtn = document.getElementById('artefacts-new-btn');
        if (newBtn) newBtn.addEventListener('click', _openCreate);

        // Detail modal close
        const closeDetailBtn = document.getElementById('close-artefact-detail');
        if (closeDetailBtn) closeDetailBtn.addEventListener('click', _closeDetailModal);

        // Live-update title bar as user types the artefact name
        const nameInput = document.getElementById('artefact-detail-name');
        if (nameInput) {
            nameInput.addEventListener('input', () => {
                const titleBar = document.getElementById('artefact-detail-title-text');
                if (titleBar) titleBar.textContent = nameInput.value.trim() || (isCreating ? 'New Artefact' : 'Artefact');
            });
        }

        // Save button
        const saveBtn = document.getElementById('artefact-save-btn');
        if (saveBtn) saveBtn.addEventListener('click', _handleSave);

        // Delete button
        const deleteBtn = document.getElementById('artefact-delete-btn');
        if (deleteBtn) deleteBtn.addEventListener('click', _handleDelete);

        // Upload photo via file input
        const fileInput = document.getElementById('artefact-photo-file-input');
        const fileBtn = document.getElementById('artefact-add-photo-file-btn');
        if (fileBtn && fileInput) {
            fileBtn.addEventListener('click', () => fileInput.click());
            fileInput.addEventListener('change', e => {
                const file = e.target.files[0];
                if (file) {
                    _handleFileUpload(file);
                    fileInput.value = ''; // reset so same file can be re-selected
                }
            });
        }

        // Choose from gallery (pick mode)
        const galBtn = document.getElementById('artefact-add-photo-gallery-btn');
        if (galBtn) {
            galBtn.addEventListener('click', () => {
                // Temporarily hide detail modal, open gallery in pick mode
                const detailModal = document.getElementById('artefact-detail-modal');
                if (detailModal) detailModal.style.display = 'none';
                Modals.NewImageGallery.openPickMode((mediaItemId) => {
                    _handlePickFromGallery(mediaItemId);
                });
            });
        }

        // Close gallery modal on background click
        const galleryModal = document.getElementById('artefacts-modal');
        if (galleryModal) {
            galleryModal.addEventListener('click', e => {
                if (e.target === galleryModal) close();
            });
        }

        // Close detail modal on background click
        const detailModal = document.getElementById('artefact-detail-modal');
        if (detailModal) {
            detailModal.addEventListener('click', e => {
                if (e.target === detailModal) _closeDetailModal();
            });
        }

        _loadTagSuggestions();
    }

    return { open, close, init };
})();
