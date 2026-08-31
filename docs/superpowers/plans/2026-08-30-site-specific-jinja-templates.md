# Site-Specific Jinja Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable user-specified site-specific Jinja templates (e.g. `github.com.jinja`) stored in `data/templates/` to customize Markdown formatting on article ingestion, falling back to the built-in formatter if no template exists or if rendering fails.

**Architecture:** A `TemplateRenderer` component wrapping `github.com/nikolalohinski/gonja/v2` handles domain pattern matching (exact and parent domain resolution) and renders template context (standard fields + OpenGraph metadata). The `Ingester` invokes this renderer during article processing, while Fiber exposes `GET /api/templates` and `App.vue` adds template auto-detection and manual selection.

**Tech Stack:** Go 1.25, `github.com/nikolalohinski/gonja/v2`, GoFiber v2, Vue 3, TypeScript.

---

### Task 1: Add Gonja v2 Dependency & Create Template Engine Interface

**Files:**
- Modify: `backend/go.mod`
- Create: `backend/internal/ingest/template.go`
- Test: `backend/internal/ingest/template_test.go`

- [ ] **Step 1: Write the failing tests for domain matching and template rendering**

Create `backend/internal/ingest/template_test.go`:
```go
package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDomainMatching(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create sample templates
	if err := os.WriteFile(filepath.Join(tempDir, "github.com.jinja"), []byte("github template"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "blog.mycoolsite.org.jinja"), []byte("blog template"), 0644); err != nil {
		t.Fatal(err)
	}

	renderer := NewGonjaTemplateRenderer(tempDir)

	tests := []struct {
		hostname string
		override string
		wantFile string
	}{
		{"github.com", "", "github.com.jinja"},
		{"gist.github.com", "", "github.com.jinja"},
		{"raw.githubusercontent.com", "", ""},
		{"blog.mycoolsite.org", "", "blog.mycoolsite.org.jinja"},
		{"other.mycoolsite.org", "", ""},
		{"randomsite.com", "github.com", "github.com.jinja"},
	}

	for _, tt := range tests {
		got := renderer.ResolveTemplate(tt.hostname, tt.override)
		if tt.wantFile == "" {
			if got != "" {
				t.Errorf("ResolveTemplate(%q, %q) = %q; want empty", tt.hostname, tt.override, got)
			}
		} else {
			if filepath.Base(got) != tt.wantFile {
				t.Errorf("ResolveTemplate(%q, %q) = %q; want %q", tt.hostname, tt.override, filepath.Base(got), tt.wantFile)
			}
		}
	}
}

func TestRenderTemplate_Success(t *testing.T) {
	tempDir := t.TempDir()
	templateContent := `---
title: {{ title }}
domain: {{ domain }}
tags: [{% for tag in tags %}"{{ tag }}"{% if not loop.last %}, {% endif %}{% endfor %}]
---

# {{ title }}

{{ content }}
`
	templatePath := filepath.Join(tempDir, "example.com.jinja")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	renderer := NewGonjaTemplateRenderer(tempDir)
	ctxData := TemplateContext{
		Title:   "Test Title",
		Domain:  "example.com",
		Tags:    []string{"go", "web"},
		Content: "Hello world markdown",
	}

	rendered, err := renderer.Render(context.Background(), templatePath, ctxData)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	expected := `---
title: Test Title
domain: example.com
tags: ["go", "web"]
---

# Test Title

Hello world markdown
`
	if rendered != expected {
		t.Errorf("Render() got:\n%s\nwant:\n%s", rendered, expected)
	}
}

func TestListTemplates(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, "github.com.jinja"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tempDir, "ignored.txt"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tempDir, "x.com.jinja"), []byte(""), 0644)

	renderer := NewGonjaTemplateRenderer(tempDir)
	templates, err := renderer.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}

	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
}
```

- [ ] **Step 2: Add gonja v2 dependency to go.mod**

Run: `cd backend && go get github.com/nikolalohinski/gonja/v2`

- [ ] **Step 3: Implement TemplateRenderer in backend/internal/ingest/template.go**

