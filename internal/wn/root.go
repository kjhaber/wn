package wn

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrNoRoot = errors.New("wn root not found: no .wn directory in current or parent directories")

// FindRoot walks up from the current directory until it finds a directory
// containing .wn, or hits the user's home. Returns the directory that
// contains .wn (the project root), or ErrNoRoot if not found.
func FindRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	for {
		wnPath := filepath.Join(dir, ".wn")
		info, err := os.Stat(wnPath)
		if err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir || (home != "" && dir == home) {
			return "", ErrNoRoot
		}
		dir = parent
	}
}
