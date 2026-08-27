package websocket

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	rooms      map[string]map[*Client]bool   // room -> clients
	userRooms  map[uuid.UUID]map[string]bool // user -> rooms
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Message
	roomsMu    sync.RWMutex
	logger     *zap.Logger
}

// NewHub creates a new Hub instance
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		rooms:      make(map[string]map[*Client]bool),
		userRooms:  make(map[uuid.UUID]map[string]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		logger:     logger,
	}
}

// Register registers a client with the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.logger.Info("Client connected",
				zap.String("client_id", client.ID.String()),
				zap.String("user_id", client.UserID.String()),
			)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.removeClientFromAllRooms(client)
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("Client disconnected",
					zap.String("client_id", client.ID.String()),
					zap.String("user_id", client.UserID.String()),
				)
			}

		case message := <-h.broadcast:
			h.handleMessage(message)
		}
	}
}

// handleMessage processes incoming messages
func (h *Hub) handleMessage(msg *Message) {
	switch msg.Type {
	case MsgTypeJoinRoom:
		h.handleJoinRoom(msg)
	case MsgTypeLeaveRoom:
		h.handleLeaveRoom(msg)
	case MsgTypeBroadcast:
		h.handleBroadcast(msg)
	case MsgTypeDirect:
		h.handleDirect(msg)
	default:
		h.logger.Warn("Unknown message type", zap.String("type", string(msg.Type)))
	}
}

// TenantRoomPrefix namespaces a room name by tenant. Clients may only join,
// leave and receive on rooms inside their own tenant namespace.
func TenantRoomPrefix(tenantID uuid.UUID) string {
	return "tenant:" + tenantID.String() + ":"
}

// scopeRoom forces a client supplied room name into that client's tenant
// namespace, so a crafted name cannot address another tenant's room.
func scopeRoom(client *Client, room string) string {
	prefix := TenantRoomPrefix(client.TenantID)
	return prefix + strings.TrimPrefix(room, prefix)
}

// handleJoinRoom handles room join requests
func (h *Hub) handleJoinRoom(msg *Message) {
	var payload RoomPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.logger.Error("Failed to unmarshal join room payload", zap.Error(err))
		return
	}

	client := h.findClientByID(msg.SenderID)
	if client == nil {
		return
	}

	payload.Room = scopeRoom(client, payload.Room)

	h.roomsMu.Lock()
	defer h.roomsMu.Unlock()

	// Add client to room
	if h.rooms[payload.Room] == nil {
		h.rooms[payload.Room] = make(map[*Client]bool)
	}
	h.rooms[payload.Room][client] = true

	// Track user's rooms
	if h.userRooms[client.UserID] == nil {
		h.userRooms[client.UserID] = make(map[string]bool)
	}
	h.userRooms[client.UserID][payload.Room] = true

	h.logger.Info("Client joined room",
		zap.String("client_id", client.ID.String()),
		zap.String("room", payload.Room),
	)

	// Notify room
	h.sendToRoom(payload.Room, &Message{
		Type:      MsgTypeRoomNotification,
		Room:      payload.Room,
		Payload:   json.RawMessage(`{"event":"user_joined","user_id":"` + client.UserID.String() + `"}`),
		Timestamp: time.Now(),
	})
}

// handleLeaveRoom handles room leave requests
func (h *Hub) handleLeaveRoom(msg *Message) {
	var payload RoomPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.logger.Error("Failed to unmarshal leave room payload", zap.Error(err))
		return
	}

	client := h.findClientByID(msg.SenderID)
	if client == nil {
		return
	}

	payload.Room = scopeRoom(client, payload.Room)

	h.roomsMu.Lock()
	defer h.roomsMu.Unlock()

	h.removeClientFromRoom(client, payload.Room)

	h.logger.Info("Client left room",
		zap.String("client_id", client.ID.String()),
		zap.String("room", payload.Room),
	)
}

