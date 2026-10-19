package annotation

import (
	"errors"
	"fmt"
	"strings"
)

// Resolver validates a caller-supplied annotation set and produces a normalized
// copy. It never fetches data; snapshot and API adapters plug in later.
type Resolver struct {
	// MinAlleleNumber is the gnomAD coverage floor used by PM2.
	MinAlleleNumber int
	// Now is an injectable clock for staleness checks (RFC3339 input).
	MaxAgeDays int
}

func NewResolver() *Resolver {
	return &Resolver{MinAlleleNumber: 1000, MaxAgeDays: 0}
}

// Normalize validates and copies the set, filling defaults and flagging conflicts.
func (r *Resolver) Normalize(in *Set, assembly string) (*Set, error) {
	if in == nil {
		return nil, nil
	}
	out := *in
	if out.Source == "" {
		out.Source = SourceCaller
	}
	if err := out.Validate(assembly); err != nil {
		return nil, err
	}
	// Every record inherits the set source unless it pinned a more specific one.
	applySource := func(record *Record) {
		if record != nil && record.Source == "" {
			record.Source = out.Source
		}
	}
	applySource(recClinVar(out.ClinVar))
	applySource(recGnomAD(out.GnomAD))
	applySource(recAlpha(out.AlphaMissense))
	applySource(recPangolin(out.Pangolin))
	applySource(recGenCC(out.GenCC))
	if cv := out.ClinVar; cv != nil {
		copy := *cv
		if copy.Conflicting || strings.Contains(strings.ToLower(copy.ReviewStatus), "conflict") {
			copy.Conflicting = true
		}
		if copy.Status == "" {
			copy.Status = StatusFound
		}
		out.ClinVar = &copy
	}
	if gn := out.GnomAD; gn != nil {
		copy := *gn
		out.GnomAD = &copy
	}
	if am := out.AlphaMissense; am != nil {
		copy := *am
		out.AlphaMissense = &copy
	}
	if pg := out.Pangolin; pg != nil {
		copy := *pg
		out.Pangolin = &copy
	}
	if gc := out.GenCC; gc != nil {
		copy := *gc
		out.GenCC = &copy
	}
	return &out, nil
}

// Conflicts lists annotation-level contradictions that must reach the reviewer.
func (r *Resolver) Conflicts(s *Set) []string {
	var out []string
	if s == nil {
		return out
	}
	if cv := s.ClinVar; cv != nil && cv.Conflicting {
		out = append(out, "clinvar: conflicting classifications")
	}
	if gc := s.GenCC; gc != nil {
		if gc.RefutedOrDisputed() {
			out = append(out, "gencc: gene-disease assertion is "+strings.ToLower(gc.Classification))
		}
		if cv := s.ClinVar; cv != nil && cv.IsPathogenic() && gc.RefutedOrDisputed() {
			out = append(out, "clinvar pathogenic conflicts with gencc refuted/disputed")
		}
	}
	if am := s.AlphaMissense; am != nil && am.Class == "likely_pathogenic" {
		if pg := s.Pangolin; pg != nil && pg.Interpretation == "no_splice_impact" {
			// Not a conflict: both can be true for a missense variant.
			_ = pg
		}
	}
	return out
}

// Complete reports whether every source in the set reached a terminal status.
// unavailable and stale are non-terminal: evidence derived from them stays
// insufficient_data rather than being treated as absent.
func (r *Resolver) Complete(s *Set) bool {
	if s == nil {
		return false
	}
	for _, record := range []*Record{recClinVar(s.ClinVar), recGnomAD(s.GnomAD), recAlpha(s.AlphaMissense), recPangolin(s.Pangolin), recGenCC(s.GenCC)} {
		if record == nil {
			continue
		}
		if record.Status == StatusUnavailable || record.Status == StatusStale {
			return false
		}
	}
	return true
}

// ErrNoAnnotations lets callers fall back to the legacy path explicitly.
var ErrNoAnnotations = errors.New("no annotations supplied")

// Require returns ErrNoAnnotations when the set is absent.
func (r *Resolver) Require(s *Set) error {
	if s == nil {
		return ErrNoAnnotations
	}
	if s.ClinVar == nil && s.GnomAD == nil && s.AlphaMissense == nil && s.Pangolin == nil && s.GenCC == nil {
		return fmt.Errorf("%w: all sources empty", ErrNoAnnotations)
	}
	return nil
}
