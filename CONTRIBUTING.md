# Contributing to orbit

Thanks for taking a look. This is a small project, so the process is deliberately light.

## Getting set up

```bash
git clone https://github.com/real-jacket/orbit.git
cd orbit
make run     # build and launch
make check   # lint + test
```

You will also want the upstream CLI available, since the app shells out to it:

```bash
brew install mole
mo --version
```

## Before you open a pull request

Run these and make sure they pass:

```bash
swift build -c release
make test
make lint
make app
```

CI runs the same steps on `macos-latest`, so a green local run means a green PR. `make check` bundles
lint and test into one command.

## Tests

`Tests/MoleStudioTests/` covers the two things most likely to break silently:

- **`TreemapLayoutTests`** — the squarified layout math. Asserts every entry gets a rect, the rects
  tile the unit square exactly (total area `1.0`), and nothing escapes the bounds.
- **`SafetyModelTests`** — the safety invariants. Asserts `protectedSystem` rows cannot be selected
  even by calling the toggle directly, and that dry-run defaults to on.

If you change `SafetyTier`, `MoleModule`, or `computeTreemapRects`, extend these rather than adding a
new file.

## App icon

`Resources/AppIcon.icns` is generated — do not edit it by hand, and do not replace it with a dropped-in
bitmap. Change `scripts/generate-app-icon.swift` and run `make icon`, then commit both the script and the
regenerated `AppIcon.icns` / `AppIcon-256.png`.

`make icon-preview` renders every size on dark and light plates to `/tmp/icon-preview.png`; check the
16px and 32px ends before you commit, since that is where marks usually fall apart.

## Code style

- **Swift** — 4-space indent. `.swiftlint.yml` is the source of truth; `make format` auto-corrects what
  it can. Line length warnings start at 200 characters.
- **HTML/CSS/JS** — 2-space indent. `Resources/src-ui/index.html` is intentionally a single file with no
  build step. Keep it that way unless there is a strong reason not to.
- **Commits** — [Conventional Commits](https://www.conventionalcommits.org/) prefixes
  (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`). Imperative subject line, under ~72 chars.

## Two things to know about the architecture

1. **The UI is HTML, not SwiftUI.** `Resources/src-ui/index.html` is the shipping surface. The SwiftUI
   files under `Sources/MoleStudio/Views/` and `Engine/MoleViewModel.swift` are a parallel native
   implementation that is not yet wired in. If you are changing something the user sees, you probably
   want the HTML.

2. **The JS bridge is a hand-written contract.** Swift posts to JS via
   `webView.evaluateJavaScript(...)`; JS posts to Swift via `window.webkit.messageHandlers.moleNative`.
   If you rename a function on one side, update the other. Current surface:

   | Direction | Names |
   | --- | --- |
   | JS → Swift | `startWindowDrag`, `toggleWindowZoom`, `revealInFinder`, `copyText`, `executeOptimize`, `killProcess` |
   | Swift → JS | `switchModule`, `openConfirmSheet`, `openDoctorModal`, `stepLiveTelemetry` |

## Adding a module

Modules are the `MoleModule` enum in `Sources/MoleStudio/Models/MoleModels.swift`. A new case needs
`sidebarTitle`, `headerTitle`, `sidebarBadge`, `cliBadgeText`, `dotColor` and `hasPlanetaryRing`, plus a
matching section in the HTML and, if it maps to a real subcommand, an entry in `MoleCLIRunner`.

## Safety changes need extra care

Anything touching deletion, the whitelist, or `SafetyTier` gets a closer review. The invariants:

- `tier == .protectedSystem` items must never become selectable.
- Destructive operations must remain previewable before execution.
- Dry-run must stay the default.
- The whitelist file format must stay compatible with upstream `mo` (`~/.config/mole/whitelist`,
  newline-separated, `#` comments allowed).

## Reporting bugs

Use the issue template. The most useful reports include your macOS version, `mo --version`, and the
exact steps that reproduced the problem. Since this app deletes files, please say whether the problem
occurred in dry-run or for real.
