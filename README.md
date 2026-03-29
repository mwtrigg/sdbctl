# sdboot-tui

A terminal UI for managing systemd-boot — browse, view, and edit boot entries and loader configuration without touching config files directly.

> **Early development.** Basic functionality works; inline editing is in progress.

---

## Implemented

- Browse all boot entries in `/boot/loader/entries/`
- View entry fields (title, version, linux, initrd, options, etc.)
- View `loader.conf` settings (timeout, default, editor, etc.)
- Keyboard navigation with arrow keys or `j`/`k`
- Tab/Shift+Tab to switch between panes
- Basic edit mode (toggle with `e`, save with Enter, cancel with Escape)

## Not yet implemented

- **Inline field editing** — edit mode currently toggles but doesn't modify values
- Set the default boot entry
- Validate that referenced kernels and initrds exist on the ESP
- Preview changes before writing
- One-shot boot override (`LoaderEntryOneShot`)
- EFI variable inspection
- Unified Kernel Image (UKI) support
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

Requires Go 1.21+.

## Usage

```bash
sdboot-tui [-esp /path/to/esp]
```

Default ESP paths: `/boot`, `/boot/efi`, `/efi`

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Navigate entry list |
| `Tab`/`Shift+Tab` | Switch panes |
| `e` | Toggle edit mode |
| `Enter` | Save changes (in edit mode) |
| `Escape` | Cancel editing |
| `q`/`Ctrl+C` | Quit |

## Project structure

```
sdboot-tui/
├── main.go
├── internal/
│   ├── boot/       # ESP discovery, entry parsing, loader.conf parsing
│   └── ui/         # Bubble Tea TUI components
└── README.md
```

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — reusable TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling

## Contributing

This is a personal project in early development. Issues and PRs welcome.

## License

MIT
