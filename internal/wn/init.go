package wn

import (
	"os"
	"path/filepath"
)

// InitRoot creates .wn and .wn/items under dir. Idempotent.
func InitRoot(dir string) error {
	itemsDir := filepath.Join(dir, ".wn", "items")
	return os.MkdirAll(itemsDir, 0755)
}
