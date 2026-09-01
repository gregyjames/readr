package vault

import (
	"context"
	"database/sql"
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

func setupTestDBAndOrganizer(t *testing.T) (*gorm.DB, *VaultOrganizer, string) {
	t.Helper()
	tempDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to initialize gorm: %v", err)
	}
	if err := db.AutoMigrate(&repository.GormArticle{}, &repository.GormArticleLink{}); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	logger := zap.NewNop()
	organizer := NewVaultOrganizer(tempDir, db, logger)
	return db, organizer, tempDir
}

func TestVaultOrganizer_EnsureTopicFolder(t *testing.T) {
	_, organizer, dataDir := setupTestDBAndOrganizer(t)

	testCases := []struct {
		inputTopic     string
		expectedFolder string
	}{
		{"Distributed Systems", "Distributed Systems"},
		{"AI / Machine Learning", "AI Machine Learning"},
		{"Invalid: * ? < > |", "Invalid"},
		{"", "General"},
		{"...", "General"},
	}

	for _, tc := range testCases {
		folderName, err := organizer.EnsureTopicFolder(tc.inputTopic)
		if err != nil {
			t.Fatalf("EnsureTopicFolder(%q) unexpected error: %v", tc.inputTopic, err)
		}
		if folderName != tc.expectedFolder {
			t.Errorf("EnsureTopicFolder(%q) got %q, expected %q", tc.inputTopic, folderName, tc.expectedFolder)
		}
		expectedPath := filepath.Join(dataDir, "articles", tc.expectedFolder)
		info, err := os.Stat(expectedPath)
		if err != nil || !info.IsDir() {
			t.Errorf("expected directory at %s, got error: %v", expectedPath, err)
		}
	}
}

func TestVaultOrganizer_FileArticle_FromRootToFolder(t *testing.T) {
	db, organizer, dataDir := setupTestDBAndOrganizer(t)
	ctx := context.Background()

	// 1. Create articles directory and root article file
	articlesDir := filepath.Join(dataDir, "articles")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		t.Fatalf("failed to create articles dir: %v", err)
	}

	articleFileName := "Note.md"
	rootFilePath := filepath.Join(articlesDir, articleFileName)
	noteContent := []byte("# My Note\nContent here")
	if err := os.WriteFile(rootFilePath, noteContent, 0644); err != nil {
		t.Fatalf("failed to write initial note: %v", err)
	}

	// 2. Insert article into DB with path /articles/Note.md
	article := repository.GormArticle{
		ID:      1,
		Title:   "My Note",
		Article: "/articles/Note.md",
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("failed to create article in DB: %v", err)
	}

	// 3. File article under "Go Programming"
	newPath, err := organizer.FileArticle(ctx, 1, "Go Programming")
	if err != nil {
		t.Fatalf("FileArticle returned error: %v", err)
	}

	expectedPath := "/articles/Go Programming/Note.md"
	if newPath != expectedPath {
		t.Errorf("FileArticle returned %q, expected %q", newPath, expectedPath)
	}

	// 4. Verify file was moved on disk
	destFilePath := filepath.Join(dataDir, "articles", "Go Programming", "Note.md")
	if _, err := os.Stat(destFilePath); err != nil {
		t.Errorf("destination file %s does not exist: %v", destFilePath, err)
	}
	if _, err := os.Stat(rootFilePath); !os.IsNotExist(err) {
		t.Errorf("source file %s should have been moved/removed", rootFilePath)
	}

	// 5. Verify database record updated
	var updated repository.GormArticle
	if err := db.First(&updated, 1).Error; err != nil {
		t.Fatalf("failed to fetch updated article from DB: %v", err)
	}
	if updated.Article != expectedPath {
		t.Errorf("expected DB path %q, got %q", expectedPath, updated.Article)
	}
}

func TestVaultOrganizer_FileArticle_ReFiling(t *testing.T) {
	db, organizer, dataDir := setupTestDBAndOrganizer(t)
	ctx := context.Background()

	// 1. Setup old topic directory and file
	oldTopicDir := filepath.Join(dataDir, "articles", "Old Topic")
	if err := os.MkdirAll(oldTopicDir, 0755); err != nil {
		t.Fatalf("failed to create old topic dir: %v", err)
	}

	oldFilePath := filepath.Join(oldTopicDir, "Note.md")
	if err := os.WriteFile(oldFilePath, []byte("# Note in Old Topic"), 0644); err != nil {
		t.Fatalf("failed to write old note: %v", err)
	}

	// 2. Insert DB record pointing to /articles/Old Topic/Note.md
	article := repository.GormArticle{
		ID:      2,
		Title:   "Note",
		Article: "/articles/Old Topic/Note.md",
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("failed to create article in DB: %v", err)
	}

	// 3. Move/refile to "New Topic"
	newPath, err := organizer.FileArticle(ctx, 2, "New Topic")
	if err != nil {
		t.Fatalf("FileArticle re-filing failed: %v", err)
	}

	expectedPath := "/articles/New Topic/Note.md"
	if newPath != expectedPath {
		t.Errorf("expected new path %q, got %q", expectedPath, newPath)
	}

	// 4. Verify physical file moved
	newFilePath := filepath.Join(dataDir, "articles", "New Topic", "Note.md")
	if _, err := os.Stat(newFilePath); err != nil {
		t.Errorf("expected new file at %s: %v", newFilePath, err)
	}
	if _, err := os.Stat(oldFilePath); !os.IsNotExist(err) {
		t.Errorf("expected old file at %s to no longer exist", oldFilePath)
	}

	// 5. Verify DB record
	var updated repository.GormArticle
	if err := db.First(&updated, 2).Error; err != nil {
		t.Fatalf("failed to find updated article in DB: %v", err)
	}
	if updated.Article != expectedPath {
		t.Errorf("expected DB article field %q, got %q", expectedPath, updated.Article)
	}
}

