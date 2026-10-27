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
	press(m, "T", "i")
	typeRunes(m, "2020-01-02")
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

func TestTagPicker(t *testing.T) {
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
	view := m.dialog()
	for _, tag := range []string{"#bug", "#feature", "#documentation", "#ci/cd", "#deployment", "#research", "#legacy"} {
		if !strings.Contains(view, tag) {
			t.Fatalf("missing %s\n%s", tag, view)
		}
	}
	if !strings.Contains(view, "[x] #legacy") {
		t.Fatalf("legacy should start selected\n%s", view)
	}
	typeRunes(m, "dep")
	filtered := m.dialog()
	if strings.Contains(filtered, "#bug") || strings.Contains(filtered, "#devops") || !strings.Contains(filtered, "#deployment") {
		t.Fatalf("filter\n%s", filtered)
	}
	press(m, "esc", "e", "t")
	if m.top() != layerTag {
		t.Fatalf("layer %d", m.top())
	}
	typeRunes(m, "legacy")
	press(m, "space")
	for range len("legacy") {
		press(m, "backspace")
	}
	typeRunes(m, "bug")
	press(m, "space", "enter")
	if got := m.edit.tags; len(got) != 1 || got[0] != "bug" {
		t.Fatalf("card edit tags %+v", got)
	}
	if len(s.Boards[0].Columns[0].Cards[0].Tags) != 1 || s.Boards[0].Columns[0].Cards[0].Tags[0] != "legacy" {
		t.Fatal("store changed before save")
	}
	press(m, "enter")
	if tags := s.Boards[0].Columns[0].Cards[0].Tags; len(tags) != 1 || tags[0] != "bug" {
		t.Fatalf("saved %+v", tags)
	}
	press(m, "t")
	typeRunes(m, "spike")
	if !strings.Contains(m.dialog(), "enter adds #spike") {
		t.Fatalf("add row\n%s", m.dialog())
	}
	press(m, "enter")
	tags := s.Boards[0].Columns[0].Cards[0].Tags
	if len(tags) != 2 || tags[0] != "bug" || tags[1] != "spike" || !strings.Contains(m.View(), "#spike") {
		t.Fatalf("custom %+v\n%s", tags, m.View())
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

func TestDeleteAddedTag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KABMAT_CONFIG", filepath.Join(dir, "config"))
	data := t.TempDir()
	s, err := store.Load(filepath.Join(data, "data"), filepath.Join(data, "data_bkp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBoard("Work"); err != nil || s.AddColumn(0, "Todo") != nil {
		t.Fatal(err)
	}
	if err := s.AddCard(0, 0, store.Card{Content: "ship", Tags: []string{"spike", "bug"}}); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(s, "Work")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 30
	press(m, "t")
	if m.tagList[m.tagSel] != "bug" {
		t.Fatalf("sel %s", m.tagList[m.tagSel])
	}
	press(m, "ctrl+d")
	if m.notice != "deleted #bug" || strings.Contains(m.dialog(), "#bug") {
		t.Fatalf("delete bug %q\n%s", m.notice, m.dialog())
	}
	typeRunes(m, "spike")
	press(m, "ctrl+d")
	if strings.Contains(m.dialog(), "#spike") || m.notice != "deleted #spike" {
		t.Fatalf("delete notice %q\n%s", m.notice, m.dialog())
	}
	if tags := s.Boards[0].Columns[0].Cards[0].Tags; len(tags) != 0 {
		t.Fatalf("card tags %+v", tags)
	}
	press(m, "esc", "t")
	if strings.Contains(m.dialog(), "#spike") {
		t.Fatalf("spike returned\n%s", m.dialog())
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
		"[tags]\n#review\n\n" +
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
	if board.top() == layerTag {
		t.Fatal("t should not open tags")
	}
	press(board, "F")
	if board.top() != layerTag || !strings.Contains(board.dialog(), "#review") {
		t.Fatalf("tags\n%s", board.dialog())
	}
	press(board, "esc", "2", "j")
	if board.cardIndex(0) != 2 {
		t.Fatalf("count move landed on %d", board.cardIndex(0))
	}

	os.WriteFile(path, []byte("[keys.board]\nnope z\n"), 0o600)
	if _, err := newModel(s, "Work"); err == nil {
		t.Fatal("bad config should fail to open")
	}
}

func TestConfigKeyAndTag(t *testing.T) {
	path := mConfigPath(t)
	body := "theme linux-dex\n\n[tags]\nspike\n\n[keys.board]\nnew-card x\n"
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
	if !strings.Contains(m.dialog(), "#spike") {
		t.Fatalf("config tag\n%s", m.dialog())
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
