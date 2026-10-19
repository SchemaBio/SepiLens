package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

var ErrOutputInvalid = errors.New("AI_OUTPUT_INVALID")
var ErrBudgetExceeded = errors.New("AI_TOOL_BUDGET_EXCEEDED")
var ErrProvider = errors.New("AI_PROVIDER_UNAVAILABLE")

type Criterion struct {
	Code          acmg2015.CriterionCode   `json:"code"`
	Status        acmg2015.CriterionStatus `json:"status"`
	Strength      acmg2015.Strength        `json:"strength"`
	Change        string                   `json:"change"`
	Reason        string                   `json:"override_reason"`
	Rationale     string                   `json:"rationale"`
	Citations     []string                 `json:"citations"`
	Facts         []string                 `json:"facts_confirmed"`
	StrengthBasis []string                 `json:"strength_basis"`
}
type Assessment struct {
	Criteria       []Criterion `json:"criteria"`
	Classification string      `json:"classification"`
}
type Reconciliation struct {
	Code     acmg2015.CriterionCode `json:"code"`
	Local    *acmg2015.Candidate    `json:"local,omitempty"`
	Proposed *Criterion             `json:"proposed,omitempty"`
	Accepted bool                   `json:"accepted"`
	Reason   string                 `json:"reason"`
}
type MergeOptions struct {
	AllowPP5BP6             bool
	Facts                   map[acmg2015.CriterionCode]map[string]bool
	StrengthLimits          map[acmg2015.CriterionCode]acmg2015.Strength
	ApprovedStrengthSources map[string]bool
}

