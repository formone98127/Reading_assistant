let sessionId = null;
let busy = false;
let sourceFilename = "paste";
let libraryBooks = [];
let selectedBookId = null;
let currentBookId = null;
let rewritePollTimer = null;
let showEasier = true;
let showChinese = false;
let pendingReadAction = null;
let lastReaderState = null;
let simplifySliderValue = 0;
let lastSlideIndex = -1;

let generateVoice = true;
let voiceOnly = false;
let ttsControl = "";
let ttsCfg = 2.0;
let playVoiceInReader = true;
let readerAudio = null;
let lastPlayedAudioIndex = -1;
let autoplay = false;

/* —— RSVP speed reader —— */
let rsvpActive = false;
let rsvpPlaying = false;
let rsvpTimer = null;
let rsvpChunks = [];
let rsvpChunkIndex = 0;
let rsvpWordsDone = 0;
let rsvpTotalWords = 0;
let rsvpStartedAt = 0;
let rsvpElapsedMs = 0;
let rsvpLastTick = 0;
let rsvpSyncedSentence = -1;
let rsvpGotoTimer = null;
let rsvpCompleteTimer = null;
let rsvpWpm = 300;
let rsvpChunkSize = 1;
let rsvpPush = false;
let rsvpStartWpm = 300;
let rsvpTargetWpm = 500;
let rsvpAutoContinue = false;
const RSVP_MAX_CHUNK_CHARS = 20;

function loadRsvpPrefs() {
  try {
    const w = parseInt(localStorage.getItem("rsvpWpm"), 10);
    if (!Number.isNaN(w)) rsvpWpm = Math.min(1000, Math.max(100, w));
    const c = parseInt(localStorage.getItem("rsvpChunk"), 10);
    if (c === 1 || c === 2 || c === 3) rsvpChunkSize = c;
    const p = localStorage.getItem("rsvpPush");
    if (p !== null) rsvpPush = p === "1";
    const s = parseInt(localStorage.getItem("rsvpStart"), 10);
    if (!Number.isNaN(s)) rsvpStartWpm = Math.min(1000, Math.max(100, s));
    const t = parseInt(localStorage.getItem("rsvpTarget"), 10);
    if (!Number.isNaN(t)) rsvpTargetWpm = Math.min(1000, Math.max(100, t));
    const a = localStorage.getItem("rsvpAutoContinue");
    if (a !== null) rsvpAutoContinue = a === "1";
  } catch (_) {}
}

function saveRsvpPrefs() {
  try {
    localStorage.setItem("rsvpWpm", String(rsvpWpm));
    localStorage.setItem("rsvpChunk", String(rsvpChunkSize));
    localStorage.setItem("rsvpPush", rsvpPush ? "1" : "0");
    localStorage.setItem("rsvpStart", String(rsvpStartWpm));
    localStorage.setItem("rsvpTarget", String(rsvpTargetWpm));
    localStorage.setItem("rsvpAutoContinue", rsvpAutoContinue ? "1" : "0");
  } catch (_) {}
}

function syncRsvpControlsUI() {
  const wpm = $("rsvp-wpm");
  if (wpm) wpm.value = String(rsvpWpm);
  if ($("rsvp-wpm-val")) $("rsvp-wpm-val").textContent = String(rsvpWpm);
  const push = $("rsvp-push");
  if (push) push.checked = rsvpPush;
  $("rsvp-push-fields")?.classList.toggle("hidden", !rsvpPush);
  const sw = $("rsvp-start-wpm");
  if (sw) sw.value = String(rsvpStartWpm);
  if ($("rsvp-start-val")) $("rsvp-start-val").textContent = String(rsvpStartWpm);
  const tw = $("rsvp-target-wpm");
  if (tw) tw.value = String(rsvpTargetWpm);
  if ($("rsvp-target-val")) $("rsvp-target-val").textContent = String(rsvpTargetWpm);
  document.querySelectorAll('input[name="rsvp-chunk"]').forEach((el) => {
    el.checked = String(rsvpChunkSize) === el.value;
  });
  const ac = $("rsvp-auto-continue");
  if (ac) ac.checked = rsvpAutoContinue;
}

function rsvpOverlayOpen() {
  return rsvpActive && !$("rsvp-overlay")?.classList.contains("hidden");
}

function tokenizeRsvpWords(text) {
  const t = String(text || "").trim();
  if (!t) return [];
  if (/\s/.test(t)) return t.split(/\s+/).filter(Boolean);
  // CJK / no-space: one char per token
  return Array.from(t.replace(/\s/g, ""));
}

function buildRsvpChunks(sentences, startSentence, chunkSize) {
  const chunks = [];
  let buf = [];
  let bufChars = 0;
  let bufSentence = startSentence;

  function flush() {
    if (!buf.length) return;
    chunks.push({ words: buf.slice(), sentenceIndex: bufSentence });
    buf = [];
    bufChars = 0;
  }

  for (let si = startSentence; si < sentences.length; si++) {
    const words = tokenizeRsvpWords(sentences[si]);
    for (const w of words) {
      const len = w.length;
      const wouldOverflow =
        buf.length > 0 &&
        (buf.length >= chunkSize ||
          (chunkSize > 1 && bufChars + len + 1 > RSVP_MAX_CHUNK_CHARS) ||
          len > RSVP_MAX_CHUNK_CHARS);
      if (wouldOverflow) flush();
      if (buf.length === 0) bufSentence = si;
      if (len > RSVP_MAX_CHUNK_CHARS && chunkSize > 1) {
        flush();
        chunks.push({ words: [w], sentenceIndex: si });
        continue;
      }
      buf.push(w);
      bufChars += (buf.length > 1 ? 1 : 0) + len;
      if (buf.length >= chunkSize) flush();
    }
  }
  flush();
  return chunks;
}

function orpIndex(word) {
  const n = word.length;
  if (n <= 1) return 0;
  if (n <= 5) return 1;
  if (n <= 9) return 2;
  if (n <= 13) return 3;
  return Math.floor(n * 0.35);
}

function renderRsvpChunkHtml(words) {
  if (!words.length) return "";
  const focusIdx = Math.floor((words.length - 1) / 2);
  const parts = words.map((w, idx) => {
    if (idx !== focusIdx) {
      return `<span class="rsvp-word">${escapeHtml(w)}</span>`;
    }
    const i = orpIndex(w);
    const before = escapeHtml(w.slice(0, i));
    const mid = escapeHtml(w.slice(i, i + 1));
    const after = escapeHtml(w.slice(i + 1));
    return `<span class="rsvp-word">${before}<span class="rsvp-orp">${mid}</span>${after}</span>`;
  });
  return parts.join(" ");
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function formatRsvpTime(ms) {
  const sec = Math.floor(ms / 1000);
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function currentRsvpWpm() {
  if (!rsvpPush || rsvpChunks.length === 0) return rsvpWpm;
  const t = Math.min(1, Math.max(0, rsvpChunkIndex / Math.max(1, rsvpChunks.length - 1)));
  const start = Math.min(rsvpStartWpm, rsvpTargetWpm);
  const target = Math.max(rsvpStartWpm, rsvpTargetWpm);
  return Math.round(start + (target - start) * t);
}

function updateRsvpStats() {
  const wpm = currentRsvpWpm();
  if ($("rsvp-wpm-label")) $("rsvp-wpm-label").textContent = `${wpm} WPM`;
  let elapsed = rsvpElapsedMs;
  if (rsvpPlaying && rsvpLastTick) elapsed += Date.now() - rsvpLastTick;
  if ($("rsvp-time")) $("rsvp-time").textContent = formatRsvpTime(elapsed);
  if ($("rsvp-progress-words")) {
    $("rsvp-progress-words").textContent = `${rsvpWordsDone} / ${rsvpTotalWords}`;
  }
  const pct = rsvpTotalWords ? Math.min(100, Math.round((rsvpWordsDone / rsvpTotalWords) * 100)) : 0;
  if ($("rsvp-progress-pct")) $("rsvp-progress-pct").textContent = `${pct}%`;
  const left = Math.max(0, rsvpTotalWords - rsvpWordsDone);
  let eta = "-- min left";
  if (wpm > 0 && left > 0) {
    const mins = left / wpm;
    eta = mins < 1 ? "<1 min left" : `${Math.ceil(mins)} min left`;
  } else if (left === 0 && rsvpTotalWords > 0) {
    eta = "done";
  }
  if ($("rsvp-eta")) $("rsvp-eta").textContent = eta;
}

function showRsvpChunk() {
  const el = $("rsvp-chunk");
  if (!el) return;
  if (rsvpChunkIndex < 0 || rsvpChunkIndex >= rsvpChunks.length) {
    el.innerHTML = "";
    return;
  }
  const chunk = rsvpChunks[rsvpChunkIndex];
  el.innerHTML = renderRsvpChunkHtml(chunk.words);
  rsvpWordsDone = 0;
  for (let i = 0; i < rsvpChunkIndex; i++) rsvpWordsDone += rsvpChunks[i].words.length;
  rsvpWordsDone += chunk.words.length;
  updateRsvpStats();
  syncRsvpSentence(chunk.sentenceIndex);
}

function syncRsvpSentence(sentenceIndex) {
  if (sentenceIndex === rsvpSyncedSentence) return;
  rsvpSyncedSentence = sentenceIndex;
  if (rsvpGotoTimer) clearTimeout(rsvpGotoTimer);
  rsvpGotoTimer = setTimeout(() => {
    rsvpGotoTimer = null;
    if (!sessionId || sentenceIndex < 0) return;
    const page = sentenceIndex + 1;
    api("/api/goto", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ page }),
    })
      .then((data) => {
        if (data?.state) {
          lastReaderState = data.state;
          syncPageInput(data.state);
        }
      })
      .catch(() => {});
  }, 400);
}

