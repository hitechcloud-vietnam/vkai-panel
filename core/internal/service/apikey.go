package service

// API keys: minting, scoping, rotation, revocation and authentication.
//
// What an API key is, in this panel, and what changed:
//
//   - It is a bearer secret with a long life, no second factor, and a home on
//     a machine the panel does not administer. That makes it the most
//     attractive credential the panel issues, and until now it was also the
//     bluntest: a key carried whatever its owner could do, forever, and
//     nothing in the running panel ever accepted one.
//
//   - A key now carries scopes (internal/auth/scope.go), and authority is the
//     intersection of those scopes with the RBAC permissions of the user the
//     key belongs to. A key cannot do what its owner cannot do, and it cannot
//     do what it was not scoped for. Both halves are checked on every request.
//
//   - A key can be replaced without a flag day. Rotation mints the replacement
//     and gives the key being replaced a deadline; both authenticate until
//     then. Rotation that requires every consumer to change in the same second
//     does not get done, and a key that is never rotated is a key that outlives
//     the person who created it.
//
//   - Revocation takes effect on the next request, not at the next expiry.
//
//   - Only a digest is stored, and it is an HMAC under a server-side pepper.
//     The key itself exists in the response that created it and nowhere else.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// Audit vocabulary for API keys. These names are owned here, the way
// twofactor.Action* and the upgrade service own theirs, because internal/audit
// is another module's file.
const (
	AuditActionAPIKeyCreated   = "api_key.created"
	AuditActionAPIKeyUpdated   = "api_key.updated"
	AuditActionAPIKeyRotated   = "api_key.rotated"
	AuditActionAPIKeyRevoked   = "api_key.revoked"
	AuditActionAPIKeyDeleted   = "api_key.deleted"
	AuditActionAPIKeyRefused   = "api_key.refused"
	AuditResourceAPIKey        = "api_key"
	auditStatusSuccess         = "success"
	auditStatusFailure         = "failure"
	auditActionAPIKeyPlaintext = "api_key.plaintext_digest_refused"
)

// Lifetimes. A key with no expiry outlives the integration it was made for and
// the person who made it, so there is no such thing here: an omitted expiry
// becomes the default.
const (
	// DefaultAPIKeyLifetime is what a key gets when the request does not say.
	DefaultAPIKeyLifetime = 90 * 24 * time.Hour
	// MaxAPIKeyLifetime is the longest life an operator may ask for.
	MaxAPIKeyLifetime = 730 * 24 * time.Hour
	// DefaultRotationOverlap is how long the old key keeps working when a
	// replacement is minted and the request does not say.
	DefaultRotationOverlap = 24 * time.Hour
	// MaxRotationOverlap caps the window. An overlap is a period in which a
	// key the operator has decided to retire still works; a month is generous
	// and a year is a second live key.
	MaxRotationOverlap = 30 * 24 * time.Hour
)

// Errors. Authentication failures all collapse into ErrAPIKeyInvalid: the
// caller is never told whether the key is unknown, expired, revoked or simply
// being used from the wrong address, because each of those is an answer an
// attacker can use.
var (
	ErrAPIKeyInvalid     = errors.New("invalid API key")
	ErrAPIKeyNotFound    = errors.New("API key not found")
	ErrAPIKeyUnavailable = errors.New("API key management is unavailable: " +
		"the panel master key (VKAI_SECRET_KEY) is missing or unreadable")

	// ErrAPIKeyStorage stands in for whatever the database actually said. The
	// real error is logged; it is not returned, because a caller who can make
	// the panel print its own SQL errors has been handed a map of the schema.
	ErrAPIKeyStorage = errors.New("the API key store could not be reached")
)

// APIKeyStore is the persistence this service needs. The concrete
// implementation is repository.APIKeyRepository; the interface exists so the
// behaviour above can be driven end to end in a test without a database, and
// the SQL is proved separately against a real PostgreSQL.
type APIKeyStore interface {
	Create(ctx context.Context, key *models.APIKey) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.APIKey, error)
	ListByPrefixes(ctx context.Context, prefixes []string) ([]models.APIKey, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.APIKey, int, error)
	Update(ctx context.Context, key *models.APIKey) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	MarkUsed(ctx context.Context, id uuid.UUID, at time.Time, ip string) error
	UpgradeHash(ctx context.Context, id uuid.UUID, oldHash, newHash string) error
	Revoke(ctx context.Context, tenantID, id uuid.UUID, reason string, at time.Time) (int64, error)
	MarkSuperseded(ctx context.Context, tenantID, id uuid.UUID, deadline time.Time) (int64, error)
}