func TestVaultOrganizer_CleanEmptyFolders(t *testing.T) {
	_, organizer, dataDir := setupTestDBAndOrganizer(t)

	articlesDir := filepath.Join(dataDir, "articles")
	emptyDir1 := filepath.Join(articlesDir, "EmptyTopic1")
	emptyDir2 := filepath.Join(articlesDir, "EmptyTopic2")
	nonEmptyDir := filepath.Join(articlesDir, "NonEmptyTopic")

	if err := os.MkdirAll(emptyDir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emptyDir2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nonEmptyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "Article.md"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := organizer.CleanEmptyFolders(); err != nil {
		t.Fatalf("CleanEmptyFolders returned error: %v", err)
	}

	// Empty dirs should be deleted
	if _, err := os.Stat(emptyDir1); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", emptyDir1)
	}
	if _, err := os.Stat(emptyDir2); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", emptyDir2)
	}

	// Non-empty dir should still exist
	if _, err := os.Stat(nonEmptyDir); err != nil {
		t.Errorf("expected %s to exist: %v", nonEmptyDir, err)
	}
}

func TestVaultOrganizer_UpdateMasterIndex(t *testing.T) {
	db, organizer, dataDir := setupTestDBAndOrganizer(t)
	ctx := context.Background()

	// Seed 2 MOCs and some member links
	mocs := []repository.GormArticle{
		{ID: 101, Title: "MOC - Artificial Intelligence", Article: "/articles/Artificial Intelligence/MOC - Artificial Intelligence.md", Tags: "moc, ai"},
		{ID: 102, Title: "MOC - Distributed Systems", Article: "/articles/Distributed Systems/MOC - Distributed Systems.md", Tags: "moc, distributed-systems"},
	}
	for _, m := range mocs {
		db.Create(&m)
	}

	db.Create(&repository.GormArticleLink{ID: 1, SourceID: 101, TargetID: 1})
	db.Create(&repository.GormArticleLink{ID: 2, SourceID: 101, TargetID: 2})
	db.Create(&repository.GormArticleLink{ID: 3, SourceID: 102, TargetID: 3})

	if err := organizer.UpdateMasterIndex(ctx); err != nil {
		t.Fatalf("UpdateMasterIndex failed: %v", err)
	}

	indexPath := filepath.Join(dataDir, "articles", "index.md")
	contentBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.md: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "# Vault Index") {
		t.Errorf("expected '# Vault Index' header in index.md")
	}
	if !strings.Contains(content, "[[MOC - Artificial Intelligence]] — 2 notes") {
		t.Errorf("expected '[[MOC - Artificial Intelligence]] — 2 notes' in index.md, got:\n%s", content)
	}
	if !strings.Contains(content, "[[MOC - Distributed Systems]] — 1 notes") {
		t.Errorf("expected '[[MOC - Distributed Systems]] — 1 notes' in index.md, got:\n%s", content)
	}
}

func TestVaultOrganizer_FileArticle_RollbackOnDBError(t *testing.T) {
	db, organizer, dataDir := setupTestDBAndOrganizer(t)
	// Create an article in the DB and on disk
	articlesDir := filepath.Join(dataDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)
	articlePath := filepath.Join(articlesDir, "Rollback Test.md")
	if err := os.WriteFile(articlePath, []byte("# Rollback Test"), 0644); err != nil {
		t.Fatal(err)
	}

	article := repository.GormArticle{
		ID:      888,
		Title:   "Rollback Test",
		Article: "/articles/Rollback Test.md",
	}
	db.Create(&article)

	// Create an SQLite trigger that fails any UPDATE on articles table
	if err := db.Exec("CREATE TRIGGER fail_update BEFORE UPDATE ON articles BEGIN SELECT RAISE(ABORT, 'forced update failure'); END;").Error; err != nil {
		t.Fatalf("failed to create fail trigger: %v", err)
	}

	_, err := organizer.FileArticle(context.Background(), 888, "Target Topic")
	if err == nil {
		t.Fatalf("expected error from failed db update, got nil")
	}

	// Verify target file does NOT exist
	targetPath := filepath.Join(dataDir, "articles", "Target Topic", "Rollback Test.md")
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("expected target file %s not to exist after rollback", targetPath)
	}

	// Verify original file was restored
	if _, err := os.Stat(articlePath); err != nil {
		t.Errorf("expected original file %s to be restored: %v", articlePath, err)
	}
}
