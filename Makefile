# orbit — native macOS client for the `mo` CLI.
#
#   make            build the SwiftPM executable (debug)
#   make run        build and launch straight from the checkout
#   make test       run the unit test suite
#   make app        package dist/MoleStudio.app
#   make icon       regenerate the app icon from source
#   make lint       run SwiftLint in strict mode
#   make check      lint + test, the same gate CI runs
#   make clean      remove build products
#
# Override the toolchain flags when needed, e.g. in a restricted sandbox:
#   make build SWIFT_FLAGS=--disable-sandbox

APP_NAME    := MoleStudio
CONFIG      ?= debug
SWIFT_FLAGS ?=
DIST_DIR    := dist
APP_BUNDLE  := $(DIST_DIR)/$(APP_NAME).app
BUILD_CACHE := .build/icon-tools

.PHONY: all build run test app icon icon-preview lint format check clean help

all: build

## Build the SwiftPM executable.
build:
	swift build --configuration $(CONFIG) $(SWIFT_FLAGS)

## Build and launch the app straight from the checkout.
run:
	swift run $(SWIFT_FLAGS) $(APP_NAME)

## Run the unit test suite.
test:
	swift test $(SWIFT_FLAGS)

## Package a distributable dist/MoleStudio.app bundle.
app:
	bash scripts/build-macos-app.sh release

## Regenerate the app icon assets from scripts/generate-app-icon.swift.
icon:
	@mkdir -p $(BUILD_CACHE)
	swiftc -O -module-cache-path $(BUILD_CACHE) -o $(BUILD_CACHE)/generate-app-icon scripts/generate-app-icon.swift
	$(BUILD_CACHE)/generate-app-icon icns Resources/AppIcon.icns
	$(BUILD_CACHE)/generate-app-icon single Resources/AppIcon-256.png 256

## Preview every icon size on a plate (writes /tmp/icon-preview.png).
icon-preview:
	@mkdir -p $(BUILD_CACHE)
	swiftc -O -module-cache-path $(BUILD_CACHE) -o $(BUILD_CACHE)/generate-app-icon scripts/generate-app-icon.swift
	$(BUILD_CACHE)/generate-app-icon sheet /tmp/icon-preview.png

## Lint Swift sources (requires: brew install swiftlint).
lint:
	swiftlint lint --strict

## Auto-correct lint violations where possible.
format:
	swiftlint lint --fix

## Lint and test — the same gate CI enforces.
check: lint test

## Remove build products.
clean:
	rm -rf .build $(DIST_DIR)

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
