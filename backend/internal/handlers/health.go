package handlers

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func RegisterHealth(app *fiber.App, hCtx *HandlerContext) {
	// Liveness Probe: lightweight check that HTTP server is receiving and answering traffic
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "alive",
			"time":   time.Now().UTC(),
		})
	})

	// Readiness Probe: checks SQLite query responsiveness and vault filesystem writability
	app.Get("/readyz", func(c *fiber.Ctx) error {
		// 1. Verify Database
		if hCtx != nil && hCtx.DB != nil {
			sqlDB, err := hCtx.DB.DB()
			if err != nil || sqlDB.Ping() != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Warn("Readiness check failed: database unreachable")
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "unready",
					"error":  "database unreachable",
				})
			}
		}

		// 2. Verify Data Directory Writability
		if hCtx != nil && hCtx.DataDir != "" {
			testFile := filepath.Join(hCtx.DataDir, ".probe-write")
			if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Warn("Readiness check failed: data directory unwritable", zap.Error(err))
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "unready",
					"error":  "data directory unwritable",
				})
			}
			_ = os.Remove(testFile)
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ready",
			"time":   time.Now().UTC(),
		})
	})
}
