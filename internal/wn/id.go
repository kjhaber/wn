package wn

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// IDPrefixLen is the length of work item IDs (6-char UUID prefix).
const IDPrefixLen = 6

// GenerateID returns a new 6-character lowercase hex ID that does not
// already exist in the store. Collision is avoided by checking the store.
func GenerateID(store Store) (string, error) {
	for i := 0; i < 100; i++ {
		b := make([]byte, IDPrefixLen/2)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		id := hex.EncodeToString(b)[:IDPrefixLen]
		_, err := store.Get(id)
		if err != nil {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not generate unique ID after 100 attempts")
}
