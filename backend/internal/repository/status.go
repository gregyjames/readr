package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FinishedThreshold is the reading progress percentage at or above which an
// article counts as finished. It is deliberately below 100: trailing padding
// and the sticky reader HUD make a true 100 unreliable to reach.
const FinishedThreshold = 98.0

// Reading status keys. These are the stable identifiers stored in
// article_status_types.key; labels and colors are data, not code.
const (
	StatusNotStarted  = "not_started"
	StatusNotFinished = "not_finished"
	StatusFinished    = "finished"
)

// ErrInvalidStatus is returned when a caller supplies an unknown status key.
var ErrInvalidStatus = errors.New("invalid reading status")

// GormArticleStatusType is the lookup table of reading statuses. New statuses
// are new rows, so adding one never requires a schema migration.
type GormArticleStatusType struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Key       string `gorm:"uniqueIndex;not null" json:"key"`
	Label     string `json:"label"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

func (GormArticleStatusType) TableName() string {
	return "article_status_types"
}

// GormArticleStatus holds per-article reading state, one row per article.
// Absence of a row means the article has never been opened.
type GormArticleStatus struct {
	ArticleID    int64     `gorm:"primaryKey" json:"article_id"`
	StatusTypeID int64     `gorm:"index" json:"status_type_id"`
	StatusKey    string    `gorm:"index" json:"status_key"`
	Progress     float64   `gorm:"default:0" json:"progress"`
	IsManual     bool      `gorm:"default:false" json:"is_manual"`
	OpenedAt     time.Time `json:"opened_at"`
	LastReadAt   time.Time `json:"last_read_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (GormArticleStatus) TableName() string {
	return "article_statuses"
}

// defaultStatusTypes is the seed set. Colors are stored so the palette can be
// changed without touching code.
var defaultStatusTypes = []GormArticleStatusType{
	{Key: StatusNotStarted, Label: "Not Started", Color: "gray", SortOrder: 0},
	{Key: StatusNotFinished, Label: "Not Finished", Color: "blue", SortOrder: 1},
	{Key: StatusFinished, Label: "Finished", Color: "green", SortOrder: 2},
}

// IsValidStatusKey reports whether key names a known reading status.
func IsValidStatusKey(key string) bool {
	switch key {
	case StatusNotStarted, StatusNotFinished, StatusFinished:
		return true
	}
	return false
}

// DeriveStatusKey resolves the reading status for a row. A nil row means the
// article was never opened. A manual override always wins over progress.
func DeriveStatusKey(s *GormArticleStatus) string {
	if s == nil {
		return StatusNotStarted
	}
	if s.IsManual && IsValidStatusKey(s.StatusKey) {
		return s.StatusKey
	}
	if s.Progress >= FinishedThreshold {
		return StatusFinished
	}
	return StatusNotFinished
}

// EnsureArticleStatusTypes seeds the lookup table, creating only the rows that
// are missing. Safe to call on every boot.
func EnsureArticleStatusTypes(db *gorm.DB) error {
	for _, seed := range defaultStatusTypes {
		var existing GormArticleStatusType
		err := db.Where(GormArticleStatusType{Key: seed.Key}).Attrs(seed).FirstOrCreate(&existing).Error
		if err != nil {
			return fmt.Errorf("failed to seed status type %q: %w", seed.Key, err)
		}
	}
	return nil
}

// statusTypeID looks up the lookup-table id for a status key. A missing row is
// not fatal: the key itself is the source of truth for callers.
func (r *GormRepository) statusTypeID(ctx context.Context, key string) int64 {
	var statusType GormArticleStatusType
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&statusType).Error; err != nil {
		return 0
	}
	return statusType.ID
}

// isBusyErr reports whether err is SQLite's transient write-contention error.
//
// The database is opened without a busy_timeout, so a write that overlaps
// another one fails immediately rather than waiting. Reading an article fires a
// progress write while the reader is also loading its graph and images, which
// is exactly when that overlap happens.
func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}

// withBusyRetry retries a read-modify-write against transient lock contention.
func withBusyRetry(fn func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = fn(); !isBusyErr(err) {
			return err
		}
		time.Sleep(time.Duration(10*(attempt+1)) * time.Millisecond)
	}
	return err
}

// RecordProgress stores reading progress for an article. Progress is a
// high-water mark, so scrolling back up never downgrades a status, and a manual
// override is never overwritten.
func (r *GormRepository) RecordProgress(ctx context.Context, articleID int64, progress float64) (*GormArticleStatus, error) {
	var result *GormArticleStatus
	err := withBusyRetry(func() error {
		var innerErr error
		result, innerErr = r.recordProgress(ctx, articleID, progress)
		return innerErr
	})
	return result, err
}

