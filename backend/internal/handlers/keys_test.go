package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/backend/internal/auth"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupKeysTestApp(t *testing.T) (*fiber.App, *HandlerContext, *gorm.DB) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&repository.APIKey{}))

	repo := repository.NewGormRepository(db)
	settingsStore := NewSettingsStore(tempDir, zap.NewNop())

	hCtx := &HandlerContext{
		DB:            db,
		Logger:        zap.NewNop(),
		DataDir:       tempDir,
		SettingsStore: settingsStore,
		Repo:          repo,
	}

	app := fiber.New()
	api := app.Group("/api")
	api.Use(AuthMiddleware(hCtx))

	RegisterAuth(api, hCtx)
	RegisterKeys(api, hCtx)

	api.Get("/protected-resource", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	return app, hCtx, db
}

func TestAPIKeysCRUDAndAuth(t *testing.T) {
	app, hCtx, _ := setupKeysTestApp(t)

	// Set a vault password so AuthMiddleware is active
	hash, err := auth.HashPassword("secretpass123")
	require.NoError(t, err)
	_, err = hCtx.SettingsStore.Update(func(s *ServerSettings) error {
		s.PasswordHash = hash
		s.SessionSecret = "test-session-secret"
		return nil
	})
	require.NoError(t, err)

	sessionToken := auth.SignSession("test-session-secret", time.Now())

	// 1. Initial list of keys should be empty
	req := httptest.NewRequest("GET", "/api/keys", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var keysList []repository.APIKey
	require.NoError(t, json.Unmarshal(body, &keysList))
	assert.Empty(t, keysList)

	// 2. Create a new API key
	reqBody, _ := json.Marshal(map[string]string{"name": "Bookmarklet Key"})
	req = httptest.NewRequest("POST", "/api/keys", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var createdResp map[string]interface{}
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &createdResp))

	apiKeyStr, ok := createdResp["key"].(string)
	require.True(t, ok)
	assert.Contains(t, apiKeyStr, "rdr_live_")
	assert.Equal(t, "Bookmarklet Key", createdResp["name"])
	assert.Equal(t, apiKeyStr[:13]+"...", createdResp["key_prefix"])
	keyID := int64(createdResp["id"].(float64))

	// 3. List keys - should contain the new key with prefix, not raw key
	req = httptest.NewRequest("GET", "/api/keys", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &keysList))
	require.Len(t, keysList, 1)
	assert.Equal(t, keyID, keysList[0].ID)
	assert.Equal(t, "Bookmarklet Key", keysList[0].Name)
	assert.Equal(t, apiKeyStr[:13]+"...", keysList[0].KeyPrefix)
	assert.Nil(t, keysList[0].LastUsedAt)

	// 4. Access protected resource with Bearer API Key
	req = httptest.NewRequest("GET", "/api/protected-resource", nil)
	req.Header.Set("Authorization", "Bearer "+apiKeyStr)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 5. Access protected resource with X-API-Key header
	req = httptest.NewRequest("GET", "/api/protected-resource", nil)
	req.Header.Set("X-API-Key", apiKeyStr)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 6. Access protected resource with invalid API key
	req = httptest.NewRequest("GET", "/api/protected-resource", nil)
	req.Header.Set("Authorization", "Bearer rdr_live_invalidkey1234567890")
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	// 7. Delete the API key
	req = httptest.NewRequest("DELETE", "/api/keys/1", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 8. Access with deleted API key should now fail
	req = httptest.NewRequest("GET", "/api/protected-resource", nil)
	req.Header.Set("Authorization", "Bearer "+apiKeyStr)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}
