package backup

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// PlanEntry is one file a restore would overwrite.
type PlanEntry struct {
	Path           string `json:"path"`
	ExistingSize   int64  `json:"existing_size"`
	ExistingSHA256 string `json:"existing_sha256,omitempty"`
	IncomingSize   int64  `json:"incoming_size"`
	IncomingSHA256 string `json:"incoming_sha256,omitempty"`
	// Identical is true when the file already on disk is byte for byte the
	// file in the archive. It is the difference between "this restore will
	// change 4 files" and "this restore will touch 40000 and change 4", which
	// is the difference between an operator reading the plan and skipping it.
	Identical bool `json:"identical"`
}

// RestorePlan is what a restore would do, and after a real run, what it did.
type RestorePlan struct {
	Destination      string      `json:"destination"`
	Kind             string      `json:"kind"`
	ArchiveSource    string      `json:"archive_source"`
	ArchiveCreatedAt string      `json:"archive_created_at"`
	DryRun           bool        `json:"dry_run"`
	Applied          bool        `json:"applied"`
	NewFiles         []string    `json:"new_files"`
	NewDirs          []string    `json:"new_dirs"`
	Overwrites       []PlanEntry `json:"overwrites"`
	Symlinks         []string    `json:"symlinks"`
	FilesTotal       int         `json:"files_total"`
	BytesTotal       int64       `json:"bytes_total"`
	FilesWritten     int         `json:"files_written"`
	BytesWritten     int64       `json:"bytes_written"`
	// ExistingNotInArchive names files already in the destination that the
	// archive does not contain. A restore does not delete them - it is a
	// restore, not a sync - but an operator restoring into a live document
	// root needs to know they will still be there afterwards.
	ExistingNotInArchive []string `json:"existing_not_in_archive"`

	// manifest is the archive's own manifest. It is not marshalled: a plan
	// goes into an API response and a manifest of half a million files does
	// not belong there. Verification, which runs in-process, reads it through
	// Manifest() rather than opening the archive a second time.
	manifest *Manifest
}

// Manifest returns the archive's manifest, or nil if the plan came from
// somewhere that did not read one.
func (p *RestorePlan) Manifest() *Manifest {
	if p == nil {
		return nil
	}
	return p.manifest
}

// ChangedOverwrites returns the overwrites that actually differ.
func (p *RestorePlan) ChangedOverwrites() []PlanEntry {
	out := make([]PlanEntry, 0, len(p.Overwrites))
	for _, e := range p.Overwrites {
		if !e.Identical {
			out = append(out, e)
		}
	}
	return out
}

// ExtractOptions controls a restore.
type ExtractOptions struct {
	// Dest is the directory the archive is restored into. It is created if it
	// does not exist.
	Dest string
	// Key decrypts the archive. Nil means the archive is not encrypted; if it
	// is, opening it fails rather than producing rubbish.
	Key *Key
	// DryRun plans the restore and writes nothing.
	DryRun bool
	// AllowOverwrite must be set for a real run that would change an existing
	// file. Without it a restore that would overwrite something stops and
	// hands back the plan, so "restore in one action" cannot become "destroy
	// in one action" by accident.
	AllowOverwrite bool
	// SurveyDestination lists what is already in Dest so the plan can report
	// files the archive will not replace. It costs a directory walk, so a
	// verification pass into an empty scratch directory turns it off.
	SurveyDestination bool
	Tracker           *Tracker
}

