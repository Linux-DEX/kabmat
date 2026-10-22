package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Linux-DEX/kabmat/internal/store"
	"github.com/Linux-DEX/kabmat/internal/tui"
)

const version = "0.2.0"

func main() {
	args, pathFlag, err := splitPath(os.Args[1:])
	if err != nil {
		fail(err)
	}
	var path, backup string
	if pathFlag != "" {
		path, backup, err = store.DataPaths(pathFlag)
	} else {
		path, backup, err = store.Paths()
	}
	if err != nil {
		fail(err)
	}
	boards, err := store.Load(path, backup)
	if err != nil {
		fail(err)
	}
	start, open, err := applyArgs(args, boards, os.Stdout)
	if flushErr := boards.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}
	if err != nil {
		fail(err)
	}
	if !start {
		return
	}
	if err := tui.Run(boards, open); err != nil {
		fail(err)
	}
}

func splitPath(args []string) ([]string, string, error) {
	var rest []string
	path := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "-f" && args[i] != "--file" {
			rest = append(rest, args[i])
			continue
		}
		if i+1 >= len(args) {
			return nil, "", fmt.Errorf("not enough arguments for option `%s`", args[i])
		}
		i++
		path = args[i]
	}
	return rest, path, nil
}

func applyArgs(args []string, s *store.Store, w io.Writer) (bool, string, error) {
	tuiOn := true
	open := ""
	kanban := false
	created := false
	column := ""
	var adds []string
	export := ""
	exportSet := false
	for _, a := range args {
		if a == "-T" || a == "--template" {
			kanban = true
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("not enough arguments for option `%s`", a)
			}
			i++
			return args[i], nil
		}
		switch a {
		case "-h", "--help":
			tuiOn = false
			fmt.Fprint(w, usage())
		case "-v", "--version":
			tuiOn = false
			fmt.Fprintf(w, "kabmat %s\n", version)
		case "-l", "--list":
			tuiOn = false
			for _, b := range s.Boards {
				fmt.Fprintln(w, b.Name)
			}
		case "-c", "--create":
			name, err := value()
			if err != nil {
				return false, "", err
			}
			var errCreate error
			if kanban {
				errCreate = s.CreateKanban(name)
			} else {
				errCreate = s.CreateBoard(name)
			}
			if errCreate != nil {
				return false, "", errCreate
			}
			created = true
		case "-C", "--column":
			name, err := value()
			if err != nil {
				return false, "", err
			}
			column = name
		case "-T", "--template":
		case "-x", "--export":
			exportSet = true
			tuiOn = false
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				export = args[i]
			}
		case "-o", "--open":
			name, err := value()
			if err != nil {
				return false, "", err
			}
			if _, err := s.Index(name); err != nil {
				return false, "", err
			}
			open = name
		case "-d", "--delete":
			name, err := value()
			if err != nil {
				return false, "", err
			}
			if err := s.DeleteBoardNamed(name); err != nil {
				return false, "", err
			}
		case "-a", "--add":
			text, err := value()
			if err != nil {
				return false, "", err
			}
			adds = append(adds, text)
			tuiOn = false
		case "-t", "--text":
			tuiOn = false
		case "-b", "--card-at-bottom":
			s.CardAtBottom = true
		default:
			return false, "", fmt.Errorf("unknown option `%s`", a)
		}
	}
	if kanban && !created {
		return false, "", fmt.Errorf("--template needs -c")
	}
	if err := addCards(s, open, column, adds); err != nil {
		return false, "", err
	}
	if exportSet {
		if err := writeExport(s, open, export, w); err != nil {
			return false, "", err
		}
	}
	return tuiOn, open, nil
}

func writeExport(s *store.Store, open, path string, w io.Writer) error {
	boards := s.Boards
	if open != "" {
		i, err := s.Index(open)
		if err != nil {
			return err
		}
		boards = s.Boards[i : i+1]
	}
	text := store.Markdown(boards)
	if path == "" {
		fmt.Fprint(w, text)
		return nil
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

func addCards(s *store.Store, open, column string, texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	if open == "" {
		p, err := store.LoadPlace(s.Path)
		if err != nil {
			return err
		}
		open = p.Board
	}
	if open == "" {
		return fmt.Errorf("no board to add to, use -o")
	}
	bi, err := s.Index(open)
	if err != nil {
		return err
	}
	if len(s.Boards[bi].Columns) == 0 {
		return fmt.Errorf("board %q has no columns", open)
	}
	ci := 0
	if column != "" {
		var err error
		ci, err = s.Boards[bi].ColumnIndex(column)
		if err != nil {
			return err
		}
	}
	for _, text := range texts {
		if err := s.AddCard(bi, ci, store.Card{Content: text}); err != nil {
			return err
		}
	}
	return nil
}

func usage() string {
	return fmt.Sprintf(`kabmat %s
TUI program for managing kanban boards with vim-like keybindings

Usage: kabmat [OPTION]...

Options:
  -h, --help              print this help message
  -v, --version           print program version

  -l, --list              list all boards
  -c, --create <name>     create a new board with the name <name>
  -T, --template          with -c, start from Todo / Doing / Done
  -o, --open <name>       open board with name <name>
  -a, --add <text>        append a card and exit
  -C, --column <name>     with -a, column title or 1-based index
  -d, --delete <name>     delete board with name <name>
  -x, --export [file]     write a Markdown snapshot (stdout if no file)
  -f, --file <path>       board file (or set KABMAT_DATA)
                          config is $XDG_CONFIG_HOME/kabmat/config (or KABMAT_CONFIG)

  -t, --text              disable tui
  -b, --card-at-bottom    when moving cards between columns, put them at the bottom

Press ? inside the TUI for keybindings
`, version)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	os.Exit(1)
}
