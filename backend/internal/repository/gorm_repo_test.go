package repository

import (
	"context"
	"database/sql"
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

	// Seed articles
	articles := []GormArticle{
		{ID: 1, Title: "Introduction to Golang and Concurrency", Tags: "golang, concurrency"},
		{ID: 2, Title: "Deep Dive into Kubernetes Operators", Tags: "k8s, containers"},
		{ID: 3, Title: "Database Optimization with SQLite FTS5", Tags: "sqlite, search"},
		{ID: 4, Title: "Cooking Pasta Recipes", Tags: "cooking, food"},
	}
	for _, a := range articles {
		db.Create(&a)
		db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", a.ID, a.Title, a.Tags)
	}

	// Case 1: Search for Golang related candidate, exclude ID 1
	candidates, err := repo.FindCandidates(ctx, 1, "Golang Microservices in Kubernetes", "We use Go channels and Kubernetes pods.", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	for _, c := range candidates {
		if c.ID == 1 {
			t.Errorf("candidate should not include excluded ID 1")
		}
	}

	// Case 2: Zero FTS matches falls back to recent articles
	candidatesFallback, err := repo.FindCandidates(ctx, 4, "Quantum Astrophysics Relativity", "Gravitational waves in spacetime.", 3)
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
	}
}
