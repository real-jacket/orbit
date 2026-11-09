<div align="center">

<img src="assets/AppIcon-256.png" width="140" alt="orbit app icon">

# orbit

**A native macOS disk cleaner, inspired by the [`mo`](https://github.com/tw93/mole) CLI.**

Go · MyGo · WKWebView — no Electron, no bundled runtime.

[![CI](https://github.com/real-jacket/orbit/actions/workflows/ci.yml/badge.svg)](https://github.com/real-jacket/orbit/actions/workflows/ci.yml)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
[![macOS 13+](https://img.shields.io/badge/macOS-13%2B-black.svg)](#requirements)
[![Go 1.27+](https://img.shields.io/badge/Go-1.27%2B-00ADD8.svg)](go.mod)
[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/orbit)

</div>

> The application is **orbit**. It is a standalone desktop app; it does not need `mo` installed.
> The bundle is `orbit.app`; the bundle identifier stays `com.mymoles.mole-studio`, so permissions and data from earlier `MoleStudio` builds carry over.

---

## What it is

orbit is a desktop disk cleaner in the spirit of [`tw93/mole`](https://github.com/tw93/mole) — the
open-source `mo` CLI. It reads real hardware telemetry, scans and cleans in-process (no subprocess, no
`mo` binary needed), and renders a warm, low-glare control surface instead of a terminal.

Its cleanup rules are ported from Mole V1.70.0 — the same path lists, rebuildability checks, running-app
guards, whitelist and age thresholds — and covers `mo clean`, `uninstall`, `optimize`, `purge`,
`installer` and `slim`. What differs is how they run: every item is sorted into **Recommended**
(selected by default), **Needs review** or **Protected**, each with a stated reason; you preview the exact
paths; removals go to the Trash with a restore manifest instead of being deleted; and orbit never asks for
administrator rights. [`docs/mole-cli-parity.md`](docs/mole-cli-parity.md) goes command by command: what
is ported, and what is deliberately left out and why.

The interface is in Chinese and English (switch in the sidebar), and the app updates itself from
GitHub Releases.

## Features

| Module | Backed by | What it does |
| --- | --- | --- |
| **Deep Clean** | `internal/scanner` + native Trash | User caches and logs, browser profiles, developer tools, Xcode, Gradle/CocoaPods, app caches and macOS user state, using Mole's `mo clean` rules; data of a running app is protected until you quit it |
| **AI Care** | AI catalog + native Trash | Rechecks Claude Code, Cursor Agent, Copilot CLI and Claude Desktop releases against launcher, process and open handles; models/worktrees are inspect-only |
| **Uninstaller** | App scanner + native Trash | Removes only selected app/cache/configuration paths; supports keeping personal data, stops the app's user-level background services (started again on restore), lists config files that still reference the app, finds leftovers of apps already gone, and slims universal apps to Apple silicon only (`mo slim`) |
| **Optimize** | Mole's 19 `mo optimize` tasks | Checks each task first; 10 can run (with receipts, and undo where something changed), the rest say why orbit does not run them (administrator rights, or data that cannot be restored) |
| **Astrolabe** | `internal/analyze` | Allocated/logical size map with current-layer search, inspectable folded items, file counts, mtimes and memory drill-down |
| **Live Status** | Kernel + `ps` + hardware sensors | CPU, memory, network and supported sensor data; search processes by name/path/PID |
| **Purge** | Build/installer scanner + native Trash | Rebuildable build artifacts (`mo purge`, scan folders shared through `~/.config/mole/purge_paths`) and leftover installer packages (`mo installer`) |
| **History & Whitelist** | Native manifests + mole whitelist | Cleanup receipts, path restoration and protected-path management |

Beyond the module list:

- **Path & Safety Inspector** — selecting any category reveals the exact subdirectories, their sizes,
  a rebuild-cost assessment, one-click Finder reveal, and direct whitelist protection.
- **Proportional volume bars** — every row carries an inline bar relative to total scan size.
- **Pre-execution diff** — deletions are previewed as a structured sheet (paths, safety tier, Trash
  recovery checkpoint) before anything runs.
- **Preview before removal** — the confirmation shows the exact paths and freezes the submitted selection.
- **Menu bar presence** — scaled CPU/network trends, native memory pressure and memory/storage capacity bars; conditional battery, read-only process links and a receipt-backed two-hour awake switch. Quick cleanup opens the frozen selected-path review in the main window; optional file-cache release requires macOS administrator authorization and never claims to clear application RAM. The main window also opens this panel directly.
- **Runtime & permissions** — the header identifies the current app bundle or temporary executable, reports actual protected-directory readability, and links to the relevant macOS permission panes.
- **Live local scan** — installed apps, leftover Library paths, and large files are sampled on the Mac and pushed into the UI over the JS bridge.
- **Three uninstall modes** — Standard, Complete, and Cache-only, with per-path selection before anything moves to the Trash.
- **Treemap drill-down** — disk tiles open into their child directories.
- **Live telemetry** — `statfs`, memory, and `/bin/ps` sampled on a 1.5 s timer.

## Requirements

- macOS 13 (Ventura) or later
- Go 1.27+ (the module declares `go 1.27.3`)
- The [MyGo](https://github.com/egoist/mygo) CLI for packaging and development. `make run` uses a pinned Go-run fallback when the CLI is not installed:

  ```bash
  go install github.com/egoist/mygo/cmd/mygo@v0.6.4
  ```

- The current scanner and cleanup paths run in-process and do not require the historical `mo` CLI.

## Build and run

```bash
git clone https://github.com/real-jacket/orbit.git
cd orbit

make             # go build -v -o mole-studio .
make run         # build and launch a stable .app bundle for macOS permission identity
make run-raw     # temporary go run executable, for bridge debugging only
make test        # unit test suite
make test-ui-webkit # isolated WebKit rendering + interaction checks
make vet         # go vet
make mole-sync   # list upstream Mole changes to the ported rules
make check       # vet + test, the same gate CI runs
make dev         # MyGo dev server with live reload (needs the `mygo` CLI)
make app         # app bundle plus a .dmg, both under build/
make app-nodmg   # app bundle only
make doctor      # check the Go / MyGo / packaging toolchain
make help        # list every target
```

**Which one should I run?** `make run` for day-to-day work: it builds and opens `build/darwin-arm64/orbit.app` (the architecture follows Go). It gives macOS a named bundle instead of a new `go-build` temporary executable on every launch. If an old `go run` instance is active, stop it with Ctrl-C in its original terminal first; the script will not kill it. `make run-raw` retains the old direct-executable path for narrow bridge debugging, but permissions may belong to its launching terminal. `make dev` additionally gives you a
live-reload dev server, but needs the CLI installed. `make dev` builds a throwaway
`orbit Dev` bundle under `.mygo/dev/`, runs it, and then watches the tree:
saving `Resources/src-ui/index.html` (or any Go file) rebuilds and relaunches it in
about five seconds. `make app-nodmg` then
`open build/darwin-arm64/orbit.app` is what users get: the same frontend,
embedded into the binary. See [runtime and permissions](docs/runtime-permissions.md) for authorization and current feature boundaries.

**The first build downloads a toolchain.** `go.mod` declares `go 1.27.3`. If the
`go` on your `PATH` is older, `GOTOOLCHAIN=auto` (the default) fetches the Go
1.27.3 toolchain into the module cache — about 240 MB, one time — and prints
`go: downloading go1.27.3 (darwin/arm64)` before the module downloads (`mygo`,
`purego`, `x/text`). That is a download, not a hang; every run after it starts in
about a second. To keep the toolchain in your version manager instead, install it
there (`gvm install go1.27.3`).

`make app` produces an ad-hoc-signed bundle whose UI is MyGo's embedded copy of
`Resources/src-ui/`, plus a versioned `.dmg` beside it. Both land in a per-platform
subdirectory — on Apple silicon `build/darwin-arm64/orbit.app` — because `mygo build`
always nests its output under `<goos>-<goarch>/`. Drag the bundle to `/Applications` to keep
it around.

**Sandboxed shells.** If the module cache or build cache is not writable, `go build` fails with a
permission error. Point all three at a scratch directory first:

```bash
export GOPATH=/tmp/gopath GOMODCACHE=/tmp/gopath/pkg/mod GOCACHE=/tmp/gocache
export PATH="/tmp/gopath/bin:$PATH"   # where `go install` put the mygo CLI
```

## Releases

Releases are cut from **main**, by [`.github/workflows/release.yml`](.github/workflows/release.yml).
Merging a version bump is the release gesture — nothing is tagged, built or published by hand:

```bash
make set-version VERSION=0.2.0   # in the pull request that ships the change
```

A push to main that changes `mygo.json` asks one question: has that version been released yet? If
not, the workflow builds that commit, verifies it, creates the tag `v<version>`, and publishes the
GitHub Release; if it has, the push is a no-op. The tag is therefore a *receipt* for a build that
passed, never an input: pushing a tag by hand does nothing, which is what keeps a release from
coming out of an unmerged branch. A failed build leaves no tag behind — fix the code, merge it, then
run the workflow from main to retry. Versions only move forward — reverting `mygo.json` on main
fails the run instead of downgrading a published release.

The version in `mygo.json` is what `CFBundleShortVersionString`, the `.dmg` name, the app's own build
stamp and the release page carry. It is `MAJOR.MINOR.PATCH` and nothing else: macOS stores that
string verbatim, so a `-rc.1` suffix would ship a bundle that misreports itself, and the workflow
refuses it. Each release is one universal `.dmg` — Apple silicon and Intel in a single download —
with the bundle inside:

- `orbit-<version>-universal.dmg` — drag `orbit.app` to `/Applications`
- `orbit-<version>-universal.dmg.sha256` — `shasum -a 256 -c` it
- `orbit-<version>-darwin-universal.tar.gz`, `orbit-<old>-to-<version>-darwin-universal.delta` and
  `update-darwin-universal.json` — the in-app update feed (below); leave them on the release

`CHANGELOG.md` needs a `## <version>` section for every release: the update window shows it, and
`mygo build` refuses to build a version the changelog does not mention.

Before publishing, the workflow runs `make check` (the same gate CI enforces) and then checks what
was actually built: both architectures via `lipo`, the bundle identifier, icon, embedded-page marker
the `codesign --verify` signature and its certificate-pinned designated requirement, every URL in
the update manifest against the files being uploaded, and it mounts the `.dmg` to confirm the app and the `/Applications`
symlink are there. The run page carries the SHA-256 in its summary, and the staged assets stay
attached to the run as an artifact for 14 days.

To rebuild the current main version by hand — a dry run, a retry after a failed build, or a repair
for a release whose assets are missing — run the workflow (Actions → Release → Run workflow)
**from main** (a hand-started run from any other branch fails on purpose). It rebuilds the tagged
commit when a tag exists, so a repair never picks up code merged later, and the current main head
when it does not, so a failed build can be retried. Turn `publish` off to build and keep the
artifacts without touching the release. Re-running replaces the existing release's assets instead
of failing.

**Version bumps.** `mygo.json` is the only place the version is written down: `app.go` reads it back
from the bundle at runtime (`mygo.App.Version()`) rather than hard-coding it, and the scanner's build
version falls back to `"dev"` only for plain `go run`. For a local `make app` build, stamp one:

```bash
make set-version VERSION=0.2.0
```

**Updates.** `mygo.json`'s `updates` block (Ed25519 `publicKey`, `github: real-jacket/orbit`) turns on
MyGo's updater plugin: ten seconds after launch and then daily, the app reads
`releases/latest/download/update-darwin-universal.json`, verifies the signature, and shows a
Sparkle-style window with the changelog (install, remind later, skip). **orbit → 检查更新…** and the
menu-bar popover check by hand. The release job signs the feed with the `MYGO_UPDATER_PRIVATE_KEY`
secret and fails without it. The matching private key also lives outside the repo; losing it means
installed copies can never update again, so never regenerate it once a release has shipped. Local
builds carry no key and say that updates are unavailable.

**Signing.** Local builds are ad-hoc signed (`macos.signingIdentity` is `-` in `mygo.json`). Releases
are signed with one fixed self-signed certificate, `orbit Self-Signed` (made once by
`scripts/make-signing-cert.sh`, imported in CI from the `MACOS_SIGNING_P12` and
`MACOS_SIGNING_P12_PASSWORD` secrets). An ad-hoc signature's designated requirement is the build's
cdhash, so macOS would treat every update as a new app and drop Full Disk Access; the certificate's
requirement is the same in every version. It is not notarized, so Gatekeeper still asks once before
the first launch — right-click the app → **Open**, or
`xattr -dr com.apple.quarantine /Applications/orbit.app`. To ship notarized builds, use a Developer
ID identity instead, add `macos.notarize.keychainProfile` to `mygo.json`, and import the profile in
`release.yml`; MyGo then signs, submits the `.dmg` with `notarytool` and staples the ticket.

## Architecture

The app is a Go program that embeds MyGo, which in turn hosts the same single-file
`Resources/src-ui/index.html` UI in a `WKWebView`. The Swift implementation this port started from is
gone from the tree — it was removed in the porting commit and lives on in git history
(`git show adea952:Sources/MoleStudio/MoleStudioApp.swift`). The right-hand column of the tables below
says which Swift file each Go file was written from, so the mapping stays useful.

| Go package | Responsibility | Ported from |
| --- | --- | --- |
| `internal/power` | Thermal (`NSProcessInfo` over purego/objc), memory-pressure and battery snapshot via `sysctl` and `ioreg` (health from the nested `BatteryData` dictionary, temperature from SMC) | `Sources/MoleStudio/PowerStatus.swift` |
| `internal/smc` | Reads AppleSMC over IOKit: CPU temperature and fan speeds, with the byte-order handling both Intel and Apple silicon need | — (new; the Swift build had no SMC reader) |
| `internal/telemetry` | Network throughput from the routing `sysctl`, disk I/O and GPU utilisation from the IORegistry, sampled on independent cache intervals | — (new; the Swift build showed fixed placeholder numbers) |
| `internal/scanner` | The local scan: installed apps, orphaned leftovers, large files, installer packages, clean catalogs, AI Care bands, the home treemap, the process table | `Engine/RealMacSystemScanner.swift`, `Engine/CatalogScanning.swift`, `Engine/OrphanScanning.swift`, `Engine/OrphanSignatures.swift` |
| `internal/analyze` | Walks a directory tree in-process (allocated + logical sizes, hard-link dedupe, progress, cancellation) and shapes it into treemap tiles; errors and cancellation return directly | `Engine/MoleAnalyzeBridge.swift`, `Engine/MoleCLIRunner.swift` |
| `internal/trayicon` | Draws the 18pt menu-bar template image from an analytic description instead of AppKit | `MoleStudioApp.swift` (`configureStatusItem`) |

The app shell itself is a handful of files in `package main` at the repository root, because that is
where MyGo wants the entry point:

| File | Responsibility | Ported from |
| --- | --- | --- |
| `main.go` | `func main() { run() }` — nothing else | `@main enum MoleStudioMain` |
| `app.go` | `run`/`buildUI`: `mygo.NewWindow`, the tray, the popover flyout, the application menu, the 1.5 s telemetry ticker, and `OnActivate`/`OnClose` handling | `MoleStudioApp.swift` (`AppDelegate`, `UILocator`) |
| `bridge.go` | `NativeBridge.Dispatch`: one switch over `payload["action"]` that reproduces the Swift `WKScriptMessageHandler` branch for branch | `MoleStudioApp.swift` (`NativeBridgeController`) |
| `preload.go` | The `PreloadScript` injected before the page's own scripts: the `isNativeMacApp`/`isMenuBarPopover` flags, and the drag-region CSS | `MoleStudioApp.swift` (WKWebView configuration) |
| `push.go` | Go → page: serialises payloads and calls the page's `window.onNative*` globals in every open window | `MoleStudioApp.swift` (`evaluateJavaScript` call sites) |
| `theme.go` | `mygo.Theme` plus a `preferences.json` in the user data directory, replacing `UserDefaults` | `MoleStudioApp.swift` (`applyTheme`) |

Five documents go deeper: [`docs/go-port.md`](docs/go-port.md) is the porting record — file-by-file
mapping, the decisions behind it, and the known gaps against the Swift build — while
[`docs/mygo-migration.md`](docs/mygo-migration.md) is the framework analysis that justified the move,
[`docs/business-logic-review.md`](docs/business-logic-review.md) is the review that followed the port:
what was fabricated, what only pretended to work (清理回执、还原、白名单、体检、退出), and how each was
replaced with a real implementation, and
[`docs/disk-analyzer.md`](docs/disk-analyzer.md) explains why the disk analyzer stopped shelling out to
`mo analyze` and how the in-process scanner counts bytes, reports progress and cancels, and
[`docs/ui-refinement.md`](docs/ui-refinement.md) records the interface pass: which decorative layers
were removed, what replaced them, and what was deliberately left alone. The design rules themselves live
in [`DESIGN.md`](DESIGN.md), the product rules in [`PRODUCT.md`](PRODUCT.md).

The port itself did not touch the frontend's visual design; the layout is still the original page. What
did change in `Resources/src-ui/index.html` is the part that talked to the native side: the page
posts straight to `NativeBridge.Dispatch` through `window.mygo.call` (`nativePost` in `index.html`), and the page now waits for a receipt
before it claims anything was deleted or restored. Actions the page sends: `revealInFinder`, `copyText`,
`killProcess`, `trashRealPaths`, `restoreTrashSnapshot`, `addWhitelist`/
`removeWhitelist`, `addPurgeRoot`/`removePurgeRoot`, `runDoctorReCheck`, `requestRealSystemScan`,
`measurePathFully`, `requestDiskAnalyze`, `requestRealTelemetry`, `cancelDiskAnalyze`,
`requestOptimizePlan`, `executeOptimize`, `undoOptimize`, `scanSlim`, `slimApps`, `toggleCaffeinate`,
`startWindowDrag`, `toggleWindowZoom`, `setTheme`, `setLanguage`, `checkForUpdates`, the `menubar*` family.
Callbacks: `onNativeRealScanComplete`, `onNativeRealTelemetry`, `onNativeDiskAnalyze`,
`onNativeTrashResult`, `onNativeRestoreResult`, `onNativeWhitelistChanged`, `onNativePurgeRootsChanged`,
`onNativeOptimizePlan`, `onNativeOptimizeResult`, `onNativeOptimizeUndo`, `onNativeSlimScan`,
`onNativeSlimResult`, `onNativeCaffeinate`, `onNativeDoctorReport`, `onNativeAnalyzeProgress`,
`switchModule`, `openConfirmSheet`. Only `setFanMode` still falls through to
`default`: it needs SMC access, which the port deliberately does not do (see
[`docs/business-logic-review.md`](docs/business-logic-review.md) §4).

## App icon

The icon is **rendered from code**, not checked in as an opaque bitmap. `scripts/generate-app-icon.swift`
draws the v10 brand mark with CoreGraphics and emits every size `iconutil` needs. The Go build never runs
it — the rendered outputs are committed under `assets/` — so it is only needed when you change the mark.
Call it directly; there is no Makefile target for it:

```bash
swift scripts/generate-app-icon.swift icns   assets/AppIcon.icns
swift scripts/generate-app-icon.swift single assets/AppIcon-256.png 256
swift scripts/generate-app-icon.swift sheet  /tmp/icon-preview.png
```

MyGo reads a PNG, not an `.icns`: `mygo.json` points at `assets/icon.png` (a copy of the 256 px mark) and
MyGo renders `Contents/Resources/AppIcon.icns` from it while packaging. That PNG is the **bundle** icon
only — the menu bar mark is drawn at runtime by `internal/trayicon`, so the two are independent. Do not
list an icon or an `.icns` again under `resources`: MyGo reserves those bundle names and rejects the
build.

The mark matches the sidebar brand SVG: a `#27272a → #09090b → #7c2d12` squircle plate, a terracotta
planet lit from the upper left (`#fff7ed → #fdba74 → #ea580c → #7c2d12`), a ring split so the far half
passes behind the planet and the near half crosses in front, and an ice-blue companion (`#38bdf8`).

Three details worth knowing if you edit the renderer:

- **Small sizes keep the same mark, with thicker strokes.** At 16–32px the planet, ring, and companion
  are enlarged so the silhouette still reads in the Dock.
- **The output is dithered.** An 8-bit gradient across a large dark surface bands visibly; a ±2 noise
  layer removes the steps and is invisible at 100%.
- **The background is a superellipse, not a rounded rectangle.** Apple's icon grid uses a continuous
  corner curve, so the body is drawn as an n=5 squircle inset 100px in a 1024px canvas (the standard
  824×824 body).

## Project status

This is an early-stage client, and the Go port is mid-flight. Being explicit about what is and is not
wired up:

- **The UI in `Resources/src-ui/index.html` is the shipping UI**, maintained as one dependency-free HTML file. In a
  `mygo build` and `make run` bundle it; `make run-raw` and `mygo dev` use a development source path.
- **The samplers are real and tested.** `internal/scanner` walks the disk for apps, leftovers, large
  files, installers and clean catalogs; `internal/power` reads thermal state, memory pressure and
  the battery from `sysctl`/`ioreg`; `internal/telemetry` reads network, disk I/O and GPU rates from
  the kernel and `internal/smc` reads temperatures and fan speeds; `internal/analyze` walks the disk
  itself; failures and cancellation do not start a second CLI scan.
  `go test ./...` covers the numeric and payload-shape behaviour of all three.
- **The bridge is implemented, not stubbed.** `NativeBridge.Dispatch` handles Finder reveal, clipboard,
  process termination, native Trash/restore, whitelist and purge folder updates, diagnostics,
  caffeinate, theme and language persistence, optimize tasks, app slimming and window/menu-bar actions.
  Trash, restore, diagnosis, optimize and slimming use native receipts. Fan control remains unsupported.
- **CI builds the Go app.** `.github/workflows/ci.yml` runs `go vet` / `go test` / `go build`, installs the
  pinned `mygo` CLI, packages with `mygo build --skip-dmg`, and then asserts the bundle is real — the
  Mach-O executable, a valid `Info.plist`, `Contents/Resources/AppIcon.icns` that unpacks with
  `iconutil`, and the embedded frontend marker in the binary. A second job gates `gofmt`, `go vet`, and a
  clean `mygo generate` (so `mygo.ts` can never be stale in a PR).
- **AI cleanup does not depend on aspirational CLI flags.** Mole CLI has no AI-clean subcommand; orbit
  scans a local, bounded catalog and submits only verified-old-version paths through native Trash.
  Like Mole's `clean_dev_ai_agents`, it keeps the current and one spare version and anything a process
  still has open; extensions, models, sessions and worktrees are retained.
- **Ported rules can follow upstream.** `make mole-sync` diffs every marked Mole function between the
  version it was ported from and the latest Mole release. See
  [`docs/mole-cli-parity.md`](docs/mole-cli-parity.md) for the command by command comparison.
- **Space analysis follows mature explorer patterns without a second rendering stack.** DaisyDisk's
  inspectable "smaller objects", SpaceSniffer's current-scope filtering and GrandPerspective's
  treemap navigation informed the bounded folded-item sample and small-window map-first layout.
- **Missing sensors remain missing.** CPU temperature comes from SMC when available, not from CPU load.
- **Frontend regressions run in CI.** `node scripts/test-ui.cjs` checks selected paths, immutable confirmation,
  mode boundaries, request receipts and primary-button contrast in both themes. Node is development-only;
  the shipped UI still has no third-party runtime.
- **Rendered UI checks are repeatable.** `make test-ui-webkit` verifies the actual page in an isolated
  macOS WebKit view across both themes and minimum-window layouts; screenshots and source-hashed reports
  stay under `.review/webkit/`. See [`docs/optimization-validation.md`](docs/optimization-validation.md).
- **Restricted reads are visible.** Directory reads waiting for macOS authorization are bounded and
  coalesced, with their paths reported as not counted, rather than as zero-byte empty directories.

## Safety model

The app deletes files, so the guardrails are the point:

- Every item has a tier. **Recommended** items are rebuildable and not in use, and are selected by
  default. **Needs review** and **Protected** items carry a plain reason; protected ones cannot be
  selected, and when the reason is a running app they say what to quit.
- Every destructive action is previewed before execution, with the exact paths frozen at confirmation.
- Cleanup moves items to the Trash, and the page waits for a native receipt before it reports success.
  History can put them back. orbit never empties the Trash.
- orbit never asks for administrator rights. Mole tasks that need `sudo` are listed with that reason
  and not run; tasks that would destroy data the Trash cannot hold are not run either.
- Settings changes (optimize) record the previous value, so they can be undone.
- The whitelist is read from `~/.config/mole/whitelist`, the same file `mo` uses.

## Relationship to upstream

This project is an independent GUI client. It is **not** affiliated with or endorsed by
[tw93/mole](https://github.com/tw93/mole), and it does not redistribute or run the `mo` binary.

Parts of its cleanup rules are ported from Mole V1.70.0 (shell → Go). Upstream `mo` is licensed
GPL-3.0, and this client is licensed GPL-3.0 to match. Each ported rule carries a comment such as
`// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_browsers`; `make mole-sync` reads those
markers and prints what changed upstream in exactly those functions since, so the rules can follow new
Mole releases. See [`docs/mole-cli-parity.md`](docs/mole-cli-parity.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports and feature requests have templates under
`.github/ISSUE_TEMPLATE/`.

## License

[GPL-3.0](LICENSE). Copyright (C) 2026 real-jacket.
