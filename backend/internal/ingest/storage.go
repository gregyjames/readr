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

func (s *DiskStorage) saveAtomically(dir, filename, tmpSuffix string, data []byte, errMsg string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s dir failed: %w", errMsg, err)
	}

	finalPath := filepath.Join(dir, filename)
	tmpPath := filepath.Join(dir, fmt.Sprintf("%s%s", filename, tmpSuffix))

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temporary %s file failed: %w", errMsg, err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename %s file failed: %w", errMsg, err)
	}
	return nil
}

func (s *DiskStorage) SaveMarkdown(filenameID int64, content []byte) (string, error) {
	articlesDir := filepath.Join(s.baseDir, "articles")
	filename := fmt.Sprintf("%d.md", filenameID)
	
	if err := s.saveAtomically(articlesDir, filename, ".tmp", content, "markdown"); err != nil {
		return "", err
	}

	return fmt.Sprintf("/articles/%d.md", filenameID), nil
}

func (s *DiskStorage) SaveImage(filenameID int64, filename string, data []byte) (string, error) {
	imagesDir := s.GetImagesDir(filenameID)
	
	if err := s.saveAtomically(imagesDir, filename, ".tmp", data, "image"); err != nil {
		return "", err
	}

	return fmt.Sprintf("/images/%d/%s", filenameID, filename), nil
}

func (s *DiskStorage) GetImagesDir(filenameID int64) string {
	return filepath.Join(s.baseDir, "images", fmt.Sprintf("%d", filenameID))
}
