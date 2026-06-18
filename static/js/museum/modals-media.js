'use strict';

Modals.FBAlbums = (() => {
        let albums = [];
        let filteredAlbums = [];
        let selectedAlbumId = null;
        let resizeStartX = null;
        let resizeStartWidth = null;

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
                const seconds = String(date.getSeconds()).padStart(2, '0');
                
                return `${day}/${month}/${year} ${hours}:${minutes}:${seconds}`;
            } catch (error) {
                return 'Invalid Date';
            }
        }

        async function loadAlbums() {
            if (!DOM.fbAlbumsList) return;
            
            DOM.fbAlbumsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">Loading albums...</div>';
            
            try {
                const response = await fetch('/facebook/albums');
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                albums = await response.json();
                filteredAlbums = albums;
                renderAlbums();
            } catch (error) {
                console.error("Failed to load FB albums:", error);
                DOM.fbAlbumsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #dc3545;">Failed to load albums: ' + error.message + '</div>';
            }
        }

        function renderAlbums() {
            if (!DOM.fbAlbumsList) return;
            
            if (filteredAlbums.length === 0) {
                DOM.fbAlbumsList.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666;">No albums found</div>';
                return;
            }
            
            DOM.fbAlbumsList.innerHTML = '';
            
            filteredAlbums.forEach(album => {
                const albumItem = document.createElement('div');
                albumItem.className = 'fb-album-item' + (selectedAlbumId === album.id ? ' selected' : '');
                albumItem.onclick = () => selectAlbum(album.id);

                const title = document.createElement('div');
                title.className = 'fb-album-item-title';
                title.textContent = album.name;

                const meta = document.createElement('div');
                meta.className = 'fb-album-item-meta';
                meta.textContent = `${album.image_count || 0} image${album.image_count !== 1 ? 's' : ''}`;

                albumItem.appendChild(title);
                albumItem.appendChild(meta);
                DOM.fbAlbumsList.appendChild(albumItem);
            });
        }

        async function selectAlbum(albumId) {
            selectedAlbumId = albumId;
            renderAlbums();
            
            const album = albums.find(a => a.id === albumId);
            if (!album) return;
            
            // Update header
            if (DOM.fbAlbumsAlbumTitle) {
                DOM.fbAlbumsAlbumTitle.textContent = album.name;
            }
            if (DOM.fbAlbumsAlbumDescription) {
                DOM.fbAlbumsAlbumDescription.textContent = album.description || '';
            }
            
            // Load images
            if (!DOM.fbAlbumsImagesContainer) return;
            
            DOM.fbAlbumsImagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666; grid-column: 1 / -1;">Loading images...</div>';
            
            try {
                const response = await fetch(`/facebook/albums/${albumId}/images`);
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const images = await response.json();
                
                if (images.length === 0) {
                    DOM.fbAlbumsImagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666; grid-column: 1 / -1;">No images in this album</div>';
                    return;
                }
                
                DOM.fbAlbumsImagesContainer.innerHTML = '';
                
                images.forEach(image => {
                    const imageCard = document.createElement('div');
                    imageCard.style.cssText = 'position: relative; border-radius: 8px; overflow: hidden; background: #f8f9fa; cursor: pointer; transition: transform 0.2s, box-shadow 0.2s; min-height: 200px;';
                    imageCard.onmouseover = () => {
                        imageCard.style.transform = 'scale(1.02)';
                        imageCard.style.boxShadow = '0 4px 12px rgba(0,0,0,0.15)';
                    };
                    imageCard.onmouseout = () => {
                        imageCard.style.transform = 'scale(1)';
                        imageCard.style.boxShadow = 'none';
                    };
                    
                    const img = document.createElement('img');
                    img.src = `/facebook/albums/images/${image.id}`;
                    img.alt = image.title || image.filename || 'Album image';
                    img.style.cssText = 'width: 100%; height: 200px; min-height: 200px; object-fit: cover; display: block;';
                    img.loading = 'lazy';
                    
                    imageCard.onclick = () => {
                        // Open full image in modal
                        if (DOM.singleImageModal && DOM.singleImageModalImg) {
                            // Hide other media elements
                            if (DOM.singleImageModalAudio) DOM.singleImageModalAudio.style.display = 'none';
                            if (DOM.singleImageModalVideo) DOM.singleImageModalVideo.style.display = 'none';
                            if (DOM.singleImageModalPdf) DOM.singleImageModalPdf.style.display = 'none';
                            
                            // Show and set image
                            DOM.singleImageModalImg.src = img.src;
                            DOM.singleImageModalImg.alt = img.alt;
                            DOM.singleImageModalImg.style.display = 'block';
                            
                            // Set image details if available
                            if (DOM.singleImageDetails) {
                                const details = [];
                                if (image.title) details.push(`<strong>Title:</strong> ${image.title}`);
                                if (image.description) details.push(`<strong>Description:</strong> ${image.description}`);
                                if (image.filename) details.push(`<strong>Filename:</strong> ${image.filename}`);
                                if (image.creation_timestamp) details.push(`<strong>Date:</strong> ${formatDateAustralian(image.creation_timestamp)}`);
                                DOM.singleImageDetails.innerHTML = details.length > 0 ? details.join('<br>') : '';
                            }
                            
                            // Open modal
                            Modals._openModal(DOM.singleImageModal);
                        }
                    };
                    
                    // Show overlay with image description if available, otherwise album title
                    // Priority: image.description > image.title > album.name
                    const overlayText = image.description || image.title || album.name;
                    const overlay = document.createElement('div');
                    overlay.style.cssText = 'position: absolute; bottom: 0; left: 0; right: 0; background: linear-gradient(to top, rgba(0,0,0,0.7), transparent); padding: 8px; color: white; font-size: 12px;';
                    overlay.textContent = overlayText;
                    imageCard.appendChild(overlay);
                    
                    imageCard.appendChild(img);
                    DOM.fbAlbumsImagesContainer.appendChild(imageCard);
                });
            } catch (error) {
                console.error("Failed to load album images:", error);
                DOM.fbAlbumsImagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #dc3545; grid-column: 1 / -1;">Failed to load images: ' + error.message + '</div>';
            }
        }

        function searchAlbums(query) {
            const searchTerm = query.toLowerCase().trim();
            if (!searchTerm) {
                filteredAlbums = albums;
            } else {
                filteredAlbums = albums.filter(album => 
                    album.name.toLowerCase().includes(searchTerm) ||
                    (album.description && album.description.toLowerCase().includes(searchTerm))
                );
            }
            renderAlbums();
        }

        function startResize(e) {
            e.preventDefault();
            resizeStartX = e.clientX;
            resizeStartWidth = DOM.fbAlbumsMasterPane.offsetWidth;
            document.addEventListener('mousemove', handleResize);
            document.addEventListener('mouseup', stopResize);
            if (DOM.fbAlbumsResizeHandle) {
                DOM.fbAlbumsResizeHandle.style.backgroundColor = '#2196F3';
            }
        }

        function handleResize(e) {
            if (resizeStartX === null || resizeStartWidth === null) return;
            
            const diff = e.clientX - resizeStartX;
            const newWidth = resizeStartWidth + diff;
            const minWidth = 200;
            const maxWidth = 600;
            
            if (newWidth >= minWidth && newWidth <= maxWidth) {
                DOM.fbAlbumsMasterPane.style.width = newWidth + 'px';
            }
        }

        function stopResize() {
            resizeStartX = null;
            resizeStartWidth = null;
            document.removeEventListener('mousemove', handleResize);
            document.removeEventListener('mouseup', stopResize);
            if (DOM.fbAlbumsResizeHandle) {
                DOM.fbAlbumsResizeHandle.style.backgroundColor = 'transparent';
            }
        }

        async function open() {
            Modals._openModal(DOM.fbAlbumsModal);
            selectedAlbumId = null;
            await loadAlbums();

            if (filteredAlbums.length > 0) {
                await selectAlbum(filteredAlbums[0].id);
            } else {
                if (DOM.fbAlbumsAlbumTitle) {
                    DOM.fbAlbumsAlbumTitle.textContent = 'Select an album';
                }
                if (DOM.fbAlbumsAlbumDescription) {
                    DOM.fbAlbumsAlbumDescription.textContent = '';
                }
                if (DOM.fbAlbumsImagesContainer) {
                    DOM.fbAlbumsImagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666; grid-column: 1 / -1;">Select an album to view images</div>';
                }
            }
        }

        async function openAndSelectAlbum(albumId) {
            // Open the Facebook Albums modal
            Modals._openModal(DOM.fbAlbumsModal);
            await loadAlbums();
            await selectAlbum(albumId);
            

        }


        function close() { 
            Modals._closeModal(DOM.fbAlbumsModal);
        }

        function init() {
            if (DOM.closeFBAlbumsModalBtn) {
                DOM.closeFBAlbumsModalBtn.addEventListener('click', close);
            }
            if (DOM.fbAlbumsModal) {
                DOM.fbAlbumsModal.addEventListener('click', (e) => {
                    if (e.target === DOM.fbAlbumsModal) close();
                });
            }
            if (DOM.fbAlbumsSearch) {
                DOM.fbAlbumsSearch.addEventListener('input', (e) => {
                    searchAlbums(e.target.value);
                });
            }
            if (DOM.fbAlbumsResizeHandle) {
                DOM.fbAlbumsResizeHandle.addEventListener('mousedown', startResize);
            }
        }

        async function openAndSelectAlbum(albumId) {
            // Open the Facebook Albums modal
            Modals._openModal(DOM.fbAlbumsModal);
            
            // Load albums and wait for them to load
            try {
                await loadAlbums();
                
                // Find album by ID (handle both string and number comparisons)
                const albumIdNum = typeof albumId === 'string' ? parseInt(albumId) : albumId;
                const album = albums.find(a => {
                    const aId = typeof a.id === 'string' ? parseInt(a.id) : a.id;
                    return aId === albumIdNum || a.id === albumId || a.id === albumIdNum;
                });
                
                if (album) {
                    // Select the album (this will also load its images)
                    // Use the album's actual ID to ensure type matching
                    await selectAlbum(album.id);
                } else {
                    console.warn(`Album with ID ${albumId} not found`);
                    // Reset slave pane if album not found
                    if (DOM.fbAlbumsAlbumTitle) {
                        DOM.fbAlbumsAlbumTitle.textContent = 'Select an album';
                    }
                    if (DOM.fbAlbumsAlbumDescription) {
                        DOM.fbAlbumsAlbumDescription.textContent = '';
                    }
                    if (DOM.fbAlbumsImagesContainer) {
                        DOM.fbAlbumsImagesContainer.innerHTML = '<div style="text-align: center; padding: 2rem; color: #666; grid-column: 1 / -1;">Album not found</div>';
                    }
                }
            } catch (error) {
                console.error('Error loading albums:', error);
            }
        }

        return {
            open,
            close,
            init,
            startResize,
            openAndSelectAlbum
        };
})();



