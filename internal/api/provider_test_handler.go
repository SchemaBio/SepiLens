package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/ssrf"
)

type ProviderTestRequest struct {
	Protocol string `json:"protocol"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"`
}

func (s *Server) handleProviderTestLogic(w http.ResponseWriter, r *http.Request) {
	var req ProviderTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, r, http.StatusBadRequest, ErrCodeBadRequest, "invalid JSON payload", false)
		return
	}

	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto != string(llm.ProtocolOpenAIResponses) {
		WriteError(w, r, http.StatusUnprocessableEntity, ErrCodeProviderCapabilityMismatch, "unsupported protocol; only openai_responses is currently enabled", false)
		return
	}

	rawURL := req.BaseURL
	if rawURL == "" {
		rawURL = "https://api.openai.com/v1"
	}

	validatedURL, err := ssrf.ValidateEndpoint(rawURL, s.cfg.AllowPrivateEndpoints)
	if err != nil {
		WriteError(w, r, http.StatusUnprocessableEntity, ErrCodeBadRequest, err.Error(), false)
		return
	}

	// Deliberately audit WITHOUT logging req.APIKey
	s.logger.Info("provider connectivity check passed validation",
		"protocol", proto,
		"model", req.Model,
		"host", validatedURL.Host,
	)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "valid",
		"protocol": proto,
		"base_url": validatedURL.String(),
		"capabilities": map[string]bool{
			"structured_output": true,
			"streaming":         true,
			"native_web_search": true,
		},
	})
}
