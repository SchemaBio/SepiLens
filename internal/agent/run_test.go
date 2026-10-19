package agent

import (
	"context"
	"testing"

	"github.com/pzweuj/sepilens/internal/llm"
)

type fakeProvider struct {
	calls   int
	forever bool
	name    string
}

func (f *fakeProvider) Capabilities(context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{}, nil
}
func (f *fakeProvider) Stream(context.Context, llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	panic("unused")
}
func (f *fakeProvider) Generate(_ context.Context, r llm.GenerateRequest) (llm.GenerateResult, error) {
	f.calls++
	if f.calls == 1 || f.forever {
		name := f.name
		if name == "" {
			name = "get_annotations"
		}
		return llm.GenerateResult{ToolCalls: []llm.ToolCall{{ID: string(rune('a' + f.calls)), Name: name, Arguments: "{}"}}, InputTokens: 3, OutputTokens: 2}, nil
	}
	return llm.GenerateResult{Content: `{"criteria":[],"classification":"ignored"}`, InputTokens: 1, OutputTokens: 1}, nil
}
func TestRunnerToolsAndBudget(t *testing.T) {
	f := &fakeProvider{}
	out, e := Run(context.Background(), f, map[string]any{"variant": "synthetic"}, func(context.Context, string) (any, error) { return map[string]any{}, nil })
	if e != nil || out.Rounds != 2 || out.InputTokens != 4 || len(out.Trace) != 1 {
		t.Fatal(out, e)
	}
	f = &fakeProvider{forever: true}
	out, e = Run(context.Background(), f, nil, func(context.Context, string) (any, error) { return nil, nil })
	if e != ErrBudgetExceeded || out.Rounds != 12 {
		t.Fatal(out, e)
	}
	f = &fakeProvider{name: "delete_records"}
	if _, e = Run(context.Background(), f, nil, func(context.Context, string) (any, error) { t.Fatal("unregistered tool executed"); return nil, nil }); e != ErrOutputInvalid {
		t.Fatal(e)
	}
}
