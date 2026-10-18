package ingest

import (
	"context"
	"fmt"
	"strings"

	"example.com/backend/internal/llm"
)

type Summarizer interface {
	Summarize(ctx context.Context, title, body, apiKey, model string) (string, error)
}

type ProviderSummarizer struct {
	provider llm.Provider
}

func NewProviderSummarizer(p ...llm.Provider) *ProviderSummarizer {
	var provider llm.Provider
	if len(p) > 0 {
		provider = p[0]
	}
	return &ProviderSummarizer{
		provider: provider,
	}
}

func NewOpenRouterSummarizer() *ProviderSummarizer {
	return NewProviderSummarizer()
}

func (s *ProviderSummarizer) Summarize(ctx context.Context, title, body, apiKey, model string) (string, error) {
	p := s.provider
	if p == nil {
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return "", fmt.Errorf("api key or provider is required")
		}
		p = llm.NewProvider(llm.Config{
			Type:   "openrouter",
			APIKey: apiKey,
		})
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	bodyRunes := []rune(body)
	if len(bodyRunes) > 6000 {
		body = string(bodyRunes[:6000]) // rune safe truncation
	}

	prompt := fmt.Sprintf("Generate a concise, high-signal 2-3 sentence executive summary of the following article.\n\nTitle: %s\n\nContent:\n%s\n\nReturn ONLY the summary text.", title, body)
	temp := 0.3

	req := llm.CompletionRequest{
		Model: model,
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "You are an expert article summarizer. Output only concise executive summaries.",
			},
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		Temperature: &temp,
	}

	resp, err := p.Complete(ctx, req)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}
