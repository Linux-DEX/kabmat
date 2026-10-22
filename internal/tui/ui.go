package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Linux-DEX/kabmat/internal/config"
	"github.com/Linux-DEX/kabmat/internal/store"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenMenu screen = iota
	screenBoard
)

type layer int

const (
	layerNone layer = iota
	layerHelp
	layerInput
	layerConfirm
	layerCard
	layerChecklist
	layerSubtasks
	layerSubtask
	layerMove
	layerTheme
	layerTag
)

type inputKind int

const (
	inputNewBoard inputKind = iota
	inputRenameBoard
	inputNewColumn
	inputRenameColumn
	inputNewItem
	inputEditItem
	inputWIP
	inputTags
	inputDue
	inputNewTheme
	inputThemeColor
	inputNewTag
)

const staleDays = 7

type confirmKind int

const (
	confirmDeleteBoard confirmKind = iota
	confirmDeleteColumn
	confirmDeleteCard
	confirmDeleteCards
)

type cardEdit struct {
	isNew     bool
	col       int
	card      int
	content   field
	desc      field
	focus     int
	checklist []store.ChecklistItem
	subtasks  []store.Subtask
	priority  int
	due       string
	tags      []string
	moved     string
}

type subEdit struct {
	index   int
	content field
	desc    field
	focus   int
	done    bool
}

type model struct {
	store        *store.Store
	width        int
	height       int
	screen       screen
	board        int
	returnToMenu bool
	quitting     bool
	notice       string

	menuSel    int
	menuScroll int
	col        int
	colScroll  int
	cardSel    []int
	cardScroll []int

	stack       []layer
	helpOff     int
	prompt      field
	promptTitle string
	promptKind  inputKind
	confirmMsg  string
	confirmKind confirmKind
	edit        cardEdit
	checkSel    int
	subSel      int
	sub         subEdit
	count       int
	clip        store.Card
	hasClip     bool
	filtering   bool
	foldSubs    bool
	seenBoard   bool
	cfg         config.Config
	moveSel     int
	search      string
	searching   bool
	searchBuf   field
	searchMiss  bool
	searchCol   int
	searchSel   []int
	picked      map[[2]int]struct{}
	hideArchive bool

	themeName  string
	themeWas   string
	schemes    []scheme
	schemeSel  int
	colorSel   int
	schemeEdit bool

	tagList   []string
	tagOn     map[string]bool
	tagSel    int
	tagFilter string
}

func Run(s *store.Store, open string) error {
	m, err := newModel(s, open)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(s *store.Store, open string) (*model, error) {
	m := &model{
		store:        s,
		width:        80,
		height:       24,
		returnToMenu: true,
		foldSubs:     true,
		themeName:    "linux-dex",
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	m.cfg = cfg
	m.loadTheme()
	if open == "" {
		p, err := store.LoadPlace(s.Path)
		if err != nil {
			return nil, err
		}
		if p.Board == "" {
			return m, nil
		}
		i, err := s.Index(p.Board)
		if err != nil {
			return m, nil
		}
		m.openBoard(i, true)
		m.col = p.Col
		m.growCardSel()
		if m.col >= 0 && m.col < len(m.cardSel) {
			m.cardSel[m.col] = p.Card
		}
		return m, nil
	}
	i, err := s.Index(open)
	if err != nil {
		return nil, err
	}
	m.openBoard(i, false)
	return m, nil
}

func (m *model) openBoard(i int, backToMenu bool) {
	m.screen = screenBoard
	m.board = i
	m.menuSel = i
	m.returnToMenu = backToMenu
	m.seenBoard = true
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
	case tea.KeyMsg:
		m.onKey(msg.String(), msg.Runes)
	default:
		if ctrlBackspaceCSI(msg) {
			m.onKey("ctrl+backspace", nil)
		}
	}
	if err := m.store.Flush(); err != nil {
		m.quitting = false
		m.notice = err.Error()
		return m, nil
	}
	if m.quitting {
		m.remember()
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	if m.top() != layerNone {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.dialog())
	}
	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	var body string
	if m.screen == screenMenu {
		body = m.menuBody(bodyH)
	} else {
		body = m.boardBody(bodyH)
	}
	return strings.TrimRight(m.header()+"\n"+body+"\n"+m.footer(), "\n")
}

func (m *model) onKey(key string, runes []rune) {
	if key == "ctrl+c" {
		m.quitting = true
		return
	}
	m.notice = ""
	if m.searching {
		m.searchKey(key, runes)
		return
	}
	switch m.top() {
	case layerHelp:
		m.helpKey(key)
	case layerInput:
		m.inputKey(key, runes)
	case layerConfirm:
		m.confirmKey(key)
	case layerChecklist:
		m.checkKey(key)
	case layerSubtasks:
		m.subListKey(key)
	case layerSubtask:
		m.subKey(key, runes)
	case layerCard:
		m.cardKey(key, runes)
	case layerMove:
		m.moveKey(key)
	case layerTheme:
		m.themeKey(key)
	case layerTag:
		m.tagKey(key)
	default:
		m.normalKey(key)
	}
}

// ctrlBackspaceCSI reports Kitty CSI-u and xterm modifyOtherKeys for ctrl+backspace.
// Those sequences differ by terminal; bubbletea 1.3 leaves them as an unknown CSI message.
func ctrlBackspaceCSI(msg tea.Msg) bool {
	s := fmt.Sprint(msg)
	// 127;5u and 127;5;1u (press)
	if strings.Contains(s, "49 50 55 59 53 117") || strings.Contains(s, "49 50 55 59 53 59 49 117") {
		return true
	}
	// ESC [ 27 ; 5 ; 127 ~
	return strings.Contains(s, "50 55 59 53 59 49 50 55 126")
}

func (m *model) menuKey(key string) {
	n := len(m.store.Boards)
	switch m.cfg.Action("menu", key) {
	case "quit":
		m.quitting = true
	case "undo":
		m.undoRedo(false)
	case "redo":
		m.undoRedo(true)
	case "help":
		m.helpOff = 0
		m.push(layerHelp)
	case "up":
		if n > 0 && m.menuSel > 0 {
			m.menuSel--
		}
	case "down":
		if n > 0 && m.menuSel < n-1 {
			m.menuSel++
		}
	case "first":
		m.menuSel = 0
	case "last":
		if n > 0 {
			m.menuSel = n - 1
		}
	case "move-up":
		m.reorderBoard(-1)
	case "move-down":
		m.reorderBoard(1)
	case "new":
		m.openInput(inputNewBoard, " New Board Name ", "")
	case "rename":
		if n > 0 {
			m.openInput(inputRenameBoard, " Rename Board ", m.store.Boards[m.menuSel].Name)
		}
	case "delete":
		if n > 0 {
			m.confirmKind = confirmDeleteBoard
			m.confirmMsg = fmt.Sprintf("Delete board %q?", shortName(m.store.Boards[m.menuSel].Name))
			m.push(layerConfirm)
		}
	case "open":
		if n > 0 {
			m.col = 0
			m.colScroll = 0
			m.cardSel = nil
			m.cardScroll = nil
			m.openBoard(m.menuSel, true)
		}
	case "theme":
		m.openThemes()
	}
}

func (m *model) boardKey(key string) {
	b := m.curBoard()
	if b == nil {
		m.quitting = true
		return
	}
	n := len(b.Columns)
	switch m.cfg.Action("board", key) {
	case "back":
		if m.returnToMenu {
			m.screen = screenMenu
			return
		}
		m.quitting = true
	case "undo":
		m.undoRedo(false)
	case "redo":
		m.undoRedo(true)
	case "search":
		m.startSearch()
	case "find-next":
		m.findNext(1)
	case "find-prev":
		m.findNext(-1)
	case "help":
		m.helpOff = 0
		m.push(layerHelp)
	case "left":
		m.stepCol(-1)
	case "right":
		m.stepCol(1)
	case "up":
		m.moveCardFocus(-1)
	case "down":
		m.moveCardFocus(1)
	case "first":
		if ids := m.shownIDs(m.col); len(ids) > 0 {
			m.growCardSel()
			m.cardSel[m.col] = ids[0]
		}
	case "last":
		if ids := m.shownIDs(m.col); len(ids) > 0 {
			m.growCardSel()
			m.cardSel[m.col] = ids[len(ids)-1]
		}
	case "move-left":
		m.shiftCard(-1)
	case "move-right":
		m.shiftCard(1)
	case "wip":
		if n > 0 {
			cur := ""
			if b.Columns[m.col].WIP > 0 {
				cur = strconv.Itoa(b.Columns[m.col].WIP)
			}
			m.openInput(inputWIP, " WIP limit (0 clears) ", cur)
		}
	case "gate":
		m.toggleGate()
	case "archive":
		m.archiveTargets()
	case "toggle-archive":
		m.hideArchive = !m.hideArchive
		m.snapCol()
		if m.hideArchive {
			m.notice = "archive hidden"
		} else {
			m.notice = "archive shown"
		}
	case "priority":
		m.cyclePriority()
	case "clear-selection":
		if len(m.picked) > 0 {
			m.picked = nil
			m.notice = "selection cleared"
		}
	case "select":
		m.togglePick()
	case "tags":
		if len(m.targets()) > 0 {
			m.openTags()
		}
	case "due":
		if len(m.targets()) > 0 {
			m.openDue()
		}
	case "open-link":
		m.openLink()
	case "move-up":
		m.swapCard(-1)
	case "move-down":
		m.swapCard(1)
	case "column-left":
		m.shiftColumn(-1)
	case "column-right":
		m.shiftColumn(1)
	case "new-column":
		m.openInput(inputNewColumn, " New Column Name ", "")
	case "rename-column":
		if n > 0 {
			m.openInput(inputRenameColumn, " Rename Column ", b.Columns[m.col].Title)
		}
	case "delete-column":
		if n > 0 {
			m.confirmKind = confirmDeleteColumn
			m.confirmMsg = fmt.Sprintf("Delete column %q?", shortName(b.Columns[m.col].Title))
			m.push(layerConfirm)
		}
	case "new-card":
		if n > 0 {
			m.edit = cardEdit{isNew: true, col: m.col, card: -1, content: newField("", false), desc: newField("", true)}
			m.push(layerCard)
		}
	case "edit":
		m.openCard()
	case "subtasks":
		if m.openCard() {
			m.push(layerSubtasks)
		}
	case "delete":
		m.confirmDelete()
	case "yank":
		m.yankCard()
	case "paste":
		m.pasteCard()
	case "duplicate":
		m.duplicateCard()
	case "filter":
		m.toggleFilter()
	case "fold":
		m.foldSubs = !m.foldSubs
	case "board-prev":
		m.switchBoard(-1)
	case "board-next":
		m.switchBoard(1)
	case "move":
		if n > 0 && m.cardIndex(m.col) >= 0 {
			m.moveSel = m.col
			m.push(layerMove)
		}
	case "clear-done":
		m.clearDone()
	case "theme":
		m.openThemes()
	}
}

func (m *model) helpKey(key string) {
	switch key {
	case "q", "esc":
		m.pop()
	case "k":
		if m.helpOff > 0 {
			m.helpOff--
		}
	case "j":
		if m.helpOff < len(helpLines)-1 {
			m.helpOff++
		}
	}
}

func (m *model) inputKey(key string, runes []rune) {
	submit, cancel := m.prompt.handle(key, runes)
	if cancel {
		m.pop()
		return
	}
	if !submit {
		return
	}
	value := m.prompt.value()
	fromCard := len(m.stack) >= 2 && m.stack[len(m.stack)-2] == layerCard
	m.pop()
	var err error
	switch m.promptKind {
	case inputNewBoard:
		err = m.store.CreateBoard(value)
		if err == nil {
			m.menuSel = len(m.store.Boards) - 1
		}
	case inputRenameBoard:
		err = m.store.RenameBoard(m.menuSel, value)
	case inputNewColumn:
		err = m.store.AddColumn(m.board, value)
		if err == nil {
			m.col = len(m.curBoard().Columns) - 1
		}
	case inputRenameColumn:
		err = m.store.RenameColumn(m.board, m.col, value)
	case inputNewItem:
		value = strings.TrimSpace(value)
		if value != "" {
			m.edit.checklist = append(m.edit.checklist, store.ChecklistItem{Content: value})
			m.checkSel = len(m.edit.checklist) - 1
		}
	case inputEditItem:
		value = strings.TrimSpace(value)
		if value != "" && m.checkSel >= 0 && m.checkSel < len(m.edit.checklist) {
			m.edit.checklist[m.checkSel].Content = value
		}
	case inputWIP:
		err = m.setWIP(value)
	case inputTags:
		tags := store.ParseTags(value)
		if fromCard {
			m.edit.tags = tags
		} else {
			err = m.store.ApplyCards(m.board, m.targets(), func(c *store.Card) {
				c.Tags = append([]string(nil), tags...)
			})
		}
	case inputDue:
		var due string
		due, err = parseDue(value)
		if err != nil {
			break
		}
		if fromCard {
			m.edit.due = due
		} else {
			err = m.store.ApplyCards(m.board, m.targets(), func(c *store.Card) {
				c.Due = due
			})
		}
	case inputNewTheme:
		m.addScheme(value)
	case inputThemeColor:
		m.setSchemeColor(value)
	case inputNewTag:
		m.addTypedTag(value)
	}
	if err != nil {
		m.notice = err.Error()
	}
}

func (m *model) confirmKey(key string) {
	switch key {
	case "y", "enter":
		m.pop()
		m.applyConfirm()
	case "n", "esc":
		m.pop()
	}
}

func (m *model) applyConfirm() {
	var err error
	switch m.confirmKind {
	case confirmDeleteBoard:
		err = m.store.DeleteBoardAt(m.menuSel)
		if m.menuSel >= len(m.store.Boards) {
			m.menuSel = len(m.store.Boards) - 1
		}
		if m.menuSel < 0 {
			m.menuSel = 0
		}
	case confirmDeleteColumn:
		err = m.store.DeleteColumn(m.board, m.col)
		if err == nil {
			if m.col < len(m.cardSel) {
				m.cardSel = append(m.cardSel[:m.col], m.cardSel[m.col+1:]...)
			}
			if m.col < len(m.cardScroll) {
				m.cardScroll = append(m.cardScroll[:m.col], m.cardScroll[m.col+1:]...)
			}
		}
		if b := m.curBoard(); b != nil && m.col >= len(b.Columns) && m.col > 0 {
			m.col--
		}
	case confirmDeleteCard:
		ci := m.cardIndex(m.col)
		if ci >= 0 {
			err = m.store.DeleteCard(m.board, m.col, ci)
		}
	case confirmDeleteCards:
		err = m.store.DeleteCards(m.board, m.targets())
		m.picked = nil
	}
	if err != nil {
		m.notice = err.Error()
	}
}

func (m *model) cardKey(key string, runes []rune) {
	f := m.activeField()
	switch key {
	case "esc":
		if f.insert {
			f.handle("esc", nil)
			return
		}
		m.pop()
	case "q":
		if f.insert {
			f.handle("", []rune{'q'})
			return
		}
		m.pop()
	case "enter":
		if m.edit.focus == 0 || !f.insert {
			m.submitCard()
			return
		}
		f.handle("enter", nil)
	case "tab":
		m.edit.focus ^= 1
	case "!":
		if !f.insert {
			m.edit.priority = (m.edit.priority + 1) % 3
			return
		}
		f.handle(key, runes)
	case "t":
		if !f.insert {
			m.openTags()
			return
		}
		f.handle(key, runes)
	case "T":
		if !f.insert {
			m.openDue()
			return
		}
		f.handle(key, runes)
	case "c":
		if !f.insert && strings.TrimSpace(m.edit.content.value()) != "" {
			if len(m.edit.checklist) == 0 {
				m.checkSel = 0
			}
			m.push(layerChecklist)
			return
		}
		f.handle(key, runes)
	case "s":
		if !f.insert && strings.TrimSpace(m.edit.content.value()) != "" {
			if len(m.edit.subtasks) == 0 {
				m.subSel = 0
			}
			m.push(layerSubtasks)
			return
		}
		f.handle(key, runes)
	default:
		f.handle(key, runes)
	}
}

func (m *model) openCard() bool {
	b := m.curBoard()
	if b == nil || m.col >= len(b.Columns) {
		return false
	}
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return false
	}
	card := b.Columns[m.col].Cards[ci]
	m.edit = cardEdit{
		col:       m.col,
		card:      ci,
		content:   newField(card.Content, false),
		desc:      newField(card.Description, true),
		checklist: cloneItems(card.Checklist),
		subtasks:  cloneSubs(card.Subtasks),
		priority:  card.Priority,
		due:       card.Due,
		tags:      cloneTags(card.Tags),
		moved:     card.Moved,
	}
	m.push(layerCard)
	return true
}

