package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func makeTestRequest(method, target string) *http.Request {
	return httptest.NewRequest(method, strings.ReplaceAll(target, " ", "%20"), nil)
}

func setupArticlesTestApp(t *testing.T) (*fiber.App, *gorm.DB, *HandlerContext, func()) {
	t.Helper()
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&repository.GormArticle{},
		&repository.GormArticleLink{},
		&repository.GormArticleStatusType{},
		&repository.GormArticleStatus{},
	))

	repo := repository.NewGormRepository(db)
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Repo:    repo,
		Logger:  zap.NewNop(),
	}

	app := fiber.New()
	api := app.Group("/api")
	RegisterArticles(api, hCtx)

	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
	return app, db, hCtx, cleanup
}

func TestGetArticles_Pagination(t *testing.T) {
	app, db, _, cleanup := setupArticlesTestApp(t)
	defer cleanup()

	// Seed 15 test articles
	for i := 1; i <= 15; i++ {
		art := repository.GormArticle{
			Title:      fmt.Sprintf("Paginated Article %02d", i),
			Article:    fmt.Sprintf("article-%02d.md", i),
			IsArchived: false,
			WordCount:  100,
		}
		require.NoError(t, db.Create(&art).Error)
	}

	// 1. Default pagination (page 1, limit 10)
	req1 := httptest.NewRequest("GET", "/api/getarticles?page=1&limit=10", nil)
	resp1, err := app.Test(req1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode)

	var envelope1 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&envelope1))
	assert.Equal(t, 10, len(envelope1.Data))
	assert.Equal(t, 1, envelope1.Page)
	assert.Equal(t, 10, envelope1.Limit)
	assert.Equal(t, int64(15), envelope1.Total)
	assert.Equal(t, 2, envelope1.TotalPages)

	// 2. Second page (page 2, limit 10)
	req2 := httptest.NewRequest("GET", "/api/getarticles?page=2&limit=10", nil)
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)

	var envelope2 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&envelope2))
	assert.Equal(t, 5, len(envelope2.Data))
	assert.Equal(t, 2, envelope2.Page)

	// 3. All articles requested (all=true)
	req3 := httptest.NewRequest("GET", "/api/getarticles?all=true", nil)
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	assert.Equal(t, 200, resp3.StatusCode)

	var envelope3 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&envelope3))
	assert.Equal(t, 15, len(envelope3.Data))
	assert.Equal(t, 1, envelope3.Page)
	assert.Equal(t, 15, envelope3.Limit)
	assert.Equal(t, int64(15), envelope3.Total)
	assert.Equal(t, 1, envelope3.TotalPages)

	// 4. Default pagination without query parameters (page 1, limit 25)
	req4 := httptest.NewRequest("GET", "/api/getarticles", nil)
	resp4, err := app.Test(req4)
	require.NoError(t, err)
	assert.Equal(t, 200, resp4.StatusCode)

	var envelope4 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp4.Body).Decode(&envelope4))
	assert.Equal(t, 15, len(envelope4.Data))
	assert.Equal(t, 1, envelope4.Page)
	assert.Equal(t, 25, envelope4.Limit)
	assert.Equal(t, int64(15), envelope4.Total)
	assert.Equal(t, 1, envelope4.TotalPages)

	// 5. Empty database pagination clamping (totalPages >= 1)
	emptyApp, _, _, emptyCleanup := setupArticlesTestApp(t)
	defer emptyCleanup()
	req5 := httptest.NewRequest("GET", "/api/getarticles", nil)
	resp5, err := emptyApp.Test(req5)
	require.NoError(t, err)
	assert.Equal(t, 200, resp5.StatusCode)

	var envelope5 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp5.Body).Decode(&envelope5))
	assert.Equal(t, 0, len(envelope5.Data))
	assert.Equal(t, 1, envelope5.Page)
	assert.Equal(t, 25, envelope5.Limit)
	assert.Equal(t, int64(0), envelope5.Total)
	assert.Equal(t, 1, envelope5.TotalPages)

	// 6. all=1 parameter requested
	req6 := httptest.NewRequest("GET", "/api/getarticles?all=1", nil)
	resp6, err := app.Test(req6)
	require.NoError(t, err)
	assert.Equal(t, 200, resp6.StatusCode)

	var envelope6 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp6.Body).Decode(&envelope6))
	assert.Equal(t, 15, len(envelope6.Data))
	assert.Equal(t, 1, envelope6.Page)
	assert.Equal(t, 15, envelope6.Limit)
	assert.Equal(t, int64(15), envelope6.Total)
	assert.Equal(t, 1, envelope6.TotalPages)
	// Verify deterministic id DESC ordering
	assert.True(t, envelope6.Data[0].ID > envelope6.Data[1].ID)

	// 7. Nil DB returns 500 error
	nilDBApp := fiber.New()
	RegisterArticles(nilDBApp.Group("/api"), &HandlerContext{DB: nil})
	req7 := httptest.NewRequest("GET", "/api/getarticles", nil)
	resp7, err := nilDBApp.Test(req7)
	require.NoError(t, err)
	assert.Equal(t, 500, resp7.StatusCode)
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
	if err := db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{}); err != nil {
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
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
			resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
	if err := db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{}); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	// Seed articles: 1 active, 1 archived
	articles := []repository.GormArticle{
		{ID: 101, Title: "Active Article", Article: "101.md", IsArchived: false, WordCount: 400},
		{ID: 102, Title: "Archived Article", Article: "102.md", IsArchived: true, WordCount: 200},
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("GET /getarticles failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var envelope struct {
			Data []repository.GormArticle `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		list := envelope.Data
		if len(list) != 1 || list[0].ID != 101 {
			t.Fatalf("expected 1 active article with ID 101, got: %+v", list)
		}
		if list[0].ReadingTime != "2 min read" {
			t.Errorf("expected ReadingTime = %q, got %q", "2 min read", list[0].ReadingTime)
		}
		if list[0].WordCount != 400 {
			t.Errorf("expected WordCount = %d, got %d", 400, list[0].WordCount)
		}
	})

	t.Run("GET /getarticles?archived=false returns active articles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles?archived=false", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("GET /getarticles?archived=false failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var envelope struct {
			Data []repository.GormArticle `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		list := envelope.Data
		if len(list) != 1 || list[0].ID != 101 {
			t.Fatalf("expected 1 active article with ID 101, got: %+v", list)
		}
	})

	// 2. GET /getarticles?archived=true returns archived articles
	t.Run("GET /getarticles?archived=true returns archived articles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles?archived=true", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("GET /getarticles?archived=true failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var envelope struct {
			Data []repository.GormArticle `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		list := envelope.Data
		if len(list) != 1 || list[0].ID != 102 {
			t.Fatalf("expected 1 archived article with ID 102, got: %+v", list)
		}
	})

	// 3. POST /articles/:id/archive archives the article
	t.Run("POST /articles/:id/archive successfully archives", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/articles/101/archive", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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

	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

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
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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

func TestEditArticle_UpdatesWordCount_And_RollsBack(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

	origContent := "# Original Title\n\nShort content."
	filePath := filepath.Join(articlesDir, "edit_test.md")
	_ = os.WriteFile(filePath, []byte(origContent), 0644)

	db.Create(&repository.GormArticle{
		ID:        50,
		Title:     "Original Title",
		Article:   "/articles/edit_test.md",
		WordCount: 4,
	})

	app := fiber.New()
	api := app.Group("/api")
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Logger:  zap.NewNop(),
	}
	RegisterArticles(api, hCtx)

	// 1. Successful edit updates word_count
	newContent := "# Original Title\n\n" + strings.Repeat("word ", 250)
	bodyBytes, _ := json.Marshal(map[string]string{"content": newContent})
	req := httptest.NewRequest("POST", "/api/edit/50", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("edit request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var updated repository.GormArticle
	db.First(&updated, 50)
	expectedWords, _ := repository.CalculateReadingTime(newContent)
	if updated.WordCount != expectedWords {
		t.Errorf("expected WordCount %d, got %d", expectedWords, updated.WordCount)
	}

	diskBytes, _ := os.ReadFile(filePath)
	if string(diskBytes) != newContent {
		t.Errorf("expected file content to be updated on disk")
	}

	// 2. Failure rollback: when the article row is deleted before DB update executes
	currentContentOnDisk := string(diskBytes)
	db.Exec("DELETE FROM articles WHERE id = 50")

	failingContent := "# Failing Title\n\nContent that should be reverted."
	failBody, _ := json.Marshal(map[string]string{"content": failingContent})
	failReq := httptest.NewRequest("POST", "/api/edit/50", bytes.NewReader(failBody))
	failReq.Header.Set("Content-Type", "application/json")

	// Pre-create article in memory with matching id for handler to find initially, or drop table to trigger DB update error
	db.Exec("DROP TABLE articles")

	failResp, err := app.Test(failReq, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("edit request failed: %v", err)
	}
	defer failResp.Body.Close()

	if failResp.StatusCode == 200 {
		t.Errorf("expected non-200 status when DB table is missing, got %d", failResp.StatusCode)
	}

	// Verify disk file was restored to previous content
	restoredBytes, _ := os.ReadFile(filePath)
	if string(restoredBytes) != currentContentOnDisk {
		t.Errorf("expected disk file to be restored to %q, got %q", currentContentOnDisk, string(restoredBytes))
	}
}

func TestEditArticle_UpdatesFrontmatterAndSyncsLinks(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}))

	// Seed target articles for wikilinks
	db.Create(&repository.GormArticle{ID: 10, Title: "Golang Concurrency"})
	db.Create(&repository.GormArticle{ID: 20, Title: "Distributed Systems"})

	// Seed source article
	filePath := filepath.Join(articlesDir, "Source.md")
	initialContent := "---\ntitle: Old Title\ntags: [old-tag]\n---\n# Old Title\nProse content."
	_ = os.WriteFile(filePath, []byte(initialContent), 0644)

	db.Create(&repository.GormArticle{
		ID:      50,
		Title:   "Old Title",
		Article: "/articles/Source.md",
		Tags:    "old-tag",
	})

	app := fiber.New()
	api := app.Group("/api")
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Logger:  zap.NewNop(),
	}
	RegisterArticles(api, hCtx)

	// Edit with new frontmatter and multiple wikilinks (including duplicate reference)
	editedContent := "---\ntitle: Modern Distributed Go\ntags: [golang, distributed, consensus]\n---\n# Modern Distributed Go\nSee [[Golang Concurrency]] and [[Distributed Systems|DistSys]], plus a second reference to [[Golang Concurrency]]."
	bodyBytes, _ := json.Marshal(map[string]string{"content": editedContent})
	req := httptest.NewRequest("POST", "/api/edit/50", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("edit request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// 1. Verify DB title and tags were synchronized from frontmatter
	var updated repository.GormArticle
	db.First(&updated, 50)
	if updated.Title != "Modern Distributed Go" {
		t.Errorf("expected Title 'Modern Distributed Go', got %q", updated.Title)
	}
	if !strings.Contains(updated.Tags, "golang") || !strings.Contains(updated.Tags, "distributed") {
		t.Errorf("expected updated tags, got %q", updated.Tags)
	}

	// 2. Verify links were synchronized and deduplicated (target 10 and target 20)
	var links []repository.GormArticleLink
	db.Where("source_id = ?", 50).Find(&links)
	if len(links) != 2 {
		t.Fatalf("expected 2 deduplicated links, got %d", len(links))
	}
	targetIDs := map[int64]bool{links[0].TargetID: true, links[1].TargetID: true}
	if !targetIDs[10] || !targetIDs[20] {
		t.Errorf("expected links to targets 10 and 20, got %+v", links)
	}

	// 3. Verify disk file content was updated atomically
	diskBytes, _ := os.ReadFile(filePath)
	if string(diskBytes) != editedContent {
		t.Errorf("expected disk file to match edited content")
	}

	// 4. Edit with empty tags: [] to verify tags are cleared in DB
	clearTagsContent := "---\ntitle: Modern Distributed Go\ntags: []\n---\n# Modern Distributed Go\nContent without tags."
	clearBodyBytes, _ := json.Marshal(map[string]string{"content": clearTagsContent})
	reqClear := httptest.NewRequest("POST", "/api/edit/50", bytes.NewReader(clearBodyBytes))
	reqClear.Header.Set("Content-Type", "application/json")
	respClear, err := app.Test(reqClear, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	defer respClear.Body.Close()
	require.Equal(t, 200, respClear.StatusCode)

	var clearedArticle repository.GormArticle
	db.First(&clearedArticle, 50)
	if clearedArticle.Tags != "" {
		t.Errorf("expected tags to be cleared, got %q", clearedArticle.Tags)
	}
}

func TestAddArticle_Integration(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "add_test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.PipelineMetric{}))
	EnsureFTS(db, zap.NewNop())

	repo := repository.NewGormRepository(db)
	fetcher := ingest.NewHTTPFetcher(10 * time.Second)
	fetcher.AllowLocalhost = true
	extractor := ingest.NewContentExtractor()
	storage := ingest.NewDiskStorage(tempDir)
	ingester := ingest.NewIngester(fetcher, extractor, storage, repo)
	settingsStore := NewSettingsStore(tempDir, zap.NewNop())

	hCtx := &HandlerContext{
		DB:            db,
		DataDir:       tempDir,
		Logger:        zap.NewNop(),
		Repo:          repo,
		Ingester:      ingester,
		SettingsStore: settingsStore,
	}

	app := fiber.New()
	api := app.Group("/api")
	RegisterArticles(api, hCtx)

	// 1. Invalid JSON returns 400
	reqBadJSON := httptest.NewRequest("POST", "/api/add", bytes.NewReader([]byte("{invalid-json")))
	reqBadJSON.Header.Set("Content-Type", "application/json")
	respBadJSON, err := app.Test(reqBadJSON, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil || respBadJSON.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid JSON, got %d, err: %v", respBadJSON.StatusCode, err)
	}

	// 2. Empty URL returns 400
	bodyEmpty, _ := json.Marshal(map[string]string{"url": ""})
	reqEmpty := httptest.NewRequest("POST", "/api/add", bytes.NewReader(bodyEmpty))
	reqEmpty.Header.Set("Content-Type", "application/json")
	respEmpty, err := app.Test(reqEmpty, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil || respEmpty.StatusCode != 400 {
		t.Fatalf("expected 400 for empty URL, got %d, err: %v", respEmpty.StatusCode, err)
	}

	// 3. Mock external web page server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Distributed Storage Architecture</title></head>
<body>
  <article>
    <h1>Distributed Storage Architecture</h1>
    <p>Consensus protocols and Raft quorum ensure reliable distributed replication across clusters.</p>
  </article>
</body>
</html>`))
	}))
	defer server.Close()

	// 4. Successful ingestion
	addPayload, _ := json.Marshal(map[string]interface{}{
		"url":  server.URL + "/architecture",
		"tags": []string{"distributed", "storage"},
	})
	reqAdd := httptest.NewRequest("POST", "/api/add", bytes.NewReader(addPayload))
	reqAdd.Header.Set("Content-Type", "application/json")
	respAdd, err := app.Test(reqAdd, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil || respAdd.StatusCode != 200 {
		t.Fatalf("expected 200 for valid ingest, got %d, err: %v", respAdd.StatusCode, err)
	}

	var addResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		ID      int64  `json:"id"`
	}
	_ = json.NewDecoder(respAdd.Body).Decode(&addResp)
	if addResp.Status != "success" || addResp.ID <= 0 {
		t.Fatalf("expected success with valid ID, got %+v", addResp)
	}

	// Verify database record
	var created repository.GormArticle
	if err := db.First(&created, addResp.ID).Error; err != nil {
		t.Fatalf("failed to query created article: %v", err)
	}
	if !strings.Contains(created.Title, "Distributed Storage") {
		t.Errorf("expected Title to contain 'Distributed Storage', got %q", created.Title)
	}

	// Verify on-disk file
	diskPath := filepath.Join(tempDir, strings.TrimPrefix(created.Article, "/"))
	diskContent, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("failed to read created markdown file: %v", err)
	}
	if !strings.Contains(string(diskContent), "Consensus protocols") {
		t.Errorf("expected file to contain extracted prose")
	}

	// 5. Duplicate ingestion returns exists status with matching ID
	reqDup := httptest.NewRequest("POST", "/api/add", bytes.NewReader(addPayload))
	reqDup.Header.Set("Content-Type", "application/json")
	respDup, err := app.Test(reqDup, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil || respDup.StatusCode != 200 {
		t.Fatalf("expected 200 for duplicate ingest, got %d, err: %v", respDup.StatusCode, err)
	}

	var dupResp struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}
	_ = json.NewDecoder(respDup.Body).Decode(&dupResp)
	if dupResp.Status != "exists" || dupResp.ID != addResp.ID {
		t.Errorf("expected duplicate response with id %d, got %+v", addResp.ID, dupResp)
	}

	// 6. Ingest decoding succeeds independently of Content-Type header
	reqNoCT := httptest.NewRequest("POST", "/api/add", bytes.NewReader(addPayload))
	respNoCT, err := app.Test(reqNoCT, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil || respNoCT.StatusCode != 200 {
		t.Fatalf("expected 200 for ingest without Content-Type header, got %d, err: %v", respNoCT.StatusCode, err)
	}
}
