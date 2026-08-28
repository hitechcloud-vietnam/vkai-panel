package repository

// Panel session storage.
//
// One row per access token, keyed by the token's jti. The row is what makes a
// stateless JWT revocable: ending a session writes revoked_at, and the next
// request carrying that token is refused.
//
// Two rules this file exists to enforce:
//
//   - Establishment is an upsert on token_id, which is UNIQUE. Two requests
//     arriving at once with a brand new token must produce one session, not
//     two, and must not fail either request.
//
//   - Ending a session never deletes the row. Deleting it would let the very
//     next request carrying the same token establish a fresh session and undo
//     the revocation. Rows are removed only once the token they belong to has
//     expired, by PurgeExpired.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

const panelSessionColumns = `id, token_id, user_id, tenant_id, origin_ip, origin_network,
	device_fingerprint, user_agent, last_seen_ip, last_seen_at, origin_changes,
	reauth_required, revoked_at, revoked_reason, created_at, expires_at`

// PanelSessionRepository stores the bindings of live sessions.
type PanelSessionRepository struct {
	db *sqlx.DB
}

// NewPanelSessionRepository creates a panel session repository.
func NewPanelSessionRepository(db *sqlx.DB) *PanelSessionRepository {
	return &PanelSessionRepository{db: db}
}

// Establish returns the session for a token, creating it from the given values
// if this is the first time the token has been seen.
//
// The insert is conditional and the read is unconditional, so a caller always
// gets back the row that is actually stored - which for a second concurrent
// request is the row the first one wrote, not the values this caller proposed.
func (r *PanelSessionRepository) Establish(ctx context.Context, session *models.PanelSession) (*models.PanelSession, error) {
	insert := `
		INSERT INTO panel_sessions (
			id, token_id, user_id, tenant_id, origin_ip, origin_network,
			device_fingerprint, user_agent, last_seen_ip, last_seen_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (token_id) DO NOTHING
	`
	if _, err := r.db.ExecContext(ctx, insert,
		session.ID,
		session.TokenID,
		session.UserID,
		session.TenantID,
		session.OriginIP,
		session.OriginNetwork,
		session.DeviceFingerprint,
		session.UserAgent,
		session.OriginIP,
		session.LastSeenAt,
		session.ExpiresAt,
	); err != nil {
		return nil, err
	}

	return r.GetByTokenID(ctx, session.TokenID)
}

// GetByTokenID reads the session belonging to an access token.
func (r *PanelSessionRepository) GetByTokenID(ctx context.Context, tokenID string) (*models.PanelSession, error) {
	var session models.PanelSession
	query := `SELECT ` + panelSessionColumns + ` FROM panel_sessions WHERE token_id = $1`
	if err := r.db.GetContext(ctx, &session, query, tokenID); err != nil {
		return nil, err
	}
	return &session, nil
}

// GetByID reads a session by its own identifier, tenant scoped.
func (r *PanelSessionRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.PanelSession, error) {
	var session models.PanelSession
	query := `SELECT ` + panelSessionColumns + ` FROM panel_sessions WHERE tenant_id = $1 AND id = $2`
	if err := r.db.GetContext(ctx, &session, query, tenantID, id); err != nil {
		return nil, err
	}
	return &session, nil
}

// Touch records that the session was used, from where, and what the binding
// policy made of it.
func (r *PanelSessionRepository) Touch(ctx context.Context, id uuid.UUID, ip string, at time.Time, originMoved, reauthRequired bool) error {
	query := `
		UPDATE panel_sessions
		SET last_seen_ip = NULLIF($1, ''),
		    last_seen_at = $2,
		    origin_changes = origin_changes + $3,
		    reauth_required = reauth_required OR $4
		WHERE id = $5
	`
	moved := 0
	if originMoved {
		moved = 1
	}
	_, err := r.db.ExecContext(ctx, query, ip, at, moved, reauthRequired, id)
	return err
}

// Rebind moves a session's binding to a new origin and clears the
// re-authentication flag. It is what proving the password buys.
func (r *PanelSessionRepository) Rebind(ctx context.Context, id uuid.UUID, ip, network string, at time.Time) error {
	// The address is passed twice rather than reused as one placeholder.
	// PostgreSQL deduces a parameter's type from how it is used, and one
	// placeholder used both as a varchar(45) column value and as an argument
	// to NULLIF is rejected outright:
	//
	//	ERROR: inconsistent types deduced for parameter $1 (SQLSTATE 42P08)
	query := `
		UPDATE panel_sessions
		SET origin_ip = $1, origin_network = $2, last_seen_ip = NULLIF($3, ''),
		    last_seen_at = $4, reauth_required = FALSE
		WHERE id = $5 AND revoked_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query, ip, network, ip, at, id)
	return err
}

// ListForUser returns a user's live sessions, newest first.
func (r *PanelSessionRepository) ListForUser(ctx context.Context, tenantID, userID uuid.UUID) ([]models.PanelSession, error) {
	var sessions []models.PanelSession
	query := `SELECT ` + panelSessionColumns + `
		FROM panel_sessions
		WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &sessions, query, tenantID, userID); err != nil {
		return nil, err
	}
	return sessions, nil
}

// Revoke ends one session belonging to a given user. The user_id condition is
// what stops one operator ending another's session through the self-service
// endpoint.
func (r *PanelSessionRepository) Revoke(ctx context.Context, tenantID, userID, id uuid.UUID, reason string, at time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE panel_sessions
		SET revoked_at = $1, revoked_reason = NULLIF($2, '')
		WHERE tenant_id = $3 AND user_id = $4 AND id = $5 AND revoked_at IS NULL`,
		at, reason, tenantID, userID, id,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// RevokeByTokenID ends the session a particular token belongs to. It is what
// the binding policy calls when it refuses a request outright.
func (r *PanelSessionRepository) RevokeByTokenID(ctx context.Context, tokenID, reason string, at time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE panel_sessions
		SET revoked_at = $1, revoked_reason = NULLIF($2, '')
		WHERE token_id = $3 AND revoked_at IS NULL`,
		at, reason, tokenID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// RevokeAllForUser ends every live session of an account. It is what an
// administrator uses on a compromised account, and what a password change
// should call.
func (r *PanelSessionRepository) RevokeAllForUser(ctx context.Context, tenantID, userID uuid.UUID, reason string, at time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE panel_sessions
		SET revoked_at = $1, revoked_reason = NULLIF($2, '')
		WHERE tenant_id = $3 AND user_id = $4 AND revoked_at IS NULL`,
		at, reason, tenantID, userID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PurgeExpired removes rows whose token has expired. A revoked row must stay
// until then: while the token could still be presented, the row is the only
// thing that refuses it.
func (r *PanelSessionRepository) PurgeExpired(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM panel_sessions WHERE expires_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
