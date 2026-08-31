package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"example.com/backend/internal/agents"
	"example.com/backend/internal/auth"
	"example.com/backend/internal/chat"
	"example.com/backend/internal/graph"
	"example.com/backend/internal/ingest"
	"example.com/backend/internal/repository"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// SSE Event Hub for Agent Broadcasts
var (
	eventClients sync.Map
)

func broadcastEvent(event string) {
	clientCount := 0
	eventClients.Range(func(key, value interface{}) bool {
		if ch, ok := key.(chan string); ok {
			clientCount++
			select {
			case ch <- event:
			default:
			}
		}
		return true
	})
	if logger != nil {
		logger.Info("SSE Broadcast sent", zap.String("event", event), zap.Int("connected_clients", clientCount))
	}
}

type ServerSettings struct {
	APIKey                string `json:"api_key"`
	Model                 string `json:"model"`
	AgentEnricher         bool   `json:"agent_enricher"`
	AgentLinker           bool   `json:"agent_linker"`
	AgentSummarizer       bool   `json:"agent_summarizer"`
	Theme                 string `json:"theme"`
	ViewMode              string `json:"view_mode"`
	GraphContextExpansion bool   `json:"graph_context_expansion"`
	PasswordHash          string `json:"password_hash,omitempty"`
	SessionSecret         string `json:"session_secret,omitempty"`
}

func loadServerSettings(dataDir string) ServerSettings {
	settingsPath := filepath.Join(dataDir, "settings.json")
	defaults := ServerSettings{
		Model:                 "openai/gpt-4o-mini",
		AgentEnricher:         true,
		AgentLinker:           true,
		AgentSummarizer:       true,
		Theme:                 "light",
		ViewMode:              "card",
		GraphContextExpansion: true,
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			defaults.SessionSecret, _ = auth.GenerateRandomSecret()
			_ = saveServerSettings(dataDir, defaults)
			return defaults
		}
		logger.Fatal("Failed to read settings file", zap.String("path", settingsPath), zap.Error(err))
	}
	s := defaults
	if err := json.Unmarshal(data, &s); err != nil {
		logger.Fatal("Failed to parse settings file", zap.String("path", settingsPath), zap.Error(err))
	}
	if s.Model == "" {
		s.Model = "openai/gpt-4o-mini"
	}
	if s.SessionSecret == "" {
		s.SessionSecret, _ = auth.GenerateRandomSecret()
		_ = saveServerSettings(dataDir, s)
	}
	return s
}

func saveServerSettings(dataDir string, s ServerSettings) error {
	settingsPath := filepath.Join(dataDir, "settings.json")
	bytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, bytes, 0600)
}

type articleFileFetcher struct {
	dataDir string
	db      *gorm.DB
}

