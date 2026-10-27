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
//
// Eval is given an EXPRESSION, not a statement: MyGo first tries to compile the
// snippet as an expression (so it can return a value), and only falls back to a
// function body when that fails with a syntax error. An `if (...) call(...);`
// statement always fails that first attempt, and on WebKit each failed attempt
// surfaces in the page as a cross-origin "Script error." — once per push, every
// 1.5 seconds. A conditional expression compiles on the first try, so it costs
// one eval instead of two and leaves the page's error log alone.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/mymoles/mole-studio/internal/analyze"
	"github.com/mymoles/mole-studio/internal/scanner"
)

// evaluate runs a statement in every page that shows the app: the main window
// and, while it is open, the menu-bar panel. It mirrors the Swift helper of the
// same name, which also evaluated on both web views.
func evaluate(script string) {
	debugEval := os.Getenv("MOLE_DEBUG") != ""
	run := func(page *mygo.Page) {
		if page == nil {
			return
		}
		if _, err := page.Eval(script); err != nil && debugEval {
			fmt.Fprintf(os.Stderr, "mole: eval failed: %v\n", err)
		}
	}
	if w := activeMainWindow(); w != nil {
		run(w.Page())
	}
	if w := flyoutWindow; w != nil && !w.IsDestroyed() && w.IsVisible() {
		run(w.Page())
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
		evaluate("window.onNativeRealTelemetry ? window.onNativeRealTelemetry(" + jsonOf(snapshot) + ") : undefined")
	}()
}

// analyzeQueue keeps at most one `mo analyze` running. Each run can take half a
// minute on a big directory, and the page asks again on every drill-down, so
// without this the requests pile up behind each other and the last answer
// arrives long after the one the user is waiting for.
var analyzeQueue struct {
	mu      sync.Mutex
	running bool
	next    *string
}

// pushDiskAnalyze sends a `mo analyze` result to the page. An empty path asks
// for the overview of the root volume; anything else analyses that directory
// and the page hangs the result under the tile it already has for it. A request
// that arrives while one is running replaces the pending one, so the newest
// click wins instead of every click being queued.
func pushDiskAnalyze(path string) {
	analyzeQueue.mu.Lock()
	if analyzeQueue.running {
		pending := path
		analyzeQueue.next = &pending
		analyzeQueue.mu.Unlock()
		return
	}
	analyzeQueue.running = true
	analyzeQueue.mu.Unlock()

	go func() {
		current := path
		for {
			var payload analyze.Payload
			if current == "" {
				payload = analyze.Overview()
			} else {
				payload = analyze.Children(expandTilde(current))
			}
			evaluate("window.onNativeDiskAnalyze ? window.onNativeDiskAnalyze(" + jsonOf(payload) + ") : undefined")

			analyzeQueue.mu.Lock()
			if analyzeQueue.next == nil {
				analyzeQueue.running = false
				analyzeQueue.mu.Unlock()
				return
			}
			current = *analyzeQueue.next
			analyzeQueue.next = nil
			analyzeQueue.mu.Unlock()
		}
	}()
}

// pushTrashResult sends the receipt for one trash request. The page only
// claims "已成功清理" when this arrives, and it reports the refused and failed
// paths instead of hiding them.
func pushTrashResult(requestID string, outcome scanner.TrashOutcome) {
	moved := make([]map[string]any, 0, len(outcome.Moved))
	for _, entry := range outcome.Moved {
		moved = append(moved, map[string]any{"original": entry.Original, "trashed": entry.Trashed})
	}
	skipped := make([]map[string]any, 0, len(outcome.Skipped))
	for _, entry := range outcome.Skipped {
		skipped = append(skipped, map[string]any{"path": entry.Path, "reason": entry.Reason})
	}
	failed := make([]map[string]any, 0, len(outcome.Failed))
	for _, entry := range outcome.Failed {
		failed = append(failed, map[string]any{"path": entry.Path, "error": entry.Error})
	}
	payload := map[string]any{
		"requestId":  requestID,
		"requested":  outcome.Requested,
		"movedCount": len(outcome.Moved),
		"moved":      moved,
		"skipped":    skipped,
		"failed":     failed,
	}
	evaluate("window.onNativeTrashResult ? window.onNativeTrashResult(" + jsonOf(payload) + ") : undefined")
}

// pushRestoreResult sends the receipt for a restore request.
func pushRestoreResult(requestID string, outcome scanner.RestoreOutcome) {
	restored := append([]string{}, outcome.Restored...)
	failed := make([]map[string]any, 0, len(outcome.Failed))
	for _, entry := range outcome.Failed {
		failed = append(failed, map[string]any{"path": entry.Path, "error": entry.Error})
	}
	payload := map[string]any{
		"requestId":     requestID,
		"requested":     outcome.Requested,
		"restored":      restored,
		"restoredCount": len(restored),
		"failed":        failed,
	}
	evaluate("window.onNativeRestoreResult ? window.onNativeRestoreResult(" + jsonOf(payload) + ") : undefined")
}

// pushDoctorReport sends the 体检 rows the page renders.
func pushDoctorReport(checks []map[string]any) {
	if checks == nil {
		checks = []map[string]any{}
	}
	evaluate("window.onNativeDoctorReport ? window.onNativeDoctorReport(" + jsonOf(map[string]any{"checks": checks}) + ") : undefined")
}

// pushCaffeinateState tells both pages whether the display is being kept awake.
// The state lives in Go, so the window and the menu-bar panel cannot disagree
// and a caffeinate that exits on its own timer still turns the switch off.
func pushCaffeinateState(active bool) {
	evaluate("window.onNativeCaffeinate ? window.onNativeCaffeinate(" + jsonOf(map[string]any{"active": active}) + ") : undefined")
}

// pushWhitelistChanged tells the page the protection list changed, so it can
// redraw the whitelist view and rescan without guessing.
func pushWhitelistChanged(action, path string, policy scanner.Whitelist, err error) {
	payload := map[string]any{
		"action":    action,
		"path":      path,
		"whitelist": scanner.Shared.UserWhitelistPaths(),
		"warnings":  policy.Warnings,
		"ok":        err == nil,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	evaluate("window.onNativeWhitelistChanged ? window.onNativeWhitelistChanged(" + jsonOf(payload) + ") : undefined")
}

// pushRealSystemScanNow scans installed apps, leftovers, large files, installer
// packages and the clean groups, then sends the whole payload to the page.
func pushRealSystemScanNow() {
	go func() {
		payload := scanner.Shared.BuildFullScanPayload()
		evaluate("window.onNativeRealScanComplete ? window.onNativeRealScanComplete(" + jsonOf(payload) + ") : undefined")
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
