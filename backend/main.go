package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"codeberg.org/readeck/go-readability"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/net/html"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

type RequestBody struct {
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

type Article struct {
	gorm.Model
	ID      int64
	Article string `json:"article"`
	Image   string `json:"image"`
	Title   string `json:"title"`
	Tags    string `json:"tags"`
}

type ArticleLink struct {
	ID       int64 `gorm:"primaryKey" json:"id"`
	SourceID int64 `json:"sourceId"`
	TargetID int64 `json:"targetId"`
}

type LinkRequest struct {
	SourceID     int64  `json:"sourceId"`
	TargetID     int64  `json:"targetId"`
	SelectedText string `json:"selectedText"`
}

type GraphNode struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"` // "article" or "tag"
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}


var logger *zap.Logger

func initLogger() {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString("[" + t.Format("1/2/06") + "]")
	}
	config.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString("[" + strings.ToUpper(l.String()) + "]")
	}
	config.ConsoleSeparator = " "

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)
	logger = zap.New(core)
}

func getDataDir() string {
	if env := os.Getenv("DATA_DIR"); env != "" {
		return env
	}
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data"
	}
	if _, err := os.Stat("../data"); err == nil {
		return "../data"
	}
	return "./data"
}

func initDB() *gorm.DB {
	dataDirectory := getDataDir()
	dbPath := filepath.Join(dataDirectory, "data.sqlite")

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		if logger != nil {
			logger.Fatal("sql.Open failed", zap.Error(err))
		}
		panic(err)
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		if logger != nil {
			logger.Fatal("GORM failed", zap.Error(err))
		}
		panic(err)
	}

	db.AutoMigrate(&Article{}, &ArticleLink{})
	return db
}

type LinkError struct {
	StatusCode int
	Message    string
}

func (e *LinkError) Error() string {
	return e.Message
}

func LinkArticles(db *gorm.DB, req LinkRequest) (*ArticleLink, error) {
	if strings.TrimSpace(req.SelectedText) == "" {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "Selected text cannot be empty"}
	}
	if req.SourceID == 0 || req.TargetID == 0 {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "Source and target IDs are required"}
	}

	// 1. Validate target article exists
	var target Article
	if err := db.First(&target, req.TargetID).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusNotFound, Message: "Target article not found"}
	}

	// 2. Validate source article exists
	var source Article
	if err := db.First(&source, req.SourceID).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusNotFound, Message: "Source article not found"}
	}

	// 3. Read and update markdown file
	sourcePath := filepath.Join(getDataDir(), "articles", fmt.Sprintf("%d.md", req.SourceID))
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not read source article"}
	}

	wikilink := fmt.Sprintf("[[%s|%s]]", target.Title, req.SelectedText)
	newContent := strings.Replace(string(content), req.SelectedText, wikilink, 1)

	if err := os.WriteFile(sourcePath, []byte(newContent), 0644); err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not update markdown"}
	}

	// 4. Save DB link only after file update succeeds
	link := ArticleLink{SourceID: req.SourceID, TargetID: req.TargetID}
	if err := db.Create(&link).Error; err != nil {
		return nil, &LinkError{StatusCode: fiber.StatusInternalServerError, Message: "Could not create link"}
	}

	return &link, nil
}

