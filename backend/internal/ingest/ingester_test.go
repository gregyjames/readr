package ingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"example.com/backend/internal/repository"
)

// MockPageFetcher
type mockPageFetcher struct {
	html     string
	htmlMap  map[string][]byte
	imageMap map[string][]byte
}

func (m *mockPageFetcher) FetchHTML(ctx context.Context, rawURL string) ([]byte, error) {
	if m.html != "" {
		return []byte(m.html), nil
	}
	if data, ok := m.htmlMap[rawURL]; ok {
		return data, nil
	}
	return nil, errors.New("404 not found")
}

func (m *mockPageFetcher) FetchImage(ctx context.Context, imgURL string) ([]byte, error) {
	if data, ok := m.imageMap[imgURL]; ok {
		return data, nil
	}
	return nil, errors.New("image not found")
}

// MockFileStorage
type mockFileStorage struct {
	mu           sync.Mutex
	markdownDocs map[int64][]byte
	images       map[string][]byte
	files        map[string][]byte
}

func newMockStorage() *mockFileStorage {
	return &mockFileStorage{
		markdownDocs: make(map[int64][]byte),
		images:       make(map[string][]byte),
		files:        make(map[string][]byte),
	}
}

func (m *mockFileStorage) SaveMarkdown(filenameID int64, content []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markdownDocs[filenameID] = content
	path := "/articles/test.md"
	m.files[path] = content
	return path, nil
}

func (m *mockFileStorage) SaveImage(filenameID int64, filename string, data []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := filename
	m.images[key] = data
	return "/images/123/" + filename, nil
}

func (m *mockFileStorage) GetImagesDir(filenameID int64) string {
	return "/tmp/images"
}

// MockArticleRepository
type mockArticleRepository struct {
	mu       sync.Mutex
	articles map[string]*IngestedArticle
}

func newMockRepository() *mockArticleRepository {
	return &mockArticleRepository{
		articles: make(map[string]*IngestedArticle),
	}
}

func (m *mockArticleRepository) FindBySourceURL(ctx context.Context, sourceURL string) (*IngestedArticle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.articles[sourceURL]; ok {
		return a, nil
	}
	return nil, errors.New("not found")
}

func (m *mockArticleRepository) FindByID(ctx context.Context, id int64) (*IngestedArticle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.articles {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockArticleRepository) SaveArticle(ctx context.Context, article *IngestedArticle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.articles[article.SourceURL] = article
	return nil
}

func (m *mockArticleRepository) GetAllArticles(ctx context.Context) ([]IngestedArticle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]IngestedArticle, 0, len(m.articles))
	for _, a := range m.articles {
		res = append(res, *a)
	}
	return res, nil
}

func (m *mockArticleRepository) GetAllLinks(ctx context.Context) ([]repository.LinkRecord, error) {
	return nil, nil
}

func (m *mockArticleRepository) CreateLink(ctx context.Context, sourceID, targetID int64) (*repository.LinkRecord, error) {
	return &repository.LinkRecord{SourceID: sourceID, TargetID: targetID}, nil
}

func (m *mockArticleRepository) DeleteArticle(ctx context.Context, id int64) error {
	return nil
}

