package api

import (
	"net/http"

	"github.com/pzweuj/sepilens/internal/annotation"
)

type AnnotationRequest struct {
	EngineID    string          `json:"engine_id"`
	Variant     SNVVariant      `json:"variant"`
	Annotations *annotation.Set `json:"annotations"`
}

type AnnotationSourcesResponse struct {
	Sources []AnnotationSourceStatus `json:"sources"`
}

type AnnotationSourceStatus struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Version string `json:"version,omitempty"`
	License string `json:"license,omitempty"`
	Note    string `json:"note,omitempty"`
}

// handleAnnotationValidate normalizes and validates a caller-supplied annotation
// set without classifying. It exists so a caller can check a payload before a
// paid evaluation.
func (s *Server) handleAnnotationValidate(w http.ResponseWriter, r *http.Request) {
	var req AnnotationRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Annotations == nil {
		WriteError(w, r, 422, ErrCodeBadRequest, "annotations are required", false)
		return
	}
	assembly := normalizeAssembly(req.Variant.Assembly)
	if err := req.Annotations.Validate(assembly); err != nil {
		WriteError(w, r, 422, ErrCodeInvalidVariant, err.Error(), false)
		return
	}
	resolver := annotation.NewResolver()
	if err := resolver.Require(req.Annotations); err != nil {
		WriteError(w, r, 422, ErrCodeBadRequest, "at least one annotation source is required", false)
		return
	}
	normalized, err := resolver.Normalize(req.Annotations, assembly)
	if err != nil {
		WriteError(w, r, 422, ErrCodeInvalidVariant, err.Error(), false)
		return
	}
	writeJSON(w, 200, map[string]any{
		"assembly":        assembly,
		"annotations":     normalized,
		"conflicts":       resolver.Conflicts(normalized),
		"complete":        resolver.Complete(normalized),
		"settlement":      "caller_supplied annotations are evaluable but not verified evidence",
		"review_required": true,
	})
}

// handleAnnotationSources reports which sources are wired. Versions and licenses
// stay empty when no locked bundle is loaded; the caller must not assume data.
func (s *Server) handleAnnotationSources(w http.ResponseWriter, r *http.Request) {
	sources := []AnnotationSourceStatus{
		{ID: "clinvar", Enabled: s.references != nil || s.coordinator != nil, Note: "caller-supplied or official snapshot; classification is evidence, not a conclusion"},
		{ID: "gnomad", Enabled: true, Note: "caller-supplied; no bundled dataset, so PM2 requires coverage_adequate"},
		{ID: "alphamissense", Enabled: true, Note: "caller-supplied; Supporting strength only until calibrated"},
		{ID: "pangolin", Enabled: true, Note: "caller-supplied; Supporting strength only until calibrated"},
		{ID: "gencc", Enabled: true, Note: "caller-supplied; Refuted and Disputed block classification"},
	}
	writeJSON(w, 200, AnnotationSourcesResponse{Sources: sources})
}
