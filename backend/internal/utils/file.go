package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// DirExists checks if a directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

// CreateDir creates a directory if it doesn't exist
func CreateDir(path string) error {
	if !DirExists(path) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

// CreateFile creates a file if it doesn't exist
func CreateFile(path string) error {
	if !FileExists(path) {
		dir := filepath.Dir(path)
		if err := CreateDir(dir); err != nil {
			return err
		}
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		file.Close()
	}
	return nil
}

// ReadFile reads a file and returns its content
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes content to a file
func WriteFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := CreateDir(dir); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// DeleteFile deletes a file
func DeleteFile(path string) error {
	return os.Remove(path)
}

// DeleteDir deletes a directory and all its contents
func DeleteDir(path string) error {
	return os.RemoveAll(path)
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	dir := filepath.Dir(dst)
	if err := CreateDir(dir); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// MoveFile moves a file from src to dst
func MoveFile(src, dst string) error {
	return os.Rename(src, dst)
}

// GetFileSize returns the size of a file
func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetFileExtension returns the extension of a file
func GetFileExtension(path string) string {
	return filepath.Ext(path)
}

// GetFileName returns the name of a file without extension
func GetFileName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// GetFileNameWithExtension returns the name of a file with extension
func GetFileNameWithExtension(path string) string {
	return filepath.Base(path)
}

// GetDirName returns the directory name
func GetDirName(path string) string {
	return filepath.Dir(path)
}

// JoinPath joins path elements
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// AbsPath returns the absolute path
func AbsPath(path string) (string, error) {
	return filepath.Abs(path)
}

// RelPath returns the relative path
func RelPath(basePath, targetPath string) (string, error) {
	return filepath.Rel(basePath, targetPath)
}

// ListFiles lists all files in a directory
func ListFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// ListDirs lists all directories in a directory
func ListDirs(dir string) ([]string, error) {
	var dirs []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs, nil
}

// ListAll lists all files and directories in a directory
func ListAll(dir string) ([]string, error) {
	var items []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		items = append(items, entry.Name())
	}
	return items, nil
}

// WalkDir walks a directory and calls a function for each file
func WalkDir(dir string, fn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return fn(path, info)
	})
}

// FormatFileSize formats file size in human-readable format
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// IsHidden checks if a file is hidden
func IsHidden(path string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".")
}

// IsSymlink checks if a path is a symlink
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// GetSymlinkTarget returns the target of a symlink
func GetSymlinkTarget(path string) (string, error) {
	return os.Readlink(path)
}
