package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"kabmat/internal/store"
)

func TestApplyArgs(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	tuiOn, open, err := applyArgs([]string{"-c", "Work", "-l"}, s, &out)
	if err != nil || tuiOn || open != "" || strings.TrimSpace(out.String()) != "Work" {
		t.Fatalf("tui=%v open=%q err=%v out=%q", tuiOn, open, err, out.String())
	}
	_, _, err = applyArgs([]string{"-o", "missing"}, s, &out)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
}
