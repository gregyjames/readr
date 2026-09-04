package agents

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"example.com/backend/internal/ingest"
	"gopkg.in/yaml.v3"
)

type MOCArticleInfo struct {
	ID       int64
	Title    string
	FilePath string
}

func formatMOCArticleWikilink(info MOCArticleInfo) string {
	fileBase := ""
	if info.FilePath != "" {
		fileBase = strings.TrimSuffix(filepath.Base(info.FilePath), ".md")
	}
	if fileBase == "" || fileBase == fmt.Sprint(info.ID) {
		fileBase = ingest.SanitizeTitleFilename(info.Title, info.ID)
	}

	cleanTitle := strings.TrimSpace(info.Title)
	if cleanTitle == "" {
		cleanTitle = fileBase
	}

	if fileBase == cleanTitle {
		return fmt.Sprintf("[[%s]]", fileBase)
	}

	displayTitle := strings.ReplaceAll(cleanTitle, "|", "—")
	return fmt.Sprintf("[[%s|%s]]", fileBase, displayTitle)
}

type MOCItem struct {
	ArticleID   int64  `json:"article_id"`
	ContextNote string `json:"context_note"`
}

type MOCSection struct {
	Title string    `json:"title"`
	Items []MOCItem `json:"items"`
}

type MOCSynthesisResponse struct {
	TopicTitle       string       `json:"topic_title"`
	ExecutiveSummary string       `json:"executive_summary"`
	Sections         []MOCSection `json:"sections"`
}

type MOCDeltaPlacement struct {
	ArticleID     int64  `json:"article_id"`
	TargetSection string `json:"target_section"`
	ContextNote   string `json:"context_note"`
}

type MOCDeltaResponse struct {
	Placements []MOCDeltaPlacement `json:"placements"`
}

// HasCustomUserNotes returns true if the MOC's ## Notes & Synthesis section contains
// custom user-written text beyond empty whitespace or the default placeholder.
func HasCustomUserNotes(mocContent string) bool {
	reNotes := regexp.MustCompile(`(?s)## Notes & Synthesis\s*\n(.*)`)
	matches := reNotes.FindStringSubmatch(mocContent)
	if len(matches) < 2 {
		return false
	}

	content := strings.TrimSpace(matches[1])
	if content == "" {
		return false
	}

	// Clean out comments like <!-- Content below this line is preserved ... -->
	reComments := regexp.MustCompile(`<!--.*?-->`)
	cleaned := strings.TrimSpace(reComments.ReplaceAllString(content, ""))
	if cleaned == "" {
		return false
	}

	// Check if it's solely the default placeholder text
	placeholder := "*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*"
	placeholderClean := strings.Trim(placeholder, "*_ ")
	trimmedClean := strings.Trim(cleaned, "*_ \n\r\t")

	if trimmedClean == "" || trimmedClean == placeholderClean || strings.EqualFold(trimmedClean, placeholderClean) {
		return false
	}
	return true
}

