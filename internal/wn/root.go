package wn

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrNoRoot = errors.New("wn root not found: no .wn directory in current or parent directories")

// FindRootForCLI resolves the wn project root for CLI use. If WN_ROOT is set
// (e.g. by agent-orch when running a subagent in a worktree), uses that path
// so subagent commands like wn list and wn export find the main repo's .wn.
// Otherwise falls back to FindRoot() (walking up from cwd).
func FindRootForCLI() (string, error) {
	if r := os.Getenv("WN_ROOT"); r != "" {
		return FindRootFromDir(r)
	}
	return FindRoot()
}

// FindRoot walks up from the current directory until it finds a directory
// containing .wn, or hits the user's home. Returns the directory that
// contains .wn (the project root), or ErrNoRoot if not found.
func FindRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findRootFrom(dir)
}

// FindRootFromDir walks up from the given directory until it finds a directory
// containing .wn, or hits the user's home. Use this when the project root path
// is known (e.g. passed from an MCP client). dir may be the project root or any
// path under it. Returns ErrNoRoot if dir is empty or no .wn is found.
func FindRootFromDir(dir string) (string, error) {
	if dir == "" {
		return "", ErrNoRoot
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return findRootFrom(abs)
}

func findRootFrom(dir string) (string, error) {
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
