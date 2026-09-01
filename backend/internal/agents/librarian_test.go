package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
							"placements": [
								{"article_id": 2, "target_section": "Consensus", "context_note": "Secondary node replication."}
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

	if result.UpdatedMOCs != 1 || result.CreatedMOCs != 0 {
		t.Errorf("expected 1 MOC updated or created, got result: %+v", result)
	}

	destMocPath := filepath.Join(articlesDir, "Distributed Systems", "MOC - Distributed Systems.md")
	updatedBytes, err := os.ReadFile(destMocPath)
	if err != nil {
		t.Fatalf("failed to read updated MOC at %s: %v", destMocPath, err)
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
	articlesDir := filepath.Join(tempDir, "articles")

	// Create 5 articles sharing tag "concurrency" so DetectClusters finds an active cluster
	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Concurrency Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "concurrency",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"Concurrency","executive_summary":"Summary","sections":[]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     100,
				"completion_tokens": 50,
				"total_tokens":      150,
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)

	var wg sync.WaitGroup
	successCount := 0
	alreadyRunningCount := 0
	var otherErrors []error
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := runner.RunLibrarianWithURL(context.Background(), "concurrent", mockServer.URL)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if strings.Contains(err.Error(), "already running") {
				alreadyRunningCount++
			} else {
				otherErrors = append(otherErrors, err)
			}
		}()
	}

	wg.Wait()

	if len(otherErrors) > 0 {
		t.Fatalf("unexpected errors during concurrent runs: %v", otherErrors)
	}
	if successCount != 1 {
		t.Errorf("expected exactly 1 execution to succeed, got %d", successCount)
	}
	if alreadyRunningCount != 2 {
		t.Errorf("expected 2 executions to fail with 'already running', got %d", alreadyRunningCount)
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

func TestLibrarian_ZeroTokenSkip_WhenUpToDate(t *testing.T) {
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
Overview of distributed systems.

## Curated Index
### Consensus
- [[Node 1|Node 1]] — Node 1 description
- [[Node 2|Node 2]] — Node 2 description
- [[Node 3|Node 3]] — Node 3 description
- [[Node 4|Node 4]] — Node 4 description
- [[Node 5|Node 5]] — Node 5 description

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
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
		title := fmt.Sprintf("Node %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	var httpCalls int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpCalls, 1)
		t.Errorf("HTTP server should not be called for up-to-date MOC")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"should not be called"}`))
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CreatedMOCs != 0 {
		t.Errorf("expected 0 CreatedMOCs, got %d", result.CreatedMOCs)
	}
	if result.UpdatedMOCs != 0 {
		t.Errorf("expected 0 UpdatedMOCs, got %d", result.UpdatedMOCs)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 Errors, got %v", result.Errors)
	}
	if atomic.LoadInt32(&httpCalls) != 0 {
		t.Errorf("expected 0 HTTP calls, got %d", atomic.LoadInt32(&httpCalls))
	}
}

func TestLibrarian_DeltaClassification_OnlySendsNewNotes(t *testing.T) {
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
Overview of distributed systems.

## Core Concepts
- [[Distributed Node 1|Distributed Node 1]] — Node 1 description
- [[Distributed Node 2|Distributed Node 2]] — Node 2 description

## Infrastructure
- [[Distributed Node 3|Distributed Node 3]] — Node 3 description
- [[Distributed Node 4|Distributed Node 4]] — Node 4 description
- [[Distributed Node 5|Distributed Node 5]] — Node 5 description

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
Custom user research notes that must never be deleted.
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
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	// Add 1 new article (ID 6, Title: "Raft Consensus Protocol", Tags: "distributed-systems")
	filePath6 := filepath.Join(articlesDir, "Raft Consensus Protocol.md")
	_ = os.WriteFile(filePath6, []byte("# Raft Consensus Protocol\nContent"), 0644)
	db.Create(&repository.GormArticle{
		ID:      6,
		Title:   "Raft Consensus Protocol",
		Tags:    "distributed-systems",
		Article: "/articles/Raft Consensus Protocol.md",
	})

	var httpCalls int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpCalls, 1)
		var reqBody struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}
		if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if reqBody.ResponseFormat.JSONSchema.Name != "moc_delta_placement" {
			t.Errorf("expected schema name 'moc_delta_placement', got %q", reqBody.ResponseFormat.JSONSchema.Name)
		}

		if len(reqBody.Messages) == 0 {
			t.Errorf("expected at least 1 message in prompt")
			return
		}
		promptContent := reqBody.Messages[0].Content

		// Request prompt contains "Raft Consensus Protocol" (ID 6).
		if !strings.Contains(promptContent, "Raft Consensus Protocol") {
			t.Errorf("expected prompt to contain 'Raft Consensus Protocol', got: %s", promptContent)
		}

		// Request prompt does NOT contain full candidate list of existing nodes 1-5.
		for i := 1; i <= 5; i++ {
			nodeTitle := fmt.Sprintf("Distributed Node %d", i)
			if strings.Contains(promptContent, nodeTitle) {
				t.Errorf("expected prompt NOT to contain existing node %q, but found in prompt: %s", nodeTitle, promptContent)
			}
		}

		// Request prompt contains existing section names: "Core Concepts", "Infrastructure".
		if !strings.Contains(promptContent, "Core Concepts") {
			t.Errorf("expected prompt to contain section 'Core Concepts', got: %s", promptContent)
		}
		if !strings.Contains(promptContent, "Infrastructure") {
			t.Errorf("expected prompt to contain section 'Infrastructure', got: %s", promptContent)
		}

		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"placements": [{"article_id": 6, "target_section": "Core Concepts", "context_note": "Raft consensus implementation."}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     150,
				"completion_tokens": 40,
				"total_tokens":      190,
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

	if result.UpdatedMOCs != 1 {
		t.Errorf("expected 1 UpdatedMOC, got %d (result: %+v)", result.UpdatedMOCs, result)
	}
	if atomic.LoadInt32(&httpCalls) != 1 {
		t.Errorf("expected 1 HTTP call, got %d", atomic.LoadInt32(&httpCalls))
	}

	// Verify old root file is removed after migration to topic folder
	if _, err := os.Stat(mocPath); !os.IsNotExist(err) {
		t.Errorf("expected old root MOC %s to be removed after migration", mocPath)
	}

	destMocPath := filepath.Join(articlesDir, "Distributed Systems", "MOC - Distributed Systems.md")
	updatedBytes, err := os.ReadFile(destMocPath)
	if err != nil {
		t.Fatalf("failed to read updated MOC at %s: %v", destMocPath, err)
	}
	updatedContent := string(updatedBytes)

	// Verify inserting - [[Raft Consensus Protocol]] - Raft consensus implementation. under ## Core Concepts
	if !strings.Contains(updatedContent, "- [[Raft Consensus Protocol]] - Raft consensus implementation.") {
		t.Errorf("expected updated MOC to contain '- [[Raft Consensus Protocol]] - Raft consensus implementation.', got:\n%s", updatedContent)
	}

	coreConceptsIdx := strings.Index(updatedContent, "## Core Concepts")
	infraIdx := strings.Index(updatedContent, "## Infrastructure")
	raftIdx := strings.Index(updatedContent, "[[Raft Consensus Protocol]]")

	if coreConceptsIdx == -1 || infraIdx == -1 || raftIdx == -1 {
		t.Fatalf("missing sections in updated MOC:\n%s", updatedContent)
	}
	if raftIdx < coreConceptsIdx || raftIdx > infraIdx {
		t.Errorf("expected Raft Consensus Protocol to be between ## Core Concepts and ## Infrastructure")
	}

	// Verify user notes in ## Notes & Synthesis remain intact
	if !strings.Contains(updatedContent, "Custom user research notes that must never be deleted.") {
		t.Errorf("expected user notes in ## Notes & Synthesis to remain intact, got:\n%s", updatedContent)
	}
}

func TestLibrarian_DeltaTelemetry_RecordsAccurateTokens(t *testing.T) {
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
Overview of distributed systems.

## Core Concepts
- [[Distributed Node 1|Distributed Node 1]] — Node 1 description
- [[Distributed Node 2|Distributed Node 2]] — Node 2 description

## Infrastructure
- [[Distributed Node 3|Distributed Node 3]] — Node 3 description
- [[Distributed Node 4|Distributed Node 4]] — Node 4 description
- [[Distributed Node 5|Distributed Node 5]] — Node 5 description

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
`
	mocPath := filepath.Join(articlesDir, "MOC - Distributed Systems.md")
	_ = os.WriteFile(mocPath, []byte(existingMOCContent), 0644)

	db.Create(&repository.GormArticle{
		ID:      100,
		Title:   "MOC - Distributed Systems",
		Tags:    "moc, distributed-systems",
		Article: "/articles/MOC - Distributed Systems.md",
	})

	// 5 existing articles (ID 1-5)
	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Distributed Node %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	// 2 new articles (ID 6, 7)
	for i := 6; i <= 7; i++ {
		title := fmt.Sprintf("Distributed Node %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nNew node content", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"placements": [{"article_id": 6, "target_section": "Core Concepts", "context_note": "Node 6 description."}, {"article_id": 7, "target_section": "Infrastructure", "context_note": "Node 7 description."}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     180,
				"completion_tokens": 65,
				"total_tokens":      245,
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"openai/gpt-4o-mini","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error running librarian: %v", err)
	}

	if result.UpdatedMOCs != 1 {
		t.Fatalf("expected 1 updated MOC, got %d", result.UpdatedMOCs)
	}

	summary, recent, err := repo.GetPipelineDiagnostics(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected diagnostics error: %v", err)
	}

	if summary.TotalRuns != 1 {
		t.Errorf("expected 1 total run, got %d", summary.TotalRuns)
	}
	if summary.TotalTokensUsed != 245 {
		t.Errorf("expected 245 total tokens used, got %d", summary.TotalTokensUsed)
	}

	if len(recent) != 1 {
		t.Fatalf("expected 1 recent pipeline metric, got %d", len(recent))
	}

	metric := recent[0]
	if metric.Status != "success" {
		t.Errorf("expected metric Status 'success', got %q", metric.Status)
	}
	if metric.PromptTokens != 180 {
		t.Errorf("expected PromptTokens 180, got %d", metric.PromptTokens)
	}
	if metric.CompletionTokens != 65 {
		t.Errorf("expected CompletionTokens 65, got %d", metric.CompletionTokens)
	}
	if metric.TotalTokens != 245 {
		t.Errorf("expected TotalTokens 245, got %d", metric.TotalTokens)
	}
	if !strings.HasPrefix(metric.ArticleTitle, "[Librarian] MOC - ") {
		t.Errorf("expected ArticleTitle to start with '[Librarian] MOC - ', got %q", metric.ArticleTitle)
	}
	if metric.DurationMs <= 0 {
		t.Errorf("expected DurationMs > 0, got %d", metric.DurationMs)
	}
}

func TestLibrarian_ArticlesWithPipesInTitle_NeverDuplicate(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Create 5 articles including one with a pipe in the title
	for i := 1; i <= 4; i++ {
		title := fmt.Sprintf("Development Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "development",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	pipeTitle := "How to Optimize NumPy with Cython | Paperspace Blog"
	filePath := filepath.Join(articlesDir, "cython.md")
	_ = os.WriteFile(filePath, []byte("# Cython Guide\nContent"), 0644)
	db.Create(&repository.GormArticle{
		ID:      5,
		Title:   pipeTitle,
		Tags:    "development",
		Article: "/articles/cython.md",
	})

	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"Development","executive_summary":"Development tools summary","sections":[{"title":"Core Tools","items":[{"article_id":5,"context_note":"NumPy and Cython optimization techniques."},{"article_id":1,"context_note":"Dev note 1."},{"article_id":2,"context_note":"Dev note 2."},{"article_id":3,"context_note":"Dev note 3."},{"article_id":4,"context_note":"Dev note 4."}]}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     300,
				"completion_tokens": 100,
				"total_tokens":      400,
			},
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"openai/gpt-4o","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)

	// Run 1: Synthesizes initial MOC
	res1, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if res1.CreatedMOCs != 1 {
		t.Fatalf("expected 1 created MOC, got %d", res1.CreatedMOCs)
	}

	mocPath := filepath.Join(articlesDir, "Development", "MOC - Development.md")
	contentBytes, err := os.ReadFile(mocPath)
	if err != nil {
		t.Fatalf("failed to read MOC: %v", err)
	}
	mocContent := string(contentBytes)

	// Verify formatted cleanly targeting the actual disk filename with clean display alias
	if strings.Contains(mocContent, "[[How to Optimize NumPy with Cython | Paperspace Blog|") {
		t.Errorf("found broken double-pipe wikilink in MOC:\n%s", mocContent)
	}
	if !strings.Contains(mocContent, "[[cython|How to Optimize NumPy with Cython — Paperspace Blog]]") {
		t.Errorf("expected clean wikilink [[cython|How to Optimize NumPy with Cython — Paperspace Blog]] in MOC, got:\n%s", mocContent)
	}

	// Run 2: Immediate subsequent run MUST zero-token skip (no new calls!)
	res2, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if res2.CreatedMOCs != 0 || res2.UpdatedMOCs != 0 {
		t.Errorf("expected 0 created/updated on second run, got created=%d, updated=%d", res2.CreatedMOCs, res2.UpdatedMOCs)
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 LLM call across both runs, got %d", callCount)
	}
}

func TestLibrarian_TopicTitlePathTraversal_SanitizedSafely(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Security Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "security",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"../../etc/passwd","executive_summary":"Traversal test summary","sections":[{"title":"Core","items":[{"article_id":1,"context_note":"Note 1."},{"article_id":2,"context_note":"Note 2."},{"article_id":3,"context_note":"Note 3."},{"article_id":4,"context_note":"Note 4."},{"article_id":5,"context_note":"Note 5."}]}]}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     150,
				"completion_tokens": 50,
				"total_tokens":      200,
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
		t.Fatalf("run failed: %v", err)
	}

	if result.CreatedMOCs != 1 {
		t.Fatalf("expected 1 created MOC, got %d", result.CreatedMOCs)
	}

	// Verify no file was created outside articlesDir
	parentDir := filepath.Dir(articlesDir)
	if _, err := os.Stat(filepath.Join(parentDir, "passwd.md")); err == nil {
		t.Errorf("path traversal vulnerability: file created in parent directory!")
	}

	// Verify the file was created inside articlesDir safely
	var createdFiles []string
	_ = filepath.Walk(articlesDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasPrefix(info.Name(), "MOC - ") {
			createdFiles = append(createdFiles, path)
		}
		return nil
	})

	if len(createdFiles) == 0 {
		t.Fatalf("expected MOC file created in articlesDir, found none")
	}
}

func TestLibrarian_TopicFolderOrganization(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Seed 5 articles with tag "distributed-systems"
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

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{
							"topic_title": "Distributed Systems",
							"executive_summary": "Distributed systems overview.",
							"sections": [
								{
									"title": "Nodes",
									"items": [
										{"article_id": 1, "context_note": "Node 1."},
										{"article_id": 2, "context_note": "Node 2."},
										{"article_id": 3, "context_note": "Node 3."},
										{"article_id": 4, "context_note": "Node 4."},
										{"article_id": 5, "context_note": "Node 5."}
									]
								}
							]
						}`,
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     200,
				"completion_tokens": 80,
				"total_tokens":      280,
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
		t.Fatalf("librarian run failed: %v", err)
	}

	if result.CreatedMOCs != 1 {
		t.Fatalf("expected 1 created MOC, got %d", result.CreatedMOCs)
	}

	// Asserts articles/Distributed Systems/MOC - Distributed Systems.md exists
	mocPath := filepath.Join(articlesDir, "Distributed Systems", "MOC - Distributed Systems.md")
	if _, err := os.Stat(mocPath); os.IsNotExist(err) {
		t.Fatalf("expected MOC file to exist at %s, but not found", mocPath)
	}

	// Asserts all 5 member note files exist in articles/Distributed Systems/
	for i := 1; i <= 5; i++ {
		memberPath := filepath.Join(articlesDir, "Distributed Systems", fmt.Sprintf("Distributed Node %d.md", i))
		if _, err := os.Stat(memberPath); os.IsNotExist(err) {
			t.Errorf("expected member note %d to exist at %s", i, memberPath)
		}
	}

	// Asserts DB records for the articles have article path prefix /articles/Distributed Systems/
	var articles []repository.GormArticle
	db.Find(&articles, "id IN ?", []int64{1, 2, 3, 4, 5})
	if len(articles) != 5 {
		t.Fatalf("expected 5 articles in db, got %d", len(articles))
	}
	for _, a := range articles {
		expectedPrefix := "/articles/Distributed Systems/"
		if !strings.HasPrefix(a.Article, expectedPrefix) {
			t.Errorf("article %d path %q does not have prefix %q", a.ID, a.Article, expectedPrefix)
		}
	}
}

func TestLibrarian_ClusterFailure_SetsDegradedStatus(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Fail Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte("# Content"), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "failure-test",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"simulated OpenRouter 500 error"}`))
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"openai/gpt-4o","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("run failed with unexpected error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", result.Status)
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected recorded errors in result.Errors, got 0")
	}
}

