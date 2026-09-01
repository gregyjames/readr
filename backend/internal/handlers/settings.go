package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"example.com/backend/internal/auth"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type ServerSettings struct {
	APIKey                  string `json:"api_key"`
	Model                   string `json:"model"`
	AgentEnricher           bool   `json:"agent_enricher"`
	AgentLinker             bool   `json:"agent_linker"`
	AgentSummarizer         bool   `json:"agent_summarizer"`
	LibrarianEnabled        bool   `json:"librarian_enabled"`
	LibrarianCron           string `json:"librarian_cron"`
	LibrarianMinClusterSize int    `json:"librarian_min_cluster_size"`
	Theme                   string `json:"theme"`
	ViewMode                string `json:"view_mode"`
	GraphContextExpansion   bool   `json:"graph_context_expansion"`
	PasswordHash            string `json:"password_hash,omitempty"`
	SessionSecret           string `json:"session_secret,omitempty"`
}

type SettingsStore struct {
	mu       sync.RWMutex
	dataDir  string
	settings ServerSettings
	logger   *zap.Logger
}

func NewSettingsStore(dataDir string, logger *zap.Logger) *SettingsStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &SettingsStore{
		dataDir: dataDir,
		logger:  logger,
	}
	s.settings = s.loadFromDisk()
	return s
}

func (s *SettingsStore) loadFromDisk() ServerSettings {
	settingsPath := filepath.Join(s.dataDir, "settings.json")
	defaults := ServerSettings{
		Model:                   "openai/gpt-4o-mini",
		AgentEnricher:           true,
		AgentLinker:             true,
		AgentSummarizer:         true,
		LibrarianEnabled:        true,
		LibrarianCron:           "0 0 * * *",
		LibrarianMinClusterSize: 5,
		Theme:                   "light",
		ViewMode:                "card",
		GraphContextExpansion:   true,
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			defaults.SessionSecret, _ = auth.GenerateRandomSecret()
			_ = s.saveToDisk(defaults)
			return defaults
		}
		s.logger.Fatal("Failed to read settings file", zap.String("path", settingsPath), zap.Error(err))
	}

	res := defaults
	if err := json.Unmarshal(data, &res); err != nil {
		s.logger.Fatal("Failed to parse settings file", zap.String("path", settingsPath), zap.Error(err))
	}
	if res.Model == "" {
		res.Model = "openai/gpt-4o-mini"
	}
	if res.SessionSecret == "" {
		res.SessionSecret, _ = auth.GenerateRandomSecret()
		_ = s.saveToDisk(res)
	}
	return res
}

func (s *SettingsStore) saveToDisk(settings ServerSettings) error {
	settingsPath := filepath.Join(s.dataDir, "settings.json")
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, bytes, 0600)
}

func (s *SettingsStore) Get() ServerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *SettingsStore) Reload() ServerSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = s.loadFromDisk()
	return s.settings
}

func (s *SettingsStore) Update(fn func(current *ServerSettings) error) (ServerSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated := s.settings
	if err := fn(&updated); err != nil {
		return s.settings, err
	}

	if err := s.saveToDisk(updated); err != nil {
		return s.settings, err
	}
	s.settings = updated
	return s.settings, nil
}

func (s *SettingsStore) ExtractOpenRouterCredentials() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.APIKey, s.settings.Model
}

func RegisterSettings(router fiber.Router, h *HandlerContext) {
	router.Get("/settings", func(c *fiber.Ctx) error {
		fresh := h.SettingsStore.Reload()
		return c.JSON(fiber.Map{
			"api_key":                    fresh.APIKey,
			"model":                      fresh.Model,
			"agent_enricher":             fresh.AgentEnricher,
			"agent_linker":               fresh.AgentLinker,
			"agent_summarizer":           fresh.AgentSummarizer,
			"librarian_enabled":          fresh.LibrarianEnabled,
			"librarian_cron":             fresh.LibrarianCron,
			"librarian_min_cluster_size": fresh.LibrarianMinClusterSize,
			"theme":                      fresh.Theme,
			"view_mode":                  fresh.ViewMode,
			"graph_context_expansion":    fresh.GraphContextExpansion,
		})
	})

	router.Post("/settings", func(c *fiber.Ctx) error {
		var req ServerSettings
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}

		updated, err := h.SettingsStore.Update(func(current *ServerSettings) error {
			current.APIKey = req.APIKey
			if req.Model != "" {
				current.Model = req.Model
			} else {
				current.Model = "openai/gpt-4o-mini"
			}
			current.AgentEnricher = req.AgentEnricher
			current.AgentLinker = req.AgentLinker
			current.AgentSummarizer = req.AgentSummarizer
			current.LibrarianEnabled = req.LibrarianEnabled
			if req.LibrarianCron != "" {
				current.LibrarianCron = req.LibrarianCron
			}
			if req.LibrarianMinClusterSize > 0 {
				current.LibrarianMinClusterSize = req.LibrarianMinClusterSize
			}
			if req.Theme != "" {
				current.Theme = req.Theme
			}
			if req.ViewMode != "" {
				current.ViewMode = req.ViewMode
			}
			current.GraphContextExpansion = req.GraphContextExpansion
			return nil
		})

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}

		if h.LibrarianCron != nil {
			if err := h.LibrarianCron.Start(updated.LibrarianCron, updated.LibrarianEnabled); err != nil && h.Logger != nil {
				h.Logger.Error("Failed to reconfigure Librarian cron scheduler",
					zap.String("cron", updated.LibrarianCron),
					zap.Bool("enabled", updated.LibrarianEnabled),
					zap.Error(err),
				)
			}
		}

		return c.JSON(fiber.Map{"status": "success"})
	})
}
