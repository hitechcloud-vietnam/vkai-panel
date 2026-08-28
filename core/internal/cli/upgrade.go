package cli

// The upgrade command: "vkai-cli upgrade", which is what the shell wrapper
// /usr/local/bin/vkai calls for "vkai upgrade".
//
//	vkai upgrade --check            report only, never touch the machine
//	vkai upgrade                    show the plan, ask, then upgrade
//	vkai upgrade --to 1.4.2 --yes   unattended upgrade to one exact version
//
// EXIT CODES. They are part of the interface, because "--check" is meant to
// drive monitoring and the daily timer:
//
//	 0  up to date, or an update exists but the installation is pinned
//	10  an update is available on the channel
//	 2  this build has no upgrade support compiled in
//	 1  the check or the upgrade failed
//
// WHAT THIS COMMAND DOES NOT DO. It does not write the check record that the
// panel displays. The wrapper owns /vkai-panel/etc/upgrade-check.json, so the
// record is produced identically whether or not the installed vkai-cli is new
// enough to have this command - an operator upgrading FROM an old build is
// exactly the case that must not silently stop reporting.
//
// TODO(upgrade-package): swap the stub for the real engine.
// core/internal/upgrade is written by another change. Until it lands, this
// command talks to a small local interface so the CLI keeps compiling and
// keeps telling the truth about the installed version. To swap:
//
//  1. delete stubUpgrader and errUpgradeNotBuilt below,
//  2. make newUpgrader() return the real client, e.g.
//     return upgrade.New(upgrade.Options{Root: config.PanelRoot()}), nil
//  3. keep upgradeStatus as the JSON contract, or alias it to the package's
//     own status type - deploy/vkai.sh, the vkai-upgrade-check timer and
//     docs/UPGRADE.md all read exactly these field names.
//
// Nothing else in this file needs to change.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Exit codes of "vkai upgrade". Documented in docs/UPGRADE.md.
const (
	exitUpToDate        = 0
	exitCheckFailed     = 1
	exitNoUpgradeEngine = 2
	exitUpdateAvailable = 10
)

// Status values written into the check record.
const (
	statusUpToDate        = "up-to-date"
	statusUpdateAvailable = "update-available"
	statusPinned          = "pinned"
	statusUnsupported     = "unsupported"
	statusError           = "error"
)

const releaseNotesURL = "https://github.com/hitechcloud-vietnam/vkai-panel/releases"

// upgradeStatus is the JSON contract of "vkai upgrade --check --json".
// deploy/vkai.sh copies this object verbatim into
// /vkai-panel/etc/upgrade-check.json, and the panel reads it from there.
// Renaming a field here breaks the shell wrapper and the panel display.
type upgradeStatus struct {
	CheckedAt        string `json:"checked_at"`
	Status           string `json:"status"`
	InstalledVersion string `json:"installed_version"`
	LatestVersion    string `json:"latest_version"`
	Channel          string `json:"channel"`
	Pinned           string `json:"pinned,omitempty"`
	ReleaseNotesURL  string `json:"release_notes_url,omitempty"`
	Detail           string `json:"detail,omitempty"`
}

func (s *upgradeStatus) updateAvailable() bool { return s.Status == statusUpdateAvailable }

// exitCode maps a status onto the process exit code. A pinned installation
// exits 0: the operator asked for that version, so monitoring must not page
// anybody about it.
func (s *upgradeStatus) exitCode() int {
	switch s.Status {
	case statusUpdateAvailable:
		return exitUpdateAvailable
	case statusUnsupported:
		return exitNoUpgradeEngine
	case statusError:
		return exitCheckFailed
	default:
		return exitUpToDate
	}
}

// upgrader is the slice of core/internal/upgrade this command needs. Keeping it
// this narrow is what lets the command compile before that package exists.
type upgrader interface {
	// Check resolves the newest version offered on the configured channel.
	// It must not modify the machine.
	Check(ctx context.Context) (*upgradeStatus, error)
	// Apply upgrades to version, or to the newest one on the channel when
	// version is empty. It is expected to roll back by itself on failure.
	Apply(ctx context.Context, version string) error
}

// errUpgradeNotBuilt is returned by the stub below. Delete it together with the
// stub when the real package lands.
var errUpgradeNotBuilt = errors.New("upgrade support is not built into this binary")

// newUpgrader returns the engine that performs checks and upgrades.
// SWAP HERE - see the TODO(upgrade-package) note at the top of the file.
func newUpgrader() upgrader { return stubUpgrader{} }

// stubUpgrader answers with the local facts it can prove from
// /vkai-panel/etc/version.json and refuses everything else. It deliberately
// still fills in the installed version: an operator running "vkai version" on a
// half-upgraded machine needs that answer even when the remote check cannot
// run.
type stubUpgrader struct{}

