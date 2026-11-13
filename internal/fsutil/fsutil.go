// Package fsutil holds file helpers shared by the store and config writers.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces path with data: the content is written to a temp
// file next to the target, fsynced, and renamed into place, so a crash mid
// write leaves the old file intact.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kabmat-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
