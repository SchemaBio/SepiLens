package annotation

import "strings"

// ReviewStars returns the ClinVar review level already defined on ClinVar.

// MinReviewStarsForRelativeCriterion is the PS1/PM5 gate. A single submitter
// assertion is not enough to treat a different variant as established.
const MinReviewStarsForRelativeCriterion = 2

// SameAminoAcid reports whether two protein changes describe the same
// substitution, e.g. p.Phe508del vs p.F508del.
func SameAminoAcid(a, b string) bool {
	pa, pb := parseProtein(a), parseProtein(b)
	return pa.substitution != "" && pa.substitution == pb.substitution
}

// SameResidue reports whether two protein changes touch the same position with a
// different substitution. Used by PM5; requires both changes to be missense-like.
func SameResidue(a, b string) (position string, same bool) {
	pa, pb := parseProtein(a), parseProtein(b)
	if pa.position == "" || pa.position != pb.position {
		return "", false
	}
	if pa.substitution == pb.substitution {
		return pa.position, false
	}
	return pa.position, true
}

type proteinChange struct {
	position     string
	substitution string
}

// parseProtein extracts a residue position and substitution from common HGVS
// protein forms: p.Phe508del, p.F508del, p.Val600Glu, p.V600E.
func parseProtein(raw string) proteinChange {
	value := normalizeProtein(raw)
	if value == "" {
		return proteinChange{}
	}
	threeLetter := map[string]string{
		"ala": "A", "arg": "R", "asn": "N", "asp": "D", "cys": "C", "gln": "Q", "glu": "E",
		"gly": "G", "his": "H", "ile": "I", "leu": "L", "lys": "K", "met": "M", "phe": "F",
		"pro": "P", "ser": "S", "thr": "T", "trp": "W", "tyr": "Y", "val": "V", "ter": "*",
	}
	for long, short := range threeLetter {
		value = strings.ReplaceAll(value, long, short)
	}
	// Separate a trailing structural keyword before scanning for digits.
	suffix := ""
	for _, candidate := range []string{"delins", "del", "dup", "ins", "fs"} {
		if strings.HasSuffix(value, candidate) {
			suffix = candidate
			value = strings.TrimSuffix(value, candidate)
			break
		}
	}
	start, end := -1, -1
	for i := 0; i < len(value); i++ {
		if value[i] >= '0' && value[i] <= '9' {
			if start < 0 {
				start = i
			}
			end = i + 1
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start <= 0 || end < 0 {
		return proteinChange{}
	}
	position := value[start:end]
	ref := strings.ToUpper(value[start-1 : start])
	alt := ""
	if suffix != "" {
		alt = suffix
	} else if rest := value[end:]; rest != "" {
		alt = strings.ToUpper(rest[:1])
	}
	return proteinChange{position: position, substitution: ref + position + alt}
}

func normalizeProtein(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.TrimPrefix(v, "p.")
	v = strings.ReplaceAll(v, "(", "")
	v = strings.ReplaceAll(v, ")", "")
	return v
}
