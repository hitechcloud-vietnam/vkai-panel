#!/usr/bin/env python3
"""Independent verifier for a vkai-audit-export-v1 audit log bundle.

    usage: verify_audit_export.py BUNDLE.json [--json]

Exit status is 0 when the chain is intact and 1 when it is not, so this can be
driven from cron or a CI job.

This file is a reference implementation of a published format. It uses nothing
but the Python standard library, it does not talk to the panel, the panel's
database or the network, and it is not needed for the format to be verifiable -
anything that can compute SHA-256 can reproduce it from the description below
and from VERIFYING.md, which is included in the same bundle directory.

Do not "fix" this script to agree with a bundle that fails. If a bundle fails,
either the log was altered or the exporter is wrong; both are findings.

THE FORMAT

  F(s)  8-byte big-endian unsigned length of the UTF-8 encoding of s, then
        those bytes. Every field is framed this way so that no value can be
        mistaken for a field boundary.

  content_hash = hex(sha256(
      F("vkai-audit-content-v1") || F(id) || F(tenant_id) || F(user_id) ||
      F(action) || F(resource) || F(resource_id) || F(details) ||
      F(ip_address) || F(user_agent) || F(status) || F(created_at)))

  entry_hash = hex(sha256(
      F("vkai-audit-chain-v1") || F(prev_hash) || F(tenant_id) || F(seq) ||
      F(content_hash)))

  Every input to F is taken verbatim out of the bundle. seq is rendered as
  decimal ASCII. prev_hash is the previous entry's entry_hash, or 64 '0'
  characters for the first entry of a chain. Nothing is re-serialised: in
  particular "details" is already a string in the bundle and must be hashed as
  that string, byte for byte.

SEALS

  A bundle also carries the panel's seals - statements, kept in a table the
  panel cannot rewrite, that the chain had a particular hash at a particular
  sequence number. They are the part of the bundle worth the most, because they
  are the part the audited party could not revise after the fact:

    * a seal inside the exported range must name the same entry_hash the bundle
      does, or that entry was altered after it was sealed;
    * a seal naming a sequence number beyond the end of a bundle that claims to
      be complete means entries that once existed are gone. The "head" cannot
      tell you this on its own - it is a mutable pointer the panel rewrites on
      every entry, so it can be adjusted to match a truncated chain.

  Keep the seals from every export you are given. Their sequence numbers must
  only ever go up.

COMPLETENESS

  "complete": true is the exporter saying the bundle holds the whole chain,
  start to tip. The tail checks apply only then, because a bundle that stops
  short is otherwise indistinguishable from a deliberate slice - exports are
  capped, so a long history arrives as a series of bundles that chain together
  through their anchors.

  An exporter can set it false to avoid those checks. What it gets for that is
  a bundle that says, in writing, "this is a fragment" - so ask for the rest,
  and compare the seals across the series.
"""

import hashlib
import json
import sys

CONTENT_DOMAIN = "vkai-audit-content-v1"
CHAIN_DOMAIN = "vkai-audit-chain-v1"
EXPORT_FORMAT = "vkai-audit-export-v1"
GENESIS = "0" * 64

CONTENT_FIELDS = (
    "id", "tenant_id", "user_id", "action", "resource", "resource_id",
    "details", "ip_address", "user_agent", "status", "created_at",
)


def framed(value):
    """F(s): 8-byte big-endian length of the UTF-8 bytes, then the bytes."""
    raw = value.encode("utf-8")
    return len(raw).to_bytes(8, "big") + raw


def content_hash(content):
    buf = framed(CONTENT_DOMAIN)
    for name in CONTENT_FIELDS:
        buf += framed(content.get(name, ""))
    return hashlib.sha256(buf).hexdigest()


def entry_hash(prev_hash, tenant_id, seq, chash):
    buf = (framed(CHAIN_DOMAIN) + framed(prev_hash) + framed(tenant_id)
           + framed(str(seq)) + framed(chash))
    return hashlib.sha256(buf).hexdigest()


def is_hash(value):
    return (isinstance(value, str) and len(value) == 64
            and all(c in "0123456789abcdef" for c in value))


