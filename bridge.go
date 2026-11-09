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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

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
	case "trashRealPaths":
		paths := stringsOf(payload["paths"])
		if len(paths) == 0 {
			return nil
		}
		requestID, wantRefs := stringOf(payload["requestId"]), boolOf(payload["configRefs"])
		go func() {
			// The page waits for this receipt before it claims success, so a
			// refused or failed path is reported instead of swallowed.
			var outcome scanner.TrashOutcome
			var refs []scanner.ConfigRef
			var services *scanner.UninstallReceipt
			if wantRefs { // an uninstall: stop the removed apps' user-level background services around the move
				var receipt scanner.UninstallReceipt
				outcome, receipt = scanner.Shared.TrashUninstall(paths, fileIdentitiesOf(payload["identities"]))
				services = &receipt
			} else {
				outcome = scanner.Shared.TrashPathsChecked(paths, fileIdentitiesOf(payload["identities"]))
			}
			if wantRefs { // list, read only, config lines that still name what was removed
				moved := make([]string, 0, len(outcome.Moved))
				for _, entry := range outcome.Moved {
					moved = append(moved, entry.Original)
				}
				refs = scanner.Shared.ConfigReferences(moved)
			}
			pushTrashResult(requestID, outcome, refs, services)
			// The single rescan after a destructive action lives here, so the
			// window and the menu-bar panel share it; the page does not ask again.
			pushRealTelemetryNow()
			pushRealSystemScanNow()
		}()
	case "scanSlim":
		requestID := stringOf(payload["requestId"])
		go func() {
			result := scanner.Shared.ScanSlim()
			evaluate("window.onNativeSlimScan ? window.onNativeSlimScan(" + jsonOf(map[string]any{"requestId": requestID, "scan": result}) + ") : undefined")
		}()
	case "slimApps":
		paths, identities, requestID := stringsOf(payload["paths"]), stringMapOf(payload["identities"]), stringOf(payload["requestId"])
		if len(paths) == 0 {
			return nil
		}
		go func() {
			targets := make([]scanner.SlimTarget, 0, len(paths))
			for _, path := range paths {
				targets = append(targets, scanner.SlimTarget{Path: path, Identity: identities[path]})
			}
			outcome := scanner.Shared.SlimApps(targets)
			evaluate("window.onNativeSlimResult ? window.onNativeSlimResult(" + jsonOf(map[string]any{"requestId": requestID, "outcome": outcome}) + ") : undefined")
			pushRealTelemetryNow()
			pushRealSystemScanNow()
		}()
	case "restoreTrashSnapshot":
		// The page's "可还原/已还原" buttons used to be decoration: the Swift
		// app ignored this action and the Go port inherited that. Restoring is
		// now a real move-back through the app's own manifest.
		originals := stringsOf(payload["paths"])
		trashed := stringMapOf(payload["trashedPaths"])
		requestID := stringOf(payload["requestId"])
		go func() {
			pushRestoreResult(requestID, scanner.Shared.RestoreTrash(originals, trashed))
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
			if err == nil {
				pushRealSystemScanNow()
			}
		}()
	case "addPurgeRoot", "removePurgeRoot":
		action := stringOf(payload["action"])
		path := stringOf(payload["path"])
		if path == "" {
			return nil
		}
		go func() {
			var roots scanner.PurgeRoots
			var err error
			if action == "addPurgeRoot" {
				roots, err = scanner.Shared.AddPurgeRoot(path)
			} else {
				roots, err = scanner.Shared.RemovePurgeRoot(path)
			}
			pushPurgeRootsChanged(action, path, roots, err)
			if err == nil {
				pushRealSystemScanNow()
			}
		}()
	case "killProcess":
		pid, name, path, requestID := intOf(payload["pid"]), stringOf(payload["name"]), stringOf(payload["path"]), stringOf(payload["requestId"])
		go func() {
			err := quitProcess(pid, name, path)
			result := map[string]any{"pid": pid, "requestId": requestID, "ok": err == nil}
			if err != nil {
				result["error"] = err.Error()
			}
			evaluate("window.onNativeProcessQuitResult ? window.onNativeProcessQuitResult(" + jsonOf(result) + ") : undefined")
			pushRealTelemetryNow()
		}()
	case "measurePathFully":
		path, requestID := stringOf(payload["path"]), stringOf(payload["requestId"])
		go func() { pushMeasureResult(requestID, scanner.Shared.MeasurePathFully(path)) }()
	case "requestRealSystemScan":
		pushRealSystemScanNow()
	case "requestDiskAnalyze":
		pushDiskAnalyze(stringOf(payload["path"]), boolOf(payload["refresh"]), stringOf(payload["requestId"]))
	case "cancelDiskAnalyze":
		go cancelDiskAnalyze(stringOf(payload["requestId"]))
	case "requestRealTelemetry":
		pushRealTelemetryNow()
	case "requestRuntimeStatus":
		checkFolders, _ := payload["checkFolders"].(bool)
		pushRuntimeStatus(stringOf(payload["requestId"]), checkFolders)
	case "relaunchApp":
		relaunchApp()
	case "menubarShowTools":
		showFlyout()
	case "requestOptimizePlan":
		requestID := stringOf(payload["requestId"])
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			pushOptimizePlan(requestID, scanner.Shared.DiagnoseOptimize(ctx))
		}()
	case "executeOptimize":
		requestID, ids := stringOf(payload["requestId"]), stringsOf(payload["tasks"])
		go func() {
			// Each task carries its own time limit; this only bounds a hung run.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			err := scanner.Shared.RunOptimize(ctx, ids, func(r scanner.OptimizeReceipt) { pushOptimizeReceipt(requestID, r) })
			pushOptimizeDone(requestID, err)
		}()
	case "undoOptimize":
		requestID, id := stringOf(payload["requestId"]), stringOf(payload["task"])
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			receipt, err := scanner.Shared.UndoOptimize(ctx, id)
			pushOptimizeUndo(requestID, id, receipt, err)
		}()
	case "releaseFileCache":
		requestID := stringOf(payload["requestId"])
		confirmed, _ := payload["confirmed"].(bool)
		started, err := beginFileCachePurge(requestID, confirmed)
		if err != nil {
			evaluate("window.onNativeCachePurgeResult ? window.onNativeCachePurgeResult(" + jsonOf(map[string]any{"requestId": requestID, "ok": false, "error": err.Error()}) + ") : undefined")
			return nil
		}
		if started {
			go executeFileCachePurge(requestID)
		}
	case "toggleCaffeinate":
		setCaffeinate(boolOf(payload["active"]))
	case "menubarOpenClean", "menubarOpenMemory":
		showMainWindow(menuBarNavigationScript(stringOf(payload["action"]), ""))
	case "menubarOpenStatus", "menubarOpenAnalyze":
		showMainWindow(menuBarNavigationScript(stringOf(payload["action"]), stringOf(payload["query"])))
	case "menubarOpenDoctor":
		showMainWindow("openDoctorModal(true);")
	case "menubarShowMain":
		showMainWindow("")
	case "menubarQuit":
		mygo.App.Quit()
	case "checkForUpdates":
		// Returns at once; the updater opens its own window, and says so when
		// this build cannot update (a dev build, or one without the key).
		updater.CheckForUpdates()
	case "setTheme":
		if theme := stringOf(payload["theme"]); theme == themeDark || theme == themeLight || theme == themeSystem {
			applyTheme(theme, true)
		}
	case "setLanguage":
		// The page owns the choice (localStorage orbit_lang) and says whether the
		// user made it; Go keeps it for the menu, tooltip and update window.
		applyLanguage(stringOf(payload["language"]), payload["explicit"] == true)
	case "openPrivacySettings":
		// A failed clean or uninstall is usually a missing TCC grant, and the
		// switch that fixes it lives two clicks deep in System Settings.
		go openPrivacyPane(stringOf(payload["pane"]))
	case "revealAppBundle":
		// Full Disk Access has no path parameter. Showing the running bundle in
		// Finder lets the user add that exact build with System Settings' "+".
		revealAppBundle()
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

