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
kabmat -l                       list boards
kabmat -c Work                  create a board
kabmat -c Work -T               create a board with Todo, Doing, and Done
kabmat -o Work                  open a board
kabmat -a "buy milk" -o Work    append a card to the first column and exit
kabmat -a "fix bug" -o Work -C "Doing"   append into a column (name or 1-based index)
kabmat -d Work                  delete a board
kabmat -x                       print a Markdown snapshot
kabmat -x board.md -o Work      write that snapshot to a file
kabmat -f ~/notes/kabmat        use this file instead of the default data path
kabmat -t                       change data without opening the TUI
kabmat -b                       drop cards at the bottom when moving them across columns
kabmat -h                       help
kabmat -v                       version
```

`KABMAT_DATA` is the same override as `-f`. Point either one at a file in a git repo when you want the board to sync with everything else. The Markdown export is a snapshot for sharing; the indent file is what you commit if you want a lossless copy.

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
| `u` / `Ctrl-r` | undo / redo |
| `Ctrl-t` | color theme |
| `q` | quit |

### Board

| Key | Action |
| --- | --- |
| `h` / `l` | focus the column to the left / right |
| `[` / `]` | previous / next board |
| `j` / `k` | focus the card above / below |
| `g` / `G` | first / last card |
| `H` / `L` | move the card to the left / right column |
| `m` | move the focused card to a chosen column |
| `Space` | select the card; bulk move, delete, tag, and due use the selection |
| `Esc` | clear the selection |
| `w` | set the column WIP limit (`0` clears it). The header and the column title show `count/limit`, and the column turns red when it is over |
| `W` | toggle a rule: cards can enter this column only when the checklist is finished. A `*` sits on the column name |
| `A` | move the card into an Archive column (created if needed) instead of deleting it |
| `v` | hide or show archive columns. The header keeps an archived count |
| `!` | cycle priority: none, `!`, `!!` |
| `t` | set tags (`#work`). `/#work` and `f` match them |
| `T` | set a due date (`YYYY-MM-DD`) or snooze (`+3`). Overdue dates are red, today is gold |
| `o` | open the first http(s) link in the title or description |
| `X` | clear finished checklist items on the focused card |
| `K` / `J` | move the focused card up / down |
| `Ctrl-h`, `Ctrl-p` | move the column left |
| `Ctrl-l`, `Ctrl-n` | move the column right |
| `C` / `E` / `D` | create / rename / delete a column |
| `c` / `e`, `Enter` / `d` | create / edit / delete a card |
| `y` / `p` | yank / paste a card |
| `Y` | duplicate the focused card |
| `u` / `Ctrl-r` | undo / redo |
| `Ctrl-t` | color theme |
| `s` | open subtasks for the focused card |
| `/` | search titles, descriptions, checklist items, and subtask titles |
| `f` | show only cards matching the last search |
| `z` | fold subtasks on cards that are not focused |
| `q` | back |

### Editing

Text fields start in normal mode. `i` inserts, `Esc` leaves insert mode, `Enter` saves, `Esc` or `q` cancels. `Ctrl-Backspace` deletes the word before the cursor. Terminals report that key differently: macOS Terminal and Linux xterm send `Ctrl-h`, Kitty and xterm modifyOtherKeys send a CSI sequence, and Option-Backspace on macOS (Alt-Backspace on Linux) is the same action.

Card info fills the screen. `Tab` switches between the summary and the description. Long description lines wrap. `Enter` saves and returns to the board. `q` leaves without saving. A `NORMAL` or `INSERT` badge sits on the card, subtask, and column prompts so the edit mode stays visible while the dialog covers the footer. The new-column and rename-column prompts are a single box around the name.

`Ctrl-t` opens color themes. `linux-dex` is the default (the Ghostty palette: blue `#4C7CFF`, mint `#7DE1D0`, text `#CDD6F4` on `#313244`). `catppuccin-mocha` is the other built-in. `c` adds a theme and writes it to `themes` beside the board file, `e` edits a color (`0`–`255` or `#rrggbb`), and `d` deletes a theme you added. The chosen name is stored in `theme`. A hand-written block looks like this:

```text
dusk
accent 111
paper 236
ink 15
```

Keys are `ink`, `paper`, `accent`, `warn`, `gold`, `tag`, `muted`, `pick`, `card`, and `archive`. A missing key keeps the linux-dex color.

`c` opens the checklist and `s` opens subtasks once the summary is set. Both lists use most of the screen. `j`/`k` select, `g`/`G` jump to the ends, `J`/`K` reorder, `Space` toggles done, `c` adds, `e` edits, and `d` deletes. `q` on either list saves and returns to the board. A subtask has its own summary and a tall description field.

On the card screen, `!` cycles priority, `t` edits tags, and `T` edits the due date. Those values are saved with the card.

A card that has sat in the same column for 7 days is drawn in a dim border with its age (`14d`). Moving it to another column resets that clock. Priority, tags, and the due date sit on the card itself so a column can be scanned without opening anything.

`J` and `K` reorder boards on the board list. On a board they move the focused card. `Shift+Up` and `Shift+Down` do the same reorder.

## Data

Boards are stored in `$HOME/.local/share/kabmat/data`. A copy is written to `data_bkp` on the first save of a session. The last board, column, and card are stored in `place` and restored on the next launch. The focused card shows the first line of its description. The file is the same indentation format as the original kabmat:

```text
board name
 column {wip:3,check}
  card
   @pri:2 due:2026-10-20 tag:work moved:2026-10-01
   -open checklist item
   +done checklist item
   >-open subtask
   >> subtask description
   >+done subtask
    description line
    another description line
 Archive {archive}
  old card
```

Column flags are optional: `wip:N`, `check` (finished checklist required to enter), and `archive`. Card flags are optional and live on one `@` line. Files written by older versions of kabmat still open.

## Layout

```text
cmd/kabmat          command-line entry point
internal/store      board file read and write
internal/tui        terminal interface
```

## License

[MIT](LICENSE)
