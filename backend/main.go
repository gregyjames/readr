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
	"strconv"
	"strings"
	"sync"
	"time"

	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"golang.org/x/net/html"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

type RequestBody struct {
	URL string `json:"url"`
	Tags []string `json:"tags"`
}

type Article struct {
	ID        int64              `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Title     string             `json:"title"`
	Content   string             `json:"article"` // Mapping to 'article' for frontend compatibility
	Tags      []Tag              `gorm:"many2many:article_tags;" json:"tags"`
	Attributes []ArticleAttribute `json:"attributes"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Tag struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"uniqueIndex" json:"name"`
}

type ArticleAttribute struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ArticleID int64  `gorm:"index:idx_article_attr,unique" json:"article_id"`
	Key       string `gorm:"index:idx_article_attr,unique" json:"key"`
	Value     string `json:"value"`
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

func main(){
	initLogger()
	defer logger.Sync()

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Static("/articles", "/app/data/articles")
	app.Static("/images", "/app/data/images")
	
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
	
	logger.Info("Available SQL drivers", zap.Strings("drivers", sql.Drivers()))
	
	sqlDB, err := sql.Open("sqlite", "/app/data/data.sqlite")
	if err != nil {
		logger.Fatal("sql.Open failed", zap.Error(err))
	}

	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		logger.Fatal("GORM failed", zap.Error(err))
	}

	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}
	
	db.AutoMigrate(&Article{}, &Tag{}, &ArticleAttribute{})

	api := app.Group("/api")

	api.Get("/", func(c *fiber.Ctx) error {
		//return c.SendString("Hello, World!")
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
	})

	api.Delete("/delete/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		logger.Info("Attempting to delete article", zap.String("id", id))

		if err := db.Select("Tags", "Attributes").Delete(&Article{ID: mustParseInt64(id)}).Error; err != nil {
			logger.Error("Failed to delete article from DB", zap.String("id", id), zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article",
			})
		}

		deleteFileError := os.Remove(fmt.Sprintf("/app/data/articles/%s.md", id)) 
		if deleteFileError != nil {
			logger.Error("Failed to delete article file", zap.String("id", id), zap.Error(deleteFileError))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to delete article file",
			})
		}

		deleteImagesError := os.RemoveAll(fmt.Sprintf("/app/data/images/%s/", id))
		if deleteImagesError != nil{
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
		if err := db.Preload("Tags").Preload("Attributes").Find(&articles).Error; err != nil {
			logger.Error("Failed to retrieve articles from DB", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to retrieve articles",
			})
		}
		return c.JSON(articles)
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
	os.MkdirAll(fmt.Sprintf("/app/data/images/%d", filenameID), os.ModePerm)
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
			savePath := fmt.Sprintf("/app/data/images/%d/", filenameID) + filename

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
	filename := fmt.Sprintf("/app/data/articles/%d.md", filenameID)
	os.MkdirAll("/app/data/articles", os.ModePerm)

	markdown := fmt.Sprintf(`
[Source](%s)

![Cover Image](%s)

%s
`, body.URL, imagePath, markdownContent)

	err = os.WriteFile(filename, []byte(markdown), 0644)
	if err != nil {
		logger.Error("Failed to save markdown file", zap.String("filename", filename), zap.Error(err))
		return c.Status(500).SendString("Failed to save markdown file")
	}

	var tags []Tag
	for _, tagName := range body.Tags {
		var tag Tag
		db.FirstOrCreate(&tag, Tag{Name: strings.TrimSpace(tagName)})
		tags = append(tags, tag)
	}

	attributes := []ArticleAttribute{
		{Key: "source_url", Value: body.URL},
		{Key: "image_path", Value: imagePath},
	}

	// Save article entry in DB
	newArticle := Article{
		ID:         filenameID,
		Title:      title,
		Content:    fmt.Sprintf("/articles/%d.md", filenameID),
		Tags:       tags,
		Attributes: attributes,
	}

	if err := db.Create(&newArticle).Error; err != nil {
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

	app.Listen(":3000")
}

func mustParseInt64(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		logger.Error("Failed to parse int64", zap.String("input", s), zap.Error(err))
		return 0
	}
	return i
}

func downloadImage(url string, dirname int64) string {
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Failed to download image", zap.String("url", url), zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	name := filepath.Base(strings.Split(url, "?")[0])
	directory := fmt.Sprintf("/app/data/images/%d/", dirname)
	os.MkdirAll(directory, os.ModePerm)

	out, err := os.Create(directory + name)
	if err != nil {
		logger.Error("Failed to create image file", zap.String("path", directory+name), zap.Error(err))
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