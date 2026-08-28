package phpfpm

// The manager: install PHP versions, write pool files, validate them, reload
// FPM, and put everything back when the reload fails.
//
// The rule this file exists to enforce is the third requirement of the task,
// and it is worth restating because it is the one that costs a customer money:
// a panel that leaves a site down after a settings change is worse than a panel
// with no settings. So every mutation here is a transaction over the
// filesystem:
//
//	1. capture the current pool file (its bytes, or the fact that it is absent)
//	2. write the new one atomically
//	3. ask php-fpm to parse the whole configuration (`php-fpm -t`)
//	4. reload the service
//	5. prove the service came back
//
// A failure at 3, 4 or 5 restores step 1 exactly - including deleting a file
// that did not exist before - and reloads again, so the site is serving the
// configuration it was serving a second ago. Only then is the error returned.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// Manager owns the PHP installation on one host.
type Manager struct {
	distro  Distro
	runner  Runner
	logger  *zap.Logger
	systemd string

	// rootDir prefixes every filesystem path. It is "" in production and a
	// temporary directory in tests, which is what makes the whole write ->
	// validate -> reload -> roll back path testable without root.
	rootDir string
}

// Options configures a Manager.
type Options struct {
	// Distro is the detected host. Zero value means "detect now".
	Distro *Distro
	// Runner executes external commands. Zero value means the real one.
	Runner Runner
	// Logger is required in production; a nil logger becomes a no-op.
	Logger *zap.Logger
	// RootDir prefixes every path. Tests set it; production leaves it empty.
	RootDir string
	// SystemctlPath overrides the systemctl binary, for tests.
	SystemctlPath string
}

