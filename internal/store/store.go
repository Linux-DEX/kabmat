package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	indentColumn  = " "
	indentCard    = "  "
	indentCheck   = "   "
	indentSub     = "   >"
	indentSubDesc = "   >>"
	indentDesc    = "    "
)

var errEmptyName = errors.New("name is empty")

type ChecklistItem struct {
	Content string
	Done    bool
}

type Subtask struct {
	Content     string
	Description string
	Done        bool
}

type Card struct {
	Content     string
	Description string
	Checklist   []ChecklistItem
	Subtasks    []Subtask
	Priority    int      // 0, 1 (!), 2 (!!)
	Due         string   // YYYY-MM-DD
	Tags        []string // stored without '#'
	Moved       string   // YYYY-MM-DD, set when the card is created or changes column
}

type Column struct {
	Title   string
	Cards   []Card
	WIP     int  // 0 means no limit
	Gate    bool // entering requires a finished checklist
	Archive bool
}

type Board struct {
	Name    string
	Columns []Column
}

const maxUndo = 100

type Store struct {
	Path         string
	Backup       string
	Boards       []Board
	CardAtBottom bool

	committed string
	undo      []string
	redo      []string
	dirty     bool
	backedUp  bool // data_bkp is written on the first Flush of the session
}

func Paths() (string, string, error) {
	if p := os.Getenv("KABMAT_DATA"); p != "" {
		return DataPaths(p)
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "kabmat")
	return filepath.Join(dir, "data"), filepath.Join(dir, "data_bkp"), nil
}

// DataPaths is the board file and its sibling backup for a custom location.
func DataPaths(path string) (string, string, error) {
	path, err := expandHome(path)
	if err != nil {
		return "", "", err
	}
	if path == "" {
		return "", "", errors.New("data path is empty")
	}
	return path, path + "_bkp", nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func Load(path, backup string) (*Store, error) {
	s := &Store{Path: path, Backup: backup}
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("couldn't read data from file %q: %w", path, err)
		}
		if err := s.write(""); err != nil {
			return nil, err
		}
		return s, nil
	}
	boards, err := parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Boards = boards
	s.committed = encode(boards)
	return s, nil
}

