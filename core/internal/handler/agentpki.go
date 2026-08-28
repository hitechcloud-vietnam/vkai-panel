package handler

// The HTTP surface of the panel's internal certificate authority for agents.
//
// Two audiences share this file, and they are authenticated differently:
//
//   - Operators, through the panel UI, with a JWT and an administrator role.
//     They mint enrolment tokens, list what is enrolled, and revoke.
//   - Agents, with no JWT at all. Their first call carries the one-time
//     enrolment token; every call after that is signed with the private key of
//     the certificate they already hold. See agentpki.VerifySignedRequest.
//
// The routes live here rather than in router.go, and are installed by
// RegisterAgentPKIRoutes, so this package can grow the agent channel without
// the central router file being edited by everyone at once.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// maxAgentBody caps what an unauthenticated caller can make the panel read. A
// certificate request is a couple of kilobytes.
const maxAgentBody = 64 << 10

// AgentPKIHandler serves the enrolment, renewal and administration endpoints.
//
// The authority may be nil: if the CA could not be opened at startup, the
// routes answer 503 rather than the process refusing to start. A panel that
// cannot manage agents is a degraded panel; a panel that will not boot is an
// outage.
type AgentPKIHandler struct {
	authority  *agentpki.Authority
	jwtManager *auth.JWTManager
	audit      *service.AuditService
	logger     *zap.Logger
}

// SetAudit installs the audit trail. Minting an enrolment token, revoking an
// agent and deleting one all change which machines this panel will accept
// instructions from, so all three belong in a trail an operator cannot edit.
//
// A setter rather than a constructor argument: NewAgentPKIHandler and
// NewAgentPKIHandlerFromEnv are both called positionally, and the certificate
// authority has to be openable on a panel whose database is not.
func (h *AgentPKIHandler) SetAudit(a *service.AuditService) {
	if h != nil {
		h.audit = a
	}
}

// NewAgentPKIHandler wraps an authority that has already been opened.
func NewAgentPKIHandler(authority *agentpki.Authority, jwtManager *auth.JWTManager, logger *zap.Logger) *AgentPKIHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentPKIHandler{authority: authority, jwtManager: jwtManager, logger: logger}
}

// NewAgentPKIHandlerFromEnv opens the CA at its default location, creating it on
// first run. A failure is logged and yields a handler that answers 503, so this
// can be called from route registration without a second error path.
func NewAgentPKIHandlerFromEnv(jwtManager *auth.JWTManager, logger *zap.Logger) *AgentPKIHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	authority, err := agentpki.New(agentpki.Options{Logger: logger})
	if err != nil {
		logger.Error("Agent PKI unavailable: the agent channel will refuse every request",
			zap.String("dir", agentpki.DefaultDir()),
			zap.Error(err))
		return NewAgentPKIHandler(nil, jwtManager, logger)
	}
	return NewAgentPKIHandler(authority, jwtManager, logger)
}

// Authority exposes the CA, for a caller that also wants an agentclient.
func (h *AgentPKIHandler) Authority() *agentpki.Authority { return h.authority }

// RegisterAgentPKIRoutes installs the agent PKI endpoints on an /api/v1 group.
//
// It applies its own authentication rather than inheriting a group's, because
// the two halves need different answers: an agent has no session to present.
func RegisterAgentPKIRoutes(rg *gin.RouterGroup, h *AgentPKIHandler) {
	// Enrolment is the one door in this file a stranger may knock on, so it is
	// held to a rate a human pasting a token never reaches.
	joining := rg.Group("/agent-pki")
	joining.Use(middleware.RateLimitWith(10, time.Minute))
	{
		joining.GET("/ca", h.CACertificate)
		joining.POST("/enrol", h.Enrol)
	}

	// Renewal and status come from agents that are already enrolled, and a
	// whole fleet can share one source address behind NAT. The limit is set for
	// that - a hundred agents reporting every thirty seconds is 200 a minute -
	// rather than for a single caller, because the real gate here is the
	// signature, not the rate.
	fleet := rg.Group("/agent-pki")
	fleet.Use(middleware.RateLimitWith(600, time.Minute))
	{
		fleet.POST("/renew", h.Renew)
		fleet.POST("/status", h.Status)
	}

	// Operator-facing.
	admin := rg.Group("/agent-pki")
	admin.Use(middleware.AuthRequired(h.jwtManager), middleware.RequireAdmin())
	{
		admin.POST("/enrolments", h.MintEnrolment)
		admin.GET("/agents", h.ListAgents)
		admin.GET("/agents/:agent_id", h.GetAgent)
		admin.POST("/agents/:agent_id/revoke", h.RevokeAgent)
		admin.DELETE("/agents/:agent_id", h.DeleteAgent)
		admin.GET("/deny-list", h.DenyList)
	}
}

