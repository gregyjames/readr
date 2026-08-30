package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileRepository_SaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	now := time.Now().Truncate(time.Second)
	session := &ChatSession{
		ID:        "session-123",
		Title:     "Discussion on Distributed Systems",
		CreatedAt: now,
		UpdatedAt: now.Add(5 * time.Minute),
		Messages: []Message{
			{Role: RoleUser, Content: "What is Raft consensus?"},
			{Role: RoleAssistant, Content: "Raft is a consensus algorithm..."},
			{Role: RoleSystem, Content: "System reminder: stay concise"},
		},
	}

	ctx := context.Background()
	err := repo.Save(ctx, session)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify JSON file exists
	jsonPath := filepath.Join(tmpDir, "session-123.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Errorf("JSON file was not created at %s", jsonPath)
	}

	// Verify no temporary files left behind
	tmpJsonPath := filepath.Join(tmpDir, "session-123.json.tmp")
	if _, err := os.Stat(tmpJsonPath); !os.IsNotExist(err) {
		t.Errorf("Temporary JSON file was not cleaned up: %s", tmpJsonPath)
	}

	// Verify Markdown file exists and check content/frontmatter
	mdPath := filepath.Join(tmpDir, "session-123.md")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("Failed to read Markdown file: %v", err)
	}
	mdContent := string(mdBytes)

	expectedDate := now.Format("2006-01-02")
	expectedFrontmatter := "---\ntitle: \"Discussion on Distributed Systems\"\ndate: \"" + expectedDate + "\"\n---\n"
	if !strings.HasPrefix(mdContent, expectedFrontmatter) {
		t.Errorf("Expected markdown to start with frontmatter:\n%s\nGot:\n%s", expectedFrontmatter, mdContent)
	}

	expectedSections := []string{
		"### User\nWhat is Raft consensus?\n",
		"### Assistant\nRaft is a consensus algorithm...\n",
		"### System\nSystem reminder: stay concise\n",
	}
	for _, sec := range expectedSections {
		if !strings.Contains(mdContent, sec) {
			t.Errorf("Markdown content missing section %q. Full content:\n%s", sec, mdContent)
		}
	}

	// Verify Get
	loaded, err := repo.Get(ctx, "session-123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.ID != session.ID {
		t.Errorf("Expected ID %q, got %q", session.ID, loaded.ID)
	}
	if loaded.Title != session.Title {
		t.Errorf("Expected Title %q, got %q", session.Title, loaded.Title)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != RoleUser || loaded.Messages[0].Content != "What is Raft consensus?" {
		t.Errorf("Message 0 mismatch: %+v", loaded.Messages[0])
	}
	if loaded.Messages[1].Role != RoleAssistant || loaded.Messages[1].Content != "Raft is a consensus algorithm..." {
		t.Errorf("Message 1 mismatch: %+v", loaded.Messages[1])
	}
	if loaded.Messages[2].Role != RoleSystem || loaded.Messages[2].Content != "System reminder: stay concise" {
		t.Errorf("Message 2 mismatch: %+v", loaded.Messages[2])
	}
}

func TestFileRepository_Save_NilSession(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	err := repo.Save(context.Background(), nil)
	if err == nil {
		t.Error("Expected error when saving nil session, got nil")
	}
}

func TestFileRepository_Save_NilMessagesSlice(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	session := &ChatSession{
		ID:        "empty-msgs",
		Title:     "Empty",
		CreatedAt: time.Now(),
		Messages:  nil,
	}

	ctx := context.Background()
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := repo.Get(ctx, "empty-msgs")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.Messages == nil {
		t.Error("Expected non-nil Messages slice after load")
	}
}

func TestFileRepository_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)

	loaded, err := repo.Get(context.Background(), "non-existent-id")
	if err == nil {
		t.Errorf("Expected error for non-existent session, got loaded session: %+v", loaded)
	}
	if !os.IsNotExist(err) {
		t.Logf("Received expected not-found error: %v", err)
	}
}

