package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHealthEndpoints(t *testing.T) {
	app := fiber.New()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})

	hCtx := &HandlerContext{
		DB:      db,
		Logger:  zap.NewNop(),
		DataDir: t.TempDir(),
	}
	RegisterHealth(app, hCtx)

	// Liveness: /healthz
	reqLiveness := httptest.NewRequest("GET", "/healthz", nil)
	respLiveness, err := app.Test(reqLiveness)
	if err != nil || respLiveness.StatusCode != http.StatusOK {
		t.Fatalf("expected /healthz 200 OK, got code %d, err: %v", respLiveness.StatusCode, err)
	}

	var liveBody map[string]interface{}
	if err := json.NewDecoder(respLiveness.Body).Decode(&liveBody); err != nil {
		t.Fatalf("failed to decode /healthz body: %v", err)
	}
	if liveBody["status"] != "alive" {
		t.Fatalf("expected live status 'alive', got %v", liveBody["status"])
	}

	// Readiness: /readyz
	reqReadiness := httptest.NewRequest("GET", "/readyz", nil)
	respReadiness, err := app.Test(reqReadiness)
	if err != nil || respReadiness.StatusCode != http.StatusOK {
		t.Fatalf("expected /readyz 200 OK, got code %d, err: %v", respReadiness.StatusCode, err)
	}

	var readyBody map[string]interface{}
	if err := json.NewDecoder(respReadiness.Body).Decode(&readyBody); err != nil {
		t.Fatalf("failed to decode /readyz body: %v", err)
	}
	if readyBody["status"] != "ready" {
		t.Fatalf("expected ready status 'ready', got %v", readyBody["status"])
	}
}

func TestHealthReadiness_ClosedDB(t *testing.T) {
	app := fiber.New()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	hCtx := &HandlerContext{
		DB:      db,
		Logger:  zap.NewNop(),
		DataDir: t.TempDir(),
	}
	RegisterHealth(app, hCtx)

	req := httptest.NewRequest("GET", "/readyz", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "unready" {
		t.Fatalf("expected status 'unready', got %v", body["status"])
	}
	if body["error"] != "database unreachable" {
		t.Fatalf("expected error 'database unreachable', got %v", body["error"])
	}
}

func TestHealthReadiness_UnwritableDataDir(t *testing.T) {
	app := fiber.New()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "regular_file")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Setting DataDir to a regular file causes filepath.Join(hCtx.DataDir, ".probe-write") to fail writing
	hCtx := &HandlerContext{
		DB:      db,
		Logger:  zap.NewNop(),
		DataDir: filePath,
	}
	RegisterHealth(app, hCtx)

	req := httptest.NewRequest("GET", "/readyz", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "unready" {
		t.Fatalf("expected status 'unready', got %v", body["status"])
	}
	if body["error"] != "data directory unwritable" {
		t.Fatalf("expected error 'data directory unwritable', got %v", body["error"])
	}
}
