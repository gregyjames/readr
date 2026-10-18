package llm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/backend/internal/llm"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestComplete_Success(t *testing.T) {
	temp := 0.7
	maxTokens := 100

	var receivedReq map[string]interface{}
	var receivedURL string
	var receivedMethod string

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		receivedURL = r.URL.String()
		receivedMethod = r.Method

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &receivedReq); err != nil {
			return nil, err
		}

		respBody := `{
			"id": "chatcmpl-123",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello, how can I help you today?"
					},
					"finish_reason": "stop"
				}
			]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(respBody)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "test-key",
		HTTPClient: mockClient,
	})

	resp, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You are a helpful assistant."},
			{Role: llm.RoleUser, Content: "Hello!"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Content != "Hello, how can I help you today?" {
		t.Errorf("expected content 'Hello, how can I help you today?', got %q", resp.Content)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST method, got %s", receivedMethod)
	}
	if !strings.HasSuffix(receivedURL, "/chat/completions") {
		t.Errorf("expected URL ending in /chat/completions, got %s", receivedURL)
	}

	// Verify request payload
	if receivedReq["model"] != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %v", receivedReq["model"])
	}
	respFormat, ok := receivedReq["response_format"].(map[string]interface{})
	if !ok || respFormat["type"] != "json_object" {
		t.Errorf("expected response_format type json_object, got %v", receivedReq["response_format"])
	}
}

func TestComplete_ErrorResponse(t *testing.T) {
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		respBody := `{
			"error": {
				"message": "Invalid API key provided",
				"type": "invalid_request_error",
				"code": "invalid_api_key"
			}
		}`
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(respBody)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "bad-key",
		HTTPClient: mockClient,
	})

	_, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid API key provided") {
		t.Errorf("expected error to mention 'Invalid API key provided', got: %v", err)
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices": []}`)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "key",
		HTTPClient: mockClient,
	})

	_, err := provider.Complete(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	})

	if err == nil {
		t.Fatal("expected error for empty choices, got nil")
	}
}

func TestStream_Success(t *testing.T) {
	var receivedReq map[string]interface{}

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &receivedReq); err != nil {
			return nil, err
		}

		sseData := strings.Join([]string{
			": keep-alive",
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			"",
			`data: {"choices":[{"delta":{"content":" world"}}]}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n")

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(sseData)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "key",
		HTTPClient: mockClient,
	})

	var collected strings.Builder
	var chunks []string
	err := provider.Stream(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	}, func(chunk string) error {
		collected.WriteString(chunk)
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	if collected.String() != "Hello world" {
		t.Errorf("expected collected content 'Hello world', got %q", collected.String())
	}
	if len(chunks) != 2 || chunks[0] != "Hello" || chunks[1] != " world" {
		t.Errorf("unexpected chunks slice: %v", chunks)
	}
	if receivedReq["stream"] != true {
		t.Errorf("expected stream: true, got %v", receivedReq["stream"])
	}
}

func TestStream_ErrorStatus(t *testing.T) {
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": {"message": "Service unavailable"}}`)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "key",
		HTTPClient: mockClient,
	})

	err := provider.Stream(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hi"},
		},
	}, func(chunk string) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected stream error on 500 status, got nil")
	}
	if !strings.Contains(err.Error(), "Service unavailable") {
		t.Errorf("expected error to contain 'Service unavailable', got %v", err)
	}
}

