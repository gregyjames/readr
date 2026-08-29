package ingest

import (
	"context"
	"errors"
)

var (
	ErrDuplicateArticle = errors.New("article already exists")
	ErrEmptyURL         = errors.New("url cannot be empty")
	ErrInvalidURL       = errors.New("invalid url")
	ErrExtractionFailed = errors.New("failed to extract readable content")
)

type IngestRequest struct {
	URL  string
	Tags []string
}

type IngestedArticle struct {
	ID        int64
	Title     string
	ImagePath string
	FilePath  string
	Tags      string
	SourceURL string
}

type ArticleIngester interface {
	Ingest(ctx context.Context, req IngestRequest) (*IngestedArticle, error)
}

type PageFetcher interface {
	FetchHTML(ctx context.Context, rawURL string) ([]byte, error)
	FetchImage(ctx context.Context, imgURL string) ([]byte, error)
}

type ArticleRepository interface {
	FindBySourceURL(ctx context.Context, sourceURL string) (*IngestedArticle, error)
	Save(ctx context.Context, article *IngestedArticle) error
}

type FileStorage interface {
	SaveMarkdown(filenameID int64, content []byte) (string, error)
	SaveImage(filenameID int64, filename string, data []byte) (string, error)
	GetImagesDir(filenameID int64) string
}