function clearRsvpTimer() {
  if (rsvpTimer) {
    clearTimeout(rsvpTimer);
    rsvpTimer = null;
  }
}

function clearRsvpCompleteTimer() {
  if (rsvpCompleteTimer) {
    clearInterval(rsvpCompleteTimer);
    rsvpCompleteTimer = null;
  }
}

function scheduleRsvpTick() {
  clearRsvpTimer();
  if (!rsvpPlaying || !rsvpActive) return;
  const chunk = rsvpChunks[rsvpChunkIndex];
  if (!chunk) {
    finishRsvp();
    return;
  }
  const wpm = Math.max(50, currentRsvpWpm());
  const ms = (60000 / wpm) * chunk.words.length;
  rsvpTimer = setTimeout(() => {
    rsvpTimer = null;
    rsvpChunkIndex += 1;
    if (rsvpChunkIndex >= rsvpChunks.length) {
      finishRsvp();
      return;
    }
    showRsvpChunk();
    scheduleRsvpTick();
  }, ms);
}

function setRsvpPlaying(on) {
  if (on === rsvpPlaying) return;
  if (on) {
    rsvpPlaying = true;
    rsvpLastTick = Date.now();
    if (!rsvpStartedAt) rsvpStartedAt = Date.now();
    $("rsvp-paused")?.classList.add("hidden");
    $("rsvp-complete")?.classList.add("hidden");
    clearRsvpCompleteTimer();
    showRsvpChunk();
    scheduleRsvpTick();
  } else {
    if (rsvpPlaying && rsvpLastTick) {
      rsvpElapsedMs += Date.now() - rsvpLastTick;
      rsvpLastTick = 0;
    }
    rsvpPlaying = false;
    clearRsvpTimer();
    $("rsvp-paused")?.classList.remove("hidden");
    updateRsvpStats();
  }
}

function finishRsvp() {
  setRsvpPlaying(false);
  rsvpChunkIndex = Math.max(0, rsvpChunks.length - 1);
  if (rsvpChunks.length) {
    rsvpWordsDone = rsvpTotalWords;
    syncRsvpSentence(rsvpChunks[rsvpChunks.length - 1].sentenceIndex);
  }
  updateRsvpStats();
  const complete = $("rsvp-complete");
  complete?.classList.remove("hidden");
  $("rsvp-paused")?.classList.add("hidden");
  const sub = $("rsvp-complete-sub");
  if (!rsvpAutoContinue) {
    if (sub) sub.textContent = "";
    return;
  }
  let left = 3;
  if (sub) sub.textContent = `Restarting in ${left}s…`;
  clearRsvpCompleteTimer();
  rsvpCompleteTimer = setInterval(() => {
    left -= 1;
    if (left <= 0) {
      clearRsvpCompleteTimer();
      restartRsvp();
      return;
    }
    if (sub) sub.textContent = `Restarting in ${left}s…`;
  }, 1000);
}

function restartRsvp() {
  clearRsvpCompleteTimer();
  $("rsvp-complete")?.classList.add("hidden");
  rsvpChunkIndex = 0;
  rsvpWordsDone = 0;
  rsvpElapsedMs = 0;
  rsvpStartedAt = 0;
  rsvpLastTick = 0;
  rsvpSyncedSentence = -1;
  showRsvpChunk();
  setRsvpPlaying(true);
}

async function enterRsvp() {
  if (!sessionId || rsvpActive) return;
  stopReaderVoice();
  loadRsvpPrefs();
  syncRsvpControlsUI();
  showError($("reader-error"), "");
  try {
    const data = await api("/api/rsvp-text");
    const sentences = Array.isArray(data.sentences) ? data.sentences : [];
    if (!sentences.length) {
      showError($("reader-error"), "No text to speed-read.");
      return;
    }
    const start = Math.max(0, Math.min(data.index || 0, sentences.length - 1));
    rsvpChunks = buildRsvpChunks(sentences, start, rsvpChunkSize);
    rsvpTotalWords = rsvpChunks.reduce((n, c) => n + c.words.length, 0);
    if (!rsvpChunks.length) {
      showError($("reader-error"), "No words to speed-read.");
      return;
    }
    rsvpActive = true;
    rsvpChunkIndex = 0;
    rsvpWordsDone = 0;
    rsvpElapsedMs = 0;
    rsvpStartedAt = 0;
    rsvpLastTick = 0;
    rsvpSyncedSentence = -1;
    $("rsvp-overlay")?.classList.remove("hidden");
    $("rsvp-complete")?.classList.add("hidden");
    $("rsvp-overlay")?.focus();
    showRsvpChunk();
    setRsvpPlaying(true);
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  }
}

async function exitRsvp(refreshState) {
  if (!rsvpActive) return;
  setRsvpPlaying(false);
  clearRsvpTimer();
  clearRsvpCompleteTimer();
  if (rsvpGotoTimer) {
    clearTimeout(rsvpGotoTimer);
    rsvpGotoTimer = null;
  }
  rsvpActive = false;
  $("rsvp-overlay")?.classList.add("hidden");
  $("rsvp-complete")?.classList.add("hidden");
  saveRsvpPrefs();
  if (refreshState !== false && sessionId) {
    try {
      const data = await api("/api/state");
      if (data?.state) renderState(data.state);
    } catch (_) {}
  }
}

function rebuildRsvpChunksFromPrefs() {
  if (!rsvpActive || !rsvpChunks.length) return;
  // Re-fetch would be cleaner; rebuild from current chunk's sentence by re-entering.
  // Keep position by sentence index of current chunk.
  const si = rsvpChunks[Math.min(rsvpChunkIndex, rsvpChunks.length - 1)]?.sentenceIndex ?? 0;
  const wasPlaying = rsvpPlaying;
  setRsvpPlaying(false);
  api("/api/rsvp-text")
    .then((data) => {
      const sentences = Array.isArray(data.sentences) ? data.sentences : [];
      rsvpChunks = buildRsvpChunks(sentences, si, rsvpChunkSize);
      rsvpTotalWords = rsvpChunks.reduce((n, c) => n + c.words.length, 0);
      rsvpChunkIndex = 0;
      rsvpSyncedSentence = -1;
      showRsvpChunk();
      if (wasPlaying) setRsvpPlaying(true);
    })
    .catch(() => {});
}

function nudgeRsvpWpm(delta) {
  if (rsvpPush) {
    rsvpStartWpm = Math.min(1000, Math.max(100, rsvpStartWpm + delta));
    rsvpTargetWpm = Math.min(1000, Math.max(100, rsvpTargetWpm + delta));
    if (rsvpTargetWpm < rsvpStartWpm) rsvpTargetWpm = rsvpStartWpm;
  } else {
    rsvpWpm = Math.min(1000, Math.max(100, rsvpWpm + delta));
  }
  syncRsvpControlsUI();
  saveRsvpPrefs();
  updateRsvpStats();
  if (rsvpPlaying) {
    clearRsvpTimer();
    scheduleRsvpTick();
  }
}

