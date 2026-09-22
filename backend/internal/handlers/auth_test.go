package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func setupAuthTestApp(t *testing.T) (*fiber.App, *HandlerContext) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "auth_test.db")), &gorm.Config{})
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

	api.Get("/protected", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	return app, hCtx
}

func TestAuthLogoutDoesNotInvalidateOtherSessions(t *testing.T) {
	app, hCtx := setupAuthTestApp(t)

	// Configure master password
	hash, err := auth.HashPassword("masterpassword123")
	require.NoError(t, err)
	var secret string
	_, err = hCtx.SettingsStore.Update(func(s *ServerSettings) error {
		s.PasswordHash = hash
		s.SessionSecret, _ = auth.GenerateRandomSecret()
		secret = s.SessionSecret
		return nil
	})
	require.NoError(t, err)

	// Generate tokens for two distinct sessions (e.g., Device A and Device B)
	tokenA := auth.SignSession(secret, time.Now().Add(-time.Minute))
	tokenB := auth.SignSession(secret, time.Now())

	// Verify both tokens access protected resource
	reqA := httptest.NewRequest("GET", "/api/protected", nil)
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	respA, err := app.Test(reqA)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respA.StatusCode)

	reqB := httptest.NewRequest("GET", "/api/protected", nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	respB, err := app.Test(reqB, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respB.StatusCode)

	// 1. Calling /api/auth/logout without credentials succeeds in clearing cookie
	unauthLogoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	unauthLogoutResp, err := app.Test(unauthLogoutReq, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, unauthLogoutResp.StatusCode, "logout succeeds to clear cookie")

	// 2. Device A logs out with its valid token
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+tokenA)
	logoutResp, err := app.Test(logoutReq, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode)
	// Verify cookie clearing
	cookies := logoutResp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "readr_session" {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "readr_session clearing cookie must be present")
	assert.True(t, sessionCookie.Value == "" || sessionCookie.Expires.Before(time.Now()) || sessionCookie.MaxAge < 0, "session cookie must be expired or emptied")

	// 3. Verify Device A's tokenA is now rejected with 401
	reqAAfter := httptest.NewRequest("GET", "/api/protected", nil)
	reqAAfter.Header.Set("Authorization", "Bearer "+tokenA)
	respAAfter, err := app.Test(reqAAfter, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, respAAfter.StatusCode, "revoked tokenA must receive 401")

	// 3. Device B's session MUST still be valid (no global session invalidation DoS)
	reqBAfter := httptest.NewRequest("GET", "/api/protected", nil)
	reqBAfter.Header.Set("Authorization", "Bearer "+tokenB)
	respBAfter, err := app.Test(reqBAfter, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respBAfter.StatusCode, "Device B session must remain valid after Device A logs out")
	// Verify the global secret did not change
	currentSecret := hCtx.SettingsStore.Get().SessionSecret
	assert.Equal(t, secret, currentSecret, "SessionSecret should not be rotated on logout")
}

func TestAuthLoginWorkflow(t *testing.T) {
	app, hCtx := setupAuthTestApp(t)

	// Configure master password
	hash, err := auth.HashPassword("masterpassword123")
	require.NoError(t, err)
	_, err = hCtx.SettingsStore.Update(func(s *ServerSettings) error {
		s.PasswordHash = hash
		s.SessionSecret, _ = auth.GenerateRandomSecret()
		return nil
	})
	require.NoError(t, err)

	// Failed login
	badBody, _ := json.Marshal(map[string]string{"password": "wrongpassword"})
	badReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(badBody))
	badReq.Header.Set("Content-Type", "application/json")
	badResp, err := app.Test(badReq, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, badResp.StatusCode)

	// Successful login
	goodBody, _ := json.Marshal(map[string]string{"password": "masterpassword123"})
	goodReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(goodBody))
	goodReq.Header.Set("Content-Type", "application/json")
	goodResp, err := app.Test(goodReq, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, goodResp.StatusCode)

	var resData map[string]interface{}
	require.NoError(t, json.NewDecoder(goodResp.Body).Decode(&resData))
	token, ok := resData["token"].(string)
	require.True(t, ok && token != "")

	// Protected access with token
	protReq := httptest.NewRequest("GET", "/api/protected", nil)
	protReq.Header.Set("Authorization", "Bearer "+token)
	protResp, err := app.Test(protReq, 10000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, protResp.StatusCode)
}
