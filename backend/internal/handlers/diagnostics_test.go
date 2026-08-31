package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetPipelineDiagnostics_Endpoint(t *testing.T) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&repository.PipelineMetric{})
	repo := repository.NewGormRepository(db)

	// Seed one metric
	_ = repo.RecordPipelineMetric(context.Background(), &repository.PipelineMetric{
		ArticleID:           101,
		ArticleTitle:        "Diagnostics Test Article",
		Model:               "google/gemini-2.5-flash",
		Status:              "success",
		DurationMs:          1500,
		RetryCount:          1,
		PromptTokens:        500,
		CompletionTokens:    100,
		TokensSavedEstimate: 850,
		CreatedAt:           time.Now(),
	})

	app := fiber.New()
	app.Get("/api/diagnostics/pipeline", GetPipelineDiagnostics(repo))

	req := httptest.NewRequest("GET", "/api/diagnostics/pipeline", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var res struct {
		Queue struct {
			PendingJobs   int `json:"pending_jobs"`
			MaxCapacity   int `json:"max_capacity"`
			ActiveWorkers int `json:"active_workers"`
		} `json:"queue"`
		Summary struct {
			TotalRuns       int64 `json:"total_runs"`
			SuccessfulRuns  int64 `json:"successful_runs"`
			TotalTokensUsed int64 `json:"total_tokens_used"`
		} `json:"summary"`
		RecentRuns []repository.PipelineMetric `json:"recent_runs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Summary.TotalRuns != 1 {
		t.Errorf("expected total_runs 1, got %d", res.Summary.TotalRuns)
	}
	if res.Summary.TotalTokensUsed != 600 {
		t.Errorf("expected total_tokens_used 600, got %d", res.Summary.TotalTokensUsed)
	}
	if len(res.RecentRuns) != 1 {
		t.Fatalf("expected 1 recent run, got %d", len(res.RecentRuns))
	}
	if res.RecentRuns[0].ArticleTitle != "Diagnostics Test Article" {
		t.Errorf("expected article title 'Diagnostics Test Article', got %q", res.RecentRuns[0].ArticleTitle)
	}
}
