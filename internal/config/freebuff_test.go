package config

import "testing"

func TestIsRootFreebuffModel(t *testing.T) {
	if !IsRootFreebuffModel("minimax/minimax-m2.7") {
		t.Fatal("minimax")
	}
	if !IsRootFreebuffModel("minimax/minimax-m2.7-20260211") {
		t.Fatal("dated minimax")
	}
	if IsRootFreebuffModel("z-ai/glm-5.1") {
		t.Fatal("glm should not be root")
	}
	if IsRootFreebuffModel("google/gemini-2.5-flash-lite") {
		t.Fatal("gemini is subagent only")
	}
}

func TestFilterFreebuffModels(t *testing.T) {
	proxy := []string{
		"google/gemini-2.5-flash-lite",
		"minimax/minimax-m2.7",
		"google/gemini-3.1-flash-lite-preview",
	}
	got := FilterFreebuffModels(proxy)
	if len(got) != 1 || got[0] != "minimax/minimax-m2.7" {
		t.Fatalf("got %v", got)
	}
	if len(FilterFreebuffModels([]string{"google/gemini-2.5-flash-lite"})) != 0 {
		t.Fatal("gemini-only should be empty")
	}
}

func TestMatchRootModelOnProxy(t *testing.T) {
	proxy := []string{"minimax/minimax-m2.7-20260211"}
	m, ok := MatchRootModelOnProxy("minimax/minimax-m2.7", proxy)
	if !ok || m != "minimax/minimax-m2.7-20260211" {
		t.Fatalf("got %q %v", m, ok)
	}
}
