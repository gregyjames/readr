package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Summarizer interface {
	Summarize(ctx context.Context, title, body, apiKey, model string) (string, error)
}

type OpenRouterSummarizer struct {
	client  *http.Client
	baseURL string
}

func NewOpenRouterSummarizer() *OpenRouterSummarizer {
	return &OpenRouterSummarizer{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://openrouter.ai/api/v1/chat/completions",
	}
}

func (s *OpenRouterSummarizer) Summarize(ctx context.Context, title, body, apiKey, model string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("api key is required")
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	content := body
	if len(content) > 6000 {
		content = content[:6000]
	}

	prompt := fmt.Sprintf("Generate a concise, high-signal 2-3 sentence executive summary of the following article.\n\nTitle: %s\n\nContent:\n%s\n\nReturn ONLY the summary text.", title, content)

	reqPayload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
	req.Header.Set("X-Title", "Readr Ingest Summarizer")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter returned status %d", resp.StatusCode)
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from LLM")
	}

	return strings.TrimSpace(res.Choices[0].Message.Content), nil
}
