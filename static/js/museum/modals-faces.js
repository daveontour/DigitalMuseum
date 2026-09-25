/**
 * People in Photos (Face Recognition) Modal
 * Browse face clusters detected by the "Detect faces in photos" background job,
 * link a cluster to a Contact to name it, review member photos, detach a face
 * that was grouped wrong ("not this person"), ignore a single bad detection,
 * or ignore an entire person (cluster) so they stop being tracked in every
 * photo they appear in.
 */

Modals.Faces = (() => {
    let currentFilter = 'all';       // 'all' | 'named' | 'unnamed' | 'suggested'
    let clustersRequestToken = 0;    // bumped per _loadClusters call; stale responses are dropped
    let minGroupSize = 1;            // 1 = no filter; >1 = only clusters with at least this many faces
    let maxGroupSize = 0;            // 0 = no filter; >0 = only clusters with at most this many faces
    let contactNameFilter = '';      // '' = no filter; only applies (and only shown) when currentFilter === 'named'
    let currentClusterId = null;

    let pageSize = 50;                // user-selectable via the "Per page" control; capped at maxFaceClustersPageSize (200) server-side
    let currentOffset = 0;
    let currentTotal = 0;

    let selectMode = false;
    let selectedClusterIds = new Set();  // cluster id set, persists across pages/filters

    let memberSelectMode = false;
    let selectedMemberFaceIds = new Set();  // face id set, scoped to the currently open cluster detail
    let currentClusterFaceCount = 0;  // face_count of the currently open detail cluster, for the ignore confirm prompt

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
        if (currentFilter === 'suggested') params.set('suggested', 'true');
        if (minGroupSize > 1) params.set('min_face_count', String(minGroupSize));
        if (maxGroupSize > 0) params.set('max_face_count', String(maxGroupSize));
        if (currentFilter === 'named' && contactNameFilter) params.set('contact_name', contactNameFilter);
        params.set('limit', String(pageSize));
        params.set('offset', String(currentOffset));

        // Clear immediately so the previous filter's cards are never mistaken
        // for this one's results while the request is in flight, and drop
        // any response that arrives after a newer request was started.
        const thisRequest = ++clustersRequestToken;
        grid.innerHTML = '<div class="faces-grid-loading"><i class="fas fa-spinner fa-spin"></i> Loading…</div>';
        if (empty) empty.style.display = 'none';

        let clusters = [];
        try {
            const resp = await fetch('/api/faces/clusters?' + params.toString());
            if (!resp.ok) throw new Error('Failed to load face clusters');
            const data = await resp.json();
            if (thisRequest !== clustersRequestToken) return;
            clusters = data.clusters || [];
            currentTotal = data.total || 0;
        } catch (err) {
            if (thisRequest !== clustersRequestToken) return;
            console.error('Error loading face clusters:', err);
            currentTotal = 0;
        }

        grid.innerHTML = '';
        _renderPagination();
        if (clusters.length === 0) {
            const emptyText = document.getElementById('faces-empty-state-text');
            if (emptyText) {
                emptyText.textContent = currentFilter === 'suggested'
                    ? 'No possible matches yet. They are found by the "Find possible matches for unnamed people" job in Configuration → Background Jobs, which runs hourly — run it now after naming more people.'
                    : emptyText.dataset.defaultText || emptyText.textContent;
            }
            if (empty) empty.style.display = 'flex';
            return;
        }
        if (empty) empty.style.display = 'none';

        clusters.forEach(cluster => {
            const card = document.createElement('div');
            card.className = 'face-cluster-card';
            if (selectedClusterIds.has(cluster.id)) {
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
                checkbox.checked = selectedClusterIds.has(cluster.id);
                selectWrap.appendChild(checkbox);
                selectWrap.addEventListener('click', (evt) => {
                    evt.preventDefault();
                    evt.stopPropagation();
                    _toggleCardSelection(card, cluster);
                });
                thumbWrap.appendChild(selectWrap);
            }

            const ignoreBtn = document.createElement('button');
            ignoreBtn.type = 'button';
            ignoreBtn.className = 'face-cluster-card-ignore-btn';
            ignoreBtn.title = 'Ignore this person (excludes them from every photo)';
            ignoreBtn.innerHTML = '<i class="fas fa-times"></i>';
            ignoreBtn.addEventListener('click', (evt) => {
                evt.stopPropagation();
                void _confirmAndIgnoreCluster(cluster.id, cluster.face_count);
            });
            thumbWrap.appendChild(ignoreBtn);

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

            // An unnamed cluster's stored "possible match" (written by the
            // "Find possible matches" background job) — shown in every tab,
            // not just Possible Matches, so the guess is visible wherever the
            // cluster appears.
            if (Array.isArray(cluster.suggestions) && cluster.suggestions.length > 0) {
                const guess = document.createElement('div');
                guess.className = 'face-cluster-card-suggestion';
                const icon = document.createElement('i');
                icon.className = 'fas fa-lightbulb';
                guess.appendChild(icon);
                guess.appendChild(document.createTextNode(' Possibly: ' + (cluster.suggestions[0].contact_name || 'Unknown')));
                body.appendChild(guess);
            }

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
        selectedClusterIds.clear();
        _updateBulkCount();
        document.querySelectorAll('.faces-filter-tab').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.filter === filter);
        });

        const contactNameControl = document.getElementById('faces-contact-name-control');
        if (contactNameControl) contactNameControl.style.display = filter === 'named' ? 'flex' : 'none';
        if (filter !== 'named' && contactNameFilter) {
            contactNameFilter = '';
            const input = document.getElementById('faces-contact-name-filter');
            if (input) input.value = '';
        }

        _loadClusters();
    }

    let contactNameFilterDebounceTimer = null;

    function _onContactNameFilterInput(evt) {
        if (contactNameFilterDebounceTimer) clearTimeout(contactNameFilterDebounceTimer);
        contactNameFilterDebounceTimer = setTimeout(() => {
            contactNameFilter = evt.target.value.trim();
            currentOffset = 0;
            selectedClusterIds.clear();
            _updateBulkCount();
            _loadClusters();
        }, 400);
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
            selectedClusterIds.clear();
            _updateBulkCount();
            _loadClusters();
        }, 400);
    }

    let maxGroupSizeDebounceTimer = null;

    function _onMaxGroupSizeInput(evt) {
        if (maxGroupSizeDebounceTimer) clearTimeout(maxGroupSizeDebounceTimer);
        maxGroupSizeDebounceTimer = setTimeout(() => {
            const raw = evt.target.value.trim();
            let n = parseInt(raw, 10);
            if (!raw || isNaN(n) || n < 1) n = 0; // 0 = no upper bound ("Any")
            maxGroupSize = n;
            currentOffset = 0;
            selectedClusterIds.clear();
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
            selectedClusterIds.clear();
            _updateSelectModeUI();
        }
    }

    function _exitSelectMode() {
        selectMode = false;
        selectedClusterIds.clear();
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
        const willSelect = !selectedClusterIds.has(cluster.id);
        if (willSelect) {
            selectedClusterIds.add(cluster.id);
        } else {
            selectedClusterIds.delete(cluster.id);
        }
        card.classList.toggle('face-cluster-card-selected', willSelect);
        const cb = card.querySelector('.face-cluster-card-select-checkbox');
        if (cb) cb.checked = willSelect;
        _updateBulkCount();
    }

    function _updateBulkCount() {
        const countEl = document.getElementById('faces-bulk-count');
        if (countEl) countEl.textContent = selectedClusterIds.size;
        const ignoreBtn = document.getElementById('faces-bulk-ignore-btn');
        if (ignoreBtn) ignoreBtn.disabled = selectedClusterIds.size === 0;
    }

    function _selectAllOnPage() {
        document.querySelectorAll('#faces-cluster-grid .face-cluster-card').forEach(card => {
            const cb = card.querySelector('.face-cluster-card-select-checkbox');
            if (!cb || cb.checked) return;
            card.click();
        });
    }

    function _clearSelection() {
        selectedClusterIds.clear();
        document.querySelectorAll('.face-cluster-card').forEach(card => {
            card.classList.remove('face-cluster-card-selected');
            const cb = card.querySelector('.face-cluster-card-select-checkbox');
            if (cb) cb.checked = false;
        });
        _updateBulkCount();
    }

    // Ignores every selected person's whole cluster in one go — each
    // selected card represents a person, not a single detection, so this
    // excludes them from every photo they appear in, not just their card's
    // representative face. See _ignoreCluster / PatchCluster's
    // {"ignored": true} handling.
    async function _ignoreSelected() {
        if (selectedClusterIds.size === 0) return;
        const clusterIds = Array.from(selectedClusterIds);
        const count = clusterIds.length;

        const ok = await AppDialogs.showAppConfirm(
            'Ignore people',
            `Ignore ${count} selected ${count === 1 ? 'person' : 'people'}? They will no longer be tracked in any of their photos.`,
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
            const results = await Promise.all(clusterIds.map(clusterId =>
                fetch('/api/faces/clusters/' + clusterId, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ignored: true })
                }).then(resp => ({ clusterId, ok: resp.ok }))
                  .catch(err => {
                      console.error('Error ignoring cluster', clusterId, err);
                      return { clusterId, ok: false };
                  })
            ));
            const failed = results.filter(r => !r.ok);
            if (failed.length > 0) {
                console.error('Failed to ignore clusters:', failed.map(f => f.clusterId));
                await AppDialogs.showAppAlert('Ignore people', `${failed.length} of ${count} could not be ignored. Please try again.`);
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

        if (currentTotal <= pageSize) {
            bar.style.display = 'none';
            return;
        }
        bar.style.display = 'flex';

        const pageStart = currentTotal === 0 ? 0 : currentOffset + 1;
        const pageEnd = Math.min(currentOffset + pageSize, currentTotal);

        const info = document.getElementById('faces-pagination-info');
        if (info) info.textContent = pageStart + '–' + pageEnd + ' of ' + currentTotal;

        const prevBtn = document.getElementById('faces-pagination-prev');
        if (prevBtn) prevBtn.disabled = currentOffset <= 0;

        const nextBtn = document.getElementById('faces-pagination-next');
        if (nextBtn) nextBtn.disabled = currentOffset + pageSize >= currentTotal;
    }

    function _prevPage() {
        if (currentOffset <= 0) return;
        currentOffset = Math.max(0, currentOffset - pageSize);
        const grid = document.getElementById('faces-cluster-grid');
        if (grid) grid.scrollTop = 0;
        _loadClusters();
    }

    function _nextPage() {
        if (currentOffset + pageSize >= currentTotal) return;
        currentOffset += pageSize;
        const grid = document.getElementById('faces-cluster-grid');
        if (grid) grid.scrollTop = 0;
        _loadClusters();
    }

    function _onPageSizeChange(evt) {
        const n = parseInt(evt.target.value, 10);
        if (isNaN(n) || n < 1) return;
        pageSize = n;
        currentOffset = 0;
        selectedClusterIds.clear();
        _updateBulkCount();
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
        _resetDetailView();
    }

    // Clears every piece of the previous cluster's data out of the detail
    // dialog so, if it's reopened before the next fetch resolves (or that
    // fetch fails), it shows an empty state rather than a stale flash of
    // whichever cluster was last viewed.
    function _resetDetailView() {
        currentClusterFaceCount = 0;

        const titleEl = document.getElementById('face-cluster-detail-title-text');
        if (titleEl) titleEl.textContent = '';

        const input = document.getElementById('face-cluster-contact-input');
        if (input) input.value = '';

        const unlinkBtn = document.getElementById('face-cluster-unlink-btn');
        if (unlinkBtn) {
            unlinkBtn.style.display = 'none';
            unlinkBtn.onclick = null;
        }

        const suggestBox = document.getElementById('face-cluster-suggestions');
        if (suggestBox) suggestBox.innerHTML = '';

        const grid = document.getElementById('face-cluster-member-grid');
        if (grid) grid.innerHTML = '';
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
        currentClusterFaceCount = data.face_count || 0;

        const titleEl = document.getElementById('face-cluster-detail-title-text');
        if (titleEl) titleEl.textContent = data.contact_name || 'Unnamed person';

        const input = document.getElementById('face-cluster-contact-input');
        if (input) input.value = data.contact_name || '';

        const unlinkBtn = document.getElementById('face-cluster-unlink-btn');
        if (unlinkBtn) {
            // contact_id 0 is the archive subject's reserved sentinel id, not
            // "unset" — must not use a falsy check here.
            unlinkBtn.style.display = (data.contact_id !== null && data.contact_id !== undefined) ? 'inline-flex' : 'none';
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

    // Fetched fresh on every call rather than cached — a contact can be
    // created at any time via the Contacts & Relationships dialog (or the
    // "Add ... as a new person" flow below) while this dialog stays open, and
    // a stale session-long cache would hide it from the "Link to contact"
    // search indefinitely (the exact bug this used to have).
    async function _fetchContactNames() {
        try {
            const resp = await fetch('/contacts/names');
            if (!resp.ok) throw new Error('Failed to load contacts');
            const data = await resp.json();
            return data.contacts || [];
        } catch (err) {
            console.error('Error loading contact names:', err);
            return [];
        }
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
        const names = await _fetchContactNames();
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
    // with another tab) — the user is told to search for and pick the
    // existing one instead.
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

    // Ignores every face belonging to a cluster in one action ("ignore this
    // person") — unlike _ignoreFace, this excludes that person from every
    // photo they were grouped into, not just the one instance clicked. See
    // PatchCluster's {"ignored": true} handling. Callable both from the
    // top-level cluster grid card and from inside the open detail modal
    // (closed afterward, since the cluster it was showing no longer has any
    // faces left to display).
    async function _ignoreCluster(clusterId) {
        try {
            const resp = await fetch('/api/faces/clusters/' + clusterId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ignored: true })
            });
            if (!resp.ok) {
                console.error('Failed to ignore cluster', await resp.text());
                return;
            }
        } catch (err) {
            console.error('Error ignoring cluster:', err);
            return;
        }
        if (currentClusterId === clusterId) {
            _closeDetail();
        }
        await _loadClusters();
    }

    async function _confirmAndIgnoreCluster(clusterId, faceCount) {
        const count = faceCount || 1;
        const ok = await AppDialogs.showAppConfirm(
            'Ignore this person',
            `Ignore this person? They will no longer be tracked in any of their ${count} photo(s).`,
            { danger: true }
        );
        if (!ok) return;
        await _ignoreCluster(clusterId);
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

        const maxGroupSizeInput = document.getElementById('faces-max-group-size');
        if (maxGroupSizeInput) maxGroupSizeInput.addEventListener('input', _onMaxGroupSizeInput);

        const contactNameFilterInput = document.getElementById('faces-contact-name-filter');
        if (contactNameFilterInput) contactNameFilterInput.addEventListener('input', _onContactNameFilterInput);

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

        const pageSizeSelect = document.getElementById('faces-page-size');
        if (pageSizeSelect) {
            pageSizeSelect.value = String(pageSize);
            pageSizeSelect.addEventListener('change', _onPageSizeChange);
        }

        const closeDetailBtn = document.getElementById('close-face-cluster-detail');
        if (closeDetailBtn) closeDetailBtn.addEventListener('click', _closeDetail);

        const ignorePersonBtn = document.getElementById('face-cluster-ignore-person-btn');
        if (ignorePersonBtn) {
            ignorePersonBtn.addEventListener('click', () => {
                if (!currentClusterId) return;
                void _confirmAndIgnoreCluster(currentClusterId, currentClusterFaceCount);
            });
        }

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