// ExtractArchive restores an archive, or plans the restore without touching
// anything.
//
// A dry run reads exactly one tar entry - the manifest - and answers from it.
// A real run reads the whole archive and holds the manifest and the payload to
// each other: a member the manifest does not list is refused, and a manifest
// entry the archive does not contain is an error once the archive ends.
func ExtractArchive(ctx context.Context, src io.Reader, opts ExtractOptions) (*RestorePlan, error) {
	if strings.TrimSpace(opts.Dest) == "" {
		return nil, errors.New("backup: no restore destination")
	}
	dest := filepath.Clean(opts.Dest)
	if !filepath.IsAbs(dest) {
		return nil, fmt.Errorf("backup: restore destination %q must be an absolute path", opts.Dest)
	}

	ar, err := openArchive(src, opts.Key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = ar.closer() }()

	manifest, err := readManifestEntry(ar.tr)
	if err != nil {
		return nil, err
	}

	opts.Tracker.SetPhase(PhaseExtract, "planning restore into "+dest)
	plan, err := buildPlan(ctx, manifest, dest, opts.SurveyDestination)
	if err != nil {
		return nil, err
	}
	plan.DryRun = opts.DryRun
	plan.manifest = manifest

	if opts.DryRun {
		return plan, nil
	}

	if !opts.AllowOverwrite && len(plan.ChangedOverwrites()) > 0 {
		return plan, fmt.Errorf(
			"backup: this restore would overwrite %d existing file(s) in %s; re-run it with overwrite allowed, or restore into an empty directory",
			len(plan.ChangedOverwrites()), dest)
	}

	opts.Tracker.SetPhase(PhaseExtract, "restoring into "+dest)
	opts.Tracker.SetTotals(manifest.FileCount, manifest.TotalSize)

	if err := applyArchive(ctx, ar.tr, manifest, dest, plan, opts.Tracker); err != nil {
		return plan, err
	}
	plan.Applied = true
	return plan, nil
}

func buildPlan(ctx context.Context, m *Manifest, dest string, survey bool) (*RestorePlan, error) {
	plan := &RestorePlan{
		Destination:      dest,
		Kind:             m.Kind,
		ArchiveSource:    m.Source,
		ArchiveCreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		FilesTotal:       m.FileCount,
		BytesTotal:       m.TotalSize,
	}

	inArchive := make(map[string]struct{}, len(m.Entries))
	for _, entry := range m.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean, err := checkMember(entry.Path)
		if err != nil {
			return nil, err
		}
		target, err := safeJoin(dest, clean)
		if err != nil {
			return nil, err
		}
		inArchive[clean] = struct{}{}

		switch entry.Type {
		case EntryDir:
			if info, statErr := os.Lstat(target); statErr != nil || !info.IsDir() {
				plan.NewDirs = append(plan.NewDirs, clean)
			}
		case EntrySymlink:
			plan.Symlinks = append(plan.Symlinks, clean)
		case EntryFile:
			info, statErr := os.Lstat(target)
			if statErr != nil {
				plan.NewFiles = append(plan.NewFiles, clean)
				continue
			}
			pe := PlanEntry{
				Path:           clean,
				ExistingSize:   info.Size(),
				IncomingSize:   entry.Size,
				IncomingSHA256: entry.SHA256,
			}
			// Hashing every existing file would double the cost of planning a
			// restore into a live tree. A different size is already proof of a
			// difference, so only same-size files are hashed - which is also
			// the only case where the answer is not obvious.
			if info.Mode().IsRegular() && info.Size() == entry.Size {
				sum, hashErr := hashFile(ctx, target)
				if hashErr != nil {
					return nil, hashErr
				}
				pe.ExistingSHA256 = sum
				pe.Identical = sum == entry.SHA256
			}
			plan.Overwrites = append(plan.Overwrites, pe)
		}
	}

	if survey {
		existing, err := surveyDestination(ctx, dest, inArchive)
		if err != nil {
			return nil, err
		}
		plan.ExistingNotInArchive = existing
	}

	sort.Strings(plan.NewFiles)
	sort.Strings(plan.NewDirs)
	sort.Strings(plan.Symlinks)
	sort.Slice(plan.Overwrites, func(i, j int) bool { return plan.Overwrites[i].Path < plan.Overwrites[j].Path })
	return plan, nil
}

