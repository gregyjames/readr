package vault

import (
	"context"
	"database/sql"
	"fmt"
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

type mockInvalidator struct {
	invalidated bool
}

func (m *mockInvalidator) InvalidateCache() {
	m.invalidated = true
}

func setupTestVaultEnv(t *testing.T) (*DefaultVault, *gorm.DB, string, *mockInvalidator) {
	t.Helper()
	tempDir := t.TempDir()

	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm sqlite: %v", err)
	}

	_ = db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{})
	_ = db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(title, content, tokenize='porter')").Error

	invalidator := &mockInvalidator{}
	v := NewVault(tempDir, db, zap.NewNop(), invalidator)

	return v, db, tempDir, invalidator
}

func TestVault_SaveArticle_And_GetArticle(t *testing.T) {
	v, _, tempDir, inv := setupTestVaultEnv(t)
	ctx := context.Background()

	input := NoteInput{
		Title:   "Deep Architecture in Go",
		Content: "# Deep Architecture in Go\n\nDeep modules provide high leverage.",
		Tags:    "architecture, go",
		Topic:   "Software Engineering",
	}

	art, err := v.SaveArticle(ctx, input)
	if err != nil {
		t.Fatalf("failed to save article: %v", err)
	}
	if art.ID == 0 {
		t.Fatalf("expected non-zero article ID")
	}
	if !inv.invalidated {
		t.Errorf("expected cache invalidator to be called on save")
	}

	// Verify file on disk
	expectedDiskPath := filepath.Join(tempDir, "articles", "Software Engineering", "Deep Architecture in Go.md")
	if _, err := os.Stat(expectedDiskPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s", expectedDiskPath)
	}

	// Verify GetArticle
	record, content, err := v.GetArticle(ctx, art.ID)
	if err != nil {
		t.Fatalf("failed to get article: %v", err)
	}
	if record.Title != input.Title {
		t.Errorf("expected title %q, got %q", input.Title, record.Title)
	}
	if !strings.Contains(content, "Deep modules provide high leverage.") {
		t.Errorf("expected content to match, got %q", content)
	}
}

func TestVault_DeleteArticle_CleansAllSubsystems(t *testing.T) {
	v, db, tempDir, inv := setupTestVaultEnv(t)
	ctx := context.Background()

	// 1. Save an article
	art, err := v.SaveArticle(ctx, NoteInput{
		Title:   "Ephemeral Note",
		Content: "# Ephemeral Note\n\nTo be deleted.",
		Tags:    "temporary",
		Topic:   "Scratch",
	})
	if err != nil {
		t.Fatalf("failed to save article: %v", err)
	}

	// 2. Add relational links
	link1 := repository.GormArticleLink{SourceID: art.ID, TargetID: 999}
	link2 := repository.GormArticleLink{SourceID: 888, TargetID: art.ID}
	db.Create(&link1)
	db.Create(&link2)

	// 3. Add images directory
	imageDir := filepath.Join(tempDir, "images", fmt.Sprint(art.ID))
	_ = os.MkdirAll(imageDir, 0755)
	_ = os.WriteFile(filepath.Join(imageDir, "thumb.png"), []byte("pngdata"), 0644)

	// 4. Execute Delete
	inv.invalidated = false
	if err := v.DeleteArticle(ctx, art.ID); err != nil {
		t.Fatalf("failed to delete article: %v", err)
	}

	// Verify DB record deleted
	var count int64
	db.Model(&repository.GormArticle{}).Where("id = ?", art.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 articles in DB, got %d", count)
	}

	// Verify article_links deleted
	db.Model(&repository.GormArticleLink{}).Where("source_id = ? OR target_id = ?", art.ID, art.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 article_links, got %d", count)
	}

	// Verify images folder removed
	if _, err := os.Stat(imageDir); !os.IsNotExist(err) {
		t.Errorf("expected images directory to be removed")
	}

	// Verify file removed from disk
	filePath := filepath.Join(tempDir, "articles", "Scratch", "ephemeral-note.md")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected markdown file to be deleted")
	}

	// Verify cache invalidated
	if !inv.invalidated {
		t.Errorf("expected cache invalidation on delete")
	}
}

func TestVault_SetArchived_And_ListArticles(t *testing.T) {
	v, _, _, inv := setupTestVaultEnv(t)
	ctx := context.Background()

	art1, _ := v.SaveArticle(ctx, NoteInput{Title: "Active Note 1", Content: "Body 1", Tags: "tech"})
	art2, _ := v.SaveArticle(ctx, NoteInput{Title: "Archived Note 2", Content: "Body 2", Tags: "tech"})

	inv.invalidated = false
	if err := v.SetArchived(ctx, art2.ID, true); err != nil {
		t.Fatalf("failed to archive note: %v", err)
	}
	if !inv.invalidated {
		t.Errorf("expected cache invalidation on archive")
	}

	// List active only
	activeFalse := false
	activeNotes, err := v.ListArticles(ctx, ArticleFilter{Archived: &activeFalse})
	if err != nil {
		t.Fatalf("failed to list active articles: %v", err)
	}
	if len(activeNotes) != 1 || activeNotes[0].ID != art1.ID {
		t.Errorf("expected only Active Note 1, got %v", activeNotes)
	}

	// List archived only
	activeTrue := true
	archivedNotes, err := v.ListArticles(ctx, ArticleFilter{Archived: &activeTrue})
	if err != nil {
		t.Fatalf("failed to list archived articles: %v", err)
	}
	if len(archivedNotes) != 1 || archivedNotes[0].ID != art2.ID {
		t.Errorf("expected only Archived Note 2, got %v", archivedNotes)
	}
}

