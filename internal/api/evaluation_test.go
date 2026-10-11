package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const snvInput = `{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38","chrom":"7","pos":100,"ref":"A","alt":"G","gene":"TEST"},"clinical_context":{"variant_origin":"germline_confirmed","sample_type":"blood","inheritance":"AR","disease_id":"MONDO:1"},"options":{"retrieval_mode":"off"}}`
const cnvInput = `{"engine_id":"cnv_loss_2020","cnv":{"assembly":"GRCh38","chrom":"1","start":100,"end":200,"type":"loss","coordinate_system":"1-based-inclusive"},"clinical_context":{"variant_origin":"germline_confirmed","sample_type":"blood"},"options":{"retrieval_mode":"off"}}`

func request(h http.Handler, method, path, body, key, idem string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-API-Key", key)
	if idem != "" {
		r.Header.Set("Idempotency-Key", idem)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func accepted(t *testing.T, h http.Handler, body, idem string) string {
	t.Helper()
	w := request(h, "POST", "/v1/variants/snv/evaluations", body, "test-api-key", idem)
	if w.Code != 202 {
		t.Fatalf("admission %d: %s", w.Code, w.Body.String())
	}
	var j struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &j); e != nil || j.Status != "queued" || j.JobID == "" {
		t.Fatal("not queued", w.Body.String())
	}
	return j.JobID
}
func result(t *testing.T, h http.Handler, id string) api.EvaluationResultPayload {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		w := request(h, "GET", "/v1/jobs/"+id+"/result", "", "test-api-key", "")
		if w.Code == 200 {
			var v api.EvaluationResultPayload
			if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
				t.Fatal(e)
			}
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("result timeout")
	return api.EvaluationResultPayload{}
}
func TestConcurrentIdempotencyAndAsyncResult(t *testing.T) {
	s, close := setupTestServer(t, true)
	defer close()
	h := s.Router()
	id := accepted(t, h, snvInput, "same")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(h, "POST", "/v1/variants/snv/evaluations", snvInput, "test-api-key", "same")
			if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(id)) {
				t.Error("replay mismatch", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	w := request(h, "POST", "/v1/variants/snv/evaluations", strings.Replace(snvInput, `"pos":100`, `"pos":101`, 1), "test-api-key", "same")
	if w.Code != 409 {
		t.Fatal("mismatched replay accepted", w.Code)
	}
	v := result(t, h, id)
	if v.SettlementEligible || v.RuleEngineResult != nil || v.ResultKind != "simulation" {
		t.Fatal("simulation classified or billed")
	}
}
func TestSSEReplayAndTenantIsolation(t *testing.T) {
	s, close := setupTestServer(t, true)
	defer close()
	h := s.Router()
	id := accepted(t, h, snvInput, "")
	_ = result(t, h, id)
	for _, path := range []string{"", "/result", "/events", "/audit"} {
		w := request(h, "GET", "/v1/jobs/"+id+path, "", "other-key", "")
		if w.Code != 404 {
			t.Fatalf("cross tenant %s: %d", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/v1/jobs/"+id+"/events", nil)
	r.Header.Set("X-API-Key", "test-api-key")
	r.Header.Set("Last-Event-ID", "1")
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(ctx))
	if strings.Contains(w.Body.String(), "id: 1\n") || !strings.Contains(w.Body.String(), "completed") {
		t.Fatal("SSE replay", w.Body.String())
	}
}
func TestStrictAdmission(t *testing.T) {
	s, close := setupTestServer(t, true)
	defer close()
	h := s.Router()
	cases := []struct {
		body   string
		status int
	}{{strings.Replace(snvInput, "germline_confirmed", "somatic", 1), 422}, {strings.Replace(snvInput, "germline_confirmed", "invalid", 1), 422}, {strings.Replace(snvInput, `"ref":"A"`, `"ref":"Z"`, 1), 422}, {strings.Replace(snvInput, `"gene":"TEST"`, `"gene":"TEST","allele_frequency":1.1`, 1), 422}, {strings.Replace(snvInput, `"options":`, `"unknown":true,"options":`, 1), 400}, {snvInput + `{}`, 400}, {strings.Replace(snvInput, "acmg_amp_2015", "svc_v4", 1), 409}}
	for _, c := range cases {
		w := request(h, "POST", "/v1/variants/snv/evaluations", c.body, "test-api-key", "")
		if w.Code != c.status {
			t.Fatalf("want %d got %d %s", c.status, w.Code, w.Body.String())
		}
	}
}
