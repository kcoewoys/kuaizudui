package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const activityEventHeartbeat = 20 * time.Second

type activityEvent struct {
	Type string `json:"type"`
}

func (s *Server) activityEvents(c *gin.Context) {
	subscription, err := s.activityUpdates.Subscribe(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, err)
		return
	}
	defer func() { _ = subscription.Close() }()

	controller := http.NewResponseController(c.Writer)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		fail(c, fmt.Errorf("disable activity event write deadline: %w", err))
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if _, err := c.Writer.WriteString(": connected\n\n"); err != nil {
		return
	}
	c.Writer.Flush()

	heartbeat := time.NewTicker(activityEventHeartbeat)
	defer heartbeat.Stop()
	messages := subscription.Channel()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := c.Writer.WriteString(": keepalive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case message, ok := <-messages:
			if !ok {
				return
			}
			payload, err := json.Marshal(activityEvent{Type: message.Payload})
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: activity\ndata: %s\n\n", payload); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