function onRsvpKey(e) {
  if (!rsvpOverlayOpen()) return false;
  if (e.target.matches("textarea, input:not([type='hidden']):not([type='range']):not([type='checkbox']):not([type='radio'])")) {
    return false;
  }
  if (e.key === "Escape") {
    e.preventDefault();
    e.stopPropagation();
    exitRsvp(true);
    return true;
  }
  if (e.key === "r" || e.key === "R") {
    e.preventDefault();
    e.stopPropagation();
    exitRsvp(true);
    return true;
  }
  if (e.key === " " || e.code === "Space") {
    e.preventDefault();
    e.stopPropagation();
    if (!$("rsvp-complete")?.classList.contains("hidden")) return true;
    setRsvpPlaying(!rsvpPlaying);
    return true;
  }
  if (e.key === "ArrowLeft") {
    e.preventDefault();
    e.stopPropagation();
    nudgeRsvpWpm(-25);
    return true;
  }
  if (e.key === "ArrowRight") {
    e.preventDefault();
    e.stopPropagation();
    nudgeRsvpWpm(25);
    return true;
  }
  return false;
}

function bindRsvpUI() {
  loadRsvpPrefs();
  syncRsvpControlsUI();
  bindClick("btn-rsvp", () => enterRsvp());
  bindClick("btn-rsvp-restart", () => restartRsvp());
  bindClick("btn-rsvp-exit", () => exitRsvp(true));
  $("rsvp-wpm")?.addEventListener("input", (e) => {
    rsvpWpm = parseInt(e.target.value, 10) || 300;
    if ($("rsvp-wpm-val")) $("rsvp-wpm-val").textContent = String(rsvpWpm);
    saveRsvpPrefs();
    updateRsvpStats();
    if (rsvpPlaying) {
      clearRsvpTimer();
      scheduleRsvpTick();
    }
  });
  $("rsvp-push")?.addEventListener("change", (e) => {
    rsvpPush = !!e.target.checked;
    $("rsvp-push-fields")?.classList.toggle("hidden", !rsvpPush);
    saveRsvpPrefs();
    updateRsvpStats();
    if (rsvpPlaying) {
      clearRsvpTimer();
      scheduleRsvpTick();
    }
  });
  $("rsvp-start-wpm")?.addEventListener("input", (e) => {
    rsvpStartWpm = parseInt(e.target.value, 10) || 300;
    if ($("rsvp-start-val")) $("rsvp-start-val").textContent = String(rsvpStartWpm);
    saveRsvpPrefs();
    updateRsvpStats();
  });
  $("rsvp-target-wpm")?.addEventListener("input", (e) => {
    rsvpTargetWpm = parseInt(e.target.value, 10) || 500;
    if ($("rsvp-target-val")) $("rsvp-target-val").textContent = String(rsvpTargetWpm);
    saveRsvpPrefs();
    updateRsvpStats();
  });
  $("rsvp-auto-continue")?.addEventListener("change", (e) => {
    rsvpAutoContinue = !!e.target.checked;
    saveRsvpPrefs();
  });
  document.querySelectorAll('input[name="rsvp-chunk"]').forEach((el) => {
    el.addEventListener("change", () => {
      if (!el.checked) return;
      rsvpChunkSize = parseInt(el.value, 10) || 1;
      saveRsvpPrefs();
      if (rsvpActive) rebuildRsvpChunksFromPrefs();
    });
  });
}

function loadTtsPrefs() {
  try {
    const v = localStorage.getItem("generateVoice");
    if (v !== null) generateVoice = v === "1";
    const vo = localStorage.getItem("voiceOnly");
    if (vo !== null) voiceOnly = vo === "1";
    const c = localStorage.getItem("ttsControl");
    if (c) ttsControl = c;
    const cfg = parseFloat(localStorage.getItem("ttsCfg"), 10);
    if (!Number.isNaN(cfg)) ttsCfg = cfg;
  } catch (_) {}
  syncVoiceOnlyUI();
}

function saveTtsPrefs() {
  try {
    localStorage.setItem("generateVoice", generateVoice ? "1" : "0");
    localStorage.setItem("voiceOnly", voiceOnly ? "1" : "0");
    localStorage.setItem("ttsControl", ttsControl);
    localStorage.setItem("ttsCfg", String(ttsCfg));
  } catch (_) {}
}

function loadVoiceReaderPrefs() {
  try {
    const v = localStorage.getItem("playVoiceInReader");
    if (v !== null) playVoiceInReader = v === "1";
  } catch (_) {}
  const cb = $("opt-voice-reader");
  if (cb) cb.checked = playVoiceInReader;
}

function saveVoiceReaderPrefs() {
  try {
    localStorage.setItem("playVoiceInReader", playVoiceInReader ? "1" : "0");
  } catch (_) {}
}

function loadAutoplayPref() {
  try {
    const v = localStorage.getItem("autoplay");
    if (v !== null) autoplay = v === "1";
  } catch (_) {}
  const cb = $("opt-autoplay");
  if (cb) cb.checked = autoplay;
}

function saveAutoplayPref() {
  try {
    localStorage.setItem("autoplay", autoplay ? "1" : "0");
  } catch (_) {}
}

function stopReaderVoice() {
  if (readerAudio) {
    readerAudio.pause();
    readerAudio = null;
  }
}

function playReaderVoice(bookId, index) {
  if (rsvpActive) return;
  if (!playVoiceInReader || !bookId || index < 0) return;
  stopReaderVoice();
  lastPlayedAudioIndex = index;
  const url = `/api/library/audio?id=${encodeURIComponent(bookId)}&index=${index}`;
  readerAudio = new Audio(url);
  if (autoplay) {
    readerAudio.addEventListener("ended", () => {
      readerAudio = null;
      if (autoplay) nextSentence();
    });
  }
  readerAudio.play().catch(() => {});
}

function syncVoiceOnlyUI() {
  const vo = $("opt-voice-only");
  const voice = $("opt-voice");
  const easier = $("opt-easier");
  const chinese = $("opt-chinese");
  const hint = $("file-upload-hint");
  const btn = $("btn-load-file");
  if (vo) vo.checked = voiceOnly;
  if (voiceOnly) {
    generateVoice = true;
    showEasier = false;
    showChinese = false;
    if (easier) {
      easier.checked = false;
      easier.disabled = true;
    }
    if (chinese) {
      chinese.checked = false;
      chinese.disabled = true;
    }
    if (voice) {
      voice.checked = true;
      voice.disabled = true;
    }
    if (hint) hint.textContent = "Original text + VoxCPM voice only — no LLM rewrite.";
    if (btn) btn.textContent = "Add to library & generate voice";
  } else {
    if (easier) easier.disabled = false;
    if (chinese) chinese.disabled = false;
    if (voice) voice.disabled = false;
    if (hint) hint.textContent = "Creates book.json (original + easier 1–3 + 中文 + optional voice).";
    if (btn) btn.textContent = "Add to library & rewrite";
  }
  syncTtsUI();
}

function syncTtsUI() {
  const voice = $("opt-voice");
  const ctrl = $("tts-control");
  const cfg = $("tts-cfg");
  const cfgVal = $("tts-cfg-val");
  if (voice) voice.checked = generateVoice;
  if (ctrl && document.activeElement !== ctrl) ctrl.value = ttsControl;
  if (cfg) cfg.value = String(ttsCfg);
  if (cfgVal) cfgVal.textContent = ttsCfg.toFixed(1);
}

function setTtsStatus(msg, kind) {
  const el = $("tts-status");
  if (!el) return;
  el.textContent = msg || "";
  el.classList.toggle("is-error", kind === "error");
  el.classList.toggle("is-ok", kind === "ok");
}

function updateTtsStatus(data) {
  if (!generateVoice) {
    setTtsStatus("");
    return false;
  }
  if (!data) {
    setTtsStatus("VoxCPM offline — run scripts\\start-voxcpm.ps1", "error");
    return true;
  }
  if (data.ok) {
    setTtsStatus(`✓ VoxCPM ready · ${data.url || "127.0.0.1:8808"}`, "ok");
    return false;
  }
  if (data.reachable && data.loading) {
    setTtsStatus("VoxCPM loading model (first run may take a few minutes)…");
    return true;
  }
  if (data.reachable) {
    setTtsStatus("VoxCPM starting…");
    return true;
  }
  setTtsStatus("VoxCPM offline — run scripts\\start-voxcpm.ps1", "error");
  return true;
}

