package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// goldenContent is one entry with every awkward case represented: a NULL user
// id and resource id (empty strings), a details object, and a timestamp whose
// microseconds have leading zeros.
var goldenContent = Content{
	ID:         "3f0ca80e-4f32-4c63-803e-9577fcb88ba6",
	TenantID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
	UserID:     "",
	Action:     "auth.sign_in",
	Resource:   "session",
	ResourceID: "",
	Details:    `{"ip": "203.0.113.7", "username": "admin"}`,
	IPAddress:  "203.0.113.7",
	UserAgent:  "Mozilla/5.0",
	Status:     "success",
	CreatedAt:  "2026-01-02T03:04:05.000007Z",
}

// These values are FROZEN. They are not "what the code currently produces" -
// they are the format. A change here means every chain ever written under the
// old definition stops verifying, so a failure of this test is a decision to
// make, never a fixture to update.
const (
	goldenContentHash = "6b2b3ae13287878ff55bde235a4dfbb72ff3121ef046043627b48f8e1f9bfe9a"
	goldenEntryHash   = "3ed208e74bf948b70fe27f56aed73447498f33f81710c3f1e9cf6f4ac88deb51"
	// The same entry with a multi-byte action, which pins that the length
	// prefix counts UTF-8 BYTES and not characters. An implementation that used
	// characters would agree with this codebase on ASCII and diverge silently
	// the first time somebody named a resource in Vietnamese.
	goldenUnicodeAction      = "auth.sign_in.é中"
	goldenUnicodeContentHash = "f3298ec23c2c91aa94fd795514c0b741b89215ddb72368b9719da6a865c72161"
)

func TestGoldenHashes(t *testing.T) {
	if got := goldenContent.ContentHash(); got != goldenContentHash {
		t.Fatalf("content_hash changed: got %s, the format says %s.\n"+
			"If this was deliberate, every existing audit chain is now unverifiable; "+
			"the format version in ContentDomain has to change alongside it.", got, goldenContentHash)
	}
	if got := EntryHash(GenesisHash, goldenContent.TenantID, 1, goldenContentHash); got != goldenEntryHash {
		t.Fatalf("entry_hash changed: got %s, the format says %s", got, goldenEntryHash)
	}

	unicode := goldenContent
	unicode.Action = goldenUnicodeAction
	if got := unicode.ContentHash(); got != goldenUnicodeContentHash {
		t.Fatalf("content_hash of a multi-byte field changed: got %s, want %s",
			got, goldenUnicodeContentHash)
	}
}

// publishedVector is the worked example in VERIFYING.md, the document an
// outside auditor implements from.
//
// If this test fails, the specification the panel hands out is wrong - which is
// worse than a bug, because somebody will implement it and conclude that an
// intact log has been tampered with. The values are in the document; changing
// them here without changing the document there is the failure this guards.
var publishedVector = Content{
	ID:         "00000000-0000-4000-8000-000000000001",
	TenantID:   "00000000-0000-4000-8000-0000000000ff",
	UserID:     "",
	Action:     "auth.sign_in",
	Resource:   "session",
	ResourceID: "",
	Details:    `{"k": "v"}`,
	IPAddress:  "203.0.113.1",
	UserAgent:  "curl/8.0",
	Status:     "success",
	CreatedAt:  "2026-01-02T03:04:05.000006Z",
}

const (
	publishedContentHash = "cd80a44d963564358eafbcb8755c91e26eb65371c665997460dbec356f69abcc"
	publishedEntryHash   = "09ffa0fed83bc77bb51d6c0b9e9b2a55ef11a92eaecc17f5f904ad6ea25accf7"
	publishedFieldHash   = "c3494ca1a2cf8eeb8a11ded316fb55b83c3bbbedb6313cd50415251e5d09e12f"
)

