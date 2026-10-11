package api_test

import (
	"strings"
	"testing"
)

func TestEvidenceSnapshotAndReviewRoles(t *testing.T) {
	s, close := setupTestServer(t, true)
	defer close()
	h := s.Router()
	id := accepted(t, h, snvInput, "")
	_ = result(t, h, id)
	w := request(h, "GET", "/v1/evaluations/"+id+"/evidence", "", "test-api-key", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "hash") {
		t.Fatal("snapshot missing", w.Code, w.Body.String())
	}
	path := "/v1/evaluations/" + id + "/reviews"
	body := `{"decision":"reject","reason":"synthetic test"}`
	if request(h, "POST", path, body, "test-api-key", "").Code != 403 {
		t.Fatal("submitter reviewed")
	}
	if request(h, "POST", path, `{"reviewer_id":"admin","decision":"reject","reason":"test"}`, "review-key", "").Code != 403 {
		t.Fatal("reviewer impersonation")
	}
	if request(h, "POST", path, `{"decision":"accept_engine_result","reason":"test"}`, "review-key", "").Code != 409 {
		t.Fatal("simulation accepted")
	}
	if request(h, "POST", path, body, "review-key", "").Code != 201 {
		t.Fatal("rejection failed")
	}
	if request(h, "GET", "/v1/jobs/"+id+"/audit", "", "admin-key", "").Code != 200 {
		t.Fatal("audit export failed")
	}
	if request(h, "DELETE", "/v1/jobs/"+id, "", "other-key", "").Code != 404 {
		t.Fatal("cross tenant deletion")
	}
	if request(h, "DELETE", "/v1/jobs/"+id, "", "admin-key", "").Code != 204 {
		t.Fatal("deletion failed")
	}
	if request(h, "GET", "/v1/jobs/"+id, "", "test-api-key", "").Code != 404 {
		t.Fatal("deleted job retained")
	}
}
