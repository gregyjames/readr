package agents

import (
	"fmt"
	"path/filepath"
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
		return summaryBlockRegex.ReplaceAllLiteralString(body, newSummaryBlock)
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
				if a.FilePath != "" {
					base := strings.TrimSuffix(filepath.Base(a.FilePath), ".md")
					isNum := len(base) > 0
					for _, r := range base {
						if r < '0' || r > '9' {
							isNum = false
							break
						}
					}
					if !isNum && base != "" {
						targetTitle = base
					}
				}
				break
			}
		}
		if targetTitle == "" {
			continue
		}

		// Format wikilink (aliased if phrase is different from title)
		var replacement string
		if phrase == targetTitle {
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
	var allRaw []string
	if strings.TrimSpace(existingTags) != "" {
		allRaw = append(allRaw, strings.Split(existingTags, ",")...)
	}
	allRaw = append(allRaw, aiTags...)
	return repository.SanitizeObsidianTags(allRaw)
}

func extractSourceURLFromFrontmatter(frontmatterStr string) string {
	if strings.TrimSpace(frontmatterStr) == "" {
		return ""
	}
	cleanYAML := strings.TrimPrefix(frontmatterStr, "---\n")
	cleanYAML = strings.TrimSuffix(cleanYAML, "---\n")
	cleanYAML = strings.TrimSuffix(cleanYAML, "---")
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(cleanYAML), &rawMap); err == nil && rawMap != nil {
		if s, ok := rawMap["source"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		if u, ok := rawMap["url"].(string); ok && strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
		if r, ok := rawMap["resource"].(string); ok && strings.TrimSpace(r) != "" {
			return strings.TrimSpace(r)
		}
	}
	return ""
}

func serializeOKFMetadata(frontmatter *OKFFrontmatterResponse, mergedTags []string, sourceURL string) (string, *OKFMetadata, error) {
	metadata := OKFMetadata{
		Type:        frontmatter.Type,
		Title:       frontmatter.Title,
		Description: frontmatter.Description,
		Source:      strings.TrimSpace(sourceURL),
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

func extractMessageContent(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	if parts, ok := raw.([]interface{}); ok {
		var sb strings.Builder
		for _, part := range parts {
			if m, ok := part.(map[string]interface{}); ok {
				if txt, ok := m["text"].(string); ok {
					sb.WriteString(txt)
				}
			}
		}
		return sb.String()
	}
	return fmt.Sprint(raw)
}

func cleanJSONBlock(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 2 {
			firstLine := strings.TrimSpace(lines[0])
			lastLine := strings.TrimSpace(lines[len(lines)-1])
			if strings.HasPrefix(firstLine, "```") && strings.HasPrefix(lastLine, "```") {
				raw = strings.Join(lines[1:len(lines)-1], "\n")
			} else if strings.HasPrefix(firstLine, "```") {
				raw = strings.Join(lines[1:], "\n")
			}
		}
	}
	return strings.TrimSpace(raw)
}