func parse(data string) ([]Board, error) {
	if data == "" {
		return nil, nil
	}
	data = strings.TrimSuffix(data, "\n")
	var boards []Board
	readingBoard, readingColumn, readingCard := false, false, false
	for i, line := range strings.Split(data, "\n") {
		n := i + 1
		switch {
		case strings.HasPrefix(line, indentSubDesc):
			if !readingCard || len(lastCard(boards).Subtasks) == 0 {
				return nil, fmt.Errorf("[line %d] reading a subtask description without a subtask", n)
			}
			text := strings.TrimPrefix(line[len(indentSubDesc):], " ")
			sub := lastSub(boards)
			sub.Description += text + "\n"
		case strings.HasPrefix(line, indentSub):
			if !readingCard {
				return nil, fmt.Errorf("[line %d] reading a subtask without a card", n)
			}
			sub, err := parseSub(line[len(indentSub):])
			if err != nil {
				return nil, fmt.Errorf("[line %d] %w", n, err)
			}
			card := lastCard(boards)
			card.Subtasks = append(card.Subtasks, sub)
		case strings.HasPrefix(line, indentDesc):
			if !readingCard {
				return nil, fmt.Errorf("[line %d] reading a description without a card", n)
			}
			card := lastCard(boards)
			card.Description += line[len(indentDesc):] + "\n"
		case strings.HasPrefix(line, indentCheck):
			if !readingCard {
				return nil, fmt.Errorf("[line %d] reading a checklist item without a card", n)
			}
			raw := line[len(indentCheck):]
			if strings.HasPrefix(raw, "@") {
				applyCardMeta(lastCard(boards), raw[1:])
				break
			}
			item, err := parseCheck(raw)
			if err != nil {
				return nil, fmt.Errorf("[line %d] %w", n, err)
			}
			card := lastCard(boards)
			card.Checklist = append(card.Checklist, item)
		case strings.HasPrefix(line, indentCard):
			if !readingColumn {
				return nil, fmt.Errorf("[line %d] reading a card without a column", n)
			}
			boards[len(boards)-1].Columns[len(boards[len(boards)-1].Columns)-1].Cards = append(
				lastColumn(boards).Cards,
				Card{Content: strings.TrimSpace(line)},
			)
			readingCard = true
		case strings.HasPrefix(line, indentColumn):
			if !readingBoard {
				return nil, fmt.Errorf("[line %d] reading a column without a board", n)
			}
			boards[len(boards)-1].Columns = append(boards[len(boards)-1].Columns, parseColumnLine(line))
			readingColumn = true
			readingCard = false
		default:
			boards = append(boards, Board{Name: strings.TrimSpace(line)})
			readingBoard = true
			readingColumn = false
			readingCard = false
		}
	}
	for i := range boards {
		boards[i].Name = stripControls(boards[i].Name)
		for j := range boards[i].Columns {
			boards[i].Columns[j].Title = stripControls(boards[i].Columns[j].Title)
			for k := range boards[i].Columns[j].Cards {
				card := &boards[i].Columns[j].Cards[k]
				card.Content = stripControls(card.Content)
				card.Description = stripControls(strings.TrimSuffix(card.Description, "\n"))
				for n := range card.Checklist {
					card.Checklist[n].Content = stripControls(card.Checklist[n].Content)
				}
				for s := range card.Subtasks {
					card.Subtasks[s].Content = stripControls(card.Subtasks[s].Content)
					card.Subtasks[s].Description = stripControls(strings.TrimSuffix(card.Subtasks[s].Description, "\n"))
				}
			}
		}
	}
	return boards, nil
}

func parseColumnLine(line string) Column {
	title := strings.TrimSpace(line)
	i := strings.LastIndex(title, " {")
	if i < 0 || !strings.HasSuffix(title, "}") {
		return Column{Title: title}
	}
	name := strings.TrimSpace(title[:i])
	if name == "" {
		return Column{Title: title}
	}
	col := Column{Title: name}
	for _, part := range strings.Split(title[i+2:len(title)-1], ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "check":
			col.Gate = true
		case part == "archive":
			col.Archive = true
		case strings.HasPrefix(part, "wip:"):
			n, err := strconv.Atoi(strings.TrimPrefix(part, "wip:"))
			if err != nil || n <= 0 {
				return Column{Title: title}
			}
			col.WIP = n
		default:
			return Column{Title: title}
		}
	}
	return col
}

func columnLabel(col Column) string {
	var flags []string
	if col.WIP > 0 {
		flags = append(flags, "wip:"+strconv.Itoa(col.WIP))
	}
	if col.Gate {
		flags = append(flags, "check")
	}
	if col.Archive {
		flags = append(flags, "archive")
	}
	if len(flags) == 0 {
		return col.Title
	}
	return col.Title + " {" + strings.Join(flags, ",") + "}"
}

func applyCardMeta(card *Card, raw string) {
	for _, f := range strings.Fields(raw) {
		k, v, ok := strings.Cut(f, ":")
		if !ok || v == "" {
			continue
		}
		switch k {
		case "pri":
			n, err := strconv.Atoi(v)
			if err == nil && n > card.Priority {
				if n > 2 {
					n = 2
				}
				card.Priority = n
			}
		case "due":
			if _, err := time.Parse("2006-01-02", v); err == nil {
				card.Due = v
			}
		case "tag":
			if tag, ok := cleanTag(v); ok {
				card.Tags = appendTag(card.Tags, tag)
			}
		case "moved":
			if _, err := time.Parse("2006-01-02", v); err == nil {
				card.Moved = v
			}
		}
	}
}

