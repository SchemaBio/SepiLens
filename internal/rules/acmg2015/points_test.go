package acmg2015_test

import (
	"testing"

	a "github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func crit(code a.CriterionCode, strength a.Strength) a.AppliedCriterion {
	return a.AppliedCriterion{Code: code, Direction: a.CanonicalCriteria[code].Direction, EvaluatedStrength: strength, Status: a.CriterionStatusMet}
}

func TestPointsClassificationBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		items  []a.AppliedCriterion
		want   string
		pathPt int
		benPt  int
	}{
		{"pathogenic at 10", []a.AppliedCriterion{crit(a.CodePVS1, a.StrengthVeryStrong), crit(a.CodePM2, a.StrengthSupporting), crit(a.CodePP3, a.StrengthSupporting)}, "Pathogenic", 10, 0},
		{"likely pathogenic at 6", []a.AppliedCriterion{crit(a.CodePVS1, a.StrengthStrong), crit(a.CodePM2, a.StrengthModerate)}, "Likely pathogenic", 6, 0},
		{"vus at 5", []a.AppliedCriterion{crit(a.CodePVS1, a.StrengthStrong), crit(a.CodePM2, a.StrengthSupporting)}, "VUS", 5, 0},
		{"likely benign at 4", []a.AppliedCriterion{crit(a.CodeBS1, a.StrengthStrong)}, "Likely benign", 0, 4},
		{"benign at 7", []a.AppliedCriterion{crit(a.CodeBS1, a.StrengthStrong), crit(a.CodeBP4, a.StrengthSupporting), crit(a.CodeBP7, a.StrengthSupporting), crit(a.CodeBP1, a.StrengthSupporting)}, "Benign", 0, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := a.CombinePoints(c.items)
			if got.Classification != c.want {
				t.Fatalf("classification %s, want %s (pts %d/%d)", got.Classification, c.want, got.PathogenicPts, got.BenignPts)
			}
			if c.pathPt != 0 && got.PathogenicPts != c.pathPt {
				t.Fatalf("pathogenic points %d, want %d", got.PathogenicPts, c.pathPt)
			}
			if c.benPt != 0 && got.BenignPts != c.benPt {
				t.Fatalf("benign points %d, want %d", got.BenignPts, c.benPt)
			}
		})
	}
}

func TestPointsExactThresholds(t *testing.T) {
	if got := a.CombinePoints([]a.AppliedCriterion{crit(a.CodePVS1, a.StrengthVeryStrong), crit(a.CodePM2, a.StrengthModerate)}); got.PathogenicPts != 10 || got.Classification != "Pathogenic" {
		t.Fatalf("8+2 should be Pathogenic: %+v", got)
	}
	if got := a.CombinePoints([]a.AppliedCriterion{crit(a.CodePS1, a.StrengthStrong), crit(a.CodePM2, a.StrengthModerate)}); got.PathogenicPts != 6 || got.Classification != "Likely pathogenic" {
		t.Fatalf("4+2 should be Likely pathogenic: %+v", got)
	}
	if got := a.CombinePoints([]a.AppliedCriterion{crit(a.CodePM2, a.StrengthModerate)}); got.Classification != "VUS" {
		t.Fatalf("2 points must stay VUS: %+v", got)
	}
}

func TestPointsConflictStaysVUS(t *testing.T) {
	got := a.CombinePoints([]a.AppliedCriterion{
		crit(a.CodePVS1, a.StrengthVeryStrong),
		crit(a.CodePS1, a.StrengthStrong),
		crit(a.CodeBS1, a.StrengthStrong),
		crit(a.CodeBP4, a.StrengthSupporting),
	})
	if !got.Conflict || got.Classification != "VUS" {
		t.Fatalf("conflict not surfaced: %+v", got)
	}
}

func TestPointsStandAloneBenignWins(t *testing.T) {
	got := a.CombinePoints([]a.AppliedCriterion{
		crit(a.CodeBA1, a.StrengthStandAlone),
		crit(a.CodePVS1, a.StrengthVeryStrong),
		crit(a.CodePS1, a.StrengthStrong),
	})
	if got.Classification != "Benign" {
		t.Fatalf("BA1 must dominate: %+v", got)
	}
}

func TestPointsIgnoreDuplicateCodes(t *testing.T) {
	got := a.CombinePoints([]a.AppliedCriterion{crit(a.CodePS1, a.StrengthStrong), crit(a.CodePS1, a.StrengthStrong)})
	if got.PathogenicPts != 4 {
		t.Fatalf("duplicate counted twice: %+v", got)
	}
	if got.SpecVersion != a.PointsSpecVersion {
		t.Fatal("spec version missing")
	}
}

func TestPointsIgnoreNonMet(t *testing.T) {
	c := crit(a.CodePS1, a.StrengthStrong)
	c.Status = a.CriterionStatusInsufficientData
	got := a.CombinePoints([]a.AppliedCriterion{c})
	if got.PathogenicPts != 0 || len(got.AppliedCodes) != 0 {
		t.Fatalf("non-met counted: %+v", got)
	}
}
