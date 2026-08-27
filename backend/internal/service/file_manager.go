package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileManager provides web-based file management operations
type FileManager struct {
	basePath string // Base path restriction for security
}

func NewFileManager(basePath string) *FileManager {
	if basePath == "" {
		basePath = "/"
	}
	return &FileManager{basePath: basePath}
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
	// Sanitize path
	dirPath = m.sanitizePath(dirPath)

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
			IsDir:     entry.IsDir(),
			ModTime:  info.ModTime().Format("2006-01-02 15:04:05"),
			Owner:    owner,
			MimeType: m.getMimeType(entry.Name(), entry.IsDir()),
		})
	}

	return files, nil
}

// ReadFile reads the content of a file
func (m *FileManager) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	filePath = m.sanitizePath(filePath)

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
	filePath = m.sanitizePath(filePath)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(filePath, content, 0644)
}

// CreateDirectory creates a new directory
func (m *FileManager) CreateDirectory(ctx context.Context, dirPath string) error {
	dirPath = m.sanitizePath(dirPath)
	return os.MkdirAll(dirPath, 0755)
}

// Delete deletes a file or directory
func (m *FileManager) Delete(ctx context.Context, path string) error {
	path = m.sanitizePath(path)
	return os.RemoveAll(path)
}

// Rename renames/moves a file or directory
func (m *FileManager) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = m.sanitizePath(oldPath)
	newPath = m.sanitizePath(newPath)
	return os.Rename(oldPath, newPath)
}

// Copy copies a file or directory
func (m *FileManager) Copy(ctx context.Context, src, dst string) error {
	src = m.sanitizePath(src)
	dst = m.sanitizePath(dst)

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
	path = m.sanitizePath(path)
	return os.Chmod(path, mode)
}

// ChangeOwner changes file owner
func (m *FileManager) ChangeOwner(ctx context.Context, path, owner string) error {
	path = m.sanitizePath(path)
	cmd := exec.CommandContext(ctx, "chown", owner, path)
	return cmd.Run()
}

// GetDiskUsage gets disk usage of a path
func (m *FileManager) GetDiskUsage(ctx context.Context, path string) (map[string]interface{}, error) {
	path = m.sanitizePath(path)

	cmd := exec.CommandContext(ctx, "du", "-sh", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	size := strings.Fields(string(output))[0]

	// Get filesystem info
	cmd = exec.CommandContext(ctx, "df", "-h", path)
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
	dir = m.sanitizePath(dir)

	cmd := exec.CommandContext(ctx, "find", dir, "-name", pattern, "-maxdepth", "5")
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
			Name:  filepath.Base(line),
			Path:  line,
			Size:  info.Size(),
			Mode:  info.Mode().String(),
			IsDir: info.IsDir(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return files, nil
}

// Compress creates a tar.gz archive
func (m *FileManager) Compress(ctx context.Context, paths []string, dest string) error {
	dest = m.sanitizePath(dest)

	args := []string{"-czf", dest}
	for _, p := range paths {
		args = append(args, m.sanitizePath(p))
	}

	cmd := exec.CommandContext(ctx, "tar", args...)
	return cmd.Run()
}

// Extract extracts a tar.gz archive
func (m *FileManager) Extract(ctx context.Context, archive, dest string) error {
	archive = m.sanitizePath(archive)
	dest = m.sanitizePath(dest)

	cmd := exec.CommandContext(ctx, "tar", "-xzf", archive, "-C", dest)
	return cmd.Run()
}

// Upload handles file upload (returns a writer)
func (m *FileManager) Upload(ctx context.Context, destPath string, reader io.Reader) error {
	destPath = m.sanitizePath(destPath)

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

// sanitizePath prevents path traversal attacks
func (m *FileManager) sanitizePath(path string) string {
	// Clean the path
	path = filepath.Clean(path)

	// Ensure it doesn't go above root
	if path == ".." || strings.HasPrefix(path, "../") {
		return m.basePath
	}

	return path
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
