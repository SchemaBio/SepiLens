package agent

import (
	"context"
	"errors"
	"github.com/pzweuj/sepilens/internal/llm"
	"testing"
)

type limitProvider struct {
	calls int
	twice bool
}

func (*limitProvider) Capabilities(context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{}, nil
}
func (*limitProvider) Stream(context.Context, llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	panic("unused")
}
func (p *limitProvider) Generate(_ context.Context, r llm.GenerateRequest) (llm.GenerateResult, error) {
	p.calls++
	if p.calls == 2 && (*r.MaxTokens != 16384 || len(r.Tools) != 0) {
		panic("incorrect retry budget or enabled tools")
	}
	if p.calls == 1 || p.twice {
		return llm.GenerateResult{FinishReason: "length", Content: "private-partial", OutputTokens: 8192, ToolCalls: []llm.ToolCall{{ID: "partial", Name: "get_annotations", Arguments: "{"}}}, llm.ErrOutputLimit
	}
	return llm.GenerateResult{FinishReason: "stop", Content: `{"ok":true}`, OutputTokens: 2}, nil
}
func TestTokenLimitRetryIsBoundedAndDiscardsPartialTools(t *testing.T) {
	for _, twice := range []bool{false, true} {
		p := &limitProvider{twice: twice}
		calls := 0
		out, err := RunContract(context.Background(), p, map[string]any{}, func(context.Context, string) (any, error) { calls++; return nil, nil }, "synthetic", map[string]any{"type": "object"})
		if p.calls != 2 || calls != 0 || out.Repairs != 1 || out.ProviderFailure != "output_token_limit" {
			t.Fatal("unsafe retry", out, err)
		}
		if twice {
			if !errors.Is(err, ErrOutputInvalid) || len(out.Raw) != 0 {
				t.Fatal("truncated output delivered")
			}
		} else if err != nil || string(out.Raw) != `{"ok":true}` || out.OutputTokens != 8194 {
			t.Fatal("recovery failed", err)
		}
	}
}
