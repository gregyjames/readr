package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CacheInvalidator is an interface for notifying subsystems (like graph cache) of data mutations.
type CacheInvalidator interface {
	InvalidateCache()
}

// NoteInput captures the parameters required to save or update an article in the vault.
type NoteInput struct {
	ID        int64
	Title     string
	Content   string
	Tags      string
	ImagePath string
	URL       string
	Topic     string
}

// ArticleFilter specifies querying criteria for articles in the vault.
type ArticleFilter struct {
	Archived *bool
	Tag      string
	Topic    string
	Limit    int
	Offset   int
}

// Vault defines the deep domain boundary for all note storage, retrieval, lifecycle, and filesystem operations.
type Vault interface {
	// Lifecycle mutations
	SaveArticle(ctx context.Context, input NoteInput) (*repository.GormArticle, error)
	DeleteArticle(ctx context.Context, id int64) error
	SetArchived(ctx context.Context, id int64, archived bool) error
	MoveArticle(ctx context.Context, id int64, topicTitle string) (string, error)

	// Read queries
	GetArticle(ctx context.Context, id int64) (*repository.GormArticle, string, error)
	ListArticles(ctx context.Context, filter ArticleFilter) ([]repository.GormArticle, error)

	// Path and organizer utilities
	ResolveFilePath(recordArticlePath string) (string, error)
	Organizer() *VaultOrganizer
}

// DefaultVault is the primary implementation of the Vault interface.
type DefaultVault struct {
	dataDir     string
	db          *gorm.DB
	logger      *zap.Logger
	organizer   *VaultOrganizer
	invalidator CacheInvalidator
}

// NewVault creates a new DefaultVault instance.
func NewVault(dataDir string, db *gorm.DB, logger *zap.Logger, invalidator CacheInvalidator) *DefaultVault {
	if logger == nil {
		logger = zap.NewNop()
	}
	organizer := NewVaultOrganizer(dataDir, db, logger)
	return &DefaultVault{
		dataDir:     dataDir,
		db:          db,
		logger:      logger,
		organizer:   organizer,
		invalidator: invalidator,
	}
}

func (v *DefaultVault) Organizer() *VaultOrganizer {
	return v.organizer
}

// ResolveFilePath safely resolves an article's recorded relative path (e.g. "/articles/Topic/Note.md")
// to an absolute disk path, preventing directory traversal.
func (v *DefaultVault) ResolveFilePath(recordArticlePath string) (string, error) {
	rel := strings.TrimSpace(recordArticlePath)
	if rel == "" {
		return "", errors.New("empty path")
	}

	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimPrefix(rel, "articles/")

	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "..\\") {
		return "", errors.New("path traversal detected")
	}

	articlesDir := filepath.Join(v.dataDir, "articles")
	fullPath := filepath.Join(articlesDir, clean)

	checkRel, err := filepath.Rel(articlesDir, fullPath)
	if err != nil || checkRel == ".." || strings.HasPrefix(checkRel, ".."+string(filepath.Separator)) || strings.HasPrefix(checkRel, "../") {
		return "", errors.New("path outside articles directory")
	}

	return fullPath, nil
}

