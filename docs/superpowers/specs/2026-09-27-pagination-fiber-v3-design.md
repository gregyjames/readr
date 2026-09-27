# Fiber v3 Pagination Design Specification (Home & Feeds)

## 1. Overview
This specification details the implementation of pagination for the **Home** (`/api/getarticles`) and **Feeds** (`/api/feeds/timeline`) views in Readr. It directly uses Fiber v3's official pagination middleware (`github.com/gofiber/fiber/v3/middleware/paginate`) without custom pagination logic, optimizes network payload sizes and database memory usage, and introduces traditional page navigation with configurable page sizes on the frontend.

---

## 2. Goals & Non-Goals

### Goals
- Use official Fiber v3 middleware: `github.com/gofiber/fiber/v3/middleware/paginate`.
- Paginate article library queries on `GET /api/getarticles`.
- Paginate RSS timeline queries on `GET /api/feeds/timeline`.
- Provide a consistent JSON envelope for paginated endpoints: `{ "data": [...], "page": P, "limit": L, "total": N, "total_pages": T }`.
- Support an explicit `?all=true` parameter on `GET /api/getarticles` to preserve complete lists for internal lookup utilities (e.g. Chat mentions and Article view next/prev navigation).
- Create a reusable, accessible frontend pagination component (`PaginationControls.vue`).
- Allow users to select page sizes (`10`, `25`, `50`, `100`), defaulting to `25` and persisting the choice in `localStorage`.
- Significantly reduce memory and network transfer overhead (reducing megabyte payloads down to ~20–80 KB per page).

### Non-Goals
- Cursor/keyset pagination (arbitrary page navigation is required).
- Modifying full-text search (`/api/search`) which already handles its own search result limits.
- Modifying static asset routes.

---

## 3. Architecture & API Design

### 3.1 Fiber v3 Paginate Middleware
Fiber v3's `paginate` middleware extracts and validates query parameters (`page`, `limit`) and stores a `*paginate.PageInfo` struct in the request context:
```go
paginationMiddleware := paginate.New(paginate.Config{
    PageKey:      "page",
    LimitKey:     "limit",
    DefaultPage:  1,
    DefaultLimit: 25,
    MaxLimit:     100,
})
```
Handlers extract pagination metadata using:
```go
pageInfo, ok := paginate.FromContext(c)
```

### 3.2 Response Envelope
Both paginated endpoints return a typed JSON envelope:
```json
{
  "data": [ ... ],
  "page": 1,
  "limit": 25,
  "total": 142,
  "total_pages": 6
}
```

### 3.3 Backend Endpoints

#### 1. `GET /api/getarticles` (`backend/internal/handlers/articles.go`)
- **Query Parameters**:
  - `archived`: boolean (`"true"` or `"false"`, default `"false"`).
  - `page`: integer (default `1`, min `1`).
  - `limit`: integer (default `25`, max `100`).
  - `all`: boolean (optional). When `"true"`, bypasses pagination and returns all articles in the envelope (or slice) for internal lookup utilities.
- **Implementation**:
  - Build base query for `is_archived`.
  - Count matching records: `query.Count(&total)`.
  - Apply pagination: `query.Offset(pageInfo.Start()).Limit(pageInfo.Limit).Find(&articles)`.
  - Calculate `totalPages = int(math.Ceil(float64(total) / float64(pageInfo.Limit)))`. If `totalPages == 0`, `totalPages = 1`.
  - Hydrate reading status, MOC progress, and reading time *only* for the current page slice (`articles`).
  - Return the envelope.

#### 2. `GET /api/feeds/timeline` (`backend/internal/handlers/feeds.go`)
- **Query Parameters**:
  - `feed_id`: optional integer.
  - `refresh`: optional boolean (`"true"` or `"1"`).
  - `page`: integer (default `1`, min `1`).
  - `limit`: integer (default `25`, max `100`).
- **Implementation**:
  - Fetch feed timeline items via `ingest.FetchFeedsTimelineWithOptions(c.Context(), feeds, 10*time.Second, forceRefresh)`.
  - Record total count: `total := int64(len(items))`.
  - Slice in-memory items according to `pageInfo.Start()` and `pageInfo.Limit`:
    ```go
    start := pageInfo.Start()
    if start > len(items) {
        start = len(items)
    }
    end := start + pageInfo.Limit
    if end > len(items) {
        end = len(items)
    }
    pageItems := items[start:end]
    ```
  - Calculate `totalPages = int(math.Ceil(float64(total) / float64(pageInfo.Limit)))`. If `totalPages == 0`, `totalPages = 1`.
  - Return the envelope.

