package handlers

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// RegisterMaintenance registers database backup and vault integrity audit endpoints.
func RegisterMaintenance(router fiber.Router, hCtx *HandlerContext) {
	router.Post("/maintenance/backup", handleBackup(hCtx))
	router.Get("/maintenance/integrity", handleIntegrityAudit(hCtx))
}

func handleBackup(hCtx *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if hCtx == nil || hCtx.Maintenance == nil {
			if hCtx != nil && hCtx.Logger != nil {
				hCtx.Logger.Error("maintenance service not configured in handler context")
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "maintenance service not configured",
			})
		}

		backupPath, err := hCtx.Maintenance.CreateBackup(c.Context())
		if err != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Error("failed to create backup", zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":      "success",
			"backup_path": backupPath,
		})
	}
}

func handleIntegrityAudit(hCtx *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if hCtx == nil || hCtx.Maintenance == nil {
			if hCtx != nil && hCtx.Logger != nil {
				hCtx.Logger.Error("maintenance service not configured in handler context")
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "maintenance service not configured",
			})
		}

		report, err := hCtx.Maintenance.AuditIntegrity(c.Context())
		if err != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Error("failed to run integrity audit", zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		return c.Status(fiber.StatusOK).JSON(report)
	}
}
