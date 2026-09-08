package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"
)

// seedMOCVault creates one MOC and the given member articles, linking each to
// the MOC. It returns the MOC's id.
func seedMOCVault(t *testing.T, db *gorm.DB, mocID int64, memberIDs ...int64) int64 {
	t.Helper()

	if err := db.Create(&GormArticle{ID: mocID, Title: "MOC - Kubernetes", Tags: "k8s", Article: "/articles/Kubernetes/MOC - Kubernetes.md"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range memberIDs {
		if err := db.Create(&GormArticle{ID: id, Title: "Member", Article: "/articles/Kubernetes/Member.md"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", mocID, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	return mocID
}

func setStatus(t *testing.T, db *gorm.DB, articleID int64, progress float64, manual bool, key string) {
	t.Helper()
	status := GormArticleStatus{
		ArticleID:  articleID,
		Progress:   progress,
		IsManual:   manual,
		StatusKey:  key,
		OpenedAt:   time.Now(),
		LastReadAt: time.Now(),
	}
	if err := db.Create(&status).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGetMOCProgressMixedRollup(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	moc := seedMOCVault(t, db, 100, 101, 102, 103, 104)

	setStatus(t, db, 101, 100, false, StatusFinished) // finished by progress
	setStatus(t, db, 102, 42, false, StatusNotFinished)
	setStatus(t, db, 103, 12, false, StatusNotFinished)
	// 104 has no row at all -> never opened

	got, err := GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}

	want := MOCProgress{Total: 4, Read: 1, InProgress: 2}
	if got[moc] != want {
		t.Fatalf("got %+v, want %+v", got[moc], want)
	}
}

// A member marked finished by hand counts as read even though its recorded
// progress is nowhere near the threshold.
func TestGetMOCProgressHonorsManualOverride(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	moc := seedMOCVault(t, db, 100, 101, 102)

	setStatus(t, db, 101, 3, true, StatusFinished)
	setStatus(t, db, 102, 99, true, StatusNotFinished) // manual override downward

	got, err := GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}

	want := MOCProgress{Total: 2, Read: 1, InProgress: 1}
	if got[moc] != want {
		t.Fatalf("got %+v, want %+v", got[moc], want)
	}
}

func TestGetMOCProgressExcludesDeletedMembers(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	moc := seedMOCVault(t, db, 100, 101, 102)

	if err := db.Delete(&GormArticle{}, 102).Error; err != nil {
		t.Fatal(err)
	}

	got, err := GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}

	if got[moc].Total != 1 {
		t.Fatalf("got Total %d, want 1", got[moc].Total)
	}
}

// article_links has no unique constraint, so a duplicated edge must not inflate
// the member count.
func TestGetMOCProgressDedupesDuplicateLinks(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	moc := seedMOCVault(t, db, 100, 101)

	if err := db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", moc, 101).Error; err != nil {
		t.Fatal(err)
	}
	setStatus(t, db, 101, 100, false, StatusFinished)

	got, err := GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}

	want := MOCProgress{Total: 1, Read: 1}
	if got[moc] != want {
		t.Fatalf("got %+v, want %+v", got[moc], want)
	}
}

// A self-link would otherwise make a hub count itself as one of its own notes.
func TestGetMOCProgressIgnoresSelfLink(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	moc := seedMOCVault(t, db, 100, 101)

	if err := db.Exec("INSERT INTO article_links (source_id, target_id) VALUES (?, ?)", moc, moc).Error; err != nil {
		t.Fatal(err)
	}

	got, err := GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}

	if got[moc].Total != 1 {
		t.Fatalf("got Total %d, want 1", got[moc].Total)
	}
}

func TestGetMOCProgressEmptyAndMemberless(t *testing.T) {
	_, db := setupStatusTestRepo(t)

	got, err := GetMOCProgress(context.Background(), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries for no ids, want 0", len(got))
	}

	moc := seedMOCVault(t, db, 100)
	got, err = GetMOCProgress(context.Background(), db, []int64{moc})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[moc]; ok {
		t.Fatalf("member-less MOC should be absent from the map, got %+v", got[moc])
	}
}

func TestHydrateMOCProgress(t *testing.T) {
	_, db := setupStatusTestRepo(t)
	seedMOCVault(t, db, 100, 101, 102)
	setStatus(t, db, 101, 100, false, StatusFinished)

	// A second hub with no members, and a plain note, must both stay nil.
	if err := db.Create(&GormArticle{ID: 200, Title: "MOC - Empty", Article: "/articles/Empty/MOC - Empty.md"}).Error; err != nil {
		t.Fatal(err)
	}

	articles := []GormArticle{
		{ID: 100, Title: "MOC - Kubernetes", Tags: "k8s"},
		{ID: 200, Title: "MOC - Empty"},
		{ID: 101, Title: "Member"},
	}
	if err := HydrateMOCProgress(context.Background(), db, articles); err != nil {
		t.Fatal(err)
	}

	if articles[0].MOCProgress == nil {
		t.Fatal("hub with members should be hydrated")
	}
	want := MOCProgress{Total: 2, Read: 1}
	if *articles[0].MOCProgress != want {
		t.Fatalf("got %+v, want %+v", *articles[0].MOCProgress, want)
	}
	if articles[1].MOCProgress != nil {
		t.Fatalf("member-less hub should stay nil, got %+v", *articles[1].MOCProgress)
	}
	if articles[2].MOCProgress != nil {
		t.Fatalf("plain note should stay nil, got %+v", *articles[2].MOCProgress)
	}
}
