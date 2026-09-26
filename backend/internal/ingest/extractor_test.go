package ingest

import (
	"bytes"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
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
	if result.OG["og:image"] != "https://example.com/cover.png" {
		t.Errorf("OG[og:image] = %q; want https://example.com/cover.png", result.OG["og:image"])
	}
}

func TestCleanMarkdownContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "collapses multiple consecutive newlines",
			input:    "Paragraph 1.\n\n\n\n\nParagraph 2.\n\n\nParagraph 3.",
			expected: "Paragraph 1.\n\nParagraph 2.\n\nParagraph 3.",
		},
		{
			name:     "removes empty links and tracking images",
			input:    "Read this []() and look at ![]() image.\n\nValid [link](https://example.com) and ![img](https://example.com/img.png).",
			expected: "Read this  and look at  image.\n\nValid [link](https://example.com) and ![img](https://example.com/img.png).",
		},
		{
			name: "strips boilerplate headings and orphaned bullets",
			input: `# Main Article Title

Some good content here.

## Share this article
- 
- * 
### Leave a comment

More good content.

## Related Stories
## Newsletter Signup
# Advertisement`,
			expected: `# Main Article Title

Some good content here.

- *

More good content.`,
		},
		{
			name: "strips boilerplate prose lines",
			input: `# Article

Content paragraph.

## Table of Contents

We may earn a commission from links on this page.
What do you think so far?
Was this helpful?

[Add as a preferred source on Google](https://www.google.com/preferences/source?q=test.com)
[Read Full Bio](https://example.com/bio)

Next paragraph.`,
			expected: `# Article

Content paragraph.

Next paragraph.`,
		},
		{
			name: "preserves code blocks and prose structure",
			input: `Here is a code snippet:

` + "```go" + `
package main

// []() and ![]() inside code block should be preserved
// ## Share this article
func main() {
    println("Hello world")
}
` + "```" + `

End of article.`,
			expected: `Here is a code snippet:

` + "```go" + `
package main

// []() and ![]() inside code block should be preserved
// ## Share this article
func main() {
    println("Hello world")
}
` + "```" + `

End of article.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := cleanMarkdownContent(tt.input)
			if actual != tt.expected {
				t.Errorf("cleanMarkdownContent() mismatch.\nGot:\n%q\nWant:\n%q", actual, tt.expected)
			}
		})
	}
}

func TestPruneHTMLTree(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>
    <header class="site-header">
        <nav>Site Nav Links</nav>
    </header>
    <div id="cookie-banner" class="consent-dialog">Please accept cookies!</div>
    <div class="newsletter-signup">Subscribe to newsletter</div>
    <div class="social-share-buttons">Share on Twitter</div>
    <div class="ad-slot ad-container">Buy stuff!</div>
    <article>
        <header>
            <h1>Real Article Title</h1>
            <p class="byline">By Jane Doe</p>
        </header>
        <p>This is paragraph 1 of the article.</p>
        <aside>Unrelated sidebar inside article</aside>
        <form action="/subscribe"><button>Subscribe</button></form>
        <p>This is paragraph 2 of the article.</p>
    </article>
    <footer>Site Footer Links</footer>
</body>
</html>`

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		t.Fatalf("failed to parse test HTML: %v", err)
	}

	prunedDoc := pruneHTMLTree(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, prunedDoc); err != nil {
		t.Fatalf("failed to render pruned HTML: %v", err)
	}
	out := buf.String()

	// Assert clutter removed
	clutterPatterns := []string{
		"Site Nav Links",
		"Please accept cookies!",
		"Subscribe to newsletter",
		"Share on Twitter",
		"Buy stuff!",
		"Unrelated sidebar",
		"Site Footer Links",
		"/subscribe",
	}
	for _, pattern := range clutterPatterns {
		if strings.Contains(out, pattern) {
			t.Errorf("pruned HTML still contains clutter: %q", pattern)
		}
	}

	// Assert article content and article header preserved
	requiredPatterns := []string{
		"Real Article Title",
		"By Jane Doe",
		"This is paragraph 1 of the article.",
		"This is paragraph 2 of the article.",
	}
	for _, pattern := range requiredPatterns {
		if !strings.Contains(out, pattern) {
			t.Errorf("pruned HTML missing expected content: %q", pattern)
		}
	}
}

