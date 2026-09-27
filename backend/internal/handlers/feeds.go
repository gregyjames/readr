package handlers

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/paginate"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AddFeedRequest struct {
	URL string `json:"url"`
}

// GetFeeds lists all RSS feeds.
func GetFeeds(hCtx *HandlerContext) fiber.Handler {
	return func(c fiber.Ctx) error {
		if hCtx.DB == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database not configured"})
		}

		var feeds []repository.GormRssFeed
		if err := hCtx.DB.WithContext(c.Context()).Order("created_at desc").Find(&feeds).Error; err != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Error("Failed to list feeds", zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list feeds"})
		}

		if feeds == nil {
			feeds = make([]repository.GormRssFeed, 0)
		}
		return c.JSON(feeds)
	}
}

// AddFeed validates and parses an RSS feed URL, then persists it.
func AddFeed(hCtx *HandlerContext) fiber.Handler {
	return func(c fiber.Ctx) error {
		if hCtx.DB == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database not configured"})
		}

		var req AddFeedRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		feedURL := strings.TrimSpace(req.URL)
		if feedURL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Feed URL is required"})
		}

		feed, err := ingest.ValidateAndParseFeed(c.Context(), feedURL)
		if err != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Warn("Failed to validate feed URL", zap.String("url", feedURL), zap.Error(err))
			}
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse feed"})
		}

		var count int64
		if err := hCtx.DB.WithContext(c.Context()).Model(&repository.GormRssFeed{}).Where("url = ?", feed.URL).Count(&count).Error; err != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Error("Failed to check existing feed", zap.String("url", feed.URL), zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check existing feed"})
		}
		if count > 0 {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "feed already exists"})
		}

		if strings.TrimSpace(feed.Title) == "" {
			feed.Title = feed.URL
		}

		feed.CreatedAt = time.Now()
		if err := hCtx.DB.WithContext(c.Context()).Create(feed).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "feed already exists"})
			}
			if hCtx.Logger != nil {
				hCtx.Logger.Error("Failed to save feed", zap.String("url", feedURL), zap.Error(err))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save feed"})
		}

		return c.Status(fiber.StatusOK).JSON(feed)
	}
}

// RemoveFeed deletes an RSS feed by its ID.
func RemoveFeed(hCtx *HandlerContext) fiber.Handler {
	return func(c fiber.Ctx) error {
		if hCtx.DB == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database not configured"})
		}

		idParam := c.Params("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || id <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid feed ID"})
		}

		res := hCtx.DB.WithContext(c.Context()).Delete(&repository.GormRssFeed{}, id)
		if res.Error != nil {
			if hCtx.Logger != nil {
				hCtx.Logger.Error("Failed to delete feed", zap.Int64("id", id), zap.Error(res.Error))
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete feed"})
		}

		if res.RowsAffected == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Feed not found"})
		}

		return c.JSON(fiber.Map{"status": "success"})
	}
}

var TimelinePaginator = paginate.New(paginate.Config{
	DefaultPage:  1,
	DefaultLimit: 25,
	MaxLimit:     100,
	PageKey:      "page",
	LimitKey:     "limit",
})

// GetTimeline fetches and combines timeline items from all feeds or a single feed if ?feed_id=X is given.
func GetTimeline(hCtx *HandlerContext) fiber.Handler {
	return func(c fiber.Ctx) error {
		if hCtx.DB == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database not configured"})
		}

		var feeds []repository.GormRssFeed
		feedIDStr := c.Query("feed_id")

		if feedIDStr != "" {
			feedID, err := strconv.ParseInt(feedIDStr, 10, 64)
			if err != nil || feedID <= 0 {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid feed ID"})
			}

			var feed repository.GormRssFeed
			if err := hCtx.DB.WithContext(c.Context()).First(&feed, feedID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Feed not found"})
				}
				if hCtx.Logger != nil {
					hCtx.Logger.Error("Failed to retrieve feed", zap.Int64("id", feedID), zap.Error(err))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve feed"})
			}
			feeds = append(feeds, feed)
		} else {
			if err := hCtx.DB.WithContext(c.Context()).Find(&feeds).Error; err != nil {
				if hCtx.Logger != nil {
					hCtx.Logger.Error("Failed to list feeds", zap.Error(err))
				}
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list feeds"})
			}
		}

		pageInfo, _ := paginate.FromContext(c)

		forceRefresh := c.Query("refresh") == "true" || c.Query("refresh") == "1"
		items := ingest.FetchFeedsTimelineWithOptions(c.Context(), feeds, 10*time.Second, forceRefresh)
		if items == nil {
			items = make([]ingest.TimelineItem, 0)
		}

		total := int64(len(items))
		page := 1
		limit := 25
		start := 0
		if pageInfo != nil {
			page = pageInfo.Page
			limit = pageInfo.Limit
			start = pageInfo.Start()
		}
		if start > len(items) {
			start = len(items)
		}
		end := start + limit
		if end > len(items) {
			end = len(items)
		}
		pagedItems := items[start:end]
		if pagedItems == nil {
			pagedItems = make([]ingest.TimelineItem, 0)
		}
		totalPages := int(math.Ceil(float64(total) / float64(limit)))
		if totalPages == 0 {
			totalPages = 1
		}

		return c.JSON(fiber.Map{
			"data":        pagedItems,
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		})
	}
}
