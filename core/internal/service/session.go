package service

// Panel sessions: establishing them, enforcing their binding, listing them and
// ending them.
//
// The panel authenticates with a stateless JWT, which has one property that is
// wrong for an administrative interface: nothing can take it back. An operator
// who realises a token has leaked, or who wants to end the session on the
// laptop they left at the office, has no way to do either - the token works
// until it expires. This service is the record that makes it possible.
//
// It also enforces the binding. The policy itself lives in
// internal/auth/sessionbinding.go, with the argument for it; this file is the
// part that knows what was recorded and writes down what happened.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// Audit vocabulary for sessions. Owned here for the same reason the API key
// names are: internal/audit belongs to another module.
const (
	AuditActionSessionEstablished = "session.established"
	AuditActionSessionRefused     = "session.refused"
	AuditActionSessionMoved       = "session.origin_changed"
	AuditActionSessionRebound     = "session.rebound"
	AuditActionSessionTerminated  = "session.terminated"
	AuditActionSessionsCleared    = "session.terminated_all"
	AuditResourceSession          = "session"
)

// purgeInterval is how often the expired-row sweep runs, at most. It is
// triggered by traffic rather than by a timer so that there is no goroutine to
// forget to start, and no second place the feature can be disconnected.
const purgeInterval = 15 * time.Minute

// Errors the handler maps onto status codes.
var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionsDisabled = errors.New("session binding is not configured on this panel")
	ErrPasswordRejected = errors.New("that password was not accepted")
)

// SessionStore is the persistence this service needs.
// repository.PanelSessionRepository implements it.
type SessionStore interface {
	Establish(ctx context.Context, session *models.PanelSession) (*models.PanelSession, error)
	GetByTokenID(ctx context.Context, tokenID string) (*models.PanelSession, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.PanelSession, error)
	Touch(ctx context.Context, id uuid.UUID, ip string, at time.Time, originMoved, reauthRequired bool) error
	Rebind(ctx context.Context, id uuid.UUID, ip, network string, at time.Time) error
	ListForUser(ctx context.Context, tenantID, userID uuid.UUID) ([]models.PanelSession, error)
	Revoke(ctx context.Context, tenantID, userID, id uuid.UUID, reason string, at time.Time) (int64, error)
	RevokeByTokenID(ctx context.Context, tokenID, reason string, at time.Time) (int64, error)
	RevokeAllForUser(ctx context.Context, tenantID, userID uuid.UUID, reason string, at time.Time) (int64, error)
	PurgeExpired(ctx context.Context, before time.Time) (int64, error)
}

// UserLookup reads the account behind a session, for the password check that
// re-binds a moved session. repository.UserRepository implements it.
type UserLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}

// SessionService binds sessions to their origin and lets operators see and end
// them.
type SessionService struct {
	store  SessionStore
	users  UserLookup
	policy auth.BindingPolicy
	audit  *AuditService
	logger *zap.Logger
	now    func() time.Time

	purgeMu   sync.Mutex
	lastPurge time.Time
}

// NewSessionService builds the service over the real repository.
func NewSessionService(store *repository.PanelSessionRepository, users UserLookup, audit *AuditService, logger *zap.Logger) *SessionService {
	var backing SessionStore
	if store != nil {
		backing = store
	}
	return NewSessionServiceWithStore(backing, users, audit, logger)
}

// NewSessionServiceWithStore is the constructor the tests use.
func NewSessionServiceWithStore(store SessionStore, users UserLookup, audit *AuditService, logger *zap.Logger) *SessionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	policy := auth.BindingPolicyFromEnv()

	logger.Info("session binding policy",
		zap.String("ip_mode", policy.IPMode),
		zap.Bool("device_binding", policy.DeviceBinding),
		zap.Bool("enforced", store != nil))
	if store == nil {
		logger.Error("session binding is NOT enforced: no session store was wired. " +
			"Sessions cannot be listed or ended, and a stolen token is usable from anywhere until it expires.")
	}

	return &SessionService{
		store:  store,
		users:  users,
		policy: policy,
		audit:  audit,
		logger: logger,
		now:    time.Now,
	}
}

// SetClock replaces the service's idea of now, for tests.
func (s *SessionService) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// SetPolicy overrides the configured binding policy. It exists so a test can
// drive each mode without setting process environment, and so an operator
// interface could offer the choice later.
func (s *SessionService) SetPolicy(policy auth.BindingPolicy) {
	s.policy = policy
}

// Policy reports the policy in force.
func (s *SessionService) Policy() auth.BindingPolicy {
	return s.policy
}

// Enforcing reports whether sessions are actually being recorded and checked.
func (s *SessionService) Enforcing() bool {
	return s != nil && s.store != nil
}

