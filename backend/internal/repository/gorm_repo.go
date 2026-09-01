package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

var defaultStopwords = map[string]struct{}{
	"about": {}, "above": {}, "after": {}, "again": {}, "against": {}, "all": {}, "and": {},
	"any": {}, "are": {}, "aren't": {}, "because": {}, "been": {}, "before": {}, "being": {},
	"below": {}, "between": {}, "both": {}, "but": {}, "cannot": {}, "could": {}, "couldn't": {},
	"did": {}, "didn't": {}, "does": {}, "doesn't": {}, "doing": {}, "don't": {}, "down": {},
	"during": {}, "each": {}, "few": {}, "for": {}, "from": {}, "further": {}, "had": {},
	"hadn't": {}, "has": {}, "hasn't": {}, "have": {}, "haven't": {}, "having": {}, "here": {},
	"how": {}, "into": {}, "more": {}, "most": {}, "mustn't": {}, "myself": {}, "nor": {},
	"not": {}, "off": {}, "once": {}, "only": {}, "other": {}, "ought": {}, "our": {},
	"ours": {}, "ourselves": {}, "out": {}, "over": {}, "own": {}, "same": {}, "shan't": {},
	"she": {}, "should": {}, "shouldn't": {}, "some": {}, "such": {}, "than": {}, "that": {},
	"the": {}, "their": {}, "theirs": {}, "them": {}, "themselves": {}, "then": {}, "there": {},
	"these": {}, "they": {}, "this": {}, "those": {}, "through": {}, "too": {}, "under": {},
	"until": {}, "very": {}, "was": {}, "wasn't": {}, "were": {}, "weren't": {}, "what": {},
	"when": {}, "where": {}, "which": {}, "while": {}, "who": {}, "whom": {}, "why": {},
	"with": {}, "won't": {}, "would": {}, "wouldn't": {}, "you": {}, "your": {}, "yours": {},
	"yourself": {}, "yourselves": {}, "http": {}, "https": {}, "www": {}, "com": {}, "org": {},
	"article": {}, "page": {}, "summary": {}, "read": {}, "using": {}, "across": {}, "explore": {},
}

func extractCandidateKeywords(title, body string, maxKeywords int) []string {
	if maxKeywords <= 0 {
		return nil
	}
	combined := title + " "
	bodyRunes := []rune(body)
	if len(bodyRunes) > 1000 {
		combined += string(bodyRunes[:1000])
	} else {
		combined += body
	}

	// Replace non-alphanumeric with spaces
	var cleaned strings.Builder
	for _, r := range combined {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			cleaned.WriteRune(r)
		} else {
			cleaned.WriteRune(' ')
		}
	}

	words := strings.Fields(strings.ToLower(cleaned.String()))
	seen := make(map[string]struct{})
	var result []string

	for _, w := range words {
		if len(w) < 3 || len(w) > 30 {
			continue
		}
		if _, isStop := defaultStopwords[w]; isStop {
			continue
		}
		if _, exists := seen[w]; exists {
			continue
		}
		seen[w] = struct{}{}
		result = append(result, w)
		if len(result) >= maxKeywords {
			break
		}
	}
	return result
}

type GormArticle struct {
	gorm.Model
	ID      int64  `gorm:"primaryKey"`
	Article string `json:"article"`
	Image   string `json:"image"`
	Title   string `json:"title"`
	Tags    string `json:"tags"`
}

func (GormArticle) TableName() string {
	return "articles"
}

type GormArticleLink struct {
	ID       int64 `gorm:"primaryKey" json:"id"`
	SourceID int64 `json:"sourceId"`
	TargetID int64 `json:"targetId"`
}

func (GormArticleLink) TableName() string {
	return "article_links"
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) FindBySourceURL(ctx context.Context, sourceURL string) (*ArticleRecord, error) {
	var a GormArticle
	if err := r.db.WithContext(ctx).Where("title = ?", sourceURL).First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ArticleRecord{
		ID:        a.ID,
		Title:     a.Title,
		ImagePath: a.Image,
		FilePath:  a.Article,
		Tags:      a.Tags,
		SourceURL: sourceURL,
	}, nil
}

func (r *GormRepository) FindByID(ctx context.Context, id int64) (*ArticleRecord, error) {
	var a GormArticle
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ArticleRecord{
		ID:        a.ID,
		Title:     a.Title,
		ImagePath: a.Image,
		FilePath:  a.Article,
		Tags:      a.Tags,
	}, nil
}

func (r *GormRepository) SaveArticle(ctx context.Context, a *ArticleRecord) error {
	article := GormArticle{
		ID:      a.ID,
		Title:   a.Title,
		Image:   a.ImagePath,
		Article: a.FilePath,
		Tags:    a.Tags,
	}
	return r.db.WithContext(ctx).Create(&article).Error
}

