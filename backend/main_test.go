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
			imgResp, err := ts.Client().Get(url)
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

func TestGetGraph(t *testing.T) {
	db := initTestDB()
	app := setupApp(db)
	req := httptest.NewRequest("GET", "/api/graph", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestGetGraph_ContentAndRelations(t *testing.T) {
	db := initTestDB()
	// Clear and set custom data
	db.Exec("DELETE FROM article_links")
	db.Exec("DELETE FROM articles")

	db.Create(&Article{
		ID:    1,
		Title: "Article One",
		Tags:  "ai, tech, machine-learning",
	})
	db.Create(&Article{
		ID:    2,
		Title: "Article Two",
		Tags:  "tech, robotics",
	})
	db.Create(&Article{
		ID:    3,
		Title: "Article Three",
		Tags:  "",
	})

	db.Create(&ArticleLink{
		ID:       1,
		SourceID: 1,
		TargetID: 2,
	})

	app := setupApp(db)
	req := httptest.NewRequest("GET", "/api/graph", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify nodes
	// 3 articles + 4 distinct tags ("ai", "tech", "machine-learning", "robotics") = 7 nodes
	if len(result.Nodes) != 7 {
		t.Errorf("Expected 7 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}

	nodeMap := make(map[string]GraphNode)
	for _, n := range result.Nodes {
		nodeMap[n.Id] = n
	}

	expectedNodes := []GraphNode{
		{Id: "article-1", Label: "Article One", Group: "article"},
		{Id: "article-2", Label: "Article Two", Group: "article"},
		{Id: "article-3", Label: "Article Three", Group: "article"},
		{Id: "tag-ai", Label: "ai", Group: "tag"},
		{Id: "tag-tech", Label: "tech", Group: "tag"},
		{Id: "tag-machine-learning", Label: "machine-learning", Group: "tag"},
		{Id: "tag-robotics", Label: "robotics", Group: "tag"},
	}

	for _, expected := range expectedNodes {
		node, exists := nodeMap[expected.Id]
		if !exists {
			t.Errorf("Expected node %s not found", expected.Id)
			continue
		}
		if node.Label != expected.Label || node.Group != expected.Group {
			t.Errorf("Node %s mismatch: expected %+v, got %+v", expected.Id, expected, node)
		}
	}

	// Verify edges
	// Article 1 has 3 tag edges ("ai", "tech", "machine-learning")
	// Article 2 has 2 tag edges ("tech", "robotics")
	// Article 3 has 0 tag edges
	// 1 link edge (1 -> 2)
	// Total edges = 3 + 2 + 1 = 6
	if len(result.Edges) != 6 {
		t.Errorf("Expected 6 edges, got %d: %+v", len(result.Edges), result.Edges)
	}

	edgeSet := make(map[string]bool)
	for _, e := range result.Edges {
		edgeSet[fmt.Sprintf("%s->%s", e.From, e.To)] = true
	}

	expectedEdges := []string{
		"article-1->tag-ai",
		"article-1->tag-tech",
		"article-1->tag-machine-learning",
		"article-2->tag-tech",
		"article-2->tag-robotics",
		"article-1->article-2",
	}

	for _, expected := range expectedEdges {
		if !edgeSet[expected] {
			t.Errorf("Expected edge %s not found", expected)
		}
	}
}

func TestGetGraph_Empty(t *testing.T) {
	db := initTestDB()
	db.Exec("DELETE FROM article_links")
	db.Exec("DELETE FROM articles")

	app := setupApp(db)
	req := httptest.NewRequest("GET", "/api/graph", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(result.Edges))
	}
}

func TestGetGraph_DuplicateTagsAndCaseSensitivity(t *testing.T) {
	db := initTestDB()
	db.Exec("DELETE FROM article_links")
	db.Exec("DELETE FROM articles")

	db.Create(&Article{
		ID:    1,
		Title: "Article 1",
		Tags:  "AI, ai, Ai, Tech, tech ",
	})
	db.Create(&Article{
		ID:    2,
		Title: "Article 2",
		Tags:  "TECH, Robotics",
	})

	app := setupApp(db)
	req := httptest.NewRequest("GET", "/api/graph", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 2 articles + 3 unique lowercase tags ("ai", "tech", "robotics") = 5 nodes
	if len(result.Nodes) != 5 {
		t.Errorf("Expected 5 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}

	// 2 tag edges from article 1 ("ai", "tech") + 2 tag edges from article 2 ("tech", "robotics") = 4 edges
	if len(result.Edges) != 4 {
		t.Errorf("Expected 4 edges, got %d: %+v", len(result.Edges), result.Edges)
	}

	edgeSet := make(map[string]int)
	for _, e := range result.Edges {
		edgeSet[fmt.Sprintf("%s->%s", e.From, e.To)]++
	}

	expectedEdges := []string{
		"article-1->tag-ai",
		"article-1->tag-tech",
		"article-2->tag-tech",
		"article-2->tag-robotics",
	}

	for _, expected := range expectedEdges {
		if count := edgeSet[expected]; count != 1 {
			t.Errorf("Expected edge %s to appear exactly once, got %d", expected, count)
		}
	}
}

func TestGetGraph_LocalSubgraph(t *testing.T) {
	db := initTestDB()
	db.Exec("DELETE FROM articles")
	db.Exec("DELETE FROM article_links")

	db.Create(&Article{ID: 1, Title: "Article 1", Tags: "ai"})
	db.Create(&Article{ID: 2, Title: "Article 2", Tags: "ml"})
	db.Create(&Article{ID: 3, Title: "Article 3", Tags: "robotics"})
	db.Create(&ArticleLink{ID: 1, SourceID: 1, TargetID: 2})
	db.Create(&ArticleLink{ID: 2, SourceID: 2, TargetID: 3})

	app := setupApp(db)

	req := httptest.NewRequest("GET", "/api/graph/local/1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 1-hop from Article 1: Article 1, Article 2, Tag "ai", Tag "ml"
	nodeMap := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeMap[n.Id] = true
	}

	if !nodeMap["article-1"] || !nodeMap["article-2"] || !nodeMap["tag-ai"] {
		t.Errorf("Expected local subgraph to contain article-1, article-2, tag-ai; got %+v", nodeMap)
	}

	if nodeMap["article-3"] {
		t.Errorf("article-3 is 2 hops away and should not be in 1-hop subgraph")
	}
}

func TestChatEndpoints_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	db := initTestDB()
	app := setupApp(db)

	// 1. List chats initially (should be empty)
	req := httptest.NewRequest("GET", "/api/chats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to list chats: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	var chats []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&chats)
	if len(chats) != 0 {
		t.Errorf("Expected empty chat list, got %d", len(chats))
	}

	// 2. Create a new chat
	createReq := httptest.NewRequest("POST", "/api/chats", strings.NewReader(`{"title": "Test Chat"}`))
	createReq.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(createReq)
	if err != nil {
		t.Fatalf("Failed to create chat: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var createdChat map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdChat)
	chatID, ok := createdChat["id"].(string)
	if !ok || chatID == "" {
		t.Fatalf("Expected non-empty chat ID, got %+v", createdChat)
	}
	if createdChat["title"] != "Test Chat" {
		t.Errorf("Expected title 'Test Chat', got %v", createdChat["title"])
	}

	// 3. Get the created chat
	getReq := httptest.NewRequest("GET", "/api/chats/"+chatID, nil)
	resp, err = app.Test(getReq)
	if err != nil {
		t.Fatalf("Failed to get chat: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var fetchedChat map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fetchedChat)
	if fetchedChat["id"] != chatID {
		t.Errorf("Expected chat ID %s, got %v", chatID, fetchedChat["id"])
	}

	// 4. Delete the chat
	delReq := httptest.NewRequest("DELETE", "/api/chats/"+chatID, nil)
	resp, err = app.Test(delReq)
	if err != nil {
		t.Fatalf("Failed to delete chat: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// 5. Verify it's gone
	getReq = httptest.NewRequest("GET", "/api/chats/"+chatID, nil)
	resp, _ = app.Test(getReq)
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 after deletion, got %d", resp.StatusCode)
	}
}

func TestChatEndpoints_StreamMessage_Unauthorized(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DATA_DIR", tempDir)

	db := initTestDB()
	app := setupApp(db)

	req := httptest.NewRequest("POST", "/api/chats/session-1/message", strings.NewReader(`{"content": "Hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401 Unauthorized, got %d", resp.StatusCode)
	}
}