func (m *model) submitCard() {
	card := store.Card{
		Content:     m.edit.content.value(),
		Description: strings.TrimRight(m.edit.desc.value(), "\n"),
		Checklist:   m.edit.checklist,
		Subtasks:    m.edit.subtasks,
		Priority:    m.edit.priority,
		Due:         m.edit.due,
		Tags:        cloneTags(m.edit.tags),
		Moved:       m.edit.moved,
	}
	var err error
	if m.edit.isNew {
		err = m.store.AddCard(m.board, m.edit.col, card)
		if err == nil {
			m.growCardSel()
			m.cardSel[m.edit.col] = len(m.curBoard().Columns[m.edit.col].Cards) - 1
		}
	} else {
		err = m.store.UpdateCard(m.board, m.edit.col, m.edit.card, card)
	}
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.pop()
}

func (m *model) leaveSubtasks() {
	before := len(m.stack)
	m.submitCard()
	if len(m.stack) == before {
		if m.top() == layerChecklist || m.top() == layerSubtasks || m.top() == layerSubtask {
			m.pop()
		}
		return
	}
	for m.top() == layerCard || m.top() == layerSubtasks || m.top() == layerSubtask {
		m.pop()
	}
}

func (m *model) checkKey(key string) {
	n := len(m.edit.checklist)
	switch key {
	case "q":
		m.leaveSubtasks()
	case "k":
		if m.checkSel > 0 {
			m.checkSel--
		}
	case "j":
		if m.checkSel < n-1 {
			m.checkSel++
		}
	case "g":
		m.checkSel = 0
	case "G":
		if n > 0 {
			m.checkSel = n - 1
		}
	case "K", "shift+up":
		m.checkSel = moveItem(m.edit.checklist, m.checkSel, -1)
	case "J", "shift+down":
		m.checkSel = moveItem(m.edit.checklist, m.checkSel, 1)
	case "c":
		m.openInput(inputNewItem, " New Item ", "")
	case "e":
		if n > 0 {
			m.openInput(inputEditItem, " Edit Item ", m.edit.checklist[m.checkSel].Content)
		}
	case " ", "space":
		if n > 0 {
			m.edit.checklist[m.checkSel].Done = !m.edit.checklist[m.checkSel].Done
		}
	case "d":
		if n > 0 {
			m.edit.checklist = append(m.edit.checklist[:m.checkSel], m.edit.checklist[m.checkSel+1:]...)
			if m.checkSel >= len(m.edit.checklist) && m.checkSel > 0 {
				m.checkSel--
			}
		}
	}
}

func (m *model) subListKey(key string) {
	n := len(m.edit.subtasks)
	switch key {
	case "q":
		m.leaveSubtasks()
	case "k":
		if m.subSel > 0 {
			m.subSel--
		}
	case "j":
		if m.subSel < n-1 {
			m.subSel++
		}
	case "g":
		m.subSel = 0
	case "G":
		if n > 0 {
			m.subSel = n - 1
		}
	case "K", "shift+up":
		m.subSel = moveSub(m.edit.subtasks, m.subSel, -1)
	case "J", "shift+down":
		m.subSel = moveSub(m.edit.subtasks, m.subSel, 1)
	case "c":
		m.openSubtask(-1)
	case "e", "enter":
		if n > 0 {
			m.openSubtask(m.subSel)
		}
	case " ", "space":
		if n > 0 {
			m.edit.subtasks[m.subSel].Done = !m.edit.subtasks[m.subSel].Done
		}
	case "d":
		if n > 0 {
			m.edit.subtasks = append(m.edit.subtasks[:m.subSel], m.edit.subtasks[m.subSel+1:]...)
			if m.subSel >= len(m.edit.subtasks) && m.subSel > 0 {
				m.subSel--
			}
		}
	}
}