func TestExtractLinkedArticlesFromMOC_AliasNotIndexedAsTarget(t *testing.T) {
	mocContent := `# MOC
- [[Real Target Note|Alias Label]] - Context
- [[Literal Title | With Pipe]] - Context
- [[Standard Note]] - Context
`
	linked := extractLinkedArticlesFromMOC(mocContent)

	// Target components must be indexed
	if !linked["Real Target Note"] || !linked["real target note"] {
		t.Errorf("expected 'Real Target Note' to be indexed in linked")
	}
	if !linked["Standard Note"] || !linked["standard note"] {
		t.Errorf("expected 'Standard Note' to be indexed in linked")
	}
	if !linked["Literal Title | With Pipe"] || !linked["literal title | with pipe"] {
		t.Errorf("expected full literal title to be indexed in linked")
	}
	if !linked["Literal Title"] {
		t.Errorf("expected first part of literal title to be indexed in linked")
	}

	// Alias label must NOT be indexed as a target
	if linked["Alias Label"] || linked["alias label"] {
		t.Errorf("expected display alias 'Alias Label' NOT to be indexed as a target")
	}
}

func TestApplyDeltaPlacements_DuplicatePlacementsDeduplicated(t *testing.T) {
	existingContent := `# MOC - Test

## Core Concepts
- [[Note 1]] - Existing note.

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
`
	placements := []MOCDeltaPlacement{
		{ArticleID: 2, TargetSection: "Core Concepts", ContextNote: "First occurrence."},
		{ArticleID: 2, TargetSection: "Core Concepts", ContextNote: "Duplicate occurrence."},
		{ArticleID: 2, TargetSection: "Other Section", ContextNote: "Duplicate across sections."},
	}
	articleInfoMap := map[int64]MOCArticleInfo{
		2: {ID: 2, Title: "Note 2", FilePath: "/articles/Note 2.md"},
	}

	result := applyDeltaPlacements(existingContent, placements, articleInfoMap)

	count := strings.Count(result, "[[Note 2]]")
	if count != 1 {
		t.Errorf("expected [[Note 2]] to appear exactly once, but appeared %d times in:\n%s", count, result)
	}
}

