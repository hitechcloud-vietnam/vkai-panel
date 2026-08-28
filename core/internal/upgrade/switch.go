package upgrade

// The switch, the health check, and the rollback.
//
// Everything before this file is reversible by deleting a directory. From the
// moment the symlink moves, the installation is changed and the only way back
// is the rollback in this file. So the switch is made as small as it can be:
// one rename of a symlink, which the kernel performs atomically - there is no
// instant at which /vkai-panel/current does not exist or points at nothing.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// promote moves the staging directory to its final name. It is the last
// preparatory act and still changes nothing about what is running.
func (u *Upgrader) promote(staging, releaseDir string) error {
	if _, err := os.Lstat(releaseDir); err == nil {
		return fmt.Errorf("cannot promote %s: %s already exists", staging, releaseDir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", releaseDir, err)
	}
	if err := os.Rename(staging, releaseDir); err != nil {
		return fmt.Errorf("promote %s to %s: %w", staging, releaseDir, err)
	}
	return nil
}

// pointCurrentAt repoints /vkai-panel/current at a release directory.
//
// The symlink is written under a temporary name and renamed over the real one,
// because os.Symlink cannot replace an existing link and "remove then create"
// leaves a window in which the panel has no current release at all - a window a
// service restart or a monitoring probe will find.
//
// The stored target is relative ("releases/1.2.3") so that the whole
// installation can be moved or bind-mounted without every symlink breaking.
func (u *Upgrader) pointCurrentAt(releaseDir string) error {
	link := u.CurrentLink()
	target, err := filepath.Rel(filepath.Dir(link), releaseDir)
	if err != nil {
		// Different volumes, or something equally unusual. An absolute
		// target still works; only relocatability is lost.
		target = releaseDir
	}

	tmp := link + ".swap"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear %s: %w", tmp, err)
	}
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("create %s -> %s: %w", tmp, target, err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("move %s into place as %s: %w", tmp, link, err)
	}
	return nil
}

// restartServices restarts each unit in order, stopping at the first failure so
// the error names the unit that would not come back.
func (u *Upgrader) restartServices(ctx context.Context) error {
	for _, svc := range u.cfg.Services {
		if _, err := u.deps.Runner.Run(ctx, "systemctl", "restart", svc); err != nil {
			return fmt.Errorf("restart %s: %w", svc, err)
		}
	}
	return nil
}

// healthCheck reports whether the installation is serving.
//
// The default is "systemctl says every unit is active, and - if HealthURL is
// configured - the panel answers a GET with a 2xx". The systemctl half catches
// a binary that will not start; the HTTP half catches one that starts and then
// fails to open its listener or its database connection, which systemd is
// perfectly happy with.
func (u *Upgrader) healthCheck(ctx context.Context) error {
	if u.cfg.HealthCheck != nil {
		return u.cfg.HealthCheck(ctx)
	}
	for _, svc := range u.cfg.Services {
		if _, err := u.deps.Runner.Run(ctx, "systemctl", "is-active", "--quiet", svc); err != nil {
			return fmt.Errorf("service %s is not active: %w", svc, err)
		}
	}
	if strings.TrimSpace(u.cfg.HealthURL) == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.HealthURL, nil)
	if err != nil {
		return fmt.Errorf("build health request for %s: %w", u.cfg.HealthURL, err)
	}
	req.Header.Set("User-Agent", u.cfg.UserAgent)
	resp, err := u.deps.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("health probe %s: %w", u.cfg.HealthURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("health probe %s returned HTTP %d", u.cfg.HealthURL, resp.StatusCode)
	}
	return nil
}

// waitHealthy polls healthCheck until it passes or Config.HealthTimeout runs
// out, and reports the last failure it saw. The timeout is what makes a hung
// service a rollback rather than an upgrade that never returns.
func (u *Upgrader) waitHealthy(ctx context.Context) error {
	deadline := u.deps.Clock.Now().Add(u.cfg.HealthTimeout)
	attempts := 0
	var last error

	for {
		attempts++
		last = u.healthCheck(ctx)
		if last == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("health check abandoned after %d attempts (%v): %w", attempts, last, err)
		}
		if !u.deps.Clock.Now().Add(u.cfg.HealthInterval).Before(deadline) {
			break
		}
		u.deps.Clock.Sleep(u.cfg.HealthInterval)
	}

	return fmt.Errorf("services did not become healthy within %s (%d attempts): %w",
		u.cfg.HealthTimeout, attempts, last)
}

// rollback puts previousRelease back and confirms it is serving.
//
// It repeats the whole switch - symlink, restart, health check - because a
// rollback that only moves the symlink leaves the failed release running in
// memory, which looks fine to a symlink check and is still down.
func (u *Upgrader) rollback(ctx context.Context, previousRelease string) error {
	if strings.TrimSpace(previousRelease) == "" {
		return ErrNoPreviousRelease
	}
	if _, err := os.Stat(previousRelease); err != nil {
		return fmt.Errorf("previous release %s is not usable: %w", previousRelease, err)
	}
	if err := u.pointCurrentAt(previousRelease); err != nil {
		return fmt.Errorf("repoint %s at %s: %w", u.CurrentLink(), previousRelease, err)
	}
	if err := u.restartServices(ctx); err != nil {
		return fmt.Errorf("restart services on %s: %w", previousRelease, err)
	}
	if err := u.waitHealthy(ctx); err != nil {
		return fmt.Errorf("previous release %s did not come back healthy: %w", previousRelease, err)
	}
	return nil
}
