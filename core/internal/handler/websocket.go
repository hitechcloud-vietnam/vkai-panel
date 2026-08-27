package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/websocket"
)

// checkWSOrigin only accepts upgrades from an origin on the configured CORS
// allowlist. A request with no Origin header comes from a non-browser client
// (which cannot be tricked into a cross-site upgrade) and is allowed; anything
// else must match exactly.
func checkWSOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	for _, allowed := range middleware.AllowedOrigins() {
		if a, err := url.Parse(allowed); err == nil {
			if strings.EqualFold(a.Host, parsed.Host) && strings.EqualFold(a.Scheme, parsed.Scheme) {
				return true
			}
		}
	}

	// Same-origin upgrades (the panel serving its own UI) are always fine.
	return strings.EqualFold(parsed.Host, r.Host)
}

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWSOrigin,
}

// tenantRoomPrefix namespaces every room by tenant so a client can neither
// broadcast into nor observe another tenant's channel.
func tenantRoomPrefix(tenantID uuid.UUID) string {
	return "tenant:" + tenantID.String() + ":"
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
	// The auth middleware stores these as strings, so they are parsed rather
	// than type-asserted (the old assertion always failed).
	uid := middleware.GetUserID(c)
	tid := middleware.GetTenantID(c)
	if uid == uuid.Nil || tid == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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

	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	scoped := tenantRoomPrefix(tenantID) + room

	c.JSON(http.StatusOK, gin.H{
		"room":              room,
		"connected_clients": h.hub.GetRoomClients(scoped),
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

	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if request.Room == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room is required"})
		return
	}

	// The room name is forced into the caller's own tenant namespace, so a
	// crafted name cannot reach another tenant's clients.
	scoped := tenantRoomPrefix(tenantID) + strings.TrimPrefix(request.Room, tenantRoomPrefix(tenantID))

	msgType := websocket.MessageType(request.Type)
	h.hub.BroadcastToRoom(scoped, msgType, request.Payload)

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

	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Only clients belonging to the caller's tenant may be addressed.
	if !h.hub.IsUserInTenant(userID, tenantID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recipient not found"})
		return
	}

	msgType := websocket.MessageType(request.Type)
	h.hub.BroadcastToUser(userID, msgType, request.Payload)

	c.JSON(http.StatusOK, gin.H{"message": "direct message sent"})
}
