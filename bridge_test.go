package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestFileCachePurgeNeedsConfirmationAndCannotOverlap(t *testing.T) {
	fileCachePurgeState.Lock()
	previousID, previousUnknown := fileCachePurgeState.requestID, fileCachePurgeState.unknown
	fileCachePurgeState.requestID, fileCachePurgeState.unknown = "", false
	fileCachePurgeState.Unlock()
	t.Cleanup(func() {
		fileCachePurgeState.Lock()
		fileCachePurgeState.requestID, fileCachePurgeState.unknown = previousID, previousUnknown
		fileCachePurgeState.Unlock()
	})
	for _, request := range []struct {
		id        string
		confirmed bool
	}{{"cache-1", false}, {"", true}, {strings.Repeat("x", 129), true}} {
		if started, err := beginFileCachePurge(request.id, request.confirmed); started || err == nil {
			t.Fatal("an unconfirmed or invalid native request was accepted")
		}
	}
	if started, err := beginFileCachePurge("cache-1", true); !started || err != nil {
		t.Fatalf("confirmed request = %v, %v", started, err)
	}
	if started, err := beginFileCachePurge("cache-1", true); started || err != nil {
		t.Fatal("duplicate request must neither run again nor send a false completion receipt")
	}
	if started, err := beginFileCachePurge("cache-2", true); started || err == nil {
		t.Fatal("another page can overlap the pending command")
	}
	fileCachePurgeState.Lock()
	fileCachePurgeState.requestID, fileCachePurgeState.unknown = "", true
	fileCachePurgeState.Unlock()
	if started, err := beginFileCachePurge("cache-3", true); started || err == nil {
		t.Fatal("unknown privileged execution must not permit another purge")
	}
}

func TestFileCachePurgeUsesOnlyTheFixedSystemCommand(t *testing.T) {
	command := fileCachePurgeCommand(context.Background())
	if command.Path != "/usr/bin/osascript" || len(command.Args) != 3 || command.Args[1] != "-e" || command.Args[2] != fileCachePurgeScript {
		t.Fatalf("authorization command = %+v", command.Args)
	}
	if strings.Count(fileCachePurgeScript, "/usr/sbin/purge") != 1 || !strings.Contains(fileCachePurgeScript, "with administrator privileges") || strings.Contains(fileCachePurgeScript, "sudo") {
		t.Fatal("purge must use explicit native authorization, not a password pipe or variable shell command")
	}
}

func TestFileCachePurgeReceiptSeparatesCancellationFailureAndUnknown(t *testing.T) {
	for _, test := range []struct {
		name, output        string
		err, contextErr     error
		ok, cancel, unknown bool
	}{
		{"success", "", nil, nil, true, false, false},
		{"cancelled authorization", "User canceled. (-128)", errors.New("exit status 1"), nil, false, true, false},
		{"authorization denied", "not authorized", errors.New("exit status 1"), nil, false, false, false},
		{"native timeout", "", context.DeadlineExceeded, context.DeadlineExceeded, false, false, true},
		{"Apple event timeout", "AppleEvent timed out. (-1712)", errors.New("exit status 1"), nil, false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := fileCachePurgeOutcome("cache-receipt", []byte(test.output), test.err, test.contextErr)
			if result["requestId"] != "cache-receipt" || (result["ok"] == true) != test.ok || (result["cancelled"] == true) != test.cancel || (result["unknown"] == true) != test.unknown {
				t.Fatalf("receipt = %v", result)
			}
			if !test.ok && result["error"] == nil {
				t.Fatal("native command failure lost its diagnostic")
			}
		})
	}
}

func TestProcessQuitRechecksIdentityAndProtectsTheNativeBoundary(t *testing.T) {
	const pid = 812345
	const path = "/Applications/Canvas Editor.app/Contents/MacOS/Canvas Editor"
	for _, tc := range []struct {
		name                                string
		pid, uid                            int
		current, expectedName, expectedPath string
	}{
		{"system pid", 1, os.Getuid(), path, "Canvas Editor", path},
		{"self", os.Getpid(), os.Getuid(), path, "Canvas Editor", path},
		{"parent", os.Getppid(), os.Getuid(), path, "Canvas Editor", path},
		{"other owner", pid, os.Getuid() + 1, path, "Canvas Editor", path},
		{"critical name", pid, os.Getuid(), "/System/Library/Finder", "Finder", "/System/Library/Finder"},
		{"recycled pid", pid, os.Getuid(), "/usr/bin/different", "Canvas Editor", path},
		{"missing identity", pid, os.Getuid(), path, "", ""},
		{"command name only", pid, os.Getuid(), "Canvas", "Canvas", "Canvas"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if processQuitReason(tc.pid, tc.uid, tc.current, tc.expectedName, tc.expectedPath) == "" {
				t.Fatal("unverified or protected process was accepted")
			}
		})
	}
	if processQuitReason(pid, os.Getuid(), path, "Canvas Editor", path) != "" {
		t.Fatal("a verified user process with spaces in its path was refused")
	}
	if quitProcess(1, "launchd", "/sbin/launchd") == nil {
		t.Fatal("native action did not reject a critical PID before signaling")
	}
}