func TestHasCustomUserNotes(t *testing.T) {
	// Case 1: Empty or missing section
	if HasCustomUserNotes("# MOC - Empty\n## Concepts\n- [[A]]") {
		t.Errorf("expected false for missing Notes & Synthesis")
	}

	// Case 2: Only default boilerplate placeholder
	defaultMoc := `# MOC - Default
## Core Concepts
- [[Note 1]]

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*
`
	if HasCustomUserNotes(defaultMoc) {
		t.Errorf("expected false for default placeholder text")
	}

	// Case 3: Custom user written notes
	customMoc := `# MOC - Custom
## Core Concepts
- [[Note 1]]

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*

Here are my custom architecture thoughts that must never be deleted!
`
	if !HasCustomUserNotes(customMoc) {
		t.Errorf("expected true for custom user thoughts")
	}
}

func TestReconcileMOCLinks(t *testing.T) {
	mocContent := `# MOC - Distributed Systems

## Core Concepts
- [[Active Note]] - Core concept note.
- [[Deleted Note]] - This note was deleted.
- [[Re-filed Note|Custom Alias]] - This note was re-filed to another topic.

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
My custom thoughts linking to [[Deleted Note]] which should NOT be touched in user section.
`

	validMembers := map[string]bool{
		"Active Note": true,
		"active note": true,
	}

	reconciled, changed := ReconcileMOCLinks(mocContent, validMembers)
	if !changed {
		t.Fatalf("expected changed=true when pruning stale links")
	}

	// Active Note should remain in Core Concepts
	if !strings.Contains(reconciled, "- [[Active Note]] - Core concept note.") {
		t.Errorf("expected Active Note to be preserved in Core Concepts, got:\n%s", reconciled)
	}

	// Deleted Note and Re-filed Note must NOT be in Core Concepts
	if strings.Contains(reconciled, "- [[Deleted Note]]") {
		t.Errorf("expected Deleted Note to be pruned from Core Concepts")
	}
	if strings.Contains(reconciled, "- [[Re-filed Note|Custom Alias]]") {
		t.Errorf("expected Re-filed Note to be pruned from Core Concepts")
	}

	// Custom user notes section MUST remain completely untouched (including [[Deleted Note]] in user thoughts)
	if !strings.Contains(reconciled, "My custom thoughts linking to [[Deleted Note]] which should NOT be touched in user section.") {
		t.Errorf("user notes section was corrupted:\n%s", reconciled)
	}
}

