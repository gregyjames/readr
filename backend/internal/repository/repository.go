package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

var (
	ErrNotFound = errors.New("record not found")
)

// IsMOCArticle returns true if the article is a Map of Content (MOC) hub note.
func IsMOCArticle(title, tags string) bool {
	lowerTitle := strings.ToLower(title)
	if strings.HasPrefix(lowerTitle, "moc - ") || strings.HasPrefix(lowerTitle, "moc:") || strings.HasPrefix(lowerTitle, "moc ") || strings.EqualFold(title, "moc") {
		return true
	}
	tagList := strings.Split(tags, ",")
	for _, t := range tagList {
		if strings.TrimSpace(strings.ToLower(t)) == "moc" {
			return true
		}
	}
	return false
}

// ReadingTimeFromWords computes the formatted reading time string (e.g. "5 min read")
// for a given word count assuming 200 words per minute.
func ReadingTimeFromWords(words int) string {
	if words <= 0 {
		return "1 min read"
	}
	minutes := int(math.Ceil(float64(words) / 200.0))
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d min read", minutes)
}

// CountWordsBytes counts whitespace-delimited words in a byte slice in O(N) time
// with 0 heap allocations. It handles ASCII whitespace as well as Unicode whitespace runes.
func CountWordsBytes(b []byte) int {
	words := 0
	inWord := false
	i := 0
	n := len(b)

	for i < n {
		c := b[i]
		// Fast-path ASCII characters (covers >99% of markdown and English prose)
		if c < utf8.RuneSelf {
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' {
				inWord = false
			} else if !inWord {
				inWord = true
				words++
			}
			i++
			continue
		}

		// Unicode rune fallback (e.g. non-breaking space, em-space, ideographic space)
		r, size := utf8.DecodeRune(b[i:])
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
		i += size
	}

	return words
}

// StripFrontmatterBytes slices away leading YAML frontmatter (--- ... ---) from a byte slice
// without allocating new memory.
func StripFrontmatterBytes(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return trimmed
	}

	rest := trimmed[3:]
	if idx := bytes.Index(rest, []byte("\n---")); idx != -1 {
		return bytes.TrimSpace(rest[idx+4:])
	} else if idx := bytes.Index(rest, []byte("\r\n---")); idx != -1 {
		return bytes.TrimSpace(rest[idx+5:])
	}

	return trimmed
}

// CalculateReadingTimeBytes computes the word count and estimated reading time string
// directly from a byte slice with 0 allocations for word counting.
func CalculateReadingTimeBytes(content []byte) (int, string) {
	body := StripFrontmatterBytes(content)
	if len(body) == 0 {
		return 0, "1 min read"
	}
	words := CountWordsBytes(body)
	return words, ReadingTimeFromWords(words)
}

// CalculateReadingTime computes the word count and estimated reading time string (e.g. "5 min read")
// for markdown content. It strips leading YAML frontmatter (--- ... ---) and counts whitespace-separated words.
func CalculateReadingTime(content string) (int, string) {
	if len(content) == 0 {
		return 0, "1 min read"
	}
	// Zero-copy string to byte slice conversion for read-only scanning
	return CalculateReadingTimeBytes(unsafe.Slice(unsafe.StringData(content), len(content)))
}

type ArticleRecord struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	ImagePath       string  `json:"image"`
	FilePath        string  `json:"article"`
	Tags            string  `json:"tags"`
	SourceURL       string  `json:"sourceUrl"`
	IsArchived      bool    `json:"is_archived"`
	ReadingStatus   string  `json:"reading_status"`
	ReadingProgress float64 `json:"reading_progress"`
	ReadingTime     string  `json:"reading_time"`
	WordCount       int     `json:"word_count"`
}

type LinkRecord struct {
	ID       int64 `json:"id"`
	SourceID int64 `json:"sourceId"`
	TargetID int64 `json:"targetId"`
}

type Repository interface {
	FindBySourceURL(ctx context.Context, sourceURL string) (*ArticleRecord, error)
	FindByID(ctx context.Context, id int64) (*ArticleRecord, error)
	SaveArticle(ctx context.Context, a *ArticleRecord) error
	GetAllArticles(ctx context.Context) ([]ArticleRecord, error)
	GetArchivedArticles(ctx context.Context) ([]ArticleRecord, error)
	SetArticleArchived(ctx context.Context, id int64, archived bool) error
	GetAllLinks(ctx context.Context) ([]LinkRecord, error)
	CreateLink(ctx context.Context, sourceID, targetID int64) (*LinkRecord, error)
	DeleteArticle(ctx context.Context, id int64) error
	FindCandidates(ctx context.Context, excludeID int64, title string, body string, limit int) ([]ArticleRecord, error)
	FindRelevantTags(ctx context.Context, excludeID int64, title string, body string, limit int) ([]string, error)
	GetDistinctTags(ctx context.Context) ([]string, error)
	UpdateArticleTags(ctx context.Context, id int64, tags string) error
	RecordPipelineMetric(ctx context.Context, metric *PipelineMetric) error
	GetPipelineDiagnostics(ctx context.Context, limit int) (*PipelineDiagnosticsSummary, []PipelineMetric, error)
}
