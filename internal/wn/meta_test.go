package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMeta_Missing(t *testing.T) {
	dir := t.TempDir()
	m, err := ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.CurrentID != "" {
		t.Errorf("CurrentID = %q", m.CurrentID)
	}
}

func TestWriteMeta_ReadMeta(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatal(err)
	}
	want := Meta{CurrentID: "abc123"}
	if err := WriteMeta(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentID != want.CurrentID {
		t.Errorf("CurrentID = %q, want %q", got.CurrentID, want.CurrentID)
	}
	path := filepath.Join(dir, ".wn", "meta.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("meta.json not created: %v", err)
	}
}
