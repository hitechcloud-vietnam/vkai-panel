package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

var (
	ownerRe         = regexp.MustCompile(`^[a-z_][a-z0-9_-]*(:[a-z_][a-z0-9_-]*)?$`)
	searchPatternRe = regexp.MustCompile(`^[A-Za-z0-9._*?\[\]-]{1,128}$`)
)

// defaultFileManagerRoot is used whenever the caller does not supply a usable
// jail: the customer site tree, and nothing above it. The file manager must
// never be rooted at "/" because it runs with the privileges of the panel
// process.
func defaultFileManagerRoot() string { return config.WebRoot() }

// deniedRoots are never reachable through the file manager, even when they
// happen to sit inside the configured base path. The panel's own subtrees are
// listed individually rather than as the whole installation root: the customer
// sites live under that same root, so denying it wholesale would deny the file
// manager its own jail.
func deniedRoots() []string {
	return []string{
		"/etc",
		"/root",
		"/proc",
		"/sys",
		"/dev",
		"/boot",
		config.CoreRoot(),
		config.UIRoot(),
		config.EtcRoot(),
		config.LogRoot(),
		config.SSLRoot(),
	}
}

// FileManager provides web-based file management operations. Every operation is
// confined to basePath: paths are resolved against it, symlinks are evaluated
// and the result must still be inside the jail.
type FileManager struct {
	basePath string // Base path restriction for security
}

// NewFileManager builds a file manager jailed to basePath. An empty base path,
// or "/", is replaced by the root from VKAI_FILEMANAGER_ROOT (or the web root,
// /vkai-panel/www/domains) because serving the whole filesystem is equivalent
// to remote root access.
func NewFileManager(basePath string) *FileManager {
	root := strings.TrimSpace(basePath)
	if root == "" || filepath.Clean(root) == "/" {
		root = strings.TrimSpace(os.Getenv("VKAI_FILEMANAGER_ROOT"))
	}
	if root == "" || filepath.Clean(root) == "/" {
		root = defaultFileManagerRoot()
	}
	root = filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return &FileManager{basePath: root}
}

// BasePath returns the jail root the file manager is confined to.
func (m *FileManager) BasePath() string {
	return m.basePath
}

// FileInfo represents a file or directory
type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	IsDir    bool   `json:"is_dir"`
	ModTime  string `json:"mod_time"`
	Owner    string `json:"owner"`
	MimeType string `json:"mime_type"`
}

// ListFiles lists files in a directory
func (m *FileManager) ListFiles(ctx context.Context, dirPath string) ([]FileInfo, error) {
	dirPath, err := m.ResolvePath(dirPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		owner := m.getFileOwner(fullPath)

		files = append(files, FileInfo{
			Name:     entry.Name(),
			Path:     fullPath,
			Size:     info.Size(),
			Mode:     info.Mode().String(),
			IsDir:    entry.IsDir(),
			ModTime:  info.ModTime().Format("2006-01-02 15:04:05"),
			Owner:    owner,
			MimeType: m.getMimeType(entry.Name(), entry.IsDir()),
		})
	}

	return files, nil
}

// ReadFile reads the content of a file
func (m *FileManager) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	filePath, err := m.ResolvePath(filePath)
	if err != nil {
		return nil, err
	}

	// Check file size (max 10MB for reading)
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.Size() > 10*1024*1024 {
		return nil, fmt.Errorf("file too large (>10MB)")
	}

	return os.ReadFile(filePath)
}

// WriteFile writes content to a file
func (m *FileManager) WriteFile(ctx context.Context, filePath string, content []byte) error {
	filePath, err := m.ResolvePath(filePath)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(filePath, content, 0644)
}

// CreateDirectory creates a new directory
func (m *FileManager) CreateDirectory(ctx context.Context, dirPath string) error {
	dirPath, err := m.ResolvePath(dirPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(dirPath, 0755)
}

// Delete deletes a file or directory
func (m *FileManager) Delete(ctx context.Context, path string) error {
	path, err := m.ResolvePath(path)
	if err != nil {
		return err
	}
	// Never allow the jail root itself to be wiped.
	if filepath.Clean(path) == filepath.Clean(m.basePath) {
		return fmt.Errorf("refusing to delete the root directory")
	}
	return os.RemoveAll(path)
}

// Rename renames/moves a file or directory
func (m *FileManager) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath, err := m.ResolvePath(oldPath)
	if err != nil {
		return err
	}
	newPath, err = m.ResolvePath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// Copy copies a file or directory
func (m *FileManager) Copy(ctx context.Context, src, dst string) error {
	src, err := m.ResolvePath(src)
	if err != nil {
		return err
	}
	dst, err = m.ResolvePath(dst)
	if err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return m.copyDir(src, dst)
	}
	return m.copyFile(src, dst)
}

