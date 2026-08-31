package handlers

import (
	"example.com/backend/internal/agents"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
)

func GetPipelineDiagnostics(repo repository.Repository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		queueDepth := 0
		maxCap := 100
		activeWorkers := 1
		if agents.Pool != nil && agents.Pool.Queue != nil {
			queueDepth = len(agents.Pool.Queue)
			maxCap = cap(agents.Pool.Queue)
		}

		summary, recent, err := repo.GetPipelineDiagnostics(c.Context(), 50)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		if recent == nil {
			recent = []repository.PipelineMetric{}
		}

		return c.JSON(fiber.Map{
			"queue": fiber.Map{
				"pending_jobs":   queueDepth,
				"max_capacity":   maxCap,
				"active_workers": activeWorkers,
			},
			"summary":     summary,
			"recent_runs": recent,
		})
	}
}

func RegisterDiagnostics(router fiber.Router, hCtx *HandlerContext) {
	router.Get("/diagnostics/pipeline", GetPipelineDiagnostics(hCtx.Repo))
}
