package main

import (
	"bytes"
	"os"
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
	if _, _, err := applyArgs([]string{"-a", "ship", "-o", "Work", "-C", "Done"}, s, &out); err != nil {
		t.Fatal(err)
	}
	if len(s.Boards[0].Columns[1].Cards) != 1 || s.Boards[0].Columns[1].Cards[0].Content != "ship" {
		t.Fatalf("column: %+v", s.Boards[0].Columns)
	}
	if _, _, err := applyArgs([]string{"-a", "n", "-o", "Work", "-C", "2"}, s, &out); err != nil {
		t.Fatal(err)
	}
}

func TestAddFlagRejectsNewlineAndKeepsFile(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	before, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, _, err = applyArgs([]string{"-a", "first\nsecond", "-o", "Work"}, s, &out)
	if err == nil || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("want newline error, got %v", err)
	}
	after, err := os.ReadFile(s.Path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed add changed the file: %v", err)
	}
	s2, err := store.Load(s.Path, s.Backup)
	if err != nil {
		t.Fatalf("file corrupted: %v", err)
	}
	if len(s2.Boards) != 1 || len(s2.Boards[0].Columns[0].Cards) != 0 {
		t.Fatalf("boards: %+v", s2.Boards)
	}
}

func TestCLIAddThenReload(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "bkp"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, _, err := applyArgs([]string{"-c", "Work", "-T", "-a", "buy milk", "-o", "Work"}, s, &out); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Load(s.Path, s.Backup)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := s2.Boards[0].Columns[0].Cards
	if len(got) != 1 || got[0].Content != "buy milk" {
		t.Fatalf("cards: %+v", got)
	}
}

func TestTemplateAndExport(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, _, err := applyArgs([]string{"-T"}, s, &out); err == nil {
		t.Fatal("template without -c should fail")
	}
	tuiOn, _, err := applyArgs([]string{"-c", "Work", "-T"}, s, &out)
	if err != nil || !tuiOn || len(s.Boards[0].Columns) != 3 || !s.Boards[0].Columns[2].Gate {
		t.Fatalf("tui=%v err=%v cols=%+v", tuiOn, err, s.Boards[0].Columns)
	}
	out.Reset()
	path := filepath.Join(dir, "board.md")
	if _, _, err := applyArgs([]string{"-x", path, "-o", "Work"}, s, &out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "# Work") || !strings.Contains(string(b), "## Done (0) · checklist") {
		t.Fatalf("export %q %v", b, err)
	}
	out.Reset()
	if _, _, err := applyArgs([]string{"-x"}, s, &out); err != nil || !strings.Contains(out.String(), "# Work") {
		t.Fatal(err, out.String())
	}
}
