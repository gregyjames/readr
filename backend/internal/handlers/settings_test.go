package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example.com/backend/internal/llm"
	"github.com/gofiber/fiber/v3"
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

func TestSettingsStoreColdStartCorruptFile_BlocksAuth(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()

	// Pre-create a corrupt settings.json before NewSettingsStore (cold start)
	settingsPath := filepath.Join(tempDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte("invalid-json-content"), 0600))

	store := NewSettingsStore(tempDir, logger)
	assert.True(t, store.IsDegraded(), "cold start on corrupt settings must be marked degraded")

	// Set up App with AuthMiddleware
	hCtx := &HandlerContext{
		DataDir:       tempDir,
		Logger:        logger,
		SettingsStore: store,
	}

	app := fiber.New()
	api := app.Group("/api")
	api.Use(AuthMiddleware(hCtx))

	api.Get("/protected-vault-data", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": "secret"})
	})

	// Request without auth must NOT bypass authentication; it must receive 503
	req := httptest.NewRequest("GET", "/api/protected-vault-data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "must deny access when configuration is degraded")
}

func TestSettingsUpdateWithoutContentType(t *testing.T) {
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

	req := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(`{"theme":"dark","model":"custom/test-model"}`))
	// Omit Content-Type header
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	current := store.Get()
	assert.Equal(t, "custom/test-model", current.Model)
}

func TestSettingsLegacyDefaultsAndLLMConfig(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()

	// Write legacy settings file without provider_type or provider_base_url
	settingsPath := filepath.Join(tempDir, "settings.json")
	legacyJSON := `{"api_key":"sk-legacy-123","model":"openai/gpt-4o-mini"}`
	require.NoError(t, os.WriteFile(settingsPath, []byte(legacyJSON), 0600))

	store := NewSettingsStore(tempDir, logger)
	current := store.Get()

	assert.Equal(t, "openrouter", current.ProviderType)
	assert.Equal(t, "https://openrouter.ai/api/v1", current.ProviderBaseURL)
	assert.Equal(t, "sk-legacy-123", current.APIKey)
	assert.Equal(t, "openai/gpt-4o-mini", current.Model)

	// Verify GetLLMConfig() returns expected values
	llmCfg := store.GetLLMConfig()
	assert.Equal(t, llm.Config{
		Type:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-legacy-123",
	}, llmCfg)

	// Verify ExtractOpenRouterCredentials() backward compatibility
	apiKey, model := store.ExtractOpenRouterCredentials()
	assert.Equal(t, "sk-legacy-123", apiKey)
	assert.Equal(t, "openai/gpt-4o-mini", model)
}

func TestSettingsCustomProviderSaveAndReload_Update(t *testing.T) {
	tempDir := t.TempDir()
	logger := zap.NewNop()

	store := NewSettingsStore(tempDir, logger)

	// Save custom provider "ollama" with custom base url via Update
	_, err := store.Update(func(current *ServerSettings) error {
		current.ProviderType = "ollama"
		current.ProviderBaseURL = "http://localhost:11434/v1"
		current.APIKey = "ollama-dummy-key"
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, llm.Config{
		Type:    "ollama",
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "ollama-dummy-key",
	}, store.GetLLMConfig())

	// Reload from new store instance and verify persisted
	reloadedStore := NewSettingsStore(tempDir, logger)
	assert.Equal(t, "ollama", reloadedStore.Get().ProviderType)
	assert.Equal(t, "http://localhost:11434/v1", reloadedStore.Get().ProviderBaseURL)
	assert.Equal(t, "ollama-dummy-key", reloadedStore.Get().APIKey)
	assert.Equal(t, llm.Config{
		Type:    "ollama",
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "ollama-dummy-key",
	}, reloadedStore.GetLLMConfig())
}

func TestSettingsCustomProviderHTTP_PostAndGet(t *testing.T) {
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

	// 1. POST custom provider settings
	body := `{"provider_type":"ollama","provider_base_url":"http://localhost:11434/v1","api_key":"test-key","model":"llama3"}`
	req := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify store state
	assert.Equal(t, "ollama", store.Get().ProviderType)
	assert.Equal(t, "http://localhost:11434/v1", store.Get().ProviderBaseURL)
	assert.Equal(t, "test-key", store.Get().APIKey)
	assert.Equal(t, "llama3", store.Get().Model)

	// 2. GET settings and verify JSON response
	getReq := httptest.NewRequest("GET", "/api/settings", nil)
	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var getResult map[string]interface{}
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)
	assert.Equal(t, "ollama", getResult["provider_type"])
	assert.Equal(t, "http://localhost:11434/v1", getResult["provider_base_url"])
	assert.Equal(t, "test-key", getResult["api_key"])
	assert.Equal(t, "llama3", getResult["model"])

	// 3. POST with provider_type "openai" and empty base_url should resolve to OpenAI default URL
	bodyOpenAI := `{"provider_type":"openai","provider_base_url":"","api_key":"sk-openai"}`
	req2 := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(bodyOpenAI))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	assert.Equal(t, "openai", store.Get().ProviderType)
	assert.Equal(t, "https://api.openai.com/v1", store.Get().ProviderBaseURL)

	// 4. POST with empty provider_type should default to "openrouter"
	bodyEmpty := `{"provider_type":"","provider_base_url":""}`
	req3 := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(bodyEmpty))
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	assert.Equal(t, "openrouter", store.Get().ProviderType)
	assert.Equal(t, "https://openrouter.ai/api/v1", store.Get().ProviderBaseURL)
}
