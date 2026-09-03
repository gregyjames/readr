package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func TestExtractCandidateKeywords(t *testing.T) {
	title := "Building Distributed Systems with Go and Kubernetes"
	body := "In this article, we explore how to deploy containerized microservices across clusters using Docker and Raft consensus."

	keywords := extractCandidateKeywords(title, body, 8)

	expectedSubset := []string{"building", "distributed", "systems", "kubernetes", "deploy", "containerized", "microservices", "clusters"}
	for _, kw := range expectedSubset {
		found := false
		for _, k := range keywords {
			if k == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected keyword %q to be in extracted keywords %v", kw, keywords)
		}
	}

	// Verify stopwords like "with", "and", "in", "this", "we", "how", "to", "across", "using" are filtered out
	stopwords := []string{"with", "and", "in", "this", "we", "how", "to", "across", "using"}
	for _, sw := range stopwords {
		for _, k := range keywords {
			if k == sw {
				t.Errorf("stopword %q should not be in extracted keywords", sw)
			}
		}
	}
}

func TestFindCandidates_FTS5AndFallback(t *testing.T) {
	tempDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db.AutoMigrate(&GormArticle{})
	db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
		title, 
		content, 
		tokenize='porter'
	)`)

	repo := NewGormRepository(db)
	ctx := context.Background()

	// Seed articles including a MOC hub note
	articles := []GormArticle{
		{ID: 1, Title: "Introduction to Golang and Concurrency", Tags: "golang, concurrency"},
		{ID: 2, Title: "Deep Dive into Kubernetes Operators", Tags: "k8s, containers"},
		{ID: 3, Title: "Database Optimization with SQLite FTS5", Tags: "sqlite, search"},
		{ID: 4, Title: "Cooking Pasta Recipes", Tags: "cooking, food"},
		{ID: 5, Title: "MOC - Distributed Systems", Tags: "moc, golang, k8s"},
	}
	for _, a := range articles {
		db.Create(&a)
		db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", a.ID, a.Title, a.Tags)
	}

	// Case 1: Search for Golang related candidate, exclude ID 1 -> must NOT return MOC (ID 5)
	candidates, err := repo.FindCandidates(ctx, 1, "Golang Microservices in Kubernetes", "We use Go channels and Kubernetes pods.", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range candidates {
		if c.ID == 1 {
			t.Errorf("candidate should not include excluded ID 1")
		}
		if c.ID == 5 || IsMOCArticle(c.Title, c.Tags) {
			t.Errorf("candidate should NOT include MOC article: %+v", c)
		}
	}

	// Case 2: Zero FTS matches falls back to recent articles -> must NOT return MOC (ID 5)
	candidatesFallback, err := repo.FindCandidates(ctx, 4, "Quantum Astrophysics Relativity", "Gravitational waves in spacetime.", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidatesFallback) == 0 {
		t.Fatalf("expected fallback candidates, got 0")
	}
	for _, c := range candidatesFallback {
		if c.ID == 4 {
			t.Errorf("fallback should not include excluded ID 4")
		}
		if c.ID == 5 || IsMOCArticle(c.Title, c.Tags) {
			t.Errorf("fallback should NOT include MOC article: %+v", c)
		}
	}
}

func TestGetDistinctTags_And_UpdateArticleTags(t *testing.T) {
	tempDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db.AutoMigrate(&GormArticle{})
	repo := NewGormRepository(db)
	ctx := context.Background()

	// Seed articles with various tags including overlaps and casing
	articles := []GormArticle{
		{ID: 1, Title: "A1", Tags: "Golang, Docker"},
		{ID: 2, Title: "A2", Tags: "docker, KUBERNETES, ai"},
		{ID: 3, Title: "A3", Tags: ""},
		{ID: 4, Title: "A4", Tags: "ai, devops"},
	}
	for _, a := range articles {
		db.Create(&a)
	}

	tags, err := repo.GetDistinctTags(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting distinct tags: %v", err)
	}

	expectedTags := []string{"ai", "docker", "devops", "golang", "kubernetes"}
	for _, exp := range expectedTags {
		found := false
		for _, tag := range tags {
			if tag == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tag %q in distinct tags: %v", exp, tags)
		}
	}

	// Test UpdateArticleTags
	err = repo.UpdateArticleTags(ctx, 3, "database, sqlite")
	if err != nil {
		t.Fatalf("unexpected error updating tags: %v", err)
	}

	var updated GormArticle
	db.First(&updated, 3)
	if updated.Tags != "database, sqlite" {
		t.Errorf("expected updated tags 'database, sqlite', got %q", updated.Tags)
	}
}

func TestFindRelevantTags(t *testing.T) {
	tempDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db.AutoMigrate(&GormArticle{})
	db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
		title, 
		content, 
		tokenize='porter'
	)`)

	repo := NewGormRepository(db)
	ctx := context.Background()

	// Seed articles with various tags
	articles := []GormArticle{
		{ID: 1, Title: "Neural Networks in PyTorch", Tags: "ai, machine-learning, python"},
		{ID: 2, Title: "Deep Learning Transformers and AI", Tags: "ai, machine-learning, llm"},
		{ID: 3, Title: "Computer Vision with PyTorch AI", Tags: "ai, vision"},
		{ID: 4, Title: "Unrelated Gardening and Plants", Tags: "gardening, nature"},
		{ID: 5, Title: "MOC - Artificial Intelligence", Tags: "moc, ai"},
	}
	for _, a := range articles {
		db.Create(&a)
		db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", a.ID, a.Title, a.Tags)
	}

	// 1. Search for AI related article: should extract "ai", "machine-learning", etc. and exclude "moc"
	tags, err := repo.FindRelevantTags(ctx, 100, "Deep Learning and Neural Networks in PyTorch", "AI transformers and computer vision", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tags) == 0 {
		t.Fatalf("expected relevant tags, got 0")
	}

	// "ai" appeared in 3 candidate notes, "machine-learning" in 2 -> "ai" must be first
	if tags[0] != "ai" {
		t.Errorf("expected most frequent tag 'ai' first, got %q", tags[0])
	}

	for _, tag := range tags {
		if tag == "moc" {
			t.Errorf("FindRelevantTags should never return 'moc' tag")
		}
		if tag == "gardening" || tag == "nature" {
			t.Errorf("FindRelevantTags should not return unrelated tag %q", tag)
		}
	}

	// 2. Search with limit 2
	limitedTags, err := repo.FindRelevantTags(ctx, 100, "Deep Learning and Neural Networks in PyTorch", "AI transformers and computer vision", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(limitedTags) > 2 {
		t.Errorf("expected at most 2 tags, got %d", len(limitedTags))
	}

	// 3. Search with non-matching keywords: should return empty slice
	emptyTags, err := repo.FindRelevantTags(ctx, 100, "Quantum Astrophysics Space", "Supernovae and black holes", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emptyTags) != 0 {
		t.Errorf("expected 0 relevant tags for non-matching article, got %v", emptyTags)
	}
}

