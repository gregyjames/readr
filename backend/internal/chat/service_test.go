package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/backend/internal/llm"
)

type mockProvider struct {
	name           string
	completeFunc   func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	streamFunc     func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error
	listModelsFunc func(ctx context.Context) ([]llm.ModelInfo, error)
}

func (m *mockProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, req)
	}
	return &llm.CompletionResponse{Content: "mock content"}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, req, onChunk)
	}
	return nil
}

func (m *mockProvider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	if m.listModelsFunc != nil {
		return m.listModelsFunc(ctx)
	}
	return []llm.ModelInfo{{ID: "mock-model", Name: "Mock Model"}}, nil
}

type mockArticleFetcher struct {
	articles map[int64]string
	err      error
}

func (m *mockArticleFetcher) GetMarkdownContent(ctx context.Context, id int64) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	content, ok := m.articles[id]
	if !ok {
		return "", errors.New("article not found")
	}
	return content, nil
}

func (m *mockArticleFetcher) GetLinkedArticles(ctx context.Context, id int64) ([]Attachment, error) {
	return nil, nil
}

func TestService_StreamMessage_MissingProvider(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	svc := NewService(repo, nil)

	err := svc.StreamMessage(context.Background(), "session-1", nil, "", false, Message{
		Role:    RoleUser,
		Content: "Hello",
	}, nil)

	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
	if !strings.Contains(err.Error(), "no LLM provider configured") && !strings.Contains(err.Error(), "missing LLM provider") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestService_StreamMessage_Success(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	fetcher := &mockArticleFetcher{
		articles: map[int64]string{
			101: "# Go Concurrency\nGo has goroutines and channels.",
		},
	}

	var capturedReq llm.CompletionRequest
	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			capturedReq = req
			chunks := []string{"Hello", " there,", " world!"}
			for _, c := range chunks {
				if err := onChunk(c); err != nil {
					return err
				}
			}
			return nil
		},
	}

	svc := NewService(repo, fetcher, mockP)

	var streamedChunks []string
	onChunk := func(chunk string) error {
		streamedChunks = append(streamedChunks, chunk)
		return nil
	}

	userMsg := Message{
		Role:    RoleUser,
		Content: "Tell me about Go",
		Attachments: []Attachment{
			{ID: 101, Title: "Go Concurrency"},
		},
	}

	ctx := context.Background()
	sessionID := "session-abc"
	err := svc.StreamMessage(ctx, sessionID, nil, "anthropic/claude-3.5-sonnet", false, userMsg, onChunk)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	// Verify Request payload delegated to Provider
	if capturedReq.Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("expected model anthropic/claude-3.5-sonnet, got %q", capturedReq.Model)
	}
	if len(capturedReq.Messages) != 1 {
		t.Fatalf("expected 1 message in payload, got %d", len(capturedReq.Messages))
	}

	firstMsg := capturedReq.Messages[0]
	if firstMsg.Role != llm.RoleUser {
		t.Errorf("expected user role, got %s", firstMsg.Role)
	}
	if !strings.Contains(firstMsg.Content, `id="101"`) || !strings.Contains(firstMsg.Content, `title="Go Concurrency"`) {
		t.Errorf("expected payload content to contain article context, got %q", firstMsg.Content)
	}
	if !strings.Contains(firstMsg.Content, "Tell me about Go") {
		t.Errorf("expected payload content to contain user request, got %q", firstMsg.Content)
	}

	// Verify streamed chunks
	expectedChunks := []string{"Hello", " there,", " world!"}
	if len(streamedChunks) != len(expectedChunks) {
		t.Fatalf("expected %d chunks, got %d: %v", len(expectedChunks), len(streamedChunks), streamedChunks)
	}
	for i, chunk := range expectedChunks {
		if streamedChunks[i] != chunk {
			t.Errorf("chunk %d expected %q, got %q", i, chunk, streamedChunks[i])
		}
	}

	// Verify Session saved in repository
	savedSession, err := repo.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get saved session: %v", err)
	}
	if savedSession.ID != sessionID {
		t.Errorf("expected session ID %q, got %q", sessionID, savedSession.ID)
	}
	if savedSession.Title != "Tell me about Go" {
		t.Errorf("expected title %q, got %q", "Tell me about Go", savedSession.Title)
	}
	if len(savedSession.Messages) != 2 {
		t.Fatalf("expected 2 messages in session, got %d", len(savedSession.Messages))
	}
	if savedSession.Messages[0].Role != RoleUser || savedSession.Messages[0].Content != "Tell me about Go" {
		t.Errorf("unexpected user message in session: %+v", savedSession.Messages[0])
	}
	if savedSession.Messages[1].Role != RoleAssistant || savedSession.Messages[1].Content != "Hello there, world!" {
		t.Errorf("unexpected assistant message in session: %+v", savedSession.Messages[1])
	}
}

