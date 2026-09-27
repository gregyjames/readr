package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/backend/internal/repository"
	"github.com/stretchr/testify/assert"
)

func TestFetchTimeline(t *testing.T) {
	AllowLocalhostFeeds = true
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
	AllowLocalhostFeeds = true
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
	AllowLocalhostFeeds = true
	FlushRSSCache()
	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Cached Blog</title><item><title>Item</title><link>http://c/1</link><pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate></item></channel></rss>`))
	}))
	defer ts.Close()

	feeds := []repository.GormRssFeed{
		{ID: 10, Title: "Cached Blog", URL: ts.URL},
	}

	// First fetch should hit network
	items1 := FetchFeedsTimeline(context.Background(), feeds, 2*time.Second)
	assert.Len(t, items1, 1)
	assert.Equal(t, int32(1), callCount.Load())

	// Second fetch without forceRefresh should hit cache (callCount remains 1)
	items2 := FetchFeedsTimeline(context.Background(), feeds, 2*time.Second)
	assert.Len(t, items2, 1)
	assert.Equal(t, int32(1), callCount.Load())

	// Third fetch with forceRefresh=true should bypass cache (callCount becomes 2)
	items3 := FetchFeedsTimelineWithOptions(context.Background(), feeds, 2*time.Second, true)
	assert.Len(t, items3, 1)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestCleanExcerpt(t *testing.T) {
	// Empty input
	assert.Equal(t, "", CleanExcerpt("", 280))

	// Strips scripts and styles
	htmlWithScript := `<script type="text/javascript">alert('evil')</script><style>.body { color: red; }</style><p>Hello <b>World</b>!</p>`
	assert.Equal(t, "Hello World!", CleanExcerpt(htmlWithScript, 280))

	// Strips nested HTML and unescapes entities
	htmlWithEntities := `<p>Read &amp; learn about Go &gt; Rust &lt; C&#43;&#43; &quot;quote&quot; &amp; &#39;apostrophe&#39;.</p>`
	assert.Equal(t, `Read & learn about Go > Rust < C++ "quote" & 'apostrophe'.`, CleanExcerpt(htmlWithEntities, 280))

	// Normalizes multiple newlines, tabs, and spaces
	whitespaceInput := "  Line 1   \n\n   \t  Line 2  \r\n   Line 3   "
	assert.Equal(t, "Line 1 Line 2 Line 3", CleanExcerpt(whitespaceInput, 280))

	// Truncates long text cleanly at word boundary with ellipsis
	longText := "The quick brown fox jumps over the lazy dog repeatedly until the sentence becomes exceedingly long and exceeds our requested character budget."
	truncated := CleanExcerpt(longText, 40)
	assert.True(t, len([]rune(truncated)) <= 42)
	assert.True(t, strings.HasSuffix(truncated, "…"))
	assert.Equal(t, "The quick brown fox jumps over the lazy…", truncated)
}
