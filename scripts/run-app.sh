#!/bin/bash
set -euo pipefail

project_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$project_dir"
go_command="${GO:-go}"
platform="$("$go_command" env GOOS)"
architecture="$("$go_command" env GOARCH)"
if [ "$platform" != "darwin" ]; then
  printf 'make run requires macOS.\n' >&2
  exit 1
fi
output_dir="${OUT_DIR:-build}"
case "$output_dir" in
  /*) bundle="$output_dir/darwin-$architecture/MoleStudio.app" ;;
  *) bundle="$project_dir/$output_dir/darwin-$architecture/MoleStudio.app" ;;
esac
if pgrep -f '/go-build[^/]*/b[0-9]+/exe/mole-studio$' >/dev/null 2>&1; then
  printf '检测到旧的 go run 临时实例。请在启动它的终端按 Ctrl-C，再执行 make run；不会自动终止该实例。\n' >&2
  exit 1
fi
for process_id in $(pgrep -f '/Contents/MacOS/MoleStudio$' || true); do
  command_path="$(ps -p "$process_id" -o command=)"
  if [ "$command_path" = "$bundle/Contents/MacOS/MoleStudio" ]; then
    osascript - "$bundle" <<'APPLESCRIPT'
on run arguments
  tell application (item 1 of arguments) to quit
end run
APPLESCRIPT
    for attempt in {1..50}; do
      if ! kill -0 "$process_id" 2>/dev/null; then break; fi
      sleep 0.1
    done
    if kill -0 "$process_id" 2>/dev/null; then
      printf '现有应用未正常退出，保留原应用包。请先退出 orbit 后重试。\n' >&2
      exit 1
    fi
  fi
done
if command -v "${MYGO:-mygo}" >/dev/null 2>&1; then
  "${MYGO:-mygo}" build -o "${OUT_DIR:-build}" --skip-dmg
else
  "$go_command" run github.com/egoist/mygo/cmd/mygo@v0.6.4 build -o "${OUT_DIR:-build}" --skip-dmg
fi
codesign --verify --deep --strict "$bundle"
open -n "$bundle"
printf '已启动 %s\n权限请授予这个应用包，而不是 go-build 临时程序；临时签名重建后仍可能需要重新确认。\n' "$bundle"
