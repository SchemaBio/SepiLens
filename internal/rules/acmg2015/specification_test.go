package acmg2015_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestSpecificationRegistry_Resolve(t *testing.T) {
	reg := acmg2015.NewSpecificationRegistry()
	if reg.Resolve("CFTR", "MONDO:0009061", "AR").IsVCEP {
		t.Fatal("unapproved default VCEP")
	}
	reg.Register(acmg2015.CFTR_VCEP)

	// 1. CFTR VCEP exact match
	cftrSpec := reg.Resolve("CFTR", "MONDO:0009061", "AR")
	if !cftrSpec.IsVCEP || cftrSpec.ID != "clingen_cftr_vcep_v1" {
		t.Fatalf("expected CFTR VCEP spec, got %+v", cftrSpec)
	}
	if cftrSpec.PM2Strength != acmg2015.StrengthSupporting {
		t.Fatalf("expected CFTR VCEP PM2 to be Supporting strength, got %s", cftrSpec.PM2Strength)
	}

	// 2. Unknown gene falls back to Richards 2015 baseline
	unknownSpec := reg.Resolve("UNKNOWN_GENE", "MONDO:0000001", "AD")
	if unknownSpec.IsVCEP || unknownSpec.ID != "richards_2015_baseline" {
		t.Fatalf("expected Richards 2015 baseline for unknown gene, got %+v", unknownSpec)
	}
	if unknownSpec.PM2Strength != acmg2015.StrengthModerate {
		t.Fatalf("expected baseline PM2 to be Moderate, got %s", unknownSpec.PM2Strength)
	}

	// 3. Register custom VCEP
	custom := acmg2015.Specification{
		ID:          "vcep_custom_test",
		Gene:        "MYH7",
		DiseaseID:   "MONDO:0005045",
		Inheritance: "AD",
		IsVCEP:      true,
	}
	reg.Register(custom)

	resolvedCustom := reg.Resolve("MYH7", "MONDO:0005045", "AD")
	if resolvedCustom.ID != "vcep_custom_test" || !resolvedCustom.IsVCEP {
		t.Fatalf("expected custom VCEP, got %+v", resolvedCustom)
	}
}
