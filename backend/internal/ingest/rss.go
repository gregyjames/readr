package ingest

import (
	"context"
	"sort"
	"sync"
	"time"

	"example.com/backend/internal/repository"
	"github.com/mmcdole/gofeed"
)

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

	return &repository.GormRssFeed{
		URL:     url,
		Title:   feed.Title,
		SiteURL: feed.Link,
	}, nil
}

func FetchFeedsTimeline(ctx context.Context, feeds []repository.GormRssFeed, timeout time.Duration) []TimelineItem {
	var wg sync.WaitGroup
	var mu sync.Mutex
	items := make([]TimelineItem, 0)

	fp := gofeed.NewParser()

	for _, f := range feeds {
		wg.Add(1)
		go func(feed repository.GormRssFeed) {
			defer wg.Done()
			fetchCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			parsed, err := fp.ParseURLWithContext(feed.URL, fetchCtx)
			if err != nil {
				return // Ignore failures gracefully
			}

			mu.Lock()
			for _, item := range parsed.Items {
				pubDate := item.PublishedParsed
				if pubDate == nil {
					pubDate = item.UpdatedParsed
				}
				if pubDate == nil {
					now := time.Now()
					pubDate = &now
				}

				items = append(items, TimelineItem{
					FeedID:      feed.ID,
					FeedTitle:   feed.Title,
					Title:       item.Title,
					URL:         item.Link,
					Description: item.Description,
					Published:   *pubDate,
				})
			}
			mu.Unlock()
		}(f)
	}

	wg.Wait()

	sort.Slice(items, func(i, j int) bool {
		return items[i].Published.After(items[j].Published)
	})

	return items
}