func (m *model) openSubtask(index int) {
	var sub store.Subtask
	if index >= 0 {
		sub = m.edit.subtasks[index]
	}
	m.sub = subEdit{
		index:   index,
		content: newField(sub.Content, false),
		desc:    newField(sub.Description, true),
		done:    sub.Done,
	}
	m.push(layerSubtask)
}

func (m *model) subKey(key string, runes []rune) {
	f := m.subField()
	switch key {
	case "esc":
		if f.insert {
			f.handle("esc", nil)
			return
		}
		m.pop()
	case "q":
		if f.insert {
			f.handle("", []rune{'q'})
			return
		}
		m.pop()
	case "enter":
		if m.sub.focus == 0 || !f.insert {
			m.saveSubtask()
			return
		}
		f.handle("enter", nil)
	case "tab":
		m.sub.focus ^= 1
	case " ", "space":
		if !f.insert {
			m.sub.done = !m.sub.done
			return
		}
		f.handle(key, runes)
	default:
		f.handle(key, runes)
	}
}

func (m *model) saveSubtask() {
	sub := store.Subtask{
		Content:     strings.TrimSpace(m.sub.content.value()),
		Description: strings.TrimRight(m.sub.desc.value(), "\n"),
		Done:        m.sub.done,
	}
	if sub.Content == "" {
		m.notice = "subtask title can't be empty"
		return
	}
	if m.sub.index < 0 {
		m.edit.subtasks = append(m.edit.subtasks, sub)
		m.subSel = len(m.edit.subtasks) - 1
	} else {
		m.edit.subtasks[m.sub.index] = sub
	}
	m.pop()
}

func (m *model) subField() *field {
	if m.sub.focus == 0 {
		return &m.sub.content
	}
	return &m.sub.desc
}

func (m *model) switchBoard(dir int) {
	next := m.board + dir
	if next < 0 || next >= len(m.store.Boards) {
		return
	}
	m.col = 0
	m.colScroll = 0
	m.cardSel = nil
	m.cardScroll = nil
	m.picked = nil
	m.openBoard(next, m.returnToMenu)
}

func (m *model) moveKey(key string) {
	n := 0
	if b := m.curBoard(); b != nil {
		n = len(b.Columns)
	}
	switch key {
	case "q", "esc":
		m.pop()
	case "k":
		if m.moveSel > 0 {
			m.moveSel--
		}
	case "j":
		if m.moveSel < n-1 {
			m.moveSel++
		}
	case "g":
		m.moveSel = 0
	case "G":
		if n > 0 {
			m.moveSel = n - 1
		}
	case "enter":
		dst := m.moveSel
		m.pop()
		m.sendCard(dst)
	}
}

func (m *model) sendCard(dst int) {
	picks := m.movePicks(0)
	if len(picks) == 0 {
		return
	}
	var blocked error
	moved := 0
	for _, p := range picks {
		if p[0] == dst {
			continue
		}
		ok, err := m.store.MoveCardTo(m.board, p[0], p[1], dst)
		if err != nil {
			blocked = err
			break
		}
		if ok {
			moved++
		}
	}
	m.picked = nil
	if blocked != nil {
		m.notice = blocked.Error()
	}
	if moved == 0 {
		return
	}
	m.col = dst
	m.growCardSel()
	if m.store.CardAtBottom {
		m.cardSel[dst] = len(m.curBoard().Columns[dst].Cards) - 1
	} else {
		m.cardSel[dst] = 0
	}
}

func (m *model) clearDone() {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	card := m.curBoard().Columns[m.col].Cards[ci]
	kept := make([]store.ChecklistItem, 0, len(card.Checklist))
	for _, item := range card.Checklist {
		if !item.Done {
			kept = append(kept, item)
		}
	}
	if len(kept) == len(card.Checklist) {
		m.notice = "no finished items"
		return
	}
	card.Checklist = kept
	if err := m.store.UpdateCard(m.board, m.col, ci, card); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) remember() {
	if !m.seenBoard {
		return
	}
	b := m.curBoard()
	if b == nil {
		return
	}
	col := m.col
	if col < 0 {
		col = 0
	}
	card := 0
	if col < len(m.cardSel) {
		card = m.cardSel[col]
	}
	_ = store.SavePlace(m.store.Path, store.Place{Board: b.Name, Col: col, Card: card})
}

func (m *model) shiftCard(dir int) {
	picks := m.movePicks(dir)
	if len(picks) == 0 {
		return
	}
	var blocked error
	moved := 0
	focus := m.col
	for _, p := range picks {
		ok, err := m.store.MoveCardHoriz(m.board, p[0], p[1], dir)
		if err != nil {
			blocked = err
			break
		}
		if ok {
			moved++
			focus = p[0] + dir
		}
	}
	m.picked = nil
	if blocked != nil {
		m.notice = blocked.Error()
	}
	if moved == 0 {
		return
	}
	m.col = focus
	m.growCardSel()
	cards := m.curBoard().Columns[m.col].Cards
	if m.store.CardAtBottom {
		m.cardSel[m.col] = len(cards) - 1
	} else {
		m.cardSel[m.col] = 0
	}
}

func (m *model) swapCard(dir int) {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	moved, err := m.store.MoveCardVert(m.board, m.col, ci, dir)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if moved {
		m.cardSel[m.col] = ci + dir
	}
}

func (m *model) reorderBoard(dir int) {
	if len(m.store.Boards) == 0 {
		return
	}
	moved, err := m.store.MoveBoard(m.menuSel, dir)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if moved {
		m.menuSel += dir
	}
}

func (m *model) shiftColumn(dir int) {
	if m.curBoard() == nil || len(m.curBoard().Columns) == 0 {
		return
	}
	moved, err := m.store.MoveColumn(m.board, m.col, dir)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if moved {
		j := m.col + dir
		m.growCardSel()
		m.cardSel[m.col], m.cardSel[j] = m.cardSel[j], m.cardSel[m.col]
		m.cardScroll[m.col], m.cardScroll[j] = m.cardScroll[j], m.cardScroll[m.col]
		m.col = j
	}
}

func (m *model) moveCardFocus(dir int) {
	ids := m.shownIDs(m.col)
	if len(ids) == 0 {
		return
	}
	cur := m.cardIndex(m.col)
	pos := 0
	for i, id := range ids {
		if id == cur {
			pos = i
			break
		}
	}
	next := pos + dir
	if next < 0 || next >= len(ids) {
		return
	}
	m.cardSel[m.col] = ids[next]
}

func (m *model) yankCard() {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	m.clip = cloneCard(m.curBoard().Columns[m.col].Cards[ci])
	m.hasClip = true
	m.notice = "yanked"
}

func (m *model) pasteCard() {
	if !m.hasClip {
		m.notice = "nothing to paste"
		return
	}
	b := m.curBoard()
	if b == nil || len(b.Columns) == 0 {
		m.notice = "no column"
		return
	}
	at := len(b.Columns[m.col].Cards)
	if ci := m.cardIndex(m.col); ci >= 0 {
		at = ci + 1
	}
	m.notice = ""
	if err := m.store.InsertCard(m.board, m.col, at, cloneCard(m.clip)); err != nil {
		m.notice = err.Error()
		return
	}
	m.growCardSel()
	m.cardSel[m.col] = at
}

func (m *model) duplicateCard() {
	m.yankCard()
	if !m.hasClip {
		return
	}
	m.pasteCard()
	if m.notice == "" {
		m.notice = "duplicated"
	}
}

func (m *model) toggleFilter() {
	if m.search == "" {
		m.notice = "no search term, press / first"
		return
	}
	m.filtering = !m.filtering
	if m.filtering {
		m.snapShown()
	}
}

func (m *model) snapShown() {
	ids := m.shownIDs(m.col)
	if len(ids) == 0 {
		return
	}
	cur := m.cardIndex(m.col)
	best := ids[0]
	for _, id := range ids {
		if id >= cur {
			best = id
			break
		}
		best = id
	}
	m.growCardSel()
	m.cardSel[m.col] = best
}

func (m *model) shownIDs(ci int) []int {
	b := m.curBoard()
	if b == nil || ci < 0 || ci >= len(b.Columns) {
		return nil
	}
	cards := b.Columns[ci].Cards
	filter := m.filtering && m.search != ""
	ids := make([]int, 0, len(cards))
	for i := range cards {
		if !filter || cardMatches(&cards[i], m.search) {
			ids = append(ids, i)
		}
	}
	return ids
}

func cloneCard(c store.Card) store.Card {
	c.Checklist = cloneItems(c.Checklist)
	c.Subtasks = cloneSubs(c.Subtasks)
	c.Tags = cloneTags(c.Tags)
	return c
}

func (m *model) openInput(kind inputKind, title, value string) {
	m.promptKind = kind
	m.promptTitle = title
	m.prompt = newField(value, false)
	if kind == inputNewTag {
		m.prompt.insert = true
	}
	m.push(layerInput)
}

func (m *model) activeField() *field {
	if m.edit.focus == 0 {
		return &m.edit.content
	}
	return &m.edit.desc
}

func (m *model) push(l layer) { m.stack = append(m.stack, l) }

func (m *model) pop() {
	if len(m.stack) > 0 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *model) top() layer {
	if len(m.stack) == 0 {
		return layerNone
	}
	return m.stack[len(m.stack)-1]
}

func (m *model) curBoard() *store.Board {
	if m.board < 0 || m.board >= len(m.store.Boards) {
		return nil
	}
	return &m.store.Boards[m.board]
}

func (m *model) growCardSel() {
	n := 0
	if b := m.curBoard(); b != nil {
		n = len(b.Columns)
	}
	if len(m.cardSel) != n {
		next := make([]int, n)
		copy(next, m.cardSel)
		m.cardSel = next
	}
	if len(m.cardScroll) != n {
		next := make([]int, n)
		copy(next, m.cardScroll)
		m.cardScroll = next
	}
}

func (m *model) cardIndex(ci int) int {
	b := m.curBoard()
	if b == nil || ci < 0 || ci >= len(b.Columns) || len(b.Columns[ci].Cards) == 0 {
		return -1
	}
	m.growCardSel()
	if m.cardSel[ci] >= len(b.Columns[ci].Cards) {
		m.cardSel[ci] = len(b.Columns[ci].Cards) - 1
	}
	if m.cardSel[ci] < 0 {
		m.cardSel[ci] = 0
	}
	return m.cardSel[ci]
}

