package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type AuditService struct {
	repo   *repository.AuditRepository
	logger *zap.Logger
}

func NewAuditService(repo *repository.AuditRepository, logger *zap.Logger) *AuditService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AuditService{
		repo:   repo,
		logger: logger,
	}
}

func (s *AuditService) Log(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, action, resource string, resourceID *uuid.UUID, details models.JSONMap, ipAddress, userAgent, status string) error {
	log := &models.AuditLog{
		TenantID:   tenantID,
		UserID:     userID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    details,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Status:     status,
	}
	return s.repo.Create(ctx, log)
}

// Record is Log for a caller that has nowhere useful to put an error.
//
// An audit write that fails is serious, but it is not a reason to fail the
// action that was being audited - refusing a sign-in because the log is full
// hands an attacker a denial of service. It is logged at error level with
// everything needed to reconstruct the entry by hand.
func (s *AuditService) Record(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, action, resource string, resourceID *uuid.UUID, details models.JSONMap, ipAddress, userAgent, status string) {
	if s == nil || s.repo == nil {
		return
	}
	if err := s.Log(ctx, tenantID, userID, action, resource, resourceID, details, ipAddress, userAgent, status); err != nil {
		s.logger.Error("AUDIT WRITE FAILED - this event is not in the trail",
			zap.String("action", action),
			zap.String("resource", resource),
			zap.String("status", status),
			zap.String("tenant_id", tenantID.String()),
			zap.String("ip", ipAddress),
			zap.Any("details", details),
			zap.Error(err))
	}
}

func (s *AuditService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.AuditLog, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *AuditService) Search(ctx context.Context, tenantID uuid.UUID, req *models.AuditLogSearchRequest) ([]models.AuditLog, int, error) {
	return s.repo.Search(ctx, tenantID, req)
}

func (s *AuditService) GetStats(ctx context.Context, tenantID uuid.UUID, days int) (*models.AuditLogStats, error) {
	return s.repo.GetStats(ctx, tenantID, days)
}

// ============================================================
// The tamper-evident chain
// ============================================================

// VerifyRequest asks for a verification pass.
type VerifyRequest struct {
	// Full ignores what previous passes established and walks from the oldest
	// surviving entry. Anything else resumes from the last pass that came back
	// clean, which is what makes this affordable to run every few minutes.
	Full bool
	// Deep re-reads audit_logs and recomputes each entry's content hash. This
	// is the half that catches an EDITED entry, and the half that costs a join.
	// Without it the pass reads audit_log_chain alone and still catches
	// removal, reordering and relinking.
	Deep bool
	// FromSeq and ToSeq bound the range explicitly. Zero means "let the service
	// decide", which is what a scheduled run wants.
	FromSeq int64
	ToSeq   int64
}

// VerifyReport is a pass and its context.
type VerifyReport struct {
	*repository.ChainVerification
	// Resumed is the sequence number this pass started from because an earlier
	// pass had already cleared everything below it. Zero for a full pass.
	Resumed int64 `json:"resumed_from"`
	// Note explains, in words an operator can act on, what the result means.
	Note string `json:"note"`
	// Checkpoint is the seal a clean full pass left behind, if it left one.
	Checkpoint *repository.Seal `json:"checkpoint,omitempty"`
}

