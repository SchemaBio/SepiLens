package responses

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/ssrf"
)

var (
	ErrEmptyAPIKey = errors.New("api key is empty")
)

type Config struct {
	BaseURL      string
	APIKey       string
	Model        string
	AllowPrivate bool
}

type Client struct {
	cfg    Config
	client *openai.Client
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	validatedURL, err := ssrf.ValidateEndpoint(cfg.BaseURL, cfg.AllowPrivate)
	if err != nil {
		return nil, fmt.Errorf("ssrf check failed: %w", err)
	}

	opts := []option.RequestOption{
		option.WithBaseURL(validatedURL.String()),
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}

	c := openai.NewClient(opts...)
	return &Client{
		cfg:    cfg,
		client: &c,
	}, nil
}

func (c *Client) Capabilities(ctx context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{
		StructuredOutput: true,
		Streaming:        true,
		FunctionCalling:  true,
		NativeWebSearch:  true,
		SearchCitations:  true,
		UsageReporting:   true,
	}, nil
}

func (c *Client) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResult, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	messages := []openai.ChatCompletionMessageParamUnion{}
	if req.System != "" {
		messages = append(messages, openai.SystemMessage(req.System))
	}
	messages = append(messages, openai.UserMessage(req.Prompt))

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: messages,
	}

	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.GenerateResult{}, fmt.Errorf("generate completion: %w", err)
	}

	content := ""
	finishReason := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
		finishReason = string(resp.Choices[0].FinishReason)
	}

	return llm.GenerateResult{
		Content:      content,
		InputTokens:  int(resp.Usage.PromptTokens),
		OutputTokens: int(resp.Usage.CompletionTokens),
		FinishReason: finishReason,
	}, nil
}

func (c *Client) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	ch := make(chan llm.ModelEvent, 1)
	ch <- llm.ModelEvent{
		Type:    "done",
		Content: "",
	}
	close(ch)
	return ch, nil
}
