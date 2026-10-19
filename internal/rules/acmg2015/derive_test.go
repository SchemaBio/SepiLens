package acmg2015_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/annotation"
	a "github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }
func bp(v bool) *bool       { return &v }

func gnomadWith(faf float64, coverage bool) *annotation.GnomAD {
	return &annotation.GnomAD{
		Record:           annotation.Record{Version: "v4.1", Assembly: "GRCh38", Status: annotation.StatusFound},
		FAF95:            fp(faf),
		PopMaxAF:         fp(faf),
		AlleleNumber:     ip(1200000),
		CoverageAdequate: bp(coverage),
	}
}

func setWith(g *annotation.GnomAD) *annotation.Set {
	return &annotation.Set{Source: annotation.SourceCaller, GnomAD: g}
}

func find(d a.Derivation, code a.CriterionCode) (a.Candidate, bool) {
	for _, c := range d.Candidates {
		if c.Code == code {
			return c, true
		}
	}
	return a.Candidate{}, false
}

func TestPM2RequiresCoverageAndDefaultsToSupporting(t *testing.T) {
	d := a.Derive(setWith(gnomadWith(0.00001, true)), a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	pm2, ok := find(d, a.CodePM2)
	if !ok || pm2.Status != a.CriterionStatusMet {
		t.Fatalf("pm2 %+v", pm2)
	}
	if pm2.Strength != a.StrengthSupporting {
		t.Fatalf("PM2 must default to Supporting, got %s", pm2.Strength)
	}

	d = a.Derive(setWith(gnomadWith(0.00001, false)), a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	pm2, _ = find(d, a.CodePM2)
	if pm2.Status != a.CriterionStatusInsufficientData {
		t.Fatalf("inadequate coverage accepted: %+v", pm2)
	}
}

func TestPM2AndBA1Boundaries(t *testing.T) {
	cases := []struct {
		faf     float64
		wantPM2 a.CriterionStatus
		wantBA1 a.CriterionStatus
	}{
		{0.00001, a.CriterionStatusMet, a.CriterionStatusNotMet},
		// Exactly 5% is neither rare (PM2) nor strictly above 5% (BA1).
		{0.05, a.CriterionStatusNotMet, a.CriterionStatusNotMet},
		{0.0500001, a.CriterionStatusNotMet, a.CriterionStatusMet},
	}
	for _, c := range cases {
		d := a.Derive(setWith(gnomadWith(c.faf, true)), a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
		pm2, _ := find(d, a.CodePM2)
		ba1, _ := find(d, a.CodeBA1)
		if pm2.Status != c.wantPM2 || ba1.Status != c.wantBA1 {
			t.Fatalf("faf %v: pm2=%s ba1=%s", c.faf, pm2.Status, ba1.Status)
		}
	}
}

func TestMissingGnomADIsNotBenign(t *testing.T) {
	d := a.Derive(&annotation.Set{}, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	pm2, ok := find(d, a.CodePM2)
	if !ok || pm2.Status != a.CriterionStatusInsufficientData {
		t.Fatalf("missing gnomAD treated as evidence: %+v", pm2)
	}
	for _, c := range d.Candidates {
		if c.Code == a.CodeBA1 && c.Status == a.CriterionStatusMet {
			t.Fatal("missing gnomAD produced BA1")
		}
	}
}

func TestComputationalCriteriaDoNotStack(t *testing.T) {
	set := &annotation.Set{
		AlphaMissense: &annotation.AlphaMissense{Record: annotation.Record{Version: "2023", Status: annotation.StatusFound}, Class: "likely_pathogenic", Transcript: "NM_000492.4"},
		Pangolin:      &annotation.Pangolin{Record: annotation.Record{Version: "1.0", Status: annotation.StatusFound}, Interpretation: "splice_altering", Transcript: "NM_000492.4"},
	}
	v := a.SNVVariantInput{Consequence: "missense_variant", Transcript: "NM_000492.4"}
	d := a.Derive(set, v, a.ClinicalContextInput{}, a.DeriveOptions{})
	count := 0
	for _, c := range d.Candidates {
		if c.Code == a.CodePP3 && c.Status == a.CriterionStatusMet {
			count++
			if c.Strength != a.StrengthSupporting {
				t.Fatalf("PP3 strength %s", c.Strength)
			}
		}
	}
	if count != 1 {
		t.Fatalf("PP3 emitted %d times", count)
	}
}

func TestTranscriptMismatchBlocksComputationalCriteria(t *testing.T) {
	set := &annotation.Set{
		AlphaMissense: &annotation.AlphaMissense{Record: annotation.Record{Version: "2023", Status: annotation.StatusFound}, Class: "likely_pathogenic", Transcript: "NM_111111.1"},
	}
	v := a.SNVVariantInput{Consequence: "missense_variant", Transcript: "NM_000492.4"}
	d := a.Derive(set, v, a.ClinicalContextInput{}, a.DeriveOptions{})
	pp3, _ := find(d, a.CodePP3)
	if pp3.Status == a.CriterionStatusMet {
		t.Fatal("mismatched transcript produced PP3")
	}
}

func TestPS1AndPM5RequireReviewGate(t *testing.T) {
	low := &annotation.ClinVar{
		Record:                       annotation.Record{Version: "2026-09", Status: annotation.StatusFound},
		ReviewStatus:                 "criteria_provided,_single_submitter",
		OtherPathogenicSameAminoAcid: []string{"c.1520_1522del"},
		OtherPathogenicSameResidue:   []string{"p.V600E"},
	}
	d := a.Derive(&annotation.Set{ClinVar: low}, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	ps1, _ := find(d, a.CodePS1)
	pm5, _ := find(d, a.CodePM5)
	if ps1.Status != a.CriterionStatusInsufficientData || pm5.Status != a.CriterionStatusInsufficientData {
		t.Fatalf("single submitter passed: %+v %+v", ps1, pm5)
	}
	low.ReviewStatus = "reviewed_by_expert_panel"
	d = a.Derive(&annotation.Set{ClinVar: low}, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	ps1, _ = find(d, a.CodePS1)
	pm5, _ = find(d, a.CodePM5)
	if ps1.Status != a.CriterionStatusMet || pm5.Status != a.CriterionStatusMet {
		t.Fatalf("expert panel rejected: %+v %+v", ps1, pm5)
	}
}

func TestClinVarConflictIsSurfaced(t *testing.T) {
	cv := &annotation.ClinVar{
		Record:                       annotation.Record{Version: "2026-09", Status: annotation.StatusFound},
		ReviewStatus:                 "reviewed_by_expert_panel",
		Conflicting:                  true,
		OtherPathogenicSameAminoAcid: []string{"c.1520_1522del"},
	}
	d := a.Derive(&annotation.Set{ClinVar: cv}, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	ps1, _ := find(d, a.CodePS1)
	if ps1.Status != a.CriterionStatusConflicting {
		t.Fatalf("conflict not surfaced: %+v", ps1)
	}
}

func TestPVS1RequiresAttestations(t *testing.T) {
	v := a.SNVVariantInput{Consequence: "stop_gained"}
	ctx := a.ClinicalContextInput{Inheritance: "AR"}
	d := a.Derive(&annotation.Set{}, v, ctx, a.DeriveOptions{})
	pvs1, _ := find(d, a.CodePVS1)
	if pvs1.Status != a.CriterionStatusInsufficientData {
		t.Fatalf("PVS1 without mechanism accepted: %+v", pvs1)
	}
	d = a.Derive(&annotation.Set{}, v, ctx, a.DeriveOptions{PVS1MechanismConfirmed: true, TranscriptRelevant: true})
	pvs1, _ = find(d, a.CodePVS1)
	if pvs1.Status != a.CriterionStatusMet || pvs1.Strength != a.StrengthVeryStrong {
		t.Fatalf("PVS1 complete input rejected: %+v", pvs1)
	}
}

func TestPVS1SpliceIsCappedBelowVeryStrong(t *testing.T) {
	v := a.SNVVariantInput{Consequence: "splice_donor_variant"}
	ctx := a.ClinicalContextInput{Inheritance: "AD"}
	d := a.Derive(&annotation.Set{}, v, ctx, a.DeriveOptions{PVS1MechanismConfirmed: true, TranscriptRelevant: true})
	pvs1, _ := find(d, a.CodePVS1)
	if pvs1.Status != a.CriterionStatusMet || pvs1.Strength == a.StrengthVeryStrong {
		t.Fatalf("splice PVS1 not capped: %+v", pvs1)
	}
}

func TestPVS1BlockedByRefutedGenCC(t *testing.T) {
	v := a.SNVVariantInput{Consequence: "frameshift_variant"}
	ctx := a.ClinicalContextInput{Inheritance: "AD"}
	set := &annotation.Set{GenCC: &annotation.GenCC{Record: annotation.Record{Version: "2026-09", Status: annotation.StatusFound}, Classification: "Refuted"}}
	d := a.Derive(set, v, ctx, a.DeriveOptions{PVS1MechanismConfirmed: true, TranscriptRelevant: true})
	pvs1, _ := find(d, a.CodePVS1)
	if pvs1.Status != a.CriterionStatusConflicting {
		t.Fatalf("refuted gencc did not block PVS1: %+v", pvs1)
	}
}

func TestPP5BP6DisabledByDefault(t *testing.T) {
	cv := &annotation.ClinVar{Record: annotation.Record{Version: "2026-09", Status: annotation.StatusFound}, Classification: "Pathogenic"}
	d := a.Derive(&annotation.Set{ClinVar: cv}, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	if _, ok := find(d, a.CodePP5); ok {
		t.Fatal("PP5 emitted while disabled")
	}
}

func TestBP7RequiresSpliceAndConservation(t *testing.T) {
	v := a.SNVVariantInput{Consequence: "synonymous_variant"}
	d := a.Derive(&annotation.Set{}, v, a.ClinicalContextInput{}, a.DeriveOptions{})
	bp7, _ := find(d, a.CodeBP7)
	if bp7.Status != a.CriterionStatusInsufficientData {
		t.Fatalf("BP7 without splice data: %+v", bp7)
	}
	pg := &annotation.Pangolin{Record: annotation.Record{Version: "1.0", Status: annotation.StatusFound}, Interpretation: "no_splice_impact"}
	d = a.Derive(&annotation.Set{Pangolin: pg}, v, a.ClinicalContextInput{}, a.DeriveOptions{})
	bp7, _ = find(d, a.CodeBP7)
	if bp7.Status == a.CriterionStatusMet {
		t.Fatal("BP7 met without conservation evidence")
	}
}

func TestDerivationIsDeterministic(t *testing.T) {
	set := setWith(gnomadWith(0.00001, true))
	first := a.Derive(set, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	second := a.Derive(set, a.SNVVariantInput{}, a.ClinicalContextInput{}, a.DeriveOptions{})
	if len(first.Candidates) != len(second.Candidates) {
		t.Fatal("candidate count differs")
	}
	for i := range first.Candidates {
		if first.Candidates[i].Code != second.Candidates[i].Code || first.Candidates[i].Status != second.Candidates[i].Status {
			t.Fatal("derivation order or status differs")
		}
	}
}
