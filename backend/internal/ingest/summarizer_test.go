package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/backend/internal/llm"
)

type mockLLMProvider struct {
	completeFunc func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error)
}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, req)
	}
	return &llm.CompletionResponse{Content: "Default summary"}, nil
}
func (m *mockLLMProvider) Stream(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
	return nil
}
func (m *mockLLMProvider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

func TestProviderSummarizer_Success(t *testing.T) {
	var capturedReq llm.CompletionRequest
	mockP := &mockLLMProvider{
		completeFunc: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			capturedReq = req
			return &llm.CompletionResponse{
				Content: "This is a great summary of the article.",
			}, nil
		},
	}

	summarizer := NewProviderSummarizer(mockP)
	summary, err := summarizer.Summarize(context.Background(), "Title", "Body content", "", "custom-model")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if summary != "This is a great summary of the article." {
		t.Errorf("got %q, want %q", summary, "This is a great summary of the article.")
	}

	if capturedReq.Model != "custom-model" {
		t.Errorf("expected model custom-model, got %s", capturedReq.Model)
	}
	if capturedReq.Temperature == nil || *capturedReq.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %v", capturedReq.Temperature)
	}
	if len(capturedReq.Messages) == 0 {
		t.Fatal("expected messages in completion request")
	}
	lastMsg := capturedReq.Messages[len(capturedReq.Messages)-1]
	if !strings.Contains(lastMsg.Content, "Title") || !strings.Contains(lastMsg.Content, "Body content") {
		t.Errorf("expected title and body in prompt, got %s", lastMsg.Content)
	}
}

func TestProviderSummarizer_MissingKeyAndProvider(t *testing.T) {
	summarizer := NewProviderSummarizer()
	_, err := summarizer.Summarize(context.Background(), "Title", "Body", "", "")
	if err == nil {
		t.Errorf("expected error when api key and provider are missing")
	}
}

func TestProviderSummarizer_ProviderError(t *testing.T) {
	mockP := &mockLLMProvider{
		completeFunc: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return nil, errors.New("upstream provider timeout")
		},
	}

	summarizer := NewProviderSummarizer(mockP)
	_, err := summarizer.Summarize(context.Background(), "Title", "Body", "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upstream provider timeout") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProviderSummarizer_RuneTruncation(t *testing.T) {
	var capturedReq llm.CompletionRequest
	mockP := &mockLLMProvider{
		completeFunc: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
			capturedReq = req
			return &llm.CompletionResponse{Content: "Summary"}, nil
		},
	}

	summarizer := NewProviderSummarizer(mockP)
	longBody := strings.Repeat("A", 10000)
	_, err := summarizer.Summarize(context.Background(), "Title", longBody, "", "")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	lastMsg := capturedReq.Messages[len(capturedReq.Messages)-1]
	if strings.Contains(lastMsg.Content, strings.Repeat("A", 6001)) {
		t.Errorf("expected body to be truncated to 6000 runes")
	}
}
