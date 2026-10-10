package acmg2015_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestCriteria_CanonicalCompleteness(t *testing.T) {
	if len(acmg2015.CanonicalCriteria) != 28 {
		t.Fatalf("expected 28 standard ACMG 2015 criteria, got %d", len(acmg2015.CanonicalCriteria))
	}

	pathCount := 0
	benignCount := 0
	for _, c := range acmg2015.CanonicalCriteria {
		if c.Direction == acmg2015.DirectionPathogenic {
			pathCount++
		} else if c.Direction == acmg2015.DirectionBenign {
			benignCount++
		}
	}
	if pathCount != 16 {
		t.Fatalf("expected 16 pathogenic criteria, got %d", pathCount)
	}
	if benignCount != 12 {
		t.Fatalf("expected 12 benign criteria, got %d", benignCount)
	}

	pvs1, err := acmg2015.LookupCriterion("PVS1")
	if err != nil || pvs1.DefaultStrength != acmg2015.StrengthVeryStrong || pvs1.Direction != acmg2015.DirectionPathogenic {
		t.Fatalf("unexpected PVS1 definition: %+v", pvs1)
	}

	ba1, err := acmg2015.LookupCriterion("BA1")
	if err != nil || ba1.DefaultStrength != acmg2015.StrengthStandAlone || ba1.Direction != acmg2015.DirectionBenign {
		t.Fatalf("unexpected BA1 definition: %+v", ba1)
	}

	_, errUnknown := acmg2015.LookupCriterion("PX9")
	if !errors.Is(errUnknown, acmg2015.ErrUnknownCriterion) {
		t.Fatalf("expected ErrUnknownCriterion, got %v", errUnknown)
	}
}
