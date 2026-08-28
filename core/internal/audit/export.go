package audit

import (
	"errors"
	"fmt"
	"time"
)

// Break names the first place a chain stops being consistent, with enough
// information to go and look: which entry, when it was written, and what is
// wrong with it.
type Break struct {
	Seq    int64  `json:"seq"`
	At     string `json:"at,omitempty"`
	LogID  string `json:"log_id,omitempty"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

func (b *Break) Error() string {
	if b == nil {
		return ""
	}
	return fmt.Sprintf("audit chain break at seq %d (%s): %s", b.Seq, b.At, b.Reason)
}

// The reasons a verification pass can report. They are part of the published
// format: an auditor's own implementation should use the same words so two
// reports of the same log read the same.
const (
	// ReasonAnchorMismatch - the first entry of the range does not chain back
	// to what precedes it (the previous entry, a seal left by a prune, or the
	// genesis hash).
	ReasonAnchorMismatch = "anchor_mismatch"
	// ReasonMissingAnchor - nothing precedes the range that could anchor it.
	ReasonMissingAnchor = "missing_anchor"
	// ReasonSequenceGap - a sequence number is missing: an entry was removed.
	ReasonSequenceGap = "sequence_gap"
	// ReasonChainBroken - prev_hash does not name the previous entry: entries
	// were reordered, or one was replaced.
	ReasonChainBroken = "chain_broken"
	// ReasonEntryHashMismatch - the recorded entry_hash is not what this
	// entry's position and contents produce.
	ReasonEntryHashMismatch = "entry_hash_mismatch"
	// ReasonContentAltered - the entry's contents no longer hash to the
	// content_hash the chain committed to.
	ReasonContentAltered = "content_altered"
	// ReasonEntryMissing - the chain has a link for an entry whose row is gone.
	ReasonEntryMissing = "entry_missing"
	// ReasonTruncatedTail - the newest entries were lopped off. Nothing in the
	// surviving chain is wrong; the head says there should be more.
	ReasonTruncatedTail = "truncated_tail"
	// ReasonTenantMismatch - an entry in the bundle belongs to another tenant.
	ReasonTenantMismatch = "tenant_mismatch"
	// ReasonMalformed - a hash is not 64 lowercase hex characters, or a
	// timestamp is not in the documented rendering.
	ReasonMalformed = "malformed"
	// ReasonSealMismatch - a seal committed to a hash at a sequence number and
	// the entry now at that sequence number has a different one. A seal cannot
	// be rewritten, so the entry was.
	ReasonSealMismatch = "seal_mismatch"
)

// ExportEntry is one entry as it appears in a bundle handed to an auditor. It
// carries the hashed strings verbatim, so verifying the bundle needs a SHA-256
// implementation and nothing else - no JSON canonicalisation, no date parsing,
// no knowledge of PostgreSQL, no part of this codebase.
type ExportEntry struct {
	Seq         int64   `json:"seq"`
	PrevHash    string  `json:"prev_hash"`
	ContentHash string  `json:"content_hash"`
	EntryHash   string  `json:"entry_hash"`
	Content     Content `json:"content"`
}

// Anchor is what the first entry of the bundle chains back to.
type Anchor struct {
	// Kind is "genesis" when the bundle starts at seq 1, "seal" when the
	// entries before it were pruned and a seal recorded the boundary hash, or
	// "entry" when the bundle is a slice of a longer chain.
	Kind string `json:"kind"`
	Seq  int64  `json:"seq"`
	Hash string `json:"hash"`
	// SealID and SealedAt are set when Kind is "seal", so an auditor can ask
	// the operator to account for the entries that are missing.
	SealID   string `json:"seal_id,omitempty"`
	SealedAt string `json:"sealed_at,omitempty"`
}

// ExportSeal is one seal: a statement, made at a time and kept in a table that
// refuses UPDATE and DELETE, that the chain had a particular hash at a
// particular sequence number.
//
// They are in the bundle because they are what an auditor can check the panel
// AGAINST rather than merely take from it. Two things fall out of them:
//
//   - a seal whose sequence number is inside the bundle must name the same
//     entry hash the bundle does, or the entry was rewritten after the seal;
//   - a seal whose sequence number is beyond the end of a bundle that claims to
//     be complete means entries that once existed are gone - and unlike the
//     head, which is a mutable pointer the panel rewrites on every entry, a
//     seal cannot be quietly adjusted to match.
type ExportSeal struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Seq        int64  `json:"seq"`
	EntryHash  string `json:"entry_hash"`
	EntryCount int64  `json:"entry_count,omitempty"`
	Note       string `json:"note,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// Head is the tip of the live chain at the moment of export. An auditor who
// holds an export and later re-exports can tell whether the log only grew.
type Head struct {
	Seq  int64  `json:"seq"`
	Hash string `json:"hash"`
}

// Export is the bundle. It is a single JSON document so that it can be signed,
// archived, diffed and re-verified as one object.
type Export struct {
	Format        string       `json:"format"`
	HashAlgorithm string       `json:"hash_algorithm"`
	ContentDomain string       `json:"content_domain"`
	ChainDomain   string       `json:"chain_domain"`
	GenesisHash   string       `json:"genesis_hash"`
	TimeFormat    string       `json:"time_format"`
	Procedure     string       `json:"procedure"`
	TenantID      string       `json:"tenant_id"`
	ExportedAt    string       `json:"exported_at"`
	Anchor        Anchor       `json:"anchor"`
	Head          *Head        `json:"head,omitempty"`
	Seals         []ExportSeal `json:"seals,omitempty"`

	// Complete is the exporter saying "this is the whole chain, from its start
	// to its tip, with nothing left out".
	//
	// It has to be said explicitly, because a bundle cannot work it out. A
	// bundle that ends before the head is EITHER a deliberate slice - the
	// export is capped, so a long history comes out as a series - OR a chain
	// whose newest entries were deleted. Those look identical from inside the
	// file. Guessing from the anchor was wrong in a way that mattered: any
	// chain longer than the export cap would have been reported as tampered
	// with, which is the worst possible false positive for this feature.
	//
	// The tail checks below apply only to a bundle that claims to be complete.
	// An exporter can lie by setting this false, and then the auditor is
	// looking at something that says "this is a fragment" - which is the honest
	// answer to give them, and why comparing seals across exports over time is
	// the procedure rather than trusting one file.
	Complete bool `json:"complete"`

	EntryCount int           `json:"entry_count"`
	Entries    []ExportEntry `json:"entries"`
}

// NewExport fills in every field that describes the format, so a bundle is
// self-describing and the caller only supplies the data.
func NewExport(tenantID string, exportedAt time.Time) *Export {
	return &Export{
		Format:        ExportFormat,
		HashAlgorithm: HashAlgorithm,
		ContentDomain: ContentDomain,
		ChainDomain:   ChainDomain,
		GenesisHash:   GenesisHash,
		TimeFormat:    "YYYY-MM-DDTHH:MM:SS.ffffffZ (UTC)",
		Procedure:     ProcedureSummary,
		TenantID:      tenantID,
		ExportedAt:    FormatTime(exportedAt),
	}
}

// VerifyResult is what a verification pass has to say.
type VerifyResult struct {
	OK       bool   `json:"ok"`
	Checked  int    `json:"checked"`
	FirstSeq int64  `json:"first_seq,omitempty"`
	LastSeq  int64  `json:"last_seq,omitempty"`
	Break    *Break `json:"break,omitempty"`
	// HeadOK reports whether the bundle reaches the tip the panel claimed.
	// False means entries newer than the last one here are unaccounted for.
	HeadOK bool `json:"head_ok"`
}

// ErrNotAnExport is returned for a document that does not declare this format.
var ErrNotAnExport = errors.New("audit: not a vkai-audit-export-v1 document")

// VerifyExport walks a bundle and reports the first break.
//
// This is the same walk the Python reference verifier performs, and the same
// one audit_chain_verify() performs inside PostgreSQL. The three exist so that
// the format is a specification rather than the behaviour of one program, and
// the test suite asserts they agree.
func VerifyExport(e *Export) (*VerifyResult, error) {
	if e == nil {
		return nil, ErrNotAnExport
	}
	if e.Format != ExportFormat {
		return nil, fmt.Errorf("%w: format is %q", ErrNotAnExport, e.Format)
	}
	if e.ContentDomain != ContentDomain || e.ChainDomain != ChainDomain {
		return nil, fmt.Errorf("audit: bundle was written under a different format version (%q/%q)",
			e.ContentDomain, e.ChainDomain)
	}

	res := &VerifyResult{OK: true, Checked: len(e.Entries), HeadOK: true}
	if len(e.Entries) == 0 {
		// An empty bundle is consistent with an empty log and with a log whose
		// entries were all removed. The head is what tells those apart.
		res.HeadOK = e.Head == nil
		if !res.HeadOK {
			res.OK = false
			res.Break = &Break{Seq: e.Head.Seq, Reason: ReasonTruncatedTail,
				Detail: "the bundle is empty but the panel reported a non-empty chain"}
		}
		return res, nil
	}

	res.FirstSeq = e.Entries[0].Seq
	res.LastSeq = e.Entries[len(e.Entries)-1].Seq

	prevHash := e.Anchor.Hash
	if e.Entries[0].Seq == 1 && e.Anchor.Kind == "genesis" {
		prevHash = GenesisHash
	}
	if !isHash(prevHash) {
		res.OK = false
		res.Break = &Break{Seq: e.Entries[0].Seq, Reason: ReasonMissingAnchor,
			Detail: "the bundle does not say what its first entry chains back to"}
		return res, nil
	}

	var prevSeq int64
	for i := range e.Entries {
		entry := &e.Entries[i]
		at := entry.Content.CreatedAt
		logID := entry.Content.ID

		fail := func(reason, detail string) (*VerifyResult, error) {
			res.OK = false
			res.HeadOK = false
			res.Break = &Break{Seq: entry.Seq, At: at, LogID: logID, Reason: reason, Detail: detail}
			return res, nil
		}

		switch {
		case !isHash(entry.PrevHash) || !isHash(entry.ContentHash) || !isHash(entry.EntryHash):
			return fail(ReasonMalformed, "a hash is not 64 lowercase hex characters")
		case entry.Seq <= 0:
			return fail(ReasonMalformed, "sequence numbers start at 1")
		case entry.Content.TenantID != e.TenantID:
			return fail(ReasonTenantMismatch,
				fmt.Sprintf("entry belongs to tenant %s, bundle is for %s",
					entry.Content.TenantID, e.TenantID))
		}

		if i > 0 && entry.Seq != prevSeq+1 {
			return fail(ReasonSequenceGap,
				fmt.Sprintf("sequence jumps from %d to %d: %d entries are unaccounted for",
					prevSeq, entry.Seq, entry.Seq-prevSeq-1))
		}
		if entry.PrevHash != prevHash {
			reason := ReasonChainBroken
			if i == 0 {
				reason = ReasonAnchorMismatch
			}
			return fail(reason, "prev_hash does not name the entry before this one")
		}
		if got := entry.Content.ContentHash(); got != entry.ContentHash {
			return fail(ReasonContentAltered,
				fmt.Sprintf("contents hash to %s, the chain committed to %s", got, entry.ContentHash))
		}
		if got := EntryHash(entry.PrevHash, entry.Content.TenantID, entry.Seq, entry.ContentHash); got != entry.EntryHash {
			return fail(ReasonEntryHashMismatch,
				fmt.Sprintf("recomputed %s, the entry claims %s", got, entry.EntryHash))
		}

		prevHash = entry.EntryHash
		prevSeq = entry.Seq
	}

	last := e.Entries[len(e.Entries)-1]
	byHash := make(map[int64]string, len(e.Entries))
	for i := range e.Entries {
		byHash[e.Entries[i].Seq] = e.Entries[i].EntryHash
	}

	// Seals first: they are the strongest thing in the bundle, because they are
	// the only part the panel cannot revise. A seal inside the range must agree
	// with the entry at that sequence number.
	for _, seal := range e.Seals {
		hash, covered := byHash[seal.Seq]
		if covered && hash != seal.EntryHash {
			res.OK = false
			res.HeadOK = false
			res.Break = &Break{
				Seq:    seal.Seq,
				Reason: ReasonSealMismatch,
				Detail: fmt.Sprintf("the %s seal recorded on %s committed to %s at this sequence number; "+
					"the bundle has %s", seal.Kind, seal.CreatedAt, seal.EntryHash, hash),
			}
			return res, nil
		}
	}

	// The tail. Everything above can be perfect on a chain whose newest entries
	// were simply thrown away, so it is checked separately - and against two
	// witnesses, because one of them is not trustworthy on its own.
	//
	// The head is a mutable pointer: whoever can delete the newest entries can
	// also move it to match, and then nothing above notices. A seal cannot be
	// moved, so a seal naming a sequence number beyond the end of a bundle that
	// claims to be complete is the finding the head would have hidden.
	//
	// Only for a bundle that claims completeness. See Export.Complete.
	complete := e.Complete

	if e.Head != nil {
		if e.Head.Seq != last.Seq || e.Head.Hash != last.EntryHash {
			res.HeadOK = false
			if e.Head.Seq > last.Seq && complete {
				res.OK = false
				res.Break = &Break{Seq: last.Seq + 1, At: last.Content.CreatedAt,
					Reason: ReasonTruncatedTail,
					Detail: fmt.Sprintf("the chain head is at %d but the bundle ends at %d",
						e.Head.Seq, last.Seq)}
				return res, nil
			}
		}
	}

	for _, seal := range e.Seals {
		if seal.Seq > last.Seq && complete && (e.Head == nil || e.Head.Seq <= last.Seq) {
			res.OK = false
			res.HeadOK = false
			res.Break = &Break{Seq: last.Seq + 1, At: last.Content.CreatedAt,
				Reason: ReasonTruncatedTail,
				Detail: fmt.Sprintf("the %s seal recorded on %s says this chain reached sequence %d; "+
					"the bundle ends at %d and the head agrees with the bundle, so entries that "+
					"once existed have been removed and the head was adjusted to match",
					seal.Kind, seal.CreatedAt, seal.Seq, last.Seq)}
			return res, nil
		}
	}

	return res, nil
}

// isHash reports whether s is 64 lowercase hex characters.
func isHash(s string) bool {
	if len(s) != HashHexLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
