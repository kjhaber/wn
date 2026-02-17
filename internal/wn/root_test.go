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
