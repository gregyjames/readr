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
