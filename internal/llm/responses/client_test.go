package responses_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
)

func TestResponsesClient_SSRFProtection(t *testing.T) {
	// Private / metadata URL without allowPrivate fails NewClient
	_, err := responses.NewClient(responses.Config{
		BaseURL:      "http://169.254.169.254/v1",
		APIKey:       "sk-test",
		AllowPrivate: false,
	})
	if err == nil {
		t.Fatalf("expected SSRF error for link-local metadata address, got nil")
	}

	// Loopback without allowPrivate fails NewClient
	_, errLoop := responses.NewClient(responses.Config{
		BaseURL:      "https://127.0.0.1:8080/v1",
		APIKey:       "sk-test",
		AllowPrivate: false,
	})
	if errLoop == nil {
		t.Fatalf("expected SSRF error for loopback, got nil")
	}
}

func TestResponsesClient_CapabilitiesAndGenerate(t *testing.T) {
	// Mock OpenAI chat/completions server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"status":"mock_result"}`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 8,
				"total_tokens":      20,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client, err := responses.NewClient(responses.Config{
		BaseURL:      mockServer.URL,
		APIKey:       "sk-mock-key",
		Model:        "gpt-4o",
		AllowPrivate: true,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	caps, err := client.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("failed to get capabilities: %v", err)
	}
	if !caps.StructuredOutput || !caps.NativeWebSearch {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	res, err := client.Generate(context.Background(), llm.GenerateRequest{
		Prompt: "test prompt",
	})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if res.Content != `{"status":"mock_result"}` {
		t.Fatalf("unexpected completion content: %q", res.Content)
	}
	if res.InputTokens != 12 || res.OutputTokens != 8 {
		t.Fatalf("mismatched tokens: in=%d, out=%d", res.InputTokens, res.OutputTokens)
	}
}
