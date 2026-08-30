package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
)

func (p *AgentPool) processAutoLinker(job Job) {
	// 1. Load all existing articles from the DB
	var allArticles []repository.ArticleRecord
	if err := p.db.Table("articles").Find(&allArticles).Error; err != nil {
		p.logger.Error("AutoLinker could not load articles", zap.Error(err))
		return
	}

	// 2. Read the new article's markdown file
	filePath := filepath.Join(p.dataDirectory, "articles", fmt.Sprintf("%d.md", job.ArticleID))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		p.logger.Error("AutoLinker could not read file", zap.Error(err))
		return
	}
	content := string(contentBytes)

	// 3. Simple Aho-Corasick / String Matching logic
	// For each existing article, see if its title appears in the new article's body.
	// If it does, and it's not already linked, link it!
	
	// Split frontmatter and body so we only inject links in the body
	frontmatter := ""
	body := content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) == 3 {
			frontmatter = "---\n" + parts[1] + "---\n"
			body = parts[2]
		}
	}

	linksAdded := 0
	for _, existing := range allArticles {
		if existing.ID == job.ArticleID {
			continue // Don't link to self
		}

		// Skip very short titles to avoid false positives (like "A", "The", "IT")
		title := strings.TrimSpace(existing.Title)
		if len(title) < 4 {
			continue
		}

		// Check if the exact title exists in the body, but NOT inside an existing link like [[Title]] or [Title](url)
		// A simple heuristic for now: check if it exists in the body lowercased.
		lowerBody := strings.ToLower(body)
		lowerTitle := strings.ToLower(title)
		
		idx := strings.Index(lowerBody, lowerTitle)
		if idx != -1 {
			// Check if it's already a wikilink by looking a few characters back
			startCheck := idx - 2
			if startCheck >= 0 && lowerBody[startCheck:idx] == "[[" {
				continue // Already linked
			}
			
			// Replace the FIRST occurrence of the title with [[Title]]
			// To keep original casing of the text, we extract it from the original body
			originalMatch := body[idx : idx+len(title)]
			replacement := fmt.Sprintf("[[%s]]", originalMatch)
			
			// Replace it in the body
			body = body[:idx] + replacement + body[idx+len(title):]
			
			// Check if link already exists in SQLite
			var count int64
			p.db.Table("article_links").Where("source_id = ? AND target_id = ?", job.ArticleID, existing.ID).Count(&count)
			if count == 0 {
				err := p.db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", job.ArticleID, existing.ID).Error
				if err != nil {
					p.logger.Error("AutoLinker failed to insert edge", zap.Error(err))
				} else {
					linksAdded++
				}
			}
		}
	}

	if linksAdded > 0 {
		newContent := frontmatter + body
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			p.logger.Error("AutoLinker could not write file", zap.Error(err))
			return
		}
		if p.InvalidateGraphCache != nil {
			p.InvalidateGraphCache()
		}
		p.logger.Info("AutoLinker successfully linked article", zap.Int64("article_id", job.ArticleID), zap.Int("links_added", linksAdded))
	} else {
		p.logger.Info("AutoLinker found no concepts to link", zap.Int64("article_id", job.ArticleID))
	}
}
