# Chat Interface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a dedicated `/chat` interface with persistent dual-storage markdown sessions, an `@mention` file attachment system, and an OpenRouter streaming proxy.

**Architecture:** The backend exposes a `/api/chats` REST API to manage chat sessions, storing state as JSON while concurrently exporting to Markdown. The frontend introduces `/chat` and `/settings` routes, using Server-Sent Events (SSE) to render AI responses dynamically and `localStorage` to manage the OpenRouter API key.

**Tech Stack:** Go (Fiber), Vue 3 (Composition API, Router), Tailwind CSS.

---

### Task 1: Define Backend Chat Domain Models

**Files:**
- Create: `backend/internal/chat/types.go`

- [ ] **Step 1: Write the minimal implementation**

```go
package chat

import "time"

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

type Attachment struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type Message struct {
	Role        MessageRole  `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/chat/types.go
git commit -m "feat(backend): define chat domain models"
```

---

### Task 2: Backend Dual-Storage Repository

**Files:**
- Create: `backend/internal/chat/repository.go`
- Create: `backend/internal/chat/repository_test.go`

- [ ] **Step 1: Write the failing test**

```go
package chat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileRepository(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "chats-*")
	defer os.RemoveAll(tmpDir)

	repo := NewFileRepository(tmpDir)
	session := &ChatSession{
		ID:        "123",
		Title:     "Test Chat",
		CreatedAt: time.Now(),
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	}

	err := repo.Save(context.Background(), session)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify JSON exists
	if _, err := os.Stat(filepath.Join(tmpDir, "123.json")); os.IsNotExist(err) {
		t.Error("JSON file was not created")
	}

	// Verify MD exists
	if _, err := os.Stat(filepath.Join(tmpDir, "123.md")); os.IsNotExist(err) {
		t.Error("Markdown file was not created")
	}

	loaded, err := repo.Get(context.Background(), "123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.Title != "Test Chat" {
		t.Errorf("Expected title 'Test Chat', got '%s'", loaded.Title)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/chat -v`
Expected: FAIL with "undefined: NewFileRepository"

- [ ] **Step 3: Write minimal implementation**

```go
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileRepository struct {
	baseDir string
}

func NewFileRepository(baseDir string) *FileRepository {
	os.MkdirAll(baseDir, 0755)
	return &FileRepository{baseDir: baseDir}
}

func (r *FileRepository) Save(ctx context.Context, session *ChatSession) error {
	// Save JSON
	jsonPath := filepath.Join(r.baseDir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		return err
	}

	// Save MD
	mdPath := filepath.Join(r.baseDir, session.ID+".md")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\ntitle: \"%s\"\ndate: \"%s\"\n---\n\n", session.Title, session.CreatedAt.Format("2006-01-02")))
	for _, m := range session.Messages {
		if m.Role == RoleUser {
			sb.WriteString("### User\n")
		} else {
			sb.WriteString(fmt.Sprintf("### %s\n", strings.Title(string(m.Role))))
		}
		sb.WriteString(m.Content + "\n\n")
	}
	return os.WriteFile(mdPath, []byte(sb.String()), 0644)
}

