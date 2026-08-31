package repository

import (
	"testing"
)

func TestExtractCandidateKeywords(t *testing.T) {
	title := "Building Distributed Systems with Go and Kubernetes"
	body := "In this article, we explore how to deploy containerized microservices across clusters using Docker and Raft consensus."

	keywords := extractCandidateKeywords(title, body, 8)

	expectedSubset := []string{"building", "distributed", "systems", "kubernetes", "deploy", "containerized", "microservices", "clusters"}
	for _, kw := range expectedSubset {
		found := false
		for _, k := range keywords {
			if k == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected keyword %q to be in extracted keywords %v", kw, keywords)
		}
	}

	// Verify stopwords like "with", "and", "in", "this", "we", "how", "to", "across", "using" are filtered out
	stopwords := []string{"with", "and", "in", "this", "we", "how", "to", "across", "using"}
	for _, sw := range stopwords {
		for _, k := range keywords {
			if k == sw {
				t.Errorf("stopword %q should not be in extracted keywords", sw)
			}
		}
	}
}
