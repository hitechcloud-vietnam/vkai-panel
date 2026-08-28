package notify

import (
	"time"

	"github.com/google/uuid"
)

// Delivery statuses as stored in notification_deliveries.status.
const (
	StatusPending    = "pending"
	StatusSending    = "sending"
	StatusSent       = "sent"
	StatusDeadLetter = "dead_letter"
)

// DeliveryRecord is one outbox row as an operator sees it over the API.
//
// It carries the channel's name and type but never its config: this struct is
// serialised straight into a response, and a struct that could hold a bot
// token is one refactor away from returning one.
type DeliveryRecord struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ChannelID   uuid.UUID `json:"channel_id" db:"channel_id"`
	ChannelName string    `json:"channel_name" db:"channel_name"`
	ChannelType string    `json:"channel_type" db:"channel_type"`

	DedupKey  string    `json:"dedup_key" db:"dedup_key"`
	EventKind EventKind `json:"event_kind" db:"event_kind"`
	Subject   string    `json:"subject" db:"subject"`
	Body      string    `json:"body" db:"body"`

	Status        string    `json:"status" db:"status"`
	Attempts      int       `json:"attempts" db:"attempts"`
	MaxAttempts   int       `json:"max_attempts" db:"max_attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at" db:"next_attempt_at"`

	// LastError is already scrubbed when it is written. It is returned to the
	// API because an operator debugging a dead channel needs to read it.
	LastError string `json:"last_error" db:"last_error"`

	SentAt         *time.Time `json:"sent_at" db:"sent_at"`
	DeadLetteredAt *time.Time `json:"dead_lettered_at" db:"dead_lettered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// DeliveryFilter narrows a listing.
type DeliveryFilter struct {
	Status    string
	ChannelID *uuid.UUID
	DedupKey  string
	Limit     int
	Offset    int
}

// AlertStateRecord is one row of notification_alert_state as the API shows it.
// It answers "why did I not get a message about this" without a database
// client.
type AlertStateRecord struct {
	TenantID           uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	DedupKey           string     `json:"dedup_key" db:"dedup_key"`
	State              string     `json:"state" db:"state"`
	FirstSeenAt        time.Time  `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt         time.Time  `json:"last_seen_at" db:"last_seen_at"`
	LastNotifiedAt     *time.Time `json:"last_notified_at" db:"last_notified_at"`
	Occurrences        int        `json:"occurrences" db:"occurrences"`
	QuietPeriodSeconds int        `json:"quiet_period_seconds" db:"quiet_period_seconds"`
	LastValue          *float64   `json:"last_value" db:"last_value"`
	Threshold          *float64   `json:"threshold" db:"threshold"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}

// EnqueueRequest is one message to be written to the outbox.
type EnqueueRequest struct {
	TenantID    uuid.UUID
	ChannelID   uuid.UUID
	DedupKey    string
	Kind        EventKind
	Subject     string
	Body        string
	Alert       Alert
	MaxAttempts int
}