async function loadTtsServerSettings() {
  try {
    const res = await fetch("/api/tts");
    if (!res.ok) return null;
    const data = await res.json();
    if (!ttsControl && data.control) ttsControl = data.control;
    if (data.cfgValue > 0 && !localStorage.getItem("ttsCfg")) ttsCfg = data.cfgValue;
    syncTtsUI();
    return data;
  } catch (_) {
    return null;
  }
}

async function saveTtsServerSettings() {
  await fetch("/api/tts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ control: ttsControl, cfgValue: ttsCfg }),
  }).catch(() => {});
}

function syncPageInput(state) {
  const pageInput = $("page-input");
  const pageTotal = $("page-total");
  if (pageTotal) pageTotal.textContent = state.total;
  if (pageInput && document.activeElement !== pageInput) {
    pageInput.min = 1;
    pageInput.max = state.total;
    pageInput.value = state.index + 1;
  }
}

const FONT_MIN = 16;
const FONT_MAX = 180;
const FONT_DEFAULT = 22;
let readerFontSize = FONT_DEFAULT;

const $ = (id) => document.getElementById(id);

function bindClick(id, handler) {
  const el = $(id);
  if (el) el.addEventListener("click", handler);
}

function bookRewritePending(state) {
  const t = state?.bookRewriteTotal || 0;
  if (t <= 0) return false;
  const aud = state.bookAudioDone ?? 0;
  const tts = state.bookTTSEnabled;
  if (state.bookVoiceOnly) {
    return !!state.bookRewriteActive || (tts && aud < t);
  }
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  if (eng < t || zh < t) return !!state.bookRewriteActive || eng < t || zh < t;
  return !!state.bookRewriteActive || (tts && aud < t);
}

function bookRewriteIndeterminate(state) {
  if (!state?.bookTTSEnabled) return false;
  if (state.bookAudioGenerating) return true;
  const t = state?.bookRewriteTotal || 0;
  if (!t || !state.bookRewriteActive) return false;
  const aud = state.bookAudioDone ?? 0;
  if (aud > 0) return false;
  if (state.bookVoiceOnly) return true;
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  return eng >= t && zh >= t;
}

function bookRewritePercent(state) {
  const t = state?.bookRewriteTotal || 0;
  if (!t) return 0;
  const aud = state.bookAudioDone ?? 0;
  const tts = state.bookTTSEnabled;
  if (state.bookVoiceOnly) {
    return tts ? (100 * aud) / t : 0;
  }
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  if (eng < t) return (100 * eng) / t;
  if (zh < t) return (100 * zh) / t;
  if (tts && aud < t) return (100 * aud) / t;
  return 100;
}

function bookRewriteMessage(state) {
  const t = state?.bookRewriteTotal || 0;
  if (!t) return "";
  if (state.bookAudioGenerating) return "Generating voice…";
  const aud = state.bookAudioDone ?? 0;
  const tts = state.bookTTSEnabled;
  if (state.bookVoiceOnly) {
    if (tts && aud < t) return `voice ${aud}/${t} — you can read now`;
    return "";
  }
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  if (eng < t) return `english.json ${eng}/${t} — you can read now`;
  if (zh < t) return `chinese.json ${zh}/${t} — you can read now`;
  if (tts && aud < t) return `voice ${aud}/${t} — you can read now`;
  return "";
}

function libraryStatusLabel(b) {
  const t = b.totalSentences || 0;
  if (b.voiceOnly) {
    if (b.ttsEnabled && t > 0 && (b.audioRewriteDone ?? 0) < t) {
      return `voice ${b.audioRewriteDone ?? 0}/${t}`;
    }
    if (b.rewriteStatus === "done") return "ready";
    if (b.rewriteStatus === "pending") return "queued";
    if (b.rewriteStatus === "error") return "rewrite paused";
    return b.rewriteStatus || "unknown";
  }
  if (
    b.rewriteStatus === "rewriting" &&
    t > 0 &&
    b.rewriteDone >= t &&
    b.chineseRewriteDone >= t &&
    b.ttsEnabled &&
    (b.audioRewriteDone ?? 0) < t
  ) {
    return `voice ${b.audioRewriteDone ?? 0}/${t}`;
  }
  if (
    b.rewriteStatus === "rewriting" &&
    b.rewriteDone >= b.totalSentences &&
    b.chineseRewriteDone < b.totalSentences
  ) {
    return `chinese ${b.chineseRewriteDone}/${b.totalSentences}`;
  }
  switch (b.rewriteStatus) {
    case "rewriting":
      return `english ${b.rewriteDone}/${b.totalSentences}`;
    case "done":
      if (b.chineseRewriteDone < b.totalSentences) {
        return `chinese ${b.chineseRewriteDone}/${b.totalSentences}`;
      }
      if (b.ttsEnabled && t > 0 && (b.audioRewriteDone ?? 0) < t) {
        return `voice ${b.audioRewriteDone ?? 0}/${t}`;
      }
      return "ready";
    case "pending":
      return "queued";
    case "error":
      return "rewrite paused";
    default:
      return b.rewriteStatus || "unknown";
  }
}

function applyFontSize(px) {
  readerFontSize = Math.min(FONT_MAX, Math.max(FONT_MIN, px));
  document.documentElement.style.setProperty("--reader-font-size", `${readerFontSize}px`);
  document.documentElement.style.setProperty(
    "--compare-font-size",
    `${Math.max(FONT_MIN, readerFontSize - 4)}px`
  );
  const slider = $("font-slider");
  const label = $("font-size-value");
  if (slider) slider.value = String(readerFontSize);
  if (label) label.textContent = `${readerFontSize}px`;
  try {
    localStorage.setItem("readerFontSize", String(readerFontSize));
  } catch (_) {}
}

function initFontSize() {
  try {
    const saved = parseInt(localStorage.getItem("readerFontSize"), 10);
    if (!Number.isNaN(saved)) applyFontSize(saved);
    else applyFontSize(FONT_DEFAULT);
  } catch (_) {
    applyFontSize(FONT_DEFAULT);
  }
  $("font-smaller")?.addEventListener("click", () => applyFontSize(readerFontSize - 2));
  $("font-larger")?.addEventListener("click", () => applyFontSize(readerFontSize + 2));
  $("font-slider")?.addEventListener("input", (e) => applyFontSize(parseInt(e.target.value, 10)));
}

initFontSize();
loadTtsPrefs();
loadVoiceReaderPrefs();
loadAutoplayPref();

$("opt-voice-only")?.addEventListener("change", (e) => {
  voiceOnly = e.target.checked;
  saveTtsPrefs();
  syncVoiceOnlyUI();
});

$("opt-voice")?.addEventListener("change", (e) => {
  generateVoice = e.target.checked;
  saveTtsPrefs();
  loadTtsServerSettings().then((d) => updateTtsStatus(d));
});

$("tts-control")?.addEventListener("input", (e) => {
  ttsControl = e.target.value;
  saveTtsPrefs();
});

$("tts-cfg")?.addEventListener("input", (e) => {
  ttsCfg = parseFloat(e.target.value, 10);
  const cfgVal = $("tts-cfg-val");
  if (cfgVal) cfgVal.textContent = ttsCfg.toFixed(1);
  saveTtsPrefs();
});

$("opt-voice-reader")?.addEventListener("change", (e) => {
  playVoiceInReader = e.target.checked;
  saveVoiceReaderPrefs();
  if (!playVoiceInReader) stopReaderVoice();
  else if (lastReaderState?.hasAudio && currentBookId)
    playReaderVoice(currentBookId, lastReaderState.index);
});

$("opt-autoplay")?.addEventListener("change", (e) => {
  autoplay = e.target.checked;
  saveAutoplayPref();
});

function showError(el, msg) {
  if (!msg) {
    el.classList.add("hidden");
    el.textContent = "";
    return;
  }
  el.textContent = msg;
  el.classList.remove("hidden");
}