func cleanTag(s string) (string, bool) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "#")
	if !validTag(s) {
		return "", false
	}
	return s, true
}

// validTag rejects spaces, controls, and terminal escapes. A tag is a short label, not a path.
func validTag(s string) bool {
	if s == "" || len(s) > 32 || strings.Contains(s, "..") {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '/' || c == '-' || c == '_' || c == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// stripControls drops terminal control characters from text that will be drawn.
func stripControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func appendTag(tags []string, tag string) []string {
	for _, t := range tags {
		if t == tag {
			return tags
		}
	}
	return append(tags, tag)
}

// ParseTags splits "#work, home" into unique tags.
func ParseTags(s string) []string {
	s = strings.ReplaceAll(s, ",", " ")
	var out []string
	for _, p := range strings.Fields(s) {
		if tag, ok := cleanTag(p); ok {
			out = appendTag(out, tag)
		}
	}
	return out
}

func today() string {
	return time.Now().Format("2006-01-02")
}

func checklistDone(card Card) bool {
	for _, item := range card.Checklist {
		if !item.Done {
			return false
		}
	}
	return true
}

func (col Column) admit(card Card) error {
	if col.WIP > 0 && len(col.Cards) >= col.WIP {
		return fmt.Errorf("%s is at its WIP limit (%d)", col.Title, col.WIP)
	}
	if col.Gate && !checklistDone(card) {
		return fmt.Errorf("finish the checklist before moving into %s", col.Title)
	}
	return nil
}

func parseCheck(raw string) (ChecklistItem, error) {
	rest := strings.TrimSpace(raw)
	if rest == "" {
		return ChecklistItem{}, errors.New("empty checklist item")
	}
	if rest[0] != '+' && rest[0] != '-' {
		return ChecklistItem{}, fmt.Errorf("checklist item %q must start with + or -", rest)
	}
	return ChecklistItem{Done: rest[0] == '+', Content: rest[1:]}, nil
}

func parseSub(raw string) (Subtask, error) {
	rest := strings.TrimSpace(raw)
	if rest == "" || (rest[0] != '+' && rest[0] != '-') {
		return Subtask{}, fmt.Errorf("subtask %q must start with + or -", rest)
	}
	return Subtask{Done: rest[0] == '+', Content: strings.TrimSpace(rest[1:])}, nil
}

func lastSub(boards []Board) *Subtask {
	card := lastCard(boards)
	return &card.Subtasks[len(card.Subtasks)-1]
}

func lastCard(boards []Board) *Card {
	col := lastColumn(boards)
	return &col.Cards[len(col.Cards)-1]
}

func lastColumn(boards []Board) *Column {
	b := &boards[len(boards)-1]
	return &b.Columns[len(b.Columns)-1]
}

func encode(boards []Board) string {
	var b strings.Builder
	for _, board := range boards {
		b.WriteString(board.Name)
		b.WriteByte('\n')
		for _, col := range board.Columns {
			b.WriteString(indentColumn)
			b.WriteString(columnLabel(col))
			b.WriteByte('\n')
			for _, card := range col.Cards {
				b.WriteString(indentCard)
				b.WriteString(card.Content)
				b.WriteByte('\n')
				writeCardMeta(&b, card)
				for _, item := range card.Checklist {
					mark := "-"
					if item.Done {
						mark = "+"
					}
					b.WriteString(indentCheck)
					b.WriteString(mark)
					b.WriteString(item.Content)
					b.WriteByte('\n')
				}
				for _, sub := range card.Subtasks {
					mark := "-"
					if sub.Done {
						mark = "+"
					}
					b.WriteString(indentSub)
					b.WriteString(mark)
					b.WriteString(sub.Content)
					b.WriteByte('\n')
					if sub.Description != "" {
						b.WriteString(indentSubDesc)
						b.WriteByte(' ')
						b.WriteString(strings.ReplaceAll(sub.Description, "\n", "\n"+indentSubDesc+" "))
						b.WriteByte('\n')
					}
				}
				if card.Description != "" {
					b.WriteString(indentDesc)
					b.WriteString(strings.ReplaceAll(card.Description, "\n", "\n"+indentDesc))
					b.WriteByte('\n')
				}
			}
		}
	}
	return b.String()
}

func writeCardMeta(b *strings.Builder, card Card) {
	var parts []string
	if card.Priority > 0 {
		parts = append(parts, "pri:"+strconv.Itoa(card.Priority))
	}
	if card.Due != "" {
		parts = append(parts, "due:"+card.Due)
	}
	for _, tag := range card.Tags {
		parts = append(parts, "tag:"+tag)
	}
	if card.Moved != "" {
		parts = append(parts, "moved:"+card.Moved)
	}
	if len(parts) == 0 {
		return
	}
	b.WriteString(indentCheck)
	b.WriteByte('@')
	b.WriteString(strings.Join(parts, " "))
	b.WriteByte('\n')
}

func (s *Store) Index(name string) (int, error) {
	for i := range s.Boards {
		if s.Boards[i].Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no board named %q was found", name)
}

func validBoardName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("board name can't be empty")
	}
	if strings.ContainsAny(name, "\r\n") {
		return "", errors.New("board name can't contain a newline")
	}
	return name, nil
}

func (s *Store) nameTaken(name string, except int) bool {
	for i := range s.Boards {
		if i != except && s.Boards[i].Name == name {
			return true
		}
	}
	return false
}

func (s *Store) CreateBoard(name string) error {
	return s.create(name, nil)
}

// CreateKanban makes a board with Todo, Doing, and a Done column that requires a finished checklist.
func (s *Store) CreateKanban(name string) error {
	return s.create(name, []Column{
		{Title: "Todo"},
		{Title: "Doing"},
		{Title: "Done", Gate: true},
	})
}

func (s *Store) create(name string, cols []Column) error {
	name, err := validBoardName(name)
	if err != nil {
		return err
	}
	if s.nameTaken(name, -1) {
		return fmt.Errorf("board %q already exists", name)
	}
	s.Boards = append(s.Boards, Board{Name: name, Columns: cols})
	return s.save()
}

func (s *Store) RenameBoard(i int, name string) error {
	name, err := validBoardName(name)
	if err != nil {
		return err
	}
	if s.nameTaken(name, i) {
		return fmt.Errorf("board %q already exists", name)
	}
	s.Boards[i].Name = name
	return s.save()
}

func (s *Store) DeleteBoardAt(i int) error {
	s.Boards = append(s.Boards[:i], s.Boards[i+1:]...)
	return s.save()
}

func (s *Store) DeleteBoardNamed(name string) error {
	i, err := s.Index(name)
	if err != nil {
		return err
	}
	return s.DeleteBoardAt(i)
}

func (s *Store) MoveBoard(i, dir int) (bool, error) {
	j := i + dir
	if j < 0 || j >= len(s.Boards) {
		return false, nil
	}
	s.Boards[i], s.Boards[j] = s.Boards[j], s.Boards[i]
	return true, s.save()
}

func (s *Store) AddColumn(bi int, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errEmptyName
	}
	s.Boards[bi].Columns = append(s.Boards[bi].Columns, Column{Title: title})
	return s.save()
}

