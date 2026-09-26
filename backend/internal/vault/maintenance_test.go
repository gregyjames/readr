package vault

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVault_OnlineBackupAndIntegrityAudit(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	backupsDir := filepath.Join(tempDir, "backups")
	_ = os.MkdirAll(articlesDir, 0755)
	_ = os.MkdirAll(backupsDir, 0755)

	dbPath := filepath.Join(tempDir, "test.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	// Seed 1 article in DB and file on disk
	db.Create(&repository.GormArticle{ID: 1, Title: "Article 1", Article: "/articles/1.md"})
	_ = os.WriteFile(filepath.Join(articlesDir, "1.md"), []byte("# Article 1\nContent"), 0644)

	// Seed 1 orphan file on disk (no DB record)
	_ = os.WriteFile(filepath.Join(articlesDir, "orphan.md"), []byte("# Orphan"), 0644)

	// Seed 1 missing file in DB (DB record exists, file missing)
	db.Create(&repository.GormArticle{ID: 2, Title: "Missing Article", Article: "/articles/missing.md"})

	m := NewMaintenanceService(tempDir, db, zap.NewNop())

	// Test Backup
	backupPath, err := m.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file does not exist at %s: %v", backupPath, err)
	}

	// Test Integrity Audit
	report, err := m.AuditIntegrity(context.Background())
	if err != nil {
		t.Fatalf("AuditIntegrity failed: %v", err)
	}

	if len(report.OrphanFiles) != 1 || report.OrphanFiles[0] != "orphan.md" {
		t.Errorf("expected 1 orphan file 'orphan.md', got: %v", report.OrphanFiles)
	}
	if len(report.MissingFiles) != 1 || report.MissingFiles[0].ID != 2 {
		t.Errorf("expected 1 missing file for ID 2, got: %v", report.MissingFiles)
	}
}

func TestVault_MaintenanceEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	techDir := filepath.Join(articlesDir, "Tech")
	_ = os.MkdirAll(techDir, 0755)

	dbPath := filepath.Join(tempDir, "test.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	// 1. Nested topic file (valid)
	db.Create(&repository.GormArticle{ID: 10, Title: "Tech Note", Article: "/articles/Tech/Note.md"})
	_ = os.WriteFile(filepath.Join(techDir, "Note.md"), []byte("# Tech Note"), 0644)

	// 2. Non-markdown file in articlesDir (should NOT be reported as orphan)
	_ = os.WriteFile(filepath.Join(articlesDir, "notes.txt"), []byte("not markdown"), 0644)
	_ = os.WriteFile(filepath.Join(articlesDir, ".DS_Store"), []byte("binary"), 0644)

	// 3. Soft-deleted article whose file is missing (should NOT be reported as missing)
	now := time.Now()
	db.Create(&repository.GormArticle{
		Model:   gorm.Model{ID: 20, DeletedAt: gorm.DeletedAt{Time: now, Valid: true}},
		ID:      20,
		Title:   "Soft Deleted",
		Article: "/articles/deleted.md",
	})

	// 4. Missing nested file
	db.Create(&repository.GormArticle{ID: 30, Title: "Missing Tech Note", Article: "/articles/Tech/Missing.md"})

	m := NewMaintenanceService(tempDir, db, zap.NewNop())

	// Consecutive backups should generate unique paths without collision error
	b1, err := m.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup 1 failed: %v", err)
	}
	b2, err := m.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup 2 failed: %v", err)
	}
	if b1 == b2 {
		t.Errorf("consecutive backups should have distinct paths: %s vs %s", b1, b2)
	}

	report, err := m.AuditIntegrity(context.Background())
	if err != nil {
		t.Fatalf("AuditIntegrity failed: %v", err)
	}

	// Should not have any orphans (.txt and .DS_Store ignored, Tech/Note.md matches ID 10)
	if len(report.OrphanFiles) != 0 {
		t.Errorf("expected 0 orphan files, got: %v", report.OrphanFiles)
	}

	// Should have exactly 1 missing file (ID 30)
	if len(report.MissingFiles) != 1 || report.MissingFiles[0].ID != 30 {
		t.Errorf("expected missing file ID 30, got: %v", report.MissingFiles)
	}
	if report.TotalArticles != 2 { // ID 10 and ID 30 (soft deleted ID 20 excluded)
		t.Errorf("expected 2 total active articles, got: %d", report.TotalArticles)
	}
}

func TestVault_BackupPathWithSingleQuote(t *testing.T) {
	tempDir := t.TempDir()
	specialDir := filepath.Join(tempDir, "user's data")
	_ = os.MkdirAll(specialDir, 0755)

	dbPath := filepath.Join(specialDir, "test.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	m := NewMaintenanceService(specialDir, db, zap.NewNop())
	backupPath, err := m.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("CreateBackup failed with single quote in path: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file does not exist: %v", err)
	}
}
