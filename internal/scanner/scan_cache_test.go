package scanner

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessSnapshotReusesDisplayReadsButNotLiveOnes(t *testing.T) {
	calls := 0
	s := &Scanner{processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		return "node\n", nil
	}}
	for i := 0; i < 3; i++ {
		if _, err := s.processSnapshot(false); err != nil {
			t.Fatalf("processSnapshot(display) failed: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("display reads ran ps %d times, want 1 within the TTL", calls)
	}
	s.processSnapshot(true)
	s.processSnapshot(true)
	if calls != 3 {
		t.Fatalf("live reads must always run ps: calls = %d, want 3", calls)
	}
	s.psAt = time.Now().Add(-2 * time.Hour)
	s.processSnapshot(false)
	if calls != 4 {
		t.Fatalf("an expired snapshot must be refreshed: calls = %d, want 4", calls)
	}
}

func TestProcessSnapshotWithoutTTLNeverCaches(t *testing.T) {
	calls := 0
	s := &Scanner{processProbe: func() (string, error) {
		calls++
		return "", nil
	}}
	s.processSnapshot(false)
	s.processSnapshot(false)
	if calls != 2 {
		t.Fatalf("a zero TTL must not cache: calls = %d, want 2", calls)
	}
}

func TestProcessSnapshotDoesNotCacheFailures(t *testing.T) {
	fail := true
	calls := 0
	s := &Scanner{processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		if fail {
			return "", errors.New("ps failed")
		}
		return "node\n", nil
	}}
	if _, err := s.processSnapshot(false); err == nil {
		t.Fatal("processSnapshot must report a failed ps")
	}
	fail = false
	if out, err := s.processSnapshot(false); err != nil || out != "node\n" {
		t.Fatalf("processSnapshot after a failure = %q, %v; want a fresh read", out, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestAIHandleOpenUsesTheSnapshotOnlyForDisplay(t *testing.T) {
	snapshotCalls, pathCalls := 0, 0
	s := &Scanner{openSnapshot: func(string) (bool, error) {
		snapshotCalls++
		return false, nil
	}}
	s.handleProbe = nil
	if _, err := s.aiHandleOpen("/nonexistent/orbit-handle-test", false); err != nil || snapshotCalls != 1 {
		t.Fatalf("display check: snapshotCalls = %d, err = %v; want the snapshot used once", snapshotCalls, err)
	}
	s.aiHandleOpen("/nonexistent/orbit-handle-test", true)
	if snapshotCalls != 1 {
		t.Fatalf("the live check must not read the snapshot: snapshotCalls = %d", snapshotCalls)
	}
	s.handleProbe = func(string) (bool, error) {
		pathCalls++
		return true, nil
	}
	if open, _ := s.aiHandleOpen("/x", false); !open || pathCalls != 1 || snapshotCalls != 1 {
		t.Fatalf("an injected probe wins over the snapshot: open=%v pathCalls=%d snapshotCalls=%d", open, pathCalls, snapshotCalls)
	}
	if open, _ := s.aiHandleOpen("/x", true); !open || pathCalls != 2 {
		t.Fatalf("live check must ask the probe: open=%v pathCalls=%d", open, pathCalls)
	}
}

func TestScanViewKeepsProbesAndHasItsOwnProcessCache(t *testing.T) {
	calls := 0
	s := &Scanner{home: t.TempDir(), processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		return "node\n", nil
	}}
	view := s.scanView()
	view.processSnapshot(false)
	view.processSnapshot(false)
	if calls != 1 || view.openSnapshot == nil || view.home != s.home {
		t.Fatalf("scanView: calls = %d, openSnapshot nil = %v, home = %q", calls, view.openSnapshot == nil, view.home)
	}
	if s.openSnapshot != nil {
		t.Fatal("the scanner itself must not carry a display snapshot")
	}
}

func TestScanViewAppActivityIsRecheckedAtMoveBoundary(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Editor.app")
	helper := filepath.Join(app, "Contents/Frameworks/Helper.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.microsoft.VSCode</string></dict></plist>`)
	writeFile(t, filepath.Join(helper, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.Helper</string></dict></plist>`)
	cache := fixtureDir(t, home, "Library/Application Support/Code/Code Cache", 4096)
	helperCache := fixtureDir(t, home, "Library/Caches/com.example.Helper", 4096)
	processes := "/sbin/launchd\n"
	s := &Scanner{home: home, processProbe: func() (string, error) { return processes, nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	view := s.scanView()
	if reason := view.CleanupReason(cache); reason != "" {
		t.Fatalf("idle cache refused: %s", reason)
	}
	processes = "912 " + filepath.Join(helper, "Contents/MacOS/Helper") + "\n"
	for _, path := range []string{cache, helperCache} {
		if reason := view.cleanupReason(path, true); !strings.Contains(reason, "正在运行") {
			t.Errorf("live move missed helper activity: %s: %q", path, reason)
		}
		if reason := s.scanView().CleanupReason(path); !strings.Contains(reason, "正在运行") {
			t.Errorf("new scan reused an idle app snapshot: %s: %q", path, reason)
		}
	}
	processes = ""
	if reason := view.cleanupReason(cache, true); !strings.Contains(reason, "无法确认") {
		t.Fatalf("live move accepted an empty process table: %q", reason)
	}
}
