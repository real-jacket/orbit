#!/bin/bash
# Lists what changed in tw93/Mole since the version orbit's rules were ported from.
# Every ported rule carries "Ported from tw93/Mole V<x.y.z> <file> <function-or-variable>";
# this script diffs exactly those functions and variables between that version and a newer
# one, then lists other changed rule files nobody has looked at yet.
#
# usage: scripts/mole-sync.sh [target-tag]   (default: latest release)
#        MOLE_SRC=/path/to/Mole scripts/mole-sync.sh   (reuse a local clone)
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
src=${MOLE_SRC:-"${TMPDIR:-/tmp}/orbit-mole-sync"}

if [[ ! -d "$src/.git" ]]; then
    git clone -q --filter=blob:none https://github.com/tw93/Mole.git "$src"
fi
git -C "$src" fetch -q --tags origin

target=${1:-$(git -C "$src" tag --list 'V*' --sort=-v:refname | head -1)}

# "V1.70.0<TAB>lib/optimize/tasks.sh<TAB>opt_sqlite_vacuum" for every marker in the repo.
markers=$(grep -rhoE 'Ported from tw93/Mole V[0-9.]+ [A-Za-z0-9_./-]+\.sh( [A-Za-z0-9_]+)?' \
    "$root/internal" "$root"/*.go "$root/Resources/src-ui/index.html" 2>/dev/null |
    sed -E 's/^Ported from tw93\/Mole //' | awk '{print $1"\t"$2"\t"$3}' | sort -u || true)

if [[ -z "$markers" ]]; then
    echo "No 'Ported from tw93/Mole' markers found." >&2
    exit 1
fi

# body prints a shell function, or the lines assigning a variable, from file at rev.
body() {
    local rev=$1 file=$2 name=$3
    git -C "$src" show "$rev:$file" 2>/dev/null | awk -v n="$name" '
        $0 ~ "^(function )?" n "[ ]*\\(\\)" { infn = 1 }
        infn { print; if ($0 ~ /^}/) infn = 0; next }
        n != "" && $0 ~ "(^|[ ])(readonly |export |local |declare [-a-zA-Z]+ )?" n "=" { print }'
}

echo "# Mole sync report: ported versions -> $target"
echo

changed=0
seen_files=""
while IFS=$'\t' read -r from file name; do
    seen_files+="$file"$'\n'
    [[ "$from" == "$target" ]] && continue
    if [[ -z "$name" ]]; then
        if ! git -C "$src" diff --quiet "$from" "$target" -- "$file"; then
            echo "## $file (whole file, ported from $from) changed"
            changed=$((changed + 1))
        fi
        continue
    fi
    old=$(body "$from" "$file" "$name")
    new=$(body "$target" "$file" "$name")
    if [[ "$old" != "$new" ]]; then
        changed=$((changed + 1))
        echo "## $file $name (ported from $from)"
        if [[ -z "$new" ]]; then
            echo "   removed or renamed in $target"
        else
            diff -u <(printf '%s\n' "$old") <(printf '%s\n' "$new") | tail -n +3 || true
        fi
        echo
    fi
done <<<"$markers"

oldest=$(cut -f1 <<<"$markers" | sort -V | head -1)
echo "## Rule files changed since $oldest that no marker references"
git -C "$src" diff --name-only "$oldest" "$target" -- bin lib | while read -r f; do
    grep -qxF "$f" <<<"$seen_files" || echo "   $f"
done
echo
echo "$changed ported rule(s) changed. After porting the changes, bump their markers to $target."
