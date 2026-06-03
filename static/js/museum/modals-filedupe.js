'use strict';

Modals.FileDupe = (() => {
    let wired = false;
    let abortController = null;
    let currentDuplicates = [];
    let resultMeta = { dir1Files: 0, dir2Files: 0, scanDuration: '' };

    const IMAGE_EXTENSIONS = new Set([
        '.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.heic', '.heif',
    ]);

    const ICON_PREVIEW = '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 5c-5 0-9 4.5-9 7.5S7 20 12 20s9-4.5 9-7.5S17 5 12 5Zm0 12.5c-3.6 0-6.5-2.8-6.5-5S8.4 7.5 12 7.5s6.5 2.8 6.5 5-2.9 5-6.5 5Zm0-8a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z"/></svg>';
    const ICON_FOLDER = '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M10 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7l-2-2Z"/></svg>';

    const phaseLabels = {
        starting: 'Starting',
        scanning_dir1: 'Scanning directory 1',
        scanning_dir2: 'Scanning directory 2',
        comparing: 'Comparing',
        complete: 'Complete',
    };

    function getEl(id) {
        return document.getElementById(id);
    }

    function dir1Input() { return getEl('file-dupe-dir1'); }
    function dir2Input() { return getEl('file-dupe-dir2'); }
    function dir1ExcludeInput() { return getEl('file-dupe-dir1-exclude'); }
    function dir2ExcludeInput() { return getEl('file-dupe-dir2-exclude'); }

    async function browseFolder(targetId, browseBtn) {
        const input = getEl(targetId);
        if (!input) return;

        if (browseBtn) browseBtn.disabled = true;

        try {
            const res = await fetch('/api/filedupe/browse', { method: 'POST' });
            const data = await res.json();
            if (!res.ok) {
                throw new Error(data.error || 'Browse failed');
            }
            if (data.path) {
                input.value = data.path;
            }
        } catch (err) {
            showError((err && err.message) ? err.message : 'Could not open folder picker.');
        } finally {
            if (browseBtn) browseBtn.disabled = false;
        }
    }

    function startScan() {
        const dir1 = dir1Input()?.value.trim() || '';
        const dir2 = dir2Input()?.value.trim() || '';

        if (!dir1 || !dir2) {
            showError('Please specify both directories.');
            return;
        }

        hideError();
        resetResults();
        setScanning(true);
        getEl('file-dupe-progress-section')?.classList.remove('hidden');
        getEl('file-dupe-progress-fill')?.classList.add('indeterminate');
        updateProgress({ phase: 'starting', message: 'Connecting to server...' });

        abortController = new AbortController();

        fetch('/api/filedupe/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                dir1,
                dir2,
                dir1Exclude: parseExcludePatterns(dir1ExcludeInput()?.value || ''),
                dir2Exclude: parseExcludePatterns(dir2ExcludeInput()?.value || ''),
            }),
            signal: abortController.signal,
        })
            .then(async (res) => {
                if (!res.ok) {
                    const text = await res.text();
                    throw new Error(text || `Scan failed (${res.status})`);
                }
                await consumeSSE(res.body);
            })
            .catch((err) => {
                if (err.name === 'AbortError') {
                    updateProgress({ phase: 'complete', message: 'Scan cancelled.' });
                    return;
                }
                showError(err.message);
            })
            .finally(() => {
                setScanning(false);
                getEl('file-dupe-progress-fill')?.classList.remove('indeterminate');
                abortController = null;
            });
    }

    function cancelScan() {
        if (abortController) abortController.abort();
    }

    async function consumeSSE(body) {
        const reader = body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const parts = buffer.split('\n\n');
            buffer = parts.pop() || '';

            for (const part of parts) {
                const line = part.trim();
                if (!line.startsWith('data: ')) continue;
                const event = JSON.parse(line.slice(6));
                handleEvent(event);
            }
        }
    }

    function handleEvent(event) {
        if (event.type === 'error') {
            showError(event.error);
            return;
        }
        if (event.type === 'progress') {
            updateProgress(event);
            return;
        }
        if (event.type === 'complete') {
            updateProgress(event);
            if (event.result) showResults(event.result);
        }
    }

    function updateProgress(event) {
        const phaseEl = getEl('file-dupe-progress-phase');
        const detailEl = getEl('file-dupe-progress-detail');
        const countEl = getEl('file-dupe-progress-count');
        const label = phaseLabels[event.phase] || event.phase || 'Working';
        if (phaseEl) phaseEl.textContent = label;
        if (detailEl) {
            if (event.message) detailEl.textContent = event.message;
            else if (event.currentPath) detailEl.textContent = event.currentPath;
        }
        if (countEl) {
            countEl.textContent = event.filesScanned != null
                ? `${event.filesScanned.toLocaleString()} file(s) scanned`
                : '';
        }
    }

    function showResults(result) {
        getEl('file-dupe-results-section')?.classList.remove('hidden');
        currentDuplicates = result.duplicates || [];
        resultMeta = {
            dir1Files: result.dir1Files || 0,
            dir2Files: result.dir2Files || 0,
            scanDuration: result.scanDuration || '',
        };

        updateResultsSummary();
        const body = getEl('file-dupe-results-body');
        if (body) body.innerHTML = '';
        const selectAll = getEl('file-dupe-select-all');
        if (selectAll) {
            selectAll.checked = false;
            selectAll.indeterminate = false;
        }

        if (currentDuplicates.length === 0) {
            getEl('file-dupe-results-empty')?.classList.remove('hidden');
            getEl('file-dupe-results-table')?.classList.add('hidden');
            getEl('file-dupe-delete-btn')?.classList.add('hidden');
            return;
        }

        getEl('file-dupe-results-empty')?.classList.add('hidden');
        getEl('file-dupe-results-table')?.classList.remove('hidden');
        getEl('file-dupe-delete-btn')?.classList.remove('hidden');
        updateDeleteButton();
        applyResultsFilter();
    }

    function updateResultsSummary() {
        const summaryEl = getEl('file-dupe-results-summary');
        if (!summaryEl) return;
        const dupCount = currentDuplicates.length;
        const shownCount = getVisibleDuplicates().length;
        const shownText = getEl('file-dupe-same-dir-only')?.checked
            ? `${shownCount} shown of ${dupCount}`
            : `${dupCount}`;
        summaryEl.textContent =
            `${shownText} duplicate pair(s) · ` +
            `${resultMeta.dir1Files.toLocaleString()} files in dir 1 · ` +
            `${resultMeta.dir2Files.toLocaleString()} files in dir 2 · ` +
            `${resultMeta.scanDuration}`;
    }

    function getSelectedPaths() {
        return [...document.querySelectorAll('#file-dupe-results-body .file-dupe-file-check:checked')]
            .map((checkbox) => checkbox.dataset.path)
            .filter(Boolean);
    }

    function toggleSelectAll() {
        const selectAll = getEl('file-dupe-select-all');
        const checked = selectAll?.checked || false;
        document.querySelectorAll('#file-dupe-results-body .file-dupe-file-check').forEach((checkbox) => {
            if (!checkbox.disabled) checkbox.checked = checked;
        });
        updateDeleteButton();
        updateAllGroupSelectAllStates();
    }

    function updateSelectAllState() {
        const selectAll = getEl('file-dupe-select-all');
        const boxes = [...document.querySelectorAll('#file-dupe-results-body .file-dupe-file-check')].filter((cb) => !cb.disabled);
        const checked = boxes.filter((cb) => cb.checked);
        if (!selectAll) return;
        if (boxes.length === 0) {
            selectAll.checked = false;
            selectAll.indeterminate = false;
            return;
        }
        selectAll.checked = checked.length === boxes.length;
        selectAll.indeterminate = checked.length > 0 && checked.length < boxes.length;
    }

    function toggleGroupSelection(groupId, dir, checked) {
        const selector = dir === 'dir1' ? '.file-dupe-file-check-dir1' : '.file-dupe-file-check-dir2';
        document.querySelectorAll(`#file-dupe-results-body tr.file-dupe-duplicate-row[data-group-id="${groupId}"] ${selector}`).forEach((checkbox) => {
            if (!checkbox.disabled) checkbox.checked = checked;
        });
        updateDeleteButton();
        updateSelectAllState();
        updateGroupSelectAllState(groupId);
    }

    function updateGroupSelectAllState(groupId) {
        if (!groupId) return;
        updateGroupSelectAllCheckbox(groupId, '.file-dupe-group-select-dir1', '.file-dupe-file-check-dir1');
        updateGroupSelectAllCheckbox(groupId, '.file-dupe-group-select-dir2', '.file-dupe-file-check-dir2');
    }

    function updateGroupSelectAllCheckbox(groupId, groupSelector, fileSelector) {
        const groupCheckbox = document.querySelector(`${groupSelector}[data-group-id="${groupId}"]`);
        if (!groupCheckbox) return;
        const boxes = [...document.querySelectorAll(`#file-dupe-results-body tr.file-dupe-duplicate-row[data-group-id="${groupId}"] ${fileSelector}`)].filter((cb) => !cb.disabled);
        const checked = boxes.filter((cb) => cb.checked);
        if (boxes.length === 0) {
            groupCheckbox.checked = false;
            groupCheckbox.indeterminate = false;
            return;
        }
        groupCheckbox.checked = checked.length === boxes.length;
        groupCheckbox.indeterminate = checked.length > 0 && checked.length < boxes.length;
    }

    function updateAllGroupSelectAllStates() {
        document.querySelectorAll('#file-dupe-results-body .file-dupe-group-header').forEach((header) => {
            updateGroupSelectAllState(header.dataset.groupId);
        });
    }

    function toggleGroupCollapse(groupId) {
        const header = document.querySelector(`#file-dupe-results-body .file-dupe-group-header[data-group-id="${groupId}"]`);
        if (header) setGroupCollapsed(groupId, !header.classList.contains('collapsed'));
    }

    function setGroupCollapsed(groupId, collapsed) {
        const header = document.querySelector(`#file-dupe-results-body .file-dupe-group-header[data-group-id="${groupId}"]`);
        if (!header) return;
        header.classList.toggle('collapsed', collapsed);
        const toggle = header.querySelector('.file-dupe-group-toggle');
        if (toggle) {
            toggle.setAttribute('aria-expanded', String(!collapsed));
            toggle.textContent = collapsed ? '▶' : '▼';
        }
        document.querySelectorAll(`#file-dupe-results-body tr.file-dupe-duplicate-row[data-group-id="${groupId}"]`).forEach((row) => {
            row.classList.toggle('hidden', collapsed);
        });
    }

    function collapseAllGroups() {
        document.querySelectorAll('#file-dupe-results-body .file-dupe-group-header').forEach((header) => {
            setGroupCollapsed(header.dataset.groupId, true);
        });
    }

    function updateDeleteButton() {
        const deleteBtn = getEl('file-dupe-delete-btn');
        if (!deleteBtn) return;
        const selectedCount = getSelectedPaths().length;
        deleteBtn.disabled = selectedCount === 0;
        deleteBtn.textContent = selectedCount === 0
            ? 'Delete Selected'
            : `Delete Selected (${selectedCount})`;
    }

    async function deleteSelected() {
        const paths = getSelectedPaths();
        if (paths.length === 0) return;

        const message = `Delete ${paths.length} selected file(s)? This cannot be undone.`;
        if (!window.confirm(message)) return;

        const deleteBtn = getEl('file-dupe-delete-btn');
        if (deleteBtn) deleteBtn.disabled = true;

        try {
            const res = await fetch('/api/filedupe/delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    dir1: dir1Input()?.value.trim() || '',
                    dir2: dir2Input()?.value.trim() || '',
                    paths,
                }),
            });

            const data = await res.json();
            if (!res.ok && !data.deleted?.length) {
                let errMsg = data.error || 'Delete failed';
                if (data.hint) errMsg += '\n\n' + data.hint;
                throw new Error(errMsg);
            }

            if (data.failed && Object.keys(data.failed).length > 0) {
                let errMsg = 'Some files could not be deleted:\n' +
                    Object.entries(data.failed).map(([path, reason]) => `${path}: ${reason}`).join('\n');
                if (data.hint) errMsg += '\n\n' + data.hint;
                showError(errMsg, data.hint ? 20000 : 8000);
            } else if (data.hint) {
                showError(data.hint, 20000);
            }

            if (data.deleted?.length > 0) {
                startScan();
                return;
            }
            updateDeleteButton();
        } catch (err) {
            showError(err.message);
            updateDeleteButton();
        }
    }

    function renderDuplicateRows() {
        const body = getEl('file-dupe-results-body');
        if (!body) return;
        body.innerHTML = '';

        const groups = groupDuplicatesByDir2Folder(getVisibleDuplicates());
        const sortedKeys = [...groups.keys()].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));

        for (const [index, groupName] of sortedKeys.entries()) {
            const duplicates = groups.get(groupName);
            const groupId = `file-dupe-group-${index}`;

            const header = document.createElement('tr');
            header.className = 'file-dupe-group-header';
            header.dataset.groupId = groupId;
            header.innerHTML = `
                <td class="file-dupe-col-check">
                    <input type="checkbox" class="file-dupe-group-select-dir1" data-group-id="${escapeAttr(groupId)}" title="Select all Directory 1 files in this group" aria-label="Select all Directory 1 files in ${escapeAttr(groupName)}">
                </td>
                <td colspan="3" class="file-dupe-group-title-cell">
                    <button type="button" class="file-dupe-group-toggle" data-group-id="${escapeAttr(groupId)}" aria-expanded="true" aria-label="Toggle ${escapeAttr(groupName)}">▼</button>
                    <span class="file-dupe-group-name">${escapeHtml(groupName)}</span>
                    <span class="file-dupe-group-count">${duplicates.length} duplicate pair(s)</span>
                </td>
                <td class="file-dupe-col-check">
                    <input type="checkbox" class="file-dupe-group-select-dir2" data-group-id="${escapeAttr(groupId)}" title="Select all Directory 2 files in this group" aria-label="Select all Directory 2 files in ${escapeAttr(groupName)}">
                </td>
                <td class="file-dupe-col-path"></td>`;
            body.appendChild(header);

            for (const dup of duplicates) {
                const row = document.createElement('tr');
                row.className = 'file-dupe-duplicate-row';
                row.dataset.groupId = groupId;
                row.innerHTML = `
                    <td class="file-dupe-col-check">
                        <input type="checkbox" class="file-dupe-file-check file-dupe-file-check-dir1" data-path="${escapeAttr(dup.path1)}" aria-label="Select ${escapeAttr(dup.path1)}">
                    </td>
                    <td class="file-dupe-col-filename">${escapeHtml(dup.fileName)}</td>
                    <td class="file-dupe-col-size">${formatSize(dup.size)}</td>
                    <td class="file-dupe-col-path">${renderPathCell(dup.path1, dup.fileName)}</td>
                    <td class="file-dupe-col-check">
                        <input type="checkbox" class="file-dupe-file-check file-dupe-file-check-dir2" data-path="${escapeAttr(dup.path2)}" aria-label="Select ${escapeAttr(dup.path2)}">
                    </td>
                    <td class="file-dupe-col-path">${renderPathCell(dup.path2, dup.fileName)}</td>`;
                body.appendChild(row);
            }
        }

        updateAllGroupSelectAllStates();
        collapseAllGroups();
    }

    function applyResultsFilter() {
        const visible = getVisibleDuplicates();
        updateResultsSummary();

        if (visible.length === 0) {
            const emptyEl = getEl('file-dupe-results-empty');
            if (emptyEl) {
                emptyEl.classList.remove('hidden');
                emptyEl.textContent = getEl('file-dupe-same-dir-only')?.checked
                    ? 'No duplicates match the same-directory filter.'
                    : 'No duplicates found.';
            }
            getEl('file-dupe-results-table')?.classList.add('hidden');
            getEl('file-dupe-delete-btn')?.classList.add('hidden');
            const body = getEl('file-dupe-results-body');
            if (body) body.innerHTML = '';
            updateDeleteButton();
            return;
        }

        getEl('file-dupe-results-empty')?.classList.add('hidden');
        getEl('file-dupe-results-table')?.classList.remove('hidden');
        getEl('file-dupe-delete-btn')?.classList.remove('hidden');
        renderDuplicateRows();
        updateDeleteButton();
    }

    function getVisibleDuplicates() {
        if (!getEl('file-dupe-same-dir-only')?.checked) return currentDuplicates;
        return currentDuplicates.filter((dup) => sameRelativeDirectory(dup.path1, dup.path2));
    }

    function sameRelativeDirectory(path1, path2) {
        return relativeDirectoryKey(path1, dir1Input()?.value.trim() || '') ===
            relativeDirectoryKey(path2, dir2Input()?.value.trim() || '');
    }

    function relativeDirectoryKey(fullPath, rootPath) {
        const full = normalizePath(fullPath);
        const root = normalizePath(rootPath);
        if (root && full.startsWith(root)) {
            let rel = full.slice(root.length).replace(/^[/\\]+/, '');
            if (!rel) return '';
            const parts = rel.split(/[/\\]/);
            parts.pop();
            return parts.join('/');
        }
        return directoryOnly(full);
    }

    function directoryOnly(path) {
        const parts = path.split(/[/\\]/);
        parts.pop();
        return parts.join('/');
    }

    function groupDuplicatesByDir2Folder(duplicates) {
        const groups = new Map();
        for (const dup of duplicates) {
            const folder = getDir2FolderName(dup.path2);
            if (!groups.has(folder)) groups.set(folder, []);
            groups.get(folder).push(dup);
        }
        for (const items of groups.values()) {
            items.sort((a, b) => a.path2.localeCompare(b.path2, undefined, { sensitivity: 'base' }));
        }
        return groups;
    }

    function getDir2FolderName(path2) {
        const root = normalizePath(dir2Input()?.value.trim() || '');
        const full = normalizePath(path2);
        if (root && full.startsWith(root)) {
            let rel = full.slice(root.length).replace(/^[/\\]+/, '');
            if (!rel) return '(root)';
            const parts = rel.split(/[/\\]/);
            if (parts.length === 1) return '(root)';
            return parts[0];
        }
        const parts = full.split(/[/\\]/);
        if (parts.length < 2) return '(root)';
        return parts[parts.length - 2];
    }

    function normalizePath(path) {
        return path.replace(/[/\\]+$/, '').toLowerCase();
    }

    function resetResults() {
        getEl('file-dupe-results-section')?.classList.add('hidden');
        getEl('file-dupe-results-empty')?.classList.add('hidden');
        const emptyEl = getEl('file-dupe-results-empty');
        if (emptyEl) emptyEl.textContent = 'No duplicates found.';
        getEl('file-dupe-results-table')?.classList.add('hidden');
        const body = getEl('file-dupe-results-body');
        if (body) body.innerHTML = '';
        const summaryEl = getEl('file-dupe-results-summary');
        if (summaryEl) summaryEl.textContent = '';
        getEl('file-dupe-delete-btn')?.classList.add('hidden');
        const deleteBtn = getEl('file-dupe-delete-btn');
        if (deleteBtn) {
            deleteBtn.disabled = true;
            deleteBtn.textContent = 'Delete Selected';
        }
        const selectAll = getEl('file-dupe-select-all');
        if (selectAll) {
            selectAll.checked = false;
            selectAll.indeterminate = false;
        }
        const sameDir = getEl('file-dupe-same-dir-only');
        if (sameDir) sameDir.checked = false;
        currentDuplicates = [];
        resultMeta = { dir1Files: 0, dir2Files: 0, scanDuration: '' };
    }

    function setScanning(scanning) {
        getEl('file-dupe-scan-btn') && (getEl('file-dupe-scan-btn').disabled = scanning);
        if (dir1Input()) dir1Input().disabled = scanning;
        if (dir1ExcludeInput()) dir1ExcludeInput().disabled = scanning;
        if (dir2Input()) dir2Input().disabled = scanning;
        if (dir2ExcludeInput()) dir2ExcludeInput().disabled = scanning;
        document.querySelectorAll('.file-dupe-browse-btn').forEach((b) => { b.disabled = scanning; });
        getEl('file-dupe-cancel-btn')?.classList.toggle('hidden', !scanning);
        const deleteBtn = getEl('file-dupe-delete-btn');
        if (deleteBtn) deleteBtn.disabled = scanning || getSelectedPaths().length === 0;
    }

    function parseExcludePatterns(text) {
        return text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    }

    function showError(message, timeoutMs = 8000) {
        const banner = getEl('file-dupe-error-banner');
        if (!banner) return;
        banner.textContent = message;
        banner.classList.remove('hidden');
        setTimeout(hideError, timeoutMs);
    }

    function hideError() {
        getEl('file-dupe-error-banner')?.classList.add('hidden');
    }

    function formatSize(bytes) {
        if (bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        const value = bytes / Math.pow(1024, i);
        return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function escapeAttr(str) {
        return escapeHtml(str).replace(/"/g, '&quot;');
    }

    function isImageFile(fileName) {
        const dot = fileName.lastIndexOf('.');
        if (dot === -1) return false;
        return IMAGE_EXTENSIONS.has(fileName.slice(dot).toLowerCase());
    }

    function renderPathCell(path, fileName) {
        const previewBtn = isImageFile(fileName)
            ? `<button type="button" class="file-dupe-icon-btn file-dupe-preview-btn" data-path="${escapeAttr(path)}" title="View image" aria-label="View image">${ICON_PREVIEW}</button>`
            : '';
        return `
            <div class="file-dupe-path-cell">
                <span class="file-dupe-path-text">${escapeHtml(path)}</span>
                <div class="file-dupe-path-actions">
                    ${previewBtn}
                    <button type="button" class="file-dupe-icon-btn file-dupe-open-path-btn" data-path="${escapeAttr(path)}" title="Open in File Explorer" aria-label="Open in File Explorer">${ICON_FOLDER}</button>
                </div>
            </div>`;
    }

    async function openPathInExplorer(path) {
        if (!path) return;
        try {
            const res = await fetch('/api/filedupe/open-path', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    path,
                    dir1: dir1Input()?.value.trim() || '',
                    dir2: dir2Input()?.value.trim() || '',
                }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || 'Could not open path');
        } catch (err) {
            showError(err.message);
        }
    }

    function buildPreviewUrl(path) {
        const params = new URLSearchParams({
            path,
            dir1: dir1Input()?.value.trim() || '',
            dir2: dir2Input()?.value.trim() || '',
        });
        return `/api/filedupe/preview?${params.toString()}`;
    }

    async function openPreview(path) {
        if (!path) return;
        const modal = getEl('file-dupe-preview-modal');
        const previewImage = getEl('file-dupe-preview-image');
        modal?.classList.remove('hidden');
        const pathEl = getEl('file-dupe-preview-path');
        if (pathEl) pathEl.textContent = path;
        getEl('file-dupe-preview-loading')?.classList.remove('hidden');
        getEl('file-dupe-preview-error')?.classList.add('hidden');
        const errEl = getEl('file-dupe-preview-error');
        if (errEl) errEl.textContent = '';
        previewImage?.classList.add('hidden');
        previewImage?.removeAttribute('src');
        if (previewImage?.dataset.objectUrl) {
            URL.revokeObjectURL(previewImage.dataset.objectUrl);
            delete previewImage.dataset.objectUrl;
        }

        try {
            const res = await fetch(buildPreviewUrl(path));
            if (!res.ok) {
                let message = `Preview failed (${res.status})`;
                try {
                    const data = await res.json();
                    if (data.error) message = data.error;
                } catch {
                    const text = await res.text();
                    if (text) message = text;
                }
                throw new Error(message);
            }
            const blob = await res.blob();
            const objectUrl = URL.createObjectURL(blob);
            if (previewImage) {
                previewImage.dataset.objectUrl = objectUrl;
                previewImage.src = objectUrl;
            }
            getEl('file-dupe-preview-loading')?.classList.add('hidden');
            previewImage?.classList.remove('hidden');
        } catch (err) {
            getEl('file-dupe-preview-loading')?.classList.add('hidden');
            if (errEl) {
                errEl.textContent = err.message || 'Could not load image preview.';
                errEl.classList.remove('hidden');
            }
        }
    }

    function closePreview() {
        const modal = getEl('file-dupe-preview-modal');
        const previewImage = getEl('file-dupe-preview-image');
        modal?.classList.add('hidden');
        previewImage?.removeAttribute('src');
        if (previewImage?.dataset.objectUrl) {
            URL.revokeObjectURL(previewImage.dataset.objectUrl);
            delete previewImage.dataset.objectUrl;
        }
    }

    function init() {
        if (wired) return;
        wired = true;

        getEl('file-dupe-browse-dir1')?.addEventListener('click', (event) => {
            const btn = event.currentTarget;
            void browseFolder(btn?.dataset?.target || 'file-dupe-dir1', btn);
        });
        getEl('file-dupe-browse-dir2')?.addEventListener('click', (event) => {
            const btn = event.currentTarget;
            void browseFolder(btn?.dataset?.target || 'file-dupe-dir2', btn);
        });
        getEl('file-dupe-scan-btn')?.addEventListener('click', startScan);
        getEl('file-dupe-cancel-btn')?.addEventListener('click', cancelScan);
        getEl('file-dupe-delete-btn')?.addEventListener('click', () => { void deleteSelected(); });
        getEl('file-dupe-select-all')?.addEventListener('change', toggleSelectAll);
        getEl('file-dupe-same-dir-only')?.addEventListener('change', applyResultsFilter);

        const resultsBody = getEl('file-dupe-results-body');
        resultsBody?.addEventListener('change', (event) => {
            const target = event.target;
            if (target.classList.contains('file-dupe-file-check')) {
                updateDeleteButton();
                updateSelectAllState();
                updateGroupSelectAllState(target.closest('tr.file-dupe-duplicate-row')?.dataset.groupId);
                return;
            }
            if (target.classList.contains('file-dupe-group-select-dir1')) {
                toggleGroupSelection(target.dataset.groupId, 'dir1', target.checked);
                return;
            }
            if (target.classList.contains('file-dupe-group-select-dir2')) {
                toggleGroupSelection(target.dataset.groupId, 'dir2', target.checked);
            }
        });

        resultsBody?.addEventListener('click', (event) => {
            const previewBtn = event.target.closest('.file-dupe-preview-btn');
            if (previewBtn) {
                void openPreview(previewBtn.dataset.path);
                return;
            }
            const openPathBtn = event.target.closest('.file-dupe-open-path-btn');
            if (openPathBtn) {
                void openPathInExplorer(openPathBtn.dataset.path);
                return;
            }
            if (event.target.closest('.file-dupe-group-select-dir1, .file-dupe-group-select-dir2, .file-dupe-file-check')) return;
            const titleCell = event.target.closest('.file-dupe-group-title-cell');
            if (titleCell) {
                const header = titleCell.closest('.file-dupe-group-header');
                if (header) {
                    toggleGroupCollapse(header.dataset.groupId);
                    return;
                }
            }
            const toggle = event.target.closest('.file-dupe-group-toggle');
            if (toggle) toggleGroupCollapse(toggle.dataset.groupId);
        });

        document.querySelectorAll('[data-file-dupe-close-preview]').forEach((el) => {
            el.addEventListener('click', closePreview);
        });

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape' && !getEl('file-dupe-preview-modal')?.classList.contains('hidden')) {
                closePreview();
            }
        });
    }

    return { init };
})();

Modals.FileDupe.init();
