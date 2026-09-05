package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/backend/internal/agents"
	"example.com/backend/internal/chat"
	"example.com/backend/internal/graph"
	"example.com/backend/internal/handlers"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"example.com/backend/internal/vault"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

type Article = repository.GormArticle
type ArticleLink = repository.GormArticleLink
type ArticleStatusType = repository.GormArticleStatusType
type ArticleStatus = repository.GormArticleStatus
type LinkRequest = handlers.LinkRequest
type LinkError = handlers.LinkError
type GraphNode = graph.Node
type GraphEdge = graph.Edge
type SearchResult = handlers.SearchResult
type ServerSettings = handlers.ServerSettings

var logger = zap.NewNop()

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

	db.AutoMigrate(&Article{}, &ArticleLink{}, &repository.PipelineMetric{},
		&ArticleStatusType{}, &ArticleStatus{})
	handlers.EnsureFTS(db, logger)
	if err := repository.EnsureArticleStatusTypes(db); err != nil && logger != nil {
		logger.Error("Failed to seed reading status types", zap.Error(err))
	}
	handlers.MigrateLegacyArticleFilenames(db, getDataDir(), logger)
	handlers.MigrateLegacyArticleTags(db, getDataDir(), logger)

	return db
}

func LinkArticles(db *gorm.DB, req LinkRequest) (*ArticleLink, error) {
	return handlers.LinkArticles(db, getDataDir(), req)
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
		db.AutoMigrate(&Article{}, &ArticleLink{}, &repository.PipelineMetric{},
			&ArticleStatusType{}, &ArticleStatus{})

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

	handlers.EnsureFTS(db, logger)
	if err := repository.EnsureArticleStatusTypes(db); err != nil && logger != nil {
		logger.Error("Failed to seed reading status types", zap.Error(err))
	}

	dataDirectory := getDataDir()
	settingsStore := handlers.NewSettingsStore(dataDirectory, logger)
	eventHub := handlers.NewEventHub(logger)

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Cache-Control, Pragma, Authorization, X-Openrouter-Key, X-Openrouter-Model, X-OpenRouter-Key, X-OpenRouter-Model, X-Api-Key, X-Agent-Enricher, X-Agent-Linker, X-Agent-Summarizer",
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

	app.Static("/images", filepath.Join(dataDirectory, "images"))

	repo := repository.NewGormRepository(db)
	graphEngine := graph.NewEngine(repo)

	// Start 1 Background Agent for the Vault (sequential execution prevents OpenRouter in-flight credit exhaustion)
	agents.InitPool(logger, db, repo, dataDirectory, 1, func() {
		graphEngine.InvalidateCache()
		eventHub.Broadcast("graph-updated")
	})

	ingester := ingest.NewIngester(
		ingest.NewHTTPFetcher(30*time.Second),
		ingest.NewContentExtractor(),
		ingest.NewDiskStorage(dataDirectory),
		repo,
	)
	templatesDir := filepath.Join(dataDirectory, "templates")
	templateRenderer := ingest.NewGonjaTemplateRenderer(templatesDir)
	ingester.SetTemplateRenderer(templateRenderer)
	ingester.SetSummarizer(ingest.NewOpenRouterSummarizer())

	chatRepo := chat.NewFileRepository(filepath.Join(dataDirectory, "chats"))
	articleFetcher := &handlers.ArticleFileFetcher{DataDir: dataDirectory, DB: db}
	chatService := chat.NewService(chatRepo, articleFetcher)
	vaultInstance := vault.NewVault(dataDirectory, db, logger, graphEngine)

	hCtx := &handlers.HandlerContext{
		DB:               db,
		Logger:           logger,
		DataDir:          dataDirectory,
		SettingsStore:    settingsStore,
		Repo:             repo,
		Vault:            vaultInstance,
		Ingester:         ingester,
		GraphEngine:      graphEngine,
		ChatRepo:         chatRepo,
		ChatService:      chatService,
		TemplateRenderer: templateRenderer,
		EventHub:         eventHub,
		ArticleFetcher:   articleFetcher,
	}

	api := app.Group("/api")

	api.Use(handlers.AuthMiddleware(hCtx))

	api.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
	})

	librarianRunner := agents.NewLibrarianRunner(logger, db, repo, dataDirectory, func() {
		graphEngine.InvalidateCache()
		eventHub.Broadcast("graph-updated")
	})
	librarianCron := agents.NewLibrarianCronManager(librarianRunner, logger)
	hCtx.LibrarianCron = librarianCron
	initialSettings := settingsStore.Get()
	if err := librarianCron.Start(initialSettings.LibrarianCron, initialSettings.LibrarianEnabled); err != nil {
		logger.Error("Failed to start Librarian background cron scheduler",
			zap.String("cron", initialSettings.LibrarianCron),
			zap.Bool("enabled", initialSettings.LibrarianEnabled),
			zap.Error(err),
		)
	}

	handlers.RegisterAuth(api, hCtx)
	handlers.RegisterArticles(api, hCtx)
	handlers.RegisterArticleStatus(api, hCtx)
	handlers.RegisterGraph(api, hCtx)
	handlers.RegisterChat(api, hCtx)
	handlers.RegisterSettings(api, hCtx)
	handlers.RegisterTemplates(api, hCtx)
	handlers.RegisterSearch(api, hCtx)
	handlers.RegisterEvents(api, hCtx)
	handlers.RegisterDiagnostics(api, hCtx)
	handlers.RegisterLibrarian(api, hCtx, librarianRunner, librarianCron)

	distDir := os.Getenv("DIST_DIR")
	if distDir == "" {
		candidates := []string{"/app/dist", "../frontend/dist", "./dist"}
		for _, cand := range candidates {
			if info, err := os.Stat(cand); err == nil && info.IsDir() {
				distDir = cand
				break
			}
		}
	}

	if distDir != "" {
		if info, err := os.Stat(distDir); err == nil && info.IsDir() {
			logger.Info("Serving static frontend files from", zap.String("distDir", distDir))
			app.Static("/", distDir)

			// SPA Fallback for client-side routing
			app.Get("*", func(c *fiber.Ctx) error {
				if strings.HasPrefix(c.Path(), "/api") || strings.HasPrefix(c.Path(), "/images") {
					return c.Next()
				}
				return c.SendFile(filepath.Join(distDir, "index.html"))
			})
		}
	}

	return app
}

func main() {
	initLogger()
	defer logger.Sync()

	dataDirectory := getDataDir()
	os.MkdirAll(filepath.Join(dataDirectory, "articles"), os.ModePerm)
	os.MkdirAll(filepath.Join(dataDirectory, "images"), os.ModePerm)
	os.MkdirAll(filepath.Join(dataDirectory, "templates"), os.ModePerm)

	db := initDB()
	app := setupApp(db)

	port := os.Getenv("PORT")
	if port == "" {
		port = ":8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	logger.Info("Starting server on port", zap.String("port", port))
	if err := app.Listen(port); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