// EvaluateSession implements auth.SessionEvaluator. It is called for every
// request that carries a valid access token.
func (s *SessionService) EvaluateSession(ctx context.Context, req auth.SessionRequest) (auth.SessionVerdict, error) {
	if !s.Enforcing() {
		return auth.SessionVerdict{Allow: true}, ErrSessionsDisabled
	}
	if strings.TrimSpace(req.TokenID) == "" {
		// A token with no jti cannot be tied to a session. It is still a valid
		// token as far as the signature goes, so it is allowed through - but
		// it is worth saying out loud, because it means this control is not
		// covering that request.
		s.logger.Warn("access token carries no jti: this request cannot be bound to a session",
			zap.String("path", req.Path))
		return auth.SessionVerdict{Allow: true}, nil
	}

	now := s.now()
	s.maybePurge(ctx, now)

	fingerprint := auth.DeviceFingerprint(req.UserAgent)

	session, err := s.store.GetByTokenID(ctx, req.TokenID)
	if err != nil || session == nil {
		return s.establish(ctx, req, fingerprint, now)
	}

	if session.RevokedAt != nil {
		// This is the whole point of the table. The token is still perfectly
		// valid; the session behind it was ended.
		return auth.SessionVerdict{
			SessionID: session.ID,
			Allow:     false,
			Reason:    "revoked",
		}, nil
	}
	if !session.ExpiresAt.After(now) {
		return auth.SessionVerdict{SessionID: session.ID, Allow: false, Reason: "expired"}, nil
	}

	decision := s.policy.Evaluate(
		auth.Binding{
			IP:          session.OriginIP,
			Network:     session.OriginNetwork,
			Fingerprint: session.DeviceFingerprint,
		},
		auth.Observation{
			IP:          req.IP,
			Fingerprint: fingerprint,
			Method:      req.Method,
		},
	)

	// A session already carrying the re-authentication flag keeps it until the
	// password is proved, even if the request happens to arrive from the bound
	// network again: the flag records that something moved, and clearing it on
	// a lucky routing change would make it meaningless.
	if session.ReauthRequired && decision.Verdict == auth.VerdictAllow &&
		auth.ActionForMethod(req.Method) != auth.ActionRead {
		decision = auth.Decision{Verdict: auth.VerdictReauthenticate, Reason: "network_changed"}
	}

	switch decision.Verdict {
	case auth.VerdictRefuse:
		if _, err := s.store.RevokeByTokenID(ctx, req.TokenID, decision.Reason, now); err != nil {
			s.logger.Error("failed to end a session the binding policy refused",
				zap.String("session_id", session.ID.String()), zap.Error(err))
		}
		s.logger.Warn("session refused and ended",
			zap.String("session_id", session.ID.String()),
			zap.String("reason", decision.Reason),
			zap.String("ip", req.IP),
			zap.String("path", req.Path))
		s.record(ctx, session.TenantID, session.UserID, AuditActionSessionRefused, session.ID, models.JSONMap{
			"reason":     decision.Reason,
			"origin_ip":  session.OriginIP,
			"request_ip": req.IP,
			"path":       req.Path,
		}, req.IP, req.UserAgent, auditStatusFailure)
		return auth.SessionVerdict{SessionID: session.ID, Allow: false, Reason: decision.Reason}, nil

	case auth.VerdictReauthenticate:
		if err := s.store.Touch(ctx, session.ID, req.IP, now, decision.OriginMoved, true); err != nil {
			s.logger.Warn("failed to record a session origin change", zap.Error(err))
		}
		if decision.OriginMoved {
			s.record(ctx, session.TenantID, session.UserID, AuditActionSessionMoved, session.ID, models.JSONMap{
				"reason":     decision.Reason,
				"origin_ip":  session.OriginIP,
				"request_ip": req.IP,
				"outcome":    "reauthentication_required",
				"path":       req.Path,
			}, req.IP, req.UserAgent, auditStatusFailure)
		}
		return auth.SessionVerdict{
			SessionID:      session.ID,
			Allow:          false,
			ReauthRequired: true,
			Reason:         decision.Reason,
		}, nil

	case auth.VerdictAllowChanged:
		if err := s.store.Touch(ctx, session.ID, req.IP, now, decision.OriginMoved, false); err != nil {
			s.logger.Warn("failed to record a session origin change", zap.Error(err))
		}
		s.record(ctx, session.TenantID, session.UserID, AuditActionSessionMoved, session.ID, models.JSONMap{
			"reason":     decision.Reason,
			"origin_ip":  session.OriginIP,
			"request_ip": req.IP,
			"outcome":    "allowed",
			"path":       req.Path,
		}, req.IP, req.UserAgent, auditStatusSuccess)
		return auth.SessionVerdict{SessionID: session.ID, Allow: true, Reason: decision.Reason}, nil

	default:
		if err := s.store.Touch(ctx, session.ID, req.IP, now, false, false); err != nil {
			s.logger.Warn("failed to record session activity", zap.Error(err))
		}
		return auth.SessionVerdict{SessionID: session.ID, Allow: true}, nil
	}
}

