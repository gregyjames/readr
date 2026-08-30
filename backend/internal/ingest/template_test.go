package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		{"github.com", "none", ""},
		{"github.com", "default", ""},
		{"github.com", "builtin", ""},
		{"github.com", "auto", "github.com.jinja"},
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

func TestRenderTemplate_ToJSONFilter(t *testing.T) {
	tempDir := t.TempDir()
	templateContent := `---
title: {{ title | tojson }}
source: {{ source | tojson }}
tags: {{ tags | tojson }}
cover: {{ cover_image | tojson }}
saved: {{ saved_date | tojson }}
type: github-repo
---

{{ content }}
`
	templatePath := filepath.Join(tempDir, "github.com.jinja")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	renderer := NewGonjaTemplateRenderer(tempDir)
	ctxData := TemplateContext{
		Title:      `Pongo2: Django "Template" Engine`,
		Source:     "https://github.com/flosch/pongo2",
		Tags:       []string{"go", "django:syntax"},
		CoverImage: "/images/cover.png",
		SavedDate:  "2026-08-30",
		Content:    "Sample content",
	}

	rendered, err := renderer.Render(context.Background(), templatePath, ctxData)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	if !strings.Contains(rendered, `title: "Pongo2: Django \"Template\" Engine"`) {
		t.Errorf("expected safely serialized title, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `tags: ["go","django:syntax"]`) && !strings.Contains(rendered, `tags: ["go", "django:syntax"]`) {
		t.Errorf("expected safely serialized tags, got:\n%s", rendered)
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

func TestRequiresSummary(t *testing.T) {
	tempDir := t.TempDir()
	withSummary := filepath.Join(tempDir, "with.jinja")
	withoutSummary := filepath.Join(tempDir, "without.jinja")

	os.WriteFile(withSummary, []byte("Hello {{ summary }}"), 0644)
	os.WriteFile(withoutSummary, []byte("Hello {{ title }}"), 0644)

	renderer := NewGonjaTemplateRenderer(tempDir)
	if !renderer.RequiresSummary(withSummary) {
		t.Errorf("expected RequiresSummary to be true for template with summary")
	}
	if renderer.RequiresSummary(withoutSummary) {
		t.Errorf("expected RequiresSummary to be false for template without summary")
	}
	if renderer.RequiresSummary("") {
		t.Errorf("expected RequiresSummary to be false for empty path")
	}
}
