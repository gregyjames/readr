# Pipeline Queue & Diagnostics Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a durable telemetry system and real-time Diagnostics Dashboard in Settings to monitor agent pipeline queue depth, execution latency, retry counts, error logs, and token savings analytics.

**Architecture:** A SQLite `pipeline_metrics` table managed via GORM records pipeline execution events and retry statistics. A backend handler `GET /api/diagnostics/pipeline` aggregates latency and token metrics, while the Vue 3 frontend renders a live auto-refreshing dashboard tab in Settings.

**Tech Stack:** Go (Fiber, GORM, SQLite, go-retryablehttp, zap), Vue 3 (Composition API, TypeScript, Tailwind CSS).

---

### Task 1: Telemetry Data Model & Repository Persistence

**Files:**
- Create: `backend/internal/repository/metrics.go`
- Create: `backend/internal/repository/metrics_test.go`
- Modify: `backend/internal/repository/repository.go`
- Modify: `backend/internal/repository/gorm_repo.go`

- [ ] **Step 1: Write the failing tests in `backend/internal/repository/metrics_test.go`**

```go
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

	// 1. Record metrics
	m1 := &PipelineMetric{
		ArticleID:           101,
		ArticleTitle:        "Go Concurrency",
		Model:               "google/gemini-2.5-flash",
		Status:              "success",
		DurationMs:          2000,
		RetryCount:          0,
		PromptTokens:        1000,
		CompletionTokens:    200,
		TokensSavedEstimate: 1850,
		CreatedAt:           time.Now().Add(-2 * time.Minute),
	}
	m2 := &PipelineMetric{
		ArticleID:           102,
		ArticleTitle:        "Docker Guide",
		Model:               "google/gemini-2.5-flash",
		Status:              "failed",
		DurationMs:          4000,
		RetryCount:          2,
		PromptTokens:        1200,
		CompletionTokens:    0,
		TokensSavedEstimate: 0,
		ErrorMessage:        "LLM provider timeout",
		CreatedAt:           time.Now().Add(-1 * time.Minute),
	}

	if err := repo.RecordPipelineMetric(ctx, m1); err != nil {
		t.Fatalf("failed to record m1: %v", err)
	}
	if err := repo.RecordPipelineMetric(ctx, m2); err != nil {
		t.Fatalf("failed to record m2: %v", err)
	}

	// 2. Fetch diagnostics summary & history
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
	if summary.TotalTokensSaved != 1850 {
		t.Errorf("expected total tokens saved 1850, got %d", summary.TotalTokensSaved)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent runs, got %d", len(recent))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/repository -run TestRecordAndGetPipelineDiagnostics`
Expected: Compilation failure (undefined `PipelineMetric`, `RecordPipelineMetric`, `GetPipelineDiagnostics`).

- [ ] **Step 3: Implement `PipelineMetric` model and repository methods in `metrics.go` and `gorm_repo.go`**

```go
// backend/internal/repository/metrics.go
package repository

import (
	"context"
	"time"
)

type PipelineMetric struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ArticleID           int64     `gorm:"index" json:"article_id"`
	ArticleTitle        string    `json:"article_title"`
	Model               string    `json:"model"`
	Status              string    `gorm:"index" json:"status"` // "running", "success", "failed"
	DurationMs          int64     `json:"duration_ms"`
	RetryCount          int       `json:"retry_count"`
	PromptTokens        int       `json:"prompt_tokens"`
	CompletionTokens    int       `json:"completion_tokens"`
	TokensSavedEstimate int       `json:"tokens_saved_estimate"`
	ErrorMessage        string    `json:"error_message"`
	CreatedAt           time.Time `gorm:"index" json:"created_at"`
}

type PipelineDiagnosticsSummary struct {
	TotalRuns        int64 `json:"total_runs"`
	SuccessfulRuns   int64 `json:"successful_runs"`
	FailedRuns       int64 `json:"failed_runs"`
	TotalRetries     int64 `json:"total_retries"`
	AvgDurationMs    int64 `json:"avg_duration_ms"`
	P95DurationMs    int64 `json:"p95_duration_ms"`
	TotalTokensUsed  int64 `json:"total_tokens_used"`
	TotalTokensSaved int64 `json:"total_tokens_saved"`
}
```

