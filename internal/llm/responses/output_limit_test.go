package responses

import (
	"context"
	"errors"
	"github.com/pzweuj/sepilens/internal/llm"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTokenLimitNeverDeliversPartialOutputOrTools(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIChat, llm.ProtocolOpenAIResponses} {
		c := &Client{cfg: Config{BaseURL: "https://example.com/v1", APIKey: "synthetic", Model: "synthetic", Protocol: protocol}}
		body := `{"choices":[{"finish_reason":"length","message":{"content":"private-partial","tool_calls":[{"id":"partial","type":"function","function":{"name":"get_annotations","arguments":"{"}}]}}]}`
		if protocol == llm.ProtocolOpenAIResponses {
			body = `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","call_id":"partial","name":"get_annotations","arguments":"{"}]}`
		}
		c.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		_, err := c.Generate(context.Background(), llm.GenerateRequest{})
		if !errors.Is(err, llm.ErrOutputLimit) {
			t.Fatalf("token limit not recognized: %v", err)
		}
	}
}
