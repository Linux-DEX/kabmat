package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kabmat/internal/store"
)

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
	for _, corner := range []string{"╭", "╮", "╰", "╯", "│"} {
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
	press(m, "f")
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
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
