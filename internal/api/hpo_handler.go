package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/llm/responses"
)

type HPOExtractOptions struct {
	ProviderProfileID  string `json:"provider_profile_id"`
	MaxCandidates      int    `json:"max_candidates,omitempty"`
	AllowLocalFallback bool   `json:"allow_local_fallback,omitempty"`
}
type HPOExtractRequest struct {
	Text    string            `json:"text"`
	Options HPOExtractOptions `json:"options"`
}

// Text is processed synchronously and never persisted in jobs, snapshots or logs.
func (s *Server) handleHPOExtract(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin", "submitter") {
		return
	}
	var req HPOExtractRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" || utf8.RuneCountInString(req.Text) > 10000 || req.Options.MaxCandidates < 0 || req.Options.MaxCandidates > 100 || req.Options.AllowLocalFallback {
		WriteError(w, r, 422, ErrCodeBadRequest, "text must contain 1–10000 characters; AI-only extraction, at most 100 candidates", false)
		return
	}
	if s.cfg.AIMode != "enabled" {
		WriteError(w, r, 409, ErrCodeProviderUnavailable, "AI extraction is disabled", false)
		return
	}
	if s.hpoExtractor == nil {
		WriteError(w, r, 503, ErrCodeKnowledgeNotLoaded, "HPO terminology unavailable", false)
		return
	}
	if s.profiles == nil || req.Options.ProviderProfileID == "" {
		WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider profile required", false)
		return
	}
	profile, err := s.profiles.Get(r.Context(), GetTenantID(r.Context()), req.Options.ProviderProfileID, 0)
	if err != nil {
		WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider profile unavailable", false)
		return
	}
	provider, err := responses.NewClient(profile.Config)
	if err != nil {
		WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider configuration invalid", false)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()
	result, err := s.hpoExtractor.Extract(ctx, req.Text, hpo.ExtractOptions{Provider: provider, Model: profile.Config.Model, MaxCandidates: req.Options.MaxCandidates, AllowLocalFallback: false})
	if err != nil {
		WriteError(w, r, 503, ErrCodeAIOutputInvalid, "AI HPO extraction failed; no local fallback was used", true)
		return
	}
	if result.ExecutionMode != "model" || result.Degraded {
		WriteError(w, r, 503, ErrCodeAIOutputInvalid, "AI HPO result unavailable", false)
		return
	}
	writeJSON(w, 200, result)
}
