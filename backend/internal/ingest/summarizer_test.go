package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterSummarizer_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		res := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "This is a great summary of the article.",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(res)
	}))
	defer ts.Close()

	summarizer := NewOpenRouterSummarizer()
	summarizer.baseURL = ts.URL

	summary, err := summarizer.Summarize(context.Background(), "Title", "Body content", "test-key", "model-test")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if summary != "This is a great summary of the article." {
		t.Errorf("got %q, want %q", summary, "This is a great summary of the article.")
	}
}

func TestOpenRouterSummarizer_MissingKey(t *testing.T) {
	summarizer := NewOpenRouterSummarizer()
	_, err := summarizer.Summarize(context.Background(), "Title", "Body", "", "")
	if err == nil {
		t.Errorf("expected error when api key is missing")
	}
}
