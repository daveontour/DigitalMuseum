'use strict';

Modals.Contacts = (() => {
        const selectedIds = new Set();
        let sortColumn = 'name';
        let sortOrder = 'asc';
        let profileNamesSet = new Set();

        function updateDeleteSelectedUI() {
            const count = selectedIds.size;
            if (DOM.contactsDeleteSelectedBtn) {
                DOM.contactsDeleteSelectedBtn.style.display = count > 0 ? '' : 'none';
            }
            if (DOM.contactsSelectedCount) {
                DOM.contactsSelectedCount.textContent = count;
            }
        }

        function updateSortIndicators() {
            document.querySelectorAll('.contacts-sortable-header').forEach(th => {
                const col = th.dataset.sort;
                th.classList.remove('contacts-sort-asc', 'contacts-sort-desc');
                if (col === sortColumn) {
                    th.classList.add(sortOrder === 'asc' ? 'contacts-sort-asc' : 'contacts-sort-desc');
                }
            });
        }

        function onSortHeaderClick(e) {
            const th = e.target.closest('.contacts-sortable-header');
            if (!th) return;
            const col = th.dataset.sort;
            if (!col) return;
            if (sortColumn === col) {
                sortOrder = sortOrder === 'asc' ? 'desc' : 'asc';
            } else {
                sortColumn = col;
                sortOrder = 'desc';
            }
            loadContacts();
        }

        async function loadContacts() {
            if (!DOM.contactsLoading || !DOM.contactsTableContainer || !DOM.contactsTableBody) return;
            DOM.contactsLoading.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Loading contacts...';
            DOM.contactsLoading.style.display = 'block';
            DOM.contactsTableContainer.style.display = 'none';
            try {
                const hasMessagesOnly = DOM.contactsHasMessagesOnly && DOM.contactsHasMessagesOnly.checked;
                const emailContainsAt = DOM.contactsEmailContainsAt && DOM.contactsEmailContainsAt.checked;
                const excludePhoneNumbers = DOM.contactsExcludePhoneNumbers && DOM.contactsExcludePhoneNumbers.checked;
                const searchTerm = DOM.contactsSearch ? DOM.contactsSearch.value.trim() : '';
                const url = new URL('/contacts', window.location.origin);
                url.searchParams.set('limit', '99999');
                url.searchParams.set('offset', '0');
                url.searchParams.set('order_by', sortColumn);
                url.searchParams.set('order', sortOrder);
                if (hasMessagesOnly) url.searchParams.set('has_messages', 'true');
                if (emailContainsAt) url.searchParams.set('email_contains_at', 'true');
                if (excludePhoneNumbers) url.searchParams.set('exclude_phone_numbers', 'true');
                if (searchTerm) url.searchParams.set('search', searchTerm);
                const [contactsResponse, profileNamesResponse] = await Promise.all([
                    fetch(url.toString()),
                    fetch('/chat/complete-profile/names')
                ]);
                if (!contactsResponse.ok) throw new Error('Failed to load contacts');
                const data = await contactsResponse.json();
                const contacts = data.contacts || [];

                if (profileNamesResponse.ok) {
                    const profileData = await profileNamesResponse.json();
                    profileNamesSet = new Set((profileData.names || []).map(n => String(n).trim()));
                } else {
                    profileNamesSet = new Set();
                }

                DOM.contactsTableBody.innerHTML = '';
                contacts.forEach(c => {

                    const row = document.createElement('tr');
                    row.style.borderBottom = '1px solid #eee';
                    row.dataset.contactId = c.id;
                    const canSelect = c.id !== 0;
                    const cb = canSelect ? `<input type="checkbox" class="contacts-row-cb" data-contact-id="${c.id}">` : '';
                    const contactName = c.name || '';
                    const hasProfile = contactName && profileNamesSet.has(contactName);
                    const totalMessages = (c.numemail || 0) + (c.numsms || 0) + (c.numwhatsapp || 0) + (c.numimessages || 0) + (c.numinstagram || 0) + (c.numfacebook || 0);
                    const runProfileBtn = contactName && totalMessages > 10 ? `<button type="button" class="contacts-run-profile-btn contacts-generate-profile-btn modal-btn modal-btn-secondary" data-contact-name="${escapeHtml(contactName)}" title="Generate complete profile">Generate Profile</button>` : '';
                    const viewProfileBtn = hasProfile ? `<button type="button" class="contacts-view-profile-btn modal-btn modal-btn-primary" data-contact-name="${escapeHtml(contactName)}" title="View complete profile"><i class="fas fa-id-card"></i></button>` : '';
                    const actionBtns = [viewProfileBtn, runProfileBtn].filter(Boolean).join(' ');
                    row.innerHTML = `
                        <td class="contacts-col-select">${cb}</td>
                        <td class="contacts-col-name">${escapeHtml(contactName)}</td>
                        <td class="contacts-col-email">${escapeHtml(c.email || '-')}</td>
                        <td class="contacts-col-count">${renderEmailCountCell(c.numemail, contactName)}</td>
                        <td class="contacts-col-count">${renderCountCell(c.numsms, contactName)}</td>
                        <td class="contacts-col-count">${renderCountCell(c.numwhatsapp, contactName)}</td>
                        <td class="contacts-col-count">${renderCountCell(c.numimessages, contactName)}</td>
                        <td class="contacts-col-count">${renderCountCell(c.numinstagram, contactName)}</td>
                        <td class="contacts-col-count">${renderCountCell(c.numfacebook, contactName)}</td>
                        <td class="contacts-col-actions">${actionBtns}</td>
                    `;
                    DOM.contactsTableBody.appendChild(row);
                });

                DOM.contactsTableBody.querySelectorAll('.contacts-row-cb').forEach(el => {
                    el.addEventListener('change', (e) => {
                        const id = parseInt(e.target.dataset.contactId, 10);
                        if (e.target.checked) selectedIds.add(id);
                        else selectedIds.delete(id);
                        updateDeleteSelectedUI();
                    });
                    if (selectedIds.has(parseInt(el.dataset.contactId, 10))) el.checked = true;
                });

                DOM.contactsTableBody.querySelectorAll('.contacts-run-profile-btn').forEach(btn => {
                    btn.addEventListener('click', (e) => handleRunCompleteProfile(e));
                });
                DOM.contactsTableBody.querySelectorAll('.contacts-view-profile-btn').forEach(btn => {
                    btn.addEventListener('click', (e) => handleViewCompleteProfile(e));
                });

                if (DOM.contactsSelectAll) {
                    DOM.contactsSelectAll.checked = false;
                    DOM.contactsSelectAll.indeterminate = false;
                }

                updateDeleteSelectedUI();
                updateSortIndicators();

                DOM.contactsLoading.style.display = 'none';
                DOM.contactsTableContainer.style.display = 'flex';
            } catch (err) {
                console.error('Error loading contacts:', err);
                DOM.contactsLoading.innerHTML = `<span style="color: #c00;">Error: ${err.message}</span>`;
                DOM.contactsLoading.style.display = 'block';
                DOM.contactsTableContainer.style.display = 'none';
            }
        }

        async function handleDeleteSelected() {
            const ids = Array.from(selectedIds);
            if (ids.length === 0) return;
            if (ids.length > 1) {
                const ok = await AppDialogs.showAppConfirm(
                    'Delete contacts',
                    `Delete ${ids.length} contacts? This cannot be undone.`,
                    { danger: true }
                );
                if (!ok) return;
            }
            try {
                const response = await fetch('/contacts/bulk-delete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ ids })
                });
                if (!response.ok) {
                    const err = await response.json().catch(() => ({}));
                    throw new Error(err.detail || `HTTP ${response.status}`);
                }
                ids.forEach(id => selectedIds.delete(id));
                await loadContacts();
            } catch (err) {
                console.error('Error deleting contacts:', err);
                await AppDialogs.showAppAlert('Error', `${err.message}`);
            }
        }
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function renderCountCell(value, contactName) {
            const isClickable = typeof value === 'number' && value > 0;
            const display = (typeof value === 'number' && value > 0) ? String(value) : '';
            if (isClickable) {
                return `<span class="contacts-count-link" data-contact-name="${escapeHtml(contactName || '')}" title="View messages">${display}</span>`;
            }
            return escapeHtml(display);
        }

        function renderEmailCountCell(value, contactName) {
            const isClickable = typeof value === 'number' && value > 0;
            const display = (typeof value === 'number' && value > 0) ? String(value) : '';
            if (isClickable) {
                return `<span class="contacts-email-count-link" data-contact-name="${escapeHtml(contactName || '')}" title="View emails">${display}</span>`;
            }
            return escapeHtml(display);
        }

        function onCountCellClick(e) {
            const link = e.target.closest('.contacts-count-link');
            if (link) {
                const contactName = link.dataset.contactName;
                if (contactName && Modals.SMSMessages && Modals.SMSMessages.openWithFilter) {
                    Modals.Contacts.close();
                    Modals.SMSMessages.openWithFilter(contactName);
                }
                return;
            }
            const emailLink = e.target.closest('.contacts-email-count-link');
            if (emailLink) {
                const contactName = emailLink.dataset.contactName;
                if (contactName && Modals.EmailGallery && Modals.EmailGallery.openContact) {
                    Modals.Contacts.close();
                    Modals.EmailGallery.openContact(contactName);
                }
            }
        }

        async function handleRunCompleteProfile(e) {
            const btn = e.target.closest('.contacts-run-profile-btn');
            if (!btn) return;
            const name = btn.dataset.contactName;
            if (!name) return;
            btn.disabled = true;
            btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';
            try {
                const provider = (typeof DOM !== 'undefined' && DOM.llmProviderSelect?.value) ? DOM.llmProviderSelect.value : '';
                const resp = await fetch('/chat/complete-profile', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ full_name: name, provider })
                });
                if (!resp.ok) {
                    const err = await resp.json().catch(() => ({}));
                    throw new Error(err.detail || `HTTP ${resp.status}`);
                }
                const data = await resp.json();
                await AppDialogs.showAppAlert(
                    'Profile',
                    data.message || 'Complete profile generation started. This runs in the background and may take several minutes.'
                );
                if (typeof Modals !== 'undefined' && Modals.Profiles && typeof Modals.Profiles.refreshProfilesTable === 'function') {
                    Modals.Profiles.refreshProfilesTable();
                }
                if (typeof Modals !== 'undefined' && Modals.Contacts && typeof Modals.Contacts.reloadContactsPage === 'function') {
                    Modals.Contacts.reloadContactsPage();
                }
            } catch (err) {
                console.error('Run complete profile error:', err);
                await AppDialogs.showAppAlert('Error', err.message);
            } finally {
                btn.disabled = false;
                btn.innerHTML = '<i class="fas fa-sync-alt"></i>';
            }
        }

        let currentProfileName = '';
        let currentProfileText = '';

        function switchCompleteProfileTab(tab) {
            const profileModal = document.getElementById('complete-profile-modal');
            if (tab === 'edit' && profileModal && profileModal.dataset.profilePending === 'true') {
                AppDialogs.showAppAlert('Profile', 'This profile is still being generated. Try again in a few minutes.');
                return;
            }
            const viewPane = document.getElementById('complete-profile-view-pane');
            const editPane = document.getElementById('complete-profile-edit-pane');
            document.querySelectorAll('.complete-profile-tab').forEach(btn => {
                const on = btn.dataset.tab === tab;
                btn.classList.toggle('active', on);
                btn.setAttribute('aria-selected', on ? 'true' : 'false');
            });
            if (tab === 'view') {
                if (viewPane) viewPane.style.display = 'block';
                if (editPane) editPane.style.display = 'none';
                const contentEl = document.getElementById('complete-profile-content');
                if (contentEl && typeof marked !== 'undefined' && currentProfileText) {
                    contentEl.innerHTML = marked.parse(currentProfileText);
                } else if (contentEl) {
                    contentEl.textContent = currentProfileText || '(No profile content)';
                }
            } else {
                if (viewPane) viewPane.style.display = 'none';
                if (editPane) editPane.style.display = 'block';
                const textarea = document.getElementById('complete-profile-edit-textarea');
                if (textarea) textarea.value = currentProfileText;
            }
        }

        async function handleSaveCompleteProfile() {
            const name = currentProfileName;
            const textarea = document.getElementById('complete-profile-edit-textarea');
            const errEl = document.getElementById('complete-profile-save-error');
            if (!name) return;
            const profileText = textarea ? textarea.value : '';
            if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
            try {
                const resp = await fetch('/chat/complete-profile', {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name, profile: profileText })
                });
                if (!resp.ok) {
                    const err = await resp.json().catch(() => ({}));
                    throw new Error(err.detail || `HTTP ${resp.status}`);
                }
                currentProfileText = profileText;
                switchCompleteProfileTab('view');
            } catch (err) {
                console.error('Save complete profile error:', err);
                if (errEl) { errEl.textContent = err.message; errEl.style.display = 'block'; }
            }
        }

        async function _openProfileByName(name, silentOn404 = false) {
            if (!name) return false;
            try {
                const resp = await fetch('/chat/complete-profile?name=' + encodeURIComponent(name));
                if (!resp.ok) {
                    if (resp.status === 404 && silentOn404) return false;
                    if (resp.status === 404) {
                        await AppDialogs.showAppAlert('No complete profile found for this contact. Generate one first.');
                        return false;
                    }
                    throw new Error('Failed to load profile');
                }
                const data = await resp.json();
                currentProfileName = data.name || name;
                currentProfileText = data.profile || '';
                const modal = document.getElementById('complete-profile-modal');
                const titleEl = document.getElementById('complete-profile-modal-title');
                const contentEl = document.getElementById('complete-profile-content');
                if (modal) {
                    modal.dataset.profilePending = data.pending ? 'true' : 'false';
                }
                if (titleEl) titleEl.textContent = 'Complete Profile: ' + currentProfileName;
                if (contentEl) {
                    if (data.pending) {
                        contentEl.innerHTML = '<p class="complete-profile-pending-msg"><i class="fas fa-spinner fa-spin" aria-hidden="true"></i> This profile is still being generated. You can close this dialog and open it again later.</p>';
                    } else if (typeof marked !== 'undefined' && currentProfileText) {
                        contentEl.innerHTML = marked.parse(currentProfileText);
                    } else {
                        contentEl.textContent = currentProfileText || '(No profile content)';
                    }
                }
                const textarea = document.getElementById('complete-profile-edit-textarea');
                if (textarea) textarea.value = currentProfileText;
                switchCompleteProfileTab('view');
                if (modal) {
                    modal.style.display = 'flex';
                    Modals._openModal(modal);
                }
                return true;
            } catch (err) {
                console.error('View complete profile error:', err);
                if (!silentOn404) await AppDialogs.showAppAlert('Error', err.message);
                return false;
            }
        }

        async function handleViewCompleteProfile(e) {
            const btn = e.target.closest('.contacts-view-profile-btn');
            if (!btn) return;
            const name = btn.dataset.contactName;
            if (name) await _openProfileByName(name);
        }

        function loadContactsModalSubTab(tabName) {
            if (tabName === 'relationships') {
                if (Modals.Relationships?.openTab) Modals.Relationships.openTab();
            } else if (Modals.Relationships?.tearDown) {
                Modals.Relationships.tearDown();
            }
            if (tabName === 'email-matches' && Modals.EmailMatches?.load) Modals.EmailMatches.load();
            if (tabName === 'email-exclusions' && Modals.EmailExclusions?.load) Modals.EmailExclusions.load();
            if (tabName === 'email-classifications' && Modals.EmailClassifications?.load) Modals.EmailClassifications.load();
        }

        function showContactsModalTab(tabName) {
            const modal = document.getElementById('contacts-modal');
            if (!modal) return;
            modal.querySelectorAll('.manage-contacts-tab-btn').forEach(b => {
                b.classList.toggle('active', b.getAttribute('data-manage-contacts-tab') === tabName);
            });
            modal.querySelectorAll('.manage-contacts-tab-content').forEach(c => c.classList.remove('active'));
            const content = document.getElementById(`${tabName}-tab-content`);
            if (content) content.classList.add('active');
        }

        function open(tabName = 'contacts-directory') {
            selectedIds.clear();
            Modals._openModal(DOM.contactsModal);
            showContactsModalTab(tabName);
            loadContactsModalSubTab(tabName);
            if (tabName === 'contacts-directory') loadContacts();
        }
        function close() {
            if (Modals.Relationships?.tearDown) Modals.Relationships.tearDown();
            Modals._closeModal(DOM.contactsModal);
        }
        async function pollContactsExtractUntilDone(maxMs = 600000) {
            const start = Date.now();
            while (Date.now() - start < maxMs) {
                await new Promise((r) => setTimeout(r, 1500));
                const res = await fetch('/contacts/extract/status');
                if (!res.ok) {
                    continue;
                }
                const st = await res.json();
                if (!st || !st.in_progress) {
                    if (st.status === 'completed') {
                        return st.status_line || 'Contact totals recalculated.';
                    }
                    if (st.status === 'error') {
                        throw new Error(st.error_message || st.status_line || 'Contacts recalculation failed');
                    }
                    if (st.status === 'cancelled') {
                        throw new Error(st.status_line || 'Contacts recalculation cancelled');
                    }
                    return st.status_line || 'Contact totals recalculated.';
                }
            }
            throw new Error('Timed out waiting for contact totals to finish recalculating');
        }

        function waitForContactsExtract(onProgress) {
            return new Promise((resolve, reject) => {
                let settled = false;
                const finish = (ok, msg) => {
                    if (settled) {
                        return;
                    }
                    settled = true;
                    es.close();
                    if (ok) {
                        resolve(msg);
                    } else {
                        reject(new Error(msg || 'Contacts recalculation failed'));
                    }
                };
                const es = new EventSource('/contacts/extract/stream');
                es.onmessage = (event) => {
                    try {
                        const ed = JSON.parse(event.data);
                        const type = ed.type;
                        const data = ed.data || {};
                        if (typeof onProgress === 'function' && (type === 'progress' || type === 'status')) {
                            onProgress(data.status_line || 'Recalculating totals…');
                        }
                        if (type === 'completed') {
                            finish(true, data.status_line || 'Contact totals recalculated.');
                        } else if (type === 'error') {
                            finish(false, data.error_message || data.status_line || 'Contacts recalculation failed');
                        } else if (type === 'cancelled') {
                            finish(false, data.status_line || 'Contacts recalculation cancelled');
                        }
                    } catch (e) {
                        console.warn('Contacts extract SSE parse error:', e);
                    }
                };
                es.onerror = () => {
                    es.close();
                    if (settled) {
                        return;
                    }
                    pollContactsExtractUntilDone()
                        .then((msg) => finish(true, msg))
                        .catch((err) => finish(false, err.message));
                };
            });
        }

        async function recalculateContactTotals() {
            const btn = DOM.contactsRecalculateTotalsBtn || DOM.extractContactsBtn;
            if (!btn || btn.disabled) {
                return;
            }
            const origHtml = btn.innerHTML;
            btn.disabled = true;
            const setLabel = (text) => {
                btn.innerHTML = `<i class="fas fa-spinner fa-spin"></i> ${escapeHtml(String(text || 'Recalculating totals…'))}`;
            };
            setLabel('Starting…');
            try {
                const statusRes = await fetch('/contacts/extract/status');
                if (statusRes.ok) {
                    const st = await statusRes.json();
                    if (st && st.in_progress) {
                        await waitForContactsExtract(setLabel);
                        await loadContacts();
                        return;
                    }
                }
                const response = await fetch('/contacts/extract', { method: 'POST' });
                if (!response.ok) {
                    const err = await response.json().catch(() => ({}));
                    throw new Error(err.detail || `HTTP ${response.status}`);
                }
                await waitForContactsExtract(setLabel);
                await loadContacts();
            } catch (err) {
                console.error('Error recalculating contact totals:', err);
                await AppDialogs.showAppAlert('Error', err.message || 'Could not recalculate contact totals');
            } finally {
                btn.disabled = false;
                btn.innerHTML = origHtml;
            }
        }

        async function extractContacts() {
            await recalculateContactTotals();
        }
        function onSelectAllClick(e) {
            const checked = e.target.checked;
            if (!DOM.contactsTableBody) return;
            DOM.contactsTableBody.querySelectorAll('.contacts-row-cb').forEach(el => {
                el.checked = checked;
                const id = parseInt(el.dataset.contactId, 10);
                if (checked) selectedIds.add(id);
                else selectedIds.delete(id);
            });
            updateDeleteSelectedUI();
        }

        function init() {
            if (DOM.closeContactsModalBtn) DOM.closeContactsModalBtn.addEventListener('click', close);
            if (DOM.contactsModal) DOM.contactsModal.addEventListener('click', (e) => { if (e.target === DOM.contactsModal) close(); });
            if (DOM.extractContactsBtn) DOM.extractContactsBtn.addEventListener('click', extractContacts);
            if (DOM.contactsRecalculateTotalsBtn) DOM.contactsRecalculateTotalsBtn.addEventListener('click', () => { void recalculateContactTotals(); });
            if (DOM.contactsDeleteSelectedBtn) DOM.contactsDeleteSelectedBtn.addEventListener('click', handleDeleteSelected);
            if (DOM.contactsSelectAll) DOM.contactsSelectAll.addEventListener('click', onSelectAllClick);
            document.querySelectorAll('.contacts-sortable-header').forEach(th => {
                th.addEventListener('click', onSortHeaderClick);
            });
            if (DOM.contactsHasMessagesOnly) DOM.contactsHasMessagesOnly.addEventListener('change', () => loadContacts());
            if (DOM.contactsEmailContainsAt) DOM.contactsEmailContainsAt.addEventListener('change', () => loadContacts());
            if (DOM.contactsExcludePhoneNumbers) DOM.contactsExcludePhoneNumbers.addEventListener('change', () => loadContacts());
            if (DOM.contactsTableBody) DOM.contactsTableBody.addEventListener('click', onCountCellClick);
            if (DOM.contactsSearch) {
                const runSearch = () => loadContacts();
                DOM.contactsSearch.addEventListener('keydown', (e) => { if (e.key === 'Enter') runSearch(); });
                DOM.contactsSearch.addEventListener('blur', runSearch);
            }
            const closeCompleteProfileBtn = document.getElementById('close-complete-profile-modal');
            if (closeCompleteProfileBtn) {
                closeCompleteProfileBtn.addEventListener('click', () => {
                    const m = document.getElementById('complete-profile-modal');
                    if (m) Modals._closeModal(m);
                });
            }
            const completeProfileModal = document.getElementById('complete-profile-modal');
            if (completeProfileModal) {
                completeProfileModal.addEventListener('click', (e) => {
                    if (e.target === completeProfileModal) Modals._closeModal(completeProfileModal);
                });
            }
            document.querySelectorAll('.complete-profile-tab').forEach(btn => {
                btn.addEventListener('click', () => switchCompleteProfileTab(btn.dataset.tab));
            });
            const saveProfileBtn = document.getElementById('complete-profile-save-btn');
            if (saveProfileBtn) saveProfileBtn.addEventListener('click', handleSaveCompleteProfile);
            const contactsModal = document.getElementById('contacts-modal');
            if (contactsModal) {
                contactsModal.querySelectorAll('.manage-contacts-tab-btn').forEach(btn => {
                    btn.addEventListener('click', () => {
                        const tabName = btn.getAttribute('data-manage-contacts-tab');
                        if (!tabName) return;
                        showContactsModalTab(tabName);
                        loadContactsModalSubTab(tabName);
                        if (tabName === 'contacts-directory') loadContacts();
                    });
                });
            }
        }
        return {
            init,
            open,
            close,
            openProfileByName: (name) => _openProfileByName(name),
            reloadContactsPage: () => loadContacts()
        };
})();