func TestIngester_Success(t *testing.T) {
	sampleHTML := `<!DOCTYPE html>
<html>
<head><title>Architecture in Go</title></head>
<body>
  <nav><img src="https://example.com/logo.png" alt="Nav Logo" /></nav>
  <article>
    <h1>Architecture in Go</h1>
    <p>Go interfaces provide high leverage boundaries for modules.</p>
    <img src="https://example.com/diagram.png" alt="Diagram" />
  </article>
</body>
</html>`

	fetcher := &mockPageFetcher{
		htmlMap: map[string][]byte{
			"https://example.com/posts/architecture": []byte(sampleHTML),
		},
		imageMap: map[string][]byte{
			"https://example.com/diagram.png": []byte("fake-image-bytes"),
		},
	}
	storage := newMockStorage()
	repo := newMockRepository()

	ingester := NewIngester(fetcher, nil, storage, repo)
	ingester.SetIDGenerator(func() int64 { return 1700000000 })

	req := IngestRequest{
		URL:  "https://example.com/posts/architecture",
		Tags: []string{"golang", "architecture"},
	}

	article, err := ingester.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected ingest error: %v", err)
	}

	if article.Title != "Architecture in Go" {
		t.Errorf("expected title 'Architecture in Go', got %q", article.Title)
	}

	if article.ID != 1700000000 {
		t.Errorf("expected ID 1700000000, got %d", article.ID)
	}

	// Verify markdown content saved with frontmatter
	savedMD, ok := storage.markdownDocs[1700000000]
	if !ok {
		t.Fatalf("expected markdown document to be saved")
	}

	mdStr := string(savedMD)
	if !strings.Contains(mdStr, `title: "Architecture in Go"`) {
		t.Errorf("expected frontmatter title, got:\n%s", mdStr)
	}
	if !strings.Contains(mdStr, `tags: [golang,architecture]`) {
		t.Errorf("expected frontmatter tags, got:\n%s", mdStr)
	}
	if !strings.Contains(mdStr, `source: "https://example.com/posts/architecture"`) {
		t.Errorf("expected frontmatter source, got:\n%s", mdStr)
	}

	// Verify image localization: diagram.png was downloaded, logo.png (in nav) was NOT downloaded
	if _, ok := storage.images["diagram.png"]; !ok {
		t.Errorf("expected diagram.png to be localized")
	}
	if _, ok := storage.images["logo.png"]; ok {
		t.Errorf("logo.png in nav should NOT have been downloaded (scoped to article content)")
	}

	// Verify repo saved
	if _, err := repo.FindBySourceURL(context.Background(), "https://example.com/posts/architecture"); err != nil {
		t.Errorf("expected article to be persisted in repository")
	}
}

func TestIngester_DuplicateDetection(t *testing.T) {
	fetcher := &mockPageFetcher{
		htmlMap: map[string][]byte{
			"https://example.com/existing": []byte("<html><body><article><p>Hello</p></article></body></html>"),
		},
	}
	storage := newMockStorage()
	repo := newMockRepository()

	// Pre-populate repository with existing article
	repo.articles["https://example.com/existing"] = &IngestedArticle{
		ID:        999,
		Title:     "Existing Article",
		SourceURL: "https://example.com/existing",
	}

	ingester := NewIngester(fetcher, nil, storage, repo)

	req := IngestRequest{
		URL:  "https://example.com/existing",
		Tags: []string{"test"},
	}

	article, err := ingester.Ingest(context.Background(), req)
	if !errors.Is(err, ErrDuplicateArticle) {
		t.Fatalf("expected ErrDuplicateArticle, got error: %v", err)
	}
	if article == nil || article.ID != 999 {
		t.Errorf("expected existing article with ID 999, got %+v", article)
	}
}

func TestIngester_InvalidURL(t *testing.T) {
	ingester := NewIngester(&mockPageFetcher{}, nil, newMockStorage(), newMockRepository())

	_, err := ingester.Ingest(context.Background(), IngestRequest{URL: ""})
	if !errors.Is(err, ErrEmptyURL) {
		t.Errorf("expected ErrEmptyURL, got: %v", err)
	}

	_, err = ingester.Ingest(context.Background(), IngestRequest{URL: "not-a-url"})
	if !errors.Is(err, ErrInvalidURL) {
		t.Errorf("expected ErrInvalidURL, got: %v", err)
	}
}

