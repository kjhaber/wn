//go:build windows

package wn

import "os"

func lockFile(f *os.File) error {
	// Advisory locking not implemented on Windows; no-op.
	return nil
}

func unlockFile(f *os.File) error {
	return nil
}
