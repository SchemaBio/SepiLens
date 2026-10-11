package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
)

type SNVEvaluationRequest struct {
	DraftAssessment *svcv4.DraftAssessment  `json:"draft_assessment,omitempty"`
	Evidence        []evidence.EvidenceItem `json:"evidence,omitempty"`
	Sources         []evidence.Source       `json:"sources,omitempty"`
	EngineID        string                  `json:"engine_id"`
	Variant         SNVVariant              `json:"variant"`
	ClinicalContext SNVContext              `json:"clinical_context"`
	Options         EvaluationOptions       `json:"options"`
}

type SNVVariant struct {
	Assembly        string   `json:"assembly"`
	Chrom           string   `json:"chrom"`
	Pos             int64    `json:"pos"`
	Ref             string   `json:"ref"`
	Alt             string   `json:"alt"`
	Transcript      string   `json:"transcript,omitempty"`
	HGVSc           string   `json:"hgvs_c,omitempty"`
	HGVSp           string   `json:"hgvs_p,omitempty"`
	Gene            string   `json:"gene"`
	Consequence     string   `json:"consequence,omitempty"`
	AlleleFrequency *float64 `json:"allele_frequency,omitempty"`
}

type SNVContext struct {
	DiseaseID     string   `json:"disease_id"`
	Inheritance   string   `json:"inheritance"`
	VariantOrigin string   `json:"variant_origin"`
	SampleType    string   `json:"sample_type"`
	HPOTerms      []string `json:"hpo_terms"`
}

type CNVEvaluationRequest struct {
	Evidence        []evidence.EvidenceItem `json:"evidence,omitempty"`
	Sources         []evidence.Source       `json:"sources,omitempty"`
	EngineID        string                  `json:"engine_id"`
	CNV             CNVRegion               `json:"cnv"`
	ClinicalContext CNVContext              `json:"clinical_context"`
	Options         EvaluationOptions       `json:"options"`
}

type CNVRegion struct {
	Orientation         string `json:"orientation,omitempty"`
	Assembly            string `json:"assembly"`
	Chrom               string `json:"chrom"`
	Start               int64  `json:"start"`
	End                 int64  `json:"end"`
	CoordinateSystem    string `json:"coordinate_system"`
	Type                string `json:"type"` // "loss" or "gain"
	CopyNumber          int    `json:"copy_number,omitempty"`
	BreakpointPrecision string `json:"breakpoint_precision,omitempty"`
	Mosaicism           string `json:"mosaicism,omitempty"`
}

type CNVContext struct {
	VariantOrigin string   `json:"variant_origin"`
	SampleType    string   `json:"sample_type"`
	DiseaseID     string   `json:"disease_id,omitempty"`
	HPOTerms      []string `json:"hpo_terms"`
}

type EvaluationOptions struct {
	AllowExperimental  bool   `json:"allow_experimental,omitempty"`
	AllowLocalFallback bool   `json:"allow_local_fallback,omitempty"`
	RetrievalMode      string `json:"retrieval_mode,omitempty"`
	WebSearch          *bool  `json:"web_search,omitempty"`
	ProviderProfileID  string `json:"provider_profile_id,omitempty"`
}

func resolveRetrievalMode(opts EvaluationOptions) retrieval.Mode {
	if opts.WebSearch != nil {
		if !*opts.WebSearch {
			return retrieval.ModeOff
		}
		if opts.RetrievalMode == string(retrieval.ModeDeep) {
			return retrieval.ModeDeep
		}
		return retrieval.ModeTargeted
	}
	if opts.RetrievalMode != "" {
		return retrieval.Mode(opts.RetrievalMode)
	}
	return retrieval.ModeOff
}

type HPOExtractRequest struct {
	Text    string            `json:"text"`
	Options EvaluationOptions `json:"options"`
}

