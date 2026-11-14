# Contributing to sdboot-tui

## Dev loop

```bash
go build ./...          # or: make build
go test ./...           # or: make test
go vet ./...
gofmt -l internal/      # must be empty
```

TUI code is best checked by running the built binary against a copy of your
ESP or a fake tree:

```bash
make build
sudo ./sdboot-tui -esp /path/to/fake-esp
```

A fake ESP only needs `loader/loader.conf` and `loader/entries/*.conf`; any
referenced kernel/initrd files that do not exist will be flagged with `[!]`,
which is useful for exercising the validation paths.

## Layout

- `internal/boot` — all file parsing/writing, validation, diagnostics and EFI
  variables. It must not import anything from `internal/ui` and must not
  assume a terminal. Keep writes atomic (`writeFileAtomic`) and keep
  round-trip fidelity: comments and unknown keys in config files survive a
  load/save cycle.
- `internal/ui` — Bubble Tea model. Key handling lives in `model.go`, field
  definitions and validators in `edit.go`, per-pane rendering in
  `pane_*.go`, overlays in `view.go`.

## Ground rules for changes

1. **Follow the specs, not folklore.** Entry semantics come from
   [UAPI.1 Boot Loader Specification](https://uapi-group.org/specifications/specs/boot_loader_specification/);
   loader options from
   [`loader.conf(5)`](https://www.freedesktop.org/software/systemd/man/latest/loader.conf.html);
   EFI variables from
   [Boot Loader Interface](https://systemd.io/BOOT_LOADER_INTERFACE/).
   When systemd grows a key, add it to the parser with a lenient fallback —
   unknown keys must never make a valid config unparseable.
2. **Never write what you did not read.** Preserve raw lines/comments when
   rewriting existing files; new fields need to appear in the struct, the
   parser, `valueMap`, the edit fields, the diff builder, and the detail
   pane.
3. Anything that hides or breaks a boot (`linux` path typos, `default=`
   that matches nothing, dropped comments, EFI variable format) needs a test
   before it ships. `internal/ui/model_test.go` shows the pattern for
   headless model testing.
4. Release flow: tag `vX.Y.Z` and push — `.github/workflows/release.yml` +
   GoReleaser build the binaries and GitHub release. `AppVersion` is
   overridden at build time via ldflags; the value in `styles.go` is only a
   fallback.