// SaveArticle atomically writes markdown to disk and updates SQLite records & FTS indexes.
func (v *DefaultVault) SaveArticle(ctx context.Context, input NoteInput) (*repository.GormArticle, error) {
	if strings.TrimSpace(input.Title) == "" {
		return nil, errors.New("article title cannot be empty")
	}

	topicFolder := ""
	if input.Topic != "" {
		var err error
		topicFolder, err = v.organizer.EnsureTopicFolder(input.Topic)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure topic folder: %w", err)
		}
	}

	var article repository.GormArticle
	isNew := input.ID <= 0

	// 1. Allocate / fetch DB record to establish the unique article ID
	if isNew {
		article = repository.GormArticle{
			Title: input.Title,
			Tags:  input.Tags,
		}
		if input.ImagePath != "" {
			article.Image = input.ImagePath
		}
		if err := v.db.WithContext(ctx).Create(&article).Error; err != nil {
			return nil, fmt.Errorf("failed to allocate article record in db: %w", err)
		}
	} else {
		if err := v.db.WithContext(ctx).First(&article, input.ID).Error; err != nil {
			article = repository.GormArticle{ID: input.ID}
		}
	}

	sanitizedTitle := ingest.SanitizeTitleFilename(input.Title, article.ID)
	var relPath string
	var absPath string

	if topicFolder != "" {
		relPath = fmt.Sprintf("/articles/%s/%s", topicFolder, sanitizedTitle)
		absPath = filepath.Join(v.dataDir, "articles", topicFolder, sanitizedTitle)
	} else {
		relPath = fmt.Sprintf("/articles/%s", sanitizedTitle)
		absPath = filepath.Join(v.dataDir, "articles", sanitizedTitle)
	}

	// 2. Write markdown file atomically
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		if isNew {
			_ = v.db.WithContext(ctx).Delete(&repository.GormArticle{}, article.ID)
		}
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf("%s.tmp", filepath.Base(absPath)))
	if err := os.WriteFile(tmpFile, []byte(input.Content), 0644); err != nil {
		if isNew {
			_ = v.db.WithContext(ctx).Delete(&repository.GormArticle{}, article.ID)
		}
		return nil, fmt.Errorf("failed to write tmp markdown: %w", err)
	}
	if err := os.Rename(tmpFile, absPath); err != nil {
		_ = os.Remove(tmpFile)
		if isNew {
			_ = v.db.WithContext(ctx).Delete(&repository.GormArticle{}, article.ID)
		}
		return nil, fmt.Errorf("failed to commit markdown file: %w", err)
	}

	// 3. Persist updated metadata and sync FTS
	err := v.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		article.Title = input.Title
		article.Tags = input.Tags
		article.Article = relPath
		if input.ImagePath != "" {
			article.Image = input.ImagePath
		}

		if err := tx.Save(&article).Error; err != nil {
			return err
		}

		// Sync FTS
		_ = tx.Exec("DELETE FROM articles_fts WHERE rowid = ?", article.ID).Error
		_ = tx.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", article.ID, article.Title, input.Content).Error

		return nil
	})

	if err != nil {
		_ = os.Remove(absPath)
		if isNew {
			_ = v.db.WithContext(ctx).Delete(&repository.GormArticle{}, article.ID)
		}
		return nil, fmt.Errorf("failed to save article record in db: %w", err)
	}

	if v.invalidator != nil {
		v.invalidator.InvalidateCache()
	}
	_ = v.organizer.UpdateMasterIndex(ctx)

	return &article, nil
}

// DeleteArticle performs a full atomic deletion of an article across database, relations, FTS, disk, and images.
func (v *DefaultVault) DeleteArticle(ctx context.Context, id int64) error {
	v.logger.Info("Executing Vault DeleteArticle", zap.Int64("id", id))

	var article repository.GormArticle
	_ = v.db.WithContext(ctx).First(&article, id).Error

	// 1. Delete DB record, links, and FTS within a transaction
	err := v.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&repository.GormArticle{}, id).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM article_links WHERE source_id = ? OR target_id = ?", id, id).Error; err != nil {
			return err
		}
		if err := repository.DeleteStatus(tx, id); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM articles_fts WHERE rowid = ?", id).Error; err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
				return err
			}
		}
		return nil
	})

	if err != nil {
		v.logger.Error("Failed to delete article in database transaction", zap.Int64("id", id), zap.Error(err))
		return fmt.Errorf("database deletion failed: %w", err)
	}

	var cleanupErr error

	// 2. Remove markdown file from disk
	if article.Article != "" {
		if p, err := v.ResolveFilePath(article.Article); err == nil {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				v.logger.Error("Failed to remove article file from disk", zap.String("path", p), zap.Error(err))
				cleanupErr = fmt.Errorf("failed to remove article file: %w", err)
			}
		}
	}
	// Fallback check in case path was stored as id.md
	if p, err := v.ResolveFilePath(fmt.Sprintf("%d.md", id)); err == nil {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) && cleanupErr == nil {
			v.logger.Error("Failed to remove fallback article file from disk", zap.String("path", p), zap.Error(err))
			cleanupErr = fmt.Errorf("failed to remove fallback article file: %w", err)
		}
	}

	// 3. Remove image attachments directory
	imageDir := filepath.Join(v.dataDir, "images", fmt.Sprint(id))
	if err := os.RemoveAll(imageDir); err != nil && !os.IsNotExist(err) {
		v.logger.Error("Failed to delete article images directory", zap.Int64("id", id), zap.Error(err))
		if cleanupErr == nil {
			cleanupErr = fmt.Errorf("failed to delete article images directory: %w", err)
		}
	}

	// 4. Invalidate graph cache and master index
	if v.invalidator != nil {
		v.invalidator.InvalidateCache()
	}
	_ = v.organizer.UpdateMasterIndex(ctx)

	if cleanupErr != nil {
		return fmt.Errorf("article deleted from database but cleanup failed: %w", cleanupErr)
	}

	return nil
}

