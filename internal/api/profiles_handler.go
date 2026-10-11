package api

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	if s.profiles == nil {
		WriteError(w, r, 503, ErrCodeProviderUnavailable, "encrypted profile store unavailable", false)
		return
	}
	var req ProviderTestRequest
	if !decode(w, r, &req) {
		return
	}
	id := chi.URLParam(r, "profile_id")
	p, e := s.profiles.Save(r.Context(), GetTenantID(r.Context()), id, responses.Config{BaseURL: req.BaseURL, APIKey: req.APIKey, Model: req.Model, Protocol: llm.Protocol(req.Protocol)})
	if e != nil {
		WriteError(w, r, 422, ErrCodeBadRequest, "invalid provider configuration", false)
		return
	}
	writeJSON(w, 201, map[string]any{"id": p.ID, "version": p.Version, "protocol": p.Config.Protocol, "model": p.Config.Model, "base_url": p.Config.BaseURL})
}
func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	if s.profiles == nil || s.profiles.Delete(r.Context(), GetTenantID(r.Context()), chi.URLParam(r, "profile_id")) != nil {
		WriteError(w, r, 404, ErrCodeJobNotFound, "profile not found", false)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	if s.profiles == nil {
		WriteError(w, r, 404, ErrCodeJobNotFound, "profile not found", false)
		return
	}
	p, e := s.profiles.Get(r.Context(), GetTenantID(r.Context()), chi.URLParam(r, "profile_id"), 0)
	if e != nil {
		WriteError(w, r, 404, ErrCodeJobNotFound, "profile not found", false)
		return
	}
	writeJSON(w, 200, map[string]any{"id": p.ID, "version": p.Version, "protocol": p.Config.Protocol, "model": p.Config.Model, "base_url": p.Config.BaseURL})
}
func probe(ctx context.Context, c *responses.Client) map[string]string {
	states := map[string]string{"connectivity": "unknown", "structured_output": "unknown", "streaming": "unknown", "usage_reporting": "unknown", "native_web_search": "unknown", "function_calling": "unknown", "search_citations": "unknown"}
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`), &schema)
	req := llm.GenerateRequest{Prompt: `Return {"ok":true}.`, JSONSchema: schema}
	if out, e := c.Generate(ctx, req); e == nil {
		states["connectivity"] = "supported"
		states["structured_output"] = "supported"
		if out.InputTokens+out.OutputTokens > 0 {
			states["usage_reporting"] = "supported"
		}
	}
	ch, e := c.Stream(ctx, llm.GenerateRequest{Prompt: "Reply OK."})
	if e == nil {
		for ev := range ch {
			if ev.Type == "error" {
				break
			}
			if ev.Type == "done" {
				states["streaming"] = "supported"
			}
		}
	}
	return states
}
func (s *Server) handleProviderTestLogic(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	var req ProviderTestRequest
	if !decode(w, r, &req) {
		return
	}
	proto := llm.Protocol(strings.TrimSpace(req.Protocol))
	c, e := responses.NewClient(responses.Config{BaseURL: req.BaseURL, APIKey: req.APIKey, Model: req.Model, Protocol: proto, AllowPrivate: s.cfg.AllowPrivateEndpoints})
	if e != nil {
		WriteError(w, r, 422, ErrCodeProviderCapabilityMismatch, "provider endpoint or protocol invalid", false)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	states := probe(ctx, c)
	writeJSON(w, 200, map[string]any{"protocol": proto, "capabilities": states})
}
