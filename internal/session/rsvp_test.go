package session

import "testing"

func TestTextsAtUnifiedLevel(t *testing.T) {
	s := &Session{
		Sentences: []string{"Hello world.", "Second."},
		levels: map[int]map[int]string{
			0: {1: "Hi world.", LevelChinese: "你好世界。"},
			1: {2: "Two."},
		},
	}
	orig := s.TextsAtUnifiedLevel(0)
	if len(orig) != 2 || orig[0] != "Hello world." {
		t.Fatalf("level 0: %#v", orig)
	}
	e1 := s.TextsAtUnifiedLevel(1)
	if e1[0] != "Hi world." || e1[1] != "Second." {
		t.Fatalf("level 1: %#v", e1)
	}
	e2 := s.TextsAtUnifiedLevel(2)
	if e2[0] != "Hi world." || e2[1] != "Two." {
		t.Fatalf("level 2 fallback: %#v", e2)
	}
	zh := s.TextsAtUnifiedLevel(LevelChinese)
	if zh[0] != "你好世界。" || zh[1] != "Second." {
		t.Fatalf("chinese: %#v", zh)
	}
}
