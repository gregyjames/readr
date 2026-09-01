package agents

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func setupTestPipelineEnv(t *testing.T) (string, *gorm.DB, repository.Repository) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		t.Fatal(err)
	}

	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.PipelineMetric{}); err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, content, tokenize='porter')`)

	repo := repository.NewGormRepository(db)

	settingsJSON := `{"api_key":"test-key","model":"test-model"}`
	if err := os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	return tempDir, db, repo
}

func TestProcessPipeline_AllEnabled(t *testing.T) {
	tempDir, db, repo := setupTestPipelineEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Create candidate article in DB and FTS
	db.Create(&repository.GormArticle{ID: 402, Title: "Kubernetes Guide", Tags: "k8s"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (402, 'Kubernetes Guide', 'k8s')")

	// Create source article in DB, FTS, and file
	sourcePath := filepath.Join(articlesDir, "401.md")
	initialContent := "---\ntitle: Old Title\n---\n\nCloud computing relies heavily on Kubernetes Guide for container orchestration."
	if err := os.WriteFile(sourcePath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	db.Create(&repository.GormArticle{ID: 401, Title: "Cloud Native Overview", Tags: "cloud"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (401, 'Cloud Native Overview', 'cloud')")

	promptReceived := ""
	var capturedResponseFormat map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if rf, ok := req["response_format"].(map[string]interface{}); ok {
			capturedResponseFormat = rf
		}
		if msgs, ok := req["messages"].([]interface{}); ok && len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]interface{}); ok {
				promptReceived = fmt.Sprint(m["content"])
			}
		}

		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{
							"summary": "This article discusses cloud computing and Kubernetes orchestration.",
							"frontmatter": {
								"type": "Reference Article",
								"title": "Cloud Native Overview",
								"description": "A guide to modern cloud computing architecture.",
								"resource": "https://example.com/cloud",
								"tags": ["Cloud", "Kubernetes", "DevOps"]
							},
							"links_to_inject": [
								{
									"existing_article_id": 402,
									"exact_phrase_in_text": "Kubernetes Guide"
								}
							]
						}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	pool := &AgentPool{
		Queue:         make(chan Job, 10),
		logger:        zap.NewNop(),
		db:            db,
		repo:          repo,
		dataDirectory: tempDir,
	}

	job := Job{
		ArticleID: 401,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: true,
			Enricher:   true,
			Linker:     true,
		},
	}

	pool.processPipelineWithURL(job, ts.URL)

	// 1. Verify captured response_format
	if capturedResponseFormat == nil || capturedResponseFormat["type"] != "json_schema" {
		t.Fatalf("expected response_format type 'json_schema', got: %v", capturedResponseFormat)
	}
	schemaDef, ok := capturedResponseFormat["json_schema"].(map[string]interface{})
	if !ok || schemaDef["name"] != "unified_pipeline_response" || schemaDef["strict"] != true {
		t.Fatalf("expected strict schema 'unified_pipeline_response', got: %v", schemaDef)
	}
	schema, ok := schemaDef["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema map in json_schema, got: %v", schemaDef)
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties map in schema, got: %v", schema)
	}
	if _, ok := props["summary"]; !ok {
		t.Errorf("expected 'summary' property in schema")
	}
	if _, ok := props["frontmatter"]; !ok {
		t.Errorf("expected 'frontmatter' property in schema")
	}
	if _, ok := props["links_to_inject"]; !ok {
		t.Errorf("expected 'links_to_inject' property in schema")
	}
	reqFields, ok := schema["required"].([]interface{})
	if !ok {
		t.Fatalf("expected required list in schema, got: %v", schema)
	}
	reqMap := make(map[string]bool)
	for _, f := range reqFields {
		reqMap[fmt.Sprint(f)] = true
	}
	if !reqMap["summary"] || !reqMap["frontmatter"] || !reqMap["links_to_inject"] {
		t.Errorf("expected summary, frontmatter, and links_to_inject in required, got: %v", reqFields)
	}

	// 2. Verify prompt contents
	if !strings.Contains(promptReceived, "1. SUMMARY") {
		t.Errorf("expected SUMMARY instruction in prompt")
	}
	if !strings.Contains(promptReceived, "2. FRONTMATTER") {
		t.Errorf("expected FRONTMATTER instruction in prompt")
	}
	if !strings.Contains(promptReceived, "Existing Vault Tags:") {
		t.Errorf("expected Existing Vault Tags in prompt")
	}
	if !strings.Contains(promptReceived, "3. SMART LINKING") {
		t.Errorf("expected SMART LINKING instruction in prompt")
	}
	if !strings.Contains(promptReceived, "- ID: 402, Title: Kubernetes Guide") {
		t.Errorf("expected candidate 402 in prompt candidate list, got:\n%s", promptReceived)
	}

	// 3. Verify written markdown file
	updatedBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	updatedContent := string(updatedBytes)

	if !strings.HasPrefix(updatedContent, "---\n") {
		t.Fatalf("expected YAML frontmatter at beginning of file, got:\n%s", updatedContent)
	}
	parts := strings.SplitN(updatedContent, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("expected 3 parts with frontmatter delimiters, got: %d", len(parts))
	}

	var parsed OKFMetadata
	if err := yaml.Unmarshal([]byte(parts[1]), &parsed); err != nil {
		t.Fatalf("failed to parse YAML frontmatter: %v", err)
	}
	if parsed.Type != "Reference Article" {
		t.Errorf("expected type 'Reference Article', got %q", parsed.Type)
	}
	if parsed.Title != "Cloud Native Overview" {
		t.Errorf("expected title 'Cloud Native Overview', got %q", parsed.Title)
	}
	if parsed.Description != "A guide to modern cloud computing architecture." {
		t.Errorf("expected description, got %q", parsed.Description)
	}
	if parsed.Resource != "https://example.com/cloud" {
		t.Errorf("expected resource, got %q", parsed.Resource)
	}
	expectedTags := []string{"cloud", "kubernetes", "devops"}
	if len(parsed.Tags) != len(expectedTags) {
		t.Errorf("expected %d tags, got %v", len(expectedTags), parsed.Tags)
	} else {
		for i, tag := range expectedTags {
			if parsed.Tags[i] != tag {
				t.Errorf("expected tag %s, got %s", tag, parsed.Tags[i])
			}
		}
	}
	if parsed.Generated.By != "agent/readr-pipeline" {
		t.Errorf("expected generated.by 'agent/readr-pipeline', got %q", parsed.Generated.By)
	}
	if parsed.Generated.At == "" {
		t.Errorf("expected non-empty generated.at timestamp")
	}

	// Verify Summary block in body
	if !strings.Contains(parts[2], "> 💡 **Summary:** This article discusses cloud computing and Kubernetes orchestration.") {
		t.Errorf("expected summary block in body, got:\n%s", parts[2])
	}

	// Verify Wikilink in body
	if !strings.Contains(parts[2], "[[Kubernetes Guide]]") {
		t.Errorf("expected [[Kubernetes Guide]] injected in body, got:\n%s", parts[2])
	}

	// 4. Verify database article_links record inserted
	var linkCount int64
	if err := db.Table("article_links").Where("source_id = ? AND target_id = ?", 401, 402).Count(&linkCount).Error; err != nil {
		t.Fatalf("failed to query article_links: %v", err)
	}
	if linkCount != 1 {
		t.Errorf("expected 1 record in article_links for source 401 -> target 402, got %d", linkCount)
	}

	// 5. Verify database article tags were updated and synchronized
	var updatedArticle repository.GormArticle
	if err := db.First(&updatedArticle, 401).Error; err != nil {
		t.Fatalf("failed to query updated article: %v", err)
	}
	if updatedArticle.Tags != "cloud, kubernetes, devops" {
		t.Errorf("expected article tags 'cloud, kubernetes, devops', got %q", updatedArticle.Tags)
	}
}

func TestProcessPipeline_OnlySummarizer(t *testing.T) {
	tempDir, db, repo := setupTestPipelineEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	sourcePath := filepath.Join(articlesDir, "403.md")
	initialContent := "---\ntitle: \"Preserved Original Title\"\ncustom_field: \"preserved_val\"\n---\n\n> 💡 **Summary:** Old placeholder summary\n\nThis is the body of the article."
	if err := os.WriteFile(sourcePath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	db.Create(&repository.GormArticle{ID: 403, Title: "Preserved Original Title"})

	promptReceived := ""
	var capturedResponseFormat map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if rf, ok := req["response_format"].(map[string]interface{}); ok {
			capturedResponseFormat = rf
		}
		if msgs, ok := req["messages"].([]interface{}); ok && len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]interface{}); ok {
				promptReceived = fmt.Sprint(m["content"])
			}
		}

		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"summary": "Updated AI summary for article."}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	pool := &AgentPool{
		Queue:         make(chan Job, 10),
		logger:        zap.NewNop(),
		db:            db,
		repo:          repo,
		dataDirectory: tempDir,
	}

	job := Job{
		ArticleID: 403,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: true,
			Enricher:   false,
			Linker:     false,
		},
	}

	pool.processPipelineWithURL(job, ts.URL)

	// 1. Verify schema ONLY contains summary
	if capturedResponseFormat == nil {
		t.Fatalf("expected response_format to be captured")
	}
	schemaDef, _ := capturedResponseFormat["json_schema"].(map[string]interface{})
	schema, _ := schemaDef["schema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})

	if _, ok := props["summary"]; !ok {
		t.Errorf("expected 'summary' property in schema")
	}
	if _, ok := props["frontmatter"]; ok {
		t.Errorf("schema should NOT contain 'frontmatter' when Enricher is disabled")
	}
	if _, ok := props["links_to_inject"]; ok {
		t.Errorf("schema should NOT contain 'links_to_inject' when Linker is disabled")
	}

	reqFields, _ := schema["required"].([]interface{})
	if len(reqFields) != 1 || fmt.Sprint(reqFields[0]) != "summary" {
		t.Errorf("expected required to contain only 'summary', got: %v", reqFields)
	}

	// 2. Verify prompt requests only summary
	if !strings.Contains(promptReceived, "1. SUMMARY") {
		t.Errorf("expected SUMMARY instruction in prompt")
	}
	if strings.Contains(promptReceived, "2. FRONTMATTER") {
		t.Errorf("prompt should NOT contain FRONTMATTER instruction")
	}
	if strings.Contains(promptReceived, "3. SMART LINKING") {
		t.Errorf("prompt should NOT contain SMART LINKING instruction")
	}

	// 3. Verify file content: frontmatter preserved, summary block updated
	updatedBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	updatedContent := string(updatedBytes)

	if !strings.Contains(updatedContent, "title: \"Preserved Original Title\"") || !strings.Contains(updatedContent, "custom_field: \"preserved_val\"") {
		t.Errorf("expected original frontmatter to be preserved, got:\n%s", updatedContent)
	}
	if !strings.Contains(updatedContent, "> 💡 **Summary:** Updated AI summary for article.") {
		t.Errorf("expected updated summary block in body, got:\n%s", updatedContent)
	}
	if !strings.Contains(updatedContent, "This is the body of the article.") {
		t.Errorf("expected original body text to remain, got:\n%s", updatedContent)
	}

	// 4. Verify no links inserted
	var linkCount int64
	db.Table("article_links").Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("expected 0 article_links, got %d", linkCount)
	}
}

func TestProcessPipeline_EnricherAndLinkerOnly(t *testing.T) {
	tempDir, db, repo := setupTestPipelineEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	// Target candidate
	db.Create(&repository.GormArticle{ID: 405, Title: "Golang Basics", Tags: "golang"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (405, 'Golang Basics', 'golang')")

	// Source article
	sourcePath := filepath.Join(articlesDir, "404.md")
	initialContent := "---\ntitle: Draft\n---\n\nLearning Golang Basics is very rewarding."
	if err := os.WriteFile(sourcePath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	db.Create(&repository.GormArticle{ID: 404, Title: "Go Guide", Tags: "go"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (404, 'Go Guide', 'go')")

	promptReceived := ""
	var capturedResponseFormat map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if rf, ok := req["response_format"].(map[string]interface{}); ok {
			capturedResponseFormat = rf
		}
		if msgs, ok := req["messages"].([]interface{}); ok && len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]interface{}); ok {
				promptReceived = fmt.Sprint(m["content"])
			}
		}

		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{
							"frontmatter": {
								"type": "Tutorial",
								"title": "Go Guide",
								"description": "A tutorial for Go fundamentals.",
								"resource": "",
								"tags": ["Golang", "Basics"]
							},
							"links_to_inject": [
								{
									"existing_article_id": 405,
									"exact_phrase_in_text": "Golang Basics"
								}
							]
						}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	pool := &AgentPool{
		Queue:         make(chan Job, 10),
		logger:        zap.NewNop(),
		db:            db,
		repo:          repo,
		dataDirectory: tempDir,
	}

	job := Job{
		ArticleID: 404,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: false,
			Enricher:   true,
			Linker:     true,
		},
	}

	pool.processPipelineWithURL(job, ts.URL)

	// 1. Verify schema contains frontmatter and links_to_inject, NO summary
	if capturedResponseFormat == nil {
		t.Fatalf("expected response_format to be captured")
	}
	schemaDef, _ := capturedResponseFormat["json_schema"].(map[string]interface{})
	schema, _ := schemaDef["schema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})

	if _, ok := props["summary"]; ok {
		t.Errorf("schema should NOT contain 'summary' when Summarizer is disabled")
	}
	if _, ok := props["frontmatter"]; !ok {
		t.Errorf("expected 'frontmatter' property in schema")
	}
	if _, ok := props["links_to_inject"]; !ok {
		t.Errorf("expected 'links_to_inject' property in schema")
	}

	reqFields, _ := schema["required"].([]interface{})
	for _, f := range reqFields {
		if fmt.Sprint(f) == "summary" {
			t.Errorf("required list should NOT contain 'summary'")
		}
	}

	// 2. Verify prompt contains frontmatter and smart linking, NO summary
	if strings.Contains(promptReceived, "1. SUMMARY") {
		t.Errorf("prompt should NOT contain SUMMARY instruction")
	}
	if !strings.Contains(promptReceived, "2. FRONTMATTER") {
		t.Errorf("expected FRONTMATTER instruction in prompt")
	}
	if !strings.Contains(promptReceived, "3. SMART LINKING") {
		t.Errorf("expected SMART LINKING instruction in prompt")
	}

	// 3. Verify file output: updated frontmatter, wikilinks injected, NO summary block
	updatedBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	updatedContent := string(updatedBytes)

	parts := strings.SplitN(updatedContent, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("expected frontmatter delimiter, got:\n%s", updatedContent)
	}

	var parsed OKFMetadata
	if err := yaml.Unmarshal([]byte(parts[1]), &parsed); err != nil {
		t.Fatalf("failed to parse YAML frontmatter: %v", err)
	}
	if parsed.Type != "Tutorial" || parsed.Title != "Go Guide" {
		t.Errorf("unexpected parsed frontmatter: %+v", parsed)
	}
	if parsed.Generated.By != "agent/readr-pipeline" {
		t.Errorf("expected generated.by 'agent/readr-pipeline', got %q", parsed.Generated.By)
	}

	// Verify wikilink injected
	if !strings.Contains(parts[2], "[[Golang Basics]]") {
		t.Errorf("expected [[Golang Basics]] in body, got:\n%s", parts[2])
	}

	// Verify no summary block was generated
	if strings.Contains(parts[2], "> 💡 **Summary:**") {
		t.Errorf("body should NOT contain summary block, got:\n%s", parts[2])
	}

	// 4. Verify DB link inserted
	var linkCount int64
	db.Table("article_links").Where("source_id = ? AND target_id = ?", 404, 405).Count(&linkCount)
	if linkCount != 1 {
		t.Errorf("expected 1 record in article_links, got %d", linkCount)
	}
}

func TestProcessPipeline_AllDisabled_ZeroCalls(t *testing.T) {
	tempDir, db, repo := setupTestPipelineEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	sourcePath := filepath.Join(articlesDir, "406.md")
	initialContent := "---\ntitle: Disabled Pipeline Article\n---\n\nBody remains completely untouched."
	if err := os.WriteFile(sourcePath, []byte(initialContent), 0644); err != nil {
		t.Fatal(err)
	}
	db.Create(&repository.GormArticle{ID: 406, Title: "Disabled Pipeline Article"})

	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	pool := &AgentPool{
		Queue:         make(chan Job, 10),
		logger:        zap.NewNop(),
		db:            db,
		repo:          repo,
		dataDirectory: tempDir,
	}

	job := Job{
		ArticleID: 406,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: false,
			Enricher:   false,
			Linker:     false,
		},
	}

	pool.processPipelineWithURL(job, ts.URL)

	// Assert mock HTTP server was NEVER called
	if called {
		t.Errorf("expected mock HTTP server NOT to be called when all pipeline stages are disabled")
	}

	// Assert markdown file is untouched
	currentBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(currentBytes) != initialContent {
		t.Errorf("expected file content to remain untouched, got:\n%s", string(currentBytes))
	}
}

func TestProcessPipeline_RecordsMetrics(t *testing.T) {
	tempDir, db, repo := setupTestPipelineEnv(t)
	articlesDir := filepath.Join(tempDir, "articles")

	sourcePath := filepath.Join(articlesDir, "407.md")
	os.WriteFile(sourcePath, []byte("Content about Golang concurrency and channels."), 0644)
	db.Create(&repository.GormArticle{ID: 407, Title: "Golang Concurrency"})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"summary": "A deep dive into Go concurrency channels."}`,
					},
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     150,
				"completion_tokens": 40,
				"total_tokens":      190,
			},
		}
		json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	pool := &AgentPool{
		Queue:         make(chan Job, 10),
		logger:        zap.NewNop(),
		db:            db,
		repo:          repo,
		dataDirectory: tempDir,
	}

	job := Job{
		ArticleID: 407,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: true,
		},
	}

	pool.processPipelineWithURL(job, ts.URL)

	// Assert metrics were recorded in DB
	summary, recent, err := repo.GetPipelineDiagnostics(context.Background(), 10)
	if err != nil {
		t.Fatalf("failed to get diagnostics: %v", err)
	}
	if summary.TotalRuns != 1 {
		t.Fatalf("expected 1 recorded run, got %d", summary.TotalRuns)
	}
	if summary.SuccessfulRuns != 1 {
		t.Errorf("expected 1 successful run, got %d", summary.SuccessfulRuns)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent run, got %d", len(recent))
	}
	if recent[0].ArticleID != 407 {
		t.Errorf("expected metric article_id 407, got %d", recent[0].ArticleID)
	}
	if recent[0].Status != "success" {
		t.Errorf("expected metric status 'success', got %q", recent[0].Status)
	}
	if recent[0].PromptTokens != 150 || recent[0].CompletionTokens != 40 {
		t.Errorf("expected prompt 150 and completion 40, got %d / %d", recent[0].PromptTokens, recent[0].CompletionTokens)
	}
	if recent[0].TotalTokens != 190 {
		t.Errorf("expected total tokens 190, got %d", recent[0].TotalTokens)
	}
}

