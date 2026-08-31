package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LinkRequest struct {
	SourceID     int64  `json:"sourceId"`
	TargetID     int64  `json:"targetId"`
	SelectedText string `json:"selectedText"`
}

type LinkError struct {
	StatusCode int
	Message    string
}

func (e *LinkError) Error() string {
	return e.Message
}

func LinkArticles(db *gorm.DB, dataDir string, req LinkRequest) (*repository.GormArticleLink, error) {
	if strings.TrimSpace(req.SelectedText) == "" {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "Selected text cannot be empty"}
	}
	if req.SourceID == 0 || req.TargetID == 0 {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "Source and target IDs are required"}
	}
	if req.SourceID == req.TargetID {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "An article cannot link to itself"}
	}

	// 1. Validate target article exists
	var target repository.GormArticle
	if err := db.First(&target, req.TargetID).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusNotFound, Message: "Target article not found"}
	}

	// 2. Validate source article exists
	var source repository.GormArticle
	if err := db.First(&source, req.SourceID).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusNotFound, Message: "Source article not found"}
	}

	// 3. Read and update markdown file
	sourcePath := filepath.Join(dataDir, "articles", fmt.Sprintf("%d.md", req.SourceID))
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not read source article"}
	}

	wikilink := fmt.Sprintf("[[%s|%s]]", target.Title, req.SelectedText)
	newContent := strings.Replace(string(content), req.SelectedText, wikilink, 1)

	if err := os.WriteFile(sourcePath, []byte(newContent), 0644); err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not update markdown"}
	}

	// 4. Save DB link only after file update succeeds
	link := repository.GormArticleLink{SourceID: req.SourceID, TargetID: req.TargetID}
	if err := db.Create(&link).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not create link"}
	}

	return &link, nil
}

func RegisterGraph(router fiber.Router, h *HandlerContext) {
	router.Post("/link", func(c *fiber.Ctx) error {
		var req LinkRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		link, err := LinkArticles(h.DB, h.DataDir, req)
		if err != nil {
			if linkErr, ok := err.(*LinkError); ok {
				return c.Status(linkErr.StatusCode).JSON(fiber.Map{"error": linkErr.Message})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		if h.GraphEngine != nil {
			h.GraphEngine.InvalidateCache()
		}
		return c.JSON(fiber.Map{"status": "success", "linkId": link.ID})
	})

	router.Get("/graph", func(c *fiber.Ctx) error {
		if h.GraphEngine == nil {
			return c.JSON(fiber.Map{"nodes": []any{}, "edges": []any{}})
		}
		graphData, err := h.GraphEngine.BuildGlobalGraph(c.Context())
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to fetch graph", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch graph"})
		}
		return c.JSON(graphData)
	})

	router.Get("/graph/local/:id", func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		var articleID int64
		if _, err := fmt.Sscanf(idParam, "%d", &articleID); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		if h.GraphEngine == nil {
			return c.JSON(fiber.Map{"nodes": []any{}, "edges": []any{}})
		}
		graphData, err := h.GraphEngine.BuildLocalGraph(c.Context(), articleID, 1)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to fetch local graph", zap.Int64("id", articleID), zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch local graph"})
		}
		return c.JSON(graphData)
	})
}
