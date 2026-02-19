package wn

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const metaLockName = "meta.lock"

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

// WithMetaLock runs fn with exclusive lock on meta; fn receives current Meta and returns the updated Meta to write.
// Use for read-modify-write of meta (e.g. setting CurrentID) so concurrent callers are serialized.
func WithMetaLock(root string, fn func(Meta) (Meta, error)) error {
	dir := filepath.Join(root, ".wn")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	lockPath := filepath.Join(dir, metaLockName)
	lf, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := lockFile(lf); err != nil {
		return err
	}
	defer func() { _ = unlockFile(lf) }()

	m, err := ReadMeta(root)
	if err != nil {
		return err
	}
	updated, err := fn(m)
	if err != nil {
		return err
	}
	return WriteMeta(root, updated)
}
