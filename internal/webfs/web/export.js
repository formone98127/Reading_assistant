const $ = (id) => document.getElementById(id);

function showError(el, msg) {
  if (!el) return;
  if (!msg) {
    el.textContent = "";
    el.classList.add("hidden");
    return;
  }
  el.textContent = msg;
  el.classList.remove("hidden");
}

function bookStatusLabel(b) {
  const total = b.totalSentences || 0;
  const eng = b.rewriteDone ?? 0;
  const zh = b.chineseRewriteDone ?? 0;
  if (b.rewriteStatus === "rewriting" && total > 0 && eng >= total && zh < total) {
    return `chinese ${zh}/${total}`;
  }
  switch (b.rewriteStatus) {
    case "rewriting":
      return total > 0 ? `english ${eng}/${total}` : "rewriting";
    case "done":
      if (total > 0 && zh < total) {
        return `chinese ${zh}/${total}`;
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

function exportMode() {
  const el = $("export-mode");
  return el?.value === "english" ? "english" : "english_chinese";
}

function bookSiteURL(bookId) {
  const mode = exportMode();
  const base = `${location.origin}/book/${encodeURIComponent(bookId)}`;
  return mode === "english_chinese" ? `${base}?mode=english_chinese` : base;
}

function downloadURL(bookId) {
  const mode = exportMode();
  let url = `/api/library/export?id=${encodeURIComponent(bookId)}`;
  if (mode === "english_chinese") url += "&mode=english_chinese";
  return url;
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (_) {
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  }
}

function renderBooks(books) {
  const ul = $("export-book-list");
  const status = $("export-status");
  if (!ul) return;
  ul.innerHTML = "";
  if (!books.length) {
    if (status) status.textContent = "";
    ul.innerHTML =
      '<li class="export-empty">No books in the library yet. <a href="/">Upload one</a> first.</li>';
    return;
  }
  if (status) status.textContent = `${books.length} book${books.length === 1 ? "" : "s"} in library`;
  books.forEach((b) => {
    const li = document.createElement("li");
    li.className = "export-book-card";
    const title = b.title || "Untitled";
    const siteUrl = bookSiteURL(b.id);
    li.innerHTML = `
      <h3>${escapeHtml(title)}</h3>
      <p class="export-book-meta">${b.totalSentences || 0} sentences · ${bookStatusLabel(b)}</p>
      <div class="export-book-actions">
        <a class="btn-primary" href="${siteUrl}" target="_blank" rel="noopener">Open website</a>
        <a class="btn-secondary" href="${downloadURL(b.id)}" download>Download HTML</a>
        <button type="button" class="btn-ghost btn-publish" data-id="${b.id}">Save reader.html</button>
      </div>
      <div class="export-url-row">
        <input type="text" readonly value="${escapeHtml(siteUrl)}" aria-label="Book site URL" />
        <button type="button" class="btn-ghost btn-copy" data-url="${escapeHtml(siteUrl)}">Copy link</button>
      </div>
    `;
    ul.appendChild(li);
  });

  ul.querySelectorAll(".btn-copy").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const url = btn.dataset.url;
      if (await copyText(url)) {
        const prev = btn.textContent;
        btn.textContent = "Copied";
        setTimeout(() => {
          btn.textContent = prev;
        }, 1500);
      }
    });
  });

  ul.querySelectorAll(".btn-publish").forEach((btn) => {
    btn.addEventListener("click", () => publishBook(btn.dataset.id, btn));
  });
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/"/g, "&quot;");
}

async function loadBooks() {
  const errEl = $("export-error");
  showError(errEl, "");
  const status = $("export-status");
  if (status) status.textContent = "Loading library…";
  try {
    const res = await fetch("/api/library");
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    renderBooks(Array.isArray(data.books) ? data.books : []);
  } catch (e) {
    if (status) status.textContent = "";
    showError(errEl, e.message || String(e));
  }
}

async function publishBook(bookId, btn) {
  const errEl = $("export-error");
  showError(errEl, "");
  const mode = exportMode();
  const prev = btn.textContent;
  btn.disabled = true;
  btn.textContent = "Saving…";
  try {
    let url = `/api/library/publish?id=${encodeURIComponent(bookId)}`;
    if (mode === "english_chinese") url += "&mode=english_chinese";
    const res = await fetch(url, { method: "POST" });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    btn.textContent = "Saved";
    const statusEl = $("export-status");
    if (statusEl) {
      statusEl.textContent = `Saved ${data.fileName} in library folder — ${data.siteUrl}`;
    }
    setTimeout(() => {
      btn.textContent = prev;
      btn.disabled = false;
    }, 2000);
  } catch (e) {
    btn.textContent = prev;
    btn.disabled = false;
    showError(errEl, e.message || String(e));
  }
}

window.addEventListener("error", (e) => {
  showError($("export-error"), e.message || "Script error");
});

$("btn-refresh-books")?.addEventListener("click", () => loadBooks());
$("export-mode")?.addEventListener("change", () => loadBooks());
loadBooks().catch((e) => showError($("export-error"), e?.message || String(e)));