// AuthorityLoader reads the RBAC authority of the user a key belongs to.
// repository.UserRepository implements it.
type AuthorityLoader interface {
	GetRoleNames(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetPermissionNames(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// APIKeyService handles API key business logic
type APIKeyService struct {
	repo      APIKeyStore
	authority AuthorityLoader
	hasher    *auth.KeyHasher
	audit     *AuditService
	logger    *zap.Logger
	now       func() time.Time
}

// NewAPIKeyService creates a new API key service.
//
// The hasher is built from the panel master key. Without it the service is
// constructed anyway and every operation answers ErrAPIKeyUnavailable, which
// the handler turns into a 503 naming the cause. That is deliberate: a panel
// that silently fell back to an unpeppered digest would keep working and stop
// providing what it says it provides, and nothing would say so.
func NewAPIKeyService(repo *repository.APIKeyRepository, authority AuthorityLoader, audit *AuditService, logger *zap.Logger) *APIKeyService {
	var store APIKeyStore
	if repo != nil {
		store = repo
	}
	return NewAPIKeyServiceWithStore(store, authority, audit, logger)
}

// NewAPIKeyServiceWithStore is the constructor the tests use, and the one the
// production constructor ends in.
func NewAPIKeyServiceWithStore(repo APIKeyStore, authority AuthorityLoader, audit *AuditService, logger *zap.Logger) *APIKeyService {
	if logger == nil {
		logger = zap.NewNop()
	}

	hasher, err := auth.NewKeyHasherFromEnv()
	if err != nil {
		logger.Error("API keys are unavailable: they cannot be minted or verified without the panel master key",
			zap.Error(err))
		hasher = nil
	}

	return &APIKeyService{
		repo:      repo,
		authority: authority,
		hasher:    hasher,
		audit:     audit,
		logger:    logger,
		now:       time.Now,
	}
}

// SetClock replaces the service's idea of now. Tests use it to walk a rotation
// overlap without sleeping through it.
func (s *APIKeyService) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Available reports whether the service can mint and verify keys.
func (s *APIKeyService) Available() bool {
	return s != nil && s.hasher != nil && s.repo != nil
}

// APIKeyWithPlaintext holds an API key with its plaintext value (returned only once)
type APIKeyWithPlaintext struct {
	*models.APIKey
	// Key is the only time this value exists outside the caller's memory. It
	// is not stored and cannot be recovered.
	Key string `json:"key"`
	// ReplacedKeyID and ReplacedKeyValidUntil are set when this key was minted
	// to replace another one, so the operator can see the window they have.
	ReplacedKeyID         *uuid.UUID `json:"replaced_key_id,omitempty"`
	ReplacedKeyValidUntil *time.Time `json:"replaced_key_valid_until,omitempty"`
}

// APIKeyPrincipal is what a valid key resolves to: the key, its grant, and the
// authority of the account behind it.
type APIKeyPrincipal struct {
	KeyID       uuid.UUID
	KeyName     string
	UserID      uuid.UUID
	TenantID    uuid.UUID
	Scopes      auth.ScopeSet
	RoleIDs     []string
	Permissions []string
}

// CreateAPIKey mints a key, stores its digest, and returns the key once.
func (s *APIKeyService) CreateAPIKey(ctx context.Context, tenantID, userID uuid.UUID, req *models.CreateAPIKeyRequest) (*APIKeyWithPlaintext, error) {
	if !s.Available() {
		return nil, ErrAPIKeyUnavailable
	}
	if req == nil {
		return nil, errors.New("a request is required")
	}

	scopes, err := auth.ParseScopeSet(req.Scopes)
	if err != nil {
		return nil, err
	}

	cidrs, err := auth.ValidateCIDRs(req.AllowedCIDRs)
	if err != nil {
		return nil, err
	}

	expiresAt, err := s.resolveExpiry(req.ExpiresAt)
	if err != nil {
		return nil, err
	}

	key, err := s.mint(ctx, tenantID, userID, strings.TrimSpace(req.Name), scopes, cidrs, expiresAt, nil)
	if err != nil {
		return nil, err
	}

	s.record(ctx, tenantID, userID, AuditActionAPIKeyCreated, key.ID, models.JSONMap{
		"name":       key.Name,
		"scopes":     scopes.Strings(),
		"expires_at": expiresAt,
		"prefix":     key.KeyPrefix,
	}, auditStatusSuccess)

	s.logger.Info("API key created",
		zap.String("id", key.ID.String()),
		zap.String("name", key.Name),
		zap.String("tenant_id", tenantID.String()),
		zap.Strings("scopes", scopes.Strings()),
		zap.Time("expires_at", expiresAt))

	return key, nil
}

// Rotate mints a replacement for a key and puts the original into an overlap
// window during which both authenticate.
//
// The replacement inherits the original's owner, scopes and address
// restriction: a rotation replaces the secret, not the grant. Changing what a
// key may do is an update, and keeping the two operations apart is what makes
// "rotate this key" safe to do on a schedule without reading the diff.
func (s *APIKeyService) Rotate(ctx context.Context, tenantID, actorID, keyID uuid.UUID, req *models.RotateAPIKeyRequest) (*APIKeyWithPlaintext, error) {
	if !s.Available() {
		return nil, ErrAPIKeyUnavailable
	}
	if req == nil {
		req = &models.RotateAPIKeyRequest{}
	}

	existing, err := s.repo.GetByID(ctx, tenantID, keyID)
	if err != nil {
		return nil, ErrAPIKeyNotFound
	}
	if existing.RevokedAt != nil {
		return nil, errors.New("this key has been revoked; create a new key instead of rotating a revoked one")
	}

	now := s.now()

	overlap := time.Duration(req.OverlapHours) * time.Hour
	if overlap <= 0 {
		overlap = DefaultRotationOverlap
	}
	if overlap > MaxRotationOverlap {
		return nil, fmt.Errorf("the rotation overlap may be at most %d hours", int(MaxRotationOverlap.Hours()))
	}
	deadline := now.Add(overlap)

	// The old key can never outlive its own expiry just because it is being
	// rotated: an expired key stays expired.
	if existing.ExpiresAt != nil && existing.ExpiresAt.Before(deadline) {
		deadline = *existing.ExpiresAt
	}

	scopes, dropped := auth.ParseScopeSetLenient(existing.Scopes)
	if len(dropped) > 0 {
		s.logger.Warn("API key carries scopes that no longer parse; they are not carried into the replacement",
			zap.String("id", existing.ID.String()),
			zap.Strings("dropped", dropped))
	}
	if len(scopes) == 0 {
		return nil, errors.New("this key carries no usable scopes; update its scopes before rotating it")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = existing.Name
	}

	expiresAt, err := s.resolveExpiry(req.ExpiresAt)
	if err != nil {
		return nil, err
	}

	replacement, err := s.mint(ctx, tenantID, existing.UserID, name, scopes,
		[]string(existing.AllowedCIDRs), expiresAt, &existing.ID)
	if err != nil {
		return nil, err
	}

	// Order matters. The replacement exists before the original is given a
	// deadline, so there is no instant in which the integration has no working
	// key.
	if _, err := s.repo.MarkSuperseded(ctx, tenantID, existing.ID, deadline); err != nil {
		s.logger.Error("API key rotated but the original could not be given a deadline; it stays fully valid",
			zap.String("id", existing.ID.String()), zap.Error(err))
		return nil, ErrAPIKeyStorage
	}

	replacement.ReplacedKeyID = &existing.ID
	replacement.ReplacedKeyValidUntil = &deadline

	s.record(ctx, tenantID, actorID, AuditActionAPIKeyRotated, replacement.ID, models.JSONMap{
		"name":                 replacement.Name,
		"replaces":             existing.ID.String(),
		"replaced_prefix":      existing.KeyPrefix,
		"previous_valid_until": deadline,
		"scopes":               scopes.Strings(),
	}, auditStatusSuccess)

	s.logger.Info("API key rotated",
		zap.String("new_id", replacement.ID.String()),
		zap.String("previous_id", existing.ID.String()),
		zap.Time("previous_valid_until", deadline))

	return replacement, nil
}

// Revoke retires a key. It takes effect on the next request that presents it.
func (s *APIKeyService) Revoke(ctx context.Context, tenantID, actorID, keyID uuid.UUID, reason string) error {
	if s == nil || s.repo == nil {
		return ErrAPIKeyUnavailable
	}

	existing, err := s.repo.GetByID(ctx, tenantID, keyID)
	if err != nil {
		return ErrAPIKeyNotFound
	}

	changed, err := s.repo.Revoke(ctx, tenantID, keyID, strings.TrimSpace(reason), s.now())
	if err != nil {
		s.logger.Error("failed to revoke an API key", zap.Error(err))
		return ErrAPIKeyStorage
	}
	if changed == 0 {
		// Already revoked. Report success: the caller asked for a state, and
		// the state holds.
		return nil
	}

	s.record(ctx, tenantID, actorID, AuditActionAPIKeyRevoked, keyID, models.JSONMap{
		"name":   existing.Name,
		"prefix": existing.KeyPrefix,
		"reason": reason,
	}, auditStatusSuccess)

	s.logger.Info("API key revoked",
		zap.String("id", keyID.String()),
		zap.String("name", existing.Name),
		zap.String("reason", reason))

	return nil
}

// Get retrieves an API key by ID
func (s *APIKeyService) Get(ctx context.Context, tenantID, id uuid.UUID) (*models.APIKey, error) {
	if s == nil || s.repo == nil {
		return nil, ErrAPIKeyUnavailable
	}
	return s.repo.GetByID(ctx, tenantID, id)
}

// List retrieves all API keys for a tenant with pagination
func (s *APIKeyService) List(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.APIKey, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrAPIKeyUnavailable
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage
	return s.repo.ListByTenant(ctx, tenantID, perPage, offset)
}

// Update updates an existing API key. It cannot change the secret - that is
// what Rotate is for - and it cannot un-revoke a key.
func (s *APIKeyService) Update(ctx context.Context, tenantID, actorID, id uuid.UUID, req *models.UpdateAPIKeyRequest) (*models.APIKey, error) {
	if s == nil || s.repo == nil {
		return nil, ErrAPIKeyUnavailable
	}
	if req == nil {
		return nil, errors.New("a request is required")
	}

	apiKey, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, ErrAPIKeyNotFound
	}

	changes := models.JSONMap{}

	if name := strings.TrimSpace(req.Name); name != "" && name != apiKey.Name {
		changes["name"] = name
		apiKey.Name = name
	}

	if req.Scopes != nil {
		scopes, err := auth.ParseScopeSet(req.Scopes)
		if err != nil {
			return nil, err
		}
		changes["scopes"] = scopes.Strings()
		apiKey.Scopes = scopes.Strings()
	}

	if req.AllowedCIDRs != nil {
		cidrs, err := auth.ValidateCIDRs(req.AllowedCIDRs)
		if err != nil {
			return nil, err
		}
		changes["allowed_cidrs"] = cidrs
		apiKey.AllowedCIDRs = cidrs
	}

	if req.ExpiresAt != nil {
		expiresAt, err := s.resolveExpiry(req.ExpiresAt)
		if err != nil {
			return nil, err
		}
		changes["expires_at"] = expiresAt
		apiKey.ExpiresAt = &expiresAt
	}

	// The only status an operator may set here is "revoked". Re-activating a
	// key that was revoked is not a state transition this panel offers: the
	// secret was retired, and whatever caused that decision is not undone by
	// flipping a column.
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		if status != "revoked" {
			if apiKey.RevokedAt != nil {
				return nil, errors.New("a revoked key cannot be re-activated; create a new key")
			}
		} else if apiKey.RevokedAt == nil {
			now := s.now()
			apiKey.Status = "revoked"
			apiKey.RevokedAt = &now
			changes["status"] = "revoked"
		}
	}

	if err := s.repo.Update(ctx, apiKey); err != nil {
		s.logger.Error("failed to update an API key", zap.Error(err))
		return nil, ErrAPIKeyStorage
	}

	s.record(ctx, tenantID, actorID, AuditActionAPIKeyUpdated, id, models.JSONMap{
		"name":    apiKey.Name,
		"prefix":  apiKey.KeyPrefix,
		"changes": changes,
	}, auditStatusSuccess)

	return apiKey, nil
}

// Delete removes an API key permanently.
func (s *APIKeyService) Delete(ctx context.Context, tenantID, actorID, id uuid.UUID) error {
	if s == nil || s.repo == nil {
		return ErrAPIKeyUnavailable
	}

	existing, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return ErrAPIKeyNotFound
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		s.logger.Error("failed to delete an API key", zap.Error(err))
		return ErrAPIKeyStorage
	}

	s.record(ctx, tenantID, actorID, AuditActionAPIKeyDeleted, id, models.JSONMap{
		"name":   existing.Name,
		"prefix": existing.KeyPrefix,
	}, auditStatusSuccess)

	return nil
}

