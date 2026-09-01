package handlers

import (
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
