// Command mole-studio is the native macOS front end for the Mole CLI, built on
// MyGo.
//
// The app is a shell around Resources/src-ui/index.html: the page draws the
// whole interface and posts actions to Go through the bridge in bridge.go, and
// Go pushes the machine's real numbers back with the three helpers in push.go.
// The page itself is unchanged from the version the AppKit app shipped — the
// preload script in preload.go gives it the environment it expects.
//
// Nothing here reimplements the Mole CLI. `mo analyze --json` supplies the disk
// analysis, and everything else is measured with the standard tools the Swift
// app used: ps, vm_stat, statfs, sysctl and ioreg.
package main

func main() {
	run()
}
