package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
}

type Column struct {
	Title string
	Cards []Card
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
			item, err := parseCheck(line[len(indentCheck):])
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
			boards[len(boards)-1].Columns = append(boards[len(boards)-1].Columns, Column{
				Title: strings.TrimSpace(line),
			})
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
		for j := range boards[i].Columns {
			for k := range boards[i].Columns[j].Cards {
				card := &boards[i].Columns[j].Cards[k]
				card.Description = strings.TrimSuffix(card.Description, "\n")
				for s := range card.Subtasks {
					card.Subtasks[s].Description = strings.TrimSuffix(card.Subtasks[s].Description, "\n")
				}
			}
		}
	}
	return boards, nil
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
			b.WriteString(col.Title)
			b.WriteByte('\n')
			for _, card := range col.Cards {
				b.WriteString(indentCard)
				b.WriteString(card.Content)
				b.WriteByte('\n')
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

func (s *Store) Index(name string) (int, error) {
	for i := range s.Boards {
		if s.Boards[i].Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no board named %q was found", name)
}

func (s *Store) CreateBoard(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("can't create a board with empty name")
	}
	s.Boards = append(s.Boards, Board{Name: name})
	return s.save()
}

func (s *Store) RenameBoard(i int, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("board name can't be empty")
	}
	s.Boards[i].Name = name
	return s.save()
}

func (s *Store) DeleteBoardAt(i int) error {
	s.Boards = append(s.Boards[:i], s.Boards[i+1:]...)
	return s.save()
}

func (s *Store) DeleteBoardNamed(name string) error {
	if _, err := s.Index(name); err != nil {
		return err
	}
	kept := s.Boards[:0]
	for _, b := range s.Boards {
		if b.Name != name {
			kept = append(kept, b)
		}
	}
	s.Boards = kept
	return s.save()
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
	col := &s.Boards[bi].Columns[ci]
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
	s.Boards[bi].Columns[ci].Cards[cardi] = card
	return s.save()
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
	cols := s.Boards[bi].Columns
	dst := src + dir
	if dst < 0 || dst >= len(cols) || cardi < 0 || cardi >= len(cols[src].Cards) {
		return false, nil
	}
	card := cols[src].Cards[cardi]
	cols[src].Cards = append(cols[src].Cards[:cardi], cols[src].Cards[cardi+1:]...)
	if s.CardAtBottom {
		cols[dst].Cards = append(cols[dst].Cards, card)
	} else {
		cols[dst].Cards = append([]Card{card}, cols[dst].Cards...)
	}
	return true, s.save()
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
