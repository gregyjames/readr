package handlers

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateLegacyArticleFilenames(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	// 1. Seed legacy numeric articles
	db.Create(&repository.GormArticle{
		ID:      101,
		Title:   "Building Distributed Systems in Go",
		Article: "/articles/101.md",
		Tags:    "golang, distributed",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "101.md"), []byte("# Distributed Systems in Go"), 0644)

	db.Create(&repository.GormArticle{
		ID:      102,
		Title:   "What is Raft? A 2026 Guide: Consensus",
		Article: "/articles/102.md",
		Tags:    "raft",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "102.md"), []byte("# Raft Consensus Guide"), 0644)

	// Article already migrated / non-numeric
	db.Create(&repository.GormArticle{
		ID:      103,
		Title:   "Modern SQLite Optimization",
		Article: "/articles/Modern SQLite Optimization.md",
		Tags:    "sqlite",
	})
	_ = os.WriteFile(filepath.Join(articlesDir, "Modern SQLite Optimization.md"), []byte("# Modern SQLite"), 0644)

	count, err := MigrateLegacyArticleFilenames(db, tempDir, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected migration error: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 migrated articles, got %d", count)
	}

	// Verify Article 101
	var a101 repository.GormArticle
	db.First(&a101, 101)
	if a101.Article != "/articles/Building Distributed Systems in Go.md" {
		t.Errorf("expected article path '/articles/Building Distributed Systems in Go.md', got %q", a101.Article)
	}
	if _, err := os.Stat(filepath.Join(articlesDir, "Building Distributed Systems in Go.md")); os.IsNotExist(err) {
		t.Errorf("expected file 'Building Distributed Systems in Go.md' to exist")
	}
	if _, err := os.Stat(filepath.Join(articlesDir, "101.md")); !os.IsNotExist(err) {
		t.Errorf("expected legacy file '101.md' to be removed/renamed")
	}

	// Verify Article 102
	var a102 repository.GormArticle
	db.First(&a102, 102)
	if a102.Article != "/articles/What is Raft A 2026 Guide Consensus.md" {
		t.Errorf("expected article path '/articles/What is Raft A 2026 Guide Consensus.md', got %q", a102.Article)
	}
	if _, err := os.Stat(filepath.Join(articlesDir, "What is Raft A 2026 Guide Consensus.md")); os.IsNotExist(err) {
		t.Errorf("expected file 'What is Raft A 2026 Guide Consensus.md' to exist")
	}

	// Running migration a second time should migrate 0 files
	countSecond, err := MigrateLegacyArticleFilenames(db, tempDir, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected second migration error: %v", err)
	}
	if countSecond != 0 {
		t.Errorf("expected 0 migrated articles on rerun, got %d", countSecond)
	}
}

func TestGetArticleContent_EndpointVariations(t *testing.T) {
	tempDir := t.TempDir()
	articlesDir := filepath.Join(tempDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.GormArticle{})

	_ = os.WriteFile(filepath.Join(articlesDir, "Building Distributed Systems in Go.md"), []byte("# Distributed Systems Content"), 0644)
	db.Create(&repository.GormArticle{
		ID:      101,
		Title:   "Building Distributed Systems in Go",
		Article: "/articles/Building Distributed Systems in Go.md",
	})

	repo := repository.NewGormRepository(db)
	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Repo:    repo,
	}

	app := fiber.New()
	RegisterArticles(app, hCtx)

	pathsToTest := []string{
		"/articles/Building%20Distributed%20Systems%20in%20Go.md",
		"/articles/101.md",
		"/articles/101",
		"/articles/Building%20Distributed%20Systems%20in%20Go",
	}

	for _, p := range pathsToTest {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			resp, err := app.Test(req, 5000)
			if err != nil {
				t.Fatalf("request %s failed: %v", p, err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("expected 200 for %s, got %d", p, resp.StatusCode)
			}
		})
	}
}
