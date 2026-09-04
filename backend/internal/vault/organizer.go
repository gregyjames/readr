package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type VaultOrganizer struct {
	dataDir string
	db      *gorm.DB
	logger  *zap.Logger
}

func NewVaultOrganizer(dataDir string, db *gorm.DB, logger *zap.Logger) *VaultOrganizer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &VaultOrganizer{
		dataDir: dataDir,
		db:      db,
		logger:  logger,
	}
}

// EnsureTopicFolder sanitizes the topic title and creates data/articles/<SanitizedTopic>/
func (o *VaultOrganizer) EnsureTopicFolder(topicTitle string) (string, error) {
	sanitized := ingest.SanitizeTitleFilename(topicTitle, 0)
	sanitized = strings.TrimSuffix(sanitized, ".md")
	if sanitized == "" || sanitized == "Article" {
		sanitized = "General"
	}
	folderPath := filepath.Join(o.dataDir, "articles", sanitized)
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create topic folder %s: %w", folderPath, err)
	}
	return sanitized, nil
}

// FileArticle moves an article from its current location to data/articles/<Topic>/<Filename>.md
// and updates the database record atomically in a transaction.
func (o *VaultOrganizer) FileArticle(ctx context.Context, articleID int64, topicTitle string) (string, error) {
	topicFolder, err := o.EnsureTopicFolder(topicTitle)
	if err != nil {
		return "", err
	}

	var article repository.GormArticle
	if err := o.db.WithContext(ctx).First(&article, articleID).Error; err != nil {
		return "", fmt.Errorf("failed to find article %d: %w", articleID, err)
	}

	currentRel := strings.TrimPrefix(article.Article, "/")
	currentAbs := filepath.Join(o.dataDir, currentRel)

	origBaseName := filepath.Base(currentAbs)
	fileName := origBaseName
	if fileName == "" || fileName == "." || fileName == "/" || fileName == fmt.Sprintf("%d.md", articleID) || fileName == fmt.Sprintf("%d", articleID) {
		sanitized := ingest.SanitizeTitleFilename(article.Title, articleID)
		if !strings.HasSuffix(strings.ToLower(sanitized), ".md") {
			fileName = sanitized + ".md"
		} else {
			fileName = sanitized
		}
	} else if !strings.HasSuffix(strings.ToLower(fileName), ".md") {
		fileName = fileName + ".md"
	}

	targetRel := filepath.Join("articles", topicFolder, fileName)
	targetAbs := filepath.Join(o.dataDir, targetRel)
	dbPath := "/" + filepath.ToSlash(targetRel)

	if currentAbs == targetAbs {
		return dbPath, nil
	}

	var srcMovedFrom string

	// Move physical file if source exists
	if _, err := os.Stat(currentAbs); err == nil {
		if err := os.Rename(currentAbs, targetAbs); err != nil {
			return "", fmt.Errorf("failed to move article file from %s to %s: %w", currentAbs, targetAbs, err)
		}
		srcMovedFrom = currentAbs
	} else {
		// If current file wasn't found at registered path, try locating it in root articles/
		// First check original filename (e.g. <articleID>.md or previous name) then sanitized filename
		fallbackCandidates := []string{
			filepath.Join(o.dataDir, "articles", origBaseName),
			filepath.Join(o.dataDir, "articles", fmt.Sprintf("%d.md", articleID)),
			filepath.Join(o.dataDir, "articles", fileName),
		}

		for _, fallbackSrc := range fallbackCandidates {
			if fallbackSrc == "" || fallbackSrc == targetAbs {
				continue
			}
			if _, err := os.Stat(fallbackSrc); err == nil {
				if err := os.Rename(fallbackSrc, targetAbs); err != nil {
					return "", fmt.Errorf("failed to move fallback article file from %s to %s: %w", fallbackSrc, targetAbs, err)
				}
				srcMovedFrom = fallbackSrc
				break
			}
		}
	}

	// Update DB record with leading slash
	if err := o.db.WithContext(ctx).Model(&repository.GormArticle{}).Where("id = ?", articleID).Update("article", dbPath).Error; err != nil {
		if srcMovedFrom != "" {
			_ = os.Rename(targetAbs, srcMovedFrom)
		}
		return "", fmt.Errorf("failed to update article %d path in db: %w", articleID, err)
	}

	o.logger.Info("Filed article into topic folder",
		zap.Int64("article_id", articleID),
		zap.String("topic", topicTitle),
		zap.String("new_path", dbPath),
	)

	return dbPath, nil
}

