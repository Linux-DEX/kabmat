package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	press(m, "enter", "space", "q", "enter")
	item := s.Boards[0].Columns[0].Cards[0].Checklist
	if len(item) != 1 || item[0].Content != "box" || !item[0].Done {
		t.Fatalf("checklist: %+v", item)
	}
	press(m, "q")
	if m.screen != screenMenu {
		t.Fatal("expected menu")
	}
	if !strings.Contains(m.View(), "Work") {
		t.Fatal(m.View())
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
