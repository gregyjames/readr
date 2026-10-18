package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Config configures an LLM provider.
type Config struct {
	Type          string            `json:"type"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key"`
	CustomHeaders map[string]string `json:"custom_headers"`
	HTTPClient    *http.Client      `json:"-"`
}

// OpenAICompatibleProvider interacts with any OpenAI-compatible API endpoint.
type OpenAICompatibleProvider struct {
	cfg        Config
	httpClient *http.Client
}

// NewProvider returns a Provider implementation based on the supplied configuration.
func NewProvider(cfg Config) Provider {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		switch strings.ToLower(cfg.Type) {
		case "ollama":
			baseURL = "http://localhost:11434/v1"
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		default:
			baseURL = "https://openrouter.ai/api/v1"
		}
	}
	cfg.BaseURL = baseURL

	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &OpenAICompatibleProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

// Name returns the provider identifier or type.
func (p *OpenAICompatibleProvider) Name() string {
	if p.cfg.Type != "" {
		return p.cfg.Type
	}
	return "openai-compatible"
}

func (p *OpenAICompatibleProvider) applyHeaders(req *http.Request) {
	if strings.ToLower(p.cfg.Type) == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://readr.app")
		req.Header.Set("X-Title", "Readr")
	}

	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	for k, v := range p.cfg.CustomHeaders {
		req.Header.Set(k, v)
	}
}

type chatCompletionPayload struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    *float64  `json:"temperature,omitempty"`
	MaxTokens      *int      `json:"max_tokens,omitempty"`
	Stream         bool      `json:"stream,omitempty"`
	ResponseFormat any       `json:"response_format,omitempty"`
}

func buildResponseFormat(req CompletionRequest) any {
	if req.ResponseSchema != nil {
		return map[string]any{
			"type":        "json_schema",
			"json_schema": req.ResponseSchema,
		}
	}
	if req.JSONMode {
		return map[string]string{"type": "json_object"}
	}
	return nil
}

func parseErrorResponse(statusCode int, body io.Reader) error {
	snippet, _ := io.ReadAll(io.LimitReader(body, 2048))
	var errPayload struct {
		Error struct {
			Message string      `json:"message"`
			Code    interface{} `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(snippet, &errPayload); err == nil && errPayload.Error.Message != "" {
		return fmt.Errorf("provider error (HTTP %d): %s", statusCode, errPayload.Error.Message)
	}
	trimmed := strings.TrimSpace(string(snippet))
	if len(trimmed) > 0 {
		return fmt.Errorf("provider error (HTTP %d): %s", statusCode, trimmed)
	}
	return fmt.Errorf("provider error (HTTP %d)", statusCode)
}

// Complete performs a non-streaming chat completion request.
func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	payload := chatCompletionPayload{
		Model:          req.Model,
		Messages:       req.Messages,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		Stream:         false,
		ResponseFormat: buildResponseFormat(req),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request payload: %w", err)
	}

	reqURL := p.cfg.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create completion request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.applyHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("completion request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseErrorResponse(resp.StatusCode, resp.Body)
	}

	var respPayload struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string      `json:"message"`
			Code    interface{} `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, fmt.Errorf("failed to decode completion response: %w", err)
	}

	if respPayload.Error != nil && respPayload.Error.Message != "" {
		return nil, fmt.Errorf("provider error: %s", respPayload.Error.Message)
	}

	if len(respPayload.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in completion response")
	}

	var usage *UsageInfo
	if respPayload.Usage != nil {
		usage = &UsageInfo{
			PromptTokens:     respPayload.Usage.PromptTokens,
			CompletionTokens: respPayload.Usage.CompletionTokens,
			TotalTokens:      respPayload.Usage.TotalTokens,
		}
	}

	return &CompletionResponse{
		Content: respPayload.Choices[0].Message.Content,
		Usage:   usage,
	}, nil
}

// Stream performs a streaming chat completion request and invokes onChunk for each incoming content delta.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req CompletionRequest, onChunk func(chunk string) error) error {
	payload := chatCompletionPayload{
		Model:          req.Model,
		Messages:       req.Messages,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		Stream:         true,
		ResponseFormat: buildResponseFormat(req),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request payload: %w", err)
	}

	reqURL := p.cfg.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	p.applyHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorResponse(resp.StatusCode, resp.Body)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("stream read error: %w", readErr)
		}

		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			var streamChunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if jsonErr := json.Unmarshal([]byte(data), &streamChunk); jsonErr == nil {
				for _, choice := range streamChunk.Choices {
					if choice.Delta.Content != "" && onChunk != nil {
						if err := onChunk(choice.Delta.Content); err != nil {
							return err
						}
					}
				}
			}
		}

		if readErr == io.EOF {
			break
		}
	}

	return nil
}

// ListModels queries /models and returns the list of available models.
func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	reqURL := p.cfg.BaseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create list models request: %w", err)
	}

	p.applyHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("list models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseErrorResponse(resp.StatusCode, resp.Body)
	}

	var modelsResp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("decode list models response: %w", err)
	}

	models := make([]ModelInfo, 0, len(modelsResp.Data))
	for _, item := range modelsResp.Data {
		name := item.Name
		if name == "" {
			name = item.ID
		}
		models = append(models, ModelInfo{
			ID:   item.ID,
			Name: name,
		})
	}

	return models, nil
}
