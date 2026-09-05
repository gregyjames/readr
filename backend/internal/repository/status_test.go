package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func setupStatusTestRepo(t *testing.T) (*GormRepository, *gorm.DB) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&GormArticle{}, &GormArticleStatusType{}, &GormArticleStatus{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureArticleStatusTypes(db); err != nil {
		t.Fatal(err)
	}
	db.Create(&GormArticle{ID: 1, Title: "Test Article", Article: "/articles/1.md"})

	return NewGormRepository(db), db
}

func TestDeriveStatusKey(t *testing.T) {
	tests := []struct {
		name   string
		status *GormArticleStatus
		want   string
	}{
		{"never opened", nil, StatusNotStarted},
		{"opened, no progress", &GormArticleStatus{Progress: 0}, StatusNotFinished},
		{"halfway", &GormArticleStatus{Progress: 50}, StatusNotFinished},
		{"just under threshold", &GormArticleStatus{Progress: 97.9}, StatusNotFinished},
		{"at threshold", &GormArticleStatus{Progress: FinishedThreshold}, StatusFinished},
		{"complete", &GormArticleStatus{Progress: 100}, StatusFinished},
		{
			"manual finish beats low progress",
			&GormArticleStatus{Progress: 12, IsManual: true, StatusKey: StatusFinished},
			StatusFinished,
		},
		{
			"manual unfinished beats high progress",
			&GormArticleStatus{Progress: 100, IsManual: true, StatusKey: StatusNotFinished},
			StatusNotFinished,
		},
		{
			"manual with unknown key falls back to progress",
			&GormArticleStatus{Progress: 100, IsManual: true, StatusKey: "bogus"},
			StatusFinished,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveStatusKey(tt.status); got != tt.want {
				t.Errorf("DeriveStatusKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureArticleStatusTypesIsIdempotent(t *testing.T) {
	_, db := setupStatusTestRepo(t)

	// setupStatusTestRepo already seeded once; a second pass must not duplicate.
	if err := EnsureArticleStatusTypes(db); err != nil {
		t.Fatal(err)
	}

	var count int64
	db.Model(&GormArticleStatusType{}).Count(&count)
	if count != int64(len(defaultStatusTypes)) {
		t.Errorf("expected %d status types, got %d", len(defaultStatusTypes), count)
	}
}

func TestRecordProgressIsMonotonic(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordProgress(ctx, 1, 50); err != nil {
		t.Fatal(err)
	}
	status, err := repo.RecordProgress(ctx, 1, 20)
	if err != nil {
		t.Fatal(err)
	}

	if status.Progress != 50 {
		t.Errorf("progress regressed to %v, want 50", status.Progress)
	}
	if DeriveStatusKey(status) != StatusNotFinished {
		t.Errorf("status = %q, want %q", DeriveStatusKey(status), StatusNotFinished)
	}
}

func TestRecordProgressFinishesAtThreshold(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)
	ctx := context.Background()

	status, err := repo.RecordProgress(ctx, 1, FinishedThreshold)
	if err != nil {
		t.Fatal(err)
	}
	if DeriveStatusKey(status) != StatusFinished {
		t.Errorf("status = %q, want %q", DeriveStatusKey(status), StatusFinished)
	}
	if status.StatusTypeID == 0 {
		t.Error("expected the lookup table id to be resolved")
	}
}

func TestRecordProgressClampsRange(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)
	ctx := context.Background()

	status, err := repo.RecordProgress(ctx, 1, 250)
	if err != nil {
		t.Fatal(err)
	}
	if status.Progress != 100 {
		t.Errorf("progress = %v, want it clamped to 100", status.Progress)
	}
}

func TestManualFinishSurvivesLaterProgress(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)
	ctx := context.Background()

	if _, err := repo.SetManualStatus(ctx, 1, StatusFinished); err != nil {
		t.Fatal(err)
	}
	status, err := repo.RecordProgress(ctx, 1, 5)
	if err != nil {
		t.Fatal(err)
	}

	if DeriveStatusKey(status) != StatusFinished {
		t.Errorf("manual finish was overwritten: status = %q", DeriveStatusKey(status))
	}
	if status.Progress != 100 {
		t.Errorf("progress = %v, want 100", status.Progress)
	}
}

func TestSetManualStatusRejectsUnknownKey(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)

	if _, err := repo.SetManualStatus(context.Background(), 1, "sideways"); err == nil {
		t.Error("expected an error for an unknown status key")
	}
}

func TestSetManualNotStartedClearsRow(t *testing.T) {
	repo, db := setupStatusTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordProgress(ctx, 1, 60); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetManualStatus(ctx, 1, StatusNotStarted); err != nil {
		t.Fatal(err)
	}

	var count int64
	db.Model(&GormArticleStatus{}).Where("article_id = ?", 1).Count(&count)
	if count != 0 {
		t.Errorf("expected the status row to be removed, found %d", count)
	}
}

func TestClearStatusRoundTrip(t *testing.T) {
	repo, _ := setupStatusTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordProgress(ctx, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearStatus(ctx, 1); err != nil {
		t.Fatal(err)
	}

	status, err := repo.GetStatus(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if status != nil {
		t.Fatalf("expected no status row after clear, got %+v", status)
	}
	if DeriveStatusKey(status) != StatusNotStarted {
		t.Errorf("status = %q, want %q", DeriveStatusKey(status), StatusNotStarted)
	}
}

func TestGetStatusesBulkLookup(t *testing.T) {
	repo, db := setupStatusTestRepo(t)
	ctx := context.Background()

	db.Create(&GormArticle{ID: 2, Title: "Second", Article: "/articles/2.md"})
	if _, err := repo.RecordProgress(ctx, 1, 30); err != nil {
		t.Fatal(err)
	}

	statuses, err := repo.GetStatuses(ctx, []int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status row, got %d", len(statuses))
	}
	if statuses[1].Progress != 30 {
		t.Errorf("progress = %v, want 30", statuses[1].Progress)
	}
	if _, ok := statuses[2]; ok {
		t.Error("unopened article should have no status row")
	}
}

func TestDeleteArticleRemovesStatus(t *testing.T) {
	repo, db := setupStatusTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordProgress(ctx, 1, 42); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteArticle(ctx, 1); err != nil {
		t.Fatal(err)
	}

	var count int64
	db.Model(&GormArticleStatus{}).Where("article_id = ?", 1).Count(&count)
	if count != 0 {
		t.Errorf("orphaned status row left behind after delete: %d", count)
	}
}