func TestLibrarian_ReconcilesStaleLinksInExistingMOC(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// 1. Create existing MOC in topic folder with links to Note 1, Note 2, and Note 3 (which was deleted/re-filed)
	topicFolder := filepath.Join(articlesDir, "Distributed Systems")
	_ = os.MkdirAll(topicFolder, 0755)
	mocPath := filepath.Join(topicFolder, "MOC - Distributed Systems.md")

	existingMocBody := `# MOC - Distributed Systems

## Core Concepts
- [[Active Note 1]] - Active member 1.
- [[Active Note 2]] - Active member 2.
- [[Deleted Note 3]] - This note was deleted from the vault.

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*
`
	_ = os.WriteFile(mocPath, []byte(existingMocBody), 0644)

	db.Create(&repository.GormArticle{
		ID:      100,
		Title:   "MOC - Distributed Systems",
		Tags:    "moc, distributed-systems",
		Article: "/articles/Distributed Systems/MOC - Distributed Systems.md",
	})

	// Active cluster members: 5 notes (Note 1, 2, 4, 5, 6), Note 3 is absent/deleted
	for i := 1; i <= 6; i++ {
		if i == 3 {
			continue // Deleted Note 3
		}
		title := fmt.Sprintf("Active Note %d", i)
		filePath := filepath.Join(topicFolder, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)

		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "distributed-systems",
			Article: fmt.Sprintf("/articles/Distributed Systems/%s.md", title),
		})
	}

	// Seed legacy article_links
	db.Create(&repository.GormArticleLink{ID: 1, SourceID: 100, TargetID: 1})
	db.Create(&repository.GormArticleLink{ID: 2, SourceID: 100, TargetID: 2})
	db.Create(&repository.GormArticleLink{ID: 3, SourceID: 100, TargetID: 3}) // Stale link

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{
							"placements": [
								{"article_id": 4, "target_section": "Core Concepts", "context_note": "Placed note 4."},
								{"article_id": 5, "target_section": "Core Concepts", "context_note": "Placed note 5."},
								{"article_id": 6, "target_section": "Core Concepts", "context_note": "Placed note 6."}
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

	if result.UpdatedMOCs != 1 {
		t.Errorf("expected 1 MOC updated, got %+v", result)
	}

	updatedBytes, err := os.ReadFile(mocPath)
	if err != nil {
		t.Fatalf("failed to read updated MOC: %v", err)
	}
	updatedContent := string(updatedBytes)

	// Deleted Note 3 must be pruned
	if strings.Contains(updatedContent, "[[Deleted Note 3]]") {
		t.Errorf("expected Deleted Note 3 to be pruned from MOC markdown, got:\n%s", updatedContent)
	}

	// Active Note 1, 2, 4, 5, 6 must all be present
	for _, expectedNote := range []string{"Active Note 1", "Active Note 2", "Active Note 4", "Active Note 5", "Active Note 6"} {
		if !strings.Contains(updatedContent, fmt.Sprintf("[[%s]]", expectedNote)) {
			t.Errorf("expected %s to be present in MOC markdown", expectedNote)
		}
	}

	// In article_links, TargetID 3 must be deleted
	var staleCount int64
	db.Model(&repository.GormArticleLink{}).Where("source_id = ? AND target_id = ?", 100, 3).Count(&staleCount)
	if staleCount != 0 {
		t.Errorf("expected stale article_link for target_id 3 to be deleted, got count %d", staleCount)
	}
}

func TestLibrarian_PruneEmptyMOC_WhenNoUserNotes(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// 1. Seed an empty MOC with 0 member notes and no custom notes
	topicFolder := filepath.Join(articlesDir, "Orphaned Topic")
	_ = os.MkdirAll(topicFolder, 0755)
	mocPath := filepath.Join(topicFolder, "MOC - Orphaned Topic.md")

	mocContent := `# MOC - Orphaned Topic

## Core Concepts

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*
`
	_ = os.WriteFile(mocPath, []byte(mocContent), 0644)

	db.Create(&repository.GormArticle{
		ID:      200,
		Title:   "MOC - Orphaned Topic",
		Tags:    "moc, orphaned-topic",
		Article: "/articles/Orphaned Topic/MOC - Orphaned Topic.md",
	})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("LLM should not be called when 0 clusters are detected")
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PrunedMOCs != 1 {
		t.Errorf("expected 1 MOC pruned, got %d", result.PrunedMOCs)
	}

	// Verify MOC file is deleted from disk
	if _, err := os.Stat(mocPath); !os.IsNotExist(err) {
		t.Errorf("expected MOC file %s to be deleted", mocPath)
	}

	// Verify topic folder is deleted
	if _, err := os.Stat(topicFolder); !os.IsNotExist(err) {
		t.Errorf("expected empty topic folder %s to be pruned", topicFolder)
	}

	// Verify DB record is deleted
	var count int64
	db.Model(&repository.GormArticle{}).Where("id = ?", 200).Count(&count)
	if count != 0 {
		t.Errorf("expected MOC DB record to be deleted, got count %d", count)
	}
}

func TestLibrarian_RetainEmptyMOC_WhenUserNotesExist(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// 1. Seed an empty MOC with 0 member notes BUT with custom user thoughts
	topicFolder := filepath.Join(articlesDir, "User Essay Topic")
	_ = os.MkdirAll(topicFolder, 0755)
	mocPath := filepath.Join(topicFolder, "MOC - User Essay Topic.md")

	mocContent := `# MOC - User Essay Topic

## Core Concepts

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*

I wrote extensive custom notes and reflections on this topic here! DO NOT DELETE ME!
`
	_ = os.WriteFile(mocPath, []byte(mocContent), 0644)

	db.Create(&repository.GormArticle{
		ID:      300,
		Title:   "MOC - User Essay Topic",
		Tags:    "moc, user-essay-topic",
		Article: "/articles/User Essay Topic/MOC - User Essay Topic.md",
	})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("LLM should not be called when 0 clusters are detected")
	}))
	defer mockServer.Close()

	settingsJSON := `{"api_key":"test-key","model":"test-model","librarian_enabled":true,"librarian_min_cluster_size":5}`
	_ = os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	runner := NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	result, err := runner.RunLibrarianWithURL(context.Background(), "manual", mockServer.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PrunedMOCs != 0 {
		t.Errorf("expected 0 MOCs pruned, got %d", result.PrunedMOCs)
	}

	// Verify MOC file and folder are preserved
	if _, err := os.Stat(mocPath); err != nil {
		t.Errorf("expected MOC file %s with user notes to be preserved: %v", mocPath, err)
	}

	// Verify DB record is preserved
	var count int64
	db.Model(&repository.GormArticle{}).Where("id = ?", 300).Count(&count)
	if count != 1 {
		t.Errorf("expected MOC DB record to be preserved, got count %d", count)
	}
}

