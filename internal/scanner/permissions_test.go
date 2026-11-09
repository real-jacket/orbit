package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFdaFromProbeMapsOpenResults(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, "granted"},
		{&os.PathError{Op: "open", Err: syscall.EPERM}, "denied"},
		{&os.PathError{Op: "open", Err: syscall.EACCES}, "denied"},
		{os.ErrNotExist, ""},
		{errors.New("i/o"), ""},
	}
	for _, c := range cases {
		if got := fdaFromProbe(c.err); got != c.want {
			t.Errorf("fdaFromProbe(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestFullDiskProbeReadsTheTCCFileOnlyToOpenIt(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}
	if got := s.probeFullDisk(); got != "" {
		t.Fatalf("missing TCC.db must stay undecided, got %q", got)
	}
	if err := os.MkdirAll(filepath.Dir(s.tccDBPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.tccDBPath(), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := s.FullDiskReadCheck()
	if result["passed"] != true || result["status"] != "granted" {
		t.Fatalf("openable TCC.db means granted: %v", result)
	}
	if os.Geteuid() == 0 {
		return
	}
	if err := os.Chmod(s.tccDBPath(), 0); err != nil {
		t.Fatal(err)
	}
	result = s.FullDiskReadCheck()
	if result["passed"] != false || result["status"] != "denied" {
		t.Fatalf("refused TCC.db means denied: %v", result)
	}
}

func fakeBundle(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Tool.app")
	if err := os.MkdirAll(filepath.Join(path, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRootOwnedAppGetsTheAdministratorMessageNotAppManagement(t *testing.T) {
	bundle := fakeBundle(t)
	denied := &os.PathError{Op: "rename", Err: syscall.EPERM}

	message, pane := humanizeTrashError(bundle, denied)
	if pane != "appManagement" || strings.Contains(message, "管理员密码") {
		t.Fatalf("user-owned app: pane=%q message=%q", pane, message)
	}

	real := currentUID
	t.Cleanup(func() { currentUID = real })
	currentUID = func() uint32 { return real() + 1 } // pretend root owns the bundle
	message, pane = humanizeTrashError(bundle, denied)
	if pane != "" || message != appNeedsAdminReason {
		t.Fatalf("root-owned app: pane=%q message=%q", pane, message)
	}
	if !bundleNeedsAdmin(bundle) {
		t.Fatal("a bundle owned by someone else needs admin")
	}
}

func TestAppManagementEvidenceIsPersistedPerIdentity(t *testing.T) {
	t.Setenv("MOLE_STUDIO_STATE_DIR", t.TempDir())
	old := CodeIdentity
	t.Cleanup(func() { CodeIdentity = old })

	CodeIdentity = ""
	recordAppManagement("effective", "/Applications/A.app", time.Now())
	if ReadAppManagement() != nil {
		t.Fatal("a bare executable has no identity to record against")
	}

	CodeIdentity = "/Applications/orbit.app|cert-1"
	if ReadAppManagement() != nil {
		t.Fatal("no evidence before the first move")
	}
	recordAppManagement("denied", "/Applications/A.app", time.UnixMilli(1000))
	recordAppManagement("effective", "/Applications/B.app", time.UnixMilli(2000))
	got := ReadAppManagement()
	if got == nil || got.State != "effective" || got.App != "B.app" || got.At != 2000 {
		t.Fatalf("newest outcome should win: %+v", got)
	}
	CodeIdentity = "/tmp/orbit.app|cdhash-2"
	if ReadAppManagement() != nil {
		t.Fatal("a new code identity resets the record")
	}
}

func TestTrashingAppsRecordsAppManagementEvidence(t *testing.T) {
	s := newTestScanner(t)
	old := CodeIdentity
	t.Cleanup(func() { CodeIdentity = old })
	CodeIdentity = "test-identity"
	real := currentUID
	t.Cleanup(func() { currentUID = real })
	bundle := func(name string) string {
		path := filepath.Join(s.home, "Applications", name+".app")
		writeFile(t, filepath.Join(path, "Contents", "Info.plist"), "x")
		return path
	}

	// A root-owned app (fake uid) is never evidence, whatever happens to it.
	currentUID = func() uint32 { return real() + 1 }
	s.TrashPaths([]string{bundle("Rooted")})
	if ReadAppManagement() != nil {
		t.Fatal("root-owned app must not count as evidence")
	}

	currentUID = real
	if outcome := s.TrashPaths([]string{bundle("Mine")}); len(outcome.Moved) != 1 {
		t.Fatalf("owned app should move: %+v", outcome)
	}
	if got := ReadAppManagement(); got == nil || got.State != "effective" || got.App != "Mine.app" {
		t.Fatalf("a successful move proves App Management works: %+v", got)
	}

	if os.Geteuid() == 0 {
		return
	}
	blocked := bundle("Blocked")
	dir := filepath.Dir(blocked)
	if err := os.Chmod(dir, 0o500); err != nil { // rename out of a read-only folder fails with EACCES
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if outcome := s.TrashPaths([]string{blocked}); len(outcome.Failed) != 1 {
		t.Fatalf("move should fail: %+v", outcome)
	}
	if got := ReadAppManagement(); got == nil || got.State != "denied" || got.App != "Blocked.app" {
		t.Fatalf("a refused move of an owned app means not effective: %+v", got)
	}
}

func TestRootOwnedAppIsListedAsAdministratorOnly(t *testing.T) {
	s := newTestScanner(t)
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	writeFile(t, filepath.Join(s.home, "Applications", "Store.app", "Contents", "MacOS", "Store"), "x")
	real := currentUID
	t.Cleanup(func() { currentUID = real })

	if rows := s.scanInstalledApps(filepath.Join(s.home, "Applications")); rows[0]["needsAdmin"] != false {
		t.Fatalf("an app the user owns is movable: %+v", rows[0])
	}
	currentUID = func() uint32 { return real() + 1 }
	rows := s.scanInstalledApps(filepath.Join(s.home, "Applications"))
	bundle := rows[0]["subpaths"].([]map[string]any)[0]
	if rows[0]["needsAdmin"] != true || bundle["tier"] != "protected" || bundle["reason"] != appNeedsAdminReason || bundle["checked"] != false {
		t.Fatalf("root-owned bundle must be listed with its plain reason: %+v", bundle)
	}
}