Implement `RecordPipelineMetric` and `GetPipelineDiagnostics` on `GormRepository`, and add to `Repository` interface in `repository.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/repository -run TestRecordAndGetPipelineDiagnostics`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/
git commit -m "feat(repository): implement PipelineMetric data model and diagnostics aggregation"
```

---

### Task 2: Pipeline Telemetry & Retry Tracking Integration

**Files:**
- Modify: `backend/internal/agents/pipeline.go`
- Modify: `backend/internal/agents/pipeline_test.go`

- [ ] **Step 1: Write the failing tests in `backend/internal/agents/pipeline_test.go`**

```go
func TestProcessPipeline_RecordsMetrics(t *testing.T) {
	// Setup test environment with SQLite and repository
	// Trigger processPipelineWithURL
	// Assert pipeline_metrics table contains a completed record with duration, model, tokens, and success status
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/agents -run TestProcessPipeline_RecordsMetrics`
Expected: FAIL (no metrics recorded).

- [ ] **Step 3: Integrate telemetry tracking into `pipeline.go`**

- Before dispatching LLM request: Create initial metric entry or start stopwatch.
- Intercept retry count in `retryClient.RequestLogHook` or retry loop.
- Calculate prompt/completion tokens and estimated tokens saved:
  `tokensSaved := (promptTokens * 2) - 150; if tokensSaved < 0 { tokensSaved = 0 }`
- On success/error: Save `PipelineMetric` via `repo.RecordPipelineMetric(context.Background(), &metric)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/agents`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/agents/
git commit -m "feat(agents): record execution latency, retries, and token telemetry in pipeline"
```

---

### Task 3: Diagnostics HTTP API Endpoint

**Files:**
- Create: `backend/internal/handlers/diagnostics.go`
- Create: `backend/internal/handlers/diagnostics_test.go`
- Modify: `backend/main.go`

- [ ] **Step 1: Write the failing test in `backend/internal/handlers/diagnostics_test.go`**

```go
func TestGetPipelineDiagnostics_Endpoint(t *testing.T) {
	// Create Fiber app with diagnostics handler
	// Call GET /api/diagnostics/pipeline
	// Assert JSON status 200 with queue info, summary, and recent_runs array
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/handlers -run TestGetPipelineDiagnostics_Endpoint`
Expected: FAIL.

- [ ] **Step 3: Implement `GET /api/diagnostics/pipeline` handler and register route in `main.go`**

```go
func GetPipelineDiagnostics(repo repository.Repository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		queueDepth := 0
		if agents.Pool != nil && agents.Pool.Queue != nil {
			queueDepth = len(agents.Pool.Queue)
		}
		summary, recent, err := repo.GetPipelineDiagnostics(c.Context(), 50)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"queue": fiber.Map{
				"pending_jobs":   queueDepth,
				"max_capacity":   100,
				"active_workers": 1,
			},
			"summary":     summary,
			"recent_runs": recent,
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/handlers ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/ backend/main.go
git commit -m "feat(handlers): expose GET /api/diagnostics/pipeline endpoint"
```

---

### Task 4: Frontend Diagnostics UI Tab in `SettingsView.vue`

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

- [ ] **Step 1: Add "Diagnostics" Tab to Settings Sidebar & URL Router**
- [ ] **Step 2: Implement Metric Stat Cards & Auto-Refresh Header**
  - Queue Load (depth / capacity)
  - Success Rate (% and counts)
  - Latency & Total Retries
  - Token Savings Estimate
- [ ] **Step 3: Implement Run History Table with Status Filters and Error Expansion**
- [ ] **Step 4: Verify Frontend Build and TypeScript Compilation**
  Run: `npm run build` in `frontend/`
- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "feat(frontend): implement pipeline diagnostics and queue dashboard tab in settings"
```