function setLoading(on, msg = "") {
  const el = $("status");
  if (!el) return;
  if (on) {
    el.textContent = msg || "Working…";
    el.classList.remove("hidden");
    el.classList.add("loading");
  } else {
    el.classList.add("hidden");
    el.classList.remove("loading");
    el.textContent = "";
  }
  const navBusy = on;
  $("btn-simplify").disabled = navBusy;
  $("btn-prev").disabled = navBusy;
  $("btn-next").disabled = navBusy;
  $("btn-harder").disabled = navBusy;
  $("sentence").classList.toggle("is-loading", on);
}

function trackFromState(state) {
  if (state?.showEasier !== undefined || state?.showChinese !== undefined) {
    const e = !!state.showEasier;
    const c = !!state.showChinese;
    if (!e && !c) return "original";
    if (e) return "easier";
    return "chinese";
  }
  if (state?.track) return state.track;
  return "original";
}

function levelLabel(state) {
  const track = trackFromState(state);
  const both = state.showEasier && state.showChinese;
  if (track === "chinese") {
    return state.level === 0 ? "Original" : "中文";
  }
  if (both) {
    if (state.level === 0) return "Original";
    if (state.level >= (state.maxLevel || 4)) return "中文";
    return `Easier · level ${state.level}/${state.maxLevel || 4}`;
  }
  if (track === "easier") {
    if (state.level === 0) return "Original";
    return `Easier · level ${state.level}/${state.maxLevel || 3}`;
  }
  return "Original";
}

function readingFlagsPayload() {
  return { showEasier, showChinese };
}

function applyReadingFlags(e, c) {
  showEasier = !!e;
  showChinese = !!c;
  try {
    localStorage.setItem("showEasier", showEasier ? "1" : "0");
    localStorage.setItem("showChinese", showChinese ? "1" : "0");
  } catch (_) {}
  syncReadingFlagInputs();
}

function syncReadingFlagInputs() {
  const e = $("opt-easier");
  const c = $("opt-chinese");
  if (e) e.checked = showEasier;
  if (c) c.checked = showChinese;
  const oe = $("overlay-opt-easier");
  const oc = $("overlay-opt-chinese");
  if (oe) oe.checked = showEasier;
  if (oc) oc.checked = showChinese;
}

function readFlagsFromInputs(useOverlay) {
  const e = useOverlay ? $("overlay-opt-easier") : $("opt-easier");
  const c = useOverlay ? $("overlay-opt-chinese") : $("opt-chinese");
  return { showEasier: !!e?.checked, showChinese: !!c?.checked };
}

function modeOverlayOpen() {
  const o = $("reading-mode-overlay");
  return o && !o.classList.contains("hidden");
}

function showReadingModeOverlay(onConfirm) {
  pendingReadAction = onConfirm;
  syncReadingFlagInputs();
  const overlay = $("reading-mode-overlay");
  overlay.classList.remove("hidden");
  overlay.focus();
}

function hideReadingModeOverlay(runAction) {
  $("reading-mode-overlay").classList.add("hidden");
  if (runAction && pendingReadAction) {
    const fn = pendingReadAction;
    pendingReadAction = null;
    fn();
  } else {
    pendingReadAction = null;
  }
}

function withReadingMode(action) {
  showReadingModeOverlay(action);
}

function onModeOverlayKey(e) {
  if (!modeOverlayOpen()) return;
  if (e.key === "Escape") {
    e.preventDefault();
    hideReadingModeOverlay(false);
    return;
  }
  if (e.key === "Enter") {
    e.preventDefault();
    const f = readFlagsFromInputs(true);
    applyReadingFlags(f.showEasier, f.showChinese);
    hideReadingModeOverlay(true);
  }
}

function initReadingMode() {
  try {
    const se = localStorage.getItem("showEasier");
    const sc = localStorage.getItem("showChinese");
    if (se !== null || sc !== null) {
      applyReadingFlags(se !== "0", sc === "1");
    } else if (localStorage.getItem("readingTrack") === "chinese") {
      applyReadingFlags(false, true);
    } else if (localStorage.getItem("readingMode") === "english_chinese") {
      applyReadingFlags(false, true);
    } else if (localStorage.getItem("readingMode") === "easier_chinese") {
      applyReadingFlags(true, true);
    }
  } catch (_) {}
  const onLoadFlagChange = () => {
    const f = readFlagsFromInputs(false);
    applyReadingFlags(f.showEasier, f.showChinese);
    if (sessionId) changeReadingModeInSession();
  };
  $("opt-easier")?.addEventListener("change", onLoadFlagChange);
  $("opt-chinese")?.addEventListener("change", onLoadFlagChange);
  $("reading-mode-overlay")?.addEventListener("keydown", onModeOverlayKey);
}

initReadingMode();

function zhPendingMessage(state) {
  if (!state.bookRewriteActive || !state.bookRewriteTotal) {
    return "中文 not ready for this sentence yet";
  }
  const t = state.bookRewriteTotal;
  const eng = state.bookRewriteDone ?? 0;
  if (eng < t) {
    return `english.json ${eng}/${t}, then chinese.json…`;
  }
  const zh = state.bookChineseDone ?? 0;
  return `chinese.json ${zh}/${t}…`;
}

function syncSimplifySlider(state) {
  const area = $("simplify-slider-area");
  const slider = $("simplify-slider");
  if (!slider || !area) return;
  const levelTexts = state?.levelTexts || [];
  const hasLevels = levelTexts.length > 0;
  area.classList.toggle("hidden", !hasLevels);
  if (!hasLevels) {
    $("sentence-easier")?.classList.add("hidden");
    $("sentence-easier")?.classList.remove("sentence-active");
    $("sentence")?.classList.remove("sentence-faded");
    $("sentence")?.style.removeProperty("opacity");
    return;
  }
  // reset slider on sentence change
  if (lastSlideIndex !== state.index) {
    lastSlideIndex = state.index;
    simplifySliderValue = 0;
    if (slider) slider.value = 0;
  }
  if (simplifySliderValue > 0) {
    applySimplifyCrossfade(levelTexts, simplifySliderValue);
  } else {
    $("sentence-easier")?.classList.add("hidden");
    $("sentence-easier")?.classList.remove("sentence-active");
    $("sentence")?.classList.remove("sentence-faded");
    $("sentence")?.style.removeProperty("opacity");
  }
}

function applySimplifyCrossfade(levelTexts, value) {
  const main = $("sentence");
  const overlay = $("sentence-easier");
  if (!main || !overlay) return;
  if (value <= 0) {
    overlay.classList.add("hidden");
    overlay.classList.remove("sentence-active");
    main.classList.remove("sentence-faded");
    main.style.opacity = "1";
    return;
  }
  // which level index (0 = level1, 1 = level2, 2 = level3)
  const nLevels = levelTexts.length;
  const idx = Math.min(Math.floor((value / 100) * nLevels), nLevels - 1);
  if (idx < 0) return;
  const text = levelTexts[idx];
  if (!text) return;
  overlay.textContent = text;
  overlay.classList.remove("hidden");
  overlay.classList.add("sentence-active");
  const fade = value / 100; // 0→1
  main.style.opacity = Math.max(0, 1 - fade * 1.3).toFixed(2);
  main.classList.add("sentence-faded");
}

function peekSimplify() {
  // toggle on/off
  const slider = $("simplify-slider");
  if (simplifySliderValue > 0) {
    simplifySliderValue = 0;
    if (slider) slider.value = 0;
  } else {
    simplifySliderValue = 60;
    if (slider) slider.value = 60;
  }
  const state = lastReaderState;
  const levelTexts = state?.levelTexts || [];
  applySimplifyCrossfade(levelTexts, simplifySliderValue);
}

