package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/profiles"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/validation"
	"github.com/pzweuj/sepilens/internal/workflow"
)

type Server struct {
	cancel          context.CancelFunc
	maintenanceDone chan struct{}
	queue           *workflow.Queue
	profiles        *profiles.Store
	approvals       map[engine.EngineID]engine.Approval
	cfg             config.Config
	store           jobs.Store
	db              *sql.DB
	logger          *slog.Logger
	router          chi.Router
	hpoExtractor    *hpo.Extractor
	evidenceStore   evidence.Store
	coordinator     *retrieval.SearchCoordinator
}

func (s *Server) SetEvidenceStore(es evidence.Store) {
	s.evidenceStore = es
}

func (s *Server) SetSearchCoordinator(sc *retrieval.SearchCoordinator) {
	s.coordinator = sc
}

func NewServer(cfg config.Config, store jobs.Store, db *sql.DB, logger *slog.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		db:     db,
		logger: logger,
	}

	r := chi.NewRouter()
	r.Use(RequestIDMiddleware)
	r.Use(RecovererMiddleware)
	r.Use(LoggingMiddleware(logger))
	r.Use(AuthMiddleware(cfg))

	// Probe endpoints
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	r.Route("/v2", s.routesV2)

	s.router = r
	return s
}

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.db != nil {
		if err := s.db.PingContext(r.Context()); err != nil {
			WriteError(w, r, http.StatusServiceUnavailable, ErrCodeInternalError, "database not ready", true)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ready"})
}

type CapabilitiesResponse struct {
	HPOCandidateExtractionAvailable bool                `json:"hpo_candidate_extraction_available"`
	Version                         string              `json:"version"`
	Engines                         []engine.Descriptor `json:"engines"`
	RetrievalModes                  []string            `json:"retrieval_modes"`
	AIMode                          string              `json:"ai_mode"`
	RuleApprovalRequired            bool                `json:"rule_approval_required"`
	EvaluationModes                 []string            `json:"evaluation_modes"`
	SimulationEnabled               bool                `json:"simulation_enabled"`
	SupportedProtocols              []string            `json:"supported_protocols"`
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	version := s.cfg.Version
	if version == "" {
		version = "0.3.0-dev"
	}
	resp := CapabilitiesResponse{
		Version: version,
		AIMode:  s.cfg.AIMode, RuleApprovalRequired: !s.cfg.AllowUnapprovedAI, EvaluationModes: []string{"ai_assisted"},
		HPOCandidateExtractionAvailable: s.hpoExtractor != nil && s.cfg.AIMode == "enabled",
		Engines:                         s.descriptors(),
		RetrievalModes:                  []string{"off", "targeted", "deep"},
		SimulationEnabled:               s.cfg.Simulation,
		SupportedProtocols:              []string{"openai_responses", "openai_chat"},
	}
	if !s.cfg.AIOnly {
		resp.EvaluationModes = append(resp.EvaluationModes, "local")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRulesets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"rulesets": s.descriptors(),
	})
}
func (s *Server) descriptors() []engine.Descriptor {
	out := engine.All()
	for i := range out {
		if out[i].ID == engine.EngineHPOExtract {
			out[i].AIAvailable = s.hpoExtractor != nil && s.cfg.AIMode == "enabled"
			continue
		}
		out[i].AIAvailable = s.cfg.AIMode == "enabled" && out[i].ID != engine.EngineHPOExtract && (out[i].ID == engine.EngineSVCv4 || s.cfg.AllowsCandidateAI(string(out[i].ID)))
		if _, approved := s.approvals[out[i].ID]; approved && s.cfg.AIMode == "enabled" {
			out[i].AIAvailable = true
		}
		out[i].BillingAvailable = out[i].AIAvailable && (s.cfg.BillACMGAI && out[i].ID == engine.EngineACMG2015 || s.cfg.BillSVCAI && out[i].ID == engine.EngineSVCv4)
		if out[i].AIAvailable && out[i].ID != engine.EngineSVCv4 {
			if content, _, err := validation.Rules(out[i].ID); err == nil {
				out[i].RulesetHash = engine.Hash(content)
			}
		}
		if out[i].ID == engine.EngineSVCv4 {
			continue
		}
		if a, ok := s.approvals[out[i].ID]; ok {
			out[i].Enabled = true
			out[i].Status = engine.StatusEnabled
			out[i].RulesetHash = a.ContentHash
		}
	}
	return out
}