// Authenticate resolves a presented key to a principal.
//
// Every rejection returns ErrAPIKeyInvalid. The reason is logged and audited on
// this side and never returned to the caller: "expired" and "unknown" are
// different answers, and the difference tells whoever is guessing which of
// their guesses was once real.
func (s *APIKeyService) Authenticate(ctx context.Context, rawKey, sourceIP string) (*APIKeyPrincipal, error) {
	if !s.Available() {
		return nil, ErrAPIKeyUnavailable
	}

	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrAPIKeyInvalid
	}

	candidates, err := s.repo.ListByPrefixes(ctx, auth.APIKeyLookupPrefixes(rawKey))
	if err != nil {
		s.logger.Error("API key lookup failed", zap.Error(err))
		return nil, ErrAPIKeyInvalid
	}

	matched, needsUpgrade := s.matchKey(rawKey, candidates)
	if matched == nil {
		return nil, ErrAPIKeyInvalid
	}

	now := s.now()
	if reason := s.refusalReason(matched, now, sourceIP); reason != "" {
		s.logger.Warn("API key refused",
			zap.String("id", matched.ID.String()),
			zap.String("name", matched.Name),
			zap.String("reason", reason),
			zap.String("ip", sourceIP))
		s.record(ctx, matched.TenantID, matched.UserID, AuditActionAPIKeyRefused, matched.ID, models.JSONMap{
			"name":   matched.Name,
			"prefix": matched.KeyPrefix,
			"reason": reason,
			"ip":     sourceIP,
		}, auditStatusFailure)
		return nil, ErrAPIKeyInvalid
	}

	if needsUpgrade {
		// The digest was in the old unpeppered form. Rewrite it now that the
		// key is known to be correct; this is the only moment the panel ever
		// holds the material needed to do so.
		if err := s.repo.UpgradeHash(ctx, matched.ID, matched.KeyHash, s.hasher.Hash(rawKey)); err != nil {
			s.logger.Warn("failed to upgrade an API key digest to the peppered form",
				zap.String("id", matched.ID.String()), zap.Error(err))
		}
	}

	scopes, dropped := auth.ParseScopeSetLenient(matched.Scopes)
	if len(dropped) > 0 {
		s.logger.Warn("API key carries scopes that do not parse; they grant nothing",
			zap.String("id", matched.ID.String()),
			zap.Strings("dropped", dropped))
	}

	principal := &APIKeyPrincipal{
		KeyID:    matched.ID,
		KeyName:  matched.Name,
		UserID:   matched.UserID,
		TenantID: matched.TenantID,
		Scopes:   scopes,
	}

	if s.authority != nil {
		if roles, err := s.authority.GetRoleNames(ctx, matched.UserID); err == nil {
			principal.RoleIDs = roles
		} else {
			s.logger.Error("failed to load the roles of an API key's owner; the key authorises nothing this request",
				zap.String("id", matched.ID.String()), zap.Error(err))
		}
		if perms, err := s.authority.GetPermissionNames(ctx, matched.UserID); err == nil {
			principal.Permissions = perms
		} else {
			s.logger.Error("failed to load the permissions of an API key's owner; the key authorises nothing this request",
				zap.String("id", matched.ID.String()), zap.Error(err))
		}
	}

	if err := s.repo.MarkUsed(ctx, matched.ID, now, sourceIP); err != nil {
		s.logger.Warn("failed to record API key use", zap.Error(err))
	}

	return principal, nil
}

