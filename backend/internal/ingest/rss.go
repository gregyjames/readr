package ingest

import (
	"context"
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/repository"
	"github.com/mmcdole/gofeed"
	"github.com/patrickmn/go-cache"
)

var rssCache = cache.New(5*time.Minute, 10*time.Minute)

var (
	htmlTagRegex    = regexp.MustCompile(`(?i)<[^>]*>`)
	scriptTagRegex  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRegex   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
	punctSpaceRegex = regexp.MustCompile(`\s+([,.:;!?…])`)
)

// CleanExcerpt strips HTML tags, removes script/style blocks, decodes HTML entities,
// collapses whitespace, and truncates to maxChars cleanly on a word boundary.
func CleanExcerpt(raw string, maxChars int) string {
	if raw == "" {
		return ""
	}

	// 1. Remove script and style blocks first
	cleaned := scriptTagRegex.ReplaceAllString(raw, " ")
	cleaned = styleTagRegex.ReplaceAllString(cleaned, " ")

	// 2. Strip all remaining HTML tags
	cleaned = htmlTagRegex.ReplaceAllString(cleaned, " ")

	// 3. Decode HTML entities (e.g. &amp;, &quot;, &#39;, &nbsp;)
	cleaned = html.UnescapeString(cleaned)

	// 4. Normalize whitespace (replaces tabs, newlines, multiple spaces with a single space)
	cleaned = strings.TrimSpace(whitespaceRegex.ReplaceAllString(cleaned, " "))

	// 5. Clean up spaces before punctuation (e.g. "word !" -> "word!")
	cleaned = punctSpaceRegex.ReplaceAllString(cleaned, "$1")

	if maxChars <= 0 {
		return cleaned
	}

	runes := []rune(cleaned)
	if len(runes) <= maxChars {
		return cleaned
	}

	truncated := string(runes[:maxChars])
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxChars/2 {
		truncated = truncated[:lastSpace]
	}

	return strings.TrimSpace(truncated) + "…"
}

// FlushRSSCache clears all cached feed timeline entries.
func FlushRSSCache() {
	rssCache.Flush()
}

type TimelineItem struct {
	FeedID      int64     `json:"feedId"`
	FeedTitle   string    `json:"feedTitle"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Published   time.Time `json:"published"`
}

// AllowLocalhostFeeds controls whether feed parsing allows fetching from localhost (used in test suites).
var AllowLocalhostFeeds = false

func newFeedParser(timeout time.Duration) *gofeed.Parser {
	fetcher := NewHTTPFetcher(timeout)
	if AllowLocalhostFeeds || os.Getenv("ALLOW_LOCALHOST") == "true" {
		fetcher.AllowLocalhost = true
	}
	fp := gofeed.NewParser()
	fp.Client = fetcher.client.StandardClient()
	fp.MaxByteSize = MaxHTMLBytes // 10MB response-size limit
	return fp
}

func ValidateAndParseFeed(ctx context.Context, url string) (*repository.GormRssFeed, error) {
	fp := newFeedParser(5 * time.Second)
	parsedCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	feed, err := fp.ParseURLWithContext(url, parsedCtx)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(feed.Title)
	if title == "" {
		title = url
	}

	return &repository.GormRssFeed{
		URL:     url,
		Title:   title,
		SiteURL: feed.Link,
	}, nil
}

// FetchFeedsTimeline fetches and combines timeline items from feeds using the in-memory cache.
func FetchFeedsTimeline(ctx context.Context, feeds []repository.GormRssFeed, timeout time.Duration) []TimelineItem {
	return FetchFeedsTimelineWithOptions(ctx, feeds, timeout, false)
}

// FetchFeedsTimelineWithOptions fetches timeline items, optionally bypassing and updating the in-memory cache.
func FetchFeedsTimelineWithOptions(ctx context.Context, feeds []repository.GormRssFeed, timeout time.Duration, forceRefresh bool) []TimelineItem {
	var wg sync.WaitGroup
	var mu sync.Mutex
	items := make([]TimelineItem, 0)

	fp := newFeedParser(timeout)

	for _, f := range feeds {
		cacheKey := fmt.Sprintf("feed:%d:%s", f.ID, f.URL)

		if !forceRefresh {
			if cached, found := rssCache.Get(cacheKey); found {
				if cachedItems, ok := cached.([]TimelineItem); ok {
					mu.Lock()
					items = append(items, cachedItems...)
					mu.Unlock()
					continue
				}
			}
		}

		wg.Add(1)
		go func(feed repository.GormRssFeed, key string) {
			defer wg.Done()
			fetchCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			parsed, err := fp.ParseURLWithContext(feed.URL, fetchCtx)
			if err != nil {
				return // Ignore failures gracefully
			}

			// Sort feed items newest-first before capping
			sort.Slice(parsed.Items, func(i, j int) bool {
				pubI := parsed.Items[i].PublishedParsed
				if pubI == nil {
					pubI = parsed.Items[i].UpdatedParsed
				}
				pubJ := parsed.Items[j].PublishedParsed
				if pubJ == nil {
					pubJ = parsed.Items[j].UpdatedParsed
				}
				if pubI != nil && pubJ != nil {
					return pubI.After(*pubJ)
				}
				return false
			})

			// Cap items per feed (30 items max) to avoid massive historical payloads
			const maxItemsPerFeed = 30
			if len(parsed.Items) > maxItemsPerFeed {
				parsed.Items = parsed.Items[:maxItemsPerFeed]
			}

			feedItems := make([]TimelineItem, 0, len(parsed.Items))
			for _, item := range parsed.Items {
				pubDate := item.PublishedParsed
				if pubDate == nil {
					pubDate = item.UpdatedParsed
				}
				if pubDate == nil {
					now := time.Now()
					pubDate = &now
				}

				rawDesc := item.Description
				if rawDesc == "" {
					rawDesc = item.Content
				}

				feedItems = append(feedItems, TimelineItem{
					FeedID:      feed.ID,
					FeedTitle:   feed.Title,
					Title:       strings.TrimSpace(item.Title),
					URL:         item.Link,
					Description: CleanExcerpt(rawDesc, 280),
					Published:   *pubDate,
				})
			}

			rssCache.Set(key, feedItems, 5*time.Minute)

			mu.Lock()
			items = append(items, feedItems...)
			mu.Unlock()
		}(f, cacheKey)
	}

	wg.Wait()

	sort.Slice(items, func(i, j int) bool {
		return items[i].Published.After(items[j].Published)
	})

	const maxTotalTimelineItems = 200
	if len(items) > maxTotalTimelineItems {
		items = items[:maxTotalTimelineItems]
	}

	return items
}
