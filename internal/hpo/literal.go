package hpo

import (
	"sort"
	"strings"
)

// LiteralCandidates is used only for explicitly requested local fallback.
// It requires an ontology name or synonym actually present in the original text.
func (idx *Index) LiteralCandidates(text string, max int) []CandidateTerm {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if max <= 0 {
		max = 50
	}
	var out []CandidateTerm
	for _, term := range idx.terms {
		if term.Obsolete {
			continue
		}
		names := append([]string{term.NameCN, term.Name}, term.Synonyms...)
		// Local terminology equivalents; ambiguous dizziness is deliberately not mapped to vertigo.
		if term.ID == "HP:0100543" {
			names = append(names, "认知障碍")
		}
		if term.ID == "HP:0002354" {
			names = append(names, "记忆力下降")
		}
		for _, name := range names {
			if len([]rune(name)) >= 2 && strings.Contains(strings.ToLower(text), strings.ToLower(name)) {
				out = append(out, CandidateTerm{Term: term, Score: 25, MatchedWords: []string{name}, Source: "literal_local"})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Term.ID < out[j].Term.ID })
	if len(out) > max {
		out = out[:max]
	}
	return out
}
