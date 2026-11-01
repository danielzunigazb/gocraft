# Build, test and package gocraft.
#   make          build the game and the server
#   make check    go vet and the tests
#   make release  dist/gocraft-<version>-<os>-<arch>.tar.gz and its sha256

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOOS    ?= $(shell go env GOOS)
GOARCH  ?= $(shell go env GOARCH)
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG     := gocraft-$(VERSION)-$(GOOS)-$(GOARCH)

.PHONY: all check release clean

all: gocraft gocraft-server

gocraft:
	go build -ldflags "$(LDFLAGS)" -o gocraft ./cmd/gocraft

gocraft-server:
	go build -ldflags "$(LDFLAGS)" -o gocraft-server .

check:
	go vet ./...
	go test ./...

# The game links GLFW and OpenGL through cgo, so a release is built on the platform it is for.
release: clean
	mkdir -p dist/$(PKG)
	go build -ldflags "$(LDFLAGS)" -o dist/$(PKG)/gocraft ./cmd/gocraft
	go build -ldflags "$(LDFLAGS)" -o dist/$(PKG)/gocraft-server .
	cp README.md ACCOUNT.md ROADMAP.md dist/$(PKG)/
	tar -C dist -czf dist/$(PKG).tar.gz $(PKG)
	cd dist && sha256sum $(PKG).tar.gz > $(PKG).tar.gz.sha256
	@echo dist/$(PKG).tar.gz

clean:
	rm -rf dist
