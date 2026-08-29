package ingest

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"codeberg.org/readeck/go-readability"
	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

type ExtractedArticle struct {
	Title           string
	CoverImageURL   string
	MarkdownContent string
	BodyImages      []string
}

type ContentExtractor struct {
	converter *markdown.Converter
}

func NewContentExtractor() *ContentExtractor {
	return &ContentExtractor{
		converter: markdown.NewConverter("", true, &markdown.Options{}),
	}
}

func (e *ContentExtractor) Extract(htmlBytes []byte, sourceURL *url.URL) (*ExtractedArticle, error) {
	article, err := readability.FromReader(bytes.NewReader(htmlBytes), sourceURL)
	if err != nil {
		return nil, fmt.Errorf("readability extraction failed: %w", err)
	}

	// Extract images scoped strictly to article.Content (the readable body)
	bodyDoc, err := html.Parse(strings.NewReader(article.Content))
	var bodyImages []string
	if err == nil {
		bodyImages = extractImageSources(bodyDoc, sourceURL)
	}

	markdownContent, err := e.converter.ConvertString(article.Content)
	if err != nil {
		return nil, fmt.Errorf("html to markdown conversion failed: %w", err)
	}

	return &ExtractedArticle{
		Title:           article.Title,
		CoverImageURL:   article.Image,
		MarkdownContent: markdownContent,
		BodyImages:      bodyImages,
	}, nil
}

func extractImageSources(n *html.Node, baseURL *url.URL) []string {
	var images []string
	seen := make(map[string]bool)

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" && attr.Val != "" {
					resolvedURL := resolveURL(baseURL, attr.Val)
					if resolvedURL != "" && !seen[resolvedURL] {
						seen[resolvedURL] = true
						images = append(images, resolvedURL)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return images
}

func resolveURL(base *url.URL, ref string) string {
	refURL, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	if base == nil {
		return refURL.String()
	}
	return base.ResolveReference(refURL).String()
}
