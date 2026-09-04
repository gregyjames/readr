# Unified Markdown Parsing Engine & Structured Document Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace fragile regex-based Markdown text manipulation and string-slicing across Readr with a robust Goldmark AST walker in the Go backend, a structured `MOCDocument` domain model, and spec-compliant YAML/code-fence-safe wikilink rendering in the frontend.

**Architecture:** 
1. Build a foundational `backend/internal/markdown` package using `github.com/yuin/goldmark` AST traversal to safely inject wikilinks, insert summary blocks, and split/assemble YAML frontmatter without corrupting code fences or inline spans.
2. Refactor `backend/internal/agents/moc_document.go` to provide a typed `MOCDocument` AST model for deterministic serialization, link reconciliation, and delta placement insertion.
3. Upgrade `frontend/src/utils/markdown.ts` with `js-yaml` and code-fence masking for safe wikilink HTML anchor rendering.

**Tech Stack:** Go 1.25, `github.com/yuin/goldmark`, `gopkg.in/yaml.v3`, Vue 3, TypeScript, `js-yaml`.

---

### Task 1: Backend `internal/markdown` Frontmatter & Goldmark AST Transformer

**Files:**
- Create: `backend/internal/markdown/frontmatter.go`
- Create: `backend/internal/markdown/transformer.go`
- Create: `backend/internal/markdown/transformer_test.go`

- [ ] **Step 1: Write failing unit tests for frontmatter and AST transformations**

```go
// backend/internal/markdown/transformer_test.go
package markdown

import (
	"strings"
	"testing"
)

func TestSplitAndAssembleDocument(t *testing.T) {
	raw := "---\ntitle: Sample Note\ntags:\n  - tech\n  - go\n---\n\n# Sample Note\n\nBody content."
	doc, err := SplitDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error splitting document: %v", err)
	}

	if doc.Frontmatter["title"] != "Sample Note" {
		t.Errorf("expected title 'Sample Note', got %v", doc.Frontmatter["title"])
	}

	assembled, err := AssembleDocument(doc)
	if err != nil {
		t.Fatalf("unexpected error assembling document: %v", err)
	}

	if !strings.Contains(assembled, "title: Sample Note") || !strings.Contains(assembled, "# Sample Note") {
		t.Errorf("assembled document missing expected components:\n%s", assembled)
	}
}

func TestInjectWikilinks_SkipsCodeBlocksAndInlineCode(t *testing.T) {
	input := `# Title

Here is a paragraph mentioning Distributed Systems and Artificial Intelligence.

` + "```python\n# Do not replace Distributed Systems in code block\nprint('Distributed Systems')\n```" + `

And an inline code reference to ` + "`Distributed Systems`" + ` should also stay untouched.

Another paragraph mentioning Distributed Systems here.
`

	replacements := map[string]string{
		"Distributed Systems": "[[Distributed Systems]]",
	}

	output := InjectWikilinks(input, replacements)

	// In code fence: must remain untouched
	if strings.Contains(output, "print('[[Distributed Systems]]')") {
		t.Errorf("wikilink injected inside fenced code block:\n%s", output)
	}

	// In inline code: must remain untouched
	if strings.Contains(output, "`[[Distributed Systems]]`") {
		t.Errorf("wikilink injected inside inline code span:\n%s", output)
	}

	// In prose: should be replaced
	if !strings.Contains(output, "mentioning [[Distributed Systems]] and") {
		t.Errorf("expected wikilink in prose paragraph, got:\n%s", output)
	}
}

func TestApplySummaryBlock(t *testing.T) {
	body := "# Note Title\n\nNote body text."
	summary := "Key insight about architecture."

	updated := ApplySummaryBlock(body, summary)
	expectedPrefix := "> 💡 **Summary:** Key insight about architecture.\n\n# Note Title"
	if !strings.HasPrefix(updated, expectedPrefix) {
		t.Errorf("expected summary block at top, got:\n%s", updated)
	}

	// Applying new summary replaces existing block
	updated2 := ApplySummaryBlock(updated, "Updated insight.")
	if strings.Count(updated2, "> 💡 **Summary:**") != 1 {
		t.Errorf("expected single summary block after re-application, got:\n%s", updated2)
	}
	if !strings.Contains(updated2, "Updated insight.") {
		t.Errorf("expected updated summary text, got:\n%s", updated2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v ./internal/markdown/...`