Create `backend/internal/ingest/template.go`:
```go
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

type TemplateContext struct {
	Title       string            `json:"title"`
	Source      string            `json:"source"`
	URL         string            `json:"url"`
	Domain      string            `json:"domain"`
	Content     string            `json:"content"`
	Tags        []string          `json:"tags"`
	TagsStr     string            `json:"tags_str"`
	CoverImage  string            `json:"cover_image"`
	SavedDate   string            `json:"saved_date"`
	Timestamp   int64             `json:"timestamp"`
	Author      string            `json:"author"`
	Description string            `json:"description"`
	SiteName    string            `json:"site_name"`
	OG          map[string]string `json:"og"`
}

type TemplateInfo struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type TemplateRenderer interface {
	ResolveTemplate(hostname string, override string) string
	Render(ctx context.Context, templatePath string, data TemplateContext) (string, error)
	ListTemplates() ([]TemplateInfo, error)
}

type GonjaTemplateRenderer struct {
	templatesDir string
}

func NewGonjaTemplateRenderer(templatesDir string) *GonjaTemplateRenderer {
	if templatesDir != "" {
		_ = os.MkdirAll(templatesDir, 0755)
	}
	return &GonjaTemplateRenderer{
		templatesDir: templatesDir,
	}
}

func (r *GonjaTemplateRenderer) ResolveTemplate(hostname string, override string) string {
	if r.templatesDir == "" {
		return ""
	}

	// 1. Manual override
	if override != "" {
		overrideName := strings.TrimSuffix(override, ".jinja") + ".jinja"
		target := filepath.Join(r.templatesDir, overrideName)
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return target
		}
	}

	// 2. Domain matching with hierarchy (e.g. sub.domain.com -> domain.com)
	host := strings.ToLower(strings.TrimSpace(hostname))
	parts := strings.Split(host, ".")

	for i := 0; i < len(parts)-1; i++ {
		candidateDomain := strings.Join(parts[i:], ".")
		target := filepath.Join(r.templatesDir, candidateDomain+".jinja")
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return target
		}
	}

	return ""
}

func (r *GonjaTemplateRenderer) Render(ctx context.Context, templatePath string, data TemplateContext) (string, error) {
	tpl, err := gonja.FromFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	// gonja uses exec.NewContext(map[string]interface{})
	contextMap := exec.NewContext(map[string]interface{}{
		"title":       data.Title,
		"source":      data.Source,
		"url":         data.Source,
		"domain":      data.Domain,
		"content":     data.Content,
		"tags":        data.Tags,
		"tags_str":    data.TagsStr,
		"cover_image": data.CoverImage,
		"saved_date":  data.SavedDate,
		"timestamp":   data.Timestamp,
		"author":      data.Author,
		"description": data.Description,
		"site_name":   data.SiteName,
		"og":          data.OG,
	})

	out, err := tpl.Execute(contextMap)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}

	return out, nil
}

func (r *GonjaTemplateRenderer) ListTemplates() ([]TemplateInfo, error) {
	if r.templatesDir == "" {
		return []TemplateInfo{}, nil
	}

	entries, err := os.ReadDir(r.templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TemplateInfo{}, nil
		}
		return nil, err
	}

	var templates []TemplateInfo
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jinja") {
			name := strings.TrimSuffix(entry.Name(), ".jinja")
			templates = append(templates, TemplateInfo{
				Name:     name,
				Filename: entry.Name(),
			})
		}
	}

	return templates, nil
}
```

- [ ] **Step 4: Run template tests to verify they pass**

Run: `cd backend && go test -v ./internal/ingest -run "TestDomainMatching|TestRenderTemplate|TestListTemplates"`
Expected: PASS

- [ ] **Step 5: Commit Task 1**

```bash
git add backend/go.mod backend/go.sum backend/internal/ingest/template.go backend/internal/ingest/template_test.go
git commit -m "feat(ingest): add GonjaTemplateRenderer and domain resolution logic"
```

---

### Task 2: Enhance ContentExtractor with OpenGraph & Metadata Extraction

**Files:**
- Modify: `backend/internal/ingest/extractor.go`
- Modify: `backend/internal/ingest/types.go`
- Test: `backend/internal/ingest/extractor_test.go` (create or modify)

- [ ] **Step 1: Write test for OpenGraph and metadata extraction**

