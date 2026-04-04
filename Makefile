all: build

GIT_COMMIT:=$(shell git rev-list -1 HEAD)
GIT_LAST_TAG:=$(shell git describe --abbrev=0 --tags)
GIT_EXACT_TAG:=$(shell git name-rev --name-only --tags HEAD)

VERSION_PATH:=github.com/guttermonk/bleamd
LDFLAGS:=-X main.GitCommit=${GIT_COMMIT} \
	-X main.GitLastTag=${GIT_LAST_TAG} \
	-X main.GitExactTag=${GIT_EXACT_TAG}

PREFIX  ?= /usr/local
DESTDIR ?=

BINDIR  := $(DESTDIR)$(PREFIX)/bin
ICONDIR := $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps
APPDIR  := $(DESTDIR)$(PREFIX)/share/applications

build:
	go build -ldflags "$(LDFLAGS)" .

install: build
	install -Dm755 bleamd        $(BINDIR)/bleamd
	install -Dm644 bleamd-icon.svg $(ICONDIR)/bleamd-icon.svg
	install -Dm644 bleamd.desktop  $(APPDIR)/bleamd.desktop

uninstall:
	rm -f $(BINDIR)/bleamd
	rm -f $(ICONDIR)/bleamd-icon.svg
	rm -f $(APPDIR)/bleamd.desktop

releases:
	gox -ldflags "$(LDFLAGS)" -output "dist/{{.Dir}}_{{.OS}}_{{.Arch}}"

.PHONY: build install uninstall releases