func TestVault_MoveArticle(t *testing.T) {
	v, _, tempDir, inv := setupTestVaultEnv(t)
	ctx := context.Background()

	art, err := v.SaveArticle(ctx, NoteInput{
		Title:   "Unfiled Discovery",
		Content: "# Unfiled Discovery\n\nInitial thought.",
		Tags:    "ai",
	})
	if err != nil {
		t.Fatalf("failed to save article: %v", err)
	}

	inv.invalidated = false
	newRelPath, err := v.MoveArticle(ctx, art.ID, "Artificial Intelligence")
	if err != nil {
		t.Fatalf("failed to move article: %v", err)
	}

	expectedRel := "/articles/Artificial Intelligence/Unfiled Discovery.md"
	if newRelPath != expectedRel {
		t.Errorf("expected path %q, got %q", expectedRel, newRelPath)
	}

	// Check disk
	expectedAbs := filepath.Join(tempDir, "articles", "Artificial Intelligence", "Unfiled Discovery.md")
	if _, err := os.Stat(expectedAbs); os.IsNotExist(err) {
		t.Errorf("expected file at %s", expectedAbs)
	}

	if !inv.invalidated {
		t.Errorf("expected cache invalidation on move")
	}
}

func TestVault_ResolveFilePath_PathTraversalGuards(t *testing.T) {
	v, _, tempDir, _ := setupTestVaultEnv(t)

	// Valid path
	resolved, err := v.ResolveFilePath("/articles/Topic/Note.md")
	if err != nil {
		t.Fatalf("unexpected error resolving valid path: %v", err)
	}
	expected := filepath.Join(tempDir, "articles", "Topic", "Note.md")
	if resolved != expected {
		t.Errorf("expected %s, got %s", expected, resolved)
	}

	// Traversal attacks
	malicious := []string{
		"../../etc/passwd",
		"/articles/../../secret",
		"..",
		"Topic/../../../root",
	}

	for _, mal := range malicious {
		_, err := v.ResolveFilePath(mal)
		if err == nil {
			t.Errorf("expected error for path traversal %q, got nil", mal)
		}
	}
}

func TestVault_SaveArticle_TransactionFailure_CleansUpFile(t *testing.T) {
	v, db, tempDir, _ := setupTestVaultEnv(t)
	ctx := context.Background()

	// Drop articles table so the DB transaction fails
	_ = db.Migrator().DropTable(&repository.GormArticle{})

	input := NoteInput{
		Title:   "Doomed Article",
		Content: "# Doomed Article\n\nContent that should not remain on disk.",
		Tags:    "failure-test",
		Topic:   "Testing",
	}

	_, err := v.SaveArticle(ctx, input)
	if err == nil {
		t.Fatalf("expected error from SaveArticle when DB table is dropped, got nil")
	}

	expectedDiskPath := filepath.Join(tempDir, "articles", "Testing", "Doomed Article.md")
	if _, err := os.Stat(expectedDiskPath); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be cleaned up after DB transaction failure, but file still exists", expectedDiskPath)
	}
}

func TestVault_SaveArticle_AllocatesUniqueIDsAndDistinctPaths(t *testing.T) {
	v, _, tempDir, _ := setupTestVaultEnv(t)
	ctx := context.Background()

	// 1. Create first article with Windows reserved name "CON"
	art1, err := v.SaveArticle(ctx, NoteInput{
		Title:   "CON",
		Content: "# CON 1",
		Tags:    "reserved",
	})
	if err != nil {
		t.Fatalf("failed to save first article: %v", err)
	}

	// 2. Create second article with same Windows reserved name "CON"
	art2, err := v.SaveArticle(ctx, NoteInput{
		Title:   "CON",
		Content: "# CON 2",
		Tags:    "reserved",
	})
	if err != nil {
		t.Fatalf("failed to save second article: %v", err)
	}

	if art1.ID == art2.ID {
		t.Fatalf("expected distinct IDs for both articles, got %d for both", art1.ID)
	}

	expectedPath1 := fmt.Sprintf("/articles/Article %d.md", art1.ID)
	expectedPath2 := fmt.Sprintf("/articles/Article %d.md", art2.ID)

	if art1.Article != expectedPath1 {
		t.Errorf("expected art1 path %q, got %q", expectedPath1, art1.Article)
	}
	if art2.Article != expectedPath2 {
		t.Errorf("expected art2 path %q, got %q", expectedPath2, art2.Article)
	}

	// Verify both distinct files exist on disk
	diskPath1 := filepath.Join(tempDir, "articles", fmt.Sprintf("Article %d.md", art1.ID))
	diskPath2 := filepath.Join(tempDir, "articles", fmt.Sprintf("Article %d.md", art2.ID))

	if _, err := os.Stat(diskPath1); os.IsNotExist(err) {
		t.Errorf("expected disk file for art1 at %s", diskPath1)
	}
	if _, err := os.Stat(diskPath2); os.IsNotExist(err) {
		t.Errorf("expected disk file for art2 at %s", diskPath2)
	}
}
