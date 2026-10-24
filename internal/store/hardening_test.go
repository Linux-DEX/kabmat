package store

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Load(filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "bkp"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCardContentRejectsNewline(t *testing.T) {
	for _, text := range []string{"a\nb", "a\rb", "a\r\nb", "\nlead"} {
		dir := t.TempDir()
		s, err := Load(filepath.Join(dir, "data"), filepath.Join(dir, "bkp"))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.CreateBoard("Work"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddColumn(0, "Todo"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddCard(0, 0, Card{Content: "keep"}); err != nil {
			t.Fatal(err)
		}
		s.Flush()
		before, err := os.ReadFile(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []struct {
			name string
			run  func() error
		}{
			{"AddCard", func() error { return s.AddCard(0, 0, Card{Content: text}) }},
			{"InsertCard", func() error { return s.InsertCard(0, 0, 0, Card{Content: text}) }},
			{"UpdateCard", func() error { return s.UpdateCard(0, 0, 0, Card{Content: text}) }},
		} {
			if err := call.run(); err == nil || !strings.Contains(err.Error(), "newline") {
				t.Fatalf("%s %q: want newline error, got %v", call.name, text, err)
			}
		}
		after, err := os.ReadFile(s.Path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("%q changed the file: %v", text, err)
		}
		if _, err := Load(s.Path, s.Backup); err != nil {
			t.Fatalf("%q: file no longer readable: %v", text, err)
		}
	}
}

func TestColumnTitleRejectsBraces(t *testing.T) {
	for _, title := range []string{"Urgent {wip:2}", "Use {x}", "Odd }", "{open", "br acket\nx"} {
		s := newStore(t)
		if err := s.CreateBoard("Work"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddColumn(0, "Todo"); err != nil {
			t.Fatal(err)
		}
		if err := s.AddColumn(0, title); err == nil || !strings.Contains(err.Error(), "braces") {
			t.Fatalf("AddColumn %q: want braces error, got %v", title, err)
		}
		if err := s.RenameColumn(0, 0, title); err == nil || !strings.Contains(err.Error(), "braces") {
			t.Fatalf("RenameColumn %q: want braces error, got %v", title, err)
		}
		if _, err := Load(s.Path, s.Backup); err != nil {
			t.Fatalf("%q: file no longer readable: %v", title, err)
		}
		if s.Boards[0].Columns[0].Title != "Todo" {
			t.Fatalf("%q: title got renamed to %q", title, s.Boards[0].Columns[0].Title)
		}
	}
}

func TestValidTitlesStillWork(t *testing.T) {
	titles := []string{"Todo", "Doing / review step", "QA + sign-off", "#feedback", "周 会"}
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	for _, title := range titles {
		if err := s.AddColumn(0, title); err != nil {
			t.Fatalf("AddColumn %q: %v", title, err)
		}
	}
	if err := s.AddCard(0, 0, Card{Content: "note {wip:1}", Tags: []string{"bug"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(s.Path, s.Backup)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(s2.Boards[0].Columns) != len(titles) {
		t.Fatalf("columns: %+v", s2.Boards[0].Columns)
	}
	for i, title := range titles {
		if s2.Boards[0].Columns[i].Title != title {
			t.Fatalf("title %d: %q != %q", i, s2.Boards[0].Columns[i].Title, title)
		}
	}
	if got := s2.Boards[0].Columns[0].Cards[0].Content; got != "note {wip:1}" {
		t.Fatalf("card content: %q", got)
	}
}

func TestBlankLineSkipped(t *testing.T) {
	boards, err := parse("b1\n\n c\n  x\n\n\nb2\n")
	if err != nil {
		t.Fatalf("blank lines must be ignored: %v", err)
	}
	if len(boards) != 2 {
		t.Fatalf("boards: %+v", boards)
	}
	if boards[0].Name != "b1" || boards[1].Name != "b2" {
		t.Fatalf("names: %+v", boards)
	}
	if got := boards[0].Columns[0].Cards; len(got) != 1 || got[0].Content != "x" {
		t.Fatalf("cards: %+v", got)
	}
	// empty description lines (whitespace-only) are still description content
	boards, err = parse("b\n c\n  x\n    desc\n    \n    more\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := boards[0].Columns[0].Cards[0].Description; got != "desc\n\nmore" {
		t.Fatalf("description: %q", got)
	}
}

// A board line that looks indented stays a loud parse error, never a silent
// mis-parse: it pins the parser so hand-edited files fail with line numbers.
func TestLeadingSpaceBoardErrors(t *testing.T) {
	_, err := parse("  Hacked\n c\n  x\n")
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("want line 1 error, got %v", err)
	}
}

func TestBulkMoveOneUndoStep(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Todo", "Doing", "Done"} {
		if err := s.AddColumn(0, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"a", "b", "c"} {
		if err := s.AddCard(0, 0, Card{Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCard(0, 1, Card{Content: "d"}); err != nil {
		t.Fatal(err)
	}
	before := encode(s.Boards)
	steps := len(s.undo)

	// picks look like tui.movePicks: same column bottom-up, dst column last
	dst, n, err := s.MoveCardsTo(0, [][2]int{{0, 2}, {0, 1}, {0, 0}, {1, 0}}, 2)
	if err != nil || n != 4 || dst != 2 {
		t.Fatalf("dst=%d n=%d err=%v", dst, n, err)
	}
	if got := len(s.undo) - steps; got != 1 {
		t.Fatalf("bulk move saved %d undo steps, want 1", got)
	}
	done := s.Boards[0].Columns[2].Cards
	for i, want := range []string{"d", "a", "b", "c"} {
		if done[i].Content != want {
			t.Fatalf("Done[%d]=%q want %q: %+v", i, done[i].Content, want, done)
		}
	}
	if ok, err := s.Undo(); !ok || err != nil {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if again := encode(s.Boards); again != before {
		t.Fatalf("undo did not revert the whole batch\n%s", again)
	}
}

func TestBulkShiftOneUndoStep(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Todo", "Doing", "Done"} {
		if err := s.AddColumn(0, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"a", "b"} {
		if err := s.AddCard(0, 0, Card{Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCard(0, 1, Card{Content: "c"}); err != nil {
		t.Fatal(err)
	}
	before := encode(s.Boards)
	steps := len(s.undo)

	dst, n, err := s.ShiftCards(0, [][2]int{{0, 1}, {0, 0}}, 1)
	if err != nil || n != 2 || dst != 1 {
		t.Fatalf("dst=%d n=%d err=%v", dst, n, err)
	}
	if got := len(s.undo) - steps; got != 1 {
		t.Fatalf("bulk shift saved %d undo steps, want 1", got)
	}
	doing := s.Boards[0].Columns[1].Cards
	if doing[0].Content != "a" || doing[1].Content != "b" || doing[2].Content != "c" {
		t.Fatalf("Doing: %+v", doing)
	}
	if ok, err := s.Undo(); !ok || err != nil {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if again := encode(s.Boards); again != before {
		t.Fatalf("undo did not revert the batch\n%s", again)
	}
}

func TestBulkArchive(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Todo", "Doing"} {
		if err := s.AddColumn(0, title); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"a", "b", "c"} {
		if err := s.AddCard(0, 0, Card{Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	before := encode(s.Boards)
	steps := len(s.undo)

	dst, n, err := s.ArchiveCards(0, [][2]int{{0, 2}, {0, 1}, {0, 0}, {1, 0}})
	if err != nil || n != 3 || dst < 0 {
		t.Fatalf("dst=%d n=%d err=%v", dst, n, err)
	}
	if got := len(s.undo) - steps; got != 1 {
		t.Fatalf("bulk archive saved %d undo steps, want 1", got)
	}
	arch := s.Boards[0].Columns[dst]
	if !arch.Archive || len(arch.Cards) != 3 {
		t.Fatalf("archive: %+v", arch)
	}
	// archiving picks that are already in the archive column skips them
	_, n, err = s.ArchiveCards(0, [][2]int{{dst, 0}, {dst, 1}})
	if err != nil || n != 0 {
		t.Fatalf("re-archive: n=%d err=%v", n, err)
	}
	if ok, err := s.Undo(); !ok || err != nil {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if again := encode(s.Boards); again != before {
		t.Fatalf("undo did not revert the batch\n%s", again)
	}
}

func TestArchiveCardsSkipsEveryArchiveColumn(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Todo", "Old", "Other"} {
		if err := s.AddColumn(0, title); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCard(0, 0, Card{Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 1, Card{Content: "keep"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 2, Card{Content: "stay"}); err != nil {
		t.Fatal(err)
	}
	s.Boards[0].Columns[1].Archive = true
	s.Boards[0].Columns[2].Archive = true
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	before := encode(s.Boards)

	dst, n, err := s.ArchiveCards(0, [][2]int{{2, 0}, {0, 0}, {1, 0}})
	if err != nil || n != 1 || dst != 1 {
		t.Fatalf("dst=%d n=%d err=%v", dst, n, err)
	}
	old := s.Boards[0].Columns[1].Cards
	if len(old) != 2 || old[0].Content != "a" || old[1].Content != "keep" {
		t.Fatalf("first archive: %+v", old)
	}
	other := s.Boards[0].Columns[2].Cards
	if len(other) != 1 || other[0].Content != "stay" {
		t.Fatalf("second archive: %+v", other)
	}
	if ok, err := s.Undo(); !ok || err != nil {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if again := encode(s.Boards); again != before {
		t.Fatalf("undo did not revert\n%s", again)
	}
}

func TestBulkMovePartialBlockedKeepsOneUndoStep(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Done"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWIP(0, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, Card{Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, Card{Content: "b"}); err != nil {
		t.Fatal(err)
	}
	before := encode(s.Boards)
	steps := len(s.undo)

	// first pick fits under the WIP limit, the second is refused: earlier
	// moves stay and the whole (partial) batch is one undo step
	_, n, err := s.MoveCardsTo(0, [][2]int{{0, 1}, {0, 0}}, 1)
	if err == nil || !strings.Contains(err.Error(), "WIP") {
		t.Fatalf("want WIP error, got %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 moved card, got %d", n)
	}
	if got := len(s.undo) - steps; got != 1 {
		t.Fatalf("partial batch saved %d undo steps, want 1", got)
	}
	if got := s.Boards[0].Columns[1].Cards; len(got) != 1 || got[0].Content != "b" {
		t.Fatalf("Done: %+v", got)
	}
	if ok, err := s.Undo(); !ok || err != nil {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if again := encode(s.Boards); again != before {
		t.Fatalf("undo did not revert the partial batch\n%s", again)
	}
}

func TestRedoCapped(t *testing.T) {
	s := newStore(t)
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxUndo+50; i++ {
		if err := s.AddCard(0, 0, Card{Content: strings.Repeat("x", 1) + " " + strings.Repeat("card", 1) + string(rune('a'+i%26)) + itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.undo) > maxUndo {
		t.Fatalf("undo stack grew to %d", len(s.undo))
	}
	for i := 0; i < maxUndo+50; i++ {
		s.Undo()
	}
	if len(s.redo) > maxUndo {
		t.Fatalf("redo stack grew to %d", len(s.redo))
	}
	for i := 0; i < maxUndo+50; i++ {
		s.Redo()
	}
	if len(s.undo) > maxUndo || len(s.redo) > maxUndo {
		t.Fatalf("undo=%d redo=%d", len(s.undo), len(s.redo))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

func TestLoadRejectsHugeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(int64(maxDataSize) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := Load(path, path+"_bkp"); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want too-large error, got %v", err)
	}
}

// TestRoundtripFuzz pins the file format: anything the store can save must
// reload unchanged. Fixed seed keeps failures reproducible.
func TestRoundtripFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	trimmed := func(minimum, maximum int) string {
		out := ""
		for out == "" { // an all-space token would make an empty board name
			n := minimum + rng.Intn(maximum-minimum+1)
			var b []rune
			for i := 0; i < n; i++ {
				b = append(b, rtCharset[rng.Intn(len(rtCharset))])
			}
			out = strings.Trim(string(b), " ")
		}
		return out
	}
	date := func() string {
		base := []string{"2025-03-01", "2026-10-24", "2027-01-02", "1999-12-31"}
		return base[rng.Intn(len(base))]
	}

	for iter := 0; iter < 500; iter++ {
		boards := make([]Board, 1+rng.Intn(4))
		for bi := range boards {
			boards[bi].Name = trimmed(1, 12)
			boards[bi].Columns = make([]Column, rng.Intn(5))
			for ci := range boards[bi].Columns {
				col := &boards[bi].Columns[ci]
				col.Title = trimmed(1, 12)
				switch rng.Intn(3) {
				case 0:
					col.WIP = 2
				case 1:
					col.Gate = true
				case 2:
					col.Archive = true
				}
				col.Cards = make([]Card, rng.Intn(6))
				for ki := range col.Cards {
					card := &col.Cards[ki]
					card.Content = trimmed(1, 20)
					// description: ~half the cards have one
					if rng.Intn(2) == 0 {
						var lines []string
						for d := 0; d < rng.Intn(3); d++ {
							line := ""
							if n := rng.Intn(3); n > 0 {
								line = strings.Repeat(" ", rng.Intn(3)) + trimmed(1, 15)
							}
							lines = append(lines, line)
						}
						card.Description = strings.Join(lines, "\n")
					}
					if rng.Intn(2) == 0 {
						card.Priority = 1 + rng.Intn(2)
					}
					if rng.Intn(3) == 0 {
						card.Due = date()
					}
					if rng.Intn(2) == 0 {
						card.Moved = date()
					}
					for t := 0; t < rng.Intn(3); t++ {
						card.Tags = appendTag(card.Tags, tag(rng))
					}
					for c := 0; c < rng.Intn(4); c++ {
						card.Checklist = append(card.Checklist, ChecklistItem{
							Content: trimmed(1, 15),
							Done:    rng.Intn(2) == 0,
						})
					}
					for st := 0; st < rng.Intn(3); st++ {
						sub := Subtask{Content: trimmed(1, 15), Done: rng.Intn(2) == 0}
						if rng.Intn(2) == 0 {
							sub.Description = trimmed(1, 15)
						}
						card.Subtasks = append(card.Subtasks, sub)
					}
				}
			}
		}

		text := encode(boards)
		got, err := parse(text)
		if err != nil {
			t.Fatalf("iter %d: reload failed: %v\n%s", iter, err, text)
		}
		if again := encode(got); again != text {
			t.Fatalf("iter %d: roundtrip diverged\n--- saved ---\n%s\n--- loaded ---\n%s", iter, text, again)
		}
	}
}

var rtCharset = []rune("abcXYZ 0123.-#>@/é")

func tag(rng *rand.Rand) string {
	n := 1 + rng.Intn(8)
	var b []rune
	for i := 0; i < n; i++ {
		b = append(b, []rune("abcdefghijklmnopqrstuvwxyz0123456789")[rng.Intn(36)])
	}
	return string(b)
}
