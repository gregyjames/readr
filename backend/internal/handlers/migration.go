package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MigrateLegacyArticleFilenames scans all articles in SQLite. If an article points to a numeric path like "/articles/123.md"
// and the corresponding file exists on disk, it renames the file to the title-based filename and updates the database.
func MigrateLegacyArticleFilenames(db *gorm.DB, dataDir string, logger *zap.Logger) (int, error) {
	if db == nil {
		return 0, nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	articlesDir := filepath.Join(dataDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)

	var articles []repository.GormArticle
	if err := db.Where("deleted_at IS NULL").Find(&articles).Error; err != nil {
		return 0, fmt.Errorf("failed to fetch articles for migration: %w", err)
	}

	migratedCount := 0
	for _, a := range articles {
		if a.Title == "" {
			continue
		}

		currentRel := a.Article
		currentBase := filepath.Base(currentRel)
		currentPath := filepath.Join(dataDir, strings.TrimPrefix(currentRel, "/"))

		// Check if it's a numeric legacy filename (e.g. "123.md" or "1725134892100.md")
		nameOnly := strings.TrimSuffix(currentBase, ".md")
		isNumeric := len(nameOnly) > 0
		for _, r := range nameOnly {
			if r < '0' || r > '9' {
				isNumeric = false
				break
			}
		}

		if !isNumeric {
			continue
		}

		// Check if the legacy file exists
		if _, err := os.Stat(currentPath); err != nil {
			continue
		}

		// Generate new title-based filename
		targetBase := ingest.SanitizeTitleFilename(a.Title, a.ID)
		targetPath := filepath.Join(articlesDir, targetBase)

		// Collision check
		counter := 1
		nameWithoutExt := strings.TrimSuffix(targetBase, ".md")
		for {
			if targetPath == currentPath {
				break
			}
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				break
			}
			targetBase = fmt.Sprintf("%s (%d).md", nameWithoutExt, counter)
			targetPath = filepath.Join(articlesDir, targetBase)
			counter++
		}

		if targetPath != currentPath {
			if err := os.Rename(currentPath, targetPath); err != nil {
				logger.Warn("Failed to rename legacy article file", zap.Int64("id", a.ID), zap.String("from", currentPath), zap.String("to", targetPath), zap.Error(err))
				continue
			}
		}

		newRel := fmt.Sprintf("/articles/%s", targetBase)
		if err := db.Model(&repository.GormArticle{}).Where("id = ?", a.ID).Update("article", newRel).Error; err != nil {
			logger.Warn("Failed to update article path in database", zap.Int64("id", a.ID), zap.Error(err))
			continue
		}

		migratedCount++
		logger.Info("Migrated article file to title-based name",
			zap.Int64("id", a.ID),
			zap.String("title", a.Title),
			zap.String("new_file", targetBase),
		)
	}

	return migratedCount, nil
}
