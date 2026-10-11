package acmg2015_test

import (
	"github.com/pzweuj/sepilens/internal/evidence"
	a "github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"testing"
)

func TestEvaluatorNeverInfersMaterial(t *testing.T) {
	af := 0.1
	e := a.NewEvaluator(nil)
	for _, v := range []a.SNVVariantInput{{Gene: "CFTR", Consequence: "stop_gained"}, {Consequence: "synonymous_variant"}, {AlleleFrequency: &af}} {
		out := e.Evaluate(v, a.ClinicalContextInput{}, nil)
		if out.Classification != a.ClassVUS || len(out.AppliedCriteria) != 0 || len(out.MissingInformation) == 0 {
			t.Fatalf("unsupported inference: %+v", out)
		}
	}
}

// Synthetic material tests verify prerequisites. These are not clinical golden cases.
func TestAllCriteriaRequireIndependentVerifiedMaterial(t *testing.T) {
	e := a.NewEvaluator(nil)
	src := evidence.Source{ID: "S", ValidationStatus: evidence.ValidationVerified, IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true}
	for code, def := range a.CanonicalCriteria {
		t.Run(string(code), func(t *testing.T) {
			facts := map[string]bool{}
			for _, f := range a.RequiredFacts[code] {
				facts[f] = true
			}
			crit := a.AppliedCriterion{Code: code, Direction: def.Direction, EvaluatedStrength: def.DefaultStrength, Status: a.CriterionStatusMet, SourceIDs: []string{"S"}, Facts: facts}
			frequency := .001
			if code == a.CodeBA1 || code == a.CodeBS1 {
				frequency = .1
			}
			evaluate := func(s []evidence.Source) a.EvaluationOutput {
				return e.EvaluateEvidence(a.SNVVariantInput{AlleleFrequency: &frequency}, a.ClinicalContextInput{}, []a.AppliedCriterion{crit}, s)
			}
			if len(evaluate([]evidence.Source{src}).AppliedCriteria) != 1 {
				t.Fatal("complete material rejected")
			}
			if code == a.CodePM2 || code == a.CodeBA1 || code == a.CodeBS1 {
				if missing := e.EvaluateEvidence(a.SNVVariantInput{}, a.ClinicalContextInput{}, []a.AppliedCriterion{crit}, []evidence.Source{src}); len(missing.AppliedCriteria) != 0 {
					t.Fatal("missing AF accepted despite alleged fact")
				}
			}
			if len(evaluate(nil).AppliedCriteria) != 0 {
				t.Fatal("missing source accepted")
			}
			broken := src
			broken.MaterialVerified = false
			if len(evaluate([]evidence.Source{broken}).AppliedCriteria) != 0 {
				t.Fatal("unverified material accepted")
			}
			for _, f := range a.RequiredFacts[code] {
				facts[f] = false
				if len(evaluate([]evidence.Source{src}).AppliedCriteria) != 0 {
					t.Fatalf("missing %s accepted", f)
				}
				facts[f] = true
			}
		})
	}
}
