package graph

import (
	"context"
	"fmt"
	"time"

	"example.com/backend/internal/repository"
	"github.com/patrickmn/go-cache"
)

const (
	GlobalGraphCacheKey = "global_graph"
	DefaultCacheTTL     = 10 * time.Minute
	DefaultCleanupTTL   = 15 * time.Minute
)

type GraphEngine struct {
	repo  repository.Repository
	cache *cache.Cache
}

func NewEngine(repo repository.Repository) *GraphEngine {
	return &GraphEngine{
		repo:  repo,
		cache: cache.New(DefaultCacheTTL, DefaultCleanupTTL),
	}
}

func (e *GraphEngine) BuildGlobalGraph(ctx context.Context) (*GraphData, error) {
	if cached, found := e.cache.Get(GlobalGraphCacheKey); found {
		if data, ok := cached.(*GraphData); ok {
			return data, nil
		}
	}

	articles, err := e.repo.GetAllArticles(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch articles failed: %w", err)
	}

	links, err := e.repo.GetAllLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch links failed: %w", err)
	}

	graph := BuildTopology(articles, links)
	e.cache.Set(GlobalGraphCacheKey, graph, cache.DefaultExpiration)
	return graph, nil
}

func (e *GraphEngine) BuildLocalGraph(ctx context.Context, articleID int64, depth int) (*GraphData, error) {
	globalGraph, err := e.BuildGlobalGraph(ctx)
	if err != nil {
		return nil, err
	}
	return ExtractLocalSubgraph(globalGraph, articleID, depth), nil
}

func (e *GraphEngine) InvalidateCache() {
	e.cache.Flush()
}
