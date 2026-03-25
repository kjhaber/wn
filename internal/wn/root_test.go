package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	tmp := t.TempDir()
	wnDir := filepath.Join(tmp, ".wn")
	itemsDir := filepath.Join(wnDir, "items")
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// From inside .wn we should find tmp as root (parent of .wn)
	sub := filepath.Join(wnDir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	root, err := FindRoot()
	if err != nil {
		t.Fatalf("FindRoot() err = %v", err)
	}
	// Normalize for macOS /private/var vs /var
	normRoot, _ := filepath.EvalSymlinks(root)
	normTmp, _ := filepath.EvalSymlinks(tmp)
	if normRoot != normTmp {
		t.Errorf("FindRoot() = %q (norm %q), want %q (norm %q)", root, normRoot, tmp, normTmp)
	}
}

func TestFindRoot_NoWn(t *testing.T) {
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	_, err := FindRoot()
	if err == nil {
		t.Error("FindRoot() expected error when .wn does not exist")
	}
}

func TestFindRootFromDir(t *testing.T) {
	tmp := t.TempDir()
	wnDir := filepath.Join(tmp, ".wn")
	itemsDir := filepath.Join(wnDir, "items")
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// From project root
	root, err := FindRootFromDir(tmp)
	if err != nil {
		t.Fatalf("FindRootFromDir(project root): %v", err)
	}
	if root != tmp {
		t.Errorf("FindRootFromDir(%q) = %q", tmp, root)
	}

	// From subdirectory under project
	sub := filepath.Join(tmp, "src", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	root, err = FindRootFromDir(sub)
	if err != nil {
		t.Fatalf("FindRootFromDir(subdir): %v", err)
	}
	if root != tmp {
		t.Errorf("FindRootFromDir(subdir) = %q, want %q", root, tmp)
	}
}

func TestFindRootFromDir_NoWn(t *testing.T) {
	tmp := t.TempDir()
	_, err := FindRootFromDir(tmp)
	if err == nil {
		t.Error("FindRootFromDir expected error when .wn does not exist")
	}
	if err != ErrNoRoot {
		t.Errorf("FindRootFromDir err = %v, want ErrNoRoot", err)
	}
}

func TestFindRootForCLI_UsesWN_ROOT(t *testing.T) {
	tmp := t.TempDir()
	wnDir := filepath.Join(tmp, ".wn")
	itemsDir := filepath.Join(wnDir, "items")
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run from a directory with no .wn (e.g. worktree), but WN_ROOT points to project
	otherDir := t.TempDir()
	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Setenv("WN_ROOT", tmp)
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	root, err := FindRootForCLI()
	if err != nil {
		t.Fatalf("FindRootForCLI() err = %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normTmp, _ := filepath.EvalSymlinks(tmp)
	if normRoot != normTmp {
		t.Errorf("FindRootForCLI() = %q (norm %q), want %q (norm %q)", root, normRoot, tmp, normTmp)
	}
}

func TestFindRootForCLI_GitWorktree(t *testing.T) {
	// Set up a main repo with .wn and a linked worktree (no .wn there).
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	// Create .wn in the main repo.
	if err := os.MkdirAll(filepath.Join(mainRepo, ".wn", "items"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a linked worktree.
	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-test-worktree")

	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Unsetenv("WN_ROOT")
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}

	root, err := FindRootForCLI()
	if err != nil {
		t.Fatalf("FindRootForCLI() from worktree: %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normMain, _ := filepath.EvalSymlinks(mainRepo)
	if normRoot != normMain {
		t.Errorf("FindRootForCLI() from worktree = %q (norm %q), want main repo %q (norm %q)", root, normRoot, mainRepo, normMain)
	}
}

func TestFindRootForCLI_GitWorktree_NoWn(t *testing.T) {
	// Worktree of a repo without .wn should still return ErrNoRoot.
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-no-wn-test")

	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Unsetenv("WN_ROOT")
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}

	_, err := FindRootForCLI()
	if err == nil {
		t.Error("FindRootForCLI() from worktree without .wn: expected error, got nil")
	}
}

func TestFindRootFromDir_Empty(t *testing.T) {
	_, err := FindRootFromDir("")
	if err == nil {
		t.Error("FindRootFromDir(\"\") expected error")
	}
	if err != ErrNoRoot {
		t.Errorf("FindRootFromDir(\"\") err = %v, want ErrNoRoot", err)
	}
}

func TestFindRoot_GitWorktree(t *testing.T) {
	// FindRoot() should find .wn in the main repo when called from a linked worktree.
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	if err := os.MkdirAll(filepath.Join(mainRepo, ".wn", "items"), 0755); err != nil {
		t.Fatal(err)
	}

	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-findroot-worktree-test")

	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Unsetenv("WN_ROOT")
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}

	root, err := FindRoot()
	if err != nil {
		t.Fatalf("FindRoot() from worktree: %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normMain, _ := filepath.EvalSymlinks(mainRepo)
	if normRoot != normMain {
		t.Errorf("FindRoot() from worktree = %q (norm %q), want main repo %q (norm %q)", root, normRoot, mainRepo, normMain)
	}
}

func TestGitRepoRoot_NormalRepo(t *testing.T) {
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	origWd, _ := os.Getwd()
	if err := os.Chdir(mainRepo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	root, err := GitRepoRoot()
	if err != nil {
		t.Fatalf("GitRepoRoot() err = %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normMain, _ := filepath.EvalSymlinks(mainRepo)
	if normRoot != normMain {
		t.Errorf("GitRepoRoot() = %q (norm %q), want %q (norm %q)", root, normRoot, mainRepo, normMain)
	}
}

func TestGitRepoRoot_Subdirectory(t *testing.T) {
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	sub := filepath.Join(mainRepo, "src", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	root, err := GitRepoRoot()
	if err != nil {
		t.Fatalf("GitRepoRoot() from subdir err = %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normMain, _ := filepath.EvalSymlinks(mainRepo)
	if normRoot != normMain {
		t.Errorf("GitRepoRoot() from subdir = %q (norm %q), want %q (norm %q)", root, normRoot, mainRepo, normMain)
	}
}

func TestGitRepoRoot_Worktree(t *testing.T) {
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-gitreporoot-test")

	origWd, _ := os.Getwd()
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	root, err := GitRepoRoot()
	if err != nil {
		t.Fatalf("GitRepoRoot() from worktree err = %v", err)
	}
	normRoot, _ := filepath.EvalSymlinks(root)
	normMain, _ := filepath.EvalSymlinks(mainRepo)
	if normRoot != normMain {
		t.Errorf("GitRepoRoot() from worktree = %q (norm %q), want main %q (norm %q)", root, normRoot, mainRepo, normMain)
	}
}

func TestGitRepoRoot_NotGit(t *testing.T) {
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	_, err := GitRepoRoot()
	if err == nil {
		t.Error("GitRepoRoot() expected error when not in git repo")
	}
}

func TestFindRoot_GitWorktree_NoWn(t *testing.T) {
	// FindRoot() from a worktree of a repo without .wn should return ErrNoRoot.
	mainRepo := t.TempDir()
	setupGitRepo(t, mainRepo)

	worktreeDir := t.TempDir()
	execIn(t, mainRepo, "git", "worktree", "add", worktreeDir, "-b", "wn-findroot-nowwn-test")

	origWd, _ := os.Getwd()
	origEnv := os.Getenv("WN_ROOT")
	_ = os.Unsetenv("WN_ROOT")
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		if origEnv == "" {
			_ = os.Unsetenv("WN_ROOT")
		} else {
			_ = os.Setenv("WN_ROOT", origEnv)
		}
	})
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}

	_, err := FindRoot()
	if err == nil {
		t.Error("FindRoot() from worktree without .wn: expected error, got nil")
	}
}
