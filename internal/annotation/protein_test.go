package annotation_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/annotation"
)

func TestSameAminoAcid(t *testing.T) {
	if !annotation.SameAminoAcid("p.Phe508del", "p.F508del") {
		t.Fatal("three-letter and one-letter forms must match")
	}
	if annotation.SameAminoAcid("p.Phe508del", "p.Phe508Cys") {
		t.Fatal("different substitutions matched")
	}
	if annotation.SameAminoAcid("", "p.Phe508del") {
		t.Fatal("empty value matched")
	}
}

func TestSameResidue(t *testing.T) {
	position, same := annotation.SameResidue("p.Val600Glu", "p.V600Lys")
	if !same || position != "600" {
		t.Fatalf("same residue %q %v", position, same)
	}
	if _, same = annotation.SameResidue("p.Val600Glu", "p.Val601Glu"); same {
		t.Fatal("different residues matched")
	}
	if _, same = annotation.SameResidue("p.Val600Glu", "p.Val600Glu"); same {
		t.Fatal("identical changes must not satisfy PM5")
	}
}

func TestReviewStarsGate(t *testing.T) {
	cv := &annotation.ClinVar{ReviewStatus: "criteria_provided,_single_submitter"}
	if cv.ReviewStars() >= annotation.MinReviewStarsForRelativeCriterion {
		t.Fatal("single submitter passed the PS1/PM5 gate")
	}
	cv.ReviewStatus = "criteria_provided,_multiple_submitters,_no_conflicts"
	if cv.ReviewStars() < annotation.MinReviewStarsForRelativeCriterion {
		t.Fatal("multi-submitter failed the PS1/PM5 gate")
	}
}
