'use strict';

/**
 * UploadImport — handles local-path data import for the Electron desktop app.
 *
 * Two flows:
 *  1. ZIP archive import  → pick .zip via native dialog → POST /import/from-path → SSE /import/upload/stream
 *  2. Photo directory import → pick directory via native dialog → POST /images/import → SSE /images/import/stream
 *
 * Self-initialises because it is loaded after app.js (defer order).
 */
const UploadImport = (() => {
    // ── ZIP import state ──────────────────────────────────────────────────────
    let zipFilePath = null;
    let zipEventSource = null;
    let zipImporting = false;

    // ── Photo import state ────────────────────────────────────────────────────
    let photoDirPath = null;
    let photoImporting = false;
    let photoEventSource = null;

    /** Display labels for ZIP import type. */
    const ZIP_ARCHIVE_LABELS = {
        facebook: 'Facebook (full export)',
        instagram: 'Instagram',
        whatsapp: 'WhatsApp',
        imessage: 'iMessage / SMS (iMaizing export)',
    };

    // ── Helpers ───────────────────────────────────────────────────────────────

    function setProgress(barEl, pct) {
        if (barEl) barEl.style.width = Math.min(100, Math.max(0, pct)) + '%';
    }

    function showResult(el, ok, html) {
        if (!el) return;
        el.style.display = 'block';
        el.style.backgroundColor = ok ? 'rgba(16,185,129,0.1)' : 'rgba(239,68,68,0.1)';
        el.style.border = ok ? '1px solid rgba(16,185,129,0.4)' : '1px solid rgba(239,68,68,0.4)';
        el.style.color = ok ? '#065f46' : '#991b1b';
        el.innerHTML = html;
    }

    function closeZipSSE() {
        if (zipEventSource) { zipEventSource.close(); zipEventSource = null; }
    }

    function closePhotoSSE() {
        if (photoEventSource) { photoEventSource.close(); photoEventSource = null; }
    }

    /** Open a native Electron file/directory picker. Falls back to null if API unavailable. */
    async function showOpenDialog(options) {
        if (window.electronAPI && window.electronAPI.showOpenDialog) {
            const result = await window.electronAPI.showOpenDialog(options);
            if (result && !result.canceled && result.filePaths && result.filePaths.length > 0) {
                return result.filePaths[0];
            }
        }
        return null;
    }

    // ── ZIP / Directory modal ─────────────────────────────────────────────────

    function initZipModal() {
        const modal = document.getElementById('upload-zip-modal');
        const confirmModal = document.getElementById('upload-zip-confirm-modal');
        if (!modal) return;

        const closeBtn      = document.getElementById('close-upload-zip-modal');
        const cancelBtn     = document.getElementById('upload-zip-cancel-btn');
        const startBtn      = document.getElementById('upload-zip-start-btn');
        const filenameEl    = document.getElementById('upload-zip-filename');
        const selectedPathEl = document.getElementById('upload-zip-selected-path');
        const progressWrap  = document.getElementById('upload-zip-progress-wrap');
        const progressBar   = document.getElementById('upload-zip-progress-bar');
        const progressLabel = document.getElementById('upload-zip-progress-label');
        const statusEl      = document.getElementById('upload-zip-status');
        const resultEl      = document.getElementById('upload-zip-result');

        const confirmCloseBtn = document.getElementById('close-upload-zip-confirm-modal');
        const confirmBackBtn  = document.getElementById('upload-zip-confirm-back-btn');
        const confirmStartBtn = document.getElementById('upload-zip-confirm-start-btn');

        // ── Helpers ───────────────────────────────────────────────────────────
        function syncPathFromAnySource() {
            if (!zipFilePath && selectedPathEl && selectedPathEl.value) {
                zipFilePath = selectedPathEl.value;
            }
            if (!zipFilePath && filenameEl && filenameEl.title) {
                zipFilePath = filenameEl.title;
            }
            if (!zipFilePath && typeof window.__uploadImportSelectedPath === 'string' && window.__uploadImportSelectedPath) {
                zipFilePath = window.__uploadImportSelectedPath;
            }
            if (zipFilePath && selectedPathEl && !selectedPathEl.value) {
                selectedPathEl.value = zipFilePath;
            }
        }

        function updateStartButtonState() {
            if (!startBtn) return;
            syncPathFromAnySource();
            startBtn.disabled = zipImporting || !zipFilePath;
        }

        function getSelectedZipArchiveType() {
            const el = document.querySelector('input[name="upload-zip-archive-type"]:checked');
            const value = el && el.value ? el.value : 'facebook';
            const label = ZIP_ARCHIVE_LABELS[value] || 'archive';
            return { value, label };
        }

        function openConfirmModal() { if (confirmModal) confirmModal.style.display = 'flex'; }
        function closeConfirmModal() { if (confirmModal) confirmModal.style.display = 'none'; }

        function resetArchiveTypeRadios() {
            const fb = document.querySelector('input[name="upload-zip-archive-type"][value="facebook"]');
            if (fb) fb.checked = true;
        }

        function resetZipModal() {
            zipFilePath = null;
            zipImporting = false;
            closeZipSSE();
            closeConfirmModal();
            resetArchiveTypeRadios();
            if (filenameEl) { filenameEl.textContent = ''; filenameEl.style.display = 'none'; }
            if (selectedPathEl) selectedPathEl.value = '';
            window.__uploadImportSelectedPath = '';
            if (progressWrap) progressWrap.style.display = 'none';
            if (progressBar) progressBar.style.width = '0%';
            if (statusEl) statusEl.textContent = '';
            if (resultEl) { resultEl.style.display = 'none'; resultEl.textContent = ''; }
            if (startBtn) startBtn.innerHTML = '<i class="fas fa-file-import"></i> Import';
            updateStartButtonState();
        }

        function closeModal() {
            zipImporting = false;
            resetZipModal();
            modal.style.display = 'none';
        }

        function setSelectedPath(p) {
            if (!p) return;
            zipFilePath = p;
            if (selectedPathEl) selectedPathEl.value = p;
            window.__uploadImportSelectedPath = p;
            if (resultEl) resultEl.style.display = 'none';
            if (filenameEl) {
                filenameEl.textContent = getPathDisplayName(p);
                filenameEl.title = p;
                filenameEl.style.display = 'block';
            }
            updateStartButtonState();
        }

        function getPathDisplayName(p) {
            const normalized = String(p || '').replace(/[\\/]+$/, '');
            const parts = normalized.split(/[\\/]/);
            return parts[parts.length - 1] || normalized;
        }

        function clearSelectedPath() {
            zipFilePath = null;
            if (selectedPathEl) selectedPathEl.value = '';
            window.__uploadImportSelectedPath = '';
            if (filenameEl) {
                filenameEl.textContent = '';
                filenameEl.title = '';
                filenameEl.style.display = 'none';
            }
            if (resultEl) resultEl.style.display = 'none';
            updateStartButtonState();
        }

        if (modal) {
            modal.addEventListener('click', (e) => {
                const target = e.target;
                if (!(target instanceof Element)) return;
                if (target.closest('#upload-zip-browse-zip-btn') || target.closest('#upload-zip-browse-dir-btn')) return;
            });
        }
        document.addEventListener('upload-import-path-selected', (e) => {
            const p = e && e.detail && typeof e.detail.path === 'string' ? e.detail.path : '';
            if (p) setSelectedPath(p);
        });
        updateStartButtonState();

        if (closeBtn) closeBtn.addEventListener('click', closeModal);
        if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
        modal.addEventListener('click', (e) => { if (e.target === modal) closeModal(); });

        // ── Start button → confirm modal ──────────────────────────────────────

        if (startBtn) {
            startBtn.addEventListener('click', () => {
                if (zipImporting) return;
                syncPathFromAnySource();
                if (!zipFilePath) {
                    showResult(resultEl, false, 'Please choose a ZIP file or directory first.');
                    return;
                }
                if (resultEl) resultEl.style.display = 'none';
                openConfirmModal();
            });
        }

        if (confirmModal) {
            confirmModal.addEventListener('click', (e) => { if (e.target === confirmModal) closeConfirmModal(); });
        }
        if (confirmCloseBtn) confirmCloseBtn.addEventListener('click', closeConfirmModal);
        if (confirmBackBtn)  confirmBackBtn.addEventListener('click', closeConfirmModal);

        if (confirmStartBtn) {
            confirmStartBtn.addEventListener('click', () => {
                syncPathFromAnySource();
                if (zipImporting || !zipFilePath) return;
                const { value: importType, label: importTypeLabel } = getSelectedZipArchiveType();
                closeConfirmModal();
                beginImport(importType, importTypeLabel);
            });
        }

        // ── Run the import ────────────────────────────────────────────────────

        function beginImport(importType, importTypeLabel) {
            zipImporting = true;
            if (startBtn) {
                startBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Importing…';
            }
            updateStartButtonState();
            if (progressWrap) progressWrap.style.display = 'block';
            if (progressLabel) progressLabel.textContent = 'Starting import…';
            if (statusEl) statusEl.textContent = '';
            if (resultEl) resultEl.style.display = 'none';

            fetch('/import/from-path', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ file_path: zipFilePath, type: importType }),
            }).then(async (res) => {
                if (!res.ok) {
                    const errBody = await res.json().catch(() => ({}));
                    const msg = errBody.error || ('Import request failed: HTTP ' + res.status);
                    zipImporting = false;
                    if (startBtn) startBtn.innerHTML = '<i class="fas fa-file-import"></i> Import';
                    updateStartButtonState();
                    if (progressWrap) progressWrap.style.display = 'none';
                    showResult(resultEl, false, msg);
                    return;
                }
                modal.style.display = 'none';
                resetZipModal();
                if (window.ImportControls && window.ImportControls.attachUploadStream) {
                    window.ImportControls.attachUploadStream(importTypeLabel, importType);
                }
            }).catch((err) => {
                zipImporting = false;
                if (startBtn) startBtn.innerHTML = '<i class="fas fa-file-import"></i> Import';
                updateStartButtonState();
                if (progressWrap) progressWrap.style.display = 'none';
                showResult(resultEl, false, err.message || 'Import request failed.');
            });
        }

        // ── Public API ────────────────────────────────────────────────────────

        function openZipModalWithArchiveType(archiveType) {
            resetZipModal();
            const r = document.querySelector('input[name="upload-zip-archive-type"][value="' + String(archiveType).replace(/"/g, '') + '"]');
            if (r) r.checked = true;
            modal.style.display = 'flex';
        }

        window.UploadImport = window.UploadImport || {};
        window.UploadImport.openZipModalWithArchiveType = openZipModalWithArchiveType;

        document.querySelectorAll('[data-open-modal="upload-zip-modal"]').forEach(el => {
            el.addEventListener('click', () => { resetZipModal(); modal.style.display = 'flex'; });
        });
    }

    // ── Photo import modal ────────────────────────────────────────────────────

    function initPhotosModal() {
        const modal = document.getElementById('upload-photos-modal');
        if (!modal) return;

        const closeBtn      = document.getElementById('close-upload-photos-modal');
        const cancelBtn     = document.getElementById('upload-photos-cancel-btn');
        const backgroundBtn = document.getElementById('upload-photos-background-btn');
        const startBtn      = document.getElementById('upload-photos-start-btn');
        const dropzone      = document.getElementById('upload-photos-dropzone');
        const dropText      = document.getElementById('upload-photos-drop-text');
        const countEl       = document.getElementById('upload-photos-count');
        const progressWrap  = document.getElementById('upload-photos-progress-wrap');
        const progressBar   = document.getElementById('upload-photos-progress-bar');
        const progressLabel = document.getElementById('upload-photos-progress-label');
        const statusEl      = document.getElementById('upload-photos-status');
        const resultEl      = document.getElementById('upload-photos-result');
        const backgroundHint = document.getElementById('upload-photos-background-hint');

        function resetPhotosModal() {
            photoDirPath = null;
            photoImporting = false;
            closePhotoSSE();
            if (backgroundHint) backgroundHint.style.display = 'none';
            modal.classList.remove('upload-photos-modal-busy');
            if (dropText) dropText.style.display = '';
            if (countEl) { countEl.textContent = ''; countEl.style.display = 'none'; }
            if (progressWrap) progressWrap.style.display = 'none';
            if (progressBar) progressBar.style.width = '0%';
            if (statusEl) statusEl.textContent = '';
            if (resultEl) { resultEl.style.display = 'none'; resultEl.textContent = ''; }
            if (startBtn) { startBtn.disabled = false; startBtn.innerHTML = '<i class="fas fa-folder-open"></i> Import Photos'; }
            if (backgroundBtn) backgroundBtn.style.display = 'none';
            if (cancelBtn) cancelBtn.removeAttribute('title');
            if (closeBtn) closeBtn.removeAttribute('title');
        }

        function dismissPhotoModal() {
            if (!photoImporting) resetPhotosModal();
            modal.style.display = 'none';
        }

        function setDirPath(p) {
            if (!p) return;
            photoDirPath = p;
            if (resultEl) resultEl.style.display = 'none';
            if (dropText) dropText.style.display = 'none';
            if (countEl) {
                countEl.textContent = p;
                countEl.style.display = 'block';
            }
        }

        // Dropzone click → native directory picker
        if (dropzone) {
            dropzone.style.cursor = 'pointer';
            dropzone.addEventListener('click', () => {
                if (photoImporting) return;
                void (async () => {
                    const p = await showOpenDialog({
                        title: 'Select photo folder',
                        properties: ['openDirectory'],
                    });
                    if (p) setDirPath(p);
                })();
            });
        }

        if (closeBtn) closeBtn.addEventListener('click', dismissPhotoModal);
        if (cancelBtn) cancelBtn.addEventListener('click', () => {
            if (photoImporting) {
                fetch('/images/import/cancel', { method: 'POST', credentials: 'same-origin' }).catch(() => {});
                closePhotoSSE();
            }
            resetPhotosModal();
            modal.style.display = 'none';
        });
        if (backgroundBtn) backgroundBtn.addEventListener('click', dismissPhotoModal);
        modal.addEventListener('click', (e) => { if (e.target === modal) dismissPhotoModal(); });

        if (startBtn) {
            startBtn.addEventListener('click', () => {
                if (photoImporting) return;
                if (!photoDirPath) {
                    showResult(resultEl, false, 'Please choose a photo folder first.');
                    return;
                }

                photoImporting = true;
                if (startBtn) {
                    startBtn.disabled = true;
                    startBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Importing…';
                }
                if (progressWrap) progressWrap.style.display = 'block';
                if (progressLabel) progressLabel.textContent = 'Starting import…';
                modal.classList.add('upload-photos-modal-busy');
                if (backgroundBtn) backgroundBtn.style.display = '';
                if (resultEl) resultEl.style.display = 'none';

                const includeSubfoldersEl = document.getElementById('upload-photos-include-subfolders');
                const overwriteExistingEl = document.getElementById('upload-photos-overwrite-existing');
                const includeSubfolders = includeSubfoldersEl ? includeSubfoldersEl.checked : true;
                const overwriteExisting = overwriteExistingEl ? overwriteExistingEl.checked : false;

                fetch('/images/import', {
                    method: 'POST',
                    credentials: 'same-origin',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        root_directory: photoDirPath,
                        include_subfolders: includeSubfolders,
                        overwrite_existing: overwriteExisting,
                    }),
                }).then(async (res) => {
                    if (!res.ok) {
                        const errBody = await res.json().catch(() => ({}));
                        const msg = errBody.error || ('Import request failed: HTTP ' + res.status);
                        photoImporting = false;
                        resetPhotosModal();
                        showResult(resultEl, false, msg);
                        modal.style.display = 'flex';
                        return;
                    }
                    photoImporting = false;
                    resetPhotosModal();
                    modal.style.display = 'none';
                    if (window.ImportControls && typeof window.ImportControls.attachFilesystemStream === 'function') {
                        window.ImportControls.attachFilesystemStream();
                    }
                }).catch((err) => {
                    photoImporting = false;
                    resetPhotosModal();
                    showResult(resultEl, false, err.message || 'Import request failed.');
                    modal.style.display = 'flex';
                });
            });
        }

        document.querySelectorAll('[data-open-modal="upload-photos-modal"]').forEach(el => {
            el.addEventListener('click', () => {
                if (!photoImporting) resetPhotosModal();
                modal.style.display = 'flex';
            });
        });
    }

    // ── init ──────────────────────────────────────────────────────────────────

    function init() {
        initZipModal();
        initPhotosModal();
    }

    init();

    return { init };
})();
