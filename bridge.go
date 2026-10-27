package main

// This file is the Go half of the page bridge: one method that receives the
// payload the page would have posted through WebKit's moleNative handler and
// answers it the way the Swift app's WKScriptMessageHandler did.
//
// The frontend sends twenty different actions with different payload shapes.
// Rather than expose twenty bound methods with a dozen signatures, the preload
// shim forwards the whole payload to Dispatch, which keeps the original single
// switch in one place — the same shape the AppKit app had, and the reason the
// page needs no changes at all.

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"

	"github.com/mymoles/mole-studio/internal/scanner"
)

// NativeBridge is the service the page calls. MyGo names it after the type, so
// the shim calls "NativeBridge.Dispatch".
type NativeBridge struct{}

// Dispatch answers one message from the page.
func (b *NativeBridge) Dispatch(payload map[string]any) error {
	switch payload["action"] {
	case "startWindowDrag":
		// Nothing to do: the preload marks the window chrome with
		// --app-region, and MyGo starts the drag itself. The page still
		// posts the action, so it stays a known one.
	case "toggleWindowZoom":
		if w := activeMainWindow(); w != nil {
			w.ToggleMaximize()
		}
	case "revealInFinder":
		revealInFinder(stringOf(payload["path"]))
	case "copyText":
		mygo.Clipboard.WriteText(stringOf(payload["text"]))
	case "trashRealPaths", "executeClean", "trashPath":
		paths := stringsOf(payload["paths"])
		if len(paths) == 0 {
			if path := stringOf(payload["path"]); path != "" {
				paths = []string{path}
			}
		}
		go func() {
			scanner.Shared.MovePathsToTrash(paths)
			pushRealTelemetryNow()
			pushRealSystemScanNow()
		}()
	case "killProcess":
		killProcess(intOf(payload["pid"]))
	case "requestRealSystemScan":
		pushRealSystemScanNow()
	case "requestDiskAnalyze":
		pushDiskAnalyze(stringOf(payload["path"]))
	case "requestRealTelemetry":
		pushRealTelemetryNow()
	case "executeOptimize":
		go func() { _ = exec.Command("/usr/bin/qlmanage", "-r", "cache").Run() }()
	case "toggleCaffeinate":
		setCaffeinate(boolOf(payload["active"]))
	case "menubarOpenClean":
		showMainWindow("switchModule('clean'); openConfirmSheet();")
	case "menubarOpenDoctor":
		showMainWindow("openDoctorModal(true);")
	case "menubarShowMain":
		showMainWindow("")
	case "menubarQuit":
		mygo.App.Quit()
	case "setTheme":
		if theme := stringOf(payload["theme"]); theme == themeDark || theme == themeLight || theme == themeSystem {
			applyTheme(theme, true)
		}
	default:
		// addWhitelist, restoreTrashSnapshot, runDoctorReCheck and
		// setFanMode arrive here. The Swift app ignored them too: the
		// page keeps that state itself.
	}
	return nil
}

// revealInFinder shows path in the Finder, falling back the way the Swift app
// did: the frontend appends labels to some paths, so everything after the first
// space is dropped, ~ is expanded, and a path that no longer exists reveals its
// parent directory instead.
func revealInFinder(raw string) {
	path := raw
	if space := strings.Index(path, " "); space >= 0 {
		path = path[:space]
	}
	path = expandTilde(path)
	if path == "" {
		return
	}
	if exists(path) {
		mygo.Shell.ShowItemInFolder(path)
		return
	}
	if parent := filepath.Dir(path); parent != "" && exists(parent) {
		mygo.Shell.ShowItemInFolder(parent)
		return
	}
	mygo.Shell.ShowItemInFolder(homeDir())
}

// killProcess asks a process to quit, then kills it if it is still there a
// moment later. The Swift app did the same, with SIGTERM followed by SIGKILL.
func killProcess(pid int) {
	if pid <= 1 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	go func() {
		time.Sleep(400 * time.Millisecond)
		if processAlive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		pushRealTelemetryNow()
	}()
}

// processAlive reports whether a process still exists. Signal 0 probes for it
// without delivering anything.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// caffeinateProcess is the running caffeinate command, if any.
var (
	caffeinateMu      sync.Mutex
	caffeinateProcess *exec.Cmd
)

// setCaffeinate keeps the display and the system awake while active. Turning it
// off stops the previous command, as the Swift app terminated its stored
// process.
func setCaffeinate(active bool) {
	caffeinateMu.Lock()
	defer caffeinateMu.Unlock()
	if caffeinateProcess != nil {
		_ = caffeinateProcess.Process.Signal(syscall.SIGTERM)
		caffeinateProcess = nil
	}
	if !active {
		return
	}
	cmd := exec.Command("/usr/bin/caffeinate", "-d", "-i", "-t", "7200")
	if err := cmd.Start(); err == nil {
		caffeinateProcess = cmd
	}
}

// showMainWindow brings the main window forward, closing the menu-bar panel
// first, and optionally runs a script in the page. It is the Go form of the
// Swift app's showMainWindow.
func showMainWindow(script string) {
	if w := flyoutWindow; w != nil && !w.IsDestroyed() && w.IsVisible() {
		w.Hide()
	}
	w := activeMainWindow()
	if w == nil {
		return
	}
	w.Show()
	w.Focus()
	mygo.App.Focus()
	if script != "" {
		_, _ = w.Page().Eval(script)
	}
}

// activeMainWindow returns the main window unless it has been destroyed.
func activeMainWindow() *mygo.Window {
	if mainWindow == nil || mainWindow.IsDestroyed() {
		return nil
	}
	return mainWindow
}

// stringOf reads a string out of a decoded JSON payload.
func stringOf(value any) string {
	text, _ := value.(string)
	return text
}

// boolOf reads a bool out of a decoded JSON payload.
func boolOf(value any) bool {
	flag, _ := value.(bool)
	return flag
}

// intOf reads an integer out of a decoded JSON payload. JSON numbers decode as
// float64, but a caller may pass an int or a json.Number, as the Swift helper
// accepted Int, Int32 and Double.
func intOf(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	case json.Number:
		if parsed, err := number.Int64(); err == nil {
			return int(parsed)
		}
	}
	return 0
}

// stringsOf reads a list of strings out of a decoded JSON payload.
func stringsOf(value any) []string {
	switch list := value.(type) {
	case []string:
		return list
	case []any:
		paths := make([]string, 0, len(list))
		for _, item := range list {
			if text, ok := item.(string); ok {
				paths = append(paths, text)
			}
		}
		return paths
	}
	return nil
}
