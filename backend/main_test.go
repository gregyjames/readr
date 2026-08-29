package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func initTestDB() *gorm.DB {
	sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		panic(err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&Article{}, &ArticleLink{})
	db.Exec("DELETE FROM article_links")
	db.Exec("DELETE FROM articles")

	db.Create(&Article{
		ID:      1,
		Title:   "Source Article",
		Article: "/articles/1.md",
	})
	db.Create(&Article{
		ID:      2,
		Title:   "Target Article",
		Article: "/articles/2.md",
	})

	return db
}

func BenchmarkDownloadImagesSequential(b *testing.B) {
	// Setup a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake image data"))
	}))
	defer ts.Close()

	images := make([]string, 10)
	for i := 0; i < 10; i++ {
		images[i] = fmt.Sprintf("%s/image%d.png", ts.URL, i)
	}

	tempDir, _ := os.MkdirTemp("", "bench")
	defer os.RemoveAll(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		markdownContent := "test content"
		filenameID := int64(12345)

		for _, imgURL := range images {
			imgResp, err := http.Get(imgURL)
			if err != nil || imgResp.StatusCode != 200 {
				continue
			}
			// Simulate the current bug of defer in loop for baseline
			// Actually, to make it fair and not leak, I'll close it inside the loop in the baseline if I were to fix it,
			// but I should replicate the current code as closely as possible.
			// However, b.N can be large, so defer in loop will definitely cause issues.

			func() {
				defer imgResp.Body.Close()
				parts := strings.Split(imgURL, "/")
				filename := parts[len(parts)-1]
				savePath := tempDir + "/" + filename

				out, _ := os.Create(savePath)
				io.Copy(out, imgResp.Body)
				out.Close()

				markdownContent = strings.ReplaceAll(markdownContent, imgURL, fmt.Sprintf("/images/%d/", filenameID)+filename)
			}()
		}
		_ = markdownContent
	}
}

func TestDownloadImagesParallel(t *testing.T) {
	// Setup a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake image data"))
	}))
	defer ts.Close()

	images := []string{
		ts.URL + "/img1.png",
		ts.URL + "/img2.png",
	}

	tempDir, err := os.MkdirTemp("", "test_parallel")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	markdownContent := "Here is img1: " + images[0] + " and img2: " + images[1]
	filenameID := int64(999)

	var wg sync.WaitGroup
	var mu sync.Mutex
	replacements := make([]string, 0, len(images)*2)

	for _, imgURL := range images {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			imgResp, err := http.Get(url)
			if err != nil || imgResp.StatusCode != 200 {
				return
			}
			defer imgResp.Body.Close()

			parts := strings.Split(url, "/")
			filename := parts[len(parts)-1]
			savePath := tempDir + "/" + filename

			out, err := os.Create(savePath)
			if err != nil {
				return
			}
			io.Copy(out, imgResp.Body)
			out.Close()

			mu.Lock()
			replacements = append(replacements, url, fmt.Sprintf("/images/%d/", filenameID)+filename)
			mu.Unlock()
		}(imgURL)
	}
	wg.Wait()

	replacer := strings.NewReplacer(replacements...)
	updatedContent := replacer.Replace(markdownContent)

	// Verify replacements
	if !strings.Contains(updatedContent, "/images/999/img1.png") {
		t.Errorf("Expected content to contain /images/999/img1.png, got: %s", updatedContent)
	}
	if !strings.Contains(updatedContent, "/images/999/img2.png") {
		t.Errorf("Expected content to contain /images/999/img2.png, got: %s", updatedContent)
	}

	// Verify files exist
	if _, err := os.Stat(tempDir + "/img1.png"); os.IsNotExist(err) {
		t.Error("img1.png was not saved")
	}
	if _, err := os.Stat(tempDir + "/img2.png"); os.IsNotExist(err) {
		t.Error("img2.png was not saved")
	}
}

func BenchmarkDownloadImagesParallel(b *testing.B) {
	// Setup a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake image data"))
	}))
	defer ts.Close()

	images := make([]string, 10)
	for i := 0; i < 10; i++ {
		images[i] = fmt.Sprintf("%s/image%d.png", ts.URL, i)
	}

	tempDir, _ := os.MkdirTemp("", "bench_parallel")
	defer os.RemoveAll(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		markdownContent := "test content"
		filenameID := int64(12345)

		var wg sync.WaitGroup
		var mu sync.Mutex
		replacements := make([]string, 0, len(images)*2)

		for _, imgURL := range images {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				imgResp, err := http.Get(url)
				if err != nil || imgResp.StatusCode != 200 {
					return
				}
				defer imgResp.Body.Close()

				parts := strings.Split(url, "/")
				filename := parts[len(parts)-1]
				savePath := tempDir + "/" + filename

				out, err := os.Create(savePath)
				if err != nil {
					return
				}
				io.Copy(out, imgResp.Body)
				out.Close()

				mu.Lock()
				replacements = append(replacements, url, fmt.Sprintf("/images/%d/", filenameID)+filename)
				mu.Unlock()
			}(imgURL)
		}
		wg.Wait()

		replacer := strings.NewReplacer(replacements...)
		markdownContent = replacer.Replace(markdownContent)
		_ = markdownContent
	}
}

