package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kabmat/internal/store"
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
)

type inputKind int

const (
	inputNewBoard inputKind = iota
	inputRenameBoard
	inputNewColumn
	inputRenameColumn
	inputNewItem
	inputEditItem
)

type confirmKind int

const (
	confirmDeleteBoard confirmKind = iota
	confirmDeleteColumn
	confirmDeleteCard
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
	}
	if open == "" {
		return m, nil
	}
	i, err := s.Index(open)
	if err != nil {
		return nil, err
	}
	m.screen = screenBoard
	m.board = i
	m.returnToMenu = false
	return m, nil
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
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		m.notice = ""
		switch m.top() {
		case layerHelp:
			m.helpKey(msg.String())
		case layerInput:
			m.inputKey(msg.String(), msg.Runes)
		case layerConfirm:
			m.confirmKey(msg.String())
		case layerChecklist:
			m.checkKey(msg.String())
		case layerSubtasks:
			m.subListKey(msg.String())
		case layerSubtask:
			m.subKey(msg.String(), msg.Runes)
		case layerCard:
			m.cardKey(msg.String(), msg.Runes)
		default:
			if m.screen == screenMenu {
				m.menuKey(msg.String())
			} else {
				m.boardKey(msg.String())
			}
		}
	}
	if m.quitting {
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

func (m *model) menuKey(key string) {
	n := len(m.store.Boards)
	switch key {
	case "q":
		m.quitting = true
	case "?":
		m.helpOff = 0
		m.push(layerHelp)
	case "k":
		if n > 0 && m.menuSel > 0 {
			m.menuSel--
		}
	case "j":
		if n > 0 && m.menuSel < n-1 {
			m.menuSel++
		}
	case "g":
		m.menuSel = 0
	case "G":
		if n > 0 {
			m.menuSel = n - 1
		}
	case "K", "shift+up":
		m.reorderBoard(-1)
	case "J", "shift+down":
		m.reorderBoard(1)
	case "c":
		m.openInput(inputNewBoard, " New Board Name ", "")
	case "r", "e":
		if n > 0 {
			m.openInput(inputRenameBoard, " Rename Board ", m.store.Boards[m.menuSel].Name)
		}
	case "d":
		if n > 0 {
			m.confirmKind = confirmDeleteBoard
			m.confirmMsg = fmt.Sprintf("Delete board %q?", shortName(m.store.Boards[m.menuSel].Name))
			m.push(layerConfirm)
		}
	case "enter":
		if n > 0 {
			m.board = m.menuSel
			m.col = 0
			m.colScroll = 0
			m.cardSel = nil
			m.screen = screenBoard
			m.returnToMenu = true
		}
	}
}

func (m *model) boardKey(key string) {
	b := m.curBoard()
	if b == nil {
		m.quitting = true
		return
	}
	n := len(b.Columns)
	switch key {
	case "q":
		if m.returnToMenu {
			m.screen = screenMenu
			return
		}
		m.quitting = true
	case "?":
		m.helpOff = 0
		m.push(layerHelp)
	case "h":
		if m.col > 0 {
			m.col--
		}
	case "l":
		if m.col < n-1 {
			m.col++
		}
	case "k":
		m.moveCardFocus(-1)
	case "j":
		m.moveCardFocus(1)
	case "g":
		if n > 0 {
			m.growCardSel()
			m.cardSel[m.col] = 0
		}
	case "G":
		if n > 0 && len(b.Columns[m.col].Cards) > 0 {
			m.growCardSel()
			m.cardSel[m.col] = len(b.Columns[m.col].Cards) - 1
		}
	case "H":
		m.shiftCard(-1)
	case "L":
		m.shiftCard(1)
	case "K", "shift+up":
		m.swapCard(-1)
	case "J", "shift+down":
		m.swapCard(1)
	case "ctrl+h", "ctrl+p":
		m.shiftColumn(-1)
	case "ctrl+l", "ctrl+n":
		m.shiftColumn(1)
	case "C":
		m.openInput(inputNewColumn, " New Column Name ", "")
	case "E":
		if n > 0 {
			m.openInput(inputRenameColumn, " Rename Column ", b.Columns[m.col].Title)
		}
	case "D":
		if n > 0 {
			m.confirmKind = confirmDeleteColumn
			m.confirmMsg = fmt.Sprintf("Delete column %q?", shortName(b.Columns[m.col].Title))
			m.push(layerConfirm)
		}
	case "c":
		if n > 0 {
			m.edit = cardEdit{isNew: true, col: m.col, card: -1, content: newField("", false), desc: newField("", true)}
			m.push(layerCard)
		}
	case "e", "enter":
		m.openCard()
	case "s":
		if m.openCard() {
			m.push(layerSubtasks)
		}
	case "d":
		ci := m.cardIndex(m.col)
		if ci >= 0 {
			m.confirmKind = confirmDeleteCard
			m.confirmMsg = fmt.Sprintf("Delete card %q?", shortName(b.Columns[m.col].Cards[ci].Content))
			m.push(layerConfirm)
		}
	}
}

func (m *model) helpKey(key string) {
	switch key {
	case "q":
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
		if b := m.curBoard(); b != nil && m.col >= len(b.Columns) && m.col > 0 {
			m.col--
		}
	case confirmDeleteCard:
		ci := m.cardIndex(m.col)
		if ci >= 0 {
			err = m.store.DeleteCard(m.board, m.col, ci)
		}
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
		if strings.TrimSpace(card.Content) == "" {
			return
		}
	}
	m.pop()
}

func (m *model) leaveSubtasks() {
	before := len(m.stack)
	m.submitCard()
	if len(m.stack) == before {
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

func (m *model) shiftCard(dir int) {
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	if _, err := m.store.MoveCardHoriz(m.board, m.col, ci, dir); err != nil {
		m.notice = err.Error()
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
	ci := m.cardIndex(m.col)
	if ci < 0 {
		return
	}
	next := ci + dir
	n := len(m.curBoard().Columns[m.col].Cards)
	if next < 0 || next >= n {
		return
	}
	m.cardSel[m.col] = next
}

func (m *model) openInput(kind inputKind, title, value string) {
	m.promptKind = kind
	m.promptTitle = title
	m.prompt = newField(value, false)
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

func (m *model) pinCol() {
	b := m.curBoard()
	if b == nil || len(b.Columns) == 0 {
		m.col = 0
		m.colScroll = 0
		return
	}
	if m.col >= len(b.Columns) {
		m.col = len(b.Columns) - 1
	}
	if m.col < 0 {
		m.col = 0
	}
	vis := m.visibleCols()
	if m.col < m.colScroll {
		m.colScroll = m.col
	}
	if m.col >= m.colScroll+vis {
		m.colScroll = m.col - vis + 1
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
	title := " kabmat "
	if m.screen == screenBoard && m.curBoard() != nil {
		title = " " + m.curBoard().Name + " "
	}
	return barStyle.Width(m.width).Render(title)
}

func (m *model) footer() string {
	hint := "q quit   j/k select   J/K reorder   c new   enter open   ? help"
	if m.screen == screenBoard {
		hint = "q back   j/k focus   J/K move card   e edit   s subtasks   ? help"
	}
	if m.notice != "" {
		hint = m.notice
	}
	mode := ""
	editing := m.top() == layerInput || m.top() == layerCard || m.top() == layerSubtask
	inserting := m.top() == layerInput && m.prompt.insert ||
		m.top() == layerCard && m.activeField().insert ||
		m.top() == layerSubtask && m.subField().insert
	if inserting {
		mode = " INSERT "
	} else if editing {
		mode = " NORMAL "
	}
	if mode != "" {
		mode = modeStyle.Render(mode)
	}
	pad := m.width - lipgloss.Width(hint) - lipgloss.Width(mode)
	if pad < 1 {
		hint = fit(hint, max(1, m.width-lipgloss.Width(mode)-1))
		pad = 1
	}
	return barStyle.Render(hint + strings.Repeat(" ", pad) + mode)
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
			name := fit(m.store.Boards[i].Name, textW)
			if i == m.menuSel {
				name = selStyle.Width(textW).Render(name)
			}
			lines = append(lines, name)
		}
	}
	box := boxOnStyle.Width(innerW).Height(innerH).Render(" Boards \n" + strings.Join(lines, "\n") + "\n" + mutedStyle.Render(fit("j/k select   J/K reorder   enter open", textW)))
	return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *model) boardBody(h int) string {
	b := m.curBoard()
	if b == nil || len(b.Columns) == 0 {
		return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, mutedStyle.Render("C to create a new column"))
	}
	m.pinCol()
	vis := m.visibleCols()
	gap := 1
	colW := (m.width - gap*(vis-1)) / vis
	if colW < 16 {
		colW = 16
	}
	end := min(len(b.Columns), m.colScroll+vis)
	boxes := make([]string, 0, end-m.colScroll)
	for i := m.colScroll; i < end; i++ {
		boxes = append(boxes, m.columnBox(i, colW, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

func (m *model) columnBox(ci, w, h int) string {
	col := m.curBoard().Columns[ci]
	innerW := max(4, w-4)
	innerH := max(2, h-2)
	cardRoom := max(1, innerH-1)
	sel := -1
	if ci == m.col {
		sel = m.cardIndex(ci)
	}
	start := 0
	textW := max(1, innerW-2)
	lines := []string{titleStyle.Render(fit(col.Title, textW))}
	if len(col.Cards) == 0 {
		lines = append(lines, mutedStyle.Render("c new card"))
	}
	start = m.cardWindow(ci, cardRoom, textW)
	for i := start; i < len(col.Cards) && len(lines) <= cardRoom; i++ {
		for _, line := range cardLines(col.Cards[i], textW, i == sel) {
			if len(lines) > cardRoom {
				break
			}
			lines = append(lines, line)
		}
	}
	st := boxStyle
	if ci == m.col {
		st = boxOnStyle
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
	default:
		return ""
	}
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
	body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render("j/k scroll    q close")
	return panelBox(" Help ", body, w, h, true)
}

func (m *model) inputDialog() string {
	w := max(24, min(48, m.width*2/5))
	return boxOnStyle.Width(w).Render(m.promptTitle + "\n" + m.prompt.render(w) + "\n" + mutedStyle.Render("i insert   enter save   esc cancel"))
}

func (m *model) cardWindow(ci, room, textW int) int {
	b := m.curBoard()
	if b == nil || ci < 0 || ci >= len(b.Columns) || room < 1 {
		return 0
	}
	cards := b.Columns[ci].Cards
	sel := m.cardIndex(ci)
	if sel < 0 || len(cards) == 0 {
		return 0
	}
	m.growCardSel()
	if m.cardScroll[ci] > sel {
		m.cardScroll[ci] = sel
	}
	used := 0
	for i := m.cardScroll[ci]; i <= sel && i < len(cards); i++ {
		used += max(1, len(cardLines(cards[i], textW, false)))
	}
	for used > room && m.cardScroll[ci] < sel {
		used -= max(1, len(cardLines(cards[m.cardScroll[ci]], textW, false)))
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
	desc := panelBox(" Description ", m.edit.desc.render(max(1, textW-4)), textW, descH, m.edit.focus == 1)
	mode := "NORMAL"
	if m.activeField().insert {
		mode = "INSERT"
	}
	hint := mode + "   tab field   i insert   c checklist   s subtasks   enter save   q board"
	if m.notice != "" {
		hint = m.notice
	}
	return panelBox(" Card Info ", content+"\n"+desc+"\n"+mutedStyle.Render(fit(hint, textW)), totalW, totalH, true)
}

func (m *model) subDialog() string {
	totalW := max(48, m.width-2)
	totalH := max(18, m.height-1)
	textW := max(20, totalW-8)
	summaryH := 5
	descH := max(8, totalH-summaryH-6)
	content := panelBox(" Summary ", m.sub.content.render(max(1, textW-4)), textW, summaryH, m.sub.focus == 0)
	desc := panelBox(" Description ", m.sub.desc.render(max(1, textW-4)), textW, descH, m.sub.focus == 1)
	state := "open"
	if m.sub.done {
		state = "done"
	}
	mode := "NORMAL"
	if m.subField().insert {
		mode = "INSERT"
	}
	hint := mode + "   tab field   space marks " + state + "   enter save   esc cancel"
	if m.notice != "" {
		hint = m.notice
	}
	title := " Subtask "
	if parent := strings.TrimSpace(m.edit.content.value()); parent != "" {
		title = " Subtask · " + parent + " "
	}
	return panelBox(title, content+"\n"+desc+"\n"+mutedStyle.Render(fit(hint, textW)), totalW, totalH, true)
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

func cardLines(card store.Card, width int, selected bool) []string {
	title := fit(cardLabel(card), width)
	if selected {
		title = selStyle.Width(width).Render(title)
	}
	lines := []string{title}
	for _, sub := range card.Subtasks {
		mark := "  ▸ "
		if sub.Done {
			mark = "  ✓ "
		}
		lines = append(lines, fit(mark+sub.Content, width))
	}
	return lines
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
	barStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("255"))
	modeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("33"))
	titleStyle = lipgloss.NewStyle().Bold(true)
	selStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("255"))
	mutedStyle = lipgloss.NewStyle().Faint(true)
	doneStyle  = lipgloss.NewStyle().Faint(true).Strikethrough(true)
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	boxOnStyle = boxStyle.BorderForeground(lipgloss.Color("33"))
)

var helpLines = []string{
	"Help: j/k scroll, q close",
	"",
	"Main menu",
	"q quit    ? help",
	"j/k move highlight    g/G first/last",
	"K/J or shift+up/down reorder the highlighted board",
	"c create    r/e rename    d delete    enter open",
	"",
	"Input, normal: h/l move, 0/$ ends, i/a/I/A insert",
	"S clear line, d delete line, enter save, esc/q cancel",
	"Input, insert: esc normal, enter save or newline, backspace",
	"",
	"Confirm: y/enter yes, n/esc no",
	"",
	"Board",
	"q back    ? help    h/l column    j/k card",
	"g/G first/last card    H/L move card across",
	"K/J or shift+up/down move the focused card up/down",
	"ctrl+h/ctrl+p move column left",
	"ctrl+l/ctrl+n move column right",
	"C new column    E rename    D delete column",
	"c new card    e or enter edit    s subtasks    d delete card",
	"",
	"Card info fills the screen: tab switches content/description",
	"i insert    enter save    q returns to the board",
	"c checklist    s subtasks",
	"",
	"Subtasks: j/k select    J/K reorder    g/G first/last",
	"c new    e or enter edit    space done    d delete    q save and return to the board",
	"Subtask edit: tab fields    space done    enter save    esc cancel",
	"",
	"Checklist: j/k select    J/K reorder    g/G first/last",
	"c add    e edit    space toggle    d delete    q save and return to the board",
}
