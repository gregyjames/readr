package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordAndGetPipelineDiagnostics(t *testing.T) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&PipelineMetric{})

	repo := NewGormRepository(db)
	ctx := context.Background()

	// 1. Empty state
	emptySummary, emptyRecent, err := repo.GetPipelineDiagnostics(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error on empty db: %v", err)
	}
	if emptySummary.TotalRuns != 0 || len(emptyRecent) != 0 {
		t.Errorf("expected 0 runs on empty db, got %d", emptySummary.TotalRuns)
	}

	// 2. Record metrics
	m1 := &PipelineMetric{
		ArticleID:        101,
		ArticleTitle:     "Go Concurrency",
		Model:            "google/gemini-2.5-flash",
		Status:           "success",
		DurationMs:       2000,
		RetryCount:       0,
		PromptTokens:     1000,
		CompletionTokens: 200,
		TotalTokens:      1200,
		CreatedAt:        time.Now().Add(-2 * time.Minute),
	}
	m2 := &PipelineMetric{
		ArticleID:        102,
		ArticleTitle:     "Docker Guide",
		Model:            "google/gemini-2.5-flash",
		Status:           "failed",
		DurationMs:       4000,
		RetryCount:       2,
		PromptTokens:     1200,
		CompletionTokens: 0,
		TotalTokens:      1200,
		ErrorMessage:     "LLM provider timeout",
		CreatedAt:        time.Now().Add(-1 * time.Minute),
	}

	if err := repo.RecordPipelineMetric(ctx, m1); err != nil {
		t.Fatalf("failed to record m1: %v", err)
	}
	if err := repo.RecordPipelineMetric(ctx, m2); err != nil {
		t.Fatalf("failed to record m2: %v", err)
	}

	// 3. Fetch diagnostics summary & history
	summary, recent, err := repo.GetPipelineDiagnostics(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get diagnostics: %v", err)
	}

	if summary.TotalRuns != 2 {
		t.Errorf("expected total runs 2, got %d", summary.TotalRuns)
	}
	if summary.SuccessfulRuns != 1 {
		t.Errorf("expected successful runs 1, got %d", summary.SuccessfulRuns)
	}
	if summary.FailedRuns != 1 {
		t.Errorf("expected failed runs 1, got %d", summary.FailedRuns)
	}
	if summary.TotalRetries != 2 {
		t.Errorf("expected total retries 2, got %d", summary.TotalRetries)
	}
	if summary.AvgDurationMs != 3000 {
		t.Errorf("expected avg duration 3000, got %d", summary.AvgDurationMs)
	}
	if summary.TotalTokensUsed != 2400 {
		t.Errorf("expected total tokens used 2400, got %d", summary.TotalTokensUsed)
	}
	if summary.TotalPromptTokens != 2200 {
		t.Errorf("expected total prompt tokens 2200, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 200 {
		t.Errorf("expected total completion tokens 200, got %d", summary.TotalCompletionTokens)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent runs, got %d", len(recent))
	}
}
