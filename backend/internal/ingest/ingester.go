package ingest

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type Ingester struct {
	fetcher    PageFetcher
	extractor  *ContentExtractor
	storage    FileStorage
	repo       ArticleRepository
	renderer   TemplateRenderer
	summarizer Summarizer
	idGen      func() int64
}

func NewIngester(
	fetcher PageFetcher,
	extractor *ContentExtractor,
	storage FileStorage,
	repo ArticleRepository,
) *Ingester {
	if repo == nil {
		panic("repo is required")
	}
	if extractor == nil {
		extractor = NewContentExtractor()
	}
	return &Ingester{
		fetcher:   fetcher,
		extractor: extractor,
		storage:   storage,
		repo:      repo,
		idGen:     func() int64 { return time.Now().UnixMilli() },
	}
}

func (ing *Ingester) SetIDGenerator(fn func() int64) {
	ing.idGen = fn
}

func (ing *Ingester) SetTemplateRenderer(r TemplateRenderer) {
	ing.renderer = r
}

func (ing *Ingester) SetSummarizer(s Summarizer) {
	ing.summarizer = s
}

func (ing *Ingester) Ingest(ctx context.Context, req IngestRequest) (*IngestedArticle, error) {
	trimmedURL := strings.TrimSpace(req.URL)
	if trimmedURL == "" {
		return nil, ErrEmptyURL
	}

	parsedURL, err := url.Parse(trimmedURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("%w: %s", ErrInvalidURL, trimmedURL)
	}

	existing, err := ing.repo.FindBySourceURL(ctx, trimmedURL)
	if err == nil && existing != nil {
		return existing, ErrDuplicateArticle
	}

	// 2. Fetch remote HTML content
	htmlBytes, err := ing.fetcher.FetchHTML(ctx, trimmedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch page failed: %w", err)
	}

	// 3. Extract readable article content and body images
	extracted, err := ing.extractor.Extract(htmlBytes, parsedURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrExtractionFailed, err.Error())
	}

	filenameID := ing.idGen()

	// 4. Concurrently download and localize body images
	replacements := ing.localizeImages(ctx, filenameID, extracted.BodyImages)

	markdownContent := extracted.MarkdownContent
	if len(replacements) > 0 {
		replacer := strings.NewReplacer(replacements...)
		markdownContent = replacer.Replace(markdownContent)
	}

	// 5. Download cover image if present
	coverImagePath := ""
	if extracted.CoverImageURL != "" {
		coverImagePath = ing.localizeSingleImage(ctx, filenameID, extracted.CoverImageURL, "cover")
	}

	// 6. Format Markdown Document (via TemplateRenderer or built-in default fallback)
	sanitizedTags := SanitizeObsidianTags(req.Tags)
	tagsString := strings.Join(sanitizedTags, ",")
	savedDate := time.UnixMilli(filenameID).UTC().Format("2006-01-02")

	var markdownDoc string
	renderedWithTemplate := false

	if ing.renderer != nil {
		tplPath := ing.renderer.ResolveTemplate(parsedURL.Hostname(), req.Template)
		if tplPath != "" {
			ctxData := TemplateContext{
				Title:       extracted.Title,
				Summary:     extracted.Description,
				Source:      trimmedURL,
				URL:         trimmedURL,
				Domain:      parsedURL.Hostname(),
				Content:     markdownContent,
				Tags:        sanitizedTags,
				TagsStr:     tagsString,
				CoverImage:  coverImagePath,
				SavedDate:   savedDate,
				Timestamp:   filenameID,
				Author:      extracted.Author,
				Description: extracted.Description,
				SiteName:    extracted.SiteName,
				OG:          extracted.OG,
			}
			rendered, err := ing.renderer.Render(ctx, tplPath, ctxData)
			if err == nil && strings.TrimSpace(rendered) != "" {
				markdownDoc = rendered
				renderedWithTemplate = true
			}
		}
	}

	if !renderedWithTemplate {
		// Built-in default formatting fallback
		frontmatter := fmt.Sprintf("---\ntitle: %q\nsource: %q\ntags: [%s]\ncover: %q\nsaved: %s\n---\n",
			extracted.Title,
			trimmedURL,
			tagsString,
			coverImagePath,
			savedDate,
		)
		markdownDoc = frontmatter + "\n" + markdownContent
	}

	// 7. Atomic write to markdown storage
	relFilePath, err := ing.storage.SaveMarkdownByTitle(extracted.Title, filenameID, []byte(markdownDoc))
	if err != nil {
		return nil, fmt.Errorf("save markdown failed: %w", err)
	}

	// 8. Persist to ArticleRepository
	article := &IngestedArticle{
		ID:        filenameID,
		Title:     extracted.Title,
		ImagePath: coverImagePath,
		FilePath:  relFilePath,
		Tags:      tagsString,
		SourceURL: trimmedURL,
	}

	if err := ing.repo.SaveArticle(ctx, article); err != nil {
		return nil, fmt.Errorf("save article to repository failed: %w", err)
	}

	return article, nil
}

func (ing *Ingester) localizeImages(ctx context.Context, filenameID int64, imgURLs []string) []string {
	if len(imgURLs) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	replacements := make([]string, 0, len(imgURLs)*2)
	usedFilenames := make(map[string]int)

	for _, imgURL := range imgURLs {
		wg.Add(1)
		go func(rawURL string) {
			defer wg.Done()

			data, err := ing.fetcher.FetchImage(ctx, rawURL)
			if err != nil || len(data) == 0 {
				return
			}

			// Generate safe unique filename per article
			cleanName := sanitizeImageFilename(rawURL)
			mu.Lock()
			usedFilenames[cleanName]++
			count := usedFilenames[cleanName]
			if count > 1 {
				ext := path.Ext(cleanName)
				base := strings.TrimSuffix(cleanName, ext)
				cleanName = fmt.Sprintf("%s_%d%s", base, count, ext)
			}
			mu.Unlock()

			savedRelPath, err := ing.storage.SaveImage(filenameID, cleanName, data)
			if err != nil {
				return
			}

			mu.Lock()
			replacements = append(replacements, rawURL, savedRelPath)
			mu.Unlock()
		}(imgURL)
	}

	wg.Wait()
	return replacements
}

func (ing *Ingester) localizeSingleImage(ctx context.Context, filenameID int64, imgURL, prefix string) string {
	data, err := ing.fetcher.FetchImage(ctx, imgURL)
	if err != nil || len(data) == 0 {
		return ""
	}

	cleanName := sanitizeImageFilename(imgURL)
	if prefix != "" && !strings.HasPrefix(cleanName, prefix) {
		cleanName = fmt.Sprintf("%s_%s", prefix, cleanName)
	}

	savedRelPath, err := ing.storage.SaveImage(filenameID, cleanName, data)
	if err != nil {
		return ""
	}

	return savedRelPath
}

func sanitizeImageFilename(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	var base string
	if err == nil && parsed.Path != "" {
		base = path.Base(parsed.Path)
	} else {
		parts := strings.Split(rawURL, "/")
		base = parts[len(parts)-1]
	}

	base = strings.Split(base, "?")[0]
	base = strings.Split(base, "#")[0]

	if base == "" || base == "/" || base == "." {
		base = "image.png"
	}
	if path.Ext(base) == "" {
		base += ".png"
	}
	return base
}
