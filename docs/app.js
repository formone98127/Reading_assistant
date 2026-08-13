const $ = (id) => document.getElementById(id);
const LIB_KEY = "ra_online_library_v1";
const FONT_MIN = 16;
const FONT_MAX = 180;
const FONT_DEFAULT = 22;
const RSVP_MAX_CHUNK_CHARS = 20;
const ABBREV_DOT = "\uE001";
const ABBREV_RE = /\b(?:e\.g\.|i\.e\.|u\.s\.|u\.k\.|mr|mrs|ms|dr|prof|rev|hon|sr|jr|st|vs|etc|no)\./gi;

let books = [];
let selectedBookId = null;
let currentBook = null;
let index = 0;
let readerFontSize = FONT_DEFAULT;

let rsvpActive = false;
let rsvpPlaying = false;
let rsvpTimer = null;
let rsvpChunks = [];
let rsvpChunkIndex = 0;
let rsvpWordsDone = 0;
let rsvpTotalWords = 0;
let rsvpElapsedMs = 0;
let rsvpLastTick = 0;
let rsvpCompleteTimer = null;
let rsvpWpm = 300;
let rsvpChunkSize = 1;
let rsvpPush = false;
let rsvpStartWpm = 300;
let rsvpTargetWpm = 500;
let rsvpAutoContinue = false;

