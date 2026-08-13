const $ = (id) => document.getElementById(id);

function showError(msg) {
  const el = $("public-error");
  if (!el) return;
  el.textContent = msg || "";
  el.classList.toggle("hidden", !msg);
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/"/g, "&quot;");
}

async function copyText(text, btn) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (_) {
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    document.body.removeChild(ta);
  }
  const prev = btn.textContent;
  btn.textContent = "Copied";
  setTimeout(() => (btn.textContent = prev), 1500);
}

function publishQueryString() {
  const e = $("publish-opt-easier")?.checked ?? true;
  const c = $("publish-opt-chinese")?.checked ?? false;
  return `showEasier=${e}&showChinese=${c}`;
}

function bindCopyButtons(root = document) {
  root.querySelectorAll("[data-copy-url]").forEach((btn) => {
    btn.addEventListener("click", () => copyText(btn.dataset.copyUrl, btn));
  });
}

function renderLibraryBooks(books) {
  const ul = $("library-publish-list");
  if (!ul) return;
  ul.innerHTML = "";
  if (!books.length) {
    ul.innerHTML = '<li class="export-empty">No library books yet. <a href="/">Upload one</a> first.</li>';
    return;
  }
  for (const b of books) {
    const li = document.createElement("li");
    li.className = "export-book-card";
    li.innerHTML = `
      <h3>${escapeHtml(b.title || "Untitled")}</h3>
      <p class="export-book-meta">${b.totalSentences || 0} sentences · ${escapeHtml(b.rewriteStatus || "unknown")}</p>
      <div class="export-book-actions">
        <button type="button" class="btn-primary" data-publish-id="${escapeHtml(b.id)}">Publish to public</button>
        <a class="btn-secondary" href="/book/${encodeURIComponent(b.id)}?${publishQueryString()}" target="_blank" rel="noopener">Preview</a>
      </div>
    `;
    ul.appendChild(li);
  }
  ul.querySelectorAll("[data-publish-id]").forEach((btn) => {
    btn.addEventListener("click", () => publishLibraryBook(btn.dataset.publishId, btn));
  });
}

function renderPublicBooks(books) {
  const ul = $("public-book-list");
  if (!ul) return;
  ul.innerHTML = "";
  if (!books.length) {
    ul.innerHTML = '<li class="export-empty">No public uploads yet.</li>';
    return;
  }
  for (const b of books) {
    const li = document.createElement("li");
    li.className = "export-book-card";
    li.innerHTML = `
      <h3>${escapeHtml(b.title || b.fileName)}</h3>
      <p class="export-book-meta">${Math.round((b.size || 0) / 1024)} KB · ${escapeHtml(b.fileName)}</p>
      <div class="export-book-actions">
        <a class="btn-primary" href="${escapeHtml(b.url)}" target="_blank" rel="noopener">Open public page</a>
        <button type="button" class="btn-ghost" data-copy-url="${escapeHtml(b.url)}">Copy public link</button>
      </div>
      <div class="export-url-row">
        <input type="text" readonly value="${escapeHtml(b.url)}" aria-label="Public URL" />
      </div>
    `;
    ul.appendChild(li);
  }
  bindCopyButtons(ul);
}

async function loadAll() {
  showError("");
  const status = $("public-status");
  if (status) status.textContent = "Loading…";
  const [libraryRes, publicRes] = await Promise.all([fetch("/api/library"), fetch("/api/public")]);
  if (!libraryRes.ok) throw new Error(await libraryRes.text());
  if (!publicRes.ok) throw new Error(await publicRes.text());
  const library = await libraryRes.json();
  const pub = await publicRes.json();
  renderLibraryBooks(Array.isArray(library.books) ? library.books : []);
  renderPublicBooks(Array.isArray(pub.books) ? pub.books : []);
  if (status) status.textContent = "Ready";
}

async function publishLibraryBook(bookId, btn) {
  showError("");
  const prev = btn.textContent;
  btn.disabled = true;
  btn.textContent = "Publishing…";
  try {
    const res = await fetch(`/api/public/publish?id=${encodeURIComponent(bookId)}&${publishQueryString()}`, {
      method: "POST",
    });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    $("public-status").innerHTML = `Published: <a href="${escapeHtml(data.url)}" target="_blank" rel="noopener">${escapeHtml(data.url)}</a>`;
    await loadAll();
  } catch (e) {
    showError(e.message || String(e));
  } finally {
    btn.disabled = false;
    btn.textContent = prev;
  }
}

async function uploadPublicHTML(e) {
  e.preventDefault();
  showError("");
  const file = $("public-file")?.files?.[0];
  if (!file) {
    showError("Choose an exported .html file first.");
    return;
  }
  const fd = new FormData();
  fd.append("file", file);
  const title = $("public-title")?.value?.trim();
  if (title) fd.append("title", title);
  const res = await fetch("/api/public/upload", { method: "POST", body: fd });
  if (!res.ok) throw new Error(await res.text());
  const data = await res.json();
  $("public-status").innerHTML = `Uploaded: <a href="${escapeHtml(data.url)}" target="_blank" rel="noopener">${escapeHtml(data.url)}</a>`;
  $("upload-form").reset();
  await loadAll();
}

$("upload-form")?.addEventListener("submit", (e) => {
  uploadPublicHTML(e).catch((err) => showError(err.message || String(err)));
});
$("btn-refresh-public")?.addEventListener("click", () => loadAll().catch((e) => showError(e.message || String(e))));
$("publish-opt-easier")?.addEventListener("change", () => loadAll().catch((e) => showError(e.message || String(e))));
$("publish-opt-chinese")?.addEventListener("change", () => loadAll().catch((e) => showError(e.message || String(e))));
loadAll().catch((e) => showError(e.message || String(e)));
