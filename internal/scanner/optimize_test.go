package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSys stands in for plutil, defaults, sqlite3, qlmanage, mdfind and osascript,
// so no test touches the real preferences or runs a real system command.
type fakeSys struct {
	mu        sync.Mutex
	log       []string
	prefs     map[string]map[string]string // domain -> key -> plist fragment
	probe     map[string]string            // database path -> page_count and freelist_count output
	integrity string
	mdfind    map[string]bool
	osascript string
}

const plistHead = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0">`

func wrapPlist(inner string) string { return plistHead + inner + "</plist>" }

func newFakeSys(s *Scanner) *fakeSys {
	f := &fakeSys{prefs: map[string]map[string]string{}, probe: map[string]string{}, integrity: "ok", mdfind: map[string]bool{}}
	s.optimizeRun = f.run
	return f
}

func (f *fakeSys) setPref(domain, key, inner string) {
	if f.prefs[domain] == nil {
		f.prefs[domain] = map[string]string{}
	}
	f.prefs[domain][key] = wrapPlist(inner)
}

func (f *fakeSys) ran(prefix string) bool {
	for _, l := range f.log {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

func (f *fakeSys) run(ctx context.Context, stdin, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, filepath.Base(name)+" "+strings.Join(args, " "))
	switch filepath.Base(name) {
	case "plutil":
		switch args[0] {
		case "-lint":
			bad := false
			for _, file := range args[1:] {
				if b, err := os.ReadFile(file); err == nil && strings.Contains(string(b), "BROKEN") {
					bad = true
				}
			}
			if bad {
				return "", errors.New("exit status 1: Property List error")
			}
		case "-convert":
			b, err := os.ReadFile(args[len(args)-1])
			return string(b), err
		case "-extract":
			domain := strings.TrimPrefix(stdin, "DOC:")
			if frag, ok := f.prefs[domain][args[1]]; ok {
				return frag, nil
			}
			return "", errors.New("exit status 1: No value at that key path or invalid key path")
		}
	case "defaults":
		switch args[0] {
		case "export":
			return "DOC:" + args[1], nil
		case "write":
			if f.prefs[args[1]] == nil {
				f.prefs[args[1]] = map[string]string{}
			}
			if len(args) >= 5 && args[3] == "-bool" {
				f.prefs[args[1]][args[2]] = wrapPlist("<true/>")
			} else if strings.Contains(args[len(args)-1], "<plist") {
				f.prefs[args[1]][args[2]] = args[len(args)-1]
			} else {
				return "", errors.New("exit status 1: unsupported write")
			}
		case "delete":
			delete(f.prefs[args[1]], args[2])
		}
	case "sqlite3":
		sql := args[len(args)-1]
		switch {
		case strings.Contains(sql, "page_count"):
			return f.probe[args[len(args)-2]], nil
		case strings.Contains(sql, "integrity_check"):
			return f.integrity + "\n", nil
		}
	case "mdfind":
		if f.mdfind[strings.Split(args[0], "'")[1]] {
			return "/Applications/Somewhere.app\n", nil
		}
	case "osascript":
		return f.osascript, nil
	}
	return "", nil
}

func ageDir(t *testing.T, path string, days int) {
	t.Helper()
	old := time.Now().AddDate(0, 0, -days)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func lib(s *Scanner, parts ...string) string {
	return filepath.Join(append([]string{s.home, "Library"}, parts...)...)
}

func diagnose(t *testing.T, s *Scanner, id string) OptimizeItem {
	t.Helper()
	for _, it := range s.DiagnoseOptimize(context.Background()).Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("task %s missing from plan", id)
	return OptimizeItem{}
}

func runOne(t *testing.T, s *Scanner, id string) OptimizeReceipt {
	t.Helper()
	var got []OptimizeReceipt
	if err := s.RunOptimize(context.Background(), []string{id}, func(r OptimizeReceipt) { got = append(got, r) }); err != nil || len(got) != 1 {
		t.Fatalf("RunOptimize(%s): %v, %d receipts", id, err, len(got))
	}
	return got[0]
}

func TestOptimizeCatalogShape(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	s.processProbe = func() (string, error) { return "1 /sbin/launchd", nil }
	plan := s.DiagnoseOptimize(context.Background())
	if len(plan.Items) != 19 {
		t.Fatalf("plan has %d tasks, want Mole's 19", len(plan.Items))
	}
	seen := map[string]bool{}
	for _, it := range plan.Items {
		if seen[it.ID] {
			t.Errorf("duplicate id %s", it.ID)
		}
		seen[it.ID] = true
		if it.Name == "" || it.Detail == "" || it.Undo == "" {
			t.Errorf("%s: name, detail and undo must be filled: %+v", it.ID, it)
		}
		if (it.Tier == "review" || it.Tier == "protected") && it.Reason == "" {
			t.Errorf("%s: %s tier without a reason", it.ID, it.Tier)
		}
		if it.Reason == "" {
			t.Errorf("%s: empty reason", it.ID)
		}
		if it.Checked && (!it.Runnable || it.Tier != "safe") {
			t.Errorf("%s: checked but runnable=%v tier=%s", it.ID, it.Runnable, it.Tier)
		}
		if it.Status == OptimizeUnavailable && (it.Runnable || it.Checked) {
			t.Errorf("%s: unavailable but runnable", it.ID)
		}
	}
	for _, id := range []string{"network_optimization", "network_stack_optimize", "disk_permissions_repair", "spotlight_index_optimize", "periodic_maintenance"} {
		if it := diagnose(t, s, id); it.Status != OptimizeUnavailable || it.Reason != "需要管理员权限，orbit 不执行" {
			t.Errorf("%s: want the sudo reason, got %+v", id, it)
		}
	}
	if it := diagnose(t, s, "login_items_audit"); it.Tier != "review" || !it.Runnable || it.Checked {
		t.Errorf("login items must be review and unchecked: %+v", it)
	}
}

func TestRunOptimizeNeverRunsUnavailableOrUnknown(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	var got []OptimizeReceipt
	err := s.RunOptimize(context.Background(), []string{"network_optimization", "nope", "network_optimization"}, func(r OptimizeReceipt) { got = append(got, r) })
	if err != nil || len(got) != 2 {
		t.Fatalf("err=%v receipts=%d, want 2 (duplicate dropped)", err, len(got))
	}
	for _, r := range got {
		if r.OK || r.Outcome == OutcomeApplied {
			t.Errorf("%s must not succeed: %+v", r.ID, r)
		}
	}
	if len(f.log) != 0 {
		t.Errorf("no command may run, got %v", f.log)
	}
}

func TestCacheRefreshRunsQuickLookOnly(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	r := runOne(t, s, "cache_refresh")
	if !r.OK || r.Outcome != OutcomeApplied || r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if !f.ran("qlmanage -r cache") || !f.ran("qlmanage -r") {
		t.Errorf("qlmanage not run: %v", f.log)
	}
}

func TestSavedStateTrashesOldOnesAndUndoRestores(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	oldDir := lib(s, "Saved Application State", "com.old.app.savedState")
	newDir := lib(s, "Saved Application State", "com.new.app.savedState")
	writeFile(t, filepath.Join(oldDir, "windows.plist"), "x")
	writeFile(t, filepath.Join(newDir, "windows.plist"), "x")
	ageDir(t, oldDir, 45)

	if it := diagnose(t, s, "saved_state_cleanup"); it.Status != OptimizeNeeded || !it.Checked || len(it.Items) != 1 {
		t.Fatalf("diagnosis: %+v", it)
	}
	r := runOne(t, s, "saved_state_cleanup")
	if !r.OK || r.Outcome != OutcomeApplied || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old state should be in the Trash")
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("recent state must stay: %v", err)
	}
	u, err := s.UndoOptimize(context.Background(), "saved_state_cleanup")
	if err != nil || !u.OK {
		t.Fatalf("undo: %v %+v", err, u)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Errorf("undo should put the folder back: %v", err)
	}
	if _, err := s.UndoOptimize(context.Background(), "saved_state_cleanup"); err == nil {
		t.Errorf("a second undo has nothing to revert")
	}
}

func TestBrokenPrefsOnlyThirdPartyAndUndoable(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	bad := lib(s, "Preferences", "com.example.bad.plist")
	writeFile(t, bad, "BROKEN")
	writeFile(t, lib(s, "Preferences", "com.example.good.plist"), "fine")
	writeFile(t, lib(s, "Preferences", "com.apple.finder.plist"), "BROKEN")
	writeFile(t, lib(s, "Preferences", "loginwindow.plist"), "BROKEN")
	byHost := lib(s, "Preferences", "ByHost", "com.example.host.ABC.plist")
	writeFile(t, byHost, "BROKEN")

	it := diagnose(t, s, "fix_broken_configs")
	if it.Status != OptimizeNeeded || len(it.Items) != 2 {
		t.Fatalf("want the two third-party broken files, got %+v", it)
	}
	r := runOne(t, s, "fix_broken_configs")
	if !r.OK || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	for _, p := range []string{bad, byHost} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be trashed", p)
		}
	}
	for _, p := range []string{lib(s, "Preferences", "com.apple.finder.plist"), lib(s, "Preferences", "loginwindow.plist"), lib(s, "Preferences", "com.example.good.plist")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must be left alone: %v", p, err)
		}
	}
	if u, err := s.UndoOptimize(context.Background(), "fix_broken_configs"); err != nil || !u.OK {
		t.Fatalf("undo: %v %+v", err, u)
	}
	if _, err := os.Stat(bad); err != nil {
		t.Errorf("undo should restore the plist: %v", err)
	}
}

