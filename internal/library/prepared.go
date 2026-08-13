package library

// PreparedData holds per-sentence variants. On disk: english.json (levels 1–3), chinese.json (level 4).
// In session memory these merge into one map: 1–3 easier, 4 = 繁體中文.
type PreparedData struct {
	English map[int]map[int]string
	Chinese map[int]string
}