func (s *Store) RenameColumn(bi, ci int, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errEmptyName
	}
	s.Boards[bi].Columns[ci].Title = title
	return s.save()
}

func (s *Store) DeleteColumn(bi, ci int) error {
	cols := s.Boards[bi].Columns
	s.Boards[bi].Columns = append(cols[:ci], cols[ci+1:]...)
	return s.save()
}

func (s *Store) MoveColumn(bi, ci, dir int) (bool, error) {
	cols := s.Boards[bi].Columns
	j := ci + dir
	if j < 0 || j >= len(cols) {
		return false, nil
	}
	cols[ci], cols[j] = cols[j], cols[ci]
	return true, s.save()
}

func (s *Store) AddCard(bi, ci int, card Card) error {
	return s.InsertCard(bi, ci, len(s.Boards[bi].Columns[ci].Cards), card)
}

func (s *Store) InsertCard(bi, ci, at int, card Card) error {
	card.Content = strings.TrimSpace(card.Content)
	if card.Content == "" {
		return errEmptyName
	}
	card.Tags = ParseTags(strings.Join(card.Tags, " "))
	if card.Priority < 0 {
		card.Priority = 0
	}
	if card.Priority > 2 {
		card.Priority = 2
	}
	col := &s.Boards[bi].Columns[ci]
	if err := col.admit(card); err != nil {
		return err
	}
	if card.Moved == "" {
		card.Moved = today()
	}
	if at < 0 || at > len(col.Cards) {
		at = len(col.Cards)
	}
	col.Cards = append(col.Cards, Card{})
	copy(col.Cards[at+1:], col.Cards[at:])
	col.Cards[at] = card
	return s.save()
}

