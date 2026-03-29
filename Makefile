BINARY    := sdboot-tui
PREFIX    ?= /usr/local
BINDIR    := $(PREFIX)/bin

# Version from git tag, falling back to "dev"
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X github.com/mwtrigg/sdboot-tui/internal/ui.AppVersion=$(VERSION)

.PHONY: build install uninstall test clean

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
