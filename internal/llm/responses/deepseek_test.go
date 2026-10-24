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

func TestDeepSeekToolContinuationKeepsReasoningPrivate(t *testing.T) {
	client := &Client{cfg: Config{BaseURL: "https://api.deepseek.com/v1", APIKey: "synthetic", Model: "deepseek-flash", Protocol: llm.ProtocolOpenAIChat}}
	calls := 0
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f := body["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
		if f["strict"] != nil {
			t.Fatal("beta-only strict flag sent to standard endpoint")
		}
		message := map[string]any{"content": "done"}
		finish := "stop"
		if calls == 1 {
			message = map[string]any{"reasoning_content": "synthetic continuation", "tool_calls": []any{map[string]any{"id": "tool1", "type": "function", "function": map[string]any{"name": "get_annotations", "arguments": "{}"}}}}
			finish = "tool_calls"
		} else if body["messages"].([]any)[2].(map[string]any)["reasoning_content"] != "synthetic continuation" {
			t.Fatal("provider continuation lost")
		}
		wire, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": finish}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(wire))), Header: make(http.Header)}, nil
	})}
	req := llm.GenerateRequest{Prompt: "synthetic", Tools: []llm.Tool{{Name: "get_annotations", Parameters: map[string]any{"type": "object"}}}}
	first, err := client.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	turn := llm.ToolTurn{Calls: first.ToolCalls, ReasoningContent: first.ReasoningContent, Outputs: []llm.ToolOutput{{CallID: "tool1", Output: "{}"}}}
	for _, v := range []any{first, turn} {
		encoded, _ := json.Marshal(v)
		if strings.Contains(string(encoded), "synthetic continuation") {
			t.Fatal("reasoning exposed to audit")
		}
	}
	req.History = []llm.ToolTurn{turn}
	if _, err = client.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}