func TestService_StreamMessage_MultiTurnAndLongTitle(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()
	sessionID := "multi-turn-session"

	initialSession := &ChatSession{
		ID:        sessionID,
		Title:     "Initial Conversation",
		CreatedAt: time.Now().Add(-10 * time.Minute),
		UpdatedAt: time.Now().Add(-10 * time.Minute),
		Messages: []Message{
			{Role: RoleUser, Content: "Hello assistant"},
			{Role: RoleAssistant, Content: "Hello user"},
		},
	}
	if err := repo.Save(ctx, initialSession); err != nil {
		t.Fatalf("failed to save initial session: %v", err)
	}

	var capturedReq llm.CompletionRequest
	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			capturedReq = req
			if onChunk != nil {
				_ = onChunk("Sure, here is the answer.")
			}
			return nil
		},
	}

	svc := NewService(repo, nil, mockP)

	longUserMsg := Message{
		Role:    RoleUser,
		Content: "This is a very long message that definitely exceeds thirty characters in length.",
	}

	err := svc.StreamMessage(ctx, sessionID, nil, "", false, longUserMsg, nil)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	// Verify all 3 turns were sent to Provider
	if len(capturedReq.Messages) != 3 {
		t.Fatalf("expected 3 messages sent to API, got %d", len(capturedReq.Messages))
	}

	// Verify session saved has 4 messages and retained original title
	updatedSession, err := repo.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to load updated session: %v", err)
	}
	if updatedSession.Title != "Initial Conversation" {
		t.Errorf("expected session title 'Initial Conversation', got %q", updatedSession.Title)
	}
	if len(updatedSession.Messages) != 4 {
		t.Fatalf("expected 4 messages in session, got %d", len(updatedSession.Messages))
	}
	if updatedSession.Messages[3].Content != "Sure, here is the answer." {
		t.Errorf("expected assistant response, got %q", updatedSession.Messages[3].Content)
	}
}

func TestService_StreamMessage_TitleTruncationNewSession(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			if onChunk != nil {
				_ = onChunk("OK")
			}
			return nil
		},
	}

	svc := NewService(repo, nil, mockP)

	longMsg := "This is a very long message that definitely exceeds thirty characters in length."
	sessionID := "truncated-title-session"
	err := svc.StreamMessage(context.Background(), sessionID, nil, "", false, Message{
		Role:    RoleUser,
		Content: longMsg,
	}, nil)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	session, err := repo.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	expectedTitle := longMsg[:30] + "..."
	if session.Title != expectedTitle {
		t.Errorf("expected title %q, got %q", expectedTitle, session.Title)
	}
}

func TestService_StreamMessage_PreCreatedNewChatTitleUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()
	sessionID := "pre-created-session"

	initialSession := &ChatSession{
		ID:        sessionID,
		Title:     "New Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{},
	}
	if err := repo.Save(ctx, initialSession); err != nil {
		t.Fatalf("failed to save initial session: %v", err)
	}

	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			if onChunk != nil {
				_ = onChunk("Answer")
			}
			return nil
		},
	}

	svc := NewService(repo, nil, mockP)

	err := svc.StreamMessage(ctx, sessionID, nil, "", false, Message{
		Role:    RoleUser,
		Content: "how do i do this?",
	}, nil)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	updatedSession, err := repo.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if updatedSession.Title != "how do i do this?" {
		t.Errorf("expected updated title 'how do i do this?', got %q", updatedSession.Title)
	}
}

