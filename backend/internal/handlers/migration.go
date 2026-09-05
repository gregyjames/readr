package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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

// MigrateLegacyArticleTags scans all articles in SQLite and sanitizes any legacy tags with spaces into Obsidian-compatible kebab-case tags, updating both the database and the markdown files on disk.
func MigrateLegacyArticleTags(db *gorm.DB, dataDir string, logger *zap.Logger) (int, error) {
	if db == nil {
		return 0, nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	var articles []repository.GormArticle
	if err := db.Where("deleted_at IS NULL AND tags != '' AND tags IS NOT NULL").Find(&articles).Error; err != nil {
		return 0, fmt.Errorf("failed to fetch articles for tag migration: %w", err)
	}

	migratedCount := 0
	for _, a := range articles {
		sanitizedTags := repository.SanitizeObsidianTags(strings.Split(a.Tags, ","))
		sanitizedStr := strings.Join(sanitizedTags, ", ")

		if sanitizedStr == a.Tags {
			continue
		}

		if err := db.Model(&repository.GormArticle{}).Where("id = ?", a.ID).Update("tags", sanitizedStr).Error; err != nil {
			logger.Warn("Failed to update sanitized tags in database", zap.Int64("id", a.ID), zap.Error(err))
			continue
		}

		// Update markdown frontmatter on disk if file exists
		if a.Article != "" {
			filePath := filepath.Join(dataDir, strings.TrimPrefix(a.Article, "/"))
			if contentBytes, err := os.ReadFile(filePath); err == nil {
				content := string(contentBytes)
				if strings.HasPrefix(content, "---\n") {
					parts := strings.SplitN(content[4:], "\n---\n", 2)
					if len(parts) == 2 {
						frontmatterRaw := parts[0]
						body := parts[1]

						var rawMap map[string]interface{}
						if err := yaml.Unmarshal([]byte(frontmatterRaw), &rawMap); err == nil && rawMap != nil {
							rawMap["tags"] = sanitizedTags
							if newYaml, err := yaml.Marshal(rawMap); err == nil {
								newDoc := "---\n" + string(newYaml) + "---\n" + body
								_ = os.WriteFile(filePath, []byte(newDoc), 0644)
							}
						}
					}
				}
			}
		}

		migratedCount++
		logger.Info("Migrated article tags to Obsidian format",
			zap.Int64("id", a.ID),
			zap.String("old_tags", a.Tags),
			zap.String("new_tags", sanitizedStr),
		)
	}

	return migratedCount, nil
}

// MigrateLegacyWordCounts scans all articles in SQLite without a word_count. If the corresponding markdown
// file exists on disk, it calculates and stores the word count in the database.
func MigrateLegacyWordCounts(db *gorm.DB, dataDir string, logger *zap.Logger) (int, error) {
	if db == nil {
		return 0, nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	var articles []repository.GormArticle
	if err := db.Where("deleted_at IS NULL AND (word_count = 0 OR word_count IS NULL)").Find(&articles).Error; err != nil {
		return 0, fmt.Errorf("failed to fetch articles for word count migration: %w", err)
	}

	migratedCount := 0
	for _, a := range articles {
		if a.Article == "" {
			continue
		}

		filePath, err := resolveArticleFromRecord(dataDir, a.Article)
		if err != nil {
			continue
		}

		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		words, _ := repository.CalculateReadingTime(string(contentBytes))
		if words > 0 {
			if err := db.Model(&repository.GormArticle{}).Where("id = ?", a.ID).Update("word_count", words).Error; err != nil {
				logger.Warn("Failed to update word_count in database", zap.Int64("id", a.ID), zap.Error(err))
				continue
			}
			migratedCount++
		}
	}

	if migratedCount > 0 {
		logger.Info("Migrated article word counts", zap.Int("count", migratedCount))
	}

	return migratedCount, nil
}
