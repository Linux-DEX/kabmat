package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `Work
 Todo
  write tests
   -parser
   +cli
    line one
    line two
 Done
  ship
Home
`

func TestParseRoundTrip(t *testing.T) {
	boards, err := parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 || boards[0].Name != "Work" || boards[1].Name != "Home" {
		t.Fatalf("boards: %+v", boards)
	}
	col := boards[0].Columns
	if len(col) != 2 || col[0].Title != "Todo" || col[1].Title != "Done" {
		t.Fatalf("columns: %+v", col)
	}
	card := col[0].Cards[0]
	if card.Content != "write tests" || card.Description != "line one\nline two" {
		t.Fatalf("card: %+v", card)
	}
	if len(card.Checklist) != 2 || card.Checklist[0].Done || card.Checklist[0].Content != "parser" || !card.Checklist[1].Done {
		t.Fatalf("checklist: %+v", card.Checklist)
	}
	if got := encode(boards); got != sample {
		t.Fatalf("encode\n%s", got)
	}
}

func TestSubtaskRoundTrip(t *testing.T) {
	const raw = "Work\n" +
		" Todo\n" +
		"  parent\n" +
		"   -check\n" +
		"   >-child\n" +
		"   >> notes\n" +
		"   >+done child\n" +
		"    parent desc\n"
	boards, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	card := boards[0].Columns[0].Cards[0]
	if card.Content != "parent" || card.Description != "parent desc" || len(card.Checklist) != 1 {
		t.Fatalf("card: %+v", card)
	}
	if len(card.Subtasks) != 2 || card.Subtasks[0].Content != "child" || card.Subtasks[0].Done || card.Subtasks[0].Description != "notes" {
		t.Fatalf("subtasks: %+v", card.Subtasks)
	}
	if !card.Subtasks[1].Done || card.Subtasks[1].Content != "done child" {
		t.Fatalf("done subtask: %+v", card.Subtasks[1])
	}
	if got := encode(boards); got != raw {
		t.Fatalf("encode\n%s", got)
	}
}

