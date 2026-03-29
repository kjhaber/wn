package main

import (
	"os"
	"testing"
)

func TestContextFileExists(t *testing.T) {
	if _, err := os.Stat("context.go"); err != nil {
		t.Errorf("expected context.go to exist: %v", err)
	}
}

func TestNewCmdCtxNoRoot(t *testing.T) {
	// Save and restore original WN_ROOT and working directory.
	origWNRoot := os.Getenv("WN_ROOT")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		_ = os.Setenv("WN_ROOT", origWNRoot)
	})

	// Point to a temp dir with no .wn so FindRootForCLI returns an error.
	tmp := t.TempDir()
	// Unset WN_ROOT to prevent it from overriding cwd-based search.
	_ = os.Unsetenv("WN_ROOT")
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	_, err = newCmdCtx("")
	if err == nil {
		t.Fatal("expected error when no .wn root found, got nil")
	}
}

func TestNewCmdCtxWithRoot(t *testing.T) {
	// Save and restore original WN_ROOT and working directory.
	origWNRoot := os.Getenv("WN_ROOT")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		_ = os.Setenv("WN_ROOT", origWNRoot)
	})

	// Create a temp dir with a .wn directory to simulate a valid wn root.
	tmp := t.TempDir()
	if err := os.Mkdir(tmp+"/.wn", 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("WN_ROOT")
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	ctx, err := newCmdCtx("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Root == "" {
		t.Error("expected Root to be set")
	}
	if ctx.Store == nil {
		t.Error("expected Store to be set")
	}
}

func TestNewCmdCtxExplicitRoot(t *testing.T) {
	// Create a temp dir with a .wn directory.
	tmp := t.TempDir()
	if err := os.Mkdir(tmp+"/.wn", 0755); err != nil {
		t.Fatal(err)
	}

	ctx, err := newCmdCtx(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Root != tmp {
		t.Errorf("expected Root %q, got %q", tmp, ctx.Root)
	}
	if ctx.Store == nil {
		t.Error("expected Store to be set")
	}
}