func TestPlistLintFallsBackPerFileAndIgnoresRunnerFailure(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	good, bad := lib(s, "Preferences", "a.plist"), lib(s, "Preferences", "b.plist")
	writeFile(t, good, "ok")
	writeFile(t, bad, "BROKEN")
	broken, _ := s.lintPlists(context.Background(), []string{good, bad})
	if len(broken) != 1 || broken[0] != bad {
		t.Fatalf("broken = %v", broken)
	}
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("fork/exec: no such file")
	}
	if broken, _ := s.lintPlists(context.Background(), []string{good, bad}); len(broken) != 0 {
		t.Errorf("a plutil that cannot run must condemn nothing, got %v", broken)
	}
}

func sqliteFixture(t *testing.T, s *Scanner, f *fakeSys, probe string) string {
	t.Helper()
	db := lib(s, "Safari", "History.db")
	writeFile(t, db, "SQLite format 3\x00 rest of header")
	f.probe[db] = probe
	return db
}

func TestSQLiteVacuumNeededRunsIntegrityThenVacuum(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	s.processProbe = func() (string, error) { return "1 /sbin/launchd", nil }
	sqliteFixture(t, s, f, "100\n20\n")
	if it := diagnose(t, s, "sqlite_vacuum"); it.Status != OptimizeNeeded || !it.Checked {
		t.Fatalf("diagnosis: %+v", it)
	}
	r := runOne(t, s, "sqlite_vacuum")
	if !r.OK || r.Outcome != OutcomeApplied || r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if !f.ran("sqlite3 -readonly") || !f.ran("sqlite3 "+lib(s, "Safari", "History.db")+" VACUUM;") {
		t.Errorf("commands: %v", f.log)
	}
}

