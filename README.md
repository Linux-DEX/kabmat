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
| `J` / `K` | reorder the highlighted board |
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
| `K` / `J` | move the card up / down |
| `Ctrl-h`, `Ctrl-p` | move the column left |
| `Ctrl-l`, `Ctrl-n` | move the column right |
| `C` / `E` / `D` | create / rename / delete a column |
| `c` / `e` / `d` | create / edit / delete a card |
| `q` | back |

### Editing

Text fields start in normal mode. `i` inserts, `Esc` leaves insert mode, `Enter` saves, `Esc` or `q` cancels.

On a card, `Tab` switches between the title and the description. `c` opens the checklist once the title is set. In the checklist, `Space` toggles an item, `c` adds one, `e` edits it, and `d` deletes it.

## Data

Boards are stored in `$HOME/.local/share/kabmat/data`. A copy is written to `data_bkp` before each save. The file is the same indentation format as the original kabmat:

```text
board name
 column
  card
   -open checklist item
   +done checklist item
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
