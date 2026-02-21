package wn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSettings_missingFile(t *testing.T) {
	// ReadSettings should return empty settings and no error when file does not exist
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("readSettingsFromPath(missing) err = %v", err)
	}
	if got.Sort != "" {
		t.Errorf("Sort = %q, want empty", got.Sort)
	}
}

func TestReadSettings_withSort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"sort":"updated:desc,priority"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFromPath(path)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Sort != "updated:desc,priority" {
		t.Errorf("Sort = %q, want updated:desc,priority", got.Sort)
	}
}