// New builds a Manager, detecting the distribution if the caller did not.
//
// Detection failing is NOT fatal here, and that is deliberate: a panel whose
// PHP management refuses to construct is a panel where every PHP route 500s
// with a stack trace. Instead the manager records the failure and every
// operation that needs the distribution refuses with that reason, by name.
func New(opts Options) (*Manager, error) {
	distro := Distro{}
	if opts.Distro != nil {
		distro = *opts.Distro
	} else {
		detected, err := DetectDistro()
		if err != nil {
			return nil, err
		}
		distro = detected
	}

	runner := opts.Runner
	if runner == nil {
		runner = NewExecRunner()
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	systemctl := opts.SystemctlPath
	if systemctl == "" {
		systemctl = "systemctl"
	}

	return &Manager{
		distro:  distro,
		runner:  runner,
		logger:  logger,
		systemd: systemctl,
		rootDir: opts.RootDir,
	}, nil
}

// Distro is the detected host, for the capability report.
func (m *Manager) Distro() Distro { return m.distro }

// path prefixes an absolute layout path with the manager's root.
func (m *Manager) path(p string) string {
	if m.rootDir == "" {
		return p
	}
	return filepath.Join(m.rootDir, p)
}

// Layout resolves the layout for a version on this host.
func (m *Manager) Layout(version string) (Layout, error) {
	return LayoutFor(m.distro, version)
}

// ---------------------------------------------------------------------------
// Installing versions
// ---------------------------------------------------------------------------

// InstallVersion installs one PHP version and its FPM package.
//
// On a distribution with no side-by-side repository this refuses with
// *ErrMultiVersionUnsupported and installs nothing. It does not fall back to
// "install the system PHP and call it 8.3", which would leave the panel's
// database claiming a version the host does not have.
func (m *Manager) InstallVersion(ctx context.Context, version string, extensions []string) error {
	if err := ValidateVersion(version); err != nil {
		return err
	}
	if !m.distro.SupportsMultiVersion() {
		return &ErrMultiVersionUnsupported{Distro: m.distro}
	}
	layout, err := m.Layout(version)
	if err != nil {
		return err
	}
	for _, ext := range extensions {
		if !extensionRe.MatchString(ext) {
			return fmt.Errorf("invalid extension name %q", ext)
		}
	}

	if err := m.ensureRepository(ctx); err != nil {
		return fmt.Errorf("could not enable the PHP repository: %w", err)
	}

	packages := []string{layout.Package, layout.ExtensionPackagePrefix + "cli"}
	for _, ext := range extensions {
		packages = append(packages, layout.ExtensionPackage(ext))
	}

	if err := m.installPackages(ctx, packages); err != nil {
		return fmt.Errorf("could not install PHP %s: %w", version, err)
	}

	if _, err := m.runner.Run(ctx, m.systemd, "enable", "--now", layout.Service); err != nil {
		return fmt.Errorf("PHP %s installed but its FPM service would not start: %w", version, err)
	}

	m.logger.Info("PHP version installed",
		zap.String("version", version),
		zap.String("service", layout.Service),
		zap.String("pool_dir", layout.PoolDir),
		zap.Strings("extensions", extensions),
	)
	return nil
}

// RemoveVersion removes a PHP version's packages. It refuses while any pool
// file for that version is still present, because removing the package under a
// live pool takes every site on it down at once.
func (m *Manager) RemoveVersion(ctx context.Context, version string) error {
	if err := ValidateVersion(version); err != nil {
		return err
	}
	if !m.distro.SupportsMultiVersion() {
		return &ErrMultiVersionUnsupported{Distro: m.distro}
	}
	layout, err := m.Layout(version)
	if err != nil {
		return err
	}

	pools, err := m.ListPools(version)
	if err != nil {
		return err
	}
	if len(pools) > 0 {
		return fmt.Errorf("PHP %s still has %d pool(s) (%s): move those sites to another "+
			"version first, or they go down when the package is removed",
			version, len(pools), strings.Join(pools, ", "))
	}

	if _, err := m.runner.Run(ctx, m.systemd, "disable", "--now", layout.Service); err != nil {
		m.logger.Warn("could not stop the FPM service before removing it",
			zap.String("service", layout.Service), zap.Error(err))
	}
	if err := m.removePackages(ctx, []string{layout.Package}); err != nil {
		return fmt.Errorf("could not remove PHP %s: %w", version, err)
	}

	m.logger.Info("PHP version removed", zap.String("version", version))
	return nil
}

// EnsureExtensions installs the distribution packages for a set of extensions
// on one PHP version and restarts that version's FPM.
//
// This is where "the extensions enabled" is really enforced: a PHP extension is
// loaded by the FPM master process, so it is a property of the version, not of
// a pool. See PoolSpec.renderExtensions.
//
// It restarts rather than reloads: a reload re-reads pool files, it does not
// load a new module into a running master.
func (m *Manager) EnsureExtensions(ctx context.Context, version string, extensions []string) error {
	if err := ValidateVersion(version); err != nil {
		return err
	}
	layout, err := m.Layout(version)
	if err != nil {
		return err
	}
	if len(extensions) == 0 {
		return nil
	}
	packages := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if !extensionRe.MatchString(ext) {
			return fmt.Errorf("invalid extension name %q", ext)
		}
		packages = append(packages, layout.ExtensionPackage(ext))
	}
	if !m.distro.SupportsMultiVersion() && len(packages) > 0 {
		// The packages exist on Leap and AL2023 too; only the *versioned*
		// package names do not. Refuse rather than install the wrong thing.
		return &ErrMultiVersionUnsupported{Distro: m.distro}
	}
	if err := m.installPackages(ctx, packages); err != nil {
		return fmt.Errorf("could not install extensions for PHP %s: %w", version, err)
	}
	if _, err := m.runner.Run(ctx, m.systemd, "restart", layout.Service); err != nil {
		return fmt.Errorf("extensions installed but PHP %s would not restart: %w", version, err)
	}
	m.logger.Info("PHP extensions installed",
		zap.String("version", version), zap.Strings("extensions", extensions))
	return nil
}

func (m *Manager) ensureRepository(ctx context.Context) error {
	switch m.distro.Family {
	case FamilyDebian:
		// The sury repository is added by deploy/install.sh; refreshing the
		// index is all that is needed here, and it is the step whose absence
		// makes an install fail with "no candidate version".
		_, err := m.runner.Run(ctx, "apt-get", "update", "-o", "DPkg::Lock::Timeout=600")
		return err
	case FamilyRHEL:
		// Remi is enabled by the installer. dnf resolves the module stream
		// itself; nothing to do beyond a metadata refresh.
		_, err := m.runner.Run(ctx, "dnf", "-y", "makecache")
		return err
	default:
		return &ErrMultiVersionUnsupported{Distro: m.distro}
	}
}