function renderState(state) {
  if (state?.showEasier !== undefined) applyReadingFlags(state.showEasier, state.showChinese);
  lastReaderState = state;
  const track = trackFromState(state);
  const both = state.showEasier && state.showChinese;

  syncPageInput(state);
  $("level-badge").textContent = levelLabel(state);
  const modeBadge = $("reading-mode-badge");
  if (modeBadge) {
    let label = "Original only";
    if (both) label = "Easier + 中文";
    else if (state.showChinese) label = "中文";
    else if (state.showEasier) label = "Easier";
    modeBadge.textContent = label;
    modeBadge.classList.remove("hidden");
  }
  renderPrepBar(state);
  syncSimplifySlider(state);
  const zhEl = $("chinese-text");
  if (zhEl) {
    zhEl.textContent = "";
    zhEl.classList.add("hidden");
    zhEl.setAttribute("aria-hidden", "true");
  }
  const compare = $("compare-panel");
  const compareLabel = $("compare-label");
  let main = state.sentence || state.original || "";
  if (track === "chinese" && state.level === 0 && !state.canGoHarder) {
    if (!state.chineseReady && state.bookRewriteActive) main = zhPendingMessage(state);
    else if (!state.chineseReady) main = state.original || "";
  } else if (
    both &&
    state.level >= 3 &&
    !state.chineseReady &&
    state.canGoHarder &&
    !state.canSimplify
  ) {
    main = zhPendingMessage(state);
  }
  $("sentence").textContent = main;
  if (state.level > 0 && state.previous && track !== "original") {
    if (compareLabel) compareLabel.textContent = "Original";
    $("previous-text").textContent = state.previous;
    compare.classList.remove("hidden");
    compare.setAttribute("aria-hidden", "false");
  } else {
    compare.classList.add("hidden");
    compare.setAttribute("aria-hidden", "true");
  }
  const keysHint = $("html-keys-hint");
  if (keysHint) {
    if (both) {
      keysHint.textContent = "↓ prev · ↑ next · ← harder · → easier 1→2→3→中文 · M options · Esc back";
    } else if (track === "easier" || track === "chinese") {
      keysHint.textContent = "↓ prev · ↑ next · ← harder · → simpler · M options · Esc back";
    } else {
      keysHint.textContent = "↓ prev · ↑ next · Esc back";
    }
  }
  $("btn-simplify").disabled = busy || !state.canSimplify;
  $("btn-simplify").title = state.canSimplify ? "Simpler (→)" : "At limit";
  $("btn-harder").disabled = busy || !state.canGoHarder;
  $("btn-harder").title = state.canGoHarder ? "Harder (←)" : "At limit";
  const voiceReader = $("opt-voice-reader");
  if (voiceReader) {
    voiceReader.disabled = !state.bookTTSEnabled;
    voiceReader.title = state.bookTTSEnabled
      ? "Play pre-generated voice for each sentence"
      : "No voice files for this book (upload with voice enabled)";
  }
  if (playVoiceInReader && currentBookId && state.hasAudio) {
    if (lastPlayedAudioIndex !== state.index) playReaderVoice(currentBookId, state.index);
  } else if (!state.hasAudio) {
    lastPlayedAudioIndex = -1;
  }
}

async function api(path, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  if (sessionId) headers["X-Session-Id"] = sessionId;
  const res = await fetch(path, { ...opts, headers });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  return res.json();
}

async function loadBody(body, isJSON) {
  const opts = { method: "POST", body };
  if (isJSON) {
    opts.headers = { "Content-Type": "application/json" };
  }
  return api("/api/load", opts);
}

async function doStartPaste() {
  const text = $("paste-text").value.trim();
  if (!text) {
    showError($("load-error"), "Paste some text first.");
    return;
  }
  showError($("load-error"), "");
  const f = readFlagsFromInputs(false);
  applyReadingFlags(f.showEasier, f.showChinese);
  const data = await loadBody(
    JSON.stringify({ text, ...readingFlagsPayload() }),
    true
  );
  beginSession(data);
}

async function refreshLibrary() {
  const ul = $("library-list");
  const hint = $("library-status");
  try {
    const res = await fetch("/api/library");
    if (!res.ok) {
      const t = await res.text();
      throw new Error(t || res.statusText);
    }
    const data = await res.json();
    libraryBooks = Array.isArray(data.books) ? data.books : [];
    ul.innerHTML = "";
    selectedBookId = null;
    if (hint) hint.textContent = "";
    if (!libraryBooks.length) {
      ul.innerHTML = "<li class='library-empty'>No saved books — upload a file below.</li>";
      return;
    }
    libraryBooks.forEach((b) => {
      const li = document.createElement("li");
      const title = b.title || "Untitled";
      li.textContent = `${title} — ${libraryStatusLabel(b)}`;
      li.dataset.id = b.id;
      li.title =
        b.rewriteStatus === "error" && b.rewriteError
          ? b.rewriteError
          : "";
      li.addEventListener("click", () => {
        selectedBookId = b.id;
        ul.querySelectorAll("li").forEach((el) => el.classList.remove("selected"));
        li.classList.add("selected");
      });
      ul.appendChild(li);
    });
  } catch (e) {
    libraryBooks = [];
    selectedBookId = null;
    ul.innerHTML = "";
    if (hint) hint.textContent = "";
    const msg = e && e.message ? e.message : String(e);
    ul.innerHTML = `<li class='library-empty'>Could not load library: ${msg}</li>`;
  }
}

function showRewriteOverlay(title) {
  $("rewrite-title").textContent = `Rewriting: ${title}`;
  $("rewrite-progress").value = 0;
  $("rewrite-label").textContent = "0 / 0 sentences";
  $("rewrite-overlay").classList.remove("hidden");
}

function hideRewriteOverlay() {
  $("rewrite-overlay").classList.add("hidden");
  if (rewritePollTimer) {
    clearTimeout(rewritePollTimer);
    rewritePollTimer = null;
  }
}

function libraryRewriteFinished(book) {
  const t = book?.totalSentences || 0;
  if (!t) return book?.rewriteStatus === "done";
  if (book.voiceOnly) {
    if (book.rewriteStatus !== "done") return false;
    return !book.ttsEnabled || (book.audioRewriteDone ?? 0) >= t;
  }
  const eng = book.rewriteDone ?? 0;
  const zh = book.chineseRewriteDone ?? 0;
  const aud = book.audioRewriteDone ?? 0;
  if (book.rewriteStatus !== "done" || eng < t || zh < t) return false;
  if (book.ttsEnabled) return aud >= t;
  return true;
}

function pollRewriteProgress(bookId, onDone) {
  if (rewritePollTimer) {
    clearTimeout(rewritePollTimer);
    rewritePollTimer = null;
  }
  const tick = async () => {
    try {
      const data = await api(`/api/library/book?id=${encodeURIComponent(bookId)}`);
      const book = data.book;
      const inReader =
        document.body.classList.contains("reader-active") && currentBookId === bookId;

      if (inReader && sessionId) {
        const st = await api("/api/state");
        renderState(st.state);
      } else {
        const t = book.totalSentences || 0;
        const eng = book.rewriteDone ?? 0;
        const zh = book.chineseRewriteDone ?? 0;
        const aud = book.audioRewriteDone ?? 0;
        let phase = "english";
        let done = eng;
        const voiceGen = !!book.audioGenerating;
        if (book.voiceOnly) {
          phase = "voice";
          done = aud;
        } else if (t > 0 && eng >= t && zh < t) {
          phase = "chinese";
          done = zh;
        } else if (t > 0 && eng >= t && zh >= t && book.ttsEnabled && aud < t) {
          phase = "voice";
          done = aud;
        }
        const indet = voiceGen || (phase === "voice" && aud === 0 && book.rewriteStatus === "rewriting");
        const pct = t ? (100 * done) / t : 0;
        const prog = $("rewrite-progress");
        const label = $("rewrite-label");
        setProgressBar(prog, pct, indet);
        if (label) {
          label.textContent = voiceGen
            ? "Generating voice…"
            : phase === "voice"
              ? `voice ${aud} / ${t}`
              : phase === "chinese"
                ? `chinese.json ${zh} / ${t}`
                : `english.json ${eng} / ${t}`;
        }
      }

      if (libraryRewriteFinished(book)) {
        hideRewriteOverlay();
        refreshLibrary();
        if (onDone) onDone();
        return;
      }
      if (book.rewriteStatus === "error") {
        hideRewriteOverlay();
        showError($("load-error"), book.rewriteError || "Rewrite failed");
        refreshLibrary();
        return;
      }
      const pollMs = book.audioGenerating ? 800 : 1500;
      rewritePollTimer = setTimeout(tick, pollMs);
    } catch (_) {
      rewritePollTimer = setTimeout(tick, 1000);
    }
  };
  tick();
}

async function openLibraryBook(bookId) {
  const data = await api("/api/library/open", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ bookId }),
  });
  beginSession(data);
}

async function changeReadingModeInSession() {
  if (!sessionId) return;
  try {
    const data = await api("/api/reading-mode", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(readingFlagsPayload()),
    });
    renderState(data.state);
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  }
}