Expected: FAIL with undefined functions `SplitDocument`, `AssembleDocument`, `InjectWikilinks`, `ApplySummaryBlock`.

- [ ] **Step 3: Implement `frontmatter.go` and `transformer.go`**

```go
// backend/internal/markdown/frontmatter.go
package markdown

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Frontmatter map[string]interface{}
	RawYAML     string
	Body        string
}

func SplitDocument(raw string) (*Document, error) {
	trimmed := strings.TrimLeft(raw, "\r\n\t ")
	if !strings.HasPrefix(trimmed, "---") {
		return &Document{
			Frontmatter: make(map[string]interface{}),
			RawYAML:     "",
			Body:        raw,
		}, nil
	}

	parts := strings.SplitN(trimmed[3:], "---", 2)
	if len(parts) < 2 {
		return &Document{
			Frontmatter: make(map[string]interface{}),
			RawYAML:     "",
			Body:        raw,
		}, nil
	}

	yamlStr := strings.TrimSpace(parts[0])
	body := strings.TrimLeft(parts[1], "\r\n")

	fm := make(map[string]interface{})
	if yamlStr != "" {
		if err := yaml.Unmarshal([]byte(yamlStr), &fm); err != nil {
			return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
		}
	}

	return &Document{
		Frontmatter: fm,
		RawYAML:     yamlStr,
		Body:        body,
	}, nil
}

func AssembleDocument(doc *Document) (string, error) {
	if len(doc.Frontmatter) == 0 {
		return doc.Body, nil
	}

	yamlBytes, err := yaml.Marshal(doc.Frontmatter)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML frontmatter: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(string(yamlBytes))
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimLeft(doc.Body, "\r\n"))
	return sb.String(), nil
}
```

```go
// backend/internal/markdown/transformer.go
package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var summaryBlockRegex = regexp.MustCompile(`(?m)^>\s*(?:💡\s*)?(?:\*\*)?(?:AI\s+)?Summary:(?:\*\*)?.*(?:\n>.*)*`)

// ApplySummaryBlock injects or replaces the AI summary callout block at the top of the body.
func ApplySummaryBlock(body string, summary string) string {
	summaryText := strings.TrimSpace(summary)
	if summaryText == "" {
		return body
	}
	newSummaryBlock := fmt.Sprintf("> 💡 **Summary:** %s", summaryText)
	if summaryBlockRegex.MatchString(body) {
		return summaryBlockRegex.ReplaceAllLiteralString(body, newSummaryBlock)
	}
	return newSummaryBlock + "\n\n" + strings.TrimLeft(body, "\n")
}

type textSpan struct {
	start int
	end   int
}

// InjectWikilinks walks the AST and replaces target phrases with wikilinks strictly inside prose text.
func InjectWikilinks(body string, replacements map[string]string) string {
	if len(replacements) == 0 || strings.TrimSpace(body) == "" {
		return body
	}

	src := []byte(body)
	reader := text.NewReader(src)
	doc := goldmark.DefaultParser().Parse(reader)

	var allowedSpans []textSpan

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Skip all code fences, inline code, HTML, autolinks, and raw blocks
		switch n.Kind() {
		case ast.KindFencedCodeBlock, ast.KindCodeBlock, ast.KindCodeSpan, ast.KindHTMLBlock, ast.KindAutoLink:
			return ast.WalkSkipChildren, nil
		case ast.KindText:
			t := n.(*ast.Text)
			seg := t.Segment
			if seg.Len() > 0 {
				allowedSpans = append(allowedSpans, textSpan{start: seg.Start, end: seg.Stop})
			}
		case ast.KindString:
			s := n.(*ast.String)
			seg := s.Segment
			if seg.Len() > 0 {
				allowedSpans = append(allowedSpans, textSpan{start: seg.Start, end: seg.Stop})
			}
		}

		return ast.WalkContinue, nil
	})

	if len(allowedSpans) == 0 {
		return body
	}

	// Apply replacements sequentially from end to start to maintain index offsets
	var result strings.Builder
	lastIdx := 0

	for _, span := range allowedSpans {
		if span.start < lastIdx {
			continue
		}
		result.WriteString(string(src[lastIdx:span.start]))
		segText := string(src[span.start:span.end])

		for phrase, replacement := range replacements {
			if strings.Contains(segText, phrase) {
				segText = strings.Replace(segText, phrase, replacement, 1)
			}
		}

		result.WriteString(segText)
		lastIdx = span.end
	}

	if lastIdx < len(src) {
		result.WriteString(string(src[lastIdx:]))
	}

	return result.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v ./internal/markdown/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/markdown/
git commit -m "feat(markdown): add frontmatter parser and goldmark AST text transformer"
```

