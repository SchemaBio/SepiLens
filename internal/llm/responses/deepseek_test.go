package responses

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/llm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeepSeekJSONModeRetainsSchemaValidation(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		client := &Client{cfg: Config{BaseURL: "https://api.deepseek.com/v1", APIKey: "synthetic", Model: "deepseek-flash", Protocol: llm.ProtocolOpenAIChat}}
		client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["response_format"].(map[string]any)["type"] != "json_object" {
				t.Fatal("unsupported DeepSeek response format", body["response_format"])
			}
			system := body["messages"].([]any)[0].(map[string]any)["content"].(string)
			if !strings.Contains(system, "JSON Schema") || !strings.Contains(system, "additionalProperties") {
				t.Fatal("schema instructions missing")
			}
			content := `{"ok":true}`
			if invalid {
				content = `{"ok":"not a boolean"}`
			}
			wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(wire))), Header: make(http.Header)}, nil
		})}
		schema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false}
		_, err := client.Generate(context.Background(), llm.GenerateRequest{Prompt: "synthetic test", JSONSchema: schema})
		if (err != nil) != invalid {
			t.Fatal("local schema validation changed", invalid, err)
		}
	}
}