func surveyDestination(ctx context.Context, dest string, inArchive map[string]struct{}) ([]string, error) {
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var out []string
	err = filepath.WalkDir(dest, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // an unreadable corner of the destination is not a reason to refuse the plan
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == dest || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dest, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := inArchive[rel]; !ok {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func applyArchive(ctx context.Context, tr *tar.Reader, m *Manifest, dest string, plan *RestorePlan, tracker *Tracker) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("backup: cannot create the restore destination %q: %w", dest, err)
	}

	byPath := make(map[string]Entry, len(m.Entries))
	for _, entry := range m.Entries {
		clean, err := checkMember(entry.Path)
		if err != nil {
			return err
		}
		byPath[clean] = entry
	}

	seen := make(map[string]struct{}, len(byPath))
	dirModes := map[string]uint32{}
	// Symlinks are held back until every directory and regular file is on
	// disk, so nothing this restore writes can pass through a link this
	// restore created.
	var links []Entry

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("backup: archive is truncated or corrupt: %w", err)
		}

		clean, err := checkMember(header.Name)
		if err != nil {
			return err
		}
		entry, known := byPath[clean]
		if !known {
			return &UnsafeMemberError{
				Member: header.Name,
				Reason: "member is not listed in the archive manifest",
			}
		}
		if _, dup := seen[clean]; dup {
			return &UnsafeMemberError{Member: header.Name, Reason: "member appears twice in the archive"}
		}
		seen[clean] = struct{}{}

		target, err := safeJoin(dest, clean)
		if err != nil {
			return err
		}
		if err := checkMemberMode(header.Name, header.FileInfo().Mode()); err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if entry.Type != EntryDir {
				return &UnsafeMemberError{Member: header.Name, Reason: "member type does not match the manifest"}
			}
			if err := ensureDir(dest, clean); err != nil {
				return err
			}
			dirModes[target] = entry.Mode
		case tar.TypeSymlink:
			if entry.Type != EntrySymlink {
				return &UnsafeMemberError{Member: header.Name, Reason: "member type does not match the manifest"}
			}
			links = append(links, entry)
		case tar.TypeReg:
			if entry.Type != EntryFile {
				return &UnsafeMemberError{Member: header.Name, Reason: "member type does not match the manifest"}
			}
			if err := writeRestoredFile(ctx, tr, dest, clean, entry); err != nil {
				return err
			}
			plan.FilesWritten++
			plan.BytesWritten += entry.Size
			tracker.Advance(1, entry.Size)
		default:
			return &UnsafeMemberError{
				Member: header.Name,
				Reason: fmt.Sprintf("member type %q is not something a restore creates", string(header.Typeflag)),
			}
		}
	}

	if missing := missingMembers(byPath, seen); len(missing) > 0 {
		return &ManifestMismatchError{Missing: missing}
	}

	for _, link := range links {
		clean, err := checkMember(link.Path)
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, clean)
		if err != nil {
			return err
		}
		if err := ensureDir(dest, path.Dir(clean)); err != nil {
			return err
		}
		// A restore replaces what is there. Remove first: os.Symlink will not
		// overwrite, and leaving the old link would make the restore a lie.
		if _, lstatErr := os.Lstat(target); lstatErr == nil {
			if err := os.Remove(target); err != nil {
				return fmt.Errorf("backup: cannot replace %q: %w", target, err)
			}
		}
		if err := os.Symlink(link.Link, target); err != nil {
			return fmt.Errorf("backup: cannot restore the symlink %q: %w", clean, err)
		}
	}

	// Directory modes are applied last: a directory restored mode 0500 would
	// otherwise stop the files inside it from being written.
	paths := make([]string, 0, len(dirModes))
	for p := range dirModes {
		paths = append(paths, p)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	for _, p := range paths {
		if err := os.Chmod(p, os.FileMode(dirModes[p]).Perm()); err != nil {
			return fmt.Errorf("backup: cannot set the mode of %q: %w", p, err)
		}
	}
	return nil
}