func TestCreateLink(t *testing.T) {
	app := setupApp()
	reqBody := `{"sourceId": 1, "targetId": 2, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestCreateLink_MarkdownUpdated(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	app := setupApp()

	// Initial source article markdown
	articlesDir := filepath.Join(tempDir, "articles")
	sourceFile := filepath.Join(articlesDir, "1.md")
	initialContent := "Deep learning and neural networks are transforming AI."
	if err := os.WriteFile(sourceFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial article file: %v", err)
	}

	reqBody := `{"sourceId": 1, "targetId": 2, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["status"] != "success" {
		t.Errorf("Expected status 'success', got %v", result["status"])
	}
	if result["linkId"] == nil {
		t.Errorf("Expected linkId in response, got nil")
	}

	// Verify markdown file content was updated with Wikilink format
	updatedContent, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read updated article file: %v", err)
	}

	expectedWikilink := "[[Target Article|neural networks]]"
	if !strings.Contains(string(updatedContent), expectedWikilink) {
		t.Errorf("Expected updated content to contain %q, got %q", expectedWikilink, string(updatedContent))
	}
}

func TestCreateLink_EmptySelectedText(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	db := initTestDB()
	app := setupApp(db)

	articlesDir := filepath.Join(tempDir, "articles")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		t.Fatalf("Failed to create articles dir: %v", err)
	}
	sourceFile := filepath.Join(articlesDir, "1.md")
	initialContent := "Deep learning and neural networks are transforming AI."
	if err := os.WriteFile(sourceFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial source file: %v", err)
	}

	testCases := []string{
		`{"sourceId": 1, "targetId": 2, "selectedText": ""}`,
		`{"sourceId": 1, "targetId": 2, "selectedText": "   "}`,
	}

	for _, reqBody := range testCases {
		req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Failed to execute request: %v", err)
		}

		if resp.StatusCode != 400 {
			t.Errorf("Expected 400 for empty selected text, got %d", resp.StatusCode)
		}

		// Verify file was NOT modified
		content, _ := os.ReadFile(sourceFile)
		if string(content) != initialContent {
			t.Errorf("Source file should not have been modified")
		}

		// Verify NO link was created in DB
		var linkCount int64
		db.Model(&ArticleLink{}).Count(&linkCount)
		if linkCount != 0 {
			t.Errorf("Expected 0 links in DB, got %d", linkCount)
		}
	}
}

func TestCreateLink_InvalidRequest(t *testing.T) {
	app := setupApp()
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader("invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestCreateLink_TargetNotFound(t *testing.T) {
	db := initTestDB()
	app := setupApp(db)

	reqBody := `{"sourceId": 1, "targetId": 9999, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 for missing target article, got %d", resp.StatusCode)
	}

	// Verify no dangling link record in DB
	var linkCount int64
	db.Model(&ArticleLink{}).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("Expected 0 links in DB when target not found, got %d", linkCount)
	}
}

func TestCreateLink_SourceNotFound(t *testing.T) {
	db := initTestDB()
	app := setupApp(db)

	reqBody := `{"sourceId": 9999, "targetId": 2, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 for missing source article, got %d", resp.StatusCode)
	}

	// Verify no dangling link record in DB
	var linkCount int64
	db.Model(&ArticleLink{}).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("Expected 0 links in DB when source not found, got %d", linkCount)
	}
}

func TestCreateLink_FileReadFailure_NoDanglingLink(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	db := initTestDB()
	app := setupApp(db)

	// Note: 1.md does not exist in tempDir/articles
	reqBody := `{"sourceId": 1, "targetId": 2, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 500 {
		t.Errorf("Expected 500 when file cannot be read, got %d", resp.StatusCode)
	}

	// Verify no dangling link record in DB
	var linkCount int64
	db.Model(&ArticleLink{}).Count(&linkCount)
	if linkCount != 0 {
		t.Errorf("Expected 0 links in DB after file error, got %d", linkCount)
	}
}

func TestLinkArticles_Service(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	db := initTestDB()
	articlesDir := filepath.Join(tempDir, "articles")
	os.MkdirAll(articlesDir, 0755)
	sourceFile := filepath.Join(articlesDir, "1.md")
	os.WriteFile(sourceFile, []byte("Topic: neural networks and graphs."), 0644)

	link, err := LinkArticles(db, LinkRequest{
		SourceID:     1,
		TargetID:     2,
		SelectedText: "neural networks",
	})
	if err != nil {
		t.Fatalf("Unexpected error from LinkArticles: %v", err)
	}
	if link.ID == 0 {
		t.Errorf("Expected non-zero link ID")
	}

	content, _ := os.ReadFile(sourceFile)
	if !strings.Contains(string(content), "[[Target Article|neural networks]]") {
		t.Errorf("Expected wikilink in file, got %s", string(content))
	}
}



