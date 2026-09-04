package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func makeTestRequest(method, target string) *http.Request {
	if strings.Contains(target, " ") {
		return &http.Request{
			Method:     method,
			URL:        &url.URL{Path: target, RawPath: strings.ReplaceAll(target, " ", "%20")},
			RequestURI: target,
			Header:     make(http.Header),
		}
	}
	return httptest.NewRequest(method, target, nil)
}

func TestGetArticleContent_NestedTopicDirectories(t *testing.T) {
	tempDir := t.TempDir()
	nestedDir := filepath.Join(tempDir, "articles", "Distributed Systems")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested directory: %v", err)
	}

	expectedContent := "# Raft Consensus Algorithm\n\nRaft is a consensus algorithm designed for understandability."
	articleFilePath := filepath.Join(nestedDir, "Raft Consensus.md")
	if err := os.WriteFile(articleFilePath, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("failed to write article file: %v", err)
	}

	// Create a secret file outside articles directory to verify path traversal rejection
	secretFilePath := filepath.Join(tempDir, "secret.txt")
	if err := os.WriteFile(secretFilePath, []byte("SUPER_SECRET_DATA"), 0644); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&repository.GormArticle{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	article := repository.GormArticle{
		ID:      501,
		Title:   "Raft Consensus",
		Article: "/articles/Distributed Systems/Raft Consensus.md",
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("failed to create article record: %v", err)
	}

	repo := repository.NewGormRepository(db)
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Repo:    repo,
		Logger:  zap.NewNop(),
	}

	app := fiber.New()
	RegisterArticles(app, hCtx)

	validRequests := []string{
		"/articles/501",
		"/articles/Distributed Systems/Raft Consensus.md",
		"/articles/Distributed%20Systems/Raft%20Consensus.md",
	}

	for _, reqPath := range validRequests {
		t.Run(reqPath, func(t *testing.T) {
			req := makeTestRequest("GET", reqPath)
			resp, err := app.Test(req, 5000)
			if err != nil {
				t.Fatalf("request %s failed: %v", reqPath, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Fatalf("expected status 200 for %s, got %d", reqPath, resp.StatusCode)
			}
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body for %s: %v", reqPath, err)
			}
			if string(bodyBytes) != expectedContent {
				t.Errorf("expected body %q, got %q", expectedContent, string(bodyBytes))
			}
		})
	}

	traversalRequests := []string{
		"/articles/Distributed Systems/../../secret.txt",
		"/articles/Distributed%20Systems/../../secret.txt",
		"/articles/Distributed%20Systems/%2E%2E%2F%2E%2E%2Fsecret.txt",
		"/articles/Distributed Systems/..%2F..%2Fsecret.txt",
	}

	for _, reqPath := range traversalRequests {
		t.Run("traversal_"+reqPath, func(t *testing.T) {
			req := makeTestRequest("GET", reqPath)
			resp, err := app.Test(req, 5000)
			if err != nil {
				t.Fatalf("request %s failed: %v", reqPath, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 404 && resp.StatusCode != 403 && resp.StatusCode != 400 {
				t.Fatalf("expected 403 or 404 or 400 for path traversal %s, got %d", reqPath, resp.StatusCode)
			}
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body for %s: %v", reqPath, err)
			}
			if strings.Contains(string(bodyBytes), "SUPER_SECRET_DATA") {
				t.Errorf("path traversal leaked secret content for %s: %s", reqPath, string(bodyBytes))
			}
		})
	}
}

func TestArticleArchiveHandlers(t *testing.T) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	// Seed articles: 1 active, 1 archived
	articles := []repository.GormArticle{
		{ID: 101, Title: "Active Article", Article: "101.md", IsArchived: false},
		{ID: 102, Title: "Archived Article", Article: "102.md", IsArchived: true},
	}
	for _, a := range articles {
		if err := db.Create(&a).Error; err != nil {
			t.Fatalf("failed to seed article: %v", err)
		}
	}

	repo := repository.NewGormRepository(db)
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Repo:    repo,
		Logger:  zap.NewNop(),
	}

	app := fiber.New()
	RegisterArticles(app, hCtx)

	// 1. GET /getarticles with no param or archived=false returns active articles
	t.Run("GET /getarticles returns active articles by default", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("GET /getarticles failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var list []repository.GormArticle
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(list) != 1 || list[0].ID != 101 {
			t.Fatalf("expected 1 active article with ID 101, got: %+v", list)
		}
	})

	t.Run("GET /getarticles?archived=false returns active articles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles?archived=false", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("GET /getarticles?archived=false failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var list []repository.GormArticle
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(list) != 1 || list[0].ID != 101 {
			t.Fatalf("expected 1 active article with ID 101, got: %+v", list)
		}
	})

	// 2. GET /getarticles?archived=true returns archived articles
	t.Run("GET /getarticles?archived=true returns archived articles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles?archived=true", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("GET /getarticles?archived=true failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var list []repository.GormArticle
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(list) != 1 || list[0].ID != 102 {
			t.Fatalf("expected 1 archived article with ID 102, got: %+v", list)
		}
	})

	// 3. POST /articles/:id/archive archives the article
	t.Run("POST /articles/:id/archive successfully archives", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/101/archive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("POST /articles/101/archive failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var res map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res["success"] != true || res["is_archived"] != true {
			t.Fatalf("unexpected archive response: %+v", res)
		}

		// Verify DB status
		var a repository.GormArticle
		if err := db.First(&a, 101).Error; err != nil {
			t.Fatalf("failed to fetch article: %v", err)
		}
		if !a.IsArchived {
			t.Errorf("expected article 101 to be archived in DB")
		}
	})

	// 4. POST /articles/:id/unarchive unarchives the article
	t.Run("POST /articles/:id/unarchive successfully unarchives", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/101/unarchive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("POST /articles/101/unarchive failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var res map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res["success"] != true || res["is_archived"] != false {
			t.Fatalf("unexpected unarchive response: %+v", res)
		}

		// Verify DB status
		var a repository.GormArticle
		if err := db.First(&a, 101).Error; err != nil {
			t.Fatalf("failed to fetch article: %v", err)
		}
		if a.IsArchived {
			t.Errorf("expected article 101 to be active in DB")
		}
	})

	// 5. Error handling: 404 if article does not exist
	t.Run("POST /articles/99999/archive returns 404", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/99999/archive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /articles/99999/unarchive returns 404", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/99999/unarchive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	// 6. Error handling: 400 on invalid ID
	t.Run("POST /articles/abc/archive returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/abc/archive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /articles/abc/unarchive returns 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/abc/unarchive", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestDeleteArticle_CleansArticleLinks(t *testing.T) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{})

	// Seed an article and links
	article := repository.GormArticle{
		ID:    10,
		Title: "Target Article",
	}
	db.Create(&article)

	link1 := repository.GormArticleLink{ID: 1, SourceID: 99, TargetID: 10}
	link2 := repository.GormArticleLink{ID: 2, SourceID: 10, TargetID: 100}
	db.Create(&link1)
	db.Create(&link2)

	app := fiber.New()
	api := app.Group("/api")
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Logger:  zap.NewNop(),
	}
	RegisterArticles(api, hCtx)

	req := httptest.NewRequest("DELETE", "/api/delete/10", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify links were cleaned up
	var count int64
	db.Model(&repository.GormArticleLink{}).Where("source_id = 10 OR target_id = 10").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 remaining article_links for article 10, got %d", count)
	}
}