func (m *Manager) installPackages(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	switch m.distro.Family {
	case FamilyDebian:
		args := append([]string{"install", "-y", "--no-install-recommends",
			"-o", "DPkg::Lock::Timeout=600"}, packages...)
		_, err := m.runner.Run(ctx, "apt-get", args...)
		return err
	case FamilyRHEL:
		args := append([]string{"install", "-y"}, packages...)
		_, err := m.runner.Run(ctx, "dnf", args...)
		return err
	case FamilySUSE:
		args := append([]string{"--non-interactive", "install", "--no-recommends"}, packages...)
		_, err := m.runner.Run(ctx, "zypper", args...)
		return err
	default:
		return fmt.Errorf("no package manager is known for %s", m.distro.Pretty)
	}
}

func (m *Manager) removePackages(ctx context.Context, packages []string) error {
	switch m.distro.Family {
	case FamilyDebian:
		args := append([]string{"remove", "-y", "-o", "DPkg::Lock::Timeout=600"}, packages...)
		_, err := m.runner.Run(ctx, "apt-get", args...)
		return err
	case FamilyRHEL:
		args := append([]string{"remove", "-y"}, packages...)
		_, err := m.runner.Run(ctx, "dnf", args...)
		return err
	case FamilySUSE:
		args := append([]string{"--non-interactive", "remove"}, packages...)
		_, err := m.runner.Run(ctx, "zypper", args...)
		return err
	default:
		return fmt.Errorf("no package manager is known for %s", m.distro.Pretty)
	}
}

// InstalledVersions reports which of the supported versions actually have an
// FPM binary on this host. It reads the filesystem rather than the database,
// because the database is what was believed and the filesystem is what is true.
func (m *Manager) InstalledVersions() []string {
	var found []string
	for _, version := range SupportedVersions() {
		layout, err := m.Layout(version)
		if err != nil {
			continue
		}
		if _, err := os.Stat(m.path(layout.Binary)); err == nil {
			found = append(found, version)
		}
	}
	return found
}

// ListPools returns the names of the pool files present for one version.
func (m *Manager) ListPools(version string) ([]string, error) {
	layout, err := m.Layout(version)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.path(layout.PoolDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read the pool directory for PHP %s: %w", version, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".conf"))
	}
	return names, nil
}

// ---------------------------------------------------------------------------
// Applying a pool: the transaction
// ---------------------------------------------------------------------------

// ApplyResult describes what a successful apply did, so the caller can log and
// return something specific rather than "ok".
type ApplyResult struct {
	// PoolPath is the file that was written.
	PoolPath string
	// SocketPath is the listen socket sites are pointed at.
	SocketPath string
	// Service is the systemd unit that was reloaded.
	Service string
	// PreviousVersion is set when this apply moved a pool between versions.
	PreviousVersion string
	// Created is true when this pool did not exist before.
	Created bool
}

