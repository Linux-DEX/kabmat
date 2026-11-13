package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Linux-DEX/kabmat/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kabmat-config")
	if err != nil {
		panic(err)
	}
	os.Setenv("KABMAT_CONFIG", filepath.Join(dir, "config"))
	os.Exit(m.Run())
}

// A failing Flush must not trap the user: ctrl+c still quits.
func TestQuitSurvivesWriteError(t *testing.T) {
	dir := t.TempDir()
	defer os.Chmod(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, "data"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	// The directory is now unwritable, so every Flush attempt fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	if err := s.CreateBoard("X"); err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(*model)
	if !m2.quitting {
		t.Fatalf("write error blocked quitting (notice %q)", m2.notice)
	}
	if cmd == nil {
		t.Fatal("no quit command returned")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("expected quit command, got %v", msg)
	}
}

func TestEscHidesChecklist(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "e", "c", "c", "i")
	typeRunes(m, "box")
	press(m, "enter", "esc")
	if m.top() != layerCard {
		t.Fatalf("esc should return to the card, layer %d", m.top())
	}
	if len(s.Boards[0].Columns[0].Cards[0].Checklist) != 0 {
		t.Fatal("esc should leave the card unsaved")
	}
	press(m, "esc")
	if m.top() != layerNone {
		t.Fatalf("esc should close the card, layer %d", m.top())
	}
	press(m, "m")
	view := m.View()
	if !strings.Contains(view, "Move to") || !strings.Contains(view, "Work") || !strings.Contains(view, "ship") {
		t.Fatal(view)
	}
}

func TestBoardFlow(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	press(m, "c", "i")
	typeRunes(m, "Work")
	press(m, "enter", "enter")
	if m.screen != screenBoard || s.Boards[0].Name != "Work" {
		t.Fatalf("open: screen=%d boards=%+v", m.screen, s.Boards)
	}
	press(m, "C", "i")
	typeRunes(m, "Todo")
	press(m, "enter")
	press(m, "c", "i")
	typeRunes(m, "ship")
	press(m, "enter")
	if s.Boards[0].Columns[0].Title != "Todo" || s.Boards[0].Columns[0].Cards[0].Content != "ship" {
		t.Fatalf("board: %+v", s.Boards[0])
	}
	press(m, "e", "c", "c", "i")
	typeRunes(m, "box")
	press(m, "enter", "space", "q")
	if m.top() != layerNone || m.screen != screenBoard {
		t.Fatalf("checklist q should return to the board, screen=%d layer=%d", m.screen, m.top())
	}
	item := s.Boards[0].Columns[0].Cards[0].Checklist
	if len(item) != 1 || item[0].Content != "box" || !item[0].Done {
		t.Fatalf("checklist: %+v", item)
	}
	press(m, "e", "s", "c", "i")
	typeRunes(m, "child")
	press(m, "enter", "q")
	if m.top() != layerNone || m.screen != screenBoard {
		t.Fatalf("expected the board, screen=%d layer=%d", m.screen, m.top())
	}
	subs := s.Boards[0].Columns[0].Cards[0].Subtasks
	if len(subs) != 1 || subs[0].Content != "child" || subs[0].Done {
		t.Fatalf("subtasks: %+v", subs)
	}
	if !strings.Contains(m.View(), "▸ child") && !strings.Contains(m.View(), "child") {
		t.Fatal(m.View())
	}
	press(m, "q")
	if m.screen != screenMenu {
		t.Fatal("expected menu")
	}
	view := m.View()
	if !strings.Contains(view, "Work") || !strings.Contains(view, "1 col") || !strings.Contains(view, "1 card") {
		t.Fatal(view)
	}
}

func TestReorderBoardAndCard(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Alpha"); err != nil || s.CreateBoard("Beta") != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 40
	press(m, "j", "K")
	if s.Boards[0].Name != "Beta" || m.menuSel != 0 {
		t.Fatalf("board order: %+v sel=%d", s.Boards, m.menuSel)
	}
	press(m, "enter", "C", "i")
	typeRunes(m, "Todo")
	press(m, "enter", "c", "i")
	typeRunes(m, "one")
	press(m, "enter", "c", "i")
	typeRunes(m, "two")
	press(m, "enter", "k", "J")
	cards := s.Boards[0].Columns[0].Cards
	if cards[0].Content != "two" || cards[1].Content != "one" {
		t.Fatalf("card order: %+v", cards)
	}
	view := m.View()
	one := strings.Index(view, "one")
	two := strings.Index(view, "two")
	if two < 0 || one < 0 || two > one {
		t.Fatalf("card move not visible:\n%s", view)
	}
}

func TestSubtaskPanelFillsScreen(t *testing.T) {
	m := &model{
		width:  100,
		height: 40,
		edit: cardEdit{
			content:  newField("Parent", false),
			subtasks: []store.Subtask{{Content: "child", Description: "details"}},
		},
	}
	view := m.subListDialog()
	if lipgloss.Width(view) < 90 || lipgloss.Height(view) < 36 {
		t.Fatalf("subtask list %dx%d\n%s", lipgloss.Width(view), lipgloss.Height(view), view)
	}
	m.sub = subEdit{index: 0, content: newField("child", false), desc: newField("details", true)}
	edit := m.subDialog()
	if lipgloss.Width(edit) < 90 || lipgloss.Height(edit) < 36 {
		t.Fatalf("subtask editor %dx%d\n%s", lipgloss.Width(edit), lipgloss.Height(edit), edit)
	}
}

