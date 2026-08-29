# Architecture & Implementation Plan: Article Document Pipeline & Local Graph Seam (004-frontend-document-pipeline)

## 1. Overview & Architectural Goals

The goal of this refactor is to decompose the 550-line `frontend/src/components/Article.vue` monolith into cohesive, single-responsibility modules and extract a backend endpoint `GET /api/graph/local/:id` using the newly built `GraphEngine`.

### Architectural Alignment
- **Module Depth & Locality**:
  - `frontend/src/utils/markdown.ts`: Pure functions for YAML frontmatter parsing, frontmatter stripping, and `[[wikilink]]` transformation.
  - `frontend/src/composables/useArticleDocument.ts`: Pure document state, fetching, frontmatter extraction, saving edits, and prose rendering.
  - `frontend/src/components/LocalGraph.vue`: Self-contained Vis-Network canvas, dark mode observer, and graph navigation for local subgraphs.
  - `frontend/src/components/ArticleLinkerModal.vue`: Floating text-selection listener, target article search, and `/api/link` mutation trigger.
  - `frontend/src/components/Article.vue`: Clean ~140-line editorial view orchestrating the layout, provenance masthead, prose reader, edit modal, and local graph footer.
- **Backend Graph Seam**: Exposes `GET /api/graph/local/:id` calling `graphEngine.BuildLocalGraph(ctx, articleID, 1)`.

---

## 2. Package & Component Structure

```
frontend/src/
├── utils/
│   ├── markdown.ts             # Pure YAML frontmatter + [[wikilink]] parser & tests
│   └── markdown.test.ts        # Unit tests
├── composables/
│   └── useArticleDocument.ts   # Document fetching, caching, editing & metadata state
└── components/
    ├── Article.vue             # Clean Editorial Prose Reader & layout
    ├── LocalGraph.vue          # Encapsulated local connection network canvas
    └── ArticleLinkerModal.vue  # Text selection linker popup & search modal

backend/
└── main.go                     # Exposes GET /api/graph/local/:id via GraphEngine
```

---

## 3. Implementation Steps

1. **Step 1: Backend Local Subgraph Route (`backend/main.go`)**
   - Add `api.Get("/graph/local/:id", ...)` delegating to `graphEngine.BuildLocalGraph(c.Context(), id, 1)`.
   - Verify with backend tests.

2. **Step 2: Markdown Utilities (`frontend/src/utils/markdown.ts` & `markdown.test.ts`)**
   - Implement `parseFrontmatter(rawMarkdown)` -> `{ properties, content }`.
   - Implement `replaceWikilinks(content, router)` -> HTML link strings `<a class="wikilink" ...>`.
   - Implement `getHostname(rawUrl)` -> clean hostname string.
   - Add pure unit tests verifying frontmatter parsing and wikilink conversion.

3. **Step 3: Document Composable (`frontend/src/composables/useArticleDocument.ts`)**
   - Encapsulates `article`, `properties`, `renderedContent`, `loading`, `error`, `editContent`, `fetchArticle(id)`, and `saveEdit(content)`.

4. **Step 4: Local Graph Component (`frontend/src/components/LocalGraph.vue`)**
   - Encapsulates Vis-Network container, dark mode `MutationObserver`, fetching `/api/graph/local/:id`, node click navigation, and zero parent state leakage.

5. **Step 5: Article Linker Modal (`frontend/src/components/ArticleLinkerModal.vue`)**
   - Encapsulates DOM selection tracking (`mouseup`), popup button positioning (`top`, `left`), article search autocomplete, and `/api/link` submission.

6. **Step 6: Refactor `frontend/src/components/Article.vue`**
   - Connect `useArticleDocument`, `LocalGraph.vue`, and `ArticleLinkerModal.vue`.
   - Render the Editorial Provenance Masthead, `<h1>` title, markdown body, edit mode, and local graph footer.

7. **Step 7: Verification**
   - Run `bun run build` and backend tests `go test ./...`.