func TestService_StreamMessage_AttachmentOnlyTitle(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()
	sessionID := "attachment-title-session"

	initialSession := &ChatSession{
		ID:        sessionID,
		Title:     "New Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{},
	}
	if err := repo.Save(ctx, initialSession); err != nil {
		t.Fatalf("failed to save initial session: %v", err)
	}

	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			if onChunk != nil {
				_ = onChunk("Answer")
			}
			return nil
		},
	}

	svc := NewService(repo, nil, mockP)

	err := svc.StreamMessage(ctx, sessionID, nil, "", false, Message{
		Role:    RoleUser,
		Content: "",
		Attachments: []Attachment{
			{ID: 1, Title: "How to Get the M5 MacBook Air for Less Than at the Apple Store"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	updatedSession, err := repo.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	expectedTitle := "How to Get the M5 MacBook Air ..."
	if updatedSession.Title != expectedTitle {
		t.Errorf("expected updated title %q, got %q", expectedTitle, updatedSession.Title)
	}
}

func TestService_StreamMessage_ProviderError(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			return errors.New("provider error (HTTP 401): Unauthorized")
		},
	}

	svc := NewService(repo, nil, mockP)

	err := svc.StreamMessage(context.Background(), "session-err", nil, "", false, Message{
		Role:    RoleUser,
		Content: "Hi",
	}, nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "provider error (HTTP 401)") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestService_StreamMessage_CallbackError(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	expectedErr := errors.New("client connection closed")
	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			return onChunk("Chunk 1")
		},
	}

	svc := NewService(repo, nil, mockP)

	onChunk := func(chunk string) error {
		return expectedErr
	}

	err := svc.StreamMessage(context.Background(), "session-cb-err", nil, "", false, Message{
		Role:    RoleUser,
		Content: "Hi",
	}, onChunk)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected callback error %v, got %v", expectedErr, err)
	}
}

func TestService_StreamMessage_1HopExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	var receivedPrompt string
	mockP := &mockProvider{
		streamFunc: func(ctx context.Context, req llm.CompletionRequest, onChunk func(chunk string) error) error {
			if len(req.Messages) > 0 {
				receivedPrompt = req.Messages[0].Content
			}
			if onChunk != nil {
				_ = onChunk("Expanded answer")
			}
			return nil
		},
	}

	mockFetcher := &mockExpansionFetcher{
		articles: map[int64]string{
			1: "Main Article Content",
			2: "Connected Note Content",
		},
		links: map[int64][]Attachment{
			1: {{ID: 2, Title: "Connected Note"}},
		},
	}

	svc := NewService(repo, mockFetcher, mockP)

	userMsg := Message{
		Role:    RoleUser,
		Content: "Analyze connections",
		Attachments: []Attachment{
			{ID: 1, Title: "Main Article"},
		},
	}

	err := svc.StreamMessage(context.Background(), "session-exp", nil, "", true, userMsg, nil)
	if err != nil {
		t.Fatalf("StreamMessage with expansion failed: %v", err)
	}

	if !strings.Contains(receivedPrompt, "Main Article Content") {
		t.Errorf("expected direct mention in prompt, got: %s", receivedPrompt)
	}
	if !strings.Contains(receivedPrompt, "Connected Note Content") {
		t.Errorf("expected 1-hop connected note in prompt, got: %s", receivedPrompt)
	}
}

type mockExpansionFetcher struct {
	articles map[int64]string
	links    map[int64][]Attachment
}

func (m *mockExpansionFetcher) GetMarkdownContent(ctx context.Context, id int64) (string, error) {
	if content, ok := m.articles[id]; ok {
		return content, nil
	}
	return "", errors.New("not found")
}

func (m *mockExpansionFetcher) GetLinkedArticles(ctx context.Context, id int64) ([]Attachment, error) {
	if links, ok := m.links[id]; ok {
		return links, nil
	}
	return nil, nil
}

func TestService_GetModels(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	mockP := &mockProvider{
		listModelsFunc: func(ctx context.Context) ([]llm.ModelInfo, error) {
			return []llm.ModelInfo{
				{ID: "openai/gpt-4o", Name: "GPT-4o"},
				{ID: "anthropic/claude-3.5-sonnet", Name: "Claude 3.5 Sonnet"},
			}, nil
		},
	}

	svc := NewService(repo, nil, mockP)

	models, err := svc.GetModels(context.Background())
	if err != nil {
		t.Fatalf("GetModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "openai/gpt-4o" || models[1].ID != "anthropic/claude-3.5-sonnet" {
		t.Errorf("unexpected models returned: %+v", models)
	}
}
