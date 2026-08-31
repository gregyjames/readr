# Design Specification: Pipeline Queue & Diagnostics Dashboard

## 1. Overview
This specification outlines the architecture, database schema, backend API, and frontend user interface for the **Pipeline Queue & Diagnostics Dashboard** in Readr.

The dashboard provides real-time and historical observability into the unified background agent pipeline, tracking queue depth, execution latencies, HTTP retry occurrences, failure logs, and token usage and savings analytics.

---

## 2. Telemetry Data Model

### SQLite Schema (`pipeline_metrics`)
A dedicated GORM model stored in the primary SQLite database:

```go
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
```

### Token Estimation Rules
- **Tokens Used**: If returned by the LLM response (`usage.prompt_tokens` + `usage.completion_tokens`), use verbatim; otherwise estimate as `len(prompt)/4 + len(response)/4`.
- **Tokens Saved Estimate**: The unified pipeline combines 3 separate agent passes into 1, eliminating 2 redundant prompt rounds and duplicate candidate list transfers. Estimate as:
  $$\text{TokensSaved} = (\text{PromptTokens} \times 2) - \text{Overhead}(\sim 150\text{ tokens})$$
  (Consistent with the ~65-75% token reduction benchmark).

---

## 3. Backend Architecture

### 3.1 Telemetry Collection in `AgentPool`
In `backend/internal/agents/pipeline.go`:
1. When `processPipeline` begins, create a record with `Status: "running"`.
2. Wrap execution timer (`time.Since(startTime)`).
3. Intercept `retryablehttp` retry events (via `client.RequestLogHook` or custom retry counting policy).
4. On success/failure, update the `PipelineMetric` record with `Status`, `DurationMs`, `RetryCount`, token counts, and any `ErrorMessage`.

### 3.2 Diagnostics API Handler
Create `backend/internal/handlers/diagnostics.go`:
- Route: `GET /api/diagnostics/pipeline`
- Response Payload:
```json
{
  "queue": {
    "pending_jobs": 0,
    "max_capacity": 100,
    "active_workers": 1
  },
  "summary": {
    "total_runs": 0,
    "successful_runs": 0,
    "failed_runs": 0,
    "total_retries": 0,
    "avg_duration_ms": 0,
    "p95_duration_ms": 0,
    "total_tokens_used": 0,
    "total_tokens_saved": 0
  },
  "recent_runs": [
    {
      "id": 1,
      "article_id": 101,
      "article_title": "Example Title",
      "model": "google/gemini-2.5-flash",
      "status": "success",
      "duration_ms": 2340,
      "retry_count": 0,
      "prompt_tokens": 1200,
      "completion_tokens": 300,
      "tokens_saved_estimate": 2250,
      "error_message": "",
      "created_at": "2026-08-31T14:15:00Z"
    }
  ]
}
```

---

## 4. Frontend Architecture (`SettingsView.vue`)

### 4.1 Tab Integration
Add a new tab item `"diagnostics"` in `SettingsView.vue`:
- Sidebar navigation: `[ General, Ingestion, Agents, Diagnostics ]`
- URL binding: `/settings?tab=diagnostics`

### 4.2 UI Components
1. **Header & Auto-Refresh Bar**:
   - Status badge indicating live connection.
   - Auto-refresh toggle (default: active every 3 seconds).
   - "Refresh Now" manual trigger.
2. **Top Metrics Cards**:
   - **Queue Load**: Current queue depth (e.g. `0 / 100`) + Active Workers.
   - **Success Rate**: Overall pass percentage (e.g. `98.5% (138 / 140)`).
   - **Performance & Retries**: Average & P95 execution latency + Total retry count.
   - **Token Efficiency**: Total tokens consumed vs estimated tokens saved.
3. **Execution History Table**:
   - Columns: `Timestamp`, `Article`, `Model`, `Duration`, `Retries`, `Tokens`, `Status`.
   - Filter dropdown: `All`, `Success`, `Failed`.
   - Clickable row / accordion to inspect error payloads on failed runs.

---

## 5. Testing & Validation Plan
1. **Unit Tests (`backend/internal/handlers/diagnostics_test.go`)**:
   - Test metric recording on pipeline run completion.
   - Test `/api/diagnostics/pipeline` endpoint aggregation (averages, P95, totals).
   - Test empty metrics state.
2. **Pipeline Integration Tests (`backend/internal/agents/pipeline_test.go`)**:
   - Verify metrics table is populated when pipeline runs succeed and fail.
3. **End-to-End Test**:
   - Ingest an article, verify diagnostics tab reflects the queue state, duration, tokens, and success status.
