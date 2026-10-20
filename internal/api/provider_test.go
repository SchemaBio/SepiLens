package api_test

import (
	"strings"
	"testing"
)

func TestProfilesAreTenantBoundWriteOnlyAndRoleRestricted(t *testing.T) {
	s, close := setupTestServer(t, false)
	defer close()
	h := s.Router()
	body := `{"protocol":"openai_responses","base_url":"https://api.openai.com/v1","model":"synthetic","api_key":"secret-key"}`
	if request(h, "PUT", "/v2/providers/test", body, "test-api-key", "").Code != 403 {
		t.Fatal("profile role bypass")
	}
	w := request(h, "PUT", "/v2/providers/test", body, "admin-key", "")
	if w.Code != 201 && w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/v2/providers/test", "", "admin-key", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-key") {
		t.Fatal("key readable", w.Body.String())
	}
	if request(h, "GET", "/v2/providers/test", "", "other-key", "").Code != 404 {
		t.Fatal("cross tenant profile")
	}
	if request(h, "DELETE", "/v2/providers/test", "", "admin-key", "").Code != 204 {
		t.Fatal("profile deletion")
	}
	if request(h, "PUT", "/v2/providers/bad", strings.Replace(body, "https://api.openai.com/v1", "https://127.0.0.1", 1), "admin-key", "").Code != 422 {
		t.Fatal("private endpoint accepted")
	}
}
