package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeLaunchd stands in for launchctl and lsregister: no test touches the real
// launchd or LaunchServices. It also records whether the plist still sat in the
// LaunchAgents folder when a job was booted out.
type fakeLaunchd struct {
	mu           sync.Mutex
	log          []string
	loaded       map[string]bool   // label -> loaded in gui/<uid>
	byPlist      map[string]string // plist path -> label it loads
	plistPresent map[string]bool   // label -> plist existed at bootout
	failBootout  map[string]bool
	failBootstrp bool
}

func newFakeLaunchd(s *Scanner) *fakeLaunchd {
	f := &fakeLaunchd{loaded: map[string]bool{}, byPlist: map[string]string{}, plistPresent: map[string]bool{}, failBootout: map[string]bool{}}
	s.optimizeRun = f.run
	return f
}

func (f *fakeLaunchd) run(ctx context.Context, stdin, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, filepath.Base(name)+" "+strings.Join(args, " "))
	if filepath.Base(name) != "launchctl" {
		return "", nil
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	switch args[0] {
	case "print":
		if !f.loaded[strings.TrimPrefix(args[1], domain+"/")] {
			return "", errors.New("exit status 113")
		}
	case "bootout":
		label := strings.TrimPrefix(args[1], domain+"/")
		if f.failBootout[label] {
			return "Boot-out failed: 5: Input/output error", errors.New("exit status 5")
		}
		for plist, l := range f.byPlist {
			if l == label {
				_, err := os.Lstat(plist)
				f.plistPresent[label] = err == nil
			}
		}
		delete(f.loaded, label)
	case "bootstrap":
		if args[1] != domain || f.failBootstrp {
			return "", errors.New("exit status 5")
		}
		if _, err := os.Lstat(args[2]); err != nil {
			return "", errors.New("exit status 5")
		}
		f.loaded[f.byPlist[args[2]]] = true
	}
	return "", nil
}

