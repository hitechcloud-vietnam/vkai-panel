// Package backup implements the offsite backup engine: archiving, encryption,
// upload to a destination, restore, and the restorability check.
//
// The package is deliberately free of database and HTTP dependencies. It works
// on io.Reader, io.Writer and directories, so every guarantee it makes can be
// driven end to end by a test without a server, a bucket or a Postgres. The
// database rows that record what happened live in internal/repository, and the
// endpoints live in internal/handler; both call into here.
//
// The shape of the thing:
//
//	source tree ─┐
//	             ├─ scan ──> Manifest (path, size, mode, sha256 per file)
//	             ├─ tar ───> gzip ───> [encrypt] ───> Destination.Put
//	             └─ Progress on every stage, cancellable through the context
//
//	Destination.Get ──> [decrypt] ──> gunzip ──> untar ──> plan ──> apply
//
// Three properties the rest of the panel depends on:
//
//   - The manifest is written INSIDE the archive as its first entry, so it
//     travels with the data. Verification recomputes every checksum from the
//     bytes it actually extracted and compares against that manifest; it never
//     trusts a checksum recorded elsewhere in the panel database, because a
//     backup whose only proof of integrity is a row in the database it was
//     meant to protect is not proof of anything.
//
//   - Encryption happens before the bytes leave the process. A Destination
//     never sees plaintext when a key is configured.
//
//   - A restore is planned before it is applied. ExtractArchive with DryRun
//     set writes nothing and returns exactly what a real run would overwrite.
package backup
