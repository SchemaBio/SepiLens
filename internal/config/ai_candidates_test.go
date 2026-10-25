package config

import "testing"

func TestCandidateEnginesAreExplicitAndScoped(t *testing.T) {
	t.Setenv("SEPILENS_AUTH_MODE", "disabled")
	t.Setenv("SEPILENS_REQUIRE_RULE_APPROVAL", "true")
	t.Setenv("SEPILENS_AI_CANDIDATE_ENGINES", "acmg_amp_2015")
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	if c.AllowUnapprovedAI || !c.AllowsCandidateAI("acmg_amp_2015") || c.AllowsCandidateAI("cnv_loss_2020") || c.AllowsCandidateAI("cnv_gain_2020") {
		t.Fatal("candidate scope widened")
	}
	t.Setenv("SEPILENS_AI_CANDIDATE_ENGINES", "")
	c, e = Load()
	if e != nil || c.AllowsCandidateAI("acmg_amp_2015") {
		t.Fatal("default candidate gate changed")
	}
	for _, value := range []string{"unknown", "svc_v4", "hpo_extract"} {
		t.Setenv("SEPILENS_AI_CANDIDATE_ENGINES", value)
		if _, e := Load(); e == nil {
			t.Fatal("unsupported candidate scope accepted", value)
		}
	}
}
