package assessment

import "testing"

func TestSVCInferredContextRequiresExperimentalAI(t *testing.T) {
	r := sequenceFixture()
	r.EngineID = "svc_v4"
	r.Mode = "ai_assisted"
	r.Package.Sources = nil
	r.Package.AIContext = &AIContext{}
	r.Context.DiseaseID = ""
	r.Context.Inheritance = ""
	r.Context.SampleType = ""
	r.Context.VariantOrigin = ""
	if Validate(&r) == nil {
		t.Fatal("experimental opt-in bypassed")
	}
	r.Options.AllowExperimental = true
	if err := Validate(&r); err != nil {
		t.Fatal(err)
	}
	if r.Context.Inheritance != "unknown" || r.Context.VariantOrigin != "unknown" {
		t.Fatal("context was fabricated")
	}
	r.Mode = "local"
	if Validate(&r) == nil {
		t.Fatal("local evaluation accepted AI-only context")
	}
}
