package hpo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

var (
	rePunct = regexp.MustCompile(`[\s、,;:.，。；：\-_/\\()（）]+`)
	reHan   = regexp.MustCompile(`[\p{Han}]`)
)

type Index struct {
	mu             sync.RWMutex
	terms          map[string]Term
	byID           map[string]Term
	byNamePrefix   map[string][]string
	byNameCnPrefix map[string][]string
	byKeyword      map[string][]string
	synonyms       *SynonymMatcher
}

func LoadIndex(termsPath, namesPath string) (*Index, error) {
	termsData, err := os.ReadFile(termsPath)
	if err != nil {
		return nil, fmt.Errorf("read hpo terms json: %w", err)
	}

	var rawMap map[string]Term
	if err := json.Unmarshal(termsData, &rawMap); err != nil {
		return nil, fmt.Errorf("unmarshal hpo terms: %w", err)
	}

	idx := &Index{
		terms:          rawMap,
		byID:           make(map[string]Term, len(rawMap)),
		byNamePrefix:   make(map[string][]string, len(rawMap)*4),
		byNameCnPrefix: make(map[string][]string, len(rawMap)*4),
		byKeyword:      make(map[string][]string, len(rawMap)*6),
	}

	for id, t := range rawMap {
		normID := strings.ToLower(id)
		idx.byID[normID] = t

		idx.addPrefixIndex(t.Name, id, idx.byNamePrefix)
		idx.addPrefixIndex(t.NameCN, id, idx.byNameCnPrefix)

		idx.addKeywordIndex(t.Name, id)
		idx.addKeywordIndex(t.NameCN, id)
		idx.addKeywordIndex(t.Definition, id)
		idx.addKeywordIndex(t.DefinitionCN, id)
	}

	if namesPath != "" {
		if _, err := os.Stat(namesPath); err == nil {
			sm, err := NewSynonymMatcher(namesPath, rawMap)
			if err == nil {
				idx.synonyms = sm
			}
		}
	}

	return idx, nil
}

func LoadDefaultIndex(knowledgeDir string) (*Index, error) {
	termsPath := filepath.Join(knowledgeDir, "hpo_terms_cn.json")
	namesPath := filepath.Join(knowledgeDir, "hpo_names.txt")
	return LoadIndex(termsPath, namesPath)
}

func (idx *Index) TermCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.terms)
}

