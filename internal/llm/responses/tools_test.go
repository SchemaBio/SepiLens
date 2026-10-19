package responses_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
)

func TestToolRoundTripBothProtocols(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIResponses, llm.ProtocolOpenAIChat} {
		t.Run(string(protocol), func(t *testing.T) {
			calls := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid JSON")
				}
				if body["tools"] == nil {
					t.Error("missing tool definitions")
				}
				if calls == 1 {
					if protocol == llm.ProtocolOpenAIResponses {
						fmt.Fprint(w, `{"status":"completed","output":[{"type":"function_call","call_id":"call-1","name":"get_annotations","arguments":"{}"}]}`)
					} else {
						fmt.Fprint(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_annotations","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
					}
					return
				}
				if protocol == llm.ProtocolOpenAIResponses {
					input := body["input"].([]any)
					if len(input) != 3 || input[2].(map[string]any)["call_id"] != "call-1" || input[2].(map[string]any)["output"] != "{}" {
						t.Errorf("invalid Responses tool continuation: %+v", input)
					}
					fmt.Fprint(w, `{"status":"completed","output":[{"content":[{"type":"output_text","text":"{\"ok\":true}"}]}]}`)
				} else {
					messages := body["messages"].([]any)
					if len(messages) != 4 || messages[3].(map[string]any)["tool_call_id"] != "call-1" {
						t.Errorf("invalid Chat tool continuation: %+v", messages)
					}
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`)
				}
			}))
			defer ts.Close()
			c, e := responses.NewClient(responses.Config{BaseURL: ts.URL, APIKey: "synthetic", Model: "synthetic", AllowPrivate: true, Protocol: protocol})
			if e != nil {
				t.Fatal(e)
			}
			req := llm.GenerateRequest{Prompt: "structured synthetic input", Tools: []llm.Tool{{Name: "get_annotations", Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}}}
			first, e := c.Generate(context.Background(), req)
			if e != nil || len(first.ToolCalls) != 1 {
				t.Fatal(first, e)
			}
			req.History = []llm.ToolTurn{{Calls: first.ToolCalls, ResponseItems: first.ResponseItems, Outputs: []llm.ToolOutput{{CallID: "call-1", Output: "{}"}}}}
			last, e := c.Generate(context.Background(), req)
			if e != nil || last.Content != `{"ok":true}` {
				t.Fatal(last, e)
			}
		})
	}
}
