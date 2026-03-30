# sdboot-tui

A terminal UI for managing systemd-boot — browse, create, edit, and delete boot entries and loader configuration without touching config files directly.

---

## Features

- Browse all boot entries in `loader/entries/` and UKIs in `EFI/Linux/`
- Create new boot entries with guided field editing
- Delete existing entries with confirmation
- Set the default boot entry with a single keypress (`s`), reflected immediately in `loader.conf`
- View and edit all entry fields: title, version, linux, initrd, options, machine-id, architecture, sort-key, devicetree, devicetree-overlay
- View and edit all `loader.conf` settings: timeout, default, editor, console-mode, auto-entries, auto-firmware, auto-reboot, auto-poweroff, beep, reboot-on-error, log-level
- Add new `initrd` or `devicetree-overlay` lines to an existing entry with `Ctrl+N` in edit mode
- Per-field validation: path format, illegal characters, constrained values (architecture, console-mode, log-level, etc.)
- ESP file-existence validation — entries with missing kernel or initrd files are flagged with a `[!]` badge
- One-shot boot override: press `o` to set `LoaderEntryOneShot` for the selected entry (boots it once on next restart, then clears automatically). Shows a `[1x]` badge. EFI systems only.
- Console-mode resolution preview: press `r` from the loader pane to see a mock systemd-boot screen at each EFI text mode (80×25, 80×50, highest, auto, max, keep), cycling with arrow keys
- Inline editing with field-by-field navigation
- Tab completion for ESP filesystem paths (linux, initrd, devicetree fields)
- Context-sensitive help for each editable field
- Save confirmation with diff view
- File preview overlay — view the raw `.conf` file for any entry or `loader.conf`
- Last-modified timestamp shown for entries and loader config
- Keyboard navigation with arrow keys or `j`/`k`
- Tab/Shift+Tab to switch between panes
- Nord color theme

## Not yet implemented

- EFI variable inspection beyond one-shot
- Anything involving `efibootmgr` (boot order, EFI boot entries)

## Requirements

- Linux with systemd-boot installed
- ESP mounted at `/boot`, `/boot/efi`, or `/efi` (standard locations)
- Root or appropriate group membership to write to the ESP
- For one-shot boot: EFI system with `/sys/firmware/efi/efivars` accessible

## Installation

### From source

```bash
git clone https://codeberg.org/mwtrigg/sdboot-tui
cd sdboot-tui
go build -o sdboot-tui .
sudo mv sdboot-tui /usr/local/bin/
```

Requires Go 1.24+.

## Usage

```bash
sudo sdboot-tui [-esp /path/to/esp]
```

Default ESP paths checked: `/boot`, `/boot/efi`, `/efi`

The `-esp` flag is useful in chroot environments (e.g. during Arch installation or system recovery):

```bash
sudo sdboot-tui -esp /mnt/boot
```

Note: one-shot boot (`o` key) writes to `/sys/firmware/efi/efivars`. From inside a chroot this will only work if `/sys` is bind-mounted into the chroot:

```bash
mount --rbind /sys /mnt/sys
```

Without that, the efivars path will not exist inside the chroot and the one-shot feature will be silently disabled.

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Navigate entry list |
| `Tab`/`Shift+Tab` | Switch panes |
| `n` | New boot entry |
| `D` | Delete selected entry |
| `s` | Set selected entry as default |
| `o` | Toggle one-shot boot (EFI only) |
| `e` | Enter edit mode |
| `↑`/`↓` | Navigate fields (in edit mode) |
| `Tab` | Next field / path completion (in edit mode) |
| `Ctrl+N` | Add new initrd/overlay line (in edit mode) |
| `Enter` | Save changes (in edit mode) |
| `Escape` | Cancel editing / discard new entry |
| `p` | Preview raw file (entries or loader.conf) |
| `r` | Console-mode resolution preview (loader pane) |
| `?` | Toggle help overlay |
| `q`/`Ctrl+C` | Quit |

## Project structure

```
sdboot-tui/
├── main.go
├── internal/
│   ├── boot/               # ESP discovery, entry and loader.conf parsing, validation, EFI variables
│   └── ui/                 # Bubble Tea TUI
│       ├── model.go        # Model, Update, key handling
│       ├── view.go         # Layout, overlays, header, help bar
│       ├── edit.go         # Inline editing, tab completion, field definitions, validators
│       ├── pane_entries.go # Boot entries list pane
│       ├── pane_detail.go  # Entry details / edit pane
│       ├── pane_loader.go  # loader.conf view / edit pane
│       ├── styles.go       # Lip Gloss styles (Nord palette)
│       └── helpers.go      # Utility functions
└── testdata/               # Sample ESP layout for tests
```

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — reusable TUI components (textinput, spinner)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — terminal styling

## Contributing

This is a personal project in active development. Issues and PRs welcome.

## License

MIT
