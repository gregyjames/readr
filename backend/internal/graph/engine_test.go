package graph

import (
	"context"
	"testing"

	"example.com/backend/internal/repository"
)

type mockGraphRepository struct {
	articles []repository.ArticleRecord
	links    []repository.LinkRecord
}

func (m *mockGraphRepository) GetAllArticles(ctx context.Context) ([]repository.ArticleRecord, error) {
	return m.articles, nil
}

func (m *mockGraphRepository) GetAllLinks(ctx context.Context) ([]repository.LinkRecord, error) {
	return m.links, nil
}

func (m *mockGraphRepository) FindBySourceURL(ctx context.Context, sourceURL string) (*repository.ArticleRecord, error) {
	return nil, nil
}

func (m *mockGraphRepository) FindByID(ctx context.Context, id int64) (*repository.ArticleRecord, error) {
	return nil, nil
}

func (m *mockGraphRepository) SaveArticle(ctx context.Context, a *repository.ArticleRecord) error {
	return nil
}

func (m *mockGraphRepository) CreateLink(ctx context.Context, sourceID, targetID int64) (*repository.LinkRecord, error) {
	return nil, nil
}

func (m *mockGraphRepository) DeleteArticle(ctx context.Context, id int64) error {
	return nil
}

func (m *mockGraphRepository) FindCandidates(ctx context.Context, excludeID int64, title string, body string, limit int) ([]repository.ArticleRecord, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetDistinctTags(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockGraphRepository) UpdateArticleTags(ctx context.Context, id int64, tags string) error {
	return nil
}

func (m *mockGraphRepository) RecordPipelineMetric(ctx context.Context, metric *repository.PipelineMetric) error {
	return nil
}

func (m *mockGraphRepository) GetPipelineDiagnostics(ctx context.Context, limit int) (*repository.PipelineDiagnosticsSummary, []repository.PipelineMetric, error) {
	return &repository.PipelineDiagnosticsSummary{}, nil, nil
}

func TestBuildTopology_NodesAndEdges(t *testing.T) {
	articles := []repository.ArticleRecord{
		{ID: 1, Title: "Article One", Tags: "Go, backend, GO"},
		{ID: 2, Title: "Article Two", Tags: "backend, database"},
	}
	links := []repository.LinkRecord{
		{ID: 1, SourceID: 1, TargetID: 2},
	}

	graph := BuildTopology(articles, links)

	// Expected nodes: Article 1, Article 2, Tag "go", Tag "backend", Tag "database" (Total 5)
	if len(graph.Nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d: %+v", len(graph.Nodes), graph.Nodes)
	}

	// Expected edges: Article 1->go, Article 1->backend, Article 2->backend, Article 2->database, Article 1->Article 2 (Total 5)
	if len(graph.Edges) != 5 {
		t.Errorf("expected 5 edges, got %d: %+v", len(graph.Edges), graph.Edges)
	}

	// Verify deduplicated tags
	tagCount := 0
	for _, n := range graph.Nodes {
		if n.Group == "tag" {
			tagCount++
		}
	}
	if tagCount != 3 {
		t.Errorf("expected 3 tag nodes (go, backend, database), got %d", tagCount)
	}
}

func TestExtractLocalSubgraph(t *testing.T) {
	articles := []repository.ArticleRecord{
		{ID: 1, Title: "Root Article", Tags: "tech"},
		{ID: 2, Title: "Direct Connection", Tags: "science"},
		{ID: 3, Title: "Distant Article", Tags: "history"},
	}
	links := []repository.LinkRecord{
		{ID: 1, SourceID: 1, TargetID: 2},
		{ID: 2, SourceID: 2, TargetID: 3},
	}

	global := BuildTopology(articles, links)

	// 1-hop subgraph from Article 1
	subgraph := ExtractLocalSubgraph(global, 1, 1)

	// Should contain: Article 1, Article 2, Tag "tech", Tag "science" (Distant Article 3 and Tag "history" are 2 hops away)
	nodeIDs := make(map[string]bool)
	for _, n := range subgraph.Nodes {
		nodeIDs[n.Id] = true
	}

	if !nodeIDs["article-1"] || !nodeIDs["article-2"] || !nodeIDs["tag-tech"] {
		t.Errorf("expected local 1-hop subgraph to contain article-1, article-2, tag-tech; got: %+v", nodeIDs)
	}

	if nodeIDs["article-3"] {
		t.Errorf("article-3 is 2 hops away and should NOT be in 1-hop subgraph")
	}
	if nodeIDs["tag-history"] {
		t.Errorf("tag-history should NOT be in 1-hop subgraph")
	}
}

func TestGraphEngine_CachingAndInvalidation(t *testing.T) {
	repo := &mockGraphRepository{
		articles: []repository.ArticleRecord{
			{ID: 1, Title: "First Article", Tags: "tag1"},
		},
		links: []repository.LinkRecord{},
	}

	engine := NewEngine(repo)

	// First query: populates cache
	g1, err := engine.BuildGlobalGraph(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g1.Nodes) != 2 {
		t.Errorf("expected 2 nodes (article + tag), got %d", len(g1.Nodes))
	}

	// Mutate repo data directly
	repo.articles = append(repo.articles, repository.ArticleRecord{ID: 2, Title: "Second Article"})

	// Query without invalidation: returns cached graph (2 nodes)
	g2, err := engine.BuildGlobalGraph(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g2.Nodes) != 2 {
		t.Errorf("expected cached 2 nodes, got %d", len(g2.Nodes))
	}

	// Invalidate cache
	engine.InvalidateCache()

	// Query after invalidation: returns fresh graph (3 nodes)
	g3, err := engine.BuildGlobalGraph(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g3.Nodes) != 3 {
		t.Errorf("expected 3 nodes after cache invalidation, got %d", len(g3.Nodes))
	}
}
