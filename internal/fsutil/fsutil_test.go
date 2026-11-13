package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicCreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	if err := WriteFileAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "hello" {
		t.Fatalf("read %q %v", b, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v %v", info.Mode().Perm(), err)
	}
	// No temp files left behind.
	if _, err := os.ReadDir(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFileAtomicReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "new" {
		t.Fatalf("read %q %v", b, err)
	}
	// The old file's inode is gone; no stale temp file remains.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "data" && strings.HasPrefix(e.Name(), ".kabmat-") {
			t.Fatalf("leftover temp file %q", e.Name())
		}
	}
}

func TestWriteFileAtomicFailsClean(t *testing.T) {
	dir := t.TempDir()
	// A directory in place of a file makes the rename fail.
	path := filepath.Join(dir, "data")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("rename over a directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "data" && strings.HasPrefix(e.Name(), ".kabmat-") {
			t.Fatalf("leftover temp file %q", e.Name())
		}
	}
}
