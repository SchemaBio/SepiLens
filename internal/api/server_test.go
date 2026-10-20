package api_test

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

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func setupTestServer(t *testing.T, sim bool) (*api.Server, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "api_test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.Config{
		AuthMode:    "required",
		Simulation:  sim,
		MasterKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		KeyToTenant: map[string]string{"test-api-key": "tenant-test"},
		Principals: map[string]config.Principal{
			"test-api-key": {Tenant: "tenant-test", Actor: "submitter", Role: "submitter"},
			"review-key":   {Tenant: "tenant-test", Actor: "reviewer", Role: "reviewer"},
			"admin-key":    {Tenant: "tenant-test", Actor: "admin", Role: "admin"},
			"other-key":    {Tenant: "other-tenant", Actor: "other", Role: "admin"},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := jobs.NewSQLiteStore(db)
	srv := api.NewServer(cfg, store, db, logger)
	srv.SetEvidenceStore(evidence.NewSQLiteStore(db))
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}

	return srv, func() {
		cancel()
		srv.Close()
		_ = db.Close()
	}
}

func TestServer_ProbesAndAuth(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()

	router := srv.Router()

	// 1. /healthz without auth should succeed
	reqHealth := httptest.NewRequest("GET", "/healthz", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz, got %d", recHealth.Code)
	}

	// 2. /readyz without auth should succeed
	reqReady := httptest.NewRequest("GET", "/readyz", nil)
	recReady := httptest.NewRecorder()
	router.ServeHTTP(recReady, reqReady)
	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 for readyz, got %d", recReady.Code)
	}

	// 3. /v2/capabilities without auth should return 401
	reqCapNoAuth := httptest.NewRequest("GET", "/v2/capabilities", nil)
	recCapNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recCapNoAuth, reqCapNoAuth)
	if recCapNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized /v2/capabilities, got %d", recCapNoAuth.Code)
	}
	var errResp api.ErrorEnvelope
	if err := json.Unmarshal(recCapNoAuth.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Error.Code != api.ErrCodeAuthFailed {
		t.Fatalf("expected AUTH_FAILED code, got %s", errResp.Error.Code)
	}

	// 4. /v2/capabilities with valid Bearer token should succeed
	reqCapAuth := httptest.NewRequest("GET", "/v2/capabilities", nil)
	reqCapAuth.Header.Set("Authorization", "Bearer test-api-key")
	recCapAuth := httptest.NewRecorder()
	router.ServeHTTP(recCapAuth, reqCapAuth)
	if recCapAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid key, got %d", recCapAuth.Code)
	}

	var caps api.CapabilitiesResponse
	if err := json.Unmarshal(recCapAuth.Body.Bytes(), &caps); err != nil {
		t.Fatalf("failed to unmarshal capabilities response: %v", err)
	}
	if len(caps.Engines) != 5 {
		t.Fatalf("expected 5 engines, got %d", len(caps.Engines))
	}
	for _, eng := range caps.Engines {
		if eng.ID == "svc_v4" && eng.Enabled {
			t.Fatalf("svc_v4 engine must not be enabled in capabilities")
		}
	}

	// 5. /v2/rulesets
	reqRules := httptest.NewRequest("GET", "/v2/rulesets", nil)
	reqRules.Header.Set("X-API-Key", "test-api-key")
	recRules := httptest.NewRecorder()
	router.ServeHTTP(recRules, reqRules)
	if recRules.Code != http.StatusOK {
		t.Fatalf("expected 200 for rulesets, got %d", recRules.Code)
	}
}

func request(h http.Handler, method, path, body, key, idem string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+key)
	if idem != "" {
		r.Header.Set("Idempotency-Key", idem)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
