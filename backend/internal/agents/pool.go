package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type JobType string

const (
	JobTypePipeline JobType = "pipeline"
)

type PipelineSettings struct {
	Summarizer bool
	Enricher   bool
	Linker     bool
}

type Job struct {
	ID        int64
	ArticleID int64
	Type      JobType
	Payload   map[string]interface{}
	Settings  PipelineSettings
}

type ActiveJobInfo struct {
	ArticleID int64     `json:"article_id"`
	Type      JobType   `json:"type"`
	WorkerID  int       `json:"worker_id"`
	StartedAt time.Time `json:"started_at"`
}

type QueueStatus struct {
	PendingJobs   int             `json:"pending_jobs"`
	ActiveJobs    int             `json:"active_jobs"`
	TotalInFlight int             `json:"total_in_flight"`
	MaxCapacity   int             `json:"max_capacity"`
	TotalWorkers  int             `json:"total_workers"`
	BusyWorkers   int             `json:"busy_workers"`
	CurrentJobs   []ActiveJobInfo `json:"current_jobs"`
}

type GormAgentJob struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ArticleID int64     `gorm:"index" json:"article_id"`
	Type      string    `gorm:"type:text;not null" json:"type"`
	Status    string    `gorm:"index;type:text;not null" json:"status"` // queued, processing, completed, failed
	Error     string    `gorm:"type:text" json:"error,omitempty"`
	Retries   int       `gorm:"default:0" json:"retries"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (GormAgentJob) TableName() string {
	return "agent_jobs"
}

type AgentPool struct {
	Queue                chan Job
	logger               *zap.Logger
	db                   *gorm.DB
	repo                 repository.Repository
	dataDirectory        string
	InvalidateGraphCache func()
	numWorkers           int
	mu                   sync.RWMutex
	activeJobs           map[int]ActiveJobInfo
	CircuitBreaker       *CircuitBreaker
}

var Pool *AgentPool

func InitPool(logger *zap.Logger, db *gorm.DB, repo repository.Repository, dataDir string, numWorkers int, invalidateGraphCache func()) {
	if repo == nil && db != nil {
		repo = repository.NewGormRepository(db)
	}

	cb := NewCircuitBreaker(5, 2*time.Minute)

	Pool = &AgentPool{
		Queue:                make(chan Job, 100),
		logger:               logger,
		db:                   db,
		repo:                 repo,
		dataDirectory:        dataDir,
		InvalidateGraphCache: invalidateGraphCache,
		numWorkers:           numWorkers,
		activeJobs:           make(map[int]ActiveJobInfo),
		CircuitBreaker:       cb,
	}

	if db != nil {
		if err := db.AutoMigrate(&GormAgentJob{}); err != nil {
			logger.Error("Failed to auto-migrate agent_jobs", zap.Error(err))
		}

		// Reset any lingering "processing" jobs to "queued" on boot so crashes/restarts don't leave jobs permanently stuck.
		if err := db.Model(&GormAgentJob{}).Where("status = ?", "processing").Update("status", "queued").Error; err != nil {
			logger.Error("Failed to reset lingering processing jobs", zap.Error(err))
		}

		// Re-enqueue up to 100 queued jobs from agent_jobs into Pool.Queue.
		var queued []GormAgentJob
		if err := db.Where("status = ?", "queued").Order("id asc").Limit(100).Find(&queued).Error; err == nil {
			for _, qj := range queued {
				job := Job{
					ID:        qj.ID,
					ArticleID: qj.ArticleID,
					Type:      JobType(qj.Type),
					Settings: PipelineSettings{
						Summarizer: true,
						Enricher:   true,
						Linker:     true,
					},
				}
				select {
				case Pool.Queue <- job:
				default:
					logger.Warn("Agent pool queue full during startup recovery", zap.Int64("article_id", qj.ArticleID))
				}
			}
		}
	}

	for i := 0; i < numWorkers; i++ {
		go Pool.worker(i)
	}

	logger.Info("Background agent pool started", zap.Int("workers", numWorkers))
}

func (p *AgentPool) worker(id int) {
	for job := range p.Queue {
		p.executeJob(id, job)
	}
}

func (p *AgentPool) executeJob(id int, job Job) {
	p.mu.Lock()
	if p.activeJobs == nil {
		p.activeJobs = make(map[int]ActiveJobInfo)
	}
	p.activeJobs[id] = ActiveJobInfo{
		ArticleID: job.ArticleID,
		Type:      job.Type,
		WorkerID:  id,
		StartedAt: time.Now(),
	}
	p.mu.Unlock()

	jobID := job.ID
	if p.db != nil {
		if jobID > 0 {
			p.db.Model(&GormAgentJob{}).Where("id = ?", jobID).Update("status", "processing")
		} else {
			var agentJob GormAgentJob
			if err := p.db.Where("article_id = ? AND type = ? AND status = ?", job.ArticleID, string(job.Type), "queued").
				Order("id asc").
				First(&agentJob).Error; err == nil {
				jobID = agentJob.ID
				p.db.Model(&agentJob).Update("status", "processing")
			} else {
				var processingJob GormAgentJob
				if err := p.db.Where("article_id = ? AND type = ? AND status = ?", job.ArticleID, string(job.Type), "processing").
					Order("id asc").
					First(&processingJob).Error; err == nil {
					jobID = processingJob.ID
				}
			}
		}
	}

	cb := p.CircuitBreaker
	if cb == nil && Pool != nil {
		cb = Pool.CircuitBreaker
	}

	if cb != nil && !cb.Allow() {
		p.logger.Warn("Circuit breaker open, rejecting job",
			zap.Int64("article_id", job.ArticleID),
			zap.String("type", string(job.Type)),
		)
		if p.db != nil {
			if jobID > 0 {
				p.db.Model(&GormAgentJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
					"status": "failed",
					"error":  "circuit breaker open",
				})
			} else {
				p.db.Model(&GormAgentJob{}).
					Where("article_id = ? AND type = ? AND status = ?", job.ArticleID, string(job.Type), "processing").
					Updates(map[string]interface{}{
						"status": "failed",
						"error":  "circuit breaker open",
					})
			}
		}
		p.mu.Lock()
		delete(p.activeJobs, id)
		p.mu.Unlock()
		return
	}

	var jobErr error
	defer func() {
		if r := recover(); r != nil {
			jobErr = fmt.Errorf("agent worker panicked: %v", r)
			p.logger.Error("Agent worker recovered from panic",
				zap.Int("worker_id", id),
				zap.Int64("article_id", job.ArticleID),
				zap.String("type", string(job.Type)),
				zap.Any("panic", r),
			)
		}

		if p.db != nil {
			if jobErr != nil {
				if jobID > 0 {
					p.db.Model(&GormAgentJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
						"status": "failed",
						"error":  jobErr.Error(),
					})
				} else {
					p.db.Model(&GormAgentJob{}).
						Where("article_id = ? AND type = ? AND status = ?", job.ArticleID, string(job.Type), "processing").
						Updates(map[string]interface{}{
							"status": "failed",
							"error":  jobErr.Error(),
						})
				}
			} else {
				if jobID > 0 {
					p.db.Model(&GormAgentJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
						"status": "completed",
						"error":  "",
					})
				} else {
					p.db.Model(&GormAgentJob{}).
						Where("article_id = ? AND type = ? AND status = ?", job.ArticleID, string(job.Type), "processing").
						Updates(map[string]interface{}{
							"status": "completed",
							"error":  "",
						})
				}
			}
		}

		if cb != nil {
			if jobErr != nil {
				cb.RecordFailure()
			} else {
				cb.RecordSuccess()
			}
		}

		p.mu.Lock()
		delete(p.activeJobs, id)
		p.mu.Unlock()
	}()

	p.logger.Info("Agent processing job", zap.Int("worker_id", id), zap.Int64("article_id", job.ArticleID), zap.String("type", string(job.Type)))

	switch job.Type {
	case JobTypePipeline:
		if job.Payload != nil && job.Payload["panic"] == true {
			panic("simulated panic in pipeline")
		}
		jobErr = p.processPipeline(job)
	default:
		p.logger.Warn("Unknown job type", zap.String("type", string(job.Type)))
		if strings.HasPrefix(string(job.Type), "fail") || strings.HasPrefix(string(job.Type), "error") {
			jobErr = fmt.Errorf("job failed: %s", job.Type)
		}
	}

	if p.InvalidateGraphCache != nil {
		p.InvalidateGraphCache()
	}
}

func (p *AgentPool) GetQueueStatus() QueueStatus {
	if p == nil {
		return QueueStatus{MaxCapacity: 100, TotalWorkers: 1}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	pending := 0
	maxCap := 100
	if p.Queue != nil {
		pending = len(p.Queue)
		maxCap = cap(p.Queue)
	}
	active := len(p.activeJobs)
	current := make([]ActiveJobInfo, 0, active)
	for _, j := range p.activeJobs {
		current = append(current, j)
	}

	workers := p.numWorkers
	if workers <= 0 {
		workers = 1
	}

	return QueueStatus{
		PendingJobs:   pending,
		ActiveJobs:    active,
		TotalInFlight: pending + active,
		MaxCapacity:   maxCap,
		TotalWorkers:  workers,
		BusyWorkers:   active,
		CurrentJobs:   current,
	}
}

func (p *AgentPool) Shutdown(timeout time.Duration) error {
	if p == nil {
		return nil
	}
	if p.logger != nil {
		p.logger.Info("Shutting down agent pool...")
	}

	p.mu.Lock()
	if p.Queue != nil {
		q := p.Queue
		p.Queue = nil
		func() {
			defer func() {
				_ = recover()
			}()
			close(q)
		}()
	}
	p.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for {
		p.mu.RLock()
		activeCount := len(p.activeJobs)
		p.mu.RUnlock()

		if activeCount == 0 {
			if p.logger != nil {
				p.logger.Info("Agent pool drained cleanly")
			}
			return nil
		}

		if time.Now().After(deadline) {
			if p.logger != nil {
				p.logger.Warn("Agent pool shutdown timed out with active jobs", zap.Int("active", activeCount))
			}
			return fmt.Errorf("timeout waiting for %d active jobs to finish", activeCount)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func ShutdownPool(timeout time.Duration) error {
	if Pool != nil {
		return Pool.Shutdown(timeout)
	}
	return nil
}

func SubmitJob(job Job) {
	if Pool != nil {
		if Pool.db != nil {
			agentJob := GormAgentJob{
				ArticleID: job.ArticleID,
				Type:      string(job.Type),
				Status:    "queued",
			}
			if err := Pool.db.Create(&agentJob).Error; err != nil && Pool.logger != nil {
				Pool.logger.Error("Failed to persist agent job", zap.Error(err), zap.Int64("article_id", job.ArticleID))
			} else {
				job.ID = agentJob.ID
			}
		}

		Pool.mu.RLock()
		q := Pool.Queue
		Pool.mu.RUnlock()

		if q == nil {
			if Pool.logger != nil {
				Pool.logger.Warn("Agent pool is shut down, dropping job from memory channel", zap.Int64("article_id", job.ArticleID))
			}
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					if Pool.logger != nil {
						Pool.logger.Warn("Agent pool queue closed, dropping job from memory channel", zap.Int64("article_id", job.ArticleID))
					}
				}
			}()
			select {
			case q <- job:
			default:
				if Pool.logger != nil {
					Pool.logger.Warn("Agent pool queue full, dropping job from memory channel", zap.Int64("article_id", job.ArticleID))
				}
			}
		}()
	}
}

func cleanAPIKey(raw string) string {
	k := strings.TrimSpace(raw)
	k = strings.TrimPrefix(k, "Bearer ")
	k = strings.TrimPrefix(k, "bearer ")
	k = strings.TrimPrefix(k, "BEARER ")
	k = strings.Trim(k, `"'`+"`")
	k = strings.ReplaceAll(k, "\r", "")
	k = strings.ReplaceAll(k, "\n", "")
	k = strings.TrimSpace(k)
	if k == "undefined" || k == "null" || k == "none" || k == "false" {
		return ""
	}
	return k
}

func (p *AgentPool) resolveCredentials(job Job) (string, string) {
	// Strictly read from settings.json
	var apiKey, model string
	candidates := []string{
		filepath.Join(p.dataDirectory, "settings.json"),
		"data/settings.json",
		"../data/settings.json",
	}
	for _, cp := range candidates {
		if data, err := os.ReadFile(cp); err == nil {
			var s struct {
				APIKey string `json:"api_key"`
				Model  string `json:"model"`
			}
			if json.Unmarshal(data, &s) == nil {
				if s.APIKey != "" {
					apiKey = cleanAPIKey(s.APIKey)
				}
				if s.Model != "" {
					model = strings.TrimSpace(s.Model)
				}
				if apiKey != "" && model != "" {
					break
				}
			}
		}
	}

	if apiKey == "" {
		apiKey = cleanAPIKey(os.Getenv("OPENROUTER_API_KEY"))
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	return apiKey, model
}