func (s *Store) UpdateCard(bi, ci, cardi int, card Card) error {
	card.Content = strings.TrimSpace(card.Content)
	if card.Content == "" {
		return errEmptyName
	}
	card.Tags = ParseTags(strings.Join(card.Tags, " "))
	if card.Priority < 0 {
		card.Priority = 0
	}
	if card.Priority > 2 {
		card.Priority = 2
	}
	prev := s.Boards[bi].Columns[ci].Cards[cardi]
	if card.Moved == "" {
		card.Moved = prev.Moved
	}
	s.Boards[bi].Columns[ci].Cards[cardi] = card
	return s.save()
}

func (s *Store) SetWIP(bi, ci, n int) error {
	if n < 0 {
		return errors.New("WIP limit can't be negative")
	}
	s.Boards[bi].Columns[ci].WIP = n
	return s.save()
}

func (s *Store) ToggleGate(bi, ci int) (bool, error) {
	col := &s.Boards[bi].Columns[ci]
	col.Gate = !col.Gate
	return col.Gate, s.save()
}

// ColumnIndex matches a column title or a 1-based index.
func (b Board) ColumnIndex(name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("column name is empty")
	}
	for i := range b.Columns {
		if b.Columns[i].Title == name {
			return i, nil
		}
	}
	n, err := strconv.Atoi(name)
	if err == nil && n >= 1 && n <= len(b.Columns) {
		return n - 1, nil
	}
	return 0, fmt.Errorf("no column %q", name)
}

func (s *Store) DeleteCard(bi, ci, cardi int) error {
	cards := s.Boards[bi].Columns[ci].Cards
	s.Boards[bi].Columns[ci].Cards = append(cards[:cardi], cards[cardi+1:]...)
	return s.save()
}

func (s *Store) MoveCardVert(bi, ci, cardi, dir int) (bool, error) {
	cards := s.Boards[bi].Columns[ci].Cards
	j := cardi + dir
	if j < 0 || j >= len(cards) {
		return false, nil
	}
	cards[cardi], cards[j] = cards[j], cards[cardi]
	return true, s.save()
}

func (s *Store) MoveCardHoriz(bi, src, cardi, dir int) (bool, error) {
	return s.MoveCardTo(bi, src, cardi, src+dir)
}

func (s *Store) MoveCardTo(bi, src, cardi, dst int) (bool, error) {
	if err := s.relocate(bi, src, cardi, dst); err != nil {
		if errors.Is(err, errNoMove) {
			return false, nil
		}
		return false, err
	}
	return true, s.save()
}

var errNoMove = errors.New("card stays")

// ErrArchived is returned when the card is already in an archive column.
var ErrArchived = errors.New("already archived")

