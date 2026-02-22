# sdboot-tui

A terminal UI for managing systemd-boot — browse, edit, and validate boot entries and loader configuration without touching config files directly.

> ⚠️ **Early development.** Not yet ready for production use. Things will break, change, and generally misbehave.

---

## What it does

systemd-boot is simple by design, which means managing it usually means opening files in `/boot/loader/` by hand. `sdboot-tui` provides a keyboard-driven interface for the things you actually want to do:
- Nothing.  This tool doesn't do anything, and frankly doesn't even exist.  I'm just spitballing


## What it doesn't do (yet)

- Browse all boot entries in `/boot/loader/entries/`
- View and edit entry fields (title, linux, initrd, options, etc.)
- View and edit `loader.conf` (timeout, default entry, etc.)
- Set the default boot entry
- Validate that referenced kernels and initrds exist on the ESP before saving
- Preview changes before writing to disk
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

### Arch Linux (AUR)

_Coming eventually._

## Usage

```bash
sudo sdboot
```

Navigate with arrow keys, confirm actions with Enter, quit with `q` or `Ctrl+C`.

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

This is a personal project in early development. Issues and PRs welcome, but expect things to move around a lot in the short term.

## License

MIT