// SetArchived updates the archived state of an article, updating master index and graph cache.
func (v *DefaultVault) SetArchived(ctx context.Context, id int64, archived bool) error {
	v.logger.Info("Executing Vault SetArchived", zap.Int64("id", id), zap.Bool("archived", archived))

	res := v.db.WithContext(ctx).Model(&repository.GormArticle{}).Where("id = ?", id).Update("is_archived", archived)
	if res.Error != nil {
		return fmt.Errorf("failed to set article archive state: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	if v.invalidator != nil {
		v.invalidator.InvalidateCache()
	}
	_ = v.organizer.UpdateMasterIndex(ctx)

	return nil
}

// MoveArticle relocates an article into a topic folder and updates database and master index.
func (v *DefaultVault) MoveArticle(ctx context.Context, id int64, topicTitle string) (string, error) {
	relPath, err := v.organizer.FileArticle(ctx, id, topicTitle)
	if err != nil {
		return "", err
	}
	if v.invalidator != nil {
		v.invalidator.InvalidateCache()
	}
	return relPath, nil
}

// GetArticle returns the database record and the raw markdown content from disk.
func (v *DefaultVault) GetArticle(ctx context.Context, id int64) (*repository.GormArticle, string, error) {
	var article repository.GormArticle
	if err := v.db.WithContext(ctx).First(&article, id).Error; err != nil {
		return nil, "", err
	}

	absPath, err := v.ResolveFilePath(article.Article)
	if err != nil {
		return &article, "", fmt.Errorf("failed to resolve article path: %w", err)
	}

	bytes, err := os.ReadFile(absPath)
	if err != nil {
		return &article, "", fmt.Errorf("failed to read article file %s: %w", absPath, err)
	}

	article.WordCount, article.ReadingTime = repository.CalculateReadingTime(string(bytes))
	return &article, string(bytes), nil
}

// ListArticles retrieves articles according to the provided filter options.
func (v *DefaultVault) ListArticles(ctx context.Context, filter ArticleFilter) ([]repository.GormArticle, error) {
	var articles []repository.GormArticle
	query := v.db.WithContext(ctx).Model(&repository.GormArticle{}).Where("deleted_at IS NULL")

	if filter.Archived != nil {
		if *filter.Archived {
			query = query.Where("is_archived = ?", true)
		} else {
			query = query.Where("is_archived = ? OR is_archived IS NULL", false)
		}
	}

	if filter.Tag != "" {
		query = query.Where("tags LIKE ?", "%"+filter.Tag+"%")
	}

	if filter.Topic != "" {
		query = query.Where("article LIKE ?", "%/articles/"+filter.Topic+"/%")
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	if err := query.Find(&articles).Error; err != nil {
		return nil, err
	}

	if err := v.hydrateReadingStatus(ctx, articles); err != nil {
		v.logger.Error("Failed to hydrate reading status", zap.Error(err))
	}
	v.hydrateReadingTime(articles)

	return articles, nil
}

func (v *DefaultVault) hydrateReadingTime(articles []repository.GormArticle) {
	for i := range articles {
		filePath, err := v.ResolveFilePath(articles[i].Article)
		if err != nil {
			articles[i].ReadingTime = "1 min read"
			continue
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			articles[i].ReadingTime = "1 min read"
			continue
		}
		words, rt := repository.CalculateReadingTime(string(content))
		articles[i].WordCount = words
		articles[i].ReadingTime = rt
	}
}

func (v *DefaultVault) hydrateReadingStatus(ctx context.Context, articles []repository.GormArticle) error {
	if len(articles) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(articles))
	for _, article := range articles {
		ids = append(ids, article.ID)
	}

	statuses, err := repository.GetStatuses(ctx, v.db, ids)
	if err != nil {
		return err
	}

	for i := range articles {
		status, ok := statuses[articles[i].ID]
		if !ok {
			articles[i].ReadingStatus = repository.StatusNotStarted
			continue
		}
		articles[i].ReadingStatus = repository.DeriveStatusKey(&status)
		articles[i].ReadingProgress = status.Progress
	}
	return nil
}
