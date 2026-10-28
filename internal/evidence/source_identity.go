package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// DocumentGroup is a publication identity group, never proof of independent
// clinical cases. All records remain available for citation and audit.
type DocumentGroup struct {
	Key     string
	Records []Source
}

func DocumentKeys(source Source) []string {
	keys := []string{}
	pmid := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(source.PMID)), "PMID:"))
	if source.Type != SourceTypeClinVar && reDigits.MatchString(pmid) {
		if n, err := strconv.ParseUint(pmid, 10, 64); err == nil && n > 0 {
			keys = append(keys, "pmid:"+strconv.FormatUint(n, 10))
		}
	}
	doi := strings.TrimSpace(source.DOI)
	for _, prefix := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		if strings.HasPrefix(strings.ToLower(doi), prefix) {
			doi = doi[len(prefix):]
			break
		}
	}
	if decoded, err := url.PathUnescape(doi); err == nil {
		doi = decoded
	}
	doi = strings.ToLower(strings.TrimSpace(doi))
	if source.Type != SourceTypeClinVar && reDOI.MatchString(doi) {
		keys = append(keys, "doi:"+doi)
	}
	clinvar := strings.ToUpper(strings.TrimSpace(source.ClinVarID))
	if (source.Type == SourceTypeClinVar || len(keys) == 0) && reClinVar.MatchString(clinvar) {
		keys = append(keys, "clinvar:"+clinvar)
	}
	if len(keys) == 0 {
		if parsed, err := url.Parse(strings.TrimSpace(source.URL)); err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			parsed.Host = strings.ToLower(parsed.Host)
			parsed.Fragment = ""
			query := parsed.Query()
			for key := range query {
				if strings.HasPrefix(strings.ToLower(key), "utm_") {
					query.Del(key)
				}
			}
			parsed.RawQuery = query.Encode()
			keys = append(keys, "url:"+parsed.String())
		}
	}
	if len(keys) == 0 && source.ID != "" {
		keys = append(keys, "record:"+string(source.Type)+":"+source.ID)
	}
	return keys
}

// Shared PMID/DOI keys are linked transitively, so DOI-only and PMID-only copies
// of an article are not counted independently when a bridge record supplies both.
// Strong IDs take precedence over broad landing-page URLs. Namespaces prevent a
// numeric ClinVar accession from colliding with an unrelated numeric PMID.
func GroupSourceDocuments(sources []Source) []DocumentGroup {
	parents := make([]int, len(sources))
	for i := range parents {
		parents[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parents[i] != i {
			parents[i] = root(parents[i])
		}
		return parents[i]
	}
	byKey := map[string]int{}
	for i, source := range sources {
		for _, key := range DocumentKeys(source) {
			if previous, ok := byKey[key]; ok {
				a, b := root(i), root(previous)
				if a < b {
					parents[b] = a
				} else {
					parents[a] = b
				}
			} else {
				byKey[key] = i
			}
		}
	}
	groups := []DocumentGroup{}
	positions := map[int]int{}
	for i, source := range sources {
		r := root(i)
		position, exists := positions[r]
		if !exists {
			position = len(groups)
			positions[r] = position
			keys := DocumentKeys(source)
			key := ""
			if len(keys) > 0 {
				key = keys[0]
			}
			groups = append(groups, DocumentGroup{Key: key})
		}
		groups[position].Records = append(groups[position].Records, source)
	}
	return groups
}

func sourceFingerprint(source Source) string {
	source.ID = ""
	source.RetrievedAt = ""
	source.VerifiedAt = ""
	source.CaseIDs = append([]string(nil), source.CaseIDs...)
	sort.Strings(source.CaseIDs)
	raw, _ := json.Marshal(source)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func SourceRecordFingerprint(source Source) string { return sourceFingerprint(source) }

// Keep every distinct record without OR-ing trust flags or discarding content
// under an already cited ID. Changed records receive a stable internal ID; the
// original record and all earlier source/criterion references remain intact.
func MergeSourceRecords(existing, incoming []Source) []Source {
	out := append([]Source(nil), existing...)
	byID := map[string]string{}
	for _, source := range out {
		byID[source.ID] = sourceFingerprint(source)
	}
	for _, source := range incoming {
		fingerprint := sourceFingerprint(source)
		if source.ID == "" {
			source.ID = "retrieved-" + fingerprint
		}
		if previous, exists := byID[source.ID]; exists {
			if previous == fingerprint {
				continue
			}
			hash := sha256.Sum256([]byte(source.ID + "/" + fingerprint))
			source.ID = "retrieved-" + hex.EncodeToString(hash[:])
			if _, exists := byID[source.ID]; exists {
				continue
			}
		}
		byID[source.ID] = fingerprint
		out = append(out, source)
	}
	return out
}

// Counts are conservative source metadata only. They never create clinical
// identities or rule facts. Within a document group use the union of explicit
// case IDs if supplied, otherwise the maximum reported count, never their sum.
func UniqueSourceCaseCount(sources []Source) int {
	total := 0
	for _, group := range GroupSourceDocuments(sources) {
		ids := map[string]bool{}
		maximum := 0
		for _, source := range group.Records {
			if source.CaseCount > maximum {
				maximum = source.CaseCount
			}
			for _, id := range source.CaseIDs {
				if id != "" {
					ids[id] = true
				}
			}
		}
		if len(ids) > 0 {
			total += len(ids)
		} else if group.Key != "" {
			total += maximum
		}
	}
	return total
}

// Stable public document identifiers may be shown to a model; record-only keys
// can contain patient IDs and must instead use the existing opaque source aliases.
func PublicDocumentKey(group DocumentGroup) string {
	keys := []string{}
	for _, source := range group.Records {
		for _, key := range DocumentKeys(source) {
			if strings.HasPrefix(key, "pmid:") || strings.HasPrefix(key, "doi:") || strings.HasPrefix(key, "clinvar:") {
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// Only an independently matched, material-verified conflict can hold a prior
// structured claim from the same document. Unverified search findings cannot
// poison a trusted baseline merely by carrying a conflict label.
func ConflictingSourceGroupIDs(sources []Source) map[string]bool {
	blocked := map[string]bool{}
	for _, group := range GroupSourceDocuments(sources) {
		conflict := false
		for _, source := range group.Records {
			conflict = conflict || source.ValidationStatus == ValidationConflict && source.IdentifierValid && source.RecordExists && source.ObjectMatched && source.MaterialVerified
		}
		if conflict {
			for _, source := range group.Records {
				blocked[source.ID] = true
			}
		}
	}
	return blocked
}
