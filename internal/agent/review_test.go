package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/llm"
)

type reviewProvider struct {
	calls     int
	corrected bool
	malformed bool
	t         *testing.T
}

func (*reviewProvider) Capabilities(context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{}, nil
}
func (*reviewProvider) Stream(context.Context, llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	panic("unused")
}
func (p *reviewProvider) Generate(ctx context.Context, r llm.GenerateRequest) (llm.GenerateResult, error) {
	p.calls++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 3*time.Minute {
		p.t.Fatal("shared deadline absent")
	}
	if p.calls > 2 {
		p.t.Fatal("second repair attempted")
	}
	if p.calls == 2 && len(r.Tools) != 0 {
		p.t.Fatal("correction enabled tools")
	}
	if p.calls == 1 && p.malformed {
		return llm.GenerateResult{Content: "malformed", FinishReason: "stop"}, nil
	}
	content := `{"ok":false}`
	if p.calls == 2 && p.corrected {
		content = `{"ok":true}`
	}
	return llm.GenerateResult{Content: content, FinishReason: "stop", InputTokens: 2, OutputTokens: 3}, nil
}
func TestEvidenceReviewSharesStructuralRepairBudget(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		for _, corrected := range []bool{false, true} {
			p := &reviewProvider{malformed: malformed, corrected: corrected, t: t}
			out, err := RunContractWithReview(context.Background(), p, map[string]any{}, func(context.Context, string) (any, error) { t.Fatal("unexpected tool"); return nil, nil }, "synthetic", map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false}, func(raw json.RawMessage) []string {
				if strings.Contains(string(raw), "false") {
					return []string{"criteria[0].evidence_bindings: unknown_fact_id"}
				}
				return nil
			})
			if err != nil || p.calls != 2 || out.Repairs != 1 {
				t.Fatal("shared budget changed", err, p.calls, out.Repairs)
			}
			if corrected && len(out.ReviewIssues) != 0 {
				t.Fatal("resolved issues retained")
			}
			if !corrected && (len(out.ReviewIssues) == 0 || len(out.Raw) == 0) {
				t.Fatal("pending review not reported for selective projection")
			}
			if !malformed && !corrected && len(out.RejectedOutputs) != 2 {
				t.Fatal("original rejected outputs lost")
			}
		}
	}
}
