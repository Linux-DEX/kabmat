package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config is the user file under $XDG_CONFIG_HOME/kabmat/config
// (or ~/.config/kabmat/config). A missing file is the built-in default.
type Config struct {
	Path   string
	Exists bool
	Theme  string
	Tags   []string
	Themes map[string]map[string]string
	Menu   map[string][]string
	Board  map[string][]string

	menuKey  map[string]string
	boardKey map[string]string
}

type bind struct {
	action string
	keys   []string
}

var menuDefaults = []bind{
	{"quit", []string{"q"}},
	{"undo", []string{"u"}},
	{"redo", []string{"ctrl+r"}},
	{"help", []string{"?"}},
	{"up", []string{"k"}},
	{"down", []string{"j"}},
	{"first", []string{"g"}},
	{"last", []string{"G"}},
	{"move-up", []string{"K", "shift+up"}},
	{"move-down", []string{"J", "shift+down"}},
	{"new", []string{"c"}},
	{"rename", []string{"r", "e"}},
	{"delete", []string{"d"}},
	{"open", []string{"enter"}},
	{"theme", []string{"ctrl+t"}},
}

var boardDefaults = []bind{
	{"back", []string{"q"}},
	{"undo", []string{"u"}},
	{"redo", []string{"ctrl+r"}},
	{"search", []string{"/"}},
	{"find-next", []string{"n"}},
	{"find-prev", []string{"N"}},
	{"help", []string{"?"}},
	{"left", []string{"h"}},
	{"right", []string{"l"}},
	{"up", []string{"k"}},
	{"down", []string{"j"}},
	{"first", []string{"g"}},
	{"last", []string{"G"}},
	{"move-left", []string{"H"}},
	{"move-right", []string{"L"}},
	{"wip", []string{"w"}},
	{"gate", []string{"W"}},
	{"archive", []string{"A"}},
	{"toggle-archive", []string{"v"}},
	{"priority", []string{"!"}},
	{"clear-selection", []string{"esc"}},
	{"select", []string{"space"}},
	{"tags", []string{"t"}},
	{"due", []string{"T"}},
	{"open-link", []string{"o"}},
	{"move-up", []string{"K", "shift+up"}},
	{"move-down", []string{"J", "shift+down"}},
	{"column-left", []string{"ctrl+h", "ctrl+p"}},
	{"column-right", []string{"ctrl+l", "ctrl+n"}},
	{"new-column", []string{"C"}},
	{"rename-column", []string{"E"}},
	{"delete-column", []string{"D"}},
	{"new-card", []string{"c"}},
	{"edit", []string{"e", "enter"}},
	{"subtasks", []string{"s"}},
	{"delete", []string{"d"}},
	{"yank", []string{"y"}},
	{"paste", []string{"p"}},
	{"duplicate", []string{"Y"}},
	{"filter", []string{"f"}},
	{"fold", []string{"z"}},
	{"board-prev", []string{"["}},
	{"board-next", []string{"]"}},
	{"move", []string{"m"}},
	{"clear-done", []string{"X"}},
	{"theme", []string{"ctrl+t"}},
}

var repeatAction = map[string]bool{
	"up": true, "down": true, "left": true, "right": true,
	"move-up": true, "move-down": true,
	"find-next": true, "find-prev": true, "paste": true,
	"board-prev": true, "board-next": true,
	"column-left": true, "column-right": true,
}

var colorKeys = map[string]bool{
	"ink": true, "paper": true, "accent": true, "warn": true, "gold": true,
	"tag": true, "muted": true, "pick": true, "card": true, "archive": true,
}

// standardTags is the tag list used when the config file has no [tags] section.
var standardTags = []string{
	"bug",
	"feature",
	"documentation",
	"testing",
	"chore",
	"refactor",
	"performance",
	"ci/cd",
	"devops",
	"deployment",
	"security",
	"research",
}

func Default() Config {
	c := Config{
		Theme:  "linux-dex",
		Tags:   append([]string(nil), standardTags...),
		Themes: map[string]map[string]string{},
		Menu:   binds(menuDefaults),
		Board:  binds(boardDefaults),
	}
	c.reindex()
	return c
}

func binds(in []bind) map[string][]string {
	out := make(map[string][]string, len(in))
	for _, b := range in {
		out[b.action] = append([]string(nil), b.keys...)
	}
	return out
}

// Path is KABMAT_CONFIG, or $XDG_CONFIG_HOME/kabmat/config.
func Path() (string, error) {
	if p := os.Getenv("KABMAT_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "kabmat", "config"), nil
}

// Load reads the config file. A missing file returns Default.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	cfg.Path = path
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	cfg, err = parse(string(b), cfg)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Path = path
	cfg.Exists = true
	return cfg, nil
}

func parse(s string, cfg Config) (Config, error) {
	section := ""
	sawTags := false
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isComment(line, section) {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "tags" && !sawTags {
				cfg.Tags = nil
				sawTags = true
			}
			switch {
			case section == "tags" || section == "keys.menu" || section == "keys.board":
			case strings.HasPrefix(section, "theme."):
				name := strings.TrimPrefix(section, "theme.")
				if name == "" || strings.Contains(name, ".") {
					return Config{}, fmt.Errorf("line %d: bad theme name", i+1)
				}
				if cfg.Themes[name] == nil {
					cfg.Themes[name] = map[string]string{}
				}
			default:
				return Config{}, fmt.Errorf("line %d: unknown section %q", i+1, section)
			}
			continue
		}
		var err error
		switch {
		case section == "":
			err = parseTop(line, &cfg)
		case section == "tags":
			err = parseTag(line, &cfg)
		case section == "keys.menu":
			err = parseKeys(line, cfg.Menu, &cfg.menuKey)
		case section == "keys.board":
			err = parseKeys(line, cfg.Board, &cfg.boardKey)
		default:
			err = parseColor(line, cfg.Themes[strings.TrimPrefix(section, "theme.")])
		}
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	if cfg.Theme == "" {
		cfg.Theme = "linux-dex"
	}
	return cfg, nil
}

