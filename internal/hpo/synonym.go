package hpo

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

type SynonymMatcher struct {
	namesLines         []string
	namesTrigramIndex  map[string][]int
	nameToHpoID        map[string]string
}

func NewSynonymMatcher(namesPath string, hpoTerms map[string]Term) (*SynonymMatcher, error) {
	file, err := os.Open(namesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	trigramIndex := make(map[string][]int, len(lines)*3)
	for idx, line := range lines {
		runes := []rune(strings.ToLower(line))
		if len(runes) < 3 {
			continue
		}
		seenTrigrams := make(map[string]bool)
		for i := 0; i <= len(runes)-3; i++ {
			tg := string(runes[i : i+3])
			if !seenTrigrams[tg] {
				seenTrigrams[tg] = true
				trigramIndex[tg] = append(trigramIndex[tg], idx)
			}
		}
	}

	nameToID := make(map[string]string, len(hpoTerms)*2)
	for id, t := range hpoTerms {
		en := strings.ToLower(strings.TrimSpace(t.Name))
		cn := strings.ToLower(strings.TrimSpace(t.NameCN))
		if en != "" {
			nameToID[en] = id
		}
		if cn != "" {
			nameToID[cn] = id
		}
	}

	return &SynonymMatcher{
		namesLines:        lines,
		namesTrigramIndex: trigramIndex,
		nameToHpoID:       nameToID,
	}, nil
}

func extractRuneNgrams(s string, n int) []string {
	runes := []rune(s)
	if len(runes) < n {
		return nil
	}
	res := make([]string, 0, len(runes)-n+1)
	for i := 0; i <= len(runes)-n; i++ {
		res = append(res, string(runes[i:i+n]))
	}
	return res
}

func scoreRuneOverlap(query, candidate string) float64 {
	qNgrams := extractRuneNgrams(strings.ToLower(query), 2)
	cNgrams := extractRuneNgrams(strings.ToLower(candidate), 2)
	if len(qNgrams) == 0 || len(cNgrams) == 0 {
		return 0
	}

	qSet := make(map[string]bool, len(qNgrams))
	for _, ng := range qNgrams {
		qSet[ng] = true
	}

	intersection := 0
	for _, ng := range cNgrams {
		if qSet[ng] {
			intersection++
		}
	}

	union := len(qSet) + len(cNgrams) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

type lineScored struct {
	lineIdx int
	score   float64
}

func (sm *SynonymMatcher) SearchSynonyms(symptomWord string, topK int) []string {
	runes := []rune(strings.ToLower(symptomWord))
	if len(runes) < 2 {
		return nil
	}

	lineHits := make(map[int]int)
	if len(runes) >= 3 {
		for i := 0; i <= len(runes)-3; i++ {
			tg := string(runes[i : i+3])
			matches := sm.namesTrigramIndex[tg]
			for _, lineIdx := range matches {
				lineHits[lineIdx]++
			}
		}
	}

	if len(lineHits) == 0 {
		return nil
	}

	type hitEntry struct {
		lineIdx int
		hits    int
	}
	hitsList := make([]hitEntry, 0, len(lineHits))
	for idx, count := range lineHits {
		hitsList = append(hitsList, hitEntry{lineIdx: idx, hits: count})
	}
	sort.Slice(hitsList, func(i, j int) bool {
		return hitsList[i].hits > hitsList[j].hits
	})

	if len(hitsList) > 100 {
		hitsList = hitsList[:100]
	}

	scored := make([]lineScored, 0, len(hitsList))
	for _, item := range hitsList {
		name := sm.namesLines[item.lineIdx]
		sc := scoreRuneOverlap(symptomWord, name)
		if sc > 0.25 {
			scored = append(scored, lineScored{lineIdx: item.lineIdx, score: sc})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, sm.namesLines[s.lineIdx])
	}
	return out
}

func (sm *SynonymMatcher) ExpandSynonyms(words []string, topKPerWord int) []string {
	seenIDs := make(map[string]bool)
	var idList []string

	for _, word := range words {
		if utf8.RuneCountInString(word) < 2 {
			continue
		}
		syns := sm.SearchSynonyms(word, topKPerWord)
		for _, name := range syns {
			id, ok := sm.nameToHpoID[strings.ToLower(strings.TrimSpace(name))]
			if ok && !seenIDs[id] {
				seenIDs[id] = true
				idList = append(idList, id)
			}
		}
	}
	return idList
}
