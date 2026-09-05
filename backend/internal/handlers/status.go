package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// parseArticleID reads and validates the :id route parameter.
func parseArticleID(c *fiber.Ctx) (int64, bool) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// statusRepo resolves a repository for reading-status writes, tolerating a
// HandlerContext that was built with only a DB (as the tests do).
func statusRepo(h *HandlerContext) *repository.GormRepository {
	if h.Repo != nil {
		return h.Repo
	}
	if h.DB != nil {
		return repository.NewGormRepository(h.DB)
	}
	return nil
}

// hydrateReadingStatus fills the non-persisted reading status fields on a slice
// of articles. Used on the Vault-less path; the Vault hydrates its own results.
func hydrateReadingStatus(ctx context.Context, h *HandlerContext, articles []repository.GormArticle) {
	if len(articles) == 0 || h.DB == nil {
		return
	}

	ids := make([]int64, 0, len(articles))
	for _, article := range articles {
		ids = append(ids, article.ID)
	}

	statuses, err := repository.GetStatuses(ctx, h.DB, ids)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("Failed to load reading statuses", zap.Error(err))
		}
		return
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
}

// articleExists reports whether an article is present and not soft-deleted, so
// progress cannot be recorded against a missing id.
func articleExists(h *HandlerContext, id int64) (bool, error) {
	var count int64
	if err := h.DB.Model(&repository.GormArticle{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func statusResponse(id int64, status *repository.GormArticleStatus) fiber.Map {
	return fiber.Map{
		"success":          true,
		"id":               id,
		"reading_status":   repository.DeriveStatusKey(status),
		"reading_progress": progressOf(status),
	}
}

func progressOf(status *repository.GormArticleStatus) float64 {
	if status == nil {
		return 0
	}
	return status.Progress
}

// RegisterArticleStatus wires the reading status routes.
//
// These are all POST by necessity: the wildcard GET /articles/* route in
// RegisterArticles would otherwise swallow any GET /articles/:id/<sub>.
func RegisterArticleStatus(router fiber.Router, h *HandlerContext) {
	router.Post("/articles/:id/progress", func(c *fiber.Ctx) error {
		id, ok := parseArticleID(c)
		if !ok {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		var body struct {
			Progress *float64 `json:"progress"`
		}
		if err := json.Unmarshal(c.Body(), &body); err != nil || body.Progress == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid progress payload"})
		}
		if *body.Progress < 0 || *body.Progress > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "Progress must be between 0 and 100"})
		}

		repo := statusRepo(h)
		if repo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to record progress"})
		}

		exists, err := articleExists(h, id)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to check article existence", zap.Int64("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to record progress"})
		}
		if !exists {
			return c.Status(404).JSON(fiber.Map{"error": "Article not found"})
		}

		status, err := repo.RecordProgress(c.Context(), id, *body.Progress)
		if err != nil {
			if errors.Is(err, repository.ErrInvalidStatus) {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid progress value"})
			}
			if h.Logger != nil {
				h.Logger.Error("Failed to record reading progress", zap.Int64("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to record progress"})
		}

		return c.JSON(statusResponse(id, status))
	})

	router.Post("/articles/:id/status", func(c *fiber.Ctx) error {
		id, ok := parseArticleID(c)
		if !ok {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		var body struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid status payload"})
		}
		if !repository.IsValidStatusKey(body.Status) {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid reading status"})
		}

		repo := statusRepo(h)
		if repo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to set reading status"})
		}

		exists, err := articleExists(h, id)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to check article existence", zap.Int64("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to set reading status"})
		}
		if !exists {
			return c.Status(404).JSON(fiber.Map{"error": "Article not found"})
		}

		status, err := repo.SetManualStatus(c.Context(), id, body.Status)
		if err != nil {
			if errors.Is(err, repository.ErrInvalidStatus) {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid reading status"})
			}
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, repository.ErrNotFound) {
				return c.Status(404).JSON(fiber.Map{"error": "Article not found"})
			}
			if h.Logger != nil {
				h.Logger.Error("Failed to set reading status", zap.Int64("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to set reading status"})
		}

		return c.JSON(statusResponse(id, status))
	})

	router.Post("/articles/:id/status/reset", func(c *fiber.Ctx) error {
		id, ok := parseArticleID(c)
		if !ok {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		repo := statusRepo(h)
		if repo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to reset reading status"})
		}

		if err := repo.ClearStatus(c.Context(), id); err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to reset reading status", zap.Int64("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to reset reading status"})
		}

		return c.JSON(statusResponse(id, nil))
	})
}
