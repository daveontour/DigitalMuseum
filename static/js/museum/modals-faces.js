/**
 * People in Photos (Face Recognition) Modal
 * Browse face clusters detected by the "Detect faces in photos" background job,
 * link a cluster to a Contact to name it, review member photos, and detach a
 * face that was grouped wrong ("not this person").
 */

Modals.Faces = (() => {
    let currentFilter = 'all';       // 'all' | 'named' | 'unnamed'
    let minGroupSize = 1;            // 1 = no filter; >1 = only clusters with at least this many faces
    let currentClusterId = null;
    let allContactNames = null;      // [{id, name}], loaded lazily and cached for the session

    const PAGE_SIZE = 48;            // matches defaultFaceClustersPageSize in face_handler.go
    let currentOffset = 0;
    let currentTotal = 0;

    let selectMode = false;
    let selectedFaceIds = new Set();  // representative_face_id set, persists across pages/filters

    let memberSelectMode = false;
    let selectedMemberFaceIds = new Set();  // face id set, scoped to the currently open cluster detail

    // -------------------------------------------------------------------------
    // Gallery Modal
    // -------------------------------------------------------------------------

    function open() {
        const modal = document.getElementById('faces-modal');
        if (modal) modal.style.display = 'flex';
        currentOffset = 0;
        _exitSelectMode();
        _loadClusters();
    }

    function close() {
        const modal = document.getElementById('faces-modal');
        if (modal) modal.style.display = 'none';
    }

    async function _loadClusters() {
        const grid = document.getElementById('faces-cluster-grid');
        const empty = document.getElementById('faces-empty-state');
        if (!grid) return;

        const params = new URLSearchParams();
        if (currentFilter === 'named') params.set('named', 'true');
        if (currentFilter === 'unnamed') params.set('named', 'false');
        if (minGroupSize > 1) params.set('min_face_count', String(minGroupSize));
        params.set('limit', String(PAGE_SIZE));
        params.set('offset', String(currentOffset));

        let clusters = [];
        try {
            const resp = await fetch('/api/faces/clusters?' + params.toString());
            if (!resp.ok) throw new Error('Failed to load face clusters');
            const data = await resp.json();
            clusters = data.clusters || [];
            currentTotal = data.total || 0;
        } catch (err) {
            console.error('Error loading face clusters:', err);
            currentTotal = 0;
        }

        grid.innerHTML = '';
        _renderPagination();
        if (clusters.length === 0) {
            if (empty) empty.style.display = 'flex';
            return;
        }
        if (empty) empty.style.display = 'none';

        clusters.forEach(cluster => {
            const card = document.createElement('div');
            card.className = 'face-cluster-card';
            if (cluster.representative_face_id && selectedFaceIds.has(cluster.representative_face_id)) {
                card.classList.add('face-cluster-card-selected');
            }
            card.addEventListener('click', () => {
                if (selectMode) {
                    _toggleCardSelection(card, cluster);
                } else {
                    _openDetail(cluster.id);
                }
            });

            // A padding-top-based aspect-ratio box, not the CSS aspect-ratio
            // property: that property was not reliably taking effect here
            // (thumbnails rendered squashed to a thin strip even after the
            // backend crop-geometry fix), so this uses the older, universally
            // supported technique instead — a square box sized purely from
            // its own width, with the <img> absolutely positioned to fill it.
            const thumbWrap = document.createElement('div');
            thumbWrap.className = 'face-cluster-card-thumb-wrap';

            const img = document.createElement('img');
            img.className = 'face-cluster-card-thumb';
            img.src = cluster.thumbnail_url;
            img.alt = cluster.contact_name || 'Unnamed person';
            img.onerror = function () {
                this.style.visibility = 'hidden';
            };
            thumbWrap.appendChild(img);

            if (cluster.representative_face_id) {
                const selectWrap = document.createElement('div');
                selectWrap.className = 'face-cluster-card-select-wrap';
                const checkbox = document.createElement('input');
                checkbox.type = 'checkbox';
                checkbox.className = 'face-cluster-card-select-checkbox';
                checkbox.tabIndex = -1;
                checkbox.checked = selectedFaceIds.has(cluster.representative_face_id);
                selectWrap.appendChild(checkbox);
                selectWrap.addEventListener('click', (evt) => {
                    evt.preventDefault();
                    evt.stopPropagation();
                    _toggleCardSelection(card, cluster);
                });
                thumbWrap.appendChild(selectWrap);
            }

            if (cluster.representative_face_id) {
                const ignoreBtn = document.createElement('button');
                ignoreBtn.type = 'button';
                ignoreBtn.className = 'face-cluster-card-ignore-btn';
                ignoreBtn.title = 'Ignore this face';
                ignoreBtn.innerHTML = '<i class="fas fa-times"></i>';
                ignoreBtn.addEventListener('click', (evt) => {
                    evt.stopPropagation();
                    _ignoreFace(cluster.representative_face_id);
                });
                thumbWrap.appendChild(ignoreBtn);
            }

            card.appendChild(thumbWrap);

            const body = document.createElement('div');
            body.className = 'face-cluster-card-body';

            const name = document.createElement('div');
            name.className = 'face-cluster-card-name';
            if (cluster.contact_name) {
                name.textContent = cluster.contact_name;
            } else {
                name.innerHTML = '<em>Unnamed</em>';
                name.classList.add('face-cluster-card-unnamed');
            }
            body.appendChild(name);

            const count = document.createElement('div');
            count.className = 'face-cluster-card-count';
            count.textContent = cluster.face_count + (cluster.face_count === 1 ? ' photo' : ' photos');
            body.appendChild(count);

            card.appendChild(body);
            grid.appendChild(card);
        });
    }

    function _setFilter(filter) {
        currentFilter = filter;
        currentOffset = 0;
        selectedFaceIds.clear();
        _updateBulkCount();
        document.querySelectorAll('.faces-filter-tab').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.filter === filter);
        });
        _loadClusters();
    }

    let minGroupSizeDebounceTimer = null;

    function _onMinGroupSizeInput(evt) {
        if (minGroupSizeDebounceTimer) clearTimeout(minGroupSizeDebounceTimer);
        minGroupSizeDebounceTimer = setTimeout(() => {
            const raw = evt.target.value.trim();
            let n = parseInt(raw, 10);
            if (!raw || isNaN(n) || n < 1) n = 1;
            minGroupSize = n;
            currentOffset = 0;
            selectedFaceIds.clear();
            _updateBulkCount();
            _loadClusters();
        }, 400);
    }

    // -------------------------------------------------------------------------
    // Multi-select (bulk ignore)
    // -------------------------------------------------------------------------

    function _toggleSelectMode() {
        if (selectMode) {
            _exitSelectMode();
        } else {
            selectMode = true;
            selectedFaceIds.clear();
            _updateSelectModeUI();
        }
    }

    function _exitSelectMode() {
        selectMode = false;
        selectedFaceIds.clear();
        _updateSelectModeUI();
    }

    function _updateSelectModeUI() {
        const grid = document.getElementById('faces-cluster-grid');
        if (grid) grid.classList.toggle('select-mode', selectMode);

        const toggleBtn = document.getElementById('faces-select-mode-toggle');
        if (toggleBtn) {
            toggleBtn.classList.toggle('active', selectMode);
            toggleBtn.innerHTML = selectMode
                ? '<i class="fas fa-times"></i> Cancel'
                : '<i class="fas fa-check-square"></i> Select';
        }

        const toolbar = document.getElementById('faces-bulk-toolbar');
        if (toolbar) toolbar.style.display = selectMode ? 'flex' : 'none';

        document.querySelectorAll('.face-cluster-card').forEach(card => {
            card.classList.remove('face-cluster-card-selected');
            const cb = card.querySelector('.face-cluster-card-select-checkbox');
            if (cb) cb.checked = false;
        });

        _updateBulkCount();
    }

    function _toggleCardSelection(card, cluster) {
        const faceId = cluster.representative_face_id;
        if (!faceId) return;
        const willSelect = !selectedFaceIds.has(faceId);
        if (willSelect) {
            selectedFaceIds.add(faceId);
        } else {
            selectedFaceIds.delete(faceId);
        }
        card.classList.toggle('face-cluster-card-selected', willSelect);
        const cb = card.querySelector('.face-cluster-card-select-checkbox');
        if (cb) cb.checked = willSelect;
        _updateBulkCount();
    }

    function _updateBulkCount() {
        const countEl = document.getElementById('faces-bulk-count');
        if (countEl) countEl.textContent = selectedFaceIds.size;
        const ignoreBtn = document.getElementById('faces-bulk-ignore-btn');
        if (ignoreBtn) ignoreBtn.disabled = selectedFaceIds.size === 0;
    }

    function _selectAllOnPage() {
        document.querySelectorAll('#faces-cluster-grid .face-cluster-card').forEach(card => {
            const cb = card.querySelector('.face-cluster-card-select-checkbox');
            if (!cb || cb.checked) return;
            card.click();
        });
    }

    function _clearSelection() {
        selectedFaceIds.clear();
        document.querySelectorAll('.face-cluster-card').forEach(card => {
            card.classList.remove('face-cluster-card-selected');
            const cb = card.querySelector('.face-cluster-card-select-checkbox');
            if (cb) cb.checked = false;
        });
        _updateBulkCount();
    }

    async function _ignoreSelected() {
        if (selectedFaceIds.size === 0) return;
        const faceIds = Array.from(selectedFaceIds);
        const count = faceIds.length;

        const ok = await AppDialogs.showAppConfirm(
            'Ignore faces',
            `Ignore ${count} selected face(s)? They will no longer be shown or re-grouped.`,
            { danger: true }
        );
        if (!ok) return;

        const btn = document.getElementById('faces-bulk-ignore-btn');
        const originalHTML = btn ? btn.innerHTML : null;
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Ignoring…';
        }
        try {
            const results = await Promise.all(faceIds.map(faceId =>
                fetch('/api/faces/' + faceId, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ignored: true })
                }).then(resp => ({ faceId, ok: resp.ok }))
                  .catch(err => {
                      console.error('Error ignoring face', faceId, err);
                      return { faceId, ok: false };
                  })
            ));
            const failed = results.filter(r => !r.ok);
            if (failed.length > 0) {
                console.error('Failed to ignore faces:', failed.map(f => f.faceId));
                await AppDialogs.showAppAlert('Ignore faces', `${failed.length} of ${count} could not be ignored. Please try again.`);
            }
        } finally {
            if (btn) btn.innerHTML = originalHTML;
        }

        _exitSelectMode();
        await _loadClusters();
    }

    function _renderPagination() {
        const bar = document.getElementById('faces-pagination');
        if (!bar) return;

        if (currentTotal <= PAGE_SIZE) {
            bar.style.display = 'none';
            return;
        }
        bar.style.display = 'flex';

        const pageStart = currentTotal === 0 ? 0 : currentOffset + 1;
        const pageEnd = Math.min(currentOffset + PAGE_SIZE, currentTotal);

        const info = document.getElementById('faces-pagination-info');
        if (info) info.textContent = pageStart + '–' + pageEnd + ' of ' + currentTotal;

        const prevBtn = document.getElementById('faces-pagination-prev');
        if (prevBtn) prevBtn.disabled = currentOffset <= 0;

        const nextBtn = document.getElementById('faces-pagination-next');
        if (nextBtn) nextBtn.disabled = currentOffset + PAGE_SIZE >= currentTotal;
    }

    function _prevPage() {
        if (currentOffset <= 0) return;
        currentOffset = Math.max(0, currentOffset - PAGE_SIZE);
        const grid = document.getElementById('faces-cluster-grid');
        if (grid) grid.scrollTop = 0;
        _loadClusters();
    }

    function _nextPage() {
        if (currentOffset + PAGE_SIZE >= currentTotal) return;
        currentOffset += PAGE_SIZE;
        const grid = document.getElementById('faces-cluster-grid');
        if (grid) grid.scrollTop = 0;
        _loadClusters();
    }

    // -------------------------------------------------------------------------
    // Cluster Detail / Naming Modal
    // -------------------------------------------------------------------------

    async function _openDetail(clusterId) {
        currentClusterId = clusterId;
        const modal = document.getElementById('face-cluster-detail-modal');
        if (modal) modal.style.display = 'flex';
        _exitMemberSelectMode();
        await _loadDetail();
    }

    function _closeDetail() {
        const modal = document.getElementById('face-cluster-detail-modal');
        if (modal) modal.style.display = 'none';
        currentClusterId = null;
        _hideContactResults();
        _exitMemberSelectMode();
    }

    async function _loadDetail() {
        if (!currentClusterId) return;
        let data;
        try {
            const resp = await fetch('/api/faces/clusters/' + currentClusterId);
            if (!resp.ok) throw new Error('Failed to load cluster');
            data = await resp.json();
        } catch (err) {
            console.error('Error loading face cluster detail:', err);
            return;
        }
        _renderDetail(data);
    }

    function _renderDetail(data) {
        const titleEl = document.getElementById('face-cluster-detail-title-text');
        if (titleEl) titleEl.textContent = data.contact_name || 'Unnamed person';

        const input = document.getElementById('face-cluster-contact-input');
        if (input) input.value = data.contact_name || '';

        const unlinkBtn = document.getElementById('face-cluster-unlink-btn');
        if (unlinkBtn) {
            unlinkBtn.style.display = data.contact_id ? 'inline-flex' : 'none';
            unlinkBtn.onclick = () => _linkContact(null);
        }

        // Suggestions (only present for unnamed clusters — see backend).
        const suggestBox = document.getElementById('face-cluster-suggestions');
        if (suggestBox) {
            suggestBox.innerHTML = '';
            const suggestions = data.suggestions || [];
            if (suggestions.length > 0) {
                const label = document.createElement('div');
                label.className = 'face-cluster-suggestions-label';
                label.textContent = 'Possible match:';
                suggestBox.appendChild(label);
                suggestions.forEach(s => {
                    const chip = document.createElement('button');
                    chip.type = 'button';
                    chip.className = 'face-cluster-suggestion-chip';
                    chip.textContent = s.contact_name || ('#' + s.contact_id);
                    chip.addEventListener('click', () => _linkContact(s.contact_id));
                    suggestBox.appendChild(chip);
                });
            }
        }

        // Member faces grid.
        const grid = document.getElementById('face-cluster-member-grid');
        if (grid) {
            grid.classList.toggle('select-mode', memberSelectMode);
            grid.innerHTML = '';
            (data.faces || []).forEach(face => {
                const cell = document.createElement('div');
                cell.className = 'face-cluster-member-cell';
                if (selectedMemberFaceIds.has(face.id)) {
                    cell.classList.add('face-cluster-member-cell-selected');
                }
                cell.addEventListener('click', () => {
                    if (memberSelectMode) {
                        _toggleMemberCellSelection(cell, face.id);
                    } else {
                        _openFullImage(face.media_item_id);
                    }
                });
                cell.classList.add('face-cluster-member-cell-clickable');

                // See _loadClusters' thumbWrap comment: a padding-top
                // aspect-ratio box, not the CSS aspect-ratio property.
                const thumbWrap = document.createElement('div');
                thumbWrap.className = 'face-cluster-member-thumb-wrap';

                const img = document.createElement('img');
                img.className = 'face-cluster-member-thumb';
                img.src = face.crop_url;
                img.alt = 'Detected face';
                thumbWrap.appendChild(img);
                cell.appendChild(thumbWrap);

                const selectWrap = document.createElement('div');
                selectWrap.className = 'face-cluster-member-select-wrap';
                const checkbox = document.createElement('input');
                checkbox.type = 'checkbox';
                checkbox.className = 'face-cluster-member-select-checkbox';
                checkbox.tabIndex = -1;
                checkbox.checked = selectedMemberFaceIds.has(face.id);
                selectWrap.appendChild(checkbox);
                selectWrap.addEventListener('click', (evt) => {
                    evt.preventDefault();
                    evt.stopPropagation();
                    _toggleMemberCellSelection(cell, face.id);
                });
                cell.appendChild(selectWrap);

                const notBtn = document.createElement('button');
                notBtn.type = 'button';
                notBtn.className = 'face-cluster-member-detach-btn';
                notBtn.title = 'Not this person (will be re-grouped next time)';
                notBtn.innerHTML = '<i class="fas fa-times"></i>';
                notBtn.addEventListener('click', (evt) => {
                    evt.stopPropagation();
                    _detachFace(face.id);
                });
                cell.appendChild(notBtn);

                const ignoreBtn = document.createElement('button');
                ignoreBtn.type = 'button';
                ignoreBtn.className = 'face-cluster-member-ignore-btn';
                ignoreBtn.title = 'Ignore this face (not a real face, or never track this)';
                ignoreBtn.innerHTML = '<i class="fas fa-eye-slash"></i>';
                ignoreBtn.addEventListener('click', (evt) => {
                    evt.stopPropagation();
                    _ignoreFace(face.id);
                });
                cell.appendChild(ignoreBtn);

                grid.appendChild(cell);
            });
        }
        _updateMemberBulkCount();
    }

    // -------------------------------------------------------------------------
    // Multi-select (bulk ignore) — cluster detail's member-face grid
    // -------------------------------------------------------------------------

    function _toggleMemberSelectMode() {
        if (memberSelectMode) {
            _exitMemberSelectMode();
        } else {
            memberSelectMode = true;
            selectedMemberFaceIds.clear();
            _updateMemberSelectModeUI();
        }
    }

    function _exitMemberSelectMode() {
        memberSelectMode = false;
        selectedMemberFaceIds.clear();
        _updateMemberSelectModeUI();
    }

    function _updateMemberSelectModeUI() {
        const grid = document.getElementById('face-cluster-member-grid');
        if (grid) grid.classList.toggle('select-mode', memberSelectMode);

        const toggleBtn = document.getElementById('face-cluster-member-select-mode-toggle');
        if (toggleBtn) {
            toggleBtn.classList.toggle('active', memberSelectMode);
            toggleBtn.innerHTML = memberSelectMode
                ? '<i class="fas fa-times"></i> Cancel'
                : '<i class="fas fa-check-square"></i> Select';
        }

        const toolbar = document.getElementById('face-cluster-member-bulk-toolbar');
        if (toolbar) toolbar.style.display = memberSelectMode ? 'flex' : 'none';

        document.querySelectorAll('.face-cluster-member-cell').forEach(cell => {
            cell.classList.remove('face-cluster-member-cell-selected');
            const cb = cell.querySelector('.face-cluster-member-select-checkbox');
            if (cb) cb.checked = false;
        });

        _updateMemberBulkCount();
    }

    function _toggleMemberCellSelection(cell, faceId) {
        const willSelect = !selectedMemberFaceIds.has(faceId);
        if (willSelect) {
            selectedMemberFaceIds.add(faceId);
        } else {
            selectedMemberFaceIds.delete(faceId);
        }
        cell.classList.toggle('face-cluster-member-cell-selected', willSelect);
        const cb = cell.querySelector('.face-cluster-member-select-checkbox');
        if (cb) cb.checked = willSelect;
        _updateMemberBulkCount();
    }

    function _updateMemberBulkCount() {
        const countEl = document.getElementById('face-cluster-member-bulk-count');
        if (countEl) countEl.textContent = selectedMemberFaceIds.size;
        const ignoreBtn = document.getElementById('face-cluster-member-bulk-ignore-btn');
        if (ignoreBtn) ignoreBtn.disabled = selectedMemberFaceIds.size === 0;
    }

    function _selectAllMembers() {
        document.querySelectorAll('#face-cluster-member-grid .face-cluster-member-cell').forEach(cell => {
            const cb = cell.querySelector('.face-cluster-member-select-checkbox');
            if (!cb || cb.checked) return;
            cell.click();
        });
    }

    function _clearMemberSelection() {
        selectedMemberFaceIds.clear();
        document.querySelectorAll('.face-cluster-member-cell').forEach(cell => {
            cell.classList.remove('face-cluster-member-cell-selected');
            const cb = cell.querySelector('.face-cluster-member-select-checkbox');
            if (cb) cb.checked = false;
        });
        _updateMemberBulkCount();
    }

    async function _ignoreSelectedMembers() {
        if (selectedMemberFaceIds.size === 0) return;
        const faceIds = Array.from(selectedMemberFaceIds);
        const count = faceIds.length;

        const ok = await AppDialogs.showAppConfirm(
            'Ignore faces',
            `Ignore ${count} selected face(s) from this person? They will no longer be shown or re-grouped.`,
            { danger: true }
        );
        if (!ok) return;

        const btn = document.getElementById('face-cluster-member-bulk-ignore-btn');
        const originalHTML = btn ? btn.innerHTML : null;
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Ignoring…';
        }
        try {
            const results = await Promise.all(faceIds.map(faceId =>
                fetch('/api/faces/' + faceId, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ignored: true })
                }).then(resp => ({ faceId, ok: resp.ok }))
                  .catch(err => {
                      console.error('Error ignoring face', faceId, err);
                      return { faceId, ok: false };
                  })
            ));
            const failed = results.filter(r => !r.ok);
            if (failed.length > 0) {
                console.error('Failed to ignore faces:', failed.map(f => f.faceId));
                await AppDialogs.showAppAlert('Ignore faces', `${failed.length} of ${count} could not be ignored. Please try again.`);
            }
        } finally {
            if (btn) btn.innerHTML = originalHTML;
        }

        _exitMemberSelectMode();
        await _loadDetail();
        await _loadClusters();
    }

    // Opens the full source photo for a member face crop, reusing the same
    // detail/lightbox modal as the main Images gallery — which also draws
    // face bounding-box overlays for every detected face in the photo (see
    // modals-media.js's _loadFaceOverlay), not just the one that was clicked.
    async function _openFullImage(mediaItemId) {
        if (!mediaItemId) return;
        let metadata;
        try {
            const resp = await fetch('/images/' + mediaItemId + '/metadata');
            if (!resp.ok) throw new Error('Failed to load image metadata');
            metadata = await resp.json();
        } catch (err) {
            console.error('Error loading full image metadata:', err);
            return;
        }
        if (Modals.ImageDetailModal && typeof Modals.ImageDetailModal.open === 'function') {
            Modals.ImageDetailModal.open(metadata);
        }
    }

    async function _ensureContactNames() {
        if (allContactNames) return allContactNames;
        try {
            const resp = await fetch('/contacts/names');
            if (!resp.ok) throw new Error('Failed to load contacts');
            const data = await resp.json();
            allContactNames = data.contacts || [];
        } catch (err) {
            console.error('Error loading contact names:', err);
            allContactNames = [];
        }
        return allContactNames;
    }

    async function _onContactInput(evt) {
        const rawTerm = evt.target.value.trim();
        const term = rawTerm.toLowerCase();
        const resultsEl = document.getElementById('face-cluster-contact-results');
        if (!resultsEl) return;
        if (!term) {
            _hideContactResults();
            return;
        }
        const names = await _ensureContactNames();
        const matches = names.filter(c => c.name && c.name.toLowerCase().includes(term)).slice(0, 8);
        const exactMatch = names.some(c => c.name && c.name.toLowerCase() === term);

        resultsEl.innerHTML = '';
        matches.forEach(c => {
            const item = document.createElement('div');
            item.className = 'face-cluster-contact-result-item';
            item.textContent = c.name;
            item.addEventListener('click', () => _linkContact(c.id));
            resultsEl.appendChild(item);
        });

        // No contact already has exactly this name — offer to create one, so
        // an "Unnamed person" cluster can be named even when the archive
        // subject isn't in Contacts yet (e.g. a first-time appearance).
        if (!exactMatch) {
            const addItem = document.createElement('div');
            addItem.className = 'face-cluster-contact-result-item face-cluster-contact-result-item-new';
            const icon = document.createElement('i');
            icon.className = 'fas fa-plus';
            addItem.appendChild(icon);
            addItem.appendChild(document.createTextNode(' Add "' + rawTerm + '" as a new person'));
            addItem.addEventListener('click', () => _createAndLinkContact(rawTerm));
            resultsEl.appendChild(addItem);
        }

        resultsEl.style.display = 'block';
    }

    function _hideContactResults() {
        const resultsEl = document.getElementById('face-cluster-contact-results');
        if (resultsEl) {
            resultsEl.style.display = 'none';
            resultsEl.innerHTML = '';
        }
    }

    async function _linkContact(contactId) {
        if (!currentClusterId) return;
        try {
            const resp = await fetch('/api/faces/clusters/' + currentClusterId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ contact_id: contactId })
            });
            if (!resp.ok) {
                const errBody = await resp.json().catch(() => ({}));
                console.error('Failed to link cluster to contact:', errBody);
                alert('Could not update this person: ' + (errBody.detail || errBody.error || resp.statusText));
                return;
            }
        } catch (err) {
            console.error('Error linking cluster to contact:', err);
            return;
        }
        _hideContactResults();
        await _loadDetail();
        await _loadClusters();
    }

    // Creates a brand-new Contact (just a name) and links this cluster to it
    // in one step, for naming an "Unnamed person" who isn't in Contacts yet.
    // Rejected with a 409 if a contact with this name already exists (a race
    // with another tab, or the cached name list being stale) — the user is
    // told to search for and pick the existing one instead.
    async function _createAndLinkContact(name) {
        if (!currentClusterId) return;
        let contact;
        try {
            const resp = await fetch('/contacts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                console.error('Failed to create contact:', data);
                alert('Could not create new person: ' + (data.detail || data.error || resp.statusText));
                return;
            }
            contact = data;
        } catch (err) {
            console.error('Error creating contact:', err);
            return;
        }
        if (allContactNames) {
            allContactNames.push({ id: contact.id, name: contact.name });
        }
        await _linkContact(contact.id);
    }

    async function _detachFace(faceId) {
        try {
            const resp = await fetch('/api/faces/' + faceId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ face_cluster_id: null })
            });
            if (!resp.ok) {
                console.error('Failed to detach face', await resp.text());
                return;
            }
        } catch (err) {
            console.error('Error detaching face:', err);
            return;
        }
        await _loadDetail();
        await _loadClusters();
    }

    // Permanently excludes a face (a false detection, or someone the user
    // simply doesn't want tracked) so it is never shown or re-clustered
    // again — see PatchFace's {"ignored": true} handling. Callable both from
    // the top-level cluster grid (ignoring a cluster's representative face,
    // detail modal not necessarily open) and from the cluster detail's
    // member-face grid, so it only refreshes the detail view when it's
    // actually open.
    async function _ignoreFace(faceId) {
        try {
            const resp = await fetch('/api/faces/' + faceId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ignored: true })
            });
            if (!resp.ok) {
                console.error('Failed to ignore face', await resp.text());
                return;
            }
        } catch (err) {
            console.error('Error ignoring face:', err);
            return;
        }
        if (currentClusterId) await _loadDetail();
        await _loadClusters();
    }

    // -------------------------------------------------------------------------
    // Init
    // -------------------------------------------------------------------------

    function init() {
        const closeBtn = document.getElementById('close-faces-modal');
        if (closeBtn) closeBtn.addEventListener('click', close);

        const modal = document.getElementById('faces-modal');
        if (modal) {
            modal.addEventListener('click', (evt) => {
                if (evt.target === modal) close();
            });
        }

        document.querySelectorAll('.faces-filter-tab').forEach(btn => {
            btn.addEventListener('click', () => _setFilter(btn.dataset.filter));
        });

        const minGroupSizeInput = document.getElementById('faces-min-group-size');
        if (minGroupSizeInput) minGroupSizeInput.addEventListener('input', _onMinGroupSizeInput);

        const selectModeToggle = document.getElementById('faces-select-mode-toggle');
        if (selectModeToggle) selectModeToggle.addEventListener('click', _toggleSelectMode);

        const selectAllBtn = document.getElementById('faces-bulk-select-all-btn');
        if (selectAllBtn) selectAllBtn.addEventListener('click', _selectAllOnPage);

        const clearSelectionBtn = document.getElementById('faces-bulk-clear-btn');
        if (clearSelectionBtn) clearSelectionBtn.addEventListener('click', _clearSelection);

        const bulkIgnoreBtn = document.getElementById('faces-bulk-ignore-btn');
        if (bulkIgnoreBtn) bulkIgnoreBtn.addEventListener('click', () => { void _ignoreSelected(); });

        const prevBtn = document.getElementById('faces-pagination-prev');
        if (prevBtn) prevBtn.addEventListener('click', _prevPage);

        const nextBtn = document.getElementById('faces-pagination-next');
        if (nextBtn) nextBtn.addEventListener('click', _nextPage);

        const closeDetailBtn = document.getElementById('close-face-cluster-detail');
        if (closeDetailBtn) closeDetailBtn.addEventListener('click', _closeDetail);

        const detailModal = document.getElementById('face-cluster-detail-modal');
        if (detailModal) {
            detailModal.addEventListener('click', (evt) => {
                if (evt.target === detailModal) _closeDetail();
            });
        }

        const memberSelectModeToggle = document.getElementById('face-cluster-member-select-mode-toggle');
        if (memberSelectModeToggle) memberSelectModeToggle.addEventListener('click', _toggleMemberSelectMode);

        const memberSelectAllBtn = document.getElementById('face-cluster-member-bulk-select-all-btn');
        if (memberSelectAllBtn) memberSelectAllBtn.addEventListener('click', _selectAllMembers);

        const memberClearSelectionBtn = document.getElementById('face-cluster-member-bulk-clear-btn');
        if (memberClearSelectionBtn) memberClearSelectionBtn.addEventListener('click', _clearMemberSelection);

        const memberBulkIgnoreBtn = document.getElementById('face-cluster-member-bulk-ignore-btn');
        if (memberBulkIgnoreBtn) memberBulkIgnoreBtn.addEventListener('click', () => { void _ignoreSelectedMembers(); });

        const contactInput = document.getElementById('face-cluster-contact-input');
        if (contactInput) {
            contactInput.addEventListener('input', _onContactInput);
            contactInput.addEventListener('focus', _onContactInput);
        }
        document.addEventListener('click', (evt) => {
            const picker = document.querySelector('.face-cluster-contact-picker');
            if (picker && !picker.contains(evt.target)) _hideContactResults();
        });
    }

    return { open, close, init };
})();