Modals.FBPosts = (() => {
    let posts = [];
    let filteredPosts = [];
    let selectedPostId = null;
    let resizeStartX = 0;
    let resizeStartWidth = 0;

    function formatDate(isoStr) {
        if (!isoStr) return '';
        const d = new Date(isoStr);
        const day = String(d.getDate()).padStart(2, '0');
        const mon = String(d.getMonth() + 1).padStart(2, '0');
        const yr = d.getFullYear();
        const hr = String(d.getHours()).padStart(2, '0');
        const min = String(d.getMinutes()).padStart(2, '0');
        return `${day}/${mon}/${yr} ${hr}:${min}`;
    }

    function postTypeIcon(post) {
        if (post.post_type === 'photo' || post.post_type === 'mixed') return ' 📷';
        if (post.post_type === 'link') return ' 🔗';
        return '';
    }

    async function loadPosts(search = '', postIds = null) {
        const container = DOM.fbPostsList;
        if (!container) return;
        container.innerHTML = '<div style="text-align:center;padding:2rem;color:#666;">Loading...</div>';
        try {
            let url = `/facebook/posts?page=1&page_size=200`;
            if (search) url += '&search=' + encodeURIComponent(search);
            if (postIds && postIds.length > 0) url += '&post_ids=' + postIds.join(',');
            const resp = await fetch(url);
            const data = await resp.json();
            posts = data.posts || [];
            filteredPosts = posts;
            renderPosts(filteredPosts);
            if (filteredPosts.length > 0 && !selectedPostId) {
                selectPost(filteredPosts[0].id);
            }
        } catch (err) {
            container.innerHTML = `<div style="color:#dc3545;padding:1rem;">Error loading posts: ${err.message}</div>`;
        }
    }

    function renderPosts(list) {
        const container = DOM.fbPostsList;
        if (!container) return;
        if (list.length === 0) {
            container.innerHTML = '<div style="text-align:center;padding:2rem;color:#666;">No posts found</div>';
            return;
        }
        container.innerHTML = list.map(p => {
            const preview = (p.post_text || p.title || '(no text)').substring(0, 80) + ((p.post_text || '').length > 80 ? '…' : '');
            const icon = postTypeIcon(p);
            const isSelected = p.id === selectedPostId;
            return `<div class="fb-post-list-item${isSelected ? ' selected' : ''}" data-post-id="${p.id}">
                <div class="fb-post-list-item-date">${formatDate(p.timestamp)}${icon}</div>
                <div class="fb-post-list-item-preview">${preview}</div>
            </div>`;
        }).join('');

        container.querySelectorAll('.fb-post-list-item').forEach(el => {
            el.addEventListener('click', () => selectPost(parseInt(el.dataset.postId)));
        });
    }

    async function selectPost(postId) {
        selectedPostId = postId;
        renderPosts(filteredPosts);

        const post = filteredPosts.find(p => p.id === postId);
        if (!post) return;

        if (DOM.fbPostsPostDate) DOM.fbPostsPostDate.textContent = formatDate(post.timestamp) + (post.title ? `  —  ${post.title}` : '');
        if (DOM.fbPostsPostText) DOM.fbPostsPostText.textContent = post.post_text || '';
        if (DOM.fbPostsPostLink) {
            if (post.external_url) {
                DOM.fbPostsPostLink.href = post.external_url;
                DOM.fbPostsPostLink.textContent = post.external_url;
                DOM.fbPostsPostLink.style.display = 'block';
            } else {
                DOM.fbPostsPostLink.style.display = 'none';
            }
        }

        const imgContainer = DOM.fbPostsImagesContainer;
        if (!imgContainer) return;

        if (post.media_count === 0) {
            imgContainer.innerHTML = '<div style="text-align:center;padding:2rem;color:#888;grid-column:1/-1;">No media attached</div>';
            return;
        }

        imgContainer.innerHTML = '<div style="text-align:center;padding:2rem;color:#666;grid-column:1/-1;">Loading media...</div>';
        try {
            const resp = await fetch(`/facebook/posts/${postId}/media`);
            let body;
            try {
                body = await resp.json();
            } catch {
                body = null;
            }
            // Error responses use { detail: "..." }; (body || []) is still an object, so .map would throw.
            if (!resp.ok) {
                const msg = (body && typeof body.detail === 'string' && body.detail) || resp.statusText || 'Failed to load media';
                imgContainer.innerHTML = `<div style="color:#dc3545;padding:1rem;grid-column:1/-1;">${msg.replace(/</g, '&lt;')}</div>`;
                return;
            }
            const mediaItems = Array.isArray(body) ? body : [];
            if (mediaItems.length === 0) {
                imgContainer.innerHTML = '<div style="text-align:center;padding:2rem;color:#888;grid-column:1/-1;">No media found</div>';
                return;
            }
            imgContainer.innerHTML = mediaItems.map(mi => `
                <div style="position:relative;overflow:hidden;border-radius:8px;background:#eee;aspect-ratio:1;cursor:pointer;" onclick="Modals.SingleImageDisplay && Modals.SingleImageDisplay.showSingleImageModal('${(mi.title||'').replace(/'/g,"\\'")}','/facebook/posts/media/${mi.id}',0,0,0)">
                    <img src="/facebook/posts/media/${mi.id}" loading="lazy" alt="${mi.title || ''}"
                        style="width:100%;height:100%;object-fit:cover;transition:transform 0.2s;"
                        onmouseover="this.style.transform='scale(1.04)'" onmouseout="this.style.transform='scale(1)'">
                    ${mi.title ? `<div style="position:absolute;bottom:0;left:0;right:0;background:linear-gradient(transparent,rgba(0,0,0,0.6));color:#fff;font-size:0.75em;padding:0.4rem 0.5rem;">${mi.title}</div>` : ''}
                </div>`).join('');
        } catch (err) {
            imgContainer.innerHTML = `<div style="color:#dc3545;padding:1rem;grid-column:1/-1;">Error loading media: ${err.message}</div>`;
        }
    }

    function searchPosts(query) {
        const q = query.toLowerCase().trim();
        filteredPosts = q ? posts.filter(p =>
            (p.post_text || '').toLowerCase().includes(q) ||
            (p.title || '').toLowerCase().includes(q)
        ) : posts;
        renderPosts(filteredPosts);
    }

    function startResize(e) {
        resizeStartX = e.clientX;
        resizeStartWidth = DOM.fbPostsMasterPane ? DOM.fbPostsMasterPane.offsetWidth : 350;
        document.addEventListener('mousemove', handleResize);
        document.addEventListener('mouseup', stopResize);
        e.preventDefault();
    }

    function handleResize(e) {
        if (!DOM.fbPostsMasterPane) return;
        const newWidth = Math.max(200, Math.min(600, resizeStartWidth + (e.clientX - resizeStartX)));
        DOM.fbPostsMasterPane.style.width = newWidth + 'px';
    }

    function stopResize() {
        document.removeEventListener('mousemove', handleResize);
        document.removeEventListener('mouseup', stopResize);
    }

    function open() {
        if (DOM.fbPostsModal) {
            DOM.fbPostsModal.style.display = 'flex';
            loadPosts();
        }
    }
    async function openAndFilterOnPosts(postIds) {
        if (!DOM.fbPostsModal || !postIds || postIds.length === 0) return;
        const ids = Array.isArray(postIds)
            ? postIds.map(id => Number(id))
            : String(postIds).split(',').map(id => parseInt(id, 10));
        if (ids.length === 0) return;
        DOM.fbPostsModal.style.display = 'flex';
        selectedPostId = null;
        await loadPosts('', ids);
        if (filteredPosts.length > 0) {
            selectPost(filteredPosts[0].id);
        }
    }

    function close() {
        if (DOM.fbPostsModal) DOM.fbPostsModal.style.display = 'none';
    }

    function init() {
        if (DOM.closeFBPostsModalBtn) DOM.closeFBPostsModalBtn.addEventListener('click', close);
        if (DOM.fbPostsModal) DOM.fbPostsModal.addEventListener('click', e => { if (e.target === DOM.fbPostsModal) close(); });
        if (DOM.fbPostsSearch) DOM.fbPostsSearch.addEventListener('input', e => searchPosts(e.target.value));
        if (DOM.fbPostsClearBtn) DOM.fbPostsClearBtn.addEventListener('click', async () => {
            if (DOM.fbPostsSearch) DOM.fbPostsSearch.value = '';
            selectedPostId = null;
            await loadPosts();
            if (filteredPosts.length > 0) selectPost(filteredPosts[0].id);
        });
        if (DOM.fbPostsResizeHandle) DOM.fbPostsResizeHandle.addEventListener('mousedown', startResize);
    }

    return { open, close, init, openAndFilterOnPosts };
})();


