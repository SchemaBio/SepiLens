package negative_security_test

import (
	"bytes"
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
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func setupSecurityServer(t *testing.T, logBuf *bytes.Buffer) (*api.Server, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "security_test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	cfg := config.Config{
		AuthMode:              "required",
		Simulation:            true,
		MasterKey:             "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		Principals:            map[string]config.Principal{"key-tenant-alpha": {Tenant: "tenant-alpha", Actor: "alpha", Role: "admin"}, "key-tenant-beta": {Tenant: "tenant-beta", Actor: "beta", Role: "admin"}},
		AllowPrivateEndpoints: false, // strict public check
		KeyToTenant: map[string]string{
			"key-tenant-alpha": "tenant-alpha",
			"key-tenant-beta":  "tenant-beta",
		},
	}

	var handler slog.Handler
	if logBuf != nil {
		handler = slog.NewTextHandler(logBuf, nil)
	} else {
		handler = slog.NewTextHandler(io.Discard, nil)
	}
	logger := slog.New(handler)

	store := jobs.NewSQLiteStore(db)
	evidenceStore := evidence.NewSQLiteStore(db)
	acmgEval := acmg2015.NewEvaluator(nil)
	loss := cnvloss2020.NewEngine()
	gain := cnvgain2020.NewEngine()
	ann := cnv.NewAnnotator()

	srv := api.NewServer(cfg, store, db, logger)
	srv.SetEvidenceStore(evidenceStore)
	srv.SetACMGEvaluator(acmgEval)
	srv.SetCNVEngines(loss, gain, ann)

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

func TestSecurity_MalformedCoordinatesAndBounds(t *testing.T) {
	srv, teardown := setupSecurityServer(t, nil)
	defer teardown()
	router := srv.Router()
	authHeader := "Bearer key-tenant-alpha"

	tests := []struct {
		name         string
		endpoint     string
		payload      map[string]any
		expectedCode int
		expectedErr  api.ErrorCode
	}{
		{
			name:     "CNV start > end inverted interval",
			endpoint: "/v1/variants/cnv/evaluations",
			payload: map[string]any{
				"engine_id": "cnv_loss_2020",
				"cnv": map[string]any{
					"assembly":          "GRCh38",
					"chrom":             "7",
					"start":             20000000,
					"end":               10000000, // inverted!
					"coordinate_system": "1-based-inclusive",
					"type":              "loss",
				},
				"clinical_context": map[string]any{
					"variant_origin": "germline_confirmed", "sample_type": "blood",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeInvalidVariant,
		},
		{
			name:     "CNV negative start position",
			endpoint: "/v1/variants/cnv/evaluations",
			payload: map[string]any{
				"engine_id": "cnv_loss_2020",
				"cnv": map[string]any{
					"assembly":          "GRCh38",
					"chrom":             "7",
					"start":             -500,
					"end":               10000,
					"coordinate_system": "1-based-inclusive",
					"type":              "loss",
				},
				"clinical_context": map[string]any{
					"variant_origin": "germline_confirmed", "sample_type": "blood",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeInvalidVariant,
		},
		{
			name:     "CNV unsupported assembly",
			endpoint: "/v1/variants/cnv/evaluations",
			payload: map[string]any{
				"engine_id": "cnv_loss_2020",
				"cnv": map[string]any{
					"assembly":          "NCBI34",
					"chrom":             "7",
					"start":             1000,
					"end":               5000,
					"coordinate_system": "1-based-inclusive",
					"type":              "loss",
				},
				"clinical_context": map[string]any{
					"variant_origin": "germline_confirmed", "sample_type": "blood",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeAssemblyMismatch,
		},
		{
			name:     "SNV negative position",
			endpoint: "/v1/variants/snv/evaluations",
			payload: map[string]any{
				"engine_id": "acmg_amp_2015",
				"variant": map[string]any{
					"assembly": "GRCh38",
					"chrom":    "chr7",
					"pos":      -10,
					"ref":      "A",
					"alt":      "G",
					"gene":     "CFTR",
				},
				"clinical_context": map[string]any{
					"variant_origin": "unknown", "sample_type": "blood",
					"disease_id":  "MONDO:0009061",
					"inheritance": "AR",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeInvalidVariant,
		},
		{
			name:     "SNV empty ref/alt allele",
			endpoint: "/v1/variants/snv/evaluations",
			payload: map[string]any{
				"engine_id": "acmg_amp_2015",
				"variant": map[string]any{
					"assembly": "GRCh38",
					"chrom":    "chr7",
					"pos":      1000,
					"ref":      "",
					"alt":      "",
					"gene":     "CFTR",
				},
				"clinical_context": map[string]any{
					"variant_origin": "unknown", "sample_type": "blood",
					"disease_id":  "MONDO:0009061",
					"inheritance": "AR",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeInvalidVariant,
		},
		{
			name:     "Somatic confirmed CNV strictly rejected",
			endpoint: "/v1/variants/cnv/evaluations",
			payload: map[string]any{
				"engine_id": "cnv_loss_2020",
				"cnv": map[string]any{
					"assembly":          "GRCh38",
					"chrom":             "10",
					"start":             87000000,
					"end":               88000000,
					"coordinate_system": "1-based-inclusive",
					"type":              "loss",
				},
				"clinical_context": map[string]any{
					"variant_origin": "somatic_confirmed", "sample_type": "blood",
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
			expectedErr:  api.ErrCodeUnsupportedVariantOrigin,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest("POST", tc.endpoint, bytes.NewReader(b))
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.expectedCode {
				t.Fatalf("expected HTTP %d, got %d (body: %s)", tc.expectedCode, rec.Code, rec.Body.String())
			}
			var errEnvelope api.ErrorEnvelope
			_ = json.Unmarshal(rec.Body.Bytes(), &errEnvelope)
			if errEnvelope.Error.Code != tc.expectedErr {
				t.Fatalf("expected error code %s, got %s", tc.expectedErr, errEnvelope.Error.Code)
			}
		})
	}
}

func TestSecurity_CrossTenantIsolation_IDOR(t *testing.T) {
	srv, teardown := setupSecurityServer(t, nil)
	defer teardown()
	router := srv.Router()

	// Tenant Alpha creates an evaluation job
	snvPayload := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117548628,
			"ref":      "G",
			"alt":      "T",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"variant_origin": "unknown", "sample_type": "blood",
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	body, _ := json.Marshal(snvPayload)
	reqAlpha := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	reqAlpha.Header.Set("Authorization", "Bearer key-tenant-alpha")
	recAlpha := httptest.NewRecorder()
	router.ServeHTTP(recAlpha, reqAlpha)

	var alphaResp map[string]any
	_ = json.Unmarshal(recAlpha.Body.Bytes(), &alphaResp)
	jobID := alphaResp["job_id"].(string)

	// Tenant Beta tries to access Tenant Alpha's job -> 404
	reqBetaJob := httptest.NewRequest("GET", "/v1/jobs/"+jobID, nil)
	reqBetaJob.Header.Set("Authorization", "Bearer key-tenant-beta")
	recBetaJob := httptest.NewRecorder()
	router.ServeHTTP(recBetaJob, reqBetaJob)
	if recBetaJob.Code != http.StatusNotFound {
		t.Fatalf("CRITICAL IDOR: Tenant Beta could read Tenant Alpha's job (HTTP %d)", recBetaJob.Code)
	}

	// Tenant Beta tries to read evidence snapshot -> 404
	reqBetaEvidence := httptest.NewRequest("GET", "/v1/evaluations/"+jobID+"/evidence", nil)
	reqBetaEvidence.Header.Set("Authorization", "Bearer key-tenant-beta")
	recBetaEvidence := httptest.NewRecorder()
	router.ServeHTTP(recBetaEvidence, reqBetaEvidence)
	if recBetaEvidence.Code != http.StatusNotFound {
		t.Fatalf("CRITICAL IDOR: Tenant Beta could read Tenant Alpha's evidence (HTTP %d)", recBetaEvidence.Code)
	}

	// Tenant Beta tries to post review to Tenant Alpha's evaluation -> 404
	reviewPayload := map[string]any{
		"reviewer_id": "malicious-actor",
		"decision":    "override",
		"reason":      "Unauthorized override attempt",
	}
	bRev, _ := json.Marshal(reviewPayload)
	reqBetaReview := httptest.NewRequest("POST", "/v1/evaluations/"+jobID+"/reviews", bytes.NewReader(bRev))
	reqBetaReview.Header.Set("Authorization", "Bearer key-tenant-beta")
	recBetaReview := httptest.NewRecorder()
	router.ServeHTTP(recBetaReview, reqBetaReview)
	if recBetaReview.Code != http.StatusNotFound {
		t.Fatalf("CRITICAL IDOR: Tenant Beta could review Tenant Alpha's evaluation (HTTP %d)", recBetaReview.Code)
	}
}

func TestSecurity_SSRF_MetadataAndPrivateIPsBlocked(t *testing.T) {
	srv, teardown := setupSecurityServer(t, nil)
	defer teardown()
	router := srv.Router()

	ssrfTargets := []string{
		"https://169.254.169.254/latest/meta-data",
		"https://169.254.169.254/computeMetadata/v1",
		"https://metadata.google.internal/computeMetadata/v1",
		"https://127.0.0.1:8080/internal",
		"https://localhost:9000/keys",
		"https://10.0.0.1/admin",
		"https://192.168.1.1/router",
		"http://169.254.169.254/",
	}

	for _, target := range ssrfTargets {
		payload := map[string]any{
			"protocol": "openai_responses",
			"base_url": target,
			"model":    "gpt-4o",
			"api_key":  "sk-test-secret-1234",
		}
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/v1/providers/test", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer key-tenant-alpha")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("CRITICAL SSRF VULNERABILITY: endpoint %s was not blocked (HTTP %d)", target, rec.Code)
		}
	}
}

func TestSecurity_APIKeyLeakageProtectionInLogs(t *testing.T) {
	var logBuf bytes.Buffer
	srv, teardown := setupSecurityServer(t, &logBuf)
	defer teardown()
	router := srv.Router()

	secretKey := "sk-super-secret-production-token-998877"

	testPayload := map[string]any{
		"protocol": "openai_responses",
		"base_url": "https://api.openai.com/v1",
		"model":    "gpt-4o",
		"api_key":  secretKey,
	}
	b, _ := json.Marshal(testPayload)
	req := httptest.NewRequest("POST", "/v1/providers/test", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+secretKey)
	req.Header.Set("X-API-Key", secretKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	logOutput := logBuf.String()
	if strings.Contains(logOutput, secretKey) {
		t.Fatalf("CRITICAL CREDENTIAL LEAK: secret key found in server logs:\n%s", logOutput)
	}
}
