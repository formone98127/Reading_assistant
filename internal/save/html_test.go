package save

import (
	"bytes"
	"strings"
	"testing"

	"reading-assistant/internal/session"
)

func TestEffectiveExportReadingMode(t *testing.T) {
	if got := EffectiveExportReadingMode(session.ModeEnglish, []ReaderSentence{
		{Original: "Hi", Chinese: "你好"},
	}); got != session.ModeEnglishChinese {
		t.Fatalf("got %q want english_chinese", got)
	}
	if got := EffectiveExportReadingMode(session.ModeEnglish, []ReaderSentence{
		{Original: "Hi"},
	}); got != session.ModeEnglish {
		t.Fatalf("got %q want english", got)
	}
}

func TestBuildBookHTML_Base64NotScriptEscaped(t *testing.T) {
	export := ReaderExport{
		Title:       "test",
		MaxLevel:    3,
		ReadingMode: session.ModeEnglishChinese,
		Sentences: []ReaderSentence{
			{
				Original: "Cost is $5 + tax / shipping.",
				Levels:   []string{"a/b+c", "x<y", "z"},
				Chinese:  "测试</script>",
			},
		},
	}
	data, err := BuildBookHTML(export)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if strings.Contains(html, `\/`) || strings.Contains(html, `\x3C`) {
		t.Fatalf("base64 was script-escaped in HTML export:\n%s", html)
	}
	if !bytes.Contains(data, []byte(`id="book-b64"`)) {
		t.Fatal("missing book-b64 textarea")
	}
	if !bytes.Contains(data, []byte(`english_chinese`)) {
		t.Fatal("export should embed english_chinese reading mode when 中文 present")
	}
	if bytes.Contains(data, []byte(`<script type="text/plain" id="book-b64"`)) {
		t.Fatal("still using script tag for book data")
	}
}