Modals.NewImageGallery = (() => {
        let imageData = [];
        let selectedImageIndex = -1;
        let currentPage = 0;
        let itemsPerPage = 20;
        let isLoading = false;
        let hasMoreData = true;
        let searchTimeout = null;
        let selectedImageIds = new Set(); // Track selected image IDs
        let lastCtrlSelectedIndex = -1;
        let _isPickMode = false;
        let _pickModeCallback = null;

        function _regionLabel(code) {
            return Regions.label(code);
        }

        function formatDate(year, month, day) {
            if (!year && !month && !day) return 'No Date';
            const monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
                'July', 'August', 'September', 'October', 'November', 'December'];
            if (year && month && day) {
                return `${day} ${monthNames[month - 1]} ${year}`;
            }
            if (year && month) {
                return `${monthNames[month - 1]} ${year}`;
            } else if (year) {
                return year.toString();
            } else if (month) {
                return monthNames[month - 1];
            }
            return 'No Date';
        }

        function _setAccordionItemOpen(item, open) {
            if (!item) return;
            const head = item.querySelector('.new-image-gallery-accordion-head');
            const body = item.querySelector('.new-image-gallery-accordion-body');
            item.classList.toggle('is-collapsed', !open);
            if (head) head.setAttribute('aria-expanded', open ? 'true' : 'false');
            if (body) body.hidden = !open;
        }

        function _initSidebarAccordion() {
            const accordion = document.querySelector('.new-image-gallery-sidebar-accordion');
            if (!accordion) return;
            const items = accordion.querySelectorAll('.new-image-gallery-accordion-item');
            items.forEach((item) => {
                const head = item.querySelector('.new-image-gallery-accordion-head');
                if (!head) return;
                const open = !item.classList.contains('is-collapsed');
                _setAccordionItemOpen(item, open);
                head.addEventListener('click', () => {
                    const isOpen = !item.classList.contains('is-collapsed');
                    items.forEach((other) => _setAccordionItemOpen(other, false));
                    if (!isOpen) {
                        _setAccordionItemOpen(item, true);
                    }
                });
            });
        }

        function init() {
            _initSidebarAccordion();
            DOM.closeNewImageGalleryModalBtn.addEventListener('click', close);
            DOM.newImageGallerySearchBtn.addEventListener('click', _handleSearch);
            DOM.newImageGalleryClearBtn.addEventListener('click', _handleClear);
            if (DOM.newImageGallerySimilarBtn) {
                DOM.newImageGallerySimilarBtn.addEventListener('click', () => { void _handleSimilarByTags(); });
            }
            const similarClearBtn = document.getElementById('new-image-gallery-similar-clear-btn');
            if (similarClearBtn) {
                similarClearBtn.addEventListener('click', () => {
                    if (DOM.newImageGallerySimilarTags) DOM.newImageGallerySimilarTags.value = '';
                    if (DOM.newImageGallerySimilarN) DOM.newImageGallerySimilarN.value = '25';
                    imageData = [];
                    _renderThumbnailGrid();
                });
            }
            
            // Add event listeners for filter changes with debouncing
            const filterInputs = [
                DOM.newImageGalleryTitle,
                DOM.newImageGalleryTags,
                DOM.newImageGalleryAuthor,
                DOM.newImageGallerySource,
                DOM.newImageGalleryRegionFilter,
                DOM.newImageGalleryYearFilter,
                DOM.newImageGalleryMonthFilter,
                DOM.newImageGalleryRating,
                DOM.newImageGalleryRatingMin,
                DOM.newImageGalleryRatingMax,
                DOM.newImageGalleryHasGps,
                DOM.newImageGalleryNoGps,
                DOM.newImageGalleryAIClassified,
                DOM.newImageGalleryAIUnclassified
            ];

            if (DOM.newImageGalleryHasGps && DOM.newImageGalleryNoGps) {
                DOM.newImageGalleryHasGps.addEventListener('change', () => {
                    if (DOM.newImageGalleryHasGps.checked) {
                        DOM.newImageGalleryNoGps.checked = false;
                    }
                });
                DOM.newImageGalleryNoGps.addEventListener('change', () => {
                    if (DOM.newImageGalleryNoGps.checked) {
                        DOM.newImageGalleryHasGps.checked = false;
                    }
                });
            }

            filterInputs.forEach(input => {
                if (input) {
                    if (input.type === 'checkbox') {
                        input.addEventListener('change', () => {
                            if (searchTimeout) clearTimeout(searchTimeout);
                            searchTimeout = setTimeout(() => {
                                _handleSearch();
                            }, 300);
                        });
                    } else if (input.tagName === 'SELECT') {
                        input.addEventListener('change', () => {
                            if (searchTimeout) clearTimeout(searchTimeout);
                            searchTimeout = setTimeout(() => {
                                _handleSearch();
                            }, 300);
                        });
                    } else {
                        input.addEventListener('input', () => {
                            if (searchTimeout) clearTimeout(searchTimeout);
                            searchTimeout = setTimeout(() => {
                                _handleSearch();
                            }, 300);
                        });
                    }
                }
            });
            
            // Add scroll event listener for lazy loading
            DOM.newImageGalleryThumbnailGrid.addEventListener('scroll', _handleThumbnailScroll);
            
            // Bulk tag application
            if (DOM.newImageGalleryApplyTagsBtn) {
                DOM.newImageGalleryApplyTagsBtn.addEventListener('click', async () => {
                    await _applyTagsToSelected();
                });
            }

            if (DOM.newImageGalleryAIClassificationBtn) {
                DOM.newImageGalleryAIClassificationBtn.addEventListener('click', async () => {
                    await _startAIClassificationForSelected();
                });
            }

            if (DOM.newImageGallerySetGpsBtn) {
                DOM.newImageGallerySetGpsBtn.addEventListener('click', () => {
                    Modals.BulkGpsPicker.open(Array.from(selectedImageIds), {
                        getImageById: (id) => imageData.find((img) => img.id === id),
                        onSuccess: (result) => {
                            const updates = Array.isArray(result.updates) ? result.updates : [];
                            updates.forEach((u) => {
                                const img = imageData.find((im) => im.id === u.id);
                                if (!img) return;
                                img.latitude = u.latitude;
                                img.longitude = u.longitude;
                                img.has_gps = true;
                            });
                            _renderThumbnailGrid();
                            _updateSelectionUI();
                            const msg = result.message || `Updated ${result.updated_count} image(s).`;
                            void AppDialogs.showAppAlert('Set location', msg);
                        }
                    });
                });
            }
            
            // Bulk delete selected images
            if (DOM.newImageGalleryDeleteSelectedBtn) {
                DOM.newImageGalleryDeleteSelectedBtn.addEventListener('click', async () => {
                    await _deleteSelectedImages();
                });
            }
            
            // Clear selection
            if (DOM.newImageGalleryClearSelectionBtn) {
                DOM.newImageGalleryClearSelectionBtn.addEventListener('click', () => {
                    selectedImageIds.clear();
                    _updateSelectionUI();
                });
            }
            
            // Enable/disable apply button based on tags input
            if (DOM.newImageGalleryBulkTags) {
                DOM.newImageGalleryBulkTags.addEventListener('input', () => {
                    _updateSelectionUI();
                });
            }
            
            // Initialize resizable panes
            _initResizablePanes();
            
            // Note: Detail modal handlers are now managed by Modals.ImageDetailModal.init()
        }
        
        function _initResizablePanes() {
            if (!DOM.newImageGalleryDivider || !DOM.newImageGalleryMasterPane || !DOM.newImageGalleryDetailPane) {
                return;
            }
            
            // Load saved divider position from localStorage
            const savedPosition = localStorage.getItem('newImageGalleryDividerPosition');
            const defaultPosition = 35; // 35% for master pane
            const masterPaneWidth = savedPosition ? parseFloat(savedPosition) : defaultPosition;
            
            _setPaneWidths(masterPaneWidth);
            
            let isResizing = false;
            let startX = 0;
            let startMasterWidth = 0;
            
            DOM.newImageGalleryDivider.addEventListener('mousedown', (e) => {
                isResizing = true;
                startX = e.clientX;
                startMasterWidth = parseFloat(getComputedStyle(DOM.newImageGalleryMasterPane).width);
                document.body.style.cursor = 'col-resize';
                document.body.style.userSelect = 'none';
                e.preventDefault();
            });
            
            document.addEventListener('mousemove', (e) => {
                if (!isResizing) return;
                
                const deltaX = e.clientX - startX;
                const modalWidth = DOM.newImageGalleryModal.offsetWidth;
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
                    const currentWidth = parseFloat(getComputedStyle(DOM.newImageGalleryMasterPane).width);
                    const modalWidth = DOM.newImageGalleryModal.offsetWidth;
                    const percentage = (currentWidth / modalWidth) * 100;
                    localStorage.setItem('newImageGalleryDividerPosition', percentage.toString());
                }
            });
        }
        
        function _setPaneWidths(masterPanePercentage) {
            if (!DOM.newImageGalleryMasterPane || !DOM.newImageGalleryDetailPane) {
                return;
            }
            
            DOM.newImageGalleryMasterPane.style.width = `${masterPanePercentage}%`;
            DOM.newImageGalleryDetailPane.style.width = `${100 - masterPanePercentage}%`;
        }

        function _syncGalleryPickModeZIndex() {
            if (!DOM.newImageGalleryModal) return;
            if (_isPickMode) {
                DOM.newImageGalleryModal.classList.add('new-image-gallery-pick-mode');
            } else {
                DOM.newImageGalleryModal.classList.remove('new-image-gallery-pick-mode');
            }
        }

        function _setGalleryLoading(show) {
            const loadingEl = document.getElementById('new-image-gallery-loading');
            const main = document.querySelector('.new-image-gallery-main');
            if (loadingEl) {
                loadingEl.hidden = !show;
                loadingEl.style.display = show ? 'flex' : 'none';
            }
            if (main) main.setAttribute('aria-busy', show ? 'true' : 'false');
        }

        /** Clears metadata / similar / filter inputs only (no grid refresh). Call after _setupFilters when selects exist. */
        function _resetGallerySearchCriteria() {
            if (DOM.newImageGalleryTitle) DOM.newImageGalleryTitle.value = '';
            if (DOM.newImageGalleryTags) DOM.newImageGalleryTags.value = '';
            if (DOM.newImageGallerySimilarTags) DOM.newImageGallerySimilarTags.value = '';
            if (DOM.newImageGallerySimilarN) DOM.newImageGallerySimilarN.value = '25';
            if (DOM.newImageGalleryAuthor) DOM.newImageGalleryAuthor.value = '';
            if (DOM.newImageGallerySource) DOM.newImageGallerySource.value = '';
            if (DOM.newImageGalleryRegionFilter) DOM.newImageGalleryRegionFilter.value = '';
            if (DOM.newImageGalleryYearFilter) DOM.newImageGalleryYearFilter.value = '0';
            if (DOM.newImageGalleryMonthFilter) DOM.newImageGalleryMonthFilter.value = '0';
            if (DOM.newImageGalleryCreatedFrom) DOM.newImageGalleryCreatedFrom.value = '';
            if (DOM.newImageGalleryCreatedTo) DOM.newImageGalleryCreatedTo.value = '';
            if (DOM.newImageGalleryRating) DOM.newImageGalleryRating.value = '';
            if (DOM.newImageGalleryRatingMin) DOM.newImageGalleryRatingMin.value = '';
            if (DOM.newImageGalleryRatingMax) DOM.newImageGalleryRatingMax.value = '';
            if (DOM.newImageGalleryHasGps) DOM.newImageGalleryHasGps.checked = false;
            if (DOM.newImageGalleryNoGps) DOM.newImageGalleryNoGps.checked = false;
            if (DOM.newImageGalleryAIClassified) DOM.newImageGalleryAIClassified.checked = false;
            if (DOM.newImageGalleryAIUnclassified) DOM.newImageGalleryAIUnclassified.checked = false;
        }

        /**
         * @param {{ resetCriteriaAndSearch?: boolean }} [options]
         * When resetCriteriaAndSearch is true (sidebar Images button), filters are cleared.
         */
        async function open(options) {
            const resetCriteriaAndSearch = !!(options && options.resetCriteriaAndSearch);
            DOM.newImageGalleryModal.style.display = 'flex';
            _syncGalleryPickModeZIndex();
            if (resetCriteriaAndSearch) {
                imageData = [];
                if (DOM.newImageGalleryThumbnailGrid) DOM.newImageGalleryThumbnailGrid.innerHTML = '';
            }
            await _setupFilters();
            if (resetCriteriaAndSearch) {
                _resetGallerySearchCriteria();
            }
            selectedImageIndex = -1;
            selectedImageIds.clear();
            lastCtrlSelectedIndex = -1;
            if (DOM.newImageGalleryBulkTags) {
                DOM.newImageGalleryBulkTags.value = '';
            }
            _updateSelectionUI();
            _updatePickModeBanner();
            await _updateThumbnailProcessingBanner();
            await _refreshSimilarEmbedAvailability();
            // Don't load images automatically — wait for user to run a search
            // (or pick-mode / openTaggedImages / openImagesFromDate path triggers loading).
            imageData = [];
            _renderThumbnailGrid();
        }

        async function _refreshSimilarEmbedAvailability() {
            try {
                const r = await fetch('/api/embed/availability');
                const d = await r.json().catch(() => ({}));
                const ok = !!(d && d.available);
                if (DOM.newImageGallerySimilarTags) {
                    DOM.newImageGallerySimilarTags.disabled = !ok;
                    DOM.newImageGallerySimilarTags.title = ok ? '' : 'Configure LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL';
                }
                if (DOM.newImageGallerySimilarBtn) {
                    DOM.newImageGallerySimilarBtn.disabled = !ok;
                }
                if (DOM.newImageGallerySimilarN) {
                    DOM.newImageGallerySimilarN.disabled = false;
                }
                if (DOM.newImageGallerySimilarHint) {
                    DOM.newImageGallerySimilarHint.textContent = ok ? 'Normalized tags, local embedding model' : 'Embedding API unavailable — check local AI settings';
                }
            } catch (e) {
                console.warn('embed availability check failed', e);
            }
        }

        async function openPickMode(callback) {
            _isPickMode = true;
            _pickModeCallback = callback;
            await open();
        }

        function _updatePickModeBanner() {
            const modal = DOM.newImageGalleryModal;
            if (!modal) return;
            let banner = modal.querySelector('.pick-mode-banner');
            if (_isPickMode) {
                if (!banner) {
                    banner = document.createElement('div');
                    banner.className = 'pick-mode-banner';
                    banner.setAttribute('role', 'status');
                    banner.innerHTML = '<span><i class="fas fa-info-circle" aria-hidden="true"></i> Select an image to add it to the artefact</span>';
                    const body = modal.querySelector('.new-image-gallery-modal-body');
                    const content = modal.querySelector('.new-image-gallery-modal-content');
                    if (body && content) {
                        content.insertBefore(banner, body);
                    } else if (content) {
                        content.appendChild(banner);
                    }
                }
            } else if (banner) {
                banner.remove();
            }
        }

        async function _updateThumbnailProcessingBanner() {
            const modal = DOM.newImageGalleryModal;
            if (!modal) return;
            const modalContent = modal.querySelector('.new-image-gallery-modal-content');
            if (!modalContent) return;
            let banner = modal.querySelector('.image-gallery-thumbnail-warning-banner');
            if (banner) banner.remove();
            try {
                const response = await fetch('/images/process-thumbnails/status');
                if (!response.ok) return;
                const data = await response.json();
                const inProgress = data.status === 'in_progress' || data.in_progress === true;
                if (!inProgress) return;
                banner = document.createElement('div');
                banner.className = 'image-gallery-thumbnail-warning-banner';
                banner.innerHTML = '<span><i class="fas fa-info-circle"></i> Some thumbnails are still being created.</span><button class="image-gallery-thumbnail-warning-dismiss" type="button" aria-label="Dismiss"><i class="fas fa-times"></i></button>';
                const body = modal.querySelector('.new-image-gallery-modal-body');
                if (body) body.appendChild(banner);
                else modalContent.appendChild(banner);
                banner.querySelector('.image-gallery-thumbnail-warning-dismiss').addEventListener('click', () => banner.remove());
            } catch (e) {
                // Ignore fetch errors (e.g. network, 404)
            }
        }

        async function openTaggedImages(tags) {
            await open();
            if (DOM.newImageGalleryTags) {
                DOM.newImageGalleryTags.value = tags;
            }
            await _loadImageData();
        }

        async function openImagesFromDate(year, month) {
            await open();
            if (DOM.newImageGalleryYearFilter) {
                DOM.newImageGalleryYearFilter.value = year;
            }
            if (DOM.newImageGalleryMonthFilter) {
                DOM.newImageGalleryMonthFilter.value = month;
            }
            _handleSearch();
        }
        

        function close() {
            DOM.newImageGalleryModal.style.display = 'none';
            selectedImageIndex = -1;
            selectedImageIds.clear();
            lastCtrlSelectedIndex = -1;
            _isPickMode = false;
            _pickModeCallback = null;
            _syncGalleryPickModeZIndex();
        }

        async function _setupFilters() {
            // Setup year filter - fetch distinct years from API
            try {
                const response = await fetch('/images/years');
                if (!response.ok) {
                    throw new Error(`Failed to fetch years: ${response.statusText}`);
                }
                const data = await response.json();
                const distinctYears = data.years || [];
                
                // Build years array with "All Years" option first, then distinct years
                const years = [
                    { value: 0, text: 'All Years' }
                ];
                
                // Add distinct years from database
                distinctYears.forEach(year => {
                    years.push({ value: year, text: year.toString() });
                });
                
                if (DOM.newImageGalleryYearFilter) {
                    DOM.newImageGalleryYearFilter.innerHTML = '';
                    years.forEach(year => {
                        const option = document.createElement('option');
                        option.value = year.value;
                        option.textContent = year.text;
                        DOM.newImageGalleryYearFilter.appendChild(option);
                    });
                }
            } catch (error) {
                console.error('Error loading years:', error);
                // Fallback to "All Years" option if API call fails
                if (DOM.newImageGalleryYearFilter) {
                    DOM.newImageGalleryYearFilter.innerHTML = '<option value="0" selected>All Years</option>';
                }
            }

            // Setup tags datalist - fetch distinct tags from API
            try {
                const tagsResponse = await fetch('/images/tags');
                if (!tagsResponse.ok) {
                    throw new Error(`Failed to fetch tags: ${tagsResponse.statusText}`);
                }
                const tagsData = await tagsResponse.json();
                const distinctTags = tagsData.tags || [];
                
                // Populate datalist with distinct tags
                const tagsDatalist = document.getElementById('new-image-gallery-tags-list');
                if (tagsDatalist) {
                    tagsDatalist.innerHTML = '';
                    distinctTags.forEach(tag => {
                        const option = document.createElement('option');
                        option.value = tag;
                        tagsDatalist.appendChild(option);
                    });
                }
            } catch (error) {
                console.error('Error loading tags:', error);
                // Continue without tags datalist if API call fails
            }

            // Setup region filter — distinct regions from archive
            try {
                const regionResponse = await fetch('/images/regions', { cache: 'no-store' });
                if (!regionResponse.ok) {
                    throw new Error(`Failed to fetch regions: ${regionResponse.statusText}`);
                }
                const regionData = await regionResponse.json();
                const distinctRegions = regionData.regions || [];
                const prevRegion = DOM.newImageGalleryRegionFilter
                    ? (DOM.newImageGalleryRegionFilter.value || '')
                    : '';
                if (DOM.newImageGalleryRegionFilter) {
                    DOM.newImageGalleryRegionFilter.innerHTML = '';
                    const allOpt = document.createElement('option');
                    allOpt.value = '';
                    allOpt.textContent = 'All regions';
                    DOM.newImageGalleryRegionFilter.appendChild(allOpt);
                    distinctRegions.forEach(row => {
                        const code = row && row.region != null ? String(row.region).trim() : '';
                        if (!code) return;
                        const option = document.createElement('option');
                        option.value = code;
                        const count = row.count != null ? Number(row.count) : 0;
                        option.textContent = count > 0
                            ? `${_regionLabel(code)} (${count.toLocaleString()})`
                            : _regionLabel(code);
                        DOM.newImageGalleryRegionFilter.appendChild(option);
                    });
                    if (prevRegion && Array.from(DOM.newImageGalleryRegionFilter.options).some(o => o.value === prevRegion)) {
                        DOM.newImageGalleryRegionFilter.value = prevRegion;
                    }
                }
            } catch (error) {
                console.error('Error loading regions:', error);
                if (DOM.newImageGalleryRegionFilter && DOM.newImageGalleryRegionFilter.options.length === 0) {
                    DOM.newImageGalleryRegionFilter.innerHTML = '<option value="" selected>All regions</option>';
                }
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
            
            if (DOM.newImageGalleryMonthFilter) {
                DOM.newImageGalleryMonthFilter.innerHTML = '';
                months.forEach(month => {
                    const option = document.createElement('option');
                    option.value = month.value;
                    option.textContent = month.text;
                    DOM.newImageGalleryMonthFilter.appendChild(option);
                });
            }
        }

        async function _loadImageData() {
            _setGalleryLoading(true);
            const params = new URLSearchParams();
            
            // Build query parameters from filter inputs
            if (DOM.newImageGalleryTitle && DOM.newImageGalleryTitle.value.trim()) {
                params.append('title', DOM.newImageGalleryTitle.value.trim());
            }
            if (DOM.newImageGalleryTags && DOM.newImageGalleryTags.value.trim()) {
                params.append('tags', DOM.newImageGalleryTags.value.trim());
            }
            if (DOM.newImageGalleryAuthor && DOM.newImageGalleryAuthor.value.trim()) {
                params.append('author', DOM.newImageGalleryAuthor.value.trim());
            }
            if (DOM.newImageGallerySource) {
                const src = (DOM.newImageGallerySource.value || '').trim();
                if (src) params.append('source', src);
            }
            if (DOM.newImageGalleryRegionFilter) {
                const region = (DOM.newImageGalleryRegionFilter.value || '').trim();
                if (region) params.append('region', region);
            }
            if (DOM.newImageGalleryYearFilter && DOM.newImageGalleryYearFilter.value && DOM.newImageGalleryYearFilter.value !== '0') {
                params.append('year', DOM.newImageGalleryYearFilter.value);
            }
            if (DOM.newImageGalleryMonthFilter && DOM.newImageGalleryMonthFilter.value && DOM.newImageGalleryMonthFilter.value !== '0') {
                params.append('month', DOM.newImageGalleryMonthFilter.value);
            }
            const createdFrom = DOM.newImageGalleryCreatedFrom ? DOM.newImageGalleryCreatedFrom.value.trim() : '';
            const createdTo = DOM.newImageGalleryCreatedTo ? DOM.newImageGalleryCreatedTo.value.trim() : '';
            if (createdFrom && createdTo && createdFrom > createdTo) {
                _setGalleryLoading(false);
                if (window.AppDialogs) {
                    await AppDialogs.showAppAlert('Invalid date range', 'From date must be on or before To date.');
                }
                return;
            }
            if (createdFrom) {
                params.append('created_from', createdFrom);
            }
            if (createdTo) {
                params.append('created_to', createdTo);
            }
            if (DOM.newImageGalleryRating && DOM.newImageGalleryRating.value) {
                params.append('rating', DOM.newImageGalleryRating.value);
            }
            if (DOM.newImageGalleryRatingMin && DOM.newImageGalleryRatingMin.value) {
                params.append('rating_min', DOM.newImageGalleryRatingMin.value);
            }
            if (DOM.newImageGalleryRatingMax && DOM.newImageGalleryRatingMax.value) {
                params.append('rating_max', DOM.newImageGalleryRatingMax.value);
            }
            if (DOM.newImageGalleryHasGps && DOM.newImageGalleryHasGps.checked) {
                params.append('has_gps', 'true');
            }
            if (DOM.newImageGalleryNoGps && DOM.newImageGalleryNoGps.checked) {
                params.append('has_gps', 'false');
            }
            if (DOM.newImageGalleryAIClassified && DOM.newImageGalleryAIClassified.checked) {
                params.append('ai_classified', 'true');
            }
            if (DOM.newImageGalleryAIUnclassified && DOM.newImageGalleryAIUnclassified.checked) {
                params.append('ai_unclassified', 'true');
            }

            try {
                const response = await fetch('/images/search?' + params.toString());
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const data = await response.json();
                imageData = data;
                _renderThumbnailGrid();
            } catch (error) {
                console.error('Error loading image data:', error);
                imageData = [];
                _renderThumbnailGrid();
            } finally {
                _setGalleryLoading(false);
            }
        }

        function _renderThumbnailGrid() {
            // Reset pagination when rendering new grid
            currentPage = 0;
            hasMoreData = true;
            DOM.newImageGalleryThumbnailGrid.innerHTML = '';

            if (imageData.length === 0) {
                const noResults = document.createElement('div');
                noResults.style.textAlign = 'center';
                noResults.style.padding = '2em';
                noResults.style.color = '#666';
                noResults.style.gridColumn = '1 / -1';
                // Check if we have any search criteria
                const hasCriteria = _hasSearchCriteria();
                noResults.textContent = hasCriteria 
                    ? 'No images found matching your criteria' 
                    : 'Enter search criteria above and click Search to find images';
                DOM.newImageGalleryThumbnailGrid.appendChild(noResults);
                return;
            }

            _loadMoreThumbnails();
        }

        function _hasSearchCriteria() {
            return (
                (DOM.newImageGalleryTitle && DOM.newImageGalleryTitle.value.trim()) ||
                (DOM.newImageGalleryTags && DOM.newImageGalleryTags.value.trim()) ||
                (DOM.newImageGallerySimilarTags && DOM.newImageGallerySimilarTags.value.trim()) ||
                (DOM.newImageGalleryAuthor && DOM.newImageGalleryAuthor.value.trim()) ||
                (DOM.newImageGallerySource && (DOM.newImageGallerySource.value || '').trim()) ||
                (DOM.newImageGalleryYearFilter && DOM.newImageGalleryYearFilter.value && DOM.newImageGalleryYearFilter.value !== '0') ||
                (DOM.newImageGalleryMonthFilter && DOM.newImageGalleryMonthFilter.value && DOM.newImageGalleryMonthFilter.value !== '0') ||
                (DOM.newImageGalleryRating && DOM.newImageGalleryRating.value) ||
                (DOM.newImageGalleryRatingMin && DOM.newImageGalleryRatingMin.value) ||
                (DOM.newImageGalleryRatingMax && DOM.newImageGalleryRatingMax.value) ||
                (DOM.newImageGalleryHasGps && DOM.newImageGalleryHasGps.checked) ||
                (DOM.newImageGalleryAIClassified && DOM.newImageGalleryAIClassified.checked) ||
                (DOM.newImageGalleryAIUnclassified && DOM.newImageGalleryAIUnclassified.checked)
            );
        }

        function _getSourceAbbreviation(source) {
            // Return short abbreviation for source
            const abbreviations = {
                'email_attachment': 'Email',
                'gmail_attachment':"GMail",
                'facebook_messenger': 'Facebook Messenger',
                'filesystem': 'Image Import',
                'instagram': 'Insta',
                'whatsapp': 'WhatsApp',
                'imessage': 'iMessage',
                'sms': 'SMS',
                'facebook_post': 'Facebook Post',
                'facebook_album': 'Facebook Album'
            };
            return abbreviations[source] || source.substring(0, 2).toUpperCase();
        }

        function _loadMoreThumbnails() {
            if (isLoading || !hasMoreData) return;

            isLoading = true;
            
            const startIndex = currentPage * itemsPerPage;
            const endIndex = startIndex + itemsPerPage;
            const imagesToRender = imageData.slice(startIndex, endIndex);

            if (imagesToRender.length === 0) {
                hasMoreData = false;
                isLoading = false;
                return;
            }
            
            imagesToRender.forEach((image, localIndex) => {
                const actualIndex = startIndex + localIndex;
                
                const thumbnailItem = document.createElement('div');
                thumbnailItem.className = 'new-image-gallery-thumbnail-item';
                thumbnailItem.dataset.index = actualIndex;
                
                const img = document.createElement('img');
                img.loading = 'lazy';
                img.src = `/images/${image.id}?type=metadata&preview=true`;
                img.alt = image.title || 'Image thumbnail';
                img.onerror = function() {
                    this.src = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="150" height="150"%3E%3Crect fill="%23ddd" width="150" height="150"/%3E%3Ctext fill="%23999" font-family="sans-serif" font-size="14" x="50%25" y="50%25" text-anchor="middle" dy=".3em"%3ENo Image%3C/text%3E%3C/svg%3E';
                };
                
                thumbnailItem.appendChild(img);
                
                // Add source indicator badge
                if (image.source) {
                    const sourceBadge = document.createElement('div');
                    sourceBadge.className = 'new-image-gallery-source-badge';
                    sourceBadge.dataset.source = image.source;
                    sourceBadge.textContent = _getSourceAbbreviation(image.source);
                    thumbnailItem.appendChild(sourceBadge);
                }
                
                thumbnailItem.addEventListener('click', (e) => _selectImage(actualIndex, e));
                
                if (selectedImageIds.has(image.id)) {
                    thumbnailItem.classList.add('bulk-selected');
                }
                
                DOM.newImageGalleryThumbnailGrid.appendChild(thumbnailItem);
            });

            currentPage++;
            hasMoreData = endIndex < imageData.length;
            isLoading = false;

            // Check if we need to load more to fill the viewport
            // Use setTimeout to allow DOM to update before checking
            setTimeout(() => {
                if (hasMoreData) {
                    _checkAndLoadMoreIfNeeded();
                    // Add loading indicator if viewport is filled and there's more data
                    //const grid = DOM.newImageGalleryThumbnailGrid;
                    // if (grid.scrollHeight > grid.clientHeight + 50) {
                    //     _addLoadingIndicator();
                    // }
                }
            }, 100);
        }

        function _addLoadingIndicator() {
            // Remove existing loading indicator
            const existingIndicator = DOM.newImageGalleryThumbnailGrid.querySelector('.loading-indicator');
            if (existingIndicator) {
                existingIndicator.remove();
            }

            const loadingIndicator = document.createElement('div');
            loadingIndicator.className = 'loading-indicator';
            loadingIndicator.style.gridColumn = '1 / -1';
            loadingIndicator.style.textAlign = 'center';
            loadingIndicator.style.padding = '1em';
            loadingIndicator.style.color = '#666';
            loadingIndicator.innerHTML = `
                <div style="display: inline-block; width: 20px; height: 20px; border: 2px solid #f3f3f3; border-top: 2px solid #4a90e2; border-radius: 50%; animation: spin 1s linear infinite;"></div>
                <div style="margin-top: 0.5em;">Loading more images...</div>
            `;
            DOM.newImageGalleryThumbnailGrid.appendChild(loadingIndicator);
        }

        function _handleThumbnailScroll() {
            const grid = DOM.newImageGalleryThumbnailGrid;
            const { scrollTop, scrollHeight, clientHeight } = grid;
            
            // Load more thumbnails when user scrolls to within 200px of the bottom
            if (scrollTop + clientHeight >= scrollHeight - 200) {
                _loadMoreThumbnails();
            }
        }

        function _checkAndLoadMoreIfNeeded() {
            // Check if viewport needs more content and load if necessary
            const grid = DOM.newImageGalleryThumbnailGrid;
            if (hasMoreData && grid.scrollHeight <= grid.clientHeight + 50) {
                // Viewport is not filled enough, load more thumbnails
                _loadMoreThumbnails();
            }
        }

        async function _selectImage(index, e) {
            if (index < 0 || index >= imageData.length) return;

            const image = imageData[index];

            // Pick mode: fire callback and close
            if (_isPickMode && _pickModeCallback) {
                const cb = _pickModeCallback;
                close();
                cb(image.id);
                return;
            }

            if (e && (e.ctrlKey || e.metaKey)) {
                // Ctrl/Cmd-click: toggle selection
                if (selectedImageIds.has(image.id)) {
                    selectedImageIds.delete(image.id);
                } else {
                    selectedImageIds.add(image.id);
                    lastCtrlSelectedIndex = index;
                }
                _updateSelectionUI();
                return;
            }

            if (e && e.shiftKey && lastCtrlSelectedIndex >= 0) {
                // Shift-click: range select from last Ctrl-clicked index
                const from = Math.min(lastCtrlSelectedIndex, index);
                const to   = Math.max(lastCtrlSelectedIndex, index);
                for (let i = from; i <= to; i++) {
                    selectedImageIds.add(imageData[i].id);
                }
                _updateSelectionUI();
                return;
            }

            // Plain click: open detail modal
            selectedImageIndex = index;

            const thumbnails = DOM.newImageGalleryThumbnailGrid.querySelectorAll('.new-image-gallery-thumbnail-item');
            thumbnails.forEach((thumb) => {
                const actualIdx = parseInt(thumb.dataset.index);
                if (actualIdx === index) {
                    thumb.classList.add('selected');
                    thumb.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                } else {
                    thumb.classList.remove('selected');
                }
            });

            Modals.ImageDetailModal.open(image, {
                allowRedirects: true,
                onSave: (updatedImage, updateData) => {
                    const imageIndex = imageData.findIndex(img => img.id === updatedImage.id);
                    if (imageIndex !== -1) {
                        if ('tags' in updateData) {
                            imageData[imageIndex].tags = updateData.tags;
                        }
                        if (updateData && updateData.cleared_gps) {
                            imageData[imageIndex].has_gps = false;
                            imageData[imageIndex].latitude = null;
                            imageData[imageIndex].longitude = null;
                            imageData[imageIndex].region = null;
                            imageData[imageIndex].google_maps_url = null;
                        }
                    }
                },
                onDelete: (deletedImage) => {
                    imageData = imageData.filter(img => img.id !== deletedImage.id);

                    if (selectedImageIndex >= imageData.length) {
                        selectedImageIndex = -1;
                    }

                    _renderThumbnailGrid();

                    const thumbnails = DOM.newImageGalleryThumbnailGrid.querySelectorAll('.new-image-gallery-thumbnail-item');
                    thumbnails.forEach(thumb => thumb.classList.remove('selected'));
                }
            });
        }
        
        function _updateSelectionUI() {
            // Update selected count
            if (DOM.newImageGallerySelectedCount) {
                DOM.newImageGallerySelectedCount.textContent = selectedImageIds.size;
            }

            // Enable/disable apply button
            if (DOM.newImageGalleryApplyTagsBtn) {
                DOM.newImageGalleryApplyTagsBtn.disabled =
                    selectedImageIds.size === 0 ||
                    !DOM.newImageGalleryBulkTags || !DOM.newImageGalleryBulkTags.value.trim();
            }

            // Enable/disable delete button
            if (DOM.newImageGalleryDeleteSelectedBtn) {
                DOM.newImageGalleryDeleteSelectedBtn.disabled = selectedImageIds.size === 0;
            }

            if (DOM.newImageGalleryAIClassificationBtn) {
                DOM.newImageGalleryAIClassificationBtn.disabled = selectedImageIds.size === 0;
            }

            if (DOM.newImageGallerySetGpsBtn) {
                DOM.newImageGallerySetGpsBtn.disabled = selectedImageIds.size === 0;
            }

            if (DOM.newImageGalleryClearSelectionBtn) {
                DOM.newImageGalleryClearSelectionBtn.disabled = selectedImageIds.size === 0;
            }
            
            // Update thumbnail visual selection state
            const thumbnails = DOM.newImageGalleryThumbnailGrid.querySelectorAll('.new-image-gallery-thumbnail-item');
            thumbnails.forEach((thumb) => {
                const actualIdx = parseInt(thumb.dataset.index);
                if (actualIdx >= 0 && actualIdx < imageData.length) {
                    const image = imageData[actualIdx];
                    if (selectedImageIds.has(image.id)) {
                        thumb.classList.add('bulk-selected');
                    } else {
                        thumb.classList.remove('bulk-selected');
                    }
                }
            });
        }
        
        async function _startAIClassificationForSelected() {
            if (selectedImageIds.size === 0) return;
            const ids = Array.from(selectedImageIds);
            const btn = DOM.newImageGalleryAIClassificationBtn;
            if (btn) {
                btn.disabled = true;
            }
            try {
                const response = await fetch('/image/ai-classification', {
                    method: 'POST',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ids })
                });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(data.detail || data.error || `HTTP ${response.status}`);
                }
                await AppDialogs.showAppAlert('AI Classification', data.message || 'Job started.');
            } catch (e) {
                console.error('AI classification start failed:', e);
                await AppDialogs.showAppAlert('AI Classification', e.message || 'Request failed.');
            } finally {
                _updateSelectionUI();
            }
        }

        async function _applyTagsToSelected() {
            if (selectedImageIds.size === 0) return;

            const tags = DOM.newImageGalleryBulkTags ? DOM.newImageGalleryBulkTags.value.trim() : '';
            if (!tags) {
                await AppDialogs.showAppAlert('Please enter tags to apply');
                return;
            }

            const imageIds = Array.from(selectedImageIds);
            const btn = DOM.newImageGalleryApplyTagsBtn;
            const originalHTML = btn ? btn.innerHTML : null;
            if (btn) {
                btn.disabled = true;
                btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Applying…';
            }
            try {
                const response = await fetch('/images/bulk-update', {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        image_ids: imageIds,
                        tags: tags
                    })
                });
                
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                
                const result = await response.json();
                
                // Update local image data
                imageIds.forEach(id => {
                    const imageIndex = imageData.findIndex(img => img.id === id);
                    if (imageIndex !== -1) {
                        // Merge tags (append if existing)
                        const existingTags = imageData[imageIndex].tags || '';
                        const newTags = existingTags ? `${existingTags}, ${tags}` : tags;
                        imageData[imageIndex].tags = newTags;
                    }
                });
                
                // Clear selection and tags input
                selectedImageIds.clear();
                if (DOM.newImageGalleryBulkTags) {
                    DOM.newImageGalleryBulkTags.value = '';
                }
                _updateSelectionUI();
                
                await AppDialogs.showAppAlert('Success', `Successfully applied tags to ${result.updated_count} image(s)`);
            } catch (error) {
                console.error('Error applying tags:', error);
                await AppDialogs.showAppAlert('Error', `Error applying tags: ${error.message}`);
            } finally {
                if (btn && originalHTML !== null) {
                    btn.innerHTML = originalHTML;
                }
                _updateSelectionUI();
            }
        }

        async function _deleteSelectedImages() {
            if (selectedImageIds.size === 0) return;
            
            const imageIds = Array.from(selectedImageIds);
            const count = imageIds.length;
            
            const okDel = await AppDialogs.showAppConfirm(
                'Delete images',
                `Are you sure you want to delete ${count} image(s)? This action cannot be undone.`,
                { danger: true }
            );
            if (!okDel) {
                return;
            }
            
            try {
                const response = await fetch('/images/bulk-delete', {
                    method: 'DELETE',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        image_ids: imageIds
                    })
                });
                
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                
                const result = await response.json();
                
                // Remove deleted images from imageData array
                imageData = imageData.filter(img => !selectedImageIds.has(img.id));
                
                // Clear selection
                selectedImageIds.clear();
                _updateSelectionUI();
                
                // Refresh thumbnail grid
                _renderThumbnailGrid();
                
                // Close detail modal if the deleted image was being viewed
                const deletedIds = new Set(imageIds);
                // Note: ImageDetailModal manages its own state, so we just close it
                if (deletedIds.size > 0 && DOM.newImageGalleryDetailModal) {
                    DOM.newImageGalleryDetailModal.style.display = 'none';
                }
                
                await AppDialogs.showAppAlert('Success', `Successfully deleted ${result.deleted_count} image(s)`);
            } catch (error) {
                console.error('Error deleting images:', error);
                await AppDialogs.showAppAlert('Error', `Error deleting images: ${error.message}`);
            }
        }


        function _handleSearch() {
            _loadImageData();
        }

        async function _runSimilarByTagsQuery(q) {
            const defaultN = 25;
            const parsedN = DOM.newImageGallerySimilarN && DOM.newImageGallerySimilarN.value
                ? parseInt(DOM.newImageGallerySimilarN.value, 10)
                : NaN;
            const n = (!Number.isFinite(parsedN) || parsedN <= 0) ? defaultN : parsedN;
            const r = await fetch('/images/similar-by-tags', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ text: q, n }),
                credentials: 'same-origin'
            });
            if (!r.ok) {
                const t = await r.text();
                throw new Error(t || r.statusText);
            }
            const data = await r.json();
            imageData = Array.isArray(data.results) ? data.results : [];
            _renderThumbnailGrid();
        }

        async function _handleSimilarByTags() {
            const q = DOM.newImageGallerySimilarTags && DOM.newImageGallerySimilarTags.value
                ? DOM.newImageGallerySimilarTags.value.trim()
                : '';
            if (!q) {
                if (window.AppDialogs) await AppDialogs.showAppAlert('Similar by tags', 'Enter text to match against stored image tags (e.g. beach, sunset).');
                return;
            }
            const btn = DOM.newImageGallerySimilarBtn;
            const originalHTML = btn ? btn.innerHTML : null;
            if (btn) {
                btn.disabled = true;
                btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Searching…';
            }
            try {
                await _runSimilarByTagsQuery(q);
            } catch (err) {
                console.error('similar-by-tags', err);
                if (window.AppDialogs) await AppDialogs.showAppAlert('Similar by tags', err.message || 'Request failed');
                imageData = [];
                _renderThumbnailGrid();
            } finally {
                if (btn && originalHTML !== null) {
                    btn.innerHTML = originalHTML;
                    btn.disabled = false;
                }
            }
        }

        /**
         * Run tag-embedding similarity search and show hits in the main Images gallery.
         * Opens the gallery if needed, closes the Image Details modal when done.
         */
        async function showSimilarByTagsFromDetail(tagsText) {
            const q = String(tagsText || '').trim();
            if (!q) {
                if (window.AppDialogs) await AppDialogs.showAppAlert('Find Similar', 'Add tags to this image first, or enter tags in the Tags field.');
                return;
            }
            const galleryEl = DOM.newImageGalleryModal;
            const wasOpen = galleryEl && galleryEl.style.display === 'flex';
            if (!wasOpen) {
                await open();
            } else {
                await _refreshSimilarEmbedAvailability();
            }
            if (DOM.newImageGallerySimilarTags) {
                DOM.newImageGallerySimilarTags.value = q;
            }
            selectedImageIndex = -1;
            selectedImageIds.clear();
            lastCtrlSelectedIndex = -1;
            _updateSelectionUI();
            try {
                await _runSimilarByTagsQuery(q);
                if (Modals.ImageDetailModal && typeof Modals.ImageDetailModal.close === 'function') {
                    Modals.ImageDetailModal.close();
                }
            } catch (err) {
                console.error('similar-by-tags', err);
                if (window.AppDialogs) await AppDialogs.showAppAlert('Find Similar', err.message || 'Request failed');
                imageData = [];
                _renderThumbnailGrid();
            }
        }

        function _handleClear() {
            _resetGallerySearchCriteria();
            selectedImageIndex = -1;
            selectedImageIds.clear();
            _updateSelectionUI();
            imageData = [];
            _renderThumbnailGrid();
        }


        return { init, open, close, openTaggedImages, openImagesFromDate, openPickMode, showSimilarByTagsFromDetail };
})();


