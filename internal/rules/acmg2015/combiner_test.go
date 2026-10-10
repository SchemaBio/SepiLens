package acmg2015_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestCombiner_AllStandardPaths(t *testing.T) {
	c := acmg2015.NewCombiner()

	tests := []struct {
		name           string
		criteria       []acmg2015.AppliedCriterion
		expectedClass  string
		expectConflict bool
	}{
		// --- Pathogenic ---
		{
			name: "Pathogenic 1a: PVS1 + PS1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},
		{
			name: "Pathogenic 1b: PVS1 + PM1 + PM2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM2, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},
		{
			name: "Pathogenic 1c: PVS1 + PM1 + PP1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},
		{
			name: "Pathogenic 1d: PVS1 + PP1 + PP2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePP2, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},
		{
			name: "Pathogenic 2: PS1 + PS2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePS2, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},
		{
			name: "Pathogenic 3a: PS1 + PM1 + PM2 + PM3",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM2, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM3, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassPathogenic,
		},

		// --- Likely Pathogenic ---
		{
			name: "Likely Pathogenic 1: PVS1 + PM1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyPathogenic,
		},
		{
			name: "Likely Pathogenic 2: PS1 + PM1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyPathogenic,
		},
		{
			name: "Likely Pathogenic 3: PS1 + PP1 + PP2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePP2, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyPathogenic,
		},
		{
			name: "Likely Pathogenic 4: PM1 + PM2 + PM3",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePM1, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM2, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePM3, EvaluatedStrength: acmg2015.StrengthModerate, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyPathogenic,
		},

		// --- Benign ---
		{
			name: "Benign 1: BA1 stand-alone",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodeBA1, EvaluatedStrength: acmg2015.StrengthStandAlone, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassBenign,
		},
		{
			name: "Benign 2: BS1 + BS2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodeBS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodeBS2, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassBenign,
		},

		// --- Likely Benign ---
		{
			name: "Likely Benign 1: BS1 + BP1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodeBS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodeBP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyBenign,
		},
		{
			name: "Likely Benign 2: BP1 + BP2",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodeBP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodeBP2, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassLikelyBenign,
		},

		// --- Conflict -> VUS ---
		{
			name: "Conflict: PVS1 + PS1 (Pathogenic) AND BA1 (Benign) -> VUS",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePVS1, EvaluatedStrength: acmg2015.StrengthVeryStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodePS1, EvaluatedStrength: acmg2015.StrengthStrong, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
				{Code: acmg2015.CodeBA1, EvaluatedStrength: acmg2015.StrengthStandAlone, Direction: acmg2015.DirectionBenign, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass:  acmg2015.ClassVUS,
			expectConflict: true,
		},

		// --- VUS insufficient ---
		{
			name: "VUS: Only PP1",
			criteria: []acmg2015.AppliedCriterion{
				{Code: acmg2015.CodePP1, EvaluatedStrength: acmg2015.StrengthSupporting, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatusMet},
			},
			expectedClass: acmg2015.ClassVUS,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := c.Combine(tc.criteria)
			if res.Classification != tc.expectedClass {
				t.Fatalf("expected classification %s, got %s (res: %+v)", tc.expectedClass, res.Classification, res)
			}
			if tc.expectConflict && !res.ConflictDetected {
				t.Fatalf("expected conflict to be detected, got false")
			}
		})
	}
}