function showError(el, msg) {
  if (!el) return;
  if (!msg) {
    el.classList.add("hidden");
    el.textContent = "";
    return;
  }
  el.textContent = msg;
  el.classList.remove("hidden");
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function normalizeWhitespace(s) {
  s = String(s || "")
    .replace(/[\u200B-\u200D\uFEFF]/g, "")
    .replace(/[\uFF0E\u2024\u00B7]/g, ".")
    .replace(/\u00A0/g, " ")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n");
  return s
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .join(" ")
    .trim();
}

function maskAbbrevDots(s) {
  return s.replace(ABBREV_RE, (m) => m.slice(0, -1) + ABBREV_DOT);
}

function unmaskAbbrevDots(s) {
  return s.split(ABBREV_DOT).join(".");
}

function shouldSplit(chunk, runes, after) {
  chunk = chunk.trim();
  if (!chunk) return false;
  while (after < runes.length && runes[after] === ".") after++;
  if (after < runes.length) {
    const r = runes[after];
    if (/\p{L}|\p{N}/u.test(r)) return false;
  }
  return true;
}

function splitSentences(text) {
  text = normalizeWhitespace(text);
  if (!text) return [];
  const masked = maskAbbrevDots(text);
  const runes = Array.from(masked);
  const out = [];
  let buf = "";
  for (let i = 0; i < runes.length; i++) {
    const ch = runes[i];
    buf += ch;
    if (ch === ";") {
      const chunk = buf.trim();
      if (chunk) out.push(unmaskAbbrevDots(chunk));
      buf = "";
      let end = i + 1;
      while (end < runes.length && /\s/.test(runes[end])) end++;
      i = end - 1;
    } else if (ch === "." || ch === "!" || ch === "?" || ch === "…") {
      let end = i + 1;
      while (
        end < runes.length &&
        (runes[end] === "." || runes[end] === "!" || runes[end] === "?")
      ) {
        buf += runes[end];
        end++;
        i = end - 1;
      }
      const chunk = buf.trim();
      if (shouldSplit(chunk, runes, end)) {
        out.push(unmaskAbbrevDots(chunk));
        buf = "";
        while (end < runes.length && /\s/.test(runes[end])) end++;
        i = end - 1;
      }
    }
  }
  const tail = buf.trim();
  if (tail) out.push(unmaskAbbrevDots(tail));
  return out.filter((s) => s.trim());
}

function loadLibrary() {
  try {
    const raw = localStorage.getItem(LIB_KEY);
    books = raw ? JSON.parse(raw) : [];
    if (!Array.isArray(books)) books = [];
  } catch (_) {
    books = [];
  }
}

function saveLibrary() {
  try {
    localStorage.setItem(LIB_KEY, JSON.stringify(books));
  } catch (_) {}
}

function uid() {
  return `b_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

function renderLibrary() {
  const list = $("library-list");
  const status = $("library-status");
  if (!list) return;
  list.innerHTML = "";
  if (!books.length) {
    if (status) status.textContent = "No books yet — paste or upload text.";
    return;
  }
  if (status) status.textContent = `${books.length} book(s) in this browser.`;
  for (const b of books) {
    const li = document.createElement("li");
    li.dataset.id = b.id;
    if (b.id === selectedBookId) li.classList.add("selected");
    li.innerHTML = `${escapeHtml(b.title)}<span class="meta">${b.sentences.length} sentences · idx ${
      (b.index || 0) + 1
    }</span>`;
    li.addEventListener("click", () => {
      selectedBookId = b.id;
      renderLibrary();
    });
    list.appendChild(li);
  }
}

function addBook(title, text) {
  const sentences = splitSentences(text);
  if (!sentences.length) throw new Error("No sentences found.");
  const book = {
    id: uid(),
    title: title || "Untitled",
    sentences,
    index: 0,
    updatedAt: Date.now(),
  };
  books.unshift(book);
  saveLibrary();
  selectedBookId = book.id;
  renderLibrary();
  return book;
}

function openBook(book) {
  currentBook = book;
  index = Math.max(0, Math.min(book.index || 0, book.sentences.length - 1));
  document.body.classList.add("reader-active");
  $("reader-panel")?.classList.remove("hidden");
  if ($("reader-book-title")) $("reader-book-title").textContent = book.title;
  renderSentence();
  $("html-reader-main")?.focus();
}

function persistProgress() {
  if (!currentBook) return;
  currentBook.index = index;
  currentBook.updatedAt = Date.now();
  const i = books.findIndex((b) => b.id === currentBook.id);
  if (i >= 0) books[i] = currentBook;
  saveLibrary();
}

function renderSentence() {
  if (!currentBook) return;
  const s = currentBook.sentences[index] || "";
  if ($("sentence")) $("sentence").textContent = s;
  if ($("page-total")) $("page-total").textContent = String(currentBook.sentences.length);
  const input = $("page-input");
  if (input && document.activeElement !== input) {
    input.min = 1;
    input.max = currentBook.sentences.length;
    input.value = String(index + 1);
  }
  persistProgress();
}

function backToLibrary() {
  if (rsvpActive) exitRsvp();
  persistProgress();
  currentBook = null;
  document.body.classList.remove("reader-active");
  $("reader-panel")?.classList.add("hidden");
  showError($("reader-error"), "");
  renderLibrary();
}

function setFont(px) {
  readerFontSize = Math.min(FONT_MAX, Math.max(FONT_MIN, px));
  document.documentElement.style.setProperty("--reader-font-size", `${readerFontSize}px`);
  try {
    localStorage.setItem("readerFontSize", String(readerFontSize));
  } catch (_) {}
}

/* —— RSVP —— */
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
  if ($("rsvp-wpm")) $("rsvp-wpm").value = String(rsvpWpm);
  if ($("rsvp-wpm-val")) $("rsvp-wpm-val").textContent = String(rsvpWpm);
  if ($("rsvp-push")) $("rsvp-push").checked = rsvpPush;
  $("rsvp-push-fields")?.classList.toggle("hidden", !rsvpPush);
  if ($("rsvp-start-wpm")) $("rsvp-start-wpm").value = String(rsvpStartWpm);
  if ($("rsvp-start-val")) $("rsvp-start-val").textContent = String(rsvpStartWpm);
  if ($("rsvp-target-wpm")) $("rsvp-target-wpm").value = String(rsvpTargetWpm);
  if ($("rsvp-target-val")) $("rsvp-target-val").textContent = String(rsvpTargetWpm);
  document.querySelectorAll('input[name="rsvp-chunk"]').forEach((el) => {
    el.checked = String(rsvpChunkSize) === el.value;
  });
  if ($("rsvp-auto-continue")) $("rsvp-auto-continue").checked = rsvpAutoContinue;
}

function tokenizeRsvpWords(text) {
  const t = String(text || "").trim();
  if (!t) return [];
  if (/\s/.test(t)) return t.split(/\s+/).filter(Boolean);
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
  return words
    .map((w, idx) => {
      if (idx !== focusIdx) return `<span class="rsvp-word">${escapeHtml(w)}</span>`;
      const i = orpIndex(w);
      return `<span class="rsvp-word">${escapeHtml(w.slice(0, i))}<span class="rsvp-orp">${escapeHtml(
        w.slice(i, i + 1)
      )}</span>${escapeHtml(w.slice(i + 1))}</span>`;
    })
    .join(" ");
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
  index = chunk.sentenceIndex;
  renderSentence();
  updateRsvpStats();
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
    index = rsvpChunks[rsvpChunks.length - 1].sentenceIndex;
    renderSentence();
  }
  updateRsvpStats();
  $("rsvp-complete")?.classList.remove("hidden");
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
  rsvpLastTick = 0;
  showRsvpChunk();
  setRsvpPlaying(true);
}

function enterRsvp() {
  if (!currentBook || rsvpActive) return;
  loadRsvpPrefs();
  syncRsvpControlsUI();
  rsvpChunks = buildRsvpChunks(currentBook.sentences, index, rsvpChunkSize);
  rsvpTotalWords = rsvpChunks.reduce((n, c) => n + c.words.length, 0);
  if (!rsvpChunks.length) {
    showError($("reader-error"), "No words to speed-read.");
    return;
  }
  rsvpActive = true;
  rsvpChunkIndex = 0;
  rsvpWordsDone = 0;
  rsvpElapsedMs = 0;
  rsvpLastTick = 0;
  $("rsvp-overlay")?.classList.remove("hidden");
  $("rsvp-complete")?.classList.add("hidden");
  $("rsvp-overlay")?.focus();
  showRsvpChunk();
  setRsvpPlaying(true);
}

function exitRsvp() {
  if (!rsvpActive) return;
  setRsvpPlaying(false);
  clearRsvpTimer();
  clearRsvpCompleteTimer();
  rsvpActive = false;
  $("rsvp-overlay")?.classList.add("hidden");
  $("rsvp-complete")?.classList.add("hidden");
  saveRsvpPrefs();
  renderSentence();
}

function rebuildRsvpChunksFromPrefs() {
  if (!rsvpActive || !currentBook) return;
  const si = rsvpChunks[Math.min(rsvpChunkIndex, rsvpChunks.length - 1)]?.sentenceIndex ?? index;
  const wasPlaying = rsvpPlaying;
  setRsvpPlaying(false);
  rsvpChunks = buildRsvpChunks(currentBook.sentences, si, rsvpChunkSize);
  rsvpTotalWords = rsvpChunks.reduce((n, c) => n + c.words.length, 0);
  rsvpChunkIndex = 0;
  showRsvpChunk();
  if (wasPlaying) setRsvpPlaying(true);
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

function onKey(e) {
  if (rsvpActive) {
    if (e.target.matches("textarea, input:not([type='hidden']):not([type='range']):not([type='checkbox']):not([type='radio'])")) {
      return;
    }
    if (e.key === "Escape" || e.key === "r" || e.key === "R") {
      e.preventDefault();
      exitRsvp();
      return;
    }
    if (e.key === " " || e.code === "Space") {
      e.preventDefault();
      if (!$("rsvp-complete")?.classList.contains("hidden")) return;
      setRsvpPlaying(!rsvpPlaying);
      return;
    }
    if (e.key === "ArrowLeft") {
      e.preventDefault();
      nudgeRsvpWpm(-25);
      return;
    }
    if (e.key === "ArrowRight") {
      e.preventDefault();
      nudgeRsvpWpm(25);
      return;
    }
    return;
  }

  if (!currentBook) return;
  if (e.target.matches("textarea, input:not([type='hidden'])")) return;

  if (e.key === "Escape") {
    e.preventDefault();
    backToLibrary();
    return;
  }
  if (e.key === "r" || e.key === "R") {
    e.preventDefault();
    enterRsvp();
    return;
  }
  if (e.key === "ArrowDown") {
    e.preventDefault();
    if (index > 0) {
      index--;
      renderSentence();
    }
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    if (index + 1 < currentBook.sentences.length) {
      index++;
      renderSentence();
    }
  }
}

function bindUI() {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((t) => t.classList.remove("active"));
      tab.classList.add("active");
      const name = tab.dataset.tab;
      $("tab-paste")?.classList.toggle("hidden", name !== "paste");
      $("tab-file")?.classList.toggle("hidden", name !== "file");
    });
  });

  $("btn-load-paste")?.addEventListener("click", () => {
    showError($("load-error"), "");
    try {
      const text = $("paste-text")?.value || "";
      const book = addBook("Paste", text);
      openBook(book);
    } catch (e) {
      showError($("load-error"), e.message || String(e));
    }
  });

  $("btn-load-file")?.addEventListener("click", async () => {
    showError($("load-error"), "");
    const file = $("file-input")?.files?.[0];
    if (!file) {
      showError($("load-error"), "Choose a .txt file first.");
      return;
    }
    try {
      const text = await file.text();
      const book = addBook(file.name.replace(/\.txt$/i, "") || "Upload", text);
      openBook(book);
    } catch (e) {
      showError($("load-error"), e.message || String(e));
    }
  });

  $("btn-open-book")?.addEventListener("click", () => {
    const book = books.find((b) => b.id === selectedBookId);
    if (!book) {
      showError($("load-error"), "Select a book first.");
      return;
    }
    showError($("load-error"), "");
    openBook(book);
  });

  $("btn-delete-book")?.addEventListener("click", () => {
    if (!selectedBookId) return;
    books = books.filter((b) => b.id !== selectedBookId);
    selectedBookId = books[0]?.id || null;
    saveLibrary();
    renderLibrary();
  });

  $("btn-new")?.addEventListener("click", () => backToLibrary());
  $("btn-rsvp")?.addEventListener("click", () => enterRsvp());
  $("btn-rsvp-restart")?.addEventListener("click", () => restartRsvp());
  $("btn-rsvp-exit")?.addEventListener("click", () => exitRsvp());
  $("font-smaller")?.addEventListener("click", () => setFont(readerFontSize - 4));
  $("font-larger")?.addEventListener("click", () => setFont(readerFontSize + 4));

  const pageInput = $("page-input");
  pageInput?.addEventListener("change", () => {
    if (!currentBook) return;
    const n = parseInt(pageInput.value, 10);
    if (Number.isNaN(n)) return;
    index = Math.max(0, Math.min(currentBook.sentences.length - 1, n - 1));
    renderSentence();
  });
  pageInput?.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      pageInput.blur();
    }
  });

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

  window.addEventListener("keydown", onKey, true);
}

loadLibrary();
try {
  const f = parseInt(localStorage.getItem("readerFontSize"), 10);
  if (!Number.isNaN(f)) setFont(f);
  else setFont(FONT_DEFAULT);
} catch (_) {
  setFont(FONT_DEFAULT);
}
loadRsvpPrefs();
syncRsvpControlsUI();
renderLibrary();
bindUI();
