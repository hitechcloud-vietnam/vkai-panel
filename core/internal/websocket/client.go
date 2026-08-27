package websocket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

// Client represents a WebSocket client connection
type Client struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	TenantID uuid.UUID
	hub      *Hub
	conn     *ws.Conn
	send     chan *Message
	logger   *zap.Logger
}

// NewClient creates a new WebSocket client
func NewClient(hub *Hub, conn *ws.Conn, userID, tenantID uuid.UUID, logger *zap.Logger) *Client {
	return &Client{
		ID:       uuid.New(),
		UserID:   userID,
		TenantID: tenantID,
		hub:      hub,
		conn:     conn,
		send:     make(chan *Message, 256),
		logger:   logger,
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if ws.IsUnexpectedCloseError(err, ws.CloseGoingAway, ws.CloseNormalClosure) {
				c.logger.Error("WebSocket read error",
					zap.String("client_id", c.ID.String()),
					zap.Error(err),
				)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			c.logger.Error("Failed to unmarshal message", zap.Error(err))
			continue
		}

		// Set sender info
		msg.SenderID = c.UserID
		msg.TenantID = c.TenantID
		msg.Timestamp = time.Now()

		// Send to hub for processing
		c.hub.broadcast <- &msg
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(ws.TextMessage)
			if err != nil {
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				c.logger.Error("Failed to marshal message", zap.Error(err))
				continue
			}

			w.Write(data)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// SendMessage sends a message directly to this client
func (c *Client) SendMessage(msg *Message) {
	select {
	case c.send <- msg:
	default:
		c.logger.Warn("Client send buffer full, dropping message",
			zap.String("client_id", c.ID.String()),
		)
	}
}
