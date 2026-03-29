# sdboot-tui

A terminal UI for managing systemd-boot — browse, view, and edit boot entries and loader configuration without touching config files directly.

---

## Features

- Browse all boot entries in `loader/entries/`
- View and edit entry fields (title, version, linux, initrd, options, etc.)
- View and edit `loader.conf` settings (timeout, default, editor, console-mode, etc.)
- Inline editing with field-by-field navigation and required-field validation
- Tab completion for ESP filesystem paths (linux, initrd, devicetree fields)
- Context-sensitive help for each editable field
- File preview overlay — view the raw `.conf` file for any entry or `loader.conf`
- Last-modified timestamp shown for entries and loader config
- Keyboard navigation with arrow keys or `j`/`k`
- Tab/Shift+Tab to switch between panes
- Nord color theme

## Not yet implemented

- Set the default boot entry interactively
- Validate that referenced kernels and initrds exist on the ESP
- One-shot boot override (`LoaderEntryOneShot`)
- EFI variable inspection
- Anything involving `efibootmgr`

## Requirements

- Linux with systemd-boot installed
- ESP mounted at `/boot`, `/boot/efi`, or `/efi` (standard locations)
- Root or appropriate group membership to write to the ESP

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

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Navigate entry list |
| `Tab`/`Shift+Tab` | Switch panes |
| `e` | Enter edit mode |
| `↑`/`↓` | Navigate fields (in edit mode) |
| `Tab` | Next field / path completion (in edit mode) |
| `Enter` | Save changes (in edit mode) |
| `Escape` | Cancel editing |
| `p` | Preview raw file (entries or loader.conf) |
| `?` | Toggle help overlay |
| `q`/`Ctrl+C` | Quit |

## Project structure

```
sdboot-tui/
├── main.go
├── internal/
│   ├── boot/               # ESP discovery, entry and loader.conf parsing, validation
│   └── ui/                 # Bubble Tea TUI
│       ├── model.go        # Model, Update, key handling
│       ├── view.go         # Layout, overlays, header, help bar
│       ├── edit.go         # Inline editing, tab completion, field definitions
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

This is a personal project in early development. Issues and PRs welcome.

## License

MIT
