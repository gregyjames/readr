package repository

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrNotFound = errors.New("record not found")
)

// IsMOCArticle returns true if the article is a Map of Content (MOC) hub note.
func IsMOCArticle(title, tags string) bool {
	lowerTitle := strings.ToLower(title)
	if strings.HasPrefix(lowerTitle, "moc - ") || strings.HasPrefix(lowerTitle, "moc:") || strings.HasPrefix(lowerTitle, "moc ") || strings.EqualFold(title, "moc") {
		return true
	}
	tagList := strings.Split(tags, ",")
	for _, t := range tagList {
		if strings.TrimSpace(strings.ToLower(t)) == "moc" {
			return true
		}
	}
	return false
}

type ArticleRecord struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ImagePath string `json:"image"`
	FilePath  string `json:"article"`
	Tags      string `json:"tags"`
	SourceURL string `json:"sourceUrl"`
}

type LinkRecord struct {
	ID       int64 `json:"id"`
	SourceID int64 `json:"sourceId"`
	TargetID int64 `json:"targetId"`
}

type Repository interface {
	FindBySourceURL(ctx context.Context, sourceURL string) (*ArticleRecord, error)
	FindByID(ctx context.Context, id int64) (*ArticleRecord, error)
	SaveArticle(ctx context.Context, a *ArticleRecord) error
	GetAllArticles(ctx context.Context) ([]ArticleRecord, error)
	GetAllLinks(ctx context.Context) ([]LinkRecord, error)
	CreateLink(ctx context.Context, sourceID, targetID int64) (*LinkRecord, error)
	DeleteArticle(ctx context.Context, id int64) error
	FindCandidates(ctx context.Context, excludeID int64, title string, body string, limit int) ([]ArticleRecord, error)
	GetDistinctTags(ctx context.Context) ([]string, error)
	UpdateArticleTags(ctx context.Context, id int64, tags string) error
	RecordPipelineMetric(ctx context.Context, metric *PipelineMetric) error
	GetPipelineDiagnostics(ctx context.Context, limit int) (*PipelineDiagnosticsSummary, []PipelineMetric, error)
}

