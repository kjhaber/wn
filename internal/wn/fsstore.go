package wn

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const itemsDirName = "items"

// NewFileStore returns a file-based store at root (directory containing .wn).
// It creates .wn/items if it does not exist.
func NewFileStore(root string) (Store, error) {
	wnDir := filepath.Join(root, ".wn")
	itemsDir := filepath.Join(wnDir, itemsDirName)
	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		return nil, err
	}
	return &fileStore{root: root, itemsDir: itemsDir}, nil
}

type fileStore struct {
	root     string
	itemsDir string
}

func (s *fileStore) Root() string { return s.root }

func (s *fileStore) itemPath(id string) string {
	return filepath.Join(s.itemsDir, id+".json")
}

func (s *fileStore) List() ([]*Item, error) {
	entries, err := os.ReadDir(s.itemsDir)
	if err != nil {
		return nil, err
	}
	var items []*Item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		item, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *fileStore) Get(id string) (*Item, error) {
	data, err := os.ReadFile(s.itemPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("item %s not found", id)
		}
		return nil, err
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *fileStore) Put(item *Item) error {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	path := s.itemPath(item.ID)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer func() { _ = unlockFile(f) }()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// UpdateItem runs fn with the item under exclusive lock (read-modify-write).
func (s *fileStore) UpdateItem(id string, fn func(*Item) (*Item, error)) error {
	path := s.itemPath(id)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("item %s not found", id)
		}
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer func() { _ = unlockFile(f) }()
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	updated, err := fn(&item)
	if err != nil {
		return err
	}
	if updated == nil {
		return nil
	}
	data, err = json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func (s *fileStore) Delete(id string) error {
	path := s.itemPath(id)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("item %s not found", id)
		}
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer func() { _ = unlockFile(f) }()
	return os.Remove(path)
}
