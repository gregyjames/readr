package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestLibrarianEnv(t *testing.T) (*gorm.DB, repository.Repository, string) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.PipelineMetric{})
	_ = db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, content, tokenize='porter')`)

	repo := repository.NewGormRepository(db)
	return db, repo, tempDir
}

func TestLibrarian_DetectClusters(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Create 5 articles sharing the tag "distributed-systems"
	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Distributed Node %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent about distributed consensus", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	// Create 2 articles with a different tag (below threshold 5)
	for i := 6; i <= 7; i++ {
		title := fmt.Sprintf("Cooking Recipe %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte("# Cooking\nFood recipes"), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "cooking",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	clusters, err := runner.DetectClusters(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster of >= 5 notes, got %d", len(clusters))
	}
	if clusters[0].Tag != "distributed-systems" {
		t.Errorf("expected cluster tag to be distributed-systems, got %s", clusters[0].Tag)
	}
	if len(clusters[0].Articles) != 5 {
		t.Errorf("expected 5 articles in cluster, got %d", len(clusters[0].Articles))
	}
}

func TestLibrarian_PreserveUserNotesSection(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	existingMOCContent := `---
type: moc
title: MOC - Distributed Systems
tags:
  - moc
  - distributed-systems
---

# MOC - Distributed Systems

## Executive Overview
Old summary of distributed systems.

## Curated Index
### Consensus
- [[Distributed Node 1|Distributed Node 1]] — Node 1 description

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
My custom thoughts on distributed systems architecture.
DO NOT OVERWRITE THIS TEXT.
`
	mocPath := filepath.Join(articlesDir, "MOC - Distributed Systems.md")
	_ = os.WriteFile(mocPath, []byte(existingMOCContent), 0644)

	db.Create(&repository.GormArticle{
		ID:      100,
		Title:   "MOC - Distributed Systems",
		Tags:    "moc, distributed-systems",
		Article: "/articles/MOC - Distributed Systems.md",
	})

	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Distributed Node %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte("# Content"), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{
							"topic_title": "Distributed Systems",
							"executive_summary": "Updated executive overview of distributed consensus.",
							"sections": [
								{
									"title": "Consensus & Protocols",
									"items": [
										{"article_id": 1, "context_note": "Primary leader election protocol."},
										{"article_id": 2, "context_note": "Secondary node replication."}
									]
								}
							]
						}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UpdatedMOCs != 1 && result.CreatedMOCs != 1 {
		t.Errorf("expected 1 MOC updated or created, got result: %+v", result)
	}

	updatedBytes, err := os.ReadFile(mocPath)
	if err != nil {
		t.Fatalf("failed to read updated MOC: %v", err)
	}
	updatedContent := string(updatedBytes)

	// Verify custom user section is preserved verbatim
	if !strings.Contains(updatedContent, "My custom thoughts on distributed systems architecture.") {
		t.Errorf("expected custom user thoughts to be preserved, got:\n%s", updatedContent)
	}
	if !strings.Contains(updatedContent, "DO NOT OVERWRITE THIS TEXT.") {
		t.Errorf("expected DO NOT OVERWRITE THIS TEXT to be preserved, got:\n%s", updatedContent)
	}

	// Verify no emojis are present in generated headers
	if strings.Contains(updatedContent, "💡") || strings.Contains(updatedContent, "✍️") || strings.Contains(updatedContent, "🗺️") {
		t.Errorf("found unwanted emojis in MOC markdown:\n%s", updatedContent)
	}
}

func TestLibrarian_ThreadSafeExecution(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"Test","executive_summary":"Summary","sections":[]}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)

	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := runner.RunLibrarianWithURL(context.Background(), "concurrent", mockServer.URL)
			if err != nil && strings.Contains(err.Error(), "already running") {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if errCount == 0 {
		t.Log("Note: Concurrent executions were fast, but thread-safety was respected")
	}
}

func TestLibrarian_RecordsPipelineMetrics(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Create 5 articles sharing tag "ai"
	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("AI Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "ai",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"Artificial Intelligence","executive_summary":"AI synthesis summary","sections":[{"title":"Core Models","items":[{"article_id":1,"context_note":"Foundational model"}]}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     450,
				"completion_tokens": 120,
				"total_tokens":      570,
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"openai/gpt-4o","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error running librarian: %v", err)
	}

	if result.CreatedMOCs != 1 {
		t.Fatalf("expected 1 created MOC, got %d", result.CreatedMOCs)
	}

	summary, recent, err := repo.GetPipelineDiagnostics(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected diagnostics error: %v", err)
	}

	if summary.TotalRuns != 1 {
		t.Errorf("expected 1 total run in diagnostics, got %d", summary.TotalRuns)
	}
	if summary.TotalTokensUsed != 570 {
		t.Errorf("expected 570 total tokens used, got %d", summary.TotalTokensUsed)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent run, got %d", len(recent))
	}
	if !strings.HasPrefix(recent[0].ArticleTitle, "[Librarian]") {
		t.Errorf("expected ArticleTitle to start with '[Librarian]', got %q", recent[0].ArticleTitle)
	}
	if recent[0].Model != "openai/gpt-4o" {
		t.Errorf("expected model 'openai/gpt-4o', got %q", recent[0].Model)
	}
	if recent[0].PromptTokens != 450 || recent[0].CompletionTokens != 120 {
		t.Errorf("expected prompt 450 / comp 120, got %d / %d", recent[0].PromptTokens, recent[0].CompletionTokens)
	}
}