func validateSNVAdmission(req *SNVEvaluationRequest) (status int, code ErrorCode, msg string, ok bool) {
	if req.EngineID == "" {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, "missing engine_id", false
	}
	desc, err := engine.Lookup(engine.EngineID(req.EngineID))
	if err != nil {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, fmt.Sprintf("unrecognized engine_id: %s", req.EngineID), false
	}

	if desc.ID == engine.EngineSVCv4 && !req.Options.AllowExperimental {
		return http.StatusConflict, ErrCodeRulesetNotAvailable, "SVC v4 requires explicit allow_experimental; clinical classification remains unavailable", false
	}
	if desc.ID != engine.EngineACMG2015 && desc.ID != engine.EngineSVCv4 {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, fmt.Sprintf("engine %s is not applicable for SNV/InDel evaluation", req.EngineID), false
	}

	asm := strings.ToUpper(strings.TrimSpace(req.Variant.Assembly))
	if asm != "GRCH38" && asm != "GRCH37" && asm != "HG38" && asm != "HG19" {
		return http.StatusUnprocessableEntity, ErrCodeAssemblyMismatch, "unsupported genome assembly; must be GRCh38 or GRCh37", false
	}

	if req.Variant.Chrom == "" || req.Variant.Pos <= 0 || req.Variant.Ref == "" || req.Variant.Alt == "" || req.Variant.Gene == "" {
		return http.StatusUnprocessableEntity, ErrCodeInvalidVariant, "variant requires chrom, positive pos, ref, alt, and gene", false
	}

	if strings.TrimSpace(req.ClinicalContext.DiseaseID) == "" || strings.TrimSpace(req.ClinicalContext.Inheritance) == "" {
		return http.StatusUnprocessableEntity, ErrCodeNeedsClinicalContext, "disease_id and inheritance/MOI context are required for sequence variant interpretation", false
	}
	if !regexp.MustCompile(`^[ACGTN]+$`).MatchString(req.Variant.Ref) || !regexp.MustCompile(`^[ACGTN]+$`).MatchString(req.Variant.Alt) || req.Variant.Ref == req.Variant.Alt {
		return 422, ErrCodeInvalidVariant, "invalid reference or alternate bases", false
	}
	if af := req.Variant.AlleleFrequency; af != nil && (*af < 0 || *af > 1 || math.IsNaN(*af) || math.IsInf(*af, 0)) {
		return 422, ErrCodeInvalidVariant, "allele frequency must be between zero and one", false
	}
	if code, msg := validateContext(req.ClinicalContext.VariantOrigin, req.ClinicalContext.SampleType, req.Options); code != "" {
		return 422, code, msg, false
	}
	if desc.ID == engine.EngineSVCv4 {
		if err := svcv4.Validate(req.DraftAssessment); err != nil {
			return 422, ErrCodeInvalidVariant, err.Error(), false
		}
		if req.DraftAssessment.MOI != req.ClinicalContext.Inheritance {
			return 422, ErrCodeNeedsClinicalContext, "draft moi must match clinical_context.inheritance", false
		}
	} else if req.DraftAssessment != nil || req.Options.AllowExperimental {
		return 422, ErrCodeInvalidVariant, "draft assessment and experimental mode apply only to svc_v4", false
	}
	req.Variant.Assembly = normalizeAssembly(req.Variant.Assembly)

	return 0, "", "", true
}

