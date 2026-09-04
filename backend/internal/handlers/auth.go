package handlers

import (
	"context"
	"strings"
	"time"

	"example.com/backend/internal/auth"
	"github.com/gofiber/fiber/v2"
)

func ExtractSessionToken(c *fiber.Ctx) string {
	var token string
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	} else if authHeader != "" && !strings.Contains(authHeader, " ") {
		token = authHeader
	}

	if token == "" {
		token = strings.TrimSpace(c.Get("X-API-Key"))
		if token == "" {
			token = strings.TrimSpace(c.Get("X-Api-Key"))
		}
	}

	if token == "" {
		token = strings.TrimSpace(c.Cookies("readr_session"))
	}

	token = strings.Trim(token, "\"'`")
	return token
}

func buildSessionCookie(c *fiber.Ctx, value string, maxAge int) *fiber.Cookie {
	isSecure := c.Protocol() == "https" || c.Get("X-Forwarded-Proto") == "https"
	return &fiber.Cookie{
		Name:     "readr_session",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HTTPOnly: true,
		Secure:   isSecure,
		SameSite: "Lax",
	}
}

func SetSessionCookie(c *fiber.Ctx, token string) {
	c.Cookie(buildSessionCookie(c, token, int(auth.SessionMaxAge.Seconds())))
}

func ClearSessionCookie(c *fiber.Ctx) {
	c.Cookie(buildSessionCookie(c, "", -1))
}

func AuthMiddleware(h *HandlerContext) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		if path == "/api/auth/status" || path == "/api/auth/login" || path == "/api/auth/setup" || path == "/api/auth/logout" {
			return c.Next()
		}

		current := h.SettingsStore.Get()
		pwdHash := current.PasswordHash
		secret := current.SessionSecret

		token := ExtractSessionToken(c)

		// If no password is set, allow access
		if pwdHash == "" {
			if strings.HasPrefix(token, "rdr_live_") && h.Repo != nil {
				hash := auth.HashAPIKey(token)
				if apiKey, err := h.Repo.FindAPIKeyByHash(c.Context(), hash); err == nil && apiKey != nil {
					go func(id int64) {
						_ = h.Repo.TouchAPIKeyLastUsed(context.Background(), id, time.Now())
					}(apiKey.ID)
				}
			}
			return c.Next()
		}

		// Check if it's an API Key (starts with rdr_live_)
		if strings.HasPrefix(token, "rdr_live_") {
			if h.Repo != nil {
				hash := auth.HashAPIKey(token)
				apiKey, err := h.Repo.FindAPIKeyByHash(c.Context(), hash)
				if err == nil && apiKey != nil {
					go func(id int64) {
						_ = h.Repo.TouchAPIKeyLastUsed(context.Background(), id, time.Now())
					}(apiKey.ID)
					return c.Next()
				}
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid API key"})
		}

		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}

		valid, err := auth.VerifySession(secret, token, time.Now())
		if err != nil || !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}

		return c.Next()
	}
}

func RegisterAuth(router fiber.Router, h *HandlerContext) {
	router.Get("/auth/status", func(c *fiber.Ctx) error {
		current := h.SettingsStore.Get()
		authConfigured := current.PasswordHash != ""
		authenticated := false

		if authConfigured {
			cookieOrHeaderToken := ExtractSessionToken(c)
			if cookieOrHeaderToken != "" {
				valid, err := auth.VerifySession(current.SessionSecret, cookieOrHeaderToken, time.Now())
				if err == nil && valid {
					authenticated = true
				}
			}
		}

		theme := current.Theme
		if theme == "" {
			theme = "light"
		}

		return c.JSON(fiber.Map{
			"auth_configured": authConfigured,
			"authenticated":   authenticated,
			"theme":           theme,
		})
	})

	router.Post("/auth/setup", func(c *fiber.Ctx) error {
		current := h.SettingsStore.Get()
		if current.PasswordHash != "" {
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

		var token string
		_, updateErr := h.SettingsStore.Update(func(s *ServerSettings) error {
			s.PasswordHash = hash
			if s.SessionSecret == "" {
				s.SessionSecret, _ = auth.GenerateRandomSecret()
			}
			token = auth.SignSession(s.SessionSecret, time.Now())
			return nil
		})

		if updateErr != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}

		SetSessionCookie(c, token)
		return c.JSON(fiber.Map{"status": "success", "token": token})
	})

	router.Post("/auth/login", func(c *fiber.Ctx) error {
		current := h.SettingsStore.Get()
		if current.PasswordHash == "" {
			return c.JSON(fiber.Map{"status": "success", "message": "Auth not required"})
		}

		var req struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		if !auth.VerifyPassword(current.PasswordHash, req.Password) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid password"})
		}

		token := auth.SignSession(current.SessionSecret, time.Now())
		SetSessionCookie(c, token)

		return c.JSON(fiber.Map{"status": "success", "token": token})
	})

	router.Post("/auth/logout", func(c *fiber.Ctx) error {
		_, _ = h.SettingsStore.Update(func(s *ServerSettings) error {
			if newSecret, err := auth.GenerateRandomSecret(); err == nil {
				s.SessionSecret = newSecret
			}
			return nil
		})

		ClearSessionCookie(c)
		return c.JSON(fiber.Map{"status": "success"})
	})

	router.Post("/auth/change-password", func(c *fiber.Ctx) error {
		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := c.BodyParser(&req); err != nil || len(strings.TrimSpace(req.NewPassword)) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": "New password must be at least 6 characters"})
		}

		current := h.SettingsStore.Get()
		if current.PasswordHash != "" {
			if !auth.VerifyPassword(current.PasswordHash, req.CurrentPassword) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Current password incorrect"})
			}
		}

		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}

		var token string
		_, updateErr := h.SettingsStore.Update(func(s *ServerSettings) error {
			s.PasswordHash = hash
			if newSecret, err := auth.GenerateRandomSecret(); err == nil && newSecret != "" {
				s.SessionSecret = newSecret
			}
			token = auth.SignSession(s.SessionSecret, time.Now())
			return nil
		})

		if updateErr != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save settings"})
		}

		SetSessionCookie(c, token)
		return c.JSON(fiber.Map{"status": "success", "token": token})
	})
}
