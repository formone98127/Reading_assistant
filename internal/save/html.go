package save

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"reading-assistant/internal/session"
)

// EffectiveExportReadingMode enables EN+中文 in the export when 中文 is present on disk
// even if meta.readingMode was left as english.
func EffectiveExportReadingMode(mode string, sentences []ReaderSentence) string {
	mode = session.NormalizeReadingMode(mode)
	if session.ChineseEnabled(mode) {
		return mode
	}
	for _, s := range sentences {
		if strings.TrimSpace(s.Chinese) != "" {
			return session.ModeEnglishChinese
		}
	}
	return mode
}

// SafeFilename returns a name safe for download paths.
func SafeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "book"
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			continue
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "book"
	}
	return s
}

// ReaderSentence is one sentence with optional simplify levels (index 0 = level 1).
type ReaderSentence struct {
	Original string   `json:"original"`
	Levels   []string `json:"levels"`
	Chinese  string   `json:"chinese,omitempty"`
}

// ReaderExport is embedded in exported HTML for offline arrow-key reading.
type ReaderExport struct {
	Title       string           `json:"title"`
	Note        string           `json:"note,omitempty"`
	StartIndex  int              `json:"startIndex"`
	StartLevel  int              `json:"startLevel"`
	MaxLevel    int              `json:"maxLevel"`
	ReadingMode string           `json:"readingMode,omitempty"`
	Sentences   []ReaderSentence `json:"sentences"`
}

// BuildReaderSentences maps library/session data into export sentences.
func BuildReaderSentences(sentences []string, english map[int]map[int]string, chinese map[int]string, maxLevel int) []ReaderSentence {
	if maxLevel < 1 {
		maxLevel = 3
	}
	out := make([]ReaderSentence, len(sentences))
	for i, orig := range sentences {
		rs := ReaderSentence{
			Original: orig,
			Levels:   make([]string, maxLevel),
		}
		if levels, ok := english[i]; ok {
			for lv := 1; lv <= maxLevel; lv++ {
				if t, ok := levels[lv]; ok && strings.TrimSpace(t) != "" {
					rs.Levels[lv-1] = t
				}
			}
		}
		if chinese != nil {
			if t, ok := chinese[i]; ok && strings.TrimSpace(t) != "" {
				rs.Chinese = t
			}
		}
		out[i] = rs
	}
	return out
}

type readerHTMLPage struct {
	Title    string
	BookData template.HTML // base64 JSON (A–Z0–9+/=); must not pass through script-context escaping
}

