package audit

import _ "embed"

// The published verification procedure and its reference implementation travel
// with the panel and are served from the API, so an auditor who is handed a
// bundle can also be handed the rules it is meant to satisfy without asking the
// audited party for source code.
//
// They are embedded rather than read from disk because an installed panel is
// not a source tree: a path that resolves in the repository and not on a
// customer's server is exactly the class of failure this project has already
// paid for.

//go:embed VERIFYING.md
var procedureDoc string

//go:embed verify_audit_export.py
var referenceVerifier string

// ProcedureSummary is the one-line pointer embedded in every export bundle.
const ProcedureSummary = "See VERIFYING.md, served at GET /api/v1/audit/chain/procedure. " +
	"Each entry: content_hash = SHA-256 over length-prefixed content fields; " +
	"entry_hash = SHA-256 over length-prefixed prev_hash, tenant_id, seq and content_hash. " +
	"Verification needs SHA-256 and nothing else."

// ProcedureDoc is the specification an outside auditor works from.
func ProcedureDoc() string { return procedureDoc }

// ReferenceVerifier is a standalone Python 3 verifier that depends on nothing
// but the standard library.
func ReferenceVerifier() string { return referenceVerifier }
