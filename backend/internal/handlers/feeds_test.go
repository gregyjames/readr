package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const mockRSSXML = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <title>Mock RSS Feed</title>
  <link>https://example.com</link>
  <description>A test feed</description>
  <item>
    <title>Post 1</title>
    <link>https://example.com/post-1</link>
    <description>Description 1</description>
    <pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate>
  </item>
  <item>
    <title>Post 2</title>
    <link>https://example.com/post-2</link>
    <description>Description 2</description>
    <pubDate>Tue, 03 Jan 2006 15:04:05 MST</pubDate>
  </item>
</channel>
</rss>`

func setupFeedTestApp(t *testing.T) (*fiber.App, *HandlerContext, *gorm.DB) {
	tempDir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(tempDir, "feeds_test.db")), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&repository.GormRssFeed{}))

	repo := repository.NewGormRepository(db)
	hCtx := &HandlerContext{
		DB:      db,
		Logger:  zap.NewNop(),
		DataDir: tempDir,
		Repo:    repo,
	}

	app := fiber.New()
	api := app.Group("/api")

	api.Get("/feeds", GetFeeds(hCtx))
	api.Post("/feeds", AddFeed(hCtx))
	api.Delete("/feeds/:id", RemoveFeed(hCtx))
	api.Get("/feeds/timeline", GetTimeline(hCtx))

	return app, hCtx, db
}

func TestFeedsEndpoints(t *testing.T) {
	app, _, db := setupFeedTestApp(t)

	// Mock RSS HTTP Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRSSXML))
	}))
	defer mockServer.Close()

	// 1. Initial GET /api/feeds should be empty array
	req := httptest.NewRequest("GET", "/api/feeds", nil)
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var feeds []repository.GormRssFeed
	require.NoError(t, json.Unmarshal(body, &feeds))
	assert.Empty(t, feeds)
	assert.Equal(t, "[]", string(bytes.TrimSpace(body)))

	// 2. Initial GET /api/feeds/timeline should return empty array [] (never null)
	req = httptest.NewRequest("GET", "/api/feeds/timeline", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(bytes.TrimSpace(body)))

	// 3. POST /api/feeds with invalid URL should fail with 400
	invalidBody, _ := json.Marshal(map[string]string{"url": "not-a-valid-feed-url"})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(invalidBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 4. POST /api/feeds with empty URL should fail with 400
	emptyBody, _ := json.Marshal(map[string]string{"url": ""})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(emptyBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 5. POST /api/feeds adds a valid feed
	addBody, _ := json.Marshal(map[string]string{"url": mockServer.URL})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var createdFeed repository.GormRssFeed
	require.NoError(t, json.Unmarshal(body, &createdFeed))
	assert.Equal(t, mockServer.URL, createdFeed.URL)
	assert.Equal(t, "Mock RSS Feed", createdFeed.Title)
	assert.Equal(t, "https://example.com", createdFeed.SiteURL)
	feedID := createdFeed.ID

	// 5b. POST /api/feeds with same URL should return 409 Conflict
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
	conflictBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(conflictBody), "feed already exists")

	// 6. GET /api/feeds lists the created feed
	req = httptest.NewRequest("GET", "/api/feeds", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	feeds = nil
	require.NoError(t, json.Unmarshal(body, &feeds))
	require.Len(t, feeds, 1)
	assert.Equal(t, feedID, feeds[0].ID)
	assert.Equal(t, "Mock RSS Feed", feeds[0].Title)

	// 7. GET /api/feeds/timeline returns items from the feed
	req = httptest.NewRequest("GET", "/api/feeds/timeline", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var timeline []ingest.TimelineItem
	require.NoError(t, json.Unmarshal(body, &timeline))
	require.Len(t, timeline, 2)
	assert.Equal(t, "Post 2", timeline[0].Title) // Sorted desc by pubDate
	assert.Equal(t, "Post 1", timeline[1].Title)
	assert.Equal(t, feedID, timeline[0].FeedID)
	assert.Equal(t, "Mock RSS Feed", timeline[0].FeedTitle)

	// 8. GET /api/feeds/timeline?feed_id=X returns items for specific feed
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/feeds/timeline?feed_id=%d", feedID), nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var singleTimeline []ingest.TimelineItem
	require.NoError(t, json.Unmarshal(body, &singleTimeline))
	require.Len(t, singleTimeline, 2)

	// 9. GET /api/feeds/timeline?feed_id=999 returns 404 for non-existent feed
	req = httptest.NewRequest("GET", "/api/feeds/timeline?feed_id=999", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// 10. GET /api/feeds/timeline?feed_id=invalid returns 400
	req = httptest.NewRequest("GET", "/api/feeds/timeline?feed_id=invalid", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 11. DELETE /api/feeds/invalid returns 400
	req = httptest.NewRequest("DELETE", "/api/feeds/invalid", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 12. DELETE /api/feeds/999 returns 404
	req = httptest.NewRequest("DELETE", "/api/feeds/999", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// 13. DELETE /api/feeds/:id deletes the feed
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/feeds/%d", feedID), nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify it was deleted from DB
	var remainingCount int64
	db.Model(&repository.GormRssFeed{}).Count(&remainingCount)
	assert.Equal(t, int64(0), remainingCount)

	// 14. GET /api/feeds after delete should be empty
	req = httptest.NewRequest("GET", "/api/feeds", nil)
	resp, err = app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	feeds = nil
	require.NoError(t, json.Unmarshal(body, &feeds))
	assert.Empty(t, feeds)
}

func TestFeedTitleFallback(t *testing.T) {
	app, _, _ := setupFeedTestApp(t)

	mockServerNoTitle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <link>https://example.com/notitle</link>
  <description>No title feed</description>
</channel>
</rss>`))
	}))
	defer mockServerNoTitle.Close()

	addBody, _ := json.Marshal(map[string]string{"url": mockServerNoTitle.URL})
	req := httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var feed repository.GormRssFeed
	require.NoError(t, json.Unmarshal(body, &feed))
	assert.Equal(t, mockServerNoTitle.URL, feed.Title)
}
