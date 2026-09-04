package markdown_test

import (
	"strings"
	"testing"

	"example.com/backend/internal/markdown"
)

func TestSplitAndAssembleDocument(t *testing.T) {
	t.Run("document with valid frontmatter and body", func(t *testing.T) {
		raw := `---
title: My Test Document
tags:
    - go
    - testing
type: article
---

# Heading 1

This is the document body.
`
		doc, err := markdown.SplitDocument(raw)
		if err != nil {
			t.Fatalf("SplitDocument returned unexpected error: %v", err)
		}

		if doc.Frontmatter["title"] != "My Test Document" {
			t.Errorf("expected title 'My Test Document', got %v", doc.Frontmatter["title"])
		}
		if doc.Frontmatter["type"] != "article" {
			t.Errorf("expected type 'article', got %v", doc.Frontmatter["type"])
		}
		tags, ok := doc.Frontmatter["tags"].([]interface{})
		if !ok || len(tags) != 2 {
			t.Fatalf("expected 2 tags, got %v", doc.Frontmatter["tags"])
		}

		if !strings.Contains(doc.Body, "# Heading 1") {
			t.Errorf("expected body to contain '# Heading 1', got %q", doc.Body)
		}

		// Test AssembleDocument
		assembled, err := markdown.AssembleDocument(doc)
		if err != nil {
			t.Fatalf("AssembleDocument returned unexpected error: %v", err)
		}

		if !strings.HasPrefix(assembled, "---\n") {
			t.Errorf("expected assembled doc to start with '---\\n', got %q", assembled)
		}
		if !strings.Contains(assembled, "title: My Test Document") {
			t.Errorf("expected assembled doc to contain title, got %q", assembled)
		}
		if !strings.Contains(assembled, "# Heading 1") {
			t.Errorf("expected assembled doc to contain body heading, got %q", assembled)
		}
	})

	t.Run("document without frontmatter", func(t *testing.T) {
		raw := `# Simple Note

Just standard markdown text without YAML frontmatter.
`
		doc, err := markdown.SplitDocument(raw)
		if err != nil {
			t.Fatalf("SplitDocument returned unexpected error: %v", err)
		}

		if len(doc.Frontmatter) != 0 {
			t.Errorf("expected empty frontmatter map, got %v", doc.Frontmatter)
		}
		if doc.Body != raw {
			t.Errorf("expected body to match raw string, got %q", doc.Body)
		}

		assembled, err := markdown.AssembleDocument(doc)
		if err != nil {
			t.Fatalf("AssembleDocument returned unexpected error: %v", err)
		}
		if assembled != raw {
			t.Errorf("expected assembled string to match raw string, got %q", assembled)
		}
	})

	t.Run("document with empty frontmatter delimiters", func(t *testing.T) {
		raw := "---\n---\n\n# Body Only\n"
		doc, err := markdown.SplitDocument(raw)
		if err != nil {
			t.Fatalf("SplitDocument returned unexpected error: %v", err)
		}

		if !doc.HasFrontmatter {
			t.Errorf("expected HasFrontmatter to be true for empty delimiters")
		}

		assembled, err := markdown.AssembleDocument(doc)
		if err != nil {
			t.Fatalf("AssembleDocument returned unexpected error: %v", err)
		}

		if assembled != raw {
			t.Errorf("expected assembled empty frontmatter %q, got %q", raw, assembled)
		}
	})

	t.Run("document with invalid YAML frontmatter", func(t *testing.T) {
		raw := `---
title: [invalid yaml
---

Some body
`
		_, err := markdown.SplitDocument(raw)
		if err == nil {
			t.Fatal("expected SplitDocument to fail on invalid YAML, got nil error")
		}
	})
}

func TestInjectWikilinks_SkipsCodeBlocksAndInlineCode(t *testing.T) {
	replacements := map[string]string{
		"Golang": "[[Golang]]",
		"Python": "[[Python]]",
		"Docker": "[[Docker|Container Engine]]",
	}

	input := `# Programming Guide

Golang is great for backend microservices.

` + "```python" + `
# In Python, we write scripts:
def run_docker():
    print("Golang and Docker in code block should not be replaced")
` + "```" + `

Here is some inline code: ` + "`Golang` and `Docker`" + ` shouldn't change.
Also Python should be linked in standard prose.

Another code block:
` + "```" + `
Golang
` + "```" + `

End of document with Golang and Docker.
`

	result := markdown.InjectWikilinks(input, replacements)

	// Check prose replacements
	if !strings.Contains(result, "[[Golang]] is great for backend microservices.") {
		t.Errorf("expected prose 'Golang' to be replaced with '[[Golang]]', got:\n%s", result)
	}
	if !strings.Contains(result, "Also [[Python]] should be linked in standard prose.") {
		t.Errorf("expected prose 'Python' to be replaced with '[[Python]]', got:\n%s", result)
	}
	if !strings.Contains(result, "End of document with [[Golang]] and [[Docker|Container Engine]].") {
		t.Errorf("expected final line to contain both replaced links, got:\n%s", result)
	}

	// Verify code blocks are untouched
	if !strings.Contains(result, `print("Golang and Docker in code block should not be replaced")`) {
		t.Errorf("expected fenced code block content to remain untouched, got:\n%s", result)
	}
	if !strings.Contains(result, "```\nGolang\n```") {
		t.Errorf("expected unfenced/fenced code block without lang to remain untouched, got:\n%s", result)
	}

	// Verify inline code spans are untouched
	if !strings.Contains(result, "`Golang` and `Docker`") {
		t.Errorf("expected inline code spans to remain untouched, got:\n%s", result)
	}
}