async function doOpenSelectedBook() {
  if (!selectedBookId) {
    showError($("load-error"), "Select a book from the library.");
    return;
  }
  showError($("load-error"), "");
  await openLibraryBook(selectedBookId);
}

async function doImportBookFile() {
  const input = $("file-input");
  if (!input.files?.length) {
    showError($("load-error"), "Choose a file.");
    return;
  }
  showError($("load-error"), "");
  saveTtsPrefs();
  if (generateVoice) await saveTtsServerSettings();
  const fd = new FormData();
  fd.append("showEasier", showEasier ? "true" : "false");
  fd.append("showChinese", showChinese ? "true" : "false");
  fd.append("generateVoice", generateVoice ? "true" : "false");
  fd.append("voiceOnly", voiceOnly ? "true" : "false");
  fd.append("file", input.files[0]);
  const res = await fetch("/api/library/import", { method: "POST", body: fd });
  if (!res.ok) {
    throw new Error(await res.text());
  }
  const data = await res.json();
  refreshLibrary();
  await openLibraryBook(data.book.id);
  refreshLibrary();
}

function selectedBookTitle() {
  const b = libraryBooks.find((x) => x.id === selectedBookId);
  return b?.title || "this book";
}

async function deleteSelectedBook() {
  if (!selectedBookId) {
    showError($("load-error"), "Select a book from the library.");
    return;
  }
  const title = selectedBookTitle();
  if (!confirm(`Delete “${title}” from your library? This cannot be undone.`)) {
    return;
  }
  showError($("load-error"), "");
  const id = selectedBookId;
  try {
    await api("/api/library/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bookId: id }),
    });
    if (currentBookId === id) {
      hideRewriteOverlay();
      newSession();
    }
    selectedBookId = null;
    await refreshLibrary();
  } catch (e) {
    showError($("load-error"), friendlyError(e));
  }
}

function setProgressBar(prog, pct, indeterminate) {
  if (!prog) return;
  if (indeterminate) {
    prog.classList.add("indeterminate");
    prog.removeAttribute("value");
  } else {
    prog.classList.remove("indeterminate");
    prog.value = pct;
  }
}

function renderPrepBar(state) {
  const bar = $("prep-status-bar");
  const spinner = $("prep-spinner");
  const text = $("prep-status-text");
  const prog = $("prep-book-progress");
  if (!bar || !text) return;
  const bookPending = bookRewritePending(state);
  const bookMsg = bookRewriteMessage(state);
  const indet = bookRewriteIndeterminate(state);
  let msg = state.prepStatus || (state.prepActive ? "Preparing…" : "Ready");
  if (bookMsg) msg = bookMsg;
  text.textContent = msg;
  const active = bookPending;
  bar.classList.toggle("hidden", !active);
  if (prog) {
    if (bookPending) {
      prog.classList.remove("hidden");
      setProgressBar(prog, bookRewritePercent(state), indet);
    } else {
      prog.classList.add("hidden");
      prog.classList.remove("indeterminate");
    }
  }
  if (active) {
    spinner?.classList.toggle("hidden", bookPending && !indet);
    bar.classList.remove("ready");
  } else {
    spinner?.classList.add("hidden");
    bar.classList.add("ready");
  }
}

function pollUntilReady() {
  if (!sessionId) return;
  const tick = async () => {
    try {
      const data = await api("/api/state");
      renderState(data.state);
      const waitingZh =
        data.state.showChinese &&
        !data.state.chineseReady &&
        ((data.state.showEasier && data.state.level >= 3) ||
          (!data.state.showEasier && data.state.level > 0));
      const pollMs = data.state.bookAudioGenerating ? 800 : 1500;
      if (data.state.prepActive || bookRewritePending(data.state) || waitingZh) {
        setTimeout(tick, pollMs);
      }
    } catch (_) {}
  };
  setTimeout(tick, 800);
}

async function syncReadingModeToServer() {
  if (!sessionId) return;
  try {
    const data = await api("/api/reading-mode", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(readingFlagsPayload()),
    });
    renderState(data.state);
  } catch (_) {}
}

async function beginSession(data) {
  sessionId = data.sessionId;
  currentBookId = data.bookId || null;
  sourceFilename = data.sourceFilename || "paste";
  document.body.classList.add("reader-active");
  $("load-panel").classList.add("hidden");
  $("reader-panel").classList.remove("hidden");
  const titleEl = $("reader-book-title");
  if (titleEl) titleEl.textContent = sourceFilename || "Reading";
  renderState(data.state);
  await syncReadingModeToServer();
  $("html-reader-main")?.focus();
  pollUntilReady();
  if (currentBookId) pollRewriteProgress(currentBookId);
}

async function saveProgress(closeRewrite = false) {
  if (!sessionId) return;
  try {
    await api("/api/progress", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId, close: closeRewrite }),
    });
  } catch (_) {}
}

function saveProgressBeacon(closeRewrite = false) {
  if (!sessionId) return;
  const body = JSON.stringify({ sessionId, close: closeRewrite });
  const blob = new Blob([body], { type: "application/json" });
  navigator.sendBeacon("/api/progress", blob);
}

function newSession() {
  if (rsvpActive) exitRsvp(false);
  saveProgressBeacon(true);
  stopReaderVoice();
  lastPlayedAudioIndex = -1;
  sessionId = null;
  currentBookId = null;
  busy = false;
  hideReadingModeOverlay(false);
  document.body.classList.remove("reader-active");
  $("reader-panel").classList.add("hidden");
  $("load-panel").classList.remove("hidden");
  showError($("reader-error"), "");
  refreshLibrary();
}

async function downloadBlob(res, fallbackName) {
  if (!res.ok) {
    throw new Error(await res.text());
  }
  const blob = await res.blob();
  const disp = res.headers.get("Content-Disposition") || "";
  const m = /filename="([^"]+)"/i.exec(disp);
  const name = m ? m[1] : fallbackName;
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  URL.revokeObjectURL(a.href);
}

async function downloadExport(kind) {
  if (!sessionId) return;
  const res = await fetch(`/api/export/${kind}`, {
    headers: { "X-Session-Id": sessionId },
  });
  const base = sourceFilename.replace(/\.[^.]+$/, "") || "paste";
  const fallback = kind === "html" ? `${base}.html` : `${base}.${kind}.txt`;
  await downloadBlob(res, fallback);
}

async function exportLibraryHTML(bookId) {
  const id = bookId || selectedBookId;
  if (!id) {
    throw new Error("Select a book from the library.");
  }
  const res = await fetch(`/api/library/export?id=${encodeURIComponent(id)}`);
  await downloadBlob(res, "book.html");
}

async function easier() {
  if (!sessionId || busy) return;
  busy = true;
  showError($("reader-error"), "");
  try {
    const data = await api("/api/easier", { method: "POST" });
    renderState(data.state);
    if (data.state.chinesePreparing) pollUntilReady();
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  } finally {
    busy = false;
    if (sessionId) {
      const data = await api("/api/state");
      renderState(data.state);
    }
  }
}

async function nextSentence() {
  if (!sessionId || busy) return;
  busy = true;
  try {
    const data = await api("/api/next", { method: "POST" });
    renderState(data.state);
    pollUntilReady();
  } finally {
    busy = false;
  }
}

async function prevSentence() {
  if (!sessionId || busy) return;
  busy = true;
  try {
    const data = await api("/api/prev", { method: "POST" });
    renderState(data.state);
    pollUntilReady();
  } finally {
    busy = false;
  }
}

async function gotoPage(pageNum) {
  if (!sessionId || busy || !lastReaderState) return;
  const total = lastReaderState.total;
  const n = parseInt(String(pageNum), 10);
  if (!Number.isFinite(n) || n < 1 || n > total) {
    syncPageInput(lastReaderState);
    return;
  }
  if (n === lastReaderState.index + 1) return;
  busy = true;
  try {
    const data = await api("/api/goto", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ page: n }),
    });
    renderState(data.state);
    pollUntilReady();
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
    syncPageInput(lastReaderState);
  } finally {
    busy = false;
  }
}

async function harder() {
  if (!sessionId || busy) return;
  busy = true;
  showError($("reader-error"), "");
  try {
    const data = await api("/api/harder", { method: "POST" });
    renderState(data.state);
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  } finally {
    busy = false;
  }
}

