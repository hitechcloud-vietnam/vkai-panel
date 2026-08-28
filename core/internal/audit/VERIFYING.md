# Verifying a vKAI Panel audit log export

This document is the whole specification. Everything needed to check a bundle is
here: no part of the vKAI Panel source, no access to the panel, no access to its
database, no network. If the panel's own code and this document ever disagree,
this document is what the format is.

`verify_audit_export.py`, shipped alongside this file, is a reference
implementation in Python 3 using only the standard library. It is a convenience,
not the definition; write your own from this document if you would rather not
trust a script the audited party handed you. That is the point of publishing the
procedure.

## What the bundle claims

A bundle is one JSON document. It covers one tenant and a contiguous range of
audit entries.

```
{
  "format":         "vkai-audit-export-v1",
  "hash_algorithm": "SHA-256",
  "content_domain": "vkai-audit-content-v1",
  "chain_domain":   "vkai-audit-chain-v1",
  "genesis_hash":   "000...0",              64 zeros
  "tenant_id":      "<uuid>",
  "exported_at":    "2026-08-28T07:15:40.238782Z",
  "anchor":  { "kind": "genesis" | "seal" | "entry" | "none", "seq": N,
               "hash": "<hex>", "seal_id": "<uuid>", "sealed_at": "..." },
  "head":    { "seq": N, "hash": "<hex>" },
  "complete": true,
  "seals":   [ { "id": "<uuid>", "kind": "checkpoint" | "export" | "prune",
                 "seq": N, "entry_hash": "<hex>", "created_at": "..." } ],
  "entry_count": N,
  "entries": [
    { "seq": 1,
      "prev_hash":    "<hex>",
      "content_hash": "<hex>",
      "entry_hash":   "<hex>",
      "content": { "id": "...", "tenant_id": "...", "user_id": "",
                   "action": "...", "resource": "...", "resource_id": "",
                   "details": "{\"k\": \"v\"}", "ip_address": "...",
                   "user_agent": "...", "status": "...",
                   "created_at": "2026-08-28T07:15:40.238782Z" } },
    ...
  ]
}
```

Every value under `content` is a string, and every one of them is hashed exactly
as it appears. In particular `details` is already serialised: hash the string,
do not parse it and re-serialise it. A JSON parser that reorders keys or
normalises numbers would change the bytes and the hash with them, which is why
the bundle carries the rendering rather than the object.

An absent value is the empty string, never `null`.

## The two hashes

Write `F(s)` for the framing of one field: the 8-byte big-endian unsigned length
of the **UTF-8 encoding** of `s`, followed by those bytes. Framing every field
is what stops a value containing a separator from impersonating a field
boundary — there is no separator to imitate.

    content_hash = hex(SHA-256(
        F("vkai-audit-content-v1")
        F(content.id)          F(content.tenant_id)   F(content.user_id)
        F(content.action)      F(content.resource)    F(content.resource_id)
        F(content.details)     F(content.ip_address)  F(content.user_agent)
        F(content.status)      F(content.created_at)))

    entry_hash = hex(SHA-256(
        F("vkai-audit-chain-v1")
        F(prev_hash)  F(content.tenant_id)  F(str(seq))  F(content_hash)))

`hex` is lowercase. `str(seq)` is decimal ASCII with no padding and no sign;
sequence numbers start at 1.

### Test vectors

Check an implementation against these before pointing it at real data. A
mismatch here is a bug in the implementation; a mismatch on real data is a
finding.

The framing of a single field:

    F("abc")          = 00 00 00 00 00 00 00 03  61 62 63
    SHA-256(F("abc")) = c3494ca1a2cf8eeb8a11ded316fb55b83c3bbbedb6313cd50415251e5d09e12f

A whole entry:

    content = {
      "id":          "00000000-0000-4000-8000-000000000001",
      "tenant_id":   "00000000-0000-4000-8000-0000000000ff",
      "user_id":     "",
      "action":      "auth.sign_in",
      "resource":    "session",
      "resource_id": "",
      "details":     "{\"k\": \"v\"}",
      "ip_address":  "203.0.113.1",
      "user_agent":  "curl/8.0",
      "status":      "success",
      "created_at":  "2026-01-02T03:04:05.000006Z"
    }

    content_hash = cd80a44d963564358eafbcb8755c91e26eb65371c665997460dbec356f69abcc
    entry_hash   = 09ffa0fed83bc77bb51d6c0b9e9b2a55ef11a92eaecc17f5f904ad6ea25accf7
                   (at seq 1, prev_hash = 64 zeros)