func (m *model) visibleCols() int {
	switch {
	case m.width < 48:
		return 1
	case m.width < 90:
		return 2
	default:
		return 3
	}
}

func (m *model) shownCols() []int {
	b := m.curBoard()
	if b == nil {
		return nil
	}
	out := make([]int, 0, len(b.Columns))
	for i := range b.Columns {
		if m.hideArchive && b.Columns[i].Archive {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (m *model) snapCol() {
	ids := m.shownCols()
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		if id == m.col {
			return
		}
	}
	best := ids[0]
	for _, id := range ids {
		if id <= m.col {
			best = id
		}
	}
	m.col = best
}

func (m *model) stepCol(dir int) {
	ids := m.shownCols()
	pos := -1
	for i, id := range ids {
		if id == m.col {
			pos = i
			break
		}
	}
	if pos < 0 {
		m.snapCol()
		return
	}
	next := pos + dir
	if next < 0 || next >= len(ids) {
		return
	}
	m.col = ids[next]
}

func (m *model) pinCol() {
	ids := m.shownCols()
	if len(ids) == 0 {
		m.colScroll = 0
		return
	}
	m.snapCol()
	pos := 0
	for i, id := range ids {
		if id == m.col {
			pos = i
			break
		}
	}
	vis := m.visibleCols()
	if pos < m.colScroll {
		m.colScroll = pos
	}
	if pos >= m.colScroll+vis {
		m.colScroll = pos - vis + 1
	}
	if m.colScroll < 0 {
		m.colScroll = 0
	}
}

func (m *model) pinMenu(window int) {
	n := len(m.store.Boards)
	if n == 0 {
		m.menuSel = 0
		m.menuScroll = 0
		return
	}
	if m.menuSel >= n {
		m.menuSel = n - 1
	}
	if m.menuSel < 0 {
		m.menuSel = 0
	}
	if window < 1 {
		window = 1
	}
	if m.menuSel < m.menuScroll {
		m.menuScroll = m.menuSel
	}
	if m.menuSel >= m.menuScroll+window {
		m.menuScroll = m.menuSel - window + 1
	}
}

func (m *model) header() string {
	if m.screen != screenBoard || m.curBoard() == nil {
		label := " kabmat"
		if n := len(m.store.Boards); n > 0 && m.screen == screenMenu {
			label += "  ·  " + unit(n, "board", "boards")
		}
		return barStyle.Width(m.width).Render(fit(label, max(1, m.width)))
	}
	b := m.curBoard()
	cards, archived := 0, 0
	var parts []string
	for i := range b.Columns {
		col := b.Columns[i]
		n := len(col.Cards)
		cards += n
		if col.Archive {
			archived += n
			if m.hideArchive {
				continue
			}
		}
		parts = append(parts, colHeadPlain(col))
	}
	right := unit(cards, "card", "cards")
	if m.hideArchive && archived > 0 {
		right += " · " + strconv.Itoa(archived) + " archived"
	}
	left := " " + b.Name
	mid := strings.Join(parts, "  ")
	if lipgloss.Width(left)+2+lipgloss.Width(mid)+2+lipgloss.Width(right) > m.width {
		mid = ""
	}
	line := left
	if mid != "" {
		line += "  " + mid
	}
	pad := m.width - lipgloss.Width(line) - lipgloss.Width(right)
	if pad < 1 {
		line = fit(left, max(1, m.width-lipgloss.Width(right)-1))
		pad = m.width - lipgloss.Width(line) - lipgloss.Width(right)
		if pad < 1 {
			pad = 1
		}
	}
	return barStyle.Width(m.width).Render(line + strings.Repeat(" ", pad) + right)
}

func (m *model) footer() string {
	if m.searching {
		return m.searchBar()
	}
	hint := fmt.Sprintf("%s quit   %s/%s select   %s/%s reorder   %s new   %s open   %s help",
		m.cfg.Label("menu", "quit"), m.cfg.Label("menu", "down"), m.cfg.Label("menu", "up"),
		m.cfg.Label("menu", "move-down"), m.cfg.Label("menu", "move-up"),
		m.cfg.Label("menu", "new"), m.cfg.Label("menu", "open"), m.cfg.Label("menu", "help"))
	if m.screen == screenBoard {
		hint = fmt.Sprintf("%s back   %s/%s focus   %s/%s yank/paste   %s search   %s filter   %s fold   %s help",
			m.cfg.Label("board", "back"), m.cfg.Label("board", "down"), m.cfg.Label("board", "up"),
			m.cfg.Label("board", "yank"), m.cfg.Label("board", "paste"),
			m.cfg.Label("board", "search"), m.cfg.Label("board", "filter"),
			m.cfg.Label("board", "fold"), m.cfg.Label("board", "help"))
	}
	if m.notice != "" {
		hint = m.notice
	} else if n := len(m.picked); n > 0 && m.screen == screenBoard {
		hint = fmt.Sprintf("%d selected   H/L move   d delete   A archive   t tag   esc clear", n)
	} else if m.count > 0 {
		hint = fmt.Sprintf("count: %d", m.count)
	}
	mode := ""
	editing := m.top() == layerInput || m.top() == layerCard || m.top() == layerSubtask
	inserting := m.top() == layerInput && m.prompt.insert ||
		m.top() == layerCard && m.activeField().insert ||
		m.top() == layerSubtask && m.subField().insert
	if editing {
		mode = modeBadge(inserting)
	}
	if p := m.position(); p != "" {
		mode = p + mode
	}
	pad := m.width - lipgloss.Width(hint) - lipgloss.Width(mode)
	if pad < 1 {
		hint = fit(hint, max(1, m.width-lipgloss.Width(mode)-1))
		pad = 1
	}
	return barStyle.Render(hint + strings.Repeat(" ", pad) + mode)
}

func modeBadge(insert bool) string {
	word := " NORMAL "
	if insert {
		word = " INSERT "
	}
	return modeStyle.Render(word)
}

func modeLine(insert bool, hint string, width int) string {
	badge := modeBadge(insert)
	room := width - lipgloss.Width(badge) - 2
	if room < 1 {
		return badge
	}
	return badge + "  " + mutedStyle.Render(fit(hint, room))
}

func (m *model) menuBody(h int) string {
	innerW := max(36, min(64, m.width-8))
	textW := max(8, innerW-2)
	innerH := max(8, h-2)
	window := max(1, innerH-2)
	m.pinMenu(window)
	var lines []string
	if len(m.store.Boards) == 0 {
		lines = append(lines, mutedStyle.Render("c to create a new board"))
	} else {
		for i := m.menuScroll; i < len(m.store.Boards) && i < m.menuScroll+window; i++ {
			row := boardRow(m.store.Boards[i], textW)
			if i == m.menuSel {
				row = selStyle.Width(textW).Render(row)
			}
			lines = append(lines, row)
		}
	}
	box := boxOnStyle.Width(innerW).Height(innerH).Render(" Boards \n" + strings.Join(lines, "\n") + "\n" + mutedStyle.Render(fit("j/k select   J/K reorder   enter open", textW)))
	return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, box)
}

func boardMeta(b store.Board) string {
	cards := 0
	for i := range b.Columns {
		cards += len(b.Columns[i].Cards)
	}
	return unit(len(b.Columns), "col", "cols") + "  " + unit(cards, "card", "cards")
}

func unit(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func boardRow(b store.Board, width int) string {
	meta := boardMeta(b)
	metaW := lipgloss.Width(meta)
	if metaW+4 > width {
		return fit(b.Name, width)
	}
	name := fit(b.Name, width-metaW-1)
	gap := width - lipgloss.Width(name) - metaW
	if gap < 1 {
		gap = 1
	}
	return name + strings.Repeat(" ", gap) + meta
}

func (m *model) boardBody(h int) string {
	b := m.curBoard()
	if b == nil || len(b.Columns) == 0 {
		return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, mutedStyle.Render("C to create a new column"))
	}
	ids := m.shownCols()
	if len(ids) == 0 {
		return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, mutedStyle.Render("v to show the archive"))
	}
	m.pinCol()
	vis := m.visibleCols()
	gap := 1
	colW := (m.width - gap*(vis-1)) / vis
	if colW < 16 {
		colW = 16
	}
	end := min(len(ids), m.colScroll+vis)
	boxes := make([]string, 0, end-m.colScroll)
	for n, i := range ids[m.colScroll:end] {
		if n > 0 {
			boxes = append(boxes, strings.Repeat(" ", gap))
		}
		boxes = append(boxes, m.columnBox(i, colW, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

func (m *model) columnBox(ci, w, h int) string {
	col := m.curBoard().Columns[ci]
	innerW := max(4, w-4)
	innerH := max(2, h-2)
	cardRoom := max(1, innerH-1)
	textW := max(1, innerW-2)
	if ci == m.col && m.filtering && m.search != "" {
		m.snapShown()
	}
	focus := m.cardIndex(ci)
	sel := -1
	if ci == m.col {
		sel = focus
	}
	ids := m.shownIDs(ci)
	blocks := make([][]string, len(ids))
	ensure := func(i int) []string {
		if blocks[i] == nil {
			id := ids[i]
			blocks[i] = cardLines(col.Cards[id], textW, id == sel, m.foldSubs, m.isPicked(ci, id), col.Archive)
		}
		return blocks[i]
	}
	selPos := -1
	for i, id := range ids {
		if id == focus {
			selPos = i
			break
		}
	}
	start := m.scrollShown(ci, len(ids), selPos, cardRoom, func(i int) int {
		return len(ensure(i))
	})
	plain, over := columnHeading(col, len(ids), m.filtering && m.search != "")
	title := fit(plain, textW)
	if over {
		title = warnStyle.Render(title)
	} else {
		title = titleStyle.Render(title)
	}
	lines := []string{title}
	if len(ids) == 0 {
		msg := "c new card"
		if len(col.Cards) > 0 {
			msg = "no matches"
		}
		lines = append(lines, mutedStyle.Render(msg))
	}
	for i := start; i < len(ids); i++ {
		block := ensure(i)
		if i > start && len(lines)-1+len(block) > cardRoom {
			break // next card doesn't fit whole, skip it
		}
		for _, line := range block {
			if len(lines)-1 >= cardRoom {
				break
			}
			lines = append(lines, line)
		}
	}
	st := boxStyle
	switch {
	case ci == m.col:
		st = boxOnStyle
	case over:
		st = boxHotStyle
	case col.Archive:
		st = boxArchiveStyle
	}
	return st.Width(innerW).Height(innerH).Render(strings.Join(lines, "\n"))
}

func (m *model) dialog() string {
	switch m.top() {
	case layerHelp:
		return m.helpDialog()
	case layerInput:
		return m.inputDialog()
	case layerConfirm:
		return boxOnStyle.Render(m.confirmMsg + "\n\n" + mutedStyle.Render("y confirm    n cancel"))
	case layerCard:
		return m.cardDialog()
	case layerChecklist:
		return m.checkDialog()
	case layerSubtasks:
		return m.subListDialog()
	case layerSubtask:
		return m.subDialog()
	case layerMove:
		return m.moveDialog()
	case layerTheme:
		return m.themeDialog()
	case layerTag:
		return m.tagDialog()
	default:
		return ""
	}
}

func (m *model) moveDialog() string {
	b := m.curBoard()
	if b == nil {
		return ""
	}
	w := max(24, min(48, m.width*2/5))
	textW := max(8, w-4)
	room := max(1, min(len(b.Columns), m.height-8))
	off := 0
	if m.moveSel >= room {
		off = m.moveSel - room + 1
	}
	var lines []string
	for i := off; i < len(b.Columns) && i < off+room; i++ {
		row := colHeadPlain(b.Columns[i])
		if i == m.col {
			row += " ·"
		}
		row = fit(row, textW)
		if i == m.moveSel {
			row = selStyle.Width(textW).Render(row)
		}
		lines = append(lines, row)
	}
	body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render("enter move   q cancel")
	return panelBox(" Move to ", body, w, room+4, true)
}

func (m *model) helpDialog() string {
	h := max(14, m.height-2)
	w := max(48, min(92, m.width-2))
	textW := max(20, w-6)
	end := min(len(helpLines), m.helpOff+h-4)
	var lines []string
	for _, line := range helpLines[m.helpOff:end] {
		lines = append(lines, fit(line, textW))
	}
	body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render("j/k scroll    q/esc close")
	return panelBox(" Help ", body, w, h, true)
}

func (m *model) inputDialog() string {
	if m.promptKind == inputNewColumn || m.promptKind == inputRenameColumn {
		return m.columnDialog()
	}
	w := max(24, min(48, m.width*2/5))
	textW := max(8, w-4)
	body := m.promptTitle + "\n" + m.prompt.render(textW) + "\n" + modeLine(m.prompt.insert, "i insert   enter save   esc cancel", textW)
	return boxOnStyle.Width(w).Render(body)
}

func (m *model) columnDialog() string {
	inner := max(28, min(52, m.width-8))
	textW := max(12, inner-2)
	title := "New column"
	if m.promptKind == inputRenameColumn {
		title = "Rename column"
	}
	body := title + "\n\n" + m.prompt.render(textW) + "\n\n" +
		modeLine(m.prompt.insert, "i insert    enter save    esc cancel", textW)
	return boxOnStyle.Width(inner).Render(body)
}

func (m *model) scrollShown(ci, n, selPos, room int, count func(int) int) int {
	if selPos < 0 || n == 0 || room < 1 {
		return 0
	}
	m.growCardSel()
	if m.cardScroll[ci] > selPos {
		m.cardScroll[ci] = selPos
	}
	if m.cardScroll[ci] < 0 {
		m.cardScroll[ci] = 0
	}
	used := 0
	for i := m.cardScroll[ci]; i <= selPos && i < n; i++ {
		used += max(1, count(i))
	}
	for used > room && m.cardScroll[ci] < selPos {
		used -= max(1, count(m.cardScroll[ci]))
		m.cardScroll[ci]++
	}
	return m.cardScroll[ci]
}

func (m *model) cardDialog() string {
	totalW := max(48, m.width-2)
	totalH := max(18, m.height-1)
	textW := max(20, totalW-8)
	summaryH := 5
	descH := max(8, totalH-summaryH-6)
	content := panelBox(" Content ", m.edit.content.render(max(1, textW-4)), textW, summaryH, m.edit.focus == 0)
	desc := panelBox(" Description ", m.edit.desc.renderWrap(max(1, textW-4), max(1, descH-3)), textW, descH, m.edit.focus == 1)
	hint := "! pri   t tags   T due   tab   c checklist   s subtasks   enter save"
	if status := cardStatus(m.edit); status != "" {
		hint = status + "   " + hint
	}
	if m.notice != "" {
		hint = m.notice
	}
	return panelBox(" Card Info ", content+"\n"+desc+"\n"+modeLine(m.activeField().insert, hint, textW), totalW, totalH, true)
}

func (m *model) subDialog() string {
	totalW := max(48, m.width-2)
	totalH := max(18, m.height-1)
	textW := max(20, totalW-8)
	summaryH := 5
	descH := max(8, totalH-summaryH-6)
	content := panelBox(" Summary ", m.sub.content.render(max(1, textW-4)), textW, summaryH, m.sub.focus == 0)
	desc := panelBox(" Description ", m.sub.desc.renderWrap(max(1, textW-4), max(1, descH-3)), textW, descH, m.sub.focus == 1)
	state := "open"
	if m.sub.done {
		state = "done"
	}
	hint := "tab field   space marks " + state + "   enter save   esc cancel"
	if m.notice != "" {
		hint = m.notice
	}
	title := " Subtask "
	if parent := strings.TrimSpace(m.edit.content.value()); parent != "" {
		title = " Subtask · " + parent + " "
	}
	return panelBox(title, content+"\n"+desc+"\n"+modeLine(m.subField().insert, hint, textW), totalW, totalH, true)
}

func (m *model) subListDialog() string {
	totalW := max(48, m.width-2)
	totalH := max(16, m.height-1)
	textW := max(20, totalW-6)
	var lines []string
	parent := strings.TrimSpace(m.edit.content.value())
	if parent == "" {
		parent = "card"
	}
	open, done := 0, 0
	for _, sub := range m.edit.subtasks {
		if sub.Done {
			done++
		} else {
			open++
		}
	}
	lines = append(lines, mutedStyle.Render(fit(fmt.Sprintf("%s    %d open    %d done", parent, open, done), textW)))
	lines = append(lines, "")
	if len(m.edit.subtasks) == 0 {
		lines = append(lines, mutedStyle.Render("c to add a subtask"))
	}
	for i, sub := range m.edit.subtasks {
		mark := "[ ] "
		state := "OPEN"
		title := sub.Content
		if sub.Done {
			mark = "[x] "
			state = "DONE"
			title = doneStyle.Render(sub.Content)
		}
		row := fmt.Sprintf("%2d  %s%s", i+1, mark, title)
		gap := textW - lipgloss.Width(row) - len(state)
		if gap < 1 {
			row = fit(fmt.Sprintf("%2d  %s%s", i+1, mark, sub.Content), textW-len(state)-1)
			gap = 1
		}
		row += strings.Repeat(" ", gap) + state
		if i == m.subSel {
			row = selStyle.Width(textW).Render(fit(fmt.Sprintf("%2d  %s%s", i+1, mark, sub.Content), textW))
		}
		lines = append(lines, row)
		preview := strings.TrimSpace(sub.Description)
		if i := strings.IndexByte(preview, '\n'); i >= 0 {
			preview = preview[:i]
		}
		if preview != "" && i != m.subSel {
			lines = append(lines, mutedStyle.Render(fit("      "+preview, textW)))
		}
	}
	hint := "j/k select   J/K reorder   g/G ends   c new   e edit   space done   d delete   q board"
	body := strings.Join(lines, "\n") + "\n\n" + mutedStyle.Render(fit(hint, textW))
	return panelBox(" Subtasks ", body, totalW, totalH, true)
}

func (m *model) checkDialog() string {
	totalW := max(48, min(m.width-4, m.width*3/4))
	totalH := max(14, m.height*2/3)
	textW := max(16, totalW-6)
	var lines []string
	if len(m.edit.checklist) == 0 {
		lines = append(lines, mutedStyle.Render("c to add an item"))
	}
	for i, item := range m.edit.checklist {
		mark := "[ ] "
		text := item.Content
		if item.Done {
			mark = "[x] "
			text = doneStyle.Render(item.Content)
		}
		row := fit(mark+text, textW)
		if i == m.checkSel {
			row = selStyle.Width(textW).Render(fit(mark+item.Content, textW))
		}
		lines = append(lines, row)
	}
	hint := mutedStyle.Render(fit("j/k select   J/K reorder   g/G ends   c add   e edit   space toggle   d delete   q board", textW))
	return panelBox(" Checklist ", strings.Join(lines, "\n")+"\n\n"+hint, totalW, totalH, true)
}

func cardLabel(card store.Card) string {
	label := card.Content
	if n := len(card.Checklist); n > 0 {
		done := 0
		for _, item := range card.Checklist {
			if item.Done {
				done++
			}
		}
		label += fmt.Sprintf(" %d/%d", done, n)
	}
	return label
}

func cardLines(card store.Card, width int, selected, fold, picked, archive bool) []string {
	inner := max(1, width-2) // border takes 1 column on each side
	mark, markW := priMark(card.Priority)
	title := fit(cardLabel(card), max(1, inner-markW))
	switch {
	case selected:
		title = titleStyle.Render(title)
	case daysSince(card.Moved) >= staleDays && !archive:
		title = staleStyle.Render(title)
	}
	lines := []string{mark + title}
	if meta := metaLine(card, inner, !archive); meta != "" {
		lines = append(lines, meta)
	}

	if selected {
		if preview := firstLine(card.Description); preview != "" {
			lines = append(lines, mutedStyle.Render(fit(preview, inner)))
		}
	}

	if fold && !selected && len(card.Subtasks) > 0 {
		lines = append(lines, fit(fmt.Sprintf("  %d subtasks", len(card.Subtasks)), inner))
	} else {
		for _, sub := range card.Subtasks {
			mark := "  ▸ "
			if sub.Done {
				mark = "  ✓ "
			}
			lines = append(lines, fit(mark+sub.Content, inner))
		}
	}

	st := cardStyle
	switch {
	case selected:
		st = cardOnStyle
	case picked:
		st = cardPickStyle
	case daysSince(card.Moved) >= staleDays && !archive:
		st = cardStaleStyle
	}
	return strings.Split(st.Width(inner).Render(strings.Join(lines, "\n")), "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func cardMatches(card *store.Card, term string) bool {
	if term == "" {
		return false
	}
	fold := term == strings.ToLower(term)
	hit := func(s string) bool {
		if fold {
			return strings.Contains(strings.ToLower(s), term)
		}
		return strings.Contains(s, term)
	}
	if hit(card.Content) || hit(card.Description) {
		return true
	}
	for i := range card.Checklist {
		if hit(card.Checklist[i].Content) {
			return true
		}
	}
	for i := range card.Subtasks {
		if hit(card.Subtasks[i].Content) {
			return true
		}
	}
	for _, tag := range card.Tags {
		if hit("#"+tag) || hit(tag) {
			return true
		}
	}
	return card.Due != "" && hit(card.Due)
}

func panelBox(title, body string, totalW, totalH int, on bool) string {
	st := boxStyle
	if on {
		st = boxOnStyle
	}
	contentW := max(10, totalW-2)
	contentH := max(3, totalH-2)
	textW := max(1, contentW-2)
	lines := []string{fit(title, textW)}
	for _, line := range strings.Split(body, "\n") {
		if len(lines) >= contentH {
			break
		}
		lines = append(lines, clampLine(line, textW))
	}
	for len(lines) < contentH {
		lines = append(lines, "")
	}
	return st.Width(contentW).Height(contentH).Render(strings.Join(lines, "\n"))
}

func panel(title, body string, total int, on bool) string {
	st := boxStyle
	if on {
		st = boxOnStyle
	}
	contentW := max(8, total-2)
	textW := max(1, contentW-2)
	lines := []string{fit(title, textW)}
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, clampLine(line, textW))
	}
	return st.Width(contentW).Render(strings.Join(lines, "\n"))
}

func clampLine(line string, n int) string {
	if lipgloss.Width(line) <= n {
		return line
	}
	return fit(line, n)
}

func cloneTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneItems(in []store.ChecklistItem) []store.ChecklistItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.ChecklistItem, len(in))
	copy(out, in)
	return out
}

func cloneSubs(in []store.Subtask) []store.Subtask {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Subtask, len(in))
	copy(out, in)
	return out
}