---

### Task 2: Integrate AST Transformer into Agents Pipeline (`transform.go`)

**Files:**
- Modify: `backend/internal/agents/transform.go`
- Test: `backend/internal/agents/pipeline_test.go`

- [ ] **Step 1: Update `transform.go` to use `internal/markdown` AST utilities**

Replace regex-based `applySummary` and `injectLinksIntoBody` with `markdown.ApplySummaryBlock` and `markdown.InjectWikilinks`.

- [ ] **Step 2: Run pipeline tests**

Run: `cd backend && go test -v ./internal/agents/... -run "TestProcessPipeline"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/agents/transform.go
git commit -m "refactor(agents): use markdown AST transformer for safe link and summary injection"
```

---

### Task 3: Structured `MOCDocument` Domain Model & AST Reconciliation (`moc_document.go`)

**Files:**
- Modify: `backend/internal/agents/moc_document.go`
- Create: `backend/internal/agents/moc_document_test.go`
- Modify: `backend/internal/agents/librarian.go`

- [ ] **Step 1: Write failing unit tests for `MOCDocument` parser and operations**

```go
// backend/internal/agents/moc_document_test.go
package agents

import (
	"strings"
	"testing"
)

func TestMOCDocument_ParseAndRoundtrip(t *testing.T) {
	raw := `---
type: moc
title: MOC - Distributed Systems
tags:
  - moc
  - distributed-systems
---

# MOC - Distributed Systems

## Executive Overview
High level synthesis of distributed systems.

## Curated Index

### Consensus & Raft
- [[Raft Consensus Algorithm]] - Core consensus paper.
- [[Paxos Made Simple]] - Classical consensus.

### Storage Engines
- [[LSM Trees]] - Write-optimized storage.

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*User custom note here.*
`

	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing MOC: %v", err)
	}

	if doc.Title != "MOC - Distributed Systems" {
		t.Errorf("expected title 'MOC - Distributed Systems', got %q", doc.Title)
	}
	if !doc.HasCustomUserNotes() {
		t.Errorf("expected custom user notes to be detected")
	}
	if len(doc.CuratedSections) != 2 {
		t.Fatalf("expected 2 curated sections, got %d", len(doc.CuratedSections))
	}

	serialized := doc.Serialize()
	if !strings.Contains(serialized, "[[Raft Consensus Algorithm]]") || !strings.Contains(serialized, "*User custom note here.*") {
		t.Errorf("roundtrip serialization missed content:\n%s", serialized)
	}
}

func TestMOCDocument_ReconcileLinks_PrunesStaleBulletsOnly(t *testing.T) {
	raw := `# MOC - Test

### Core
- [[Active Note]] - Kept.
- [[Deleted Note]] - Should be pruned.