func (s *Store) relocate(bi, src, cardi, dst int) error {
	cols := s.Boards[bi].Columns
	if src == dst || src < 0 || dst < 0 || src >= len(cols) || dst >= len(cols) || cardi < 0 || cardi >= len(cols[src].Cards) {
		return errNoMove
	}
	card := cols[src].Cards[cardi]
	if err := cols[dst].admit(card); err != nil {
		return err
	}
	card.Moved = today()
	cols[src].Cards = append(cols[src].Cards[:cardi], cols[src].Cards[cardi+1:]...)
	if s.CardAtBottom {
		cols[dst].Cards = append(cols[dst].Cards, card)
	} else {
		cols[dst].Cards = append([]Card{card}, cols[dst].Cards...)
	}
	return nil
}

// ArchiveCard moves a card into the board's archive column, creating one if needed.
func (s *Store) ArchiveCard(bi, src, cardi int) (int, error) {
	cols := s.Boards[bi].Columns
	if src < 0 || src >= len(cols) || cardi < 0 || cardi >= len(cols[src].Cards) {
		return 0, errNoMove
	}
	if cols[src].Archive {
		return src, ErrArchived
	}
	dst := -1
	for i := range cols {
		if cols[i].Archive {
			dst = i
			break
		}
	}
	if dst < 0 {
		s.Boards[bi].Columns = append(s.Boards[bi].Columns, Column{Title: "Archive", Archive: true})
		dst = len(s.Boards[bi].Columns) - 1
	}
	if err := s.relocate(bi, src, cardi, dst); err != nil {
		return dst, err
	}
	return dst, s.save()
}

// ApplyCards updates each [column, card] and writes one undo step.
func (s *Store) ApplyCards(bi int, picks [][2]int, fn func(*Card)) error {
	if len(picks) == 0 {
		return nil
	}
	cols := s.Boards[bi].Columns
	for _, p := range picks {
		if p[0] < 0 || p[0] >= len(cols) || p[1] < 0 || p[1] >= len(cols[p[0]].Cards) {
			continue
		}
		fn(&s.Boards[bi].Columns[p[0]].Cards[p[1]])
	}
	return s.save()
}

// DeleteCards removes each [column, card]. Higher indexes in a column go first.
func (s *Store) DeleteCards(bi int, picks [][2]int) error {
	if len(picks) == 0 {
		return nil
	}
	ordered := append([][2]int(nil), picks...)
	sortPicks(ordered)
	cols := s.Boards[bi].Columns
	seen := map[[2]int]bool{}
	for _, p := range ordered {
		if seen[p] || p[0] < 0 || p[0] >= len(cols) || p[1] < 0 || p[1] >= len(s.Boards[bi].Columns[p[0]].Cards) {
			continue
		}
		seen[p] = true
		cards := s.Boards[bi].Columns[p[0]].Cards
		s.Boards[bi].Columns[p[0]].Cards = append(cards[:p[1]], cards[p[1]+1:]...)
	}
	return s.save()
}