func Parse(raw string) (Assessment, error) {
	var a Assessment
	if len(raw) > 256<<10 {
		return a, ErrOutputInvalid
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || len(a.Criteria) > 28 {
		return a, ErrOutputInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return a, ErrOutputInvalid
	}
	seen := map[acmg2015.CriterionCode]bool{}
	for _, c := range a.Criteria {
		if _, ok := acmg2015.CanonicalCriteria[c.Code]; !ok || seen[c.Code] {
			return a, ErrOutputInvalid
		}
		seen[c.Code] = true
		switch c.Status {
		case "met", "not_met", "not_applicable", "insufficient_data", "conflicting":
		default:
			return a, ErrOutputInvalid
		}
		switch c.Change {
		case "confirmed", "upgraded_from_insufficient_data", "downgraded", "negated", "strength_changed", "added":
		default:
			return a, ErrOutputInvalid
		}
		if strength(c.Strength) == 0 || len(c.Reason) > 4000 || len(c.Rationale) > 8000 {
			return a, ErrOutputInvalid
		}
	}
	return a, nil
}
func strength(s acmg2015.Strength) int {
	return map[acmg2015.Strength]int{"Supporting": 1, "Moderate": 2, "Strong": 3, "VeryStrong": 4, "StandAlone": 5}[s]
}
func verified(s evidence.Source) bool {
	return s.ValidationStatus == evidence.ValidationVerified && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified
}

// Merge uses independently established server facts, never the model's fact
// assertions, to authorize a met criterion. Rejected proposals retain baseline.
func Merge(local acmg2015.Derivation, a Assessment, sources []evidence.Source, opts MergeOptions) (acmg2015.Derivation, []Reconciliation) {
	out := acmg2015.Derivation{Gaps: append([]string(nil), local.Gaps...), Conflicts: append([]string(nil), local.Conflicts...)}
	base := map[acmg2015.CriterionCode]acmg2015.Candidate{}
	for _, c := range local.Candidates {
		base[c.Code] = c
	}
	srcs := map[string]evidence.Source{}
	for _, s := range sources {
		srcs[s.ID] = s
	}
	seen := map[acmg2015.CriterionCode]bool{}
	var audit []Reconciliation
	for _, p := range a.Criteria {
		seen[p.Code] = true
		b, exists := base[p.Code]
		entry := Reconciliation{Code: p.Code, Proposed: &p}
		if exists {
			copy := b
			entry.Local = &copy
		}
		reject := func(reason string) {
			entry.Reason = "unsupported_override: " + reason
			audit = append(audit, entry)
			if exists {
				out.Candidates = append(out.Candidates, b)
			}
		}
		if exists && p.Change == "confirmed" && p.Status == b.Status && p.Strength == b.Strength {
			entry.Accepted = true
			entry.Reason = "confirmed"
			audit = append(audit, entry)
			out.Candidates = append(out.Candidates, b)
			continue
		}
		if p.Reason == "" {
			reject("override reason required")
			continue
		}
		validChange := false
		switch p.Change {
		case "added":
			validChange = !exists
		case "upgraded_from_insufficient_data":
			validChange = exists && b.Status == "insufficient_data" && p.Status == "met"
		case "downgraded":
			validChange = exists && ((b.Status == "met" && (p.Status == "insufficient_data" || p.Status == "conflicting")) || (p.Status == b.Status && strength(p.Strength) < strength(b.Strength)))
		case "negated":
			validChange = exists && b.Status == "met" && p.Status == "not_met"
		case "strength_changed":
			validChange = exists && p.Status == b.Status && p.Strength != b.Strength
		case "confirmed":
			validChange = exists && b.Status == "met" && (p.Status == "insufficient_data" || p.Status == "not_met")
		}
		if !validChange {
			reject("illegal change transition")
			continue
		}
		citationsOK := len(p.Citations) > 0
		for _, id := range p.Citations {
			citationsOK = citationsOK && verified(srcs[id])
		}
		if !citationsOK {
			reject("unverified or mismatched citation")
			continue
		}
		def := acmg2015.CanonicalCriteria[p.Code]
		limit := def.DefaultStrength
		if exists && strength(b.Strength) > 0 {
			limit = b.Strength
		}
		if configured, ok := opts.StrengthLimits[p.Code]; ok {
			limit = configured
		}
		if strength(p.Strength) > strength(limit) {
			reject("strength exceeds server-authorized limit")
			continue
		}
		if strength(p.Strength) > strength(def.DefaultStrength) {
			ok := len(p.StrengthBasis) > 0
			for _, id := range p.StrengthBasis {
				ok = ok && opts.ApprovedStrengthSources[id] && verified(srcs[id])
			}
			if !ok {
				reject("published strength basis is not server approved")
				continue
			}
		}
		if (p.Code == "PP5" || p.Code == "BP6") && !opts.AllowPP5BP6 {
			reject("criterion disabled")
			continue
		}
		facts := map[string]bool{}
		for k, v := range opts.Facts[p.Code] {
			facts[k] = v
		}
		if p.Status == "met" {
			claims := map[string]bool{}
			for _, f := range p.Facts {
				claims[f] = true
			}
			complete := true
			for _, f := range acmg2015.RequiredFacts[p.Code] {
				complete = complete && facts[f] && claims[f]
			}
			if !complete {
				reject("required facts are not independently verified")
				continue
			}
		}
		entry.Accepted = true
		entry.Reason = p.Change
		audit = append(audit, entry)
		out.Candidates = append(out.Candidates, acmg2015.Candidate{Code: p.Code, Status: p.Status, Strength: p.Strength, Direction: def.Direction, Reason: p.Rationale, Facts: facts, SourceIDs: append([]string(nil), p.Citations...)})
	}
	for _, b := range local.Candidates {
		if !seen[b.Code] {
			copy := b
			audit = append(audit, Reconciliation{Code: b.Code, Local: &copy, Reason: "ai_omitted"})
			out.Candidates = append(out.Candidates, b)
		}
	}
	sort.Slice(out.Candidates, func(i, j int) bool { return out.Candidates[i].Code < out.Candidates[j].Code })
	sort.Slice(audit, func(i, j int) bool { return audit[i].Code < audit[j].Code })
	return out, audit
}

func OutputSchema() map[string]any {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	arr := func() map[string]any { return map[string]any{"type": "array", "items": str()} }
	props := map[string]any{"code": str(), "status": str(), "strength": str(), "change": str(), "override_reason": str(), "rationale": str(), "citations": arr(), "facts_confirmed": arr(), "strength_basis": arr()}
	required := []any{"code", "status", "strength", "change", "override_reason", "rationale", "citations", "facts_confirmed", "strength_basis"}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []any{"criteria", "classification"}, "properties": map[string]any{"classification": str(), "criteria": map[string]any{"type": "array", "maxItems": float64(28), "items": map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}}}}
}
