package store

import (
	"os"
	"path/filepath"
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