func (stubUpgrader) Check(context.Context) (*upgradeStatus, error) {
	local, _ := readVersionFile()
	return &upgradeStatus{
		CheckedAt:        time.Now().UTC().Format(time.RFC3339),
		Status:           statusUnsupported,
		InstalledVersion: local.Version,
		Channel:          local.Channel,
		Pinned:           local.Pin,
		ReleaseNotesURL:  releaseNotesURL,
		Detail:           errUpgradeNotBuilt.Error(),
	}, errUpgradeNotBuilt
}

func (stubUpgrader) Apply(context.Context, string) error { return errUpgradeNotBuilt }

// versionFile mirrors /vkai-panel/etc/version.json, written by
// deploy/install.sh and updated by every successful upgrade. It is the record
// of what is installed even when the binaries were replaced by hand.
type versionFile struct {
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Pin         string `json:"pin"`
	InstalledAt string `json:"installed_at"`
	UpdatedAt   string `json:"updated_at"`
	Release     string `json:"release"`
}

// versionFilePath is /vkai-panel/etc/version.json, moving with VKAI_PANEL_ROOT.
func versionFilePath() string { return filepath.Join(config.EtcRoot(), "version.json") }

// readVersionFile never fails hard: a missing or corrupt file means "unknown",
// which is still a better answer than refusing to print anything.
func readVersionFile() (versionFile, error) {
	v := versionFile{Version: "unknown", Channel: "stable"}
	raw, err := os.ReadFile(versionFilePath())
	if err != nil {
		return v, err
	}
	var parsed versionFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return v, fmt.Errorf("%s is not valid JSON: %w", versionFilePath(), err)
	}
	if strings.TrimSpace(parsed.Version) != "" {
		v.Version = strings.TrimSpace(parsed.Version)
	}
	if strings.TrimSpace(parsed.Channel) != "" {
		v.Channel = strings.TrimSpace(parsed.Channel)
	}
	v.Pin = strings.TrimSpace(parsed.Pin)
	v.InstalledAt = parsed.InstalledAt
	v.UpdatedAt = parsed.UpdatedAt
	v.Release = parsed.Release
	return v, nil
}

func init() {
	// Registered here rather than in root.go so this command is self-contained.
	rootCmd.AddCommand(newUpgradeCmd())
}

