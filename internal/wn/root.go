package wn

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoRoot = errors.New("wn root not found: no .wn directory in current or parent directories")

// FindRootForCLI resolves the wn project root for CLI use. Tries in order:
//  1. WN_ROOT env var (set e.g. by agent-orch for subagents)
//  2. Walk up from cwd looking for .wn
//  3. Git worktree detection: if cwd is a linked worktree, find the main
//     repo via git rev-parse --git-common-dir and look for .wn there
func FindRootForCLI() (string, error) {
	if r := os.Getenv("WN_ROOT"); r != "" {
		return FindRootFromDir(r)
	}
	root, err := FindRoot()
	if err == nil {
		return root, nil
	}
	if err != ErrNoRoot {
		return "", err
	}
	return findRootViaGitWorktree()
}

// findRootViaGitWorktree detects if cwd is a git linked worktree and, if so,
// looks for .wn in the main repo. git rev-parse --git-common-dir returns the
// common .git directory (absolute path when in a worktree), whose parent is
// the main repo root.
func findRootViaGitWorktree() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ErrNoRoot
	}
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", ErrNoRoot
	}
	gitCommonDir := strings.TrimSpace(string(out))
	if gitCommonDir == "" {
		return "", ErrNoRoot
	}
	if !filepath.IsAbs(gitCommonDir) {
		gitCommonDir = filepath.Join(cwd, gitCommonDir)
	}
	mainRepoRoot := filepath.Dir(gitCommonDir)
	root, err := FindRootFromDir(mainRepoRoot)
	if err != nil {
		return "", ErrNoRoot
	}
	return root, nil
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
