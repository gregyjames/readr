package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"example.com/backend/internal/chat"
	"example.com/backend/internal/llm"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func validateProviderBaseURL(rawURL string, providerType string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", errors.New("empty provider base URL")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid provider base URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("provider base URL must not contain credentials, query parameters, or fragments")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("provider base URL must use http or https scheme")
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "" || hostname == "metadata.google.internal" || strings.HasSuffix(hostname, ".internal") {
		return "", errors.New("disallowed provider base URL host")
	}

	isLoopbackHost := hostname == "localhost"
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() {
			isLoopbackHost = true
		} else if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return "", errors.New("disallowed private or link-local IP in provider base URL")
		}
	}

	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "openrouter":
		if !isLoopbackHost && (scheme != "https" || hostname != "openrouter.ai") {
			return "", errors.New("openrouter base URL must use https://openrouter.ai")
		}
	case "openai":
		if !isLoopbackHost && (scheme != "https" || hostname != "api.openai.com") {
			return "", errors.New("openai base URL must use https://api.openai.com")
		}
	}

	return strings.TrimRight(u.String(), "/"), nil
}

func RegisterChat(router fiber.Router, h *HandlerContext) {
	router.Get("/chats", func(c fiber.Ctx) error {
		if h.ChatRepo == nil {
			return c.JSON([]any{})
		}
		sessions, err := h.ChatRepo.List(c.Context())
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to list chat sessions", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to list chat sessions"})
		}
		return c.JSON(sessions)
	})

	router.Post("/chats", func(c fiber.Ctx) error {
		if h.ChatRepo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Chat repository unavailable"})
		}
		var req struct {
			Title string `json:"title"`
		}
		_ = c.Bind().Body(&req)

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

		if err := h.ChatRepo.Save(c.Context(), session); err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to create chat session", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create chat session"})
		}
		return c.JSON(session)
	})

	router.Get("/chats/:id", func(c fiber.Ctx) error {
		if h.ChatRepo == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chat session not found"})
		}
		id := c.Params("id")
		session, err := h.ChatRepo.Get(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Chat session not found"})
		}
		return c.JSON(session)
	})

	router.Delete("/chats/:id", func(c fiber.Ctx) error {
		if h.ChatRepo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Chat repository unavailable"})
		}
		id := c.Params("id")
		if err := h.ChatRepo.Delete(c.Context(), id); err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to delete chat session", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to delete chat session"})
		}
		return c.JSON(fiber.Map{"status": "success"})
	})

	handleGetModels := func(c fiber.Ctx) error {
		if h.ChatService == nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Chat service unavailable"})
		}

		llmCfg := h.SettingsStore.GetLLMConfig()
		storedType := strings.TrimSpace(llmCfg.Type)
		if storedType == "" {
			storedType = "openrouter"
		}
		storedBaseURL := strings.TrimRight(strings.TrimSpace(llmCfg.BaseURL), "/")
		if storedBaseURL == "" {
			storedBaseURL = llm.DefaultBaseURL(storedType)
		}

		headerType := strings.TrimSpace(c.Get("X-Provider-Type"))
		headerBaseURL := strings.TrimSpace(c.Get("X-Provider-Base-Url"))
		headerAPIKey := strings.TrimSpace(c.Get("X-OpenRouter-Key"))

		providerChanged := false
		if headerType != "" {
			if !strings.EqualFold(headerType, storedType) {
				providerChanged = true
			}
			llmCfg.Type = headerType
		}
		if headerBaseURL != "" {
			validatedURL, err := validateProviderBaseURL(headerBaseURL, llmCfg.Type)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			if validatedURL != storedBaseURL {
				providerChanged = true
			}
			llmCfg.BaseURL = validatedURL
		} else if providerChanged {
			llmCfg.BaseURL = llm.DefaultBaseURL(llmCfg.Type)
		}

		if providerChanged {
			llmCfg.APIKey = ""
		}
		if headerAPIKey != "" {
			llmCfg.APIKey = headerAPIKey
		}

		provider := llm.NewProvider(llmCfg)
		models, err := h.ChatService.GetModels(c.Context(), provider)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to fetch models from provider", zap.Error(err))
			}
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to fetch models from provider"})
		}
		return c.JSON(fiber.Map{"data": models})
	}
	router.Get("/models", handleGetModels)
	router.Get("/chat/models", handleGetModels)

	router.Post("/chats/:id/message", func(c fiber.Ctx) error {
		llmCfg := h.SettingsStore.GetLLMConfig()
		if llm.RequiresAPIKey(llmCfg.Type) && strings.TrimSpace(llmCfg.APIKey) == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "API key required in settings.json to use chat"})
		}
		provider := llm.NewProvider(llmCfg)

		var req struct {
			Role          chat.MessageRole  `json:"role"`
			Content       string            `json:"content"`
			Attachments   []chat.Attachment `json:"attachments,omitempty"`
			Model         string            `json:"model,omitempty"`
			ExpandContext bool              `json:"expandContext,omitempty"`
		}
		if err := c.Bind().Body(&req); err != nil {
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

		return c.SendStreamWriter(func(w *bufio.Writer) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			err := h.ChatService.StreamMessage(ctx, sessionID, provider, req.Model, req.ExpandContext, msg, func(chunk string) error {
				data, _ := json.Marshal(fiber.Map{"text": chunk})
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return err
				}
				return w.Flush()
			})
			if err != nil {
				if h.Logger != nil {
					h.Logger.Error("StreamMessage error", zap.Error(err))
				}
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				w.Flush()
				return
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			w.Flush()
		})
	})
}
