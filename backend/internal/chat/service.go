package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ArticleFetcher interface {
	GetMarkdownContent(ctx context.Context, id int64) (string, error)
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

func (s *Service) StreamMessage(ctx context.Context, sessionID string, apiKey string, userMsg Message, onChunk func(string) error) error {
	if apiKey == "" {
		return errors.New("missing OpenRouter API key")
	}

	session, err := s.repo.Get(ctx, sessionID)
	if err != nil || session == nil {
		title := "New Chat"
		if len(userMsg.Content) > 30 {
			title = userMsg.Content[:30] + "..."
		} else if len(userMsg.Content) > 0 {
			title = userMsg.Content
		}
		session = &ChatSession{
			ID:        sessionID,
			Title:     title,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages:  make([]Message, 0),
		}
	}

	// 1. Build Context Prompt from Attachments
	var contextContent strings.Builder
	for _, att := range userMsg.Attachments {
		if s.fetcher != nil {
			content, err := s.fetcher.GetMarkdownContent(ctx, att.ID)
			if err == nil && content != "" {
				contextContent.WriteString(fmt.Sprintf("<article id=\"%d\" title=\"%s\">\n%s\n</article>\n\n", att.ID, att.Title, content))
			}
		}
	}

	effectiveUserContent := userMsg.Content
	if contextContent.Len() > 0 {
		effectiveUserContent = fmt.Sprintf("Use the following referenced articles as context:\n\n%sUser Request:\n%s", contextContent.String(), userMsg.Content)
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

	reqPayload := openRouterRequest{
		Model:    s.defaultModel,
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
