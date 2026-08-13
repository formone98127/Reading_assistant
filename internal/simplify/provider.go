package simplify

// Backend selects which local API rewrites use.
type Backend string

const (
	BackendOllama   Backend = "ollama"
	BackendFreebuff Backend = "freebuff"
)

func NormalizeBackend(b string) Backend {
	switch Backend(b) {
	case BackendFreebuff:
		return BackendFreebuff
	default:
		return BackendOllama
	}
}
