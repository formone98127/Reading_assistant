package simplify

// Keep prompts short — Gemma 4 on Ollama can return empty output with long system messages.

func BatchPrompt(original string) string {
	return `Simplify for an English learner. Same meaning, keep structure, not longer than the original.

Reply with exactly 3 lines:
LEVEL1: <slightly easier>
LEVEL2: <easier>
LEVEL3: <simplest>

Original: ` + original
}

func SinglePrompt(original string, level int, previous string) string {
	switch level {
	case 1:
		return "Slightly easier for an English learner. Reply with ONLY the new sentence:\n\n" + original
	case 2:
		return "Even easier. Reply with ONLY the new sentence:\n\n" + previous
	default:
		return "Simplest version, same meaning. Reply with ONLY the new sentence:\n\n" + previous
	}
}

// ChinesePrompt asks for a natural Traditional Chinese translation of one English sentence.
func ChinesePrompt(original string) string {
	return `Translate this English sentence into natural Traditional Chinese (繁體中文, used in Taiwan/Hong Kong) for a learner reading along with the English.

Rules:
- Use Traditional characters only (繁體), not Simplified (简体).
- Output ONLY the Chinese translation, one sentence.
- Do not include English, pinyin, or notes.

English: ` + original
}

// System is unused with Gemma 4 (instructions are in the user message).
func System() string { return "" }
