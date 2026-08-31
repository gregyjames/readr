package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var windowsReservedNames = map[string]bool{
	"CON":  true,
	"PRN":  true,
	"AUX":  true,
	"NUL":  true,
	"COM1": true,
	"COM2": true,
	"COM3": true,
	"COM4": true,
	"COM5": true,
	"COM6": true,
	"COM7": true,
	"COM8": true,
	"COM9": true,
	"LPT1": true,
	"LPT2": true,
	"LPT3": true,
	"LPT4": true,
	"LPT5": true,
	"LPT6": true,
	"LPT7": true,
	"LPT8": true,
	"LPT9": true,
}

func isWindowsReservedName(name string) bool {
	return windowsReservedNames[strings.ToUpper(strings.TrimSpace(name))]
}

// SanitizeTitleFilename produces a safe, cross-platform filename suitable for Obsidian vaults.
func SanitizeTitleFilename(title string, fallbackID int64) string {
	var b strings.Builder
	for _, r := range title {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r < 32 {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	s := b.String()

	fields := strings.Fields(s)
	cleaned := strings.Join(fields, " ")
	cleaned = strings.Trim(cleaned, ". -_")

	runes := []rune(cleaned)
	if len(runes) > 100 {
		runes = runes[:100]
		cleaned = strings.Trim(string(runes), ". -_")
	}

	if cleaned == "" || isWindowsReservedName(cleaned) {
		if fallbackID > 0 {
			return fmt.Sprintf("Article %d.md", fallbackID)
		}
		return "Article.md"
	}

	return cleaned + ".md"
}

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

func (s *DiskStorage) SaveMarkdownByTitle(title string, fallbackID int64, content []byte) (string, error) {
	articlesDir := filepath.Join(s.baseDir, "articles")
	baseFilename := SanitizeTitleFilename(title, fallbackID)
	nameWithoutExt := strings.TrimSuffix(baseFilename, ".md")

	filename := baseFilename
	counter := 1

	for {
		targetPath := filepath.Join(articlesDir, filename)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			break
		}
		filename = fmt.Sprintf("%s (%d).md", nameWithoutExt, counter)
		counter++
	}

	if err := s.saveAtomically(articlesDir, filename, ".tmp", content, "markdown"); err != nil {
		return "", err
	}

	return fmt.Sprintf("/articles/%s", filename), nil
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