func sortPicks(picks [][2]int) {
	for i := 1; i < len(picks); i++ {
		j := i
		for j > 0 && (picks[j][0] < picks[j-1][0] || picks[j][0] == picks[j-1][0] && picks[j][1] > picks[j-1][1]) {
			picks[j], picks[j-1] = picks[j-1], picks[j]
			j--
		}
	}
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kabmat-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (s *Store) write(data string) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	if !s.backedUp {
		if cur, err := os.ReadFile(s.Path); err == nil {
			if len(cur) > 0 {
				if err := writeFileAtomic(s.Backup, cur, 0o600); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		s.backedUp = true
	}
	return writeFileAtomic(s.Path, []byte(data), 0o600)
}

// save records the undo snapshot and marks the store dirty. Flush writes the file.
func (s *Store) save() error {
	cur := encode(s.Boards)
	if cur != s.committed {
		s.undo = append(s.undo, s.committed)
		if len(s.undo) > maxUndo {
			s.undo = s.undo[1:]
		}
		s.redo = nil
		s.committed = cur
		s.dirty = true
	}
	return nil
}

// Flush writes the committed state once. Repeated calls are a no-op until the next edit.
func (s *Store) Flush() error {
	if !s.dirty {
		return nil
	}
	if err := s.write(s.committed); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

type Place struct {
	Board string
	Col   int
	Card  int
}

func placePath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "place")
}

func LoadPlace(dataPath string) (Place, error) {
	b, err := os.ReadFile(placePath(dataPath))
	if err != nil {
		if os.IsNotExist(err) {
			return Place{}, nil
		}
		return Place{}, err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) < 3 || lines[0] == "" {
		return Place{}, nil
	}
	col, err1 := strconv.Atoi(lines[1])
	card, err2 := strconv.Atoi(lines[2])
	if err1 != nil || err2 != nil || col < 0 || card < 0 {
		return Place{}, nil
	}
	return Place{Board: lines[0], Col: col, Card: card}, nil
}

func SavePlace(dataPath string, p Place) error {
	data := p.Board + "\n" + strconv.Itoa(p.Col) + "\n" + strconv.Itoa(p.Card) + "\n"
	return writeFileAtomic(placePath(dataPath), []byte(data), 0o600)
}

// Markdown is a board snapshot for sharing. It leaves out last-moved dates.
func Markdown(boards []Board) string {
	var b strings.Builder
	for i, board := range boards {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("# ")
		b.WriteString(board.Name)
		b.WriteByte('\n')
		for _, col := range board.Columns {
			b.WriteString("\n## ")
			b.WriteString(col.Title)
			b.WriteString(" (")
			b.WriteString(strconv.Itoa(len(col.Cards)))
			if col.WIP > 0 {
				b.WriteByte('/')
				b.WriteString(strconv.Itoa(col.WIP))
			}
			b.WriteByte(')')
			if col.Gate {
				b.WriteString(" · checklist")
			}
			if col.Archive {
				b.WriteString(" · archive")
			}
			b.WriteByte('\n')
			for _, card := range col.Cards {
				b.WriteString("\n- ")
				switch card.Priority {
				case 1:
					b.WriteString("! ")
				case 2:
					b.WriteString("!! ")
				}
				b.WriteString(card.Content)
				for _, tag := range card.Tags {
					b.WriteString(" #")
					b.WriteString(tag)
				}
				if card.Due != "" {
					b.WriteString(" · due ")
					b.WriteString(card.Due)
				}
				b.WriteByte('\n')
				if card.Description != "" {
					for _, line := range strings.Split(card.Description, "\n") {
						b.WriteString("  ")
						b.WriteString(line)
						b.WriteByte('\n')
					}
				}
				for _, item := range card.Checklist {
					b.WriteString("  - [")
					if item.Done {
						b.WriteByte('x')
					} else {
						b.WriteByte(' ')
					}
					b.WriteString("] ")
					b.WriteString(item.Content)
					b.WriteByte('\n')
				}
				for _, sub := range card.Subtasks {
					b.WriteString("  - ")
					if sub.Done {
						b.WriteString("done ")
					}
					b.WriteString(sub.Content)
					b.WriteByte('\n')
					if sub.Description != "" {
						for _, line := range strings.Split(sub.Description, "\n") {
							b.WriteString("    ")
							b.WriteString(line)
							b.WriteByte('\n')
						}
					}
				}
			}
		}
	}
	return b.String()
}

func (s *Store) Undo() (bool, error) { return s.step(&s.undo, &s.redo) }
func (s *Store) Redo() (bool, error) { return s.step(&s.redo, &s.undo) }

func (s *Store) step(from, to *[]string) (bool, error) {
	if len(*from) == 0 {
		return false, nil
	}
	last := len(*from) - 1
	target := (*from)[last]
	boards, err := parse(target)
	if err != nil {
		return false, err
	}
	*from = (*from)[:last]
	*to = append(*to, s.committed)
	s.Boards = boards
	s.committed = target
	s.dirty = true
	return true, nil
}
