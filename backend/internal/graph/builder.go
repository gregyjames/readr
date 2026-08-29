package graph

import (
	"fmt"
	"strings"

	"example.com/backend/internal/repository"
)

// BuildTopology generates full nodes and edges for articles, tags, and wikilinks.
func BuildTopology(articles []repository.ArticleRecord, links []repository.LinkRecord) *GraphData {
	nodes := make([]Node, 0, len(articles))
	edges := make([]Edge, 0, len(links))
	tagSet := make(map[string]bool)

	for _, article := range articles {
		nodes = append(nodes, Node{
			Id:    fmt.Sprintf("article-%d", article.ID),
			Label: article.Title,
			Group: "article",
		})

		if article.Tags != "" {
			tags := strings.Split(article.Tags, ",")
			articleTagSet := make(map[string]bool)

			for _, rawTag := range tags {
				tag := strings.ToLower(strings.TrimSpace(rawTag))
				if tag == "" || articleTagSet[tag] {
					continue
				}
				articleTagSet[tag] = true

				if !tagSet[tag] {
					nodes = append(nodes, Node{
						Id:    fmt.Sprintf("tag-%s", tag),
						Label: tag,
						Group: "tag",
					})
					tagSet[tag] = true
				}

				edges = append(edges, Edge{
					From: fmt.Sprintf("article-%d", article.ID),
					To:   fmt.Sprintf("tag-%s", tag),
				})
			}
		}
	}

	for _, link := range links {
		edges = append(edges, Edge{
			From: fmt.Sprintf("article-%d", link.SourceID),
			To:   fmt.Sprintf("article-%d", link.TargetID),
		})
	}

	return &GraphData{
		Nodes: nodes,
		Edges: edges,
	}
}

// ExtractLocalSubgraph filters the global graph for a target article up to `depth` hops.
func ExtractLocalSubgraph(global *GraphData, targetArticleID int64, depth int) *GraphData {
	if depth <= 0 {
		depth = 1
	}

	targetNodeID := fmt.Sprintf("article-%d", targetArticleID)

	// Build adjacency map
	adj := make(map[string][]string)
	for _, edge := range global.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		adj[edge.To] = append(adj[edge.To], edge.From)
	}

	// BFS traversal up to depth
	visited := make(map[string]bool)
	queue := []string{targetNodeID}
	visited[targetNodeID] = true

	for currentDepth := 0; currentDepth < depth && len(queue) > 0; currentDepth++ {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			curr := queue[0]
			queue = queue[1:]

			for _, neighbor := range adj[curr] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
	}

	// Filter nodes
	subNodes := make([]Node, 0)
	nodeMap := make(map[string]Node)
	for _, node := range global.Nodes {
		nodeMap[node.Id] = node
		if visited[node.Id] {
			subNodes = append(subNodes, node)
		}
	}

	// Filter edges where both endpoints are in visited
	subEdges := make([]Edge, 0)
	for _, edge := range global.Edges {
		if visited[edge.From] && visited[edge.To] {
			subEdges = append(subEdges, edge)
		}
	}

	return &GraphData{
		Nodes: subNodes,
		Edges: subEdges,
	}
}