Create/Update `backend/internal/ingest/extractor_test.go`:
```go
package ingest

import (
	"net/url"
	"testing"
)

func TestExtract_OpenGraphMetadata(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Original Title</title>
    <meta name="author" content="Jane Doe">
    <meta name="description" content="A comprehensive guide to Go.">
    <meta property="og:site_name" content="Tech Blog">
    <meta property="og:title" content="OG Title">
    <meta property="og:description" content="OG Description">
    <meta property="og:image" content="https://example.com/cover.png">
</head>
<body>
    <article>
        <h1>Article Heading</h1>
        <p>Article body paragraph text.</p>
    </article>
</body>
</html>`

	u, _ := url.Parse("https://example.com/post")
	extractor := NewContentExtractor()
	result, err := extractor.Extract([]byte(html), u)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result.Author != "Jane Doe" {
		t.Errorf("Author = %q; want Jane Doe", result.Author)
	}
	if result.Description != "A comprehensive guide to Go." {
		t.Errorf("Description = %q; want A comprehensive guide to Go.", result.Description)
	}
	if result.SiteName != "Tech Blog" {
		t.Errorf("SiteName = %q; want Tech Blog", result.SiteName)
	}
	if result.OG["og:site_name"] != "Tech Blog" {
		t.Errorf("OG[og:site_name] = %q; want Tech Blog", result.OG["og:site_name"])
	}
}
```

- [ ] **Step 2: Update ExtractedContent struct and ContentExtractor implementation**

Update `backend/internal/ingest/types.go` ExtractedContent:
```go
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
```

Update `backend/internal/ingest/extractor.go` to parse `<meta>` tags from HTML bytes using `golang.org/x/net/html`:
```go
// Extract metadata from HTML head
func extractMetadata(htmlBytes []byte) (author, desc, siteName string, og map[string]string) {
	og = make(map[string]string)
	doc, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var name, prop, content string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "name":
					name = strings.ToLower(a.Val)
				case "property":
					prop = strings.ToLower(a.Val)
				case "content":
					content = a.Val
				}
			}
			if prop != "" && content != "" {
				og[prop] = content
			}
			if name == "author" && author == "" {
				author = content
			}
			if (name == "description" || prop == "og:description") && desc == "" {
				desc = content
			}
			if (name == "site_name" || prop == "og:site_name") && siteName == "" {
				siteName = content
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return
}
```

- [ ] **Step 3: Run extractor tests to verify they pass**

Run: `cd backend && go test -v ./internal/ingest -run "TestExtract_OpenGraphMetadata"`
Expected: PASS

- [ ] **Step 4: Commit Task 2**

```bash
git add backend/internal/ingest/types.go backend/internal/ingest/extractor.go backend/internal/ingest/extractor_test.go
git commit -m "feat(ingest): extract author, description, and OpenGraph metadata"
```

---

### Task 3: Integrate TemplateRenderer into Ingester Pipeline

**Files:**
- Modify: `backend/internal/ingest/ingester.go`
- Modify: `backend/internal/ingest/types.go`
- Test: `backend/internal/ingest/ingester_test.go`

- [ ] **Step 1: Write unit test for Ingester with custom template rendering**

Add to `backend/internal/ingest/ingester_test.go`:
```go
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
```

- [ ] **Step 2: Update IngestRequest and Ingester in backend/internal/ingest/**

Update `IngestRequest` in `backend/internal/ingest/types.go`:
```go
type IngestRequest struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
}
```

Update `Ingester` in `backend/internal/ingest/ingester.go`:
```go
type Ingester struct {
	fetcher   PageFetcher
	extractor *ContentExtractor
	storage   FileStorage
	repo      ArticleRepository
	renderer  TemplateRenderer
	idGen     func() int64
}