func TestFileRepository_List_OrderAndFilter(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()

	now := time.Now()

	sessions := []*ChatSession{
		{
			ID:        "session-old",
			Title:     "Old Session",
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-3 * time.Hour),
		},
		{
			ID:        "session-newest",
			Title:     "Newest Session",
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now,
		},
		{
			ID:        "session-mid",
			Title:     "Mid Session",
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
	}

	for _, s := range sessions {
		if err := repo.Save(ctx, s); err != nil {
			t.Fatalf("Failed to save session %s: %v", s.ID, err)
		}
	}

	// Add extraneous files and directories that should be skipped by List()
	if err := os.Mkdir(filepath.Join(tmpDir, "nested-dir.json"), 0755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("not json"), 0644); err != nil {
		t.Fatalf("Failed to create txt file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "session-fake.json.tmp"), []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create tmp file: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("Expected 3 sessions in list, got %d", len(list))
	}

	// Verify descending order by UpdatedAt: session-newest, session-mid, session-old
	expectedIDs := []string{"session-newest", "session-mid", "session-old"}
	for i, expectedID := range expectedIDs {
		if list[i].ID != expectedID {
			t.Errorf("List[%d] ID expected %s, got %s", i, expectedID, list[i].ID)
		}
	}
}

func TestFileRepository_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()

	session := &ChatSession{
		ID:        "session-to-delete",
		Title:     "Will Be Deleted",
		CreatedAt: time.Now(),
	}

	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	jsonPath := filepath.Join(tmpDir, "session-to-delete.json")
	mdPath := filepath.Join(tmpDir, "session-to-delete.md")

	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("JSON file does not exist before delete: %v", err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("MD file does not exist before delete: %v", err)
	}

	// Perform delete
	if err := repo.Delete(ctx, "session-to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify both .json and .md files are removed
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("JSON file still exists after delete")
	}
	if _, err := os.Stat(mdPath); !os.IsNotExist(err) {
		t.Errorf("MD file still exists after delete")
	}

	// Deleting again (non-existent) should not return error
	if err := repo.Delete(ctx, "session-to-delete"); err != nil {
		t.Errorf("Deleting already deleted session returned unexpected error: %v", err)
	}
	if err := repo.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("Deleting non-existent session returned unexpected error: %v", err)
	}
}

func TestFileRepository_SanitizeID_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()

	invalidIDs := []string{
		"",
		"   ",
		".",
		"..",
		"/",
		"\\",
	}

	for _, id := range invalidIDs {
		// Test Get
		if _, err := repo.Get(ctx, id); err == nil {
			t.Errorf("Expected error for Get with invalid ID %q, got nil", id)
		}
		// Test Delete
		if err := repo.Delete(ctx, id); err == nil {
			t.Errorf("Expected error for Delete with invalid ID %q, got nil", id)
		}
		// Test Save
		session := &ChatSession{ID: id, Title: "Bad ID"}
		if err := repo.Save(ctx, session); err == nil {
			t.Errorf("Expected error for Save with invalid ID %q, got nil", id)
		}
	}

	// Path traversal ID like "../test" should be sanitized to "test" and stored in baseDir
	traversalSession := &ChatSession{
		ID:        "../../escaped-session",
		Title:     "Escaped",
		CreatedAt: time.Now(),
	}
	if err := repo.Save(ctx, traversalSession); err != nil {
		t.Fatalf("Save with relative path should sanitize and succeed: %v", err)
	}

	// Check file was saved inside tmpDir under sanitized name
	expectedPath := filepath.Join(tmpDir, "escaped-session.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected sanitized file at %s, but not found", expectedPath)
	}

	// Verify Get with relative path sanitizes to same file
	loaded, err := repo.Get(ctx, "../../escaped-session")
	if err != nil {
		t.Fatalf("Get with relative path failed: %v", err)
	}
	if loaded.Title != "Escaped" {
		t.Errorf("Expected title 'Escaped', got %q", loaded.Title)
	}
}

func TestFileRepository_Concurrency(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewFileRepository(tmpDir)
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				sessionID := filepath.Join(string(rune('a'+workerID)), string(rune('0'+(i%5))))
				session := &ChatSession{
					ID:        sessionID,
					Title:     "Concurrent Test",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Messages: []Message{
						{Role: RoleUser, Content: "Hello"},
					},
				}

				// Perform Save
				_ = repo.Save(ctx, session)

				// Perform Get
				_, _ = repo.Get(ctx, sessionID)

				// Perform List
				_, _ = repo.List(ctx)

				// Perform Delete occasionally
				if i%2 == 0 {
					_ = repo.Delete(ctx, sessionID)
				}
			}
		}(w)
	}

	wg.Wait()
}
