# Fiber v3 Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement server-side and client-side pagination for Home articles and Feeds timeline using Fiber v3's official `github.com/gofiber/fiber/v3/middleware/paginate` middleware and a reusable Vue 3 `PaginationControls.vue` component.

**Architecture:** Mount Fiber v3's `paginate` middleware on `/api/getarticles` and `/api/feeds/timeline` returning a consistent `{ data, total, page, limit, total_pages }` envelope. Connect this to a reusable frontend `PaginationControls.vue` component supporting traditional page numbers, previous/next buttons, and selectable page sizes stored in `localStorage`.

**Tech Stack:** Go 1.24, Fiber v3 (`github.com/gofiber/fiber/v3/middleware/paginate`), GORM, SQLite (WAL mode), Vue 3 (Composition API, `<script setup>`), TypeScript, Bun, Tailwind CSS.

---

### Task 1: Backend Articles Pagination

**Files:**
- Modify: `backend/internal/handlers/articles.go:88-140`
- Test: `backend/internal/handlers/articles_test.go`

- [ ] **Step 1: Write failing tests for `/api/getarticles` pagination**

In `backend/internal/handlers/articles_test.go`, add test `TestGetArticles_Pagination`:
```go
func TestGetArticles_Pagination(t *testing.T) {
	app, db, _, cleanup := setupArticlesTestApp(t)
	defer cleanup()

	// Seed 15 test articles
	for i := 1; i <= 15; i++ {
		art := repository.GormArticle{
			Title:      fmt.Sprintf("Paginated Article %02d", i),
			Article:    fmt.Sprintf("article-%02d.md", i),
			IsArchived: false,
			WordCount:  100,
		}
		require.NoError(t, db.Create(&art).Error)
	}

	// 1. Default pagination (page 1, limit 10)
	req1 := httptest.NewRequest("GET", "/api/getarticles?page=1&limit=10", nil)
	resp1, err := app.Test(req1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode)

	var envelope1 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&envelope1))
	assert.Equal(t, 10, len(envelope1.Data))
	assert.Equal(t, 1, envelope1.Page)
	assert.Equal(t, 10, envelope1.Limit)
	assert.Equal(t, int64(15), envelope1.Total)
	assert.Equal(t, 2, envelope1.TotalPages)

	// 2. Second page (page 2, limit 10)
	req2 := httptest.NewRequest("GET", "/api/getarticles?page=2&limit=10", nil)
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)

	var envelope2 struct {
		Data       []repository.GormArticle `json:"data"`
		Page       int                      `json:"page"`
		Limit      int                      `json:"limit"`
		Total      int64                    `json:"total"`
		TotalPages int                      `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&envelope2))
	assert.Equal(t, 5, len(envelope2.Data))
	assert.Equal(t, 2, envelope2.Page)

	// 3. All articles requested (all=true)
	req3 := httptest.NewRequest("GET", "/api/getarticles?all=true", nil)
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	assert.Equal(t, 200, resp3.StatusCode)

	var envelope3 struct {
		Data       []repository.GormArticle `json:"data"`
		Total      int64                    `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&envelope3))
	assert.Equal(t, 15, len(envelope3.Data))
	assert.Equal(t, int64(15), envelope3.Total)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -race -v ./internal/handlers/ -run TestGetArticles_Pagination`
Expected: FAIL (handler does not return `{ data, page, limit, total, total_pages }` envelope).

- [ ] **Step 3: Update `backend/internal/handlers/articles.go` with Fiber paginate middleware**

In `backend/internal/handlers/articles.go`:
1. Add import `"github.com/gofiber/fiber/v3/middleware/paginate"` and `"math"`.
2. Update `RegisterArticles`:
```go
	articlePaginator := paginate.New(paginate.Config{
		DefaultPage:  1,
		DefaultLimit: 25,
		MaxLimit:     100,
		PageKey:      "page",
		LimitKey:     "limit",
	})

	router.Get("/getarticles", articlePaginator, func(c fiber.Ctx) error {
		archivedParam := c.Query("archived")
		isArchived := archivedParam == "true"
		isAll := c.Query("all") == "true" || c.Query("all") == "1"

		if h.Vault != nil {
			var isArchivedPtr *bool
			if archivedParam != "" {
				isArchivedPtr = &isArchived
			} else {
				f := false
				isArchivedPtr = &f
			}
			articles, err := h.Vault.ListArticles(c.Context(), vault.ArticleFilter{
				Archived: isArchivedPtr,
			})
			if err != nil {
				if h.Logger != nil {
					h.Logger.Error("Failed to retrieve articles from Vault", zap.Error(err))
				}
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to retrieve articles",
				})
			}
			total := int64(len(articles))
			if isAll {
				return c.JSON(fiber.Map{
					"data":        articles,
					"page":        1,
					"limit":       total,
					"total":       total,
					"total_pages": 1,
				})
			}

			pageInfo, _ := paginate.FromContext(c)
			start := 0
			limit := 25
			page := 1
			if pageInfo != nil {
				start = pageInfo.Start()
				limit = pageInfo.Limit
				page = pageInfo.Page
			}
			if start > len(articles) {
				start = len(articles)
			}
			end := start + limit
			if end > len(articles) {
				end = len(articles)
			}
			pageSlice := articles[start:end]
			totalPages := int(math.Ceil(float64(total) / float64(limit)))
			if totalPages == 0 {
				totalPages = 1
			}
			return c.JSON(fiber.Map{
				"data":        pageSlice,
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
			})
		}

		query := h.DB.Model(&repository.GormArticle{}).Where("is_archived = ?", isArchived)
		if !isArchived {
			query = h.DB.Model(&repository.GormArticle{}).Where("is_archived = ? OR is_archived IS NULL", false)
		}

		var total int64
		if err := query.Count(&total).Error; err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to count articles", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to count articles"})
		}

		var articles []repository.GormArticle
		if isAll {
			if err := query.Find(&articles).Error; err != nil {
				if h.Logger != nil {
					h.Logger.Error("Failed to retrieve articles from DB", zap.Error(err))
				}
				return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve articles"})
			}
			hydrateReadingStatus(c.Context(), h, articles)
			hydrateMOCProgress(c.Context(), h, articles)
			hydrateReadingTime(articles)
			return c.JSON(fiber.Map{
				"data":        articles,
				"page":        1,
				"limit":       total,
				"total":       total,
				"total_pages": 1,
			})
		}

		pageInfo, _ := paginate.FromContext(c)
		limit := 25
		offset := 0
		page := 1
		if pageInfo != nil {
			limit = pageInfo.Limit
			offset = pageInfo.Start()
			page = pageInfo.Page
		}

		if err := query.Offset(offset).Limit(limit).Find(&articles).Error; err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to retrieve articles from DB", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve articles"})
		}

		hydrateReadingStatus(c.Context(), h, articles)
		hydrateMOCProgress(c.Context(), h, articles)
		hydrateReadingTime(articles)

		totalPages := int(math.Ceil(float64(total) / float64(limit)))
		if totalPages == 0 {
			totalPages = 1
		}

		return c.JSON(fiber.Map{
			"data":        articles,
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		})
	})
```

- [ ] **Step 4: Update existing tests in `articles_test.go` and run tests**

Update existing assertions in `articles_test.go` that inspect `/api/getarticles` to decode the envelope's `Data` field.
Run: `cd backend && go test -race -v ./internal/handlers/ -run "TestGetArticles"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/articles.go backend/internal/handlers/articles_test.go
git commit -m "feat(api): implement fiber paginate middleware for /api/getarticles"
```

---

### Task 2: Backend Feeds Timeline Pagination

**Files:**
- Modify: `backend/internal/handlers/feeds.go:126-184`
- Test: `backend/internal/handlers/feeds_test.go`

- [ ] **Step 1: Write failing tests for `/api/feeds/timeline` pagination**

In `backend/internal/handlers/feeds_test.go`, add test `TestGetTimeline_Pagination`:
```go
func TestGetTimeline_Pagination(t *testing.T) {
	// Setup app with feeds handler
	// Verify GET /api/feeds/timeline?page=1&limit=5 returns envelope with total, page=1, limit=5, data length 5
	// Verify GET /api/feeds/timeline?page=2&limit=5 returns envelope with page=2, data length 5
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -race -v ./internal/handlers/ -run TestGetTimeline_Pagination`
Expected: FAIL.

- [ ] **Step 3: Update `backend/internal/handlers/feeds.go` with paginate middleware**

In `backend/internal/handlers/feeds.go`:
1. Add import `"github.com/gofiber/fiber/v3/middleware/paginate"` and `"math"`.
2. Update `GetTimeline`:
```go
func GetTimeline(hCtx *HandlerContext) fiber.Handler {
	timelinePaginator := paginate.New(paginate.Config{
		DefaultPage:  1,
		DefaultLimit: 25,
		MaxLimit:     100,
		PageKey:      "page",
		LimitKey:     "limit",
	})

	innerHandler := func(c fiber.Ctx) error {
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

		forceRefresh := c.Query("refresh") == "true" || c.Query("refresh") == "1"
		items := ingest.FetchFeedsTimelineWithOptions(c.Context(), feeds, 10*time.Second, forceRefresh)
		if items == nil {
			items = make([]ingest.TimelineItem, 0)
		}

		pageInfo, _ := paginate.FromContext(c)
		page := 1
		limit := 25
		start := 0
		if pageInfo != nil {
			page = pageInfo.Page
			limit = pageInfo.Limit
			start = pageInfo.Start()
		}

		total := int64(len(items))
		if start > len(items) {
			start = len(items)
		}
		end := start + limit
		if end > len(items) {
			end = len(items)
		}
		pagedItems := items[start:end]

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

	return func(c fiber.Ctx) error {
		return timelinePaginator(c)
	}
}
```
*Note:* In `backend/main.go` or `backend/internal/handlers/feeds.go`, wire the middleware directly in route registration:
`router.Get("/feeds/timeline", paginate.New(paginate.Config{...}), GetTimeline(hCtx))` to ensure Fiber's middleware chain executes cleanly.

- [ ] **Step 4: Update existing tests in `feeds_test.go` and run tests**

Update existing timeline tests to expect `{ data, total, page, limit, total_pages }`.
Run: `cd backend && go test -race -v ./internal/handlers/ -run "TestGetTimeline"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/feeds.go backend/internal/handlers/feeds_test.go
git commit -m "feat(api): implement fiber paginate middleware for /api/feeds/timeline"
```

---

### Task 3: Frontend `PaginationControls.vue` Component

**Files:**
- Create: `frontend/src/components/PaginationControls.vue`
- Create: `frontend/src/components/PaginationControls.test.ts`

- [ ] **Step 1: Write unit tests for `PaginationControls.vue`**

In `frontend/src/components/PaginationControls.test.ts`:
```ts
import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import PaginationControls from './PaginationControls.vue'

describe('PaginationControls.vue', () => {
  it('renders range summary and navigation buttons', () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    expect(wrapper.text()).toContain('Showing 1–25 of 120 items')
    expect(wrapper.find('button[aria-label="Previous page"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('button[aria-label="Next page"]').attributes('disabled')).toBeUndefined()
  })

  it('emits update:page when next or page button is clicked', async () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    await wrapper.find('button[aria-label="Next page"]').trigger('click')
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])
  })

  it('emits update:pageSize when selector changes', async () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    const select = wrapper.find('select')
    await select.setValue('50')
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([50])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && bun test src/components/PaginationControls.test.ts`
Expected: FAIL (component not found).

- [ ] **Step 3: Implement `frontend/src/components/PaginationControls.vue`**

Create `frontend/src/components/PaginationControls.vue`:
```vue
<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    currentPage: number
    totalPages: number
    totalItems: number
    pageSize: number
    pageSizeOptions?: number[]
  }>(),
  {
    pageSizeOptions: () => [10, 25, 50, 100]
  }
)

const emit = defineEmits<{
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', size: number): void
}>()

const startItem = computed(() => {
  if (props.totalItems === 0) return 0
  return (props.currentPage - 1) * props.pageSize + 1
})

const endItem = computed(() => {
  return Math.min(props.currentPage * props.pageSize, props.totalItems)
})

// Generate visible page numbers with smart ellipsis: [1, '...', 4, 5, 6, '...', 10]
const visiblePages = computed(() => {
  const total = props.totalPages
  const current = props.currentPage
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const pages: (number | string)[] = []
  if (current <= 4) {
    for (let i = 1; i <= 5; i++) pages.push(i)
    pages.push('...')
    pages.push(total)
  } else if (current >= total - 3) {
    pages.push(1)
    pages.push('...')
    for (let i = total - 4; i <= total; i++) pages.push(i)
  } else {
    pages.push(1)
    pages.push('...')
    for (let i = current - 1; i <= current + 1; i++) pages.push(i)
    pages.push('...')
    pages.push(total)
  }
  return pages
})

const setPage = (page: number) => {
  if (page < 1 || page > props.totalPages || page === props.currentPage) return
  emit('update:page', page)
}

const handlePageSizeChange = (event: Event) => {
  const target = event.target as HTMLSelectElement
  const newSize = parseInt(target.value, 10)
  if (!isNaN(newSize)) {
    emit('update:pageSize', newSize)
  }
}
</script>

<template>
  <div v-if="totalItems > 0" class="flex flex-col sm:flex-row items-center justify-between gap-4 py-4 px-2 border-t border-zinc-200 dark:border-zinc-800 text-xs text-zinc-600 dark:text-zinc-400">
    <div class="flex items-center gap-2">
      <span>Showing <strong class="font-medium text-zinc-800 dark:text-zinc-200">{{ startItem }}–{{ endItem }}</strong> of <strong class="font-medium text-zinc-800 dark:text-zinc-200">{{ totalItems }}</strong> items</span>
    </div>

    <div class="flex items-center gap-1">
      <button
        type="button"
        aria-label="Previous page"
        :disabled="currentPage <= 1"
        @click="setPage(currentPage - 1)"
        class="inline-flex items-center justify-center px-2.5 py-1.5 rounded border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 font-medium hover:bg-zinc-50 dark:hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        Prev
      </button>

      <template v-for="(p, idx) in visiblePages" :key="idx">
        <span v-if="p === '...'" class="px-2 text-zinc-400">…</span>
        <button
          v-else
          type="button"
          :aria-label="`Page ${p}`"
          :class="[
            'inline-flex items-center justify-center min-w-[28px] h-7 px-2 rounded font-medium transition-colors',
            p === currentPage
              ? 'bg-emerald-600 text-white font-semibold'
              : 'border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800'
          ]"
          @click="setPage(Number(p))"
        >
          {{ p }}
        </button>
      </template>

      <button
        type="button"
        aria-label="Next page"
        :disabled="currentPage >= totalPages"
        @click="setPage(currentPage + 1)"
        class="inline-flex items-center justify-center px-2.5 py-1.5 rounded border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 font-medium hover:bg-zinc-50 dark:hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        Next
      </button>
    </div>

    <div class="flex items-center gap-2">
      <label for="page-size-select" class="text-zinc-500">Per page:</label>
      <select
        id="page-size-select"
        :value="pageSize"
        @change="handlePageSizeChange"
        class="rounded border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-2 py-1 text-xs text-zinc-800 dark:text-zinc-200 focus:outline-none focus:ring-1 focus:ring-emerald-500"
      >
        <option v-for="opt in pageSizeOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select>
    </div>
  </div>
</template>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && bun test src/components/PaginationControls.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/PaginationControls.vue frontend/src/components/PaginationControls.test.ts
git commit -m "feat(ui): create reusable PaginationControls component"
```

---

### Task 4: Align Services and Dependent Views

**Files:**
- Modify: `frontend/src/services/feeds.ts`
- Modify: `frontend/src/services/feeds.test.ts`
- Modify: `frontend/src/views/ArchiveView.vue`
- Modify: `frontend/src/components/Article.vue`
- Modify: `frontend/src/views/ChatView.vue`

- [ ] **Step 1: Update `feedsAPI.getTimeline` in `frontend/src/services/feeds.ts`**

In `frontend/src/services/feeds.ts`:
1. Add `PaginatedResponse<T>` interface:
```ts
export interface PaginatedResponse<T> {
  data: T[]
  page: number
  limit: number
  total: number
  total_pages: number
}
```
2. Update `getTimeline`:
```ts
  getTimeline: async (
    feedId?: number | null,
    refresh?: boolean,
    page?: number,
    limit?: number
  ): Promise<PaginatedResponse<TimelineItem>> => {
    const params = new URLSearchParams()
    if (feedId) {
      params.append('feed_id', feedId.toString())
    }
    if (refresh) {
      params.append('refresh', 'true')
    }
    if (page) {
      params.append('page', page.toString())
    }
    if (limit) {
      params.append('limit', limit.toString())
    }
    const query = params.toString() ? `?${params.toString()}` : ''
    const headers = getAuthHeaders()
    const res = await fetch(`${API_BASE}/feeds/timeline${query}`, { headers })
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}))
      throw new Error(errData.error || `HTTP error ${res.status}`)
    }
    return res.json()
  },
```

- [ ] **Step 2: Update `frontend/src/services/feeds.test.ts`**

Update `feeds.test.ts` mocks for `getTimeline` to return `{ data: [...], total: 1, page: 1, limit: 25, total_pages: 1 }`.
Run: `cd frontend && bun test src/services/feeds.test.ts`
Expected: PASS.

- [ ] **Step 3: Update `ArchiveView.vue`, `Article.vue`, and `ChatView.vue`**

1. In `Article.vue`: Update calls to `/api/getarticles?all=true` and `/api/getarticles?archived=true&all=true`. Extract `res.data.data || res.data`.
2. In `ChatView.vue`: Update call to `/api/getarticles?all=true`. Extract `res.data.data || res.data`.
3. In `ArchiveView.vue`: Update to extract `res.data.data || res.data` and support `PaginationControls.vue`.

- [ ] **Step 4: Verify frontend typechecks and tests**

Run: `cd frontend && (bun test || npx bun test) && (bun x vue-tsc -b || npx vue-tsc -b)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/services/feeds.ts frontend/src/services/feeds.test.ts frontend/src/views/ArchiveView.vue frontend/src/components/Article.vue frontend/src/views/ChatView.vue
git commit -m "refactor(frontend): update api service and views for paginated responses"
```

---

### Task 5: Integrate Pagination into Home and Feeds Views

**Files:**
- Modify: `frontend/src/components/Home.vue`
- Modify: `frontend/src/views/FeedsView.vue`
- Modify: `frontend/src/components/HomeAndArticleViews.test.ts`
- Modify: `frontend/src/views/FeedsView.test.ts`

- [ ] **Step 1: Integrate pagination in `Home.vue`**

In `frontend/src/components/Home.vue`:
1. Import `PaginationControls from './PaginationControls.vue'`.
2. Add reactive state:
```ts
const PAGE_SIZE_STORAGE_KEY = 'readr_page_size'
const storedSize = parseInt(localStorage.getItem(PAGE_SIZE_STORAGE_KEY) || '25', 10)
const pageSize = ref([10, 25, 50, 100].includes(storedSize) ? storedSize : 25)
const currentPage = ref(1)
const totalArticles = ref(0)
const totalPages = ref(1)
```
3. Update `fetchArticles`:
```ts
const fetchArticles = async () => {
  try {
    const res = await axios.get('/api/getarticles', {
      params: {
        page: currentPage.value,
        limit: pageSize.value,
        archived: false,
      }
    })
    const payload = res.data.data ? res.data : { data: res.data, total: res.data.length, total_pages: 1 }
    articles.value = payload.data.map((article: any) => ({
      ...article,
      parsedTags: article.tags ? article.tags.split(',').map((tag: string) => tag.trim()) : []
    }))
    totalArticles.value = payload.total
    totalPages.value = payload.total_pages
    await nextTick()
    initReveal()
  } catch (err: any) {
    if (axios.isCancel(err) || err?.code === 'ERR_CANCELED' || err?.name === 'CanceledError') return
    console.error('Failed to load articles', err)
  }
}
```
4. Handle page and pageSize updates:
```ts
const handlePageChange = async (newPage: number) => {
  currentPage.value = newPage
  await fetchArticles()
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

const handlePageSizeChange = async (newSize: number) => {
  pageSize.value = newSize
  localStorage.setItem(PAGE_SIZE_STORAGE_KEY, newSize.toString())
  currentPage.value = 1
  await fetchArticles()
}
```
5. Reset `currentPage.value = 1` on tag filter selection or search query changes.
6. Render `<PaginationControls>` below the article grid:
```vue
<PaginationControls
  :current-page="currentPage"
  :total-pages="totalPages"
  :total-items="totalArticles"
  :page-size="pageSize"
  @update:page="handlePageChange"
  @update:page-size="handlePageSizeChange"
/>
```

- [ ] **Step 2: Integrate pagination in `FeedsView.vue`**

In `frontend/src/views/FeedsView.vue`:
1. Import `PaginationControls from '../components/PaginationControls.vue'`.
2. Add pagination reactive state: `currentPage`, `pageSize` (using same `PAGE_SIZE_STORAGE_KEY`), `totalTimelineItems`, `totalPages`.
3. Update `fetchTimeline(feedId, forceRefresh)` to pass `currentPage.value` and `pageSize.value`. Unpack `res.data` into `timeline.value`, `totalTimelineItems.value = res.total`, `totalPages.value = res.total_pages`.
4. When switching feeds or clicking refresh, reset `currentPage.value = 1`.
5. Render `<PaginationControls>` below the timeline card container.

- [ ] **Step 3: Update and run tests**

Update `HomeAndArticleViews.test.ts` and `FeedsView.test.ts` fixtures with the `{ data: [...], total, page, limit, total_pages }` response shape.
Run:
`cd frontend && bun test src/components/HomeAndArticleViews.test.ts src/views/FeedsView.test.ts`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/Home.vue frontend/src/views/FeedsView.vue frontend/src/components/HomeAndArticleViews.test.ts frontend/src/views/FeedsView.test.ts
git commit -m "feat(ui): integrate pagination controls into Home and Feeds views"
```

---

### Task 6: Full Verification & Dependencies Check

**Files:**
- All modified files

- [ ] **Step 1: Run full verification suite (`make check`)**

Run: `make check`
Verify:
1. `go fmt` and `go vet` clean.
2. `go test -race -count=1 ./...` passes.
3. Frontend unit tests (`bun test`) pass.
4. Frontend typecheck (`vue-tsc -b`) passes.
5. Frontend production build passes.

- [ ] **Step 2: Commit any final polish**

```bash
git commit --allow-empty -m "chore: complete pagination implementation and verification"
```
