package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// field is a vim-like line editor. multi allows j/k and newlines.
type field struct {
	lines  []string
	row    int
	col    int
	insert bool
	multi  bool
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

// handle reports submit/cancel for normal-mode enter and esc/q.
// Single-line enter in insert mode also submits. Multi-line insert enter splits the line.
func (f *field) handle(key string, runes []rune) (submit, cancel bool) {
	if f.insert {
		switch key {
		case "esc":
			f.insert = false
		case "enter":
			if !f.multi {
				return true, false
			}
			r := f.line()
			f.lines[f.row] = string(r[:f.col])
			rest := string(r[f.col:])
			f.lines = append(f.lines[:f.row+1], append([]string{rest}, f.lines[f.row+1:]...)...)
			f.row++
			f.col = 0
		case "backspace", "delete":
			f.backspace()
		default:
			if key == "space" {
				runes = []rune{' '}
			}
			if len(runes) > 0 && key != "tab" {
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

	switch key {
	case "esc", "q":
		return false, true
	case "enter":
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
		if f.multi {
			f.row = 0
		}
	case "G":
		if f.multi {
			f.row = len(f.lines) - 1
		}
	case "i":
		f.insert = true
	case "a":
		f.col++
		f.insert = true
	case "I":
		f.col = 0
		f.insert = true
	case "A":
		f.col = len(f.line())
		f.insert = true
	case "S":
		f.lines[f.row] = ""
		f.col = 0
		f.insert = true
	case "d":
		if f.multi && len(f.lines) > 1 {
			f.lines = append(f.lines[:f.row], f.lines[f.row+1:]...)
			if f.row >= len(f.lines) {
				f.row = len(f.lines) - 1
			}
		}
	}
	f.clamp()
	return false, false
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