Modals.ImageDetailModal = (() => {
        let currentImageInModal = null;
        let originalImageData = null;
        let onSaveCallback = null;
        let onDeleteCallback = null;
        let allowRedirects = true;

        function formatDate(year, month, day) {
            if (!year && !month && !day) return 'No Date';
            const monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
                'July', 'August', 'September', 'October', 'November', 'December'];
            if (year && month && day) {
                return `${day} ${monthNames[month - 1]} ${year}`;
            }
            if (year && month) {
                return `${monthNames[month - 1]} ${year}`;
            } else if (year) {
                return year.toString();
            } else if (month) {
                return monthNames[month - 1];
            }
            return 'No Date';
        }

        function formatDateTime(dateTime) {
            if (!dateTime) return 'N/A';
            try {
                const date = new Date(dateTime);
                if (isNaN(date.getTime())) return 'N/A';
                
                const day = String(date.getDate()).padStart(2, '0');
                const month = String(date.getMonth() + 1).padStart(2, '0');
                const year = date.getFullYear();
                const hours = String(date.getHours()).padStart(2, '0');
                const minutes = String(date.getMinutes()).padStart(2, '0');
                
                return `${day}/${month}/${year} ${hours}:${minutes}`;
            } catch (e) {
                return dateTime.toString();
            }
        }

        function _clearGpsActionButtons() {
            [DOM.newImageDetailActionsGrid, DOM.newImageDetailActionsBottom].forEach((container) => {
                if (!container) return;
                container
                    .querySelectorAll('.image-detail-action-btn--gps')
                    .forEach((btn) => btn.remove());
            });
        }

        function _setupChangeTracking() {
            // Remove existing listeners to avoid duplicates
            const tagsEdit = DOM.newImageDetailTagsEdit;
            if (tagsEdit) {
                tagsEdit.removeEventListener('input', _checkForChanges);
                tagsEdit.addEventListener('input', _checkForChanges);
            }
        }
        
        function _checkForChanges() {
            if (!originalImageData || !currentImageInModal) return;
            
            const currentTags = DOM.newImageDetailTagsEdit ? DOM.newImageDetailTagsEdit.value.trim() : '';
            const hasChanges = currentTags !== (originalImageData.tags || '');
            
            if (DOM.newImageGallerySaveBtn) {
                DOM.newImageGallerySaveBtn.disabled = !hasChanges;
            }
        }
        
        async function saveChanges() {
            if (!currentImageInModal) return;
            
            const imageId = currentImageInModal.id;
            const tags = DOM.newImageDetailTagsEdit ? DOM.newImageDetailTagsEdit.value.trim() : null;
            
            const updateData = {};
            if (tags !== null) updateData.tags = tags || null;
            
            try {
                const response = await fetch(`/images/${imageId}`, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(updateData)
                });
                
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                
                // Update currentImageInModal
                currentImageInModal.tags = tags;
                
                // Update original data
                originalImageData = {
                    tags: tags || ''
                };
                
                // Disable save button
                if (DOM.newImageGallerySaveBtn) {
                    DOM.newImageGallerySaveBtn.disabled = true;
                }
                
                // Call callback if provided
                if (onSaveCallback) {
                    onSaveCallback(currentImageInModal, updateData);
                }
                
                await AppDialogs.showAppAlert('Success', 'Image metadata updated successfully');
            } catch (error) {
                console.error('Error updating image:', error);
                await AppDialogs.showAppAlert('Error', `Error updating image: ${error.message}`);
            }
        }

        async function removeGpsInfo() {
            if (!currentImageInModal) return;

            const okRemove = await AppDialogs.showAppConfirm(
                'Remove GPS info',
                'Remove latitude, longitude, region, and Google Maps link for this image?',
                { danger: true }
            );
            if (!okRemove) return;

            const imageId = currentImageInModal.id;
            try {
                const response = await fetch(`/images/${imageId}/gps`, { method: 'DELETE' });
                if (!response.ok) {
                    const errorData = await response.json().catch(() => ({}));
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }

                currentImageInModal.has_gps = false;
                currentImageInModal.latitude = null;
                currentImageInModal.longitude = null;
                currentImageInModal.google_maps_url = null;
                currentImageInModal.region = null;

                DOM.newImageDetailRegion.textContent = 'N/A';
                DOM.newImageDetailGps.innerHTML = '';
                DOM.newImageDetailGpsRow.style.display = 'none';
                _clearGpsActionButtons();

                if (onSaveCallback) {
                    onSaveCallback(currentImageInModal, { cleared_gps: true });
                }
            } catch (error) {
                console.error('Error removing GPS info:', error);
                await AppDialogs.showAppAlert('Error', `Error removing GPS info: ${error.message}`);
            }
        }

        async function deleteImage() {
            if (!currentImageInModal) return;
            
            const imageId = currentImageInModal.id;
            const imageTitle = currentImageInModal.title || 'Image';
            
            const okDelOne = await AppDialogs.showAppConfirm(
                'Delete image',
                `Are you sure you want to delete "${imageTitle}"? This action cannot be undone.`,
                { danger: true }
            );
            if (!okDelOne) {
                return;
            }
            
            try {
                const response = await fetch(`/images/${imageId}`, {
                    method: 'DELETE'
                });
                
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.detail || `HTTP error! status: ${response.status}`);
                }
                
                // Close the modal
                DOM.newImageGalleryDetailModal.style.display = 'none';
                const deletedImage = currentImageInModal;
                currentImageInModal = null;
                originalImageData = null;
                
                // Call callback if provided
                if (onDeleteCallback) {
                    onDeleteCallback(deletedImage);
                }
                
                await AppDialogs.showAppAlert('Success', `Successfully deleted image: ${imageTitle}`);
            } catch (error) {
                console.error('Error deleting image:', error);
                await AppDialogs.showAppAlert('Error', `Error deleting image: ${error.message}`);
            }
        }

        function open(image, options = {}) {
            // Extract options
            allowRedirects = options.allowRedirects !== undefined ? options.allowRedirects : true;
            onSaveCallback = options.onSave || null;
            onDeleteCallback = options.onDelete || null;

            // Check if source is Email - if so, navigate to Email Gallery instead (if redirects allowed)
            if (allowRedirects && image.source === "Email" && image.source_reference) {
                const emailId = parseInt(image.source_reference);
                
                if (!isNaN(emailId)) {
                    // Close Image Gallery modals
                    if (DOM.newImageGalleryModal) DOM.newImageGalleryModal.style.display = 'none';
                    if (DOM.newImageGalleryDetailModal) DOM.newImageGalleryDetailModal.style.display = 'none';
                    
                    // Open Email Gallery and select the email
                    Modals.EmailGallery.openAndSelectEmail(emailId);
                    return; // Don't open image detail modal
                }
            }
            
            // Check if source is Facebook Album - if so, navigate to Facebook Albums Gallery instead (if redirects allowed)
            if (allowRedirects && image.source === "Facebook Album" && image.source_reference) {
                const albumId = image.source_reference;
                
                if (albumId) {
                    // Close Image Gallery modals
                    if (DOM.newImageGalleryModal) DOM.newImageGalleryModal.style.display = 'none';
                    if (DOM.newImageGalleryDetailModal) DOM.newImageGalleryDetailModal.style.display = 'none';
                    
                    // Open Facebook Albums Gallery and select the album
                    Modals.FBAlbums.openAndSelectAlbum(albumId);
                    return; // Don't open image detail modal
                }
            }


            
            currentImageInModal = image;
            
            // Store original values for change detection
            originalImageData = {
                tags: image.tags || ''
            };
            
            // Set image source
            DOM.newImageGalleryDetailImage.src = `/images/${image.id}?type=metadata&convert_heic_to_jpg=true`;
            DOM.newImageGalleryDetailImage.alt = image.title || 'Image';
            
            // Remove GPS buttons left over from a previously shown image
            _clearGpsActionButtons();
            
            if (DOM.newImageGallerySaveBtn) {
                DOM.newImageGallerySaveBtn.disabled = true;
            }
            
            // Populate all metadata fields
            DOM.newImageDetailTitle.textContent = image.title || 'N/A';
            
            // Populate editable fields
            if (DOM.newImageDetailTagsEdit) {
                DOM.newImageDetailTagsEdit.value = image.tags || '';
            }
            
            DOM.newImageDetailAuthor.textContent = image.author || 'N/A';
            DOM.newImageDetailDate.textContent = formatDate(image.year, image.month, image.day);
            DOM.newImageDetailImageType.textContent = image.media_type || image.image_type || 'N/A';
            
            // Source: Show button if not "Filesystem", otherwise show text
            const sourceValue = image.source || 'N/A';

            //translate the source value to the full name
            const sourceFullName = _getSourceFullName(sourceValue); 

            DOM.newImageDetailSource.innerHTML = '';
            if (sourceValue && sourceValue !== 'N/A' && sourceValue.toLowerCase() !== 'filesystem') {
                const openSourceButton = document.createElement('button');
                openSourceButton.type = 'button';
                openSourceButton.className = 'modal-btn modal-btn-secondary';
                openSourceButton.style.cssText = 'padding: 0.3em 0.8em; font-size: 0.85em;margin-left: 10px;';
                //openSourceButton.textContent = 'Open Source ('+sourceFullName+')';
                openSourceButton.textContent = 'Open';
                openSourceButton.onclick = async function(e) {


                    e.preventDefault();
                    e.stopPropagation();
                    
                    // Handle email-attachment source
                    if (sourceValue.toLowerCase() === 'email_attachment' || sourceValue.toLowerCase() === 'gmail_attachment' || sourceValue.toLowerCase() === 'email') {
                        if (image.source_reference) {
                            Modals.NewImageGallery.close();
                            const emailId = parseInt(image.source_reference);
                            if (!isNaN(emailId)) {
                                // Open email gallery and select the email
                                Modals.ImageDetailModal.close();
                                Modals.EmailGallery.openAndSelectEmail(emailId);
                            } else {
                                console.error('Invalid email ID in source_reference:', image.source_reference);
                                await AppDialogs.showAppAlert('Unable to open email: Invalid email ID');
                            }
                        } else {
                            console.error('No source_reference found for email attachment');
                            await AppDialogs.showAppAlert('Unable to open email: No email reference found');
                        }
                    } else if (sourceValue.toLowerCase() === 'message_attachment' || sourceValue.toLowerCase() === 'imessage' || sourceValue.toLowerCase() === 'sms' || sourceValue.toLowerCase() === 'whatsapp' || sourceValue.toLowerCase() === 'facebook') {
                        //Open the SMS Messages modal and select the conversation
                        Modals.NewImageGallery.close();
                        Modals.ImageDetailModal.close();
                        Modals.SMSMessages.openAndSelectConversation(image.source_reference);
                    } else if (sourceValue.toLowerCase() === 'facebook_album') {
                        //Open the Facebook Albums modal and select the album
                        Modals.NewImageGallery.close();
                        Modals.ImageDetailModal.close();
                        Modals.FBAlbums.openAndSelectAlbum(image.source_reference);
                    } else if (sourceValue.toLowerCase() === 'facebook_post') {
                        //Open the Facebook Posts modal and select the post
                        Modals.NewImageGallery.close();
                        Modals.ImageDetailModal.close();
                        Modals.FBPosts.openAndFilterOnPosts([image.source_reference]);
                    } else if (sourceValue.toLowerCase() === 'facebook_messenger') {
                        //Open the Facebook Messenger modal and select the conversation
                        Modals.NewImageGallery.close();
                        Modals.ImageDetailModal.close();
                        Modals.SMSMessages.openAndSelectConversation(image.source_reference);
                    } else if (sourceValue.toLowerCase() === 'instagram') {
                        //Open the Instagram modal and select the post
                        Modals.NewImageGallery.close();
                        Modals.ImageDetailModal.close();
                        Modals.SMSMessages.openAndSelectConversation(image.source_reference);
                    }
                    // Add other source types here as needed
                };
                DOM.newImageDetailSource.textContent = sourceFullName;
                DOM.newImageDetailSource.appendChild(openSourceButton);
            } else {
                DOM.newImageDetailSource.textContent = sourceValue;
            }
            
            DOM.newImageDetailSourceReference.textContent = image.source_reference || 'N/A';
            DOM.newImageDetailRegion.textContent = (image.region != null && String(image.region).trim() !== '')
                ? Regions.label(image.region)
                : 'N/A';
            DOM.newImageDetailProcessed.textContent = image.processed ? 'Yes' : 'No';
            DOM.newImageDetailCreatedAt.textContent = formatDateTime(image.created_at);
            DOM.newImageDetailUpdatedAt.textContent = formatDateTime(image.updated_at);
            
            // GPS Location
        
            if (image.has_gps && (image.latitude || image.longitude)) {
                let gpsText = '';
                if (image.latitude && image.longitude) {
                    gpsText = `${image.latitude.toFixed(6)}, ${image.longitude.toFixed(6)}`;
                    
                    // Create Google Maps URL
                    const googleMapsUrl = `https://www.google.com/maps?q=${image.latitude},${image.longitude}`;
                    
                    // Add button to open Google Maps in new tab
                    const openMapsButton = document.createElement('button');
                    openMapsButton.type = 'button';
                    openMapsButton.className = 'modal-btn image-detail-action-btn image-detail-action-btn--gps image-detail-action-btn--maps';
                    openMapsButton.innerHTML = '<i class="fas fa-map-marker-alt"></i> Open in Google Maps';
                    openMapsButton.onclick = function(e) {
                        e.preventDefault();
                        e.stopPropagation();
                        window.open(googleMapsUrl, '_blank');
                    };

                    const findNearbyButton = document.createElement('button');
                    findNearbyButton.type = 'button';
                    findNearbyButton.className = 'modal-btn image-detail-action-btn image-detail-action-btn--gps image-detail-action-btn--nearby';
                    findNearbyButton.innerHTML = '<i class="fas fa-search-location"></i> Find Nearby';
                    findNearbyButton.onclick = async function(e) {
                        e.preventDefault();
                        e.stopPropagation();
                        const radiusStr = await AppDialogs.showAppPrompt(
                            'Find Nearby',
                            'Enter search radius in kilometers:',
                            '25',
                            { promptLabel: 'Radius (km)' }
                        );
                        if (radiusStr == null) return;
                        const radiusKm = parseFloat(String(radiusStr).trim());
                        if (!Number.isFinite(radiusKm) || radiusKm <= 0) {
                            await AppDialogs.showAppAlert('Invalid radius', 'Please enter a positive number.');
                            return;
                        }
                        Modals.NearbyLocations.open(image.latitude, image.longitude, radiusKm, image.id);
                    };

                    const removeGpsButton = document.createElement('button');
                    removeGpsButton.type = 'button';
                    removeGpsButton.className = 'modal-btn image-detail-action-btn image-detail-action-btn--gps image-detail-action-btn--remove-gps';
                    removeGpsButton.innerHTML = '<i class="fas fa-eraser"></i> Remove GPS Info';
                    removeGpsButton.onclick = async function(e) {
                        e.preventDefault();
                        e.stopPropagation();
                        await removeGpsInfo();
                    };
                    
                    // Coordinates stay in the metadata row; buttons go to the actions block
                    DOM.newImageDetailGps.textContent = gpsText;
                    if (DOM.newImageDetailActionsGrid) {
                        DOM.newImageDetailActionsGrid.appendChild(openMapsButton);
                        DOM.newImageDetailActionsGrid.appendChild(findNearbyButton);
                    }
                    if (DOM.newImageDetailActionsBottom) {
                        DOM.newImageDetailActionsBottom.appendChild(removeGpsButton);
                    }
                } else {
                    DOM.newImageDetailGps.textContent = 'GPS data available';
                }
                DOM.newImageDetailGpsRow.style.display = 'flex';
            } else {
                DOM.newImageDetailGpsRow.style.display = 'none';
            }
            
            // Altitude
            if (image.altitude !== null && image.altitude !== undefined) {
                DOM.newImageDetailAltitude.textContent = `${image.altitude.toFixed(2)} meters`;
                DOM.newImageDetailAltitudeRow.style.display = 'flex';
            } else {
                DOM.newImageDetailAltitudeRow.style.display = 'none';
            }
            
            // Set up change tracking
            _setupChangeTracking();
            
            // Show modal
            DOM.newImageGalleryDetailModal.style.display = 'flex';
        }

        function _getSourceFullName(sourceValue) {

            const abbreviations = {
                'email_attachment': 'Email',
                'gmail_attachment':"GMail",
                'facebook_messenger': 'Facebook Messenger',
                'filesystem': 'Image Import',
                'instagram': 'Insta',
                'whatsapp': 'WhatsApp',
                'imessage': 'iMessage',
                'sms': 'SMS',
                'facebook_post': 'Facebook Post',
                'facebook_album': 'Facebook Album'
            };
            return abbreviations[sourceValue] || sourceValue.substring(0, 2).toUpperCase();
        }   

        function close() {
            if (DOM.newImageGalleryDetailModal) {
                DOM.newImageGalleryDetailModal.style.display = 'none';
            }
            currentImageInModal = null;
            originalImageData = null;
            if (DOM.newImageGallerySaveBtn) {
                DOM.newImageGallerySaveBtn.disabled = true;
            }
        }

        function init() {
            // Close detail modal handler
            if (DOM.closeNewImageGalleryDetailModalBtn) {
                DOM.closeNewImageGalleryDetailModalBtn.addEventListener('click', close);
            }
            
            // Close modal when clicking outside
            if (DOM.newImageGalleryDetailModal) {
                DOM.newImageGalleryDetailModal.addEventListener('click', (e) => {
                    if (e.target === DOM.newImageGalleryDetailModal) {
                        close();
                    }
                });
            }

            if (DOM.newImageDetailFindSimilarBtn) {
                DOM.newImageDetailFindSimilarBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    const fromField = DOM.newImageDetailTagsEdit && DOM.newImageDetailTagsEdit.value
                        ? DOM.newImageDetailTagsEdit.value.trim()
                        : '';
                    const fromImage = currentImageInModal && currentImageInModal.tags
                        ? String(currentImageInModal.tags).trim()
                        : '';
                    const tags = fromField || fromImage;
                    if (Modals.NewImageGallery && typeof Modals.NewImageGallery.showSimilarByTagsFromDetail === 'function') {
                        await Modals.NewImageGallery.showSimilarByTagsFromDetail(tags);
                    }
                });
            }
            
            if (DOM.newImageDetailFindSameTimeBtn) {
                DOM.newImageDetailFindSameTimeBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    const img = currentImageInModal;
                    const createdAtRaw = img && img.created_at ? img.created_at : null;
                    if (!createdAtRaw) {
                        if (window.AppDialogs) {
                            await AppDialogs.showAppAlert('Find Same Time', 'This image has no created_at timestamp.');
                        }
                        return;
                    }
                    const createdAt = new Date(createdAtRaw);
                    if (isNaN(createdAt.getTime())) {
                        if (window.AppDialogs) {
                            await AppDialogs.showAppAlert('Find Same Time', 'Unable to parse created_at date for this image.');
                        }
                        return;
                    }

                    const dateToYMD = (d) => {
                        const yyyy = d.getFullYear();
                        const mm = String(d.getMonth() + 1).padStart(2, '0');
                        const dd = String(d.getDate()).padStart(2, '0');
                        return `${yyyy}-${mm}-${dd}`;
                    };

                    const fromDate = new Date(createdAt.getTime());
                    fromDate.setDate(fromDate.getDate() - 7);
                    const toDate = new Date(createdAt.getTime());
                    toDate.setDate(toDate.getDate() + 7);

                    // Close image details first; then open gallery and run metadata search.
                    close();
                    if (Modals.NewImageGallery && typeof Modals.NewImageGallery.open === 'function') {
                        await Modals.NewImageGallery.open({ resetCriteriaAndSearch: true });
                    }

                    if (DOM.newImageGalleryCreatedFrom) {
                        DOM.newImageGalleryCreatedFrom.value = dateToYMD(fromDate);
                    }
                    if (DOM.newImageGalleryCreatedTo) {
                        DOM.newImageGalleryCreatedTo.value = dateToYMD(toDate);
                    }

                    if (DOM.newImageGallerySearchBtn) {
                        DOM.newImageGallerySearchBtn.click();
                    }
                });
            }
            
            // Delete button handler
            if (DOM.newImageGalleryDeleteBtn) {
                DOM.newImageGalleryDeleteBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await deleteImage();
                });
            }
            
            // Save button handler
            if (DOM.newImageGallerySaveBtn) {
                DOM.newImageGallerySaveBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await saveChanges();
                });
            }
        }

        return { init, open, close };
})();


