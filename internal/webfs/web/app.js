let sessionId = null;
let busy = false;
let sourceFilename = "paste";
let libraryBooks = [];
let selectedBookId = null;
let currentBookId = null;
let rewritePollTimer = null;

const FONT_MIN = 16;
const FONT_MAX = 180;
const FONT_DEFAULT = 22;
let readerFontSize = FONT_DEFAULT;

const $ = (id) => document.getElementById(id);

function bindClick(id, handler) {
  const el = $(id);
  if (el) el.addEventListener("click", handler);
}

function libraryStatusLabel(b) {
  switch (b.rewriteStatus) {
    case "rewriting":
      return `rewriting ${b.rewriteDone}/${b.totalSentences}`;
    case "done":
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

function levelLabel(level) {
  if (level === 0) return "Original";
  return `Easier · level ${level}/3`;
}

function renderState(state) {
  $("progress").textContent = `${state.index + 1} / ${state.total}`;
  $("level-badge").textContent = levelLabel(state.level);
  renderPrepBar(state);
  $("sentence").textContent = state.sentence;
  const compare = $("compare-panel");
  if (state.level > 0 && state.original) {
    $("previous-text").textContent = state.original;
    compare.classList.remove("hidden");
    compare.setAttribute("aria-hidden", "false");
  } else {
    compare.classList.add("hidden");
    compare.setAttribute("aria-hidden", "true");
  }
  if (!state.canSimplify) {
    $("btn-simplify").disabled = true;
    $("btn-simplify").title = "Already at simplest level";
  } else if (!busy) {
    $("btn-simplify").disabled = false;
    $("btn-simplify").title = "Simplify (↓)";
  }
  $("btn-harder").disabled = busy || !state.canGoHarder;
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

async function startPaste() {
  const text = $("paste-text").value.trim();
  if (!text) {
    showError($("load-error"), "Paste some text first.");
    return;
  }
  showError($("load-error"), "");
  const data = await loadBody(JSON.stringify({ text }), true);
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

function pollRewriteProgress(bookId, onDone) {
  const tick = async () => {
    try {
      const data = await api(`/api/library/book?id=${encodeURIComponent(bookId)}`);
      const book = data.book;
      const pct = book.totalSentences ? (100 * book.rewriteDone) / book.totalSentences : 0;
      $("rewrite-progress").value = pct;
      $("rewrite-label").textContent = `${book.rewriteDone} / ${book.totalSentences} sentences`;
      if (book.rewriteStatus === "done") {
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
    } catch (e) {
      rewritePollTimer = setTimeout(tick, 2000);
    }
  };
  tick();
}

async function importBookFile() {
  const input = $("file-input");
  if (!input.files?.length) {
    showError($("load-error"), "Choose a file.");
    return;
  }
  showError($("load-error"), "");
  const fd = new FormData();
  fd.append("file", input.files[0]);
  const res = await fetch("/api/library/import", { method: "POST", body: fd });
  if (!res.ok) {
    throw new Error(await res.text());
  }
  const data = await res.json();
  refreshLibrary();
  await openLibraryBook(data.book.id);
}

async function openLibraryBook(bookId) {
  const data = await api("/api/library/open", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ bookId }),
  });
  beginSession(data);
}

async function startFile() {
  try {
    await importBookFile();
  } catch (e) {
    showError($("load-error"), friendlyError(e));
  }
}

async function openSelectedBook() {
  if (!selectedBookId) {
    showError($("load-error"), "Select a book from the library.");
    return;
  }
  showError($("load-error"), "");
  try {
    await openLibraryBook(selectedBookId);
  } catch (e) {
    showError($("load-error"), friendlyError(e));
  }
}

function renderPrepBar(state) {
  const bar = $("prep-status-bar");
  const spinner = $("prep-spinner");
  const text = $("prep-status-text");
  const msg = state.prepStatus || (state.prepActive ? "Preparing…" : "Ready");
  text.textContent = msg;
  const active = state.prepActive || state.bookRewriteActive;
  if (active) {
    spinner.classList.remove("hidden");
    bar.classList.remove("ready");
  } else {
    spinner.classList.add("hidden");
    bar.classList.add("ready");
  }
}

function pollUntilReady() {
  if (!sessionId) return;
  const tick = async () => {
    try {
      const data = await api("/api/state");
      renderState(data.state);
      if (data.state.prepActive || data.state.bookRewriteActive) {
        setTimeout(tick, 1500);
      }
    } catch (_) {}
  };
  setTimeout(tick, 800);
}

function beginSession(data) {
  sessionId = data.sessionId;
  currentBookId = data.bookId || null;
  sourceFilename = data.sourceFilename || "paste";
  $("load-panel").classList.add("hidden");
  $("reader-panel").classList.remove("hidden");
  renderState(data.state);
  $("sentence").focus();
  pollUntilReady();
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
  renderPrepBar({ prepStatus: "Simplifying with Gemma…", prepActive: true });
  setLoading(true, "");
  try {
    const data = await api("/api/easier", { method: "POST" });
    renderState(data.state);
  } catch (e) {
    showError($("reader-error"), friendlyError(e));
  } finally {
    busy = false;
    setLoading(false);
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
  if ($("reader-panel").classList.contains("hidden")) return;
  if (busy) return;
  if (e.target.matches("textarea, input")) return;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    easier();
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    harder();
  } else if (e.key === "ArrowRight") {
    e.preventDefault();
    nextSentence();
  } else if (e.key === "ArrowLeft") {
    e.preventDefault();
    prevSentence();
  }
}

async function checkHealth() {
  const el = $("server-status");
  try {
    const res = await fetch("/api/health");
    if (!res.ok) throw new Error("bad status");
    const h = await res.json();
    el.textContent = `Model: ${h.model} · Ollama: ${h.ollama}`;
  } catch (_) {
    el.textContent = "Server not running — start with: go run .";
    el.classList.add("error");
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
  startPaste().catch((e) => showError($("load-error"), friendlyError(e)))
);
bindClick("btn-load-file", () =>
  startFile().catch((e) => showError($("load-error"), friendlyError(e)))
);
bindClick("btn-open-book", () => openSelectedBook());
bindClick("btn-export-html", () =>
  exportLibraryHTML().catch((e) => showError($("load-error"), friendlyError(e)))
);
bindClick("btn-refresh-library", () => refreshLibrary());
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
window.addEventListener("beforeunload", () => saveProgressBeacon(true));
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") saveProgressBeacon(false);
});

checkHealth();
refreshLibrary();
