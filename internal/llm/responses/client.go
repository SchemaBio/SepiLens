package responses

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/ssrf"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

var ErrEmptyAPIKey = errors.New("api key is empty")

type Config struct {
	BaseURL, APIKey, Model string
	AllowPrivate           bool
	Protocol               llm.Protocol
}
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	caps llm.ModelCapabilities
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if _, err := ssrf.ValidateEndpoint(cfg.BaseURL, cfg.AllowPrivate); err != nil {
		return nil, err
	}
	if cfg.Protocol == "" {
		cfg.Protocol = llm.ProtocolOpenAIResponses
	}
	if cfg.Protocol != llm.ProtocolOpenAIResponses && cfg.Protocol != llm.ProtocolOpenAIChat {
		return nil, errors.New("unsupported model protocol")
	}
	return &Client{cfg: cfg, http: ssrf.NewHTTPClient(cfg.AllowPrivate, nil)}, nil
}
func (c *Client) Capabilities(context.Context) (llm.ModelCapabilities, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps, nil
}
func (c *Client) request(ctx context.Context, r llm.GenerateRequest, stream bool) (*http.Response, error) {
	if c.cfg.APIKey == "" {
		return nil, ErrEmptyAPIKey
	}
	model := r.Model
	if model == "" {
		model = c.cfg.Model
	}
	if model == "" {
		return nil, errors.New("model is required")
	}
	endpoint, _ := url.Parse(c.cfg.BaseURL)
	deepseekChat := endpoint != nil && strings.EqualFold(endpoint.Hostname(), "api.deepseek.com") && c.cfg.Protocol == llm.ProtocolOpenAIChat
	system := r.System
	if deepseekChat && r.JSONSchema != nil {
		// DeepSeek Chat supports JSON mode rather than OpenAI's json_schema wire
		// format. Include the schema in the prompt and still validate it locally.
		schema, err := json.Marshal(r.JSONSchema)
		if err != nil {
			return nil, errors.New("invalid model schema")
		}
		system += "\nReturn only a JSON object conforming to this JSON Schema: " + string(schema)
	}
	p := map[string]any{"model": model, "stream": stream}
	path := "responses"
	if c.cfg.Protocol == llm.ProtocolOpenAIResponses {
		p["input"] = r.Prompt
		p["instructions"] = r.System
		p["store"] = false
		if r.JSONSchema != nil {
			p["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "sepilens", "strict": true, "schema": r.JSONSchema}}
		}
		if r.MaxTokens != nil {
			p["max_output_tokens"] = *r.MaxTokens
		}
	} else {
		path = "chat/completions"
		p["messages"] = []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": r.Prompt}}
		if r.JSONSchema != nil {
			p["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "sepilens", "strict": true, "schema": r.JSONSchema}}
			if deepseekChat {
				p["response_format"] = map[string]any{"type": "json_object"}
			}
		}
		if r.MaxTokens != nil {
			p["max_completion_tokens"] = *r.MaxTokens
			if deepseekChat {
				delete(p, "max_completion_tokens")
				p["max_tokens"] = *r.MaxTokens
			}
		}
		if stream {
			p["stream_options"] = map[string]bool{"include_usage": true}
		}
	}
	if r.Temperature != nil {
		p["temperature"] = *r.Temperature
	}
	if len(r.Tools) > 0 || len(r.History) > 0 {
		if stream {
			return nil, errors.New("streaming tool execution is not supported")
		}
		tools := []any{}
		for _, t := range r.Tools {
			f := map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters, "strict": true}
			if c.cfg.Protocol == llm.ProtocolOpenAIResponses {
				f["type"] = "function"
				tools = append(tools, f)
			} else {
				tools = append(tools, map[string]any{"type": "function", "function": f})
			}
		}
		if len(tools) > 0 {
			p["tools"] = tools
			p["parallel_tool_calls"] = false
		}
		if c.cfg.Protocol == llm.ProtocolOpenAIResponses {
			input := []any{map[string]any{"role": "user", "content": r.Prompt}}
			for _, turn := range r.History {
				if len(turn.ResponseItems) > 0 {
					for _, item := range turn.ResponseItems {
						input = append(input, item)
					}
				} else {
					for _, call := range turn.Calls {
						input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments})
					}
				}
				for _, out := range turn.Outputs {
					input = append(input, map[string]any{"type": "function_call_output", "call_id": out.CallID, "output": out.Output})
				}
			}
			p["input"] = input
		} else {
			messages := []any{map[string]any{"role": "system", "content": system}, map[string]any{"role": "user", "content": r.Prompt}}
			for _, turn := range r.History {
				calls := []any{}
				for _, call := range turn.Calls {
					calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.Arguments}})
				}
				messages = append(messages, map[string]any{"role": "assistant", "tool_calls": calls})
				for _, out := range turn.Outputs {
					messages = append(messages, map[string]any{"role": "tool", "tool_call_id": out.CallID, "content": out.Output})
				}
			}
			p["messages"] = messages
		}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, errors.New("invalid model request")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.cfg.BaseURL, "/")+"/"+path, bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("invalid model endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("model transport unavailable")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("model request failed (HTTP %d)", res.StatusCode)
	}
	return res, nil
}