// ready reports whether the CA is usable, answering 503 when it is not.
func (h *AgentPKIHandler) ready(c *gin.Context) bool {
	if h.authority == nil {
		utils.ServiceUnavailable(c, "The agent certificate authority is not available on this panel")
		return false
	}
	return true
}

// ============================================================
// AGENT FACING
// ============================================================

// CACertificate hands out the CA certificate. It is a public key: an agent that
// has just enrolled uses it as its trust anchor, and it discloses nothing.
func (h *AgentPKIHandler) CACertificate(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	utils.Success(c, gin.H{
		"ca_pem":         string(h.authority.CACertPEM()),
		"ca_fingerprint": h.authority.CAFingerprint(),
	})
}

type issuedResponse struct {
	AgentID    string    `json:"agent_id"`
	Serial     string    `json:"serial"`
	CertPEM    string    `json:"certificate_pem"`
	CAPEM      string    `json:"ca_pem"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	RenewAfter time.Time `json:"renew_after"`
}

func issuedBody(issued *agentpki.Issued) issuedResponse {
	return issuedResponse{
		AgentID:    issued.AgentID,
		Serial:     issued.Serial,
		CertPEM:    string(issued.CertPEM),
		CAPEM:      string(issued.CAPEM),
		NotBefore:  issued.NotBefore,
		NotAfter:   issued.NotAfter,
		RenewAfter: issued.RenewAfter,
	}
}

// Enrol spends a one-time token and issues an agent its first certificate.
func (h *AgentPKIHandler) Enrol(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var req agentpki.EnrolRequest
	if err := bindLimited(c, &req); err != nil {
		utils.BadRequest(c, "Invalid enrolment request: "+err.Error())
		return
	}
	issued, err := h.authority.Enrol(c.Request.Context(), req)
	if err != nil {
		h.enrolmentError(c, err)
		return
	}
	utils.Created(c, issuedBody(issued))
}

// enrolmentError keeps the three failures an operator has to tell apart -
// wrong, spent, expired - distinct, while giving a caller who has not proved
// anything nothing to work with.
func (h *AgentPKIHandler) enrolmentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agentpki.ErrBadToken):
		h.logger.Warn("Agent PKI: enrolment refused", zap.String("client_ip", c.ClientIP()), zap.Error(err))
		utils.Unauthorized(c, "The enrolment token is not valid")
	case errors.Is(err, agentpki.ErrTokenUsed):
		utils.Conflict(c, "This enrolment token has already been used. Mint a new one.")
	case errors.Is(err, agentpki.ErrTokenExpired):
		utils.Error(c, http.StatusGone, "This enrolment token has expired. Mint a new one.")
	default:
		h.logger.Error("Agent PKI: enrolment failed", zap.Error(err))
		utils.BadRequest(c, "Enrolment failed: "+err.Error())
	}
}

type renewRequest struct {
	CSRPEM string `json:"csr_pem"`
}

// Renew issues the next certificate to an agent that has proved possession of
// the key in the one it holds. No enrolment token is involved, which is what
// makes rotation something the agent can do unattended.
func (h *AgentPKIHandler) Renew(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	body, rec, ok := h.authenticateAgent(c)
	if !ok {
		return
	}
	var req renewRequest
	if err := decodeJSON(body, &req); err != nil {
		utils.BadRequest(c, "Invalid renewal request: "+err.Error())
		return
	}
	issued, err := h.authority.Renew(c.Request.Context(), rec.AgentID, []byte(req.CSRPEM))
	if err != nil {
		if errors.Is(err, agentpki.ErrRevoked) {
			utils.Forbidden(c, "This agent's certificate has been revoked")
			return
		}
		h.logger.Error("Agent PKI: renewal failed", zap.String("agent_id", rec.AgentID), zap.Error(err))
		utils.BadRequest(c, "Renewal failed: "+err.Error())
		return
	}
	utils.Success(c, issuedBody(issued))
}

// Status is the agent reporting in. It replaces the old heartbeat, which
// authenticated itself by carrying the shared secret in a header and again in
// the body; this one is signed with a key that never left the managed server.
//
// The reply carries the deny list, so an agent learns about a revoked panel
// certificate within one status interval rather than at its next expiry.
func (h *AgentPKIHandler) Status(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	_, rec, ok := h.authenticateAgent(c)
	if !ok {
		return
	}
	now := h.authority.Now()
	rec.LastSeenAt = &now
	if err := h.authority.Store().PutAgent(c.Request.Context(), rec); err != nil {
		h.logger.Warn("Agent PKI: cannot record the agent's last contact",
			zap.String("agent_id", rec.AgentID), zap.Error(err))
	}
	serials, err := h.authority.DeniedSerials(c.Request.Context())
	if err != nil {
		utils.InternalError(c, err)
		return
	}
	utils.Success(c, gin.H{
		"agent_id":        rec.AgentID,
		"denied_serials":  serials,
		"renew_after":     rec.Current.NotAfter.Add(-h.authority.RenewBefore()),
		"cert_not_after":  rec.Current.NotAfter,
		"overlap_seconds": int(h.authority.Overlap().Seconds()),
	})
}

// authenticateAgent reads the body once, verifies the signature over it, and
// hands both back. The body has to be read here rather than bound by gin,
// because the signature covers the exact bytes that arrived.
func (h *AgentPKIHandler) authenticateAgent(c *gin.Context) ([]byte, *agentpki.AgentRecord, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxAgentBody))
	if err != nil {
		utils.BadRequest(c, "Cannot read the request body")
		return nil, nil, false
	}
	rec, err := h.authority.VerifySignedRequest(c.Request.Context(),
		agentpki.SignedHeadersFrom(c.Request.Header), body)
	if err != nil {
		switch {
		case errors.Is(err, agentpki.ErrRevoked):
			h.logger.Warn("Agent PKI: a revoked agent was refused",
				zap.String("agent_id", c.Request.Header.Get(agentpki.HeaderAgentID)),
				zap.String("client_ip", c.ClientIP()))
			utils.Forbidden(c, "This agent's certificate has been revoked")
		default:
			h.logger.Warn("Agent PKI: an agent request was refused",
				zap.String("agent_id", c.Request.Header.Get(agentpki.HeaderAgentID)),
				zap.String("client_ip", c.ClientIP()),
				zap.Error(err))
			utils.Unauthorized(c, "The request is not correctly signed by an enrolled agent")
		}
		return nil, nil, false
	}
	return body, rec, true
}

// ============================================================
// OPERATOR FACING
// ============================================================

type mintEnrolmentRequest struct {
	ServerID   string `json:"server_id"`
	Hostname   string `json:"hostname"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// MintEnrolment creates the one-time token an operator pastes into an installer.
// The token is in the response and nowhere else: the panel keeps only a digest,
// so it cannot be shown again.
func (h *AgentPKIHandler) MintEnrolment(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var req mintEnrolmentRequest
	if err := bindLimited(c, &req); err != nil {
		utils.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if req.TTLSeconds > 0 && (ttl < time.Minute || ttl > 24*time.Hour) {
		utils.BadRequest(c, "ttl_seconds must be between 60 and 86400")
		return
	}
	createdBy := middleware.GetUserID(c).String()
	invite, err := h.authority.MintEnrolment(c.Request.Context(), req.ServerID, req.Hostname, createdBy, ttl)
	if err != nil {
		RecordRequestAudit(c, h.audit, audit.ActionAgentEnrolmentMinted, audit.ResourceAgent, nil,
			models.JSONMap{"hostname": req.Hostname, "server_id": req.ServerID, "error": err.Error()},
			audit.StatusFailure)
		utils.InternalError(c, err)
		return
	}

	// The token itself is deliberately NOT in the details: an audit log an
	// operator can read is not a place to put a credential that still works.
	RecordRequestAudit(c, h.audit, audit.ActionAgentEnrolmentMinted, audit.ResourceAgent, nil,
		models.JSONMap{
			"enrolment_id": invite.ID,
			"hostname":     invite.Hostname,
			"server_id":    invite.ServerID,
			"expires_at":   invite.ExpiresAt,
		}, audit.StatusSuccess)

	utils.Created(c, gin.H{
		"enrolment_id":   invite.ID,
		"token":          invite.Token,
		"expires_at":     invite.ExpiresAt,
		"hostname":       invite.Hostname,
		"server_id":      invite.ServerID,
		"ca_fingerprint": h.authority.CAFingerprint(),
		"install_hint": "Run the agent on the target server with VKAI_PANEL_URL set to this panel " +
			"and VKAI_AGENT_ENROLMENT_TOKEN set to the token above. The token works once and expires.",
	})
}

// ListAgents lists what is enrolled.
func (h *AgentPKIHandler) ListAgents(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	records, err := h.authority.Store().ListAgents(c.Request.Context())
	if err != nil {
		utils.InternalError(c, err)
		return
	}
	utils.Success(c, records)
}

// GetAgent returns one record.
func (h *AgentPKIHandler) GetAgent(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	rec, err := h.authority.Store().GetAgent(c.Request.Context(), c.Param("agent_id"))
	if err != nil {
		utils.NotFound(c, "No such agent")
		return
	}
	utils.Success(c, rec)
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

// RevokeAgent puts every certificate an agent holds on the deny list. It takes
// effect on the next handshake and the next signed request, not at expiry.
func (h *AgentPKIHandler) RevokeAgent(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var req revokeRequest
	_ = bindLimited(c, &req)
	agentID := c.Param("agent_id")
	if err := h.authority.Revoke(c.Request.Context(), agentID, req.Reason); err != nil {
		if errors.Is(err, agentpki.ErrNotFound) {
			utils.NotFound(c, "No such agent")
			return
		}
		utils.InternalError(c, err)
		return
	}
	h.logger.Warn("Agent PKI: an operator revoked an agent",
		zap.String("agent_id", agentID),
		zap.String("reason", req.Reason),
		zap.String("by_user", middleware.GetUserID(c).String()))
	RecordRequestAudit(c, h.audit, audit.ActionAgentRevoked, audit.ResourceAgent, nil,
		models.JSONMap{"agent_id": agentID, "reason": req.Reason}, audit.StatusSuccess)
	utils.Success(c, gin.H{"agent_id": agentID, "revoked": true})
}

// DeleteAgent revokes and then forgets an agent. The serials stay denied: a
// record that is gone must not become a record that is trusted again.
func (h *AgentPKIHandler) DeleteAgent(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	agentID := c.Param("agent_id")
	if err := h.authority.Revoke(c.Request.Context(), agentID, "agent removed"); err != nil && !errors.Is(err, agentpki.ErrNotFound) {
		utils.InternalError(c, err)
		return
	}
	if err := h.authority.Store().DeleteAgent(c.Request.Context(), agentID); err != nil {
		if errors.Is(err, agentpki.ErrNotFound) {
			utils.NotFound(c, "No such agent")
			return
		}
		utils.InternalError(c, err)
		return
	}
	RecordRequestAudit(c, h.audit, audit.ActionAgentDeleted, audit.ResourceAgent, nil,
		models.JSONMap{"agent_id": agentID}, audit.StatusSuccess)
	utils.Success(c, gin.H{"agent_id": agentID, "deleted": true})
}

// DenyList shows the revoked serials, so an operator can see what is refused
// and why.
func (h *AgentPKIHandler) DenyList(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	entries, err := h.authority.Store().DenyList(c.Request.Context())
	if err != nil {
		utils.InternalError(c, err)
		return
	}
	utils.Success(c, entries)
}

// ============================================================
// HELPERS
// ============================================================

// bindLimited decodes a JSON body with a size cap. An empty body decodes to the
// zero value rather than an error, so an operation with no arguments can be
// called with no body.
func bindLimited(c *gin.Context, out any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxAgentBody))
	if err != nil {
		return err
	}
	return decodeJSON(body, out)
}

// decodeJSON is the one decoder for every body in this file. It rejects unknown
// fields, because an agent sending a field this panel does not know is a
// version mismatch that should be visible rather than silently dropped.
func decodeJSON(body []byte, out any) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