Modals.ConfirmationModal = (() => {
        function init() {
            if (typeof AppDialogs !== 'undefined' && AppDialogs.init) {
                AppDialogs.init();
            }
        }

        function open(title, text, onConfirmFn) {
            if (typeof AppDialogs !== 'undefined' && AppDialogs.openLegacy) {
                AppDialogs.openLegacy(title, text, onConfirmFn);
            }
        }

        function close() {
            if (typeof AppDialogs !== 'undefined' && AppDialogs.close) {
                AppDialogs.close();
            }
        }

        return { init, open, close };
})();


Modals.MultiImageDisplay = (() => { 
        let currentGalleryImages = [];
        let currentGalleryIndex = 0;
        let imageModalElement = null;
        let imageModalImgElement = null;
        let imageModalVideoElement = null;
        let imageModalAudioElement = null;
        let imageModalPdfElement = null;

        const updateGalleryImage = () => {

          

            if (imageModalImgElement && currentGalleryImages.length > 0) {
                const item = currentGalleryImages[currentGalleryIndex];

    
                let src = "/getImage?id="+item.file_id
                let alt = item.photo_description || `Image ${currentGalleryIndex + 1}`;
                let srcType = item.file_type;
                let file_extension = null;

                // if the item is a string that start with /getImage?id= then set src to the item
                if (typeof item === 'string' && item.startsWith('/getImage?id=')) {
                    src = item;
                    file_extension = src.split('ext=').pop(); 
                    file_extension = file_extension.trim();
                    file_extension = file_extension.toLowerCase();
                    file_extension = file_extension.replace('.', '');

                    if (file_extension === "jpg" || file_extension === "jpeg" || file_extension === "png" || file_extension === "gif" || file_extension === "webp") {
                        srcType = 'image';
                    }else if ( file_extension === "zip" || file_extension === "doc" || file_extension === "docx" || file_extension === "heic" || file_extension === "pptx" || file_extension === "ppt" || file_extension === "xls" || file_extension === "xlsx" || file_extension === "txt") {
                        srcType = 'image';
                        src = src+'&preview=true';
                    } else if (file_extension === "mp4" || file_extension === "mov" || file_extension === "avi" || file_extension === "mkv" || file_extension === "webm") {
                        srcType = 'video';
                    } else if (file_extension === "mp3" || file_extension === "wav" || file_extension === "ogg" || file_extension === "m4a" || file_extension === "aac" || file_extension === "opus") {
                        srcType = 'audio';
                    } else if (file_extension === "pdf") {
                        srcType = 'pdf';
                    }
                } else  if (!item.file_id) {
                    src = item;
                    if (typeof item === 'string' && item.startsWith('/getImage?id=')) {
                        file_extension = src.split('ext=').pop(); 
                    } else {
                        file_extension = src.split('.').pop();
                    }
                
                    if (file_extension === "jpg" || file_extension === "jpeg" || file_extension === "png" || file_extension === "gif" || file_extension === "webp") {
                        srcType = 'image';
                    }else if ( file_extension === "zip" || file_extension === "doc" || file_extension === "docx" || file_extension === "heic" || file_extension === "pptx" || file_extension === "ppt" || file_extension === "xls" || file_extension === "xlsx" || file_extension === "txt") {
                        srcType = 'image';
                        src = src+'&preview=true';
                    }else if (file_extension === "mp4" || file_extension === "mov" || file_extension === "avi" || file_extension === "mkv" || file_extension === "webm") {
                        srcType = 'video';
                    } else if (file_extension === "mp3" || file_extension === "wav" || file_extension === "ogg" || file_extension === "m4a" || file_extension === "aac" || file_extension === "opus") {
                        srcType = 'audio';
                    } else if (file_extension === "pdf") {
                        srcType = 'pdf';
                    }
                }
            
                imageModalVideoElement.style.display = 'none';
                imageModalAudioElement.style.display = 'none';
                imageModalImgElement.style.display = 'none';
                imageModalPdfElement.style.display = 'none';

                if (srcType === 'image') {
                    imageModalImgElement.src = src;
                    imageModalImgElement.alt = alt;
                    imageModalImgElement.style.display = 'block';
                } else if (srcType === 'video') {
                    imageModalVideoElement.src = src;
                    imageModalVideoElement.alt = alt;
                    imageModalVideoElement.style.display = 'block';
                    imageModalVideoElement.controls = true;
                    imageModalVideoElement.autoplay = true;
                    imageModalVideoElement.loop = true;
                    imageModalVideoElement.muted = false;
                    imageModalVideoElement.playsinline = true;
                    imageModalVideoElement.style.width = '100%';
                    imageModalVideoElement.style.height = '100%';
                    imageModalVideoElement.style.objectFit = 'contain';
                } else if (srcType === 'audio') {
                    imageModalAudioElement.src = src;
                    imageModalAudioElement.alt = alt;
                    imageModalAudioElement.controls = true;
                    imageModalAudioElement.autoplay = true;
                    imageModalAudioElement.style.objectFit = 'contain';
                    imageModalAudioElement.style.display = 'block';
                } else if (srcType === 'pdf') {
                    imageModalPdfElement.src = src;
                    imageModalPdfElement.alt = alt;
                    imageModalPdfElement.style.display = 'block';
                    imageModalPdfElement.style.width = '100%';
                    imageModalPdfElement.style.height = '100%';
                }
            }
        };
        


        function closeImageGalleryModal() {
            if (imageModalElement) {
                imageModalElement.remove();
                imageModalElement = null;
                imageModalImgElement = null;
            }
            document.removeEventListener('keydown', handleGalleryKeyPress);
        }

        // Shows a gallery for image attachments in messages
        function showMultiImageModal(images, currentImageUri) {
            closeImageGalleryModal(); // Close any existing

            currentGalleryImages = images;
            currentGalleryIndex = images.findIndex(img => (img.file_id || img) === currentImageUri);
            if (currentGalleryIndex === -1) currentGalleryIndex = 0;

            imageModalElement = document.createElement('div');
            imageModalElement.className = 'image-modal';
            
            imageModalImgElement = document.createElement('img');
            imageModalVideoElement = document.createElement('video');
            imageModalAudioElement = document.createElement('audio');
            imageModalPdfElement = document.createElement('iframe');
            // updateGalleryImage will set src and alt

            const closeBtn = document.createElement('button');
            closeBtn.className = 'modal-close';
            closeBtn.innerHTML = '×';
            closeBtn.onclick = closeImageGalleryModal;
            imageModalElement.appendChild(closeBtn);

            // Add download button
            const downloadBtn = document.createElement('button');
            downloadBtn.className = 'download-btn';
            downloadBtn.style.position = 'absolute';
            downloadBtn.style.top = '15px';
            downloadBtn.style.right = '60px';
            downloadBtn.style.zIndex = '1001';
            downloadBtn.innerHTML = '<i class="fas fa-download"></i> Download';
            downloadBtn.title = 'Download this item';
            downloadBtn.onclick = _downloadCurrentMultiImageItem;
            imageModalElement.appendChild(downloadBtn);

            if (currentGalleryImages.length > 1) {
                const prevBtn = document.createElement('button');
                prevBtn.className = 'nav-arrow prev';
                prevBtn.innerHTML = '❮';
                prevBtn.onclick = (e) => {
                    e.stopPropagation();
                    currentGalleryIndex = (currentGalleryIndex - 1 + currentGalleryImages.length) % currentGalleryImages.length;
                    updateGalleryImage();
                };
                imageModalElement.appendChild(prevBtn);

                const nextBtn = document.createElement('button');
                nextBtn.className = 'nav-arrow next';
                nextBtn.innerHTML = '❯';
                nextBtn.onclick = (e) => {
                    e.stopPropagation();
                    currentGalleryIndex = (currentGalleryIndex + 1) % currentGalleryImages.length;
                    updateGalleryImage();
                };
                imageModalElement.appendChild(nextBtn);
            }
            
            imageModalElement.appendChild(imageModalImgElement);
            imageModalElement.appendChild(imageModalVideoElement);
            imageModalElement.appendChild(imageModalAudioElement);
            imageModalElement.appendChild(imageModalPdfElement);
            document.body.appendChild(imageModalElement);
            updateGalleryImage(); // Set initial image

            imageModalElement.addEventListener('click', (e) => {
                if (e.target === imageModalElement) closeImageGalleryModal();
            });
            document.addEventListener('keydown', handleGalleryKeyPress);
        }

        
        function closeTileModal() {
            DOM.multiImageContainer.innerHTML = '';
            Modals._closeModal(DOM.fbImageModal);
        }

        function _downloadCurrentMultiImageItem() {
            if (currentGalleryImages.length === 0 || currentGalleryIndex < 0) {
                console.error('No file information available for download');
                return;
            }

            const currentItem = currentGalleryImages[currentGalleryIndex];
            let file_id = '';
            let filename = 'download';

            // Extract file_id and filename from the current item
            if (typeof currentItem === 'string' && currentItem.startsWith('/getImage?id=')) {
                file_id = currentItem.split('=')[1];
                filename = file_id;
            } else if (currentItem.file_id) {
                file_id = currentItem.file_id;
                filename = currentItem.file || currentItem.photo_description || file_id;
            } else {
                console.error('Unable to determine file information for download');
                return;
            }

            // Use the download endpoint
            let downloadUrl = `/downloadFile?id=${file_id}`;

            // Create a temporary link element to trigger download
            const link = document.createElement('a');
            link.href = downloadUrl;
            link.download = filename;
            link.target = '_blank';
            
            // Append to body, click, and remove
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
        }

        const handleGalleryKeyPress = (e) => {
            if (!imageModalElement) return;
            if (e.key === 'ArrowLeft' && currentGalleryImages.length > 1) {
                currentGalleryIndex = (currentGalleryIndex - 1 + currentGalleryImages.length) % currentGalleryImages.length;
                updateGalleryImage();
            } else if (e.key === 'ArrowRight' && currentGalleryImages.length > 1) {
                currentGalleryIndex = (currentGalleryIndex + 1) % currentGalleryImages.length;
                updateGalleryImage();
            } else if (e.key === 'Escape') {
                closeImageGalleryModal();
            }
        };         
        
        const handleKeydownForTileModal = (event) => {
            if (event.key === 'Escape' && DOM.fbImageModal.style.display === 'flex') {
                closeTileModal();
            }
        };

        function init() {
            if (DOM.multiImageModalCloseBtn) DOM.multiImageModalCloseBtn.addEventListener('click', closeTileModal);
            DOM.fbImageModal.addEventListener('click', (e) => { if (e.target === DOM.fbImageModal) closeTileModal(); });
            document.addEventListener('keydown', handleKeydownForTileModal);
        }

        return { init, showMultiImageModal };
})();


