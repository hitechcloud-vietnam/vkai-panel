package backup

import "fmt"

// UnsafeMemberError is returned when a member of an archive would write
// outside the restore destination, or would write something a restore must not
// create. Extraction stops at the first such member: an archive containing a
// traversal is not a backup with one bad file in it.
type UnsafeMemberError struct {
	Member string
	Target string
	Reason string
}

func (e *UnsafeMemberError) Error() string {
	if e.Target != "" {
		return fmt.Sprintf("refusing to restore archive member %q (link target %q): %s", e.Member, e.Target, e.Reason)
	}
	return fmt.Sprintf("refusing to restore archive member %q: %s", e.Member, e.Reason)
}

// ManifestMismatchError is returned when the tar and the manifest inside the
// same archive disagree.
//
// It matters more than it looks. A dry run plans from the manifest alone,
// because reading one tar entry is cheap and reading a hundred gigabytes is
// not. That shortcut is only sound if the manifest and the payload are the same
// list, so extraction enforces it on every real run: a member with no manifest
// entry is refused, and a manifest entry with no member is an error at the end.
type ManifestMismatchError struct {
	Extra   []string
	Missing []string
}

func (e *ManifestMismatchError) Error() string {
	return fmt.Sprintf(
		"archive is inconsistent with its own manifest: %d member(s) not in the manifest, %d manifest entry(ies) not in the archive",
		len(e.Extra), len(e.Missing))
}
