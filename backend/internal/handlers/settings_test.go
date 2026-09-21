package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSettingsStoreGracefulDegradationOnCorruptFile(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()

	// 1. Initial creation writes default settings
	store := NewSettingsStore(tempDir, logger)
	initialSettings := store.Get()
	assert.NotEmpty(t, initialSettings.SessionSecret)
	assert.Equal(t, "openai/gpt-4o-mini", initialSettings.Model)

	// Verify file was written to disk
	settingsPath := filepath.Join(tempDir, "settings.json")
	require.FileExists(t, settingsPath)

	// 2. Corrupt the file with invalid JSON
	require.NoError(t, os.WriteFile(settingsPath, []byte("{invalid-json"), 0600))

	// Reload must NOT panic or os.Exit(1) via logger.Fatal; it must fall back to cached in-memory settings
	var reloaded ServerSettings
	assert.NotPanics(t, func() {
		reloaded = store.Reload()
	})
	assert.Equal(t, initialSettings.SessionSecret, reloaded.SessionSecret, "must fall back to cached settings on parse error")

	// 3. Remove file entirely (unreadable / missing)
	require.NoError(t, os.Remove(settingsPath))
	assert.NotPanics(t, func() {
		reloaded = store.Reload()
	})
	assert.NotEmpty(t, reloaded.SessionSecret, "must return valid settings without panicking")
}

func TestSettingsHTTPRouteWithCorruptFile(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()
	store := NewSettingsStore(tempDir, logger)

	app := fiber.New()
	api := app.Group("/api")
	hCtx := &HandlerContext{
		DataDir:       tempDir,
		Logger:        logger,
		SettingsStore: store,
	}
	RegisterSettings(api, hCtx)

	// Corrupt settings on disk
	settingsPath := filepath.Join(tempDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte("broken json content"), 0600))

	// Request to /api/settings must return 200 with fallback data, not crash
	req := httptest.NewRequest("GET", "/api/settings", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