func TestApplySummaryBlock(t *testing.T) {
	t.Run("add summary to body without existing summary", func(t *testing.T) {
		body := `# Title

This is the main article content.
`
		summary := "This article explains how to use markdown AST."
		expected := `> 💡 **Summary:** This article explains how to use markdown AST.

# Title

This is the main article content.
`
		result := markdown.ApplySummaryBlock(body, summary)
		if result != expected {
			t.Errorf("expected:\n%q\ngot:\n%q", expected, result)
		}
	})

	t.Run("replace existing summary in body", func(t *testing.T) {
		body := `> 💡 **Summary:** Old outdated summary.
> More summary lines.

# Title

Article body continues here.
`
		newSummary := "Updated fresh summary."
		result := markdown.ApplySummaryBlock(body, newSummary)

		if strings.Contains(result, "Old outdated summary") {
			t.Errorf("expected old summary to be removed, got:\n%s", result)
		}
		if !strings.HasPrefix(result, "> 💡 **Summary:** Updated fresh summary.\n\n# Title") {
			t.Errorf("expected updated summary at the top, got:\n%s", result)
		}
	})

	t.Run("remove summary when summary is empty", func(t *testing.T) {
		body := `> 💡 **Summary:** Old summary.

# Title

Article body.
`
		result := markdown.ApplySummaryBlock(body, "")
		if strings.Contains(result, "Summary:") {
			t.Errorf("expected summary block to be removed, got:\n%s", result)
		}
		if !strings.HasPrefix(result, "# Title") {
			t.Errorf("expected body without summary to start with '# Title', got:\n%s", result)
		}
	})

	t.Run("no change if empty summary and no existing summary", func(t *testing.T) {
		body := "# Title\n\nArticle body.\n"
		result := markdown.ApplySummaryBlock(body, "")
		if result != body {
			t.Errorf("expected %q, got %q", body, result)
		}
	})

	t.Run("preserves later blockquote containing Summary: in document body", func(t *testing.T) {
		body := `# Article Heading

Here is an introductory paragraph.

> **Summary:** This is a quoted summary inside the body text from another author.
> It should remain untouched in the body.

Final paragraph.
`
		newSummary := "Executive high level summary."
		result := markdown.ApplySummaryBlock(body, newSummary)

		if !strings.HasPrefix(result, "> 💡 **Summary:** Executive high level summary.\n\n# Article Heading") {
			t.Errorf("expected leading summary at the top, got:\n%s", result)
		}

		if !strings.Contains(result, "> **Summary:** This is a quoted summary inside the body text from another author.") {
			t.Errorf("expected inner body quote to remain intact, got:\n%s", result)
		}
	})
}

func TestInjectWikilinks_EdgeCases(t *testing.T) {
	t.Run("empty body and empty replacements", func(t *testing.T) {
		if res := markdown.InjectWikilinks("", map[string]string{"foo": "bar"}); res != "" {
			t.Errorf("expected empty string, got %q", res)
		}
		if res := markdown.InjectWikilinks("Hello world", nil); res != "Hello world" {
			t.Errorf("expected unchanged string, got %q", res)
		}
	})

	t.Run("special regex characters in replacement keys", func(t *testing.T) {
		input := "We use C++ and Node.js for backend systems."
		replacements := map[string]string{
			"C++":     "[[C++|C Plus Plus]]",
			"Node.js": "[[Node.js]]",
		}
		res := markdown.InjectWikilinks(input, replacements)
		expected := "We use [[C++|C Plus Plus]] and [[Node.js]] for backend systems."
		if res != expected {
			t.Errorf("expected %q, got %q", expected, res)
		}
	})

	t.Run("nested lists and blockquotes in markdown", func(t *testing.T) {
		input := `> Quoted text with Golang here.
>
> - Item 1: Golang
> - Item 2: ` + "`Golang in code`" + `
`
		replacements := map[string]string{
			"Golang": "[[Golang]]",
		}
		res := markdown.InjectWikilinks(input, replacements)
		if !strings.Contains(res, "> Quoted text with [[Golang]] here.") {
			t.Errorf("expected blockquote text replaced, got:\n%s", res)
		}
		if !strings.Contains(res, "> - Item 1: [[Golang]]") {
			t.Errorf("expected list item replaced, got:\n%s", res)
		}
		if !strings.Contains(res, "`Golang in code`") {
			t.Errorf("expected code in list item not replaced, got:\n%s", res)
		}
	})
}
