package wpcli

// File ownership and modes for a WordPress installation.
//
// The requirement is "set file ownership and modes so the web server can write
// what it must and no more", and the two halves of that sentence pull against
// each other. WordPress needs to write to exactly one place - wp-content/uploads
// - for a customer to upload an image. Everything else it wants to write to
// (plugins, themes, core files) it only needs when the customer clicks
// "update", and a panel that has WP-CLI can do those updates itself.
//
// So the layout below is the one that survives contact with a compromised
// plugin: PHP can write uploads and nothing else. A vulnerable plugin that
// gets a file write primitive lands it in a directory that the web server is
// separately configured not to execute, instead of overwriting index.php.
//
//	owner            the site user (the FPM pool user, the WP-CLI user)
//	group            the site group; the web server is a member where the
//	                 web server serves static files directly
//	directories      0750, except uploads which is 0770
//	files            0640, except wp-config.php which is 0600
//	uploads tree     group-writable, because that is what PHP must write
//
// wp-config.php at 0600 is the one that matters most: it holds the database
// password in plaintext, and 0640 with the web server in the group means any
// other site served by the same web server user can read it.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Permissions describes the modes applied to an installation.
type Permissions struct {
	DirMode        fs.FileMode
	FileMode       fs.FileMode
	UploadsDirMode fs.FileMode
	ConfigMode     fs.FileMode
}

// DefaultPermissions is the layout described above.
func DefaultPermissions() Permissions {
	return Permissions{
		DirMode:        0o750,
		FileMode:       0o640,
		UploadsDirMode: 0o770,
		ConfigMode:     0o600,
	}
}

// ApplyOwnership walks a WordPress installation and sets owner, group and mode.
//
// It runs as the panel (root), because chown is a root operation. It refuses a
// root identity for the same reason everything else here does: a WordPress tree
// owned by root is a WordPress a customer cannot update and a compromise cannot
// be contained in.
//
// Symbolic links are never followed: lchown and a mode skip. A plugin that
// drops a symlink to /etc/shadow into wp-content must not turn a permissions
// pass into a privilege escalation.
func ApplyOwnership(root string, identity Identity, perms Permissions) error {
	cleaned, err := Path("wordpress directory", root)
	if err != nil {
		return err
	}
	if identity.UID == 0 || identity.GID == 0 {
		return &ErrWouldRunAsRoot{Requested: identity.Name, UID: identity.UID, GID: identity.GID}
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", cleaned, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", cleaned)
	}

	uploads := filepath.Join(cleaned, "wp-content", "uploads")
	config := filepath.Join(cleaned, "wp-config.php")

	return filepath.WalkDir(cleaned, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Lchown, not Chown: a symlink's target must not be given away.
		if err := os.Lchown(path, int(identity.UID), int(identity.GID)); err != nil {
			return fmt.Errorf("cannot set the owner of %s: %w", path, err)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			// A symlink's mode is meaningless and chmod would follow it.
			return nil
		}
		switch {
		case entry.IsDir():
			mode := perms.DirMode
			if path == uploads || strings.HasPrefix(path, uploads+string(os.PathSeparator)) {
				mode = perms.UploadsDirMode
			}
			return chmod(path, mode)
		case path == config:
			return chmod(path, perms.ConfigMode)
		case strings.HasPrefix(path, uploads+string(os.PathSeparator)):
			// An uploaded file is data. It is never executable, whatever
			// extension it was given.
			return chmod(path, 0o660)
		default:
			return chmod(path, perms.FileMode)
		}
	})
}

func chmod(path string, mode fs.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("cannot set the mode of %s: %w", path, err)
	}
	return nil
}

// EnsureUploads creates wp-content/uploads before ApplyOwnership walks, so a
// brand-new installation gets the group-writable directory it needs rather than
// having WordPress create it later with whatever umask the pool had.
func EnsureUploads(root string, identity Identity, perms Permissions) error {
	cleaned, err := Path("wordpress directory", root)
	if err != nil {
		return err
	}
	uploads := filepath.Join(cleaned, "wp-content", "uploads")
	if err := os.MkdirAll(uploads, perms.UploadsDirMode); err != nil {
		return fmt.Errorf("cannot create %s: %w", uploads, err)
	}
	return nil
}
