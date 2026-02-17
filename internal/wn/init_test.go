package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitRoot(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatalf("InitRoot: %v", err)
	}
	wnPath := filepath.Join(dir, ".wn")
	info, err := os.Stat(wnPath)
	if err != nil || !info.IsDir() {
		t.Fatalf(".wn not created or not dir: err=%v", err)
	}
	itemsPath := filepath.Join(dir, ".wn", "items")
	info, err = os.Stat(itemsPath)
	if err != nil || !info.IsDir() {
		t.Fatalf(".wn/items not created or not dir: err=%v", err)
	}
}
