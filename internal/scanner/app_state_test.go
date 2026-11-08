package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const launchPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>%LABEL%</string><key>ProgramArguments</key><array><string>%PROGRAM%</string><string>--daemon</string></array><key>RunAtLoad</key><true/></dict></plist>`

func launchPlist(label, program string) string {
	return strings.NewReplacer("%LABEL%", label, "%PROGRAM%", program).Replace(launchPlistTemplate)
}

func TestLaunchItemsMatchByBundlePathLabelAndNeverBarePrefix(t *testing.T) {
	home := t.TempDir()
	app := "/Applications/Lemon.app"
	writeFile(t, filepath.Join(home, "Library/LaunchAgents/com.tencent.Lemon.plist"), launchPlist("com.tencent.Lemon", "/bin/sh"))
	writeFile(t, filepath.Join(home, "Library/LaunchAgents/com.tencent.Lemon.helper.plist"), launchPlist("com.tencent.Lemon.helper", "/bin/sh"))
	writeFile(t, filepath.Join(home, "Library/LaunchAgents/other.plist"), launchPlist("org.other.job", app+"/Contents/Library/LoginItems/Agent.app/Contents/MacOS/Agent"))
	writeFile(t, filepath.Join(home, "Library/LaunchAgents/com.tencent.LemonMonitor.plist"), launchPlist("com.tencent.LemonMonitor", "/usr/local/bin/monitor"))
	writeFile(t, filepath.Join(home, "Library/LaunchAgents/unrelated.plist"), launchPlist("org.unrelated", "/usr/bin/true"))
	items := (&Scanner{home: home}).loadLaunchItems()
	got := map[string]bool{}
	for _, path := range launchPathsFor(items, app, "com.tencent.Lemon") {
		got[filepath.Base(path)] = true
	}
	for _, want := range []string{"com.tencent.Lemon.plist", "com.tencent.Lemon.helper.plist", "other.plist"} {
		if !got[want] {
			t.Errorf("%s was not matched: %v", want, got)
		}
	}
	if got["com.tencent.LemonMonitor.plist"] || got["unrelated.plist"] {
		t.Errorf("a bare prefix or unrelated job was claimed: %v", got)
	}
	if paths := launchPathsFor(items, app, "local.app.Lemon"); len(paths) != 1 {
		t.Errorf("a synthesized bundle id must only match by executable path: %v", paths)
	}
}

func TestParseMdlsLastUsedKeepsUnknownUnknown(t *testing.T) {
	got := parseMdlsLastUsed("(null)\x002026-10-28 05:44:12 +0000\x00garbage", 3)
	if got[0] != nil || got[2] != nil || got[1] == nil || got[1].Unix() != 1793166252 {
		t.Fatalf("parsed %v", got)
	}
	// mdls aborts on a missing file and prints the error instead of N values.
	for _, value := range parseMdlsLastUsed("/x.app: could not find /y.app.\n", 2) {
		if value != nil {
			t.Fatal("a count mismatch must not invent dates")
		}
	}
}

func TestRunningAppIsDetectedAndRefusedAtTheTrashBoundary(t *testing.T) {
	s := newTestScanner(t)
	app := filepath.Join(s.home, "Applications/Example.app")
	writeFile(t, filepath.Join(app, "Contents/MacOS/Example"), "fixture")
	helper := app + "/Contents/Frameworks/Helper.app/Contents/MacOS/Helper"
	s.processProbe = func() (string, error) {
		return "  501 /usr/bin/true\n  900 " + helper + "\n  812 " + app + "/Contents/MacOS/Example\n", nil
	}
	running, main := appRunning(s.runningProcesses(), app)
	if !running || main.pid != 812 || main.path != app+"/Contents/MacOS/Example" {
		t.Fatalf("running=%v main=%+v", running, main)
	}
	if running, _ := appRunning(s.runningProcesses(), filepath.Join(s.home, "Applications/Other.app")); running {
		t.Fatal("an unrelated app was reported running")
	}
	rows := s.scanInstalledApps(filepath.Join(s.home, "Applications"))
	if len(rows) != 1 || rows[0]["running"] != true || rows[0]["runningPid"] != 812 {
		t.Fatalf("payload lost the running state: %+v", rows)
	}
	outcome := s.TrashPaths([]string{app})
	if len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 || !strings.Contains(outcome.Skipped[0].Reason, "运行") {
		t.Fatalf("a running app bundle was moved: %+v", outcome)
	}
	if _, err := os.Stat(app); err != nil {
		t.Fatal("the running bundle disappeared")
	}
}

func TestInstalledAppsListUserLaunchItemsAndRefuseSystemOnes(t *testing.T) {
	s := newTestScanner(t)
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	app := filepath.Join(s.home, "Applications/Example.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.app</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/Example"), "fixture")
	agent := filepath.Join(s.home, "Library/LaunchAgents/com.example.app.plist")
	writeFile(t, agent, launchPlist("com.example.app", "/bin/sh"))
	rows := s.scanInstalledApps(filepath.Join(s.home, "Applications"))
	var found map[string]any
	for _, sp := range rows[0]["subpaths"].([]map[string]any) {
		if sp["kind"] == "launch" {
			found = sp
		}
	}
	if found == nil || found["checked"] != true || found["tier"] != nil || !strings.Contains(rows[0]["impact"].(string), "注销或重启") {
		t.Fatalf("user launch item missing or unsafe: %+v", found)
	}
	for _, path := range []string{"/Library/LaunchDaemons/com.example.app.plist", "/Library/LaunchAgents/com.example.app.plist"} {
		if reason := s.CleanupReason(path); !strings.Contains(reason, "管理员") {
			t.Errorf("%s must be listed only, got %q", path, reason)
		}
	}
}
