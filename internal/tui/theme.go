package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette is one color theme. Empty fields keep the linux-dex colors.
type palette struct {
	ink, paper, accent, warn, gold, tag, muted, pick, card, archive string
}

type scheme struct {
	name string
	pal  palette
	user bool
}

var colorSlots = []struct{ key, label string }{
	{"accent", "accent"},
	{"ink", "ink"},
	{"paper", "paper"},
	{"warn", "warn"},
	{"gold", "gold"},
	{"tag", "tag"},
	{"muted", "muted"},
	{"pick", "pick"},
	{"card", "card"},
	{"archive", "archive"},
}

func defaultPalette() palette {
	return palette{
		ink: "#CDD6F4", paper: "#313244", accent: "#4C7CFF", warn: "#C4A7FF", gold: "#7EC8FF",
		tag: "#7DE1D0", muted: "#5C6180", pick: "#B49AFF", card: "#5C6180", archive: "#313244",
	}
}

func builtinSchemes() []scheme {
	return []scheme{
		{name: "linux-dex", pal: defaultPalette()},
		{name: "catppuccin-mocha", pal: palette{
			ink: "#CDD6F4", paper: "#313244", accent: "#89B4FA", warn: "#F38BA8", gold: "#F9E2AF",
			tag: "#94E2D5", muted: "#6C7086", pick: "#CBA6F7", card: "#45475A", archive: "#313244",
		}},
	}
}

func (p palette) fill() palette {
	d := defaultPalette()
	if p.ink == "" {
		p.ink = d.ink
	}
	if p.paper == "" {
		p.paper = d.paper
	}
	if p.accent == "" {
		p.accent = d.accent
	}
	if p.warn == "" {
		p.warn = d.warn
	}
	if p.gold == "" {
		p.gold = d.gold
	}
	if p.tag == "" {
		p.tag = d.tag
	}
	if p.muted == "" {
		p.muted = d.muted
	}
	if p.pick == "" {
		p.pick = d.pick
	}
	if p.card == "" {
		p.card = d.card
	}
	if p.archive == "" {
		p.archive = d.archive
	}
	return p
}

func (p palette) get(key string) string {
	p = p.fill()
	switch key {
	case "ink":
		return p.ink
	case "paper":
		return p.paper
	case "accent":
		return p.accent
	case "warn":
		return p.warn
	case "gold":
		return p.gold
	case "tag":
		return p.tag
	case "muted":
		return p.muted
	case "pick":
		return p.pick
	case "card":
		return p.card
	case "archive":
		return p.archive
	default:
		return ""
	}
}

func (p *palette) set(key, val string) bool {
	switch key {
	case "ink":
		p.ink = val
	case "paper":
		p.paper = val
	case "accent":
		p.accent = val
	case "warn":
		p.warn = val
	case "gold":
		p.gold = val
	case "tag":
		p.tag = val
	case "muted":
		p.muted = val
	case "pick":
		p.pick = val
	case "card":
		p.card = val
	case "archive":
		p.archive = val
	default:
		return false
	}
	return true
}

func knownColorKey(key string) bool {
	for _, s := range colorSlots {
		if s.key == key {
			return true
		}
	}
	return false
}

func validColor(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) != 3 && len(h) != 6 {
			return false
		}
		for _, c := range h {
			switch {
			case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			default:
				return false
			}
		}
		return true
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return s != "" && n <= 255
}

func applyPalette(p palette) {
	p = p.fill()
	ink := lipgloss.Color(p.ink)
	paper := lipgloss.Color(p.paper)
	accent := lipgloss.Color(p.accent)
	warn := lipgloss.Color(p.warn)
	gold := lipgloss.Color(p.gold)
	barStyle = lipgloss.NewStyle().Foreground(ink).Background(paper)
	modeStyle = lipgloss.NewStyle().Foreground(ink).Background(accent)
	selStyle = lipgloss.NewStyle().Foreground(ink).Background(paper)
	warnStyle = lipgloss.NewStyle().Foreground(warn).Bold(true)
	priStyle = lipgloss.NewStyle().Foreground(gold).Bold(true)
	priHotStyle = lipgloss.NewStyle().Foreground(warn).Bold(true)
	dueStyle = lipgloss.NewStyle().Foreground(gold)
	dueHotStyle = lipgloss.NewStyle().Foreground(warn)
	tagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.tag))
	staleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.muted))
	border := lipgloss.RoundedBorder()
	boxStyle = lipgloss.NewStyle().Border(border).Padding(0, 1)
	boxOnStyle = boxStyle.BorderForeground(accent)
	boxHotStyle = boxStyle.BorderForeground(warn)
	boxArchiveStyle = boxStyle.BorderForeground(lipgloss.Color(p.archive))
	cardStyle = lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color(p.card))
	cardOnStyle = cardStyle.BorderForeground(accent)
	cardPickStyle = cardStyle.BorderForeground(lipgloss.Color(p.pick))
	cardStaleStyle = cardStyle.BorderForeground(lipgloss.Color(p.archive))
}

