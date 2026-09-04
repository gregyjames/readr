package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var frontmatterRegex = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?(.*)$`)

// Document represents a parsed Markdown document containing frontmatter and body.
type Document struct {
	Frontmatter map[string]interface{}
	RawYAML     string
	Body        string
}

// SplitDocument parses a raw Markdown string into frontmatter metadata and markdown body.
func SplitDocument(raw string) (*Document, error) {
	cleanRaw := strings.TrimPrefix(raw, "\ufeff")

	if !strings.HasPrefix(cleanRaw, "---") {
		return &Document{
			Frontmatter: make(map[string]interface{}),
			RawYAML:     "",
			Body:        raw,
		}, nil
	}

	matches := frontmatterRegex.FindStringSubmatch(cleanRaw)
	if len(matches) < 3 {
		// Starts with --- but doesn't have a valid closing --- delimiter
		return &Document{
			Frontmatter: make(map[string]interface{}),
			RawYAML:     "",
			Body:        raw,
		}, nil
	}

	rawYAML := matches[1]
	body := matches[2]

	var fm map[string]interface{}
	if strings.TrimSpace(rawYAML) != "" {
		if err := yaml.Unmarshal([]byte(rawYAML), &fm); err != nil {
			return nil, fmt.Errorf("failed to parse yaml frontmatter: %w", err)
		}
	}
	if fm == nil {
		fm = make(map[string]interface{})
	}

	return &Document{
		Frontmatter: fm,
		RawYAML:     rawYAML,
		Body:        body,
	}, nil
}

// AssembleDocument combines frontmatter metadata and markdown body into a single Markdown string.
func AssembleDocument(doc *Document) (string, error) {
	if doc == nil {
		return "", nil
	}

	if len(doc.Frontmatter) > 0 {
		yamlBytes, err := yaml.Marshal(doc.Frontmatter)
		if err != nil {
			return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
		}
		body := strings.TrimPrefix(doc.Body, "\r\n")
		body = strings.TrimPrefix(body, "\n")
		if body == "" {
			return fmt.Sprintf("---\n%s---\n", string(yamlBytes)), nil
		}
		return fmt.Sprintf("---\n%s---\n\n%s", string(yamlBytes), body), nil
	}

	if doc.RawYAML != "" {
		body := strings.TrimPrefix(doc.Body, "\r\n")
		body = strings.TrimPrefix(body, "\n")
		if body == "" {
			return fmt.Sprintf("---\n%s\n---\n", doc.RawYAML), nil
		}
		return fmt.Sprintf("---\n%s\n---\n\n%s", doc.RawYAML, body), nil
	}

	return doc.Body, nil
}
