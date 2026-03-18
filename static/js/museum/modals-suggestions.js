'use strict';

Modals.Suggestions = (() => {
        function executeSuggestion(sugg, cat) {
            Modals._closeModal(DOM.suggestionsModal);
            if (sugg.function && AppActions[sugg.function]) {
                AppActions[sugg.function]();
            } else {
                App.processFormSubmit(sugg.prompt, cat.category, sugg.title);
            }
        }

        function toggleCategory(headerRow) {
            const category = headerRow.dataset.category;
            const expanded = headerRow.dataset.expanded !== 'false';
            const icon = headerRow.querySelector('.suggestions-category-toggle');
            const rows = DOM.suggestionsListContainer.querySelectorAll(`tr.suggestions-table-row[data-category="${CSS.escape(category)}"]`);

            if (expanded) {
                rows.forEach(r => { r.classList.add('suggestions-row-collapsed'); });
                headerRow.dataset.expanded = 'false';
                if (icon) {
                    icon.classList.remove('fa-chevron-down');
                    icon.classList.add('fa-chevron-right');
                }
            } else {
                // Close any other open categories first
                const allHeaders = DOM.suggestionsListContainer.querySelectorAll('tr.suggestions-category-header');
                allHeaders.forEach(otherHeader => {
                    if (otherHeader !== headerRow && otherHeader.dataset.expanded === 'true') {
                        const otherCategory = otherHeader.dataset.category;
                        const otherRows = DOM.suggestionsListContainer.querySelectorAll(`tr.suggestions-table-row[data-category="${CSS.escape(otherCategory)}"]`);
                        otherRows.forEach(r => { r.classList.add('suggestions-row-collapsed'); });
                        otherHeader.dataset.expanded = 'false';
                        const otherIcon = otherHeader.querySelector('.suggestions-category-toggle');
                        if (otherIcon) {
                            otherIcon.classList.remove('fa-chevron-down');
                            otherIcon.classList.add('fa-chevron-right');
                        }
                    }
                });

                rows.forEach(r => { r.classList.remove('suggestions-row-collapsed'); });
                headerRow.dataset.expanded = 'true';
                if (icon) {
                    icon.classList.remove('fa-chevron-right');
                    icon.classList.add('fa-chevron-down');
                }
            }
        }

        function open() {
            Modals._openModal(DOM.suggestionsModal);
            DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">Loading...</td></tr>';
            ApiService.fetchSuggestionsConfig()
                .then(data => {
                    DOM.suggestionsListContainer.innerHTML = '';
                    if (data.categories && Array.isArray(data.categories)) {
                        data.categories.forEach(cat => {
                            if (Array.isArray(cat.suggestions)) {
                                const categoryHeader = document.createElement('tr');
                                categoryHeader.className = 'suggestions-category-header';
                                categoryHeader.dataset.category = cat.category;
                                categoryHeader.dataset.expanded = 'false';
                                categoryHeader.innerHTML = `
                                    <td colspan="3" class="suggestions-category-header-cell">
                                        <i class="fas fa-chevron-right suggestions-category-toggle"></i>
                                        <span class="suggestions-category-name">${cat.category}</span>
                                        <span class="suggestions-category-count">(${cat.suggestions.length})</span>
                                    </td>
                                `;
                                categoryHeader.addEventListener('click', () => toggleCategory(categoryHeader));
                                DOM.suggestionsListContainer.appendChild(categoryHeader);

                                cat.suggestions.forEach(sugg => {
                                    const tr = document.createElement('tr');
                                    tr.className = 'suggestions-table-row suggestions-row-collapsed';
                                    tr.dataset.category = cat.category;

                                    const titleCell = document.createElement('td');
                                    titleCell.className = 'suggestions-col-title';
                                    titleCell.colSpan = 2;
                                    titleCell.textContent = sugg.title;

                                    const actionCell = document.createElement('td');
                                    actionCell.className = 'suggestions-col-action';
                                    const btn = document.createElement('button');
                                    btn.type = 'button';
                                    btn.className = 'modal-btn modal-btn-primary suggestions-execute-btn';
                                    btn.textContent = 'Execute';
                                    btn.addEventListener('click', (e) => {
                                        e.stopPropagation();
                                        executeSuggestion(sugg, cat);
                                    });

                                    actionCell.appendChild(btn);
                                    tr.appendChild(titleCell);
                                    tr.appendChild(actionCell);
                                    DOM.suggestionsListContainer.appendChild(tr);
                                });
                            }
                        });
                        if (DOM.suggestionsListContainer.children.length === 0) {
                            DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">No suggestions found.</td></tr>';
                        }
                    } else {
                        DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #666;">No suggestions found.</td></tr>';
                    }
                })
                .catch(err => {
                    console.error("Failed to load suggestions:", err);
                    DOM.suggestionsListContainer.innerHTML = '<tr><td colspan="3" style="text-align: center; padding: 2em; color: #c00;">Failed to load suggestions.</td></tr>';
                    UI.displayError("Could not load suggestions: " + err.message);
                });
        }

        function close() {
            Modals._closeModal(DOM.suggestionsModal);
        }

        function init() {
            DOM.closeSuggestionsModalBtn.addEventListener('click', close);
            DOM.suggestionsModal.addEventListener('click', (e) => { if (e.target === DOM.suggestionsModal) close(); });
        }

        return { init, open, close };
})();

