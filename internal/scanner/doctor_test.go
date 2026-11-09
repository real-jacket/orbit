package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullDiskReadCheckDoesNotHideAnotherDeniedProbe(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the denied fixture")
	}
	home := t.TempDir()
	for _, name := range []string{"Mail", "Safari"} {
		if err := os.MkdirAll(filepath.Join(home, "Library", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	denied := filepath.Join(home, "Library", "Safari")
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o700) })
	result := (&Scanner{home: home}).FullDiskReadCheck()
	if result["passed"] != false {
		t.Fatalf("one readable probe hid another denied directory: %v", result)
	}
	checks, ok := result["checks"].([]map[string]any)
	if !ok || len(checks) != 2 || checks[1]["path"] != "~/Library/Safari" {
		t.Fatalf("read evidence does not name each checked directory: %v", result)
	}
}

func TestFullDiskReadCheckKeepsUnusableProbeUnknown(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "Mail"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "Library", "Safari"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := (&Scanner{home: home}).FullDiskReadCheck(); result["passed"] != nil {
		t.Fatalf("an unusable probe cannot be presented as fully readable: %v", result)
	}
}

func TestFolderReadChecksReportIndependentStatesWithoutReadingContents(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Desktop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "Desktop", "private-name.txt"), []byte("private contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "Documents"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := (&Scanner{home: home}).FolderReadChecks()
	if len(checks) != 3 || checks[0]["passed"] != true || checks[0]["status"] != "readable" || checks[1]["passed"] != nil || checks[1]["status"] != "unknown" || checks[2]["status"] != "missing" || checks[2]["passed"] != nil {
		t.Fatalf("folder outcomes = %v", checks)
	}
	for _, check := range checks {
		if check["id"] == "" || check["checkedAt"].(int64) <= 0 || strings.Contains(check["detail"].(string), "private") {
			t.Fatalf("permission result lost its identity/time or exposed file content: %v", check)
		}
	}
}

func TestFolderReadChecksSeparatePermissionDenialFromMissingDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the denied fixture")
	}
	home := t.TempDir()
	path := filepath.Join(home, "Documents")
	if err := os.Mkdir(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
	checks := (&Scanner{home: home}).FolderReadChecks()
	if checks[0]["status"] != "missing" || checks[1]["status"] != "denied" || checks[1]["passed"] != false || checks[2]["status"] != "missing" {
		t.Fatalf("missing paths were confused with a denied grant: %v", checks)
	}
}

func TestFullDiskReadCheckIncludesAllAvailableEvidence(t *testing.T) {
	home := t.TempDir()
	scanner := &Scanner{home: home}
	if result := scanner.FullDiskReadCheck(); result["passed"] != nil || result["status"] != "missing" {
		t.Fatalf("absent probes cannot prove authorization: %v", result)
	}
	if err := os.MkdirAll(filepath.Join(home, "Library", "Safari"), 0o700); err != nil {
		t.Fatal(err)
	}
	result := scanner.FullDiskReadCheck()
	if result["passed"] != true || result["status"] != "readable" || result["detail"] != "已授权" {
		t.Fatalf("a readable protected folder means granted: %v", result)
	}
}