def verify(bundle):
    """Return (ok, result_dict). result_dict['break'] names the first break."""
    if bundle.get("format") != EXPORT_FORMAT:
        return False, {"error": "not a %s document (format is %r)"
                                % (EXPORT_FORMAT, bundle.get("format"))}
    if (bundle.get("content_domain") != CONTENT_DOMAIN
            or bundle.get("chain_domain") != CHAIN_DOMAIN):
        return False, {"error": "bundle was written under a different format version"}

    tenant = bundle.get("tenant_id")
    entries = bundle.get("entries") or []
    head = bundle.get("head")
    anchor = bundle.get("anchor") or {}

    result = {"checked": len(entries), "head_ok": True}

    if not entries:
        if head:
            return False, dict(result, head_ok=False, **{"break": {
                "seq": head.get("seq"), "reason": "truncated_tail",
                "detail": "the bundle is empty but the panel reported a non-empty chain"}})
        return True, result

    result["first_seq"] = entries[0].get("seq")
    result["last_seq"] = entries[-1].get("seq")

    prev_hash = anchor.get("hash")
    if entries[0].get("seq") == 1 and anchor.get("kind") == "genesis":
        prev_hash = GENESIS
    if not is_hash(prev_hash):
        return False, dict(result, **{"break": {
            "seq": entries[0].get("seq"), "reason": "missing_anchor",
            "detail": "the bundle does not say what its first entry chains back to"}})

    prev_seq = None
    for entry in entries:
        seq = entry.get("seq")
        content = entry.get("content") or {}
        where = {"seq": seq, "at": content.get("created_at"),
                 "log_id": content.get("id")}

        def broken(reason, detail):
            where.update(reason=reason, detail=detail)
            return False, dict(result, head_ok=False, **{"break": where})

        if not (is_hash(entry.get("prev_hash")) and is_hash(entry.get("content_hash"))
                and is_hash(entry.get("entry_hash"))):
            return broken("malformed", "a hash is not 64 lowercase hex characters")
        if not isinstance(seq, int) or seq <= 0:
            return broken("malformed", "sequence numbers start at 1")
        if content.get("tenant_id") != tenant:
            return broken("tenant_mismatch", "entry belongs to tenant %r, bundle is for %r"
                          % (content.get("tenant_id"), tenant))
        if prev_seq is not None and seq != prev_seq + 1:
            return broken("sequence_gap",
                          "sequence jumps from %d to %d: %d entries are unaccounted for"
                          % (prev_seq, seq, seq - prev_seq - 1))
        if entry["prev_hash"] != prev_hash:
            return broken("anchor_mismatch" if prev_seq is None else "chain_broken",
                          "prev_hash does not name the entry before this one")

        got = content_hash(content)
        if got != entry["content_hash"]:
            return broken("content_altered", "contents hash to %s, the chain committed to %s"
                          % (got, entry["content_hash"]))

        got = entry_hash(entry["prev_hash"], content["tenant_id"], seq, entry["content_hash"])
        if got != entry["entry_hash"]:
            return broken("entry_hash_mismatch", "recomputed %s, the entry claims %s"
                          % (got, entry["entry_hash"]))

        prev_hash = entry["entry_hash"]
        prev_seq = seq

    last = entries[-1]
    by_seq = {e.get("seq"): e.get("entry_hash") for e in entries}
    seals = bundle.get("seals") or []

    # Seals first: they are the strongest thing in the bundle, because they are
    # the only part the panel cannot revise. A seal inside the range must agree
    # with the entry at that sequence number.
    for seal in seals:
        seq = seal.get("seq")
        if seq in by_seq and by_seq[seq] != seal.get("entry_hash"):
            return False, dict(result, head_ok=False, **{"break": {
                "seq": seq, "reason": "seal_mismatch",
                "detail": "the %s seal recorded on %s committed to %s at this sequence "
                          "number; the bundle has %s"
                          % (seal.get("kind"), seal.get("created_at"),
                             seal.get("entry_hash"), by_seq[seq])}})

    # The tail, against two witnesses. The head is a mutable pointer: whoever
    # can delete the newest entries can move it to match. A seal cannot be
    # moved, so a seal naming a sequence beyond the end of a bundle that claims
    # to be complete is the finding the head would have hidden.
    # The exporter's own claim, not a guess. A bundle that ends before the head
    # is either a deliberate slice - exports are capped, so a long history comes
    # out as a series - or a chain whose newest entries were deleted, and the
    # file cannot tell those apart. Inferring it from the anchor would report
    # every chain longer than the export cap as tampered with.
    complete = bool(bundle.get("complete"))

    if head:
        if head.get("seq") != last.get("seq") or head.get("hash") != last.get("entry_hash"):
            result["head_ok"] = False
            if head.get("seq", 0) > last.get("seq", 0) and complete:
                return False, dict(result, **{"break": {
                    "seq": last["seq"] + 1,
                    "at": (last.get("content") or {}).get("created_at"),
                    "reason": "truncated_tail",
                    "detail": "the chain head is at %s but the bundle ends at %s"
                              % (head.get("seq"), last.get("seq"))}})

    for seal in seals:
        if (seal.get("seq", 0) > last.get("seq", 0) and complete
                and (not head or head.get("seq", 0) <= last.get("seq", 0))):
            return False, dict(result, head_ok=False, **{"break": {
                "seq": last["seq"] + 1,
                "at": (last.get("content") or {}).get("created_at"),
                "reason": "truncated_tail",
                "detail": "the %s seal recorded on %s says this chain reached sequence %s; "
                          "the bundle ends at %s and the head agrees with the bundle, so "
                          "entries that once existed have been removed and the head was "
                          "adjusted to match"
                          % (seal.get("kind"), seal.get("created_at"), seal.get("seq"),
                             last.get("seq"))}})

    return True, result


def main(argv):
    if len(argv) < 2 or argv[1] in ("-h", "--help"):
        print(__doc__.strip())
        return 2

    as_json = "--json" in argv[2:]
    with open(argv[1], "r", encoding="utf-8") as handle:
        bundle = json.load(handle)

    ok, result = verify(bundle)

    if as_json:
        print(json.dumps(dict(result, ok=ok), indent=2, sort_keys=True))
        return 0 if ok else 1

    if "error" in result:
        print("REFUSED: %s" % result["error"])
        return 1
    if ok:
        print("INTACT: %d entries, seq %s..%s, tenant %s"
              % (result["checked"], result.get("first_seq"), result.get("last_seq"),
                 bundle.get("tenant_id")))
        if not result["head_ok"]:
            print("NOTE: this bundle is a slice; it does not reach the chain head.")
        return 0

    where = result["break"]
    print("BROKEN at seq %s (written %s)" % (where.get("seq"), where.get("at")))
    print("  reason: %s" % where.get("reason"))
    print("  detail: %s" % where.get("detail"))
    print("  entries verified before the break: %s"
          % (where["seq"] - result["first_seq"] if result.get("first_seq") else 0))
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
