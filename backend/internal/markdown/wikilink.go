package markdown

import (
	"regexp"
	"strings"
)

// WikilinkRegex is the canonical regex matching Obsidian wikilinks [[Target]] or [[Target|Alias]].
var WikilinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// Wikilink represents an extracted wikilink with its target and optional display alias.
type Wikilink struct {
	Target  string
	Display string
}

// ExtractWikilinks extracts all [[Target|Display]] and [[Target]] wikilinks from markdown text.
func ExtractWikilinks(text string) []Wikilink {
	matches := WikilinkRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	links := make([]Wikilink, len(matches))
	for i, m := range matches {
		target := strings.TrimSpace(m[1])
		display := target
		if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
			display = strings.TrimSpace(m[2])
		}
		links[i] = Wikilink{Target: target, Display: display}
	}
	return links
}

// ExtractUniqueWikilinkTargets extracts unique non-empty target names from markdown text (case-preserving).
func ExtractUniqueWikilinkTargets(text string) []string {
	links := ExtractWikilinks(text)
	if len(links) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(links))
	result := make([]string, 0, len(links))
	for _, l := range links {
		norm := strings.ToLower(l.Target)
		if norm != "" {
			if _, exists := seen[norm]; !exists {
				seen[norm] = struct{}{}
				result = append(result, l.Target)
			}
		}
	}
	return result
}
