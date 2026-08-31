package repository

import (
	"time"
)

type PipelineMetric struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ArticleID        int64     `gorm:"index" json:"article_id"`
	ArticleTitle     string    `json:"article_title"`
	Model            string    `json:"model"`
	Status           string    `gorm:"index" json:"status"` // "running", "success", "failed"
	DurationMs       int64     `json:"duration_ms"`
	RetryCount       int       `json:"retry_count"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	ErrorMessage     string    `json:"error_message"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
}

type PipelineDiagnosticsSummary struct {
	TotalRuns             int64 `json:"total_runs"`
	SuccessfulRuns        int64 `json:"successful_runs"`
	FailedRuns            int64 `json:"failed_runs"`
	TotalRetries          int64 `json:"total_retries"`
	AvgDurationMs         int64 `json:"avg_duration_ms"`
	P95DurationMs         int64 `json:"p95_duration_ms"`
	TotalTokensUsed       int64 `json:"total_tokens_used"`
	TotalPromptTokens     int64 `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
}