func TestPermissionReadChecksDoNotClearScanWarnings(t *testing.T) {
	home := t.TempDir()
	for _, path := range []string{"Library/Mail", "Library/Safari", "Desktop", "Documents", "Downloads"} {
		if err := os.MkdirAll(filepath.Join(home, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{filepath.Join(home, "Library/Mail"), filepath.Join(home, "Desktop")}
	for _, path := range paths {
		recordReadWarning(path, "本次扫描测量未完成")
	}
	t.Cleanup(func() {
		directoryReads.Lock()
		defer directoryReads.Unlock()
		for _, path := range paths {
			delete(directoryReads.failed, path)
		}
	})
	scanner := &Scanner{home: home}
	if scanner.FullDiskReadCheck()["passed"] != true || scanner.FolderReadChecks()[0]["passed"] != true {
		t.Fatal("the isolated directories should remain readable")
	}
	warnings := scanReadWarnings(home)
	if len(warnings) != len(paths) {
		t.Fatalf("permission checks erased actual scan warnings: %v", warnings)
	}
	for _, warning := range warnings {
		if warning["reason"] != "本次扫描测量未完成" {
			t.Fatalf("a permission check changed the original scan reason: %v", warning)
		}
	}
}

func TestPermissionReadChecksDoNotCreateScanWarnings(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library"), 0o700); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(home, "Library/Mail"), filepath.Join(home, "Documents")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		directoryReads.Lock()
		defer directoryReads.Unlock()
		for _, path := range paths {
			delete(directoryReads.failed, path)
		}
	})
	scanner := &Scanner{home: home}
	if scanner.FullDiskReadCheck()["status"] != "unknown" || scanner.FolderReadChecks()[1]["status"] != "unknown" {
		t.Fatal("unusable permission probes must retain their own unknown results")
	}
	if warnings := scanReadWarnings(home); len(warnings) != 0 {
		t.Fatalf("permission probes fabricated warnings for a file scan that did not run: %v", warnings)
	}
}

func TestDoctorChecksHaveTitlesAndTriStateResults(t *testing.T) {
	checks := Shared.DoctorChecks()
	if len(checks) == 0 {
		t.Fatal("no checks reported; the machine can always answer at least one of them")
	}
	seen := map[string]bool{}
	for _, check := range checks {
		title, _ := check["title"].(string)
		if title == "" {
			t.Errorf("%v has no title", check)
		}
		if seen[title] {
			t.Errorf("duplicate check %q", title)
		}
		seen[title] = true
		if detail, _ := check["detail"].(string); detail == "" {
			t.Errorf("%s has no detail", title)
		}
		switch check["passed"].(type) {
		case bool, nil:
		default:
			t.Errorf("%s passed = %v (%T), want bool or nil for 未知", title, check["passed"], check["passed"])
		}
	}
}

func TestThermalCheckMapsEveryState(t *testing.T) {
	cases := map[string]any{
		"nominal":  true,
		"fair":     false,
		"serious":  false,
		"critical": false,
		"":         nil,
		"wat":      nil,
	}
	for state, want := range cases {
		if got := thermalCheck(state)["passed"]; got != want {
			t.Errorf("thermal %q passed = %v, want %v", state, got, want)
		}
	}
}

func TestMemoryPressureCheckMapsEveryState(t *testing.T) {
	cases := map[string]any{
		"normal":   true,
		"warn":     false,
		"critical": false,
		"":         nil,
	}
	for state, want := range cases {
		if got := memoryPressureCheck(state)["passed"]; got != want {
			t.Errorf("pressure %q passed = %v, want %v", state, got, want)
		}
	}
}

func TestDiskFreeCheckUsesRealThresholds(t *testing.T) {
	// 990 GB of 1 TB free is fine, 120 GB (12%) is a warning, 50 GB (5%) fails.
	if got := diskFreeCheck(990, 1000)["passed"]; got != true {
		t.Errorf("99%% free passed = %v, want true", got)
	}
	if got := diskFreeCheck(120, 1000)["passed"]; got != false {
		t.Errorf("12%% free passed = %v, want false", got)
	}
	if got := diskFreeCheck(50, 1000)["passed"]; got != false {
		t.Errorf("5%% free passed = %v, want false", got)
	}
	// An unreadable volume is unknown, not a failure.
	if got := diskFreeCheck(0, 0)["passed"]; got != nil {
		t.Errorf("unknown volume passed = %v, want nil", got)
	}
}

func TestBatteryHealthCheckIsLeftOutWithoutAHealthReading(t *testing.T) {
	if _, ok := batteryHealthCheck(nil, intPtr(120), intPtr(80)); ok {
		t.Error("health check reported without a health reading")
	}
	check, ok := batteryHealthCheck(intPtr(81), intPtr(266), intPtr(100))
	if !ok {
		t.Fatal("health check missing for a healthy battery")
	}
	if check["passed"] != true {
		t.Errorf("81%% health passed = %v, want true", check["passed"])
	}
	if check, _ := batteryHealthCheck(intPtr(55), nil, nil); check["passed"] != false {
		t.Errorf("55%% health passed = %v, want false", check["passed"])
	}
}

func TestDoctorRowKeepsUnknownUnknown(t *testing.T) {
	if row := doctorRow("x", nil, "d"); row["passed"] != nil {
		t.Errorf("unknown row passed = %v, want nil", row["passed"])
	}
	if row := doctorRow("x", pass(), "d"); row["passed"] != true {
		t.Errorf("pass row passed = %v, want true", row["passed"])
	}
	if row := doctorRow("x", fail(), "d"); row["passed"] != false {
		t.Errorf("fail row passed = %v, want false", row["passed"])
	}
}

func intPtr(v int) *int { return &v }

// The 体检 panel is where a user finds out which build is running; that row has
// to exist, carry the version, and never claim a pass/fail verdict.
func TestBuildCheckReportsTheRunningBuild(t *testing.T) {
	oldVersion, oldCommit, oldTime := BuildVersion, BuildCommit, BuildTime
	t.Cleanup(func() { BuildVersion, BuildCommit, BuildTime = oldVersion, oldCommit, oldTime })

	BuildVersion, BuildCommit, BuildTime = "9.9.9", "abc1234", "2026-10-28 00:41"

	row := buildCheck()
	if title, _ := row["title"].(string); title != "应用构建" {
		t.Fatalf("title = %q", title)
	}
	if verdict, ok := row["passed"]; !ok || verdict != nil {
		t.Fatalf("build row must be informational (nil verdict), got %v", row["passed"])
	}
	detail, _ := row["detail"].(string)
	for _, want := range []string{"9.9.9", "abc1234", "2026-10-28 00:41"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail %q does not mention %q", detail, want)
		}
	}

	if got := DescribeExecutable(); got == "" {
		t.Fatal("executable timestamp is empty on this platform")
	}
}