var readerHTMLTemplate = template.Must(template.New("reader").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="dark">
<title>{{.Title}}</title>
<style>
  :root {
    --reader-font-size: 22px;
    font-family: Georgia, "Times New Roman", serif;
  }
  * { box-sizing: border-box; }
  html {
    height: 100%;
    height: -webkit-fill-available;
  }
  body {
    margin: 0;
    min-height: 100%;
    min-height: 100dvh;
    min-height: -webkit-fill-available;
    background: #000;
    color: #fff;
    overflow: hidden;
    -webkit-text-size-adjust: 100%;
  }
  .top-bar {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    z-index: 10;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    padding: max(0.75rem, env(safe-area-inset-top)) max(1rem, env(safe-area-inset-right))
      0.5rem max(1rem, env(safe-area-inset-left));
    pointer-events: none;
  }
  .book-title {
    margin: 0;
    font-size: 0.8rem;
    font-weight: 500;
    font-family: system-ui, sans-serif;
    color: #888;
    line-height: 1.3;
    max-width: 70%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    pointer-events: auto;
  }
  .font-controls {
    display: flex;
    gap: 0.35rem;
    flex-shrink: 0;
    pointer-events: auto;
  }
  .font-btn {
    min-width: 2.25rem;
    height: 2.25rem;
    padding: 0;
    border: 1px solid #444;
    border-radius: 6px;
    background: #000;
    color: #fff;
    font-size: 1.15rem;
    line-height: 1;
    cursor: pointer;
    touch-action: manipulation;
    -webkit-tap-highlight-color: transparent;
  }
  .font-btn:active {
    background: #1a1a1a;
  }
  #reader {
    position: fixed;
    inset: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 3.25rem 1.25rem 2.75rem;
    touch-action: none;
    -webkit-touch-callout: none;
    -webkit-user-select: none;
    user-select: none;
    -webkit-transform: translateZ(0);
    transform: translateZ(0);
    perspective: 900px;
    outline: none;
  }
  .reader-stack {
    width: 100%;
    max-width: 40rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.85rem;
    transform-origin: center center;
    will-change: transform, opacity;
  }
  #sentence {
    margin: 0;
    width: 100%;
    font-size: var(--reader-font-size);
    line-height: 1.65;
    text-align: center;
    color: #fff;
    overflow-wrap: anywhere;
    word-break: break-word;
  }
  #chinese-panel {
    margin: 0;
    width: 100%;
    padding-top: 0.65rem;
    border-top: 1px solid #333;
    font-size: calc(var(--reader-font-size) * 0.92);
    line-height: 1.6;
    text-align: center;
    color: #bbb;
    overflow-wrap: anywhere;
    word-break: break-word;
    font-family: "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif;
  }
  #chinese-panel.hidden { display: none; }
  #easier-line {
    margin: 0;
    width: 100%;
    font-size: calc(var(--reader-font-size) * 0.88);
    line-height: 1.5;
    text-align: center;
    color: #888;
  }
  #easier-line.hidden { display: none; }
  @keyframes turn-out-next {
    to {
      opacity: 0;
      transform: translateY(-1.25rem) rotateX(10deg);
    }
  }
  @keyframes turn-in-next {
    from {
      opacity: 0;
      transform: translateY(1.25rem) rotateX(-10deg);
    }
    to {
      opacity: 1;
      transform: translateY(0) rotateX(0);
    }
  }
  @keyframes turn-out-prev {
    to {
      opacity: 0;
      transform: translateY(1.25rem) rotateX(-10deg);
    }
  }
  @keyframes turn-in-prev {
    from {
      opacity: 0;
      transform: translateY(-1.25rem) rotateX(10deg);
    }
    to {
      opacity: 1;
      transform: translateY(0) rotateX(0);
    }
  }
  .reader-stack.turn-out-next { animation: turn-out-next 0.22s ease-in forwards; }
  .reader-stack.turn-in-next { animation: turn-in-next 0.26s ease-out forwards; }
  .reader-stack.turn-out-prev { animation: turn-out-prev 0.22s ease-in forwards; }
  .reader-stack.turn-in-prev { animation: turn-in-prev 0.26s ease-out forwards; }
  @media (prefers-reduced-motion: reduce) {
    .reader-stack.turn-out-next,
    .reader-stack.turn-in-next,
    .reader-stack.turn-out-prev,
    .reader-stack.turn-in-prev {
      animation: none !important;
    }
  }
  .page-num,
  .level-badge {
    position: fixed;
    bottom: max(0.75rem, env(safe-area-inset-bottom));
    z-index: 10;
    font-family: system-ui, sans-serif;
    font-size: 0.7rem;
    font-weight: 500;
    letter-spacing: 0.02em;
    pointer-events: none;
  }
  .page-num {
    right: max(1rem, env(safe-area-inset-right));
    color: #666;
  }
  .level-badge {
    left: max(1rem, env(safe-area-inset-left));
    color: #888;
  }
  .keys-hint {
    position: fixed;
    left: 0;
    right: 0;
    bottom: max(2.5rem, env(safe-area-inset-bottom));
    z-index: 10;
    margin: 0;
    text-align: center;
    font-family: system-ui, sans-serif;
    font-size: 0.68rem;
    color: #555;
    pointer-events: none;
  }