func TestSQLiteVacuumSkipsOnFailedIntegrityCheck(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	s.processProbe = func() (string, error) { return "1 /sbin/launchd", nil }
	sqliteFixture(t, s, f, "100\n20\n")
	f.integrity = "*** in database main ***"
	r := runOne(t, s, "sqlite_vacuum")
	if r.OK || r.Outcome != OutcomeFailed {
		t.Fatalf("receipt: %+v", r)
	}
	if f.ran("sqlite3 " + lib(s, "Safari", "History.db") + " VACUUM;") {
		t.Errorf("must never VACUUM a database that fails integrity_check")
	}
}

func TestSQLiteVacuumStatuses(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	sqliteFixture(t, s, f, "100\n20\n")

	s.processProbe = func() (string, error) { return "7 /Applications/Safari.app/Contents/MacOS/Safari", nil }
	it := diagnose(t, s, "sqlite_vacuum")
	if it.Status != OptimizeSkipped || it.Tier != "protected" || !strings.Contains(it.Unlock, "退出 Safari") || it.Runnable {
		t.Errorf("running Safari must protect the databases: %+v", it)
	}
	if r := runOne(t, s, "sqlite_vacuum"); r.OK || f.ran("sqlite3 "+lib(s, "Safari", "History.db")+" VACUUM;") {
		t.Errorf("must not vacuum while Safari runs: %+v", r)
	}

	s.processProbe = func() (string, error) { return "", errors.New("ps failed") }
	if it := diagnose(t, s, "sqlite_vacuum"); it.Status != OptimizeUnavailable || it.Runnable {
		t.Errorf("unknown process list must be unavailable: %+v", it)
	}

	s.processProbe = func() (string, error) { return "1 /sbin/launchd", nil }
	f.probe[lib(s, "Safari", "History.db")] = "100\n1\n"
	if it := diagnose(t, s, "sqlite_vacuum"); it.Status != OptimizeClear {
		t.Errorf("a compact database is not needed: %+v", it)
	}
}

