package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultCompleteTimeout   = 120 * time.Second
	defaultListModelsTimeout = 30 * time.Second
	defaultMaxAttempts       = 3
	maxStreamScannerBuffer   = 1024 * 1024 // 1 MiB
)

// Config configures an LLM provider.
type Config struct {
	Type          string            `json:"type"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key"`
	CustomHeaders map[string]string `json:"custom_headers"`
	HTTPClient    *http.Client      `json:"-"`
}

// DefaultBaseURL returns the canonical default base URL for the given provider type.
func DefaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "ollama":
		return "http://localhost:11434/v1"
	case "openai":
		return "https://api.openai.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return "https://openrouter.ai/api/v1"
	}
}

// RequiresAPIKey reports whether the provider type requires a non-empty API key.
// Local and custom OpenAI-compatible endpoints ("ollama", "custom") may run without an API key.
func RequiresAPIKey(providerType string) bool {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "ollama", "custom":
		return false
	default:
		return true
	}
}

func newDefaultHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	// Do not set a total client.Timeout so long-running SSE streams in Stream() are not cut off after headers arrive.
	return &http.Client{
		Transport: transport,
	}
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
		baseURL = DefaultBaseURL(cfg.Type)
	}
	cfg.BaseURL = baseURL

	client := cfg.HTTPClient
	if client == nil {
		client = newDefaultHTTPClient()
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

// Complete performs a non-streaming chat completion request with automatic retry on transport errors, HTTP 429, and HTTP 5xx.
// If ctx has no deadline, a default 120-second deadline is applied; existing caller deadlines (such as 90s Librarian contexts) are preserved.
func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCompleteTimeout)
		defer cancel()
	}

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
	var lastErr error
	retriesPerformed := 0

	for attempt := 0; attempt < defaultMaxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, &CompletionError{
					Err:        ctx.Err(),
					RetryCount: retriesPerformed,
				}
			case <-timer.C:
			}
			retriesPerformed = attempt
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, &CompletionError{
				Err:        fmt.Errorf("create completion request: %w", err),
				RetryCount: retriesPerformed,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		p.applyHeaders(httpReq)

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("completion request failed: %w", err)
			if ctx.Err() != nil {
				return nil, &CompletionError{
					Err:        lastErr,
					RetryCount: retriesPerformed,
				}
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
			lastErr = parseErrorResponse(resp.StatusCode, resp.Body)
			_ = resp.Body.Close()
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			nonRetryErr := parseErrorResponse(resp.StatusCode, resp.Body)
			_ = resp.Body.Close()
			return nil, &CompletionError{
				Err:        nonRetryErr,
				RetryCount: retriesPerformed,
			}
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
		decodeErr := json.NewDecoder(resp.Body).Decode(&respPayload)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, &CompletionError{
				Err:        fmt.Errorf("failed to decode completion response: %w", decodeErr),
				RetryCount: retriesPerformed,
			}
		}

		if respPayload.Error != nil && respPayload.Error.Message != "" {
			return nil, &CompletionError{
				Err:        fmt.Errorf("provider error: %s", respPayload.Error.Message),
				RetryCount: retriesPerformed,
			}
		}

		if len(respPayload.Choices) == 0 {
			return nil, &CompletionError{
				Err:        fmt.Errorf("empty choices in completion response"),
				RetryCount: retriesPerformed,
			}
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
			Content:    respPayload.Choices[0].Message.Content,
			Usage:      usage,
			RetryCount: retriesPerformed,
		}, nil
	}

	return nil, &CompletionError{
		Err:        lastErr,
		RetryCount: retriesPerformed,
	}
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

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamScannerBuffer)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
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
			Error *struct {
				Message string      `json:"message"`
				Code    interface{} `json:"code"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal([]byte(data), &streamChunk); jsonErr != nil {
			return fmt.Errorf("decode stream chunk: %w", jsonErr)
		}
		if streamChunk.Error != nil && streamChunk.Error.Message != "" {
			return fmt.Errorf("provider stream error: %s", streamChunk.Error.Message)
		}

		for _, choice := range streamChunk.Choices {
			if choice.Delta.Content != "" && onChunk != nil {
				if err := onChunk(choice.Delta.Content); err != nil {
					return err
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}

// ListModels queries /models and returns the list of available models.
// If ctx has no deadline, a default 30-second deadline is applied.
func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultListModelsTimeout)
		defer cancel()
	}

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
