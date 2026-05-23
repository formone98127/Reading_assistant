package sentences

import (
	"reflect"
	"testing"
)

func TestSplit_Semicolon(t *testing.T) {
	got := Split("First part; second part; third.")
	want := []string{"First part;", "second part;", "third."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_Titles(t *testing.T) {
	got := Split("Mr. Smith met Mrs. Jones and Dr. Lee. They left.")
	want := []string{"Mr. Smith met Mrs. Jones and Dr. Lee.", "They left."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_TitlesUppercase(t *testing.T) {
	got := Split("MR. AND MRS. DURSLEY were proud of their son.")
	want := []string{"MR. AND MRS. DURSLEY were proud of their son."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_TitlesAnd(t *testing.T) {
	got := Split("Hello Mr. and Mrs. World. Goodbye.")
	want := []string{"Hello Mr. and Mrs. World.", "Goodbye."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_RealSentenceEnd(t *testing.T) {
	got := Split("Hello world. Goodbye.")
	want := []string{"Hello world.", "Goodbye."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_FullwidthPeriod(t *testing.T) {
	// U+FF0E fullwidth full stop (common in PDF copy-paste)
	got := Split("Mr\uFF0E Smith met Mrs\uFF0E Jones. Done.")
	want := []string{"Mr. Smith met Mrs. Jones.", "Done."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSplit_ZeroWidthBeforePeriod(t *testing.T) {
	got := Split("Mr\u200b. Smith went home.")
	want := []string{"Mr. Smith went home."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
