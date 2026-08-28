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
//	StepLock            take /vkai-panel/etc/upgrade.lock, recovering a stale one
//	StepCheck           fetch the feed, order versions, honour min_upgrade_from
//	StepDownload        fetch the tarball into /vkai-panel/tmp
//	StepVerify          sha256 against the manifest - nothing unverified is opened
//	StepStage           extract into releases/.staging-<version>-<pid>
//	StepPreflight       disk space, target absent, services healthy, dir writable
//	StepBackupDatabase  dump the database and record where it went
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
// # Injection
//
// Everything that touches the outside world is a field on Deps: the HTTP client,
// the command runner used for systemctl and pg_dump, the clock, the free-space
// probe and the "is this pid alive" probe. Config.Root moves the whole layout.
// The tests in this package run entirely against a temporary directory, a fake
// runner and a fake clock, and never open a socket or touch a real service.
package upgrade