func TestStream_CallbackError(t *testing.T) {
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		sseData := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"chunk 1"}}]}`,
			"",
			`data: {"choices":[{"delta":{"content":"chunk 2"}}]}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(bytes.NewBufferString(sseData)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "key",
		HTTPClient: mockClient,
	})

	expectedErr := errors.New("abort processing")
	err := provider.Stream(context.Background(), llm.CompletionRequest{
		Model: "gpt-4o",
	}, func(chunk string) error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestListModels_Success(t *testing.T) {
	var receivedURL string
	var receivedMethod string

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		receivedURL = r.URL.String()
		receivedMethod = r.Method

		respBody := `{
			"data": [
				{
					"id": "openai/gpt-4o",
					"name": "GPT-4o"
				},
				{
					"id": "meta-llama/llama-3-8b"
				}
			]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(respBody)),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openrouter",
		BaseURL:    "https://openrouter.ai/api/v1",
		APIKey:     "key",
		HTTPClient: mockClient,
	})

	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedMethod != http.MethodGet {
		t.Errorf("expected GET method, got %s", receivedMethod)
	}
	if !strings.HasSuffix(receivedURL, "/models") {
		t.Errorf("expected URL ending in /models, got %s", receivedURL)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "openai/gpt-4o" || models[0].Name != "GPT-4o" {
		t.Errorf("model 0 mismatch: %+v", models[0])
	}
	if models[1].ID != "meta-llama/llama-3-8b" || models[1].Name != "meta-llama/llama-3-8b" {
		t.Errorf("model 1 mismatch (fallback to ID): %+v", models[1])
	}
}

func TestListModels_Error(t *testing.T) {
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(bytes.NewBufferString("gateway timeout")),
		}, nil
	})

	provider := llm.NewProvider(llm.Config{
		Type:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		HTTPClient: mockClient,
	})

	_, err := provider.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error from bad gateway, got nil")
	}
}

func TestHeaders_Formatting(t *testing.T) {
	var capturedHeaders http.Header

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		capturedHeaders = r.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices": [{"message": {"content": "ok"}}]}`)),
		}, nil
	})

	// 1. Remote provider with API key & OpenRouter attribution headers
	openRouterProvider := llm.NewProvider(llm.Config{
		Type:       "openrouter",
		BaseURL:    "https://openrouter.ai/api/v1",
		APIKey:     "sk-or-test1234",
		HTTPClient: mockClient,
	})

	_, err := openRouterProvider.Complete(context.Background(), llm.CompletionRequest{
		Model: "some-model",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if auth := capturedHeaders.Get("Authorization"); auth != "Bearer sk-or-test1234" {
		t.Errorf("expected Bearer sk-or-test1234, got %q", auth)
	}
	if ref := capturedHeaders.Get("HTTP-Referer"); ref == "" {
		t.Errorf("expected non-empty HTTP-Referer header for openrouter")
	}
	if title := capturedHeaders.Get("X-Title"); title == "" {
		t.Errorf("expected non-empty X-Title header for openrouter")
	}

	// 2. Local provider without API key (e.g. Ollama)
	ollamaProvider := llm.NewProvider(llm.Config{
		Type:       "ollama",
		BaseURL:    "http://localhost:11434/v1",
		APIKey:     "",
		HTTPClient: mockClient,
	})

	capturedHeaders = nil
	_, err = ollamaProvider.Complete(context.Background(), llm.CompletionRequest{
		Model: "llama3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if auth := capturedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("expected no Authorization header for empty APIKey, got %q", auth)
	}

	// 3. Custom headers
	customProvider := llm.NewProvider(llm.Config{
		Type:       "custom",
		BaseURL:    "https://api.custom.com/v1",
		HTTPClient: mockClient,
		CustomHeaders: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	})

	capturedHeaders = nil
	_, err = customProvider.Complete(context.Background(), llm.CompletionRequest{
		Model: "custom-model",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val := capturedHeaders.Get("X-Custom-Header"); val != "custom-value" {
		t.Errorf("expected custom header value 'custom-value', got %q", val)
	}
}

func TestNewProvider_DefaultBaseURLs(t *testing.T) {
	testCases := []struct {
		providerType    string
		expectedName    string
		expectedBaseURL string
	}{
		{"openrouter", "openrouter", "https://openrouter.ai/api/v1"},
		{"ollama", "ollama", "http://localhost:11434/v1"},
		{"openai", "openai", "https://api.openai.com/v1"},
		{"custom", "custom", "https://openrouter.ai/api/v1"},
		{"", "openai-compatible", "https://openrouter.ai/api/v1"},
	}

	for _, tc := range testCases {
		var requestedURL string
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			requestedURL = r.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": []}`)),
			}, nil
		})

		p := llm.NewProvider(llm.Config{
			Type:       tc.providerType,
			HTTPClient: mockClient,
		})

		if p.Name() != tc.expectedName {
			t.Errorf("expected Name() %q, got %q", tc.expectedName, p.Name())
		}

		_, err := p.ListModels(context.Background())
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.providerType, err)
		}

		expectedFullURL := tc.expectedBaseURL + "/models"
		if requestedURL != expectedFullURL {
			t.Errorf("for provider type %q: expected URL %q, got %q", tc.providerType, expectedFullURL, requestedURL)
		}
	}
}

func TestProvider_Complete_ResponseSchema(t *testing.T) {
	var capturedBody map[string]interface{}
	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"role":"assistant","content":"{\"summary\":\"ok\"}"}}]}`)),
		}, nil
	})

	p := llm.NewProvider(llm.Config{
		Type:       "ollama",
		HTTPClient: mockClient,
	})

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "llama3.2",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Summarize"},
		},
		JSONMode: true,
		ResponseSchema: &llm.JSONSchemaConfig{
			Name:   "test_schema",
			Strict: true,
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{"type": "string"},
				},
				"required":             []string{"summary"},
				"additionalProperties": false,
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.Content != `{"summary":"ok"}` {
		t.Errorf("unexpected response content: %s", resp.Content)
	}

	rf, ok := capturedBody["response_format"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response_format in request payload, got: %v", capturedBody["response_format"])
	}
	if rf["type"] != "json_schema" {
		t.Errorf("expected response_format.type 'json_schema', got %v", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected json_schema object in response_format, got: %v", rf["json_schema"])
	}
	if js["name"] != "test_schema" || js["strict"] != true {
		t.Errorf("expected name='test_schema' and strict=true, got: %v", js)
	}
}