func (r *GormRepository) GetAllArticles(ctx context.Context) ([]ArticleRecord, error) {
	var articles []GormArticle
	if err := r.db.WithContext(ctx).Find(&articles).Error; err != nil {
		return nil, err
	}

	result := make([]ArticleRecord, len(articles))
	for i, a := range articles {
		result[i] = ArticleRecord{
			ID:        a.ID,
			Title:     a.Title,
			ImagePath: a.Image,
			FilePath:  a.Article,
			Tags:      a.Tags,
		}
	}
	return result, nil
}

func (r *GormRepository) GetAllLinks(ctx context.Context) ([]LinkRecord, error) {
	var links []GormArticleLink

	// Join with articles to ensure both source and target are NOT deleted
	err := r.db.WithContext(ctx).
		Joins("JOIN articles AS source ON source.id = article_links.source_id").
		Joins("JOIN articles AS target ON target.id = article_links.target_id").
		Where("source.deleted_at IS NULL AND target.deleted_at IS NULL").
		Find(&links).Error

	if err != nil {
		return nil, err
	}

	result := make([]LinkRecord, len(links))
	for i, l := range links {
		result[i] = LinkRecord{
			ID:       l.ID,
			SourceID: l.SourceID,
			TargetID: l.TargetID,
		}
	}
	return result, nil
}

func (r *GormRepository) CreateLink(ctx context.Context, sourceID, targetID int64) (*LinkRecord, error) {
	link := GormArticleLink{
		SourceID: sourceID,
		TargetID: targetID,
	}
	if err := r.db.WithContext(ctx).Create(&link).Error; err != nil {
		return nil, err
	}
	return &LinkRecord{
		ID:       link.ID,
		SourceID: link.SourceID,
		TargetID: link.TargetID,
	}, nil
}

func (r *GormRepository) DeleteArticle(ctx context.Context, id int64) error {
	r.db.WithContext(ctx).Exec("DELETE FROM article_links WHERE source_id = ? OR target_id = ?", id, id)
	return r.db.WithContext(ctx).Delete(&GormArticle{}, id).Error
}

func (r *GormRepository) FindCandidates(ctx context.Context, excludeID int64, title string, body string, limit int) ([]ArticleRecord, error) {
	if limit <= 0 {
		limit = 15
	}

	keywords := extractCandidateKeywords(title, body, 10)
	var candidates []ArticleRecord
	seenIDs := make(map[int64]struct{})
	if excludeID > 0 {
		seenIDs[excludeID] = struct{}{}
	}

	// 1. If we have keywords, try FTS5 query (excluding MOC hub notes)
	if len(keywords) > 0 {
		var queryParts []string
		for _, kw := range keywords {
			queryParts = append(queryParts, kw+"*")
		}
		safeFTSQuery := strings.Join(queryParts, " OR ")

		var matchedArticles []GormArticle
		err := r.db.WithContext(ctx).Raw(`
			SELECT a.id, a.title, a.image, a.article, a.tags
			FROM articles_fts fts
			JOIN articles a ON a.id = fts.rowid
			WHERE articles_fts MATCH ?
			  AND a.deleted_at IS NULL
			  AND a.id != ?
			  AND a.title NOT LIKE 'MOC - %'
			  AND a.title NOT LIKE 'MOC %'
			  AND a.title NOT LIKE 'MOC:%'
			  AND (a.tags NOT LIKE '%moc%' OR a.tags IS NULL)
			ORDER BY bm25(articles_fts, 2.0, 1.0)
			LIMIT ?
		`, safeFTSQuery, excludeID, limit).Scan(&matchedArticles).Error

		if err == nil {
			for _, a := range matchedArticles {
				if IsMOCArticle(a.Title, a.Tags) {
					continue
				}
				if _, exists := seenIDs[a.ID]; !exists {
					seenIDs[a.ID] = struct{}{}
					candidates = append(candidates, ArticleRecord{
						ID:        a.ID,
						Title:     a.Title,
						ImagePath: a.Image,
						FilePath:  a.Article,
						Tags:      a.Tags,
					})
				}
			}
		}
	}

	// 2. If under limit, backfill with recent active articles (excluding MOC hub notes)
	if len(candidates) < limit {
		needed := limit - len(candidates)
		var excludedList []int64
		for id := range seenIDs {
			excludedList = append(excludedList, id)
		}

		var fallbackArticles []GormArticle
		query := r.db.WithContext(ctx).Where("deleted_at IS NULL").
			Where("title NOT LIKE 'MOC - %' AND title NOT LIKE 'MOC %' AND title NOT LIKE 'MOC:%'").
			Where("tags NOT LIKE '%moc%' OR tags IS NULL")
		if len(excludedList) > 0 {
			query = query.Where("id NOT IN (?)", excludedList)
		}
		err := query.Order("created_at DESC").Limit(needed * 2).Find(&fallbackArticles).Error
		if err == nil {
			for _, a := range fallbackArticles {
				if IsMOCArticle(a.Title, a.Tags) {
					continue
				}
				if _, exists := seenIDs[a.ID]; !exists {
					seenIDs[a.ID] = struct{}{}
					candidates = append(candidates, ArticleRecord{
						ID:        a.ID,
						Title:     a.Title,
						ImagePath: a.Image,
						FilePath:  a.Article,
						Tags:      a.Tags,
					})
					if len(candidates) >= limit {
						break
					}
				}
			}
		}
	}

	return candidates, nil
}