function friendlyError(e) {
  const msg = e.message || String(e);
  if (msg.includes("fetch") || msg.includes("Failed")) {
    return "Cannot reach server. Start the app: go run .  then open this page.";
  }
  return msg;
}

function onReaderKey(e) {
  if (onRsvpKey(e)) return;
  if (modeOverlayOpen()) return;
  if ($("reader-panel").classList.contains("hidden")) return;
  if (busy) return;
  if (e.target.matches("textarea, input:not([type='hidden'])")) return;

  if (e.key === "Escape") {
    e.preventDefault();
    newSession();
    return;
  }
  if (e.key === "r" || e.key === "R") {
    e.preventDefault();
    enterRsvp();
    return;
  }
  if (e.key === "m" || e.key === "M") {
    e.preventDefault();
    withReadingMode(() => changeReadingModeInSession());
    return;
  }
  if (e.key === "ArrowDown") {
    e.preventDefault();
    prevSentence();
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    nextSentence();
  } else if (e.key === "ArrowRight") {
    e.preventDefault();
    easier(); // EN+中文 mode: show original + 中文
  } else if (e.key === "ArrowLeft") {
    e.preventDefault();
    harder(); // EN+中文 mode: back to original only
  }
}

async function loadLLMSettings() {
  const prov = $("llm-provider");
  const sel = $("llm-model");
  if (!sel) return;
  try {
    const res = await fetch("/api/llm");
    if (!res.ok) throw new Error("bad status");
    const data = await res.json();
    const provider = data.provider || "ollama";
    if (prov) prov.value = provider;
    try {
      localStorage.setItem("llmProvider", provider);
    } catch (_) {}
    const models = Array.isArray(data.models) ? data.models : [];
    const current = data.model || "";
    sel.innerHTML = "";
    const hint = document.querySelector(".llm-hint");
    if (hint && data.freebuffWarning) {
      hint.textContent = data.freebuffWarning;
    }
    if (models.length === 0) {
      const opt = document.createElement("option");
      opt.value = current;
      opt.textContent =
        data.freebuffWarning ||
        current ||
        (provider === "freebuff" ? "(no root models on proxy — reinstall Freebuff2API)" : "(no models — ollama pull …)");
      sel.appendChild(opt);
    } else {
      for (const name of models) {
        const opt = document.createElement("option");
        opt.value = name;
        opt.textContent = name;
        sel.appendChild(opt);
      }
      if (current && !models.includes(current)) {
        const opt = document.createElement("option");
        opt.value = current;
        opt.textContent = `${current} (saved)`;
        sel.insertBefore(opt, sel.firstChild);
      }
    }
    sel.value = current || models[0] || "";
    sel.disabled = false;
    updateServerStatusLine(data);
  } catch (_) {
    sel.innerHTML = "<option>LLM unavailable</option>";
    sel.disabled = true;
  }
}

async function setLLMSettings() {
  const provider = $("llm-provider")?.value || "ollama";
  const model = $("llm-model")?.value || "";
  if (!model) return;
  try {
    localStorage.setItem("llmProvider", provider);
    localStorage.setItem("llmModel", model);
  } catch (_) {}
  const res = await fetch("/api/llm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, model }),
  });
  if (!res.ok) throw new Error(await res.text());
  const data = await res.json();
  updateServerStatusLine({ provider: data.provider, model: data.model, llmOk: true });
}

function updateServerStatusLine(data) {
  const el = $("server-status");
  if (!el) return;
  el.classList.remove("error", "ok", "retry");
  const model = data.model || $("llm-model")?.value || "?";
  const provider = data.provider || $("llm-provider")?.value || "ollama";
  const url = data.url || "";
  const label = provider === "freebuff" ? "FreeBuff" : "Ollama";
  if (data.llmOk === false || data.ollamaOk === false) {
    el.textContent = `⚠ ${label}: ${model} · NOT REACHABLE — rewrite won't work`;
    el.classList.add("error");
  } else {
    el.textContent = url ? `✓ ${label} · ${model} · ${url}` : `✓ ${label} · ${model}`;
    el.classList.add("ok");
  }
}

let healthTimer = null;

async function checkHealth() {
  if (healthTimer) {
    clearTimeout(healthTimer);
    healthTimer = null;
  }
  await loadLLMSettings();
  const ttsData = await loadTtsServerSettings();
  const llmErr = $("server-status")?.classList.contains("error");
  const ttsPending = updateTtsStatus(ttsData);
  if (llmErr || ttsPending) {
    $("server-status")?.classList.add("retry");
    const delay = ttsData?.reachable ? 5000 : 10000;
    healthTimer = setTimeout(checkHealth, delay);
  }
}

document.querySelectorAll(".tab").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((b) => b.classList.remove("active"));
    btn.classList.add("active");
    const t = btn.dataset.tab;
    $("tab-paste").classList.toggle("hidden", t !== "paste");
    $("tab-file").classList.toggle("hidden", t !== "file");
  });
});

bindClick("btn-load-paste", () =>
  withReadingMode(() => doStartPaste().catch((e) => showError($("load-error"), friendlyError(e))))
);
bindClick("btn-load-file", () =>
  withReadingMode(() => doImportBookFile().catch((e) => {
    hideRewriteOverlay();
    showError($("load-error"), friendlyError(e));
  }))
);
bindClick("btn-open-book", () =>
  withReadingMode(() =>
    doOpenSelectedBook().catch((e) => showError($("load-error"), friendlyError(e))
  ))
);
bindClick("btn-export-html", () =>
  exportLibraryHTML().catch((e) => showError($("load-error"), friendlyError(e)))
);
bindClick("btn-refresh-library", () => refreshLibrary());
bindClick("btn-delete-book", () =>
  deleteSelectedBook().catch((e) => showError($("load-error"), friendlyError(e)))
);
bindClick("btn-simplify", () => easier());
bindClick("btn-harder", () => harder());
bindClick("btn-next", () => nextSentence());
bindClick("btn-prev", () => prevSentence());
bindClick("btn-dl-original", () =>
  downloadExport("original").catch((e) => showError($("reader-error"), friendlyError(e)))
);
bindClick("btn-dl-rewritten", () =>
  downloadExport("rewritten").catch((e) => showError($("reader-error"), friendlyError(e)))
);
bindClick("btn-dl-html", () => {
  const run = currentBookId
    ? exportLibraryHTML(currentBookId)
    : downloadExport("html");
  run.catch((e) => showError($("reader-error"), friendlyError(e)));
});
bindClick("btn-new", () => newSession());
const pageInput = $("page-input");
if (pageInput) {
  pageInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      pageInput.blur();
      gotoPage(pageInput.value);
    }
  });
  pageInput.addEventListener("change", () => gotoPage(pageInput.value));
}
// Simplify slider
const simplifySlider = $("simplify-slider");
if (simplifySlider) {
  simplifySlider.addEventListener("input", function () {
    simplifySliderValue = parseInt(this.value, 10);
    const state = lastReaderState;
    if (!state) return;
    const levelTexts = state?.levelTexts || [];
    if (simplifySliderValue <= 0) {
      $("sentence-easier")?.classList.add("hidden");
      $("sentence-easier")?.classList.remove("sentence-active");
      $("sentence")?.classList.remove("sentence-faded");
      $("sentence")?.style.removeProperty("opacity");
      return;
    }
    applySimplifyCrossfade(levelTexts, simplifySliderValue);
  });
}

// Double-tap / double-click on sentence for quick peek
const sentenceEl = $("sentence");
if (sentenceEl) {
  sentenceEl.addEventListener("dblclick", function (e) {
    e.preventDefault();
    peekSimplify();
  });
}

window.addEventListener("keydown", onReaderKey, true);
window.addEventListener("keydown", onModeOverlayKey, true);
window.addEventListener("beforeunload", () => saveProgressBeacon(true));
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") saveProgressBeacon(false);
});

$("llm-provider")?.addEventListener("change", () => {
  const provider = $("llm-provider")?.value || "ollama";
  fetch("/api/llm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider }),
  })
    .then(() => loadLLMSettings())
    .catch((err) => showError($("load-error"), friendlyError(err)));
});
$("llm-model")?.addEventListener("change", (e) => {
  setLLMSettings().catch((err) => {
    showError($("load-error"), friendlyError(err));
    loadLLMSettings();
  });
});

checkHealth();
refreshLibrary();
bindRsvpUI();