func TestTrashReceiptMapsAcceptOnlyStringTargets(t *testing.T) {
	values := stringMapOf(map[string]any{"/original": "/trash/item", "/invalid": 12})
	if len(values) != 1 || values["/original"] != "/trash/item" {
		t.Fatalf("receipt targets = %+v", values)
	}
	if len(stringMapOf(nil)) != 0 || len(stringMapOf([]string{"/trash"})) != 0 {
		t.Fatal("malformed receipt maps were accepted")
	}
	if stringMapOf(map[string]string{"a": "b"})["a"] != "b" {
		t.Fatal("typed native callers lost their receipt targets")
	}
}

func TestMenuBarNavigationKeepsReviewInTheMainWindow(t *testing.T) {
	if menuBarNavigationScript("menubarOpenClean", "") != "openQuickClean();" || menuBarNavigationScript("menubarOpenMemory", "") != "openMemoryMaintenance();" {
		t.Fatal("quick tools must open their review in the main window rather than execute from the popup")
	}
	if script := menuBarNavigationScript("menubarOpenAnalyze", "ignored"); script != "switchModule('analyze');" {
		t.Fatalf("analysis route = %q", script)
	}
	for _, query := range []string{"", "8123", "Owner's \"process\"\n</script>"} {
		script := menuBarNavigationScript("menubarOpenStatus", query)
		if script != "switchModule('status'); handleSearch("+jsonOf(query)+");" {
			t.Fatalf("status route does not safely preserve its query: %q", script)
		}
		if strings.Contains(script, "openConfirmSheet") || strings.Contains(script, "killProcess") {
			t.Fatal("menu bar navigation authorized a destructive action")
		}
	}
}

func TestSystemScanRequestsCoalesceButKeepOneFreshScan(t *testing.T) {
	systemScanQueue.Lock()
	previousRunning, previousAgain := systemScanQueue.running, systemScanQueue.again
	systemScanQueue.running, systemScanQueue.again = false, false
	systemScanQueue.Unlock()
	t.Cleanup(func() {
		systemScanQueue.Lock()
		systemScanQueue.running, systemScanQueue.again = previousRunning, previousAgain
		systemScanQueue.Unlock()
	})
	if !queueSystemScan() {
		t.Fatal("first scan did not start")
	}
	for i := 0; i < 30; i++ {
		if queueSystemScan() {
			t.Fatal("overlapping request started another scan")
		}
	}
	if !nextSystemScan() || nextSystemScan() {
		t.Fatal("duplicates must coalesce into exactly one follow-up scan")
	}
	if !queueSystemScan() || nextSystemScan() {
		t.Fatal("the idle queue must accept a new scan and return idle afterward")
	}
}

func TestOptimizeOutcomeReportsActualCommandResult(t *testing.T) {
	success := optimizeOutcome("opt-1", []byte("cache reset"), nil)
	if success["ok"] != true || success["requestId"] != "opt-1" {
		t.Fatalf("success = %v", success)
	}
	failure := optimizeOutcome("opt-2", []byte("Permission denied"), errors.New("exit status 1"))
	if failure["ok"] != false || !strings.Contains(failure["error"].(string), "Permission denied") {
		t.Fatalf("failure = %v", failure)
	}
}

func TestCancelAnalysisDropsOnlyTheRequestedQueuedScan(t *testing.T) {
	analyzeQueue.mu.Lock()
	previousCurrent, previousNext, previousRunning := analyzeQueue.current, analyzeQueue.next, analyzeQueue.running
	analyzeQueue.current = analyzeRequest{id: "current"}
	analyzeQueue.next = &analyzeRequest{id: "latest", path: "/tmp"}
	analyzeQueue.running = true
	analyzeQueue.mu.Unlock()
	t.Cleanup(func() {
		analyzeQueue.mu.Lock()
		analyzeQueue.current, analyzeQueue.next, analyzeQueue.running = previousCurrent, previousNext, previousRunning
		analyzeQueue.mu.Unlock()
	})
	cancelDiskAnalyze("obsolete")
	analyzeQueue.mu.Lock()
	next := analyzeQueue.next
	analyzeQueue.mu.Unlock()
	if next == nil || next.id != "latest" {
		t.Fatal("an obsolete cancel request affected the newest queued scan")
	}
	cancelDiskAnalyze("latest")
	analyzeQueue.mu.Lock()
	dropped := analyzeQueue.next == nil
	analyzeQueue.mu.Unlock()
	if !dropped {
		t.Fatal("a canceled queued scan would still start later")
	}
}