func TestPublishedTestVectorsAreCorrect(t *testing.T) {
	if got := publishedVector.ContentHash(); got != publishedContentHash {
		t.Fatalf("this codebase computes content_hash %s; VERIFYING.md publishes %s",
			got, publishedContentHash)
	}
	if got := EntryHash(GenesisHash, publishedVector.TenantID, 1, publishedContentHash); got != publishedEntryHash {
		t.Fatalf("this codebase computes entry_hash %s; VERIFYING.md publishes %s",
			got, publishedEntryHash)
	}

	sum := sha256.Sum256(appendField(nil, "abc"))
	if got := hex.EncodeToString(sum[:]); got != publishedFieldHash {
		t.Fatalf("SHA-256(F(\"abc\")) is %s; VERIFYING.md publishes %s", got, publishedFieldHash)
	}

	// And the document has to actually contain them, or an auditor reading it
	// has nothing to check against.
	doc := ProcedureDoc()
	for _, want := range []string{publishedContentHash, publishedEntryHash, publishedFieldHash} {
		if !strings.Contains(doc, want) {
			t.Fatalf("VERIFYING.md no longer publishes the vector %s", want)
		}
	}
}

func TestFramingIsLengthPrefixed(t *testing.T) {
	raw := goldenContent.CanonicalBytes()

	// The first field is the domain string, framed.
	wantPrefix := "0000000000000015" + hex.EncodeToString([]byte(ContentDomain))
	if got := hex.EncodeToString(raw[:8+len(ContentDomain)]); got != wantPrefix {
		t.Fatalf("domain framing: got %s, want %s", got, wantPrefix)
	}
	if len(ContentDomain) != 0x15 {
		t.Fatalf("this test hard-codes the domain length; ContentDomain is now %d bytes", len(ContentDomain))
	}

	// Framing must make a field boundary unforgeable: moving a character from
	// one field into the next has to change the bytes, and therefore the hash.
	a := Content{Action: "ab", Resource: "c"}
	b := Content{Action: "a", Resource: "bc"}
	if a.ContentHash() == b.ContentHash() {
		t.Fatal("two different field splits hash the same: the framing is not doing its job")
	}
}

func TestFormatTimeIsSixDigitsUTC(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 7000, time.FixedZone("ICT", 7*3600))
	if got, want := FormatTime(at), "2026-01-01T20:04:05.000007Z"; got != want {
		t.Fatalf("FormatTime: got %s, want %s", got, want)
	}
}

// buildChain returns an intact export of n entries, the way the database
// produces one.
func buildChain(t *testing.T, n int) *Export {
	t.Helper()

	exp := NewExport(goldenContent.TenantID, time.Date(2026, 8, 28, 7, 0, 0, 0, time.UTC))
	exp.Anchor = Anchor{Kind: "genesis", Seq: 0, Hash: GenesisHash}

	prev := GenesisHash
	base := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	for i := 1; i <= n; i++ {
		content := goldenContent
		content.ID = uuidLike(i)
		content.Action = "act." + itoa(i)
		content.CreatedAt = FormatTime(base.Add(time.Duration(i) * time.Second))

		ch := content.ContentHash()
		eh := EntryHash(prev, content.TenantID, int64(i), ch)

		exp.Entries = append(exp.Entries, ExportEntry{
			Seq: int64(i), PrevHash: prev, ContentHash: ch, EntryHash: eh, Content: content,
		})
		prev = eh
	}

	exp.EntryCount = len(exp.Entries)
	exp.Complete = true
	if n > 0 {
		exp.Head = &Head{Seq: int64(n), Hash: prev}
	}
	return exp
}

func uuidLike(i int) string {
	s := itoa(i)
	return "00000000-0000-4000-8000-" + strings.Repeat("0", 12-len(s)) + s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}

func mustVerify(t *testing.T, exp *Export) *VerifyResult {
	t.Helper()
	res, err := VerifyExport(exp)
	if err != nil {
		t.Fatalf("VerifyExport returned an error: %v", err)
	}
	return res
}

func TestVerifyIntactChain(t *testing.T) {
	exp := buildChain(t, 25)

	res := mustVerify(t, exp)
	if !res.OK {
		t.Fatalf("an intact chain did not verify: %+v", res.Break)
	}
	if res.Checked != 25 || res.FirstSeq != 1 || res.LastSeq != 25 {
		t.Fatalf("unexpected coverage: %+v", res)
	}
	if !res.HeadOK {
		t.Fatal("head should match the last entry of a complete export")
	}
}

