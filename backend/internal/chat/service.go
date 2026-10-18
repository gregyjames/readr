package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/backend/internal/llm"
)

type ArticleFetcher interface {
	GetMarkdownContent(ctx context.Context, id int64) (string, error)
	GetLinkedArticles(ctx context.Context, id int64) ([]Attachment, error)
}

type Service struct {
	repo         *FileRepository
	fetcher      ArticleFetcher
	provider     llm.Provider
	defaultModel string
}

func NewService(repo *FileRepository, fetcher ArticleFetcher, provider ...llm.Provider) *Service {
	var p llm.Provider
	if len(provider) > 0 {
		p = provider[0]
	}

	return &Service{
		repo:         repo,
		fetcher:      fetcher,
		provider:     p,
		defaultModel: "openai/gpt-4o-mini",
	}
}

func (s *Service) SetProvider(p llm.Provider) {
	s.provider = p
}

func (s *Service) GetModels(ctx context.Context, provider ...llm.Provider) ([]llm.ModelInfo, error) {
	active := s.provider
	if len(provider) > 0 && provider[0] != nil {
		active = provider[0]
	}
	if active == nil {
		return nil, errors.New("no LLM provider configured")
	}

	return active.ListModels(ctx)
}

func (s *Service) FetchModels(ctx context.Context, provider ...llm.Provider) ([]byte, error) {
	models, err := s.GetModels(ctx, provider...)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"data": models})
}

func (s *Service) StreamMessage(ctx context.Context, sessionID string, provider llm.Provider, model string, expandContext bool, userMsg Message, onChunk func(string) error) error {
	activeProvider := provider
	if activeProvider == nil {
		activeProvider = s.provider
	}
	if activeProvider == nil {
		return errors.New("no LLM provider configured")
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

	// 2. Prepare payload for LLM Provider
	reqMessages := make([]llm.Message, len(session.Messages))
	for i, m := range session.Messages {
		content := m.Content
		// For the latest message, use the injected context content
		if i == len(session.Messages)-1 && m.Role == RoleUser {
			content = effectiveUserContent
		}
		reqMessages[i] = llm.Message{
			Role:    llm.Role(m.Role),
			Content: content,
		}
	}

	chosenModel := strings.TrimSpace(model)
	if chosenModel == "" {
		chosenModel = s.defaultModel
	}

	compReq := llm.CompletionRequest{
		Model:    chosenModel,
		Messages: reqMessages,
	}

	var fullAssistantResponse strings.Builder
	err = activeProvider.Stream(ctx, compReq, func(chunk string) error {
		fullAssistantResponse.WriteString(chunk)
		if onChunk != nil {
			return onChunk(chunk)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 3. Save assistant response to session
	session.Messages = append(session.Messages, Message{
		Role:    RoleAssistant,
		Content: fullAssistantResponse.String(),
	})
	session.UpdatedAt = time.Now()

	return s.repo.Save(ctx, session)
}
