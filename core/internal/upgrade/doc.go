// Package upgrade upgrades a running VKAI Panel installation in place.
//
// # Why this is not a shell script
//
// The panel runs on customer servers that nobody at HiTechCloud can log in to.
// An upgrade that half-succeeds there - a new release unpacked but the symlink
// still pointing at the old one, a database dump that was never taken, services
// restarted into a binary that will not start - is worse than having no upgrade
// feature at all, because the customer is now down and the operator cannot see
// why. This package therefore refuses to be one long function. It is a fixed
// sequence of named steps, each of which either completes or aborts the whole
// upgrade, with a single explicit rollback path and a callback so the CLI or the
// API can show the operator which step is running.
//
// # The installed layout it operates on
//
//	/vkai-panel/                      Config.Root
//	/vkai-panel/releases/<version>/   one directory per release
//	/vkai-panel/current -> releases/<version>   what the systemd units point at
//	/vkai-panel/etc/upgrade.lock      the "an upgrade is running" lock
//	/vkai-panel/etc/upgrade_state.json  current/previous version, last dump
//	/vkai-panel/www/backup/databases/ pre-upgrade database dumps
//	/vkai-panel/tmp/                  downloaded tarballs, deleted afterwards
//
// The current release is never written to. A new release is extracted into a
// staging directory next to it, promoted to its final name only after preflight
// passes, and becomes live only when the symlink is repointed - one rename, the
// single moment the installation changes.
//
// # The sequence
//
//	StepLock            take /vkai-panel/etc/upgrade.lock with flock(2)
//	StepCheck           fetch the feed, verify signatures, order versions,
//	                    honour min_upgrade_from across everything skipped
//	StepDownload        check disk and health first, then fetch the tarball
//	                    into /vkai-panel/tmp
//	StepVerify          sha256 against the manifest - nothing unverified is opened
//	StepStage           re-verify against the open file, then extract into
//	                    releases/.staging-<version>-<pid>
//	StepPreflight       disk space, target absent, services healthy, dir writable
//	StepBackupDatabase  dump the database, read the dump back, record where it went
//	StepSwitch          promote the staging directory, repoint current
//	StepRestart         restart vkai-api, vkai-ui, vkai-agent
//	StepHealthCheck     poll until healthy, bounded by Config.HealthTimeout
//	StepRollback        only on failure at or after StepSwitch
//	StepPrune           keep the newest Config.KeepReleases, never current/previous
//	StepCleanup         remove the tarball and any staging leftovers
//	StepDone
//
// # Rollback
//
// Rollback is attempted only for failures that happen once the symlink has
// moved, because that is the only window in which the installation is not
// already in its original state. It repoints the symlink at the previous
// release, restarts and health-checks again. If that itself fails the returned
// error is a *RollbackFailedError, which says in as many words that a human has
// to intervene: at that point the panel is down and this package has run out of
// safe moves.
//
// # This runs as root, so what it refuses matters more than what it does
//
// The upgrade unpacks an archive chosen by a remote server into a directory
// owned by root and then restarts the machine's services onto it. Every check
// below exists because its absence is a remote root write:
//
//   - The release tarball may contain regular files and directories, and
//     nothing else. Symlinks and hard links are refused outright rather than
//     validated, because a link target cannot be judged from the archive: the
//     kernel resolves symlinks before it applies "..", so a chain of links each
//     pointing at "." or ".." climbs one real directory per hop while passing
//     any check made by cleaning strings. See archive.go.
//   - Files are created O_CREATE|O_EXCL|O_NOFOLLOW and directories one
//     component at a time, so nothing is written through a link that was
//     already on disk.
//   - Modes come from this package. A member carrying setuid, setgid or the
//     sticky bit is refused, not stripped.
//   - The archive's sha256 is checked against the open file descriptor that is
//     about to be decompressed, in constant time, on every path into the
//     extractor.
//   - A version string is validated in full, build metadata included, because
//     it becomes a directory name under /vkai-panel/releases.
//   - min_upgrade_from is read from every release the upgrade would step over,
//     not only from the target, so a release cannot become installable from
//     anywhere by leaving the field out.
//   - The lock is a flock(2), so it cannot be broken by two upgrades that both
//     decide it is abandoned.
//
// # What this package does not defend against
//
// Stated plainly, because the alternative is an operator believing otherwise:
//
//   - An unsigned feed is trusted on TLS alone. With Config.ReleasePublicKeys
//     unset - the default - whoever can answer for the feed URL chooses what
//     this machine installs as root: its certificate, its CA, its DNS, its CDN
//     and everyone with write access to the bucket behind it are all in the
//     trust boundary. Setting ReleasePublicKeys removes all of that, and
//     publishing signed manifests is the single highest-value change left in
//     this package. It is off by default only because turning it on before the
//     release tooling signs anything would stop every installation upgrading.
//   - A signature proves who published a release, not that the release is good.
//     A compromised build pipeline signs a backdoor exactly as well as it signs
//     a release.
//   - The version numbers are the feed's to choose. Refusing a downgrade is not
//     the same as refusing a hostile upgrade: a feed can always publish a
//     larger number.
//   - The database dump is proved to be a complete, parseable dump. Nothing
//     here proves it restores into a working panel, and nothing here tests the
//     restore path.
//   - The disk check before the download uses the manifest's declared size and
//     an estimate of how far it expands. A release that expands much further
//     than that can still fill the disk during extraction; that aborts the
//     upgrade with the installation untouched, but it does fill the disk.
//   - Staging directories left by a process that died are removed only by the
//     process that created them, so debris from a crashed upgrade with a
//     different pid survives until the next run of that pid or an operator.
//
// # Injection
//
// Everything that touches the outside world is a field on Deps: the HTTP client,
// the command runner used for systemctl and pg_dump, the clock, the free-space
// probe and the "is this pid alive" probe. Config.Root moves the whole layout.
// The tests in this package run entirely against a temporary directory, a fake
// runner and a fake clock, and never open a socket or touch a real service.
package upgrade
