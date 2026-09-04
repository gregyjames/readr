package agents

import (
	"testing"

	"example.com/backend/internal/repository"
)

func TestDetectClustersFromArticles_TagNormalizationWithExistingMOC(t *testing.T) {
	existingMOC := repository.ArticleRecord{
		ID:    100,
		Title: "MOC - Machine Learning",
		Tags:  "#machine_learning, moc",
	}

	articles := []repository.ArticleRecord{
		existingMOC,
		{
			ID:    1,
			Title: "Intro to ML",
			Tags:  "#Machine_Learning, ai",
		},
		{
			ID:    2,
			Title: "Deep Learning Guide",
			Tags:  "machine-learning, python",
		},
	}

	clusters := DetectClustersFromArticles(articles, 2)
	var mlCluster *ClusterCandidate
	for i := range clusters {
		if clusters[i].Tag == "machine-learning" {
			mlCluster = &clusters[i]
			break
		}
	}

	if mlCluster == nil {
		t.Fatalf("expected cluster for 'machine-learning', got none. Clusters: %+v", clusters)
	}

	if mlCluster.ExistingMOC == nil {
		t.Fatalf("expected existing MOC to be matched to cluster, got nil")
	}

	if mlCluster.ExistingMOC.ID != 100 {
		t.Errorf("expected existing MOC ID 100, got %d", mlCluster.ExistingMOC.ID)
	}

	if len(mlCluster.Articles) != 2 {
		t.Errorf("expected 2 articles in cluster, got %d", len(mlCluster.Articles))
	}
}
