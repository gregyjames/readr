package main

import (
"regexp"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/backend/internal/graph"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

type RequestBody struct {
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

type Article = repository.GormArticle
type ArticleLink = repository.GormArticleLink
type GraphNode = graph.Node
type GraphEdge = graph.Edge

type LinkRequest struct {
	SourceID     int64  `json:"sourceId"`
	TargetID     int64  `json:"targetId"`
	SelectedText string `json:"selectedText"`
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
	if req.SourceID == req.TargetID {
		return nil, &LinkError{StatusCode: fiber.StatusBadRequest, Message: "An article cannot link to itself"}
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

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Cache-Control, Pragma",
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

	dataDirectory := getDataDir()
	app.Get("/articles/:filename", func(c *fiber.Ctx) error {
		filename := c.Params("filename")
		clean := filepath.Clean(filename)
		filePath := filepath.Join(dataDirectory, "articles", clean)
		return c.SendFile(filePath)
	})
	app.Static("/images", filepath.Join(dataDirectory, "images"))

	repo := repository.NewGormRepository(db)
	graphEngine := graph.NewEngine(repo)
	ingester := ingest.NewIngester(
		ingest.NewHTTPFetcher(30*time.Second),
		ingest.NewContentExtractor(),
		ingest.NewDiskStorage(dataDirectory),
		repo,
	)

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

		graphEngine.InvalidateCache()
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

		graphEngine.InvalidateCache()
		return c.JSON(fiber.Map{"status": "success", "linkId": link.ID})
	})

	api.Get("/graph", func(c *fiber.Ctx) error {
		graphData, err := graphEngine.BuildGlobalGraph(c.Context())
		if err != nil {
			logger.Error("Failed to fetch graph", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch graph"})
		}
		return c.JSON(graphData)
	})

	api.Get("/graph/local/:id", func(c *fiber.Ctx) error {
		idParam := c.Params("id")
		var articleID int64
		if _, err := fmt.Sscanf(idParam, "%d", &articleID); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid article ID"})
		}

		graphData, err := graphEngine.BuildLocalGraph(c.Context(), articleID, 1)
		if err != nil {
			logger.Error("Failed to fetch local graph", zap.Int64("id", articleID), zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch local graph"})
		}
		return c.JSON(graphData)
	})

	api.Post("/edit/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		
		var req struct {
			Content string `json:"content"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
		}

		var article Article
		if err := db.First(&article, id).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Article not found"})
		}

		sourcePath := filepath.Join(dataDirectory, "articles", fmt.Sprintf("%s.md", id))
		if err := os.WriteFile(sourcePath, []byte(req.Content), 0644); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save article"})
		}
		
		// Sync links to database
		linkRegex := regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
		matches := linkRegex.FindAllStringSubmatch(req.Content, -1)
		
		// Delete existing outgoing links to prevent stale links
		db.Where("source_id = ?", article.ID).Delete(&ArticleLink{})
		
		for _, match := range matches {
			targetTitle := strings.TrimSpace(match[1])
			var target Article
			if err := db.Where("LOWER(title) = LOWER(?)", targetTitle).First(&target).Error; err == nil {
				// Prevent self-linking
				if article.ID != target.ID {
					link := ArticleLink{SourceID: article.ID, TargetID: target.ID}
					db.Create(&link)
				}
			}
		}

		graphEngine.InvalidateCache()
		return c.JSON(fiber.Map{"status": "success"})
	})

	api.Post("/add", func(c *fiber.Ctx) error {
		var body RequestBody

		if err := json.Unmarshal(c.Body(), &body); err != nil {
			logger.Error("Failed to unmarshal request body", zap.Error(err))
			return c.Status(400).SendString("Invalid JSON")
		}

		logger.Info("Adding new article", zap.String("url", body.URL))

		article, err := ingester.Ingest(c.Context(), ingest.IngestRequest{
			URL:  body.URL,
			Tags: body.Tags,
		})
		if err != nil {
			logger.Error("Failed to ingest article", zap.String("url", body.URL), zap.Error(err))
			if errors.Is(err, ingest.ErrDuplicateArticle) && article != nil {
				return c.JSON(fiber.Map{
					"status":  "exists",
					"message": "Article already exists",
					"id":      article.ID,
				})
			}
			if errors.Is(err, ingest.ErrEmptyURL) || errors.Is(err, ingest.ErrInvalidURL) {
				return c.Status(400).SendString("Invalid URL")
			}
			return c.Status(500).SendString("Failed to fetch the page")
		}

		graphEngine.InvalidateCache()
		logger.Info("Article added successfully", zap.Int64("id", article.ID), zap.String("url", body.URL))
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "Article saved",
			"id":      article.ID,
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