func newUpgradeCmd() *cobra.Command {
	var (
		checkOnly bool
		toVersion string
		assumeYes bool
		asJSON    bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for a new VKAI Panel release and install it",
		Long: `Check for a new VKAI Panel release and install it.

  vkai upgrade --check            Report only. Changes nothing.
  vkai upgrade                    Print the plan, ask for confirmation, upgrade.
  vkai upgrade --to 1.4.2         Upgrade to one exact version.
  vkai upgrade --yes              Skip the confirmation (for automation).

Exit codes: 0 up to date (or pinned), 10 an update is available,
2 this build has no upgrade support, 1 the check or the upgrade failed.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if checkOnly {
				return runUpgradeCheck(cmd, ctx, asJSON)
			}
			return runUpgradeApply(cmd, ctx, strings.TrimSpace(toVersion), assumeYes)
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "Report whether an update is available and change nothing")
	cmd.Flags().StringVar(&toVersion, "to", "", "Upgrade to this exact version instead of the newest on the channel")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Do not ask for confirmation (for automation)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the check result as JSON")
	return cmd
}

// runUpgradeCheck reports and exits. It calls os.Exit directly because the exit
// code carries the answer: returning an error would collapse "an update is
// available" and "the check failed" into the same 1.
func runUpgradeCheck(cmd *cobra.Command, ctx context.Context, asJSON bool) error {
	status, err := newUpgrader().Check(ctx)
	if status == nil {
		local, _ := readVersionFile()
		status = &upgradeStatus{
			CheckedAt:        time.Now().UTC().Format(time.RFC3339),
			Status:           statusError,
			InstalledVersion: local.Version,
			Channel:          local.Channel,
			Pinned:           local.Pin,
		}
	}
	if err != nil && status.Detail == "" {
		status.Detail = err.Error()
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(status); encErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", encErr)
			os.Exit(exitCheckFailed)
		}
	} else {
		printUpgradeStatus(cmd, status)
	}

	os.Exit(status.exitCode())
	return nil
}

func printUpgradeStatus(cmd *cobra.Command, s *upgradeStatus) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Installed version : %s\n", orUnknown(s.InstalledVersion))
	fmt.Fprintf(out, "Release channel   : %s\n", orUnknown(s.Channel))
	if s.Pinned != "" {
		fmt.Fprintf(out, "Pinned to         : %s\n", s.Pinned)
	}
	fmt.Fprintf(out, "Latest version    : %s\n", orUnknown(s.LatestVersion))
	fmt.Fprintf(out, "Checked at        : %s\n", orUnknown(s.CheckedAt))

	switch s.Status {
	case statusUpdateAvailable:
		fmt.Fprintf(out, "\nAn update is available: %s -> %s\n", s.InstalledVersion, s.LatestVersion)
		fmt.Fprintf(out, "Install it with: sudo vkai upgrade\n")
		if s.ReleaseNotesURL != "" {
			fmt.Fprintf(out, "Release notes  : %s\n", s.ReleaseNotesURL)
		}
	case statusPinned:
		fmt.Fprintf(out, "\nA newer release exists but this installation is pinned to %s.\n", s.Pinned)
		fmt.Fprintf(out, "Clear the pin in %s to allow upgrades again.\n", versionFilePath())
	case statusUpToDate:
		fmt.Fprintf(out, "\nThis panel is up to date.\n")
	case statusUnsupported:
		fmt.Fprintf(cmd.ErrOrStderr(), "\nThis build cannot check for updates: %s\n", orUnknown(s.Detail))
		fmt.Fprintf(cmd.ErrOrStderr(), "Upgrade manually from a release package - see docs/UPGRADE.md.\n")
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "\nThe update check failed: %s\n", orUnknown(s.Detail))
	}
}

func runUpgradeApply(cmd *cobra.Command, ctx context.Context, version string, assumeYes bool) error {
	if os.Geteuid() != 0 {
		return errors.New("upgrading needs root: sudo vkai upgrade")
	}

	engine := newUpgrader()
	status, err := engine.Check(ctx)
	if errors.Is(err, errUpgradeNotBuilt) {
		return fmt.Errorf("%w.\nUpgrade from a release package instead:\n"+
			"  sudo /vkai-panel/bin/vkai-deploy deploy /tmp/vkai-panel-<version>.tar.gz\n"+
			"See docs/UPGRADE.md", errUpgradeNotBuilt)
	}
	if err != nil {
		return fmt.Errorf("update check failed: %w", err)
	}

	target := version
	if target == "" {
		target = status.LatestVersion
	}
	if target == "" {
		return errors.New("no target version: the channel offered nothing and --to was not given")
	}
	if version == "" && status.Status == statusPinned {
		fmt.Fprintf(cmd.OutOrStdout(),
			"This panel is pinned to %s, so it will not follow channel %s to %s.\n"+
				"Clear \"pin\" in %s to allow upgrades again, or override this one run with --to %s.\n",
			status.Pinned, orUnknown(status.Channel), orUnknown(status.LatestVersion),
			versionFilePath(), orUnknown(status.LatestVersion))
		return nil
	}
	if version == "" && !status.updateAvailable() {
		fmt.Fprintf(cmd.OutOrStdout(),
			"Already running %s on channel %s; nothing to do.\n"+
				"Use --to %s to reinstall this version anyway.\n",
			orUnknown(status.InstalledVersion), orUnknown(status.Channel), orUnknown(status.InstalledVersion))
		return nil
	}
	if version != "" && status.Pinned != "" && status.Pinned != version {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: this installation is pinned to %s; --to %s overrides the pin for this run only.\n",
			status.Pinned, version)
	}

	printUpgradePlan(cmd, status, target)

	if !assumeYes {
		ok, err := confirm(cmd, "Proceed with the upgrade?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. Nothing was changed.")
			return nil
		}
	}

	if err := engine.Apply(ctx, target); err != nil {
		return fmt.Errorf("the upgrade to %s failed: %w\n"+
			"The previous release should have been restored automatically; confirm with:\n"+
			"  vkai version && vkai status\n"+
			"If it was not, follow the manual recovery in docs/UPGRADE.md", target, err)
	}

	printSuccess("Upgraded to %s.", target)
	printInfo("Verify with: vkai version && vkai status")
	return nil
}

func printUpgradePlan(cmd *cobra.Command, s *upgradeStatus, target string) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nUpgrade plan\n")
	fmt.Fprintf(out, "  From            : %s\n", orUnknown(s.InstalledVersion))
	fmt.Fprintf(out, "  To              : %s\n", target)
	fmt.Fprintf(out, "  Channel         : %s\n", orUnknown(s.Channel))
	fmt.Fprintf(out, "\nWhat will happen\n")
	fmt.Fprintf(out, "  1. Download and verify the release package.\n")
	fmt.Fprintf(out, "  2. Unpack it into a NEW directory under /vkai-panel/releases/.\n")
	fmt.Fprintf(out, "  3. Back up the database into /vkai-panel/www/backup/.\n")
	fmt.Fprintf(out, "  4. Run the migrations the new release brings.\n")
	fmt.Fprintf(out, "  5. Repoint /vkai-panel/current and restart vkai-api and vkai-ui.\n")
	fmt.Fprintf(out, "  6. Health check both, and roll back to the current release on failure.\n")
	fmt.Fprintf(out, "\nThe panel is unreachable for a few seconds during step 5.\n")
	fmt.Fprintf(out, "Customer websites are served by nginx and stay up throughout.\n\n")
}

// confirm asks once and accepts only an explicit yes. With no terminal on
// stdin it refuses rather than assuming consent, so a cron job that forgot
// --yes stops instead of upgrading a fleet unattended.
func confirm(cmd *cobra.Command, question string) (bool, error) {
	in, ok := cmd.InOrStdin().(*os.File)
	if ok {
		info, err := in.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice == 0 {
			return false, errors.New("stdin is not a terminal: pass --yes to confirm without being asked")
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s [yes/no] ", question)
	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("could not read the answer: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func orUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