func TestPruneHTMLTree_ProtectsArticleAndMain(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<body>
    <article class="advertisement-slot">
        <header>
            <h1>Article in Ad-named Container</h1>
        </header>
        <p>Paragraph inside article with junk class.</p>
    </article>
    <main class="banner">
        <p>Paragraph inside main with banner class.</p>
    </main>
    <section role="main" class="newsletter">
        <p>Paragraph inside role main with newsletter class.</p>
    </section>
    <div role="article" class="cookie-consent">
        <p>Paragraph inside role article with cookie class.</p>
    </div>
</body>
</html>`

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		t.Fatalf("failed to parse test HTML: %v", err)
	}

	prunedDoc := pruneHTMLTree(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, prunedDoc); err != nil {
		t.Fatalf("failed to render pruned HTML: %v", err)
	}
	out := buf.String()

	protectedStrings := []string{
		"Article in Ad-named Container",
		"Paragraph inside article with junk class.",
		"Paragraph inside main with banner class.",
		"Paragraph inside role main with newsletter class.",
		"Paragraph inside role article with cookie class.",
	}

	for _, s := range protectedStrings {
		if !strings.Contains(out, s) {
			t.Errorf("protected node content missing from pruned HTML: %q", s)
		}
	}
}

func TestExtract_ClutteredHTML_PrunesJunk(t *testing.T) {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Post Title</title>
</head>
<body>
    <div id="consent-banner">Cookie terms and conditions</div>
    <div class="newsletter-bar"><input placeholder="email"/></div>
    <article>
        <h1>Main Headline</h1>
        <p>This is clean article text that must be kept.</p>
        <div class="share-buttons"><a href="#">Share</a></div>
    </article>
</body>
</html>`

	u, _ := url.Parse("https://example.com/clean-test")
	extractor := NewContentExtractor()
	result, err := extractor.Extract([]byte(htmlContent), u)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if strings.Contains(result.MarkdownContent, "Cookie terms") {
		t.Errorf("Markdown contains cookie banner: %s", result.MarkdownContent)
	}
	if strings.Contains(result.MarkdownContent, "newsletter-bar") {
		t.Errorf("Markdown contains newsletter junk: %s", result.MarkdownContent)
	}
	if strings.Contains(result.MarkdownContent, "Share") {
		t.Errorf("Markdown contains share buttons: %s", result.MarkdownContent)
	}
	if !strings.Contains(result.MarkdownContent, "This is clean article text that must be kept.") {
		t.Errorf("Markdown missing article body: %s", result.MarkdownContent)
	}
}

func TestExtract_FallbackWhenPruningEmpties(t *testing.T) {
	// Case 1: Minimal HTML with bare divs
	htmlContent := `<div>Only text in bare divs without article tags</div>`
	u, _ := url.Parse("https://example.com/fallback-test")
	extractor := NewContentExtractor()
	result, err := extractor.Extract([]byte(htmlContent), u)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if !strings.Contains(result.MarkdownContent, "Only text in bare divs") {
		t.Errorf("Fallback extraction failed to retrieve content: %s", result.MarkdownContent)
	}

	// Case 2: Content only exists in a pruned element (e.g., ad container or aside), so pruning empties the tree
	prunedOnly := `<div class="ad-container"><p>Crucial text that only exists in pruned container that readability can recover.</p></div>`
	result2, err := extractor.Extract([]byte(prunedOnly), u)
	if err != nil {
		t.Fatalf("Fallback extract failed: %v", err)
	}
	if !strings.Contains(result2.MarkdownContent, "Crucial text that only exists in pruned container") {
		t.Errorf("Fallback extraction failed to retrieve unpruned content: %s", result2.MarkdownContent)
	}
}

func TestPruneHTMLTree_WholeTokenMatchingAndParagraphGuards(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<body>
    <div class="paywall-content-wrapper">
        <p>This is legitimate content wrapped in a paywall-content-wrapper class that should NOT be pruned as a substring match.</p>
    </div>
    <div id="comments-and-body-container">
        <p>This is legitimate content inside comments-and-body-container that should also be preserved.</p>
    </div>
    <div class="paywall">
        <p>Actual paywall overlay text.</p>
    </div>
    <div id="comments">
        <p>Actual user comments section.</p>
    </div>
</body>
</html>`

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		t.Fatalf("failed to parse test HTML: %v", err)
	}

	prunedDoc := pruneHTMLTree(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, prunedDoc); err != nil {
		t.Fatalf("failed to render pruned HTML: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "paywall-content-wrapper") {
		t.Errorf("wrapper with paywall substring was incorrectly pruned: %s", out)
	}
	if !strings.Contains(out, "comments-and-body-container") {
		t.Errorf("wrapper with comments substring was incorrectly pruned: %s", out)
	}
	if strings.Contains(out, "Actual paywall overlay text.") {
		t.Errorf("actual paywall was not pruned: %s", out)
	}
	if strings.Contains(out, "Actual user comments section.") {
		t.Errorf("actual comments was not pruned: %s", out)
	}
}
