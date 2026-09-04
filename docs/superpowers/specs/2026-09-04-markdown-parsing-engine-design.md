# Unified Markdown Parsing Engine & Structured Document Model Design

## Overview
This design replaces fragile regex-based Markdown text manipulation and ad-hoc string slicing across Readr with a robust, AST-driven architecture. It encompasses:
1. **Backend Markdown Engine (`backend/internal/markdown`)**: Goldmark AST walker for fence-safe wikilink injection, summary block insertion, and YAML frontmatter management.
2. **Structured MOC Domain Model (`backend/internal/agents/moc_document.go`)**: Typed in-memory representation (`MOCDocument`) for Map of Content documents supporting deterministic link reconciliation, delta placements, and user notes protection.
3. **Frontend Markdown & Wikilink Robustness (`frontend/src/utils/markdown.ts`)**: Spec-compliant YAML frontmatter extraction via `js-yaml` and code-fence-protected wikilink rendering.

---

## 1. Backend Markdown AST Engine (`backend/internal/markdown`)

### Package Structure
```
backend/internal/markdown/
├── frontmatter.go       # YAML frontmatter splitting and assembly
├── transformer.go       # Goldmark AST walker for safe transformations
└── transformer_test.go  # Unit tests covering code fences, inline spans, and HTML
```

### Frontmatter Separation & Assembly
- `SplitDocument(raw string) (*Document, error)`: Parses leading `---` frontmatter block using `gopkg.in/yaml.v3`. Returns frontmatter map, raw YAML header, and remaining markdown body.
- `AssembleDocument(doc *Document) (string, error)`: Serializes YAML frontmatter with standardized `---` delimiters followed by the body content.

### AST-Safe Text Transformation
Using `github.com/yuin/goldmark` (a fast CommonMark compliant parser in Go):
- `InjectWikilinks(body string, replacements map[string]string) string`:
  - Parses `body` into a Goldmark AST.
  - Walks the AST node tree.
  - Skips `ast.KindFencedCodeBlock`, `ast.KindCodeBlock`, `ast.KindCodeSpan`, `ast.KindHTMLBlock`, and `ast.KindAutoLink`.
  - Performs phrase matching and replacement only on text segments inside valid prose paragraphs and list items.
- `ApplySummaryBlock(body string, summary string) string`:
  - Replaces existing `> 💡 **Summary:** ...` blockquote if present, or prepends at the top of the body before the first header.

---

## 2. Structured `MOCDocument` Domain Model (`backend/internal/agents/moc_document.go`)

### Data Model
```go
type MOCSectionItem struct {
    ArticleID   int64  `json:"article_id"`
    ContextNote string `json:"context_note"`
}

type MOCSection struct {
    Title string           `json:"title"`
    Items []MOCSectionItem `json:"items"`
}

type MOCDocument struct {
    Frontmatter      map[string]interface{}
    Title            string
    ExecutiveSummary string
    CuratedSections  []MOCSection
    UserNotesBody    string // Protected content below ## Notes & Synthesis
}
```

### Key Operations
1. `ParseMOCDocument(raw string) (*MOCDocument, error)`:
   - Extracts YAML frontmatter.
   - Extracts `# Title` and `## Executive Overview`.
   - Parses each `### Section Title` and its `- [[Wikilink]] - Note` items.
   - Isolates `## Notes & Synthesis` content verbatim.
2. `(doc *MOCDocument) Serialize() string`:
   - Emits standardized Obsidian frontmatter, top header, Executive Overview, Curated Index, and Notes & Synthesis.
3. `(doc *MOCDocument) ReconcileLinks(validTitles map[string]bool) (pruned bool)`:
   - Prunes items whose target wikilinks are no longer in `validTitles`.
4. `(doc *MOCDocument) ApplyDeltaPlacements(placements []MOCDeltaPlacement, infoMap map[int64]MOCArticleInfo)`:
   - Deduplicates and appends delta items to matching existing sections or creates new sections ahead of `## Notes & Synthesis`.
5. `(doc *MOCDocument) HasCustomUserNotes() bool`:
   - Returns true if `UserNotesBody` contains custom text beyond default placeholder and comments.

---

## 3. Frontend Markdown & Wikilink Robustness (`frontend/src/utils/markdown.ts`)

### YAML Frontmatter Parsing
- Integrate `js-yaml` in `parseFrontmatter(rawText: string)`:
  - Supports arbitrary YAML data types (nested objects, lists, booleans, dates, multiline strings).
  - Handles malformed frontmatter gracefully without runtime errors.

### Code-Fence Protected Wikilink Rendering
- Update `replaceWikilinks(content: string)`:
  - Masks multi-line code fences (```` ```...``` ````), inline code spans (`` `...` ``), and HTML tags with opaque unique placeholders (`__CODE_BLOCK_0__`).
  - Converts `[[Target|Label]]` and `[[Target]]` into `<a class="wikilink" data-target="Target">Label</a>`.
  - Restores masked code tokens before returning HTML.

---

## 4. Verification & Testing Plan

1. **Backend Markdown Unit Tests (`internal/markdown/transformer_test.go`)**:
   - Verify wikilinks are NEVER injected inside fenced code blocks or inline backticks.
   - Verify frontmatter extraction handles empty frontmatter, standard frontmatter, and malformed blocks.
2. **MOC Document Unit Tests (`internal/agents/moc_document_test.go`)**:
   - Verify parsing, round-trip serialization, link reconciliation, and delta placement insertion on sample MOCs.
   - Verify user manual notes in `## Notes & Synthesis` are 100% preserved.
3. **Frontend Tests (`frontend/src/utils/markdown.test.ts`)**:
   - Verify `js-yaml` parsing with complex nested YAML.
   - Verify `replaceWikilinks` does not replace `[[Note]]` inside code fences or inline backticks.
4. **Integration & Regressions**:
   - Run `go test -race ./...` across all backend packages.
   - Run `npm run build` / `bun run build` in `frontend/`.
