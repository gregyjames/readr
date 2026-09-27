package handlers

import (
	"context"
	"os"
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
			if err != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Warn("Readiness check failed: database unreachable")
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "unready",
					"error":  "database unreachable",
				})
			}
			ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err != nil {
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
			f, err := os.CreateTemp(hCtx.DataDir, ".probe-write-*")
			if err != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Warn("Readiness check failed: data directory unwritable", zap.Error(err))
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "unready",
					"error":  "data directory unwritable",
				})
			}
			closeErr := f.Close()
			removeErr := os.Remove(f.Name())
			if closeErr != nil || removeErr != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Warn("Readiness check failed: data directory unwritable")
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "unready",
					"error":  "data directory unwritable",
				})
			}
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ready",
			"time":   time.Now().UTC(),
		})
	})
}