## Notes & Synthesis
*Keep this text.*
`
	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pruned := doc.ReconcileLinks(map[string]bool{"Active Note": true, "active note": true})
	if !pruned {
		t.Errorf("expected pruned to be true")
	}

	out := doc.Serialize()
	if strings.Contains(out, "[[Deleted Note]]") {
		t.Errorf("expected Deleted Note to be pruned from output:\n%s", out)
	}
	if !strings.Contains(out, "[[Active Note]]") {
		t.Errorf("expected Active Note to remain in output:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v ./internal/agents/... -run "TestMOCDocument"`
Expected: FAIL

- [ ] **Step 3: Implement `ParseMOCDocument`, `Serialize`, `ReconcileLinks`, `ApplyDeltaPlacements` in `moc_document.go`**

Implement complete structured object model in `moc_document.go` while keeping backward-compatible wrapper functions (`assembleMOCMarkdown`, `ReconcileMOCLinks`, `applyDeltaPlacements`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test -v ./internal/agents/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/agents/moc_document.go backend/internal/agents/moc_document_test.go
git commit -m "feat(agents): implement structured MOCDocument object model and AST operations"
```

---

### Task 4: Frontend Spec-Compliant YAML Parsing & Code-Fence Safe Wikilinks (`markdown.ts`)

**Files:**
- Modify: `frontend/src/utils/markdown.ts`
- Modify: `frontend/src/utils/markdown.test.ts`

- [ ] **Step 1: Write failing frontend unit tests**

```typescript
// frontend/src/utils/markdown.test.ts
import { describe, it, expect } from 'vitest';
import { parseFrontmatter, replaceWikilinks } from './markdown';

describe('markdown utilities', () => {
  it('parses complex YAML frontmatter with js-yaml', () => {
    const raw = `---
title: "Advanced Distributed Systems"
tags: ["system-design", "consensus"]
metadata:
  nested: true
  score: 9.5
---

# Content`;

    const fm = parseFrontmatter(raw);
    expect(fm.title).toBe('Advanced Distributed Systems');
    expect(fm.tags).toEqual(['system-design', 'consensus']);
    expect(fm.metadata).toEqual({ nested: true, score: 9.5 });
  });

  it('preserves code blocks without converting wikilinks inside them', () => {
    const markdown = `# Title

Paragraph with [[Target Note|Label]].

\`\`\`python
# Example wikilink inside code
def test():
    return "[[Code Link]]"
\`\`\`

Inline code with \`[[Not A Link]]\` here.`;

    const html = replaceWikilinks(markdown);
    expect(html).toContain('<a class="wikilink');
    expect(html).toContain('data-target="Target Note"');
    expect(html).toContain('return "[[Code Link]]"');
    expect(html).toContain('`[[Not A Link]]`');
  });
});
```

- [ ] **Step 2: Update `frontend/src/utils/markdown.ts` with `js-yaml` and code-fence masking**

- [ ] **Step 3: Run frontend tests & build**

Run: `cd frontend && bun run build`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/utils/markdown.ts frontend/src/utils/markdown.test.ts frontend/package.json frontend/bun.lockb
git commit -m "feat(frontend): upgrade frontmatter parser with js-yaml and code-fence safe wikilinks"
```

---

### Task 5: End-to-End System Verification & Regression Suite

**Files:**
- Repository-wide verification

- [ ] **Step 1: Format and tidy codebase**

Run: `cd backend && go fmt ./... && go mod tidy`

- [ ] **Step 2: Run static analysis & linters**

Run: `cd backend && go vet ./...`

- [ ] **Step 3: Run full backend test suite with race detector**

Run: `cd backend && go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 4: Verify frontend production build**

Run: `cd frontend && bun run build`
Expected: PASS with zero TypeScript or bundling errors.

- [ ] **Step 5: Commit any formatting or lock adjustments**

```bash
git commit -a -m "chore: format and verify markdown parsing engine suite"
```