const fileCachePurgeScript = `with timeout of 80 seconds
do shell script "/usr/sbin/purge" with administrator privileges
end timeout`

var fileCachePurgeState struct {
	sync.Mutex
	requestID string
	unknown   bool
}

func beginFileCachePurge(requestID string, confirmed bool) (bool, error) {
	fileCachePurgeState.Lock()
	defer fileCachePurgeState.Unlock()
	if !confirmed || requestID == "" || len(requestID) > 128 {
		return false, fmt.Errorf("需要明确确认和有效的请求标识，未执行文件缓存释放")
	}
	if fileCachePurgeState.requestID != "" {
		if fileCachePurgeState.requestID == requestID {
			return false, nil
		}
		return false, fmt.Errorf("已有文件缓存请求等待系统回执，未重复执行")
	}
	if fileCachePurgeState.unknown {
		return false, fmt.Errorf("上次文件缓存命令状态未知，本次未重复执行；请先在活动监视器核对 purge 进程")
	}
	fileCachePurgeState.requestID = requestID
	return true, nil
}

func fileCachePurgeCommand(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "/usr/bin/osascript", "-e", fileCachePurgeScript)
}

func fileCachePurgeOutcome(requestID string, output []byte, err, contextErr error) map[string]any {
	result := optimizeOutcome(requestID, output, err)
	if err != nil {
		result["cancelled"] = strings.Contains(string(output), "(-128)")
		result["unknown"] = contextErr != nil || strings.Contains(string(output), "(-1712)")
	}
	return result
}

