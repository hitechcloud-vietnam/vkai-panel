package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/websocket"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking
		return true
	},
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub    *websocket.Hub
	logger *zap.Logger
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *websocket.Hub, logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		hub:    hub,
		logger: logger,
	}
}

// HandleConnection handles WebSocket upgrade requests
func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
	// Get user info from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tenantID, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	uid, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id"})
		return
	}

	tid, ok := tenantID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid tenant id"})
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}

	// Create client
	client := websocket.NewClient(h.hub, conn, uid, tid, h.logger)

	// Register client with hub
	h.hub.Register(client)

	// Start read/write pumps
	go client.WritePump()
	go client.ReadPump()

	h.logger.Info("WebSocket connection established",
		zap.String("user_id", uid.String()),
		zap.String("tenant_id", tid.String()),
	)
}

// GetStatus returns WebSocket hub status
func (h *WebSocketHandler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"connected_clients": h.hub.GetConnectedClients(),
		"active_rooms":      h.hub.GetRooms(),
	})
}

// GetRoomStatus returns status of a specific room
func (h *WebSocketHandler) GetRoomStatus(c *gin.Context) {
	room := c.Param("room")
	if room == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room name required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"room":           room,
		"connected_clients": h.hub.GetRoomClients(room),
	})
}

// BroadcastMessage broadcasts a message to a room or all clients
func (h *WebSocketHandler) BroadcastMessage(c *gin.Context) {
	var request struct {
		Room    string      `json:"room"`
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msgType := websocket.MessageType(request.Type)
	h.hub.BroadcastToRoom(request.Room, msgType, request.Payload)

	c.JSON(http.StatusOK, gin.H{"message": "broadcast sent"})
}

// SendDirectMessage sends a message to a specific user
func (h *WebSocketHandler) SendDirectMessage(c *gin.Context) {
	var request struct {
		UserID  string      `json:"user_id"`
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := uuid.Parse(request.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	msgType := websocket.MessageType(request.Type)
	h.hub.BroadcastToUser(userID, msgType, request.Payload)

	c.JSON(http.StatusOK, gin.H{"message": "direct message sent"})
}