</style>
</head>
<body>
  <header class="top-bar">
    <h1 class="book-title">{{.Title}}</h1>
    <div class="font-controls">
      <button type="button" class="font-btn" id="font-smaller" aria-label="Smaller text">−</button>
      <button type="button" class="font-btn" id="font-larger" aria-label="Larger text">+</button>
    </div>
  </header>
  <main id="reader" tabindex="0" aria-live="polite">
    <div class="reader-stack" id="reader-stack">
      <p id="sentence"></p>
      <p id="chinese-panel" class="hidden" lang="zh-Hant"></p>
      <p id="easier-line" class="easier-line hidden"></p>
    </div>
  </main>
  <div class="level-badge" id="level-badge">Original</div>
  <div class="page-num" id="progress">1 / 1</div>
  <p id="keys-hint" class="keys-hint hidden"></p>
  <textarea id="book-b64" hidden readonly aria-hidden="true">{{.BookData}}</textarea>
<script>
(function () {
  function decodeB64Utf8(b64) {
    let clean = b64.replace(/\s/g, "");
    clean = clean.replace(/-/g, "+").replace(/_/g, "/");
    const pad = clean.length % 4;
    if (pad === 2) clean += "==";
    else if (pad === 3) clean += "=";
    else if (pad === 1) throw new Error("invalid base64 length");
    const bin = atob(clean);
    if (typeof TextDecoder !== "undefined") {
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      return new TextDecoder("utf-8").decode(bytes);
    }
    return decodeURIComponent(escape(bin));
  }

  let book;
  try {
    const b64el = document.getElementById("book-b64");
    const raw = b64el ? (b64el.value || b64el.textContent || "") : "";
    if (!raw.trim()) throw new Error("missing data");
    book = JSON.parse(decodeB64Utf8(raw));
    if (!book.sentences || !book.sentences.length) throw new Error("empty book");
  } catch (err) {
    document.body.innerHTML =
      '<p style="color:#f88;font-family:system-ui;padding:1.5rem">Could not open book data.' +
      (err && err.message ? " (" + err.message + ")" : "") + "</p>";
    return;
  }
  const maxLevel = book.maxLevel || 3;
  const showChinese = book.readingMode === "english_chinese";
  let index = Math.min(Math.max(0, book.startIndex || 0), book.sentences.length - 1);
  let level = showChinese ? 0 : Math.min(Math.max(0, book.startLevel || 0), maxLevel);
  let chineseVisible = showChinese ? false : true;

  const $ = (id) => document.getElementById(id);
  const keysHint = $("keys-hint");
  if (keysHint) {
    keysHint.textContent = showChinese
      ? "↓ prev · ↑ next · → English + 中文 · ← original only"
      : "↓ prev · ↑ next · → simpler · ← harder";
    keysHint.classList.remove("hidden");
  }
  const FONT_MIN = 16, FONT_MAX = 180, FONT_DEFAULT = 22;
  let fontSize = FONT_DEFAULT;
  try {
    const saved = parseInt(localStorage.getItem("readerExportFontSize"), 10);
    if (!Number.isNaN(saved)) fontSize = saved;
  } catch (_) {}

  function applyFontSize(px) {
    fontSize = Math.min(FONT_MAX, Math.max(FONT_MIN, px));
    document.documentElement.style.setProperty("--reader-font-size", fontSize + "px");
    try { localStorage.setItem("readerExportFontSize", String(fontSize)); } catch (_) {}
  }

  function levelAvailable(idx, lv) {
    if (lv === 0) return true;
    const s = book.sentences[idx];
    if (!s || !s.levels) return false;
    const t = s.levels[lv - 1];
    return t && String(t).trim();
  }

  function textAt(idx, lv) {
    const s = book.sentences[idx];
    if (!s) return "";
    if (lv === 0) return s.original || "";
    const t = s.levels && s.levels[lv - 1];
    return (t && String(t).trim()) ? t : (s.original || "");
  }

  function canSimplify() {
    return level < maxLevel && levelAvailable(index, level + 1);
  }

  while (level > 0 && !levelAvailable(index, level)) level--;

  let animating = false;
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches || isIOS;

  function levelLabel() {
    if (showChinese) return chineseVisible ? "Original + 中文" : "Original";
    return level === 0 ? "Original" : "Easier · level " + level + "/" + maxLevel;
  }

  function chineseAt(idx) {
    const s = book.sentences[idx];
    if (!s || !s.chinese) return "";
    return String(s.chinese).trim();
  }

  function render() {
    const total = book.sentences.length;
    $("progress").textContent = (index + 1) + " / " + total;
    $("level-badge").textContent = levelLabel();
    const zh = $("chinese-panel");
    const easier = $("easier-line");
    if (showChinese) {
      $("sentence").textContent = textAt(index, 0);
      const t = chineseVisible ? chineseAt(index) : "";
      if (t && zh) {
        zh.textContent = t;
        zh.classList.remove("hidden");
      } else if (zh) {
        zh.textContent = chineseAt(index) ? "（按 → 顯示中文）" : "";
        zh.classList.toggle("hidden", !zh.textContent);
      }
      if (easier) easier.classList.add("hidden");
    } else {
      $("sentence").textContent = textAt(index, level);
      if (zh) {
        zh.textContent = "";
        zh.classList.add("hidden");
      }
      if (easier) easier.classList.add("hidden");
    }
  }

  function turnPage(dir) {
    if (dir > 0 && index + 1 >= book.sentences.length) return;
    if (dir < 0 && index <= 0) return;
    if (animating) return;

    if (reduceMotion) {
      if (dir > 0) index++;
      else index--;
      level = 0;
      if (showChinese) chineseVisible = false;
      render();
      return;
    }

    const el = $("reader-stack");
    animating = true;
    const outClass = dir > 0 ? "turn-out-next" : "turn-out-prev";
    const inClass = dir > 0 ? "turn-in-next" : "turn-in-prev";

    function finishTurn() {
      el.classList.remove(outClass, inClass);
      animating = false;
    }

    function afterOut() {
      el.classList.remove(outClass);
      if (dir > 0) index++;
      else index--;
      level = 0;
      if (showChinese) chineseVisible = false;
      render();
      el.classList.add(inClass);
      const done = setTimeout(finishTurn, 280);
      const onIn = function (ev) {
        if (ev && ev.target !== el) return;
        clearTimeout(done);
        el.removeEventListener("animationend", onIn);
        el.removeEventListener("webkitAnimationEnd", onIn);
        finishTurn();
      };
      el.addEventListener("animationend", onIn);
      el.addEventListener("webkitAnimationEnd", onIn);
    }

    el.classList.add(outClass);
    const outFallback = setTimeout(afterOut, 260);
    const onOut = function (ev) {
      if (ev && ev.target !== el) return;
      clearTimeout(outFallback);
      el.removeEventListener("animationend", onOut);
      el.removeEventListener("webkitAnimationEnd", onOut);
      afterOut();
    };
    el.addEventListener("animationend", onOut);
    el.addEventListener("webkitAnimationEnd", onOut);
  }

  function prev() {
    turnPage(-1);
  }

  function next() {
    turnPage(1);
  }

  function showZh() {
    if (!showChinese) return;
    chineseVisible = true;
    render();
  }

  function hideZh() {
    if (!showChinese) return;
    chineseVisible = false;
    render();
  }

  function simpler() {
    if (showChinese) {
      showZh();
      return;
    }
    if (!canSimplify()) return;
    level++;
    render();
  }

  function harder() {
    if (showChinese) {
      hideZh();
      return;
    }
    if (level <= 0) return;
    level--;
    render();
  }

  const SWIPE_MIN_PX = 40;
  const SWIPE_LOCK_PX = 14;
  const SWIPE_MAX_MS = 800;

  function applySwipe(dx, dy, axis) {
    const adx = Math.abs(dx);
    const ady = Math.abs(dy);
    if (Math.max(adx, ady) < SWIPE_MIN_PX) return;
    if (axis === "x" || (!axis && adx >= ady)) {
      if (showChinese) {
        if (dx > 0) showZh();
        else hideZh();
      } else {
        if (dx > 0) harder();
        else simpler();
      }
    } else {
      if (dy > 0) prev();
      else next();
    }
  }

  function onTouchStart(e) {
    if (e.touches.length !== 1) return;
    if (e.target.closest(".font-btn")) return;
    const t = e.touches[0];
    const start = { x: t.clientX, y: t.clientY, time: Date.now() };
    let axis = null;

    function onMove(ev) {
      if (ev.touches.length !== 1) return;
      const m = ev.touches[0];
      const dx = m.clientX - start.x;
      const dy = m.clientY - start.y;
      if (!axis) {
        if (Math.abs(dx) < SWIPE_LOCK_PX && Math.abs(dy) < SWIPE_LOCK_PX) return;
        axis = Math.abs(dx) >= Math.abs(dy) ? "x" : "y";
      }
      ev.preventDefault();
    }

    function onEnd(ev) {
      swipeEl.removeEventListener("touchmove", onMove);
      swipeEl.removeEventListener("touchend", onEnd);
      swipeEl.removeEventListener("touchcancel", onEnd);
      const pt = (ev.changedTouches && ev.changedTouches[0]) || t;
      if (Date.now() - start.time <= SWIPE_MAX_MS) {
        applySwipe(pt.clientX - start.x, pt.clientY - start.y, axis);
      }
      if (ev.cancelable) ev.preventDefault();
    }

    swipeEl.addEventListener("touchmove", onMove, { passive: false });
    swipeEl.addEventListener("touchend", onEnd, { passive: false });
    swipeEl.addEventListener("touchcancel", onEnd, { passive: false });
  }

  function onKey(e) {
    if (e.target.matches("input, textarea, select")) return;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        prev();
        break;
      case "ArrowUp":
        e.preventDefault();
        next();
        break;
      case "ArrowLeft":
        e.preventDefault();
        harder();
        break;
      case "ArrowRight":
        e.preventDefault();
        simpler();
        break;
    }
  }

  const swipeEl = $("reader");
  swipeEl.addEventListener("touchstart", onTouchStart, { passive: true });
  window.addEventListener("keydown", onKey);

  $("font-smaller").addEventListener("click", () => applyFontSize(fontSize - 2));
  $("font-larger").addEventListener("click", () => applyFontSize(fontSize + 2));
  applyFontSize(fontSize);
  render();
  swipeEl.focus();
})();
</script>
</body>
</html>`))

// BuildBookHTML returns a standalone interactive HTML reader (arrow-key navigation).
func BuildBookHTML(export ReaderExport) ([]byte, error) {
	if strings.TrimSpace(export.Title) == "" {
		export.Title = "Book"
	}
	if export.MaxLevel < 1 {
		export.MaxLevel = 3
	}
	if len(export.Sentences) == 0 {
		return nil, fmt.Errorf("html: no sentences")
	}
	if export.StartIndex < 0 || export.StartIndex >= len(export.Sentences) {
		export.StartIndex = 0
	}
	if export.StartLevel < 0 || export.StartLevel > export.MaxLevel {
		export.StartLevel = 0
	}
	export.ReadingMode = EffectiveExportReadingMode(export.ReadingMode, export.Sentences)
	raw, err := json.Marshal(export)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString(raw)
	var buf bytes.Buffer
	// template.HTML: html/template must not apply script-context escaping to base64
	// (would turn "/" into "\/" and break atob).
	if err := readerHTMLTemplate.Execute(&buf, readerHTMLPage{
		Title:    export.Title,
		BookData: template.HTML(b64),
	}); err != nil {
		return nil, fmt.Errorf("html: %w", err)
	}
	return buf.Bytes(), nil
}
