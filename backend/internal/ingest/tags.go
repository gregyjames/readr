package ingest

import (
	"example.com/backend/internal/repository"
)

// SanitizeObsidianTag converts a tag into a valid Obsidian-compatible tag.
func SanitizeObsidianTag(tag string) string {
	return repository.SanitizeObsidianTag(tag)
}

// SanitizeObsidianTags sanitizes a list of tags.
func SanitizeObsidianTags(tags []string) []string {
	return repository.SanitizeObsidianTags(tags)
}