func (r *FileRepository) Get(ctx context.Context, id string) (*ChatSession, error) {
	jsonPath := filepath.Join(r.baseDir, id+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *FileRepository) List(ctx context.Context) ([]*ChatSession, error) {
	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, err
	}
	var sessions []*ChatSession
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			id := strings.TrimSuffix(f.Name(), ".json")
			if s, err := r.Get(ctx, id); err == nil {
				sessions = append(sessions, s)
			}
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (r *FileRepository) Delete(ctx context.Context, id string) error {
	os.Remove(filepath.Join(r.baseDir, id+".json"))
	os.Remove(filepath.Join(r.baseDir, id+".md"))
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/chat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/chat/
git commit -m "feat(backend): implement dual-storage file repository for chats"
```

---

### Task 3: Backend OpenRouter Streaming Proxy

**Files:**
- Create: `backend/internal/chat/service.go`

- [ ] **Step 1: Write the minimal implementation**

```go
package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ArticleFetcher interface {
	GetMarkdownContent(ctx context.Context, id int64) (string, error)
}

type Service struct {
	repo    *FileRepository
	fetcher ArticleFetcher
}

func NewService(repo *FileRepository, fetcher ArticleFetcher) *Service {
	return &Service{repo: repo, fetcher: fetcher}
}

type OpenRouterRequest struct {
	Model    string        `json:"model"`
	Messages []interface{} `json:"messages"`
	Stream   bool          `json:"stream"`
}

func (s *Service) StreamMessage(ctx context.Context, sessionID string, apiKey string, userMsg Message, onChunk func(string) error) error {
	session, err := s.repo.Get(ctx, sessionID)
	if err != nil {
		session = &ChatSession{ID: sessionID, Title: "New Chat", CreatedAt: time.Now()}
	}

	// Build Context Prompt from Attachments
	var contextContent string
	for _, att := range userMsg.Attachments {
		if s.fetcher != nil {
			content, err := s.fetcher.GetMarkdownContent(ctx, att.ID)
			if err == nil {
				contextContent += fmt.Sprintf("<article title=\"%s\">\n%s\n</article>\n", att.Title, content)
			}
		}
	}

	if contextContent != "" {
		userMsg.Content = "Use the following articles as context:\n" + contextContent + "\n\nUser Question:\n" + userMsg.Content
	}
	session.Messages = append(session.Messages, userMsg)

	// Map to OpenRouter
	var apiMsgs []interface{}
	for _, m := range session.Messages {
		apiMsgs = append(apiMsgs, map[string]string{"role": string(m.Role), "content": m.Content})
	}

	reqBody, _ := json.Marshal(OpenRouterRequest{
		Model:    "openai/gpt-3.5-turbo", // Default fallback if needed
		Messages: apiMsgs,
		Stream:   true,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("openrouter returned %d", resp.StatusCode)
	}

	var fullResponse string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			data := strings.TrimPrefix(line, "data: ")
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				if len(chunk.Choices) > 0 {
					text := chunk.Choices[0].Delta.Content
					fullResponse += text
					if onChunk != nil {
						onChunk(text)
					}
				}
			}
		}
	}

	session.Messages = append(session.Messages, Message{Role: RoleAssistant, Content: fullResponse})
	session.UpdatedAt = time.Now()
	return s.repo.Save(ctx, session)
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/chat/service.go
git commit -m "feat(backend): implement OpenRouter streaming proxy"
```

---

### Task 4: Setup Vue Routes

**Files:**
- Modify: `frontend/src/router.ts`
- Create: `frontend/src/views/SettingsView.vue`
- Create: `frontend/src/views/ChatView.vue`

- [ ] **Step 1: Write minimal components**

```html
<!-- frontend/src/views/SettingsView.vue -->
<template>
  <div class="p-8 max-w-2xl mx-auto mt-20 bg-white rounded-3xl shadow-sm border border-gray-200">
    <h1 class="text-2xl font-bold mb-6">Settings</h1>
    <div class="mb-4">
      <label class="block text-sm font-medium text-gray-700 mb-2">OpenRouter API Key</label>
      <input v-model="apiKey" type="password" class="w-full p-3 border rounded-xl" placeholder="sk-or-v1-..." />
    </div>
    <button @click="save" class="px-6 py-2 bg-emerald-600 text-white rounded-xl font-bold">Save</button>
  </div>
</template>
<script setup lang="ts">
import { ref, onMounted } from 'vue'
const apiKey = ref('')
onMounted(() => { apiKey.value = localStorage.getItem('OPENROUTER_API_KEY') || '' })
const save = () => { localStorage.setItem('OPENROUTER_API_KEY', apiKey.value); alert('Saved!') }
</script>
```

```html
<!-- frontend/src/views/ChatView.vue -->
<template>
  <div class="flex h-screen pt-20">
    <div class="w-64 border-r border-gray-200 p-4">
      <h2 class="font-bold mb-4">Previous Chats</h2>
    </div>
    <div class="flex-1 p-8 flex flex-col">
      <div class="flex-1 overflow-y-auto mb-4">
        <p class="text-gray-500">Chat area placeholder</p>
      </div>
      <div class="border rounded-2xl p-2 bg-white flex">
        <input type="text" class="flex-1 outline-none px-4" placeholder="Message or @mention an article..." />
        <button class="px-4 py-2 bg-black text-white rounded-xl font-bold">Send</button>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Update Router**

```typescript
// Add these lines to frontend/src/router.ts to register the routes
import SettingsView from './views/SettingsView.vue'
import ChatView from './views/ChatView.vue'

// Add to the routes array:
// { path: '/settings', name: 'settings', component: SettingsView },
// { path: '/chat', name: 'chat', component: ChatView },
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/ frontend/src/router.ts
git commit -m "feat(frontend): add chat and settings routes"
```
