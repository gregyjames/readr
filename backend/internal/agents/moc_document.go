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

// MOCParsedItem represents an item within a curated section of an MOCDocument.
type MOCParsedItem struct {
	RawLine     string
	ArticleID   int64
	Wikilink    string
	TargetTitle string
	AliasTitle  string
	ContextNote string
}

// MOCParsedSection represents a section in the Curated Index.
type MOCParsedSection struct {
	Title string
	Items []MOCParsedItem
}

// MOCDocument represents a structured Map of Content document.
type MOCDocument struct {
	Frontmatter      map[string]interface{}
	RawYAML          string
	Title            string
	ExecutiveSummary string
	CuratedSections  []MOCParsedSection
	UserNotesBody    string
}

// ParseMOCDocument parses a markdown string into a structured MOCDocument.
func ParseMOCDocument(raw string) (*MOCDocument, error) {
	doc := &MOCDocument{
		Frontmatter: make(map[string]interface{}),
	}

	trimmed := strings.TrimLeft(raw, "\r\n\t ")
	body := raw

	if strings.HasPrefix(trimmed, "---") {
		parts := strings.SplitN(trimmed[3:], "---", 2)
		if len(parts) == 2 {
			yamlStr := strings.TrimSpace(parts[0])
			body = strings.TrimLeft(parts[1], "\r\n")
			doc.RawYAML = yamlStr
			if yamlStr != "" {
				_ = yaml.Unmarshal([]byte(yamlStr), &doc.Frontmatter)
			}
		}
	}

	// Split into Curated area and User Notes area
	notesSplit := strings.SplitN(body, "## Notes & Synthesis", 2)
	curatedArea := notesSplit[0]
	if len(notesSplit) > 1 {
		doc.UserNotesBody = strings.TrimLeft(notesSplit[1], "\r\n")
	}

	lines := strings.Split(curatedArea, "\n")
	var currentSection *MOCParsedSection
	reWikilink := regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]+))?\]\]`)

	inExecutiveOverview := false
	var overviewLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Document Title (# Title)
		if strings.HasPrefix(trimmedLine, "# ") && doc.Title == "" {
			doc.Title = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "# "))
			inExecutiveOverview = false
			continue
		}

		// Top-level sections (## ...)
		if strings.HasPrefix(trimmedLine, "## ") {
			secTitle := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "## "))
			if strings.EqualFold(secTitle, "Executive Overview") {
				inExecutiveOverview = true
				currentSection = nil
				continue
			} else if strings.EqualFold(secTitle, "Curated Index") {
				inExecutiveOverview = false
				currentSection = nil
				continue
			} else {
				// Treat as a section if not Overview/Index/Notes
				inExecutiveOverview = false
				doc.CuratedSections = append(doc.CuratedSections, MOCParsedSection{
					Title: secTitle,
				})
				currentSection = &doc.CuratedSections[len(doc.CuratedSections)-1]
				continue
			}
		}

		// Sub-level sections (### ...)
		if strings.HasPrefix(trimmedLine, "### ") {
			inExecutiveOverview = false
			secTitle := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "### "))
			doc.CuratedSections = append(doc.CuratedSections, MOCParsedSection{
				Title: secTitle,
			})
			currentSection = &doc.CuratedSections[len(doc.CuratedSections)-1]
			continue
		}

		if inExecutiveOverview {
			overviewLines = append(overviewLines, line)
			continue
		}

		// Curated item line
		if currentSection != nil && strings.HasPrefix(trimmedLine, "- ") {
			item := MOCParsedItem{
				RawLine: line,
			}
			matches := reWikilink.FindStringSubmatch(line)
			if len(matches) > 1 {
				item.Wikilink = matches[0]
				item.TargetTitle = strings.TrimSpace(matches[1])
				if len(matches) > 2 && matches[2] != "" {
					item.AliasTitle = strings.TrimSpace(matches[2])
				}
			}

			// Extract context note after " - "
			bulletContent := strings.TrimPrefix(trimmedLine, "- ")
			if item.Wikilink != "" {
				linkIdx := strings.Index(bulletContent, item.Wikilink)
				if linkIdx != -1 {
					afterLink := strings.TrimSpace(bulletContent[linkIdx+len(item.Wikilink):])
					item.ContextNote = strings.TrimPrefix(afterLink, "- ")
					item.ContextNote = strings.TrimPrefix(item.ContextNote, "-")
					item.ContextNote = strings.TrimSpace(item.ContextNote)
				}
			} else {
				item.ContextNote = bulletContent
			}

			currentSection.Items = append(currentSection.Items, item)
		}
	}

	doc.ExecutiveSummary = strings.TrimSpace(strings.Join(overviewLines, "\n"))
	return doc, nil
}

// Serialize serializes the MOCDocument into markdown.
func (doc *MOCDocument) Serialize() string {
	var sb strings.Builder

	if len(doc.Frontmatter) > 0 {
		yamlBytes, _ := yaml.Marshal(doc.Frontmatter)
		sb.WriteString("---\n")
		sb.WriteString(string(yamlBytes))
		sb.WriteString("---\n\n")
	}

	title := doc.Title
	if title == "" {
		if t, ok := doc.Frontmatter["title"].(string); ok && t != "" {
			title = t
		} else {
			title = "MOC"
		}
	}

	sb.WriteString(fmt.Sprintf("# %s\n\n", title))

	if strings.TrimSpace(doc.ExecutiveSummary) != "" {
		sb.WriteString("## Executive Overview\n")
		sb.WriteString(strings.TrimSpace(doc.ExecutiveSummary) + "\n\n")
	}

	if len(doc.CuratedSections) > 0 {
		sb.WriteString("## Curated Index\n\n")
		for _, section := range doc.CuratedSections {
			sb.WriteString(fmt.Sprintf("### %s\n", strings.TrimSpace(section.Title)))
			for _, item := range section.Items {
				if item.RawLine != "" && strings.HasPrefix(strings.TrimSpace(item.RawLine), "- ") {
					sb.WriteString(item.RawLine + "\n")
				} else if item.Wikilink != "" {
					if item.ContextNote != "" {
						sb.WriteString(fmt.Sprintf("- %s - %s\n", item.Wikilink, item.ContextNote))
					} else {
						sb.WriteString(fmt.Sprintf("- %s\n", item.Wikilink))
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Notes & Synthesis\n")
	userNotes := strings.TrimSpace(doc.UserNotesBody)
	if userNotes == "" {
		userNotes = "<!-- Content below this line is preserved across automated Librarian updates -->\n"
	}
	sb.WriteString(userNotes + "\n")

	return sb.String()
}

// HasCustomUserNotes returns true if the MOC's UserNotesBody contains custom user text.
func (doc *MOCDocument) HasCustomUserNotes() bool {
	content := strings.TrimSpace(doc.UserNotesBody)
	if content == "" {
		return false
	}

	reComments := regexp.MustCompile(`<!--.*?-->`)
	cleaned := strings.TrimSpace(reComments.ReplaceAllString(content, ""))
	if cleaned == "" {
		return false
	}

	placeholder := "*Add your manual observations, key takeaways, and cross-cutting synthesis across these notes here.*"
	placeholderClean := strings.Trim(placeholder, "*_ ")
	trimmedClean := strings.Trim(cleaned, "*_ \n\r\t")

	if trimmedClean == "" || trimmedClean == placeholderClean || strings.EqualFold(trimmedClean, placeholderClean) {
		return false
	}
	return true
}

// ReconcileLinks prunes items whose target wikilinks are no longer in validMemberTitles.
func (doc *MOCDocument) ReconcileLinks(validMemberTitles map[string]bool) bool {
	if validMemberTitles == nil {
		return false
	}

	changed := false
	var updatedSections []MOCParsedSection

	for _, sec := range doc.CuratedSections {
		var validItems []MOCParsedItem
		for _, item := range sec.Items {
			if item.Wikilink == "" {
				validItems = append(validItems, item)
				continue
			}

			targetValid := validMemberTitles[item.TargetTitle] || validMemberTitles[strings.ToLower(item.TargetTitle)]
			aliasValid := item.AliasTitle != "" && (validMemberTitles[item.AliasTitle] || validMemberTitles[strings.ToLower(item.AliasTitle)])

			if targetValid || aliasValid {
				validItems = append(validItems, item)
			} else {
				changed = true
			}
		}
		sec.Items = validItems
		updatedSections = append(updatedSections, sec)
	}

	doc.CuratedSections = updatedSections
	return changed
}

// ApplyDeltaPlacements applies delta placements to the MOCDocument.
func (doc *MOCDocument) ApplyDeltaPlacements(placements []MOCDeltaPlacement, articleInfoMap map[int64]MOCArticleInfo) {
	if len(placements) == 0 {
		return
	}

	// Index existing items by article ID, target title, and wikilink
	existingLinks := make(map[string]bool)
	for _, sec := range doc.CuratedSections {
		for _, item := range sec.Items {
			if item.Wikilink != "" {
				existingLinks[item.Wikilink] = true
			}
			if item.TargetTitle != "" {
				existingLinks[item.TargetTitle] = true
				existingLinks[strings.ToLower(item.TargetTitle)] = true
			}
			if item.AliasTitle != "" {
				existingLinks[item.AliasTitle] = true
				existingLinks[strings.ToLower(item.AliasTitle)] = true
			}
		}
	}

	scheduledArticles := make(map[int64]bool)

	for _, p := range placements {
		if scheduledArticles[p.ArticleID] {
			continue
		}

		info, ok := articleInfoMap[p.ArticleID]
		if !ok {
			info = MOCArticleInfo{ID: p.ArticleID, Title: fmt.Sprintf("Article %d", p.ArticleID)}
		}

		wikilink := formatMOCArticleWikilink(info)
		fileBase := ""
		if info.FilePath != "" {
			fileBase = strings.TrimSuffix(filepath.Base(info.FilePath), ".md")
		}

		if existingLinks[wikilink] ||
			(info.Title != "" && (existingLinks[info.Title] || existingLinks[strings.ToLower(info.Title)])) ||
			(fileBase != "" && (existingLinks[fileBase] || existingLinks[strings.ToLower(fileBase)])) {
			continue
		}

		scheduledArticles[p.ArticleID] = true
		existingLinks[wikilink] = true
		if info.Title != "" {
			existingLinks[info.Title] = true
			existingLinks[strings.ToLower(info.Title)] = true
		}

		targetSecName := strings.TrimSpace(p.TargetSection)
		if targetSecName == "" {
			targetSecName = "Uncategorized"
		}

		newItem := MOCParsedItem{
			ArticleID:   p.ArticleID,
			Wikilink:    wikilink,
			TargetTitle: info.Title,
			ContextNote: strings.TrimSpace(p.ContextNote),
		}
		if newItem.ContextNote != "" {
			newItem.RawLine = fmt.Sprintf("- %s - %s", wikilink, newItem.ContextNote)
		} else {
			newItem.RawLine = fmt.Sprintf("- %s", wikilink)
		}

		// Find section
		sectionFound := false
		for i := range doc.CuratedSections {
			if strings.EqualFold(doc.CuratedSections[i].Title, targetSecName) {
				doc.CuratedSections[i].Items = append(doc.CuratedSections[i].Items, newItem)
				sectionFound = true
				break
			}
		}

		if !sectionFound {
			doc.CuratedSections = append(doc.CuratedSections, MOCParsedSection{
				Title: targetSecName,
				Items: []MOCParsedItem{newItem},
			})
		}
	}
}

// HasCustomUserNotes returns true if the MOC's ## Notes & Synthesis section contains
// custom user-written text beyond empty whitespace or the default placeholder.
func HasCustomUserNotes(mocContent string) bool {
	doc, err := ParseMOCDocument(mocContent)
	if err != nil {
		return false
	}
	return doc.HasCustomUserNotes()
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

	doc, err := ParseMOCDocument(mocContent)
	if err != nil {
		return mocContent, false
	}

	pruned := doc.ReconcileLinks(validMemberTitles)
	if !pruned {
		return mocContent, false
	}
	return doc.Serialize(), true
}

func extractMOCSections(mocContent string) []string {
	doc, err := ParseMOCDocument(mocContent)
	if err != nil {
		return nil
	}
	var sections []string
	for _, sec := range doc.CuratedSections {
		t := strings.TrimSpace(sec.Title)
		if t != "" && !strings.EqualFold(t, "Notes & Synthesis") && !strings.EqualFold(t, "Executive Overview") && !strings.EqualFold(t, "Curated Index") {
			sections = append(sections, t)
		}
	}
	return sections
}

func assembleMOCMarkdown(synthesis *MOCSynthesisResponse, mocTitle, tag, existingBody string, articleInfoMap map[int64]MOCArticleInfo) string {
	userNotesContent := ""
	if existingBody != "" {
		if existingDoc, err := ParseMOCDocument(existingBody); err == nil && existingDoc.UserNotesBody != "" {
			userNotesContent = existingDoc.UserNotesBody
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

	var parsedSections []MOCParsedSection
	for _, section := range synthesis.Sections {
		var items []MOCParsedItem
		for _, item := range section.Items {
			info, ok := articleInfoMap[item.ArticleID]
			if !ok {
				info = MOCArticleInfo{ID: item.ArticleID, Title: fmt.Sprintf("Article %d", item.ArticleID)}
			}
			wikilink := formatMOCArticleWikilink(info)
			contextNote := strings.TrimSpace(item.ContextNote)
			rawLine := ""
			if contextNote != "" {
				rawLine = fmt.Sprintf("- %s - %s", wikilink, contextNote)
			} else {
				rawLine = fmt.Sprintf("- %s", wikilink)
			}
			items = append(items, MOCParsedItem{
				RawLine:     rawLine,
				ArticleID:   item.ArticleID,
				Wikilink:    wikilink,
				TargetTitle: info.Title,
				ContextNote: contextNote,
			})
		}
		parsedSections = append(parsedSections, MOCParsedSection{
			Title: strings.TrimSpace(section.Title),
			Items: items,
		})
	}

	doc := &MOCDocument{
		Frontmatter:      frontmatterData,
		Title:            mocTitle,
		ExecutiveSummary: strings.TrimSpace(synthesis.ExecutiveSummary),
		CuratedSections:  parsedSections,
		UserNotesBody:    userNotesContent,
	}

	return doc.Serialize()
}

func applyDeltaPlacements(existingContent string, placements []MOCDeltaPlacement, articleInfoMap map[int64]MOCArticleInfo) string {
	if len(placements) == 0 {
		return existingContent
	}

	doc, err := ParseMOCDocument(existingContent)
	if err != nil {
		return existingContent
	}

	doc.ApplyDeltaPlacements(placements, articleInfoMap)
	return doc.Serialize()
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