func (r *GormRepository) GetDistinctTags(ctx context.Context) ([]string, error) {
	var rawTags []string
	err := r.db.WithContext(ctx).
		Model(&GormArticle{}).
		Where("deleted_at IS NULL AND tags != '' AND tags IS NOT NULL").
		Pluck("tags", &rawTags).Error
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var result []string
	for _, raw := range rawTags {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			tag := SanitizeObsidianTag(p)
			if tag == "" || tag == "moc" || len(tag) > 40 {
				continue
			}
			if _, exists := seen[tag]; !exists {
				seen[tag] = struct{}{}
				result = append(result, tag)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

func (r *GormRepository) UpdateArticleTags(ctx context.Context, id int64, tags string) error {
	sanitized := strings.Join(SanitizeObsidianTags(strings.Split(tags, ",")), ", ")
	return r.db.WithContext(ctx).
		Model(&GormArticle{}).
		Where("id = ?", id).
		Update("tags", sanitized).Error
}

func (r *GormRepository) RecordPipelineMetric(ctx context.Context, metric *PipelineMetric) error {
	if metric.CreatedAt.IsZero() {
		metric.CreatedAt = time.Now()
	}
	if metric.ID > 0 {
		return r.db.WithContext(ctx).Save(metric).Error
	}
	return r.db.WithContext(ctx).Create(metric).Error
}

func (r *GormRepository) GetPipelineDiagnostics(ctx context.Context, limit int) (*PipelineDiagnosticsSummary, []PipelineMetric, error) {
	if limit <= 0 {
		limit = 50
	}

	var metrics []PipelineMetric
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&metrics).Error
	if err != nil {
		return nil, nil, err
	}

	var summary PipelineDiagnosticsSummary
	type aggResult struct {
		TotalRuns             int64 `gorm:"column:total_runs"`
		SuccessfulRuns        int64 `gorm:"column:successful_runs"`
		FailedRuns            int64 `gorm:"column:failed_runs"`
		TotalRetries          int64 `gorm:"column:total_retries"`
		TotalDuration         int64 `gorm:"column:total_duration"`
		TotalTokensUsed       int64 `gorm:"column:total_tokens_used"`
		TotalPromptTokens     int64 `gorm:"column:total_prompt_tokens"`
		TotalCompletionTokens int64 `gorm:"column:total_completion_tokens"`
	}

	var agg aggResult
	r.db.WithContext(ctx).Model(&PipelineMetric{}).Select(`
		COUNT(*) as total_runs,
		SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as successful_runs,
		SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_runs,
		COALESCE(SUM(retry_count), 0) as total_retries,
		COALESCE(SUM(duration_ms), 0) as total_duration,
		COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total_tokens_used,
		COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) as total_completion_tokens
	`).Scan(&agg)

	totalRuns := agg.TotalRuns
	avgDuration := int64(0)
	if totalRuns > 0 {
		avgDuration = agg.TotalDuration / totalRuns
	}

	var p95Duration int64
	if totalRuns > 0 {
		var durations []int64
		r.db.WithContext(ctx).Model(&PipelineMetric{}).Order("duration_ms ASC").Pluck("duration_ms", &durations)
		if len(durations) > 0 {
			p95Index := int(float64(len(durations)) * 0.95)
			if p95Index >= len(durations) {
				p95Index = len(durations) - 1
			}
			p95Duration = durations[p95Index]
		}
	}

	summary = PipelineDiagnosticsSummary{
		TotalRuns:             totalRuns,
		SuccessfulRuns:        agg.SuccessfulRuns,
		FailedRuns:            agg.FailedRuns,
		TotalRetries:          agg.TotalRetries,
		AvgDurationMs:         avgDuration,
		P95DurationMs:         p95Duration,
		TotalTokensUsed:       agg.TotalTokensUsed,
		TotalPromptTokens:     agg.TotalPromptTokens,
		TotalCompletionTokens: agg.TotalCompletionTokens,
	}

	return &summary, metrics, nil
}
