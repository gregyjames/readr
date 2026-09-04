package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"example.com/backend/internal/agents"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RequestBody struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
}

func dispatchArticleJobs(articleID int64, apiKey string, settings ServerSettings) {
	// If no agents are enabled, skip submission
	if !settings.AgentEnricher && !settings.AgentLinker && !settings.AgentSummarizer {
		return
	}

	agents.SubmitJob(agents.Job{
		ArticleID: articleID,
		Type:      agents.JobTypePipeline,
		Settings: agents.PipelineSettings{
			Summarizer: apiKey != "" && settings.AgentSummarizer,
			Enricher:   settings.AgentEnricher,
			Linker:     settings.AgentLinker,
		},
	})
}

// resolveArticleFilePath safely resolves a candidate filename or relative path strictly under dataDir/articles.
// It rejects absolute paths, empty paths, and any path containing ".." components or escaping the directory.
func resolveArticleFilePath(dataDir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("empty path")
	}

	// Reject absolute paths
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", errors.New("invalid absolute path")
	}

	clean := filepath.Clean(name)
	// Reject paths starting with ".." or equaling "." / ".."
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "..\\") {
		return "", errors.New("path traversal detected")
	}

	articlesDir := filepath.Join(dataDir, "articles")
	fullPath := filepath.Join(articlesDir, clean)

	// Ensure the resolved full path is strictly inside articlesDir
	rel, err := filepath.Rel(articlesDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.HasPrefix(rel, "../") {
		return "", errors.New("path outside articles directory")
	}

	return fullPath, nil
}

func resolveArticleFromRecord(dataDir, recordArticlePath string) (string, error) {
	rel := strings.TrimPrefix(recordArticlePath, "/")
	rel = strings.TrimPrefix(rel, "articles/")
	return resolveArticleFilePath(dataDir, rel)
}