func (f *fakeLaunchd) count(prefix string) int {
	n := 0
	for _, l := range f.log {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func uninstallFixture(t *testing.T) (*Scanner, *fakeLaunchd, string, string) {
	t.Helper()
	s := newTestScanner(t)
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	s.appDirs = []string{filepath.Join(s.home, "Applications")}
	f := newFakeLaunchd(s)
	caskrooms = []string{filepath.Join(s.home, "Caskroom")}
	t.Cleanup(func() { caskrooms = []string{"/opt/homebrew/Caskroom", "/usr/local/Caskroom"} })
	app := filepath.Join(s.home, "Applications/Example.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.app</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/Example"), "fixture")
	helper := filepath.Join(app, "Contents/Library/LoginItems/Launcher.app")
	writeFile(t, filepath.Join(helper, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.app.launcher</string></dict></plist>`)
	agent := filepath.Join(s.home, "Library/LaunchAgents/com.example.app.plist")
	writeFile(t, agent, launchPlist("com.example.app", "/bin/sh"))
	f.byPlist[agent] = "com.example.app"
	return s, f, app, agent
}

func TestUninstallStopsUserServicesAroundTheMoveAndRestoreStartsThemAgain(t *testing.T) {
	s, f, app, agent := uninstallFixture(t)
	f.loaded["com.example.app"], f.loaded["com.example.app.launcher"] = true, true

	outcome, receipt := trashUninstallSync(s, []string{app, agent}, nil)
	if len(outcome.Moved) != 2 {
		t.Fatalf("moved %d, skipped %v, failed %v", len(outcome.Moved), outcome.Skipped, outcome.Failed)
	}
	if !f.plistPresent["com.example.app"] {
		t.Error("the agent must be booted out while its plist is still in place, before it moves to the Trash")
	}
	if len(receipt.Stopped) != 2 || receipt.Stopped[0] != (StoppedService{Label: "com.example.app.launcher", Kind: "helper"}) || receipt.Stopped[1] != (StoppedService{Label: "com.example.app", Kind: "agent"}) {
		t.Fatalf("stopped = %+v", receipt.Stopped)
	}
	if !receipt.LoginItemsManual || len(receipt.Failed) != 0 || len(receipt.Reloaded) != 0 {
		t.Errorf("receipt = %+v", receipt)
	}
	// -u targets the copy in the Trash: the original path no longer exists.
	if trashed := outcome.Moved[0].Trashed; f.count("lsregister -u "+trashed) != 1 || f.count("lsregister -u "+app) != 0 || f.count("lsregister -gc") != 1 {
		t.Errorf("lsregister calls = %v", f.log)
	}
	manifest, _ := readTrashManifest()
	var recorded []LaunchUnload
	for _, e := range manifest.Entries {
		if e.Original == agent {
			recorded = e.Unloaded
		}
	}
	if len(recorded) != 1 || recorded[0].Label != "com.example.app" {
		t.Fatalf("manifest did not record the stopped job: %+v", manifest.Entries)
	}

	restore := s.RestoreTrash([]string{agent})
	if len(restore.Restored) != 1 || len(restore.Restarted) != 1 || restore.Restarted[0] != "com.example.app" || len(restore.RestartFailed) != 0 {
		t.Fatalf("restore = %+v", restore)
	}
	if !f.loaded["com.example.app"] {
		t.Error("the agent was not bootstrapped again after its plist came back")
	}
}

func TestUninstallLeavesAnAgentThatIsNotLoadedAndOneThatIsNotOwned(t *testing.T) {
	s, f, app, agent := uninstallFixture(t)
	other := filepath.Join(s.home, "Library/LaunchAgents/org.other.job.plist")
	writeFile(t, other, launchPlist("org.other.job", "/usr/bin/true"))
	f.byPlist[other], f.loaded["org.other.job"] = "org.other.job", true

	outcome, receipt := trashUninstallSync(s, []string{app, agent, other}, nil)
	if len(outcome.Moved) != 3 {
		t.Fatalf("moved %d, failed %v", len(outcome.Moved), outcome.Failed)
	}
	if f.count("launchctl bootout") != 0 || len(receipt.Stopped) != 0 {
		t.Errorf("nothing owned was loaded, yet %v / %+v", f.log, receipt.Stopped)
	}
	manifest, _ := readTrashManifest()
	for _, e := range manifest.Entries {
		if len(e.Unloaded) != 0 {
			t.Errorf("entry %s claims it unloaded %v", e.Original, e.Unloaded)
		}
	}
}

func TestUninstallNeverStopsAnythingForAPlistSelectedWithoutItsApp(t *testing.T) {
	s, f, _, agent := uninstallFixture(t)
	f.loaded["com.example.app"] = true
	outcome, receipt := trashUninstallSync(s, []string{agent}, nil)
	if len(outcome.Moved) != 1 || f.count("launchctl") != 0 || len(receipt.Stopped) != 0 || receipt.LoginItemsManual {
		t.Fatalf("a cache-only selection touched launchd: %v %+v", f.log, receipt)
	}
}

func TestUninstallBootsAnAgentBackInWhenTheTrashMoveFails(t *testing.T) {
	s, f, app, agent := uninstallFixture(t)
	f.loaded["com.example.app"], f.loaded["com.example.app.launcher"] = true, true
	trash := filepath.Join(s.home, ".Trash")
	if err := os.MkdirAll(trash, 0o755); err != nil || os.Chmod(trash, 0o500) != nil {
		t.Fatal("cannot make the Trash read only")
	}
	t.Cleanup(func() { _ = os.Chmod(trash, 0o755) })

	outcome, receipt := trashUninstallSync(s, []string{app, agent}, nil)
	if len(outcome.Moved) != 0 || len(outcome.Failed) != 2 {
		t.Skipf("the Trash stayed writable (running as root?): %+v", outcome)
	}
	if !f.loaded["com.example.app"] || !f.loaded["com.example.app.launcher"] {
		t.Errorf("services were left stopped: %v", f.log)
	}
	if len(receipt.Stopped) != 0 || len(receipt.Reloaded) != 1 || receipt.Reloaded[0] != "com.example.app" || receipt.LoginItemsManual {
		t.Errorf("receipt = %+v", receipt)
	}
	if f.count("lsregister") != 0 || f.count("launchctl bootout gui/"+strconv.Itoa(os.Getuid())+"/com.example.app.launcher") != 0 {
		t.Errorf("the app did not move, yet LaunchServices or its helper was touched: %v", f.log)
	}
}

func TestUninstallReportsServicesThatCannotBeStoppedOrStartedAgain(t *testing.T) {
	s, f, app, agent := uninstallFixture(t)
	f.loaded["com.example.app"], f.loaded["com.example.app.launcher"] = true, true
	f.failBootout["com.example.app"], f.failBootout["com.example.app.launcher"] = true, true
	outcome, receipt := trashUninstallSync(s, []string{app, agent}, nil)
	if len(outcome.Moved) != 2 || len(receipt.Stopped) != 0 || len(receipt.Failed) != 2 {
		t.Fatalf("moved %d, receipt %+v", len(outcome.Moved), receipt)
	}
	if receipt.Failed[0].Detail != "Boot-out failed: 5: Input/output error" {
		t.Errorf("launchctl's words were not kept: %+v", receipt.Failed[0])
	}

	// A restore whose bootstrap fails still puts the file back and says so.
	manifest, _ := readTrashManifest()
	for i := range manifest.Entries {
		if manifest.Entries[i].Original == agent {
			manifest.Entries[i].Unloaded = []LaunchUnload{{Label: "com.example.app"}}
		}
	}
	if err := writeTrashManifest(manifest); err != nil {
		t.Fatal(err)
	}
	f.failBootstrp = true
	restore := s.RestoreTrash([]string{agent})
	if len(restore.Restored) != 1 || len(restore.Restarted) != 0 || len(restore.RestartFailed) != 1 {
		t.Fatalf("restore = %+v", restore)
	}
}

func TestRestoreIgnoresAManifestEntryThatPointsOutsideUserLaunchAgents(t *testing.T) {
	s, f, _, _ := uninstallFixture(t)
	outside := filepath.Join(s.home, "Library/Caches/Example/evil.plist")
	writeFile(t, outside, launchPlist("com.example.evil", "/bin/sh"))
	started, failed := s.restartServices(TrashEntry{Original: outside, Unloaded: []LaunchUnload{{Label: "com.example.evil"}}})
	agents := filepath.Join(s.home, "Library/LaunchAgents/x.plist")
	started2, _ := s.restartServices(TrashEntry{Original: agents, Unloaded: []LaunchUnload{{Label: "com.apple.something"}}})
	if started != nil || failed != nil || started2 != nil || f.count("launchctl") != 0 {
		t.Fatalf("restore bootstrapped something it must not: %v", f.log)
	}
}

func TestLaunchLabelOKRefusesAppleAndMalformedLabels(t *testing.T) {
	for label, want := range map[string]bool{"com.example.app": true, "com.apple.Safari.helper": false, "COM.APPLE.x": false, "../etc": false, "nodots": false, "a b.c": false, "": false} {
		if launchLabelOK(label) != want {
			t.Errorf("launchLabelOK(%q) = %v, want %v", label, !want, want)
		}
	}
}

func TestCaskNoteComesFromTheCaskroomWithoutRunningBrew(t *testing.T) {
	s, f, app, _ := uninstallFixture(t)
	// A cask that links this very bundle gets a note, with the exact command.
	version := filepath.Join(s.home, "Caskroom/example-app/1.0")
	if err := os.MkdirAll(version, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(app, filepath.Join(version, "Example.app")); err != nil {
		t.Fatal(err)
	}
	plan := s.planUninstall([]string{app})
	if len(plan.receipt.Casks) != 1 || plan.receipt.Casks[0] != (CaskNote{App: "Example.app", Token: "example-app", Command: "brew uninstall --cask example-app"}) {
		t.Fatalf("casks = %+v", plan.receipt.Casks)
	}
	if f.count("brew") != 0 {
		t.Error("brew was run")
	}

	// A second cask holding a link to it makes the owner ambiguous: no note.
	other := filepath.Join(s.home, "Caskroom/other-app/2.0")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(app, filepath.Join(other, "Example.app")); err != nil {
		t.Fatal(err)
	}
	if plan := s.planUninstall([]string{app}); len(plan.receipt.Casks) != 0 {
		t.Errorf("ambiguous owner still got a note: %+v", plan.receipt.Casks)
	}

	// An app that no cask links gets none either.
	plain := filepath.Join(s.home, "Applications/Plain.app")
	writeFile(t, filepath.Join(plain, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.plain</string></dict></plist>`)
	if plan := s.planUninstall([]string{plain}); len(plan.receipt.Casks) != 0 {
		t.Errorf("casks = %+v", plan.receipt.Casks)
	}
}

func TestUninstallKeepsHelpersRunningWhileASameBundleSiblingStaysInstalled(t *testing.T) {
	s, f, app, _ := uninstallFixture(t)
	f.loaded["com.example.app.launcher"] = true
	sibling := filepath.Join(s.home, "Applications/Example Beta.app")
	writeFile(t, filepath.Join(sibling, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.app</string></dict></plist>`)

	outcome, receipt := trashUninstallSync(s, []string{app}, nil)
	if len(outcome.Moved) != 1 || f.count("launchctl bootout") != 0 || len(receipt.Stopped) != 0 {
		t.Fatalf("helper was booted out although a sibling stays: moved %d, log %v", len(outcome.Moved), f.log)
	}
	if !f.loaded["com.example.app.launcher"] || f.count("lsregister -u") != 1 {
		t.Errorf("helper loaded=%v, log %v", f.loaded["com.example.app.launcher"], f.log)
	}

	// A sibling that is part of the same uninstall does not count: both go.
	s2, f2, app2, _ := uninstallFixture(t)
	f2.loaded["com.example.app.launcher"] = true
	sibling2 := filepath.Join(s2.home, "Applications/Example Beta.app")
	writeFile(t, filepath.Join(sibling2, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.app</string></dict></plist>`)
	if _, receipt := trashUninstallSync(s2, []string{app2, sibling2}, nil); len(receipt.Stopped) != 1 {
		t.Errorf("stopped = %+v", receipt.Stopped)
	}
}

// trashUninstallSync is TrashUninstall with the best effort LaunchServices work run before it returns.
func trashUninstallSync(s *Scanner, paths []string, identities map[string]FileIdentity) (TrashOutcome, UninstallReceipt) {
	outcome, receipt, after := s.trashUninstall(paths, identities)
	after()
	return outcome, receipt
}
