package retrieval

import (
	"regexp"
	"strings"
)

var safeHGVS = regexp.MustCompile(`^[A-Za-z0-9_.:>()*+?=-]+$`)
var aminoSubstitution = regexp.MustCompile(`^([A-Z][a-z]{2})([0-9]+)([A-Z][a-z]{2})$`)
var aminoLetters = map[string]string{"Ala": "A", "Arg": "R", "Asn": "N", "Asp": "D", "Cys": "C", "Gln": "Q", "Glu": "E", "Gly": "G", "His": "H", "Ile": "I", "Leu": "L", "Lys": "K", "Met": "M", "Phe": "F", "Pro": "P", "Ser": "S", "Thr": "T", "Trp": "W", "Tyr": "Y", "Val": "V"}

func sequenceLiteratureTerm(q Query) string {
	terms := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" && safeHGVS.MatchString(value) {
			terms = append(terms, `"`+value+`"`)
		}
	}
	dna := q.HGVS
	if i := strings.LastIndex(dna, ":"); i >= 0 {
		dna = dna[i+1:]
	}
	add(dna)
	protein := q.ProteinHGVS
	if i := strings.LastIndex(protein, ":"); i >= 0 {
		protein = protein[i+1:]
	}
	protein = strings.Trim(strings.TrimPrefix(protein, "p."), "()")
	add(protein)
	if m := aminoSubstitution.FindStringSubmatch(protein); len(m) == 4 && aminoLetters[m[1]] != "" && aminoLetters[m[3]] != "" {
		add(aminoLetters[m[1]] + m[2] + aminoLetters[m[3]])
	}
	if len(terms) == 0 {
		return q.Gene
	}
	return q.Gene + " AND (" + strings.Join(terms, " OR ") + ")"
}

func sequenceObjectMatched(q Query, content string) bool {
	if q.Gene == "" || !regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])`+regexp.QuoteMeta(q.Gene)+`([^A-Za-z0-9]|$)`).MatchString(content) {
		return false
	}
	for _, match := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(sequenceLiteratureTerm(q), -1) {
		if strings.Contains(strings.ToLower(content), strings.ToLower(match[1])) {
			return true
		}
	}
	return false
}