// ChangePermissions changes file permissions
func (m *FileManager) ChangePermissions(ctx context.Context, path string, mode os.FileMode) error {
	path, err := m.ResolvePath(path)
	if err != nil {
		return err
	}
	// Never hand out setuid/setgid bits through the API.
	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || mode&^os.FileMode(0777) != 0 {
		return fmt.Errorf("only permission bits 0000-0777 may be set")
	}
	return os.Chmod(path, mode)
}

// ChangeOwner changes file owner
func (m *FileManager) ChangeOwner(ctx context.Context, path, owner string) error {
	path, err := m.ResolvePath(path)
	if err != nil {
		return err
	}
	if !ownerRe.MatchString(owner) {
		return fmt.Errorf("owner must match ^[a-z_][a-z0-9_-]*(:[a-z_][a-z0-9_-]*)?$")
	}
	cmd := exec.CommandContext(ctx, "chown", "--", owner, path)
	return cmd.Run()
}

// GetDiskUsage gets disk usage of a path
func (m *FileManager) GetDiskUsage(ctx context.Context, path string) (map[string]interface{}, error) {
	path, err := m.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "du", "-sh", "--", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return nil, fmt.Errorf("unexpected du output")
	}
	size := fields[0]

	// Get filesystem info
	cmd = exec.CommandContext(ctx, "df", "-h", "--", path)
	dfOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"size":       size,
		"filesystem": string(dfOutput),
	}, nil
}

// SearchFiles searches for files matching a pattern
func (m *FileManager) SearchFiles(ctx context.Context, dir, pattern string) ([]FileInfo, error) {
	dir, err := m.ResolvePath(dir)
	if err != nil {
		return nil, err
	}
	if !searchPatternRe.MatchString(pattern) {
		return nil, fmt.Errorf("search pattern contains unsupported characters")
	}

	cmd := exec.CommandContext(ctx, "find", dir, "-maxdepth", "5", "-name", pattern)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		info, err := os.Stat(line)
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    filepath.Base(line),
			Path:    line,
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return files, nil
}

// Compress creates a tar.gz archive
func (m *FileManager) Compress(ctx context.Context, paths []string, dest string) error {
	dest, err := m.ResolvePath(dest)
	if err != nil {
		return err
	}

	args := []string{"-czf", dest, "--"}
	for _, p := range paths {
		resolved, err := m.ResolvePath(p)
		if err != nil {
			return err
		}
		args = append(args, resolved)
	}

	cmd := exec.CommandContext(ctx, "tar", args...)
	return cmd.Run()
}

// Extract extracts a tar.gz archive
func (m *FileManager) Extract(ctx context.Context, archive, dest string) error {
	archive, err := m.ResolvePath(archive)
	if err != nil {
		return err
	}
	dest, err = m.ResolvePath(dest)
	if err != nil {
		return err
	}

	// --no-absolute-names / -P off keeps entries from escaping dest, and
	// --no-same-owner avoids restoring root-owned files from a hostile archive.
	cmd := exec.CommandContext(ctx, "tar", "-xzf", archive, "-C", dest,
		"--no-absolute-names", "--no-same-owner", "--no-same-permissions")
	return cmd.Run()
}

