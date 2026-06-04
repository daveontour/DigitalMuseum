'use strict';

const ChatInactivityNudge = (() => {
    const CONFIG_KEY = 'chat_inactivity_nudge';
    const DEFAULTS = { enabled: false, min_seconds: 120, max_seconds: 300 };
    const MIN_ALLOWED = 30;
    const LOG_PREFIX = '[ChatInactivityNudge]';

    let settings = { ...DEFAULTS };
    let timerId = null;
    let scheduledSeconds = 0;
    let nudgeInFlight = false;
    let hadActiveTimer = false;
    /** @type {Set<string>} */
    const suppressionReasons = new Set();

    function log(...args) {
        console.log(LOG_PREFIX, ...args);
    }

    function clearTimer(reason) {
        if (timerId) {
            clearTimeout(timerId);
            timerId = null;
            if (reason) log('timer cleared:', reason);
        }
    }

    function normalizeSettings(parsed) {
        let minSeconds = parseInt(parsed.min_seconds, 10);
        let maxSeconds = parseInt(parsed.max_seconds, 10);
        if (!Number.isFinite(minSeconds) || minSeconds < MIN_ALLOWED) minSeconds = DEFAULTS.min_seconds;
        if (!Number.isFinite(maxSeconds) || maxSeconds < MIN_ALLOWED) maxSeconds = DEFAULTS.max_seconds;
        if (minSeconds > maxSeconds) {
            const swap = minSeconds;
            minSeconds = maxSeconds;
            maxSeconds = swap;
        }
        return {
            enabled: !!parsed.enabled,
            min_seconds: minSeconds,
            max_seconds: maxSeconds,
        };
    }

    function parseSettingsFromConfigRows(rows) {
        const row = Array.isArray(rows) ? rows.find((r) => r && r.key === CONFIG_KEY) : null;
        if (!row || row.value == null || String(row.value).trim() === '') return { ...DEFAULTS };
        try {
            return normalizeSettings(JSON.parse(row.value));
        } catch (_) {
            return { ...DEFAULTS };
        }
    }

    async function loadSettings() {
        try {
            const res = await fetch('/api/configuration', { credentials: 'same-origin' });
            if (!res.ok) return;
            const rows = await res.json();
            settings = parseSettingsFromConfigRows(rows);
        } catch (e) {
            console.warn('ChatInactivityNudge: failed to load settings', e);
        }
    }

    function isSuppressed() {
        return suppressionReasons.size > 0;
    }

    /** Pause inactivity nudges while another chat mode is active (Have a Chat, Interviewer, etc.). */
    function setSuppressed(reason, suppressed) {
        const key = String(reason || '').trim();
        if (!key) return;
        if (suppressed) {
            if (!suppressionReasons.has(key)) {
                suppressionReasons.add(key);
                log('suppressed', { reason: key, active: [...suppressionReasons] });
            }
            clearTimer(`suppressed:${key}`);
            return;
        }
        if (suppressionReasons.delete(key)) {
            log('suppression cleared', { reason: key, active: [...suppressionReasons] });
        }
        if (!isSuppressed() && settings.enabled && !document.hidden) {
            scheduleTimer(`${key}_ended`);
        }
    }

    function isChatBusy() {
        return typeof App !== 'undefined'
            && App.isChatRequestInFlight
            && App.isChatRequestInFlight();
    }

    function canSchedule() {
        if (!settings.enabled) return false;
        if (isSuppressed()) return false;
        if (document.hidden) return false;
        if (isChatBusy()) return false;
        if (nudgeInFlight) return false;
        return true;
    }

    function pickRandomDelayMs() {
        const min = Math.min(settings.min_seconds, settings.max_seconds);
        const max = Math.max(settings.min_seconds, settings.max_seconds);
        scheduledSeconds = min + Math.floor(Math.random() * (max - min + 1));
        return scheduledSeconds * 1000;
    }

    function scheduleTimer(reason) {
        clearTimer(reason ? `reschedule:${reason}` : undefined);
        if (!settings.enabled) {
            log('timer not scheduled: feature disabled', { reason });
            return;
        }
        if (!canSchedule()) {
            log('timer not scheduled: blocked by current state', {
                reason,
                hidden: document.hidden,
                suppressed: isSuppressed(),
                suppressionReasons: [...suppressionReasons],
                chatBusy: isChatBusy(),
                nudgeInFlight,
            });
            return;
        }
        hadActiveTimer = true;
        const delayMs = pickRandomDelayMs();
        const action = reason === 'app_load' ? 'timer started' : 'timer restarted';
        log(action, {
            reason: reason || 'unknown',
            timeoutSeconds: scheduledSeconds,
            timeoutMs: delayMs,
            minSeconds: settings.min_seconds,
            maxSeconds: settings.max_seconds,
        });
        timerId = setTimeout(() => {
            timerId = null;
            log('timer expired', { timeoutSeconds: scheduledSeconds });
            void onTimerExpired();
        }, delayMs);
    }

    async function onTimerExpired() {
        if (!canSchedule()) {
            if (settings.enabled && !document.hidden && !isSuppressed()) {
                scheduleTimer('timer_expired_retry');
            }
            return;
        }
        await fireNudge();
    }

    function buildNudgePayload(provider, selectedVoice, selectedMood, conversationId, whosAsking) {
        return {
            prompt: '.',
            inactivity_nudge: true,
            inactivity_seconds: scheduledSeconds,
            voice: selectedVoice,
            mood: selectedMood,
            companionMode: DOM.companionModeCheckbox ? DOM.companionModeCheckbox.checked : false,
            allowExplicitContent: DOM.allowExplicitContentCheckbox ? DOM.allowExplicitContentCheckbox.checked : false,
            enableSnarkiness: DOM.enableSnarkinessCheckbox ? DOM.enableSnarkinessCheckbox.checked : false,
            temperature: parseFloat(DOM.creativityLevel ? DOM.creativityLevel.value : '0'),
            conversation_id: conversationId,
            provider,
            whos_asking: whosAsking,
        };
    }

    async function fireNudge() {
        if (nudgeInFlight || !settings.enabled) return;
        if (typeof App === 'undefined' || !App.runChatWithProviderFailover) {
            scheduleTimer('nudge_app_unavailable');
            return;
        }
        nudgeInFlight = true;
        try {
            const selectedVoice = VoiceSelector.getSelectedVoice();
            const selectedMood = (selectedVoice === 'owner' && DOM.ownerMood) ? DOM.ownerMood.value : null;
            const conversationId = Modals.ConversationManager
                ? Modals.ConversationManager.getCurrentConversationId()
                : null;
            const itsMeVisitorSwitch = document.getElementById('its-me-visitor-switch');
            const whosAsking = itsMeVisitorSwitch
                ?.querySelector('.its-me-visitor-option.active')
                ?.dataset?.value || 'visitor';

            log('sending LLM inactivity nudge', {
                inactivity_seconds: scheduledSeconds,
                internalPrompt: '[User has been inactive — respond naturally.]',
                clientPrompt: '.',
                voice: selectedVoice,
                mood: selectedMood,
                conversation_id: conversationId,
                whos_asking: whosAsking,
            });

            const result = await App.runChatWithProviderFailover(
                CONSTANTS.API_PATHS.CHAT,
                (provider) => buildNudgePayload(provider, selectedVoice, selectedMood, conversationId, whosAsking),
                null,
            );

            if (result.ok) {
                const data = result.data;
                log('LLM inactivity nudge response', {
                    responseLength: data.response ? data.response.length : 0,
                    responsePreview: data.response ? data.response.slice(0, 200) : '',
                    providerSwitched: !!result.switched,
                });
                Chat.addMessage('assistant', data.response, true, null, data.embedded_json);
                if (typeof UI !== 'undefined' && UI.setChatLastRequestStatsFromEmbedded) {
                    UI.setChatLastRequestStatsFromEmbedded(data.embedded_json);
                }
                if (typeof UI !== 'undefined' && UI.scrollToBottom) UI.scrollToBottom();
            } else if (!result.aborted) {
                console.warn(LOG_PREFIX, 'nudge failed', result.error);
            }
        } catch (e) {
            console.warn(LOG_PREFIX, 'nudge error', e);
        } finally {
            nudgeInFlight = false;
            scheduleTimer('nudge_complete');
        }
    }

    function onUserMessageSent() {
        if (!settings.enabled) return;
        scheduleTimer('user_message');
    }

    async function reloadSettings() {
        await loadSettings();
        clearTimer('settings_reload');
        hadActiveTimer = false;
        if (settings.enabled) scheduleTimer('settings_reload');
    }

    function onVisibilityChange() {
        if (document.hidden) {
            if (timerId) {
                hadActiveTimer = true;
                clearTimer('tab_hidden');
            }
            return;
        }
        if (settings.enabled && hadActiveTimer && !isSuppressed()) {
            hadActiveTimer = false;
            scheduleTimer('visibility_resume');
        }
    }

    async function init() {
        await loadSettings();
        document.addEventListener('visibilitychange', onVisibilityChange);
        log('initialized', { enabled: settings.enabled, settings });
        if (settings.enabled) scheduleTimer('app_load');
    }

    return { init, onUserMessageSent, reloadSettings, clearTimer, setSuppressed };
})();