func (f *articleFileFetcher) GetMarkdownContent(ctx context.Context, id int64) (string, error) {
	path := filepath.Join(f.dataDir, "articles", fmt.Sprintf("%d.md", id))
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (f *articleFileFetcher) GetLinkedArticles(ctx context.Context, id int64) ([]chat.Attachment, error) {
	if f.db == nil {
		return nil, nil
	}
	var links []ArticleLink
	if err := f.db.WithContext(ctx).Where("source_id = ? OR target_id = ?", id, id).Find(&links).Error; err != nil {
		return nil, err
	}

	linkedIDMap := make(map[int64]bool)
	for _, l := range links {
		if l.SourceID == id && l.TargetID != id {
			linkedIDMap[l.TargetID] = true
		} else if l.TargetID == id && l.SourceID != id {
			linkedIDMap[l.SourceID] = true
		}
	}

	var results []chat.Attachment
	for linkedID := range linkedIDMap {
		var art Article
		if err := f.db.WithContext(ctx).Select("id, title").First(&art, linkedID).Error; err == nil {
			results = append(results, chat.Attachment{
				ID:    art.ID,
				Title: art.Title,
			})
		}
	}
	return results, nil
}

type RequestBody struct {
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Template string   `json:"template,omitempty"`
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

	db.AutoMigrate(&Article{}, &ArticleLink{})
	ensureFTS(db)

	return db
}

func ensureFTS(db *gorm.DB) {
	err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
		title, 
		content, 
		tokenize='porter'
	)`).Error
	if err != nil {
		if logger != nil {
			logger.Error("Failed to create FTS5 table", zap.Error(err))
		}
		return
	}

	var articleCount, indexedCount int64
	db.Model(&Article{}).Count(&articleCount)
	if err := db.Raw("SELECT count(*) FROM articles_fts").Scan(&indexedCount).Error; err != nil {
		if logger != nil {
			logger.Error("Failed to count FTS5 rows", zap.Error(err))
		}
		return
	}
	if articleCount == indexedCount {
		return
	}

	var articles []Article
	if err := db.Find(&articles).Error; err != nil {
		if logger != nil {
			logger.Error("Failed to load articles for FTS5 rebuild", zap.Error(err))
		}
		return
	}

	db.Exec("DELETE FROM articles_fts")
	for _, article := range articles {
		syncArticleToFTS(db, article.ID, article.Title, article.Tags)
	}
	if logger != nil {
		logger.Info("Rebuilt FTS5 index", zap.Int64("articles", articleCount), zap.Int64("previouslyIndexed", indexedCount))
	}
}

type SearchResult struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
}

func syncArticleToFTS(db *gorm.DB, id int64, title string, tags string) {
	db.Exec("DELETE FROM articles_fts WHERE rowid = ?", id)
	err := db.Exec("INSERT INTO articles_fts(rowid, title, content) VALUES (?, ?, ?)", id, title, tags).Error
	if err != nil && logger != nil {
		logger.Error("Failed to sync article to FTS5", zap.Int64("id", id), zap.Error(err))
	}
}

func deleteArticleFromFTS(db *gorm.DB, id string) {
	err := db.Exec("DELETE FROM articles_fts WHERE rowid = ?", id).Error
	if err != nil && logger != nil {
		logger.Error("Failed to delete article from FTS5", zap.String("id", id), zap.Error(err))
	}
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

	ensureFTS(db)

	dataDirectory := getDataDir()
	serverSettings := loadServerSettings(dataDirectory)
	var settingsMu sync.RWMutex

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
	agents.InitPool(logger, db, dataDirectory, 1, func() {
		graphEngine.InvalidateCache()
		broadcastEvent("graph-updated")
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
	articleFetcher := &articleFileFetcher{dataDir: dataDirectory, db: db}
	chatService := chat.NewService(chatRepo, articleFetcher)

	api := app.Group("/api")

	extractSessionToken := func(c *fiber.Ctx) string {
		token := c.Cookies("readr_session")
		if token == "" {
			authHeader := c.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			}
		}
		return token
	}

	setSessionCookie := func(c *fiber.Ctx, token string) {
		isSecure := c.Protocol() == "https" || c.Get("X-Forwarded-Proto") == "https"
		c.Cookie(&fiber.Cookie{
			Name:     "readr_session",
			Value:    token,
			Path:     "/",
			MaxAge:   int(auth.SessionMaxAge.Seconds()),
			HTTPOnly: true,
			Secure:   isSecure,
			SameSite: "Lax",
		})
	}

	clearSessionCookie := func(c *fiber.Ctx) {
		isSecure := c.Protocol() == "https" || c.Get("X-Forwarded-Proto") == "https"
		c.Cookie(&fiber.Cookie{
			Name:     "readr_session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HTTPOnly: true,
			Secure:   isSecure,
			SameSite: "Lax",
		})
	}

	api.Use(func(c *fiber.Ctx) error {
		path := c.Path()
		if path == "/api/auth/status" || path == "/api/auth/login" || path == "/api/auth/setup" || path == "/api/auth/logout" {
			return c.Next()
		}

		settingsMu.RLock()
		pwdHash := serverSettings.PasswordHash
		secret := serverSettings.SessionSecret
		settingsMu.RUnlock()

		// If no password is set, allow access
		if pwdHash == "" {
			return c.Next()
		}

		token := extractSessionToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}

		valid, err := auth.VerifySession(secret, token, time.Now())
		if err != nil || !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}

		return c.Next()
	})

	api.Get("/auth/status", func(c *fiber.Ctx) error {
		settingsMu.RLock()
		pwdHash := serverSettings.PasswordHash
		secret := serverSettings.SessionSecret
		theme := serverSettings.Theme
		settingsMu.RUnlock()

		authConfigured := pwdHash != ""
		authenticated := false

		if authConfigured {
			token := extractSessionToken(c)
			if token != "" {
				valid, err := auth.VerifySession(secret, token, time.Now())
				if err == nil && valid {
					authenticated = true
				}
			}
		}

		if theme == "" {
			theme = "light"
		}

		return c.JSON(fiber.Map{
			"auth_configured": authConfigured,
			"authenticated":   authenticated,
			"theme":           theme,
		})
	})

	api.Post("/auth/setup", func(c *fiber.Ctx) error {
		settingsMu.Lock()
		defer settingsMu.Unlock()

		if serverSettings.PasswordHash != "" {
			return c.Status(400).JSON(fiber.Map{"error": "Authentication is already configured"})
		}

		var req struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil || len(strings.TrimSpace(req.Password)) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}

		newSettings := serverSettings
		newSettings.PasswordHash = hash
		if newSettings.SessionSecret == "" {
			newSettings.SessionSecret, _ = auth.GenerateRandomSecret()
		}

		if err := saveServerSettings(dataDirectory, newSettings); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}
		serverSettings = newSettings

		// Issue session
		token := auth.SignSession(serverSettings.SessionSecret, time.Now())
		setSessionCookie(c, token)

		return c.JSON(fiber.Map{"status": "success", "token": token})
	})

	api.Post("/auth/login", func(c *fiber.Ctx) error {
		settingsMu.RLock()
		pwdHash := serverSettings.PasswordHash
		secret := serverSettings.SessionSecret
		settingsMu.RUnlock()

		if pwdHash == "" {
			return c.JSON(fiber.Map{"status": "success", "message": "Auth not required"})
		}

		var req struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		if !auth.VerifyPassword(pwdHash, req.Password) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid password"})
		}

		token := auth.SignSession(secret, time.Now())
		setSessionCookie(c, token)

		return c.JSON(fiber.Map{"status": "success", "token": token})
	})

	api.Post("/auth/logout", func(c *fiber.Ctx) error {
		settingsMu.Lock()
		if newSecret, err := auth.GenerateRandomSecret(); err == nil {
			newSettings := serverSettings
			newSettings.SessionSecret = newSecret
			if err := saveServerSettings(dataDirectory, newSettings); err == nil {
				serverSettings = newSettings
			}
		}
		settingsMu.Unlock()

		clearSessionCookie(c)
		return c.JSON(fiber.Map{"status": "success"})
	})

	api.Post("/auth/change-password", func(c *fiber.Ctx) error {
		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := c.BodyParser(&req); err != nil || len(strings.TrimSpace(req.NewPassword)) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": "New password must be at least 6 characters"})
		}

		settingsMu.Lock()
		defer settingsMu.Unlock()

		if serverSettings.PasswordHash != "" {
			if !auth.VerifyPassword(serverSettings.PasswordHash, req.CurrentPassword) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Current password incorrect"})
			}
		}

		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}

		newSecret, _ := auth.GenerateRandomSecret()
		newSettings := serverSettings
		newSettings.PasswordHash = hash
		if newSecret != "" {
			newSettings.SessionSecret = newSecret
		}
		if err := saveServerSettings(dataDirectory, newSettings); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}
		serverSettings = newSettings

		token := auth.SignSession(serverSettings.SessionSecret, time.Now())
		setSessionCookie(c, token)

		return c.JSON(fiber.Map{"status": "success", "token": token})
	})

	extractOpenRouterCredentials := func(c *fiber.Ctx) (string, string) {
		settingsMu.RLock()
		defer settingsMu.RUnlock()
		return serverSettings.APIKey, serverSettings.Model
	}

	api.Get("/templates", func(c *fiber.Ctx) error {
		templates, err := templateRenderer.ListTemplates()
		if err != nil {
			logger.Error("Failed to list templates", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to list templates"})
		}
		return c.JSON(templates)
	})

	api.Get("/articles/:filename", func(c *fiber.Ctx) error {
		filename := c.Params("filename")
		clean := filepath.Clean(filename)
		filePath := filepath.Join(dataDirectory, "articles", clean)

		content, err := os.ReadFile(filePath)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Article not found")
		}
		c.Set("Content-Type", "text/markdown; charset=utf-8")
		// Prevent aggressive browser caching of raw files
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		return c.Send(content)
	})

	api.Get("/chats", func(c *fiber.Ctx) error {
		sessions, err := chatRepo.List(c.Context())
		if err != nil {
			logger.Error("Failed to list chat sessions", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to list chat sessions"})
		}
		return c.JSON(sessions)
	})

	// @title Readr Vault API
	api.Post("/chats", func(c *fiber.Ctx) error {
		var req struct {
			Title string `json:"title"`
		}
		_ = c.BodyParser(&req)

		title := req.Title
		if strings.TrimSpace(title) == "" {
			title = "New Chat"
		}

		session := &chat.ChatSession{
			ID:        uuid.New().String(),
			Title:     title,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Messages:  make([]chat.Message, 0),
		}

		if err := chatRepo.Save(c.Context(), session); err != nil {
			logger.Error("Failed to create chat session", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create chat session"})
		}
		return c.JSON(session)
	})

	api.Get("/chats/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		session, err := chatRepo.Get(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chat session not found"})
		}
		return c.JSON(session)
	})

	api.Delete("/chats/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if err := chatRepo.Delete(c.Context(), id); err != nil {
			logger.Error("Failed to delete chat session", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Failed to delete chat session"})
		}
		return c.JSON(fiber.Map{"status": "success"})
	})

	api.Get("/models", func(c *fiber.Ctx) error {
		apiKey, _ := extractOpenRouterCredentials(c)

		data, err := chatService.FetchModels(c.Context(), apiKey)
		if err != nil {
			logger.Error("Failed to fetch models from OpenRouter", zap.Error(err))
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to fetch models from OpenRouter"})
		}
		c.Set("Content-Type", "application/json")
		return c.Send(data)
	})

	api.Post("/chats/:id/message", func(c *fiber.Ctx) error {
		apiKey, _ := extractOpenRouterCredentials(c)
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "API key required in settings.json to use chat"})
		}

		var req struct {
			Role          chat.MessageRole  `json:"role"`
			Content       string            `json:"content"`
			Attachments   []chat.Attachment `json:"attachments,omitempty"`
			Model         string            `json:"model,omitempty"`
			ExpandContext bool              `json:"expandContext,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid message payload"})
		}
		if req.Role == "" {
			req.Role = chat.RoleUser
		}

		msg := chat.Message{
			Role:        req.Role,
			Content:     req.Content,
			Attachments: req.Attachments,
		}

		sessionID := c.Params("id")

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			err := chatService.StreamMessage(ctx, sessionID, apiKey, req.Model, req.ExpandContext, msg, func(chunk string) error {
				data, _ := json.Marshal(fiber.Map{"text": chunk})
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return err
				}
				return w.Flush()
			})
			if err != nil {
				logger.Error("StreamMessage error", zap.Error(err))
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				w.Flush()
				return
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			w.Flush()
		})
		return nil
	})

	api.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello from Go!"})
	})


	api.Get("/settings", func(c *fiber.Ctx) error {
		// Reload from disk to catch manual edits
		freshSettings := loadServerSettings(dataDirectory)
		
		settingsMu.Lock()
		serverSettings = freshSettings
		settingsMu.Unlock()

		return c.JSON(fiber.Map{
			"api_key":                 freshSettings.APIKey,
			"model":                   freshSettings.Model,
			"agent_enricher":          freshSettings.AgentEnricher,
			"agent_linker":            freshSettings.AgentLinker,
			"agent_summarizer":        freshSettings.AgentSummarizer,
			"theme":                   freshSettings.Theme,
			"view_mode":               freshSettings.ViewMode,
			"graph_context_expansion": freshSettings.GraphContextExpansion,
		})
	})

	api.Post("/settings", func(c *fiber.Ctx) error {
		var req ServerSettings
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}
		settingsMu.Lock()
		defer settingsMu.Unlock()

		newSettings := req
		if newSettings.Model == "" {
			newSettings.Model = "openai/gpt-4o-mini"
		}
		if newSettings.PasswordHash == "" {
			newSettings.PasswordHash = serverSettings.PasswordHash
		}
		if newSettings.SessionSecret == "" {
			newSettings.SessionSecret = serverSettings.SessionSecret
		}

		if err := saveServerSettings(dataDirectory, newSettings); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}

		serverSettings = newSettings
		return c.JSON(fiber.Map{"status": "success"})
	})

	api.Post("/articles/:id/reparse", func(c *fiber.Ctx) error {
		idParam := c.Params("id")

		var article Article
		if err := db.First(&article, idParam).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Article not found"})
		}

		apiKey, _ := extractOpenRouterCredentials(c)

		settingsMu.RLock()
		agentEnricher := serverSettings.AgentEnricher
		agentLinker := serverSettings.AgentLinker
		agentSummarizer := serverSettings.AgentSummarizer
		settingsMu.RUnlock()

		if agentEnricher {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeEnrichFrontmatter,
			})
		}
		if agentLinker {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeAutoLinker,
			})
		}
		if apiKey != "" && agentSummarizer {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeSummarizer,
			})
		}

		return c.JSON(fiber.Map{"status": "ok", "message": "Agents triggered"})
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

		deleteArticleFromFTS(db, id)
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

	api.Get("/search", func(c *fiber.Ctx) error {
		results := make([]SearchResult, 0)

		// Format query for prefix matching (e.g. "pay" -> "pay*")
		cleanQuery := strings.ReplaceAll(c.Query("q"), "\"", "")
		cleanQuery = strings.ReplaceAll(cleanQuery, "'", "")
		cleanQuery = strings.ReplaceAll(cleanQuery, "*", "")

		parts := strings.Fields(cleanQuery)
		if len(parts) == 0 {
			return c.JSON(results)
		}
		for i, p := range parts {
			parts[i] = p + "*"
		}
		safeQuery := strings.Join(parts, " OR ")

		err := db.Raw(`
			SELECT rowid as id, title, snippet(articles_fts, 1, '<mark>', '</mark>', '...', 25) as excerpt
			FROM articles_fts
			WHERE articles_fts MATCH ?
			ORDER BY bm25(articles_fts, 1.0, 3.0)
			LIMIT 15
		`, safeQuery).Scan(&results).Error

		if err != nil {
			logger.Error("FTS search failed", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": "Search failed"})
		}

		return c.JSON(results)
	})

	api.Get("/events", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			ch := make(chan string, 10)
			eventClients.Store(ch, true)
			defer eventClients.Delete(ch)

			// Send initial comment to establish SSE stream
			fmt.Fprintf(w, ": connected\n\n")
			w.Flush()

			for msg := range ch {
				fmt.Fprintf(w, "data: %s\n\n", msg)
				if err := w.Flush(); err != nil {
					return
				}
			}
		})
		return nil
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

		syncArticleToFTS(db, article.ID, article.Title, article.Tags)

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

		apiKey, model := extractOpenRouterCredentials(c)

		article, err := ingester.Ingest(c.Context(), ingest.IngestRequest{
			URL:      body.URL,
			Tags:     body.Tags,
			Template: body.Template,
			APIKey:   apiKey,
			Model:    model,
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

		syncArticleToFTS(db, article.ID, article.Title, article.Tags)

		graphEngine.InvalidateCache()
		logger.Info("Article added successfully", zap.Int64("id", article.ID), zap.String("url", body.URL))

		settingsMu.RLock()
		agentEnricher := serverSettings.AgentEnricher
		agentLinker := serverSettings.AgentLinker
		agentSummarizer := serverSettings.AgentSummarizer
		settingsMu.RUnlock()

		if agentEnricher {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeEnrichFrontmatter,
			})
		}
		if agentLinker {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeAutoLinker,
			})
		}
		if apiKey != "" && agentSummarizer {
			agents.SubmitJob(agents.Job{
				ArticleID: article.ID,
				Type:      agents.JobTypeSummarizer,
			})
		}
		return c.JSON(fiber.Map{
			"status":  "success",
			"message": "Article saved",
			"id":      article.ID,
		})
	})

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
				path := c.Path()
				if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/images") || strings.HasPrefix(path, "/articles") {
					return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Not Found"})
				}
				indexPath := filepath.Join(distDir, "index.html")
				if _, err := os.Stat(indexPath); err == nil {
					return c.SendFile(indexPath)
				}
				return c.Status(fiber.StatusNotFound).SendString("index.html not found")
			})
		}
	}

	return app
}

func main() {
	initLogger()
	defer logger.Sync()

	logger.Info("Available SQL drivers", zap.Strings("drivers", sql.Drivers()))

	db := initDB()

	app := setupApp(db)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	logger.Info("Starting server", zap.String("port", port))
	if err := app.Listen(port); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