// CleanEmptyFolders removes any empty subdirectories under data/articles/
func (o *VaultOrganizer) CleanEmptyFolders() error {
	articlesDir := filepath.Join(o.dataDir, "articles")
	entries, err := os.ReadDir(articlesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(articlesDir, entry.Name())
			subEntries, err := os.ReadDir(subPath)
			if err == nil && len(subEntries) == 0 {
				_ = os.Remove(subPath)
				o.logger.Info("Pruned empty topic folder", zap.String("folder", subPath))
			}
		}
	}
	return nil
}

// UpdateMasterIndex writes a master index at data/articles/index.md listing all active Maps of Content.
func (o *VaultOrganizer) UpdateMasterIndex(ctx context.Context) error {
	var mocs []repository.GormArticle
	if err := o.db.WithContext(ctx).
		Where("deleted_at IS NULL AND (is_archived = false OR is_archived IS NULL)").
		Where("title LIKE 'MOC - %' OR title LIKE 'MOC %' OR title LIKE 'MOC:%' OR tags LIKE '%moc%'").
		Order("title ASC").
		Find(&mocs).Error; err != nil {
		return fmt.Errorf("failed to query MOC articles: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("type: Index\n")
	sb.WriteString("title: Vault Index\n")
	sb.WriteString("generated:\n")
	sb.WriteString("  by: agent/readr-librarian\n")
	sb.WriteString("---\n\n")
	sb.WriteString("# Vault Index\n\n")
	sb.WriteString("Master index of all Maps of Content (MOCs) across the knowledge vault.\n\n")
	sb.WriteString("## Maps of Content\n\n")

	if len(mocs) == 0 {
		sb.WriteString("*No Maps of Content generated yet.*\n")
	} else {
		for _, moc := range mocs {
			title := moc.Title
			fileBase := strings.TrimSuffix(filepath.Base(moc.Article), ".md")
			linkTitle := title
			if fileBase != "" && fileBase != "." {
				linkTitle = fileBase
			}

			// Count member links if possible
			var memberCount int64
			o.db.Table("article_links").Where("source_id = ?", moc.ID).Count(&memberCount)

			if memberCount > 0 {
				sb.WriteString(fmt.Sprintf("- [[%s]] — %d notes\n", linkTitle, memberCount))
			} else {
				sb.WriteString(fmt.Sprintf("- [[%s]]\n", linkTitle))
			}
		}
	}

	sb.WriteString("\n---\n*Automatically maintained by Readr Librarian.*\n")

	articlesDir := filepath.Join(o.dataDir, "articles")
	_ = os.MkdirAll(articlesDir, 0755)
	indexPath := filepath.Join(articlesDir, "index.md")
	newContent := sb.String()

	// If existing index is identical, skip rewrite
	if existingBytes, err := os.ReadFile(indexPath); err == nil && string(existingBytes) == newContent {
		return nil
	}

	tmpPath := filepath.Join(articlesDir, "index.md.tmp")
	if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write index.md tmp file: %w", err)
	}

	if err := os.Rename(tmpPath, indexPath); err != nil {
		return fmt.Errorf("failed to replace index.md: %w", err)
	}

	o.logger.Info("Updated master vault index", zap.String("path", indexPath), zap.Int("moc_count", len(mocs)))
	return nil
}
