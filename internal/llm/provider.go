package llm

import (
	"context"
	"encoding/json"
)

type Protocol string

const (
	ProtocolOpenAIResponses Protocol = "openai_responses"
	ProtocolOpenAIChat      Protocol = "openai_chat"
	ProtocolAnthropic       Protocol = "anthropic_messages"
)

type ModelCapabilities struct {
	StructuredOutput bool `json:"structured_output"`
	Streaming        bool `json:"streaming"`
	FunctionCalling  bool `json:"function_calling"`
	NativeWebSearch  bool `json:"native_web_search"`
	SearchCitations  bool `json:"search_citations"`
	UsageReporting   bool `json:"usage_reporting"`
}

type GenerateRequest struct {
	Tools       []Tool            `json:"tools,omitempty"`
	History     []ToolTurn        `json:"history,omitempty"`
	Model       string            `json:"model"`
	Prompt      string            `json:"prompt"`
	System      string            `json:"system,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	JSONSchema  map[string]any    `json:"json_schema,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type GenerateResult struct {
	ToolCalls     []ToolCall        `json:"tool_calls,omitempty"`
	ResponseItems []json.RawMessage `json:"response_items,omitempty"`
	Content       string            `json:"content"`
	InputTokens   int               `json:"input_tokens"`
	OutputTokens  int               `json:"output_tokens"`
	Citations     []string          `json:"citations,omitempty"`
	FinishReason  string            `json:"finish_reason"`
	RawResponse   map[string]any    `json:"raw_response,omitempty"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type ToolOutput struct {
	CallID string `json:"call_id"`
	Output string `json:"output"`
}
type ToolTurn struct {
	Calls         []ToolCall        `json:"calls"`
	Outputs       []ToolOutput      `json:"outputs"`
	ResponseItems []json.RawMessage `json:"response_items,omitempty"`
}

type ModelEvent struct {
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Type         string `json:"type"` // "token", "citation", "done", "error"
	Content      string `json:"content,omitempty"`
	Error        error  `json:"error,omitempty"`
}

type Provider interface {
	Capabilities(ctx context.Context) (ModelCapabilities, error)
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
	Stream(ctx context.Context, req GenerateRequest) (<-chan ModelEvent, error)
}