func validateCNVAdmission(req *CNVEvaluationRequest) (status int, code ErrorCode, msg string, ok bool) {
	if req.Options.AllowExperimental {
		return 422, ErrCodeInvalidVariant, "experimental mode applies only to svc_v4", false
	}
	if req.EngineID == "" {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, "missing engine_id", false
	}
	desc, err := engine.Lookup(engine.EngineID(req.EngineID))
	if err != nil {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, fmt.Sprintf("unrecognized engine_id: %s", req.EngineID), false
	}

	if desc.ID != engine.EngineCNVLoss2020 && desc.ID != engine.EngineCNVGain2020 {
		return http.StatusUnprocessableEntity, ErrCodeUnknownEngine, fmt.Sprintf("engine %s is not applicable for CNV evaluation", req.EngineID), false
	}

	asm := strings.ToUpper(strings.TrimSpace(req.CNV.Assembly))
	if asm != "GRCH38" && asm != "GRCH37" && asm != "HG38" && asm != "HG19" {
		return http.StatusUnprocessableEntity, ErrCodeAssemblyMismatch, "unsupported genome assembly; must be GRCh38 or GRCh37", false
	}

	if req.CNV.CoordinateSystem != "1-based-inclusive" && req.CNV.CoordinateSystem != "0-based-half-open" {
		return http.StatusUnprocessableEntity, ErrCodeInvalidCoordinateSystem, "cnv coordinate_system is required (e.g. 1-based-inclusive)", false
	}

	if req.CNV.CoordinateSystem == "0-based-half-open" {
		if req.CNV.Start < 0 || req.CNV.End <= req.CNV.Start {
			return 422, ErrCodeInvalidVariant, "invalid half-open interval", false
		}
		req.CNV.Start++
		req.CNV.CoordinateSystem = "1-based-inclusive"
	}
	if req.CNV.Chrom == "" || req.CNV.Start <= 0 || req.CNV.End < req.CNV.Start {
		return http.StatusUnprocessableEntity, ErrCodeInvalidVariant, "cnv coordinates must have positive start <= end", false
	}

	cnvType := strings.ToLower(strings.TrimSpace(req.CNV.Type))
	if desc.ID == engine.EngineCNVLoss2020 && cnvType != "loss" {
		return http.StatusUnprocessableEntity, ErrCodeInvalidVariant, fmt.Sprintf("engine cnv_loss_2020 requires type=loss, got %q", req.CNV.Type), false
	}
	if desc.ID == engine.EngineCNVGain2020 && cnvType != "gain" {
		return http.StatusUnprocessableEntity, ErrCodeInvalidVariant, fmt.Sprintf("engine cnv_gain_2020 requires type=gain, got %q", req.CNV.Type), false
	}

	origin := strings.ToLower(strings.TrimSpace(req.ClinicalContext.VariantOrigin))
	if origin == "somatic_confirmed" {
		return http.StatusUnprocessableEntity, ErrCodeUnsupportedVariantOrigin, "constitutional CNV standards cannot be applied to confirmed somatic variants", false
	}
	if code, msg := validateContext(req.ClinicalContext.VariantOrigin, req.ClinicalContext.SampleType, req.Options); code != "" {
		return 422, code, msg, false
	}
	req.CNV.Assembly = normalizeAssembly(req.CNV.Assembly)
	if req.CNV.CopyNumber < 0 || req.CNV.CopyNumber > 100 {
		return 422, ErrCodeInvalidVariant, "invalid copy_number", false
	}
	if !oneOf(req.CNV.BreakpointPrecision, "", "unknown", "approximate", "exact") || !oneOf(req.CNV.Mosaicism, "", "unknown", "absent", "present") || !oneOf(req.CNV.Orientation, "", "unknown", "tandem", "inverted", "dispersed") {
		return 422, ErrCodeInvalidVariant, "invalid CNV structural uncertainty", false
	}
	req.CNV.Type = cnvType

	return 0, "", "", true
}
func oneOf(value string, choices ...string) bool {
	for _, c := range choices {
		if value == c {
			return true
		}
	}
	return false
}
func normalizeAssembly(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "GRCH37", "HG19":
		return "GRCh37"
	default:
		return "GRCh38"
	}
}
func validateContext(origin, sample string, opts EvaluationOptions) (ErrorCode, string) {
	if origin == "somatic_confirmed" {
		return ErrCodeUnsupportedVariantOrigin, "somatic variants are outside germline interpretation scope"
	}
	if origin != "germline_confirmed" && origin != "unknown" {
		return ErrCodeUnsupportedVariantOrigin, "variant_origin must be germline_confirmed or unknown"
	}
	switch sample {
	case "blood", "saliva", "buccal", "fibroblast", "tissue", "other", "unknown":
	default:
		return ErrCodeNeedsClinicalContext, "invalid sample_type"
	}
	if opts.RetrievalMode != "" && opts.RetrievalMode != "off" && opts.RetrievalMode != "targeted" && opts.RetrievalMode != "deep" {
		return ErrCodeBadRequest, "invalid retrieval_mode"
	}
	return "", ""
}

func computeBodyHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Server) processSNVEvaluation(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	var req SNVEvaluationRequest
	if !decode(w, r, &req) {
		return
	}
	bodyBytes, _ = json.Marshal(req)

	status, code, msg, ok := validateSNVAdmission(&req)
	if !ok {
		WriteError(w, r, status, code, msg, false)
		return
	}
	if reference := s.references[req.Variant.Assembly]; reference != nil {
		bases, e := reference.Bases(req.Variant.Chrom, req.Variant.Pos, len(req.Variant.Ref))
		if e != nil || bases != req.Variant.Ref {
			WriteError(w, r, 422, ErrCodeInvalidVariant, "reference sequence mismatch or unavailable", false)
			return
		}
	}

	s.dispatchACMG2015Job(w, r, &req, bodyBytes)
}

func (s *Server) processCNVEvaluation(w http.ResponseWriter, r *http.Request) {
	var bodyBytes []byte
	var req CNVEvaluationRequest
	if !decode(w, r, &req) {
		return
	}
	bodyBytes, _ = json.Marshal(req)

	status, code, msg, ok := validateCNVAdmission(&req)
	if !ok {
		WriteError(w, r, status, code, msg, false)
		return
	}

	s.dispatchCNVJob(w, r, &req, bodyBytes)
}

func (s *Server) processHPOExtract(w http.ResponseWriter, r *http.Request) {
	var req HPOExtractRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		WriteError(w, r, http.StatusBadRequest, ErrCodeBadRequest, "clinical text is required", false)
		return
	}

	if s.hpoExtractor == nil {
		if !s.cfg.Simulation {
			WriteError(w, r, http.StatusConflict, ErrCodeKnowledgeNotLoaded, "HPO ontology and local index are not loaded; extraction unavailable", false)
			return
		}
		bodyBytes, _ := json.Marshal(req)
		s.dispatchSimulationJob(w, r, string(engine.EngineHPOExtract), bodyBytes)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	opts := hpo.ExtractOptions{AllowLocalFallback: req.Options.AllowLocalFallback}
	if req.Options.ProviderProfileID != "" {
		if s.profiles == nil {
			WriteError(w, r, 503, ErrCodeProviderUnavailable, "profile store unavailable", false)
			return
		}
		p, e := s.profiles.Get(r.Context(), GetTenantID(r.Context()), req.Options.ProviderProfileID, 0)
		if e != nil {
			WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider profile unavailable", false)
			return
		}
		client, e := responses.NewClient(p.Config)
		if e != nil {
			WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider unavailable", false)
			return
		}
		opts.Provider = client
	}
	isStream := r.URL.Query().Get("stream") == "true" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	if isStream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "streaming not supported", false)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		onStage := func(stage string, payload any) {
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", stage, string(b))
			flusher.Flush()
		}

		opts.OnStage = onStage

		res, err := s.hpoExtractor.Extract(r.Context(), req.Text, opts)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
			flusher.Flush()
			return
		}

		dataBytes, _ := json.Marshal(res.Terms)
		fmt.Fprintf(w, "event: data\ndata: %s\n\n", string(dataBytes))
		flusher.Flush()
		return
	}

	res, err := s.hpoExtractor.Extract(r.Context(), req.Text, opts)
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, err.Error(), false)
		return
	}

	if format == "tsv" {
		w.Header().Set("Content-Type", "text/tab-separated-values; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "HPO_ID\tname\tname_cn\tconfidence\tremark")
		for _, item := range res.Terms {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				item.HPO, item.Name, item.NameCN, item.Confidence, item.Remark)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}
