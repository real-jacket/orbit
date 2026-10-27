package main

// This file holds the three things the shell pushes INTO the page: the live
// telemetry snapshot, a disk analysis result, and the results of the full
// system scan.
//
// The Swift app built each payload, serialised it, and evaluated
//
//	if (window.onNativeRealTelemetry) window.onNativeRealTelemetry(<json>);
//
// in the main web view and, when it was open, in the popover. The Go side does
// the same thing through Page.Eval, which is safe to call from any goroutine
// because MyGo posts it to the main thread itself.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mymoles/mole-studio/internal/analyze"
	"github.com/mymoles/mole-studio/internal/scanner"
)

// evaluate runs a statement in every page that shows the app: the main window
// and, while it is open, the menu-bar panel. It mirrors the Swift helper of the
// same name, which also evaluated on both web views.
func evaluate(script string) {
	if w := activeMainWindow(); w != nil {
		if page := w.Page(); page != nil {
			_, _ = page.Eval(script)
		}
	}
	if w := flyoutWindow; w != nil && !w.IsDestroyed() && w.IsVisible() {
		if page := w.Page(); page != nil {
			_, _ = page.Eval(script)
		}
	}
}

// pushRealTelemetryNow sends the live CPU, memory, disk, process and power
// snapshot to the page and updates the menu-bar tooltip. It does its measuring
// on its own goroutine, since sampling processes and the battery takes long
// enough to be felt on the main thread — the Swift app used a background queue
// for the same reason.
func pushRealTelemetryNow() {
	go func() {
		snapshot := scanner.Shared.BuildTelemetry()
		if trayItem != nil {
			trayItem.SetToolTip(fmt.Sprintf("orbit · CPU %.1f%% · 剩余 %.1f GB", snapshot.CPU, snapshot.FreeDiskGb))
		}
		evaluate("if(window.onNativeRealTelemetry) window.onNativeRealTelemetry(" + jsonOf(snapshot) + ");")
	}()
}

// pushDiskAnalyze sends a `mo analyze` result to the page. An empty path asks
// for the overview of the root volume; anything else analyses that directory
// and the page hangs the result under the tile it already has for it.
func pushDiskAnalyze(path string) {
	go func() {
		var payload analyze.Payload
		if path == "" {
			payload = analyze.Overview()
		} else {
			payload = analyze.Children(expandTilde(path))
		}
		evaluate("if(window.onNativeDiskAnalyze) window.onNativeDiskAnalyze(" + jsonOf(payload) + ");")
	}()
}

// pushRealSystemScanNow scans installed apps, leftovers, large files, installer
// packages and the clean groups, then sends the whole payload to the page.
func pushRealSystemScanNow() {
	go func() {
		payload := scanner.Shared.BuildFullScanPayload()
		evaluate("if(window.onNativeRealScanComplete) window.onNativeRealScanComplete(" + jsonOf(payload) + ");")
	}()
}

// jsonOf serialises a payload for the script above. A payload that cannot be
// serialised becomes null, which the page's handlers already ignore.
func jsonOf(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return "null"
	}
	return string(data)
}

// expandTilde replaces a leading ~ with the home directory.
func expandTilde(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

// exists reports whether path exists.
func exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// homeDir returns the user's home directory.
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}
