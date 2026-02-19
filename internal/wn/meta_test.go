package wn

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestWithMetaLock_Concurrent verifies that concurrent read-modify-write of meta
// is serialized and never produces corrupted JSON or lost updates.
func TestWithMetaLock_Concurrent(t *testing.T) {
	dir := t.TempDir()
	if err := InitRoot(dir); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		wg.Add(1)
		go func(currentID string) {
			defer wg.Done()
			err := WithMetaLock(dir, func(m Meta) (Meta, error) {
				m.CurrentID = currentID
				return m, nil
			})
			if err != nil {
				t.Errorf("WithMetaLock: %v", err)
			}
		}(id)
	}
	wg.Wait()

	m, err := ReadMeta(dir)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	// One of the 10 values should have been written last.
	if len(m.CurrentID) != 1 || m.CurrentID < "a" || m.CurrentID > "j" {
		t.Errorf("CurrentID = %q, want one of a..j", m.CurrentID)
	}
}

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