// ApplyPool writes, validates and reloads one pool, rolling back on failure.
//
// previousVersion, when non-empty and different from spec.Version, means this
// is a version switch: the pool file is removed from the old version's
// directory and both services are reloaded. A failure anywhere puts the old
// file back and reloads the old service, so the site keeps serving.
func (m *Manager) ApplyPool(ctx context.Context, spec *PoolSpec, previousVersion string) (*ApplyResult, error) {
	layout, err := m.Layout(spec.Version)
	if err != nil {
		return nil, err
	}
	spec.SocketPath = layout.SocketPath(spec.Name)

	content, err := spec.Render()
	if err != nil {
		return nil, err
	}

	poolPath := m.path(layout.PoolPath(spec.Name))

	// Step 1: capture what is there now.
	before, existed, err := readIfExists(poolPath)
	if err != nil {
		return nil, err
	}

	// A version switch also has to capture the pool file on the OLD version,
	// because rolling back means putting that one back too.
	var (
		oldLayout   Layout
		oldPoolPath string
		oldBefore   []byte
		oldExisted  bool
		switching   = previousVersion != "" && previousVersion != spec.Version
	)
	if switching {
		if oldLayout, err = m.Layout(previousVersion); err != nil {
			return nil, fmt.Errorf("the pool's current PHP version is unusable: %w", err)
		}
		oldPoolPath = m.path(oldLayout.PoolPath(spec.Name))
		if oldBefore, oldExisted, err = readIfExists(oldPoolPath); err != nil {
			return nil, err
		}
	}

	// restore puts the filesystem back exactly as it was and reloads whatever
	// needs reloading. It is called from every failure path below.
	// Two kinds of rollback problem, and they mean opposite things to an
	// operator, so they are never collapsed into one list:
	//
	//   fileProblems  the previous pool file could NOT be put back. This is the
	//                 severe one: the master is holding a configuration that no
	//                 longer matches what is on disk, and the next reload from
	//                 any source serves something nobody chose. The site may go
	//                 down without anybody touching it again.
	//
	//   reloadProblems the file is back but php-fpm would not reload. This is
	//                 NOT a site outage: a reload that fails leaves the running
	//                 master exactly as it was, and the master was serving the
	//                 previous configuration - which is what the rollback wanted
	//                 in the first place. It still needs an operator's attention,
	//                 because the service is unhappy, but telling them the site
	//                 is down when it is serving is how a panel trains people to
	//                 ignore its errors.
	restore := func(cause error) error {
		var fileProblems, reloadProblems []string

		if err := writeOrRemove(poolPath, before, existed); err != nil {
			fileProblems = append(fileProblems, err.Error())
		}
		if switching {
			if err := writeOrRemove(oldPoolPath, oldBefore, oldExisted); err != nil {
				fileProblems = append(fileProblems, err.Error())
			}
			if _, err := m.reload(ctx, oldLayout); err != nil {
				reloadProblems = append(reloadProblems, fmt.Sprintf("reloading %s: %v", oldLayout.Service, err))
			}
		}
		if _, err := m.reload(ctx, layout); err != nil {
			reloadProblems = append(reloadProblems, fmt.Sprintf("reloading %s: %v", layout.Service, err))
		}

		if len(fileProblems) > 0 {
			m.logger.Error("pool rollback did not restore the previous pool file",
				zap.String("pool", spec.Name),
				zap.String("pool_file", poolPath),
				zap.Strings("problems", append(fileProblems, reloadProblems...)),
				zap.Error(cause))
			return fmt.Errorf("%w; the rollback then failed as well, so this site may be "+
				"serving no PHP at all - check %s: %s", cause, poolPath,
				strings.Join(append(fileProblems, reloadProblems...), "; "))
		}

		if len(reloadProblems) > 0 {
			m.logger.Error("pool change rolled back on disk, but php-fpm would not reload; "+
				"the running master still holds the previous configuration, so the site is "+
				"serving, and the service needs attention",
				zap.String("pool", spec.Name),
				zap.String("pool_file", poolPath),
				zap.Strings("problems", reloadProblems),
				zap.Error(cause))
			return fmt.Errorf("%w (rolled back: the previous pool file is restored and this site "+
				"is serving its previous configuration, but php-fpm would not reload afterwards "+
				"either, so the service needs attention: %s)", cause, strings.Join(reloadProblems, "; "))
		}

		m.logger.Warn("pool change rolled back; the site is serving its previous configuration",
			zap.String("pool", spec.Name),
			zap.String("pool_file", poolPath),
			zap.Error(cause))
		return fmt.Errorf("%w (rolled back: this site is serving its previous configuration)", cause)
	}

	// Step 2: write the new file.
	if err := os.MkdirAll(filepath.Dir(poolPath), 0o755); err != nil {
		return nil, fmt.Errorf("cannot create the pool directory: %w", err)
	}
	if err := writeAtomic(poolPath, content, 0o640); err != nil {
		return nil, fmt.Errorf("cannot write the pool file: %w", err)
	}
	if switching {
		if err := os.Remove(oldPoolPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, restore(fmt.Errorf("cannot remove the pool file from PHP %s: %w", previousVersion, err))
		}
	}

	// Step 3: make php-fpm parse the whole configuration. This is the step
	// that catches a directive FPM will not accept - and it is the step whose
	// absence turns a typo in a memory limit into a master process that does
	// not come back after the reload.
	if err := m.testConfig(ctx, layout); err != nil {
		return nil, restore(err)
	}

	// Step 4 and 5: reload, then prove it is running.
	if out, err := m.reload(ctx, layout); err != nil {
		return nil, restore(fmt.Errorf("php-fpm %s would not reload: %w: %s",
			spec.Version, err, trimOutput(string(out))))
	}
	if switching {
		if _, err := m.reload(ctx, oldLayout); err != nil {
			// The old version failing to reload after losing a pool is worth
			// logging but is not a reason to undo a successful switch: the
			// site is already being served by the new version.
			m.logger.Warn("the previous PHP version would not reload after the pool moved away",
				zap.String("version", previousVersion), zap.Error(err))
		}
	}
	if err := m.assertActive(ctx, layout); err != nil {
		return nil, restore(err)
	}

	m.logger.Info("php-fpm pool applied",
		zap.String("pool", spec.Name),
		zap.String("version", spec.Version),
		zap.String("pool_file", poolPath),
		zap.String("socket", spec.SocketPath),
		zap.String("user", spec.User),
		zap.Bool("created", !existed),
		zap.String("previous_version", previousVersion),
	)

	return &ApplyResult{
		PoolPath:        poolPath,
		SocketPath:      spec.SocketPath,
		Service:         layout.Service,
		PreviousVersion: previousVersion,
		Created:         !existed,
	}, nil
}

