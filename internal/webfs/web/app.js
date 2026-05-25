let sessionId = null;
let busy = false;
let sourceFilename = "paste";
let libraryBooks = [];
let selectedBookId = null;
let currentBookId = null;
let rewritePollTimer = null;
let readingMode = "english";
let pendingReadAction = null;
let lastReaderState = null;

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
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  return !!state.bookRewriteActive || eng < t || (eng >= t && zh < t);
}

function bookRewritePercent(state) {
  const t = state?.bookRewriteTotal || 0;
  if (!t) return 0;
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  if (eng < t) return (100 * eng) / t;
  if (zh < t) return (100 * zh) / t;
  return 100;
}

function bookRewriteMessage(state) {
  const t = state?.bookRewriteTotal || 0;
  if (!t) return "";
  const eng = state.bookRewriteDone ?? 0;
  const zh = state.bookChineseDone ?? 0;
  if (eng < t) return `english.json ${eng}/${t} — you can read now`;
  if (zh < t) return `chinese.json ${zh}/${t} — you can read now`;
  return "";
}

function libraryStatusLabel(b) {
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

function levelLabel(state) {
  if (state.showChinese) {
    return state.chineseVisible ? "Original + 中文" : "Original";
  }
  if (state.level === 0) return "Original";
  return `Easier · level ${state.level}/3`;
}

function getReadingMode() {
  return readingMode === "english_chinese" ? "english_chinese" : "english";
}

function setReadingMode(mode) {
  readingMode = mode === "english_chinese" ? "english_chinese" : "english";
  try {
    localStorage.setItem("readingMode", readingMode);
  } catch (_) {}
  highlightOverlayMode(readingMode);
}

function highlightOverlayMode(mode) {
  document.querySelectorAll(".html-mode-choice").forEach((btn) => {
    btn.classList.toggle("selected", btn.dataset.mode === mode);
  });
}

function modeOverlayOpen() {
  const o = $("reading-mode-overlay");
  return o && !o.classList.contains("hidden");
}

function showReadingModeOverlay(onConfirm, initialMode) {
  pendingReadAction = onConfirm;
  setReadingMode(initialMode || getReadingMode());
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
  showReadingModeOverlay(action, getReadingMode());
}

function onModeOverlayKey(e) {
  if (!modeOverlayOpen()) return;
  if (e.key === "Escape") {
    e.preventDefault();
    hideReadingModeOverlay(false);
    return;
  }
  if (e.key === "1") {
    e.preventDefault();
    setReadingMode("english");
  }
  if (e.key === "2") {
    e.preventDefault();
    setReadingMode("english_chinese");
  }
  if (e.key === "Enter") {
    e.preventDefault();
    hideReadingModeOverlay(true);
  }
}

function initReadingMode() {
  try {
    const saved = localStorage.getItem("readingMode");
    if (saved === "english_chinese" || saved === "english") readingMode = saved;
  } catch (_) {}
  highlightOverlayMode(readingMode);
  document.querySelectorAll(".html-mode-choice").forEach((btn) => {
    btn.addEventListener("click", () => {
      setReadingMode(btn.dataset.mode);
      hideReadingModeOverlay(true);
    });
  });
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

function renderState(state) {
  if (state?.readingMode) {
    setReadingMode(state.readingMode);
  }
  lastReaderState = state;

  $("progress").textContent = `${state.index + 1} / ${state.total}`;
  $("level-badge").textContent = levelLabel(state);
  const modeBadge = $("reading-mode-badge");
  if (state.showChinese) {
    modeBadge.textContent = "EN + 中文";
    modeBadge.classList.remove("hidden");
  } else {
    modeBadge.textContent = "";
    modeBadge.classList.add("hidden");
  }
  renderPrepBar(state);
  const zhEl = $("chinese-text");
  const nowLabel = $("now-reading-label");
  const compare = $("compare-panel");
  const compareLabel = $("compare-label");

  if (state.showChinese) {
    if (nowLabel) {
      nowLabel.textContent = state.chineseVisible ? "English + 中文" : "Original";
    }
    $("sentence").textContent = state.original || "";
    let zhText = "";
    if (state.chineseVisible) {
      if (state.chinese && String(state.chinese).trim()) {
        zhText = state.chinese;
      } else if (state.bookRewriteActive) {
        zhText = zhPendingMessage(state);
      } else if (!state.chineseReady) {
        zhText = "中文 not ready for this sentence yet";
      }
    }
    if (zhEl) {
      zhEl.textContent = zhText;
      zhEl.classList.toggle("hidden", !zhText);
      zhEl.setAttribute("aria-hidden", zhText ? "false" : "true");
    }
    compare.classList.add("hidden");
    compare.setAttribute("aria-hidden", "true");
  } else {
    if (nowLabel) nowLabel.textContent = "Now reading";
    $("sentence").textContent = state.sentence;
    if (zhEl) {
      zhEl.textContent = "";
      zhEl.classList.add("hidden");
      zhEl.setAttribute("aria-hidden", "true");
    }
    if (state.level > 0 && state.original) {
      if (compareLabel) compareLabel.textContent = "Original";
      $("previous-text").textContent = state.original;
      compare.classList.remove("hidden");
      compare.setAttribute("aria-hidden", "false");
    } else {
      compare.classList.add("hidden");
      compare.setAttribute("aria-hidden", "true");
    }
  }
  const keysHint = $("html-keys-hint");
  if (state.showChinese) {
    if (keysHint) {
      keysHint.textContent = state.chineseVisible
        ? "← original only · ↓ prev · ↑ next · Esc back"
        : "→ English + 中文 · ↓ prev · ↑ next · Esc back";
    }
    $("btn-simplify").disabled = busy || !state.canSimplify;
    $("btn-simplify").title = state.canSimplify ? "English + 中文 (→)" : "Showing English + 中文";
    $("btn-harder").disabled = busy || !state.canGoHarder;
    $("btn-harder").title = state.canGoHarder ? "Original only (←)" : "Original only";
  } else {
    if (keysHint) {
      keysHint.textContent = "↓ prev · ↑ next · ← harder · → simpler · M mode · Esc back";
    }
    if (!state.canSimplify) {
      $("btn-simplify").disabled = true;
      $("btn-simplify").title = "Already at simplest level";
    } else if (!busy) {
      $("btn-simplify").disabled = false;
      $("btn-simplify").title = "Simpler (→)";
    }
    $("btn-harder").disabled = busy || !state.canGoHarder;
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
  const data = await loadBody(
    JSON.stringify({ text, readingMode: getReadingMode() }),
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
  const eng = book.rewriteDone ?? 0;
  const zh = book.chineseRewriteDone ?? 0;
  return book.rewriteStatus === "done" && eng >= t && zh >= t;
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
        const zhPhase = t > 0 && eng >= t && zh < t;
        const done = zhPhase ? zh : eng;
        const pct = t ? (100 * done) / t : 0;
        const prog = $("rewrite-progress");
        const label = $("rewrite-label");
        if (prog) prog.value = pct;
        if (label) {
          label.textContent = zhPhase
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
      rewritePollTimer = setTimeout(tick, 1500);
    } catch (_) {
      rewritePollTimer = setTimeout(tick, 2000);
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
      body: JSON.stringify({ readingMode: getReadingMode() }),
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
  const fd = new FormData();
  fd.append("readingMode", getReadingMode());
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

function renderPrepBar(state) {
  const bar = $("prep-status-bar");
  const spinner = $("prep-spinner");
  const text = $("prep-status-text");
  const prog = $("prep-book-progress");
  if (!bar || !text) return;
  const bookPending = bookRewritePending(state);
  const bookMsg = bookRewriteMessage(state);
  let msg = state.prepStatus || (state.prepActive ? "Preparing…" : "Ready");
  if (bookMsg) msg = bookMsg;
  text.textContent = msg;
  const active = bookPending || state.prepActive;
  bar.classList.toggle("hidden", !active);
  if (prog) {
    if (bookPending) {
      prog.classList.remove("hidden");
      prog.value = bookRewritePercent(state);
    } else {
      prog.classList.add("hidden");
    }
  }
  if (active) {
    spinner?.classList.toggle("hidden", bookPending);
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
        data.state.chineseVisible &&
        !data.state.chineseReady;
      if (data.state.prepActive || bookRewritePending(data.state) || waitingZh) {
        setTimeout(tick, 1500);
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
      body: JSON.stringify({ readingMode: getReadingMode() }),
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
  saveProgressBeacon(true);
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
  const showZh =
    getReadingMode() === "english_chinese" || lastReaderState?.showChinese;
  if (!showZh) {
    renderPrepBar({ prepStatus: "Simplifying with Gemma…", prepActive: true });
    setLoading(true, "");
  }
  try {
    const data = await api("/api/easier", { method: "POST" });
    renderState(data.state);
    if (
      data.state.showChinese &&
      data.state.chineseVisible &&
      !data.state.chineseReady
    ) {
      pollUntilReady();
    }
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  } finally {
    busy = false;
    if (!showZh) setLoading(false);
    if (sessionId && !showZh) {
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
  if (modeOverlayOpen()) return;
  if ($("reader-panel").classList.contains("hidden")) return;
  if (busy) return;
  if (e.target.matches("textarea, input:not([type='hidden'])")) return;

  if (e.key === "Escape") {
    e.preventDefault();
    newSession();
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

async function loadOllamaModels() {
  const sel = $("ollama-model");
  if (!sel) return;
  try {
    const res = await fetch("/api/ollama");
    if (!res.ok) throw new Error("bad status");
    const data = await res.json();
    const models = Array.isArray(data.models) ? data.models : [];
    const current = data.model || "";
    sel.innerHTML = "";
    if (models.length === 0) {
      const opt = document.createElement("option");
      opt.value = current;
      opt.textContent = current || "(no models — ollama pull …)";
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
    sel.innerHTML = "<option>Ollama unavailable</option>";
    sel.disabled = true;
  }
}

async function setOllamaModel(model) {
  if (!model) return;
  try {
    localStorage.setItem("ollamaModel", model);
  } catch (_) {}
  const res = await fetch("/api/ollama", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ model }),
  });
  if (!res.ok) throw new Error(await res.text());
  const data = await res.json();
  updateServerStatusLine({ model: data.model, url: null, ollamaOk: true });
}

function updateServerStatusLine(data) {
  const el = $("server-status");
  if (!el) return;
  el.classList.remove("error");
  const model = data.model || $("ollama-model")?.value || "?";
  const url = data.url || data.ollama || "";
  if (data.ollamaOk === false) {
    el.textContent = `Model: ${model} · Ollama not reachable`;
    el.classList.add("error");
  } else {
    el.textContent = url ? `Using ${model} · ${url}` : `Using ${model}`;
  }
}

async function checkHealth() {
  await loadOllamaModels();
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

window.addEventListener("keydown", onReaderKey, true);
window.addEventListener("keydown", onModeOverlayKey, true);
window.addEventListener("beforeunload", () => saveProgressBeacon(true));
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") saveProgressBeacon(false);
});

$("ollama-model")?.addEventListener("change", (e) => {
  setOllamaModel(e.target.value).catch((err) => {
    showError($("load-error"), friendlyError(err));
    loadOllamaModels();
  });
});

checkHealth();
refreshLibrary();