type wire struct {
	Status string `json:"status"`
	Output []struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		Finish string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *Client) Generate(ctx context.Context, r llm.GenerateRequest) (llm.GenerateResult, error) {
	res, err := c.request(ctx, r, false)
	if err != nil {
		return llm.GenerateResult{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return llm.GenerateResult{}, errors.New("model response unreadable")
	}
	var w wire
	if json.Unmarshal(b, &w) != nil {
		return llm.GenerateResult{}, errors.New("model response malformed")
	}
	out := llm.GenerateResult{FinishReason: w.Status, InputTokens: w.Usage.Input, OutputTokens: w.Usage.Output}
	if c.cfg.Protocol == llm.ProtocolOpenAIChat {
		if len(w.Choices) == 0 {
			return out, errors.New("model returned no choices")
		}
		out.Content = w.Choices[0].Message.Content
		out.FinishReason = w.Choices[0].Finish
		out.InputTokens = w.Usage.Prompt
		out.OutputTokens = w.Usage.Completion
		for _, call := range w.Choices[0].Message.ToolCalls {
			if call.Type != "function" {
				return out, errors.New("unsupported tool call type")
			}
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
		}
	} else {
		for _, o := range w.Output {
			if o.Type == "function_call" {
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{ID: o.CallID, Name: o.Name, Arguments: o.Arguments})
			}
			for _, v := range o.Content {
				if v.Type == "output_text" {
					out.Content += v.Text
				}
			}
		}
		var raw struct {
			Output []json.RawMessage `json:"output"`
		}
		if json.Unmarshal(b, &raw) == nil {
			out.ResponseItems = raw.Output
		}
	}
	if len(out.ToolCalls) > 0 {
		if len(r.Tools) == 0 || len(out.ToolCalls) > 100 {
			return out, errors.New("unexpected tool calls")
		}
		if (c.cfg.Protocol == llm.ProtocolOpenAIResponses && w.Status != "completed") || (c.cfg.Protocol == llm.ProtocolOpenAIChat && out.FinishReason != "tool_calls") {
			return out, errors.New("incomplete tool call output")
		}
		seen := map[string]bool{}
		for _, call := range out.ToolCalls {
			if call.ID == "" || seen[call.ID] || !json.Valid([]byte(call.Arguments)) {
				return out, errors.New("invalid tool call")
			}
			seen[call.ID] = true
		}
		return out, nil
	}
	if out.Content == "" || (c.cfg.Protocol == llm.ProtocolOpenAIChat && out.FinishReason != "stop") || (c.cfg.Protocol == llm.ProtocolOpenAIResponses && out.FinishReason != "completed") {
		return out, errors.New("model output incomplete")
	}
	if r.JSONSchema != nil {
		if err = llm.ValidateJSON(out.Content, r.JSONSchema); err != nil {
			return out, errors.New("model output violates schema")
		}
	}
	c.mu.Lock()
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(b, &metadata)
	c.caps.UsageReporting = len(metadata["usage"]) > 0 && string(metadata["usage"]) != "null"
	if r.JSONSchema != nil {
		c.caps.StructuredOutput = true
	}
	c.mu.Unlock()
	return out, nil
}
func (c *Client) Stream(ctx context.Context, r llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	res, err := c.request(ctx, r, true)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.ModelEvent, 8)
	go func() {
		defer close(ch)
		defer res.Body.Close()
		send := func(e llm.ModelEvent) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 4096), 4<<20)
		done := false
		var content strings.Builder
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				done = true
				break
			}
			var e struct {
				Type     string `json:"type"`
				Delta    string `json:"delta"`
				Response wire   `json:"response"`
				Choices  []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					Finish *string `json:"finish_reason"`
				} `json:"choices"`
				Usage wireUsage `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &e) != nil {
				send(llm.ModelEvent{Type: "error", Content: "malformed stream"})
				return
			}
			if e.Type == "error" || e.Type == "response.failed" || e.Type == "response.incomplete" {
				send(llm.ModelEvent{Type: "error", Content: "model stream failed"})
				return
			}
			delta := e.Delta
			for _, v := range e.Choices {
				delta += v.Delta.Content
				if v.Finish != nil {
					if *v.Finish != "stop" {
						send(llm.ModelEvent{Type: "error", Content: "model output incomplete"})
						return
					}
					done = true
				}
			}
			if delta != "" {
				content.WriteString(delta)
				if !send(llm.ModelEvent{Type: "token", Content: delta}) {
					return
				}
			}
			if e.Type == "response.completed" {
				done = true
				e.Usage = wireUsage{e.Response.Usage.Input, e.Response.Usage.Output, 0, 0}
			}
			if e.Usage.Input+e.Usage.Output+e.Usage.Prompt+e.Usage.Completion > 0 {
				if !send(llm.ModelEvent{Type: "usage", InputTokens: e.Usage.Input + e.Usage.Prompt, OutputTokens: e.Usage.Output + e.Usage.Completion}) {
					return
				}
			}
		}
		if sc.Err() != nil || !done {
			send(llm.ModelEvent{Type: "error", Content: "model stream interrupted"})
			return
		}
		if r.JSONSchema != nil {
			if err := llm.ValidateJSON(content.String(), r.JSONSchema); err != nil {
				send(llm.ModelEvent{Type: "error", Content: "model stream violates schema"})
				return
			}
		}
		c.mu.Lock()
		c.caps.Streaming = true
		c.mu.Unlock()
		send(llm.ModelEvent{Type: "done"})
	}()
	return ch, nil
}

type wireUsage struct {
	Input      int `json:"input_tokens"`
	Output     int `json:"output_tokens"`
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
}