func TestIngester_CustomTemplateRendering(t *testing.T) {
	tempDir := t.TempDir()
	templatesDir := filepath.Join(tempDir, "templates")
	os.MkdirAll(templatesDir, 0755)

	customTpl := `---
title: {{ title }}
source: {{ source }}
site: custom-site
tags: [{% for tag in tags %}"{{ tag }}"{% if not loop.last %}, {% endif %}{% endfor %}]
---

Custom Header: {{ title }}

{{ content }}
`
	os.WriteFile(filepath.Join(templatesDir, "mysite.com.jinja"), []byte(customTpl), 0644)

	fetcher := &mockPageFetcher{
		html: `<html><body><article><p>Hello from my site</p></article></body></html>`,
	}
	storage := newMockStorage()
	repo := newMockRepository()
	renderer := NewGonjaTemplateRenderer(templatesDir)

	ingester := NewIngester(fetcher, nil, storage, repo)
	ingester.SetTemplateRenderer(renderer)
	ingester.SetIDGenerator(func() int64 { return 1700000000 })

	req := IngestRequest{
		URL:  "https://mysite.com/article/1",
		Tags: []string{"test"},
	}

	article, err := ingester.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	savedMD := string(storage.files[article.FilePath])
	if !strings.Contains(savedMD, "site: custom-site") {
		t.Errorf("saved markdown missing custom template content, got:\n%s", savedMD)
	}
	if !strings.Contains(savedMD, "Custom Header:") {
		t.Errorf("saved markdown missing custom header, got:\n%s", savedMD)
	}
}

func TestIngester_TemplateSyntaxErrorFallsBackToDefault(t *testing.T) {
	tempDir := t.TempDir()
	templatesDir := filepath.Join(tempDir, "templates")
	os.MkdirAll(templatesDir, 0755)

	// Broken Jinja syntax
	os.WriteFile(filepath.Join(templatesDir, "broken.com.jinja"), []byte(`{% for tag in tags %}`), 0644)

	fetcher := &mockPageFetcher{
		html: `<html><body><article><p>Fallback content</p></article></body></html>`,
	}
	storage := newMockStorage()
	repo := newMockRepository()
	renderer := NewGonjaTemplateRenderer(templatesDir)

	ingester := NewIngester(fetcher, nil, storage, repo)
	ingester.SetTemplateRenderer(renderer)
	ingester.SetIDGenerator(func() int64 { return 1700000000 })

	req := IngestRequest{
		URL:  "https://broken.com/article",
		Tags: []string{"test"},
	}

	article, err := ingester.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest should succeed with fallback, got err: %v", err)
	}

	savedMD := string(storage.files[article.FilePath])
	if !strings.Contains(savedMD, "Fallback content") {
		t.Errorf("expected fallback content, got:\n%s", savedMD)
	}
}

type mockSummarizer struct {
	called bool
	result string
}

func (m *mockSummarizer) Summarize(ctx context.Context, title, body, apiKey, model string) (string, error) {
	m.called = true
	return m.result, nil
}

func TestIngester_SummaryInTemplate(t *testing.T) {
	tempDir := t.TempDir()
	templatesDir := filepath.Join(tempDir, "templates")
	os.MkdirAll(templatesDir, 0755)

	templateContent := `---
title: {{ title }}
---
{% if summary %}
Summary: {{ summary }}
{% endif %}
{{ content }}`

	os.WriteFile(filepath.Join(templatesDir, "summarysite.com.jinja"), []byte(templateContent), 0644)

	fetcher := &mockPageFetcher{
		html: `<html><head><meta name="description" content="Initial description"></head><body><article><p>Main body content</p></article></body></html>`,
	}
	storage := newMockStorage()
	repo := newMockRepository()
	renderer := NewGonjaTemplateRenderer(templatesDir)

	ingester := NewIngester(fetcher, nil, storage, repo)
	ingester.SetTemplateRenderer(renderer)

	req := IngestRequest{
		URL: "https://summarysite.com/article",
	}

	article, err := ingester.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	savedMD := string(storage.files[article.FilePath])
	if !strings.Contains(savedMD, "Summary: Initial description") {
		t.Errorf("expected saved markdown to contain initial description summary, got:\n%s", savedMD)
	}
}
