package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/backend/internal/agents"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestRegisterSettings_ReconfiguresLibrarianCron(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	settingsStore := NewSettingsStore(tempDir, logger)
	runner := agents.NewLibrarianRunner(logger, nil, nil, tempDir, nil)
	cronManager := agents.NewLibrarianCronManager(runner, logger)
	t.Cleanup(cronManager.Stop)

	if err := cronManager.Start("0 0 * * *", true); err != nil {
		t.Fatalf("failed to start initial cron schedule: %v", err)
	}

	app := fiber.New()
	RegisterSettings(app, &HandlerContext{Logger: logger, SettingsStore: settingsStore}, cronManager)
	body := []byte(`{"librarian_enabled":true,"librarian_cron":"0 12 * * *"}`)
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/settings", bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("POST /settings failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	nextRun := cronManager.GetNextRun()
	if nextRun == nil {
		t.Fatal("expected the reconfigured Librarian schedule to have a next run")
	}
	if hour := nextRun.In(time.Local).Hour(); hour != 12 {
		t.Errorf("expected reconfigured schedule to run at hour 12, got %d", hour)
	}
}
