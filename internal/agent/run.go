package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
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
	ProviderFailure  string          `json:"provider_failure,omitempty"`
	FinishReason     string          `json:"finish_reason,omitempty"`
	ValidationErrors []string        `json:"output_validation_errors,omitempty"`
	RejectedOutput   string          `json:"rejected_output,omitempty"`
	RejectedOutputs  []string        `json:"rejected_outputs,omitempty"`
	Repairs          int             `json:"output_repairs,omitempty"`
	Raw              json.RawMessage `json:"assessment_json,omitempty"`
	Assessment       Assessment      `json:"assessment"`
	Trace            []Trace         `json:"tools"`
	InputTokens      int             `json:"input_tokens"`
	OutputTokens     int             `json:"output_tokens"`
	Rounds           int             `json:"rounds"`
	ReviewIssues     []string        `json:"review_issues,omitempty"`
}

func Run(ctx context.Context, p llm.Provider, input any, execute ToolExecutor) (RunResult, error) {
	return runContract(ctx, p, input, execute, SystemPrompt, OutputSchema(), true, nil)
}

// RunContract shares protocol history, budgets and read-only tool restrictions
// across engines while leaving their output vocabulary to the caller.
func RunContract(ctx context.Context, p llm.Provider, input any, execute ToolExecutor, system string, schema map[string]any) (RunResult, error) {
	return runContract(ctx, p, input, execute, system, schema, false, nil)
}

// Review uses the SAME repair counter, deadline and tool history as structural
// validation and truncation recovery. After the shared budget is exhausted,
// structurally valid output is returned with ReviewIssues for field-level
// projection by the caller; it must not be treated as validated medical evidence.
func RunContractWithReview(ctx context.Context, p llm.Provider, input any, execute ToolExecutor, system string, schema map[string]any, review func(json.RawMessage) []string) (RunResult, error) {
	return runContract(ctx, p, input, execute, system, schema, false, review)
}

func runContract(ctx context.Context, p llm.Provider, input any, execute ToolExecutor, system string, schema map[string]any, legacy bool, review func(json.RawMessage) []string) (RunResult, error) {
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
	repair := func(response llm.GenerateResult, reason string) bool {
		out.ValidationErrors = append(out.ValidationErrors, reason)
		if len(response.Content) <= 256<<10 {
			out.RejectedOutput = response.Content
			if len(out.RejectedOutputs) < 2 {
				out.RejectedOutputs = append(out.RejectedOutputs, response.Content)
			}
		}
		if out.Repairs >= 1 || len(response.ToolCalls) > 0 || response.Content == "" {
			return false
		}
		out.Repairs++
		// Keep the same evidence and tool history; a format correction cannot run tools again.
		req.Tools = nil
		req.Prompt = string(b) + "\nThe previous final JSON failed structural validation: " + reason + ". Return corrected JSON only, matching the supplied schema exactly. Preserve the evidence limits; never invent facts, sources, scores or method inputs to satisfy a required property. Use null, empty strings or [] only as permitted by the schema. Do not call tools."
		return true
	}
	for round := 0; round < 12; round++ {
		out.Rounds++
		response, err := p.Generate(ctx, req)
		// Store only allowlisted diagnostics; provider errors may contain private data.
		switch response.FinishReason {
		case "stop", "completed", "length", "incomplete", "tool_calls", "content_filter":
			out.FinishReason = response.FinishReason
		default:
			out.FinishReason = "unknown"
		}
		out.InputTokens += response.InputTokens
		out.OutputTokens += response.OutputTokens
		if err != nil {
			if errors.Is(err, llm.ErrOutputLimit) {
				out.ProviderFailure = "output_token_limit"
				// One bounded continuation under the existing request and deadline.
				// Discard partial tool calls and output; they are never executed or adopted.
				if out.Repairs == 0 {
					out.Repairs++
					maxTokens = 16384
					req.Tools = nil
					req.Prompt = string(b) + "\nThe previous generation exceeded its output token budget. Return concise final JSON matching the schema; do not call tools. Preserve uncertainty and existing evidence; omit optional unassessed criteria. Do not invent facts or sources."
					continue
				}
				return out, ErrOutputInvalid
			}
			if errors.Is(err, llm.ErrInvalidOutput) {
				reason := "structured output invalid"
				var detail *llm.SchemaValidationError
				if errors.As(err, &detail) {
					reason = detail.Error()
				}
				if repair(response, reason) {
					continue
				}
				return out, ErrOutputInvalid
			}
			out.ProviderFailure = "provider_request_failed"
			switch err.Error() {
			case "model transport unavailable":
				out.ProviderFailure = "transport_unavailable"
			case "model response unreadable":
				out.ProviderFailure = "response_unreadable"
			case "model response malformed":
				out.ProviderFailure = "response_malformed"
			case "model output incomplete":
				out.ProviderFailure = "output_incomplete"
			case "model returned no choices":
				out.ProviderFailure = "no_choices"
			case "incomplete tool call output":
				out.ProviderFailure = "tool_output_incomplete"
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				out.ProviderFailure = "deadline_exceeded"
			}
			return out, ErrProvider
		}
		if len(response.ToolCalls) == 0 {
			var parsed any
			if len(response.Content) > 256<<10 {
				return out, ErrOutputInvalid
			}
			if strictjson.Decode([]byte(response.Content), &parsed) != nil {
				if repair(response, "$: invalid JSON or duplicate keys") {
					continue
				}
				return out, ErrOutputInvalid
			}
			if err := llm.ValidateJSON(response.Content, schema); err != nil {
				if repair(response, err.Error()) {
					continue
				}
				return out, ErrOutputInvalid
			}
			if legacy {
				out.Assessment, err = Parse(response.Content)
			} else {
				if review != nil {
					issues := review(json.RawMessage(response.Content))
					if len(issues) > 24 {
						issues = issues[:24]
					}
					if len(issues) > 0 {
						out.ReviewIssues = issues
						reason := "evidence binding review: "
						for i, issue := range issues {
							if i > 0 {
								reason += "; "
							}
							reason += issue
						}
						if repair(response, reason) {
							continue
						}
					} else {
						out.ReviewIssues = nil
					}
				}
				out.Raw = json.RawMessage(response.Content)
			}
			return out, err
		}
		if out.Repairs > 0 {
			return out, ErrOutputInvalid
		}
		turn := llm.ToolTurn{Calls: response.ToolCalls, ResponseItems: response.ResponseItems, ReasoningContent: response.ReasoningContent}
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
