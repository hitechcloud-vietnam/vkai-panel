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

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// backupDatabase dumps the database and returns the path it wrote.
//
// A dump that fails aborts the upgrade. That is deliberate: an upgrade whose
// migrations run without a way back is exactly the situation that turns a bad
// release into lost customer data.
func (u *Upgrader) backupDatabase(ctx context.Context, from, to string) (string, error) {
	dir := u.DatabaseBackupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}

	stamp := u.deps.Clock.Now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("pre-upgrade-%s-to-%s-%s.dump",
		sanitizeForFilename(from), sanitizeForFilename(to), stamp)
	dest := dir + string(os.PathSeparator) + name

	args := make([]string, 0, len(u.cfg.Database.Args))
	for _, a := range u.cfg.Database.Args {
		a = strings.ReplaceAll(a, "{{dest}}", dest)
		a = strings.ReplaceAll(a, "{{database}}", u.cfg.Database.Name)
		args = append(args, a)
	}

	dumpCtx := ctx
	if u.cfg.Database.Timeout > 0 {
		var cancel context.CancelFunc
		dumpCtx, cancel = context.WithTimeout(ctx, u.cfg.Database.Timeout)
		defer cancel()
	}

	if _, err := u.deps.Runner.Run(dumpCtx, u.cfg.Database.Command, args...); err != nil {
		// A partial dump is worse than none: it looks like a backup.
		_ = os.Remove(dest)
		return "", fmt.Errorf("database backup failed, refusing to upgrade without one: %w", err)
	}

	// The command runner is injectable, so "it exited zero" is not by itself
	// proof that a file appeared. Verify what we are about to promise the
	// operator actually exists and is not empty.
	info, err := os.Stat(dest)
	if err != nil {
		return "", fmt.Errorf("database backup command reported success but %s was not created: %w", dest, err)
	}
	if info.Size() == 0 {
		_ = os.Remove(dest)
		return "", fmt.Errorf("database backup command reported success but %s is empty", dest)
	}

	return dest, nil
}
