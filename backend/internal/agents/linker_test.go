package agents

import (
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func TestProcessAutoLinker_CandidateBoundedPrompt(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	os.MkdirAll(articlesDir, 0755)

	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{})
	db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, content, tokenize='porter')`)

	repo := repository.NewGormRepository(db)

	// Create target article
	db.Create(&repository.GormArticle{ID: 201, Title: "Docker Guide", Tags: "docker"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (201, 'Docker Guide', 'docker')")

	// Create source article
	sourceArticlePath := filepath.Join(articlesDir, "202.md")
	initialContent := "---\ntitle: Container Overview\n---\n\nWe love using Docker Guide for deployments."
	os.WriteFile(sourceArticlePath, []byte(initialContent), 0644)
	db.Create(&repository.GormArticle{ID: 202, Title: "Container Overview", Tags: "containers"})
	db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (202, 'Container Overview', 'containers')")

	promptReceived := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if msgs, ok := req["messages"].([]interface{}); ok && len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]interface{}); ok {
				promptReceived = fmt.Sprint(m["content"])
			}
		}

		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"links_to_inject": [{"existing_article_id": 201, "exact_phrase_in_text": "Docker Guide"}]}`,
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

	// Write mock settings
	settingsJSON := fmt.Sprintf(`{"api_key":"test-key","model":"test-model"}`)
	os.WriteFile(filepath.Join(tempDir, "settings.json"), []byte(settingsJSON), 0644)

	job := Job{ArticleID: 202, Type: JobTypeAutoLinker}
	pool.processAutoLinkerWithURL(job, ts.URL)

	// Verify prompt contained ID 201 candidate but not excluded 202
	if !strings.Contains(promptReceived, "- ID: 201, Title: Docker Guide") {
		t.Errorf("expected candidate 201 in prompt, got prompt:\n%s", promptReceived)
	}
	if strings.Contains(promptReceived, "- ID: 202") {
		t.Errorf("prompt should not contain self article 202")
	}

	// Verify file was updated with wikilink
	updatedBytes, _ := os.ReadFile(sourceArticlePath)
	if !strings.Contains(string(updatedBytes), "[[Docker Guide]]") {
		t.Errorf("expected [[Docker Guide]] in updated content, got:\n%s", string(updatedBytes))
	}
}
