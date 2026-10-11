package api_test

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"strings"
	"testing"
)

func svcInput() string {
	body := strings.Replace(snvInput, "acmg_amp_2015", "svc_v4", 1)
	body = strings.Replace(body, `"options":{`, `"draft_assessment":{"moi":"AR","gene_disease_validity":"STRONG","population":{"faf":"0.000001","daft":"0.0001"},"variant_impact":{"workflow":"nonsense","prediction_outcome":"NMD_NO_RESCUE","initial_points":"6","gencc_mechanism":"ESTABLISHED","exon_relevance":"ALL","informative":[{"id":"iv-1","classification":"PATHOGENIC"}]}},"options":{"allow_experimental":true,`, 1)
	return body
}
func TestSVCExperimentalDeliveryAndNoClinicalPromotion(t *testing.T) {
	for _, simulation := range []bool{false, true} {
		s, close := setupTestServer(t, simulation)
		h := s.Router()
		id := accepted(t, h, svcInput(), "svc-draft")
		res := result(t, h, id)
		if res.ResultKind != "experimental" || res.ClinicalUse != "forbidden" || res.SettlementEligible || res.RuleEngineResult != nil || res.RulesetHash != svcv4.RulesetHash() {
			t.Fatal("experimental clinical/billing gate failed", res)
		}
		raw, _ := json.Marshal(res.AgentProposal)
		var proposal struct {
			Experimental svcv4.DraftResult `json:"experimental_result"`
		}
		if e := json.Unmarshal(raw, &proposal); e != nil {
			t.Fatal(e)
		}
		if proposal.Experimental.UpstreamCommit != svcv4.Commit || proposal.Experimental.Authoritative || proposal.Experimental.InputsVerified || proposal.Experimental.Total == nil || proposal.Experimental.Total.String() != "8" || proposal.Experimental.Classification != "likely_pathogenic" {
			t.Fatal("incorrect draft result", string(raw))
		}
		w := request(h, "POST", "/v1/variants/snv/evaluations", svcInput(), "test-api-key", "svc-draft")
		if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
			t.Fatal("draft replay", w.Code, w.Body.String())
		}
		w = request(h, "POST", "/v1/evaluations/"+id+"/reviews", `{"revision":1,"decision":"accept_engine_result","reason":"reviewed"}`, "review-key", "")
		if w.Code != 409 {
			t.Fatal("draft clinical review gate failed", w.Code, w.Body.String())
		}
		close()
	}
}
func TestSVCScopeAndMissingInputs(t *testing.T) {
	s, close := setupTestServer(t, false)
	defer close()
	h := s.Router()
	for _, body := range []string{strings.Replace(svcInput(), `"allow_experimental":true,`, "", 1), strings.Replace(svcInput(), `"moi":"AR"`, `"moi":"AD"`, 1), strings.Replace(svcInput(), "NMD_NO_RESCUE", "unknown_branch", 1)} {
		w := request(h, "POST", "/v1/variants/snv/evaluations", body, "test-api-key", "")
		if w.Code != 409 && w.Code != 422 {
			t.Fatal("invalid draft admitted", w.Code, w.Body.String())
		}
	}
	body := strings.Replace(svcInput(), "germline_confirmed", "unknown", 1)
	id := accepted(t, h, body, "")
	res := result(t, h, id)
	raw, _ := json.Marshal(res.AgentProposal)
	if res.ResultKind != "evidence_only" || res.SettlementEligible || strings.Contains(string(raw), "experimental_result") {
		t.Fatal("unknown origin draft classified")
	}
}
