package repository

import (
	"context"
	"errors"
	"strings"

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
	return nil, nil
}