func executeFileCachePurge(requestID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	output, err := fileCachePurgeCommand(ctx).CombinedOutput()
	result := fileCachePurgeOutcome(requestID, output, err, ctx.Err())
	fileCachePurgeState.Lock()
	fileCachePurgeState.requestID = ""
	fileCachePurgeState.unknown = result["unknown"] == true
	fileCachePurgeState.Unlock()
	evaluate("window.onNativeCachePurgeResult ? window.onNativeCachePurgeResult(" + jsonOf(result) + ") : undefined")
	pushRealTelemetryNow()
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

func appBundleFromExecutable(executable string) string {
	path := filepath.Clean(executable)
	for current := path; current != "/" && current != "."; current = filepath.Dir(current) {
		if strings.HasSuffix(strings.ToLower(current), ".app") {
			return current
		}
	}
	return path
}

func revealAppBundle() {
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return
	}
	mygo.Shell.ShowItemInFolder(appBundleFromExecutable(executable))
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

// Recheck ownership and identity at the native boundary; never escalate to SIGKILL.
func quitProcess(pid int, name, path string) error {
	if reason := processQuitReason(pid, os.Getuid(), path, name, path); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "uid=,comm=").Output()
	if err != nil {
		return fmt.Errorf("进程已退出或状态无法读取，未发送指令")
	}
	line := strings.TrimSpace(string(output))
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return fmt.Errorf("进程身份无法核验，未发送指令")
	}
	uid, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("进程所属用户无法核验，未发送指令")
	}
	if reason := processQuitReason(pid, uid, strings.TrimSpace(line[len(fields[0]):]), name, path); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("请求退出失败：%w", err)
	}
	return nil
}

func processQuitReason(pid, uid int, currentPath, expectedName, expectedPath string) string {
	if pid <= 1 || pid == os.Getpid() || pid == os.Getppid() {
		return "系统或当前应用进程已保护"
	}
	if uid != os.Getuid() {
		return "其它用户或系统进程已保护"
	}
	name := filepath.Base(currentPath)
	for _, protected := range []string{"kernel_task", "launchd", "WindowServer", "loginwindow", "Finder", "Dock", "SystemUIServer", "MoleStudio", "mole-studio", "orbit"} {
		if strings.EqualFold(name, protected) {
			return "系统关键进程或当前应用已保护"
		}
	}
	if expectedName == "" || expectedPath == "" || name != expectedName || currentPath != expectedPath {
		return "进程身份已变化，请刷新列表后重新确认"
	}
	return ""
}

// caffeinateHold is the active keep-awake assertion, if any. It is a MyGo power
// assertion rather than a caffeinate subprocess: the system drops it when the
// app exits, so it can never outlive the app.
var (
	caffeinateMu   sync.Mutex
	caffeinateHold *caffeinateAssertion
)

type caffeinateAssertion struct {
	release func()
	timer   *time.Timer
}

func currentCaffeinateActive() bool {
	caffeinateMu.Lock()
	defer caffeinateMu.Unlock()
	return caffeinateHold != nil
}

func menuBarNavigationScript(action, query string) string {
	if action == "menubarOpenClean" {
		return "openQuickClean();"
	}
	if action == "menubarOpenMemory" {
		return "openMemoryMaintenance();"
	}
	if action == "menubarOpenAnalyze" {
		return "switchModule('analyze');"
	}
	return "switchModule('status'); handleSearch(" + jsonOf(query) + ");"
}

// setCaffeinate keeps the display and the system awake while active, for at
// most two hours, as the Swift app's `caffeinate -d -i -t 7200` did. Turning it
// off, or the timer, releases the assertion.
func setCaffeinate(active bool) {
	caffeinateMu.Lock()
	if hold := caffeinateHold; hold != nil {
		hold.timer.Stop()
		hold.release()
		caffeinateHold = nil
	}
	if !active {
		caffeinateMu.Unlock()
		pushCaffeinateState(false)
		return
	}
	hold := &caffeinateAssertion{release: mygo.Power.KeepAwake(L("orbit 保持唤醒", "orbit is keeping the Mac awake"), true)}
	// The timer tells the page too, instead of leaving the switch on forever.
	hold.timer = time.AfterFunc(2*time.Hour, func() {
		caffeinateMu.Lock()
		if caffeinateHold != hold {
			caffeinateMu.Unlock()
			return
		}
		caffeinateHold = nil
		caffeinateMu.Unlock()
		hold.release()
		pushCaffeinateState(false)
	})
	caffeinateHold = hold
	caffeinateMu.Unlock()
	pushCaffeinateState(true)
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

// fileIdentitiesOf reads the {path: {size, mtime}} map the page sends with a
// move, so a file that changed since the scan is not trashed.
func fileIdentitiesOf(value any) map[string]scanner.FileIdentity {
	values, _ := value.(map[string]any)
	result := make(map[string]scanner.FileIdentity, len(values))
	for path, raw := range values {
		if entry, ok := raw.(map[string]any); ok {
			result[path] = scanner.FileIdentity{Size: int64(intOf(entry["size"])), Mtime: int64(intOf(entry["mtime"]))}
		}
	}
	return result
}

func stringMapOf(value any) map[string]string {
	if values, ok := value.(map[string]string); ok {
		return values
	}
	values, _ := value.(map[string]any)
	result := make(map[string]string, len(values))
	for key, value := range values {
		if value, ok := value.(string); ok {
			result[key] = value
		}
	}
	return result
}
