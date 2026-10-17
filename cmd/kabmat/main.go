package main

import (
	"fmt"
	"io"
	"os"

	"kabmat/internal/store"
	"kabmat/internal/tui"
)

const version = "0.1.0"

func main() {
	path, backup, err := store.Paths()
	if err != nil {
		fail(err)
	}
	boards, err := store.Load(path, backup)
	if err != nil {
		fail(err)
	}
	start, open, err := applyArgs(os.Args[1:], boards, os.Stdout)
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

func applyArgs(args []string, s *store.Store, w io.Writer) (bool, string, error) {
	tuiOn := true
	open := ""
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
			if err := s.CreateBoard(name); err != nil {
				return false, "", err
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
		case "-t", "--text":
			tuiOn = false
		case "-b", "--card-at-bottom":
			s.CardAtBottom = true
		default:
			return false, "", fmt.Errorf("unknown option `%s`", a)
		}
	}
	return tuiOn, open, nil
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
  -o, --open <name>       open board with name <name>
  -d, --delete <name>     delete board with name <name>

  -t, --text              disable tui
  -b, --card-at-bottom    when moving cards between columns, put them at the bottom

Press ? inside the TUI for keybindings
`, version)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	os.Exit(1)
}
