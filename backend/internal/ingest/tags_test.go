package ingest

import (
	"reflect"
	"testing"
)

func TestSanitizeObsidianTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"google", "google"},
		{"gemini", "gemini"},
		{"legal tech", "legal-tech"},
		{"artificial intelligence", "artificial-intelligence"},
		{"enterprise ai", "enterprise-ai"},
		{"  spaced   out   tag  ", "spaced-out-tag"},
		{"c++", "c"},
		{"web_development", "web-development"},
		{"tech.stack", "tech-stack"},
		{"#hashtag", "hashtag"},
		{"AI/MachineLearning", "ai/machinelearning"},
		{"---leading-trailing---", "leading-trailing"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := SanitizeObsidianTag(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeObsidianTag(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizeObsidianTags(t *testing.T) {
	input := []string{
		"google",
		"gemini",
		"legal tech",
		"artificial intelligence",
		"enterprise ai",
		"moc",        // reserved, should be stripped
		"google",     // duplicate
		"legal-tech", // duplicate after sanitization
		"dev, programming, web design",
	}

	expected := []string{
		"google",
		"gemini",
		"legal-tech",
		"artificial-intelligence",
		"enterprise-ai",
		"dev",
		"programming",
		"web-design",
	}

	got := SanitizeObsidianTags(input)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("SanitizeObsidianTags() = %v, expected %v", got, expected)
	}
}
