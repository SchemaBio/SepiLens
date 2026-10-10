package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

func TestServer_HPOExtract_Endpoints(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()
	router := srv.Router()

	kDir := filepath.Join("..", "..", "knowledge", "hpo")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load hpo index: %v", err)
	}

	extractor := hpo.NewExtractor(idx, nil)
	srv.SetHPOExtractor(extractor)

	authHeader := "Bearer test-api-key"

	// 1. Empty text -> 400 Bad Request
	emptyReq := map[string]any{"text": ""}
	emptyBody, _ := json.Marshal(emptyReq)
	reqEmpty := httptest.NewRequest("POST", "/v1/hpo/extract", bytes.NewReader(emptyBody))
	reqEmpty.Header.Set("Authorization", authHeader)
	recEmpty := httptest.NewRecorder()
	router.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty text, got %d", recEmpty.Code)
	}

	// 2. Synchronous JSON extraction
	extractReq := map[string]any{"text": "患者头晕，眩晕，无头痛"}
	body, _ := json.Marshal(extractReq)
	req := httptest.NewRequest("POST", "/v1/hpo/extract", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for hpo extract, got %d: %s", rec.Code, rec.Body.String())
	}

	var res hpo.ExtractionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal extraction result: %v", err)
	}
	if len(res.Terms) == 0 {
		t.Fatalf("expected terms in extraction result, got 0")
	}

	// 3. TSV export format
	reqTSV := httptest.NewRequest("POST", "/v1/hpo/extract?format=tsv", bytes.NewReader(body))
	reqTSV.Header.Set("Authorization", authHeader)
	recTSV := httptest.NewRecorder()
	router.ServeHTTP(recTSV, reqTSV)
	if recTSV.Code != http.StatusOK {
		t.Fatalf("expected 200 for TSV format, got %d", recTSV.Code)
	}
	tsvStr := recTSV.Body.String()
	if !strings.HasPrefix(tsvStr, "HPO_ID\tname\tname_cn\tconfidence\tremark") {
		t.Fatalf("expected TSV header, got: %s", tsvStr)
	}

	// 4. SSE streaming extraction
	reqSSE := httptest.NewRequest("POST", "/v1/hpo/extract?stream=true", bytes.NewReader(body))
	reqSSE.Header.Set("Authorization", authHeader)
	recSSE := httptest.NewRecorder()
	router.ServeHTTP(recSSE, reqSSE)
	if recSSE.Code != http.StatusOK {
		t.Fatalf("expected 200 for SSE format, got %d", recSSE.Code)
	}
	sseStr := recSSE.Body.String()
	if !strings.Contains(sseStr, "event: searching") || !strings.Contains(sseStr, "event: data") {
		t.Fatalf("expected searching and data SSE events, got: %s", sseStr)
	}
}
