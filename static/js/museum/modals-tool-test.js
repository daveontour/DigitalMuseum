'use strict';

Modals.ToolTest = (() => {
    let tools = [];
    let wired = false;

    function getEl(id) {
        return document.getElementById(id);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        const div = document.createElement('div');
        div.textContent = String(s);
        return div.innerHTML;
    }

    function showStatus(message, isError) {
        const el = getEl('tool-test-status');
        if (!el) return;
        el.classList.remove('is-visible', 'is-error', 'is-success');
        if (!message) {
            el.textContent = '';
            return;
        }
        el.textContent = message;
        el.classList.add('is-visible', isError ? 'is-error' : 'is-success');
    }

    function schemaProperties(tool) {
        const params = tool && tool.parameters;
        if (!params || typeof params !== 'object') return {};
        const props = params.properties;
        return props && typeof props === 'object' ? props : {};
    }

    function schemaRequired(tool) {
        const params = tool && tool.parameters;
        if (!params || !Array.isArray(params.required)) return [];
        return params.required.map(String);
    }

    function fieldType(prop) {
        if (!prop || typeof prop !== 'object') return 'string';
        if (prop.type === 'integer') return 'integer';
        if (prop.type === 'array') {
            const items = prop.items;
            if (items && items.type === 'integer') return 'integer_array';
        }
        return 'string';
    }

    function toolHasArguments(tool) {
        return Object.keys(schemaProperties(tool)).length > 0;
    }

    function buildArgFields(tool) {
        const props = schemaProperties(tool);
        const required = new Set(schemaRequired(tool));
        const keys = Object.keys(props);
        if (!keys.length) {
            return '<div class="tool-test-args-empty">No arguments required.</div>';
        }
        return keys.map((key) => {
            const prop = props[key] || {};
            const type = fieldType(prop);
            const label = escapeHtml(key) + (required.has(key) ? ' <span class="tool-test-required">*</span>' : '');
            const desc = prop.description ? `<div class="tool-test-arg-desc">${escapeHtml(prop.description)}</div>` : '';
            const inputId = `tool-test-arg-${tool.name}-${key}`.replace(/[^a-zA-Z0-9_-]/g, '_');
            let inputHtml = '';
            if (type === 'integer') {
                inputHtml = `<input type="number" step="1" class="form-control tool-test-arg-input" id="${inputId}" data-tool="${escapeHtml(tool.name)}" data-arg="${escapeHtml(key)}" data-type="integer">`;
            } else if (type === 'integer_array') {
                inputHtml = `<input type="text" class="form-control tool-test-arg-input" id="${inputId}" data-tool="${escapeHtml(tool.name)}" data-arg="${escapeHtml(key)}" data-type="integer_array" placeholder="e.g. 1, 2, 3">`;
            } else {
                inputHtml = `<input type="text" class="form-control tool-test-arg-input" id="${inputId}" data-tool="${escapeHtml(tool.name)}" data-arg="${escapeHtml(key)}" data-type="string">`;
            }
            return `<div class="tool-test-arg-row"><label for="${inputId}">${label}</label>${desc}${inputHtml}</div>`;
        }).join('');
    }

    function renderToolList() {
        const listEl = getEl('tool-test-tool-list');
        if (!listEl) return;
        listEl.innerHTML = '';
        tools.forEach((tool) => {
            const name = String(tool.name || '');
            const card = document.createElement('div');
            card.className = 'tool-test-tool-row app-card';
            card.dataset.toolName = name;
            const argsBlock = toolHasArguments(tool)
                ? `<details class="tool-test-args-details">
                    <summary>Arguments</summary>
                    <div class="tool-test-args">${buildArgFields(tool)}</div>
                   </details>`
                : '';
            card.innerHTML = `
                <div class="tool-test-tool-head">
                    <label class="tool-test-tool-select">
                        <input type="checkbox" class="tool-test-chk" data-name="${escapeHtml(name)}">
                        <code>${escapeHtml(name)}</code>
                    </label>
                </div>
                <div class="tool-test-tool-desc">${escapeHtml(tool.description || '')}</div>
                ${argsBlock}
            `;
            listEl.appendChild(card);
        });
    }

    function setAllChecked(checked) {
        document.querySelectorAll('#tool-test-tool-list .tool-test-chk').forEach((chk) => {
            chk.checked = checked;
        });
    }

    function parseArgValue(input) {
        const type = input.dataset.type || 'string';
        const raw = (input.value || '').trim();
        if (type === 'integer') {
            if (raw === '') return { empty: true, value: null };
            const n = parseInt(raw, 10);
            if (!Number.isFinite(n)) return { invalid: true };
            return { value: n };
        }
        if (type === 'integer_array') {
            if (raw === '') return { empty: true, value: [] };
            const parts = raw.split(',').map((s) => s.trim()).filter(Boolean);
            const nums = parts.map((p) => parseInt(p, 10));
            if (nums.some((n) => !Number.isFinite(n))) return { invalid: true };
            return { value: nums };
        }
        if (raw === '') return { empty: true, value: '' };
        return { value: raw };
    }

    function collectArguments(card, tool) {
        const args = {};
        const required = schemaRequired(tool);
        const missing = [];
        if (!card) return { args, missing: required.slice() };
        card.querySelectorAll('.tool-test-arg-input').forEach((input) => {
            const argName = input.dataset.arg;
            if (!argName) return;
            const parsed = parseArgValue(input);
            if (parsed.invalid) {
                missing.push(`${argName} (invalid format)`);
                return;
            }
            if (required.includes(argName) && parsed.empty) {
                missing.push(argName);
                return;
            }
            if (!parsed.empty) {
                args[argName] = parsed.value;
            }
        });
        return { args, missing };
    }

    function collectSelectedTools() {
        const selected = [];
        const errors = [];
        document.querySelectorAll('#tool-test-tool-list .tool-test-chk:checked').forEach((chk) => {
            const name = chk.dataset.name;
            const tool = tools.find((t) => t.name === name);
            if (!tool) return;
            const card = chk.closest('.tool-test-tool-row');
            const { args, missing } = collectArguments(card, tool);
            if (missing.length) {
                errors.push(`${name}: missing or invalid ${missing.join(', ')}`);
                return;
            }
            selected.push({ name, arguments: args });
        });
        return { selected, errors };
    }

    function renderResults(results) {
        const wrap = getEl('tool-test-results');
        if (!wrap) return;
        wrap.innerHTML = '';
        if (!Array.isArray(results) || !results.length) {
            wrap.innerHTML = '<div class="tool-test-results-empty">No results.</div>';
            return;
        }

        results.forEach((row) => {
            const card = document.createElement('div');
            card.className = 'tool-test-result-card app-card';
            const name = row.name || 'tool';
            const duration = row.duration_ms != null ? ` (${row.duration_ms} ms)` : '';
            const err = row.error;
            const inputJson = JSON.stringify(row.arguments || {}, null, 2);
            let outputJson;
            if (err) {
                outputJson = String(err);
            } else {
                outputJson = JSON.stringify(row.result != null ? row.result : {}, null, 2);
            }
            if (row.truncated) {
                outputJson += '\n\n(result truncated for display)';
            }
            card.innerHTML = `
                <div class="tool-test-result-title"><code>${escapeHtml(name)}</code>${escapeHtml(duration)}</div>
                <div class="tool-test-result-block">
                    <div class="tool-test-result-label">Input</div>
                    <pre class="tool-test-result-pre">${escapeHtml(inputJson)}</pre>
                </div>
                <div class="tool-test-result-block">
                    <div class="tool-test-result-label">${err ? 'Error' : 'Output'}</div>
                    <pre class="tool-test-result-pre${err ? ' tool-test-result-pre--error' : ''}">${escapeHtml(outputJson)}</pre>
                </div>
            `;
            wrap.appendChild(card);
        });
    }

    async function runSelected() {
        const runBtn = getEl('tool-test-run-btn');
        const { selected, errors } = collectSelectedTools();
        if (errors.length) {
            showStatus(errors.join(' · '), true);
            return;
        }
        if (!selected.length) {
            showStatus('Select at least one tool to run.', true);
            return;
        }
        showStatus('');
        const resultsEl = getEl('tool-test-results');
        if (resultsEl) resultsEl.innerHTML = '';
        const originalHtml = runBtn ? runBtn.innerHTML : '';
        if (runBtn) {
            runBtn.disabled = true;
            runBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Running…';
        }
        try {
            const res = await fetch('/api/settings/llm-tools/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ tools: selected })
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                showStatus(data.detail || data.error || `Request failed (${res.status})`, true);
                return;
            }
            renderResults(data.results || []);
            showStatus(`Ran ${selected.length} tool(s).`, false);
        } catch (e) {
            showStatus(e.message || 'Run failed', true);
        } finally {
            if (runBtn) {
                runBtn.disabled = false;
                runBtn.innerHTML = originalHtml;
            }
        }
    }

    async function load() {
        showStatus('');
        const listEl = getEl('tool-test-tool-list');
        const resultsEl = getEl('tool-test-results');
        if (listEl) listEl.innerHTML = '<div class="tool-test-loading">Loading tools…</div>';
        if (resultsEl) resultsEl.innerHTML = '';
        try {
            const res = await fetch('/api/settings/llm-tools/definitions', { credentials: 'same-origin' });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) {
                if (listEl) listEl.innerHTML = '';
                showStatus(data.detail || data.error || 'Could not load tool definitions. Unlock the keyring first.', true);
                return;
            }
            tools = Array.isArray(data.tools) ? data.tools : [];
            tools.sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')));
            renderToolList();
        } catch (e) {
            if (listEl) listEl.innerHTML = '';
            showStatus(e.message || 'Load failed', true);
        }
    }

    function init() {
        if (wired) return;
        wired = true;
        const selectAll = getEl('tool-test-select-all');
        const deselectAll = getEl('tool-test-deselect-all');
        const runBtn = getEl('tool-test-run-btn');
        if (selectAll) selectAll.addEventListener('click', () => setAllChecked(true));
        if (deselectAll) deselectAll.addEventListener('click', () => setAllChecked(false));
        if (runBtn) runBtn.addEventListener('click', () => { void runSelected(); });
    }

    return { init, load };
})();
