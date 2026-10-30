# Contributing to orbit

Thanks for taking a look. This is a small project, so the process is deliberately light.

## Getting set up

```bash
git clone https://github.com/real-jacket/orbit.git
cd orbit
make run     # build and launch
make check   # vet + test
```

You need Go 1.27 or newer (the module declares `go 1.27.3`). The packaging targets also need the MyGo
CLI, which is not vendored:

```bash
go install github.com/egoist/mygo/cmd/mygo@v0.6.4
export PATH="$(go env GOPATH)/bin:$PATH"
make doctor   # confirms the CLI and the rest of the toolchain are there
```

You will also want the upstream CLI available, since the app shells out to it:

```bash
brew install mole
mo --version
```

## Before you open a pull request

Run these and make sure they pass:

```bash
make check       # go vet ./... && go test ./... -count=1
make build       # the executable compiles
make app-nodmg   # the bundle packages
```

Then confirm the generated TypeScript client is current, because CI fails the PR if it is not:

```bash
make generate
git diff --exit-code -- mygo.ts
```

CI runs the same steps on `macos-latest`, using the Go version from `go.mod`.

## Tests

`make test` runs the Go suite and the dependency-free frontend checks (Node.js 18+ required for development only).
`node scripts/test-ui.cjs` checks inline JavaScript syntax, selection arithmetic, uninstall mode boundaries,
the frozen confirmation path set and duplicate submission. The Go tests live next to the code they cover:

- **`internal/model`** — the safety invariants. Asserts `protected` rows cannot be selected even by
  calling the toggle directly, and that dry-run defaults to on.
- **`internal/scanner`** — the numeric helpers and the refactor invariants: the signature table is
  non-empty and unique, the payload keys are unchanged, and leftover rows keep the fields the UI reads.
- **`internal/analyze`** — payload shaping and tile layout: sizes add up to the total, colours follow
  cleanability, and each tile starts where the previous one ended.
- **`internal/power`** — the `ioreg` / `sysctl` text parsers, including the percentage caps and the
  `AppleRawMaxCapacity` fallback.
- **`internal/trayicon`** — the menu-bar mark: every pixel is pure black, the alpha carries the shape,
  and the ring is cut where it passes behind the planet.

If you change `Tier`, `Module`, or the layout math, extend the existing file rather than adding a new one.

For layout or interaction changes, `make test-ui-webkit` loads the actual HTML in an isolated macOS
WebKit view and checks all modules, both themes, the minimum window, search, dialogs and receipts.
It uses clearly labelled fixture data, never invokes native cleanup, and saves screenshots and
`webkit-report.json` under the ignored `.review/webkit/` directory. Animations are disabled only in
the test view so screenshots capture the final state. This command needs the Xcode command-line tools.

## App icon

`assets/AppIcon-256.png` and `assets/AppIcon.icns` are generated — do not edit them by hand, and do not
replace them with a dropped-in bitmap. Change `scripts/generate-app-icon.swift` and re-render:

```bash
swift scripts/generate-app-icon.swift icns   assets/AppIcon.icns
swift scripts/generate-app-icon.swift single assets/AppIcon-256.png 256
```

`assets/icon.png` is the copy `mygo.json` reads to build the bundle icon, so refresh it too
(`cp assets/AppIcon-256.png assets/icon.png`). Commit the script and all three outputs.

The renderer can also draw a contact sheet, which is the fastest way to check the 16px and 32px ends
before committing — that is where marks usually fall apart:

```bash
swift scripts/generate-app-icon.swift sheet /tmp/icon-preview.png
```

The **menu bar** icon is separate: `internal/trayicon` draws it at runtime as a template PNG and never
reads the files above, so a change to the brand mark does not automatically follow it.

## Code style

- **Go** — `gofmt` is the source of truth; CI fails on any unformatted file. Tabs, which `gofmt` gives
  you for free.
- **HTML/CSS/JS** — 2-space indent. `Resources/src-ui/index.html` is intentionally a single file with no
  build step. Keep it that way unless there is a strong reason not to.
- **Commits** — [Conventional Commits](https://www.conventionalcommits.org/) prefixes
  (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`). Imperative subject line, under ~72 chars.

## How the pieces fit

**The UI is HTML, not SwiftUI.** `Resources/src-ui/index.html` is the shipping surface and the port did
not touch it. It talks to the shell through one object, `window.webkit.messageHandlers.moleNative`; MyGo
has no `WKScriptMessageHandler`, so `preload.go` recreates that object in a `PreloadScript` and forwards
each message to `NativeBridge.Dispatch` in `bridge.go`. That switch is the one entry point for everything
the page asks the native side to do.

The reverse direction has no equivalent to `evaluateJavaScript` on a channel the page listens to, so
`push.go` serialises a payload and calls the page's global callbacks by name. If you rename one of those
globals, change both sides:

| Direction | Names |
| --- | --- |
| JS → Go | `startWindowDrag`, `toggleWindowZoom`, `revealInFinder`, `copyText`, `killProcess`, `trashRealPaths`, `requestRealSystemScan`, `requestDiskAnalyze`, `requestRealTelemetry`, `executeOptimize`, `toggleCaffeinate`, `setTheme`, `menubar*` |
| Go → JS | `onNativeRealScanComplete`, `onNativeRealTelemetry`, `onNativeDiskAnalyze`, `switchModule`, `openConfirmSheet`, `openDoctorModal`, `stepLiveTelemetry` |

Four actions the page can post — `addWhitelist`, `restoreTrashSnapshot`, `runDoctorReCheck`, `setFanMode`
— hit the `default` branch on purpose: the page keeps that state itself, and the Swift app ignored them
too.

[`docs/go-port.md`](docs/go-port.md) maps every Swift file to its Go counterpart and lists the
behavioural differences. Read it before porting anything back.

## Adding a module

Modules are the `Module` type in `internal/model/model.go`. A new case needs its raw value (the string the
HTML already uses for the sidebar), a `Label`, a sidebar entry and section in `index.html`, and — if it
maps to a real subcommand — a call site in `internal/analyze` or `internal/scanner`.

## Safety changes need extra care

Anything touching deletion, the whitelist, or `Tier` gets a closer review. The invariants:

- `tier == "protected"` items must never become selectable.
- Destructive operations must remain previewable before execution.
- Dry-run must stay the default.
- The whitelist file format must stay compatible with upstream `mo` (`~/.config/mole/whitelist`,
  newline-separated, `#` comments allowed).

## Reporting bugs

Use the issue template. The most useful reports include your macOS version, `mo --version`, and the
exact steps that reproduced the problem. Since this app deletes files, please say whether the problem
occurred in dry-run or for real.
