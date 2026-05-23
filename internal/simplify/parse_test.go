package simplify

import "testing"

func TestParseSimplified(t *testing.T) {
	raw := `### 1. Simplified Version
> **The cat sat on the mat.**

### 2. Key Changes Made
- **Word/Phrase Swapped**: sat → was sitting (brief).`
	got := ParseSimplified(raw)
	want := "The cat sat on the mat."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