func moveSub(items []store.Subtask, i, dir int) int {
	j := i + dir
	if i < 0 || j < 0 || i >= len(items) || j >= len(items) {
		return i
	}
	items[i], items[j] = items[j], items[i]
	return j
}

func moveItem(items []store.ChecklistItem, i, dir int) int {
	j := i + dir
	if i < 0 || j < 0 || i >= len(items) || j >= len(items) {
		return i
	}
	items[i], items[j] = items[j], items[i]
	return j
}

func shortName(s string) string {
	r := []rune(s)
	if len(r) > 11 {
		return string(r[:9]) + ".."
	}
	return s
}

var (
	barStyle        lipgloss.Style
	modeStyle       lipgloss.Style
	titleStyle      = lipgloss.NewStyle().Bold(true)
	selStyle        lipgloss.Style
	mutedStyle      = lipgloss.NewStyle().Faint(true)
	doneStyle       = lipgloss.NewStyle().Faint(true).Strikethrough(true)
	warnStyle       lipgloss.Style
	priStyle        lipgloss.Style
	priHotStyle     lipgloss.Style
	dueStyle        lipgloss.Style
	dueHotStyle     lipgloss.Style
	tagStyle        lipgloss.Style
	staleStyle      lipgloss.Style
	boxStyle        lipgloss.Style
	boxOnStyle      lipgloss.Style
	boxHotStyle     lipgloss.Style
	boxArchiveStyle lipgloss.Style
	cardStyle       lipgloss.Style
	cardOnStyle     lipgloss.Style
	cardPickStyle   lipgloss.Style
	cardStaleStyle  lipgloss.Style
)

