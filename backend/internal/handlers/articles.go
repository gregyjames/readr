package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"example.com/backend/internal/agents"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type RequestBody struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
}

func RegisterArticles(router fiber.Router, h *HandlerContext) {
	router.Get("/getarticles", func(c *fiber.Ctx) error {
		var articles []repository.GormArticle
		if err := h.DB.Find(&articles).Error; err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to retrieve articles from DB", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
	})

	router.Get("/articles/:filename", func(c *fiber.Ctx) error {
		filename := c.Params("filename")
		clean := filepath.Clean(filename)
		filePath := filepath.Join(h.DataDir, "articles", clean)

		content, err := os.ReadFile(filePath)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Article not found")
		}
		c.Set("Content-Type", "text/markdown; charset=utf-8")
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		return c.Send(content)
	})

	router.Post("/add", func(c *fiber.Ctx) error {
		var body RequestBody
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to unmarshal request body", zap.Error(err))
			}
			return c.Status(400).SendString("Invalid JSON")
		}

		if h.Logger != nil {
			h.Logger.Info("Adding new article", zap.String("url", body.URL))
		}

		apiKey, model := h.SettingsStore.ExtractOpenRouterCredentials()

		article, err := h.Ingester.Ingest(c.Context(), ingest.IngestRequest{
			URL:      body.URL,
			Tags:     body.Tags,
			Template: body.Template,
			APIKey:   apiKey,
			Model:    model,
		})
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to ingest article", zap.String("url", body.URL), zap.Error(err))
			}
			if errors.Is(err, ingest.ErrDuplicateArticle) && article != nil {
				return c.JSON(fiber.Map{
					"status":  "exists",
					"message": "Article already exists",
					"id":      article.ID,
				})
			}
			if errors.Is(err, ingest.ErrEmptyURL) || errors.Is(err, ingest.ErrInvalidURL) {
				return c.Status(400).SendString("Invalid URL")
			}
			return c.Status(500).SendString("Failed to fetch the page")
		}

		SyncArticleToFTS(h.DB, article.ID, article.Title, article.Tags, h.Logger)

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		if h.Logger != nil {
			h.Logger.Info("Article added successfully", zap.Int64("id", article.ID), zap.String("url", body.URL))
		}

		settings := h.SettingsStore.Get()
		if settings.AgentEnricher {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeEnrichFrontmatter,
			})
		}
		if settings.AgentLinker {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeAutoLinker,
			})
		}
		if apiKey != "" && settings.AgentSummarizer {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeSummarizer,
			})
		}

		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "Article saved",
			"id":      article.ID,
		})
	})

	router.Delete("/delete/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if h.Logger != nil {
			h.Logger.Info("Attempting to delete article", zap.String("id", id))
		}

		if err := h.DB.Delete(&repository.GormArticle{}, id).Error; err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to delete article from DB", zap.String("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article",
			})
		}

		deleteFileError := os.Remove(filepath.Join(h.DataDir, "articles", fmt.Sprintf("%s.md", id)))
		if deleteFileError != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to delete article file", zap.String("id", id), zap.Error(deleteFileError))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article file",
			})
		}

		deleteImagesError := os.RemoveAll(filepath.Join(h.DataDir, "images", id))
		if deleteImagesError != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to delete article images", zap.String("id", id), zap.Error(deleteImagesError))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article images",
			})
		}

		DeleteArticleFromFTS(h.DB, id, h.Logger)
		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		if h.Logger != nil {
			h.Logger.Info("Article deleted successfully", zap.String("id", id))
		}
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": fmt.Sprintf("Article %s deleted", id),
		})
	})

	router.Post("/articles/:id/reparse", func(c *fiber.Ctx) error {
		idParam := c.Params("id")

		var article repository.GormArticle
		if err := h.DB.First(&article, idParam).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Article not found"})
		}

		apiKey, _ := h.SettingsStore.ExtractOpenRouterCredentials()
		settings := h.SettingsStore.Get()

		if settings.AgentEnricher {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeEnrichFrontmatter,
			})
		}
		if settings.AgentLinker {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeAutoLinker,
			})
		}
		if apiKey != "" && settings.AgentSummarizer {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeSummarizer,
			})
		}

		return c.JSON(fiber.Map{"status": "ok", "message": "Agents triggered"})
	})

	router.Post("/edit/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")

		var req struct {
			Content string `json:"content"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		var article repository.GormArticle
		if err := h.DB.First(&article, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
		}

		sourcePath := filepath.Join(h.DataDir, "articles", fmt.Sprintf("%s.md", id))
		if err := os.WriteFile(sourcePath, []byte(req.Content), 0644); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save article"})
		}

		SyncArticleToFTS(h.DB, article.ID, article.Title, article.Tags, h.Logger)

		// Sync links to database
		linkRegex := regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
		matches := linkRegex.FindAllStringSubmatch(req.Content, -1)

		// Delete existing outgoing links to prevent stale links
		h.DB.Where("source_id = ?", article.ID).Delete(&repository.GormArticleLink{})

		for _, match := range matches {
			targetTitle := strings.TrimSpace(match[1])
			var target repository.GormArticle
			if err := h.DB.Where("LOWER(title) = LOWER(?)", targetTitle).First(&target).Error; err == nil {
				// Prevent self-linking
				if article.ID != target.ID {
					link := repository.GormArticleLink{SourceID: article.ID, TargetID: target.ID}
					h.DB.Create(&link)
				}
			}
		}

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		return c.JSON(fiber.Map{"status": "success"})
	})
}
