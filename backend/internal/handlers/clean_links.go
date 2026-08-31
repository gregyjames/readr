package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CleanLinksResult struct {
	Status          string   `json:"status"`
	ScannedArticles int      `json:"scanned_articles"`
	UpdatedArticles int      `json:"updated_articles"`
	CleanedLinks    int      `json:"cleaned_links"`
	PurgedDBLinks   int64    `json:"purged_db_links"`
	Errors          []string `json:"errors,omitempty"`
}

var wikilinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// CleanBrokenLinks scans all articles in the vault, converts broken wikilinks to clean plain text
// (preserving the original sentence phrasing), and purges orphaned rows in article_links.
func CleanBrokenLinks(db *gorm.DB, dataDir string, logger *zap.Logger) (*CleanLinksResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	result := &CleanLinksResult{
		Status: "success",
	}

	// 1. Fetch all active (non-deleted) articles
	var articles []repository.GormArticle
	if err := db.Where("deleted_at IS NULL").Find(&articles).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch articles: %w", err)
	}

	result.ScannedArticles = len(articles)

	// 2. Build target vocabulary index
	validTargets := make(map[string]int64)
	activeIDs := make(map[int64]bool)

	for _, a := range articles {
		activeIDs[a.ID] = true
		validTargets[strings.ToLower(strings.TrimSpace(a.Title))] = a.ID
		validTargets[fmt.Sprintf("%d", a.ID)] = a.ID
		validTargets[fmt.Sprintf("%d.md", a.ID)] = a.ID

		if a.Article != "" {
			base := strings.TrimSuffix(filepath.Base(a.Article), ".md")
			validTargets[strings.ToLower(strings.TrimSpace(base))] = a.ID
			validTargets[strings.ToLower(strings.TrimSpace(filepath.Base(a.Article)))] = a.ID
		}
	}

	var writeErrors []string

	// 3. Scan each article's file and clean broken links
	for _, a := range articles {
		filePath := ""
		var fileInfo os.FileInfo
		if a.Article != "" {
			candidate := filepath.Join(dataDir, strings.TrimPrefix(a.Article, "/"))
			if info, err := os.Stat(candidate); err == nil {
				filePath = candidate
				fileInfo = info
			}
		}
		if filePath == "" {
			candidate := filepath.Join(dataDir, "articles", fmt.Sprintf("%d.md", a.ID))
			if info, err := os.Stat(candidate); err == nil {
				filePath = candidate
				fileInfo = info
			}
		}

		if filePath == "" {
			continue
		}

		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warn("Could not read article file during cleanup", zap.Int64("id", a.ID), zap.String("path", filePath), zap.Error(err))
			continue
		}

		content := string(contentBytes)
		articleModified := false
		linksCleanedInFile := 0

		newContent := wikilinkRegex.ReplaceAllStringFunc(content, func(match string) string {
			sub := wikilinkRegex.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}

			target := strings.TrimSpace(sub[1])
			var display string
			if len(sub) >= 3 && sub[2] != "" {
				display = strings.TrimSpace(sub[2])
			} else {
				display = target
			}

			// Check if target is valid
			normTarget := strings.ToLower(target)
			if _, exists := validTargets[normTarget]; exists {
				return match // Valid link, keep as is
			}

			// Broken link: replace with display text (preserving prose)
			articleModified = true
			linksCleanedInFile++
			return display
		})

		if articleModified {
			fileMode := os.FileMode(0644)
			if fileInfo != nil {
				fileMode = fileInfo.Mode().Perm()
			} else if info, err := os.Stat(filePath); err == nil {
				fileMode = info.Mode().Perm()
			}

			dir := filepath.Dir(filePath)
			tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(filePath)))
			if err := os.WriteFile(tmpFile, []byte(newContent), fileMode); err != nil {
				logger.Error("Failed to write temporary file during link cleanup",
					zap.Int64("id", a.ID),
					zap.String("path", tmpFile),
					zap.Error(err),
				)
				writeErrors = append(writeErrors, fmt.Sprintf("article %d (%s): write failed: %v", a.ID, a.Title, err))
				continue
			}

			if err := os.Rename(tmpFile, filePath); err != nil {
				_ = os.Remove(tmpFile)
				logger.Error("Failed to rename temporary file during link cleanup",
					zap.Int64("id", a.ID),
					zap.String("path", filePath),
					zap.Error(err),
				)
				writeErrors = append(writeErrors, fmt.Sprintf("article %d (%s): rename failed: %v", a.ID, a.Title, err))
				continue
			}

			result.UpdatedArticles++
			result.CleanedLinks += linksCleanedInFile
			logger.Info("Cleaned broken links in article",
				zap.Int64("id", a.ID),
				zap.String("title", a.Title),
				zap.Int("cleaned_count", linksCleanedInFile),
			)
		}
	}

	if len(writeErrors) > 0 {
		result.Errors = writeErrors
		if result.UpdatedArticles == 0 {
			result.Status = "failed"
		} else {
			result.Status = "partial_failure"
		}
	}

	// 4. Purge dangling rows in article_links
	var activeIDList []int64
	for id := range activeIDs {
		activeIDList = append(activeIDList, id)
	}

	if len(activeIDList) > 0 {
		res := db.Exec("DELETE FROM article_links WHERE source_id NOT IN (?) OR target_id NOT IN (?)", activeIDList, activeIDList)
		if res.Error != nil {
			logger.Error("Failed to purge dangling links from database", zap.Error(res.Error))
			return result, fmt.Errorf("failed to purge dangling article links: %w", res.Error)
		}
		result.PurgedDBLinks = res.RowsAffected
	} else {
		res := db.Exec("DELETE FROM article_links")
		if res.Error != nil {
			logger.Error("Failed to purge dangling links from database", zap.Error(res.Error))
			return result, fmt.Errorf("failed to purge dangling article links: %w", res.Error)
		}
		result.PurgedDBLinks = res.RowsAffected
	}

	if len(writeErrors) > 0 && result.UpdatedArticles == 0 {
		return result, fmt.Errorf("failed to clean links in articles: %s", strings.Join(writeErrors, "; "))
	}

	return result, nil
}
