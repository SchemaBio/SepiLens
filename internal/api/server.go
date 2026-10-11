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
	"github.com/pzweuj/sepilens/internal/knowledge"
	"github.com/pzweuj/sepilens/internal/profiles"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"github.com/pzweuj/sepilens/internal/workflow"
)

type Server struct {
	cancel          context.CancelFunc
	maintenanceDone chan struct{}
	references      map[string]*knowledge.Reference
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
	acmgEvaluator   *acmg2015.Evaluator
	cnvLossEngine   *cnvloss2020.Engine
	cnvGainEngine   *cnvgain2020.Engine
	cnvAnnotator    *cnv.Annotator
}

func (s *Server) SetReferences(refs map[string]*knowledge.Reference) { s.references = refs }

func (s *Server) SetHPOExtractor(ext *hpo.Extractor) {
	s.hpoExtractor = ext
}

func (s *Server) SetEvidenceStore(es evidence.Store) {
	s.evidenceStore = es
}

func (s *Server) SetSearchCoordinator(sc *retrieval.SearchCoordinator) {
	s.coordinator = sc
}

func (s *Server) SetACMGEvaluator(ev *acmg2015.Evaluator) {
	s.acmgEvaluator = ev
}

func (s *Server) SetCNVEngines(loss *cnvloss2020.Engine, gain *cnvgain2020.Engine, ann *cnv.Annotator) {
	s.cnvLossEngine = loss
	s.cnvGainEngine = gain
	s.cnvAnnotator = ann
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

	// API v1 routes
	r.Route("/v1", func(r chi.Router) {
		r.Get("/capabilities", s.handleCapabilities)
		r.Get("/rulesets", s.handleRulesets)
		r.Delete("/tenant/data", s.handleDeleteTenantData)
		r.Post("/providers/test", s.handleProviderTest)
		r.Get("/providers/{profile_id}", s.handleProfileGet)
		r.Put("/providers/{profile_id}", s.handleProfileSave)
		r.Delete("/providers/{profile_id}", s.handleProfileDelete)
		r.Post("/hpo/extract", s.handleHPOExtract)

		r.Route("/variants", func(r chi.Router) {
			r.Post("/snv/evaluations", s.handleSNVEvaluation)
			r.Post("/cnv/evaluations", s.handleCNVEvaluation)
		})

		r.Route("/jobs", func(r chi.Router) {
			r.Get("/{job_id}", s.handleGetJob)
			r.Get("/{job_id}/events", s.handleJobEvents)
			r.Get("/{job_id}/result", s.handleJobResult)
			r.Post("/{job_id}/cancel", s.handleCancelJob)
			r.Delete("/{job_id}", s.handleDeleteJob)
			r.Get("/{job_id}/audit", s.handleExportJob)
		})

		r.Post("/evaluations/{id}/reviews", s.handleAddReview)
		r.Get("/evaluations/{id}/evidence", s.handleGetEvidence)
	})

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
	SimulationEnabled               bool                `json:"simulation_enabled"`
	SupportedProtocols              []string            `json:"supported_protocols"`
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	resp := CapabilitiesResponse{
		Version:                         "0.2.0-rc2",
		HPOCandidateExtractionAvailable: s.hpoExtractor != nil,
		Engines:                         s.descriptors(),
		RetrievalModes:                  []string{"off", "targeted", "deep"},
		SimulationEnabled:               s.cfg.Simulation,
		SupportedProtocols:              []string{"openai_responses", "openai_chat"},
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
