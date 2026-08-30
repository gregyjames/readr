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
	if result.OG["og:image"] != "https://example.com/cover.png" {
		t.Errorf("OG[og:image] = %q; want https://example.com/cover.png", result.OG["og:image"])
	}
}
