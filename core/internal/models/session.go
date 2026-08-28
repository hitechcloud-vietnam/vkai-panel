package models

import (
	"time"

	"github.com/google/uuid"
)

// PanelSession is one row of panel_sessions: an access token, the origin it
// was issued to, and whether it is still allowed to be used.
//
// It is not models.UserSession. That type maps user_sessions, which the
// multi-user module writes for its activity screen and reads with `SELECT *`.
// This one is consulted on every authenticated request and decides whether the
// request happens at all, which is why it is keyed by the access token's jti
// and carries revocation state.
type PanelSession struct {
	ID uuid.UUID `json:"id" db:"id"`

	// TokenID is the jti of the access token. It is not the token and cannot
	// be turned back into one.
	TokenID string `json:"-" db:"token_id"`

	UserID   uuid.UUID `json:"user_id" db:"user_id"`
	TenantID uuid.UUID `json:"tenant_id" db:"tenant_id"`

	// OriginIP and OriginNetwork are what the session was bound to when it was
	// established. OriginNetwork is the /24 or /48 containing OriginIP.
	OriginIP      string `json:"origin_ip" db:"origin_ip"`
	OriginNetwork string `json:"origin_network" db:"origin_network"`

	// DeviceFingerprint is the SHA-256 of the normalised User-Agent, which is
	// what the comparison uses. UserAgent is the raw header, shown to the
	// operator so a row in the session list is recognisable.
	DeviceFingerprint string `json:"-" db:"device_fingerprint"`
	UserAgent         string `json:"user_agent" db:"user_agent"`

	LastSeenIP *string   `json:"last_seen_ip,omitempty" db:"last_seen_ip"`
	LastSeenAt time.Time `json:"last_seen_at" db:"last_seen_at"`

	// OriginChanges counts how many times this session has been used from
	// outside its bound network.
	OriginChanges int `json:"origin_changes" db:"origin_changes"`

	// ReauthRequired is set when the session moved far enough that the next
	// state-changing request has to wait for the password.
	ReauthRequired bool `json:"reauth_required" db:"reauth_required"`

	RevokedAt     *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	RevokedReason *string    `json:"revoked_reason,omitempty" db:"revoked_reason"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

// Active reports whether the session may still be used at the given instant.
func (s *PanelSession) Active(now time.Time) bool {
	if s == nil {
		return false
	}
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

// SessionView is what an operator sees in their "signed in devices" list. It
// never carries the token id or the fingerprint, only what a person can use to
// recognise a session and decide to end it.
type SessionView struct {
	ID             uuid.UUID  `json:"id"`
	OriginIP       string     `json:"origin_ip"`
	OriginNetwork  string     `json:"origin_network"`
	LastSeenIP     string     `json:"last_seen_ip,omitempty"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	UserAgent      string     `json:"user_agent"`
	OriginChanges  int        `json:"origin_changes"`
	ReauthRequired bool       `json:"reauth_required"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`

	// Current marks the session the request asking for the list is using, so
	// the interface can label it and warn before ending it.
	Current bool `json:"current"`
}

// View converts a session row into what is safe to show.
func (s *PanelSession) View(currentTokenID string) SessionView {
	view := SessionView{
		ID:             s.ID,
		OriginIP:       s.OriginIP,
		OriginNetwork:  s.OriginNetwork,
		LastSeenAt:     s.LastSeenAt,
		UserAgent:      s.UserAgent,
		OriginChanges:  s.OriginChanges,
		ReauthRequired: s.ReauthRequired,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
		RevokedAt:      s.RevokedAt,
		Current:        currentTokenID != "" && s.TokenID == currentTokenID,
	}
	if s.LastSeenIP != nil {
		view.LastSeenIP = *s.LastSeenIP
	}
	return view
}

// ReauthenticateRequest proves the password for the session making the
// request, so a session that moved network can carry on rather than having to
// be replaced by a new sign-in.
type ReauthenticateRequest struct {
	Password string `json:"password" binding:"required"`
}
