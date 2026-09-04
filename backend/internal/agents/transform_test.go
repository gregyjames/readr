package agents

import (
	"strings"
	"testing"

	"example.com/backend/internal/repository"
)

func TestInjectLinksIntoBody_DeduplicatesIdenticalPhrases(t *testing.T) {
	body := "We are exploring Raft Consensus and distributed transactions."

	links := []llmLink{
		{
			ExistingArticleID: 1,
			ExactPhraseInText: "Raft Consensus",
		},
		{
			ExistingArticleID: 2,
			ExactPhraseInText: "Raft Consensus", // Duplicate phrase targeting different or same ID
		},
	}

	candidates := []repository.ArticleRecord{
		{ID: 1, Title: "Raft Algorithm", FilePath: "articles/Raft Algorithm.md"},
		{ID: 2, Title: "Raft Paper", FilePath: "articles/Raft Paper.md"},
	}

	newBody, injected := injectLinksIntoBody(body, links, candidates, 10, nil)

	// Verify only the first replacement for the duplicate phrase was applied
	if strings.Count(newBody, "[[Raft Algorithm|Raft Consensus]]") != 1 {
		t.Errorf("expected exactly 1 instance of first replacement in body, got:\n%s", newBody)
	}

	if len(injected) != 1 {
		t.Errorf("expected 1 injected link in return array, got %d: %+v", len(injected), injected)
	}
}