func (ing *Ingester) SetTemplateRenderer(r TemplateRenderer) {
	ing.renderer = r
}
```

In `Ingester.Ingest()`:
```go
	tagsString := strings.Join(req.Tags, ",")
	savedDate := time.UnixMilli(filenameID).UTC().Format("2006-01-02")

	var markdownDoc string
	renderedWithTemplate := false

	if ing.renderer != nil {
		tplPath := ing.renderer.ResolveTemplate(parsedURL.Hostname(), req.Template)
		if tplPath != "" {
			ctxData := TemplateContext{
				Title:       extracted.Title,
				Source:      trimmedURL,
				URL:         trimmedURL,
				Domain:      parsedURL.Hostname(),
				Content:     markdownContent,
				Tags:        req.Tags,
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
```

- [ ] **Step 3: Run Ingester tests to verify they pass**

Run: `cd backend && go test -v ./internal/ingest -run "TestIngester_"`
Expected: PASS

- [ ] **Step 4: Commit Task 3**

```bash
git add backend/internal/ingest/ingester.go backend/internal/ingest/types.go backend/internal/ingest/ingester_test.go
git commit -m "feat(ingest): connect TemplateRenderer to Ingester with fallback"
```

---

### Task 4: Expose Template Endpoints & Wire to Backend Server

**Files:**
- Modify: `backend/main.go`
- Test: `backend/main_test.go`

- [ ] **Step 1: Write integration tests for template API in backend/main_test.go**

Add to `backend/main_test.go`:
```go
func TestGetTemplatesEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	templatesDir := filepath.Join(tempDir, "templates")
	os.MkdirAll(templatesDir, 0755)
	os.WriteFile(filepath.Join(templatesDir, "github.com.jinja"), []byte("content"), 0644)

	t.Setenv("DATA_DIR", tempDir)
	db := initTestDB()
	app := setupApp(db)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/templates", nil))
	if err != nil {
		t.Fatalf("GET /api/templates failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var templates []struct {
		Name     string `json:"name"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(body, &templates); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != "github.com" {
		t.Errorf("Expected [github.com], got %+v", templates)
	}
}
```

- [ ] **Step 2: Update backend/main.go to initialize renderer and register routes**

1. In `setupApp`:
```go
templatesDir := filepath.Join(dataDirectory, "templates")
templateRenderer := ingest.NewGonjaTemplateRenderer(templatesDir)
ingester.SetTemplateRenderer(templateRenderer)
```

2. Add `GET /api/templates` route:
```go
api.Get("/templates", func(c *fiber.Ctx) error {
	templates, err := templateRenderer.ListTemplates()
	if err != nil {
		logger.Error("Failed to list templates", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list templates"})
	}
	return c.JSON(templates)
})
```

3. Update `RequestBody` struct in `main.go`:
```go
type RequestBody struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
}
```
And pass `Template: body.Template` in `api.Post("/add", ...)` call to `ingester.Ingest`.

- [ ] **Step 3: Run backend integration tests**

Run: `cd backend && go test -v -run "TestGetTemplatesEndpoint"`
Expected: PASS

- [ ] **Step 4: Commit Task 4**

```bash
git add backend/main.go backend/main_test.go
git commit -m "feat(api): add /api/templates endpoint and wire template parameter"
```

---

### Task 5: Frontend Template Auto-Detection & Selection in Ingest Modal

**Files:**
- Modify: `frontend/src/App.vue`

- [ ] **Step 1: Add template state and auto-matching in App.vue**

In `<script setup lang="ts">` of `frontend/src/App.vue`:
1. Define `interface TemplateInfo { name: string; filename: string }`
2. Define `const availableTemplates = ref<TemplateInfo[]>([])` and `const selectedTemplate = ref<string>('auto')`
3. Fetch templates on mount / modal open:
```ts
const fetchTemplates = async () => {
  try {
    const res = await axios.get('/api/templates')
    availableTemplates.value = res.data || []
  } catch (err) {
    console.error('Failed to fetch templates', err)
  }
}
```
4. Compute detected template for active URL:
```ts
const matchedTemplate = computed(() => {
  if (!url.value) return null
  try {
    const host = new URL(url.value).hostname.toLowerCase()
    const parts = host.split('.')
    for (let i = 0; i < parts.length - 1; i++) {
      const candidate = parts.slice(i).join('.')
      const found = availableTemplates.value.find(t => t.name === candidate)
      if (found) return found
    }
  } catch {
    return null
  }
  return null
})
```
5. Pass `template` in `submitForm()` payload:
```ts
body: JSON.stringify({
  url: url.value,
  Tags: tags.value,
  template: selectedTemplate.value === 'auto' ? (matchedTemplate.value?.name || '') : selectedTemplate.value
})
```

- [ ] **Step 2: Add template dropdown in Ingest Modal template section**

In `<template>` of `frontend/src/App.vue`, inside the Add Article modal below Tags input:
```vue
<div v-if="availableTemplates.length > 0" class="mt-4">
  <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1.5">
    Markdown Template
  </label>
  <select
    v-model="selectedTemplate"
    class="w-full px-3 py-2 text-sm rounded-lg border border-gray-200 dark:border-white/10 bg-white dark:bg-black/40 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-emerald-500"
  >
    <option value="auto">
      Auto {{ matchedTemplate ? `(${matchedTemplate.name})` : '(Default)' }}
    </option>
    <option v-for="tpl in availableTemplates" :key="tpl.name" :value="tpl.name">
      {{ tpl.name }}
    </option>
    <option value="none">Built-in Default</option>
  </select>
</div>
```

- [ ] **Step 3: Verify frontend build compiles cleanly**

Run: `cd frontend && npx vite build`
Expected: PASS with no errors

- [ ] **Step 4: Commit Task 5**

```bash
git add frontend/src/App.vue
git commit -m "feat(ui): add template auto-detection and selection to ingest modal"
```

---

### Task 6: End-to-End Verification & Documentation

**Files:**
- Create: `backend/data/templates/github.com.jinja` (example sample)
- Modify: `README.md`

- [ ] **Step 1: Create sample github.com.jinja template**

Create `backend/data/templates/github.com.jinja`:
```jinja
---
title: {{ title }}
source: {{ source }}
tags: [{% for tag in tags %}"{{ tag }}"{% if not loop.last %}, {% endif %}{% endfor %}]
cover: {{ cover_image }}
saved: {{ saved_date }}
type: github-repo
---

# [{{ title }}]({{ source }})
{% if description %}
> {{ description }}
{% endif %}

---

{{ content }}
```

- [ ] **Step 2: Run full backend and frontend test suites**

Run: `cd backend && go test -v ./...`
Run: `cd frontend && npx vite build`
Expected: All suites pass

- [ ] **Step 3: Update documentation in README.md**

Document `$DATA_DIR/templates/` site-specific Jinja template customization and context variables in `README.md`.

- [ ] **Step 4: Commit Task 6**

```bash
git add backend/data/templates/github.com.jinja README.md
git commit -m "docs: add sample github.com template and document jinja templating"
```
