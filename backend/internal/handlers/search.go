package handlers

import (
	"strings"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SearchResult struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
}

func EnsureFTS(db *gorm.DB, logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
		title, 
		content, 
		tokenize='porter'
	)`).Error
	if err != nil {
		logger.Error("Failed to create FTS5 table", zap.Error(err))
		return
	}

	var articleCount, indexedCount int64
	db.Model(&repository.GormArticle{}).Count(&articleCount)
	if err := db.Raw("SELECT count(*) FROM articles_fts").Scan(&indexedCount).Error; err != nil {
		logger.Error("Failed to count FTS5 rows", zap.Error(err))
		return
	}
	if articleCount == indexedCount {
		return
	}

	var articles []repository.GormArticle
	if err := db.Find(&articles).Error; err != nil {
		logger.Error("Failed to load articles for FTS5 rebuild", zap.Error(err))
		return
	}

	db.Exec("DELETE FROM articles_fts")
	for _, article := range articles {
		SyncArticleToFTS(db, article.ID, article.Title, article.Tags, logger)
	}
	logger.Info("Rebuilt FTS5 index", zap.Int64("articles", articleCount), zap.Int64("previouslyIndexed", indexedCount))
}

func SyncArticleToFTS(db *gorm.DB, id int64, title string, tags string, logger *zap.Logger) {
	db.Exec("DELETE FROM articles_fts WHERE rowid = ?", id)
	err := db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", id, title, tags).Error
	if err != nil && logger != nil {
		logger.Error("Failed to sync article to FTS5", zap.Int64("id", id), zap.Error(err))
	}
}

func DeleteArticleFromFTS(db *gorm.DB, id string, logger *zap.Logger) {
	err := db.Exec("DELETE FROM articles_fts WHERE rowid = ?", id).Error
	if err != nil && logger != nil {
		logger.Error("Failed to delete article from FTS5", zap.String("id", id), zap.Error(err))
	}
}

func RegisterSearch(router fiber.Router, h *HandlerContext) {
	router.Get("/search", func(c *fiber.Ctx) error {
		results := make([]SearchResult, 0)

		cleanQuery := strings.ReplaceAll(c.Query("q"), "\"", "")
		cleanQuery = strings.ReplaceAll(cleanQuery, "'", "")
		cleanQuery = strings.ReplaceAll(cleanQuery, "*", "")

		parts := strings.Fields(cleanQuery)
		if len(parts) == 0 {
			return c.JSON(results)
		}
		for i, p := range parts {
			parts[i] = p + "*"
		}
		safeQuery := strings.Join(parts, " OR ")

		err := h.DB.Raw(`
			SELECT rowid as id, title, snippet(articles_fts, 1, '<mark>', '</mark>', '...', 25) as excerpt
			FROM articles_fts
			WHERE articles_fts MATCH ?
			ORDER BY bm25(articles_fts, 1.0, 3.0)
			LIMIT 15
		`, safeQuery).Scan(&results).Error

		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("FTS search failed", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Search failed"})
		}

		return c.JSON(results)
	})
}
