package upgrade

// The pre-upgrade database dump.
//
// This runs before anything is switched, and its path is recorded in the result
// and in the state file. That path is the single most useful piece of
// information in the worst case: when a release turns out to have corrupted
// data in a way the health check cannot see, or when the rollback fails and an
// engineer has to rebuild the machine by hand, the first question is always
// "where is the dump", and the answer has to be somewhere they can read without
// the panel running.
//
// # Created is not the same as restorable
//
// The dump used to be accepted on two facts: the command exited zero, and a
// non-empty file appeared. Neither says the file can be restored. pg_dump can
// be killed by the OOM killer after writing a prefix and still leave a
// plausible-looking file; a full disk truncates the last write; a
// misconfiguration can produce a dump of the wrong database. Every one of those
// is discovered at restore time, which is the worst possible moment.
//
// So the dump is read back before the upgrade proceeds. pg_restore --list parses
// the archive's table of contents - the whole file, including its trailer -
// without connecting to a database and without writing anything. It does not
// prove the data inside is what the operator wants; it does prove the file is a
// complete, parseable dump rather than a fragment.

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// backupResult is what a successful pre-upgrade dump produced.
type backupResult struct {
	// Path is the dump file.
	Path string
	// SHA256 is its digest as written, so the operator restoring it later
	// can tell whether they are holding the same file.
	SHA256 string
	// Verified is true when VerifyCommand read the dump back successfully.
	Verified bool
}

// backupDatabase dumps the database, proves the dump is readable, and returns
// what it wrote.
//
// A dump that fails aborts the upgrade. That is deliberate: an upgrade whose
// migrations run without a way back is exactly the situation that turns a bad
// release into lost customer data. A dump that cannot be read back aborts it
// for the same reason - a backup nothing has opened is a hope, not a backup.
func (u *Upgrader) backupDatabase(ctx context.Context, from, to string) (backupResult, error) {
	var res backupResult

	dir := u.DatabaseBackupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return res, fmt.Errorf("create %s: %w", dir, err)
	}

	stamp := u.deps.Clock.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("pre-upgrade-%s-to-%s-%s.dump",
		sanitizeForFilename(from), sanitizeForFilename(to), stamp)
	dest := dir + string(os.PathSeparator) + name

	dumpCtx := ctx
	if u.cfg.Database.Timeout > 0 {
		var cancel context.CancelFunc
		dumpCtx, cancel = context.WithTimeout(ctx, u.cfg.Database.Timeout)
		defer cancel()
	}

	if _, err := u.deps.Runner.Run(dumpCtx, u.cfg.Database.Command, u.databaseArgs(u.cfg.Database.Args, dest)...); err != nil {
		// A partial dump is worse than none: it looks like a backup.
		_ = os.Remove(dest)
		return res, fmt.Errorf("database backup failed, refusing to upgrade without one: %w", err)
	}

	// The command runner is injectable, so "it exited zero" is not by itself
	// proof that a file appeared. Verify what we are about to promise the
	// operator actually exists and is not empty.
	info, err := os.Stat(dest)
	if err != nil {
		return res, fmt.Errorf("database backup command reported success but %s was not created: %w", dest, err)
	}
	if info.Size() == 0 {
		_ = os.Remove(dest)
		return res, fmt.Errorf("database backup command reported success but %s is empty", dest)
	}
	res.Path = dest

	if cmd := strings.TrimSpace(u.cfg.Database.VerifyCommand); cmd != "" && !u.cfg.Database.SkipVerify {
		if out, err := u.deps.Runner.Run(dumpCtx, cmd, u.databaseArgs(u.cfg.Database.VerifyArgs, dest)...); err != nil {
			// Keep the file: an unreadable dump is still evidence, and
			// deleting it would remove the only thing an engineer has
			// to look at.
			return res, fmt.Errorf("the database dump at %s could not be read back by %s, so it is not a backup: %w: %s",
				dest, cmd, err, strings.TrimSpace(string(out)))
		}
		res.Verified = true
	}

	sum, err := fileSHA256(dest)
	if err != nil {
		return res, fmt.Errorf("hash the database dump %s: %w", dest, err)
	}
	res.SHA256 = sum
	return res, nil
}

// databaseArgs substitutes the placeholders the dump and verify commands share.
func (u *Upgrader) databaseArgs(args []string, dest string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.ReplaceAll(a, "{{dest}}", dest)
		a = strings.ReplaceAll(a, "{{database}}", u.cfg.Database.Name)
		out = append(out, a)
	}
	return out
}
