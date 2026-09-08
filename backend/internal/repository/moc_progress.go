package repository

import (
	"context"

	"gorm.io/gorm"
)

// MOCProgress is the reading rollup for a Map of Content: how many of its
// member notes are finished and how many are underway. Not-started is the
// remainder, so callers derive it rather than trusting a third counter.
//
// A MOC never owns an article_statuses row of its own — scroll progress through
// a generated index is meaningless — so this is the only progress a hub has.
type MOCProgress struct {
	Total      int `json:"total"`
	Read       int `json:"read"`
	InProgress int `json:"in_progress"`
}

// mocMemberRow is one MOC-to-member edge joined to that member's reading state.
// StatusArticleID is zero when the member has no article_statuses row, which is
// how "never opened" reaches us through the LEFT JOIN.
type mocMemberRow struct {
	SourceID        int64
	TargetID        int64
	StatusArticleID int64
	StatusKey       string
	Progress        float64
	IsManual        bool
}

// Membership is the MOC's outbound links: the Librarian writes an article_links
// row per curated entry in the same transaction as the markdown wikilink, so
// source_id -> target_id is the collection.
const mocMemberQuery = `
SELECT l.source_id AS source_id,
       l.target_id AS target_id,
       COALESCE(s.article_id, 0) AS status_article_id,
       COALESCE(s.status_key, '') AS status_key,
       COALESCE(s.progress, 0) AS progress,
       COALESCE(s.is_manual, 0) AS is_manual
FROM article_links l
JOIN articles a ON a.id = l.target_id
LEFT JOIN article_statuses s ON s.article_id = a.id
WHERE l.source_id IN ?
  AND a.deleted_at IS NULL
  AND l.target_id <> l.source_id
`

// GetMOCProgress rolls up member reading state for the given MOCs in one query,
// keyed by MOC id. MOCs with no resolvable members are absent from the map
// rather than present with a zero Total, so callers can tell "empty hub" from
// "hub whose members are all unread".
func GetMOCProgress(ctx context.Context, db *gorm.DB, mocIDs []int64) (map[int64]MOCProgress, error) {
	result := make(map[int64]MOCProgress, len(mocIDs))
	if len(mocIDs) == 0 {
		return result, nil
	}

	var rows []mocMemberRow
	if err := db.WithContext(ctx).Raw(mocMemberQuery, mocIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// article_links has no unique constraint, so the same member can appear
	// twice for one MOC. Count each target once.
	seen := make(map[[2]int64]struct{}, len(rows))
	for _, row := range rows {
		key := [2]int64{row.SourceID, row.TargetID}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		progress := result[row.SourceID]
		progress.Total++

		// DeriveStatusKey owns the manual-override and FinishedThreshold rules;
		// comparing Progress here would quietly diverge from the article cards.
		var status *GormArticleStatus
		if row.StatusArticleID != 0 {
			status = &GormArticleStatus{
				ArticleID: row.StatusArticleID,
				StatusKey: row.StatusKey,
				Progress:  row.Progress,
				IsManual:  row.IsManual,
			}
		}
		switch DeriveStatusKey(status) {
		case StatusFinished:
			progress.Read++
		case StatusNotFinished:
			progress.InProgress++
		}

		result[row.SourceID] = progress
	}

	return result, nil
}

// HydrateMOCProgress fills MOCProgress on every MOC in articles. Non-MOCs and
// member-less MOCs are left nil, which is what keeps them off the wire and out
// of the UI.
func HydrateMOCProgress(ctx context.Context, db *gorm.DB, articles []GormArticle) error {
	if len(articles) == 0 || db == nil {
		return nil
	}

	var mocIDs []int64
	for _, article := range articles {
		if IsMOCArticle(article.Title, article.Tags) {
			mocIDs = append(mocIDs, article.ID)
		}
	}
	if len(mocIDs) == 0 {
		return nil
	}

	progressByMOC, err := GetMOCProgress(ctx, db, mocIDs)
	if err != nil {
		return err
	}

	for i := range articles {
		progress, ok := progressByMOC[articles[i].ID]
		if !ok || progress.Total == 0 {
			continue
		}
		articles[i].MOCProgress = &progress
	}
	return nil
}
