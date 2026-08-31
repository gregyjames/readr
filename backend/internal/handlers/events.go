package handlers

import (
	"bufio"
	"fmt"
	"sync"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type EventHub struct {
	clients sync.Map
	logger  *zap.Logger
}

func NewEventHub(logger *zap.Logger) *EventHub {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EventHub{
		logger: logger,
	}
}

func (h *EventHub) Broadcast(event string) {
	clientCount := 0
	h.clients.Range(func(key, value interface{}) bool {
		if ch, ok := key.(chan string); ok {
			clientCount++
			select {
			case ch <- event:
			default:
			}
		}
		return true
	})
	if h.logger != nil {
		h.logger.Info("SSE Broadcast sent", zap.String("event", event), zap.Int("connected_clients", clientCount))
	}
}

func RegisterEvents(router fiber.Router, h *HandlerContext) {
	router.Get("/events", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			clientChan := make(chan string, 10)
			h.EventHub.clients.Store(clientChan, true)
			defer func() {
				h.EventHub.clients.Delete(clientChan)
				close(clientChan)
			}()

			fmt.Fprintf(w, "data: connected\n\n")
			if err := w.Flush(); err != nil {
				return
			}

			for msg := range clientChan {
				fmt.Fprintf(w, "data: %s\n\n", msg)
				if err := w.Flush(); err != nil {
					return
				}
			}
		})

		return nil
	})
}