func TestPipeline_MOCTagExcludedFromEnricherAndStripped(t *testing.T) {
	// 1. Verify buildPipelinePrompt excludes "moc" from Existing Vault Tags and adds warning
	settings := PipelineSettings{Enricher: true}
	existingTags := []string{"moc", "golang", "microservices"}
	prompt := buildPipelinePrompt(settings, "Article content", nil, existingTags)

	if strings.Contains(prompt, "Existing Vault Tags:\nmoc") || strings.Contains(prompt, "moc, ") || strings.Contains(prompt, ", moc") {
		t.Errorf("prompt Existing Vault Tags should NOT contain 'moc', got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "golang") || !strings.Contains(prompt, "microservices") {
		t.Errorf("prompt should still contain other valid vault tags")
	}

	// 2. Verify mergeArticleTags strips "moc" from aiTags
	aiTags := []string{"golang", "MOC", "distributed-systems"}
	merged := mergeArticleTags("existing-tag", aiTags)

	for _, tag := range merged {
		if strings.ToLower(tag) == "moc" {
			t.Errorf("mergeArticleTags should have stripped 'moc' tag, but found in: %v", merged)
		}
	}
	if len(merged) != 3 {
		t.Errorf("expected 3 merged tags (existing-tag, golang, distributed-systems), got %d: %v", len(merged), merged)
	}
}
