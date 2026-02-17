package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Meta is stored in .wn/meta.json.
type Meta struct {
	CurrentID string `json:"current_id,omitempty"`
}

// ReadMeta reads .wn/meta.json from root. Missing file returns empty Meta, no error.
func ReadMeta(root string) (Meta, error) {
	path := filepath.Join(root, ".wn", "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, nil
		}
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

// WriteMeta writes .wn/meta.json under root.
func WriteMeta(root string, m Meta) error {
	dir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "meta.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