func (idx *Index) GetTerm(id string) (Term, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	t, ok := idx.byID[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

func (idx *Index) addPrefixIndex(text, id string, dest map[string][]string) {
	if text == "" {
		return
	}
	words := rePunct.Split(strings.ToLower(strings.TrimSpace(text)), -1)
	for _, w := range words {
		runes := []rune(w)
		for i := 2; i <= len(runes) && i <= 10; i++ {
			pref := string(runes[:i])
			dest[pref] = append(dest[pref], id)
		}
	}
}

func (idx *Index) addKeywordIndex(text, id string) {
	if text == "" {
		return
	}
	words := rePunct.Split(strings.ToLower(text), -1)
	for _, w := range words {
		if len([]rune(w)) > 1 {
			idx.byKeyword[w] = append(idx.byKeyword[w], id)
		}
	}
}

type scoredID struct {
	id    string
	score float64
}

func (idx *Index) Search(query string, maxResults int) []CandidateTerm {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	normQuery := strings.ToLower(strings.TrimSpace(query))
	if normQuery == "" {
		return nil
	}

	// 1. Direct HPO ID match
	if strings.HasPrefix(normQuery, "hp:") {
		if t, ok := idx.byID[normQuery]; ok {
			return []CandidateTerm{
				{
					Term:         t,
					Score:        100.0,
					MatchedWords: []string{normQuery},
					Source:       "exact_id",
				},
			}
		}
	}

	scores := make(map[string]float64)
	matchedWords := make(map[string][]string)
	queryWords := rePunct.Split(normQuery, -1)

	for _, w := range queryWords {
		runes := []rune(w)
		if len(runes) < 2 {
			continue
		}

		// Prefix matches (+5)
		prefLen := len(runes)
		if prefLen > 10 {
			prefLen = 10
		}
		prefix := string(runes[:prefLen])

		for _, id := range idx.byNamePrefix[prefix] {
			scores[id] += 5.0
			matchedWords[id] = append(matchedWords[id], w)
		}
		for _, id := range idx.byNameCnPrefix[prefix] {
			scores[id] += 5.0
			matchedWords[id] = append(matchedWords[id], w)
		}

		// Keyword matches (+2)
		for _, id := range idx.byKeyword[w] {
			scores[id] += 2.0
			matchedWords[id] = append(matchedWords[id], w)
		}

		// Chinese substring matches (2..4 characters, +1)
		if reHan.MatchString(w) && len(runes) > 2 {
			maxSub := len(runes)
			if maxSub > 4 {
				maxSub = 4
			}
			for subLen := 2; subLen <= maxSub; subLen++ {
				for i := 0; i <= len(runes)-subLen; i++ {
					sub := string(runes[i : i+subLen])
					for _, id := range idx.byNameCnPrefix[sub] {
						scores[id] += 1.0
					}
					for _, id := range idx.byKeyword[sub] {
						scores[id] += 1.0
					}
				}
			}
		}
	}

	// Exact Name bonus (+20 for exact name, +10 for exact Chinese name)
	for id := range scores {
		t := idx.terms[id]
		if strings.ToLower(t.Name) == normQuery {
			scores[id] += 25.0
		} else if strings.ToLower(t.NameCN) == normQuery {
			scores[id] += 20.0
		}
	}

	list := make([]scoredID, 0, len(scores))
	for id, sc := range scores {
		list = append(list, scoredID{id: id, score: sc})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].id < list[j].id
	})

	if len(list) > maxResults {
		list = list[:maxResults]
	}

	out := make([]CandidateTerm, 0, len(list))
	for _, item := range list {
		out = append(out, CandidateTerm{
			Term:         idx.terms[item.id],
			Score:        item.score,
			MatchedWords: deduplicateStrings(matchedWords[item.id]),
			Source:       "index_search",
		})
	}
	return out
}

func (idx *Index) SearchCandidates(symptomWords []string, maxCandidates int) []CandidateTerm {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	termScoreMap := make(map[string]float64)
	termWordsMap := make(map[string][]string)

	for _, word := range symptomWords {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		matches := idx.Search(word, 20)
		for _, m := range matches {
			termScoreMap[m.Term.ID] += m.Score
			termWordsMap[m.Term.ID] = append(termWordsMap[m.Term.ID], word)
		}
	}

	// Synonym expansion
	if idx.synonyms != nil {
		synonymIDs := idx.synonyms.ExpandSynonyms(symptomWords, 8)
		for _, synID := range synonymIDs {
			if _, exists := termScoreMap[synID]; !exists {
				if t, ok := idx.terms[synID]; ok {
					termScoreMap[t.ID] = 0.5
					termWordsMap[t.ID] = append(termWordsMap[t.ID], "synonym")
				}
			}
		}
	}

	list := make([]scoredID, 0, len(termScoreMap))
	for id, sc := range termScoreMap {
		list = append(list, scoredID{id: id, score: sc})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].id < list[j].id
	})

	if len(list) > maxCandidates {
		list = list[:maxCandidates]
	}

	candidates := make([]CandidateTerm, 0, len(list))
	for _, item := range list {
		candidates = append(candidates, CandidateTerm{
			Term:         idx.terms[item.id],
			Score:        item.score,
			MatchedWords: deduplicateStrings(termWordsMap[item.id]),
			Source:       "candidate_pool",
		})
	}
	return candidates
}

func deduplicateStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] && s != "" {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func IsPunctOrSpace(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSpace(r)
}
