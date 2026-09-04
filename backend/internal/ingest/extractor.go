package ingest

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"codeberg.org/readeck/go-readability"
	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

type ContentExtractor struct {
	converter *markdown.Converter
}

func NewContentExtractor() *ContentExtractor {
	return &ContentExtractor{
		converter: markdown.NewConverter("", true, &markdown.Options{}),
	}
}

func (e *ContentExtractor) Extract(htmlBytes []byte, sourceURL *url.URL) (*ExtractedContent, error) {
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

	// Post-process and sanitize markdown content
	markdownContent = cleanMarkdownContent(markdownContent)

	fullDoc, _ := html.Parse(bytes.NewReader(htmlBytes))
	metaInfo := extractHTMLMetadata(fullDoc)

	author := metaInfo.author
	if author == "" {
		author = article.Byline
	}

	description := metaInfo.description
	if description == "" {
		description = article.Excerpt
	}

	siteName := metaInfo.siteName
	if siteName == "" {
		siteName = article.SiteName
	}

	coverImage := article.Image
	if coverImage == "" {
		coverImage = metaInfo.og["og:image"]
	}
	if coverImage != "" {
		coverImage = resolveURL(sourceURL, coverImage)
	}

	title := article.Title
	if title == "" {
		title = metaInfo.og["og:title"]
	}

	return &ExtractedContent{
		Title:           title,
		CoverImageURL:   coverImage,
		MarkdownContent: markdownContent,
		BodyImages:      bodyImages,
		Author:          author,
		Description:     description,
		SiteName:        siteName,
		OG:              metaInfo.og,
	}, nil
}

var (
	reMultipleNewlines    = regexp.MustCompile(`\n{3,}`)
	reEmptyLinks          = regexp.MustCompile(`\[\s*\]\([^\)]*\)`)
	reEmptyImages         = regexp.MustCompile(`!\[\s*\]\(\s*\)`)
	reOrphanedBullets     = regexp.MustCompile(`(?m)^[-*+]\s*$`)
	reBoilerplateHeadings = regexp.MustCompile(`(?i)^#{1,6}\s*(share\s+this(\s+article|\s+story|\s+post)?|share\s+on\s+\w+|newsletter(\s+signup)?|subscribe(\s+to\s+our\s+newsletter)?|leave\s+a\s+(reply|comment)|comments?|related\s+(articles?|posts?|stories)|advertisement|trending\s+now)\s*$`)
	reCodeFence           = regexp.MustCompile("(?s)(```.*?```|~~~.*?~~~)")
)

// cleanMarkdownSegment cleans non-code markdown prose
func cleanMarkdownSegment(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	// 1. Remove empty tracking images and empty links
	cleaned := reEmptyImages.ReplaceAllString(raw, "")
	cleaned = reEmptyLinks.ReplaceAllString(cleaned, "")

	// 2. Remove orphaned bullet points and boilerplate headings
	lines := strings.Split(cleaned, "\n")
	filteredLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check boilerplate headings
		if reBoilerplateHeadings.MatchString(trimmed) {
			continue
		}

		// Check orphaned bullet points
		if reOrphanedBullets.MatchString(trimmed) {
			continue
		}

		// Trim trailing whitespace per line
		filteredLines = append(filteredLines, strings.TrimRight(line, " \t\r"))
	}

	return strings.Join(filteredLines, "\n")
}

// cleanMarkdownContent removes web clutter, empty elements, boilerplate headings, and normalizes spacing while preserving code blocks.
func cleanMarkdownContent(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	// Split by code fences and apply cleaning only to non-code prose segments
	indices := reCodeFence.FindAllStringIndex(raw, -1)
	if len(indices) == 0 {
		cleaned := cleanMarkdownSegment(raw)
		cleaned = reMultipleNewlines.ReplaceAllString(cleaned, "\n\n")
		return strings.TrimSpace(cleaned)
	}

	var sb strings.Builder
	lastOffset := 0

	for _, idx := range indices {
		start, end := idx[0], idx[1]
		if start > lastOffset {
			prose := raw[lastOffset:start]
			sb.WriteString(cleanMarkdownSegment(prose))
		}
		// Write code block verbatim
		sb.WriteString(raw[start:end])
		lastOffset = end
	}

	if lastOffset < len(raw) {
		prose := raw[lastOffset:]
		sb.WriteString(cleanMarkdownSegment(prose))
	}

	cleaned := sb.String()
	cleaned = reMultipleNewlines.ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}

type metadataInfo struct {
	author      string
	description string
	siteName    string
	og          map[string]string
}

func extractHTMLMetadata(n *html.Node) metadataInfo {
	info := metadataInfo{
		og: make(map[string]string),
	}
	if n == nil {
		return info
	}

	var stdAuthor, ogAuthor string
	var stdDescription, ogDescription string
	var ogSiteName string

	var f func(*html.Node)
	f = func(curr *html.Node) {
		if curr.Type == html.ElementNode && curr.Data == "meta" {
			name := strings.ToLower(strings.TrimSpace(getAttr(curr, "name")))
			property := strings.ToLower(strings.TrimSpace(getAttr(curr, "property")))
			content := strings.TrimSpace(getAttr(curr, "content"))

			if content != "" {
				if strings.HasPrefix(property, "og:") {
					info.og[property] = content
				}
				if strings.HasPrefix(name, "og:") {
					info.og[name] = content
				}

				if name == "author" && stdAuthor == "" {
					stdAuthor = content
				} else if (property == "article:author" || property == "author" || name == "article:author") && ogAuthor == "" {
					ogAuthor = content
				}

				if name == "description" && stdDescription == "" {
					stdDescription = content
				} else if (property == "og:description" || name == "og:description" || name == "twitter:description") && ogDescription == "" {
					ogDescription = content
				}

				if (property == "og:site_name" || name == "og:site_name" || name == "site_name") && ogSiteName == "" {
					ogSiteName = content
				}
			}
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)

	if stdAuthor != "" {
		info.author = stdAuthor
	} else {
		info.author = ogAuthor
	}

	if stdDescription != "" {
		info.description = stdDescription
	} else {
		info.description = ogDescription
	}

	info.siteName = ogSiteName

	return info
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
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
