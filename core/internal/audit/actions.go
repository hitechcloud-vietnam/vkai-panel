package audit

import "github.com/google/uuid"

// The vocabulary of security-relevant actions.
//
// These names are part of the record: they are hashed into the chain and they
// are what an auditor greps for years later. Constants rather than string
// literals at each call site, because two spellings of the same event is how a
// trail stops being searchable, and a typo in a literal is invisible until
// somebody needs the entry and cannot find it.
//
// Existing constants elsewhere are left alone rather than moved here - the
// two-factor service (twofactor.Action*), the panel settings service and the
// upgrade service each own their names already, and renaming them would rewrite
// the meaning of entries that are already in customers' logs.
const (
	// Authentication.
	ActionSignIn         = "auth.sign_in"
	ActionSignInFailed   = "auth.sign_in_failed"
	ActionSignOut        = "auth.sign_out"
	ActionTokenRefreshed = "auth.token_refreshed"

	// Authorisation. A permission change is the action that makes every other
	// action possible, so it is logged with the before and the after.
	ActionRoleAssigned    = "rbac.role_assigned"
	ActionRoleRemoved     = "rbac.role_removed"
	ActionRoleCreated     = "rbac.role_created"
	ActionRoleUpdated     = "rbac.role_updated"
	ActionRoleDeleted     = "rbac.role_deleted"
	ActionUserCreated     = "user.created"
	ActionUserUpdated     = "user.updated"
	ActionUserDeleted     = "user.deleted"
	ActionPasswordChanged = "user.password_changed"

	// The agent channel.
	ActionAgentEnrolmentMinted = "agent.enrolment_minted"
	ActionAgentEnrolled        = "agent.enrolled"
	ActionAgentRenewed         = "agent.renewed"
	ActionAgentRevoked         = "agent.revoked"
	ActionAgentDeleted         = "agent.deleted"

	// Backups. Taking one is routine; restoring one replaces the state of the
	// machine and is the event an investigation cares about.
	ActionBackupRestored = "backup.restored"
	ActionBackupCreated  = "backup.created"
	ActionBackupDeleted  = "backup.deleted"

	// The audit log's own housekeeping.
	ActionChainVerified = "audit.chain_verified"
	ActionExported      = "audit.exported"
	ActionPruned        = "audit.prune"
)

// The resource names those actions are recorded against.
const (
	ResourceSession = "session"
	ResourceUser    = "user"
	ResourceRole    = "role"
	ResourceAgent   = "agent"
	ResourceBackup  = "backup"
	ResourceAudit   = "audit"
)

// Outcomes. audit_logs.status is a VARCHAR(20) with no constraint, so these
// exist to keep it to two values in practice.
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// DefaultTenantID is the "Default" tenant migration 001 seeds. Every vKAI Panel
// install has it, and on a single-server install it is the only one.
//
// It exists here for one reason: a failed sign-in for a username nobody
// recognises has no tenant to be attributed to, and audit_logs.tenant_id is NOT
// NULL with a foreign key. Dropping the entry instead would mean a password
// spray against invented usernames leaves no trace, which is exactly the
// pattern an audit log is for. So it is recorded against the default tenant
// with the attempted username in the details, and the entry says so.
//
// If an operator has removed that tenant the insert fails the foreign key, the
// write is logged at error level and the sign-in still fails normally: an audit
// problem must never become an authentication bypass or an outage.
var DefaultTenantID = uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
