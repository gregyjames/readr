package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"example.com/backend/internal/chat"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func RegisterChat(router fiber.Router, h *HandlerContext) {
	router.Get("/chats", func(c *fiber.Ctx) error {
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

	router.Post("/chats", func(c *fiber.Ctx) error {
		if h.ChatRepo == nil {
			return c.Status(500).JSON(fiber.Map{"error": "Chat repository unavailable"})
		}
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

		if err := h.ChatRepo.Save(c.Context(), session); err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to create chat session", zap.Error(err))
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create chat session"})
		}
		return c.JSON(session)
	})

	router.Get("/chats/:id", func(c *fiber.Ctx) error {
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

	router.Delete("/chats/:id", func(c *fiber.Ctx) error {
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

	router.Get("/models", func(c *fiber.Ctx) error {
		apiKey, _ := h.SettingsStore.ExtractOpenRouterCredentials()

		if h.ChatService == nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Chat service unavailable"})
		}

		data, err := h.ChatService.FetchModels(c.Context(), apiKey)
		if err != nil {
			if h.Logger != nil {
				h.Logger.Error("Failed to fetch models from OpenRouter", zap.Error(err))
			}
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to fetch models from OpenRouter"})
		}
		c.Set("Content-Type", "application/json")
		return c.Send(data)
	})

	router.Post("/chats/:id/message", func(c *fiber.Ctx) error {
		apiKey, _ := h.SettingsStore.ExtractOpenRouterCredentials()
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

			err := h.ChatService.StreamMessage(ctx, sessionID, apiKey, req.Model, req.ExpandContext, msg, func(chunk string) error {
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
		return nil
	})
}
