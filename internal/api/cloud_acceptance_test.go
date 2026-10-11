package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

// Synthetic authorization is confined to this package test. It is NOT a medical approval.
func TestMockCloudSubmitProgressReviewSettleDelete(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "acceptance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", AuthMode: "required", RetentionDays: 90, Principals: map[string]config.Principal{
		"submit": {Tenant: "A", Actor: "cloud", Role: "submitter"}, "review": {Tenant: "A", Actor: "doctor", Role: "reviewer"}, "admin": {Tenant: "A", Actor: "ops", Role: "admin"}, "other": {Tenant: "B", Actor: "other", Role: "admin"},
	}}
	s := NewServer(cfg, jobs.NewSQLiteStore(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
	s.approvals = map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic-isolated-test"}}
	if err = s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request := func(method, path, token, body, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		return w
	}
	body := `{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"TEST"},"clinical_context":{"disease_id":"MONDO:TEST","inheritance":"AD","variant_origin":"germline_confirmed","sample_type":"blood","hpo_terms":[]},"options":{"retrieval_mode":"off"}}`
	w := request("POST", "/v1/variants/snv/evaluations", "submit", body, "cloud-1")
	if w.Code != 202 {
		t.Fatalf("admission: %d %s", w.Code, w.Body.String())
	}
	var accepted struct {
		JobID string `json:"job_id"`
	}
	if json.Unmarshal(w.Body.Bytes(), &accepted) != nil || accepted.JobID == "" {
		t.Fatal("missing persisted ID")
	}
	id := accepted.JobID
	deadline := time.Now().Add(5 * time.Second)
	for {
		w = request("GET", "/v1/jobs/"+id+"/result", "submit", "", "")
		if w.Code != 202 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var result EvaluationResultPayload
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.SettlementEligible || result.DeliveryStatus != "delivered" || result.RuleEngineResult == nil {
		t.Fatalf("transactional delivery: %s", w.Body.String())
	}
	if w = request("GET", "/v1/jobs/"+id, "other", "", ""); w.Code != 404 {
		t.Fatal("cross-tenant task access")
	}
	if w = request("POST", "/v1/evaluations/"+id+"/reviews", "submit", `{"revision":1,"decision":"accept_engine_result","reason":"test"}`, ""); w.Code != 403 {
		t.Fatal("submitter became reviewer")
	}
	w = request("GET", "/v1/jobs/"+id+"/events", "submit", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "event: completed") {
		t.Fatalf("completion SSE: %s", w.Body.String())
	}
	if w = request("GET", "/v1/evaluations/"+id+"/evidence", "submit", "", ""); w.Code != 200 {
		t.Fatal("missing snapshot")
	}
	if w = request("POST", "/v1/evaluations/"+id+"/reviews", "review", `{"revision":1,"decision":"accept_engine_result","reason":"synthetic professional review"}`, ""); w.Code != 201 {
		t.Fatalf("review: %s", w.Body.String())
	}
	w = request("GET", "/v1/jobs/"+id+"/result", "submit", "", "")
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.ReviewerDecision == nil || result.ReviewerDecision.ReviewerID != "doctor" || result.ReviewRequired {
		t.Fatal("review not applied")
	}
	if w = request("POST", "/v1/variants/snv/evaluations", "submit", body, "cloud-1"); w.Code != 200 {
		t.Fatal("free replay created invocation")
	}
	if w = request("GET", "/v1/jobs/"+id+"/audit", "review", "", ""); w.Code != 200 {
		t.Fatal("audit export unavailable")
	}
	if w = request(http.MethodDelete, "/v1/tenant/data", "admin", "", ""); w.Code != 204 {
		t.Fatal("tenant delete failed")
	}
	if w = request("GET", "/v1/jobs/"+id, "submit", "", ""); w.Code != 404 {
		t.Fatal("deleted task readable")
	}
}
