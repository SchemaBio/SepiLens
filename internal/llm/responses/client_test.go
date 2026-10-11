package responses_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtocolsGenerateAndStream(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []any{"ok"}, "additionalProperties": false}
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIResponses, llm.ProtocolOpenAIChat} {
		t.Run(string(protocol), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("malformed request")
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Error("missing auth")
				}
				if protocol == llm.ProtocolOpenAIResponses {
					if r.URL.Path != "/responses" || body["store"] != false || body["text"] == nil {
						t.Error("incorrect responses wire")
					}
				} else {
					if r.URL.Path != "/chat/completions" || body["response_format"] == nil {
						t.Error("incorrect chat wire")
					}
				}
				if body["stream"] == true {
					w.Header().Set("Content-Type", "text/event-stream")
					if protocol == llm.ProtocolOpenAIResponses {
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"ok\\\":true}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
					} else {
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"ok\\\":true}\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
					}
					return
				}
				if protocol == llm.ProtocolOpenAIResponses {
					fmt.Fprint(w, `{"status":"completed","output":[{"content":[{"type":"output_text","text":"{\"ok\":true}"}]}],"usage":{"input_tokens":3,"output_tokens":2}}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
				}
			}))
			defer ts.Close()
			client, err := responses.NewClient(responses.Config{BaseURL: ts.URL, APIKey: "synthetic-key", Model: "test", AllowPrivate: true, Protocol: protocol})
			if err != nil {
				t.Fatal(err)
			}
			caps, _ := client.Capabilities(context.Background())
			if caps.StructuredOutput || caps.Streaming || caps.NativeWebSearch {
				t.Fatal("unprobed capabilities")
			}
			req := llm.GenerateRequest{Prompt: "synthetic input", JSONSchema: schema}
			out, err := client.Generate(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if out.Content != `{"ok":true}` || out.InputTokens != 3 || out.OutputTokens != 2 {
				t.Fatalf("wrong output %+v", out)
			}
			ch, err := client.Stream(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			done, usage := false, false
			for e := range ch {
				if e.Type == "error" {
					t.Fatal(e.Content)
				}
				done = done || e.Type == "done"
				usage = usage || e.Type == "usage"
			}
			if !done || !usage {
				t.Fatal("missing stream completion or usage")
			}
		})
	}
}
func TestSSRFAndSanitizedErrors(t *testing.T) {
	for _, url := range []string{"http://example.com", "https://127.0.0.1", "https://169.254.169.254", "https://user:password@example.com"} {
		if _, e := responses.NewClient(responses.Config{BaseURL: url}); e == nil {
			t.Fatal("unsafe URL accepted", url)
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); fmt.Fprint(w, "secret patient text") }))
	defer ts.Close()
	c, _ := responses.NewClient(responses.Config{BaseURL: ts.URL, APIKey: "key", Model: "m", AllowPrivate: true})
	_, e := c.Generate(context.Background(), llm.GenerateRequest{})
	if e == nil || e.Error() != "model request failed (HTTP 401)" {
		t.Fatal(e)
	}
}
