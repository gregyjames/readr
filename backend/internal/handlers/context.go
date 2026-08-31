package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/chat"
	"example.com/backend/internal/graph"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ArticleFileFetcher implements chat.ArticleFetcher for accessing article files and links.
type ArticleFileFetcher struct {
	DataDir string
	DB      *gorm.DB
}

func (f *ArticleFileFetcher) GetMarkdownContent(ctx context.Context, id int64) (string, error) {
	if f.DB != nil {
		var a repository.GormArticle
		if err := f.DB.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&a).Error; err == nil && a.Article != "" {
			candidate := filepath.Join(f.DataDir, strings.TrimPrefix(a.Article, "/"))
			if content, err := os.ReadFile(candidate); err == nil {
				return string(content), nil
			}
		}
	}

	path := filepath.Join(f.DataDir, "articles", fmt.Sprintf("%d.md", id))
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (f *ArticleFileFetcher) GetLinkedArticles(ctx context.Context, id int64) ([]chat.Attachment, error) {
	if f.DB == nil {
		return nil, nil
	}
	var links []repository.GormArticleLink
	if err := f.DB.WithContext(ctx).Where("source_id = ? OR target_id = ?", id, id).Find(&links).Error; err != nil {
		return nil, err
	}

	linkedIDMap := make(map[int64]bool)
	for _, l := range links {
		if l.SourceID == id && l.TargetID != id {
			linkedIDMap[l.TargetID] = true
		} else if l.TargetID == id && l.SourceID != id {
			linkedIDMap[l.SourceID] = true
		}
	}

	var results []chat.Attachment
	for linkedID := range linkedIDMap {
		var art repository.GormArticle
		if err := f.DB.WithContext(ctx).Select("id, title").First(&art, linkedID).Error; err == nil {
			results = append(results, chat.Attachment{
				ID:    art.ID,
				Title: art.Title,
			})
		}
	}
	return results, nil
}

// HandlerContext encapsulates services and dependencies for HTTP handlers.
type HandlerContext struct {
	DB               *gorm.DB
	Logger           *zap.Logger
	DataDir          string
	SettingsStore    *SettingsStore
	Repo             *repository.GormRepository
	Ingester         *ingest.Ingester
	GraphEngine      *graph.GraphEngine
	ChatRepo         *chat.FileRepository
	ChatService      *chat.Service
	TemplateRenderer ingest.TemplateRenderer
	EventHub         *EventHub
	ArticleFetcher   *ArticleFileFetcher
}