// RemovePool deletes a pool file and reloads. A reload that fails puts the
// file back, for the same reason ApplyPool does.
func (m *Manager) RemovePool(ctx context.Context, version, name string) error {
	if !poolNameRe.MatchString(name) {
		return fmt.Errorf("invalid pool name %q", name)
	}
	layout, err := m.Layout(version)
	if err != nil {
		return err
	}
	poolPath := m.path(layout.PoolPath(name))
	before, existed, err := readIfExists(poolPath)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	if err := os.Remove(poolPath); err != nil {
		return fmt.Errorf("cannot remove the pool file: %w", err)
	}
	if _, err := m.reload(ctx, layout); err != nil {
		if restoreErr := writeOrRemove(poolPath, before, existed); restoreErr != nil {
			return fmt.Errorf("php-fpm would not reload after removing the pool (%w) and the "+
				"pool file could not be restored (%v)", err, restoreErr)
		}
		if _, reloadErr := m.reload(ctx, layout); reloadErr != nil {
			return fmt.Errorf("php-fpm would not reload after removing the pool (%w) and would "+
				"not reload after the rollback either (%v)", err, reloadErr)
		}
		return fmt.Errorf("php-fpm would not reload after removing the pool: %w (rolled back)", err)
	}
	m.logger.Info("php-fpm pool removed",
		zap.String("pool", name), zap.String("version", version), zap.String("pool_file", poolPath))
	return nil
}

// testConfig runs `php-fpm -t`, which parses php-fpm.conf and every pool file
// it includes and exits non-zero on the first thing it will not accept.
func (m *Manager) testConfig(ctx context.Context, layout Layout) error {
	binary := m.path(layout.Binary)
	out, err := m.runner.Run(ctx, binary, "--test", "--fpm-config", m.path(layout.MainConfig))
	if err != nil {
		return fmt.Errorf("php-fpm rejected the configuration: %w", err)
	}
	// php-fpm -t reports errors on stderr and still exits 0 in some builds
	// when the failure is in an included pool file, so the output is read too.
	if text := string(out); strings.Contains(text, "[ERROR]") || strings.Contains(text, "ERROR:") {
		return fmt.Errorf("php-fpm rejected the configuration: %s", trimOutput(text))
	}
	return nil
}

func (m *Manager) reload(ctx context.Context, layout Layout) ([]byte, error) {
	return m.runner.Run(ctx, m.systemd, "reload", layout.Service)
}

// assertActive is step 5. A reload that returns 0 and a master process that
// then died is the exact failure mode this whole file is defending against, so
// the answer is asked for rather than assumed.
func (m *Manager) assertActive(ctx context.Context, layout Layout) error {
	out, err := m.runner.Run(ctx, m.systemd, "is-active", layout.Service)
	state := strings.TrimSpace(string(out))
	if err != nil || state != "active" {
		return fmt.Errorf("php-fpm %s is %q after the reload, not active: the new configuration "+
			"took the service down", layout.Version, state)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Filesystem helpers
// ---------------------------------------------------------------------------

func readIfExists(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return content, true, nil
}

// writeOrRemove is the inverse of "capture": it restores a file to exactly the
// state readIfExists found, including the state of not existing.
func writeOrRemove(path string, content []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot remove %s during rollback: %w", path, err)
		}
		return nil
	}
	if err := writeAtomic(path, content, 0o640); err != nil {
		return fmt.Errorf("cannot restore %s: %w", path, err)
	}
	return nil
}

// writeAtomic writes through a temporary file in the same directory and
// renames. A pool file that FPM reads while it is half written is a site that
// is down for as long as it takes somebody to notice.
func writeAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vkai-pool-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