func TestVerifyDetectsAlteredEntry(t *testing.T) {
	exp := buildChain(t, 10)

	// The realistic edit: change what an entry says and leave the hashes alone,
	// because the attacker cannot recompute the chain without breaking the
	// entries after it.
	exp.Entries[6].Content.Action = "something.else"

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("an altered entry verified as intact")
	}
	if res.Break.Seq != 7 {
		t.Fatalf("break reported at seq %d, the alteration is at 7", res.Break.Seq)
	}
	if res.Break.Reason != ReasonContentAltered {
		t.Fatalf("reason %q, want %q", res.Break.Reason, ReasonContentAltered)
	}
	if res.Break.At != exp.Entries[6].Content.CreatedAt {
		t.Fatalf("break timestamp %q does not name the altered entry", res.Break.At)
	}
}

func TestVerifyDetectsAlteredEntryWithRecomputedContentHash(t *testing.T) {
	exp := buildChain(t, 10)

	// The thorough attacker: edit the entry AND recompute its content hash, so
	// the content check passes. entry_hash still binds content_hash, so the
	// second level catches it.
	exp.Entries[3].Content.Action = "something.else"
	exp.Entries[3].ContentHash = exp.Entries[3].Content.ContentHash()

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("an altered entry with a recomputed content hash verified as intact")
	}
	if res.Break.Seq != 4 || res.Break.Reason != ReasonEntryHashMismatch {
		t.Fatalf("got seq %d reason %q, want seq 4 reason %q",
			res.Break.Seq, res.Break.Reason, ReasonEntryHashMismatch)
	}
}

func TestVerifyDetectsDeletedEntry(t *testing.T) {
	exp := buildChain(t, 10)
	exp.Entries = append(exp.Entries[:4], exp.Entries[5:]...) // remove seq 5
	exp.EntryCount = len(exp.Entries)

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("a chain with an entry removed verified as intact")
	}
	if res.Break.Seq != 6 || res.Break.Reason != ReasonSequenceGap {
		t.Fatalf("got seq %d reason %q, want seq 6 reason %q",
			res.Break.Seq, res.Break.Reason, ReasonSequenceGap)
	}
	if !strings.Contains(res.Break.Detail, "1 entries are unaccounted for") {
		t.Fatalf("the detail should say how many went missing: %q", res.Break.Detail)
	}
}

func TestVerifyDetectsReorderedEntries(t *testing.T) {
	exp := buildChain(t, 10)
	// Swap two entries wholesale, sequence numbers and all - the attacker who
	// wants the log to read in a different order.
	exp.Entries[3], exp.Entries[4] = exp.Entries[4], exp.Entries[3]
	exp.Entries[3].Seq, exp.Entries[4].Seq = exp.Entries[4].Seq, exp.Entries[3].Seq

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("a reordered chain verified as intact")
	}
	if res.Break.Seq != 4 || res.Break.Reason != ReasonChainBroken {
		t.Fatalf("got seq %d reason %q, want seq 4 reason %q",
			res.Break.Seq, res.Break.Reason, ReasonChainBroken)
	}
}

func TestVerifyDetectsTruncatedTail(t *testing.T) {
	exp := buildChain(t, 10)
	// Everything that remains is perfect; three entries were simply thrown
	// away. Only the head knows.
	exp.Entries = exp.Entries[:7]
	exp.EntryCount = len(exp.Entries)

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("a chain with its newest entries removed verified as intact")
	}
	if res.Break.Reason != ReasonTruncatedTail {
		t.Fatalf("reason %q, want %q", res.Break.Reason, ReasonTruncatedTail)
	}
	if res.Break.Seq != 8 {
		t.Fatalf("break at seq %d, the first missing entry is 8", res.Break.Seq)
	}
	if res.HeadOK {
		t.Fatal("head_ok must be false when the tail is missing")
	}
}

func TestVerifySliceIsNotABreak(t *testing.T) {
	full := buildChain(t, 10)

	// A bundle that deliberately covers entries 4..7 of a longer chain is a
	// slice, not a broken chain: the anchor says what it hangs from.
	slice := NewExport(full.TenantID, time.Now())
	slice.Entries = append(slice.Entries, full.Entries[3:7]...)
	slice.EntryCount = len(slice.Entries)
	slice.Anchor = Anchor{Kind: "entry", Seq: 3, Hash: full.Entries[2].EntryHash}
	slice.Head = full.Head
	slice.Complete = false

	res := mustVerify(t, slice)
	if !res.OK {
		t.Fatalf("a legitimate slice was reported broken: %+v", res.Break)
	}
	if res.HeadOK {
		t.Fatal("a slice must not claim to reach the head")
	}
}

