package agent

import (
	"context"
	"errors"
	"github.com/pzweuj/sepilens/internal/llm"
	"strings"
	"testing"
)

type repairProvider struct {
	calls      int
	failTwice  bool
	unsafeTool bool
	network    bool
}

func (*repairProvider) Capabilities(context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{}, nil
}
func (*repairProvider) Stream(context.Context, llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	panic("unused")
}
func (p *repairProvider) Generate(_ context.Context, r llm.GenerateRequest) (llm.GenerateResult, error) {
	p.calls++
	if p.network {
		return llm.GenerateResult{}, errors.New("provider failure")
	}
	if p.calls == 1 {
		return llm.GenerateResult{ToolCalls: []llm.ToolCall{{ID: "read", Name: "get_annotations", Arguments: "{}"}}, InputTokens: 2}, nil
	}
	if p.calls == 3 {
		if len(r.Tools) != 0 || len(r.History) != 1 || !strings.Contains(r.Prompt, "$.ok") {
			panic("repair lost evidence history or enabled tools")
		}
		if p.unsafeTool {
			return llm.GenerateResult{ToolCalls: []llm.ToolCall{{ID: "read-again", Name: "get_annotations", Arguments: "{}"}}}, nil
		}
		if !p.failTwice {
			return llm.GenerateResult{Content: `{"ok":true}`, OutputTokens: 3}, nil
		}
	}
	return llm.GenerateResult{Content: `{"ok":"private-invalid-value"}`, OutputTokens: 4}, errors.Join(llm.ErrInvalidOutput, &llm.SchemaValidationError{Path: "$.ok", Reason: "boolean required"})
}
func TestStructuredOutputRepairIsBoundedAndDoesNotRepeatTools(t *testing.T) {
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"ok"}, "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}}
	for _, scenario := range []string{"corrected", "invalid_twice", "tool_during_repair", "provider_failure"} {
		t.Run(scenario, func(t *testing.T) {
			p := &repairProvider{failTwice: scenario == "invalid_twice", unsafeTool: scenario == "tool_during_repair", network: scenario == "provider_failure"}
			tools := 0
			out, err := RunContract(context.Background(), p, map[string]any{"synthetic": true}, func(context.Context, string) (any, error) { tools++; return map[string]any{"evidence": "fixed"}, nil }, "JSON only", schema)
			if scenario == "provider_failure" {
				if p.calls != 1 || out.Repairs != 0 || !errors.Is(err, ErrProvider) {
					t.Fatal("provider failure retried")
				}
				return
			}
			if p.calls != 3 || tools != 1 || out.Repairs != 1 {
				t.Fatal("repair or tool budget incorrect")
			}
			if scenario == "corrected" {
				if err != nil || string(out.Raw) != `{"ok":true}` || out.OutputTokens != 7 {
					t.Fatal("repair failed", err)
				}
			} else if !errors.Is(err, ErrOutputInvalid) || len(out.Raw) > 0 {
				t.Fatal("invalid output adopted")
			}
			if len(out.ValidationErrors) == 0 || strings.Contains(strings.Join(out.ValidationErrors, ","), "private-invalid-value") || out.RejectedOutput == "" {
				t.Fatal("diagnostic privacy or audit missing")
			}
		})
	}
}
