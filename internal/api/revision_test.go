package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"github.com/pzweuj/sepilens/internal/workflow"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is an isolated synthetic acceptance test, not a clinical approval or golden validation.
func TestProfessionalRevisionCreatesImmutableHistory(t *testing.T) {
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "revision.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	cfg := config.Config{MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", AuthMode: "required", Principals: map[string]config.Principal{"review": {Tenant: "T", Actor: "R", Role: "reviewer"}}}
	s := NewServer(cfg, jobs.NewSQLiteStore(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
	if e = s.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.queue.Close() // The controlled test fixture owns execution; no polling worker may claim it.
	s.approvals = map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic-rule"}}
	frequency := .1
	in := envelope{SNV: &SNVEvaluationRequest{EngineID: "acmg_amp_2015", Variant: SNVVariant{Gene: "TEST", AlleleFrequency: &frequency}, ClinicalContext: SNVContext{Inheritance: "AD", VariantOrigin: "germline_confirmed", SampleType: "blood"}}, RulesetHash: "synthetic-rule", DatasetHash: "unavailable"}
	b, _ := json.Marshal(in)
	j, _, e := s.queue.Submit(context.Background(), "T", "acmg_amp_2015", "production", "", "hash", b)
	if e != nil {
		t.Fatal(e)
	}
	// Claim before the polling worker and atomically seed a controlled source record.
	if _, e = db.Exec(`UPDATE jobs SET status='running' WHERE job_id=?`, j.JobID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE work_inputs SET owner='test',lease_until='2099-01-01' WHERE job_id=?`, j.JobID); e != nil {
		t.Fatal(e)
	}
	initial := EvaluationResultPayload{EvaluationID: j.JobID, EngineID: j.EngineID, ResultKind: "production", Revision: 1, RuleEngineResult: map[string]any{"classification": "VUS"}, ReviewStatus: "pending"}
	raw, _ := json.Marshal(initial)
	enc, e := s.queue.Cipher.Seal(string(raw), "T:"+j.JobID+":result")
	if e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	sources := []evidence.Source{{ID: "S", Type: evidence.SourceTypePubMed, PMID: "12345", IdentifierValid: true, RecordExists: true}}
	snap := evidence.EvidenceSnapshot{SnapshotID: workflow.ID("snapshot-"), TenantID: "T", EvaluationID: j.JobID, Sources: sources, CreatedAt: time.Now().UTC()}
	snap.Hash, _ = evidence.ComputeSnapshotHash(snap.Sources, snap.Items, snap.Provenance)
	if e = s.queue.Evidence.SaveTx(context.Background(), tx, snap); e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE jobs SET status='succeeded',result_json=? WHERE job_id=?`, enc, j.JobID); e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	facts := map[string]bool{}
	for _, f := range acmg2015.RequiredFacts[acmg2015.CodeBA1] {
		facts[f] = true
	}
	req := AddReviewRequest{Revision: 1, Decision: "revise_evidence", Reason: "Synthetic population material checked", VerifiedSources: []string{"S"}, Corrections: []evidence.EvidenceItem{{ID: "E", EngineID: j.EngineID, RuleCode: "BA1", SuggestedScore: "StandAlone", Status: evidence.StatusMet, Facts: facts, SourceIDs: []string{"S"}}}}
	b, _ = json.Marshal(req)
	r := httptest.NewRequest("POST", "/v1/evaluations/"+j.JobID+"/reviews", strings.NewReader(string(b)))
	r.Header.Set("X-API-Key", "review")
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("revision %d %s", w.Code, w.Body.String())
	}
	updated, e := s.store.GetJob(context.Background(), "T", j.JobID)
	if e != nil {
		t.Fatal(e)
	}
	var result EvaluationResultPayload
	if json.Unmarshal([]byte(updated.ResultJSON), &result) != nil || result.Revision != 2 {
		t.Fatal("revision not advanced")
	}
	rule := result.RuleEngineResult.(map[string]any)
	if rule["classification"] != "Benign" {
		t.Fatalf("professional material not recalculated %+v", rule)
	}
	snapshots, e := s.queue.Evidence.History(context.Background(), "T", j.JobID)
	if e != nil || len(snapshots) != 2 || snapshots[0].Sources[0].MaterialVerified {
		t.Fatal("snapshot overwritten", e)
	}
	var history string
	if e = db.QueryRow(`SELECT payload FROM audit_versions WHERE tenant_id='T' AND evaluation_id=?`, j.JobID).Scan(&history); e != nil || !strings.HasPrefix(history, "enc1:") {
		t.Fatal("history unencrypted", e)
	}
	r = httptest.NewRequest("POST", "/v1/evaluations/"+j.JobID+"/reviews", strings.NewReader(`{"revision":1,"decision":"accept_engine_result","reason":"stale"}`))
	r.Header.Set("X-API-Key", "review")
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("stale acceptance permitted")
	}
}
