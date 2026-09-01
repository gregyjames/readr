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

	fileName := filepath.Base(currentAbs)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = ingest.SanitizeTitleFilename(article.Title, articleID)
	}

	targetRel := filepath.Join("articles", topicFolder, fileName)
	targetAbs := filepath.Join(o.dataDir, targetRel)

	if currentAbs == targetAbs {
		return targetRel, nil
	}

	// Move physical file if source exists
	if _, err := os.Stat(currentAbs); err == nil {
		if err := os.Rename(currentAbs, targetAbs); err != nil {
			return "", fmt.Errorf("failed to move article file from %s to %s: %w", currentAbs, targetAbs, err)
		}
	} else {
		// If current file wasn't found at registered path, try locating it in root articles/
		fallbackSrc := filepath.Join(o.dataDir, "articles", fileName)
		if _, err := os.Stat(fallbackSrc); err == nil {
			if err := os.Rename(fallbackSrc, targetAbs); err != nil {
				return "", fmt.Errorf("failed to move fallback article file from %s to %s: %w", fallbackSrc, targetAbs, err)
			}
		}
	}

	// Update DB record with leading slash
	dbPath := "/" + filepath.ToSlash(targetRel)
	if err := o.db.WithContext(ctx).Model(&repository.GormArticle{}).Where("id = ?", articleID).Update("article", dbPath).Error; err != nil {
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
