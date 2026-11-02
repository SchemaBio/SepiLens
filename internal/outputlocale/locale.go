package outputlocale

func Valid(locale string) bool {
	switch locale {
	case "", "zh-CN", "zh-Hant", "en", "es-419":
		return true
	}
	return false
}

func Instruction(locale string) string {
	language := "Simplified Chinese"
	switch locale {
	case "en":
		language = "English"
	case "zh-Hant":
		language = "Traditional Chinese"
	case "es-419":
		language = "Latin American Spanish"
	}
	return "\nOUTPUT LANGUAGE: Write all generated explanatory prose, summaries, rationales, reasons and warnings in " + language + ". This overrides earlier language instructions. Keep JSON keys, enums, criterion codes, identifiers, numeric values and verbatim source quotations unchanged. HPO original_symptom and source spans must remain verbatim from the input. Never translate or reinterpret clinical facts to satisfy the language requirement."
}
