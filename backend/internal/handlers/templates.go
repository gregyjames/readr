package handlers

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func RegisterTemplates(router fiber.Router, h *HandlerContext) {
	router.Get("/templates", func(c *fiber.Ctx) error {
		if h.TemplateRenderer == nil {
			return c.JSON([]any{})
		}
		templates, err := h.TemplateRenderer.ListTemplates()
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to list templates", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to list templates"})
		}
		return c.JSON(templates)
	})
}
