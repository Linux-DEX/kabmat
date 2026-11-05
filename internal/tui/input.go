package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// field is a vim-like line editor. multi allows j/k and newlines.
type field struct {
	lines     []string
	row       int
	col       int
	insert    bool
	multi     bool
	pending   byte // one-key motion prefix, "g" waits for its second key (gg, ge, gE)
	undoStack []fieldState
	redoStack []fieldState
}

// fieldState is one undo step: the editor text before a change.
type fieldState struct {
	lines []string
	row   int
	col   int
}

func newField(text string, multi bool) field {
	if text == "" {
		return field{lines: []string{""}, multi: multi}
	}
	return field{lines: strings.Split(text, "\n"), multi: multi}
}

func (f field) value() string {
	return strings.Join(f.lines, "\n")
}

func (f *field) line() []rune {
	return []rune(f.lines[f.row])
}

func (f *field) setLine(r []rune) {
	f.lines[f.row] = string(r)
}

func (f *field) clamp() {
	if f.row < 0 {
		f.row = 0
	}
	if f.row >= len(f.lines) {
		f.row = len(f.lines) - 1
	}
	n := len(f.line())
	if f.col < 0 {
		f.col = 0
	}
	if f.col > n {
		f.col = n
	}
}

// pushUndo keeps the current text so the next change can be undone.
func (f *field) pushUndo() {
	f.undoStack = append(f.undoStack, fieldState{lines: copyLines(f.lines), row: f.row, col: f.col})
}

// undo reverts the last change and remembers the current text for redo.
func (f *field) undo() bool {
	if len(f.undoStack) == 0 {
		return false
	}
	cur := fieldState{lines: copyLines(f.lines), row: f.row, col: f.col}
	last := f.undoStack[len(f.undoStack)-1]
	f.undoStack = f.undoStack[:len(f.undoStack)-1]
	f.redoStack = append(f.redoStack, cur)
	f.lines, f.row, f.col = last.lines, last.row, last.col
	f.clamp()
	return true
}

// redo re-applies the last undone change.
func (f *field) redo() bool {
	if len(f.redoStack) == 0 {
		return false
	}
	cur := fieldState{lines: copyLines(f.lines), row: f.row, col: f.col}
	last := f.redoStack[len(f.redoStack)-1]
	f.redoStack = f.redoStack[:len(f.redoStack)-1]
	f.undoStack = append(f.undoStack, cur)
	f.lines, f.row, f.col = last.lines, last.row, last.col
	f.clamp()
	return true
}

