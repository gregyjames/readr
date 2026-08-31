package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeTitleFilename(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		fallbackID int64
		expected   string
	}{
		{
			name:       "clean standard title",
			title:      "Building Distributed Systems in Go",
			fallbackID: 101,
			expected:   "Building Distributed Systems in Go.md",
		},
		{
			name:       "strips forbidden filesystem characters",
			title:      "What is Raft? A Guide: Part 1/2 <v2> *awesome* | \"draft\"",
			fallbackID: 102,
			expected:   "What is Raft A Guide Part 1 2 v2 awesome draft.md",
		},
		{
			name:       "empty title falls back to ID",
			title:      "",
			fallbackID: 103,
			expected:   "Article 103.md",
		},
		{
			name:       "whitespace and punctuation only falls back to ID",
			title:      "   ??? :::: ///   ",
			fallbackID: 104,
			expected:   "Article 104.md",
		},
		{
			name:       "collapses excessive whitespace",
			title:      "Go    Channels     and      Routines",
			fallbackID: 105,
			expected:   "Go Channels and Routines.md",
		},
		{
			name:       "bounds very long title",
			title:      strings.Repeat("A Very Long Article Title ", 10),
			fallbackID: 106,
		},
		{
			name:       "handles Windows reserved name CON",
			title:      "CON",
			fallbackID: 107,
			expected:   "Article 107.md",
		},
		{
			name:       "handles Windows reserved name aux (case insensitive)",
			title:      "aux",
			fallbackID: 108,
			expected:   "Article 108.md",
		},
		{
			name:       "handles Windows reserved name NUL without fallback ID",
			title:      "NUL",
			fallbackID: 0,
			expected:   "Article.md",
		},
		{
			name:       "handles Windows reserved name COM1",
			title:      "com1",
			fallbackID: 109,
			expected:   "Article 109.md",
		},
		{
			name:       "handles Windows reserved name LPT9",
			title:      "lpt9",
			fallbackID: 110,
			expected:   "Article 110.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeTitleFilename(tt.title, tt.fallbackID)
			if tt.expected != "" && got != tt.expected {
				t.Errorf("SanitizeTitleFilename(%q, %d) = %q, want %q", tt.title, tt.fallbackID, got, tt.expected)
			}
			if len([]rune(got)) > 130 {
				t.Errorf("SanitizeTitleFilename output length %d exceeded maximum expected length", len([]rune(got)))
			}
			if !strings.HasSuffix(got, ".md") {
				t.Errorf("expected output to end in .md, got %q", got)
			}
		})
	}
}

func TestSaveMarkdownByTitle_CollisionHandling(t *testing.T) {
	tempDir := t.TempDir()
	storage := NewDiskStorage(tempDir)

	// Save first article
	rel1, err := storage.SaveMarkdownByTitle("Concurrent Patterns", 1, []byte("Content 1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel1 != "/articles/Concurrent Patterns.md" {
		t.Errorf("expected '/articles/Concurrent Patterns.md', got %q", rel1)
	}
	data1, _ := os.ReadFile(filepath.Join(tempDir, "articles", "Concurrent Patterns.md"))
	if string(data1) != "Content 1" {
		t.Errorf("expected 'Content 1', got %q", string(data1))
	}

	// Save duplicate title article
	rel2, err := storage.SaveMarkdownByTitle("Concurrent Patterns", 2, []byte("Content 2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel2 != "/articles/Concurrent Patterns (1).md" {
		t.Errorf("expected '/articles/Concurrent Patterns (1).md', got %q", rel2)
	}
	data2, _ := os.ReadFile(filepath.Join(tempDir, "articles", "Concurrent Patterns (1).md"))
	if string(data2) != "Content 2" {
		t.Errorf("expected 'Content 2', got %q", string(data2))
	}

	// Save third duplicate title article
	rel3, err := storage.SaveMarkdownByTitle("Concurrent Patterns", 3, []byte("Content 3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel3 != "/articles/Concurrent Patterns (2).md" {
		t.Errorf("expected '/articles/Concurrent Patterns (2).md', got %q", rel3)
	}
}
