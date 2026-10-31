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
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	if os.Getenv("MOLE_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "DISPATCH action=%v paths=%v path=%v requestId=%v\n",
			payload["action"], payload["paths"], payload["path"], payload["requestId"])
	}
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
		if len(paths) == 0 {
			return nil
		}
		requestID := stringOf(payload["requestId"])
		go func() {
			// The page waits for this receipt before it claims success, so a
			// refused or failed path is reported instead of swallowed.
			pushTrashResult(requestID, scanner.Shared.TrashPaths(paths))
			pushRealTelemetryNow()
			pushRealSystemScanNow()
		}()
	case "restoreTrashSnapshot":
		// The page's "可还原/已还原" buttons used to be decoration: the Swift
		// app ignored this action and the Go port inherited that. Restoring is
		// now a real move-back through the app's own manifest.
		originals := stringsOf(payload["paths"])
		requestID := stringOf(payload["requestId"])
		go func() {
			pushRestoreResult(requestID, scanner.Shared.RestoreTrash(originals))
			pushRealTelemetryNow()
			pushRealSystemScanNow()
		}()
	case "addWhitelist", "removeWhitelist":
		action := stringOf(payload["action"])
		path := stringOf(payload["path"])
		if path == "" {
			return nil
		}
		go func() {
			var policy scanner.Whitelist
			var err error
			if action == "addWhitelist" {
				policy, err = scanner.Shared.AddWhitelistEntry(path)
			} else {
				policy, err = scanner.Shared.RemoveWhitelistEntry(path)
			}
			pushWhitelistChanged(action, path, policy, err)
			pushRealSystemScanNow()
		}()
	case "killProcess":
		killProcess(intOf(payload["pid"]))
	case "requestRealSystemScan":
		pushRealSystemScanNow()
	case "requestDiskAnalyze":
		pushDiskAnalyze(stringOf(payload["path"]), boolOf(payload["refresh"]), stringOf(payload["requestId"]))
	case "cancelDiskAnalyze":
		go cancelDiskAnalyze(stringOf(payload["requestId"]))
	case "requestRealTelemetry":
		pushRealTelemetryNow()
	case "executeOptimize":
		requestID := stringOf(payload["requestId"])
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, "/usr/bin/qlmanage", "-r", "cache").CombinedOutput()
			result := optimizeOutcome(requestID, output, err)
			evaluate("window.onNativeOptimizeResult ? window.onNativeOptimizeResult(" + jsonOf(result) + ") : undefined")
		}()
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
	case "openPrivacySettings":
		// A failed clean or uninstall is usually a missing TCC grant, and the
		// switch that fixes it lives two clicks deep in System Settings.
		go openPrivacyPane(stringOf(payload["pane"]))
	case "runDoctorReCheck":
		// The 体检 page used to fake this: a random delay, a health score that
		// only went up and seven hardcoded "通过" rows. The checks are now read
		// from the machine, and a check that cannot be decided is reported as
		// unknown rather than as a pass.
		requestID := stringOf(payload["requestId"])
		go func() { pushDoctorReport(requestID, scanner.Shared.DoctorChecks()) }()
	default:
		// setFanMode still arrives here. There is no fan-control API to call, so
		// the page no longer pretends it did something.
	}
	return nil
}

func optimizeOutcome(requestID string, output []byte, err error) map[string]any {
	result := map[string]any{"requestId": requestID, "ok": err == nil}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 500 {
			message = message[:500]
		}
		result["error"] = fmt.Sprintf("%v: %s", err, message)
	}
	return result
}

// privacyPaneURLs are the System Settings deep links for the three permissions
// this app can trip over: App Management to move another app to the Trash, Files
// and Folders to delete inside Desktop/Documents/Downloads, and Full Disk Access
// to read inside another app's Library folder.
const (
	privacyPaneAppManagementURL   = "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AppBundles"
	privacyPaneFullDiskAccessURL  = "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AllFiles"
	privacyPaneFilesAndFoldersURL = "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_FilesAndFolders"
)

// privacyPaneURL maps the pane name the page receives in a failure receipt to
// the System Settings link that grants it. Unknown panes fall back to Full Disk
// Access, which is the permission a clean needs.
func privacyPaneURL(pane string) string {
	switch pane {
	case "appManagement":
		return privacyPaneAppManagementURL
	case "filesAndFolders":
		return privacyPaneFilesAndFoldersURL
	}
	return privacyPaneFullDiskAccessURL
}

// openPrivacyPane asks the system to open the settings pane that grants pane.
func openPrivacyPane(pane string) {
	_ = exec.Command("/usr/bin/open", privacyPaneURL(pane)).Run()
}

// revealInFinder shows path in the Finder. The Swift app cut the path at its
// first space, which silently broke every folder whose name contains one
// ("Visual Studio Code.app"); the trailing label the page appends is stripped
// instead, so "~/Library/Caches (0.92 GB)" still reveals the real folder.
func revealInFinder(raw string) {
	path := expandTilde(strings.TrimSpace(raw))
	if path == "" {
		return
	}
	for _, candidate := range revealCandidates(path) {
		if exists(candidate) {
			mygo.Shell.ShowItemInFolder(candidate)
			return
		}
	}
	if parent := filepath.Dir(path); parent != "" && exists(parent) {
		mygo.Shell.ShowItemInFolder(parent)
		return
	}
	mygo.Shell.ShowItemInFolder(homeDir())
}

// revealCandidates lists the paths worth trying for one request: the path
// itself, then the same path with a trailing " (label)" removed.
func revealCandidates(path string) []string {
	candidates := []string{path}
	if open := strings.LastIndex(path, " ("); open > 0 && strings.HasSuffix(path, ")") {
		candidates = append(candidates, strings.TrimSpace(path[:open]))
	}
	return candidates
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
	if caffeinateProcess != nil {
		_ = caffeinateProcess.Process.Signal(syscall.SIGTERM)
		caffeinateProcess = nil
	}
	if !active {
		caffeinateMu.Unlock()
		pushCaffeinateState(false)
		return
	}
	cmd := exec.Command("/usr/bin/caffeinate", "-d", "-i", "-t", "7200")
	if err := cmd.Start(); err != nil {
		caffeinateMu.Unlock()
		pushCaffeinateState(false)
		return
	}
	caffeinateProcess = cmd
	caffeinateMu.Unlock()
	pushCaffeinateState(true)
	// caffeinate exits by itself when its -t limit runs out, so wait for it and
	// tell the page, instead of leaving the switch on forever.
	go func() {
		_ = cmd.Wait()
		caffeinateMu.Lock()
		if caffeinateProcess == cmd {
			caffeinateProcess = nil
			caffeinateMu.Unlock()
			pushCaffeinateState(false)
			return
		}
		caffeinateMu.Unlock()
	}()
}

// showMainWindow brings the main window forward, closing the menu-bar panel
// first, and optionally runs a script in the page. It is the Go form of the
// Swift app's showMainWindow.
func showMainWindow(script string) {
	if w := flyoutWindow.Load(); w != nil && !w.IsDestroyed() && w.IsVisible() {
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
