# kabmat

Terminal kanban boards with vim-like keybindings. A Go port of [kabmat](https://github.com/PlankCipher/kabmat).

## Install

```bash
go build -o kabmat ./cmd/kabmat
```

Run without installing:

```bash
go run ./cmd/kabmat
```

## Usage

```text
kabmat -l                 list boards
kabmat -c Work            create a board
kabmat -o Work            open a board
kabmat -d Work            delete a board
kabmat -t                 change data without opening the TUI
kabmat -b                 drop cards at the bottom when moving them across columns
kabmat -h                 help
kabmat -v                 version
```

Press `?` inside the TUI for the full key list.

### Boards

| Key | Action |
| --- | --- |
| `j` / `k` | move the highlight |
| `g` / `G` | first / last board |
| `K` / `J` | move the highlighted board up / down |
| `c` | create a board |
| `r`, `e` | rename |
| `d` | delete |
| `Enter` | open |
| `q` | quit |

### Board

| Key | Action |
| --- | --- |
| `h` / `l` | focus the column to the left / right |
| `j` / `k` | focus the card above / below |
| `g` / `G` | first / last card |
| `H` / `L` | move the card to the left / right column |
| `K` / `J` | move the focused card up / down |
| `Ctrl-h`, `Ctrl-p` | move the column left |
| `Ctrl-l`, `Ctrl-n` | move the column right |
| `C` / `E` / `D` | create / rename / delete a column |
| `c` / `e`, `Enter` / `d` | create / edit / delete a card |
| `s` | open subtasks for the focused card |
| `q` | back |

### Editing

Text fields start in normal mode. `i` inserts, `Esc` leaves insert mode, `Enter` saves, `Esc` or `q` cancels.

Card info fills the screen. `Tab` switches between the summary and the description. `Enter` saves and returns to the board. `q` leaves without saving.

`c` opens the checklist and `s` opens subtasks once the summary is set. Both lists use most of the screen. `j`/`k` select, `g`/`G` jump to the ends, `J`/`K` reorder, `Space` toggles done, `c` adds, `e` edits, and `d` deletes. `q` on either list saves and returns to the board. A subtask has its own summary and a tall description field.

`J` and `K` reorder boards on the board list. On a board they move the focused card. `Shift+Up` and `Shift+Down` do the same reorder.

## Data

Boards are stored in `$HOME/.local/share/kabmat/data`. A copy is written to `data_bkp` before each save. The file is the same indentation format as the original kabmat:

```text
board name
 column
  card
   -open checklist item
   +done checklist item
   >-open subtask
   >> subtask description
   >+done subtask
    description line
    another description line
```

## Layout

```text
cmd/kabmat          command-line entry point
internal/store      board file read and write
internal/tui        terminal interface
```

## License

[MIT](LICENSE)
