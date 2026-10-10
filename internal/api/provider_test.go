package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func TestServer_ProviderTest_SecurityAndCapabilities(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "provider_test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := config.Config{
		AuthMode:              "required",
		AllowPrivateEndpoints: false, // strictly public
		KeyToTenant:           map[string]string{"test-api-key": "tenant-test"},
	}
	store := jobs.NewSQLiteStore(db)
	srv := api.NewServer(cfg, store, db, logger)
	router := srv.Router()

	authHeader := "Bearer test-api-key"
	secretKey := "sk-super-secret-key-xyz-12345"

	// 1. Unsupported protocol (e.g. anthropic_messages) -> 422 PROVIDER_CAPABILITY_MISMATCH
	badProtoReq := map[string]any{
		"protocol": "anthropic_messages",
		"base_url": "https://api.anthropic.com/v1",
		"model":    "claude-3-5-sonnet",
		"api_key":  secretKey,
	}
	body, _ := json.Marshal(badProtoReq)
	req := httptest.NewRequest("POST", "/v1/providers/test", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unsupported protocol, got %d", rec.Code)
	}
	var errResp api.ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeProviderCapabilityMismatch {
		t.Fatalf("expected PROVIDER_CAPABILITY_MISMATCH, got %s", errResp.Error.Code)
	}

	// 2. SSRF cloud metadata endpoint -> 422
	ssrfReq := map[string]any{
		"protocol": "openai_responses",
		"base_url": "https://169.254.169.254/latest/meta-data",
		"model":    "gpt-4o",
		"api_key":  secretKey,
	}
	body, _ = json.Marshal(ssrfReq)
	req = httptest.NewRequest("POST", "/v1/providers/test", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for SSRF metadata address, got %d", rec.Code)
	}

	// 3. Valid public HTTPS endpoint -> 200
	validReq := map[string]any{
		"protocol": "openai_responses",
		"base_url": "https://api.openai.com/v1",
		"model":    "gpt-4o",
		"api_key":  secretKey,
	}
	body, _ = json.Marshal(validReq)
	req = httptest.NewRequest("POST", "/v1/providers/test", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid provider check, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Critical privacy check: ensure secretKey was NEVER written to logs!
	loggedText := logBuf.String()
	if strings.Contains(loggedText, secretKey) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: secret key leaked into service logs: %s", loggedText)
	}
}