func TestVerifyDetectsBrokenAnchor(t *testing.T) {
	full := buildChain(t, 10)

	slice := NewExport(full.TenantID, time.Now())
	slice.Entries = append(slice.Entries, full.Entries[3:7]...)
	slice.EntryCount = len(slice.Entries)
	slice.Anchor = Anchor{Kind: "seal", Seq: 3, Hash: strings.Repeat("a", 64)}
	slice.Complete = false

	res := mustVerify(t, slice)
	if res.OK {
		t.Fatal("a slice hanging from the wrong anchor verified as intact")
	}
	if res.Break.Reason != ReasonAnchorMismatch {
		t.Fatalf("reason %q, want %q", res.Break.Reason, ReasonAnchorMismatch)
	}
}

func TestVerifyDetectsAnEntryAlteredAfterItWasSealed(t *testing.T) {
	exp := buildChain(t, 10)

	// A seal taken when the chain reached entry 6, kept in a table the panel
	// cannot rewrite.
	exp.Seals = []ExportSeal{{
		ID: "11111111-1111-4111-8111-111111111111", Kind: "checkpoint", Seq: 6,
		EntryHash: exp.Entries[5].EntryHash, CreatedAt: "2026-08-28T06:00:06.000000Z",
	}}

	if res := mustVerify(t, exp); !res.OK {
		t.Fatalf("a bundle with a matching seal did not verify: %+v", res.Break)
	}

	// Now rewrite entry 6 completely - contents, content hash and entry hash -
	// and relink everything after it. The chain is internally perfect. The seal
	// is the only thing that remembers.
	prev := exp.Entries[4].EntryHash
	exp.Entries[5].Content.Action = "act.innocent"
	for i := 5; i < len(exp.Entries); i++ {
		e := &exp.Entries[i]
		e.PrevHash = prev
		e.ContentHash = e.Content.ContentHash()
		e.EntryHash = EntryHash(e.PrevHash, e.Content.TenantID, e.Seq, e.ContentHash)
		prev = e.EntryHash
	}
	exp.Head = &Head{Seq: exp.Entries[len(exp.Entries)-1].Seq, Hash: prev}

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("a completely relinked chain verified as intact; the seal was not consulted")
	}
	if res.Break.Reason != ReasonSealMismatch || res.Break.Seq != 6 {
		t.Fatalf("got %q at seq %d, want %q at 6", res.Break.Reason, res.Break.Seq, ReasonSealMismatch)
	}
}

func TestVerifyDetectsATruncatedTailHiddenByARewrittenHead(t *testing.T) {
	exp := buildChain(t, 10)
	exp.Seals = []ExportSeal{{
		ID: "22222222-2222-4222-8222-222222222222", Kind: "checkpoint", Seq: 10,
		EntryHash: exp.Entries[9].EntryHash, CreatedAt: "2026-08-28T06:00:10.000000Z",
	}}

	// Throw away the newest three entries AND move the head to match, which is
	// what an attacker who has read the code does. Nothing in the chain itself
	// disagrees.
	exp.Entries = exp.Entries[:7]
	exp.EntryCount = len(exp.Entries)
	last := exp.Entries[len(exp.Entries)-1]
	exp.Head = &Head{Seq: last.Seq, Hash: last.EntryHash}

	res := mustVerify(t, exp)
	if res.OK {
		t.Fatal("a truncated tail with a matching head verified as intact; " +
			"the head alone is not a witness and the seal was not consulted")
	}
	if res.Break.Reason != ReasonTruncatedTail || res.Break.Seq != 8 {
		t.Fatalf("got %q at seq %d, want %q at 8", res.Break.Reason, res.Break.Seq, ReasonTruncatedTail)
	}
	if !strings.Contains(res.Break.Detail, "reached sequence 10") {
		t.Fatalf("the detail should name what the seal committed to: %q", res.Break.Detail)
	}
}

