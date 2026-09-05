package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStatusTestApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&repository.GormArticle{},
		&repository.GormArticleLink{},
		&repository.GormArticleStatusType{},
		&repository.GormArticleStatus{},
	); err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}
	if err := repository.EnsureArticleStatusTypes(db); err != nil {
		t.Fatalf("failed to seed status types: %v", err)
	}
	if err := db.Create(&repository.GormArticle{ID: 101, Title: "Active Article", Article: "101.md"}).Error; err != nil {
		t.Fatalf("failed to seed article: %v", err)
	}

	hCtx := &HandlerContext{
		DB:      db,
		DataDir: tempDir,
		Repo:    repository.NewGormRepository(db),
		Logger:  zap.NewNop(),
	}

	app := fiber.New()
	RegisterArticles(app, hCtx)
	RegisterArticleStatus(app, hCtx)

	return app, db
}

func postJSON(t *testing.T, app *fiber.App, path string, body string) (*http.Response, map[string]interface{}) {
	t.Helper()

	req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	decoded := map[string]interface{}{}
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp, decoded
}

func TestRecordProgressHandler(t *testing.T) {
	app, db := setupStatusTestApp(t)

	t.Run("records progress and derives not_finished", func(t *testing.T) {
		resp, body := postJSON(t, app, "/articles/101/progress", `{"progress": 42.5}`)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if body["reading_status"] != repository.StatusNotFinished {
			t.Errorf("reading_status = %v, want %q", body["reading_status"], repository.StatusNotFinished)
		}
		if body["reading_progress"] != 42.5 {
			t.Errorf("reading_progress = %v, want 42.5", body["reading_progress"])
		}

		var status repository.GormArticleStatus
		if err := db.Where("article_id = ?", 101).First(&status).Error; err != nil {
			t.Fatalf("status row was not persisted: %v", err)
		}
		if status.Progress != 42.5 {
			t.Errorf("persisted progress = %v, want 42.5", status.Progress)
		}
	})

	t.Run("crossing the threshold derives finished", func(t *testing.T) {
		_, body := postJSON(t, app, "/articles/101/progress", `{"progress": 99}`)
		if body["reading_status"] != repository.StatusFinished {
			t.Errorf("reading_status = %v, want %q", body["reading_status"], repository.StatusFinished)
		}
	})

	t.Run("progress does not regress", func(t *testing.T) {
		_, body := postJSON(t, app, "/articles/101/progress", `{"progress": 10}`)
		if body["reading_progress"] != 99.0 {
			t.Errorf("reading_progress = %v, want it held at 99", body["reading_progress"])
		}
	})

	t.Run("unknown article returns 404", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/999/progress", `{"progress": 10}`)
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("non-numeric id returns 400", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/abc/progress", `{"progress": 10}`)
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("out of range progress returns 400", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/101/progress", `{"progress": 150}`)
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing progress field returns 400", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/101/progress", `{}`)
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("malformed json returns 400", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/101/progress", `{"progress":`)
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestManualStatusHandlers(t *testing.T) {
	app, db := setupStatusTestApp(t)

	t.Run("marking finished overrides progress", func(t *testing.T) {
		postJSON(t, app, "/articles/101/progress", `{"progress": 12}`)

		resp, body := postJSON(t, app, "/articles/101/status", `{"status": "finished"}`)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if body["reading_status"] != repository.StatusFinished {
			t.Errorf("reading_status = %v, want %q", body["reading_status"], repository.StatusFinished)
		}

		// A later low-progress ping must not undo the manual override.
		_, after := postJSON(t, app, "/articles/101/progress", `{"progress": 3}`)
		if after["reading_status"] != repository.StatusFinished {
			t.Errorf("manual finish was overwritten: %v", after["reading_status"])
		}
	})

	t.Run("reset clears the row", func(t *testing.T) {
		resp, body := postJSON(t, app, "/articles/101/status/reset", ``)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if body["reading_status"] != repository.StatusNotStarted {
			t.Errorf("reading_status = %v, want %q", body["reading_status"], repository.StatusNotStarted)
		}

		var count int64
		db.Model(&repository.GormArticleStatus{}).Where("article_id = ?", 101).Count(&count)
		if count != 0 {
			t.Errorf("expected the status row to be gone, found %d", count)
		}
	})

	t.Run("invalid status key returns 400", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/101/status", `{"status": "sideways"}`)
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("unknown article returns 404", func(t *testing.T) {
		resp, _ := postJSON(t, app, "/articles/999/status", `{"status": "finished"}`)
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestGetArticlesHydratesReadingStatus(t *testing.T) {
	app, _ := setupStatusTestApp(t)

	t.Run("never opened article reports not_started", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/getarticles", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("GET /getarticles failed: %v", err)
		}
		defer resp.Body.Close()

		var list []repository.GormArticle
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 article, got %d", len(list))
		}
		if list[0].ReadingStatus != repository.StatusNotStarted {
			t.Errorf("ReadingStatus = %q, want %q", list[0].ReadingStatus, repository.StatusNotStarted)
		}
	})

	t.Run("read article reports progress", func(t *testing.T) {
		postJSON(t, app, "/articles/101/progress", `{"progress": 60}`)

		req := httptest.NewRequest("GET", "/getarticles", nil)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("GET /getarticles failed: %v", err)
		}
		defer resp.Body.Close()

		var list []repository.GormArticle
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if list[0].ReadingStatus != repository.StatusNotFinished {
			t.Errorf("ReadingStatus = %q, want %q", list[0].ReadingStatus, repository.StatusNotFinished)
		}
		if list[0].ReadingProgress != 60 {
			t.Errorf("ReadingProgress = %v, want 60", list[0].ReadingProgress)
		}
	})
}

func TestDeleteArticleRemovesStatusRow(t *testing.T) {
	app, db := setupStatusTestApp(t)

	postJSON(t, app, "/articles/101/progress", `{"progress": 55}`)

	req := httptest.NewRequest("DELETE", "/delete/101", nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var count int64
	db.Model(&repository.GormArticleStatus{}).Where("article_id = ?", 101).Count(&count)
	if count != 0 {
		t.Errorf("orphaned status row left behind after delete: %d", count)
	}
}
