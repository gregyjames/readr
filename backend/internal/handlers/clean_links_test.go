package handlers

import (
	"encoding/json"
	"net/http/httptest"
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

func TestCleanBrokenLinks_UnlinksAndPurgesDanglingLinks(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

	// 1. Create Active Target Article (ID 201)
	db.Create(&repository.GormArticle{
		ID:      201,
		Title:   "Valid Target Article",
		Article: "/articles/Valid Target Article.md",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "Valid Target Article.md"), []byte("# Valid Target"), 0644)

	// 2. Create Source Article (ID 202) containing:
	// - 1 valid aliased link: [[Valid Target Article|valid topic]]
	// - 1 broken aliased link: [[Deleted Article|quantum algorithms]]
	// - 1 broken direct link: [[Nonexistent Note]]
	sourceContent := `---
title: "Source Note"
---

We use [[Valid Target Article|valid topic]] and explore [[Deleted Article|quantum algorithms]] in our [[Nonexistent Note]].`

	db.Create(&repository.GormArticle{
		ID:      202,
		Title:   "Source Note",
		Article: "/articles/Source Note.md",
	})
	sourcePath := filepath.Join(articlesDir, "Source Note.md")
	_ = os.WriteFile(sourcePath, []byte(sourceContent), 0644)

	// 3. Create DB links (including 1 valid link and 2 dangling links to deleted IDs 998 and 999)
	db.Create(&repository.GormArticleLink{SourceID: 202, TargetID: 201})
	db.Create(&repository.GormArticleLink{SourceID: 202, TargetID: 998})
	db.Create(&repository.GormArticleLink{SourceID: 999, TargetID: 201})

	// Run CleanBrokenLinks
	res, err := CleanBrokenLinks(db, tempDir, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ScannedArticles != 2 {
		t.Errorf("expected 2 scanned articles, got %d", res.ScannedArticles)
	}
	if res.UpdatedArticles != 1 {
		t.Errorf("expected 1 updated article, got %d", res.UpdatedArticles)
	}
	if res.CleanedLinks != 2 {
		t.Errorf("expected 2 cleaned links, got %d", res.CleanedLinks)
	}
	if res.PurgedDBLinks != 2 {
		t.Errorf("expected 2 purged DB links, got %d", res.PurgedDBLinks)
	}

	// Verify updated source file content
	updatedBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	updatedContent := string(updatedBytes)

	// Valid link should be preserved exactly
	if !strings.Contains(updatedContent, "[[Valid Target Article|valid topic]]") {
		t.Errorf("expected valid link to be preserved, got:\n%s", updatedContent)
	}

	// Broken aliased link should be replaced by alias "quantum algorithms"
	if strings.Contains(updatedContent, "[[Deleted Article") || !strings.Contains(updatedContent, "explore quantum algorithms in") {
		t.Errorf("expected 'explore quantum algorithms in', got:\n%s", updatedContent)
	}

	// Broken direct link should be replaced by "Nonexistent Note"
	if strings.Contains(updatedContent, "[[Nonexistent Note]]") || !strings.Contains(updatedContent, "our Nonexistent Note.") {
		t.Errorf("expected 'our Nonexistent Note.', got:\n%s", updatedContent)
	}

	// Verify only 1 valid DB link remains
	var remainingLinks []repository.GormArticleLink
	db.Find(&remainingLinks)
	if len(remainingLinks) != 1 {
		t.Errorf("expected 1 remaining DB link, got %d", len(remainingLinks))
	}
	if len(remainingLinks) > 0 && (remainingLinks[0].SourceID != 202 || remainingLinks[0].TargetID != 201) {
		t.Errorf("expected link from 202 to 201, got %+v", remainingLinks[0])
	}
}

func TestCleanLinksEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

	db.Create(&repository.GormArticle{
		ID:      301,
		Title:   "Vault Target",
		Article: "/articles/Vault Target.md",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "Vault Target.md"), []byte("# Target Content"), 0644)

	db.Create(&repository.GormArticle{
		ID:      302,
		Title:   "Vault Note",
		Article: "/articles/Vault Note.md",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "Vault Note.md"), []byte("Note with [[Broken Note|phrase]]"), 0644)

	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Logger:  zap.NewNop(),
	}

	app := fiber.New()
	RegisterArticles(app, hCtx)

	req := httptest.NewRequest("POST", "/vault/clean-links", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res CleanLinksResult
	json.NewDecoder(resp.Body).Decode(&res)
	if res.CleanedLinks != 1 {
		t.Errorf("expected 1 cleaned link, got %d", res.CleanedLinks)
	}
}

func TestCleanBrokenLinks_PreservesFilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

	notePath := filepath.Join(articlesDir, "Custom Perms Note.md")
	_ = os.WriteFile(notePath, []byte("Content with [[Missing Article|broken]]"), 0600)
	_ = os.Chmod(notePath, 0600)

	db.Create(&repository.GormArticle{
		ID:      401,
		Title:   "Custom Perms Note",
		Article: "/articles/Custom Perms Note.md",
	})

	res, err := CleanBrokenLinks(db, tempDir, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CleanedLinks != 1 {
		t.Errorf("expected 1 cleaned link, got %d", res.CleanedLinks)
	}

	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600 to be preserved, got %04o", info.Mode().Perm())
	}
}

func TestCleanBrokenLinks_SurfacesWriteErrors(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0555) // Read-only directory to trigger write error
	defer os.Chmod(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}, &repository.GormArticleStatusType{}, &repository.GormArticleStatus{})

	notePath := filepath.Join(articlesDir, "ReadOnly.md")
	// Temporarily allow write to create file
	_ = os.Chmod(articlesDir, 0755)
	_ = os.WriteFile(notePath, []byte("Note with [[Missing Article|broken]]"), 0644)
	_ = os.Chmod(articlesDir, 0555) // Make directory read-only so .tmp creation fails

	db.Create(&repository.GormArticle{
		ID:      501,
		Title:   "ReadOnly Note",
		Article: "/articles/ReadOnly.md",
	})

	res, err := CleanBrokenLinks(db, tempDir, zap.NewNop())
	if err == nil {
		t.Errorf("expected error when directory is read-only, got nil")
	}
	if res == nil || len(res.Errors) == 0 {
		t.Errorf("expected res.Errors to be populated, got %+v", res)
	}
	if res != nil && res.Status != "failed" && res.Status != "partial_failure" {
		t.Errorf("expected status 'failed' or 'partial_failure', got %q", res.Status)
	}
}