Modals.Profiles = (() => {
        const modal = () => document.getElementById('profiles-modal');
        const contactInput = () => document.getElementById('profiles-contact-input');
        const contactDropdown = () => document.getElementById('profiles-contact-dropdown');
        const createBtn = () => document.getElementById('profiles-create-btn');
        let allContactNames = [];
        const profilesSelectedContacts = new Set();
        const loadingEl = () => document.getElementById('profiles-loading');
        const tableContainer = () => document.getElementById('profiles-table-container');
        const tableBody = () => document.getElementById('profiles-table-body');
        const emptyMsg = () => document.getElementById('profiles-empty-msg');
        let profilesPollTimer = null;
        let profilesNameSortOrder = 'asc';

        function isAllowedProfileProvider(key) {
            return key === 'localai' || (typeof AIModels !== 'undefined' && AIModels.isValidKey(key));
        }

        function defaultProfileProvider() {
            if (typeof AIModels !== 'undefined') {
                const dk = AIModels.defaultKey();
                if (dk) return dk;
            }
            return 'localai';
        }

        function profilesLlmProviderEl() {
            if (typeof DOM !== 'undefined' && DOM.profilesLlmProviderSelect) return DOM.profilesLlmProviderSelect;
            return document.getElementById('profiles-llm-provider-select');
        }

        function syncProfilesProviderFromChat() {
            const pSel = profilesLlmProviderEl();
            const chatSel = typeof DOM !== 'undefined' ? DOM.llmProviderSelect : null;
            if (!pSel) return;
            const v = (chatSel && chatSel.value) ? String(chatSel.value).trim().toLowerCase() : defaultProfileProvider();
            pSel.value = isAllowedProfileProvider(v) ? v : defaultProfileProvider();
        }

        function selectedProfilesProvider() {
            const pSel = profilesLlmProviderEl();
            if (pSel && pSel.value) {
                const v = String(pSel.value).trim().toLowerCase();
                if (isAllowedProfileProvider(v)) return v;
            }
            const chatSel = typeof DOM !== 'undefined' ? DOM.llmProviderSelect : null;
            if (chatSel && chatSel.value) {
                const v = String(chatSel.value).trim().toLowerCase();
                if (isAllowedProfileProvider(v)) return v;
            }
            return defaultProfileProvider();
        }

        function updateProfilesNameSortHeader() {
            const th = document.querySelector('#profiles-modal .profiles-profiles-sort-name');
            if (!th) return;
            th.classList.remove('profiles-sort-asc', 'profiles-sort-desc');
            th.classList.add(profilesNameSortOrder === 'asc' ? 'profiles-sort-asc' : 'profiles-sort-desc');
        }

        function applyProfilesNameSort() {
            const tbody = tableBody();
            if (!tbody) return;
            const rows = Array.from(tbody.querySelectorAll('tr'));
            rows.sort((a, b) => {
                const na = (a.dataset.profileName || '').toLowerCase();
                const nb = (b.dataset.profileName || '').toLowerCase();
                const c = na.localeCompare(nb, undefined, { sensitivity: 'base' });
                return profilesNameSortOrder === 'asc' ? c : -c;
            });
            rows.forEach((r) => tbody.appendChild(r));
        }

        function clearProfilesPoll() {
            if (profilesPollTimer) {
                clearInterval(profilesPollTimer);
                profilesPollTimer = null;
            }
        }

        function scheduleProfilesPollIfNeeded(entries) {
            clearProfilesPoll();
            const hasPending = entries.some(e => e.pending);
            if (!hasPending) return;
            const m = modal();
            if (!m || m.style.display === 'none') return;
            profilesPollTimer = setInterval(() => {
                loadProfileNames({ quiet: true });
            }, 4000);
        }

        function escapeHtml(s) {
            const d = document.createElement('div');
            d.textContent = s;
            return d.innerHTML;
        }

        function showContactDropdown() {
            const dropdown = contactDropdown();
            const input = contactInput();
            if (!dropdown || !input) return;
            const q = (input.value || '').trim().toLowerCase();
            const matches = q
                ? allContactNames.filter(n => n.toLowerCase().includes(q))
                : [...allContactNames];
            if (matches.length === 0) {
                dropdown.style.display = 'none';
                return;
            }
            const cmp = (a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' });
            const selectedInMatches = matches.filter((n) => profilesSelectedContacts.has(n)).sort(cmp);
            const unselectedMatches = matches.filter((n) => !profilesSelectedContacts.has(n)).sort(cmp);
            const ordered = [...selectedInMatches, ...unselectedMatches];
            dropdown.innerHTML = '';
            ordered.forEach((n) => {
                const label = document.createElement('label');
                label.className = 'profiles-contact-option profiles-contact-option--row';
                const cb = document.createElement('input');
                cb.type = 'checkbox';
                cb.className = 'profiles-contact-cb';
                cb.checked = profilesSelectedContacts.has(n);
                cb.dataset.name = n;
                const span = document.createElement('span');
                span.className = 'profiles-contact-option-label';
                span.textContent = n;
                label.appendChild(cb);
                label.appendChild(span);
                cb.addEventListener('change', () => {
                    if (cb.checked) profilesSelectedContacts.add(n);
                    else profilesSelectedContacts.delete(n);
                    updateCreateBtnState();
                });
                dropdown.appendChild(label);
            });
            dropdown.style.display = 'block';
        }

        function hideContactDropdown() {
            const dropdown = contactDropdown();
            if (dropdown) dropdown.style.display = 'none';
        }

        function closeAllProfilesActionMenus() {
            document.querySelectorAll('#profiles-modal .profiles-actions-menu').forEach((menu) => {
                menu.style.display = 'none';
                menu.setAttribute('hidden', '');
                menu.style.position = '';
                menu.style.right = '';
                menu.style.left = '';
                menu.style.top = '';
                menu.style.zIndex = '';
            });
            document.querySelectorAll('#profiles-modal .profiles-actions-toggle').forEach((btn) => {
                btn.setAttribute('aria-expanded', 'false');
            });
        }

        function positionProfilesActionMenu(menu, toggle) {
            const r = toggle.getBoundingClientRect();
            menu.style.position = 'fixed';
            menu.style.right = 'auto';
            const w = menu.offsetWidth || 160;
            const left = Math.min(Math.max(6, r.right - w), window.innerWidth - w - 8);
            menu.style.left = `${left}px`;
            menu.style.top = `${r.bottom + 4}px`;
            menu.style.zIndex = '5000';
        }

        async function loadContactNames() {
            const input = contactInput();
            if (!input) return;
            try {
                const resp = await fetch('/contacts/names');
                if (!resp.ok) return;
                const data = await resp.json();
                const contacts = data.contacts || [];
                allContactNames = [...new Set(contacts.map(c => c.name).filter(n => n && n.trim()))].sort();
                input.value = '';
                profilesSelectedContacts.clear();
                updateCreateBtnState();
            } catch (err) {
                console.error('Profiles load contacts error:', err);
            }
        }

        function updateCreateBtnState() {
            const btn = createBtn();
            if (!btn) return;
            const n = profilesSelectedContacts.size;
            btn.disabled = n === 0;
            const icon = '<i class="fas fa-sync-alt" aria-hidden="true"></i> ';
            btn.innerHTML = n === 0 ? `${icon}Create profile` : n === 1 ? `${icon}Create profile` : `${icon}Create profiles (${n})`;
        }

        async function loadProfileNames(opts) {
            const quiet = opts && opts.quiet;
            const tbody = tableBody();
            const loading = loadingEl();
            const container = tableContainer();
            const empty = emptyMsg();
            if (!tbody) return;
            if (!quiet) {
                if (loading) loading.style.display = 'block';
                if (container) container.style.display = 'none';
            }
            if (empty) empty.style.display = 'none';
            try {
                const resp = await fetch('/chat/complete-profile/names');
                if (!resp.ok) throw new Error('Failed to load');
                const data = await resp.json();
                let entries = [];
                if (Array.isArray(data.profiles) && data.profiles.length > 0) {
                    entries = data.profiles
                        .map(p => ({
                            name: String(p.name || '').trim(),
                            pending: !!p.pending
                        }))
                        .filter(e => e.name);
                } else {
                    const names = (data.names || []).filter(n => n && String(n).trim());
                    entries = names.map(n => ({ name: String(n).trim(), pending: false }));
                }
                entries.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
                tbody.innerHTML = '';
                entries.forEach(({ name, pending }) => {
                    const tr = document.createElement('tr');
                    tr.dataset.profileName = name;
                    tr.dataset.pending = pending ? 'true' : 'false';
                    const safeName = String(name).replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    tr.innerHTML = `
                        <td class="profiles-name-cell"></td>
                        <td class="profiles-actions-cell"></td>
                    `;
                    const nameCell = tr.querySelector('.profiles-name-cell');
                    if (nameCell) {
                        nameCell.innerHTML = `${safeName}${pending ? ' <span class="profiles-row-pending-badge">(generating…)</span>' : ''}`;
                    }
                    const actionsCell = tr.querySelector('.profiles-actions-cell');
                    if (actionsCell) {
                        const wrap = document.createElement('div');
                        wrap.className = 'profiles-actions-wrap';
                        const toggle = document.createElement('button');
                        toggle.type = 'button';
                        toggle.className = 'modal-btn modal-btn-secondary profiles-actions-toggle profiles-row-btn';
                        toggle.setAttribute('aria-haspopup', 'true');
                        toggle.setAttribute('aria-expanded', 'false');
                        toggle.setAttribute('aria-label', `Actions for ${name}`);
                        toggle.innerHTML = 'Actions <i class="fas fa-caret-down" aria-hidden="true"></i>';
                        const menu = document.createElement('div');
                        menu.className = 'profiles-actions-menu';
                        menu.setAttribute('role', 'menu');
                        menu.style.display = 'none';
                        menu.setAttribute('hidden', '');
                        const mkItem = (action, label, itemOpts) => {
                            const o = itemOpts || {};
                            const b = document.createElement('button');
                            b.type = 'button';
                            b.className = 'profiles-actions-menu-item' + (o.danger ? ' profiles-actions-menu-item--danger' : '');
                            b.dataset.action = action;
                            b.setAttribute('role', 'menuitem');
                            b.textContent = label;
                            if (o.disabled) b.disabled = true;
                            return b;
                        };
                        menu.appendChild(mkItem('view', 'View profile', { disabled: pending }));
                        menu.appendChild(mkItem('regenerate', 'Regenerate…', { disabled: pending }));
                        menu.appendChild(mkItem('delete', 'Delete…', { danger: true }));
                        menu.querySelectorAll('.profiles-actions-menu-item').forEach((item) => {
                            item.addEventListener('click', (ev) => {
                                ev.stopPropagation();
                                closeAllProfilesActionMenus();
                                const act = item.dataset.action;
                                if (act === 'view' && !pending) Modals.Contacts.openProfileByName(name);
                                else if (act === 'regenerate' && !pending) handleRegenerateProfile(name);
                                else if (act === 'delete') handleDeleteProfile(name);
                            });
                        });
                        toggle.addEventListener('click', (ev) => {
                            ev.stopPropagation();
                            const open = menu.style.display === 'block';
                            closeAllProfilesActionMenus();
                            if (!open) {
                                menu.style.display = 'block';
                                menu.removeAttribute('hidden');
                                toggle.setAttribute('aria-expanded', 'true');
                                requestAnimationFrame(() => {
                                    positionProfilesActionMenu(menu, toggle);
                                });
                            }
                        });
                        wrap.appendChild(toggle);
                        wrap.appendChild(menu);
                        actionsCell.appendChild(wrap);
                    }
                    tr.addEventListener('click', (e) => {
                        if (e.target.closest('.profiles-actions-wrap')) return;
                        if (pending) return;
                        e.stopPropagation();
                        Modals.Contacts.openProfileByName(name);
                    });
                    tbody.appendChild(tr);
                });
                updateProfilesNameSortHeader();
                applyProfilesNameSort();
                if (loading) loading.style.display = 'none';
                if (container) container.style.display = 'block';
                if (empty) empty.style.display = entries.length === 0 ? 'block' : 'none';
                scheduleProfilesPollIfNeeded(entries);
            } catch (err) {
                console.error('Profiles load error:', err);
                if (loading && !quiet) {
                    loading.innerHTML = '<span class="profiles-loading-error">Error loading profiles</span>';
                    loading.style.display = 'block';
                }
            }
        }

        async function postCompleteProfileStart(name) {
            const provider = selectedProfilesProvider();
            const resp = await fetch('/chat/complete-profile', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ full_name: String(name).trim(), provider })
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => ({}));
                throw new Error(err.detail || `HTTP ${resp.status}`);
            }
            return resp.json();
        }

        async function startCompleteProfileGeneration(name, alertTitle) {
            const data = await postCompleteProfileStart(name);
            await AppDialogs.showAppAlert(alertTitle || 'Profile', data.message || 'Profile generation started. This runs in the background.');
            await loadProfileNames();
            if (typeof Modals !== 'undefined' && Modals.Contacts && typeof Modals.Contacts.reloadContactsPage === 'function') {
                Modals.Contacts.reloadContactsPage();
            }
        }

        async function handleRegenerateProfile(name) {
            if (!name || !(String(name).trim())) return;
            const ok = await AppDialogs.showAppConfirm(
                'Regenerate profile',
                `Regenerate the complete profile for "${name}"? The current text will be replaced when the new generation finishes (several minutes).`
            );
            if (!ok) return;
            try {
                await startCompleteProfileGeneration(String(name).trim(), 'Profile');
            } catch (err) {
                console.error('Regenerate profile error:', err);
                await AppDialogs.showAppAlert('Error', err.message);
            }
        }

        async function handleCreateProfile() {
            const input = contactInput();
            const btn = createBtn();
            if (!btn || profilesSelectedContacts.size === 0) return;
            const names = [...profilesSelectedContacts].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
            btn.disabled = true;
            btn.innerHTML = '<i class="fas fa-spinner fa-spin" aria-hidden="true"></i> Creating…';
            const errors = [];
            try {
                for (const name of names) {
                    try {
                        await postCompleteProfileStart(name);
                    } catch (err) {
                        errors.push({ name, message: err.message || String(err) });
                    }
                }
                await loadProfileNames();
                if (typeof Modals !== 'undefined' && Modals.Contacts && typeof Modals.Contacts.reloadContactsPage === 'function') {
                    Modals.Contacts.reloadContactsPage();
                }
                const failed = new Set(errors.map((e) => e.name));
                names.forEach((nm) => {
                    if (!failed.has(nm)) profilesSelectedContacts.delete(nm);
                });
                if (errors.length === 0) {
                    profilesSelectedContacts.clear();
                    if (input) input.value = '';
                    hideContactDropdown();
                    const msg = names.length === 1
                        ? 'Complete profile generation started. This runs in the background.'
                        : `Started complete profile generation for ${names.length} contacts. Each runs in the background.`;
                    await AppDialogs.showAppAlert('Profile', msg);
                } else {
                    const ok = names.length - errors.length;
                    const failLines = errors.map((e) => `${e.name}: ${e.message}`).join('\n');
                    const msg = ok > 0
                        ? `Started ${ok} of ${names.length}. Failed:\n${failLines}`
                        : `Could not start generation:\n${failLines}`;
                    await AppDialogs.showAppAlert(errors.length === names.length ? 'Error' : 'Profile', msg);
                }
            } catch (err) {
                console.error('Create profile batch error:', err);
                await AppDialogs.showAppAlert('Error', err.message);
            } finally {
                updateCreateBtnState();
            }
        }

        async function handleDeleteProfile(name) {
            const ok = await AppDialogs.showAppConfirm(
                'Delete profile',
                `Delete the complete profile for "${name}"? This cannot be undone.`,
                { danger: true }
            );
            if (!ok) return;
            try {
                const resp = await fetch('/chat/complete-profile?name=' + encodeURIComponent(name), { method: 'DELETE' });
                if (!resp.ok) {
                    const err = await resp.json().catch(() => ({}));
                    throw new Error(err.detail || `HTTP ${resp.status}`);
                }
                await loadProfileNames();
            } catch (err) {
                console.error('Delete profile error:', err);
                await AppDialogs.showAppAlert('Error', err.message);
            }
        }

        function open() {
            Modals._openModal(modal());
            syncProfilesProviderFromChat();
            loadContactNames();
            loadProfileNames();
        }

        function close() {
            closeAllProfilesActionMenus();
            clearProfilesPoll();
            profilesSelectedContacts.clear();
            const input = contactInput();
            if (input) input.value = '';
            updateCreateBtnState();
            Modals._closeModal(modal());
        }

        function init() {
            const closeBtn = document.getElementById('close-profiles-modal');
            if (closeBtn) closeBtn.addEventListener('click', close);
            const m = modal();
            if (m) m.addEventListener('click', (e) => { if (e.target === m) close(); });
            if (m) {
                m.addEventListener('mousedown', (e) => {
                    if (!e.target.closest('.profiles-actions-wrap')) {
                        closeAllProfilesActionMenus();
                    }
                });
            }
            const profilesTableScroll = document.querySelector('#profiles-modal .profiles-table-scroll');
            if (profilesTableScroll) {
                profilesTableScroll.addEventListener('scroll', () => {
                    closeAllProfilesActionMenus();
                    hideContactDropdown();
                }, { passive: true });
            }
            window.addEventListener('resize', closeAllProfilesActionMenus);
            const dd = contactDropdown();
            if (dd) {
                dd.addEventListener('mousedown', (e) => { e.preventDefault(); });
            }
            const input = contactInput();
            if (input) {
                input.addEventListener('input', () => {
                    showContactDropdown();
                });
                input.addEventListener('focus', () => showContactDropdown());
                input.addEventListener('keydown', (e) => {
                    if (e.key === 'Escape') {
                        hideContactDropdown();
                        return;
                    }
                    if (e.key === 'Enter') {
                        const firstCb = contactDropdown()?.querySelector('.profiles-contact-cb');
                        if (firstCb) {
                            firstCb.checked = !firstCb.checked;
                            firstCb.dispatchEvent(new Event('change', { bubbles: true }));
                            e.preventDefault();
                        }
                        return;
                    }
                });
                input.addEventListener('blur', () => {
                    setTimeout(hideContactDropdown, 150);
                });
            }
            const btn = createBtn();
            if (btn) btn.addEventListener('click', handleCreateProfile);
            const nameSortTh = document.querySelector('#profiles-modal .profiles-profiles-sort-name');
            if (nameSortTh) {
                nameSortTh.addEventListener('click', (e) => {
                    e.stopPropagation();
                    profilesNameSortOrder = profilesNameSortOrder === 'asc' ? 'desc' : 'asc';
                    updateProfilesNameSortHeader();
                    applyProfilesNameSort();
                });
            }
        }

        return { init, open, close, refreshProfilesTable: () => loadProfileNames() };
})();


