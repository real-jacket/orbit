#!/usr/bin/env bash
#
# set-version.sh — write the release version into mygo.json.
#
# mygo.json is the only place the number lives. `mygo build` puts it in the
# bundle's Info.plist and the .dmg name, and the app reads it back from the
# bundle at runtime (see app.go), so the About panel, the Finder, the 体检 row
# and the release page cannot disagree.
#
# The version has to be MAJOR.MINOR.PATCH, with nothing after it: macOS takes
# CFBundleShortVersionString as three numbers, so "0.2.0-rc.1" would ship a
# bundle that does not report the tag it came from.
#
# Usage:
#   scripts/set-version.sh 0.2.0
#   make set-version VERSION=0.2.0
#
# The release workflow runs this with the pushed tag, then builds; there is no
# second number to edit.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <version>   e.g. $0 0.2.0" >&2
    exit 2
fi

version="${1#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "$0: '$1' is not a MAJOR.MINOR.PATCH version" >&2
    exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
file="$root/mygo.json"

if [[ ! -f "$file" ]]; then
    echo "$0: $file not found" >&2
    exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
jq --arg version "$version" '.version = $version' "$file" > "$tmp"
mv "$tmp" "$file"

echo "mygo.json: version = $version"
