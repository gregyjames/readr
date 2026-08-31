package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type JobType string

const (
	JobTypeEnrichFrontmatter JobType = "enrich_frontmatter"
	JobTypeAutoLinker        JobType = "auto_linker"
	JobTypeSummarizer        JobType = "summarizer"
)

type Job struct {
	ArticleID int64
	Type      JobType
	Payload   map[string]interface{}
}

type openRouterRequest struct {
	Model    string        `json:"model"`
	Messages []interface{} `json:"messages"`
}

type AgentPool struct {
	Queue                chan Job
	logger               *zap.Logger
	db                   *gorm.DB
	repo                 repository.Repository
	dataDirectory        string
	InvalidateGraphCache func()
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
	}

	for i := 0; i < numWorkers; i++ {
		go Pool.worker(i)
	}
	
	logger.Info("Background agent pool started", zap.Int("workers", numWorkers))
}

func (p *AgentPool) worker(id int) {
	for job := range p.Queue {
		p.logger.Info("Agent processing job", zap.Int("worker_id", id), zap.Int64("article_id", job.ArticleID), zap.String("type", string(job.Type)))
		
		switch job.Type {
		case JobTypeEnrichFrontmatter:
			p.processEnrichFrontmatter(job)
		case JobTypeAutoLinker:
			p.processAutoLinker(job)
		case JobTypeSummarizer:
			p.processSummarizer(job)
		default:
			p.logger.Warn("Unknown job type", zap.String("type", string(job.Type)))
		}

		if p.InvalidateGraphCache != nil {
			p.InvalidateGraphCache()
		}
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