var helpLines = []string{
	"Help: j/k scroll, q or esc close",
	"",
	"Main menu",
	"  q                     quit",
	"  ?                     help",
	"  j/k                   move highlight",
	"  g/G                   first/last",
	"  K/J, shift+up/down    reorder the highlighted board",
	"  c                     create",
	"  r/e                   rename",
	"  d                     delete",
	"  enter                 open",
	"  u / ctrl+r            undo / redo",
	"  ctrl+t                color theme",
	"",
	"Input, normal",
	"  h/l                   move",
	"  0/$                   line start/end",
	"  i/a/I/A               insert",
	"  S                     clear line",
	"  d                     delete line",
	"  enter                 save",
	"  esc/q                 cancel",
	"",
	"Input, insert",
	"  esc                   normal mode",
	"  enter                 save or newline",
	"  backspace             delete character",
	"  ctrl+backspace        delete the word before the cursor",
	"  alt+backspace         same (option+backspace on macOS)",
	"",
	"Confirm",
	"  y/enter               yes",
	"  n/esc                 no",
	"",
	"Board",
	"  q                     back",
	"  ?                     help",
	"  h/l                   column",
	"  [ ]                   previous/next board",
	"  j/k                   card",
	"  g/G                   first/last card",
	"  H/L                   move card across",
	"  m                     move card to a column",
	"  space                 select for bulk move, delete, tag, due",
	"  esc                   clear the selection",
	"  w                     set the column WIP limit (0 clears)",
	"  W                     toggle the finished-checklist rule",
	"  A                     archive the card (keeps it off the working board)",
	"  v                     hide or show archive columns",
	"  !                     cycle priority: none, !, !!",
	"  t                     filter tags from the config, space toggles, ctrl+d deletes",
	"  T                     due date YYYY-MM-DD, or +days to snooze",
	"  o                     open the first http(s) link on the card",
	"  X                     clear finished checklist items",
	"  K/J, shift+up/down    move focused card up/down",
	"  ctrl+h/ctrl+p         move column left",
	"  ctrl+l/ctrl+n         move column right",
	"  C                     new column",
	"  E                     rename column",
	"  D                     delete column",
	"  c                     new card",
	"  e/enter               edit card",
	"  s                     subtasks",
	"  d                     delete card",
	"  y / p                 yank / paste card",
	"  Y                     duplicate card",
	"  f                     filter cards by the last search",
	"  z                     fold subtasks on unfocused cards",
	"  u / ctrl+r            undo / redo",
	"  ctrl+t                color theme",
	"  /                     search title, description, checklist, subtasks",
	"  n/N                   next/previous match",
	"  5j, 3L, ...           count prefix for movement keys",
	"",
	"Move to column",
	"  j/k                   select",
	"  g/G                   first/last",
	"  enter                 move the card",
	"  q/esc                 cancel",
	"",
	"Card info (fills the screen)",
	"  description           wraps instead of cutting the line",
	"  tab                   switch content/description",
	"  i                     insert",
	"  ! / t / T             priority, tags, due date",
	"  tags                  type to filter, space toggles, enter adds a new tag when nothing matches",
	"  enter                 save",
	"  q                     return to the board",
	"  c                     checklist",
	"  s                     subtasks",
	"",
	"Subtasks",
	"  j/k                   select",
	"  J/K                   reorder",
	"  g/G                   first/last",
	"  c                     new",
	"  e/enter               edit",
	"  space                 done",
	"  d                     delete",
	"  q                     save and return to the board",
	"",
	"Subtask edit",
	"  tab                   switch fields",
	"  space                 done",
	"  enter                 save",
	"  esc                   cancel",
	"",
	"Color theme",
	"  j/k                   select",
	"  enter                 apply",
	"  c                     new theme (saved in the config file)",
	"  e                     edit colors (0-255 or #rrggbb)",
	"  d                     delete a theme you added",
	"  q                     cancel",
	"",
	"Config  ~/.config/kabmat/config",
	"  missing file          built-in theme, tags, and keys are used",
	"  theme                 linux-dex or catppuccin-mocha, or [theme.name]",
	"  [tags]                the tag list. Omit the section to keep the defaults",
	"  [keys.menu]           menu bindings, one action then its keys",
	"  [keys.board]          board bindings, same shape",
	"",
	"Checklist",
	"  j/k                   select",
	"  J/K                   reorder",
	"  g/G                   first/last",
	"  c                     add",
	"  e                     edit",
	"  space                 toggle",
	"  d                     delete",
	"  q                     save and return to the board",
}

func (m *model) normalKey(key string) {
	if len(key) == 1 && (key[0] >= '1' && key[0] <= '9' || key[0] == '0' && m.count > 0) {
		m.count = min(m.count*10+int(key[0]-'0'), 999)
		return
	}
	n := 1
	ctx := "menu"
	if m.screen == screenBoard {
		ctx = "board"
	}
	if m.cfg.Repeatable(ctx, key) && m.count > 0 {
		n = m.count
	}
	m.count = 0
	for i := 0; i < n; i++ {
		if m.screen == screenMenu {
			m.menuKey(key)
		} else {
			m.boardKey(key)
		}
		if m.top() != layerNone || m.quitting {
			break
		}
	}
}

func (m *model) undoRedo(redo bool) {
	var ok bool
	var err error
	word := "undo"
	if redo {
		word = "redo"
		ok, err = m.store.Redo()
	} else {
		ok, err = m.store.Undo()
	}
	switch {
	case err != nil:
		m.notice = err.Error()
	case !ok:
		m.notice = "nothing to " + word
	default:
		m.notice = word + " done"
		if m.screen == screenBoard && m.curBoard() == nil {
			m.screen = screenMenu
		}
	}
}

func (m *model) position() string {
	if m.screen != screenBoard {
		return ""
	}
	b := m.curBoard()
	if b == nil || len(b.Columns) == 0 {
		return ""
	}
	ids := m.shownCols()
	pos := 1
	for i, id := range ids {
		if id == m.col {
			pos = i + 1
			break
		}
	}
	total := len(ids)
	if total == 0 {
		total = len(b.Columns)
		pos = m.col + 1
	}
	s := fmt.Sprintf("col %d/%d", pos, total)
	if ci := m.cardIndex(m.col); ci >= 0 {
		ids := m.shownIDs(m.col)
		pos := 0
		for i, id := range ids {
			if id == ci {
				pos = i
				break
			}
		}
		s += fmt.Sprintf("  card %d/%d", pos+1, len(ids))
	}
	return s + " "
}

