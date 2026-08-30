package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestService_StreamMessage_MissingAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	svc := NewService(repo, nil)

	err := svc.StreamMessage(context.Background(), "session-1", "", Message{
		Role:    RoleUser,
		Content: "Hello",
	}, nil)

	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
	if !strings.Contains(err.Error(), "missing OpenRouter API key") {
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

	var receivedReq openRouterRequest
	var receivedAuthHeader, receivedReferer, receivedTitle, receivedContentType string

	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		receivedAuthHeader = req.Header.Get("Authorization")
		receivedReferer = req.Header.Get("HTTP-Referer")
		receivedTitle = req.Header.Get("X-Title")
		receivedContentType = req.Header.Get("Content-Type")

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &receivedReq); err != nil {
			return nil, err
		}

		sseData := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			``,
			`data: {"choices":[{"delta":{"content":" there,"}}]}`,
			``,
			`: comment line to ignore`,
			``,
			`data: {"choices":[{"delta":{"content":" world!"}}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseData)),
		}, nil
	})

	svc := NewService(repo, fetcher)
	svc.SetHTTPClient(mockClient)

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
	err := svc.StreamMessage(ctx, sessionID, "test-api-key", userMsg, onChunk)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	// Verify Headers
	if receivedAuthHeader != "Bearer test-api-key" {
		t.Errorf("expected Authorization Bearer test-api-key, got %q", receivedAuthHeader)
	}
	if receivedReferer != "https://github.com/gregyjames/readr" {
		t.Errorf("expected HTTP-Referer header, got %q", receivedReferer)
	}
	if receivedTitle != "Readr Chat" {
		t.Errorf("expected X-Title header, got %q", receivedTitle)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}

	// Verify Request payload
	if !receivedReq.Stream {
		t.Errorf("expected stream to be true")
	}
	if receivedReq.Model != "openai/gpt-3.5-turbo" {
		t.Errorf("expected default model openai/gpt-3.5-turbo, got %q", receivedReq.Model)
	}
	if len(receivedReq.Messages) != 1 {
		t.Fatalf("expected 1 message in payload, got %d", len(receivedReq.Messages))
	}

	firstMsgMap, ok := receivedReq.Messages[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected message to be map[string]interface{}, got %T", receivedReq.Messages[0])
	}
	firstMsgContent, ok := firstMsgMap["content"].(string)
	if !ok {
		t.Fatalf("expected message content to be string")
	}
	if !strings.Contains(firstMsgContent, `<article id="101" title="Go Concurrency">`) {
		t.Errorf("expected payload content to contain article context, got %q", firstMsgContent)
	}
	if !strings.Contains(firstMsgContent, "Tell me about Go") {
		t.Errorf("expected payload content to contain user request, got %q", firstMsgContent)
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

	// Pre-populate session
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

	var receivedReq openRouterRequest
	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &receivedReq)

		sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"Sure, here is the answer.\"}}]}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseData)),
		}, nil
	})

	svc := NewService(repo, nil)
	svc.SetHTTPClient(mockClient)

	longUserMsg := Message{
		Role:    RoleUser,
		Content: "This is a very long message that definitely exceeds thirty characters in length.",
	}

	err := svc.StreamMessage(ctx, sessionID, "key", longUserMsg, nil)
	if err != nil {
		t.Fatalf("StreamMessage failed: %v", err)
	}

	// Verify all 3 turns were sent to OpenRouter
	if len(receivedReq.Messages) != 3 {
		t.Fatalf("expected 3 messages sent to API, got %d", len(receivedReq.Messages))
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

	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseData)),
		}, nil
	})

	svc := NewService(repo, nil)
	svc.SetHTTPClient(mockClient)

	longMsg := "This is a very long message that definitely exceeds thirty characters in length."
	sessionID := "truncated-title-session"
	err := svc.StreamMessage(context.Background(), sessionID, "key", Message{
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

func TestService_StreamMessage_ServerError(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(bytes.NewReader([]byte("Unauthorized"))),
		}, nil
	})

	svc := NewService(repo, nil)
	svc.SetHTTPClient(mockClient)

	err := svc.StreamMessage(context.Background(), "session-err", "bad-key", Message{
		Role:    RoleUser,
		Content: "Hi",
	}, nil)

	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if !strings.Contains(err.Error(), "openrouter error (HTTP 401)") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestService_StreamMessage_CallbackError(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		sseData := "data: {\"choices\":[{\"delta\":{\"content\":\"Chunk 1\"}}]}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sseData)),
		}, nil
	})

	svc := NewService(repo, nil)
	svc.SetHTTPClient(mockClient)

	expectedErr := errors.New("client connection closed")
	onChunk := func(chunk string) error {
		return expectedErr
	}

	err := svc.StreamMessage(context.Background(), "session-cb-err", "key", Message{
		Role:    RoleUser,
		Content: "Hi",
	}, onChunk)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected callback error %v, got %v", expectedErr, err)
	}
}

func TestService_StreamMessage_RequestFailure(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	networkErr := errors.New("network failure")
	mockClient := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return nil, networkErr
	})

	svc := NewService(repo, nil)
	svc.SetHTTPClient(mockClient)

	err := svc.StreamMessage(context.Background(), "session-req-err", "key", Message{
		Role:    RoleUser,
		Content: "Hi",
	}, nil)

	if err == nil {
		t.Fatal("expected request error, got nil")
	}
	if !strings.Contains(err.Error(), "openrouter request failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}
