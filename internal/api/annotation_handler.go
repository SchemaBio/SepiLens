package api

import (
	"net/http"
	"time"

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
	Status      string `json:"status"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
	ID          string `json:"id"`
	Enabled     bool   `json:"enabled"`
	Version     string `json:"version,omitempty"`
	License     string `json:"license,omitempty"`
	Note        string `json:"note,omitempty"`
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
	normalized.MarkCallerSupplied()
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
		{ID: "clinvar", Status: "unavailable", Note: "caller inputs remain unverified"},
		{ID: "gnomad", Status: "unavailable", Note: "caller inputs remain unverified"},
		{ID: "alphamissense", Status: "unavailable", Note: "engineering thresholds require calibration"},
		{ID: "pangolin", Status: "unavailable", Note: "no trusted offline adapter configured"},
		{ID: "gencc", Status: "unavailable", Note: "Refuted and Disputed block classification"},
	}
	if s.annotationIndex != nil {
		for n := range sources {
			for _, a := range s.annotationIndex.Assets {
				if a.Kind != sources[n].ID {
					continue
				}
				v := &sources[n]
				v.Version = a.Version
				v.License = a.License
				v.RetrievedAt = a.RetrievedAt
				v.Enabled = a.LicenseConfirmed
				v.Status = "found"
				if !a.LicenseConfirmed {
					v.Status = "unavailable"
					continue
				}
				at, e := time.Parse(time.RFC3339, a.RetrievedAt)
				days := a.MaxAgeDays
				if days == 0 {
					days = 90
				}
				if e != nil || at.After(time.Now()) {
					v.Enabled = false
					v.Status = "unavailable"
				} else if time.Since(at) > time.Duration(days)*24*time.Hour {
					v.Enabled = false
					v.Status = "stale"
				}
			}
		}
	}
	writeJSON(w, 200, AnnotationSourcesResponse{Sources: sources})
}
