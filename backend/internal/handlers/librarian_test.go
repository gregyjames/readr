package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/backend/internal/agents"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestLibrarianHandler(t *testing.T) (*fiber.App, *HandlerContext, *agents.LibrarianRunner) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.PipelineMetric{})
	repo := repository.NewGormRepository(db)
	settingsStore := NewSettingsStore(tempDir, zap.NewNop())

	hCtx := &HandlerContext{
		DB:            db,
		Logger:        zap.NewNop(),
		DataDir:       tempDir,
		SettingsStore: settingsStore,
		Repo:          repo,
	}

	runner := agents.NewLibrarianRunner(zap.NewNop(), db, repo, tempDir, nil)
	cronManager := agents.NewLibrarianCronManager(runner, zap.NewNop())

	app := fiber.New()
	RegisterLibrarian(app, hCtx, runner, cronManager)

	return app, hCtx, runner
}

func TestLibrarianEndpoints_StatusAndRun(t *testing.T) {
	app, _, _ := setupTestLibrarianHandler(t)

	// 1. GET /librarian/status
	reqStatus := httptest.NewRequest("GET", "/librarian/status", nil)
	respStatus, err := app.Test(reqStatus, 5000)
	if err != nil {
		t.Fatalf("GET /librarian/status failed: %v", err)
	}
	if respStatus.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", respStatus.StatusCode)
	}

	var status agents.LibrarianStatus
	_ = json.NewDecoder(respStatus.Body).Decode(&status)
	if !status.Enabled {
		t.Errorf("expected librarian to be enabled by default")
	}
	if status.MinClusterSize != 5 {
		t.Errorf("expected default min cluster size 5, got %d", status.MinClusterSize)
	}

	// 2. POST /librarian/run (no API key in test environment returns skipped status cleanly)
	reqRun := httptest.NewRequest("POST", "/librarian/run", nil)
	respRun, err := app.Test(reqRun, 5000)
	if err != nil {
		t.Fatalf("POST /librarian/run failed: %v", err)
	}
	if respRun.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", respRun.StatusCode)
	}

	var runResult agents.LibrarianRunResult
	_ = json.NewDecoder(respRun.Body).Decode(&runResult)
	if runResult.Status == "" {
		t.Errorf("expected runResult.Status to be non-empty")
	}
}

func TestSettingsUpdate_ReconfiguresLibrarianCron(t *testing.T) {
	app, hCtx, runner := setupTestLibrarianHandler(t)
	cronManager := agents.NewLibrarianCronManager(runner, zap.NewNop())
	hCtx.LibrarianCron = cronManager
	_ = cronManager.Start("0 0 * * *", true)

	RegisterSettings(app, hCtx)

	// Update settings with a new valid cron schedule: "0 12 * * *"
	reqBody := `{"librarian_enabled":true,"librarian_cron":"0 12 * * *"}`
	req := httptest.NewRequest("POST", "/settings", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("POST /settings failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	next := cronManager.GetNextRun()
	if next == nil {
		t.Fatalf("expected active next run after reconfiguring cron, got nil")
	}
}
