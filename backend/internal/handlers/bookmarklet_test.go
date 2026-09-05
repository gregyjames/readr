package handlers

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBookmarkletEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	publicDir := filepath.Join(tempDir, "public")
	require.NoError(t, os.MkdirAll(publicDir, 0755))

	bookmarkletContent := `(function(){ console.log("Readr Bookmarklet Loaded"); })();`
	bookmarkletPath := filepath.Join(publicDir, "bookmarklet.js")
	require.NoError(t, os.WriteFile(bookmarkletPath, []byte(bookmarkletContent), 0644))

	app := fiber.New()
	hCtx := &HandlerContext{
		Logger:  zap.NewNop(),
		DataDir: tempDir,
		DistDir: publicDir,
	}

	RegisterBookmarklet(app, hCtx)

	req := httptest.NewRequest("GET", "/bookmarklet.js", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "javascript")
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, bookmarkletContent, string(body))
}