Note `details`: it is a JSON *string* containing JSON. Hash the string. An
implementation that parses and re-serialises it will produce a different hash
and will disagree with every panel in the field.

## The procedure

1. Check `format`, `content_domain` and `chain_domain` are the strings above. A
   bundle written under a different format version must not be verified with
   these rules.
2. Set `prev` to `anchor.hash`. If the first entry's `seq` is 1 and
   `anchor.kind` is `"genesis"`, set `prev` to 64 zeros instead.
3. For each entry, in the order the array gives them:
   1. every hash is 64 lowercase hex characters, and `seq` is a positive
      integer — otherwise **malformed**;
   2. `content.tenant_id` equals the bundle's `tenant_id` — otherwise
      **tenant_mismatch**;
   3. for every entry after the first, `seq` is exactly one more than the
      previous entry's — otherwise **sequence_gap**, and the difference is the
      number of entries that were removed;
   4. `prev_hash` equals `prev` — otherwise **chain_broken** (**anchor_mismatch**
      for the first entry);
   5. recomputing `content_hash` from `content` reproduces the recorded value —
      otherwise **content_altered**: this entry was edited;
   6. recomputing `entry_hash` from `prev_hash`, `tenant_id`, `seq` and
      `content_hash` reproduces the recorded value — otherwise
      **entry_hash_mismatch**;
   7. set `prev` to this entry's `entry_hash` and continue.
4. For every seal whose `seq` falls inside the exported range, `entry_hash` must
   equal the entry at that sequence number — otherwise **seal_mismatch**: the
   entry was altered after it was sealed. This is the strongest check in the
   procedure, because a seal is the one part of the bundle the audited party
   could not revise afterwards.
5. If `head` is present and `complete` is true, the last entry's `seq` and
   `entry_hash` must equal `head.seq` and `head.hash` — otherwise
   **truncated_tail**. For a bundle that is a slice (`complete` false), a head
   further along is expected and is not a break.

   `complete` is the exporter's own claim that the bundle holds the whole chain
   from its start to its tip. It has to be a claim rather than something you
   work out: a bundle that stops short of the head is either a slice — exports
   are capped, so a long history arrives as a series — or a chain whose newest
   entries were deleted, and nothing inside one file separates those. An
   exporter that sets it false gets no tail checks, and gets a bundle that says
   in writing that it is a fragment. Ask for the rest.
6. Independently of the head: if any seal names a `seq` beyond the last entry of
   a bundle whose `complete` is true, that is **truncated_tail** whatever the
   head says. The head is a mutable pointer the panel rewrites on every entry,
   so an attacker who removes the newest entries can move it to match; a seal
   lives in a table that refuses UPDATE and DELETE and cannot be moved. This
   check is the one that survives a competent attacker.

Stop at the first failure and report its `seq` and its `content.created_at`.
Everything before it verified; nothing after it has been checked.

## What a clean result does and does not prove

It proves that the entries in the bundle are the entries that were written, in
the order they were written, and that none was removed from the middle of the
range.

It does not prove that entries were never *withheld from the export*. The chain
cannot know about an entry that was never inserted. Two things narrow that:

* `anchor.kind` — `"genesis"` means the bundle starts at the beginning of the
  tenant's history, so nothing before it is missing. `"seal"` means older
  entries were deliberately pruned under a retention policy, and `seal_id` and
  `sealed_at` identify the record of that prune, which the operator must be able
  to produce.
* `head` — the tip of the live chain at export time. Take exports periodically
  and keep them: the `head` of an older export must appear as an ordinary entry
  hash inside a newer one. A panel that cannot show you that has rewritten
  history between the two exports.
* `seals` — and keep these especially. Their sequence numbers must only ever go
  up between one export and the next, and a seal you were given in an earlier
  export must still be present, with the same hash, in a later one. A seal that
  has disappeared or changed is not a subtle discrepancy: seals live in a table
  the panel cannot rewrite, so it means somebody has been in the database
  directly.

An entry that was never written at all is outside what any hash chain can
detect, and no claim to the contrary is made here.