func TestPrivacyPaneURLCoversBothPermissions(t *testing.T) {
	if got := privacyPaneURL("appManagement"); !strings.Contains(got, "Privacy_AppBundles") {
		t.Fatalf("appManagement URL = %q", got)
	}
	if got := privacyPaneURL("fullDiskAccess"); !strings.Contains(got, "Privacy_AllFiles") {
		t.Fatalf("fullDiskAccess URL = %q", got)
	}
	// A receipt without a pane (or with an unknown one) must still open
	// something useful: the permission a clean needs.
	if got := privacyPaneURL(""); !strings.Contains(got, "Privacy_AllFiles") {
		t.Fatalf("default URL = %q", got)
	}
}

func TestPrivacyPaneURLKnowsFilesAndFolders(t *testing.T) {
	if got := privacyPaneURL("filesAndFolders"); !strings.Contains(got, "Privacy_FilesAndFolders") {
		t.Fatalf("filesAndFolders URL = %q", got)
	}
}

func TestAppBundleFromExecutableFindsTheOuterBundle(t *testing.T) {
	got := appBundleFromExecutable("/Applications/orbit.app/Contents/MacOS/orbit")
	if got != "/Applications/orbit.app" {
		t.Fatalf("bundle path = %q", got)
	}
	plain := appBundleFromExecutable("/tmp/go-build/orbit")
	if plain != "/tmp/go-build/orbit" {
		t.Fatalf("plain executable path = %q", plain)
	}
}

// Dispatch drops an action it does not know without a word, and the page's
// sendNativeBridge swallows errors, so a typo on either side is invisible. This
// ties the two together: every action the page can post must have a case.
func TestDispatchHandlesEveryActionThePagePosts(t *testing.T) {
	page, err := os.ReadFile("Resources/src-ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	// Literal actions, plus the menubar routes the page passes around by name.
	posted := map[string]bool{}
	for _, m := range regexp.MustCompile(`action:\s*['"](\w+)['"]|'(menubar(?:Open|Show)[A-Za-z]+|menubarQuit)'`).FindAllStringSubmatch(string(page), -1) {
		posted[m[1]+m[2]] = true
	}
	if len(posted) < 20 {
		t.Fatalf("found only %d page actions; the pattern no longer matches the page", len(posted))
	}

	file, err := parser.ParseFile(token.NewFileSet(), "bridge.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	handled := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Dispatch" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if clause, ok := n.(*ast.CaseClause); ok {
				for _, expr := range clause.List {
					if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						name, _ := strconv.Unquote(lit.Value)
						handled[name] = true
					}
				}
			}
			return true
		})
	}
	for action := range posted {
		if !handled[action] {
			t.Errorf("the page posts %q but Dispatch has no case for it", action)
		}
	}
}

func TestMenuBarOperationReceiptsReachHiddenPanel(t *testing.T) {
	want := map[string]bool{
		"onNativeProcessQuitResult": true, "onNativeCachePurgeResult": true,
		"onNativeTrashResult": true, "onNativeDoctorReport": true,
		"onNativeCaffeinate": true, "onNativeRuntimeStatus": true,
		"onNativeRealTelemetry": false,
	}
	seen := map[string]bool{}
	for _, path := range []string{"bridge.go", "push.go", "runtime.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "evaluate" {
				return true
			}
			ast.Inspect(call.Args[0], func(n ast.Node) bool {
				literal, ok := n.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				text, _ := strconv.Unquote(literal.Value)
				for callback, hidden := range want {
					if !strings.Contains(text, "window."+callback) {
						continue
					}
					seen[callback] = true
					actual := false
					if len(call.Args) == 2 {
						flag, ok := call.Args[1].(*ast.Ident)
						actual = ok && flag.Name == "true"
					}
					if actual != hidden {
						t.Errorf("%s hidden-panel delivery = %v, want %v", callback, actual, hidden)
					}
				}
				return true
			})
			return true
		})
	}
	for callback := range want {
		if !seen[callback] {
			t.Errorf("missing native callback %s", callback)
		}
	}
}
