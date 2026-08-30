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
