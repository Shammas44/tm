.PHONY: build build-linux test clean install uninstall install-user uninstall-user install-go install-linux uninstall-linux install-linux-user uninstall-linux-user

BIN_DIR := bin
BINARY := $(BIN_DIR)/tm
BINARY_LINUX := $(BIN_DIR)/tm-linux-amd64

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
LINUX_BIN_NAME ?= tm

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) cmd/tm/main.go

build-linux:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_LINUX) cmd/tm/main.go

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR)

install: build
	@mkdir -p $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY) $(DESTDIR)$(BINDIR)/tm

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/tm

install-user:
	@$(MAKE) install PREFIX=$(HOME)/.local

uninstall-user:
	@$(MAKE) uninstall PREFIX=$(HOME)/.local

install-linux: build-linux
	@mkdir -p $(DESTDIR)$(BINDIR)
	install -m 755 $(BINARY_LINUX) $(DESTDIR)$(BINDIR)/$(LINUX_BIN_NAME)

uninstall-linux:
	rm -f $(DESTDIR)$(BINDIR)/$(LINUX_BIN_NAME)

install-linux-user:
	@$(MAKE) install-linux PREFIX=$(HOME)/.local

uninstall-linux-user:
	@$(MAKE) uninstall-linux PREFIX=$(HOME)/.local

install-go:
	go install ./cmd/tm


