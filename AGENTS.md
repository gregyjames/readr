# Agent Guidelines & Repository Rules

## Verification Workflow
Before finalizing changes or reporting completion:
1. Run `make check` from the workspace root (runs format check, `go vet`, backend race tests, frontend tests, and typecheck).
2. Alternatively, run individual checks:
   - Backend: `cd backend && go fmt ./... && go mod tidy && go vet ./... && go test -race ./...`
   - Frontend: `cd frontend && bun test && bun x vue-tsc -b && bun run build`

## Architectural Conventions & Key Learnings

### 1. Database & SQLite Concurrency
- Always use WAL mode with a busy timeout (`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;`).
- Use bounded connection pooling (`max(4, runtime.NumCPU())` max open, `2` max idle) to allow concurrent readers while avoiding lock contention.
- `GormArticle` includes indexed fields: `Article` (path), `Title`, `SourceURL`, and `IsArchived`.
- `GormArticleLink` includes indexes on `SourceID`, `TargetID`, and a composite index on `(SourceID, TargetID)`.
- Always deduplicate `article_links` on startup via `handlers.DeduplicateArticleLinks(db, logger)` before running schema migrations.

### 2. Markdown, Frontmatter & Wikilinks
- Never write ad-hoc regexes or string slicing for frontmatter or wikilinks.
- Use `example.com/backend/internal/markdown` for all Markdown manipulation:
  - `markdown.SplitDocument(content)`: splits frontmatter and body, supporting CRLF and empty delimiters.
  - `markdown.AssembleDocument(doc)`: reconstructs document with YAML header.
  - `markdown.ExtractWikilinks(content)`: parses `[[Target]]` and `[[Target|Display]]` links.
  - `markdown.ExtractUniqueWikilinkTargets(content)`: returns deduplicated target titles.
  - `markdown.WikilinkRegex` & `markdown.FrontmatterRegex`: canonical regular expressions.

### 3. File Mutations & Vault Boundary
- Always write Markdown and settings files atomically using temporary files (`.tmp`) and `os.Rename` to prevent truncation during crashes or power loss.
- When editing articles via `POST /api/edit/:id`, synchronize updated frontmatter `title` and `tags` into `GormArticle` and `articles_fts`.
- Synchronize outgoing wikilinks using batched lookups (`WHERE LOWER(title) IN (?)`) and `CreateInBatches` with in-memory deduplication.

### 4. Background Agent Pool
- Worker goroutines in `AgentPool` must always execute jobs inside a `defer func() { recover() }()` block so unexpected panics do not crash the application process or leak active job slots.

### 5. Frontend & Development Tooling
- Bun is the primary package manager (`packageManager: "bun@latest"`, `bun.lock`). `dev.sh` and `Makefile` auto-detect Bun and fall back to npm.
- Sanitize all search excerpts rendered via `v-html` using `sanitizeSearchExcerpt` from `utils/markdown` (DOMPurify with allowed `<mark>` tags only).
- When modifying `Article.vue`, maintain modular separation across dedicated subcomponents (`ArticleProperties.vue`, `ArticleBacklinks.vue`, `ArticleInlineLinker.vue`, `ArticleResumeToast.vue`).