func setupApp(customDB ...*gorm.DB) *fiber.App {
	if logger == nil {
		logger = zap.NewNop()
	}

	var db *gorm.DB
	if len(customDB) > 0 && customDB[0] != nil {
		db = customDB[0]
	} else {
		sqlDB, err := sql.Open("sqlite", "file::memory:?cache=shared")
		if err != nil {
			panic(err)
		}
		db, err = gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
		if err != nil {
			panic(err)
		}
		db.AutoMigrate(&Article{}, &ArticleLink{})

		dataDir := getDataDir()
		os.MkdirAll(filepath.Join(dataDir, "articles"), os.ModePerm)
		os.MkdirAll(filepath.Join(dataDir, "images"), os.ModePerm)

		var count int64
		db.Model(&Article{}).Count(&count)
		if count == 0 {
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
			os.WriteFile(filepath.Join(dataDir, "articles", "1.md"), []byte("This is an article about neural networks and AI."), 0644)
			os.WriteFile(filepath.Join(dataDir, "articles", "2.md"), []byte("Deep learning and neural networks."), 0644)
		}
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	dataDirectory := getDataDir()
	app.Static("/articles", filepath.Join(dataDirectory, "articles"))
	app.Static("/images", filepath.Join(dataDirectory, "images"))

	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:8080", // Vue dev server
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		logger.Info("Request handled",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", c.Response().StatusCode()),
			zap.Duration("latency", duration),
		)

		return err
	})

	api := app.Group("/api")

	api.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
	})

	api.Delete("/delete/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		logger.Info("Attempting to delete article", zap.String("id", id))

		if err := db.Delete(&Article{}, id).Error; err != nil {
			logger.Error("Failed to delete article from DB", zap.String("id", id), zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article",
			})
		}

		deleteFileError := os.Remove(filepath.Join(dataDirectory, "articles", fmt.Sprintf("%s.md", id)))
		if deleteFileError != nil {
			logger.Error("Failed to delete article file", zap.String("id", id), zap.Error(deleteFileError))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article file",
			})
		}

		deleteImagesError := os.RemoveAll(filepath.Join(dataDirectory, "images", id))
		if deleteImagesError != nil {
			logger.Error("Failed to delete article images", zap.String("id", id), zap.Error(deleteImagesError))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article images",
			})
		}

		logger.Info("Article deleted successfully", zap.String("id", id))
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": fmt.Sprintf("Article %s deleted", id),
		})
	})

	api.Get("/getarticles", func(c *fiber.Ctx) error {
		var articles []Article
		if err := db.Find(&articles).Error; err != nil {
			logger.Error("Failed to retrieve articles from DB", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
	})


	api.Post("/link", func(c *fiber.Ctx) error {
		var req LinkRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		link, err := LinkArticles(db, req)
		if err != nil {
			if linkErr, ok := err.(*LinkError); ok {
				return c.Status(linkErr.StatusCode).JSON(fiber.Map{"error": linkErr.Message})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"status": "success", "linkId": link.ID})
	})

	api.Get("/graph", func(c *fiber.Ctx) error {
		var articles []Article
		if err := db.Find(&articles).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch articles"})
		}

		var links []ArticleLink
		if err := db.Find(&links).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch links"})
		}

		nodes := []GraphNode{}
		edges := []GraphEdge{}
		tagSet := make(map[string]bool)

		for _, article := range articles {
			nodes = append(nodes, GraphNode{
				Id:    fmt.Sprintf("article-%d", article.ID),
				Label: article.Title,
				Group: "article",
			})

			// Process tags
			if article.Tags != "" {
				tags := strings.Split(article.Tags, ",")
				for _, tag := range tags {
					tag = strings.TrimSpace(tag)
					if tag == "" {
						continue
					}
					if !tagSet[tag] {
						nodes = append(nodes, GraphNode{
							Id:    fmt.Sprintf("tag-%s", tag),
							Label: tag,
							Group: "tag",
						})
						tagSet[tag] = true
					}
					edges = append(edges, GraphEdge{
						From: fmt.Sprintf("article-%d", article.ID),
						To:   fmt.Sprintf("tag-%s", tag),
					})
				}
			}
		}

		for _, link := range links {
			edges = append(edges, GraphEdge{
				From: fmt.Sprintf("article-%d", link.SourceID),
				To:   fmt.Sprintf("article-%d", link.TargetID),
			})
		}

		return c.JSON(fiber.Map{
			"nodes": nodes,
			"edges": edges,
		})
	})



	api.Post("/add", func(c *fiber.Ctx) error {
		var body RequestBody

		if err := json.Unmarshal(c.Body(), &body); err != nil {
			logger.Error("Failed to unmarshal request body", zap.Error(err))
			return c.Status(400).SendString("Invalid JSON")
		}

		logger.Info("Adding new article", zap.String("url", body.URL))

		resp, err := http.Get(body.URL)
		if err != nil || resp.StatusCode != 200 {
			logger.Error("Failed to fetch the page", zap.String("url", body.URL), zap.Error(err))
			return c.Status(500).SendString("Failed to fetch the page")
		}
		defer resp.Body.Close()

		// Read HTML body into bytes (for readability)
		htmlBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read HTML body", zap.String("url", body.URL), zap.Error(err))
			return c.Status(500).SendString("Failed to read HTML body")
		}

		parsedURL, err := url.Parse(body.URL)
		if err != nil {
			logger.Error("Invalid URL", zap.String("url", body.URL), zap.Error(err))
			return c.Status(400).SendString("Invalid URL")
		}

		// Parse with readability
		article, err := readability.FromReader(bytes.NewReader(htmlBytes), parsedURL)
		if err != nil {
			logger.Error("Failed to parse readable content", zap.String("url", body.URL), zap.Error(err))
			return c.Status(500).SendString("Failed to parse readable content")
		}

		filenameID := time.Now().Unix()
		imagesDir := filepath.Join(dataDirectory, "images", fmt.Sprintf("%d", filenameID))
		os.MkdirAll(imagesDir, os.ModePerm)
		doc, _ := html.Parse(bytes.NewReader(htmlBytes))
		images := extractImageSources(doc)

		converter := markdown.NewConverter("", true, &markdown.Options{})

		markdownContent, err := converter.ConvertString(article.Content)

		var wg sync.WaitGroup
		var mu sync.Mutex
		replacements := make([]string, 0, len(images)*2)

		for _, imgURL := range images {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				imgResp, err := http.Get(url)
				if err != nil || imgResp.StatusCode != 200 {
					logger.Warn("Failed to download image", zap.String("imgURL", url))
					return
				}
				defer imgResp.Body.Close()

				// Extract filename
				parts := strings.Split(url, "/")
				filename := parts[len(parts)-1]
				savePath := filepath.Join(imagesDir, filename)

				// Save file
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

		if err != nil {
			logger.Error("Failed to convert HTML to markdown", zap.Error(err))
			return c.Status(500).SendString("Failed to convert HTML to markdown")
		}

		// Extract title & image
		title := article.Title
		imageURL := article.Image
		imagePath := ""

		if imageURL != "" {
			imagePath = downloadImage(imageURL, filenameID)
		}

		// Generate markdown with clean content
		articlesDir := filepath.Join(dataDirectory, "articles")
		filename := filepath.Join(articlesDir, fmt.Sprintf("%d.md", filenameID))
		os.MkdirAll(articlesDir, os.ModePerm)

		markdownDoc := fmt.Sprintf(`
[Source](%s)

![Cover Image](%s)

%s
`, body.URL, imagePath, markdownContent)

		err = os.WriteFile(filename, []byte(markdownDoc), 0644)
		if err != nil {
			logger.Error("Failed to save markdown file", zap.String("filename", filename), zap.Error(err))
			return c.Status(500).SendString("Failed to save markdown file")
		}

		tagsString := strings.Join(body.Tags, ",")

		// Save article entry in DB
		if err := db.Create(&Article{
			Title:   title,
			Image:   imagePath,
			Article: fmt.Sprintf("/articles/%d.md", filenameID),
			Tags:    tagsString,
			ID:      filenameID,
		}).Error; err != nil {
			logger.Error("Failed to save article in DB", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save article in DB"})
		}

		logger.Info("Article added successfully", zap.Int64("id", filenameID), zap.String("url", body.URL))
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "Article saved",
			"id":      filenameID,
		})
	})

	return app
}

func main() {
	initLogger()
	defer logger.Sync()

	logger.Info("Available SQL drivers", zap.Strings("drivers", sql.Drivers()))

	db := initDB()
	app := setupApp(db)

	app.Listen(":3000")
}

func downloadImage(url string, dirname int64) string {
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to download image", zap.String("url", url), zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	name := filepath.Base(strings.Split(url, "?")[0])
	directory := filepath.Join(getDataDir(), "images", fmt.Sprintf("%d", dirname))
	os.MkdirAll(directory, os.ModePerm)

	out, err := os.Create(filepath.Join(directory, name))
	if err != nil {
		logger.Error("Failed to create image file", zap.String("path", filepath.Join(directory, name)), zap.Error(err))
		return ""
	}
	defer out.Close()

	io.Copy(out, resp.Body)
	return fmt.Sprintf("/images/%d/", dirname) + name
}

func extractImageSources(n *html.Node) []string {
	var srcs []string
	var crawler func(*html.Node)
	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					srcs = append(srcs, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}
	crawler(n)
	return srcs
}