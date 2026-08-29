# Architecture & Implementation Plan: Article Ingestion Pipeline (002-article-ingester)

## 1. Overview & Architectural Goals

The goal of this refactor is to deepen the **Article Ingestion** subsystem, transforming the procedural 140-line `/api/add` HTTP handler into a deep, cohesive domain module (`ArticleIngester`).

### Architectural Vocabulary & Alignment
- **Module Depth**: The interface is minimal (`Ingest(ctx, req) (*Article, error)`), while the implementation encapsulates remote network fetching, Readability DOM extraction, article-scoped asset localization, frontmatter composition, atomic disk writing, and repository persistence.
- **Seam**: Clear boundary between HTTP routing (Fiber) and business logic. Storage and network I/O sit behind interfaces (`ArticleRepository`, `PageFetcher`).
- **Locality**: Scraped image downloads, atomic temporary file swapping, and frontmatter generation reside in one cohesive package rather than leaking across routing handlers.
- **Leverage & Testability**: Ingestion logic can be tested 100% in unit tests using in-memory mocks without launching HTTP servers, hitting external networks, or touching SQLite files on disk.

---

## 2. Decided Constraints & Behavior

Based on the architecture grilling review:
1. **Duplicate Detection**: Ingestion checks for existing articles by URL. If found, returns the existing record (or duplicate error) to avoid polluting storage.
2. **Asset Scoping**: Image scraping is strictly constrained to the readable article body (`article.Content`) rather than crawling the full HTML document, preventing tracking pixels, ads, and navigation logos from being saved.
3. **Atomic File I/O**: Markdown documents and assets are written to `{id}.md.tmp` first and atomically renamed to prevent partial writes.
4. **Repository Abstraction**: Database interactions run through an `ArticleRepository` interface.

---

## 3. Package Structure

```
backend/
├── internal/
│   └── ingest/
│       ├── ingester.go       # Core ArticleIngester orchestrator & interface
│       ├── fetcher.go        # PageFetcher interface & default HTTP implementation
│       ├── extractor.go      # Readability content extraction & body image parsing
│       ├── asset_store.go    # Image downloader, rewriter & atomic disk storage
│       ├── repository.go     # ArticleRepository interface
│       └── ingester_test.go  # Pure unit tests with mocked fetcher/repo
├── main.go                   # Fiber routes wired to ArticleIngester
└── main_test.go
```

---

## 4. Key Interfaces & Contracts

```go
package ingest

import (
    "context"
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
    FetchImage(ctx context.Context, imgURL string) ([]byte, string, error)
}

type ArticleRepository interface {
    FindByURL(ctx context.Context, url string) (*IngestedArticle, error)
    Save(ctx context.Context, article *IngestedArticle) error
}

type FileStorage interface {
    SaveMarkdown(filenameID int64, content []byte) (string, error)
    SaveImage(filenameID int64, imageName string, data []byte) (string, error)
}
```

---

## 5. Implementation Steps

### Step 1: Define Ingestion Interfaces & Models (`backend/internal/ingest/`)
- Create `backend/internal/ingest/` directory.
- Define `IngestRequest`, `IngestedArticle`, `ArticleIngester`, `PageFetcher`, `ArticleRepository`, and `FileStorage`.

### Step 2: Implement Page Fetcher & Readability Content Extractor (`fetcher.go`, `extractor.go`)
- Implement default `HTTPFetcher` with timeout and user-agent.
- Implement `ExtractContent(htmlBytes, parsedURL)` using Readability.
- Extract `<img>` tags **only from the readable article body HTML** (not the outer page wrapper).

### Step 3: Implement Atomic Asset & Document Storage (`asset_store.go`)
- Create `DiskStorage` that handles:
  - Atomic write to `data/articles/{id}.md.tmp` -> `data/articles/{id}.md`.
  - Image directory creation `data/images/{id}/` and concurrent download of article body images.
  - Markdown image URL replacements to local paths (`/images/{id}/{filename}`).
  - Clean YAML frontmatter assembly (`title`, `source`, `tags`, `cover`, `saved`).

### Step 4: Implement Ingester Orchestrator (`ingester.go`)
- Connect `PageFetcher` -> `Extractor` -> `DiskStorage` -> `ArticleRepository`.
- Check `repo.FindByURL(ctx, req.URL)` before scraping; return existing or duplicate response if found.
- Assign Unix timestamp ID or unique ID.
- Produce final `IngestedArticle` and persist to DB via repository.

### Step 5: Implement GORM Adapter & Wire in `backend/main.go`
- Create GORM implementation of `ArticleRepository` in `main.go` (or `backend/internal/repository/`).
- Instantiate `ingester := ingest.NewIngester(...)` inside `setupApp()`.
- Refactor `/api/add` handler to simply:
  ```go
  api.Post("/add", func(c *fiber.Ctx) error {
      var body RequestBody
      if err := json.Unmarshal(c.Body(), &body); err != nil {
          return c.Status(400).SendString("Invalid JSON")
      }
      article, err := ingester.Ingest(c.Context(), ingest.IngestRequest{
          URL:  body.URL,
          Tags: body.Tags,
      })
      if err != nil {
          if errors.Is(err, ingest.ErrDuplicateArticle) {
              return c.Status(409).JSON(fiber.Map{"error": "Article already exists", "id": article.ID})
          }
          return c.Status(500).SendString("Failed to ingest article")
      }
      return c.JSON(fiber.Map{"status": "success", "message": "Article saved", "id": article.ID})
  })
  ```

### Step 6: Unit Testing & Verification
- Write comprehensive unit tests in `backend/internal/ingest/ingester_test.go` verifying:
  - Extraction with mock HTML without network access.
  - Image localization and body URL rewriting.
  - Duplicate detection.
  - YAML frontmatter format integrity.
- Run `go test ./...` in `backend/` to verify integration tests and regression tests pass.
