package handlers

import (
	"path/filepath"
	"strings"
	"time"

	"example.com/backend/internal/auth"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func RegisterMaintenance(router fiber.Router, hCtx *HandlerContext) {
	router.Use("/maintenance", strictAuthMiddleware(hCtx))
	router.Post("/maintenance/backup", handleBackup(hCtx))
	router.Get("/maintenance/integrity", handleIntegrityAudit(hCtx))
}

func strictAuthMiddleware(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.SettingsStore != nil && h.SettingsStore.IsDegraded() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Configuration corrupted; access temporarily locked for security",
			})
		}
		token := ExtractSessionToken(c)
		if isSessionTokenRevoked(token) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Session revoked"})
		}
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		if strings.HasPrefix(token, "rdr_live_") {
			if h.Repo != nil {
				hash := auth.HashAPIKey(token)
				apiKey, err := h.Repo.FindAPIKeyByHash(c.Context(), hash)
				if err == nil && apiKey != nil {
					touchAPIKeyUsageAsync(h.Repo, apiKey.ID)
					return c.Next()
				}
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid API key"})
		}
		if h.SettingsStore == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Settings unavailable"})
		}
		current := h.SettingsStore.Get()
		valid, err := auth.VerifySession(current.SessionSecret, token, time.Now())
		if err != nil || !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Next()
	}
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
				"error": "failed to create backup",
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":      "success",
			"backup_file": filepath.Base(backupPath),
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
