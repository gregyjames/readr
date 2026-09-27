package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/backend/internal/repository"
	"github.com/stretchr/testify/assert"
)

func TestFetchTimeline(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Blog A</title><item><title>Item 1</title><link>http://a/1</link><pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate></item></channel></rss>`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Blog B</title><item><title>Item 2</title><link>http://b/2</link><pubDate>Tue, 03 Jan 2006 15:04:05 MST</pubDate></item></channel></rss>`))
	}))
	defer ts2.Close()

	feeds := []repository.GormRssFeed{
		{ID: 1, Title: "Blog A", URL: ts1.URL},
		{ID: 2, Title: "Blog B", URL: ts2.URL},
	}

	items := FetchFeedsTimeline(context.Background(), feeds, 2*time.Second)
	assert.Len(t, items, 2)
	assert.Equal(t, "Item 2", items[0].Title) // newest first
	assert.Equal(t, "Item 1", items[1].Title)
	assert.Equal(t, int64(2), items[0].FeedID)
	assert.Equal(t, int64(1), items[1].FeedID)
}

func TestValidateAndParseFeed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>My Blog</title><link>https://blog.com</link></channel></rss>`))
	}))
	defer ts.Close()

	feed, err := ValidateAndParseFeed(context.Background(), ts.URL)
	assert.NoError(t, err)
	assert.Equal(t, "My Blog", feed.Title)
	assert.Equal(t, "https://blog.com", feed.SiteURL)
}

func TestFetchTimelineCaching(t *testing.T) {
	FlushRSSCache()
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Cached Blog</title><item><title>Item</title><link>http://c/1</link><pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate></item></channel></rss>`))
	}))
	defer ts.Close()

	feeds := []repository.GormRssFeed{
		{ID: 10, Title: "Cached Blog", URL: ts.URL},
	}

	// First fetch should hit network
	items1 := FetchFeedsTimeline(context.Background(), feeds, 2*time.Second)
	assert.Len(t, items1, 1)
	assert.Equal(t, 1, callCount)

	// Second fetch without forceRefresh should hit cache (callCount remains 1)
	items2 := FetchFeedsTimeline(context.Background(), feeds, 2*time.Second)
	assert.Len(t, items2, 1)
	assert.Equal(t, 1, callCount)

	// Third fetch with forceRefresh=true should bypass cache (callCount becomes 2)
	items3 := FetchFeedsTimelineWithOptions(context.Background(), feeds, 2*time.Second, true)
	assert.Len(t, items3, 1)
	assert.Equal(t, 2, callCount)
}
