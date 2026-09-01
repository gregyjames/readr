package agents

import (
	"encoding/json"
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
}

var Pool *AgentPool

func InitPool(logger *zap.Logger, db *gorm.DB, repo repository.Repository, dataDir string, numWorkers int, invalidateGraphCache func()) {
	if repo == nil && db != nil {
		repo = repository.NewGormRepository(db)
	}

	Pool = &AgentPool{
		Queue:                make(chan Job, 100),
		logger:               logger,
		db:                   db,
		repo:                 repo,
		dataDirectory:        dataDir,
		InvalidateGraphCache: invalidateGraphCache,
		numWorkers:           numWorkers,
		activeJobs:           make(map[int]ActiveJobInfo),
	}

	for i := 0; i < numWorkers; i++ {
		go Pool.worker(i)
	}

	logger.Info("Background agent pool started", zap.Int("workers", numWorkers))
}

func (p *AgentPool) worker(id int) {
	for job := range p.Queue {
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

		p.logger.Info("Agent processing job", zap.Int("worker_id", id), zap.Int64("article_id", job.ArticleID), zap.String("type", string(job.Type)))

		switch job.Type {
		case JobTypePipeline:
			p.processPipeline(job)
		default:
			p.logger.Warn("Unknown job type", zap.String("type", string(job.Type)))
		}

		if p.InvalidateGraphCache != nil {
			p.InvalidateGraphCache()
		}

		p.mu.Lock()
		delete(p.activeJobs, id)
		p.mu.Unlock()
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

func SubmitJob(job Job) {
	if Pool != nil {
		Pool.Queue <- job
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
