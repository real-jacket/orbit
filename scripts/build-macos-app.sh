#!/usr/bin/env bash
#
# Packages MoleStudio.app — a self-contained macOS bundle around the SwiftPM executable.
#
# Usage:
#   scripts/build-macos-app.sh [debug|release]     # default: release
#
# Environment:
#   SWIFT_FLAGS   extra flags forwarded to `swift build` (e.g. --disable-sandbox)
#
set -euo pipefail

CONFIGURATION="${1:-release}"
APP_NAME="MoleStudio"
BUNDLE_ID="io.github.real-jacket.mole-studio"
DISPLAY_NAME="Mole Studio Pro"
MARKETING_VERSION="5.0.0"
MIN_MACOS="13.0"

case "${CONFIGURATION}" in
    debug | release) ;;
    *)
        echo "error: configuration must be 'debug' or 'release' (got '${CONFIGURATION}')" >&2
        exit 2
        ;;
esac

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist"
APP_BUNDLE="${DIST_DIR}/${APP_NAME}.app"
CONTENTS_DIR="${APP_BUNDLE}/Contents"
UI_SOURCE_DIR="${REPO_ROOT}/Resources/src-ui"

cd "${REPO_ROOT}"

echo "==> Building ${APP_NAME} (${CONFIGURATION})"
# shellcheck disable=SC2086
swift build --configuration "${CONFIGURATION}" ${SWIFT_FLAGS:-}

# shellcheck disable=SC2086
BIN_DIR="$(swift build --configuration "${CONFIGURATION}" ${SWIFT_FLAGS:-} --show-bin-path)"
BINARY="${BIN_DIR}/${APP_NAME}"

if [[ ! -x "${BINARY}" ]]; then
    echo "error: expected an executable at ${BINARY}" >&2
    exit 1
fi

if [[ ! -f "${UI_SOURCE_DIR}/index.html" ]]; then
    echo "error: missing UI entry point at ${UI_SOURCE_DIR}/index.html" >&2
    exit 1
fi

echo "==> Assembling ${APP_BUNDLE}"
rm -rf "${APP_BUNDLE}"
mkdir -p "${CONTENTS_DIR}/MacOS" "${CONTENTS_DIR}/Resources"

cp "${BINARY}" "${CONTENTS_DIR}/MacOS/${APP_NAME}"
cp -R "${UI_SOURCE_DIR}" "${CONTENTS_DIR}/Resources/src-ui"

cat > "${CONTENTS_DIR}/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>zh_CN</string>
	<key>CFBundleDisplayName</key>
	<string>${DISPLAY_NAME}</string>
	<key>CFBundleExecutable</key>
	<string>${APP_NAME}</string>
	<key>CFBundleIdentifier</key>
	<string>${BUNDLE_ID}</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>${APP_NAME}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>${MARKETING_VERSION}</string>
	<key>CFBundleVersion</key>
	<string>${MARKETING_VERSION}</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.utilities</string>
	<key>LSMinimumSystemVersion</key>
	<string>${MIN_MACOS}</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>GPL-3.0-or-later. A client for the tw93/mole CLI.</string>
</dict>
</plist>
PLIST

printf 'APPL????' > "${CONTENTS_DIR}/PkgInfo"

if command -v codesign >/dev/null 2>&1; then
    echo "==> Ad-hoc signing"
    codesign --force --sign - "${APP_BUNDLE}" >/dev/null 2>&1 ||
        echo "warning: ad-hoc codesign failed; the bundle still runs locally" >&2
fi

echo "==> Done: ${APP_BUNDLE}"