func missingMembers(byPath map[string]Entry, seen map[string]struct{}) []string {
	var missing []string
	for p := range byPath {
		if _, ok := seen[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	return missing
}

func writeRestoredFile(ctx context.Context, tr io.Reader, dest, clean string, entry Entry) error {
	if err := ensureDir(dest, path.Dir(clean)); err != nil {
		return err
	}
	target, err := safeJoin(dest, clean)
	if err != nil {
		return err
	}

	// Anything already at the path is removed rather than opened. Opening it
	// would follow a symlink that was there before the restore started; and
	// O_NOFOLLOW on its own would only turn that into an error, when what the
	// operator asked for is the file replaced.
	if info, lstatErr := os.Lstat(target); lstatErr == nil && !info.Mode().IsRegular() {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("backup: cannot replace %q: %w", target, err)
		}
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|oNoFollow, os.FileMode(entry.Mode).Perm())
	if err != nil {
		return fmt.Errorf("backup: cannot write %q: %w", target, err)
	}
	defer func() { _ = f.Close() }()

	written, err := io.Copy(f, &ctxReader{ctx: ctx, src: io.LimitReader(tr, entry.Size+1)})
	if err != nil {
		return fmt.Errorf("backup: cannot write %q: %w", target, err)
	}
	if written != entry.Size {
		return &ManifestMismatchError{Extra: []string{fmt.Sprintf(
			"%s: archive holds %d bytes, manifest says %d", clean, written, entry.Size)}}
	}
	if err := f.Chmod(os.FileMode(entry.Mode).Perm()); err != nil {
		return fmt.Errorf("backup: cannot set the mode of %q: %w", target, err)
	}
	return f.Close()
}

// ensureDir creates every component of rel under dest, one at a time, and
// refuses a component that already exists as anything other than a real
// directory. It is what stops a pre-existing symlink in the destination from
// redirecting the whole restore.
func ensureDir(dest, rel string) error {
	if rel == "." || rel == "" || rel == "/" {
		if err := os.MkdirAll(dest, 0o750); err != nil {
			return fmt.Errorf("backup: cannot create %q: %w", dest, err)
		}
		return nil
	}

	current := filepath.Clean(dest)
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case err == nil && info.IsDir():
			continue
		case err == nil:
			return &UnsafeMemberError{
				Member: rel,
				Reason: fmt.Sprintf("%q already exists and is not a directory", current),
			}
		case !os.IsNotExist(err):
			return fmt.Errorf("backup: cannot inspect %q: %w", current, err)
		}
		if err := os.Mkdir(current, 0o750); err != nil && !os.IsExist(err) {
			return fmt.Errorf("backup: cannot create %q: %w", current, err)
		}
		info, err = os.Lstat(current)
		if err != nil || !info.IsDir() {
			return &UnsafeMemberError{Member: rel, Reason: fmt.Sprintf("%q is not a directory after creating it", current)}
		}
	}
	return nil
}

// checkMember validates one archive member name. The rules are the ones
// internal/upgrade documents, in the order a hostile archive tends to try them.
func checkMember(name string) (string, error) {
	if name == "" {
		return "", &UnsafeMemberError{Member: name, Reason: "empty member name"}
	}
	if strings.ContainsAny(name, "\x00\n\r") {
		return "", &UnsafeMemberError{Member: name, Reason: "member name contains a control character"}
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", &UnsafeMemberError{Member: name, Reason: "member name is an absolute path"}
	}
	if strings.Contains(name, `\`) {
		return "", &UnsafeMemberError{Member: name, Reason: "member name contains a backslash"}
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", &UnsafeMemberError{Member: name, Reason: "member name looks like a drive-letter absolute path"}
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", &UnsafeMemberError{Member: name, Reason: `member name contains ".."`}
		}
	}

	clean := strings.TrimSuffix(path.Clean(name), "/")
	if clean == "." || clean == "" {
		return "", &UnsafeMemberError{Member: name, Reason: "member name resolves to the destination directory itself"}
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", &UnsafeMemberError{Member: name, Reason: "member name escapes the destination directory"}
	}
	return clean, nil
}

// safeJoin joins a validated relative name onto dest and proves the result did
// not leave dest. It is redundant given checkMember, and that is the point: it
// still holds if those rules are ever relaxed.
func safeJoin(dest, cleanName string) (string, error) {
	base := filepath.Clean(dest)
	joined := filepath.Clean(filepath.Join(base, filepath.FromSlash(cleanName)))
	if joined != base && !strings.HasPrefix(joined, base+string(filepath.Separator)) {
		return "", &UnsafeMemberError{
			Member: cleanName,
			Reason: fmt.Sprintf("resolves to %s, outside %s", joined, base),
		}
	}
	return joined, nil
}

// checkMemberMode refuses a member carrying a privilege bit. A restore runs as
// root; recreating a setuid binary out of an archive is a decision an operator
// makes deliberately, not one a restore makes on their behalf.
func checkMemberMode(member string, mode os.FileMode) error {
	switch {
	case mode&os.ModeSetuid != 0:
		return &UnsafeMemberError{Member: member, Reason: "member is setuid"}
	case mode&os.ModeSetgid != 0:
		return &UnsafeMemberError{Member: member, Reason: "member is setgid"}
	case mode&os.ModeSticky != 0:
		return &UnsafeMemberError{Member: member, Reason: "member has the sticky bit set"}
	}
	return nil
}
