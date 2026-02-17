package wn

import (
	"encoding/json"
	"fmt"
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
	return os.WriteFile(s.itemPath(item.ID), data, 0644)
}

func (s *fileStore) Delete(id string) error {
	return os.Remove(s.itemPath(id))
}
