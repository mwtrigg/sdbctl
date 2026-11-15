# Contributing to sdbctl

## Dev loop

```bash
go build ./...          # or: make build
go test ./...           # or: make test
go vet ./...
gofmt -l internal/      # must be empty
```

The editor is best checked by running the built binary against a copy of your
ESP or a fake tree:

```bash
make build
sudo ./sdbctl edit -esp /path/to/fake-esp
```

A fake ESP only needs `loader/loader.conf` and `loader/entries/*.conf`; any
referenced kernel/initrd files that do not exist will be flagged with `[!]`,
which is useful for exercising the validation paths.

## Layout

- `internal/boot` — file parsing/writing, validation, diagnostics, snapshots
  and EFI variables. It must not import `internal/cli` or `internal/ui` and
  must not assume a terminal. Keep writes atomic and keep round-trip
  fidelity: comments and unknown keys in config files survive a load/save
  cycle.
- `internal/cli` — the command surface, in two layers:
  - `api.go` holds every operation with typed, JSON-tagged results and no
    printing. This is the contract shared by the CLI, `sdbctl mcp` and the
    qemu oracle, so all three agree by construction.
  - `cli.go` parses arguments, renders text and JSON, and maps failures to
    exit codes.
- `internal/ui` — the Bubble Tea editor.

## Ground rules

1. **Follow the specs, not folklore.** Entry semantics come from
   [UAPI.1 Boot Loader Specification](https://uapi-group.org/specifications/specs/boot_loader_specification/);
   loader options from
   [`loader.conf(5)`](https://www.freedesktop.org/software/systemd/man/latest/loader.conf.html);
   EFI variables from
   [Boot Loader Interface](https://systemd.io/BOOT_LOADER_INTERFACE/).
   When systemd grows a key, add it with a lenient fallback — an unknown key
   must never make a valid config unparseable.
2. **Behaviour depends on the boot loader version.** v255/v257 and v258+
   disagree about conflicting image keys, and `uki`/`profile`/`extra` only
   exist from v258/v259/v261. The version is read from the `.sdmagic` section
   of the loader binary on the ESP; keep the thresholds in
   `internal/boot/validate.go` documented with the tag they came from.
3. **Never write what you did not read.** Preserve raw lines and comments
   when rewriting existing files. New fields need the struct, the parser,
   `valueMap`, the edit fields, the diff builder and the detail pane.
4. **Anything that hides or breaks a boot needs a test.** `internal/cli` has
   the headless command tests, `internal/boot` has the fuzz targets, and
   `internal/ui/model_test.go` shows the pattern for headless model testing.
5. **Destructive changes must be snapshotted first** — use
   `model.backupBefore()` / `API.mutate()` so `undo` and `--restore` can roll
   them back.
6. **The JSON schema is a public contract.** `sdbctl --json` and the MCP
   `structuredContent` share it; changing a field name is a breaking change
   for scripts and agents.
7. **MCP is read-only by default.** New tools are added to the read-only set
   unless they mutate; mutating tools must be listed in `writeToolNames` so
   they are only advertised with `--allow-write`.
8. The README screenshots are generated, not hand-made: `make screenshots`
   (needs `pip install pyte` and a Chrome binary) renders the real binary in
   a pseudo-terminal. Re-run it when the UI changes.
9. Release flow: tag `vX.Y.Z` and push — `.github/workflows/release.yml` +
   GoReleaser build the binaries and GitHub release. `AppVersion` and
   `cli.Version` are overridden at build time via ldflags; the values in the
   source are only fallbacks.

## Test tiers

| Tier | Command | Needs |
|------|---------|-------|
| Unit + fuzz seeds | `go test ./...` | nothing |
| Fuzzing | `go test ./internal/boot/ -run '^$' -fuzz FuzzParseEntry -fuzztime 60s` | nothing |
| Editor headless | `go test ./internal/ui/` | nothing |
| **Boot oracle** | `make boot-oracle` | qemu, OVMF, dosfstools, sudo, a packaged systemd-boot EFI binary |
| Screenshots | `make screenshots` | pyte, Chrome |

The oracle boots the real systemd-boot under qemu + OVMF against a synthetic
ESP and compares what sdbctl predicts with the menu the loader actually
renders. It is the only way to catch "our idea of systemd-boot's rules is
wrong", and it has already caught two such bugs. The CI workflow runs it too.
