'use strict';

/**
 * Sensitive & Private Data Gallery Modal
 *
 * Details are stored base64-encoded on the server. The table is always visible
 * but shows encoded (unreadable) text until the user enters any password and
 * clicks Unlock. The password is sent with every mutating request.
 */

Modals.SensitiveData = (() => {
    let _currentId = null;   // null = new record
    let _password  = null;   // last used password (kept for save/delete)

    function _normKeyringPw(s) {
        return (s == null ? '' : String(s)).trim().toLowerCase();
    }

    // -------------------------------------------------------------------------
    // Gallery Modal
    // -------------------------------------------------------------------------

    async function open() {
        try {
            const resp = await fetch('/sensitive-data/key-count');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            if ((data.count || 0) === 0) {
                _showNoMasterKeyModal();
                return;
            }
        } catch (err) {
            console.error('[SensitiveData] key-count error:', err);
            alert('Unable to check trusted keys. Please try again.');
            return;
        }
        const modal = document.getElementById('sensitive-data-modal');
        if (modal) modal.style.display = 'flex';
        _loadCount();
        _loadRecords();
    }

    function close() {
        const modal = document.getElementById('sensitive-data-modal');
        if (modal) modal.style.display = 'none';
        const pwInput = document.getElementById('sensitive-data-password-input');
        if (pwInput) pwInput.value = '';
        _password = null;
    }

    async function _loadCount() {
        try {
            const resp = await fetch('/sensitive-data/count');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            const badge = document.getElementById('sensitive-data-count-badge');
            if (badge) badge.textContent = `${data.count} record${data.count !== 1 ? 's' : ''}`;
        } catch (err) {
            console.error('[SensitiveData] count error:', err);
        }
    }

    async function _loadRecords(password = null) {
        const tbody = document.getElementById('sensitive-data-table-body');
        if (!tbody) return;
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;padding:2rem;color:#666;">Loading...</td></tr>';

        try {
            let url = '/sensitive-data';
            if (password) url += '?password=' + encodeURIComponent(password);
            const resp = await fetch(url);
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const records = await resp.json();
            _renderTable(records);
        } catch (err) {
            console.error('[SensitiveData] load error:', err);
            tbody.innerHTML = `<tr><td colspan="5" style="text-align:center;padding:2rem;color:#dc3545;">Failed to load records: ${_esc(err.message)}</td></tr>`;
        }
    }

    function _renderTable(records) {
        const tbody = document.getElementById('sensitive-data-table-body');
        if (!tbody) return;

        if (!records || records.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;padding:2rem;color:#666;">No records found.</td></tr>';
            return;
        }

        tbody.innerHTML = '';
        records.forEach(record => {
            const tr = document.createElement('tr');
            tr.onclick = () => _openDetail(record);

            const createdAt = record.created_at
                ? new Date(record.created_at).toLocaleDateString('en-AU')
                : '';

            tr.innerHTML = `
                <td>${_esc(record.description)}</td>
                <td style="font-family:monospace; font-size:12px;">${_esc(record.details || '')}</td>
                <td style="text-align:center;">${record.is_private  ? '&#10003;' : ''}</td>
                <td style="text-align:center;">${record.is_sensitive ? '&#10003;' : ''}</td>
                <td>${createdAt}</td>
            `;
            tbody.appendChild(tr);
        });
    }

    function _unlock() {
        const pw = document.getElementById('sensitive-data-password-input');
        const val = _normKeyringPw(pw ? pw.value : '');
        _password = val || null;
        _loadRecords(_password);
    }

    async function _showHints() {
        try {
            const resp = await fetch('/sensitive-data/hints');
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const data = await resp.json();
            const hints = data.hints || [];

            const listEl = document.getElementById('sensitive-data-hints-list');
            if (!listEl) return;

            if (hints.length === 0) {
                listEl.innerHTML = '<li style="color:#666;">No hints available.</li>';
            } else {
                listEl.innerHTML = hints.map(h => `<li>${_esc(h)}</li>`).join('');
            }

            const modal = document.getElementById('sensitive-data-hints-modal');
            if (modal) modal.style.display = 'flex';
        } catch (err) {
            console.error('[SensitiveData] hints error:', err);
            alert('Failed to load hints: ' + err.message);
        }
    }

    function _closeHintsModal() {
        const modal = document.getElementById('sensitive-data-hints-modal');
        if (modal) modal.style.display = 'none';
    }

    function _showNoMasterKeyModal() {
        const modal = document.getElementById('sensitive-data-no-master-key-modal');
        const pwInput = document.getElementById('sensitive-data-master-key-password');
        const confirmInput = document.getElementById('sensitive-data-master-key-confirm');
        const errorEl = document.getElementById('sensitive-data-no-master-key-error');
        if (modal) modal.style.display = 'flex';
        if (pwInput) pwInput.value = '';
        if (confirmInput) confirmInput.value = '';
        if (errorEl) {
            errorEl.textContent = '';
            errorEl.style.display = 'none';
        }
    }

    function _closeNoMasterKeyModal() {
        const modal = document.getElementById('sensitive-data-no-master-key-modal');
        if (modal) modal.style.display = 'none';
    }

    async function _createMasterKey() {
        const pwInput = document.getElementById('sensitive-data-master-key-password');
        const confirmInput = document.getElementById('sensitive-data-master-key-confirm');
        const errorEl = document.getElementById('sensitive-data-no-master-key-error');
        if (!pwInput) return;
        const password = _normKeyringPw(pwInput.value);
        const confirm = _normKeyringPw(confirmInput ? confirmInput.value : '');
        if (!password) {
            if (errorEl) {
                errorEl.textContent = 'Please enter a password.';
                errorEl.style.display = 'block';
            }
            return;
        }
        if (password !== confirm) {
            if (errorEl) {
                errorEl.textContent = 'Passwords do not match.';
                errorEl.style.display = 'block';
            }
            return;
        }
        if (errorEl) {
            errorEl.textContent = '';
            errorEl.style.display = 'none';
        }
        try {
            const resp = await fetch('/sensitive-data/master-key', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || `HTTP ${resp.status}`);
            }
            _closeNoMasterKeyModal();
            const modal = document.getElementById('sensitive-data-modal');
            if (modal) modal.style.display = 'flex';
            _loadCount();
            _loadRecords();
        } catch (err) {
            console.error('[SensitiveData] create master key error:', err);
            if (errorEl) {
                errorEl.textContent = err.message || 'Failed to create master key.';
                errorEl.style.display = 'block';
            }
        }
    }

    // -------------------------------------------------------------------------
    // Details tab: Edit / Preview
    // -------------------------------------------------------------------------

    function _switchTab(tab) {
        const textarea  = document.getElementById('sd-detail-details');
        const preview   = document.getElementById('sd-detail-preview');
        const tabBtns   = document.querySelectorAll('.sd-tab-btn');

        tabBtns.forEach(btn => btn.classList.toggle('active', btn.dataset.tab === tab));

        if (tab === 'preview') {
            const text = textarea ? textarea.value : '';
            if (preview) {
                preview.innerHTML = (typeof marked !== 'undefined')
                    ? marked.parse(text || '')
                    : _esc(text);
                preview.style.display = 'block';
            }
            if (textarea) textarea.style.display = 'none';
        } else {
            if (preview)  preview.style.display  = 'none';
            if (textarea) textarea.style.display = 'block';
        }
    }

    // -------------------------------------------------------------------------
    // Detail Modal
    // -------------------------------------------------------------------------

    async function _openDetail(record) {
        if (!_password) {
            alert('Enter a password and click Unlock to view record details.');
            return;
        }
        _currentId = record.id;
        _showDetailModal();

        const titleEl = document.getElementById('sensitive-data-detail-title');
        if (titleEl) titleEl.textContent = 'Loading...';
        _setField('sd-detail-description', '');
        _setField('sd-detail-details', '');
        const deleteBtn = document.getElementById('sd-detail-delete-btn');
        if (deleteBtn) deleteBtn.style.display = 'inline-block';

        try {
            const url = `/sensitive-data/${record.id}?password=${encodeURIComponent(_password)}`;
            const resp = await fetch(url);
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({ detail: resp.statusText }));
                throw new Error(err.detail || resp.statusText);
            }
            const data = await resp.json();

            if (titleEl) titleEl.textContent = 'Sensitive Recort: '+data.description || 'Record Details';
            _setField('sd-detail-description', data.description || '');
            _setField('sd-detail-details', data.details || '');

            const privateEl   = document.getElementById('sd-detail-is-private');
            const sensitiveEl = document.getElementById('sd-detail-is-sensitive');
            if (privateEl)   privateEl.checked   = !!data.is_private;
            if (sensitiveEl) sensitiveEl.checked = !!data.is_sensitive;

            _switchTab('preview');
        } catch (err) {
            console.error('[SensitiveData] fetch record error:', err);
            alert('Failed to load record: ' + err.message);
            _closeDetailModal();
        }
    }

    function _openNew() {
        if (!_password) {
            alert('Enter a password and click Unlock to create a new record.');
            return;
        }
        _currentId = null;

        const titleEl = document.getElementById('sensitive-data-detail-title');
        if (titleEl) titleEl.textContent = 'New Record';

        _setField('sd-detail-description', '');
        _setField('sd-detail-details',     '');

        const privateEl   = document.getElementById('sd-detail-is-private');
        const sensitiveEl = document.getElementById('sd-detail-is-sensitive');
        if (privateEl)   privateEl.checked   = true;
        if (sensitiveEl) sensitiveEl.checked = true;

        const deleteBtn = document.getElementById('sd-detail-delete-btn');
        if (deleteBtn) deleteBtn.style.display = 'none';

        _switchTab('edit');
        _showDetailModal();
    }

    function _showDetailModal() {
        const modal = document.getElementById('sensitive-data-detail-modal');
        if (modal) modal.style.display = 'flex';
    }

    function _closeDetailModal() {
        const modal = document.getElementById('sensitive-data-detail-modal');
        if (modal) modal.style.display = 'none';
        _currentId = null;
    }

    async function _saveRecord() {
        const description = document.getElementById('sd-detail-description')?.value.trim();
        const details     = document.getElementById('sd-detail-details')?.value;
        let is_private  = document.getElementById('sd-detail-is-private')?.checked  || false;
        let is_sensitive= document.getElementById('sd-detail-is-sensitive')?.checked || false;

        if (!is_private && !is_sensitive) {
            is_sensitive = true;
        }

        if (is_sensitive) {
            is_private = true;
        }

        if (!description) {
            alert('Description is required.');
            return;
        }

        const body = {
            description,
            details,
            is_private,
            is_sensitive,
            password: _password || '',
        };

        try {
            let resp;
            if (_currentId === null) {
                resp = await fetch('/sensitive-data', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
            } else {
                resp = await fetch(`/sensitive-data/${_currentId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                });
            }

            if (resp.status === 403) {
                alert('A password is required to save records. Enter a password and click Unlock first.');
                return;
            }
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({ detail: resp.statusText }));
                throw new Error(err.detail || resp.statusText);
            }

            _closeDetailModal();
            _loadCount();
            _loadRecords(_password);
        } catch (err) {
            console.error('[SensitiveData] save error:', err);
            alert('Error saving record: ' + err.message);
        }
    }

    async function _deleteRecord() {
        if (!_currentId) return;
        if (!confirm('Delete this record? This cannot be undone.')) return;

        try {
            const url = `/sensitive-data/${_currentId}` +
                (_password ? '?password=' + encodeURIComponent(_password) : '');
            const resp = await fetch(url, { method: 'DELETE' });

            if (resp.status === 403) {
                alert('A password is required to delete records. Enter a password and click Unlock first.');
                return;
            }
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({ detail: resp.statusText }));
                throw new Error(err.detail || resp.statusText);
            }

            _closeDetailModal();
            _loadCount();
            _loadRecords(_password);
        } catch (err) {
            console.error('[SensitiveData] delete error:', err);
            alert('Error deleting record: ' + err.message);
        }
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    function _setField(id, value) {
        const el = document.getElementById(id);
        if (el) el.value = value;
    }

    function _esc(str) {
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // -------------------------------------------------------------------------
    // Initialisation
    // -------------------------------------------------------------------------

    function init() {
        const closeBtn = document.getElementById('close-sensitive-data-modal');
        if (closeBtn) closeBtn.addEventListener('click', close);

        const galleryModal = document.getElementById('sensitive-data-modal');
        if (galleryModal) {
            galleryModal.addEventListener('click', e => {
                if (e.target === galleryModal) close();
            });
        }

        const unlockBtn = document.getElementById('sensitive-data-unlock-btn');
        if (unlockBtn) unlockBtn.addEventListener('click', _unlock);

        const hintBtn = document.getElementById('sensitive-data-hint-btn');
        if (hintBtn) hintBtn.addEventListener('click', _showHints);

        const hintsCloseBtn = document.getElementById('close-sensitive-data-hints');
        if (hintsCloseBtn) hintsCloseBtn.addEventListener('click', _closeHintsModal);

        const hintsModal = document.getElementById('sensitive-data-hints-modal');
        if (hintsModal) {
            hintsModal.addEventListener('click', e => {
                if (e.target === hintsModal) _closeHintsModal();
            });
        }

        const noMasterKeyCloseBtn = document.getElementById('close-sensitive-data-no-master-key');
        if (noMasterKeyCloseBtn) noMasterKeyCloseBtn.addEventListener('click', _closeNoMasterKeyModal);

        const noMasterKeyCancelBtn = document.getElementById('sensitive-data-no-master-key-cancel');
        if (noMasterKeyCancelBtn) noMasterKeyCancelBtn.addEventListener('click', _closeNoMasterKeyModal);

        const createMasterKeyBtn = document.getElementById('sensitive-data-create-master-key-btn');
        if (createMasterKeyBtn) createMasterKeyBtn.addEventListener('click', _createMasterKey);

        const noMasterKeyModal = document.getElementById('sensitive-data-no-master-key-modal');
        if (noMasterKeyModal) {
            noMasterKeyModal.addEventListener('click', e => {
                if (e.target === noMasterKeyModal) _closeNoMasterKeyModal();
            });
        }

        const masterKeyPwInput = document.getElementById('sensitive-data-master-key-password');
        const masterKeyConfirmInput = document.getElementById('sensitive-data-master-key-confirm');
        const pwEnterHandler = e => { if (e.key === 'Enter') _createMasterKey(); };
        if (masterKeyPwInput) masterKeyPwInput.addEventListener('keydown', pwEnterHandler);
        if (masterKeyConfirmInput) masterKeyConfirmInput.addEventListener('keydown', pwEnterHandler);

        const pwToggle = document.getElementById('sensitive-data-master-key-password-toggle');
        if (pwToggle) {
            pwToggle.addEventListener('click', () => {
                const inp = document.getElementById('sensitive-data-master-key-password');
                if (!inp) return;
                const isPassword = inp.type === 'password';
                inp.type = isPassword ? 'text' : 'password';
                pwToggle.innerHTML = isPassword ? '<i class="fas fa-eye-slash"></i>' : '<i class="fas fa-eye"></i>';
                pwToggle.title = isPassword ? 'Hide password' : 'Show password';
            });
        }
        const confirmToggle = document.getElementById('sensitive-data-master-key-confirm-toggle');
        if (confirmToggle) {
            confirmToggle.addEventListener('click', () => {
                const inp = document.getElementById('sensitive-data-master-key-confirm');
                if (!inp) return;
                const isPassword = inp.type === 'password';
                inp.type = isPassword ? 'text' : 'password';
                confirmToggle.innerHTML = isPassword ? '<i class="fas fa-eye-slash"></i>' : '<i class="fas fa-eye"></i>';
                confirmToggle.title = isPassword ? 'Hide password' : 'Show password';
            });
        }

        const pwInput = document.getElementById('sensitive-data-password-input');
        if (pwInput) {
            pwInput.addEventListener('keydown', e => {
                if (e.key === 'Enter') _unlock();
            });
        }

        const newBtn = document.getElementById('sensitive-data-new-btn');
        if (newBtn) newBtn.addEventListener('click', _openNew);

        document.querySelectorAll('.sd-tab-btn').forEach(btn => {
            btn.addEventListener('click', () => _switchTab(btn.dataset.tab));
        });

        const detailCloseBtn = document.getElementById('close-sensitive-data-detail');
        if (detailCloseBtn) detailCloseBtn.addEventListener('click', _closeDetailModal);

        const detailModal = document.getElementById('sensitive-data-detail-modal');
        if (detailModal) {
            detailModal.addEventListener('click', e => {
                if (e.target === detailModal) _closeDetailModal();
            });
        }

        const saveBtn = document.getElementById('sd-detail-save-btn');
        if (saveBtn) saveBtn.addEventListener('click', _saveRecord);

        const deleteBtn = document.getElementById('sd-detail-delete-btn');
        if (deleteBtn) deleteBtn.addEventListener('click', _deleteRecord);
    }

    return { open, close, init };
})();