// Upload handles file upload (returns a writer)
func (m *FileManager) Upload(ctx context.Context, destPath string, reader io.Reader) error {
	destPath, err := m.ResolvePath(destPath)
	if err != nil {
		return err
	}
	if err := validateUploadName(filepath.Base(destPath)); err != nil {
		return err
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Second layer of defence behind the handler's http.MaxBytesReader.
	written, err := io.Copy(f, io.LimitReader(reader, MaxUploadBytes+1))
	if err != nil {
		return err
	}
	if written > MaxUploadBytes {
		_ = os.Remove(destPath)
		return fmt.Errorf("uploaded file exceeds the %d byte limit", MaxUploadBytes)
	}
	return nil
}

// MaxUploadBytes caps a single uploaded file (100 MiB).
const MaxUploadBytes int64 = 100 << 20

// blockedUploadExts are never accepted through the upload endpoint because the
// panel writes into directories that a web server executes.
var blockedUploadExts = map[string]bool{
	".php": true, ".php3": true, ".php4": true, ".php5": true, ".php7": true,
	".php8": true, ".phps": true, ".phtml": true, ".pht": true,
	".cgi": true, ".pl": true, ".py": true, ".sh": true, ".bash": true,
	".jsp": true, ".jspx": true, ".asp": true, ".aspx": true, ".exe": true,
	".so": true, ".htaccess": true, ".htpasswd": true,
}

func validateUploadName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid file name")
	}
	lower := strings.ToLower(name)
	if blockedUploadExts[lower] {
		return fmt.Errorf("file name %q is not allowed", name)
	}
	// Check every extension so "shell.php.txt" and "shell.txt.php" both fail.
	for _, part := range strings.Split(lower, ".")[1:] {
		if blockedUploadExts["."+part] {
			return fmt.Errorf("file type .%s is not allowed", part)
		}
	}
	return nil
}

// ParseFileMode parses a string file mode (e.g., "0755", "644")
func ParseFileMode(modeStr string) (os.FileMode, error) {
	var mode uint32
	_, err := fmt.Sscanf(modeStr, "%o", &mode)
	if err != nil {
		// Try without leading zero
		_, err = fmt.Sscanf("0"+modeStr, "%o", &mode)
	}
	return os.FileMode(mode), err
}

// ResolvePath maps a client supplied path onto the jail and returns the
// absolute path to operate on. Absolute inputs, "../" segments and symlinks
// that point outside the jail are all rejected.
func (m *FileManager) ResolvePath(path string) (string, error) {
	if ContainsNullByte(path) {
		return "", fmt.Errorf("invalid path")
	}

	base := m.basePath
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	// The listing returns absolute paths, so an absolute input that already
	// sits inside the jail is taken as-is. Anything else - including an
	// absolute path pointing elsewhere - is re-anchored under the jail by
	// filepath.Clean("/"+path), which also strips any leading "..".
	cleaned := filepath.Clean(path)
	abs := filepath.Clean(filepath.Join(base, filepath.Clean("/"+path)))
	if filepath.IsAbs(cleaned) &&
		(cleaned == base || strings.HasPrefix(cleaned, base+string(filepath.Separator))) {
		abs = cleaned
	}

	// Follow symlinks on the deepest component that exists so a symlink inside
	// the jail cannot be used to reach outside of it.
	real := abs
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		real = resolved
	} else {
		parent := filepath.Dir(abs)
		if resolvedParent, perr := filepath.EvalSymlinks(parent); perr == nil {
			real = filepath.Join(resolvedParent, filepath.Base(abs))
		}
	}

	rel, err := filepath.Rel(base, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the allowed root")
	}

	if err := checkDenied(real); err != nil {
		return "", err
	}

	return real, nil
}

// checkDenied refuses paths that fall inside a sensitive system directory.
func checkDenied(p string) error {
	clean := filepath.Clean(p)
	for _, denied := range deniedRoots() {
		if clean == denied || strings.HasPrefix(clean, denied+string(filepath.Separator)) {
			return fmt.Errorf("path is not accessible")
		}
	}
	return nil
}

// ContainsNullByte reports whether s embeds a NUL byte.
func ContainsNullByte(s string) bool {
	return strings.ContainsRune(s, 0)
}

func (m *FileManager) getFileOwner(path string) string {
	cmd := exec.Command("stat", "-c", "%U", path)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func (m *FileManager) getMimeType(name string, isDir bool) string {
	if isDir {
		return "inode/directory"
	}

	ext := strings.ToLower(filepath.Ext(name))
	mimeTypes := map[string]string{
		".html": "text/html", ".css": "text/css", ".js": "application/javascript",
		".json": "application/json", ".xml": "application/xml", ".txt": "text/plain",
		".md": "text/markdown", ".py": "text/x-python", ".go": "text/x-go",
		".php": "application/x-httpd-php", ".sh": "application/x-sh",
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
		".gif": "image/gif", ".svg": "image/svg+xml", ".webp": "image/webp",
		".pdf": "application/pdf", ".zip": "application/zip", ".gz": "application/gzip",
		".tar": "application/x-tar", ".sql": "application/sql",
		".mp4": "video/mp4", ".mp3": "audio/mpeg", ".wav": "audio/wav",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

func (m *FileManager) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func (m *FileManager) copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return m.copyFile(path, dstPath)
	})
}