func (m *model) startSearch() {
	m.count = 0
	m.searching = true
	m.searchMiss = false
	m.searchBuf = newField("", false)
	m.searchBuf.insert = true
	m.searchCol = m.col
	m.cardIndex(m.col) // normalizes cardSel/cardScroll
	m.searchSel = append(m.searchSel[:0], m.cardSel...)
}

func (m *model) restoreSearchStart() {
	m.col = m.searchCol
	m.growCardSel()
	copy(m.cardSel, m.searchSel)
}

func (m *model) searchKey(key string, runes []rune) {
	switch key {
	case "esc":
		m.searching = false
		m.restoreSearchStart()
		return
	case "enter":
		m.searching = false
		term := m.searchBuf.value()
		if term == "" {
			m.restoreSearchStart()
			m.findNext(1)
			return
		}
		m.search = term
		if m.searchMiss {
			m.restoreSearchStart()
			m.notice = "pattern not found: " + term
		}
		return
	case "ctrl+u":
		m.searchBuf = newField("", false)
		m.searchBuf.insert = true
	case "backspace", "delete":
		if m.searchBuf.value() == "" {
			m.searching = false
			m.restoreSearchStart()
			return
		}
		m.searchBuf.handle(key, runes)
	default:
		before := m.searchBuf.value()
		m.searchBuf.handle(key, runes)
		if m.searchBuf.value() == before {
			return
		}
	}
	m.incSearch()
}

// incSearch re-runs the search from where the prompt was opened.
func (m *model) incSearch() {
	m.restoreSearchStart()
	m.searchMiss = false
	term := m.searchBuf.value()
	if term == "" {
		return
	}
	m.searchMiss = !m.findFrom(term, m.searchCol, m.cardIndex(m.searchCol), 1, true)
}

// findNext repeats the last confirmed search (n / N).
func (m *model) findNext(dir int) {
	if m.curBoard() == nil {
		return
	}
	if m.search == "" {
		m.notice = "no search term, press / first"
		return
	}
	if !m.findFrom(m.search, m.col, m.cardIndex(m.col), dir, false) {
		m.notice = "pattern not found: " + m.search
	}
}

// findFrom looks for term starting at card (c0, i0), wrapping around the
// board. It focuses the match and reports whether one was found.
// includeStart lets the starting card itself match.
func (m *model) findFrom(term string, c0, i0, dir int, includeStart bool) bool {
	b := m.curBoard()
	if b == nil || term == "" {
		return false
	}
	total, cur := 0, -1
	for c := range b.Columns {
		if c == c0 && i0 >= 0 {
			cur = total + i0
		}
		total += len(b.Columns[c].Cards)
	}
	if total == 0 {
		return false
	}

	start := 1
	if cur < 0 {
		if dir < 0 {
			cur = 0
		}
	} else if includeStart {
		start = 0
	}

	at := func(lin int) (int, int) {
		for c := range b.Columns {
			n := len(b.Columns[c].Cards)
			if lin < n {
				return c, lin
			}
			lin -= n
		}
		return 0, 0
	}

	for k := start; k <= total; k++ {
		c, i := at(((cur+dir*k)%total + total) % total)
		if cardMatches(&b.Columns[c].Cards[i], term) {
			m.col = c
			m.growCardSel()
			m.cardSel[c] = i
			return true
		}
	}
	return false
}

func (m *model) searchBar() string {
	text := []rune(m.searchBuf.value())
	right := ""
	if m.searchMiss {
		right = " pattern not found "
	}
	room := max(1, m.width-lipgloss.Width(right)-2)
	if len(text) > room {
		text = text[len(text)-room:]
	}
	left := "/" + string(text)
	pad := max(0, m.width-lipgloss.Width(left)-1-lipgloss.Width(right))
	return barStyle.Render(left) + modeStyle.Render(" ") +
		barStyle.Render(strings.Repeat(" ", pad)+right)
}

func columnHeading(col store.Column, shown int, filtering bool) (string, bool) {
	name := col.Title
	if col.Gate {
		name += "*"
	}
	n := len(col.Cards)
	over := col.WIP > 0 && n > col.WIP
	switch {
	case filtering:
		return fmt.Sprintf("%s %d/%d", name, shown, n), over
	case col.WIP > 0:
		return fmt.Sprintf("%s %d/%d", name, n, col.WIP), over
	default:
		return fmt.Sprintf("%s %d", name, n), false
	}
}

func colHeadPlain(col store.Column) string {
	s, over := columnHeading(col, len(col.Cards), false)
	if over {
		return s + "!"
	}
	return s
}

func (m *model) isPicked(col, card int) bool {
	_, ok := m.picked[[2]int{col, card}]
	return ok
}

func (m *model) togglePick() {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	if m.picked == nil {
		m.picked = map[[2]int]struct{}{}
	}
	k := [2]int{m.col, ci}
	if _, ok := m.picked[k]; ok {
		delete(m.picked, k)
		return
	}
	m.picked[k] = struct{}{}
}

func (m *model) targets() [][2]int {
	b := m.curBoard()
	if b == nil {
		return nil
	}
	if len(m.picked) == 0 {
		ci := m.cardIndex(m.col)
		if ci < 0 {
			return nil
		}
		return [][2]int{{m.col, ci}}
	}
	out := make([][2]int, 0, len(m.picked))
	for p := range m.picked {
		if p[0] >= 0 && p[0] < len(b.Columns) && p[1] >= 0 && p[1] < len(b.Columns[p[0]].Cards) {
			out = append(out, p)
		}
	}
	return out
}

func (m *model) movePicks(dir int) [][2]int {
	picks := m.targets()
	for i := 1; i < len(picks); i++ {
		j := i
		for j > 0 && pickBefore(picks[j], picks[j-1], dir) {
			picks[j], picks[j-1] = picks[j-1], picks[j]
			j--
		}
	}
	return picks
}

func pickBefore(a, b [2]int, dir int) bool {
	if a[0] != b[0] {
		if dir < 0 {
			return a[0] < b[0]
		}
		return a[0] > b[0]
	}
	return a[1] > b[1]
}

func (m *model) confirmDelete() {
	t := m.targets()
	if len(t) == 0 {
		return
	}
	m.confirmKind = confirmDeleteCards
	if len(t) == 1 {
		card := m.curBoard().Columns[t[0][0]].Cards[t[0][1]]
		m.confirmMsg = fmt.Sprintf("Delete card %q?", shortName(card.Content))
	} else {
		m.confirmMsg = fmt.Sprintf("Delete %d cards?", len(t))
	}
	m.push(layerConfirm)
}

func (m *model) toggleGate() {
	b := m.curBoard()
	if b == nil || m.col < 0 || m.col >= len(b.Columns) {
		return
	}
	on, err := m.store.ToggleGate(m.board, m.col)
	if err != nil {
		m.notice = err.Error()
		return
	}
	if on {
		m.notice = "checklist required to enter " + b.Columns[m.col].Title
	} else {
		m.notice = "entry rule cleared"
	}
}

func (m *model) setWIP(value string) error {
	value = strings.TrimSpace(value)
	n := 0
	if value != "" {
		var err error
		n, err = strconv.Atoi(value)
		if err != nil || n < 0 {
			return errors.New("WIP limit is a number, 0 clears")
		}
	}
	if err := m.store.SetWIP(m.board, m.col, n); err != nil {
		return err
	}
	if n == 0 {
		m.notice = "WIP limit cleared"
	} else {
		m.notice = fmt.Sprintf("WIP limit %d", n)
	}
	return nil
}

func (m *model) archiveTargets() {
	picks := m.movePicks(1)
	if len(picks) == 0 {
		return
	}
	var blocked error
	dst, n := -1, 0
	for _, p := range picks {
		d, err := m.store.ArchiveCard(m.board, p[0], p[1])
		if err != nil {
			if errors.Is(err, store.ErrArchived) {
				continue
			}
			blocked = err
			break
		}
		dst, n = d, n+1
	}
	m.picked = nil
	if blocked != nil {
		m.notice = blocked.Error()
	}
	if n == 0 {
		if m.notice == "" {
			m.notice = "already archived"
		}
		return
	}
	if dst >= 0 && !m.hideArchive {
		m.col = dst
		m.growCardSel()
		if m.store.CardAtBottom {
			m.cardSel[dst] = len(m.curBoard().Columns[dst].Cards) - 1
		} else {
			m.cardSel[dst] = 0
		}
	}
	if m.notice == "" {
		m.notice = "archived"
	}
}

func (m *model) cyclePriority() {
	t := m.targets()
	if len(t) == 0 {
		return
	}
	if err := m.store.ApplyCards(m.board, t, func(c *store.Card) {
		c.Priority = (c.Priority + 1) % 3
	}); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) openTags() {
	cur := m.currentTags()
	m.tagList = m.tagChoices(cur)
	m.tagOn = map[string]bool{}
	for _, tag := range cur {
		m.tagOn[tag] = true
	}
	m.tagFilter = ""
	m.tagSel = 0
	for i, tag := range m.tagList {
		if m.tagOn[tag] {
			m.tagSel = i
			break
		}
	}
	m.push(layerTag)
}

func (m *model) currentTags() []string {
	if m.top() == layerCard {
		return cloneTags(m.edit.tags)
	}
	if len(m.picked) == 0 {
		if t := m.targets(); len(t) == 1 {
			return cloneTags(m.curBoard().Columns[t[0][0]].Cards[t[0][1]].Tags)
		}
	}
	return nil
}

func (m *model) tagChoices(selected []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(tag string) {
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		out = append(out, tag)
	}
	for _, tag := range m.cfg.Tags {
		add(tag)
	}
	if m.store != nil {
		for i := range m.store.Boards {
			for j := range m.store.Boards[i].Columns {
				for k := range m.store.Boards[i].Columns[j].Cards {
					for _, tag := range m.store.Boards[i].Columns[j].Cards[k].Tags {
						add(tag)
					}
				}
			}
		}
	}
	for _, tag := range selected {
		add(tag)
	}
	return out
}

