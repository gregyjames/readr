package ingest

import (
	"context"
	"errors"

	"example.com/backend/internal/repository"
)

var (
	ErrDuplicateArticle = errors.New("article already exists")
	ErrEmptyURL         = errors.New("url cannot be empty")
	ErrInvalidURL       = errors.New("invalid url")
	ErrExtractionFailed = errors.New("failed to extract readable content")
)

type IngestRequest struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
}

type IngestedArticle = repository.ArticleRecord
type ArticleRepository = repository.Repository

type ExtractedContent struct {
	Title           string
	MarkdownContent string
	CoverImageURL   string
	BodyImages      []string
	Author          string
	Description     string
	SiteName        string
	OG              map[string]string
}

type ExtractedArticle = ExtractedContent

type ArticleIngester interface {
	Ingest(ctx context.Context, req IngestRequest) (*IngestedArticle, error)
}

type PageFetcher interface {
	FetchHTML(ctx context.Context, rawURL string) ([]byte, error)
	FetchImage(ctx context.Context, imgURL string) ([]byte, error)
}

type FileStorage interface {
	SaveMarkdown(filenameID int64, content []byte) (string, error)
	SaveImage(filenameID int64, filename string, data []byte) (string, error)
	GetImagesDir(filenameID int64) string
}