func TestMoveCardAcross(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveCardHoriz(0, 0, 0, 1); err != nil {
		t.Fatal(err)
	}
	if len(s.Boards[0].Columns[0].Cards) != 0 || s.Boards[0].Columns[1].Cards[0].Content != "ship" {
		t.Fatalf("top insert failed: %+v", s.Boards[0].Columns)
	}
	if err := s.AddCard(0, 1, Card{Content: "stay"}); err != nil {
		t.Fatal(err)
	}
	s.CardAtBottom = true
	if _, err := s.MoveCardHoriz(0, 1, 0, -1); err != nil {
		t.Fatal(err)
	}
	got := s.Boards[0].Columns[0].Cards
	if len(got) != 1 || got[0].Content != "ship" {
		t.Fatalf("bottom insert: %+v", got)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	rawb, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(rawb)
	again, err := parse(raw)
	if err != nil || again[0].Columns[0].Cards[0].Content != "ship" {
		t.Fatalf("reload: %v %+v", err, again)
	}
}

func TestUndoRedo(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Undo(); !ok || err != nil || len(s.Boards[0].Columns) != 0 {
		t.Fatalf("undo 1: ok=%v err=%v", ok, err)
	}
	if ok, err := s.Undo(); !ok || err != nil || len(s.Boards) != 0 {
		t.Fatalf("undo 2: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.Undo(); ok {
		t.Fatal("undo past the beginning should do nothing")
	}
	if ok, _ := s.Redo(); !ok {
		t.Fatal("redo 1")
	}
	if ok, _ := s.Redo(); !ok || len(s.Boards[0].Columns) != 1 {
		t.Fatal("redo 2")
	}
	if ok, _ := s.Redo(); ok {
		t.Fatal("redo past the end should do nothing")
	}
}

func TestBoardNames(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard(" Work "); err == nil || s.CreateBoard("bad\nname") == nil || s.CreateBoard("  ") == nil {
		t.Fatal("expected duplicate, newline, and blank names to fail")
	}
	if err := s.RenameBoard(0, "Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Home"); err != nil || s.RenameBoard(1, "Work") == nil {
		t.Fatal("rename should not collide")
	}
	if err := s.DeleteBoardNamed("Work"); err != nil || len(s.Boards) != 1 || s.Boards[0].Name != "Home" {
		t.Fatalf("delete: %+v %v", s.Boards, err)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	const raw = "Work\n" +
		" Doing {wip:2,check}\n" +
		"  ship\n" +
		"   @pri:2 due:2026-10-20 tag:work tag:home moved:2026-10-01\n" +
		"   -box\n" +
		" Archive {archive}\n" +
		" Review {draft}\n"
	boards, err := parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	col := boards[0].Columns
	if len(col) != 3 || col[0].WIP != 2 || !col[0].Gate || col[0].Title != "Doing" {
		t.Fatalf("doing: %+v", col[0])
	}
	if !col[1].Archive || col[1].Title != "Archive" || col[2].Title != "Review {draft}" {
		t.Fatalf("columns: %+v", col)
	}
	card := col[0].Cards[0]
	if card.Priority != 2 || card.Due != "2026-10-20" || card.Moved != "2026-10-01" || len(card.Tags) != 2 || card.Tags[0] != "work" {
		t.Fatalf("card: %+v", card)
	}
	if len(card.Checklist) != 1 || card.Checklist[0].Content != "box" {
		t.Fatalf("checklist: %+v", card.Checklist)
	}
	if got := encode(boards); got != raw {
		t.Fatalf("encode\n%s", got)
	}
}

func TestWIPGateArchive(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateKanban("Work"); err != nil {
		t.Fatal(err)
	}
	cols := s.Boards[0].Columns
	if len(cols) != 3 || cols[2].Title != "Done" || !cols[2].Gate {
		t.Fatalf("template: %+v", cols)
	}
	card := Card{Content: "ship", Checklist: []ChecklistItem{{Content: "box"}}}
	if err := s.AddCard(0, 0, card); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveCardTo(0, 0, 0, 2); err == nil || !strings.Contains(err.Error(), "checklist") {
		t.Fatal(err)
	}
	card.Checklist[0].Done = true
	if err := s.UpdateCard(0, 0, 0, card); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveCardTo(0, 0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWIP(0, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, Card{Content: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, Card{Content: "two"}); err == nil || !strings.Contains(err.Error(), "WIP") {
		t.Fatal(err)
	}
	if _, err := s.ArchiveCard(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(s.Boards[0].Columns[0].Cards) != 0 || s.Boards[0].Columns[3].Title != "Archive" || !s.Boards[0].Columns[3].Archive {
		t.Fatalf("archive: %+v", s.Boards[0].Columns)
	}
	if _, err := s.ArchiveCard(0, 3, 0); err != ErrArchived {
		t.Fatal(err)
	}
	md := Markdown(s.Boards)
	if !strings.Contains(md, "# Work") || !strings.Contains(md, "## Archive (1)") || !strings.Contains(md, "ship") {
		t.Fatalf("markdown:\n%s", md)
	}
	if _, err := s.Boards[0].ColumnIndex("Doing"); err != nil {
		t.Fatal(err)
	}
	if i, err := s.Boards[0].ColumnIndex("2"); err != nil || i != 1 {
		t.Fatalf("index %d %v", i, err)
	}
}

func TestDeleteCards(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if err := s.AddCard(0, 0, Card{Content: name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteCards(0, [][2]int{{0, 0}, {0, 2}}); err != nil {
		t.Fatal(err)
	}
	got := s.Boards[0].Columns[0].Cards
	if len(got) != 1 || got[0].Content != "b" {
		t.Fatalf("left: %+v", got)
	}
}

func TestFlushWritesOncePerSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	backup := filepath.Join(dir, "data_bkp")
	if err := os.WriteFile(path, []byte("Work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path, backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "Work\n" {
		t.Fatalf("disk changed before flush: %q %v", raw, err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	bkp, err := os.ReadFile(backup)
	if err != nil || string(bkp) != "Work\n" {
		t.Fatalf("backup: %q %v", bkp, err)
	}
	if err := s.AddColumn(0, "Done"); err != nil || s.Flush() != nil {
		t.Fatal(err)
	}
	bkp, err = os.ReadFile(backup)
	if err != nil || string(bkp) != "Work\n" {
		t.Fatalf("backup rewritten: %q %v", bkp, err)
	}
	raw, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "Done") {
		t.Fatalf("flush: %q %v", raw, err)
	}
}

func TestRejectsUnsafeTagsAndControls(t *testing.T) {
	if tags := ParseTags("#Bug ok ../etc a\x1b[31m b"); len(tags) != 3 || tags[0] != "bug" || tags[1] != "ok" || tags[2] != "b" {
		t.Fatalf("%v", tags)
	}
	if _, err := CheckTags("ok ../etc"); err == nil {
		t.Fatal("bad tag should be refused")
	}
	tags, err := CheckTags("#Work work, HOME")
	if err != nil || len(tags) != 2 || tags[0] != "work" || tags[1] != "home" {
		t.Fatalf("%v %v", tags, err)
	}
	boards, err := parse("Work\n Todo\n  hello\x1b[31mworld\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := boards[0].Columns[0].Cards[0].Content; got != "hello[31mworld" || strings.Contains(got, "\x1b") {
		t.Fatalf("%q", got)
	}
}
