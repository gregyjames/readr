package handlers

import (
	"example.com/backend/internal/agents"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func RegisterLibrarian(router fiber.Router, h *HandlerContext, runner *agents.LibrarianRunner, cronManager *agents.LibrarianCronManager) {
	router.Get("/librarian/status", func(c *fiber.Ctx) error {
		settings := h.SettingsStore.Get()
		var nextRun = cronManager.GetNextRun()
		status := runner.GetStatus(settings.LibrarianEnabled, settings.LibrarianCron, settings.LibrarianMinClusterSize, nextRun)
		return c.JSON(status)
	})

	router.Post("/librarian/run", func(c *fiber.Ctx) error {
		if h.Logger != nil {
			h.Logger.Info("Manual Librarian execution requested")
		}

		result, err := runner.RunLibrarian(c.Context(), "manual")
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Manual Librarian execution failed", zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":  err.Error(),
				"result": result,
			})
		}

		return c.JSON(result)
	})
}