func (r *GormRepository) recordProgress(ctx context.Context, articleID int64, progress float64) (*GormArticleStatus, error) {
	if articleID <= 0 {
		return nil, ErrNotFound
	}
	if math.IsNaN(progress) || math.IsInf(progress, 0) {
		return nil, fmt.Errorf("%w: progress must be a finite number", ErrInvalidStatus)
	}
	progress = math.Min(math.Max(progress, 0), 100)

	now := time.Now()
	var status GormArticleStatus
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).First(&status).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = GormArticleStatus{
			ArticleID: articleID,
			Progress:  progress,
			OpenedAt:  now,
		}
	case err != nil:
		return nil, err
	default:
		if status.IsManual {
			// A manual override stands; only refresh the read timestamp.
			status.LastReadAt = now
			if err := r.db.WithContext(ctx).Save(&status).Error; err != nil {
				return nil, err
			}
			return &status, nil
		}
		if progress > status.Progress {
			status.Progress = progress
		}
	}

	status.LastReadAt = now
	status.StatusKey = DeriveStatusKey(&status)
	status.StatusTypeID = r.statusTypeID(ctx, status.StatusKey)

	if err := r.db.WithContext(ctx).Save(&status).Error; err != nil {
		return nil, err
	}
	return &status, nil
}

// SetManualStatus applies a user-chosen status, overriding derivation.
// Clearing back to not-started removes the row entirely via ClearStatus.
func (r *GormRepository) SetManualStatus(ctx context.Context, articleID int64, key string) (*GormArticleStatus, error) {
	var result *GormArticleStatus
	err := withBusyRetry(func() error {
		var innerErr error
		result, innerErr = r.setManualStatus(ctx, articleID, key)
		return innerErr
	})
	return result, err
}

func (r *GormRepository) setManualStatus(ctx context.Context, articleID int64, key string) (*GormArticleStatus, error) {
	if articleID <= 0 {
		return nil, ErrNotFound
	}
	if !IsValidStatusKey(key) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, key)
	}
	if key == StatusNotStarted {
		return nil, r.ClearStatus(ctx, articleID)
	}

	now := time.Now()
	var status GormArticleStatus
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).First(&status).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = GormArticleStatus{ArticleID: articleID, OpenedAt: now}
	} else if err != nil {
		return nil, err
	}

	status.IsManual = true
	status.StatusKey = key
	if key == StatusFinished {
		status.Progress = 100
	}
	status.LastReadAt = now
	status.StatusTypeID = r.statusTypeID(ctx, key)

	if err := r.db.WithContext(ctx).Save(&status).Error; err != nil {
		return nil, err
	}
	return &status, nil
}

// ClearStatus removes an article's reading state, returning it to Not Started.
func (r *GormRepository) ClearStatus(ctx context.Context, articleID int64) error {
	return r.db.WithContext(ctx).Where("article_id = ?", articleID).Delete(&GormArticleStatus{}).Error
}

// GetStatus returns the reading state for one article, or nil when it has never
// been opened.
func (r *GormRepository) GetStatus(ctx context.Context, articleID int64) (*GormArticleStatus, error) {
	var status GormArticleStatus
	err := r.db.WithContext(ctx).Where("article_id = ?", articleID).First(&status).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// GetStatuses bulk-loads reading state for the given articles, keyed by article
// id. Used to hydrate list responses without an N+1.
func GetStatuses(ctx context.Context, db *gorm.DB, ids []int64) (map[int64]GormArticleStatus, error) {
	result := make(map[int64]GormArticleStatus, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var statuses []GormArticleStatus
	if err := db.WithContext(ctx).Where("article_id IN ?", ids).Find(&statuses).Error; err != nil {
		return nil, err
	}
	for _, status := range statuses {
		result[status.ArticleID] = status
	}
	return result, nil
}

// GetStatuses is the repository-scoped form of the package-level helper.
func (r *GormRepository) GetStatuses(ctx context.Context, ids []int64) (map[int64]GormArticleStatus, error) {
	return GetStatuses(ctx, r.db, ids)
}

// DeleteStatus removes reading state for an article on the given handle, which
// may be a transaction. Article deletion must call this: there are no foreign
// keys in this schema.
func DeleteStatus(db *gorm.DB, articleID int64) error {
	return db.Where("article_id = ?", articleID).Delete(&GormArticleStatus{}).Error
}