func TestDSStoreWritesAndUndoRestoresPriorValue(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	f.setPref(dsstoreDomain, "DSDontWriteUSBStores", "<false/>")

	if it := diagnose(t, s, "prevent_network_dsstore"); it.Status != OptimizeNeeded || len(it.Items) != 2 {
		t.Fatalf("diagnosis: %+v", it)
	}
	r := runOne(t, s, "prevent_network_dsstore")
	if !r.OK || r.Outcome != OutcomeApplied || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if it := diagnose(t, s, "prevent_network_dsstore"); it.Status != OptimizeClear {
		t.Errorf("after the write it should be clear: %+v", it)
	}
	if u, err := s.UndoOptimize(context.Background(), "prevent_network_dsstore"); err != nil || !u.OK {
		t.Fatalf("undo: %v %+v", err, u)
	}
	if _, ok := f.prefs[dsstoreDomain]["DSDontWriteNetworkStores"]; ok {
		t.Errorf("an absent key must be deleted again")
	}
	if !strings.Contains(f.prefs[dsstoreDomain]["DSDontWriteUSBStores"], "<false/>") {
		t.Errorf("the prior false value must come back: %v", f.prefs[dsstoreDomain])
	}
}

func TestUndoRefusesToOverwriteALaterChange(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	runOne(t, s, "prevent_network_dsstore")
	f.setPref(dsstoreDomain, "DSDontWriteNetworkStores", "<false/>") // the user changed it afterwards
	u, err := s.UndoOptimize(context.Background(), "prevent_network_dsstore")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Failed) != 1 || !strings.Contains(f.prefs[dsstoreDomain]["DSDontWriteNetworkStores"], "<false/>") {
		t.Errorf("the later value must survive and be reported: %+v", u)
	}
	if !u.CanUndo {
		t.Errorf("the kept record should stay for a later retry")
	}
}

func TestLegacyOverridesDeletedAndUndoable(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	f.setPref(globalDomain, "NSAppSleepDisabled", "<true/>")
	f.setPref("com.apple.frameworks.diskimages", "skip-verify", "<false/>")

	it := diagnose(t, s, "legacy_overrides_audit")
	if it.Status != OptimizeNeeded || len(it.Items) != 1 {
		t.Fatalf("only the truthy override counts: %+v", it)
	}
	r := runOne(t, s, "legacy_overrides_audit")
	if !r.OK || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if _, ok := f.prefs[globalDomain]["NSAppSleepDisabled"]; ok {
		t.Errorf("override should be deleted")
	}
	if u, err := s.UndoOptimize(context.Background(), "legacy_overrides_audit"); err != nil || !u.OK {
		t.Fatalf("undo: %v %+v", err, u)
	}
	if !fragTruthy(f.prefs[globalDomain]["NSAppSleepDisabled"]) {
		t.Errorf("undo should write the override back")
	}
}

func TestSpotlightRulesPrunesOnlyOrphans(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	f.setPref(spotlightDomain, spotlightRulesKey, "<array><string>System.files</string><string>com.apple.Safari</string><string>com.gone.app</string><string>com.here.app</string><string>Custom.thing</string></array>")
	f.mdfind["com.here.app"] = true

	it := diagnose(t, s, "spotlight_orphan_rules_cleanup")
	if it.Status != OptimizeNeeded || len(it.Items) != 1 || it.Items[0] != "com.gone.app" {
		t.Fatalf("diagnosis: %+v", it)
	}
	before := f.prefs[spotlightDomain][spotlightRulesKey]
	r := runOne(t, s, "spotlight_orphan_rules_cleanup")
	if !r.OK || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	rules, err := parseStringArray(f.prefs[spotlightDomain][spotlightRulesKey])
	if err != nil || strings.Join(rules, ",") != "System.files,com.apple.Safari,com.here.app,Custom.thing" {
		t.Fatalf("rules after = %v (%v)", rules, err)
	}
	if u, err := s.UndoOptimize(context.Background(), "spotlight_orphan_rules_cleanup"); err != nil || !u.OK {
		t.Fatalf("undo: %v %+v", err, u)
	}
	restored, _ := parseStringArray(f.prefs[spotlightDomain][spotlightRulesKey])
	original, _ := parseStringArray(before)
	if strings.Join(restored, ",") != strings.Join(original, ",") {
		t.Errorf("undo restored %v, want %v", restored, original)
	}
}