func TestArchiveRepository(t *testing.T) {
	tempDir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&GormArticle{}); err != nil {
		t.Fatal(err)
	}

	repo := NewGormRepository(db)
	ctx := context.Background()

	// Seed articles: 2 active, 1 archived
	articles := []GormArticle{
		{ID: 1, Title: "Active Article 1", Article: "active1.md", Image: "img1.png", Tags: "golang", IsArchived: false},
		{ID: 2, Title: "Active Article 2", Article: "active2.md", Image: "img2.png", Tags: "k8s", IsArchived: false},
		{ID: 3, Title: "Archived Article 3", Article: "archived3.md", Image: "img3.png", Tags: "docker", IsArchived: true},
	}
	for _, a := range articles {
		if err := db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
	}

	// 1. Test GetAllArticles excludes archived articles by default
	activeArticles, err := repo.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activeArticles) != 2 {
		t.Fatalf("expected 2 active articles, got %d", len(activeArticles))
	}
	for _, a := range activeArticles {
		if a.ID == 3 {
			t.Errorf("GetAllArticles should exclude archived article ID 3")
		}
		if a.IsArchived {
			t.Errorf("GetAllArticles should not return archived article: %+v", a)
		}
	}

	// 2. Test GetArchivedArticles returns only archived articles
	archivedArticles, err := repo.GetArchivedArticles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(archivedArticles) != 1 {
		t.Fatalf("expected 1 archived article, got %d", len(archivedArticles))
	}
	if archivedArticles[0].ID != 3 || !archivedArticles[0].IsArchived {
		t.Errorf("expected archived article with ID 3 and IsArchived=true, got: %+v", archivedArticles[0])
	}

	// 3. Test SetArticleArchived(ctx, 1, true)
	if err := repo.SetArticleArchived(ctx, 1, true); err != nil {
		t.Fatalf("unexpected error archiving article: %v", err)
	}

	activeAfterArchive, err := repo.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activeAfterArchive) != 1 {
		t.Fatalf("expected 1 active article after archiving ID 1, got %d", len(activeAfterArchive))
	}
	if activeAfterArchive[0].ID != 2 {
		t.Errorf("expected active article ID 2, got %d", activeAfterArchive[0].ID)
	}

	archivedAfterArchive, err := repo.GetArchivedArticles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(archivedAfterArchive) != 2 {
		t.Fatalf("expected 2 archived articles after archiving ID 1, got %d", len(archivedAfterArchive))
	}

	// 4. Test SetArticleArchived(ctx, 1, false) unarchives
	if err := repo.SetArticleArchived(ctx, 1, false); err != nil {
		t.Fatalf("unexpected error unarchiving article: %v", err)
	}

	activeAfterUnarchive, err := repo.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activeAfterUnarchive) != 2 {
		t.Fatalf("expected 2 active articles after unarchiving ID 1, got %d", len(activeAfterUnarchive))
	}

	// 5. Test SetArticleArchived on nonexistent ID returns ErrNotFound
	if err := repo.SetArticleArchived(ctx, 9999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for nonexistent ID, got: %v", err)
	}
}