Modals.SingleImageDisplay = (() => {
        function init() {
            // The modal is already in the HTML, just need to ensure proper event handling
            if (DOM.singleImageModal) {
                DOM.singleImageModal.addEventListener('click', (e) => {
                    if (e.target === DOM.singleImageModal) {
                        _close();
                    }
                });
            }
            if (DOM.closeSingleImageModalBtn) {
                DOM.closeSingleImageModalBtn.addEventListener('click', _close);
            }
            
            // Add keyboard support for Escape key
            document.addEventListener('keydown', (e) => {
                if (e.key === 'Escape' && DOM.singleImageModal.style.display === 'flex') {
                    _close();
                }
            });
        }

        function showSingleImageModal(filename, file_id, taken, lat, long) {
            if (!DOM.singleImageModal || !DOM.singleImageModalImg || !DOM.singleImageDetails) {
                console.error('SingleImage modal elements not found');
                return;
            }
            
            DOM.singleImageModalAudio.style.display = 'none';
            DOM.singleImageModalVideo.style.display = 'none';
            DOM.singleImageModalPdf.style.display = 'none';
            DOM.singleImageModalImg.style.display = 'none';

            // Set the image source

            if (file_id.endsWith('mp3') || file_id.endsWith('wav') || file_id.endsWith('ogg') || file_id.endsWith('m4a') || file_id.endsWith('aac') || file_id.endsWith('opus')) {
                DOM.singleImageModalAudio.src = file_id;
                DOM.singleImageModalAudio.style.display = 'block';
                DOM.singleImageModalAudio.style.width = '400px';
                DOM.singleImageModalAudio.style.height = '50px';
                DOM.singleImageModalAudio.controls = true;
                DOM.singleImageModalAudio.autoplay = true;
                DOM.singleImageModalAudio.muted = false;
                DOM.singleImageModalAudio.style.objectFit = 'contain';
            } else if (file_id.endsWith('mp4')) {
                DOM.singleImageModalVideo.src = file_id;
                DOM.singleImageModalVideo.style.display = 'block';
            } else if (file_id.endsWith('pdf')) {
                DOM.singleImageModalPdf.src = file_id;
                DOM.singleImageModalPdf.style.display = 'block';
            }else if (file_id.endsWith('doc') || file_id.endsWith('docx') || file_id.endsWith('zip') || file_id.endsWith('pptx') || file_id.endsWith('ppt') || file_id.endsWith('xls') || file_id.endsWith('xlsx') || file_id.endsWith('txt')) {
                if (file_id.startsWith('/getImage')) {
                    DOM.singleImageModalImg.src = file_id+'&preview=true';
                } else {
                    DOM.singleImageModalImg.src = '/getImage?id=' + file_id+'&preview=true';
                }
                DOM.singleImageModalImg.style.display = 'block';
            } else {
                if (file_id.startsWith('/getImage') || file_id.startsWith('/images/') || file_id.startsWith('/facebook/')) {
                    DOM.singleImageModalImg.src = file_id;
                } else {
                    DOM.singleImageModalImg.src = '/getImage?id=' + file_id;
                }
                DOM.singleImageModalImg.style.display = 'block';
            }


            // Set the details
            //DOM.singleImageDetails.innerHTML = `<p>${filename}</p><p>Taken: ${taken}</p><p>Latitude: ${lat}, Longitude: ${long}</p>`;

            // Show the modal
            Modals._openModal(DOM.singleImageModal);
        }

        function _close() {
            // Stop any playing audio
            if (DOM.singleImageModalAudio) {
                DOM.singleImageModalAudio.pause();
                DOM.singleImageModalAudio.currentTime = 0;
            }
            
            // Stop any playing video
            if (DOM.singleImageModalVideo) {
                DOM.singleImageModalVideo.pause();
                DOM.singleImageModalVideo.currentTime = 0;
            }
            
            // Clear PDF viewer
            if (DOM.singleImageModalPdf) {
                DOM.singleImageModalPdf.src = '';
            }
            
            Modals._closeModal(DOM.singleImageModal);
        }

        return { init, showSingleImageModal};
})();


