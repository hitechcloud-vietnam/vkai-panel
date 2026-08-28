package config

// Thin aliases so the tests read as intent rather than as os calls.

import (
	"io/fs"
	"os"
)

func osStat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func osWriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