// isComment is a # line. Inside [tags], #review is a tag and "# review" stays a comment.
func isComment(line, section string) bool {
	if !strings.HasPrefix(line, "#") {
		return false
	}
	if section == "tags" && len(line) > 1 && line[1] != ' ' && line[1] != '\t' {
		return false
	}
	return true
}

func parseTop(line string, cfg *Config) error {
	fields := strings.Fields(line)
	if len(fields) == 2 && fields[0] == "theme" {
		cfg.Theme = fields[1]
		return nil
	}
	return fmt.Errorf("unknown setting %q", line)
}

func parseTag(line string, cfg *Config) error {
	tag := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(line)), "#")
	if tag == "" || strings.ContainsAny(tag, " \t{},") {
		return fmt.Errorf("bad tag %q", line)
	}
	for _, have := range cfg.Tags {
		if have == tag {
			return nil
		}
	}
	cfg.Tags = append(cfg.Tags, tag)
	return nil
}

func parseKeys(line string, actions map[string][]string, index *map[string]string) error {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return fmt.Errorf("keybinding needs an action and a key")
	}
	action, keys := fields[0], fields[1:]
	if _, ok := actions[action]; !ok {
		return fmt.Errorf("unknown action %q", action)
	}
	for _, old := range actions[action] {
		delete(*index, normKey(old))
	}
	var next []string
	for _, key := range keys {
		key = normKey(key)
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			return fmt.Errorf("digit %s is a count, not a binding", key)
		}
		if prev, ok := (*index)[key]; ok && prev != action {
			return fmt.Errorf("%s is already %s", key, prev)
		}
		(*index)[key] = action
		next = append(next, key)
	}
	actions[action] = next
	return nil
}

func parseColor(line string, colors map[string]string) error {
	fields := strings.Fields(line)
	if len(fields) != 2 || !colorKeys[fields[0]] || !validColor(fields[1]) {
		return fmt.Errorf("color is a name and a value, got %q", line)
	}
	colors[fields[0]] = fields[1]
	return nil
}

func validColor(s string) bool {
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

func normKey(key string) string {
	if key == " " {
		return "space"
	}
	return key
}

func (c *Config) reindex() {
	c.menuKey = index(c.Menu)
	c.boardKey = index(c.Board)
}

func index(actions map[string][]string) map[string]string {
	out := map[string]string{}
	for action, keys := range actions {
		for _, key := range keys {
			out[normKey(key)] = action
		}
	}
	return out
}

// Action is the command bound to key in "menu" or "board".
func (c Config) Action(ctx, key string) string {
	key = normKey(key)
	if ctx == "menu" {
		return c.menuKey[key]
	}
	return c.boardKey[key]
}

// Repeatable reports keys that honor a count prefix.
func (c Config) Repeatable(ctx, key string) bool {
	return repeatAction[c.Action(ctx, key)]
}

// Label is the keys bound to an action, for hints.
func (c Config) Label(ctx, action string) string {
	var keys []string
	if ctx == "menu" {
		keys = c.Menu[action]
	} else {
		keys = c.Board[action]
	}
	if len(keys) == 0 {
		return action
	}
	return strings.Join(keys, "/")
}

// Save writes the config, creating the directory if needed.
func Save(c Config) error {
	if c.Path == "" {
		return fmt.Errorf("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(c.Path, []byte(c.encode()), 0o600)
}

func (c Config) encode() string {
	var b strings.Builder
	b.WriteString("# kabmat config. A missing file uses these defaults.\n")
	b.WriteString("# theme is linux-dex or catppuccin-mocha, or a [theme.name] block below.\n")
	b.WriteString("# [tags] is the full tag list. [keys.menu] and [keys.board] replace a binding.\n")
	fmt.Fprintf(&b, "\ntheme %s\n", c.Theme)
	b.WriteString("\n[tags]\n")
	for _, tag := range c.Tags {
		b.WriteString(tag)
		b.WriteByte('\n')
	}
	writeKeys(&b, "keys.menu", menuDefaults, c.Menu)
	writeKeys(&b, "keys.board", boardDefaults, c.Board)
	names := make([]string, 0, len(c.Themes))
	for name := range c.Themes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "\n[theme.%s]\n", name)
		colors := c.Themes[name]
		for _, key := range []string{"ink", "paper", "accent", "warn", "gold", "tag", "muted", "pick", "card", "archive"} {
			if colors[key] == "" {
				continue
			}
			fmt.Fprintf(&b, "%s %s\n", key, colors[key])
		}
	}
	return b.String()
}

func writeKeys(b *strings.Builder, section string, order []bind, actions map[string][]string) {
	fmt.Fprintf(b, "\n[%s]\n", section)
	seen := map[string]bool{}
	for _, item := range order {
		seen[item.action] = true
		keys := actions[item.action]
		if len(keys) == 0 {
			keys = item.keys
		}
		fmt.Fprintf(b, "%s %s\n", item.action, strings.Join(keys, " "))
	}
	var extra []string
	for action := range actions {
		if !seen[action] {
			extra = append(extra, action)
		}
	}
	sort.Strings(extra)
	for _, action := range extra {
		fmt.Fprintf(b, "%s %s\n", action, strings.Join(actions[action], " "))
	}
}