func TestSpotlightRulesLeavesEverythingWhenMdfindIsUncertain(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	f.setPref(spotlightDomain, spotlightRulesKey, "<array><string>com.gone.app</string></array>")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if st, err := s.spotlightScan(ctx); err != nil || len(st.orphans) != 0 {
		t.Errorf("a cancelled lookup must not make orphans: %+v %v", st, err)
	}
}

func TestParseStringArrayRejectsOtherTypes(t *testing.T) {
	if _, err := parseStringArray(wrapPlist("<array><string>a</string><integer>1</integer></array>")); err == nil {
		t.Errorf("mixed arrays must not be rewritten")
	}
	got, err := parseStringArray(stringArrayFragment([]string{"a&b", "c"}))
	if err != nil || strings.Join(got, ",") != "a&b,c" {
		t.Errorf("round trip: %v %v", got, err)
	}
}

func TestSharedFileListsTrashesBrokenOutsideRecentDocuments(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	dir := lib(s, "Application Support", "com.apple.sharedfilelist")
	bad := filepath.Join(dir, "com.apple.LSSharedFileList.FavoriteItems.sfl3")
	writeFile(t, bad, "BROKEN")
	writeFile(t, filepath.Join(dir, "good.sfl2"), "fine")
	recent := filepath.Join(dir, "ApplicationRecentDocuments", "com.x.sfl3")
	writeFile(t, recent, "BROKEN")

	r := runOne(t, s, "shared_file_list_repair")
	if !r.OK || !r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Errorf("broken list should be trashed")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent documents must never be touched: %v", err)
	}
}

func TestLaunchAgentsReportsOnly(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	gone := lib(s, "LaunchAgents", "com.gone.plist")
	writeFile(t, gone, `{"Label":"com.gone.agent","ProgramArguments":["/nonexistent/orbit-test/bin","-x"]}`)
	writeFile(t, lib(s, "LaunchAgents", "bare.plist"), `{"Label":"com.bare","ProgramArguments":["python3"]}`)
	writeFile(t, lib(s, "LaunchAgents", "ok.plist"), `{"Label":"com.ok","Program":"/bin/ls"}`)
	writeFile(t, lib(s, "LaunchAgents", "unplugged.plist"), `{"Label":"com.unplugged","Program":"/Volumes/orbit-test-not-mounted/x"}`)

	it := diagnose(t, s, "launch_agents_cleanup")
	if it.Status != OptimizeNeeded || len(it.Items) != 1 || !strings.HasPrefix(it.Items[0], "com.gone.agent") {
		t.Fatalf("diagnosis: %+v", it)
	}
	r := runOne(t, s, "launch_agents_cleanup")
	if !r.OK || r.Outcome != OutcomeAttention || r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	if _, err := os.Stat(gone); err != nil {
		t.Errorf("report only: the agent file must stay: %v", err)
	}
}

func TestLoginItemsReportsMissingApps(t *testing.T) {
	s := newTestScanner(t)
	f := newFakeSys(s)
	present := filepath.Join(s.home, "Present.app")
	if err := os.MkdirAll(present, 0o755); err != nil {
		t.Fatal(err)
	}
	f.osascript = "Present\t" + present + "\nZzqx Orbit Test Gone\t/nonexistent/Zzqx.app\n"
	r := runOne(t, s, "login_items_audit")
	if !r.OK || r.Outcome != OutcomeAttention || len(r.Details) != 1 || r.Details[0] != "Zzqx Orbit Test Gone" || r.CanUndo {
		t.Fatalf("receipt: %+v", r)
	}
	f.osascript = ""
	if r := runOne(t, s, "login_items_audit"); r.Outcome != OutcomeUnchanged {
		t.Errorf("no items: %+v", r)
	}
}

func TestRunOptimizeIsSingleFlight(t *testing.T) {
	s := newTestScanner(t)
	newFakeSys(s)
	if !optimizeBusy.TryLock() {
		t.Fatal("lock unexpectedly held")
	}
	defer optimizeBusy.Unlock()
	if err := s.RunOptimize(context.Background(), []string{"cache_refresh"}, func(OptimizeReceipt) {}); !errors.Is(err, ErrOptimizeBusy) {
		t.Errorf("want ErrOptimizeBusy, got %v", err)
	}
}
