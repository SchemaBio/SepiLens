package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"time"

	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/strictjson"
)

const PromptVersion = "acmg-ai-v1"

//go:embed prompts/acmg_ai_system.md
var SystemPrompt string

type ToolExecutor func(context.Context, string) (any, error)
type Trace struct {
	CallID string          `json:"call_id"`
	Tool   string          `json:"tool"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type RunResult struct {
	Raw          json.RawMessage `json:"assessment_json,omitempty"`
	Assessment   Assessment      `json:"assessment"`
	Trace        []Trace         `json:"tools"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	Rounds       int             `json:"rounds"`
}

func Run(ctx context.Context, p llm.Provider, input any, execute ToolExecutor) (RunResult, error) {
	return runContract(ctx, p, input, execute, SystemPrompt, OutputSchema(), true)
}

// RunContract shares protocol history, budgets and read-only tool restrictions
// across engines while leaving their output vocabulary to the caller.
func RunContract(ctx context.Context, p llm.Provider, input any, execute ToolExecutor, system string, schema map[string]any) (RunResult, error) {
	return runContract(ctx, p, input, execute, system, schema, false)
}

func runContract(ctx context.Context, p llm.Provider, input any, execute ToolExecutor, system string, schema map[string]any, legacy bool) (RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var out RunResult
	b, err := json.Marshal(input)
	if err != nil || len(b) > 1<<20 {
		return out, ErrOutputInvalid
	}
	args := map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}, "additionalProperties": false}
	tools := []llm.Tool{{Name: "get_annotations", Description: "Read the fixed annotation snapshot", Parameters: args}, {Name: "get_phenotype", Description: "Read the fixed phenotype score", Parameters: args}, {Name: "search_literature", Description: "Search verified official literature for the fixed evaluation object", Parameters: args}}
	maxTokens := 8192
	req := llm.GenerateRequest{Prompt: string(b), System: system, Tools: tools, JSONSchema: schema, MaxTokens: &maxTokens}
	seen := map[string]bool{}
	calls := 0
	for round := 0; round < 12; round++ {
		out.Rounds++
		response, err := p.Generate(ctx, req)
		out.InputTokens += response.InputTokens
		out.OutputTokens += response.OutputTokens
		if err != nil {
			return out, ErrProvider
		}
		if len(response.ToolCalls) == 0 {
			var parsed any
			if len(response.Content) > 256<<10 || strictjson.Decode([]byte(response.Content), &parsed) != nil {
				return out, ErrOutputInvalid
			}
			if err := llm.ValidateJSON(response.Content, schema); err != nil {
				return out, ErrOutputInvalid
			}
			if legacy {
				out.Assessment, err = Parse(response.Content)
			} else {
				out.Raw = json.RawMessage(response.Content)
			}
			return out, err
		}
		turn := llm.ToolTurn{Calls: response.ToolCalls, ResponseItems: response.ResponseItems}
		for _, call := range response.ToolCalls {
			calls++
			if calls > 100 {
				return out, ErrBudgetExceeded
			}
			if seen[call.ID] || call.ID == "" {
				return out, ErrOutputInvalid
			}
			seen[call.ID] = true
			if call.Name != "get_annotations" && call.Name != "get_phenotype" && call.Name != "search_literature" {
				return out, ErrOutputInvalid
			}
			if llm.ValidateJSON(call.Arguments, args) != nil {
				return out, ErrOutputInvalid
			}
			value, e := execute(ctx, call.Name)
			if e != nil {
				out.Trace = append(out.Trace, Trace{CallID: call.ID, Tool: call.Name, Error: "tool failed"})
				return out, e
			}
			raw, e := json.Marshal(value)
			if e != nil || len(raw) > 256<<10 {
				return out, ErrOutputInvalid
			}
			out.Trace = append(out.Trace, Trace{CallID: call.ID, Tool: call.Name, Output: raw})
			turn.Outputs = append(turn.Outputs, llm.ToolOutput{CallID: call.ID, Output: string(raw)})
		}
		req.History = append(req.History, turn)
	}
	return out, ErrBudgetExceeded
}
