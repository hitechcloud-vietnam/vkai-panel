package agentpki

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// The enrolment token an operator pastes into the agent installer:
//
//	vkai-enrol.v1.<id>.<secret>.<ca-fingerprint>
//
// The id names the record, the secret authenticates the bearer, and the
// fingerprint is the SHA-256 of the panel CA's public key. The fingerprint
// travels with the token so the agent can check the CA certificate it is handed
// during enrolment: the operator is the trusted channel, not the network.
//
// The secret is never stored. The panel keeps SHA-256 of it and compares in
// constant time, so reading the state file does not let its reader enrol.
const (
	tokenPrefix    = "vkai-enrol"
	tokenVersion   = "v1"
	tokenIDBytes   = 16
	tokenSecretLen = 32
)

// ErrBadToken covers every way a pasted token can be wrong that must not tell
// the caller which way it was wrong.
var ErrBadToken = errors.New("agentpki: the enrolment token is not valid")

// EnrolmentInvite is what the panel hands the operator once. The Token is shown
// exactly this once: the panel keeps only a digest of it.
type EnrolmentInvite struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerID  string    `json:"server_id,omitempty"`
	Hostname  string    `json:"hostname,omitempty"`
}

// MintEnrolment creates a one-time, time-limited invitation.
func (a *Authority) MintEnrolment(ctx context.Context, serverID, hostname, createdBy string, ttl time.Duration) (*EnrolmentInvite, error) {
	if ttl <= 0 {
		ttl = a.enrolmentTTL
	}
	idBytes := make([]byte, tokenIDBytes)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("agentpki: cannot generate an enrolment id: %w", err)
	}
	secretBytes := make([]byte, tokenSecretLen)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("agentpki: cannot generate an enrolment secret: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	digest := sha256.Sum256([]byte(secret))

	now := a.now()
	tok := &EnrolmentToken{
		ID:         id,
		SecretHash: digest[:],
		ServerID:   serverID,
		Hostname:   hostname,
		CreatedBy:  createdBy,
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
	}
	if err := a.store.PutEnrolment(ctx, tok); err != nil {
		return nil, err
	}
	a.logger.Info("Agent PKI: enrolment token issued",
		zap.String("enrolment_id", id),
		zap.String("hostname", hostname),
		zap.String("created_by", createdBy),
		zap.Time("expires_at", tok.ExpiresAt))

	return &EnrolmentInvite{
		ID:        id,
		Token:     strings.Join([]string{tokenPrefix, tokenVersion, id, secret, a.CAFingerprint()}, "."),
		ExpiresAt: tok.ExpiresAt,
		ServerID:  serverID,
		Hostname:  hostname,
	}, nil
}

// ParsedToken is the three fields a pasted token carries.
type ParsedToken struct {
	ID            string
	Secret        string
	CAFingerprint string
}

// ParseEnrolmentToken splits a pasted token. It validates shape only; whether
// the secret is right is decided by the panel.
func ParseEnrolmentToken(raw string) (ParsedToken, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 5 || parts[0] != tokenPrefix || parts[1] != tokenVersion {
		return ParsedToken{}, ErrBadToken
	}
	for _, part := range parts[2:] {
		if part == "" {
			return ParsedToken{}, ErrBadToken
		}
	}
	return ParsedToken{ID: parts[2], Secret: parts[3], CAFingerprint: parts[4]}, nil
}

// EnrolRequest is what an agent sends to trade its token for a certificate.
type EnrolRequest struct {
	Token        string `json:"token"`
	CSRPEM       string `json:"csr_pem"`
	Hostname     string `json:"hostname"`
	AgentVersion string `json:"agent_version"`
}

// Enrol spends a token and issues the agent its first certificate.
//
// The token is consumed before anything else is done with it, and the store
// makes that atomic, so two installers racing with the same token produce one
// certificate and one failure - not two certificates.
func (a *Authority) Enrol(ctx context.Context, req EnrolRequest) (*Issued, error) {
	parsed, err := ParseEnrolmentToken(req.Token)
	if err != nil {
		return nil, err
	}
	// Look the record up first so the secret can be compared in constant time,
	// then consume. Consuming first would spend a token that a wrong secret
	// should not have been able to touch.
	stored, err := a.store.GetEnrolment(ctx, parsed.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrBadToken
		}
		return nil, err
	}
	digest := sha256.Sum256([]byte(parsed.Secret))
	if subtle.ConstantTimeCompare(digest[:], stored.SecretHash) != 1 {
		a.logger.Warn("Agent PKI: enrolment attempt with a wrong secret",
			zap.String("enrolment_id", parsed.ID))
		return nil, ErrBadToken
	}

	tok, err := a.store.ConsumeEnrolment(ctx, parsed.ID, a.now())
	if err != nil {
		// Used and expired are reported distinctly: the secret has already been
		// proved correct by this point, so there is nothing left to leak, and
		// an operator needs to know which of the two happened.
		return nil, err
	}

	hostname := strings.TrimSpace(req.Hostname)
	if hostname == "" {
		hostname = tok.Hostname
	}
	agentID := "agent-" + tok.ID

	issued, rec, err := a.issueFromCSR(agentID, RoleAgent, []byte(req.CSRPEM))
	if err != nil {
		return nil, err
	}
	if err := a.recordRotation(ctx, agentID, hostname, tok.ServerID, RoleAgent, rec); err != nil {
		return nil, err
	}
	a.logger.Info("Agent PKI: agent enrolled",
		zap.String("agent_id", agentID),
		zap.String("hostname", hostname),
		zap.String("server_id", tok.ServerID),
		zap.String("serial", rec.Serial),
		zap.String("fingerprint", rec.Fingerprint),
		zap.Time("not_after", rec.NotAfter))
	return issued, nil
}

// Renew issues the next certificate for an agent that has proved possession of
// the key in the one it already holds. The certificate being replaced is kept
// acceptable for the overlap window; see recordRotation.
func (a *Authority) Renew(ctx context.Context, agentID string, csrPEM []byte) (*Issued, error) {
	rec, err := a.store.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if rec.Revoked {
		return nil, ErrRevoked
	}
	issued, certRec, err := a.issueFromCSR(agentID, rec.Role, csrPEM)
	if err != nil {
		return nil, err
	}
	if err := a.recordRotation(ctx, agentID, rec.Hostname, rec.ServerID, rec.Role, certRec); err != nil {
		return nil, err
	}
	a.logger.Info("Agent PKI: certificate rotated",
		zap.String("agent_id", agentID),
		zap.String("serial", certRec.Serial),
		zap.Time("not_after", certRec.NotAfter))
	return issued, nil
}
