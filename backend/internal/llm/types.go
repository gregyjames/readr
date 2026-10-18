package llm

import "context"

// Role represents the role of an entity in a chat conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single message in a chat completion request.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// JSONSchemaConfig defines a structured JSON schema for response_format enforcement.
type JSONSchemaConfig struct {
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

// CompletionRequest represents the input parameters for a completion.
type CompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      *int              `json:"max_tokens,omitempty"`
	JSONMode       bool              `json:"json_mode,omitempty"`
	ResponseSchema *JSONSchemaConfig `json:"response_schema,omitempty"`
}

// UsageInfo provides token usage statistics for a completion.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse represents the result of a completion.
type CompletionResponse struct {
	Content string     `json:"content"`
	Usage   *UsageInfo `json:"usage,omitempty"`
}

// ModelInfo describes a model available on the provider.
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Provider defines the interface for interacting with LLM backends.
type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest, onChunk func(chunk string) error) error
	ListModels(ctx context.Context) ([]ModelInfo, error)
}
