package simplify

import "testing"

func TestParseLevelsNumbered(t *testing.T) {
	raw := `1. The cat sat on the mat.
2. The cat was on the mat.
3. A cat was on a mat.`
	levels := LevelsFromResponse(raw, "original")
	if len(levels) != 3 {
		t.Fatalf("got %d: %v", len(levels), levels)
	}
}

func TestParseLevels(t *testing.T) {
	raw := `LEVEL1: The cat sat on the mat.
LEVEL2: The cat was on the mat.
LEVEL3: A cat was on a mat.`
	levels := LevelsFromResponse(raw, "The feline reclined upon the woven floor covering.")
	if len(levels) != 3 {
		t.Fatalf("got %d levels: %v", len(levels), levels)
	}
	if levels[0] != "The cat sat on the mat." {
		t.Fatalf("level1: %q", levels[0])
	}
}
