package agents

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"example.com/backend/internal/repository"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

var summaryBlockRegex = regexp.MustCompile(`(?m)^>\s*(?:💡\s*)?(?:\*\*)?(?:AI\s+)?Summary:(?:\*\*)?.*(?:\n>.*)*`)
var reProtected = regexp.MustCompile(`(\[\[[\s\S]*?\]\]|\[[\s\S]*?\]\([\s\S]*?\)|` + "`[\\s\\S]*?`" + `)`)

func applySummary(body string, summary string) string {
	summaryText := strings.TrimSpace(summary)
	if summaryText == "" {
		return body
	}
	newSummaryBlock := fmt.Sprintf("> 💡 **Summary:** %s", summaryText)
	if summaryBlockRegex.MatchString(body) {
		return summaryBlockRegex.ReplaceAllString(body, newSummaryBlock)
	}
	return newSummaryBlock + "\n\n" + strings.TrimLeft(body, "\n")
}

func injectLinksIntoBody(body string, links []llmLink, candidates []repository.ArticleRecord, sourceID int64, db *gorm.DB) (string, []string) {
	var injectedLinks []string
	if len(links) == 0 {
		return body, injectedLinks
	}

	for _, link := range links {
		phrase := strings.TrimSpace(link.ExactPhraseInText)
		if phrase == "" {
			continue
		}

		var targetTitle string
		for _, a := range candidates {
			if a.ID == link.ExistingArticleID {
				targetTitle = a.Title
				break
			}
		}
		if targetTitle == "" {
			continue
		}

		// Format wikilink (aliased if phrase is different from title)
		var replacement string
		if strings.EqualFold(phrase, targetTitle) {
			replacement = fmt.Sprintf("[[%s]]", targetTitle)
		} else {
			replacement = fmt.Sprintf("[[%s|%s]]", targetTitle, phrase)
		}

		// Don't inject if already linked to this target anywhere in body
		alreadyLinkedSimple := fmt.Sprintf("[[%s]]", targetTitle)
		alreadyLinkedAliasedPrefix := fmt.Sprintf("[[%s|", targetTitle)
		if strings.Contains(body, alreadyLinkedSimple) || strings.Contains(body, alreadyLinkedAliasedPrefix) {
			continue
		}

		// Split into protected tokens and unprotected segments
		parts := reProtected.Split(body, -1)
		matches := reProtected.FindAllString(body, -1)

		replaced := false
		var newBody strings.Builder
		for i, part := range parts {
			if !replaced && strings.Contains(part, phrase) {
				part = strings.Replace(part, phrase, replacement, 1)
				replaced = true
			}
			newBody.WriteString(part)
			if i < len(matches) {
				newBody.WriteString(matches[i])
			}
		}

		if replaced {
			body = newBody.String()
			injectedLinks = append(injectedLinks, fmt.Sprintf("%s (target_id: %d)", replacement, link.ExistingArticleID))
			if db != nil {
				var count int64
				db.Table("article_links").Where("source_id = ? AND target_id = ?", sourceID, link.ExistingArticleID).Count(&count)
				if count == 0 {
					db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", sourceID, link.ExistingArticleID)
				}
			}
		}
	}
	return body, injectedLinks
}

func mergeArticleTags(existingTags string, aiTags []string) []string {
	var existing []string
	if strings.TrimSpace(existingTags) != "" {
		for _, t := range strings.Split(existingTags, ",") {
			cleaned := strings.ToLower(strings.TrimSpace(t))
			if cleaned != "" {
				existing = append(existing, cleaned)
			}
		}
	}

	seen := make(map[string]struct{})
	var merged []string
	for _, t := range existing {
		if _, exists := seen[t]; !exists {
			seen[t] = struct{}{}
			merged = append(merged, t)
		}
	}
	for _, t := range aiTags {
		cleaned := strings.ToLower(strings.TrimSpace(t))
		if cleaned != "" {
			if _, exists := seen[cleaned]; !exists {
				seen[cleaned] = struct{}{}
				merged = append(merged, cleaned)
			}
		}
	}
	return merged
}

func serializeOKFMetadata(frontmatter *OKFFrontmatterResponse, mergedTags []string) (string, *OKFMetadata, error) {
	metadata := OKFMetadata{
		Type:        frontmatter.Type,
		Title:       frontmatter.Title,
		Description: frontmatter.Description,
		Resource:    frontmatter.Resource,
		Tags:        mergedTags,
		Generated: OKFGeneratedInfo{
			By: "agent/readr-pipeline",
			At: time.Now().UTC().Format(time.RFC3339),
		},
	}
	yamlBytes, err := yaml.Marshal(&metadata)
	if err != nil {
		return "", nil, err
	}
	return "---\n" + string(yamlBytes) + "---\n\n", &metadata, nil
}