// matchKey finds which candidate the presented key belongs to.
//
// Every candidate is compared, and the comparison is over digests, so nothing
// about which candidate matched or how far the comparison got is observable
// from outside. A candidate whose stored digest is in a format this panel
// refuses - the plaintext rows written by service/multi_user.go before it was
// fixed - is logged loudly and skipped.
func (s *APIKeyService) matchKey(rawKey string, candidates []models.APIKey) (*models.APIKey, bool) {
	var matched *models.APIKey
	upgrade := false

	for i := range candidates {
		ok, needsUpgrade, err := s.hasher.VerifyStoredHash(rawKey, candidates[i].KeyHash)
		if err != nil {
			s.logger.Error("API key row holds a digest this panel refuses to authenticate against; "+
				"re-mint the key. A row whose key_hash is not a digest is a plaintext secret in a table that gets backed up.",
				zap.String("id", candidates[i].ID.String()),
				zap.String("name", candidates[i].Name),
				zap.String("action", auditActionAPIKeyPlaintext),
				zap.Error(err))
			continue
		}
		if ok && matched == nil {
			key := candidates[i]
			matched = &key
			upgrade = needsUpgrade
		}
	}

	return matched, upgrade
}

// refusalReason returns why a correct key is nonetheless not usable, or "".
func (s *APIKeyService) refusalReason(key *models.APIKey, now time.Time, sourceIP string) string {
	if key.RevokedAt != nil {
		return "revoked"
	}
	switch strings.ToLower(key.Status) {
	case "revoked", "disabled", "inactive":
		return "revoked"
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(now) {
		return "expired"
	}
	if key.RotationDeadline != nil && !key.RotationDeadline.After(now) {
		return "rotation_overlap_ended"
	}
	if len(key.AllowedCIDRs) > 0 && !auth.AddressInCIDRs(sourceIP, key.AllowedCIDRs) {
		return "source_address_not_allowed"
	}
	return ""
}

// mint generates a key, stores its digest and returns the plaintext once.
func (s *APIKeyService) mint(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	name string,
	scopes auth.ScopeSet,
	cidrs []string,
	expiresAt time.Time,
	rotatedFrom *uuid.UUID,
) (*APIKeyWithPlaintext, error) {
	rawKey, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	apiKey := &models.APIKey{
		ID:           uuid.New(),
		TenantID:     tenantID,
		UserID:       userID,
		Name:         name,
		KeyHash:      s.hasher.Hash(rawKey),
		KeyPrefix:    auth.APIKeyPrefix(rawKey),
		Scopes:       scopes.Strings(),
		ExpiresAt:    &expiresAt,
		Status:       "active",
		RotatedFrom:  rotatedFrom,
		AllowedCIDRs: cidrs,
	}

	if err := s.repo.Create(ctx, apiKey); err != nil {
		s.logger.Error("failed to store a new API key", zap.Error(err))
		return nil, ErrAPIKeyStorage
	}

	return &APIKeyWithPlaintext{APIKey: apiKey, Key: rawKey}, nil
}

// resolveExpiry turns an optional requested expiry into a real one.
func (s *APIKeyService) resolveExpiry(requested *time.Time) (time.Time, error) {
	now := s.now()
	if requested == nil {
		return now.Add(DefaultAPIKeyLifetime), nil
	}
	if !requested.After(now) {
		return time.Time{}, errors.New("the expiry must be in the future")
	}
	if requested.Sub(now) > MaxAPIKeyLifetime {
		return time.Time{}, fmt.Errorf("an API key may live at most %d days", int(MaxAPIKeyLifetime.Hours()/24))
	}
	return *requested, nil
}

// record writes an audit entry, tolerating the absence of an audit service.
func (s *APIKeyService) record(ctx context.Context, tenantID, actorID uuid.UUID, action string, keyID uuid.UUID, details models.JSONMap, status string) {
	if s.audit == nil {
		return
	}
	var actor *uuid.UUID
	if actorID != uuid.Nil {
		id := actorID
		actor = &id
	}
	resourceID := keyID
	s.audit.Record(ctx, tenantID, actor, action, AuditResourceAPIKey, &resourceID, details, "", "", status)
}

// MintAPIKeyMaterial generates a key and the digest to store for it.
//
// It exists so there is exactly one way to mint an API key in this process.
// The second minting path - service/multi_user.go - used to generate its own
// key with its own prefix convention and store it under a "hash" function that
// returned the key unchanged, which put live credentials in a table in plain
// text. It now calls this.
func MintAPIKeyMaterial() (rawKey, storedHash, prefix string, err error) {
	hasher, err := auth.NewKeyHasherFromEnv()
	if err != nil {
		return "", "", "", err
	}
	rawKey, err = auth.GenerateAPIKey()
	if err != nil {
		return "", "", "", err
	}
	return rawKey, hasher.Hash(rawKey), auth.APIKeyPrefix(rawKey), nil
}
