package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ArticleFetcher interface {
	GetMarkdownContent(ctx context.Context, id int64) (string, error)
	GetLinkedArticles(ctx context.Context, id int64) ([]Attachment, error)
}

type Service struct {
	repo          *FileRepository
	fetcher       ArticleFetcher
	client        *http.Client
	openRouterURL string
	defaultModel  string
}

func NewService(repo *FileRepository, fetcher ArticleFetcher) *Service {
	return &Service{
		repo:          repo,
		fetcher:       fetcher,
		client:        &http.Client{Timeout: 120 * time.Second},
		openRouterURL: "https://openrouter.ai/api/v1/chat/completions",
		defaultModel:  "openai/gpt-3.5-turbo",
	}
}

// SetOpenRouterURL allows overriding the URL for testing
func (s *Service) SetOpenRouterURL(url string) {
	s.openRouterURL = url
}

func (s *Service) SetHTTPClient(client *http.Client) {
	s.client = client
}

type openRouterRequest struct {
	Model    string        `json:"model"`
	Messages []interface{} `json:"messages"`
	Stream   bool          `json:"stream"`
}

func (s *Service) FetchModels(ctx context.Context, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
	req.Header.Set("X-Title", "Readr Chat")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models returned status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *Service) StreamMessage(ctx context.Context, sessionID string, apiKey string, model string, expandContext bool, userMsg Message, onChunk func(string) error) error {
	if apiKey == "" {
		return errors.New("missing OpenRouter API key")
	}

	session, err := s.repo.Get(ctx, sessionID)
	if err != nil || session == nil {
		session = &ChatSession{
			ID:        sessionID,
			Title:     "New Chat",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages:  make([]Message, 0),
		}
	}

	if session.Title == "" || session.Title == "New Chat" {
		titleSource := strings.TrimSpace(userMsg.Content)
		if titleSource == "" && len(userMsg.Attachments) > 0 {
			titleSource = userMsg.Attachments[0].Title
		}
		if titleSource != "" {
			runes := []rune(titleSource)
			if len(runes) > 30 {
				session.Title = string(runes[:30]) + "..."
			} else {
				session.Title = string(runes)
			}
		}
	}

	// 1. Build Context Prompt from Attachments and optional 1-hop expansion
	var contextContent strings.Builder
	processedArticleIDs := make(map[int64]bool)

	for _, att := range userMsg.Attachments {
		processedArticleIDs[att.ID] = true
		if s.fetcher != nil {
			content, err := s.fetcher.GetMarkdownContent(ctx, att.ID)
			if err == nil && content != "" {
				contextContent.WriteString(fmt.Sprintf("<article id=\"%d\" title=\"%s\" relationship=\"direct-mention\">\n%s\n</article>\n\n", att.ID, att.Title, content))
			}

			// If 1-hop graph context expansion is enabled, pull connected notes
			if expandContext {
				linked, err := s.fetcher.GetLinkedArticles(ctx, att.ID)
				if err == nil {
					for _, link := range linked {
						if !processedArticleIDs[link.ID] {
							processedArticleIDs[link.ID] = true
							linkedContent, err := s.fetcher.GetMarkdownContent(ctx, link.ID)
							if err == nil && linkedContent != "" {
								contextContent.WriteString(fmt.Sprintf("<article id=\"%d\" title=\"%s\" relationship=\"1-hop-connected (via %s)\">\n%s\n</article>\n\n", link.ID, link.Title, att.Title, linkedContent))
							}
						}
					}
				}
			}
		}
	}

	effectiveUserContent := userMsg.Content
	if contextContent.Len() > 0 {
		effectiveUserContent = fmt.Sprintf("Use the following referenced articles and knowledge graph context to answer the user request:\n\n%sUser Request:\n%s", contextContent.String(), userMsg.Content)
	}

	// Record the user message in session
	session.Messages = append(session.Messages, userMsg)

	// 2. Prepare payload for OpenRouter
	var apiMsgs []interface{}
	for i, m := range session.Messages {
		content := m.Content
		// For the latest message, use the injected context content
		if i == len(session.Messages)-1 && m.Role == RoleUser {
			content = effectiveUserContent
		}
		apiMsgs = append(apiMsgs, map[string]string{
			"role":    string(m.Role),
			"content": content,
		})
	}

	chosenModel := strings.TrimSpace(model)
	if chosenModel == "" {
		chosenModel = s.defaultModel
	}

	reqPayload := openRouterRequest{
		Model:    chosenModel,
		Messages: apiMsgs,
		Stream:   true,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.openRouterURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/gregyjames/readr")
	req.Header.Set("X-Title", "Readr Chat")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("openrouter request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		var errPayload struct {
			Error struct {
				Message string      `json:"message"`
				Code    interface{} `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(bodySnippet, &errPayload); err == nil && errPayload.Error.Message != "" {
			return fmt.Errorf("openrouter error (HTTP %d): %s", resp.StatusCode, errPayload.Error.Message)
		}
		if len(bodySnippet) > 0 {
			return fmt.Errorf("openrouter error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(bodySnippet)))
		}
		return fmt.Errorf("openrouter error (HTTP %d)", resp.StatusCode)
	}

	var fullAssistantResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(data) == "[DONE]" {
			break
		}

		var streamChunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &streamChunk); err == nil {
			for _, choice := range streamChunk.Choices {
				if choice.Delta.Content != "" {
					fullAssistantResponse.WriteString(choice.Delta.Content)
					if onChunk != nil {
						if err := onChunk(choice.Delta.Content); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	// 3. Save assistant response to session
	session.Messages = append(session.Messages, Message{
		Role:    RoleAssistant,
		Content: fullAssistantResponse.String(),
	})
	session.UpdatedAt = time.Now()

	return s.repo.Save(ctx, session)
}