// Verify walks the chain and reports the first break.
//
// Two things are true at once and both are said out loud rather than one being
// quietly assumed:
//
//   - A full deep pass is the ground truth. It re-reads every entry and
//     recomputes every hash, and it is the only pass that would notice an entry
//     edited last year. It is O(n) and belongs on a schedule.
//   - An incremental pass trusts that the prefix was intact when the last clean
//     pass looked at it. That is a real assumption. It buys a check that costs
//     time proportional to what has been written since, which is what makes
//     running it often possible at all.
func (s *AuditService) Verify(ctx context.Context, tenantID uuid.UUID, req VerifyRequest) (*VerifyReport, error) {
	from := req.FromSeq
	var resumed int64

	if from <= 0 && !req.Full {
		last, err := s.repo.LastGoodSeq(ctx, tenantID, req.Deep)
		if err != nil {
			return nil, err
		}
		if last > 0 {
			// Start one before the last verified entry, not one after: the
			// entry at that sequence number is re-checked so the pass re-anchors
			// on something it verified itself rather than on a stored claim.
			from = last
			resumed = last
		}
	}

	v, err := s.repo.Verify(ctx, tenantID, from, req.ToSeq, req.Deep)
	if err != nil {
		return nil, err
	}

	if err := s.repo.RecordVerification(ctx, tenantID, v); err != nil {
		// A pass that ran and could not be filed is still a valid answer.
		s.logger.Warn("audit chain: could not record the verification result",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
	}

	report := &VerifyReport{
		ChainVerification: v,
		Resumed:           resumed,
		Note:              verifyNote(v, req, resumed),
	}

	// A clean FULL pass has just proved everything from the oldest surviving
	// entry to the tip. Sealing the tip turns that proof into evidence that
	// outlives the pass: audit_chain_head is mutable, so an attacker can delete
	// the newest entries and move the head to match, but a seal saying the
	// chain reached this sequence number lives in a table that refuses UPDATE
	// and DELETE. From here on, fewer entries than this is a provable deletion.
	//
	// Only on a full pass, and only when it reached the head. An incremental
	// pass has not re-read the prefix and has no business vouching for it, and
	// sealing on every poll would fill the table with noise.
	if req.Full && v.OK && v.HeadOK && v.Checked > 0 {
		report.Checkpoint = s.sealCheckpoint(ctx, tenantID, v)
	}

	return report, nil
}

// sealCheckpoint writes the seal and returns it, or nil if it could not be
// written. A verification pass that ran is a valid answer even when the seal
// fails, so this never turns into the caller's error - but it is loud, because
// silently not sealing would leave the panel claiming a protection it lost.
func (s *AuditService) sealCheckpoint(ctx context.Context, tenantID uuid.UUID, v *repository.ChainVerification) *repository.Seal {
	// Seal what the pass ACTUALLY verified, not the head as it stands now.
	// Entries keep arriving while a pass runs, and a seal that named the
	// current head would be vouching for entries nothing has read. A seal is
	// evidence; it must not claim more than was proved.
	if v.LastSeq == nil {
		return nil
	}
	sealSeq := *v.LastSeq

	sealHash, err := s.repo.EntryHashAt(ctx, tenantID, sealSeq)
	if err != nil {
		s.logger.Warn("audit chain: could not read the entry to seal a checkpoint",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
		return nil
	}
	if sealHash == "" {
		return nil
	}

	last, err := s.repo.LastCheckpointSeq(ctx, tenantID)
	if err != nil {
		s.logger.Warn("audit chain: could not read the last checkpoint",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
		return nil
	}
	if last >= sealSeq {
		// Nothing new since the last seal. Re-sealing the same tip adds a row
		// and no evidence.
		return nil
	}

	seal, err := s.repo.RecordCheckpoint(ctx, tenantID, sealSeq, sealHash,
		fmt.Sprintf("full %s verification of %d entries", v.Mode, v.Checked))
	if err != nil {
		s.logger.Error("AUDIT CHAIN: a clean verification pass could not be sealed; "+
			"a truncated tail would now be deniable",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
		return nil
	}
	return seal
}

// verifyNote turns a result into the sentence an operator needs.
func verifyNote(v *repository.ChainVerification, req VerifyRequest, resumed int64) string {
	if !v.OK {
		reason := ""
		if v.BreakReason != nil {
			reason = *v.BreakReason
		}
		seq := int64(0)
		if v.BreakSeq != nil {
			seq = *v.BreakSeq
		}
		when := "an unknown time"
		if v.BreakAt != nil {
			when = audit.FormatTime(*v.BreakAt)
		}
		return fmt.Sprintf(
			"THE AUDIT LOG HAS BEEN TAMPERED WITH. The chain breaks at entry %d, written %s (%s). "+
				"Everything below %d verified. Nothing above it has been checked. "+
				"Treat every entry from %d onwards as unproven until an export taken before the break says otherwise.",
			seq, when, reason, seq, seq)
	}

	scope := "the whole chain"
	if resumed > 0 {
		scope = fmt.Sprintf("entries %d onwards; entries below that were cleared by an earlier pass and were not re-read", resumed)
	}
	depth := "structure only - removal, reordering and relinking. An edited entry would NOT be caught by this pass; run it with deep=true for that"
	if req.Deep {
		depth = "structure and contents"
	}
	head := ""
	if !v.HeadOK {
		head = " The chain head does not match the last entry checked."
	}
	return fmt.Sprintf("Intact: %d entries verified (%s), checking %s.%s", v.Checked, scope, depth, head)
}

// Status is the cheap dashboard answer: no hashing, no scan.
func (s *AuditService) Status(ctx context.Context, tenantID uuid.UUID) (*repository.ChainStatus, error) {
	return s.repo.Status(ctx, tenantID)
}

// Seals lists the tenant's seals - the permanent record of every prune and
// every export.
func (s *AuditService) Seals(ctx context.Context, tenantID uuid.UUID, limit int) ([]repository.Seal, error) {
	return s.repo.Seals(ctx, tenantID, limit)
}

// Export builds a bundle an outside auditor can verify without this codebase,
// and records a seal so the bundle can later be tied back to the live table.
func (s *AuditService) Export(ctx context.Context, tenantID uuid.UUID, fromSeq, toSeq int64, limit int, note string) (*audit.Export, error) {
	exp, err := s.repo.ExportRange(ctx, tenantID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, err
	}

	// Verify what is about to be handed over. Shipping a bundle that does not
	// verify, without saying so, would be worse than shipping nothing.
	res, err := audit.VerifyExport(exp)
	if err != nil {
		return nil, err
	}
	if !res.OK {
		s.logger.Error("audit chain: the exported range does not verify",
			zap.String("tenant_id", tenantID.String()),
			zap.Any("break", res.Break))
		return exp, fmt.Errorf("the audit chain does not verify over the exported range: %w", res.Break)
	}

	if _, err := s.repo.SealExport(ctx, tenantID, exp, note); err != nil {
		s.logger.Warn("audit chain: could not seal the export",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
	}

	return exp, nil
}

// ProcedureDoc and ReferenceVerifier are the published verification procedure
// and a standalone implementation of it, served so that whoever is handed a
// bundle can also be handed the rules without asking for source code.
func (s *AuditService) ProcedureDoc() string      { return audit.ProcedureDoc() }
func (s *AuditService) ReferenceVerifier() string { return audit.ReferenceVerifier() }

// RetentionPlan is the answer to "clean up old audit logs", which on an
// append-only table is a question with a longer answer than it used to have.
type RetentionPlan struct {
	*repository.PrunePreview
	// Refused is always true. The panel has had DELETE on audit_logs revoked in
	// the database, deliberately, and cannot carry this out however it is
	// asked.
	Refused bool `json:"refused"`
	// Explanation and Command are what an operator actually needs back.
	Explanation string `json:"explanation"`
	Command     string `json:"command"`
	Warning     string `json:"warning"`
}

// PlanRetention reports what a retention run would remove and hands back the
// exact commands to do it.
//
// It does not do it. audit_logs and audit_log_chain have UPDATE, DELETE and
// TRUNCATE revoked for the panel's own database role, and a trigger behind that
// refuses the same three operations even if the privilege is granted back. So
// the honest answer to a "clean up" request is not a row count - it is a
// preview, an explanation of what will be lost, and the two statements an
// operator runs at a psql prompt with their own hands.
func (s *AuditService) PlanRetention(ctx context.Context, tenantID uuid.UUID, days int, dbUser string) (*RetentionPlan, error) {
	if days <= 0 {
		days = 90
	}
	cutoff := time.Now().AddDate(0, 0, -days).UTC()

	preview, err := s.repo.PrunePreview(ctx, tenantID, cutoff)
	if err != nil {
		return nil, err
	}
	preview.Retention = days

	if dbUser == "" {
		dbUser = "vkai"
	}

	plan := &RetentionPlan{
		PrunePreview: preview,
		Refused:      true,
		Explanation: "The audit log is append-only and the panel's database role has had DELETE revoked on it. " +
			"Removing entries is an operator action taken deliberately at a psql prompt, not something an HTTP " +
			"request can cause. audit_chain_prune() records a seal - the chain hash at the cut point - before it " +
			"deletes anything, so the surviving entries stay verifiable and the fact that a prune happened is " +
			"itself permanently in the log.",
		Command: strings.Join([]string{
			"-- Export first if the entries have to outlive the table:",
			fmt.Sprintf("--   GET /api/v1/audit/chain/export?to_seq=%s", seqText(preview.SealSeq)),
			"BEGIN;",
			fmt.Sprintf("  GRANT DELETE ON audit_logs, audit_log_chain TO %s;", dbUser),
			fmt.Sprintf("  SELECT * FROM audit_chain_prune('%s'::uuid, NOW() - INTERVAL '%d days', 'retention: %d days');",
				tenantID, days, days),
			fmt.Sprintf("  REVOKE DELETE ON audit_logs, audit_log_chain FROM %s;", dbUser),
			"COMMIT;",
		}, "\n"),
		Warning: "What is lost is the contents of the pruned entries and the ability to prove the chain from " +
			"genesis without the exported bundle. What survives is the seal, which records the chain hash at the " +
			"cut, so the remaining entries still verify - reported as verified from the seal rather than from " +
			"genesis. Retention and immutability genuinely pull against each other; this is where the line is drawn.",
	}

	return plan, nil
}

func seqText(seq *int64) string {
	if seq == nil {
		return "<seq>"
	}
	return fmt.Sprintf("%d", *seq)
}

// ErrRetentionIsManual is returned by CleanupOld, which no longer deletes.
var ErrRetentionIsManual = errors.New(
	"the audit log is append-only: entries cannot be deleted through the panel. " +
		"Use GET /api/v1/audit/chain/retention for the preview and the operator command")

// CleanupOld is kept so that nothing silently stops calling it, and refuses.
//
// It used to run DELETE FROM audit_logs. That statement is now refused by the
// database, which is the point of the feature; a method that swallows the
// refusal and reports "0 deleted" would read as success.
func (s *AuditService) CleanupOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	return 0, ErrRetentionIsManual
}