func TestVerifySealBeyondASliceIsNotABreak(t *testing.T) {
	full := buildChain(t, 10)

	slice := NewExport(full.TenantID, time.Now())
	slice.Entries = append(slice.Entries, full.Entries[2:6]...)
	slice.EntryCount = len(slice.Entries)
	slice.Anchor = Anchor{Kind: "entry", Seq: 2, Hash: full.Entries[1].EntryHash}
	slice.Head = full.Head
	slice.Complete = false
	slice.Seals = []ExportSeal{{
		ID: "33333333-3333-4333-8333-333333333333", Kind: "checkpoint", Seq: 10,
		EntryHash: full.Entries[9].EntryHash, CreatedAt: "2026-08-28T06:00:10.000000Z",
	}}

	res := mustVerify(t, slice)
	if !res.OK {
		t.Fatalf("a seal beyond a deliberate slice was reported as a break: %+v", res.Break)
	}
}

// TestVerifyDoesNotAccuseALongChainOfTruncation is the regression test for a
// false positive that would have fired on every large install.
//
// Exports are capped, so a chain longer than the cap comes out as a series of
// bundles. The first one starts at genesis and stops well before the head. An
// earlier version inferred "this bundle claims to be complete" from the anchor
// being genesis, and so reported every such bundle as a deleted tail - accusing
// the customer of tampering because their audit log was big. Completeness is
// now something the exporter states, not something the verifier guesses.
func TestVerifyDoesNotAccuseALongChainOfTruncation(t *testing.T) {
	full := buildChain(t, 20)

	// The first bundle of a series: starts at the beginning, stops at 8 because
	// that is as much as one bundle holds.
	first := NewExport(full.TenantID, time.Now())
	first.Entries = append(first.Entries, full.Entries[:8]...)
	first.EntryCount = len(first.Entries)
	first.Anchor = Anchor{Kind: "genesis", Seq: 0, Hash: GenesisHash}
	first.Head = full.Head
	first.Complete = false
	first.Seals = []ExportSeal{{
		ID: "66666666-6666-4666-8666-666666666666", Kind: "checkpoint", Seq: 20,
		EntryHash: full.Entries[19].EntryHash, CreatedAt: "2026-08-28T06:00:20.000000Z",
	}}

	res := mustVerify(t, first)
	if !res.OK {
		t.Fatalf("the first bundle of a series was reported as tampered with: %+v", res.Break)
	}
	if res.HeadOK {
		t.Fatal("a bundle that does not reach the head must not claim it does")
	}

	// The next bundle in the series picks up where it left off and chains to it.
	second := NewExport(full.TenantID, time.Now())
	second.Entries = append(second.Entries, full.Entries[8:]...)
	second.EntryCount = len(second.Entries)
	second.Anchor = Anchor{Kind: "entry", Seq: 8, Hash: full.Entries[7].EntryHash}
	second.Head = full.Head
	second.Complete = false

	if res := mustVerify(t, second); !res.OK {
		t.Fatalf("the second bundle of the series did not verify: %+v", res.Break)
	}

	// And the same short bundle DOES get called out when it claims to be
	// everything, which is the case the check exists for.
	first.Complete = true
	res = mustVerify(t, first)
	if res.OK {
		t.Fatal("a bundle claiming completeness while stopping short of the head verified")
	}
	if res.Break.Reason != ReasonTruncatedTail {
		t.Fatalf("reason %q, want %q", res.Break.Reason, ReasonTruncatedTail)
	}
}

func TestVerifyRejectsForeignFormat(t *testing.T) {
	exp := buildChain(t, 3)
	exp.ChainDomain = "vkai-audit-chain-v2"

	if _, err := VerifyExport(exp); err == nil {
		t.Fatal("a bundle written under another format version must be refused, not verified")
	}
}

