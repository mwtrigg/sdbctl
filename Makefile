BINARY    := sdboot-tui
PREFIX    ?= /usr/local
BINDIR    := $(PREFIX)/bin

# Version from git tag, falling back to "dev"
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X github.com/mwtrigg/sdboot-tui/internal/ui.AppVersion=$(VERSION)

.PHONY: build install uninstall test clean screenshots

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	install -Dm755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY)

# Regenerate the README screenshots. Needs pyte (pip install pyte) and a
# Chrome/Chromium binary; see tools/screenshot.py.
screenshots: build
	python3 tools/screenshot.py --out assets

# Verify sdboot-tui's show/hide predictions against the real systemd-boot,
# booted under qemu+OVMF with a synthetic ESP. Needs qemu, OVMF, dosfstools,
# sudo and the packaged systemd-boot EFI binary; see tools/bootoracle.py.
boot-oracle: build
	python3 tools/bootoracle.py