// escapeLikePattern escapes SQLite LIKE wildcards (%, _) and the escape character itself (\)
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// ReconcileMOCLinks scans the curated sections of an MOC (excluding ## Notes & Synthesis)
// and prunes any bullet lines containing [[wikilinks]] whose targets are no longer in validMemberTitles.
// Returns the updated markdown and a bool indicating if any lines were pruned.
func ReconcileMOCLinks(mocContent string, validMemberTitles map[string]bool) (string, bool) {
	if mocContent == "" || validMemberTitles == nil {
		return mocContent, false
	}

	parts := strings.SplitN(mocContent, "## Notes & Synthesis", 2)
	curatedPart := parts[0]
	userNotesPart := ""
	if len(parts) == 2 {
		userNotesPart = "## Notes & Synthesis" + parts[1]
	}

	lines := strings.Split(curatedPart, "\n")
	var newLines []string
	changed := false

	// Matches [[Target]] or [[Target|Alias]]
	reWikilink := regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]+))?\]\]`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Only check list item lines that contain a wikilink
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(line, "[[") {
			matches := reWikilink.FindAllStringSubmatch(line, -1)
			if len(matches) > 0 {
				hasValidLink := false
				for _, m := range matches {
					targetTitle := strings.TrimSpace(m[1])
					if validMemberTitles[targetTitle] || validMemberTitles[strings.ToLower(targetTitle)] {
						hasValidLink = true
						break
					}
					if len(m) > 2 && m[2] != "" {
						alias := strings.TrimSpace(m[2])
						if validMemberTitles[alias] || validMemberTitles[strings.ToLower(alias)] {
							hasValidLink = true
							break
						}
					}
				}
				if !hasValidLink {
					// Stale link, prune this line
					changed = true
					continue
				}
			}
		}
		newLines = append(newLines, line)
	}

	reconciledCurated := strings.Join(newLines, "\n")
	if userNotesPart != "" {
		return reconciledCurated + userNotesPart, changed
	}
	return reconciledCurated, changed
}

func extractMOCSections(mocContent string) []string {
	var sections []string
	lines := strings.Split(mocContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			sec := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			if sec != "" && !strings.EqualFold(sec, "Notes & Synthesis") && !strings.EqualFold(sec, "Executive Overview") && !strings.EqualFold(sec, "Curated Index") {
				sections = append(sections, sec)
			}
		} else if strings.HasPrefix(trimmed, "### ") {
			sec := strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			if sec != "" && !strings.EqualFold(sec, "Notes & Synthesis") && !strings.EqualFold(sec, "Executive Overview") && !strings.EqualFold(sec, "Curated Index") {
				sections = append(sections, sec)
			}
		}
	}
	return sections
}

func assembleMOCMarkdown(synthesis *MOCSynthesisResponse, mocTitle, tag, existingBody string, articleInfoMap map[int64]MOCArticleInfo) string {
	userNotesContent := ""
	if existingBody != "" {
		reNotes := regexp.MustCompile(`(?s)## Notes & Synthesis\s*\n(.*)`)
		if matches := reNotes.FindStringSubmatch(existingBody); len(matches) > 1 {
			userNotesContent = strings.TrimSpace(matches[1])
		}
	}

	if userNotesContent == "" {
		userNotesContent = "<!-- Content below this line is preserved across automated Librarian updates -->\n"
	}

	frontmatterData := map[string]interface{}{
		"type":  "moc",
		"title": mocTitle,
		"tags":  []string{"moc", tag},
		"date":  time.Now().Format("2006-01-02"),
		"generated": map[string]interface{}{
			"by":         "agent/librarian-moc",
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	yamlBytes, _ := yaml.Marshal(frontmatterData)
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(string(yamlBytes))
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s\n\n", mocTitle))
	sb.WriteString("## Executive Overview\n")
	sb.WriteString(strings.TrimSpace(synthesis.ExecutiveSummary) + "\n\n")

	sb.WriteString("## Curated Index\n\n")
	for _, section := range synthesis.Sections {
		sb.WriteString(fmt.Sprintf("### %s\n", strings.TrimSpace(section.Title)))
		for _, item := range section.Items {
			info, ok := articleInfoMap[item.ArticleID]
			if !ok {
				info = MOCArticleInfo{ID: item.ArticleID, Title: fmt.Sprintf("Article %d", item.ArticleID)}
			}
			wikilink := formatMOCArticleWikilink(info)
			contextNote := strings.TrimSpace(item.ContextNote)
			if contextNote != "" {
				sb.WriteString(fmt.Sprintf("- %s - %s\n", wikilink, contextNote))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", wikilink))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Notes & Synthesis\n")
	sb.WriteString(userNotesContent + "\n")

	return sb.String()
}

func applyDeltaPlacements(existingContent string, placements []MOCDeltaPlacement, articleInfoMap map[int64]MOCArticleInfo) string {
	if len(placements) == 0 {
		return existingContent
	}

	type sectionPlacements struct {
		section string
		items   []MOCDeltaPlacement
	}
	var grouped []sectionPlacements
	sectionIdx := make(map[string]int)

	for _, p := range placements {
		sec := strings.TrimSpace(p.TargetSection)
		if sec == "" {
			sec = "Uncategorized"
		}
		normSec := strings.ToLower(sec)
		if idx, exists := sectionIdx[normSec]; exists {
			grouped[idx].items = append(grouped[idx].items, p)
		} else {
			sectionIdx[normSec] = len(grouped)
			grouped = append(grouped, sectionPlacements{
				section: sec,
				items:   []MOCDeltaPlacement{p},
			})
		}
	}

	lines := strings.Split(existingContent, "\n")
	scheduledArticles := make(map[int64]bool)
	scheduledWikilinks := make(map[string]bool)

	for _, g := range grouped {
		headerIdx := -1
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
				if strings.EqualFold(headerText, g.section) {
					headerIdx = i
					break
				}
			}
		}

		var itemLines []string
		for _, item := range g.items {
			if scheduledArticles[item.ArticleID] {
				continue
			}

			info, ok := articleInfoMap[item.ArticleID]
			if !ok {
				info = MOCArticleInfo{ID: item.ArticleID, Title: fmt.Sprintf("Article %d", item.ArticleID)}
			}

			wikilink := formatMOCArticleWikilink(info)
			if scheduledWikilinks[wikilink] {
				continue
			}

			fileBase := ""
			if info.FilePath != "" {
				fileBase = strings.TrimSuffix(filepath.Base(info.FilePath), ".md")
			}

			alreadyPresent := false
			for _, line := range lines {
				if strings.Contains(line, wikilink) ||
					(fileBase != "" && (strings.Contains(line, fmt.Sprintf("[[%s]]", fileBase)) || strings.Contains(line, fmt.Sprintf("[[%s|", fileBase)))) ||
					(info.Title != "" && (strings.Contains(line, fmt.Sprintf("[[%s]]", info.Title)) || strings.Contains(line, fmt.Sprintf("[[%s|", info.Title)))) {
					alreadyPresent = true
					break
				}
			}
			if alreadyPresent {
				continue
			}

			scheduledArticles[item.ArticleID] = true
			scheduledWikilinks[wikilink] = true

			note := strings.TrimSpace(item.ContextNote)
			if note != "" {
				itemLines = append(itemLines, fmt.Sprintf("- %s - %s", wikilink, note))
			} else {
				itemLines = append(itemLines, fmt.Sprintf("- %s", wikilink))
			}
		}

		if len(itemLines) == 0 {
			continue
		}

		if headerIdx != -1 {
			nextHeaderIdx := len(lines)
			for i := headerIdx + 1; i < len(lines); i++ {
				trimmed := strings.TrimSpace(lines[i])
				if strings.HasPrefix(trimmed, "#") {
					nextHeaderIdx = i
					break
				}
			}

			insertIdx := nextHeaderIdx
			for insertIdx > headerIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
				insertIdx--
			}

			newLines := make([]string, 0, len(lines)+len(itemLines))
			newLines = append(newLines, lines[:insertIdx]...)
			newLines = append(newLines, itemLines...)
			newLines = append(newLines, lines[insertIdx:]...)
			lines = newLines
		} else {
			notesIdx := -1
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
					if strings.EqualFold(headerText, "Notes & Synthesis") {
						notesIdx = i
						break
					}
				}
			}

			var newSectionLines []string
			newSectionLines = append(newSectionLines, fmt.Sprintf("## %s", g.section))
			newSectionLines = append(newSectionLines, itemLines...)
			newSectionLines = append(newSectionLines, "")

			if notesIdx != -1 {
				newLines := make([]string, 0, len(lines)+len(newSectionLines))
				newLines = append(newLines, lines[:notesIdx]...)
				newLines = append(newLines, newSectionLines...)
				newLines = append(newLines, lines[notesIdx:]...)
				lines = newLines
			} else {
				lines = append(lines, "")
				lines = append(lines, newSectionLines...)
			}
		}
	}

	return strings.Join(lines, "\n")
}

var reMOCWikilink = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

func extractLinkedArticlesFromMOC(mocContent string) map[string]bool {
	linked := make(map[string]bool)
	matches := reMOCWikilink.FindAllStringSubmatch(mocContent, -1)
	for _, m := range matches {
		if len(m) > 1 {
			raw := strings.TrimSpace(m[1])
			if raw == "" {
				continue
			}
			// 1. Mark the entire raw string inside [[...]] as linked (preserves literal pipe-containing titles)
			linked[raw] = true
			linked[strings.ToLower(raw)] = true

			// 2. If it is a pipe-delimited [[Target|Alias]], index only the target component
			if strings.Contains(raw, "|") {
				parts := strings.Split(raw, "|")
				target := strings.TrimSpace(parts[0])
				if target != "" {
					linked[target] = true
					linked[strings.ToLower(target)] = true
				}
			}
		}
	}
	return linked
}