// TestPythonReferenceVerifierAgrees runs the published Python verifier over the
// same bundles this package verifies.
//
// This is the test that makes the format a specification rather than a
// description of one program: an outside auditor runs that script, and if it
// and this code ever disagree about a bundle, one of them is wrong and the
// auditor has no way to tell which.
func TestPythonReferenceVerifierAgrees(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed; the reference verifier cannot be cross-checked here")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "verify_audit_export.py")
	if err := os.WriteFile(script, []byte(ReferenceVerifier()), 0o600); err != nil {
		t.Fatal(err)
	}

	intact := buildChain(t, 12)

	altered := buildChain(t, 12)
	altered.Entries[5].Content.Status = "failure"

	deleted := buildChain(t, 12)
	deleted.Entries = append(deleted.Entries[:2], deleted.Entries[3:]...)

	reordered := buildChain(t, 12)
	reordered.Entries[8], reordered.Entries[9] = reordered.Entries[9], reordered.Entries[8]
	reordered.Entries[8].Seq, reordered.Entries[9].Seq = reordered.Entries[9].Seq, reordered.Entries[8].Seq

	truncated := buildChain(t, 12)
	truncated.Entries = truncated.Entries[:9]

	// A chain rewritten so thoroughly that only a seal disagrees.
	sealed := buildChain(t, 12)
	sealed.Seals = []ExportSeal{{
		ID: "44444444-4444-4444-8444-444444444444", Kind: "checkpoint", Seq: 7,
		EntryHash: sealed.Entries[6].EntryHash, CreatedAt: "2026-08-28T06:00:07.000000Z",
	}}
	relinked := buildChain(t, 12)
	relinked.Seals = sealed.Seals
	prev := relinked.Entries[5].EntryHash
	relinked.Entries[6].Content.Status = "failure"
	for i := 6; i < len(relinked.Entries); i++ {
		e := &relinked.Entries[i]
		e.PrevHash = prev
		e.ContentHash = e.Content.ContentHash()
		e.EntryHash = EntryHash(e.PrevHash, e.Content.TenantID, e.Seq, e.ContentHash)
		prev = e.EntryHash
	}
	relinked.Head = &Head{Seq: 12, Hash: prev}

	// A tail removed with the head tidied up to match.
	hidden := buildChain(t, 12)
	hidden.Seals = []ExportSeal{{
		ID: "55555555-5555-4555-8555-555555555555", Kind: "checkpoint", Seq: 12,
		EntryHash: hidden.Entries[11].EntryHash, CreatedAt: "2026-08-28T06:00:12.000000Z",
	}}
	hidden.Entries = hidden.Entries[:9]
	hidden.Head = &Head{Seq: 9, Hash: hidden.Entries[8].EntryHash}
	hidden.Complete = true

	// The same shape, honestly labelled as a fragment: not a break.
	fragment := buildChain(t, 12)
	fragment.Seals = hidden.Seals
	fragment.Entries = fragment.Entries[:9]
	fragment.Head = &Head{Seq: 12, Hash: buildChain(t, 12).Entries[11].EntryHash}
	fragment.Complete = false

	cases := map[string]*Export{
		"intact":         intact,
		"altered":        altered,
		"deleted":        deleted,
		"reordered":      reordered,
		"truncated":      truncated,
		"sealed_intact":  sealed,
		"relinked":       relinked,
		"hidden_by_head": hidden,
		"fragment":       fragment,
	}

	for name, exp := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			raw, err := json.MarshalIndent(exp, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}

			out, runErr := exec.Command(python, script, path, "--json").Output()
			if len(out) == 0 {
				t.Fatalf("the reference verifier produced no output: %v", runErr)
			}

			var got struct {
				OK    bool `json:"ok"`
				Break struct {
					Seq    int64  `json:"seq"`
					Reason string `json:"reason"`
				} `json:"break"`
			}
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("could not read the verifier's answer %q: %v", out, err)
			}

			want := mustVerify(t, exp)
			if got.OK != want.OK {
				t.Fatalf("Go says ok=%v, the published Python verifier says ok=%v", want.OK, got.OK)
			}
			if !want.OK {
				if got.Break.Seq != want.Break.Seq || got.Break.Reason != want.Break.Reason {
					t.Fatalf("the two implementations disagree about the break: "+
						"Go says seq %d %q, Python says seq %d %q",
						want.Break.Seq, want.Break.Reason, got.Break.Seq, got.Break.Reason)
				}
			}
		})
	}
}

func TestProcedureAndVerifierAreEmbedded(t *testing.T) {
	// An installed panel is not a source tree. If these are empty, the export
	// endpoint serves an auditor nothing and the whole "independently
	// verifiable" claim is theatre.
	if len(ProcedureDoc()) < 2000 {
		t.Fatalf("VERIFYING.md is not embedded (%d bytes)", len(ProcedureDoc()))
	}
	if !strings.Contains(ReferenceVerifier(), "def content_hash(") {
		t.Fatal("the Python reference verifier is not embedded")
	}
	for _, want := range []string{ContentDomain, ChainDomain, "8-byte big-endian"} {
		if !strings.Contains(ProcedureDoc(), want) {
			t.Fatalf("the published procedure does not mention %q", want)
		}
	}
}
