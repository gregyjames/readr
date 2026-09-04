package handlers

import (
	"strconv"
	"strings"

	"example.com/backend/internal/auth"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
)

type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

func RegisterKeys(router fiber.Router, h *HandlerContext) {
	router.Get("/keys", HandleListAPIKeys(h))
	router.Post("/keys", HandleCreateAPIKey(h))
	router.Delete("/keys/:id", HandleDeleteAPIKey(h))
}

func HandleListAPIKeys(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.Repo == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Repository not configured"})
		}
		keys, err := h.Repo.ListAPIKeys(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list API keys"})
		}
		if keys == nil {
			keys = []repository.APIKey{}
		}
		return c.JSON(keys)
	}
}

func HandleCreateAPIKey(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.Repo == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Repository not configured"})
		}

		var req CreateAPIKeyRequest
		if err := c.BodyParser(&req); err != nil {
			// allow empty body
			req.Name = ""
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "API Key"
		}

		fullKey, prefix, hash, err := auth.GenerateAPIKey()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate API key"})
		}

		apiKey := repository.APIKey{
			Name:      name,
			KeyHash:   hash,
			KeyPrefix: prefix,
		}

		if err := h.Repo.CreateAPIKey(c.Context(), &apiKey); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to persist API key"})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"id":         apiKey.ID,
			"name":       apiKey.Name,
			"key":        fullKey,
			"key_prefix": apiKey.KeyPrefix,
			"created_at": apiKey.CreatedAt,
		})
	}
}

func HandleDeleteAPIKey(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.Repo == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Repository not configured"})
		}

		idParam := c.Params("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || id <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid API key ID"})
		}

		if err := h.Repo.DeleteAPIKey(c.Context(), id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete API key"})
		}

		return c.JSON(fiber.Map{"status": "success"})
	}
}
