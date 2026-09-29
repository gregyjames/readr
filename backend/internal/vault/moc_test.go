package vault

import (
	"testing"

	"example.com/backend/internal/repository"
)

func TestIsMOCArticle(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		tags     string
		expected bool
	}{
		{
			name:     "prefix moc - ",
			title:    "MOC - Software Architecture",
			tags:     "architecture",
			expected: true,
		},
		{
			name:     "prefix moc: ",
			title:    "moc: Database Indexing",
			tags:     "",
			expected: true,
		},
		{
			name:     "prefix moc with space",
			title:    "  MOC Distributed Systems",
			tags:     "",
			expected: true,
		},
		{
			name:     "title is exact moc with whitespace",
			title:    "   Moc   ",
			tags:     "",
			expected: true,
		},
		{
			name:     "tag equals moc",
			title:    "System Overview",
			tags:     "architecture, moc, systems",
			expected: true,
		},
		{
			name:     "tag equals moc with extra spacing",
			title:    "System Overview",
			tags:     "architecture,   MOC  , systems",
			expected: true,
		},
		{
			name:     "substring tag democracy does not count as moc",
			title:    "Ancient Athens",
			tags:     "politics, democracy, history",
			expected: false,
		},
		{
			name:     "substring tag mockery does not count as moc",
			title:    "Mockery in Unit Tests",
			tags:     "mockery, testing",
			expected: false,
		},
		{
			name:     "regular note",
			title:    "Go Concurrency Deep Dive",
			tags:     "golang, concurrency",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsMOCArticle(tc.title, tc.tags)
			if actual != tc.expected {
				t.Errorf("IsMOCArticle(%q, %q) = %v, expected %v", tc.title, tc.tags, actual, tc.expected)
			}
		})
	}
}

func TestMocSQLCondition_DemocracyTagDoesNotCountAsMoc(t *testing.T) {
	_, db, _, _ := setupTestVaultEnv(t)

	democracyArticle := repository.GormArticle{
		Title: "The Athenian Constitution",
		Tags:  "politics, democracy, history",
	}
	mocArticle := repository.GormArticle{
		Title: "Government Models",
		Tags:  "politics, moc, history",
	}

	if err := db.Create(&democracyArticle).Error; err != nil {
		t.Fatalf("failed to create democracyArticle: %v", err)
	}
	if err := db.Create(&mocArticle).Error; err != nil {
		t.Fatalf("failed to create mocArticle: %v", err)
	}

	var mocCount int64
	if err := db.Model(&repository.GormArticle{}).Where(MocSQLCondition).Count(&mocCount).Error; err != nil {
		t.Fatalf("failed to query with MocSQLCondition: %v", err)
	}
	if mocCount != 1 {
		t.Errorf("expected 1 MOC article matched by MocSQLCondition, got %d", mocCount)
	}

	var matchedArticles []repository.GormArticle
	if err := db.Where(MocSQLCondition).Find(&matchedArticles).Error; err != nil {
		t.Fatalf("failed to query matched articles: %v", err)
	}
	if len(matchedArticles) != 1 || matchedArticles[0].ID != mocArticle.ID {
		t.Errorf("expected only mocArticle to match MocSQLCondition, matched: %+v", matchedArticles)
	}
}