func init() { applyPalette(defaultPalette()) }

func themesPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "themes")
}

func themeNamePath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "theme")
}

func parseThemes(s string) []scheme {
	var out []scheme
	var cur *scheme
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && knownColorKey(fields[0]) && validColor(fields[1]) {
			if cur != nil {
				cur.pal.set(fields[0], fields[1])
			}
			continue
		}
		out = append(out, scheme{name: line, user: true})
		cur = &out[len(out)-1]
	}
	return out
}

func mergeSchemes(user []scheme) []scheme {
	out := builtinSchemes()
	for _, u := range user {
		if u.name == "" {
			continue
		}
		u.user = true
		hit := false
		for i := range out {
			if out[i].name == u.name {
				out[i] = u
				hit = true
				break
			}
		}
		if !hit {
			out = append(out, u)
		}
	}
	return out
}

func readUserSchemes(dataPath string) []scheme {
	b, err := os.ReadFile(themesPath(dataPath))
	if err != nil {
		return nil
	}
	return parseThemes(string(b))
}

func readThemeName(dataPath string) string {
	b, err := os.ReadFile(themeNamePath(dataPath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (m *model) loadTheme() {
	m.schemes = mergeSchemes(readUserSchemes(m.store.Path))
	m.themeName = "linux-dex"
	if n := readThemeName(m.store.Path); n != "" {
		if n == "default" {
			n = "linux-dex"
		}
		for i := range m.schemes {
			if m.schemes[i].name == n {
				m.themeName = n
				m.schemeSel = i
				break
			}
		}
	}
	applyPalette(m.currentScheme().pal)
}

func (m *model) currentScheme() scheme {
	for _, s := range m.schemes {
		if s.name == m.themeName {
			return s
		}
	}
	if len(m.schemes) == 0 {
		return scheme{name: "linux-dex", pal: defaultPalette()}
	}
	return m.schemes[0]
}

func (m *model) openThemes() {
	m.schemes = mergeSchemes(readUserSchemes(m.store.Path))
	m.themeWas = m.themeName
	m.schemeEdit = false
	m.colorSel = 0
	m.schemeSel = 0
	for i := range m.schemes {
		if m.schemes[i].name == m.themeName {
			m.schemeSel = i
			break
		}
	}
	m.push(layerTheme)
}

func (m *model) themeKey(key string) {
	if m.schemeEdit {
		m.themeColorKey(key)
		return
	}
	n := len(m.schemes)
	switch key {
	case "q", "esc":
		m.themeName = m.themeWas
		applyPalette(m.currentScheme().pal)
		m.pop()
	case "k":
		if m.schemeSel > 0 {
			m.schemeSel--
			m.previewScheme()
		}
	case "j":
		if m.schemeSel < n-1 {
			m.schemeSel++
			m.previewScheme()
		}
	case "g":
		m.schemeSel = 0
		m.previewScheme()
	case "G":
		if n > 0 {
			m.schemeSel = n - 1
			m.previewScheme()
		}
	case "enter":
		m.previewScheme()
		if err := m.writeThemeName(); err != nil {
			m.notice = err.Error()
			return
		}
		m.pop()
	case "c":
		m.openInput(inputNewTheme, " Theme name ", "")
	case "e":
		if n > 0 {
			m.schemeEdit = true
			m.colorSel = 0
		}
	case "d":
		m.deleteScheme()
	}
}

func (m *model) previewScheme() {
	if m.schemeSel < 0 || m.schemeSel >= len(m.schemes) {
		return
	}
	m.themeName = m.schemes[m.schemeSel].name
	applyPalette(m.schemes[m.schemeSel].pal)
}

func (m *model) themeColorKey(key string) {
	n := len(colorSlots)
	switch key {
	case "q", "esc":
		m.schemeEdit = false
	case "k":
		if m.colorSel > 0 {
			m.colorSel--
		}
	case "j":
		if m.colorSel < n-1 {
			m.colorSel++
		}
	case "g":
		m.colorSel = 0
	case "G":
		if n > 0 {
			m.colorSel = n - 1
		}
	case "enter", "e":
		if m.schemeSel < 0 || m.schemeSel >= len(m.schemes) || m.colorSel < 0 || m.colorSel >= n {
			return
		}
		slot := colorSlots[m.colorSel]
		cur := m.schemes[m.schemeSel].pal.get(slot.key)
		m.openInput(inputThemeColor, " "+slot.label+" ", cur)
	}
}

func (m *model) addScheme(name string) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "\n") {
		m.notice = "theme name is empty"
		return
	}
	for i := range m.schemes {
		if m.schemes[i].name == name {
			m.schemeSel = i
			m.notice = "theme already exists"
			return
		}
	}
	m.schemes = append(m.schemes, scheme{
		name: name,
		pal:  m.currentScheme().pal.fill(),
		user: true,
	})
	m.schemeSel = len(m.schemes) - 1
	m.previewScheme()
	if err := m.writeUserSchemes(); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) setSchemeColor(val string) {
	if m.schemeSel < 0 || m.schemeSel >= len(m.schemes) || m.colorSel < 0 || m.colorSel >= len(colorSlots) {
		return
	}
	val = strings.TrimSpace(val)
	if !validColor(val) {
		m.notice = "color is 0-255 or #hex"
		return
	}
	m.schemes[m.schemeSel].user = true
	m.schemes[m.schemeSel].pal.set(colorSlots[m.colorSel].key, val)
	if m.schemes[m.schemeSel].name == m.themeName {
		applyPalette(m.schemes[m.schemeSel].pal)
	}
	if err := m.writeUserSchemes(); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) deleteScheme() {
	if m.schemeSel < 0 || m.schemeSel >= len(m.schemes) {
		return
	}
	s := m.schemes[m.schemeSel]
	if !s.user {
		m.notice = "built-in theme"
		return
	}
	name := s.name
	rest := append([]scheme{}, m.schemes[:m.schemeSel]...)
	rest = append(rest, m.schemes[m.schemeSel+1:]...)
	var user []scheme
	for _, sc := range rest {
		if sc.user {
			user = append(user, sc)
		}
	}
	m.schemes = mergeSchemes(user)
	if m.schemeSel >= len(m.schemes) {
		m.schemeSel = len(m.schemes) - 1
	}
	if m.themeName == name || m.themeWas == name {
		if m.themeName == name {
			m.previewScheme()
		}
		if m.themeWas == name {
			m.themeWas = m.themeName
		}
	}
	if err := m.writeUserSchemes(); err != nil {
		m.notice = err.Error()
		return
	}
	if err := m.writeThemeName(); err != nil {
		m.notice = err.Error()
	}
}

func (m *model) writeThemeName() error {
	return os.WriteFile(themeNamePath(m.store.Path), []byte(m.themeName+"\n"), 0o600)
}

func (m *model) writeUserSchemes() error {
	var b strings.Builder
	b.WriteString("# name on its own line, then key and color\n")
	b.WriteString("# keys: ink paper accent warn gold tag muted pick card archive\n")
	b.WriteString("# colors: 0-255 or #rrggbb\n")
	for _, s := range m.schemes {
		if !s.user {
			continue
		}
		p := s.pal.fill()
		fmt.Fprintf(&b, "\n%s\n", s.name)
		for _, slot := range colorSlots {
			fmt.Fprintf(&b, "%s %s\n", slot.key, p.get(slot.key))
		}
	}
	return os.WriteFile(themesPath(m.store.Path), []byte(b.String()), 0o600)
}

func (m *model) themeDialog() string {
	w := max(48, min(76, m.width-4))
	h := max(14, min(22, m.height-2))
	textW := max(20, w-4)
	var lines []string
	if m.schemeEdit && m.schemeSel >= 0 && m.schemeSel < len(m.schemes) {
		s := m.schemes[m.schemeSel]
		for i, slot := range colorSlots {
			row := fmt.Sprintf("%-8s %s", slot.label, s.pal.get(slot.key))
			row = fit(row, textW)
			if i == m.colorSel {
				row = selStyle.Width(textW).Render(row)
			}
			lines = append(lines, row)
		}
		hint := "j/k select   enter edit   q back"
		if m.notice != "" {
			hint = m.notice
		}
		body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render(fit(hint, textW))
		return panelBox(" "+s.name+" ", body, w, h, true)
	}
	for i, s := range m.schemes {
		chip := lipgloss.NewStyle().Background(lipgloss.Color(s.pal.fill().accent)).Render("  ")
		row := chip + " " + s.name
		if s.user {
			row += " ·"
		}
		row = fit(row, textW)
		if i == m.schemeSel {
			row = selStyle.Width(textW).Render(fit(s.name, textW))
		}
		lines = append(lines, row)
	}
	path := ""
	if m.store != nil {
		path = themesPath(m.store.Path)
	}
	hint := "j/k select   enter apply   c new   e colors   d delete   q cancel"
	if m.notice != "" {
		hint = m.notice
	}
	body := strings.Join(lines, "\n") + "\n" + mutedStyle.Render(fit(path, textW)) + "\n" + mutedStyle.Render(fit(hint, textW))
	return panelBox(" Color theme ", body, w, h, true)
}
