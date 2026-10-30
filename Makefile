# orbit — native macOS client for the `mo` CLI.
#
#   make            build the Go executable
#   make run        build and launch straight from the checkout
#   make test       run the unit test suite
#   make vet        run the Go static checker
#   make dev        run the MyGo dev server (live reload, needs the `mygo` CLI)
#   make app        package the .app bundle and a .dmg into build/
#   make check      vet + test, the same gate CI runs
#   make clean      remove build products
#
# Override the toolchain when needed, e.g. in a restricted sandbox:
#   make build GO=/path/to/go MYGO=/path/to/mygo
#
## Go environment — sandbox / toolchain setup, explained at the top of this file
#
# A sandboxed shell often gets a read-only module cache and build cache, and
# then `go build`/`go test` fail with "permission denied" on $HOME/go/pkg/mod.
# Point all three at a writable directory before running anything:
#
#   export GOPATH=/tmp/gopath GOMODCACHE=/tmp/gopath/pkg/mod GOCACHE=/tmp/gocache
#
# The targets that shell out to MyGo need the CLI on PATH. It is not vendored
# by this repo, so install it from the version go.mod pins:
#
#   go install github.com/egoist/mygo/cmd/mygo@v0.6.4
#   export PATH="$(go env GOPATH)/bin:$PATH"   # or: /tmp/gopath/bin
#
# `make doctor` reports whether the CLI and the rest of the toolchain are there.
#
# `make app` writes into $(OUT_DIR)/<goos>-<goarch>/ — on Apple silicon that is
# build/darwin-arm64/MoleStudio.app plus a .dmg beside it. The per-platform
# subdirectory comes from MyGo, so do not expect build/MoleStudio.app.

APP_NAME := MoleStudio
BIN      := mole-studio
OUT_DIR  := build
MYGO     ?= mygo
GO       ?= go
NODE     ?= node

.PHONY: all build run test test-ui-webkit vet dev app app-nodmg generate set-version doctor check clean help

all: build

## Build the Go executable at ./mole-studio. On its own it shows MyGo's
## "No frontend" page, because nothing is embedded: run it as
## MYGO_FRONTEND_DIST=Resources/src-ui ./mole-studio, or use `make run`.
build:
	$(GO) build -v -o $(BIN) .

## Build and launch the app straight from the checkout. No `mygo` CLI needed:
## a plain `go run` has no embedded frontend (mygo_frontend_gen.go is only
## written by `mygo build`, and it is gitignored), so point MyGo's dev fallback
## at the files on disk. Without this the window shows its "No frontend" page.
run:
	MYGO_FRONTEND_DIST=Resources/src-ui $(GO) run .

## Run the unit test suite.
test:
	$(GO) test ./... -count=1
	$(NODE) scripts/test-ui.cjs

## Render and verify isolated UI fixtures in macOS WebKit (requires the Xcode command-line tools).
test-ui-webkit:
	$(NODE) scripts/webkit-fixtures.cjs .review/webkit
	xcrun swiftc scripts/test-webkit.swift -o .review/webkit/test-webkit
	.review/webkit/test-webkit Resources/src-ui/index.html .review/webkit

## Run the Go static checker.
vet:
	$(GO) vet ./...

## Run the MyGo dev server with live reload (requires the `mygo` CLI).
dev:
	$(MYGO) dev

## Package the app bundle and a .dmg (build/darwin-arm64/MoleStudio.app).
app:
	$(MYGO) build

## Package the app bundle only, no .dmg.
app-nodmg:
	$(MYGO) build --skip-dmg

## Regenerate the typed TypeScript client (mygo.ts) from the bound services.
generate:
	$(MYGO) generate

## Write a version into mygo.json — the one place the version lives locally.
## Releases take their version from the Git tag instead; see README "Releases".
set-version:
	@test -n "$(VERSION)" || { echo "usage: make set-version VERSION=x.y.z" >&2; exit 2; }
	scripts/set-version.sh "$(VERSION)"

## Check the toolchain MyGo needs (Go, the CLI, packaging dependencies).
doctor:
	$(MYGO) doctor

## Vet and test — the same gate CI enforces.
check: vet test

## Remove build products.
clean:
	rm -rf $(BIN) $(OUT_DIR) .mygo

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
