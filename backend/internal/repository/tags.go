package repository

import (
	"strings"
	"unicode"
)

// SanitizeObsidianTag converts a tag into a valid Obsidian-compatible tag:
// - Converts to lowercase and trims whitespace
// - Replaces spaces, underscores, periods, and consecutive hyphens with a single hyphen
// - Strips invalid characters (preserving alphanumeric characters, hyphens, and slashes for nested tags)
// - Trims leading and trailing hyphens and slashes
func SanitizeObsidianTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return ""
	}

	var b strings.Builder
	lastWasHyphen := false
	for _, r := range tag {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasHyphen = false
		} else if r == '/' {
			// Support nested tags like "tech/ai"
			if !lastWasHyphen && b.Len() > 0 {
				b.WriteRune(r)
				lastWasHyphen = false
			}
		} else if r == ' ' || r == '-' || r == '_' || r == '.' {
			if !lastWasHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastWasHyphen = true
			}
		}
	}

	res := strings.Trim(b.String(), "-/")
	return res
}

// SanitizeObsidianTags sanitizes a list of tags, deduplicating them and removing empty or "moc" reserved tags.
func SanitizeObsidianTags(tags []string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, t := range tags {
		// Handle potential comma-separated tags passed in a single string
		subTags := strings.Split(t, ",")
		for _, st := range subTags {
			cleaned := SanitizeObsidianTag(st)
			if cleaned != "" && cleaned != "moc" {
				if _, exists := seen[cleaned]; !exists {
					seen[cleaned] = struct{}{}
					result = append(result, cleaned)
				}
			}
		}
	}
	return result
}