// establish records a session the first time its token is seen.
//
// The binding is taken from this request. The alternative - recording it in
// the login handler - would close a gap of one round trip between the token
// being minted and first used, and would put this feature's establishment
// inside a file that belongs to the authentication change; see the report. The
// gap is the interval in which the legitimate client has not yet made its
// first call, and a token stolen inside it is a token stolen from the response
// to the login request itself.
func (s *SessionService) establish(ctx context.Context, req auth.SessionRequest, fingerprint string, now time.Time) (auth.SessionVerdict, error) {
	expiresAt := req.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(24 * time.Hour)
	}

	candidate := &models.PanelSession{
		ID:                uuid.New(),
		TokenID:           req.TokenID,
		UserID:            req.UserID,
		TenantID:          req.TenantID,
		OriginIP:          req.IP,
		OriginNetwork:     auth.NetworkOf(req.IP),
		DeviceFingerprint: fingerprint,
		UserAgent:         req.UserAgent,
		LastSeenAt:        now,
		ExpiresAt:         expiresAt,
	}

	stored, err := s.store.Establish(ctx, candidate)
	if err != nil {
		// A session that cannot be recorded cannot be enforced, and refusing
		// the request would turn a database blip into a panel-wide outage.
		// The request proceeds and the failure is loud.
		s.logger.Error("failed to establish a session record; this request is not bound to a session",
			zap.String("user_id", req.UserID.String()), zap.Error(err))
		return auth.SessionVerdict{Allow: true}, err
	}

	// A revoked row for a token being seen "for the first time" means the
	// session was ended and the same token came back. Refuse it.
	if stored.RevokedAt != nil {
		return auth.SessionVerdict{SessionID: stored.ID, Allow: false, Reason: "revoked"}, nil
	}

	s.logger.Info("session established",
		zap.String("session_id", stored.ID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.String("origin_ip", stored.OriginIP),
		zap.String("origin_network", stored.OriginNetwork))

	s.record(ctx, req.TenantID, req.UserID, AuditActionSessionEstablished, stored.ID, models.JSONMap{
		"origin_ip":      stored.OriginIP,
		"origin_network": stored.OriginNetwork,
		"user_agent":     stored.UserAgent,
	}, req.IP, req.UserAgent, auditStatusSuccess)

	return auth.SessionVerdict{SessionID: stored.ID, Allow: true, Established: true}, nil
}

// List returns the caller's live sessions, marking the one they are using.
func (s *SessionService) List(ctx context.Context, tenantID, userID uuid.UUID, currentTokenID string) ([]models.SessionView, error) {
	if !s.Enforcing() {
		return nil, ErrSessionsDisabled
	}
	sessions, err := s.store.ListForUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	views := make([]models.SessionView, 0, len(sessions))
	for i := range sessions {
		views = append(views, sessions[i].View(currentTokenID))
	}
	return views, nil
}

// Terminate ends one of the caller's own sessions, including the one the
// request is being made with.
//
// Ending the current session is allowed on purpose: "sign me out everywhere,
// including here" is the thing an operator wants when they think something is
// wrong, and a control that refuses to end the session you are holding is a
// control you cannot use in the situation it exists for. The caller is told
// which case it was so the interface can send them to the sign-in page.
func (s *SessionService) Terminate(ctx context.Context, tenantID, userID, sessionID uuid.UUID, currentTokenID, ip, userAgent string) (self bool, err error) {
	if !s.Enforcing() {
		return false, ErrSessionsDisabled
	}

	session, err := s.store.GetByID(ctx, tenantID, sessionID)
	if err != nil || session == nil {
		return false, ErrSessionNotFound
	}
	if session.UserID != userID {
		// Not the caller's session. Report it as missing rather than as
		// forbidden: the caller has no business knowing it exists.
		return false, ErrSessionNotFound
	}

	self = currentTokenID != "" && session.TokenID == currentTokenID

	changed, err := s.store.Revoke(ctx, tenantID, userID, sessionID, "terminated_by_user", s.now())
	if err != nil {
		return self, err
	}
	if changed == 0 {
		return self, ErrSessionNotFound
	}

	s.logger.Info("session terminated",
		zap.String("session_id", sessionID.String()),
		zap.String("user_id", userID.String()),
		zap.Bool("current_session", self))

	s.record(ctx, tenantID, userID, AuditActionSessionTerminated, sessionID, models.JSONMap{
		"origin_ip":       session.OriginIP,
		"user_agent":      session.UserAgent,
		"current_session": self,
	}, ip, userAgent, auditStatusSuccess)

	return self, nil
}

