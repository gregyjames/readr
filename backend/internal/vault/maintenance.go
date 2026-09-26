package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"example.com/backend/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MissingFileRecord represents a database article whose underlying markdown file is missing from disk.
type MissingFileRecord struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Article string `json:"article"`
}

// IntegrityReport provides the results of comparing database article records against vault disk files.
type IntegrityReport struct {
	OrphanFiles   []string            `json:"orphan_files"`
	MissingFiles  []MissingFileRecord `json:"missing_files"`
	TotalArticles int                 `json:"total_articles"`
	TotalFiles    int                 `json:"total_files"`
}

// MaintenanceService handles database backups and vault integrity auditing.
type MaintenanceService struct {
	dataDir string
	db      *gorm.DB
	logger  *zap.Logger
}

// NewMaintenanceService creates a new MaintenanceService instance.
func NewMaintenanceService(dataDir string, db *gorm.DB, logger *zap.Logger) *MaintenanceService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MaintenanceService{
		dataDir: dataDir,
		db:      db,
		logger:  logger,
	}
}

// CreateBackup initiates a consistent hot SQLite backup using VACUUM INTO.
func (m *MaintenanceService) CreateBackup(ctx context.Context) (string, error) {
	backupsDir := filepath.Join(m.dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backups directory: %w", err)
	}

	timestamp := time.Now().UTC().Format("20060102-150405")
	backupFilename := fmt.Sprintf("backup-%s.sqlite", timestamp)
	backupPath := filepath.Join(backupsDir, backupFilename)

	if _, err := os.Stat(backupPath); err == nil {
		backupFilename = fmt.Sprintf("backup-%s-%d.sqlite", timestamp, time.Now().UnixNano())
		backupPath = filepath.Join(backupsDir, backupFilename)
	}

	escapedPath := strings.ReplaceAll(backupPath, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s'", escapedPath)

	if err := m.db.WithContext(ctx).Exec(query).Error; err != nil {
		m.logger.Error("SQLite VACUUM INTO failed", zap.String("path", backupPath), zap.Error(err))
		return "", fmt.Errorf("backup failed: %w", err)
	}

	m.logger.Info("Database backup created successfully", zap.String("path", backupPath))
	return backupPath, nil
}

// AuditIntegrity walks the articles directory and compares against database records.
func (m *MaintenanceService) AuditIntegrity(ctx context.Context) (*IntegrityReport, error) {
	articlesDir := filepath.Join(m.dataDir, "articles")

	diskFiles := make(map[string]string)
	if _, err := os.Stat(articlesDir); err == nil {
		err := filepath.Walk(articlesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
				return nil
			}
			rel, err := filepath.Rel(articlesDir, path)
			if err != nil {
				return err
			}
			normalized := filepath.ToSlash(rel)
			diskFiles[normalized] = path
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk articles directory: %w", err)
		}
	}

	var articles []repository.GormArticle
	if err := m.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&articles).Error; err != nil {
		m.logger.Error("failed to query articles for integrity audit", zap.Error(err))
		return nil, fmt.Errorf("failed to query articles: %w", err)
	}

	report := &IntegrityReport{
		OrphanFiles:   make([]string, 0),
		MissingFiles:  make([]MissingFileRecord, 0),
		TotalArticles: len(articles),
		TotalFiles:    len(diskFiles),
	}

	dbFiles := make(map[string]bool)

	for _, a := range articles {
		clean := strings.TrimSpace(a.Article)
		clean = strings.TrimPrefix(clean, "/")
		clean = strings.TrimPrefix(clean, "articles/")
		clean = filepath.Clean(clean)
		normalized := filepath.ToSlash(clean)

		if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "..") {
			report.MissingFiles = append(report.MissingFiles, MissingFileRecord{
				ID:      a.ID,
				Title:   a.Title,
				Article: a.Article,
			})
			continue
		}

		if _, exists := diskFiles[normalized]; !exists {
			report.MissingFiles = append(report.MissingFiles, MissingFileRecord{
				ID:      a.ID,
				Title:   a.Title,
				Article: a.Article,
			})
		} else {
			dbFiles[normalized] = true
		}
	}

	for relPath := range diskFiles {
		if !dbFiles[relPath] {
			report.OrphanFiles = append(report.OrphanFiles, relPath)
		}
	}

	sort.Strings(report.OrphanFiles)
	sort.Slice(report.MissingFiles, func(i, j int) bool {
		return report.MissingFiles[i].ID < report.MissingFiles[j].ID
	})

	return report, nil
}