func (m *model) visibleTags() []int {
	q := strings.ToLower(m.tagFilter)
	var ids []int
	for i, tag := range m.tagList {
		if q == "" || strings.Contains(tag, q) {
			ids = append(ids, i)
		}
	}
	return ids
}

func (m *model) snapTagSel() {
	ids := m.visibleTags()
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		if id == m.tagSel {
			return
		}
	}
	m.tagSel = ids[0]
}

func (m *model) moveTag(dir int) {
	ids := m.visibleTags()
	if len(ids) == 0 {
		return
	}
	pos := 0
	for i, id := range ids {
		if id == m.tagSel {
			pos = i
			break
		}
	}
	next := pos + dir
	if next < 0 || next >= len(ids) {
		return
	}
	m.tagSel = ids[next]
}

func (m *model) tagKey(key string) {
	switch key {
	case "q", "esc":
		m.pop()
	case "up", "ctrl+p":
		m.moveTag(-1)
	case "down", "ctrl+n":
		m.moveTag(1)
	case "backspace":
		r := []rune(m.tagFilter)
		if len(r) > 0 {
			m.tagFilter = string(r[:len(r)-1])
			m.snapTagSel()
		}
	case "ctrl+u":
		m.tagFilter = ""
		m.snapTagSel()
	case "ctrl+d":
		m.deleteAddedTag()
	case " ", "space":
		ids := m.visibleTags()
		if len(ids) == 0 {
			if m.tagFilter != "" {
				m.addTypedTag(m.tagFilter)
				m.tagFilter = ""
			}
			return
		}
		m.snapTagSel()
		tag := m.tagList[m.tagSel]
		if m.tagOn == nil {
			m.tagOn = map[string]bool{}
		}
		m.tagOn[tag] = !m.tagOn[tag]
	case "enter":
		if m.tagFilter != "" && len(m.visibleTags()) == 0 {
			m.addTypedTag(m.tagFilter)
		}
		m.applyPickedTags()
		m.pop()
	default:
		if r, ok := tagFilterRune(key); ok {
			m.tagFilter += string(r)
			m.snapTagSel()
		}
	}
}

func tagFilterRune(key string) (rune, bool) {
	r := []rune(key)
	if len(r) != 1 {
		return 0, false
	}
	c := r[0]
	switch {
	case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return c, true
	case c >= 'A' && c <= 'Z':
		return c - 'A' + 'a', true
	case c == '/' || c == '-' || c == '_' || c == '.':
		return c, true
	default:
		return 0, false
	}
}

func (m *model) deleteAddedTag() {
	if len(m.tagList) == 0 {
		return
	}
	m.snapTagSel()
	if m.tagSel < 0 || m.tagSel >= len(m.tagList) {
		return
	}
	tag := m.tagList[m.tagSel]
	m.tagList = append(m.tagList[:m.tagSel], m.tagList[m.tagSel+1:]...)
	delete(m.tagOn, tag)
	if m.tagSel >= len(m.tagList) && m.tagSel > 0 {
		m.tagSel--
	}
	var tags []string
	for _, have := range m.cfg.Tags {
		if have != tag {
			tags = append(tags, have)
		}
	}
	m.cfg.Tags = tags
	m.tagFilter = ""
	m.stripTag(tag)
	if err := m.saveConfig(); err != nil {
		m.notice = err.Error()
		return
	}
	m.notice = "deleted #" + tag
}

func (m *model) stripTag(tag string) {
	var kept []string
	for _, have := range m.edit.tags {
		if have != tag {
			kept = append(kept, have)
		}
	}
	m.edit.tags = kept
	if m.store == nil {
		return
	}
	for bi := range m.store.Boards {
		var picks [][2]int
		cols := m.store.Boards[bi].Columns
		for ci := range cols {
			for ki := range cols[ci].Cards {
				for _, have := range cols[ci].Cards[ki].Tags {
					if have == tag {
						picks = append(picks, [2]int{ci, ki})
						break
					}
				}
			}
		}
		if len(picks) == 0 {
			continue
		}
		if err := m.store.ApplyCards(bi, picks, func(c *store.Card) {
			next := make([]string, 0, len(c.Tags))
			for _, have := range c.Tags {
				if have != tag {
					next = append(next, have)
				}
			}
			c.Tags = next
		}); err != nil {
			m.notice = err.Error()
		}
	}
}

func (m *model) addTypedTag(value string) {
	added := store.ParseTags(value)
	if len(added) == 0 {
		m.notice = "tag is empty"
		return
	}
	if m.tagOn == nil {
		m.tagOn = map[string]bool{}
	}
	for _, tag := range added {
		at := -1
		for i, have := range m.tagList {
			if have == tag {
				at = i
				break
			}
		}
		if at < 0 {
			m.tagList = append(m.tagList, tag)
			at = len(m.tagList) - 1
		}
		m.tagOn[tag] = true
		m.tagSel = at
	}
}

func (m *model) applyPickedTags() {
	var tags []string
	for _, tag := range m.tagList {
		if m.tagOn[tag] {
			tags = append(tags, tag)
		}
	}
	fromCard := len(m.stack) >= 2 && m.stack[len(m.stack)-2] == layerCard
	if fromCard {
		m.edit.tags = tags
		return
	}
	if err := m.store.ApplyCards(m.board, m.targets(), func(c *store.Card) {
		c.Tags = append([]string(nil), tags...)
	}); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) tagDialog() string {
	w := max(36, min(52, m.width-4))
	textW := max(16, w-6)
	ids := m.visibleTags()
	room := len(ids)
	if room < 1 {
		room = 1
	}
	if maxRoom := m.height - 9; maxRoom > 0 && room > maxRoom {
		room = maxRoom
	}
	pos := 0
	for i, id := range ids {
		if id == m.tagSel {
			pos = i
			break
		}
	}
	off := 0
	if pos >= room {
		off = pos - room + 1
	}
	query := m.tagFilter
	if query == "" {
		query = ""
	}
	lines := []string{fit("/"+m.tagFilter, textW)}
	if len(ids) == 0 {
		msg := "no match"
		if m.tagFilter != "" {
			msg = "enter adds #" + m.tagFilter
		}
		lines = append(lines, mutedStyle.Render(msg))
	}
	for _, id := range ids[off:min(len(ids), off+room)] {
		tag := m.tagList[id]
		mark := "[ ] "
		if m.tagOn[tag] {
			mark = "[x] "
		}
		row := fit(mark+"#"+tag, textW)
		if id == m.tagSel {
			row = selStyle.Width(textW).Render(fit(mark+"#"+tag, textW))
		}
		lines = append(lines, row)
	}
	n := 0
	for _, tag := range m.tagList {
		if m.tagOn[tag] {
			n++
		}
	}
	hint := fmt.Sprintf("%d selected   type filters   space toggle   ctrl+d delete   enter save   esc cancel", n)
	body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render(fit(hint, textW))
	return panelBox(" Tags ", body, w, room+6, true)
}

func (m *model) openDue() {
	cur := ""
	if m.top() == layerCard {
		cur = m.edit.due
	} else if len(m.picked) == 0 {
		if t := m.targets(); len(t) == 1 {
			cur = m.curBoard().Columns[t[0][0]].Cards[t[0][1]].Due
		}
	}
	m.openInput(inputDue, " Due (YYYY-MM-DD or +days) ", cur)
}

func (m *model) openLink() {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	card := m.curBoard().Columns[m.col].Cards[ci]
	url := firstURL(card.Content, card.Description)
	if url == "" {
		m.notice = "no link on this card"
		return
	}
	if err := openBrowser(url); err != nil {
		m.notice = err.Error()
		return
	}
	m.notice = "opened " + url
}

func firstURL(parts ...string) string {
	for _, s := range parts {
		for _, f := range strings.Fields(s) {
			if strings.HasPrefix(f, "https://") || strings.HasPrefix(f, "http://") {
				return strings.TrimRight(f, ".,);]>\"'")
			}
		}
	}
	return ""
}

func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	return exec.Command(name, args...).Start()
}

func priMark(p int) (string, int) {
	switch p {
	case 1:
		return priStyle.Render("!") + " ", 2
	case 2:
		return priHotStyle.Render("!!") + " ", 3
	default:
		return "", 0
	}
}

func metaLine(card store.Card, width int, showAge bool) string {
	var plain []string
	var styled []string
	for _, tag := range card.Tags {
		p := "#" + tag
		plain = append(plain, p)
		styled = append(styled, tagStyle.Render(p))
	}
	if card.Due != "" {
		p := card.Due
		if len(p) == 10 {
			p = p[5:]
		}
		plain = append(plain, p)
		switch dueKind(card.Due) {
		case 2:
			styled = append(styled, dueHotStyle.Render(p))
		case 1:
			styled = append(styled, dueStyle.Render(p))
		default:
			styled = append(styled, mutedStyle.Render(p))
		}
	}
	if showAge {
		if d := daysSince(card.Moved); d >= staleDays {
			p := fmt.Sprintf("%dd", d)
			plain = append(plain, p)
			styled = append(styled, staleStyle.Render(p))
		}
	}
	if len(plain) == 0 {
		return ""
	}
	raw := strings.Join(plain, " ")
	if lipgloss.Width(raw) > width {
		return mutedStyle.Render(fit(raw, width))
	}
	return strings.Join(styled, " ")
}

func dueKind(due string) int {
	today := time.Now().Format("2006-01-02")
	switch {
	case due == today:
		return 1
	case due != "" && due < today:
		return 2
	default:
		return 0
	}
}

func daysSince(day string) int {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 0
	}
	now, err := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	if err != nil {
		return 0
	}
	d := int(now.Sub(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

func cardStatus(e cardEdit) string {
	var parts []string
	switch e.priority {
	case 1:
		parts = append(parts, "!")
	case 2:
		parts = append(parts, "!!")
	}
	for _, tag := range e.tags {
		parts = append(parts, "#"+tag)
	}
	if e.due != "" {
		parts = append(parts, "due "+e.due)
	}
	return strings.Join(parts, " ")
}

func parseDue(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, "+") {
		n, err := strconv.Atoi(s[1:])
		if err != nil || n < 0 {
			return "", errors.New("snooze is +days")
		}
		return time.Now().AddDate(0, 0, n).Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", errors.New("due date is YYYY-MM-DD or +days")
	}
	return s, nil
}