func TestCardInfoBorder(t *testing.T) {
	m := &model{
		width:  80,
		height: 24,
		edit: cardEdit{
			content: newField("ship the board", false),
			desc:    newField("line one\nline two that is quite long", true),
		},
	}
	view := m.cardDialog()
	for _, corner := range []string{"╭", "╮", "╰", "╯", "│", "─"} {
		if !strings.Contains(view, corner) {
			t.Fatalf("missing %s\n%s", corner, view)
		}
	}
	lines := strings.Split(view, "\n")
	width := lipgloss.Width(lines[0])
	for _, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("ragged border %d vs %d\n%s", got, width, view)
		}
	}
	if !strings.Contains(view, "Card Info") || !strings.Contains(view, "Content") || !strings.Contains(view, "Description") {
		t.Fatal(view)
	}
	if lipgloss.Width(view) < 70 || lipgloss.Height(view) < 20 {
		t.Fatalf("card info %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestYankFilterFold(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "alpha", Checklist: []store.ChecklistItem{{Content: "needle"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "beta", Subtasks: []store.Subtask{{Content: "hidden-sub"}}}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 40
	view := m.View()
	if !strings.Contains(view, "Todo 2") || strings.Contains(view, "hidden-sub") || !strings.Contains(view, "1 subtasks") {
		t.Fatalf("fold/count:\n%s", view)
	}
	press(m, "z")
	if !strings.Contains(m.View(), "hidden-sub") {
		t.Fatal(m.View())
	}
	press(m, "z", "/")
	typeRunes(m, "needle")
	press(m, "enter")
	if got := m.cardIndex(0); got != 0 {
		t.Fatalf("search card %d", got)
	}
	view = m.View()
	if strings.Contains(view, "beta") || !strings.Contains(view, "1/2") {
		t.Fatalf("filter:\n%s", view)
	}
	press(m, "f", "y", "l", "p")
	done := s.Boards[0].Columns[1].Cards
	if len(done) != 1 || done[0].Content != "alpha" || len(done[0].Checklist) != 1 || done[0].Checklist[0].Content != "needle" {
		t.Fatalf("paste: %+v", done)
	}
	press(m, "h", "Y")
	todo := s.Boards[0].Columns[0].Cards
	if len(todo) != 3 || todo[1].Content != "alpha" || m.notice != "duplicated" {
		t.Fatalf("dup notice=%q cards=%+v", m.notice, todo)
	}
}

func TestSearchFiltersEveryColumn(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "alpha"}); err != nil || s.AddCard(0, 0, store.Card{Content: "beta"}) != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 1, store.Card{Content: "alpha done"}); err != nil || s.AddCard(0, 1, store.Card{Content: "gamma"}) != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 40
	press(m, "/")
	typeRunes(m, "alpha")
	view := m.View()
	if strings.Contains(view, "beta") || strings.Contains(view, "gamma") || !strings.Contains(view, "alpha") || !strings.Contains(view, "alpha done") {
		t.Fatalf("live search:\n%s", view)
	}
	press(m, "esc")
	view = m.View()
	if !strings.Contains(view, "beta") || !strings.Contains(view, "gamma") {
		t.Fatalf("esc should restore the board:\n%s", view)
	}
	press(m, "/")
	typeRunes(m, "alpha")
	press(m, "enter")
	view = m.View()
	if strings.Contains(view, "beta") || strings.Contains(view, "gamma") || !strings.Contains(view, "Todo 1/2") || !strings.Contains(view, "Done 1/2") {
		t.Fatalf("confirmed:\n%s", view)
	}
	press(m, "f")
	view = m.View()
	if !strings.Contains(view, "beta") || !strings.Contains(view, "gamma") || !strings.Contains(view, "Todo 2") || !strings.Contains(view, "Done 2") {
		t.Fatalf("filter off:\n%s", view)
	}
	press(m, "f", "esc")
	view = m.View()
	if !strings.Contains(view, "beta") || !strings.Contains(view, "gamma") || !strings.Contains(view, "Todo 2") || !strings.Contains(view, "Done 2") {
		t.Fatalf("esc clears the filter:\n%s", view)
	}
}

