'use strict';

Modals.IdentityProfileWizard = (() => {
    const TOTAL_STEPS = 9;
    const STEP_TITLES = [
        'Basic Identity', 'Health', 'Family', 'Education',
        'Career', 'Relationships & Social', 'Interests & Hobbies', 'Personal Profile',
        'Additional Information'
    ];

    let currentStep = 0;
    let formData = {};
    let existingDocId = null;

    // ── Field descriptors per step ────────────────────────────────────────────

    // Maps step index → array of {id, key, subKey?, type}
    // type: 'input' | 'select' | 'textarea' | 'chips' | 'custom-input'
    const STEP_FIELDS = [
        // Step 0: Basic Identity
        [
            { id: 'wf-full-name',      key: 'basic', subKey: 'full_name',     type: 'input' },
            { id: 'wf-preferred-name', key: 'basic', subKey: 'preferred_name',type: 'input' },
            { id: 'wf-dob',            key: 'basic', subKey: 'date_of_birth', type: 'input' },
            { id: 'wf-gender',         key: 'basic', subKey: 'gender',        type: 'select' },
            { id: 'wf-nationality',    key: 'basic', subKey: 'nationality',   type: 'input' },
            { id: 'wf-residence',      key: 'basic', subKey: 'residence',     type: 'input' },
            { id: 'wf-emails',         key: 'basic', subKey: 'emails',        type: 'input' },
            { id: 'wf-phones',         key: 'basic', subKey: 'phones',        type: 'input' },
            { id: 'wf-linkedin',       key: 'basic', subKey: 'linkedin',      type: 'input' },
            { id: 'wf-social',         key: 'basic', subKey: 'social',        type: 'input' },
            { id: 'wf-handedness',     key: 'basic', subKey: 'handedness',    type: 'select' },
            { id: 'wf-religion',       key: 'basic', subKey: 'religion',      type: 'input' },
            { id: 'wf-basic-other',    key: 'basic', subKey: 'other',         type: 'textarea' },
        ],
        // Step 1: Health
        [
            { id: 'wf-conditions-chips',  key: 'health', subKey: 'conditions',       type: 'chips' },
            { id: 'wf-conditions-custom', key: 'health', subKey: 'conditions_extra',  type: 'custom-input' },
            { id: 'wf-hospitalisations',  key: 'health', subKey: 'hospitalisations',  type: 'textarea' },
            { id: 'wf-surgeries',         key: 'health', subKey: 'surgeries',         type: 'textarea' },
            { id: 'wf-mental-health',     key: 'health', subKey: 'mental_health',     type: 'textarea' },
        ],
        // Step 2: Family
        [
            { id: 'wf-parents',       key: 'family', subKey: 'parents',    type: 'textarea' },
            { id: 'wf-siblings',      key: 'family', subKey: 'siblings',   type: 'textarea' },
            { id: 'wf-extended-family',key:'family', subKey: 'extended',   type: 'textarea' },
            { id: 'wf-early-life',    key: 'family', subKey: 'early_life', type: 'textarea' },
        ],
        // Step 3: Education
        [
            { id: 'wf-primary',    key: 'education', subKey: 'primary',    type: 'textarea' },
            { id: 'wf-secondary',  key: 'education', subKey: 'secondary',  type: 'textarea' },
            { id: 'wf-university', key: 'education', subKey: 'university', type: 'textarea' },
            { id: 'wf-vocational', key: 'education', subKey: 'vocational', type: 'textarea' },
        ],
        // Step 4: Career
        [
            { id: 'wf-career-summary',   key: 'career', subKey: 'summary',   type: 'textarea' },
            { id: 'wf-career-timeline',  key: 'career', subKey: 'timeline',  type: 'textarea' },
            { id: 'wf-career-skills',    key: 'career', subKey: 'skills',    type: 'textarea' },
            { id: 'wf-career-anecdotes', key: 'career', subKey: 'anecdotes', type: 'textarea' },
        ],
        // Step 5: Relationships
        [
            { id: 'wf-romantic-history', key: 'relationships', subKey: 'romantic_history', type: 'textarea' },
            { id: 'wf-close-friends',    key: 'relationships', subKey: 'close_friends',    type: 'textarea' },
            { id: 'wf-social-notes',     key: 'relationships', subKey: 'social_notes',     type: 'textarea' },
        ],
        // Step 6: Interests
        [
            { id: 'wf-sports-chips',       key: 'interests', subKey: 'sports',       type: 'chips' },
            { id: 'wf-arts-chips',         key: 'interests', subKey: 'arts',         type: 'chips' },
            { id: 'wf-intellectual-chips', key: 'interests', subKey: 'intellectual', type: 'chips' },
            { id: 'wf-music',              key: 'interests', subKey: 'music',        type: 'textarea' },
            { id: 'wf-travel',             key: 'interests', subKey: 'travel',       type: 'textarea' },
            { id: 'wf-technology',         key: 'interests', subKey: 'technology',   type: 'textarea' },
            { id: 'wf-other-interests',    key: 'interests', subKey: 'other_interests', type: 'textarea' },
        ],
        // Step 7: Personal Profile
        [
            { id: 'wf-communication-style', key: 'personal', subKey: 'communication_style',  type: 'textarea' },
            { id: 'wf-values',              key: 'personal', subKey: 'values',                type: 'textarea' },
            { id: 'wf-rules-for-life',      key: 'personal', subKey: 'rules_for_life',        type: 'textarea' },
            { id: 'wf-psychological-notes', key: 'personal', subKey: 'psychological_notes',   type: 'textarea' },
            { id: 'wf-quotes',              key: 'personal', subKey: 'quotes',                type: 'textarea' },
        ],
        // Step 8: Additional Information
        [
            { id: 'wf-additional-info', key: 'additional', subKey: 'notes', type: 'textarea' },
        ],
    ];

    // ── Open / close ──────────────────────────────────────────────────────────

    async function open() {
        formData = {};
        existingDocId = null;
        currentStep = -1; // prevent collectStep overwriting loaded data on first goToStep
        await loadProfile();
        const modal = document.getElementById('identity-profile-wizard-modal');
        if (modal) modal.style.display = 'flex';
        goToStep(0);
    }

    function close() {
        const modal = document.getElementById('identity-profile-wizard-modal');
        if (modal) modal.style.display = 'none';
    }

    // ── Load existing profile ─────────────────────────────────────────────────

    async function loadProfile() {
        try {
            const res = await fetch('/api/identity-profile', { credentials: 'same-origin' });
            if (!res.ok) return;
            const data = await res.json();
            if (!data.exists) return;
            existingDocId = data.doc_id || null;
            // notes field stores formData JSON for re-population
            if (data.notes) {
                try {
                    const saved = JSON.parse(data.notes);
                    if (saved && typeof saved === 'object') {
                        formData = saved;
                    }
                } catch (_) {}
            }
        } catch (_) {}
    }

    // ── Step navigation ───────────────────────────────────────────────────────

    function goToStep(n) {
        collectStep(currentStep);
        currentStep = Math.max(0, Math.min(n, TOTAL_STEPS - 1));
        // Show/hide step panels
        for (let i = 0; i < TOTAL_STEPS; i++) {
            const el = document.getElementById('wizard-step-' + i);
            if (el) el.style.display = i === currentStep ? '' : 'none';
        }
        renderStep(currentStep);
        updateProgressBar();
        updateNav();
    }

    function updateProgressBar() {
        const bar = document.getElementById('wizard-progress-bar');
        if (!bar) return;
        bar.innerHTML = '';
        for (let i = 0; i < TOTAL_STEPS; i++) {
            const dot = document.createElement('div');
            dot.className = 'wizard-step-indicator' +
                (i === currentStep ? ' active' : i < currentStep ? ' done' : '');
            dot.title = STEP_TITLES[i];
            dot.addEventListener('click', () => goToStep(i));
            bar.appendChild(dot);
        }
    }

    function updateNav() {
        const prevBtn  = document.getElementById('wizard-prev-btn');
        const nextBtn  = document.getElementById('wizard-next-btn');
        const saveBtn  = document.getElementById('wizard-save-btn');
        const labelEl  = document.getElementById('wizard-step-label');
        if (prevBtn) prevBtn.style.display = currentStep === 0 ? 'none' : '';
        const isLast = currentStep === TOTAL_STEPS - 1;
        if (nextBtn) nextBtn.style.display = isLast ? 'none' : '';
        if (saveBtn) saveBtn.style.display = isLast ? '' : 'none';
        if (labelEl) labelEl.textContent = `Step ${currentStep + 1} of ${TOTAL_STEPS} — ${STEP_TITLES[currentStep]}`;
    }

    // ── Collect step data from DOM into formData ───────────────────────────────

    function collectStep(stepIdx) {
        if (stepIdx < 0 || stepIdx >= STEP_FIELDS.length) return;
        STEP_FIELDS[stepIdx].forEach(f => {
            const el = document.getElementById(f.id);
            if (!el) return;
            if (!formData[f.key]) formData[f.key] = {};
            if (f.type === 'chips') {
                const selected = el.querySelectorAll('.wizard-chip.selected');
                formData[f.key][f.subKey] = Array.from(selected).map(b => b.dataset.value).join(', ');
            } else {
                formData[f.key][f.subKey] = el.value || '';
            }
        });
    }

    // ── Render step — populate DOM from formData ───────────────────────────────

    function renderStep(stepIdx) {
        if (stepIdx < 0 || stepIdx >= STEP_FIELDS.length) return;
        STEP_FIELDS[stepIdx].forEach(f => {
            const el = document.getElementById(f.id);
            if (!el) return;
            const val = (formData[f.key] && formData[f.key][f.subKey]) ? formData[f.key][f.subKey] : '';
            if (f.type === 'chips') {
                const selected = val ? val.split(',').map(s => s.trim()).filter(Boolean) : [];
                el.querySelectorAll('.wizard-chip').forEach(b => {
                    b.classList.toggle('selected', selected.includes(b.dataset.value));
                });
            } else {
                el.value = val;
            }
        });
    }

    // ── Build Markdown from formData ──────────────────────────────────────────

    function buildMarkdown() {
        const b = formData.basic || {};
        const name = b.full_name || 'Unknown';
        const now = new Date().toLocaleDateString('en-AU', { year: 'numeric', month: 'long', day: 'numeric' });
        let md = `# ${name} — Identity & Core Profile\n*Last updated: ${now}*\n\n`;

        // Basic Identity
        md += `## Basic Identity\n`;
        if (b.full_name)      md += `- Full name: ${b.full_name}\n`;
        if (b.preferred_name) md += `- Preferred name: ${b.preferred_name}\n`;
        if (b.date_of_birth)  md += `- Date of birth: ${b.date_of_birth}\n`;
        if (b.gender)         md += `- Gender: ${b.gender}\n`;
        if (b.nationality)    md += `- Nationality: ${b.nationality}\n`;
        if (b.residence)      md += `- Current residence: ${b.residence}\n`;
        if (b.emails)         md += `- Email: ${b.emails}\n`;
        if (b.phones)         md += `- Phone: ${b.phones}\n`;
        if (b.linkedin)       md += `- LinkedIn: ${b.linkedin}\n`;
        if (b.social)         md += `- Other social/web: ${b.social}\n`;
        if (b.handedness)     md += `- Handedness: ${b.handedness}\n`;
        if (b.religion)       md += `- Religion/beliefs: ${b.religion}\n`;
        if (b.other)          md += `\n${b.other}\n`;
        md += '\n';

        // Health
        const h = formData.health || {};
        const conditions = [h.conditions, h.conditions_extra].filter(Boolean).join(', ');
        if (conditions || h.hospitalisations || h.surgeries || h.mental_health) {
            md += `## Physical & Mental Health\n`;
            if (conditions)          md += `- Medical conditions: ${conditions}\n`;
            if (h.hospitalisations)  md += `\n### Hospitalisations\n${h.hospitalisations}\n`;
            if (h.surgeries)         md += `\n### Surgeries\n${h.surgeries}\n`;
            if (h.mental_health)     md += `\n### Mental Health\n${h.mental_health}\n`;
            md += '\n';
        }

        // Family
        const f = formData.family || {};
        if (f.parents || f.siblings || f.extended || f.early_life) {
            md += `## Family\n`;
            if (f.parents)   md += `### Parents\n${f.parents}\n\n`;
            if (f.siblings)  md += `### Siblings\n${f.siblings}\n\n`;
            if (f.extended)  md += `### Extended Family\n${f.extended}\n\n`;
            if (f.early_life)md += `### Early Family Life\n${f.early_life}\n\n`;
        }

        // Education
        const e = formData.education || {};
        if (e.primary || e.secondary || e.university || e.vocational) {
            md += `## Education & Qualifications\n`;
            if (e.primary)    md += `### Primary School\n${e.primary}\n\n`;
            if (e.secondary)  md += `### High School / Secondary\n${e.secondary}\n\n`;
            if (e.university) md += `### University / Tertiary\n${e.university}\n\n`;
            if (e.vocational) md += `### Vocational & Professional\n${e.vocational}\n\n`;
        }

        // Career
        const c = formData.career || {};
        if (c.summary || c.timeline || c.skills || c.anecdotes) {
            md += `## Career History\n`;
            if (c.summary)   md += `${c.summary}\n\n`;
            if (c.timeline)  md += `### Career Timeline\n${c.timeline}\n\n`;
            if (c.skills)    md += `### Key Skills\n${c.skills}\n\n`;
            if (c.anecdotes) md += `### Career Highlights & Anecdotes\n${c.anecdotes}\n\n`;
        }

        // Relationships
        const rel = formData.relationships || {};
        if (rel.romantic_history || rel.close_friends || rel.social_notes) {
            md += `## Relationships & Social Life\n`;
            if (rel.romantic_history) md += `### Romantic History\n${rel.romantic_history}\n\n`;
            if (rel.close_friends)    md += `### Close Friends\n${rel.close_friends}\n\n`;
            if (rel.social_notes)     md += `### Social Notes\n${rel.social_notes}\n\n`;
        }

        // Interests
        const i = formData.interests || {};
        const hasInterests = i.sports || i.arts || i.intellectual || i.music || i.travel || i.technology || i.other_interests;
        if (hasInterests) {
            md += `## Interests, Hobbies & Travel\n`;
            if (i.sports)       md += `- Sports & fitness: ${i.sports}\n`;
            if (i.arts)         md += `- Arts & culture: ${i.arts}\n`;
            if (i.intellectual) md += `- Intellectual interests: ${i.intellectual}\n`;
            if (i.music)        md += `\n### Music\n${i.music}\n`;
            if (i.travel)       md += `\n### Travel\n${i.travel}\n`;
            if (i.technology)   md += `\n### Technology\n${i.technology}\n`;
            if (i.other_interests) md += `\n### Other Interests\n${i.other_interests}\n`;
            md += '\n';
        }

        // Personal profile
        const p = formData.personal || {};
        if (p.communication_style || p.values || p.rules_for_life || p.psychological_notes || p.quotes) {
            md += `## Personal Profile & Values\n`;
            if (p.communication_style)  md += `### Communication Style\n${p.communication_style}\n\n`;
            if (p.values)               md += `### Values & Beliefs\n${p.values}\n\n`;
            if (p.rules_for_life)       md += `### Rules for Life\n${p.rules_for_life}\n\n`;
            if (p.psychological_notes)  md += `### Psychological & Personality Notes\n${p.psychological_notes}\n\n`;
            if (p.quotes)               md += `### Favourite Quotes & Influences\n${p.quotes}\n\n`;
        }

        // Additional Information
        const add = formData.additional || {};
        if (add.notes) {
            md += `## Additional Information\n${add.notes}\n\n`;
        }

        return md.trim();
    }

    // ── Prefill from AI extraction ────────────────────────────────────────────

    // Converts any AI-returned value (string, array, nested object) into a human-readable string.
    function aiValueToString(v) {
        if (v === null || v === undefined) return '';
        if (typeof v === 'string') return v;
        if (typeof v === 'number' || typeof v === 'boolean') return String(v);
        if (Array.isArray(v)) {
            return v.map(item => {
                if (item === null || item === undefined) return '';
                if (typeof item === 'string') return item;
                if (typeof item === 'object') {
                    // Format object entries as "key: value" joined by " — "
                    return Object.entries(item)
                        .filter(([, val]) => val !== null && val !== undefined && val !== '')
                        .map(([key, val]) => `${val}`)
                        .join(' — ');
                }
                return String(item);
            }).filter(Boolean).join('\n');
        }
        if (typeof v === 'object') {
            // Plain object: format as "key: value" lines
            return Object.entries(v)
                .filter(([, val]) => val !== null && val !== undefined && val !== '')
                .map(([key, val]) => `${key}: ${aiValueToString(val)}`)
                .join('\n');
        }
        return String(v);
    }

    function prefillFromExtracted(fields) {
        if (!fields || typeof fields !== 'object') return;
        const sections = ['basic', 'health', 'family', 'education', 'career', 'relationships', 'interests', 'personal', 'additional'];
        sections.forEach(sec => {
            if (fields[sec] && typeof fields[sec] === 'object') {
                if (!formData[sec]) formData[sec] = {};
                Object.keys(fields[sec]).forEach(k => {
                    const v = fields[sec][k];
                    if (v !== null && v !== undefined && v !== '') {
                        const str = aiValueToString(v);
                        if (str) formData[sec][k] = str;
                    }
                });
            }
        });
        renderStep(currentStep);
    }

    // ── Save profile ──────────────────────────────────────────────────────────

    async function saveProfile() {
        collectStep(currentStep);
        const markdown = buildMarkdown();
        const b = formData.basic || {};

        const saveBtn = document.getElementById('wizard-save-btn');
        if (saveBtn) { saveBtn.disabled = true; saveBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Saving…'; }

        try {
            const res = await fetch('/api/identity-profile', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    markdown,
                    form_data_json: JSON.stringify(formData),
                    subject_name:    b.preferred_name || b.full_name || '',
                    gender:          b.gender || '',
                    email_addresses: b.emails || '',
                    phone_numbers:   b.phones || '',
                }),
            });
            if (!res.ok) {
                const err = await res.json().catch(() => ({}));
                throw new Error(err.detail || `HTTP ${res.status}`);
            }
            const data = await res.json();
            existingDocId = data.doc_id || null;
            if (typeof markOnboardingChecklistStepDone === 'function') {
                markOnboardingChecklistStepDone('identity_profile');
            }
            close();
            // Flash a green success banner in the main UI after modal closes
            const errEl = document.getElementById('error-display');
            if (errEl) {
                const prevDisplay = errEl.style.display;
                const prevBg = errEl.style.background;
                const prevColor = errEl.style.color;
                const prevText = errEl.textContent;
                errEl.textContent = 'Identity profile saved.';
                errEl.style.display = 'block';
                errEl.style.background = '#2e7d32';
                errEl.style.color = '#fff';
                setTimeout(() => {
                    errEl.style.display = prevDisplay;
                    errEl.style.background = prevBg;
                    errEl.style.color = prevColor;
                    errEl.textContent = prevText;
                }, 3000);
            }
        } catch (err) {
            // Show error inside the modal (UI.displayError appears behind modal)
            const statusEl = document.getElementById('wizard-step-label');
            if (statusEl) {
                const prev = statusEl.textContent;
                statusEl.textContent = 'Save failed: ' + err.message;
                statusEl.style.color = 'var(--error, #c62828)';
                setTimeout(() => { statusEl.textContent = prev; statusEl.style.color = ''; }, 5000);
            }
        } finally {
            if (saveBtn) { saveBtn.disabled = false; saveBtn.innerHTML = '<i class="fas fa-save"></i> Save Profile'; }
        }
    }

    // ── AI document parse ─────────────────────────────────────────────────────

    async function parseDocument() {
        const input = document.getElementById('wizard-parse-input');
        const status = document.getElementById('wizard-parse-status');
        const btn = document.getElementById('wizard-parse-btn');
        if (!input || !input.value.trim()) return;

        if (btn) { btn.disabled = true; btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Extracting…'; }
        if (status) status.textContent = 'Calling AI…';

        try {
            const res = await fetch('/api/identity-profile/parse', {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ text: input.value }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            if (data.warning) {
                if (status) status.textContent = 'Partial extraction: ' + data.warning;
            } else {
                if (status) status.textContent = 'Extraction complete — review each step.';
            }
            prefillFromExtracted(data.fields || {});
            // Collapse the parse section
            const details = document.getElementById('wizard-parse-details');
            if (details) details.open = false;
        } catch (err) {
            if (status) status.textContent = 'Extraction failed: ' + err.message;
        } finally {
            if (btn) { btn.disabled = false; btn.innerHTML = '<i class="fas fa-magic"></i> Extract with AI'; }
        }
    }

    // ── Init ──────────────────────────────────────────────────────────────────

    function init() {
        // Close button
        const closeBtn = document.getElementById('close-identity-profile-wizard');
        if (closeBtn) closeBtn.addEventListener('click', close);

        // Sidebar button
        const sidebarBtn = document.getElementById('identity-profile-wizard-btn');
        if (sidebarBtn) sidebarBtn.addEventListener('click', () => open());

        // Onboarding checklist button
        const checklistBtn = document.getElementById('onboarding-checklist-identity-btn');
        if (checklistBtn) checklistBtn.addEventListener('click', () => open());

        // Navigation buttons
        const prevBtn = document.getElementById('wizard-prev-btn');
        const nextBtn = document.getElementById('wizard-next-btn');
        const saveBtn = document.getElementById('wizard-save-btn');
        const parseBtn = document.getElementById('wizard-parse-btn');
        if (prevBtn) prevBtn.addEventListener('click', () => goToStep(currentStep - 1));
        if (nextBtn) nextBtn.addEventListener('click', () => goToStep(currentStep + 1));
        if (saveBtn) saveBtn.addEventListener('click', saveProfile);
        if (parseBtn) parseBtn.addEventListener('click', parseDocument);

        // Chip click-toggle for all chip containers
        document.querySelectorAll('.wizard-chip-container').forEach(container => {
            container.addEventListener('click', e => {
                const chip = e.target.closest('.wizard-chip');
                if (chip) chip.classList.toggle('selected');
            });
        });
    }

    return { open, close, init };
})();