func copyLines(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// handle reports submit/cancel for normal-mode enter and esc/q.
// Single-line enter in insert mode also submits. Multi-line insert enter splits the line.
// Normal mode carries the vim base editing set: motions, word jumps, delete, open, undo.
func (f *field) handle(key string, runes []rune) (submit, cancel bool) {
	if f.insert {
		switch key {
		case "esc":
			f.insert = false
			f.pending = 0
		case "enter":
			if !f.multi {
				return true, false
			}
			f.pushUndo()
			r := f.line()
			f.lines[f.row] = string(r[:f.col])
			rest := string(r[f.col:])
			f.lines = append(f.lines[:f.row+1], append([]string{rest}, f.lines[f.row+1:]...)...)
			f.row++
			f.col = 0
		case "backspace", "delete":
			f.pushUndo()
			f.backspace()
		case "ctrl+w", "ctrl+h", "ctrl+backspace", "alt+backspace":
			// ctrl+h is what macOS Terminal and Linux xterm send for ctrl+backspace.
			// alt+backspace is Option+Backspace on macOS and Alt+Backspace on Linux.
			f.pushUndo()
			f.deleteWord()
		case "left":
			if f.col > 0 {
				f.col--
			}
		case "right":
			f.col++
		case "up":
			if f.multi && f.row > 0 {
				f.row--
			}
		case "down":
			if f.multi && f.row < len(f.lines)-1 {
				f.row++
			}
		case "home", "ctrl+home":
			f.col = 0
		case "end", "ctrl+end":
			f.col = len(f.line())
		case "ctrl+left", "alt+left":
			f.col = prevWordStart([]rune(f.line()), f.col, true)
		case "ctrl+right", "alt+right":
			f.col = nextWordStart([]rune(f.line()), f.col, true)
		default:
			if key == "space" {
				runes = []rune{' '}
			}
			if len(runes) > 0 && key != "tab" {
				f.pushUndo()
				r := f.line()
				next := make([]rune, 0, len(r)+len(runes))
				next = append(next, r[:f.col]...)
				next = append(next, runes...)
				next = append(next, r[f.col:]...)
				f.setLine(next)
				f.col += len(runes)
			}
		}
		f.clamp()
		return false, false
	}

	motion := f.pending
	f.pending = 0
	switch key {
	case "esc", "q":
		f.pending = 0
		return false, true
	case "enter":
		f.pending = 0
		return true, false
	case "h", "left":
		f.col--
	case "l", "right":
		f.col++
	case "0", "home":
		f.col = 0
	case "$", "end":
		f.col = len(f.line())
	case "k", "up":
		if f.multi {
			f.row--
		}
	case "j", "down":
		if f.multi {
			f.row++
		}
	case "g":
		if motion == 'g' {
			f.row = 0
		} else {
			f.pending = 'g'
		}
	case "G":
		f.pending = 0
		f.row = len(f.lines) - 1
	case "w":
		f.col = nextWordStart([]rune(f.line()), f.col, true)
	case "W":
		f.col = nextWordStart([]rune(f.line()), f.col, false)
	case "b":
		f.col = prevWordStart([]rune(f.line()), f.col, true)
	case "B":
		f.col = prevWordStart([]rune(f.line()), f.col, false)
	case "e":
		if motion == 'g' {
			f.col = prevWordEnd([]rune(f.line()), f.col, true)
		} else {
			f.col = nextWordEnd([]rune(f.line()), f.col, true)
		}
	case "E":
		if motion == 'g' {
			f.col = prevWordEnd([]rune(f.line()), f.col, false)
		} else {
			f.col = nextWordEnd([]rune(f.line()), f.col, false)
		}
	case "i":
		f.pending = 0
		f.insert = true
	case "a":
		f.pending = 0
		f.col++
		f.insert = true
	case "I":
		f.pending = 0
		f.col = 0
		f.insert = true
	case "A":
		f.pending = 0
		f.col = len(f.line())
		f.insert = true
	case "S":
		f.pushUndo()
		f.lines[f.row] = ""
		f.col = 0
		f.insert = true
	case "d":
		if f.multi && len(f.lines) > 1 {
			f.pushUndo()
			f.lines = append(f.lines[:f.row], f.lines[f.row+1:]...)
			if f.row >= len(f.lines) {
				f.row = len(f.lines) - 1
			}
		}
	case "x":
		f.pushUndo()
		r := f.line()
		if f.col < len(r) {
			f.setLine(append(r[:f.col], r[f.col+1:]...))
		}
	case "X":
		f.pushUndo()
		if f.col > 0 {
			r := f.line()
			f.setLine(append(r[:f.col-1], r[f.col:]...))
			f.col--
		}
	case "o":
		f.pushUndo()
		if len(f.lines) == 0 {
			f.lines = []string{""}
			f.row = 0
			f.col = 0
		}
		if !f.multi {
			f.multi = true
		}
		next := make([]string, 0, len(f.lines)+1)
		next = append(next, f.lines[:f.row+1]...)
		next = append(next, "")
		next = append(next, f.lines[f.row+1:]...)
		f.lines = next
		f.row++
		f.col = 0
	case "O":
		f.pushUndo()
		if len(f.lines) == 0 {
			f.lines = []string{""}
			f.row = 0
			f.col = 0
		}
		if !f.multi {
			f.multi = true
		}
		next := make([]string, 0, len(f.lines)+1)
		next = append(next, f.lines[:f.row]...)
		next = append(next, "")
		next = append(next, f.lines[f.row:]...)
		f.lines = next
		f.col = 0
	case "ctrl+w", "ctrl+h", "ctrl+backspace", "alt+backspace":
		f.deleteWord()
	case "u":
		f.undo()
	case "ctrl+r":
		f.redo()
	}
	f.clamp()
	return false, false
}

// isWordChar reports whether r belongs to a word at col. keyword mode counts
// letters, digits and "_" as word characters and treats punctuation as a break;
// plain mode counts any non-space rune as part of a word.
func isWordChar(r rune, keyword bool) bool {
	if unicode.IsSpace(r) {
		return false
	}
	if keyword {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	return true
}

// nextWordStart returns the column a forward word motion lands on: the start of
// the next word, skipping the rest of the current one and any separators.
func nextWordStart(r []rune, col int, keyword bool) int {
	i := col
	if i < len(r) && isWordChar(r[i], keyword) {
		for i < len(r) && isWordChar(r[i], keyword) {
			i++
		}
	}
	for i < len(r) && !isWordChar(r[i], keyword) {
		i++
	}
	return i
}

// nextWordEnd returns the column a forward end-of-word motion lands on: the end of
// the current word when standing on one, otherwise the end of the next word.
func nextWordEnd(r []rune, col int, keyword bool) int {
	i := col
	if i < len(r) && isWordChar(r[i], keyword) {
		for i < len(r) && isWordChar(r[i], keyword) {
			i++
		}
		return i
	}
	for i < len(r) && !isWordChar(r[i], keyword) {
		i++
	}
	end := i
	for end < len(r) && isWordChar(r[end], keyword) {
		end++
	}
	return end
}

// prevWordStart returns the column a backward word motion lands on: the start of
// the previous word, or the start of the current one when standing inside it.
func prevWordStart(r []rune, col int, keyword bool) int {
	i := col
	if i > 0 && isWordChar(r[i-1], keyword) {
		for i > 0 && isWordChar(r[i-1], keyword) {
			i--
		}
		return i
	}
	for i > 0 && !isWordChar(r[i-1], keyword) {
		i--
	}
	if i == 0 {
		return 0
	}
	for i > 0 && isWordChar(r[i-1], keyword) {
		i--
	}
	return i
}

// prevWordEnd returns the column a backward end-of-word motion lands on: the end of
// the current word when standing on one, otherwise the end of the previous word.
func prevWordEnd(r []rune, col int, keyword bool) int {
	if col > 0 && isWordChar(r[col-1], keyword) {
		end := col
		for end < len(r) && isWordChar(r[end], keyword) {
			end++
		}
		return end
	}
	i := col
	for i > 0 && !isWordChar(r[i-1], keyword) {
		i--
	}
	if i == 0 {
		return 0
	}
	return i
}

// deleteWord removes the word before the cursor, after eating spaces that sit against it.
func (f *field) deleteWord() {
	if f.col == 0 {
		f.backspace()
		return
	}
	r := f.line()
	i := f.col
	for i > 0 && unicode.IsSpace(r[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	next := make([]rune, 0, len(r)-(f.col-i))
	next = append(next, r[:i]...)
	next = append(next, r[f.col:]...)
	f.setLine(next)
	f.col = i
}

func (f *field) backspace() {
	if f.col > 0 {
		r := f.line()
		f.setLine(append(r[:f.col-1], r[f.col:]...))
		f.col--
		return
	}
	if !f.multi || f.row == 0 {
		return
	}
	prev := []rune(f.lines[f.row-1])
	f.col = len(prev)
	f.lines[f.row-1] = string(prev) + f.lines[f.row]
	f.lines = append(f.lines[:f.row], f.lines[f.row+1:]...)
	f.row--
}

// renderWrap soft-wraps each line and keeps the cursor row on screen.
func (f field) renderWrap(width, rows int) string {
	if width < 1 {
		width = 1
	}
	// Leave a cell for the cursor when it sits at the end of a wrapped line.
	if width > 1 {
		width--
	}
	var lines []string
	cursor := 0
	for i, line := range f.lines {
		parts := wrapRunes([]rune(line), width)
		off := 0
		for pi, part := range parts {
			end := off + len(part)
			showCursor := i == f.row && (f.col >= off && f.col < end || pi == len(parts)-1 && f.col >= end)
			if showCursor {
				cursor = len(lines)
				lines = append(lines, paintCursor(part, f.col-off))
			} else {
				lines = append(lines, string(part))
			}
			off = end
		}
	}
	if len(lines) == 0 {
		return cursorStyle.Render(" ")
	}
	return strings.Join(windowLines(lines, cursor, rows), "\n")
}

func paintCursor(part []rune, col int) string {
	if col < 0 {
		col = 0
	}
	var b strings.Builder
	if col > len(part) {
		col = len(part)
	}
	if col > 0 {
		b.WriteString(string(part[:col]))
	}
	if col < len(part) {
		b.WriteString(cursorStyle.Render(string(part[col])))
		if col+1 < len(part) {
			b.WriteString(string(part[col+1:]))
		}
	} else {
		b.WriteString(cursorStyle.Render(" "))
	}
	return b.String()
}

func windowLines(lines []string, cursor, rows int) []string {
	if rows < 1 || len(lines) <= rows {
		return lines
	}
	start := 0
	if cursor >= rows {
		start = cursor - rows + 1
	}
	if start+rows > len(lines) {
		start = len(lines) - rows
	}
	return lines[start:]
}

// wrapRunes breaks on the last space that fits. A word longer than width is cut.
func wrapRunes(r []rune, width int) [][]rune {
	if width < 1 {
		width = 1
	}
	if len(r) == 0 {
		return [][]rune{{}}
	}
	var lines [][]rune
	for len(r) > 0 {
		if lipgloss.Width(string(r)) <= width {
			lines = append(lines, r)
			break
		}
		cut := 0
		w := 0
		lastSpace := -1
		for i, ch := range r {
			cw := lipgloss.Width(string(ch))
			if cw < 1 {
				cw = 1
			}
			if w+cw > width {
				break
			}
			w += cw
			cut = i + 1
			if ch == ' ' {
				lastSpace = cut
			}
		}
		if cut == 0 {
			cut = 1
		} else if lastSpace > 0 && lastSpace < len(r) {
			cut = lastSpace
		}
		lines = append(lines, r[:cut])
		r = r[cut:]
	}
	return lines
}

func (f field) render(width int) string {
	if width < 1 {
		width = 1
	}
	var out []string
	for i, line := range f.lines {
		r := []rune(line)
		if i != f.row {
			out = append(out, fit(line, width))
			continue
		}
		start := 0
		if f.col >= width {
			start = f.col - width + 1
		}
		end := min(len(r), start+width)
		var b strings.Builder
		if f.col > start {
			b.WriteString(string(r[start:min(f.col, end)]))
		}
		if f.col < len(r) && f.col >= start && f.col < start+width {
			b.WriteString(cursorStyle.Render(string(r[f.col])))
			if f.col+1 < end {
				b.WriteString(string(r[f.col+1 : end]))
			}
		} else {
			b.WriteString(cursorStyle.Render(" "))
		}
		out = append(out, b.String())
	}
	if len(out) == 0 {
		return cursorStyle.Render(" ")
	}
	return strings.Join(out, "\n")
}

var cursorStyle = lipgloss.NewStyle().Reverse(true)

func fit(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(stripANSI(s))
	if len(r) <= n {
		return s
	}
	plain := stripANSI(s)
	pr := []rune(plain)
	if n == 1 {
		return string(pr[:1])
	}
	return string(pr[:n-1]) + "…"
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return b.String()
}
