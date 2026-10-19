package annotation_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/annotation"
)

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }
func b(v bool) *bool       { return &v }

func validSet() *annotation.Set {
	return &annotation.Set{
		Source: annotation.SourceCaller,
		ClinVar: &annotation.ClinVar{
			Record:         annotation.Record{Version: "2026-09", Assembly: "GRCh38", Status: annotation.StatusFound},
			VariationID:    "7105",
			Classification: "Pathogenic",
			ReviewStatus:   "reviewed_by_expert_panel",
			HGVSp:          "p.Phe508del",
		},
		GnomAD: &annotation.GnomAD{
			Record:           annotation.Record{Version: "v4.1", Assembly: "GRCh38", Status: annotation.StatusFound},
			FAF95:            f(0.00001),
			PopMaxAF:         f(0.00002),
			AlleleNumber:     i(1200000),
			CoverageAdequate: b(true),
		},
		AlphaMissense: &annotation.AlphaMissense{
			Record:     annotation.Record{Version: "2023", Status: annotation.StatusFound},
			Score:      f(0.92),
			Class:      "likely_pathogenic",
			Transcript: "NM_000492.4",
		},
		Pangolin: &annotation.Pangolin{
			Record:         annotation.Record{Version: "1.0", Status: annotation.StatusFound},
			DeltaScore:     f(0.01),
			Interpretation: "no_splice_impact",
			Transcript:     "NM_000492.4",
		},
		GenCC: &annotation.GenCC{
			Record:         annotation.Record{Version: "2026-09", Status: annotation.StatusFound},
			Gene:           "CFTR",
			DiseaseID:      "MONDO:0009061",
			Classification: "Definitive",
			MOI:            "AR",
		},
	}
}

func TestValidateAcceptsCompleteSet(t *testing.T) {
	set := validSet()
	if err := set.Validate("GRCh38"); err != nil {
		t.Fatal(err)
	}
	if _, err := annotation.NewResolver().Normalize(set, "GRCh38"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsAssemblyMismatch(t *testing.T) {
	set := validSet()
	set.GnomAD.Assembly = "GRCh37"
	if err := set.Validate("GRCh38"); !errors.Is(err, annotation.ErrAssemblyMismatch) {
		t.Fatalf("expected assembly mismatch, got %v", err)
	}
}

func TestValidateRejectsMissingVersion(t *testing.T) {
	set := validSet()
	set.Pangolin.Version = ""
	if err := set.Validate("GRCh38"); !errors.Is(err, annotation.ErrMissingVersion) {
		t.Fatalf("expected missing version, got %v", err)
	}
}

func TestValidateRejectsOutOfRangeValues(t *testing.T) {
	set := validSet()
	set.GnomAD.FAF95 = f(1.5)
	if err := set.Validate("GRCh38"); !errors.Is(err, annotation.ErrInvalidFrequency) {
		t.Fatalf("frequency %v", err)
	}
	set = validSet()
	set.AlphaMissense.Score = f(-0.1)
	if err := set.Validate("GRCh38"); !errors.Is(err, annotation.ErrInvalidAlphaScore) {
		t.Fatalf("alpha %v", err)
	}
	set = validSet()
	set.Pangolin.DeltaScore = f(2)
	if err := set.Validate("GRCh38"); !errors.Is(err, annotation.ErrInvalidPangolin) {
		t.Fatalf("pangolin %v", err)
	}
}

func TestPM2RequiresCoverage(t *testing.T) {
	set := validSet()
	if !set.GnomAD.CoverageConfident(1000) {
		t.Fatal("adequate coverage rejected")
	}
	set.GnomAD.CoverageAdequate = b(false)
	if set.GnomAD.CoverageConfident(1000) {
		t.Fatal("inadequate coverage accepted")
	}
	set.GnomAD.CoverageAdequate = nil
	if set.GnomAD.CoverageConfident(1000) {
		t.Fatal("absent coverage flag accepted")
	}
	set.GnomAD.AlleleNumber = i(10)
	set.GnomAD.CoverageAdequate = b(true)
	if set.GnomAD.CoverageConfident(1000) {
		t.Fatal("low allele number accepted")
	}
}

func TestRarityPrefersFAF95(t *testing.T) {
	set := validSet()
	if v, ok := set.GnomAD.Rarity(); !ok || v != 0.00001 {
		t.Fatalf("rarity %v %v", v, ok)
	}
	set.GnomAD.FAF95 = nil
	if v, ok := set.GnomAD.Rarity(); !ok || v != 0.00002 {
		t.Fatalf("popmax fallback %v %v", v, ok)
	}
	set.GnomAD.PopMaxAF = nil
	if _, ok := set.GnomAD.Rarity(); ok {
		t.Fatal("missing frequency reported as usable")
	}
}

func TestReviewStars(t *testing.T) {
	cases := map[string]int{
		"reviewed_by_expert_panel":                             3,
		"criteria_provided,_multiple_submitters,_no_conflicts": 2,
		"criteria_provided,_single_submitter":                  1,
		"criteria_provided,_conflicting_classifications":       0,
		"no_assertion": 0,
	}
	for status, want := range cases {
		cv := &annotation.ClinVar{ReviewStatus: status}
		if got := cv.ReviewStars(); got != want {
			t.Fatalf("%s: want %d got %d", status, want, got)
		}
	}
}

func TestGenCCBlocking(t *testing.T) {
	refuted := &annotation.GenCC{Classification: "Refuted"}
	if !refuted.RefutedOrDisputed() || refuted.GenCCSupports() {
		t.Fatal("refuted must block")
	}
	definitive := &annotation.GenCC{Classification: "Definitive"}
	if definitive.RefutedOrDisputed() || !definitive.GenCCSupports() {
		t.Fatal("definitive must support")
	}
	limited := &annotation.GenCC{Classification: "Limited"}
	if limited.GenCCSupports() {
		t.Fatal("limited must not support PP4")
	}
}

func TestConflictsSurface(t *testing.T) {
	set := validSet()
	if c := annotation.NewResolver().Conflicts(set); len(c) != 0 {
		t.Fatalf("unexpected conflicts %v", c)
	}
	set.GenCC.Classification = "Refuted"
	c := annotation.NewResolver().Conflicts(set)
	if len(c) < 2 {
		t.Fatalf("refuted gencc not surfaced: %v", c)
	}
}

func TestUnavailableIsNotAbsent(t *testing.T) {
	set := validSet()
	set.GnomAD.Status = annotation.StatusUnavailable
	r := annotation.NewResolver()
	if r.Complete(set) {
		t.Fatal("unavailable source treated as terminal")
	}
	if r.Require(set) != nil {
		t.Fatal("present set rejected")
	}
	if r.Require(nil) != annotation.ErrNoAnnotations {
		t.Fatal("missing set accepted")
	}
}

func TestNormalizeDoesNotMutateInput(t *testing.T) {
	set := validSet()
	set.ClinVar.ReviewStatus = "criteria_provided,_conflicting_classifications"
	out, err := annotation.NewResolver().Normalize(set, "GRCh38")
	if err != nil {
		t.Fatal(err)
	}
	if !out.ClinVar.Conflicting {
		t.Fatal("conflict flag not set")
	}
	if set.ClinVar.Conflicting {
		t.Fatal("input mutated")
	}
}