func TestTitleContainsTopic(t *testing.T) {
	tests := []struct {
		title    string
		topic    string
		expected bool
	}{
		{"Google employees are testing Gemini", "google", true},
		{"Google employees are testing Gemini", "GOOGLE", true},
		{"Google employees are testing Gemini", " google ", true},
		{"Google employees are testing Gemini", "anthropic", false},
		{"An Anthropic researcher gave a talk", "anthropic", true},
		{"Sony sues Anthropic over copyrights", "Anthropic", true},
		{"Claiming insurance in Paris", "ai", false},  // "ai" in "claiming" should NOT match
		{"Frontier AI Safety Initiative", "ai", true}, // whole word "AI" should match
		{"Raft Distributed Systems Architecture", "distributed-systems", true},
		{"Raft Distributed Systems Architecture", " distributed_systems ", true},
	}

	for _, tt := range tests {
		got := titleContainsTopic(tt.title, tt.topic)
		if got != tt.expected {
			t.Errorf("titleContainsTopic(%q, %q) = %v, expected %v", tt.title, tt.topic, got, tt.expected)
		}
	}
}

func TestDeterminePrimaryTopicFolders(t *testing.T) {
	clusters := []ClusterCandidate{
		{
			Tag: "google",
			Articles: []repository.ArticleRecord{
				{ID: 101, Title: "Google employees are already testing the next Gemini Flash AI model", Tags: "ai, google, anthropic, business"},
				{ID: 102, Title: "Google Workspace Overview", Tags: "google"},
				{ID: 103, Title: "Google Search Engine", Tags: "google"},
				{ID: 104, Title: "Google Cloud Platform", Tags: "google"},
				{ID: 105, Title: "Google Pixel Review", Tags: "google"},
				{ID: 106, Title: "Google Maps Tips", Tags: "google"},
				{ID: 107, Title: "Google Android 15", Tags: "google"},
			}, // size 7 (large cluster)
		},
		{
			Tag: "anthropic",
			Articles: []repository.ArticleRecord{
				{ID: 101, Title: "Google employees are already testing the next Gemini Flash AI model", Tags: "ai, google, anthropic, business"},
				{ID: 201, Title: "An Anthropic researcher gave us a peek at self-improving AI", Tags: "ai, anthropic, development"},
				{ID: 202, Title: "Claude 3.5 Sonnet Release", Tags: "anthropic"},
			}, // size 3 (small cluster)
		},
		{
			Tag: "ai",
			Articles: []repository.ArticleRecord{
				{ID: 101, Title: "Google employees are already testing the next Gemini Flash AI model", Tags: "ai, google, anthropic, business"},
				{ID: 201, Title: "An Anthropic researcher gave us a peek at self-improving AI", Tags: "ai, anthropic, development"},
				{ID: 301, Title: "General AI Note 1", Tags: "ai"},
				{ID: 302, Title: "General AI Note 2", Tags: "ai"},
				{ID: 303, Title: "General AI Note 3", Tags: "ai"},
				{ID: 304, Title: "General AI Note 4", Tags: "ai"},
				{ID: 305, Title: "General AI Note 5", Tags: "ai"},
				{ID: 306, Title: "General AI Note 6", Tags: "ai"},
			}, // size 8 (largest cluster)
		},
	}

	primary := DeterminePrimaryTopicFolders(clusters)

	// Article 101 has "Google" in the title -> MUST be filed into "google", despite anthropic cluster being smaller (3 vs 7)
	if primary[101] != "google" {
		t.Errorf("expected article 101 with 'Google' in title to have primary topic 'google', got %q", primary[101])
	}

	// Article 201 has "Anthropic" in the title -> MUST be filed into "anthropic"
	if primary[201] != "anthropic" {
		t.Errorf("expected article 201 with 'Anthropic' in title to have primary topic 'anthropic', got %q", primary[201])
	}

	// Article 301 has no title match -> files into 'ai'
	if primary[301] != "ai" {
		t.Errorf("expected article 301 to have primary topic 'ai', got %q", primary[301])
	}
}