// TerminateAllForUser ends every live session of an account.
//
// It is what an administrator does to a compromised account, and it is what a
// password change should call - see the report: the call belongs in whichever
// change owns the password path, and it is one line.
func (s *SessionService) TerminateAllForUser(ctx context.Context, tenantID, targetUserID, actorID uuid.UUID, reason, ip, userAgent string) (int64, error) {
	if !s.Enforcing() {
		return 0, ErrSessionsDisabled
	}
	if strings.TrimSpace(reason) == "" {
		reason = "terminated_by_administrator"
	}

	count, err := s.store.RevokeAllForUser(ctx, tenantID, targetUserID, reason, s.now())
	if err != nil {
		return 0, err
	}

	s.logger.Info("all sessions terminated for an account",
		zap.String("user_id", targetUserID.String()),
		zap.Int64("sessions", count),
		zap.String("reason", reason))

	s.record(ctx, tenantID, actorID, AuditActionSessionsCleared, targetUserID, models.JSONMap{
		"target_user": targetUserID.String(),
		"sessions":    count,
		"reason":      reason,
	}, ip, userAgent, auditStatusSuccess)

	return count, nil
}

// Reauthenticate proves the account password and re-binds the current session
// to where it is being used from now.
//
// This is what keeps the network half of the policy usable. Without it, an
// operator whose address changed would have to sign in again - which is
// exactly the "logs real users out constantly" failure the policy is designed
// to avoid - and the session they were using would be replaced rather than
// repaired.
func (s *SessionService) Reauthenticate(ctx context.Context, tenantID, userID uuid.UUID, currentTokenID, password, ip, userAgent string) error {
	if !s.Enforcing() {
		return ErrSessionsDisabled
	}
	if s.users == nil {
		return ErrSessionsDisabled
	}

	session, err := s.store.GetByTokenID(ctx, currentTokenID)
	if err != nil || session == nil {
		return ErrSessionNotFound
	}
	if session.UserID != userID || session.RevokedAt != nil {
		return ErrSessionNotFound
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil || user == nil {
		return ErrPasswordRejected
	}
	if !utils.CheckPassword(password, user.PasswordHash) {
		s.record(ctx, tenantID, userID, AuditActionSessionRebound, session.ID, models.JSONMap{
			"request_ip": ip,
			"outcome":    "password_rejected",
		}, ip, userAgent, auditStatusFailure)
		return ErrPasswordRejected
	}

	now := s.now()
	if err := s.store.Rebind(ctx, session.ID, ip, auth.NetworkOf(ip), now); err != nil {
		return err
	}

	s.logger.Info("session re-bound after a proven password",
		zap.String("session_id", session.ID.String()),
		zap.String("previous_origin", session.OriginIP),
		zap.String("new_origin", ip))

	s.record(ctx, tenantID, userID, AuditActionSessionRebound, session.ID, models.JSONMap{
		"previous_origin": session.OriginIP,
		"new_origin":      ip,
		"outcome":         "rebound",
	}, ip, userAgent, auditStatusSuccess)

	return nil
}

// maybePurge removes expired rows, at most once every purgeInterval.
//
// It hangs off traffic rather than a background goroutine so that there is no
// second thing to wire and nothing to forget to start.
func (s *SessionService) maybePurge(ctx context.Context, now time.Time) {
	s.purgeMu.Lock()
	if !s.lastPurge.IsZero() && now.Sub(s.lastPurge) < purgeInterval {
		s.purgeMu.Unlock()
		return
	}
	s.lastPurge = now
	s.purgeMu.Unlock()

	removed, err := s.store.PurgeExpired(ctx, now)
	if err != nil {
		s.logger.Warn("failed to purge expired session records", zap.Error(err))
		return
	}
	if removed > 0 {
		s.logger.Debug("purged expired session records", zap.Int64("rows", removed))
	}
}

func (s *SessionService) record(ctx context.Context, tenantID, actorID uuid.UUID, action string, resourceID uuid.UUID, details models.JSONMap, ip, userAgent, status string) {
	if s.audit == nil {
		return
	}
	var actor *uuid.UUID
	if actorID != uuid.Nil {
		id := actorID
		actor = &id
	}
	target := resourceID
	s.audit.Record(ctx, tenantID, actor, action, AuditResourceSession, &target, details, ip, userAgent, status)
}
