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

func TestFindRootFromDir_Empty(t *testing.T) {
	_, err := FindRootFromDir("")
	if err == nil {
		t.Error("FindRootFromDir(\"\") expected error")
	}
	if err != ErrNoRoot {
		t.Errorf("FindRootFromDir(\"\") err = %v, want ErrNoRoot", err)
	}
}
