'use strict';

/**
 * "Message in the Stars" — dot-matrix text on a canvas inside #chat-starfield so the layer
 * shares the same opacity rhythm as the existing starfield (chat-starfield-layer-opacity).
 * Text is set only via assistant embedded JSON (`messageInStars`) through ChatMessageStars.
 * Each dot uses the same diameter palette as .chat-star--1 … --4 in museum_of.css.
 */
(function () {
    const STAR_DIAMETERS_CSS = [1, 1.5, 2.25, 3];
    const CELL = 4;
    const CHAR_COLS = 5;
    const CHAR_ROWS = 7;
    const CHAR_GAP_CELLS = 1;
    const TWINKLE_SPEED = 0.035;
    const LINE_GAP = 2;
    const MAX_STARS = 650;
    /** null = no LLM value (empty canvas); string = text from embedded JSON messageInStars */
    const STAR_MESSAGE_MAX_LEN = 400;
    let starMessageText = null;

    const fontMap = {
        A: [0x0e, 0x11, 0x11, 0x1f, 0x11, 0x11, 0x11],
        B: [0x1e, 0x11, 0x11, 0x1e, 0x11, 0x11, 0x1e],
        C: [0x0e, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0e],
        D: [0x1e, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1e],
        E: [0x1f, 0x10, 0x10, 0x1e, 0x10, 0x10, 0x1f],
        F: [0x1f, 0x10, 0x10, 0x1e, 0x10, 0x10, 0x10],
        G: [0x0e, 0x11, 0x10, 0x13, 0x11, 0x11, 0x0f],
        H: [0x11, 0x11, 0x11, 0x1f, 0x11, 0x11, 0x11],
        I: [0x0e, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0e],
        J: [0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0c],
        K: [0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11],
        L: [0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1f],
        M: [0x11, 0x1b, 0x15, 0x11, 0x11, 0x11, 0x11],
        N: [0x11, 0x11, 0x19, 0x15, 0x13, 0x11, 0x11],
        O: [0x0e, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0e],
        P: [0x1e, 0x11, 0x11, 0x1e, 0x10, 0x10, 0x10],
        Q: [0x0e, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0d],
        R: [0x1e, 0x11, 0x11, 0x1e, 0x14, 0x12, 0x11],
        S: [0x0f, 0x10, 0x10, 0x0e, 0x01, 0x01, 0x1e],
        T: [0x1f, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04],
        U: [0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0e],
        V: [0x11, 0x11, 0x11, 0x11, 0x11, 0x0a, 0x04],
        W: [0x11, 0x11, 0x11, 0x15, 0x15, 0x1b, 0x11],
        X: [0x11, 0x11, 0x0a, 0x04, 0x0a, 0x11, 0x11],
        Y: [0x11, 0x11, 0x0a, 0x04, 0x04, 0x04, 0x04],
        Z: [0x1f, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1f],
        a: [0x00, 0x00, 0x0e, 0x01, 0x0f, 0x11, 0x0f],
        b: [0x10, 0x10, 0x16, 0x19, 0x11, 0x11, 0x1e],
        c: [0x00, 0x00, 0x0e, 0x10, 0x10, 0x11, 0x0e],
        d: [0x01, 0x01, 0x0d, 0x13, 0x11, 0x11, 0x0f],
        e: [0x00, 0x00, 0x0e, 0x11, 0x1f, 0x10, 0x0e],
        f: [0x06, 0x09, 0x08, 0x1c, 0x08, 0x08, 0x08],
        g: [0x00, 0x0e, 0x11, 0x11, 0x0f, 0x01, 0x0e],
        h: [0x10, 0x10, 0x16, 0x19, 0x11, 0x11, 0x11],
        i: [0x04, 0x00, 0x0c, 0x04, 0x04, 0x04, 0x0e],
        j: [0x02, 0x00, 0x06, 0x02, 0x02, 0x12, 0x0c],
        k: [0x10, 0x10, 0x12, 0x14, 0x18, 0x14, 0x12],
        l: [0x0c, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0e],
        m: [0x00, 0x00, 0x1a, 0x15, 0x15, 0x11, 0x11],
        n: [0x00, 0x00, 0x16, 0x19, 0x11, 0x11, 0x11],
        o: [0x00, 0x00, 0x0e, 0x11, 0x11, 0x11, 0x0e],
        p: [0x00, 0x16, 0x19, 0x11, 0x1e, 0x10, 0x10],
        q: [0x00, 0x0d, 0x13, 0x11, 0x0f, 0x01, 0x01],
        r: [0x00, 0x00, 0x16, 0x19, 0x10, 0x10, 0x10],
        s: [0x00, 0x00, 0x0f, 0x10, 0x0e, 0x01, 0x1e],
        t: [0x08, 0x08, 0x1c, 0x08, 0x08, 0x09, 0x06],
        u: [0x00, 0x00, 0x11, 0x11, 0x11, 0x13, 0x0d],
        v: [0x00, 0x00, 0x11, 0x11, 0x11, 0x0a, 0x04],
        w: [0x00, 0x00, 0x11, 0x11, 0x15, 0x15, 0x0a],
        x: [0x00, 0x00, 0x11, 0x0a, 0x04, 0x0a, 0x11],
        y: [0x00, 0x11, 0x11, 0x11, 0x0f, 0x01, 0x0e],
        z: [0x00, 0x00, 0x1f, 0x02, 0x04, 0x08, 0x1f],
        0: [0x0e, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0e],
        1: [0x04, 0x0c, 0x04, 0x04, 0x04, 0x04, 0x0e],
        2: [0x0e, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1f],
        3: [0x1f, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0e],
        4: [0x02, 0x06, 0x0a, 0x12, 0x1f, 0x02, 0x02],
        5: [0x1f, 0x10, 0x1e, 0x01, 0x01, 0x11, 0x0e],
        6: [0x06, 0x08, 0x10, 0x1e, 0x11, 0x11, 0x0e],
        7: [0x1f, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08],
        8: [0x0e, 0x11, 0x11, 0x0e, 0x11, 0x11, 0x0e],
        9: [0x0e, 0x11, 0x11, 0x0f, 0x01, 0x02, 0x0c],
        ' ': [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
        '.': [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04],
        '!': [0x04, 0x04, 0x04, 0x04, 0x04, 0x00, 0x04],
        '?': [0x0e, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04],
        '-': [0x00, 0x00, 0x00, 0x1f, 0x00, 0x00, 0x00],
        ':': [0x00, 0x0c, 0x0c, 0x00, 0x0c, 0x0c, 0x00],
        ',': [0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x04],
        "'": [0x06, 0x04, 0x0c, 0x00, 0x00, 0x00, 0x00],
    };

    let canvas = null;
    let ctx = null;
    let cssW = 0;
    let cssH = 0;
    let stars = [];
    let rafId = 0;

    function pickDiameter() {
        const i = Math.floor(Math.random() * STAR_DIAMETERS_CSS.length);
        return STAR_DIAMETERS_CSS[i];
    }

    function currentStarMessageSourceText() {
        return starMessageText === null ? '' : starMessageText;
    }

    function maxCharsPerLine(widthPx) {
        const charW = (CHAR_COLS + CHAR_GAP_CELLS) * CELL;
        return Math.max(4, Math.floor((widthPx - CELL * 4) / charW));
    }

    function wrapToLines(text, maxChars) {
        const words = text.split(/\s+/).filter(Boolean);
        const lines = [];
        let cur = '';
        for (const w of words) {
            const tryLine = cur ? `${cur} ${w}` : w;
            if (tryLine.length <= maxChars) {
                cur = tryLine;
            } else {
                if (cur) lines.push(cur);
                if (w.length > maxChars) {
                    for (let i = 0; i < w.length; i += maxChars) {
                        lines.push(w.slice(i, i + maxChars));
                    }
                    cur = '';
                } else {
                    cur = w;
                }
            }
        }
        if (cur) lines.push(cur);
        return lines.slice(-4);
    }

    function buildStarsFromText(text) {
        const lines = wrapToLines(text, maxCharsPerLine(cssW));
        if (!lines.length) {
            stars = [];
            return;
        }

        const charW = (CHAR_COLS + CHAR_GAP_CELLS) * CELL;
        const lineH = CHAR_ROWS * CELL + LINE_GAP * CELL;
        const totalH = lines.length * lineH - LINE_GAP * CELL;
        let startY = (cssH - totalH) / 2;

        const next = [];
        outer: for (let li = 0; li < lines.length; li++) {
            const line = lines[li];
            const totalWidth = line.length * charW;
            const startX = (cssW - totalWidth) / 2;

            for (let i = 0; i < line.length; i++) {
                const ch = line[i];
                const pattern = fontMap[ch] || fontMap[' '];
                const diameter = pickDiameter();
                const baseAlpha = 0.4 + Math.random() * 0.6;

                for (let row = 0; row < CHAR_ROWS; row++) {
                    for (let col = 0; col < CHAR_COLS; col++) {
                        if ((pattern[row] >> (CHAR_COLS - 1 - col)) & 1) {
                            if (next.length >= MAX_STARS) break outer;
                            const x = startX + i * charW + col * CELL + CELL / 2;
                            const y = startY + row * CELL + CELL / 2;
                            next.push({
                                x,
                                y,
                                diameter,
                                phase: Math.random() * Math.PI * 2,
                                baseAlpha,
                            });
                        }
                    }
                }
            }
            startY += lineH;
        }
        stars = next;
    }

    function resize() {
        if (!canvas || !canvas.parentElement) return;
        const rect = canvas.parentElement.getBoundingClientRect();
        cssW = Math.max(1, rect.width);
        cssH = Math.max(1, rect.height);
        const dpr = Math.min(window.devicePixelRatio || 1, 2.5);
        canvas.width = Math.round(cssW * dpr);
        canvas.height = Math.round(cssH * dpr);
        canvas.style.width = `${cssW}px`;
        canvas.style.height = `${cssH}px`;
        if (ctx) {
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
        buildStarsFromText(currentStarMessageSourceText());
    }

    function drawFrame() {
        if (!ctx) return;
        ctx.clearRect(0, 0, cssW, cssH);

        stars.forEach((s) => {
            s.phase += TWINKLE_SPEED;
            const tw = 0.5 + Math.sin(s.phase) * 0.35;
            const alpha = Math.min(1, s.baseAlpha * tw);
            const r = s.diameter / 2;
            ctx.save();
            ctx.globalAlpha = alpha;
            ctx.fillStyle = 'rgba(255,255,255,0.95)';
            if (s.diameter >= 2.25) {
                ctx.shadowBlur = r * 1.5;
                ctx.shadowColor = 'rgba(200, 230, 255, 0.45)';
            } else {
                ctx.shadowBlur = 0;
            }
            ctx.beginPath();
            ctx.arc(s.x, s.y, r, 0, Math.PI * 2);
            ctx.fill();
            ctx.restore();
        });

        rafId = requestAnimationFrame(drawFrame);
    }

    function rebuildNow() {
        if (ctx) {
            buildStarsFromText(currentStarMessageSourceText());
        }
    }

    /** Set starfield text from assistant embedded JSON (`messageInStars`). Empty string shows no letters. */
    window.ChatMessageStars = {
        setOverride(text) {
            starMessageText = String(text == null ? '' : text).slice(0, STAR_MESSAGE_MAX_LEN);
            rebuildNow();
        },
        clearOverride() {
            starMessageText = null;
            rebuildNow();
        },
    };

    function init() {
        canvas = document.getElementById('chat-message-stars-canvas');
        if (!canvas) return;
        ctx = canvas.getContext('2d', { alpha: true });
        if (!ctx) return;

        resize();

        const wrap = document.querySelector('.chat-box-wrapper');
        if (wrap && typeof ResizeObserver !== 'undefined') {
            const ro = new ResizeObserver(() => resize());
            ro.observe(wrap);
        }
        window.addEventListener('resize', resize);

        cancelAnimationFrame(rafId);
        drawFrame();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