Modals.Relationships = (() => {
        let cy = null;
        let hideTimeout = null;
        let controlsBound = false;
        const REL_SUBJECT_NODE_ID = '__subject__';
        const LEGACY_SUBJECT_NODE_ID = '0';

        function relTab() {
            return document.getElementById('relationships-tab-content');
        }

        function escapeHtml(s) {
            if (s == null) return '';
            const d = document.createElement('div');
            d.textContent = String(s);
            return d.innerHTML;
        }

        function getTypeFilterParams() {
            const types = ['friend', 'family', 'colleague', 'acquaintance', 'business', 'social', 'promotional', 'unknown'];
            const selected = types.filter(t => {
                const cb = document.getElementById('rel-type-' + t);
                return cb && cb.checked;
            });
            return selected.length > 0 ? selected.join(',') : null;
        }

        function getSourceFilterParams() {
            const sources = ['email', 'facebook', 'whatsapp', 'sms-imessage', 'instagram'];
            const selected = sources.filter(s => {
                const cb = document.getElementById('rel-source-' + s);
                return cb && cb.checked;
            });
            return selected.length > 0 ? selected.join(',') : null;
        }

        async function fetchData() {
            const typeParam = getTypeFilterParams();
            const sourceParam = getSourceFilterParams();
            const maxNodesSlider = document.getElementById('rel-max-nodes-slider');
            const maxNodes = maxNodesSlider ? parseInt(maxNodesSlider.value, 10) : 100;
            const url = new URL('/relationship/strength', window.location.origin);
            if (typeParam) url.searchParams.set('types', typeParam);
            if (sourceParam) url.searchParams.set('sources', sourceParam);
            url.searchParams.set('max_nodes', String(maxNodes));
            const response = await fetch(url.toString());
            if (!response.ok) throw new Error('Failed to load relationship data');
            return response.json();
        }

        function isSubjectNodeId(id) {
            const s = String(id);
            return s === REL_SUBJECT_NODE_ID || s === LEGACY_SUBJECT_NODE_ID;
        }

        function buildGraphElements(data) {
            const rawNodes = Array.isArray(data.nodes) ? data.nodes : [];
            const rawLinks = Array.isArray(data.links) ? data.links : [];
            const nodes = rawNodes.map((n) => ({
                ...n,
                id: String(n.id),
            }));
            const nodeIds = new Set(nodes.map((n) => n.id));

            const needsSubject = rawLinks.some((l) => {
                const t = String(l.target);
                return t === REL_SUBJECT_NODE_ID || t === LEGACY_SUBJECT_NODE_ID;
            });
            if (needsSubject && !nodeIds.has(REL_SUBJECT_NODE_ID) && !nodeIds.has(LEGACY_SUBJECT_NODE_ID)) {
                nodes.unshift({ id: REL_SUBJECT_NODE_ID, name: 'Subject' });
                nodeIds.add(REL_SUBJECT_NODE_ID);
            }

            // Migrate legacy hub id "0" to __subject__ so cytoscape never sees a missing target.
            for (const n of nodes) {
                if (n.id === LEGACY_SUBJECT_NODE_ID) {
                    n.id = REL_SUBJECT_NODE_ID;
                }
            }
            nodeIds.delete(LEGACY_SUBJECT_NODE_ID);
            nodeIds.add(REL_SUBJECT_NODE_ID);

            const links = rawLinks
                .map((l) => {
                    let source = String(l.source);
                    let target = String(l.target);
                    if (target === LEGACY_SUBJECT_NODE_ID) target = REL_SUBJECT_NODE_ID;
                    if (source === LEGACY_SUBJECT_NODE_ID) source = REL_SUBJECT_NODE_ID;
                    return { ...l, source, target };
                })
                .filter((l) => nodeIds.has(l.source) && nodeIds.has(l.target));

            return [
                ...nodes.map((n) => ({ data: n })),
                ...links.map((l) => ({ data: l })),
            ];
        }

        function updateGraph() {
            if (!cy) return;
            const slider = document.getElementById('rel-filter-slider');
            const sizeSlider = document.getElementById('rel-size-slider');
            if (!slider || !sizeSlider ) return;
            const threshold = parseInt(slider.value);
            const baseSize = parseFloat(sizeSlider.value);
            const hub = cy.getElementById(REL_SUBJECT_NODE_ID);

            cy.edges().forEach(e => {
                if (e.data('strength') < threshold) e.addClass('hidden');
                else e.removeClass('hidden');
            });

            cy.nodes().forEach(n => {
                const hasVisibleEdge = n.connectedEdges().some(e => !e.hasClass('hidden'));
                if (!hasVisibleEdge) n.addClass('hidden');
                else n.removeClass('hidden');
            });

            const visible = cy.nodes().filter(n => !n.hasClass('hidden'));
            if (visible.length > 0) {
                const degrees = visible.map(n => n.connectedEdges().filter(e => !e.hasClass('hidden')).length);
                const min = Math.min(...degrees), max = Math.max(...degrees);
                const maxSize = baseSize * 4;
                visible.forEach(n => {
                    const deg = n.connectedEdges().filter(e => !e.hasClass('hidden')).length;
                    let s = baseSize;
                    if(max !== min) s = baseSize + (deg - min) * (maxSize - baseSize) / (max - min);
                    n.style({'width': s, 'height': s});
                });
            }
        }

        function reLayout() {
            if (!cy) return;
            updateGraph();
            const hub = cy.getElementById(REL_SUBJECT_NODE_ID);
            if (!hub || hub.length === 0) return;
            hub.unlock();
            hub.position({ x: cy.width() / 2, y: cy.height() / 2 });
            hub.lock();
            const layout = cy.layout({ name: 'cose', animate: true, fit: false, nodeRepulsion: () => 800000, gravity: 2, eles: cy.elements(':visible') });
            layout.one('layoutstop', () => { cy.animate({ center: { eles: hub }, zoom: 0.7, duration: 800 }); });
            layout.run();
        }

        function resetView() {
            if (cy) cy.animate({ fit: { eles: cy.elements(':visible'), padding: 50 }, duration: 500 });
        }

        function bindStaticControls() {
            if (controlsBound) return;
            const root = relTab();
            if (!root) return;
            const fitBtn = root.querySelector('#rel-fit-all-btn');
            const applyBtn = root.querySelector('#rel-apply-filters-btn');
            const filterSlider = root.querySelector('#rel-filter-slider');
            const sizeSlider = root.querySelector('#rel-size-slider');
            const strVal = root.querySelector('#rel-str-val');
            const sizeVal = root.querySelector('#rel-size-val');
            const maxNodesSlider = root.querySelector('#rel-max-nodes-slider');
            const maxNodesVal = root.querySelector('#rel-max-nodes-val');
            const searchInput = root.querySelector('#rel-search-input');

            if (fitBtn) fitBtn.addEventListener('click', resetView);
            if (applyBtn) {
                applyBtn.addEventListener('click', () => {
                    tearDown();
                    initGraph();
                    applyBtn.disabled = true;
                });
                const enableApplyOnFilterChange = () => { applyBtn.disabled = false; };
                root.querySelectorAll('.rel-type-cb, .rel-source-cb').forEach(el => {
                    el.addEventListener('change', enableApplyOnFilterChange);
                });
                if (maxNodesSlider) maxNodesSlider.addEventListener('input', enableApplyOnFilterChange);
            }
            if (filterSlider && strVal) {
                filterSlider.addEventListener('input', function () {
                    strVal.textContent = this.value;
                    updateGraph();
                });
            }
            if (sizeSlider && sizeVal) {
                sizeSlider.addEventListener('input', function () {
                    sizeVal.textContent = this.value;
                    updateGraph();
                });
            }
            if (maxNodesSlider && maxNodesVal) {
                maxNodesSlider.addEventListener('input', function () {
                    maxNodesVal.textContent = this.value;
                });
            }
            if (searchInput) {
                const doSearch = () => {
                    if (!cy) return;
                    const text = searchInput.value.toLowerCase().trim();
                    if (!text) return;
                    const found = cy.nodes().filter(n => n.data('name').toLowerCase().includes(text));
                    if (found.length > 0) {
                        cy.animate({ center: { eles: found[0] }, zoom: 1.2, duration: 400 });
                        found[0].select();
                    }
                };
                searchInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') doSearch(); });
            }
            controlsBound = true;
        }

        function setupEvents() {
            const root = relTab();
            const balloon = root ? root.querySelector('#rel-balloon') : null;
            if (!balloon || !cy) return;

            cy.on('mouseover', 'node', function(e) {
                const node = e.target;
                const neighborhood = node.closedNeighborhood();
                cy.elements().addClass('faded');
                neighborhood.removeClass('faded').addClass('highlight');
            });

            cy.on('mouseout', 'node', function() {
                cy.elements().removeClass('faded highlight');
            });

            function positionBalloon(renderedPos) {
                const container = root ? root.querySelector('.rel-cy-container') : null;
                if (!container) return;
                const rect = container.getBoundingClientRect();
                balloon.style.top = (renderedPos.y - rect.top - 40) + 'px';
                balloon.style.left = (renderedPos.x - rect.left + 10) + 'px';
            }

            cy.on('tap', 'edge', function(e) {
                const edge = e.target;
                clearTimeout(hideTimeout);
                cy.edges().unselect();
                edge.select();
                balloon.innerHTML = `<strong>Strength: ${edge.data('strength')}/10</strong>`;
                balloon.style.display = 'block';
                positionBalloon(e.renderedPosition);
                hideTimeout = setTimeout(() => { balloon.style.display = 'none'; edge.unselect(); }, 3000);
            });

            const CONTACT_TYPES = ['friend', 'family', 'colleague', 'acquaintance', 'business', 'social', 'promotional', 'spam', 'important', 'unknown'];
            cy.on('tap', 'node', function(e) {
                const node = e.target;
                balloon.style.display = 'none';
                cy.elements().removeClass('faded highlight');
                const detailsPanel = root ? root.querySelector('#rel-node-details') : null;
                if (!detailsPanel) return;
                const nodeId = node.id();
                const nodeName = node.data('name');
                const currentType = node.data('contact_type') || 'unknown';
                const edges = node.connectedEdges().filter(e => !e.hasClass('hidden'));
                const connections = edges.map(edge => {
                    const other = edge.source().id() === nodeId ? edge.target() : edge.source();
                    return { name: other.data('name'), strength: edge.data('strength') };
                });
                const counts = [
                    ['Email', node.data('num_emails')],
                    ['Facebook', node.data('num_facebook')],
                    ['WhatsApp', node.data('num_whatsapp')],
                    ['SMS/iMessage', (node.data('num_sms') || 0) + (node.data('num_imessages') || 0)],
                    ['Instagram', node.data('num_instagram')]
                ].filter(([, n]) => n != null && n > 0);
                let html = `<strong>${escapeHtml(nodeName)}</strong><br>`;
                if (connections.length > 0) {
                    connections.forEach(c => {
                        html += `<span style="color:#666">Connection strength ${c.strength}/10</span><br/>`;
                    });
                }
                if (counts.length > 0) {
                    html += '<div style="margin-top: 0.5em; font-size: 1em; color: #555;">';
                    html += '<label style="display:block; margin-top: 0.75em; font-weight: 600;">Number of Messages:</label>';
                    html += '<ul style="margin: 0.5em 0 0 1.2em; padding: 0;">';
                    html += counts.map(([label, n]) => `<li>${label}: ${n}</li>`).join(' ');
                    html += '</ul>';
                }
 
                if (!isSubjectNodeId(nodeId)) {
                    html += '<label style="display:block; margin-top: 0.75em; font-weight: 600;">Contact Type:</label>';
                    html += '<div id="rel-contact-type-radios" style="margin-top: 0.25em; display: flex; flex-wrap: wrap; gap: 0.5em;">';
                    CONTACT_TYPES.forEach(t => {
                        const label = t.charAt(0).toUpperCase() + t.slice(1);
                        const checked = t === currentType ? ' checked' : '';
                        html += `<label style="display: inline-flex; align-items: center; gap: 0.25em; cursor: pointer;"><input type="radio" name="rel-contact-type" value="${t}"${checked}> ${label}</label>`;
                    });
                    html += '</div>';
                }
                detailsPanel.innerHTML = html;
                if (!isSubjectNodeId(nodeId)) {
                    let lastSavedType = currentType;
                    const radioContainer = document.getElementById('rel-contact-type-radios');
                    if (radioContainer) {
                        radioContainer.addEventListener('change', async function(e) {
                            const radio = e.target;
                            if (radio.type !== 'radio' || radio.name !== 'rel-contact-type') return;
                            const newType = radio.value;
                            if (newType === lastSavedType) return;
                            try {
                                const res = await fetch('/contacts/update-classification', {
                                    method: 'PATCH',
                                    headers: { 'Content-Type': 'application/json' },
                                    body: JSON.stringify({ name: nodeName, classification: newType })
                                });
                                if (!res.ok) {
                                    const err = await res.json().catch(() => ({}));
                                    const msg = Array.isArray(err.detail) ? err.detail.map(d => d.msg || JSON.stringify(d)).join('; ') : (err.detail || res.statusText);
                                    throw new Error(msg);
                                }
                                node.data('contact_type', newType);
                                lastSavedType = newType;
                                if (typeof UI !== 'undefined' && UI.displayError) UI.displayError(null);
                            } catch (err) {
                                if (typeof UI !== 'undefined' && UI.displayError) {
                                    UI.displayError('Failed to save: ' + (err.message || 'Unknown error'));
                                } else {
                                    await AppDialogs.showAppAlert('Failed to save: ' + (err.message || 'Unknown error'));
                                }
                                const prevRadio = radioContainer.querySelector(`input[value="${lastSavedType}"]`);
                                if (prevRadio) prevRadio.checked = true;
                            }
                        });
                    }
                }
            });

            cy.on('tap', function(e) {
                if (e.target !== e.cy) return;
                balloon.style.display = 'none';
                cy.edges().unselect();
                cy.elements().removeClass('faded highlight');
                const detailsPanel = root ? root.querySelector('#rel-node-details') : null;
                if (detailsPanel) {
                    detailsPanel.innerHTML = '<em style="color: #666;">Click a person to view details</em>';
                }
            });

        }

        async function initGraph() {
            const root = relTab();
            const container = root ? root.querySelector('#rel-cy') : null;
            if (!container || typeof cytoscape === 'undefined') return;
            tearDown();
            let data;
            try {
                data = await fetchData();
            } catch (err) {
                console.error('Relationships fetch failed:', err);
                if (typeof UI !== 'undefined' && UI.displayError) UI.displayError('Could not load relationship data: ' + (err.message || 'Unknown error'));
                else await AppDialogs.showAppAlert('Could not load relationship data. Please try again.');
                return;
            }
            const elements = buildGraphElements(data);

            cy = cytoscape({
                container: container,
                elements: elements,
                minZoom: 0.1,
                maxZoom: 4,
                wheelSensitivity: 0.2,
                style: [
                    { selector: 'core', style: { 'background-color': '#e8eef5' } },
                    { selector: 'node', style: {
                        'background-color': '#4285f4',
                        'label': 'data(name)',
                        'font-size': '11px',
                        'color': '#1a2540',
                        'text-outline-color': '#e8eef5',
                        'text-outline-width': 2,
                        'text-valign': 'bottom',
                        'text-margin-y': 4
                    } },
                    { selector: `node[id="${REL_SUBJECT_NODE_ID}"]`, style: { 'background-color': '#1a73e8', 'border-width': 3, 'border-color': '#1a2540', 'z-index': 1000 } },
                    { selector: 'node[contact_type="friend"]', style: { 'background-color': '#4CAF50' } },
                    { selector: 'node[contact_type="family"]', style: { 'background-color': '#E91E63' } },
                    { selector: 'node[contact_type="colleague"]', style: { 'background-color': '#FF9800' } },
                    { selector: 'node[contact_type="acquaintance"]', style: { 'background-color': '#2196F3' } },
                    { selector: 'node[contact_type="business"]', style: { 'background-color': '#607D8B' } },
                    { selector: 'node[contact_type="social"]', style: { 'background-color': '#9C27B0' } },
                    { selector: 'node[contact_type="promotional"]', style: { 'background-color': '#FFC107' } },
                    { selector: 'node[contact_type="spam"]', style: { 'background-color': '#9E9E9E' } },
                    { selector: 'node[contact_type="important"]', style: { 'background-color': '#00BCD4' } },
                    { selector: 'node[contact_type="unknown"]', style: { 'background-color': '#BDBDBD' } },
                    { selector: 'edge', style: { 'width': 'data(strength)', 'line-color': '#64748b', 'curve-style': 'bezier', 'opacity': 0.7 } },
                    { selector: `edge[source="${REL_SUBJECT_NODE_ID}"], edge[target="${REL_SUBJECT_NODE_ID}"]`, style: { 'line-color': '#2563eb', 'opacity': 0.85 } },
                    { selector: '.hidden', style: { 'display': 'none' } },
                    { selector: 'edge:selected', style: { 'line-color': '#ff5722', 'opacity': 1, 'z-index': 999 } },
                    { selector: '.faded', style: { 'opacity': 0.08, 'text-opacity': 0 } },
                    { selector: '.highlight', style: { 'opacity': 1, 'text-opacity': 1, 'z-index': 9999 } }
                ]
            });

            setupEvents();
            const finishMount = () => {
                if (!cy) return;
                cy.resize();
                reLayout();
                resetView();
            };
            requestAnimationFrame(() => requestAnimationFrame(finishMount));
        }

        function resizeGraph() {
            if (!cy) return;
            cy.resize();
        }

        function tearDown() {
            const root = relTab();
            const cyContainer = root ? root.querySelector('.rel-cy-container') : null;
            if (cyContainer && cyContainer._relResizeObs) {
                cyContainer._relResizeObs.disconnect();
                delete cyContainer._relResizeObs;
            }
            if (cy) {
                try { cy.destroy(); } catch (e) { console.debug('Error destroying cytoscape:', e); }
                cy = null;
            }
            clearTimeout(hideTimeout);
        }

        function openTab() {
            bindStaticControls();
            const run = () => {
                if (cy) {
                    resizeGraph();
                    resetView();
                    return;
                }
                initGraph();
            };
            requestAnimationFrame(() => requestAnimationFrame(run));
            if (typeof ResizeObserver !== 'undefined') {
                const root = relTab();
                const cyContainer = root ? root.querySelector('.rel-cy-container') : null;
                if (cyContainer && !cyContainer._relResizeObs) {
                    let resizeRaf = null;
                    const obs = new ResizeObserver(() => {
                        if (resizeRaf) cancelAnimationFrame(resizeRaf);
                        resizeRaf = requestAnimationFrame(() => {
                            resizeRaf = null;
                            resizeGraph();
                        });
                    });
                    obs.observe(cyContainer);
                    cyContainer._relResizeObs = obs;
                }
            }
        }

        function open() {
            if (Modals.Contacts?.open) {
                Modals.Contacts.open('relationships');
            }
        }

        function init() {
            bindStaticControls();
        }

        return { init, open, openTab, tearDown };
})();


