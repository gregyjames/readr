package repository

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("record not found")
)

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
}

