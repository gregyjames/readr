package agents

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type JobType string

const (
	JobTypeEnrichFrontmatter JobType = "enrich_frontmatter"
	JobTypeAutoLinker        JobType = "auto_linker"
)

type Job struct {
	ArticleID int64
	Type      JobType
	Payload   map[string]interface{}
}

type AgentPool struct {
	Queue                chan Job
	logger               *zap.Logger
	db                   *gorm.DB
	dataDirectory        string
	InvalidateGraphCache func()
}

var Pool *AgentPool

func InitPool(logger *zap.Logger, db *gorm.DB, dataDir string, numWorkers int, invalidateGraphCache func()) {
	Pool = &AgentPool{
		Queue:                make(chan Job, 100),
		logger:               logger,
		db:                   db,
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
