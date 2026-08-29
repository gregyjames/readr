# Architecture & Implementation Plan: Knowledge Graph Engine (003-knowledge-graph-engine)

## 1. Overview & Architectural Goals

The goal of this refactor is to deepen the **Knowledge Graph** subsystem into a cohesive domain engine (`backend/internal/graph/`), while unifying the shared repository layer (`backend/internal/repository/`) used across both Ingestion and Graph modules.

### Architectural Alignment
- **Module Depth**: The interface is simple (`BuildGlobalGraph(ctx)`, `BuildLocalGraph(ctx, id, depth)`), while the implementation encapsulates node generation, case-insensitive tag deduplication, bidirectional link edge mapping, local subgraph filtering, and concurrent thread-safe cache invalidation.
- **Seam**: Data access sits behind the shared `Repository` interface (`GetAllArticles`, `GetAllLinks`), allowing 100% in-memory unit tests of graph topology without SQLite.
- **Locality**: Graph ID construction rules (`article-{id}`, `tag-{name}`), group mappings, and tag clustering live in one dedicated boundary.
- **Leverage**: Instant sub-millisecond graph queries via in-memory caching that invalidates automatically when an article is ingested, edited, linked, or deleted.

---

## 2. Package & Interface Structure

```
backend/
├── internal/
│   ├── graph/
│   │   ├── types.go          # GraphNode, GraphEdge, GraphData, Engine interface
│   │   ├── engine.go         # GraphEngine orchestrator with in-memory caching
│   │   ├── builder.go        # Pure topology builder (global + local subgraphs)
│   │   └── engine_test.go    # 100% unit tests for topology and cache invalidation
│   ├── repository/
│   │   ├── repository.go     # Shared Repository interface & domain records
│   │   └── gorm_repo.go      # GORM implementation of Repository
│   └── ingest/               # Existing Deep Ingester using shared repository
├── main.go                   # Clean Fiber router using GraphEngine & Ingester
└── main_test.go
```

---

## 3. Interfaces & Contracts

```go
package graph

import "context"

type Node struct {
    ID    string `json:"id"`
    Label string `json:"label"`
    Group string `json:"group"` // "article" | "tag"
}

type Edge struct {
    From string `json:"from"`
    To   string `json:"to"`
}

type GraphData struct {
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}

type Engine interface {
    BuildGlobalGraph(ctx context.Context) (*GraphData, error)
    BuildLocalGraph(ctx context.Context, articleID int64, depth int) (*GraphData, error)
    InvalidateCache()
}
```

```go
package repository

import "context"

type ArticleRecord struct {
    ID        int64
    Title     string
    ImagePath string
    FilePath  string
    Tags      string
    SourceURL string
}

type LinkRecord struct {
    ID       int64
    SourceID int64
    TargetID int64
}

type Repository interface {
    FindBySourceURL(ctx context.Context, sourceURL string) (*ArticleRecord, error)
    FindByID(ctx context.Context, id int64) (*ArticleRecord, error)
    SaveArticle(ctx context.Context, a *ArticleRecord) error
    GetAllArticles(ctx context.Context) ([]ArticleRecord, error)
    GetAllLinks(ctx context.Context) ([]LinkRecord, error)
    CreateLink(ctx context.Context, sourceID, targetID int64) (*LinkRecord, error)
    DeleteArticle(ctx context.Context, id int64) error
}
```

---

## 4. Implementation Steps

1. **Step 1: Create Shared Repository (`backend/internal/repository/`)**
   - Define domain `ArticleRecord`, `LinkRecord`, and `Repository` interface.
   - Implement `GORMRepository` wrapping `*gorm.DB`.
   - Update `internal/ingest` to use the unified repository interface.

2. **Step 2: Define Graph Types & Topology Builder (`backend/internal/graph/`)**
   - Create `types.go` with `Node`, `Edge`, `GraphData`, `Engine`.
   - Create `builder.go` implementing pure graph construction functions:
     - Global topology construction (article nodes, tag nodes with case-insensitive deduplication, tag edges, article-to-article link edges).
     - Local subgraph extraction (filtered by starting `articleID` up to `depth` hops).

3. **Step 3: Implement Graph Engine with Invalidation Cache (`engine.go`)**
   - Implement thread-safe in-memory cache (`sync.RWMutex`).
   - `BuildGlobalGraph`: Returns cached graph if available; otherwise builds, caches, and returns.
   - `InvalidateCache`: Clears memoized graph.

4. **Step 4: Write Graph Unit Tests (`engine_test.go`)**
   - Test global topology construction with mock repository (articles with tags, bidirectional links, duplicate tags).
   - Test local subgraph generation (verifying only connected nodes and tags within N hops are returned).
   - Test cache invalidation on link/article mutations.

5. **Step 5: Wire Graph Engine into `backend/main.go`**
   - Instantiate `graphEngine := graph.NewEngine(repo)` in `setupApp()`.
   - Refactor `/api/graph` route to delegate directly to `graphEngine.BuildGlobalGraph(c.Context())`.
   - Invalidate graph cache on `/api/add`, `/api/link`, and `/api/delete/:id`.
   - Remove obsolete `GraphNode`, `GraphEdge` structs from `main.go`.

6. **Step 6: Verify Backend Tests**
   - Run `go test ./...` across all backend packages.
