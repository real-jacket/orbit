## What changed

<!-- A short description of the change and why it is needed. -->

## Related issue

<!-- e.g. Closes #12 -->

## How it was verified

<!--
Paste the commands you ran and their outcome. At minimum:

    swift build -c release
    make lint
    make app
-->

## Checklist

- [ ] `swift build -c release` succeeds
- [ ] `make lint` passes
- [ ] `make app` produces a launchable `dist/MoleStudio.app`
- [ ] If the JS bridge changed, both sides were updated (`moleNative` handler in Swift,
      `evaluateJavaScript` call sites in `Resources/src-ui/index.html`)
- [ ] If a destructive path changed, `SafetyTier` invariants still hold and dry-run stays the default

## Screenshots

<!-- For UI changes. Before/after is ideal. -->
