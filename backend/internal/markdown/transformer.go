package markdown

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var summaryBlockRegex = regexp.MustCompile(`(?m)^>\s*(?:💡\s*)?(?:\*\*)?(?:AI\s+)?Summary:(?:\*\*)?.*(?:\r?\n>.*)*`)

// ApplySummaryBlock adds, updates, or removes a callout summary block at the beginning of the markdown body.
func ApplySummaryBlock(body string, summary string) string {
	summaryText := strings.TrimSpace(summary)
	if summaryText == "" {
		if summaryBlockRegex.MatchString(body) {
			cleaned := summaryBlockRegex.ReplaceAllLiteralString(body, "")
			return strings.TrimLeft(cleaned, "\r\n")
		}
		return body
	}

	newSummaryBlock := fmt.Sprintf("> 💡 **Summary:** %s", summaryText)
	if summaryBlockRegex.MatchString(body) {
		return summaryBlockRegex.ReplaceAllLiteralString(body, newSummaryBlock)
	}

	return newSummaryBlock + "\n\n" + strings.TrimLeft(body, "\r\n")
}

// InjectWikilinks replaces keywords in standard markdown prose with wikilinks,
// while skipping code blocks, inline code spans, raw HTML blocks, and autolinks via Goldmark AST parsing.
func InjectWikilinks(body string, replacements map[string]string) string {
	if len(replacements) == 0 || strings.TrimSpace(body) == "" {
		return body
	}

	keys := make([]string, 0, len(replacements))
	for k := range replacements {
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return body
	}

	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	patternParts := make([]string, len(keys))
	for i, k := range keys {
		patternParts[i] = regexp.QuoteMeta(k)
	}
	replRegex, err := regexp.Compile(strings.Join(patternParts, "|"))
	if err != nil {
		return body
	}

	src := []byte(body)
	reader := text.NewReader(src)
	doc := goldmark.New().Parser().Parse(reader)

	type textSegment struct {
		start int
		stop  int
	}
	var segments []textSegment

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n.Kind() {
		case ast.KindFencedCodeBlock, ast.KindCodeBlock, ast.KindCodeSpan, ast.KindHTMLBlock, ast.KindAutoLink, ast.KindLink, ast.KindImage:
			return ast.WalkSkipChildren, nil
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				seg := t.Segment
				if seg.Stop > seg.Start && seg.Start >= 0 && seg.Stop <= len(src) {
					segments = append(segments, textSegment{start: seg.Start, stop: seg.Stop})
				}
			}
		}

		return ast.WalkContinue, nil
	})

	if len(segments) == 0 {
		return body
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].start < segments[j].start
	})

	var sb strings.Builder
	lastOffset := 0
	for _, seg := range segments {
		if seg.start < lastOffset {
			continue
		}
		sb.WriteString(body[lastOffset:seg.start])
		origText := body[seg.start:seg.stop]
		replaced := replRegex.ReplaceAllStringFunc(origText, func(match string) string {
			if repl, ok := replacements[match]; ok {
				return repl
			}
			return match
		})
		sb.WriteString(replaced)
		lastOffset = seg.stop
	}
	if lastOffset < len(body) {
		sb.WriteString(body[lastOffset:])
	}

	return sb.String()
}