// handleBroadcast handles broadcast messages
func (h *Hub) handleBroadcast(msg *Message) {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	if msg.Room != "" {
		h.sendToRoom(msg.Room, msg)
		return
	}

	// A client-originated broadcast without a room is confined to the sender's
	// own tenant; only a server-originated message (no tenant) reaches everyone.
	for client := range h.clients {
		if msg.TenantID != uuid.Nil && client.TenantID != msg.TenantID {
			continue
		}
		select {
		case client.send <- msg:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// handleDirect handles direct messages to specific users
func (h *Hub) handleDirect(msg *Message) {
	if msg.TargetID == uuid.Nil {
		return
	}

	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	for client := range h.clients {
		if client.UserID != msg.TargetID {
			continue
		}
		// A direct message from a client never crosses a tenant boundary.
		if msg.TenantID != uuid.Nil && client.TenantID != msg.TenantID {
			continue
		}
		select {
		case client.send <- msg:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// sendToRoom sends a message to all clients in a room
func (h *Hub) sendToRoom(room string, msg *Message) {
	clients, ok := h.rooms[room]
	if !ok {
		return
	}

	for client := range clients {
		select {
		case client.send <- msg:
		default:
			close(client.send)
			delete(clients, client)
			delete(h.clients, client)
		}
	}
}

// findClientByID finds a client by their user ID
func (h *Hub) findClientByID(userID uuid.UUID) *Client {
	for client := range h.clients {
		if client.UserID == userID {
			return client
		}
	}
	return nil
}

// removeClientFromRoom removes a client from a specific room
func (h *Hub) removeClientFromRoom(client *Client, room string) {
	if clients, ok := h.rooms[room]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.rooms, room)
		}
	}

	if rooms, ok := h.userRooms[client.UserID]; ok {
		delete(rooms, room)
		if len(rooms) == 0 {
			delete(h.userRooms, client.UserID)
		}
	}
}

// removeClientFromAllRooms removes a client from all rooms
func (h *Hub) removeClientFromAllRooms(client *Client) {
	if rooms, ok := h.userRooms[client.UserID]; ok {
		for room := range rooms {
			if clients, exists := h.rooms[room]; exists {
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.rooms, room)
				}
			}
		}
		delete(h.userRooms, client.UserID)
	}
}

// BroadcastToRoom sends a message to all clients in a room (public API)
func (h *Hub) BroadcastToRoom(room string, msgType MessageType, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal broadcast payload", zap.Error(err))
		return
	}

	msg := &Message{
		Type:      msgType,
		Room:      room,
		Payload:   data,
		Timestamp: time.Now(),
	}

	h.broadcast <- msg
}

// BroadcastToUser sends a message to a specific user (public API)
func (h *Hub) BroadcastToUser(userID uuid.UUID, msgType MessageType, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal direct payload", zap.Error(err))
		return
	}

	msg := &Message{
		Type:      msgType,
		TargetID:  userID,
		Payload:   data,
		Timestamp: time.Now(),
	}

	h.broadcast <- msg
}

// GetConnectedClients returns the number of connected clients
func (h *Hub) GetConnectedClients() int {
	return len(h.clients)
}

// GetRoomClients returns the number of clients in a room
func (h *Hub) GetRoomClients(room string) int {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	if clients, ok := h.rooms[room]; ok {
		return len(clients)
	}
	return 0
}

// GetRooms returns all active rooms
func (h *Hub) GetRooms() []string {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	rooms := make([]string, 0, len(h.rooms))
	for room := range h.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// IsUserInTenant reports whether a connected client for userID belongs to the
// given tenant. Used to keep direct messages inside a tenant boundary.
func (h *Hub) IsUserInTenant(userID, tenantID uuid.UUID) bool {
	h.roomsMu.RLock()
	defer h.roomsMu.RUnlock()

	for client := range h.clients {
		if client.UserID == userID {
			return client.TenantID == tenantID
		}
	}
	return false
}
