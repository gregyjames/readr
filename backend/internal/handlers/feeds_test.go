package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v3"
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

	ingest.AllowLocalhostFeeds = true
	app := fiber.New()
	api := app.Group("/api")

	api.Get("/feeds", GetFeeds(hCtx))
	api.Post("/feeds", AddFeed(hCtx))
	api.Delete("/feeds/:id", RemoveFeed(hCtx))
	api.Get("/feeds/timeline", TimelinePaginator, GetTimeline(hCtx))

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
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var feeds []repository.GormRssFeed
	require.NoError(t, json.Unmarshal(body, &feeds))
	assert.Empty(t, feeds)
	assert.Equal(t, "[]", string(bytes.TrimSpace(body)))

	// 2. Initial GET /api/feeds/timeline should return empty data envelope
	req = httptest.NewRequest("GET", "/api/feeds/timeline", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var initialTimeline timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &initialTimeline))
	assert.Empty(t, initialTimeline.Data)
	assert.Equal(t, int64(0), initialTimeline.Total)
	assert.Equal(t, 1, initialTimeline.Page)
	assert.Equal(t, 25, initialTimeline.Limit)
	assert.Equal(t, 1, initialTimeline.TotalPages)

	// 3. POST /api/feeds with invalid URL should fail with 400
	invalidBody, _ := json.Marshal(map[string]string{"url": "not-a-valid-feed-url"})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(invalidBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 4. POST /api/feeds with empty URL should fail with 400
	emptyBody, _ := json.Marshal(map[string]string{"url": ""})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(emptyBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 5. POST /api/feeds adds a valid feed
	addBody, _ := json.Marshal(map[string]string{"url": mockServer.URL})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
	conflictBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(conflictBody), "feed already exists")

	// 6. GET /api/feeds lists the created feed
	req = httptest.NewRequest("GET", "/api/feeds", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var timeline timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &timeline))
	require.Len(t, timeline.Data, 2)
	assert.Equal(t, "Post 2", timeline.Data[0].Title) // Sorted desc by pubDate
	assert.Equal(t, "Post 1", timeline.Data[1].Title)
	assert.Equal(t, feedID, timeline.Data[0].FeedID)
	assert.Equal(t, "Mock RSS Feed", timeline.Data[0].FeedTitle)
	assert.Equal(t, int64(2), timeline.Total)
	assert.Equal(t, 1, timeline.Page)
	assert.Equal(t, 25, timeline.Limit)
	assert.Equal(t, 1, timeline.TotalPages)

	// 8. GET /api/feeds/timeline?feed_id=X returns items for specific feed
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/feeds/timeline?feed_id=%d", feedID), nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var singleTimeline timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &singleTimeline))
	require.Len(t, singleTimeline.Data, 2)
	assert.Equal(t, int64(2), singleTimeline.Total)

	// 9. GET /api/feeds/timeline?feed_id=999 returns 404 for non-existent feed
	req = httptest.NewRequest("GET", "/api/feeds/timeline?feed_id=999", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// 10. GET /api/feeds/timeline?feed_id=invalid returns 400
	req = httptest.NewRequest("GET", "/api/feeds/timeline?feed_id=invalid", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 11. DELETE /api/feeds/invalid returns 400
	req = httptest.NewRequest("DELETE", "/api/feeds/invalid", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// 12. DELETE /api/feeds/999 returns 404
	req = httptest.NewRequest("DELETE", "/api/feeds/999", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	// 13. DELETE /api/feeds/:id deletes the feed
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/feeds/%d", feedID), nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify it was deleted from DB
	var remainingCount int64
	db.Model(&repository.GormRssFeed{}).Count(&remainingCount)
	assert.Equal(t, int64(0), remainingCount)

	// 14. GET /api/feeds after delete should be empty
	req = httptest.NewRequest("GET", "/api/feeds", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
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
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var feed repository.GormRssFeed
	require.NoError(t, json.Unmarshal(body, &feed))
	assert.Equal(t, mockServerNoTitle.URL, feed.Title)
}

type timelineEnvelope struct {
	Data       []ingest.TimelineItem `json:"data"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	Total      int64                 `json:"total"`
	TotalPages int                   `json:"total_pages"`
}

func TestGetTimeline_Pagination(t *testing.T) {
	app, _, _ := setupFeedTestApp(t)

	// 4. When total is 0, returns { data: [], page: 1, limit: 25, total: 0, total_pages: 1 }
	req := httptest.NewRequest("GET", "/api/feeds/timeline", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var envelope timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Empty(t, envelope.Data)
	assert.Equal(t, 1, envelope.Page)
	assert.Equal(t, 25, envelope.Limit)
	assert.Equal(t, int64(0), envelope.Total)
	assert.Equal(t, 1, envelope.TotalPages)

	// Create mock RSS server with 10 items
	var itemsXML strings.Builder
	for i := 1; i <= 10; i++ {
		pubDate := time.Date(2026, 1, i, 12, 0, 0, 0, time.UTC).Format(time.RFC1123)
		itemsXML.WriteString(fmt.Sprintf(`
  <item>
    <title>Post %02d</title>
    <link>https://example.com/post-%d</link>
    <description>Description %d</description>
    <pubDate>%s</pubDate>
  </item>`, i, i, i, pubDate))
	}
	mockRSS10 := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <title>Mock RSS Feed 10</title>
  <link>https://example.com</link>
  <description>Test feed with 10 items</description>
  %s
</channel>
</rss>`, itemsXML.String())

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRSS10))
	}))
	defer mockServer.Close()

	// Add feed
	addBody, _ := json.Marshal(map[string]string{"url": mockServer.URL})
	req = httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 1. When requesting /api/feeds/timeline?page=1&limit=5, returns envelope:
	//    { data: [5 items], page: 1, limit: 5, total: 10, total_pages: 2 }
	req = httptest.NewRequest("GET", "/api/feeds/timeline?page=1&limit=5", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var page1 timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &page1))
	assert.Equal(t, 1, page1.Page)
	assert.Equal(t, 5, page1.Limit)
	assert.Equal(t, int64(10), page1.Total)
	assert.Equal(t, 2, page1.TotalPages)
	require.Len(t, page1.Data, 5)
	assert.Equal(t, "Post 10", page1.Data[0].Title)
	assert.Equal(t, "Post 06", page1.Data[4].Title)

	// 2. When requesting /api/feeds/timeline?page=2&limit=5, returns envelope with remaining items (page: 2)
	req = httptest.NewRequest("GET", "/api/feeds/timeline?page=2&limit=5", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var page2 timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &page2))
	assert.Equal(t, 2, page2.Page)
	assert.Equal(t, 5, page2.Limit)
	assert.Equal(t, int64(10), page2.Total)
	assert.Equal(t, 2, page2.TotalPages)
	require.Len(t, page2.Data, 5)
	assert.Equal(t, "Post 05", page2.Data[0].Title)
	assert.Equal(t, "Post 01", page2.Data[4].Title)

	// 3. When requesting with no params, defaults to limit: 25, page: 1, total_pages: 1 (if total <= 25)
	req = httptest.NewRequest("GET", "/api/feeds/timeline", nil)
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	var defaultPage timelineEnvelope
	require.NoError(t, json.Unmarshal(body, &defaultPage))
	assert.Equal(t, 1, defaultPage.Page)
	assert.Equal(t, 25, defaultPage.Limit)
	assert.Equal(t, int64(10), defaultPage.Total)
	assert.Equal(t, 1, defaultPage.TotalPages)
	require.Len(t, defaultPage.Data, 10)
}
