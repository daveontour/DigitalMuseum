'use strict';

/**
 * Memory Challenge (Personal Trivia) — test how well you remember the people, places,
 * photos and conversations in your archive.
 * Data: GET /api/trivia (internal/handler/memories_handler.go -> repository.TriviaRepo).
 *
 * Loaded after app.js, so it self-initialises at the bottom of the file.
 */
const MemoryChallenge = (() => {
    let questions = [];
    let currentIndex = 0;
    let score = 0;
    let answered = false;
    let inFlight = false;

    function el(id) { return document.getElementById(id); }

    function openItem(kind, id) {
        if (typeof Chat !== 'undefined' && typeof Chat.openSource === 'function') {
            void Chat.openSource(kind, id);
        }
    }

    async function errorText(res) {
        try {
            const body = await res.json();
            if (body && (body.detail || body.error)) return body.detail || body.error;
        } catch (_) { /* not JSON */ }
        return `HTTP ${res.status}`;
    }

    async function open() {
        const modal = el('trivia-modal');
        if (!modal) return;
        modal.style.display = 'flex';
        await startNewGame();
    }

    function close() {
        const modal = el('trivia-modal');
        if (modal) modal.style.display = 'none';
    }

    async function startNewGame() {
        if (inFlight) return;
        inFlight = true;
        currentIndex = 0;
        score = 0;
        answered = false;
        questions = [];

        const card = el('trivia-card');
        const headerBar = el('trivia-header-bar');
        if (headerBar) headerBar.style.display = 'none';
        if (card) {
            card.innerHTML = `
                <div class="trivia-loading">
                    <i class="fas fa-spinner fa-spin fa-2x" aria-hidden="true"></i>
                    <h3>Assembling your Memory Challenge…</h3>
                    <p>Scanning your photos, messages, emails and people to craft your trivia.</p>
                </div>
            `;
        }

        try {
            const res = await fetch('/api/trivia?count=10', { credentials: 'same-origin' });
            if (!res.ok) throw new Error(await errorText(res));
            const data = await res.json();
            questions = Array.isArray(data.questions) ? data.questions : [];
        } catch (err) {
            if (card) {
                card.innerHTML = `
                    <div class="trivia-empty">
                        <i class="fas fa-exclamation-triangle fa-2x" style="color:var(--color-danger);"></i>
                        <h3>Could not load questions</h3>
                        <p>${err.message}</p>
                        <button type="button" class="modal-btn modal-btn-primary" id="trivia-retry-btn">Try Again</button>
                    </div>
                `;
                el('trivia-retry-btn')?.addEventListener('click', () => { void startNewGame(); });
            }
            return;
        } finally {
            inFlight = false;
        }

        if (questions.length === 0) {
            if (card) {
                card.innerHTML = `
                    <div class="trivia-empty">
                        <i class="fas fa-brain fa-2x" style="color:var(--color-blue);"></i>
                        <h3>Not enough data yet</h3>
                        <p>The Memory Challenge needs photos, messages or named people in your archive to build trivia questions.</p>
                        <p>Import some records via <strong>Source Data</strong> or identify a few faces in <strong>People in Photos</strong> to unlock challenges!</p>
                    </div>
                `;
            }
            return;
        }

        if (headerBar) headerBar.style.display = 'flex';
        renderQuestion();
    }

    function updateProgress() {
        const total = questions.length;
        const current = currentIndex + 1;
        const progressPct = Math.round((currentIndex / total) * 100);

        const counterEl = el('trivia-question-counter');
        if (counterEl) counterEl.textContent = `Question ${current} of ${total}`;

        const scoreEl = el('trivia-score-badge');
        if (scoreEl) scoreEl.textContent = `Score: ${score}`;

        const fillEl = el('trivia-progress-fill');
        if (fillEl) fillEl.style.width = `${progressPct}%`;
    }

    function renderQuestion() {
        answered = false;
        updateProgress();

        const q = questions[currentIndex];
        const card = el('trivia-card');
        if (!card || !q) return;

        card.innerHTML = '';

        const qWrap = document.createElement('div');
        qWrap.className = 'trivia-question-wrap';

        // Optional media image (face crop or photo)
        if (q.image_url) {
            const imgWrap = document.createElement('div');
            imgWrap.className = 'trivia-media-wrap';
            const img = document.createElement('img');
            img.className = 'trivia-media-img';
            img.src = q.image_url;
            img.alt = 'Memory challenge image';
            img.onerror = function() { this.style.display = 'none'; };
            imgWrap.appendChild(img);
            qWrap.appendChild(imgWrap);
        }

        // Optional quote box
        if (q.quote) {
            const quoteEl = document.createElement('blockquote');
            quoteEl.className = 'trivia-quote-box';
            quoteEl.textContent = q.quote;
            qWrap.appendChild(quoteEl);
        }

        // Question text
        const qTitle = document.createElement('h3');
        qTitle.className = 'trivia-question-text';
        qTitle.textContent = q.question;
        qWrap.appendChild(qTitle);

        // Options Grid (4 options)
        const optsGrid = document.createElement('div');
        optsGrid.className = 'trivia-options-grid';

        const optLetters = ['A', 'B', 'C', 'D'];
        (q.options || []).forEach((optText, idx) => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'trivia-option-btn';
            btn.id = `trivia-opt-${idx}`;
            btn.innerHTML = `<span class="trivia-opt-letter">${optLetters[idx] || ''}</span><span class="trivia-opt-text">${escapeHtml(optText)}</span>`;
            btn.addEventListener('click', () => selectAnswer(idx));
            optsGrid.appendChild(btn);
        });
        qWrap.appendChild(optsGrid);

        // Feedback container (hidden until answered)
        const feedback = document.createElement('div');
        feedback.id = 'trivia-feedback';
        feedback.className = 'trivia-feedback';
        feedback.style.display = 'none';
        qWrap.appendChild(feedback);

        card.appendChild(qWrap);
    }

    function selectAnswer(selectedIndex) {
        if (answered) return;
        answered = true;

        const q = questions[currentIndex];
        const isCorrect = (selectedIndex === q.correct_index);
        if (isCorrect) {
            score++;
            updateProgress();
        }

        // Highlight options
        (q.options || []).forEach((_, idx) => {
            const btn = el(`trivia-opt-${idx}`);
            if (!btn) return;
            btn.disabled = true;
            if (idx === q.correct_index) {
                btn.classList.add('trivia-opt--correct');
            } else if (idx === selectedIndex) {
                btn.classList.add('trivia-opt--wrong');
            }
        });

        // Show feedback
        const feedback = el('trivia-feedback');
        if (!feedback) return;
        feedback.style.display = 'block';
        feedback.className = `trivia-feedback ${isCorrect ? 'trivia-feedback--correct' : 'trivia-feedback--wrong'}`;

        const isLast = (currentIndex === questions.length - 1);
        const iconHtml = isCorrect
            ? '<i class="fas fa-check-circle" style="color:var(--color-success); margin-right:6px;"></i> Correct!'
            : '<i class="fas fa-times-circle" style="color:var(--color-danger); margin-right:6px;"></i> Not quite!';

        let sourceBtnHtml = '';
        if (q.source && q.source.kind && q.source.id) {
            sourceBtnHtml = `
                <button type="button" class="modal-btn modal-btn-secondary" id="trivia-view-source-btn">
                    <i class="fas fa-external-link-alt" aria-hidden="true"></i> View in Archive
                </button>
            `;
        }

        feedback.innerHTML = `
            <div class="trivia-feedback-header">${iconHtml}</div>
            <div class="trivia-feedback-explanation">${escapeHtml(q.explanation || '')}</div>
            <div class="trivia-feedback-actions">
                ${sourceBtnHtml}
                <button type="button" class="modal-btn modal-btn-primary" id="trivia-next-btn">
                    ${isLast ? 'See Results <i class="fas fa-arrow-right"></i>' : 'Next Question <i class="fas fa-arrow-right"></i>'}
                </button>
            </div>
        `;

        if (q.source && q.source.kind && q.source.id) {
            el('trivia-view-source-btn')?.addEventListener('click', () => {
                openItem(q.source.kind, q.source.id);
            });
        }

        el('trivia-next-btn')?.addEventListener('click', () => {
            if (isLast) {
                showResults();
            } else {
                currentIndex++;
                renderQuestion();
            }
        });
    }

    function showResults() {
        const headerBar = el('trivia-header-bar');
        if (headerBar) headerBar.style.display = 'none';

        const card = el('trivia-card');
        if (!card) return;

        const total = questions.length;
        const pct = Math.round((score / total) * 100);

        let headline = 'Great Walk Down Memory Lane!';
        let iconCls = 'fa-star';
        let iconColor = 'var(--color-blue)';

        if (pct === 100) {
            headline = 'Flawless Recall!';
            iconCls = 'fa-crown';
            iconColor = '#fbbf24';
        } else if (pct >= 80) {
            headline = 'Incredible Memory!';
            iconCls = 'fa-trophy';
            iconColor = '#fbbf24';
        } else if (pct >= 50) {
            headline = 'Well Remembered!';
            iconCls = 'fa-award';
            iconColor = '#3b82f6';
        } else {
            headline = 'A Fun Refresher!';
            iconCls = 'fa-lightbulb';
            iconColor = '#10b981';
        }

        card.innerHTML = `
            <div class="trivia-result-screen">
                <div class="trivia-result-icon" style="color:${iconColor};"><i class="fas ${iconCls}"></i></div>
                <h2 class="trivia-result-headline">${headline}</h2>
                <div class="trivia-score-circle">
                    <span class="trivia-score-num">${score}</span>
                    <span class="trivia-score-total">/ ${total}</span>
                </div>
                <p class="trivia-result-summary">You answered <strong>${score} out of ${total}</strong> questions correctly (${pct}% accuracy).</p>
                <div class="trivia-result-actions">
                    <button type="button" class="modal-btn modal-btn-primary" id="trivia-play-again-btn">
                        <i class="fas fa-redo"></i> Play Again
                    </button>
                    <button type="button" class="modal-btn modal-btn-secondary" id="trivia-finish-btn">
                        Close
                    </button>
                </div>
            </div>
        `;

        el('trivia-play-again-btn')?.addEventListener('click', () => { void startNewGame(); });
        el('trivia-finish-btn')?.addEventListener('click', close);
    }

    function escapeHtml(s) {
        const d = document.createElement('div');
        d.textContent = s || '';
        return d.innerHTML;
    }

    function init() {
        el('trivia-sidebar-btn')?.addEventListener('click', () => { void open(); });
        el('close-trivia-modal')?.addEventListener('click', close);
        const modal = el('trivia-modal');
        modal?.addEventListener('click', (e) => { if (e.target === modal) close(); });
    }

    return { init, open, close, startNewGame };
})();

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => MemoryChallenge.init());
} else {
    MemoryChallenge.init();
}
