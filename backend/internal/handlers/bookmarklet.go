package handlers

import (
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// RegisterBookmarklet mounts the GET /bookmarklet.js route.
func RegisterBookmarklet(app fiber.Router, h *HandlerContext) {
	app.Get("/bookmarklet.js", HandleGetBookmarklet(h))
}

// HandleGetBookmarklet serves the standalone bookmarklet script with appropriate CORS & Content-Type headers.
func HandleGetBookmarklet(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Set CORS and script content-type headers
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Set("Content-Type", "application/javascript; charset=utf-8")
		c.Set("Cache-Control", "no-cache, no-transform")

		candidates := []string{}
		if h.PublicDir != "" {
			candidates = append(candidates, filepath.Join(h.PublicDir, "bookmarklet.js"))
		}
		if h.DistDir != "" {
			candidates = append(candidates, filepath.Join(h.DistDir, "bookmarklet.js"))
		}

		// Check common deployment / development paths
		candidates = append(candidates,
			"../frontend/public/bookmarklet.js",
			"../frontend/dist/bookmarklet.js",
			"./dist/bookmarklet.js",
			"/app/dist/bookmarklet.js",
			"/app/public/bookmarklet.js",
			filepath.Join(h.DataDir, "bookmarklet.js"),
		)

		for _, cand := range candidates {
			if info, err := os.Stat(cand); err == nil && !info.IsDir() {
				return c.SendFile(cand)
			}
		}

		if h.Logger != nil {
			h.Logger.Warn("bookmarklet.js script file not found in candidates", zap.Strings("candidates", candidates))
		}
		return c.Status(fiber.StatusNotFound).SendString("// Readr bookmarklet script not found on server")
	}
}
