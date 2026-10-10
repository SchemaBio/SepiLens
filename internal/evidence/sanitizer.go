package evidence

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	reHTMLTags   = regexp.MustCompile(`(?i)</?[a-zA-Z][a-zA-Z0-9]*(?:\s+[^>]*)?>`)
	reInjections = []*regexp.Regexp{
		regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior)\s+(instructions|prompts)`),
		regexp.MustCompile(`(?i)system\s*prompt\s*:`),
		regexp.MustCompile(`(?i)<\s*/?\s*system\s*>`),
		regexp.MustCompile(`(?i)\[\s*system\s*\]`),
		regexp.MustCompile(`(?i)you\s+are\s+now\s+(in\s+)?(admin|root|jailbreak)\s+mode`),
		regexp.MustCompile(`(?i)override\s+(clinical\s+)?verdict`),
		regexp.MustCompile(`(?i)classify\s+as\s+(benign|pathogenic)\s+regardless`),
	}
)

type Sanitizer struct {
	MaxRunes int
}

func NewSanitizer(maxRunes int) *Sanitizer {
	if maxRunes <= 0 {
		maxRunes = 4000
	}
	return &Sanitizer{MaxRunes: maxRunes}
}

func (s *Sanitizer) SanitizeText(input string) string {
	clean := html.UnescapeString(input)

	for _, pattern := range reInjections {
		clean = pattern.ReplaceAllString(clean, "[REDACTED_PROMPT_INJECTION]")
	}

	clean = reHTMLTags.ReplaceAllString(clean, " ")

	// Remove unprintable control characters except newline and tab
	var sb strings.Builder
	for _, r := range clean {
		if r == '\n' || r == '\t' || r == '\r' || r >= 32 {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}
	res := strings.TrimSpace(sb.String())

	if utf8.RuneCountInString(res) > s.MaxRunes {
		runes := []rune(res)
		res = string(runes[:s.MaxRunes]) + "... [TRUNCATED]"
	}

	return res
}
