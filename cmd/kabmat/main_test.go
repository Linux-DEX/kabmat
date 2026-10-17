package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Linux-DEX/kabmat/internal/store"
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

func TestAddCardFlag(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	tuiOn, _, err := applyArgs([]string{"-a", "buy milk", "-o", "Work"}, s, &out)
	if err != nil || tuiOn {
		t.Fatalf("tui=%v err=%v", tuiOn, err)
	}
	got := s.Boards[0].Columns[0].Cards
	if len(got) != 1 || got[0].Content != "buy milk" || len(s.Boards[0].Columns[1].Cards) != 0 {
		t.Fatalf("cards: %+v", s.Boards[0].Columns)
	}
	if _, _, err := applyArgs([]string{"-a", "nope"}, s, &out); err == nil || !strings.Contains(err.Error(), "no board") {
		t.Fatal(err)
	}
}
