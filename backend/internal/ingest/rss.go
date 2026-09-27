package ingest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/repository"
	"github.com/mmcdole/gofeed"
	"github.com/patrickmn/go-cache"
)

var rssCache = cache.New(5*time.Minute, 10*time.Minute)

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

func ValidateAndParseFeed(ctx context.Context, url string) (*repository.GormRssFeed, error) {
	fp := gofeed.NewParser()
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

	fp := gofeed.NewParser()

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

				feedItems = append(feedItems, TimelineItem{
					FeedID:      feed.ID,
					FeedTitle:   feed.Title,
					Title:       item.Title,
					URL:         item.Link,
					Description: item.Description,
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

	return items
}
