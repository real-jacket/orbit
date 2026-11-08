// Command mole-studio is the native macOS disk cleaner, built on MyGo.
//
// The app is a shell around Resources/src-ui/index.html: the page draws the
// whole interface and posts actions to Go through the bridge in bridge.go, and
// Go pushes the machine's real numbers back with the three helpers in push.go.
// The page itself is unchanged from the version the AppKit app shipped — the
// preload script in preload.go gives it the environment it expects.
//
// Nothing here shells out to the Mole CLI: the disk analysis and the cleanup
// scanners run in-process (internal/analyze, internal/scanner), and telemetry
// comes from sysctl, statfs, IOKit and a few ps/ioreg calls.
package main

func main() {
	run()
}
