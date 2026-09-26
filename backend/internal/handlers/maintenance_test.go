package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example.com/backend/internal/repository"
	"example.com/backend/internal/vault"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMaintenance_BackupEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	m := vault.NewMaintenanceService(tempDir, db, zap.NewNop())
	hCtx := &HandlerContext{
		DB:          db,
		DataDir:     tempDir,
		Maintenance: m,
		Logger:      zap.NewNop(),
	}

	app := fiber.New()
	api := app.Group("/api")
	RegisterMaintenance(api, hCtx)

	req := httptest.NewRequest("POST", "/api/maintenance/backup", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "success" {
		t.Errorf("expected status 'success', got %v", body["status"])
	}
	backupPath, ok := body["backup_path"].(string)
	if !ok || backupPath == "" {
		t.Fatalf("expected backup_path in response, got %v", body["backup_path"])
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file does not exist on disk: %v", err)
	}
}

func TestMaintenance_IntegrityEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	dbPath := filepath.Join(tempDir, "test.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	// 1 valid article
	db.Create(&repository.GormArticle{ID: 1, Title: "Note 1", Article: "/articles/1.md"})
	_ = os.WriteFile(filepath.Join(articlesDir, "1.md"), []byte("# Note 1"), 0644)

	// 1 orphan file on disk
	_ = os.WriteFile(filepath.Join(articlesDir, "extra.md"), []byte("# Extra"), 0644)

	// 1 missing file in DB
	db.Create(&repository.GormArticle{ID: 2, Title: "Missing Note", Article: "/articles/2.md"})

	m := vault.NewMaintenanceService(tempDir, db, zap.NewNop())
	hCtx := &HandlerContext{
		DB:          db,
		DataDir:     tempDir,
		Maintenance: m,
		Logger:      zap.NewNop(),
	}

	app := fiber.New()
	api := app.Group("/api")
	RegisterMaintenance(api, hCtx)

	req := httptest.NewRequest("GET", "/api/maintenance/integrity", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var report vault.IntegrityReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(report.OrphanFiles) != 1 || report.OrphanFiles[0] != "extra.md" {
		t.Errorf("expected 1 orphan file 'extra.md', got %v", report.OrphanFiles)
	}
	if len(report.MissingFiles) != 1 || report.MissingFiles[0].ID != 2 {
		t.Errorf("expected 1 missing file for ID 2, got %v", report.MissingFiles)
	}
	if report.TotalArticles != 2 {
		t.Errorf("expected 2 total articles, got %d", report.TotalArticles)
	}
	if report.TotalFiles != 2 {
		t.Errorf("expected 2 total files, got %d", report.TotalFiles)
	}
}

func TestMaintenance_UninitializedService(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	RegisterMaintenance(api, &HandlerContext{Maintenance: nil})

	req1 := httptest.NewRequest("POST", "/api/maintenance/backup", nil)
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("backup request failed: %v", err)
	}
	if resp1.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 status when uninitialized, got %d", resp1.StatusCode)
	}

	req2 := httptest.NewRequest("GET", "/api/maintenance/integrity", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("integrity request failed: %v", err)
	}
	if resp2.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500 status when uninitialized, got %d", resp2.StatusCode)
	}
}
