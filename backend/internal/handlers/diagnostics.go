package handlers

import (
	"example.com/backend/internal/agents"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
)

func GetPipelineDiagnostics(repo repository.Repository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var qStatus agents.QueueStatus
		if agents.Pool != nil {
			qStatus = agents.Pool.GetQueueStatus()
		} else {
			qStatus = agents.QueueStatus{
				PendingJobs:   0,
				ActiveJobs:    0,
				TotalInFlight: 0,
				MaxCapacity:   100,
				TotalWorkers:  1,
				BusyWorkers:   0,
			}
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
			"queue":       qStatus,
			"summary":     summary,
			"recent_runs": recent,
		})
	}
}

func RegisterDiagnostics(router fiber.Router, hCtx *HandlerContext) {
	router.Get("/diagnostics/pipeline", GetPipelineDiagnostics(hCtx.Repo))
}