Modals.BulkGpsPicker = (() => {
        const SESSION_KEY = 'dm_bulk_gps_last';
        let map = null;
        let marker = null;
        let markerIcon = null;
        let mapInitialized = false;
        let pendingEligibleIds = [];
        let pendingOptions = null;
        let pickedLat = null;
        let pickedLng = null;

        function _imageWithoutGps(img) {
            if (!img) return false;
            if (img.has_gps) return false;
            if (img.latitude != null && img.longitude != null) return false;
            return true;
        }

        function _eligibleIds(imageIds, getImageById) {
            const out = [];
            const seen = new Set();
            imageIds.forEach((id) => {
                if (seen.has(id)) return;
                seen.add(id);
                const img = getImageById(id);
                if (_imageWithoutGps(img)) {
                    out.push(id);
                }
            });
            return out;
        }

        function _updateCoordDisplay() {
            if (DOM.bulkGpsPickLat) {
                DOM.bulkGpsPickLat.textContent = pickedLat != null ? pickedLat.toFixed(6) : '—';
            }
            if (DOM.bulkGpsPickLng) {
                DOM.bulkGpsPickLng.textContent = pickedLng != null ? pickedLng.toFixed(6) : '—';
            }
        }

        function _placeMarker(lat, lng) {
            pickedLat = lat;
            pickedLng = lng;
            _updateCoordDisplay();
            if (!map) return;
            if (marker) {
                marker.setLatLng([lat, lng]);
            } else {
                marker = L.marker([lat, lng], { draggable: true, icon: markerIcon }).addTo(map);
                marker.on('dragend', () => {
                    const p = marker.getLatLng();
                    _placeMarker(p.lat, p.lng);
                });
            }
        }

        function _ensureMap() {
            if (!DOM.bulkGpsPickMap || typeof L === 'undefined') return;
            if (mapInitialized) {
                setTimeout(() => {
                    if (map) map.invalidateSize();
                }, 150);
                return;
            }
            map = L.map(DOM.bulkGpsPickMap, { minZoom: 1, maxZoom: 19 });
            L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
                maxZoom: 19,
                attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>'
            }).addTo(map);
            markerIcon = L.icon({
                iconUrl: '/static/images/marker-dark-blue.png',
                iconSize: [25, 35],
                iconAnchor: [12, 32],
                popupAnchor: [0, -32]
            });
            map.on('click', (e) => {
                _placeMarker(e.latlng.lat, e.latlng.lng);
            });
            mapInitialized = true;
        }

        function close() {
            if (DOM.bulkGpsPickModal) {
                Modals._closeModal(DOM.bulkGpsPickModal);
            }
        }

        function open(imageIds, options) {
            if (!DOM.bulkGpsPickModal || typeof L === 'undefined') {
                void AppDialogs.showAppAlert('Set location', 'Map library is not available.');
                return;
            }
            const getImageById = options && options.getImageById;
            if (!getImageById) return;

            const eligible = _eligibleIds(imageIds, getImageById);
            const skippedInSelection = imageIds.length - eligible.length;
            if (eligible.length === 0) {
                void AppDialogs.showAppAlert('Set location', 'No selected images without GPS can be updated.');
                return;
            }

            pendingEligibleIds = eligible;
            pendingOptions = options;

            if (DOM.bulkGpsPickTitle) {
                DOM.bulkGpsPickTitle.innerHTML = `<i class="fas fa-map-marker-alt"></i> Set GPS for ${eligible.length} image(s)`;
            }
            if (DOM.bulkGpsPickSkippedNote) {
                if (skippedInSelection > 0) {
                    DOM.bulkGpsPickSkippedNote.textContent =
                        `${skippedInSelection} selected image(s) already have GPS and will be skipped.`;
                    DOM.bulkGpsPickSkippedNote.style.display = 'block';
                } else {
                    DOM.bulkGpsPickSkippedNote.style.display = 'none';
                }
            }

            pickedLat = null;
            pickedLng = null;
            _updateCoordDisplay();
            if (marker && map) {
                map.removeLayer(marker);
                marker = null;
            }

            Modals._openModal(DOM.bulkGpsPickModal);
            _ensureMap();

            let lat = 20;
            let lng = 0;
            let zoom = 2;
            try {
                const saved = JSON.parse(sessionStorage.getItem(SESSION_KEY) || 'null');
                if (saved && typeof saved.lat === 'number' && typeof saved.lng === 'number') {
                    lat = saved.lat;
                    lng = saved.lng;
                    zoom = saved.zoom || 10;
                    _placeMarker(lat, lng);
                }
            } catch (_) { /* ignore */ }
            if (map) {
                map.setView([lat, lng], zoom);
            }
        }

        async function _apply() {
            if (pickedLat == null || pickedLng == null) {
                await AppDialogs.showAppAlert('Set location', 'Click the map to choose a location first.');
                return;
            }
            const btn = DOM.bulkGpsPickApplyBtn;
            const originalHTML = btn ? btn.innerHTML : null;
            if (btn) {
                btn.disabled = true;
                btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Applying…';
            }
            try {
                const response = await fetch('/images/bulk-set-gps', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({
                        image_ids: pendingEligibleIds,
                        latitude: pickedLat,
                        longitude: pickedLng
                    })
                });
                const result = await response.json().catch(() => ({}));
                if (!response.ok) {
                    throw new Error(result.detail || result.error || `HTTP ${response.status}`);
                }
                try {
                    sessionStorage.setItem(SESSION_KEY, JSON.stringify({
                        lat: pickedLat,
                        lng: pickedLng,
                        zoom: map ? map.getZoom() : 10
                    }));
                } catch (_) { /* ignore */ }
                if (pendingOptions && pendingOptions.onSuccess) {
                    pendingOptions.onSuccess(result);
                }
                close();
            } catch (e) {
                console.error('bulk-set-gps failed:', e);
                await AppDialogs.showAppAlert('Set location', e.message || 'Request failed.');
            } finally {
                if (btn && originalHTML !== null) {
                    btn.innerHTML = originalHTML;
                    btn.disabled = false;
                }
            }
        }

        function init() {
            if (!DOM.bulkGpsPickModal) return;
            if (DOM.closeBulkGpsPickModalBtn) {
                DOM.closeBulkGpsPickModalBtn.addEventListener('click', close);
            }
            if (DOM.bulkGpsPickCancelBtn) {
                DOM.bulkGpsPickCancelBtn.addEventListener('click', close);
            }
            if (DOM.bulkGpsPickApplyBtn) {
                DOM.bulkGpsPickApplyBtn.addEventListener('click', () => { void _apply(); });
            }
            DOM.bulkGpsPickModal.addEventListener('click', (e) => {
                if (e.target === DOM.bulkGpsPickModal) close();
            });
        }

        return { init, open, close };
})();