func TestMoveClearAndResume(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.CreateBoard("Home") != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{
		Content:     "ship",
		Description: "first line\nsecond line",
		Checklist:   []store.ChecklistItem{{Content: "box", Done: true}, {Content: "keep"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "other", Description: "secret note"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 40
	view := m.View()
	if !strings.Contains(view, "first line") || strings.Contains(view, "second line") || strings.Contains(view, "secret note") {
		t.Fatalf("description:\n%s", view)
	}
	if !strings.Contains(strings.Join(helpLines, "\n"), "clear finished checklist") {
		t.Fatal("help keymap missing X")
	}
	press(m, "]")
	if m.curBoard().Name != "Home" {
		t.Fatalf("board %s", m.curBoard().Name)
	}
	press(m, "[", "m", "j", "enter", "X")
	done := s.Boards[0].Columns[1].Cards
	if len(s.Boards[0].Columns[0].Cards) != 1 || len(done) != 1 || done[0].Content != "ship" {
		t.Fatalf("move: %+v", s.Boards[0].Columns)
	}
	if len(done[0].Checklist) != 1 || done[0].Checklist[0].Content != "keep" || m.col != 1 {
		t.Fatalf("clear col=%d list=%+v", m.col, done[0].Checklist)
	}
	m.remember()
	resumed, err := newModel(s, "")
	if err != nil || resumed.screen != screenBoard || resumed.curBoard().Name != "Work" || resumed.col != 1 {
		t.Fatalf("resume screen=%d col=%d err=%v", resumed.screen, resumed.col, err)
	}
}

// A store mutation must not write the file on the next unrelated keypress;
// the write is debounced and lands when the flush tick arrives.
func TestFlushDebounced(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	s.Flush()
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	if err := s.AddCard(0, 0, store.Card{Content: "debounced"}); err != nil {
		t.Fatal(err)
	}
	// An unrelated navigation keypress must not flush.
	press(m, "k")
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "debounced") {
		t.Fatalf("unrelated keypress flushed the file: %q", b)
	}
	// The scheduled tick flushes.
	m2, _ := m.Update(flushTickMsg{})
	if b, err := os.ReadFile(s.Path); err != nil || !strings.Contains(string(b), "debounced") {
		t.Fatalf("tick did not flush: %q %v", b, err)
	}
	_ = m2
	// A second tick with a clean store must not re-schedule work (cmd nil).
	if _, cmd := m.Update(flushTickMsg{}); cmd != nil {
		t.Fatalf("clean store scheduled another flush: %v", cmd)
	}
}

// Quitting must still write pending changes synchronously.
func TestQuitFlushesPendingChanges(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	s.Flush()
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	if err := s.AddCard(0, 0, store.Card{Content: "bye"}); err != nil {
		t.Fatal(err)
	}
	press(m, "q")
	b, err := os.ReadFile(s.Path)
	if err != nil || !strings.Contains(string(b), "bye") {
		t.Fatalf("quit did not flush: %q %v", b, err)
	}
}

// View must not change selection state: rendering twice in a row must give
// the same frame and leave cardSel/menuSel/col untouched.
func TestViewDoesNotMutateSelection(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Doing") != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"alpha", "beta", "gamma"} {
		if err := s.AddCard(0, 0, store.Card{Content: c}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCard(0, 1, store.Card{Content: "match"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "k") // a first Update normalizes cardSel for both columns
	// Filtering anchors the selection to the first match per column; a stale
	// selection exercises the clamping path too.
	m.filtering = true
	m.search = "match"
	m.cardSel[0] = 999
	press(m, "z") // Update handles the state change; View must not
	wantSel := append([]int(nil), m.cardSel...)
	wantCol, wantMenu := m.col, m.menuSel
	before := m.View()
	m.View() // a second render must be identical and change nothing
	after := m.View()
	if before != after {
		t.Fatalf("View changed its own output")
	}
	if !sameInts(m.cardSel, wantSel) || m.col != wantCol || m.menuSel != wantMenu {
		t.Fatalf("View mutated state: sel %v want %v col %d menu %d", m.cardSel, wantSel, m.col, wantMenu)
	}
}

// Update alone (before any View call) must anchor the menu scroll window.
func TestMenuScrollAnchoredInUpdate(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := s.CreateBoard(fmt.Sprintf("Board%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24 // menu window holds ~18 rows
	press(m, "G")
	if m.menuSel != 29 {
		t.Fatalf("menuSel %d", m.menuSel)
	}
	if m.menuScroll == 0 {
		t.Fatalf("menuScroll was not anchored by Update: %d", m.menuScroll)
	}
}

// Update alone must anchor the filtered selection to a match per column.
func TestFilterAnchoredInUpdate(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Doing") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "plain"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 1, store.Card{Content: "target"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m.filtering = true
	m.search = "target"
	press(m, "z") // harmless key; no View call yet
	if !sameInts(m.cardSel, []int{0, 0}) {
		t.Fatalf("filter anchor did not run in Update: %v", m.cardSel)
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSignals(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil || s.AddColumn(0, "Done") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{
		Content:     "ship",
		Description: "see https://example.com/board.",
		Checklist:   []store.ChecklistItem{{Content: "box"}},
		Moved:       "2020-01-01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "other"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 40
	press(m, "!", "!")
	if s.Boards[0].Columns[0].Cards[0].Priority != 2 {
		t.Fatalf("priority %d", s.Boards[0].Columns[0].Cards[0].Priority)
	}
	view := m.View()
	age := fmt.Sprintf("%dd", daysSince("2020-01-01"))
	if !strings.Contains(view, "!!") || !strings.Contains(view, "Todo 2") || !strings.Contains(view, "2 cards") || !strings.Contains(view, age) {
		t.Fatalf("header/priority/age %s:\n%s", age, view)
	}
	press(m, "t")
	typeRunes(m, "work")
	press(m, "enter")
	if tags := s.Boards[0].Columns[0].Cards[0].Tags; len(tags) != 1 || tags[0] != "work" || !strings.Contains(m.View(), "#work") {
		t.Fatalf("tags %+v\n%s", tags, m.View())
	}
	press(m, "T")
	if m.top() != layerCalendar {
		t.Fatalf("due layer %d", m.top())
	}
	m.calYear, m.calMonth, m.calSel = 2020, 0, "2020-01-02"
	press(m, "enter")
	if s.Boards[0].Columns[0].Cards[0].Due != "2020-01-02" || !strings.Contains(m.View(), "01-02") {
		t.Fatalf("due %q\n%s", s.Boards[0].Columns[0].Cards[0].Due, m.View())
	}
	press(m, "w", "i")
	typeRunes(m, "1")
	press(m, "enter")
	if !strings.Contains(m.View(), "2/1!") {
		t.Fatalf("wip:\n%s", m.View())
	}
	press(m, "W")
	if !s.Boards[0].Columns[0].Gate || !strings.Contains(m.View(), "Todo*") {
		t.Fatalf("gate:\n%s", m.View())
	}
	if got := firstURL("see https://example.com/board."); got != "https://example.com/board" {
		t.Fatal(got)
	}
	press(m, "space", "j", "space", "d", "y")
	if len(s.Boards[0].Columns[0].Cards) != 0 {
		t.Fatalf("bulk delete: %+v", s.Boards[0].Columns[0].Cards)
	}
	if _, err := s.ToggleGate(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "keep", Checklist: []store.ChecklistItem{{Content: "open"}}}); err != nil {
		t.Fatal(err)
	}
	m.picked = nil
	press(m, "A", "v")
	view = m.View()
	if strings.Contains(view, "keep") || !strings.Contains(view, "1 archived") {
		t.Fatalf("hide archive:\n%s", view)
	}
	press(m, "v")
	if !strings.Contains(m.View(), "keep") || !strings.Contains(m.View(), "Archive") {
		t.Fatal(m.View())
	}
	due, err := parseDue("+0")
	if err != nil || due == "" {
		t.Fatal(err, due)
	}
}

// fit must truncate by terminal cell width, not rune count: a string of
// wide glyphs must fit inside n columns.
func TestFitTruncatesByCellWidth(t *testing.T) {
	// "\u6f22" (漢) is 2 cells wide in every CJK-capable terminal.
	wide := strings.Repeat("\u6f22", 5) // 10 cells
	got := fit(wide, 5)
	if w := lipgloss.Width(got); w > 5 {
		t.Fatalf("fit(%q, 5) is %d cells wide: %q", wide, w, got)
	}
	if got := lipgloss.Width(fit(wide, 5)); got != 5 {
		t.Fatalf("want 5 cells, got %d: %q", got, fit(wide, 5))
	}
	if got := fit("hello", 5); got != "hello" {
		t.Fatalf("ascii %q", got)
	}
	if got := fit("hello!", 5); got != "hell…" {
		t.Fatalf("ascii ellipsis %q", got)
	}
	if w := lipgloss.Width(fit("ab", 1)); w != 1 {
		t.Fatalf("n=1 gives %d cells: %q", w, fit("ab", 1))
	}
}

func TestWordErase(t *testing.T) {
	f := newField("hello world", false)
	f.insert = true
	f.col = len([]rune(f.value()))
	f.handle("ctrl+w", nil)
	if f.value() != "hello " || f.col != len([]rune("hello ")) {
		t.Fatalf("ctrl+w %q col %d", f.value(), f.col)
	}
	f.handle("ctrl+h", nil)
	if f.value() != "" {
		t.Fatalf("ctrl+h %q", f.value())
	}
	f = newField("hello world", false)
	f.col = len([]rune(f.value()))
	f.handle("alt+backspace", nil)
	if f.value() != "hello " {
		t.Fatalf("alt %q", f.value())
	}

	if !ctrlBackspaceCSI(csiMsg("?CSI[49 50 55 59 53 117]?")) || !ctrlBackspaceCSI(csiMsg("?CSI[49 50 55 59 53 59 49 117]?")) || !ctrlBackspaceCSI(csiMsg("?CSI[50 55 59 53 59 49 50 55 126]?")) {
		t.Fatal("csi")
	}
	if ctrlBackspaceCSI(csiMsg("not a key")) {
		t.Fatal("plain")
	}

	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.edit = cardEdit{content: newField("hello world", false), desc: newField("", true)}
	m.edit.content.insert = true
	m.edit.content.col = len([]rune(m.edit.content.value()))
	m.push(layerCard)
	m.Update(csiMsg("?CSI[49 50 55 59 53 117]?"))
	if m.edit.content.value() != "hello " {
		t.Fatalf("csi erase %q", m.edit.content.value())
	}
}

func TestDescriptionWraps(t *testing.T) {
	text := strings.TrimSpace(strings.Repeat("alpha ", 40))
	m := &model{
		width:  80,
		height: 30,
		edit: cardEdit{
			content: newField("title", false),
			desc:    newField(text, true),
			focus:   1,
		},
	}
	view := m.cardDialog()
	if strings.Count(view, "alpha") != 40 {
		t.Fatalf("wrapped count %d\n%s", strings.Count(view, "alpha"), view)
	}
	m.edit.desc = newField(strings.Repeat("x", 200), true)
	view = m.cardDialog()
	if strings.Count(view, "x") != 200 {
		t.Fatalf("hard wrap %d\n%s", strings.Count(view, "x"), view)
	}
}

func TestColumnDialog(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 40
	m.openInput(inputNewColumn, " New Column Name ", "")
	col := m.dialog()
	if strings.Count(col, "╭") != 1 || !strings.Contains(col, "New column") || !strings.Contains(col, "NORMAL") {
		t.Fatalf("column dialog\n%s", col)
	}
	if h := lipgloss.Height(col); h < 5 || h > 9 {
		t.Fatalf("height %d\n%s", h, col)
	}
	press(m, "i")
	if !strings.Contains(m.dialog(), "INSERT") {
		t.Fatalf("insert\n%s", m.dialog())
	}
	press(m, "esc")
	if !strings.Contains(m.dialog(), "NORMAL") || strings.Contains(m.dialog(), "INSERT") {
		t.Fatalf("normal\n%s", m.dialog())
	}
	m.pop()
	m.openInput(inputRenameColumn, " Rename Column ", "Doing")
	ren := m.dialog()
	if !strings.Contains(ren, "Rename column") || !strings.Contains(ren, "Doing") || !strings.Contains(ren, "NORMAL") {
		t.Fatalf("rename\n%s", ren)
	}
}

func TestCardEditModeBadge(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship", Description: "notes"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "e")
	if m.top() != layerCard || !strings.Contains(m.dialog(), "NORMAL") {
		t.Fatalf("card normal\n%s", m.dialog())
	}
	press(m, "i")
	if !strings.Contains(m.dialog(), "INSERT") {
		t.Fatalf("card insert\n%s", m.dialog())
	}
	press(m, "esc", "tab", "i")
	if !m.edit.desc.insert || !strings.Contains(m.dialog(), "INSERT") {
		t.Fatalf("desc insert focus=%d\n%s", m.edit.focus, m.dialog())
	}
}

func TestTagPrompt(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship", Tags: []string{"legacy"}}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 30
	press(m, "t")
	if m.top() != layerInput || strings.Contains(m.dialog(), "#bug") || strings.Contains(m.dialog(), "#feature") {
		t.Fatalf("preset list\n%s", m.dialog())
	}
	if !strings.Contains(m.dialog(), "legacy") {
		t.Fatalf("current tags\n%s", m.dialog())
	}
	typeRunes(m, " bug, home")
	press(m, "enter")
	tags := s.Boards[0].Columns[0].Cards[0].Tags
	if len(tags) != 3 || tags[0] != "legacy" || tags[1] != "bug" || tags[2] != "home" || !strings.Contains(m.View(), "#home") {
		t.Fatalf("saved %+v\n%s", tags, m.View())
	}
	press(m, "t")
	for range "legacy bug home" {
		press(m, "backspace")
	}
	press(m, "enter")
	if len(s.Boards[0].Columns[0].Cards[0].Tags) != 0 {
		t.Fatalf("clear %+v", s.Boards[0].Columns[0].Cards[0].Tags)
	}

	press(m, "t")
	typeRunes(m, "../etc")
	press(m, "enter")
	if m.top() != layerInput || !strings.Contains(m.dialog(), "bad tag") {
		t.Fatalf("bad tag should stay open\n%s", m.dialog())
	}
	if len(s.Boards[0].Columns[0].Cards[0].Tags) != 0 {
		t.Fatalf("bad tag wiped %+v", s.Boards[0].Columns[0].Cards[0].Tags)
	}
	press(m, "esc", "esc")
	if m.top() != layerNone {
		t.Fatalf("esc should close the tag prompt, layer %d", m.top())
	}

	press(m, "t")
	typeRunes(m, "#Work work, HOME")
	press(m, "enter")
	tags = s.Boards[0].Columns[0].Cards[0].Tags
	if len(tags) != 2 || tags[0] != "work" || tags[1] != "home" {
		t.Fatalf("normalized %+v", tags)
	}
	press(m, "u")
	if len(s.Boards[0].Columns[0].Cards[0].Tags) != 0 {
		t.Fatalf("undo %+v", s.Boards[0].Columns[0].Cards[0].Tags)
	}

	press(m, "t")
	typeRunes(m, "keep")
	press(m, "enter", "e", "t")
	typeRunes(m, " extra")
	press(m, "enter")
	if got := m.edit.tags; len(got) != 2 || got[0] != "keep" || got[1] != "extra" {
		t.Fatalf("card edit %+v", got)
	}
	press(m, "esc")
	if tags := s.Boards[0].Columns[0].Cards[0].Tags; len(tags) != 1 || tags[0] != "keep" {
		t.Fatalf("discard card edit %+v", tags)
	}

	press(m, "c", "i")
	typeRunes(m, "other")
	press(m, "enter", "space", "k", "space", "t")
	typeRunes(m, "bulk")
	press(m, "enter")
	for _, card := range s.Boards[0].Columns[0].Cards {
		if len(card.Tags) != 1 || card.Tags[0] != "bulk" {
			t.Fatalf("bulk %q %+v", card.Content, card.Tags)
		}
	}
}

func TestUserTheme(t *testing.T) {
	defer applyPalette(defaultPalette())
	t.Setenv("KABMAT_CONFIG", filepath.Join(t.TempDir(), "config"))
	got := parseThemes("# note\n\ndusk\naccent 111\npaper 236\n")
	if len(got) != 1 || got[0].name != "dusk" || got[0].pal.accent != "111" || got[0].pal.paper != "236" {
		t.Fatalf("%+v", got)
	}
	if !validColor("#abc") || !validColor("#aabbcc") || validColor("#gg0000") || !validColor("0") || validColor("256") || validColor("") {
		t.Fatal("color")
	}
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 30
	if builtinSchemes()[0].name != "linux-dex" || builtinSchemes()[0].pal.accent != "#4C7CFF" || builtinSchemes()[1].pal.accent != "#89B4FA" {
		t.Fatalf("builtins %+v", builtinSchemes())
	}
	press(m, "ctrl+t", "j", "enter")
	if m.themeName != "catppuccin-mocha" || m.top() != layerNone {
		t.Fatalf("theme %s layer %d", m.themeName, m.top())
	}
	b, err := os.ReadFile(m.cfg.Path)
	if err != nil || !strings.Contains(string(b), "theme catppuccin-mocha") {
		t.Fatalf("saved %q %v", b, err)
	}
	press(m, "ctrl+t", "c", "i")
	typeRunes(m, "dusk")
	press(m, "enter", "e", "enter", "S")
	typeRunes(m, "111")
	press(m, "enter")
	if m.top() != layerTheme || m.schemes[m.schemeSel].name != "dusk" || m.schemes[m.schemeSel].pal.accent != "111" {
		t.Fatalf("edit %+v layer %d", m.schemes, m.top())
	}
	raw, err := os.ReadFile(m.cfg.Path)
	if err != nil || !strings.Contains(string(raw), "[theme.dusk]") || !strings.Contains(string(raw), "accent 111") {
		t.Fatalf("themes %q %v", raw, err)
	}
	again, err := newModel(s, "")
	if err != nil || again.themeName != "dusk" {
		t.Fatalf("reload %s %v", again.themeName, err)
	}
}

// An invalid theme name or color must leave its error visible in the theme
// dialog instead of being wiped when the input closes.
func TestThemeInputErrorsStayVisible(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 30

	// Bad theme name: the notice must survive the input closing.
	press(m, "ctrl+t", "c", "i")
	typeRunes(m, "bad name")
	press(m, "enter")
	if m.top() != layerTheme {
		t.Fatalf("layer %d", m.top())
	}
	if m.notice == "" {
		t.Fatalf("bad theme name wiped the notice, schemes %+v", m.schemes)
	}
	if !strings.Contains(m.notice, "theme name") {
		t.Fatalf("notice %q", m.notice)
	}

	// Bad color value on a user theme: the notice must survive too.
	press(m, "c", "i")
	typeRunes(m, "mine")
	press(m, "enter", "e", "enter", "S")
	typeRunes(m, "zzz")
	press(m, "enter")
	if m.top() != layerTheme || m.schemeEdit != true {
		t.Fatalf("layer %d edit %v", m.top(), m.schemeEdit)
	}
	if !strings.Contains(m.notice, "color") {
		t.Fatalf("bad color wiped the notice: %q", m.notice)
	}
}

// A broken config file must not stop the app: it falls back to defaults and
// shows the parse error as a notice.
func TestBrokenConfigStillStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("KABMAT_CONFIG", path)
	if err := os.WriteFile(path, []byte("theme"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	s, err := store.Load(filepath.Join(data, "data"), filepath.Join(data, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatalf("broken config blocked startup: %v", err)
	}
	if m.themeName != "linux-dex" {
		t.Fatalf("theme %s", m.themeName)
	}
	if !strings.Contains(m.notice, "config") {
		t.Fatalf("notice %q", m.notice)
	}
	m.width, m.height = 80, 24
	if view := m.View(); !strings.Contains(view, "Work") {
		t.Fatal(view)
	}
}

func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	t.Setenv("KABMAT_CONFIG", path)

	data := t.TempDir()
	s, err := store.Load(filepath.Join(data, "data"), filepath.Join(data, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"one", "two", "three"} {
		if err := s.AddCard(0, 0, store.Card{Content: title}); err != nil {
			t.Fatal(err)
		}
	}
	bare, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if bare.cfg.Exists || bare.themeName != "linux-dex" {
		t.Fatalf("missing file theme %s exists %v", bare.themeName, bare.cfg.Exists)
	}
	bare.width, bare.height = 100, 30
	press(bare, "c")
	if bare.top() != layerCard {
		t.Fatal("default c should open a card")
	}

	body := "" +
		"# comment\n\n" +
		"theme dusk\n\n" +
		"[keys.menu]\nquit x\n\n" +
		"[keys.board]\ntags F\n\n" +
		"[theme.dusk]\naccent #112233\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.themeName != "dusk" {
		t.Fatalf("theme %s", m.themeName)
	}
	var accent string
	for _, scheme := range m.schemes {
		if scheme.name == "dusk" {
			accent = scheme.pal.get("accent")
		}
	}
	if accent != "#112233" {
		t.Fatalf("accent %s", accent)
	}
	press(m, "q")
	if m.quitting {
		t.Fatal("q should not quit after the menu remap")
	}
	press(m, "x")
	if !m.quitting {
		t.Fatal("x should quit")
	}

	board, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	board.width, board.height = 100, 30
	press(board, "t")
	if board.top() == layerInput {
		t.Fatal("t should not open tags")
	}
	press(board, "F")
	if board.top() != layerInput || !strings.Contains(board.dialog(), "Tags") {
		t.Fatalf("tags\n%s", board.dialog())
	}
	press(board, "esc", "esc", "2", "j")
	if board.cardIndex(0) != 2 {
		t.Fatalf("count move landed on %d", board.cardIndex(0))
	}

	os.WriteFile(path, []byte("[keys.board]\nnope z\n"), 0o600)
	if m, err := newModel(s, "Work"); err != nil {
		t.Fatalf("bad config should not block startup: %v", err)
	} else if m.themeName != "linux-dex" || !strings.Contains(m.notice, "unknown action") {
		t.Fatalf("bad config fallback: theme %s notice %q", m.themeName, m.notice)
	}
}

func TestConfigKeyAndTag(t *testing.T) {
	path := mConfigPath(t)
	body := "theme linux-dex\n\n[keys.board]\nnew-card x\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "c")
	if m.top() == layerCard {
		t.Fatal("c should no longer open a card")
	}
	press(m, "x")
	if m.top() != layerCard {
		t.Fatalf("x should open a card, layer %d", m.top())
	}
	m.pop()
	press(m, "t")
	if m.top() != layerInput || strings.Contains(m.dialog(), "#bug") {
		t.Fatalf("tag prompt\n%s", m.dialog())
	}
}

// mConfigPath hands the test a fresh, isolated config file and points
// KABMAT_CONFIG at it, so test suites that rerun (go test -count=2) never see
// another test's saved bindings or tags.
func mConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("KABMAT_CONFIG", path)
	return path
}

type csiMsg string

func (c csiMsg) String() string { return string(c) }

func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(keyMsg(k))
	}
}

func typeRunes(m *model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+home":
		return tea.KeyMsg{Type: tea.KeyCtrlHome}
	case "ctrl+end":
		return tea.KeyMsg{Type: tea.KeyCtrlEnd}
	case "ctrl+right":
		return tea.KeyMsg{Type: tea.KeyCtrlRight}
	case "ctrl+left":
		return tea.KeyMsg{Type: tea.KeyCtrlLeft}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestSelectAllSendThenUndo(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddColumn(0, "Todo"); err != nil || s.AddColumn(0, "Doing") != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"a", "b", "c"} {
		if err := s.AddCard(0, 0, store.Card{Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCard(0, 1, store.Card{Content: "d"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24

	// pick all three Todo cards by selection moves: a at 0, then J>space, J>space
	press(m, "space", "J", "space", "J", "space")
	if len(m.picked) != 3 {
		t.Fatalf("picked %d cards", len(m.picked))
	}
	if err := m.store.AddCard(0, 0, store.Card{Content: "x\ny"}); err == nil {
		t.Fatal("newline content must stay refused through the model")
	}
	press(m, "L")
	// picks are index-based and processed bottom-up: contents moved with the
	// J-swaps, so the batch relocates a (idx2), c (idx1), b (idx0)
	doing := s.Boards[0].Columns[1].Cards
	for i, want := range []string{"b", "c", "a", "d"} {
		if doing[i].Content != want {
			t.Fatalf("Doing[%d]=%q want %q: %+v", i, doing[i].Content, want, doing)
		}
	}
	// one undo press reverts the whole batch (back to after the J-swaps)
	press(m, "u")
	todo := s.Boards[0].Columns[0].Cards
	var names []string
	for _, c := range todo {
		names = append(names, c.Content)
	}
	if strings.Join(names, "") != "bca" || len(s.Boards[0].Columns[1].Cards) != 1 {
		t.Fatalf("undo state: todo=%q doing=%+v", names, s.Boards[0].Columns[1].Cards)
	}
}

func TestRenameColumnRejectsBraces(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "E")
	if m.top() != layerInput {
		t.Fatalf("expected input layer, got %d", m.top())
	}
	press(m, "S")
	typeRunes(m, "Bad {x}")
	press(m, "enter")
	if m.top() != layerNone {
		t.Fatalf("dialog should close, layer=%d", m.top())
	}
	if !strings.Contains(m.notice, "braces") {
		t.Fatalf("notice: %q", m.notice)
	}
	if s.Boards[0].Columns[0].Title != "Todo" {
		t.Fatalf("title changed: %q", s.Boards[0].Columns[0].Title)
	}
}

func TestFieldVimArrowsAndWordMotions(t *testing.T) {
	f := newField("hello world", false)
	f.insert = true
	f.handle("right", nil)
	if f.col != 1 {
		t.Fatalf("insert right should move one character, col=%d", f.col)
	}
	f.handle("ctrl+right", nil)
	if f.col != 6 {
		t.Fatalf("insert ctrl+right should jump to the next word, col=%d", f.col)
	}
	f.handle("esc", nil)
	if f.insert {
		t.Fatal("esc should leave insert mode")
	}
	f.handle("b", nil)
	if f.col != 0 {
		t.Fatalf("normal b should jump to the previous word, col=%d", f.col)
	}
	f.handle("e", nil)
	if f.col != 5 {
		t.Fatalf("normal e should jump to the end of the word, col=%d", f.col)
	}
	f.handle("w", nil)
	if f.col != 6 {
		t.Fatalf("normal w should jump to the next word, col=%d", f.col)
	}
	f.handle("W", nil)
	if f.col != 11 {
		t.Fatalf("normal W should jump past the word, col=%d", f.col)
	}
	f.handle("B", nil)
	if f.col != 6 {
		t.Fatalf("normal B should jump to the previous word, col=%d", f.col)
	}
	f.handle("g", nil)
	f.handle("e", nil)
	if f.col != 5 {
		t.Fatalf("normal ge should jump to the end of the previous word, col=%d", f.col)
	}
	f.handle("esc", nil)
}

func TestFieldVimUndoRedoAndDelete(t *testing.T) {
	f := newField("hello world", false)
	f.insert = true
	f.col = len([]rune(f.value()))
	f.handle("ctrl+w", nil)
	if f.value() != "hello " {
		t.Fatalf("ctrl+w %q", f.value())
	}
	f.handle("backspace", nil)
	if f.value() != "hello" {
		t.Fatalf("backspace %q", f.value())
	}
	f = newField("hello world", false)
	f.insert = true
	f.col = len([]rune(f.value()))
	f.handle("alt+backspace", nil)
	if f.value() != "hello " {
		t.Fatalf("alt %q", f.value())
	}

	f = newField("hello world", false)
	f.insert = true
	f.col = 0
	f.handle("esc", nil)
	f.handle("x", nil)
	if f.value() != "ello world" {
		t.Fatalf("normal x should delete the character, %q", f.value())
	}
	f.handle("u", nil)
	if f.value() != "hello world" {
		t.Fatalf("normal u should undo, %q", f.value())
	}
	f.handle("ctrl+r", nil)
	if f.value() != "ello world" {
		t.Fatalf("ctrl+r should redo, %q", f.value())
	}
	sub, cancel := f.handle("q", nil)
	if !cancel || sub {
		t.Fatalf("normal q should cancel, got %v %v", sub, cancel)
	}
}

func TestFieldVimOpenLine(t *testing.T) {
	f := newField("hello world", false)
	f.insert = true
	f.handle("esc", nil)
	f.handle("o", nil)
	if f.value() != "hello world\n" || f.row != 1 || f.col != 0 {
		t.Fatalf("o split: %q row=%d col=%d", f.value(), f.row, f.col)
	}
	f = newField("hello world", false)
	f.insert = true
	f.handle("esc", nil)
	f.handle("O", nil)
	if f.value() != "\nhello world" || f.row != 0 || f.col != 0 {
		t.Fatalf("O split: %q row=%d col=%d", f.value(), f.row, f.col)
	}
}

func TestCardFieldVimArrows(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "e")
	if m.top() != layerCard {
		t.Fatalf("card layer %d", m.top())
	}
	press(m, "i")
	if !m.edit.content.insert {
		t.Fatal("content should be in insert mode")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.edit.content.col != 1 {
		t.Fatalf("insert arrow should move the cursor, col=%d", m.edit.content.col)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.edit.content.col != 4 {
		t.Fatalf("insert ctrl+arrow should jump a word, col=%d", m.edit.content.col)
	}
	press(m, "esc")
	if m.edit.content.insert {
		t.Fatal("esc should leave insert mode")
	}
	press(m, "b")
	if m.edit.content.col != 0 {
		t.Fatalf("normal b should jump to the word start, col=%d", m.edit.content.col)
	}
	press(m, "i")
	if !m.edit.content.insert {
		t.Fatal("content should be in insert mode again")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.edit.content.value() != "nship" {
		t.Fatalf("typing after jump should insert, %q", m.edit.content.value())
	}
}

func TestDueCalendarPick(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "T")
	if m.top() != layerCalendar {
		t.Fatalf("due layer %d", m.top())
	}
	m.calYear, m.calMonth, m.calSel = 2020, 0, "2020-01-02"
	if !strings.Contains(m.dialog(), "January 2020") || !strings.Contains(m.dialog(), "2020-01-02") {
		t.Fatalf("calendar dialog\n%s", m.dialog())
	}
	press(m, "enter")
	if m.top() != layerNone {
		t.Fatalf("calendar should close on pick, layer %d", m.top())
	}
	if s.Boards[0].Columns[0].Cards[0].Due != "2020-01-02" {
		t.Fatalf("due %q", s.Boards[0].Columns[0].Cards[0].Due)
	}
}

func TestDueCalendarFromCardEdit(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "e")
	if m.top() != layerCard {
		t.Fatalf("card layer %d", m.top())
	}
	press(m, "T")
	if m.top() != layerCalendar {
		t.Fatalf("due layer %d", m.top())
	}
	m.calYear, m.calMonth, m.calSel = 2020, 0, "2020-01-02"
	press(m, "enter")
	if m.top() != layerCard {
		t.Fatalf("calendar should return to the card, layer %d", m.top())
	}
	if m.edit.due != "2020-01-02" {
		t.Fatalf("edit due %q", m.edit.due)
	}
	press(m, "enter")
	if m.edit.isNew || m.top() != layerNone {
		t.Fatalf("save card layer %d", m.top())
	}
	if s.Boards[0].Columns[0].Cards[0].Due != "2020-01-02" {
		t.Fatalf("saved due %q", s.Boards[0].Columns[0].Cards[0].Due)
	}
}

func TestDueCalendarNavigation(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Load(filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship"}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	press(m, "T")
	if m.top() != layerCalendar {
		t.Fatalf("due layer %d", m.top())
	}
	m.calYear, m.calMonth, m.calSel = 2020, 0, "2020-01-02"
	press(m, "j")
	if m.calSel != "2020-01-03" {
		t.Fatalf("j moved to %q", m.calSel)
	}
	press(m, "k")
	if m.calSel != "2020-01-02" {
		t.Fatalf("k moved to %q", m.calSel)
	}
	press(m, "h")
	if m.calSel != "2020-01-01" {
		t.Fatalf("h moved to %q", m.calSel)
	}
	press(m, "l")
	if m.calSel != "2020-01-02" {
		t.Fatalf("l moved to %q", m.calSel)
	}
	press(m, "down")
	if m.calSel != "2020-01-03" {
		t.Fatalf("down moved to %q", m.calSel)
	}
	press(m, "up")
	if m.calSel != "2020-01-02" {
		t.Fatalf("up moved to %q", m.calSel)
	}
	press(m, "0")
	if m.calSel != "2020-01-01" {
		t.Fatalf("0 moved to %q", m.calSel)
	}
	press(m, "$")
	if m.calSel != "2020-01-31" {
		t.Fatalf("$ moved to %q", m.calSel)
	}
	press(m, "g")
	press(m, "g")
	if m.calSel != "2020-01-01" {
		t.Fatalf("gg moved to %q", m.calSel)
	}
	press(m, "G")
	if m.calSel != "2020-01-31" {
		t.Fatalf("G moved to %q", m.calSel)
	}
	press(m, "+", "3")
	if m.calSel != "2020-02-03" {
		t.Fatalf("snooze moved to %q", m.calSel)
	}
	press(m, "d")
	if m.calSel != "" {
		t.Fatalf("d should clear the picked day, got %q", m.calSel)
	}
	press(m, "esc")
	if m.top() != layerNone {
		t.Fatalf("esc should close the calendar, layer %d", m.top())
	}
}
