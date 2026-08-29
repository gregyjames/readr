package graph

import "context"

type NodeGroup string

const (
	GroupArticle NodeGroup = "article"
	GroupTag     NodeGroup = "tag"
)

type Node struct {
	Id    string    `json:"id"`
	Label string    `json:"label"`
	Group NodeGroup `json:"group"`
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