func RegisterArticles(router fiber.Router, h *HandlerContext) {
	router.Get("/getarticles", func(c *fiber.Ctx) error {
		var articles []repository.GormArticle
		archivedParam := c.Query("archived")
		isArchived := archivedParam == "true"

		query := h.DB.Where("is_archived = ?", isArchived)
		if !isArchived {
			query = h.DB.Where("is_archived = ? OR is_archived IS NULL", false)
		}

		if err := query.Find(&articles).Error; err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to retrieve articles from DB", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
	})

	router.Get("/articles", func(c *fiber.Ctx) error {
		articles, err := h.Repo.GetAllArticles(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
	})

	router.Get("/articles/*", func(c *fiber.Ctx) error {
		filename := c.Params("*")
		if unescaped, err := url.PathUnescape(filename); err == nil && unescaped != "" {
			filename = unescaped
		} else if unescaped, err := url.QueryUnescape(filename); err == nil && unescaped != "" {
			filename = unescaped
		}

		// 1. Direct file lookup in data/articles/<clean>
		if targetPath, err := resolveArticleFilePath(h.DataDir, filename); err == nil {
			if content, err := os.ReadFile(targetPath); err == nil {
				c.Set("Content-Type", "text/markdown; charset=utf-8")
				c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
				return c.Send(content)
			}
		}

		// 2. Direct file lookup with .md appended
		if !strings.HasSuffix(filename, ".md") {
			if targetPath, err := resolveArticleFilePath(h.DataDir, filename+".md"); err == nil {
				if content, err := os.ReadFile(targetPath); err == nil {
					c.Set("Content-Type", "text/markdown; charset=utf-8")
					c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
					return c.Send(content)
				}
			}
		}

		// 3. Lookup by numeric ID (e.g. "123" or "123.md")
		numStr := strings.TrimSuffix(filepath.Base(filename), ".md")
		if id, err := strconv.ParseInt(numStr, 10, 64); err == nil && id > 0 {
			var a repository.GormArticle
			if err := h.DB.WithContext(c.Context()).Where("id = ? AND deleted_at IS NULL", id).First(&a).Error; err == nil {
				if a.Article != "" {
					if targetPath, err := resolveArticleFromRecord(h.DataDir, a.Article); err == nil {
						if content, err := os.ReadFile(targetPath); err == nil {
							c.Set("Content-Type", "text/markdown; charset=utf-8")
							c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
							return c.Send(content)
						}
					}
				}
			}
		}

		// 4. Lookup by Title in DB
		titleToLookup := strings.TrimSuffix(filepath.Base(filename), ".md")
		var a repository.GormArticle
		if err := h.DB.WithContext(c.Context()).Where("LOWER(title) = LOWER(?) AND deleted_at IS NULL", titleToLookup).First(&a).Error; err == nil {
			if a.Article != "" {
				if targetPath, err := resolveArticleFromRecord(h.DataDir, a.Article); err == nil {
					if content, err := os.ReadFile(targetPath); err == nil {
						c.Set("Content-Type", "text/markdown; charset=utf-8")
						c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
						return c.Send(content)
					}
				}
			}
		}

		return c.Status(fiber.StatusNotFound).SendString("Article not found")
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
		dispatchArticleJobs(article.ID, apiKey, settings)

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

		var article repository.GormArticle
		_ = h.DB.First(&article, id).Error

		err := h.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&repository.GormArticle{}, id).Error; err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM article_links WHERE source_id = ? OR target_id = ?", id, id).Error; err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to delete article from DB", zap.String("id", id), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article",
			})
		}

		if article.Article != "" {
			if p, err := resolveArticleFromRecord(h.DataDir, article.Article); err == nil {
				_ = os.Remove(p)
			}
		}
		if p, err := resolveArticleFilePath(h.DataDir, fmt.Sprintf("%s.md", id)); err == nil {
			_ = os.Remove(p)
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
		dispatchArticleJobs(article.ID, apiKey, settings)

		return c.JSON(fiber.Map{"status": "ok", "message": "Agents triggered"})
	})

	router.Post("/articles/:id/archive", func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || id <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		if h.Repo != nil {
			if err := h.Repo.SetArticleArchived(c.Context(), id, true); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
				}
				if h.Logger != nil {
					h.Logger.Error("Failed to archive article via repo", zap.Int64("id", id), zap.Error(err))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to archive article"})
			}
		} else {
			res := h.DB.WithContext(c.Context()).Model(&repository.GormArticle{}).Where("id = ?", id).Update("is_archived", true)
			if res.Error != nil {
				if h.Logger != nil {
					h.Logger.Error("Failed to archive article via DB", zap.Int64("id", id), zap.Error(res.Error))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to archive article"})
			}
			if res.RowsAffected == 0 {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
			}
		}

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		if h.EventHub != nil {
			h.EventHub.Broadcast("graph-updated")
		}

		return c.JSON(fiber.Map{
			"success":     true,
			"id":          id,
			"is_archived": true,
		})
	})

	router.Post("/articles/:id/unarchive", func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || id <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		if h.Repo != nil {
			if err := h.Repo.SetArticleArchived(c.Context(), id, false); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
				}
				if h.Logger != nil {
					h.Logger.Error("Failed to unarchive article via repo", zap.Int64("id", id), zap.Error(err))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unarchive article"})
			}
		} else {
			res := h.DB.WithContext(c.Context()).Model(&repository.GormArticle{}).Where("id = ?", id).Update("is_archived", false)
			if res.Error != nil {
				if h.Logger != nil {
					h.Logger.Error("Failed to unarchive article via DB", zap.Int64("id", id), zap.Error(res.Error))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unarchive article"})
			}
			if res.RowsAffected == 0 {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
			}
		}

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		if h.EventHub != nil {
			h.EventHub.Broadcast("graph-updated")
		}

		return c.JSON(fiber.Map{
			"success":     true,
			"id":          id,
			"is_archived": false,
		})
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

		sourcePath := ""
		if article.Article != "" {
			if p, err := resolveArticleFromRecord(h.DataDir, article.Article); err == nil {
				if _, err := os.Stat(p); err == nil {
					sourcePath = p
				}
			}
		}
		if sourcePath == "" {
			if p, err := resolveArticleFilePath(h.DataDir, fmt.Sprintf("%s.md", id)); err == nil {
				sourcePath = p
			}
		}
		if sourcePath == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid article file path"})
		}

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

	router.Post("/vault/clean-links", func(c *fiber.Ctx) error {
		res, err := CleanBrokenLinks(h.DB, h.DataDir, h.Logger)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to clean broken links in vault", zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to clean broken links in vault",
			})
		}

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		if h.EventHub != nil {
			h.EventHub.Broadcast("graph-updated")
		}

		return c.JSON(res)
	})
}