func TestLibrarian_PrimaryTopicSpecificityFiling(t *testing.T) {
	db, repo, tempDir := setupTestLibrarianEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Seed 5 AI articles (broad cluster, size 6)
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

	// Seed 4 specific Anthropic articles + 1 overlapping note (specific cluster, size 5)
	for i := 10; i <= 14; i++ {
		title := fmt.Sprintf("Anthropic Note %d", i)
		tags := "anthropic"
		if i == 10 {
			tags = "ai, anthropic" // Belongs to both AI (size 6) and Anthropic (size 5)
		}
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)
		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    tags,
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"topic_title":"Synthesized Topic","executive_summary":"Summary","sections":[]}`,
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

	if result.CreatedMOCs != 2 {
		t.Errorf("expected 2 MOCs created (AI and Anthropic), got %d", result.CreatedMOCs)
	}

	// Overlapping note 10 MUST be filed in Anthropic/ (the more specific cluster, size 5), NOT AI/ (size 6)
	var note10 repository.GormArticle
	db.First(&note10, 10)
	if !strings.HasPrefix(note10.Article, "/articles/Anthropic/") && !strings.HasPrefix(note10.Article, "/articles/anthropic/") && !strings.HasPrefix(note10.Article, "/articles/Synthesized Topic/") {
		t.Errorf("expected Note 10 to be filed into specific Anthropic topic folder, got %q", note10.Article)
	}

	// Verify physical file exists in the specific folder
	anthropicFolder := filepath.Join(articlesDir, "Anthropic")
	if _, err := os.Stat(anthropicFolder); os.IsNotExist(err) {
		// Or sanitized topic
		anthropicFolder = filepath.Join(articlesDir, "Synthesized Topic")
	}
	entries, _ := os.ReadDir(anthropicFolder)
	if len(entries) < 5 {
		t.Errorf("expected Anthropic topic folder to retain all its member notes, found %d entries", len(entries))
	}
}
