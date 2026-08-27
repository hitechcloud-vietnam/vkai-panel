package websocket

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	// Connection messages
	MsgTypeConnect    MessageType = "connect"
	MsgTypeDisconnect MessageType = "disconnect"
	MsgTypeError      MessageType = "error"
	MsgTypePing       MessageType = "ping"
	MsgTypePong       MessageType = "pong"

	// Room messages
	MsgTypeJoinRoom         MessageType = "join_room"
	MsgTypeLeaveRoom        MessageType = "leave_room"
	MsgTypeRoomNotification MessageType = "room_notification"

	// Communication messages
	MsgTypeBroadcast MessageType = "broadcast"
	MsgTypeDirect    MessageType = "direct"
	MsgTypeChat      MessageType = "chat"

	// System messages
	MsgTypeServerStatus   MessageType = "server_status"
	MsgTypeMetricUpdate   MessageType = "metric_update"
	MsgTypeAlertFired     MessageType = "alert_fired"
	MsgTypeAlertResolved  MessageType = "alert_resolved"
	MsgTypeBackupStatus   MessageType = "backup_status"
	MsgTypeDeployStatus   MessageType = "deploy_status"
	MsgTypeServiceStatus  MessageType = "service_status"
	MsgTypeLogEntry       MessageType = "log_entry"
	MsgTypeNotification   MessageType = "notification"
	MsgTypeClusterEvent   MessageType = "cluster_event"
	MsgTypeNodeHeartbeat  MessageType = "node_heartbeat"
	MsgTypeJobStatus      MessageType = "job_status"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType     `json:"type"`
	Room      string          `json:"room,omitempty"`
	SenderID  uuid.UUID       `json:"sender_id,omitempty"`
	TargetID  uuid.UUID       `json:"target_id,omitempty"`
	TenantID  uuid.UUID       `json:"tenant_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// RoomPayload represents a room join/leave request
type RoomPayload struct {
	Room string `json:"room"`
}

// ErrorPayload represents an error message
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServerStatusPayload represents server status update
type ServerStatusPayload struct {
	ServerID uuid.UUID `json:"server_id"`
	Hostname string    `json:"hostname"`
	Status   string    `json:"status"`
	CPU      float64   `json:"cpu"`
	Memory   float64   `json:"memory"`
	Disk     float64   `json:"disk"`
	Load     float64   `json:"load"`
}

// MetricUpdatePayload represents a metric update
type MetricUpdatePayload struct {
	ServerID   uuid.UUID `json:"server_id"`
	MetricType string    `json:"metric_type"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
}

// AlertPayload represents an alert event
type AlertPayload struct {
	AlertID    uuid.UUID `json:"alert_id"`
	ServerID   uuid.UUID `json:"server_id"`
	Name       string    `json:"name"`
	Severity   string    `json:"severity"`
	Message    string    `json:"message"`
	MetricType string    `json:"metric_type"`
	Value      float64   `json:"value"`
	Threshold  float64   `json:"threshold"`
}

// BackupStatusPayload represents backup status update
type BackupStatusPayload struct {
	BackupID uuid.UUID `json:"backup_id"`
	Status   string    `json:"status"`
	Progress int       `json:"progress"`
	Message  string    `json:"message,omitempty"`
}

// DeployStatusPayload represents deployment status update
type DeployStatusPayload struct {
	DeploymentID uuid.UUID `json:"deployment_id"`
	Status       string    `json:"status"`
	Progress     int       `json:"progress"`
	Message      string    `json:"message,omitempty"`
	CommitHash   string    `json:"commit_hash,omitempty"`
}

// ServiceStatusPayload represents service status update
type ServiceStatusPayload struct {
	ServiceName string `json:"service_name"`
	Status      string `json:"status"`
	Action      string `json:"action"` // start, stop, restart
	Message     string `json:"message,omitempty"`
}

// LogEntryPayload represents a new log entry
type LogEntryPayload struct {
	EntryID    uuid.UUID `json:"entry_id"`
	Level      string    `json:"level"`
	Source     string    `json:"source"`
	Message    string    `json:"message"`
	ServerID   uuid.UUID `json:"server_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// NotificationPayload represents a notification
type NotificationPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
	Type           string    `json:"type"`
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	Channel        string    `json:"channel"`
}

// ClusterEventPayload represents a cluster event
type ClusterEventPayload struct {
	ClusterID uuid.UUID `json:"cluster_id"`
	EventType string    `json:"event_type"` // node_added, node_removed, failover, etc.
	NodeID    uuid.UUID `json:"node_id,omitempty"`
	Message   string    `json:"message"`
}

// NodeHeartbeatPayload represents a node heartbeat
type NodeHeartbeatPayload struct {
	NodeID    uuid.UUID `json:"node_id"`
	ClusterID uuid.UUID `json:"cluster_id"`
	Status    string    `json:"status"`
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
	Disk      float64   `json:"disk"`
}

// JobStatusPayload represents job status update
type JobStatusPayload struct {
	JobID    uuid.UUID `json:"job_id"`
	JobType  string    `json:"job_type"`
	Status   string    `json:"status"`
	Progress int       `json:"progress"`
	Message  string    `json:"message,omitempty"`
}
