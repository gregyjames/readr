package ingest

import (
	"fmt"
	"os"
	"path/filepath"
)

type DiskStorage struct {
	baseDir string
}

func NewDiskStorage(baseDir string) *DiskStorage {
	if baseDir == "" {
		baseDir = "data"
	}
	return &DiskStorage{
		baseDir: baseDir,
	}
}

func (s *DiskStorage) SaveMarkdown(filenameID int64, content []byte) (string, error) {
	articlesDir := filepath.Join(s.baseDir, "articles")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		return "", fmt.Errorf("create articles dir failed: %w", err)
	}

	finalPath := filepath.Join(articlesDir, fmt.Sprintf("%d.md", filenameID))
	tmpPath := filepath.Join(articlesDir, fmt.Sprintf("%d.md.tmp", filenameID))

	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return "", fmt.Errorf("write temporary markdown file failed: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("atomic rename markdown file failed: %w", err)
	}

	relPath := fmt.Sprintf("/articles/%d.md", filenameID)
	return relPath, nil
}

func (s *DiskStorage) SaveImage(filenameID int64, filename string, data []byte) (string, error) {
	imagesDir := s.GetImagesDir(filenameID)
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return "", fmt.Errorf("create images dir failed: %w", err)
	}

	finalPath := filepath.Join(imagesDir, filename)
	tmpPath := filepath.Join(imagesDir, fmt.Sprintf("%s.tmp", filename))

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return "", fmt.Errorf("write temporary image file failed: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("atomic rename image file failed: %w", err)
	}

	relPath := fmt.Sprintf("/images/%d/%s", filenameID, filename)
	return relPath, nil
}

func (s *DiskStorage) GetImagesDir(filenameID int64) string {
	return filepath.Join(s.baseDir, "images", fmt.Sprintf("%d", filenameID))
}
