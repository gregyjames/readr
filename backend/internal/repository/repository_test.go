package repository

import "testing"

func TestCalculateReadingTime(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantWords    int
		wantReadTime string
	}{
		{
			name:         "empty content",
			content:      "",
			wantWords:    0,
			wantReadTime: "1 min read",
		},
		{
			name:         "whitespace only",
			content:      "   \n\t  \n ",
			wantWords:    0,
			wantReadTime: "1 min read",
		},
		{
			name:         "short content under 200 words",
			content:      "This is a short note with eight words.",
			wantWords:    8,
			wantReadTime: "1 min read",
		},
		{
			name:         "exact 200 words",
			content:      generateWords(200),
			wantWords:    200,
			wantReadTime: "1 min read",
		},
		{
			name:         "201 words rounds up to 2 min read",
			content:      generateWords(201),
			wantWords:    201,
			wantReadTime: "2 min read",
		},
		{
			name:         "1000 words yields 5 min read",
			content:      generateWords(1000),
			wantWords:    1000,
			wantReadTime: "5 min read",
		},
		{
			name:         "frontmatter is stripped from word count",
			content:      "---\ntitle: \"Super Long Title With Many Metadata Words\"\ntags: [golang, devops, kubernetes]\nsource: \"https://example.com/foo/bar\"\n---\n\nActual body text here with six words.",
			wantWords:    7,
			wantReadTime: "1 min read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words, readTime := CalculateReadingTime(tt.content)
			if words != tt.wantWords {
				t.Errorf("CalculateReadingTime() words = %v, want %v", words, tt.wantWords)
			}
			if readTime != tt.wantReadTime {
				t.Errorf("CalculateReadingTime() readTime = %v, want %v", readTime, tt.wantReadTime)
			}
		})
	}
}

func BenchmarkCalculateReadingTime(b *testing.B) {
	content := "---\ntitle: \"Sample\"\ntags: [test]\n---\n\n" + generateWords(2000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = CalculateReadingTime(content)
	}
}

func generateWords(n int) string {
	res := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			res += " "
		}
		res += "word"
	}
	return res
}
