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
	articlesDir := filepath.Join(tempDir, "articles")

	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("Concurrent Note %d", i)
		filePath := filepath.Join(articlesDir, fmt.Sprintf("%s.md", title))
		_ = os.WriteFile(filePath, []byte(fmt.Sprintf("# %s\nContent", title)), 0644)
		db.Create(&repository.GormArticle{
			ID:      int64(i),
			Title:   title,
			Tags:    "concurrency",
			Article: fmt.Sprintf("/articles/%s.md", title),
		})
	}

	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseResponse
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

	var mu sync.Mutex
	successCount := 0
	alreadyRunningCount := 0
	unexpectedErrors := make([]string, 0)
	recordResult := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil:
			successCount++
		case err.Error() == "librarian is already running":
			alreadyRunningCount++
		default:
			unexpectedErrors = append(unexpectedErrors, err.Error())
		}
	}

	var firstRun sync.WaitGroup
	firstRun.Add(1)
	go func() {
		defer firstRun.Done()
		_, err := runner.RunLibrarianWithURL(context.Background(), "concurrent", mockServer.URL)
		recordResult(err)
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		close(releaseResponse)
		firstRun.Wait()
		t.Fatal("timed out waiting for the first Librarian execution to reach the mock server")
	}

	var concurrentRuns sync.WaitGroup
	for i := 0; i < 2; i++ {
		concurrentRuns.Add(1)
		go func() {
			defer concurrentRuns.Done()
			_, err := runner.RunLibrarianWithURL(context.Background(), "concurrent", mockServer.URL)
			recordResult(err)
		}()
	}

	concurrentRuns.Wait()
	close(releaseResponse)
	firstRun.Wait()

	mu.Lock()
	defer mu.Unlock()
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful execution, got %d", successCount)
	}
	if alreadyRunningCount != 2 {
		t.Errorf("expected 2 already-running errors, got %d", alreadyRunningCount)
	}
	if len(unexpectedErrors) != 0 {
		t.Errorf("expected no unexpected errors, got %v", unexpectedErrors)
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
		http.Error(w, "unexpected request", http.StatusInternalServerError)
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
	if calls := atomic.LoadInt32(&httpCalls); calls != 0 {
		t.Errorf("expected 0 HTTP calls, got %d", calls)
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
	if calls := atomic.LoadInt32(&httpCalls); calls != 1 {
		t.Errorf("expected 1 HTTP call, got %d", calls)
	}

	updatedBytes, err := os.ReadFile(mocPath)
	if err != nil {
		t.Fatalf("failed to read updated MOC: %v", err)
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

	mocPath := filepath.Join(articlesDir, "MOC - Development.md")
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
	entries, _ := os.ReadDir(articlesDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "MOC - ") {
			createdFiles = append(createdFiles, e.Name())
		}
	}

	if len(createdFiles) == 0 {
		t.Fatalf("expected MOC file created in articlesDir, found none")
	}
}