---

## 4. Frontend Design

### 4.1 Component: `frontend/src/components/PaginationControls.vue`
A dedicated, accessible pagination bar adhering to Readr's minimalist UI design system:
- **Props**:
  - `currentPage`: number (1-based).
  - `totalPages`: number.
  - `totalItems`: number.
  - `pageSize`: number.
  - `pageSizeOptions`: number[] (default: `[10, 25, 50, 100]`).
- **Emits**:
  - `update:page(page: number)`
  - `update:pageSize(size: number)`
- **Features**:
  - **Left Section**: Summary label (`Showing {start}–{end} of {totalItems} items`).
  - **Center Section**: Previous (`← Prev`), Next (`Next →`), and smart truncated page numbers (e.g. `[1] 2 3 ... 10` or `1 ... 4 [5] 6 ... 10`).
  - **Right Section**: Page size selector (`<select>` dropdown) bound to `pageSize`.

### 4.2 State & Storage
- Selected page size is read from and saved to `localStorage` under the key `readr_page_size` (default `25`).
- Changing page size resets `currentPage` to `1`.
- Changing tags, search filters, or feed selection resets `currentPage` to `1`.

### 4.3 View Integrations

#### 1. `Home.vue`
- Manages `currentPage` (1-based) and `pageSize`.
- Sends `GET /api/getarticles?page=${currentPage}&limit=${pageSize}&archived=false`.
- Unpacks `res.data.data` into `articles.value` and records `totalArticles.value` and `totalPages.value`.
- Renders `<PaginationControls>` underneath the article list.
- Scrolls smoothly to the top of the article container on page transition.

#### 2. `FeedsView.vue`
- Updates `feedsAPI.getTimeline(feedId?, forceRefresh?, page?, limit?)` in `frontend/src/services/feeds.ts`.
- Manages `currentPage` and `pageSize`.
- Renders `<PaginationControls>` at the bottom of the timeline feed cards.
- Resetting feed selection or clicking refresh resets `currentPage` to `1`.

#### 3. Other Views (`ArchiveView.vue`, `Article.vue`, `ChatView.vue`)
- `ArchiveView.vue`: Updated to utilize pagination with `archived=true`.
- `Article.vue`: Calls `/api/getarticles?all=true` so the next/previous article navigation continues to traverse the complete vault.
- `ChatView.vue`: Calls `/api/getarticles?all=true` so the `@` mention autocomplete continues to search across all vault articles.

---

## 5. Error Handling & Edge Cases
1. **Empty Datasets**: When `total == 0`, `PaginationControls` hides or displays `Showing 0 of 0 items` with disabled navigation.
2. **Page Clamping**: If a user is on page 5 and applies a tag filter with only 1 page of results, the UI clamps `currentPage` to `totalPages` and refetches.
3. **Out-of-Bounds Middleware Query**: Fiber's `paginate` middleware automatically clamps negative pages to `1` and limits exceeding `MaxLimit` to `100`.
4. **Network Failures**: UI retains existing list items and surfaces the standard error toast/banner without blanking out the view.

---

## 6. Testing Strategy
1. **Backend Tests**:
   - `backend/internal/handlers/articles_test.go`:
     - Test pagination parameters on `/api/getarticles`: verify page size, offsets, total count, and total pages.
     - Test `?all=true` returns complete unpaginated dataset.
   - `backend/internal/handlers/feeds_test.go`:
     - Test pagination on `/api/feeds/timeline`: verify slicing, bounds checking, and envelope fields.
2. **Frontend Tests**:
   - `frontend/src/components/PaginationControls.test.ts`:
     - Test page click emissions, boundary disable states (first/last page), and page size dropdown changes.
   - `frontend/src/components/HomeAndArticleViews.test.ts`:
     - Test pagination state updates and API calls on `Home.vue`.
   - `frontend/src/views/FeedsView.test.ts`:
     - Test timeline pagination rendering and page change interactions.
3. **Full System Verification**:
   - Run `make check` ensuring all Go tests (with `-race`), linting, typechecking (`vue-tsc -b`), and frontend unit tests pass.
