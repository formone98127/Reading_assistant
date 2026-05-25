package library

// PreparedData holds cached English simplify levels (1–3) and optional Chinese per sentence.
type PreparedData struct {
	English map[int]map[int]string
	Chinese map[int]string
}
