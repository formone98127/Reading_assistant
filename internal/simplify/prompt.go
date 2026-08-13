package simplify

// Keep prompts short — Gemma 4 on Ollama can return empty output with long system messages.

func BatchPrompt(original string) string {
	return `Rewrite for an English learner. Keep ALL original vocabulary. Only rephrase sentence structure. Add inline glosses (word: definition) after hard words.

Reply with exactly 3 lines:
LEVEL1: <slightly restructured>
LEVEL2: <more restructured, more glosses>
LEVEL3: <most restructured, hardest words glossed>

Original: ` + original
}

func SinglePrompt(original string, level int, previous string) string {
	switch level {
	case 1:
		return "Rewrite for an English learner. Keep ALL original vocabulary. Only rephrase structure. Add inline glosses after hard words.\n\n" + original
	case 2:
		return "Further simplify structure. Keep ALL original vocabulary. More inline glosses.\n\n" + previous
	default:
		return "Simplify structure to clearest form. Keep ALL original vocabulary. Every hard word glossed inline.\n\n" + previous
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
