package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func policyScanner(home string, procs ...builderProc) *Scanner {
	return &Scanner{
		home:         home,
		processProbe: func() (string, error) { return "/sbin/launchd\n", nil },
		builderProbe: func() ([]builderProc, error) { return procs, nil },
		handleProbe:  func(string) (bool, error) { return false, nil },
	}
}

// age sets every file and directory under path to days days ago.
func age(t *testing.T, path string, days int) {
	t.Helper()
	when := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	_ = filepath.WalkDir(path, func(p string, _ os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chtimes(p, when, when)
		}
		return nil
	})
}

// checkContract asserts the Go-to-page contract on every row and subpath.
func checkContract(t *testing.T, module string, rows []map[string]any) {
	t.Helper()
	check := func(where string, item map[string]any) {
		tier, _ := item["tier"].(string)
		reason, _ := item["reason"].(string)
		switch tier {
		case "safe":
			if item["checked"] != true {
				t.Errorf("%s %s: safe but not checked", module, where)
			}
		case "review", "protected":
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s %s: %s without a reason", module, where, tier)
			}
		default:
			t.Errorf("%s %s: tier = %q", module, where, tier)
		}
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		check(id, row)
		subpaths, _ := row["subpaths"].([]map[string]any)
		for _, sub := range subpaths {
			check(id+" > "+sub["path"].(string), sub)
		}
	}
}

func TestPayloadHonoursTierContract(t *testing.T) {
	home := fixtureHome(t)
	for _, dir := range []string{"go-build", "SomeUnknownTool", "com.apple.thing", "CloudKit", "Spotify"} {
		fixtureDir(t, home, "Library/Caches/"+dir, 4096)
		age(t, filepath.Join(home, "Library/Caches", dir), 60)
	}
	fixtureDir(t, home, "Library/Logs/Old", 4096)
	age(t, filepath.Join(home, "Library/Logs/Old"), 90)
	fixtureDir(t, home, "Library/Logs/Fresh", 4096)
	writeFile(t, filepath.Join(home, "Projects/old/package.json"), "{}")
	fixtureDir(t, home, "Projects/old/node_modules", 21_000_000)
	age(t, filepath.Join(home, "Projects/old"), 60)
	writeFile(t, filepath.Join(home, "Projects/mid/package.json"), "{}")
	fixtureDir(t, home, "Projects/mid/node_modules", 21_000_000)
	age(t, filepath.Join(home, "Projects/mid"), 15)
	writeFile(t, filepath.Join(home, "Projects/new/package.json"), "{}")
	fixtureDir(t, home, "Projects/new/node_modules", 21_000_000)
	writeTestFile(t, filepath.Join(home, "Downloads/App.dmg"), 21_000_000)
	writeTestFile(t, filepath.Join(home, "Downloads/macOS_Installer.zip"), 21_000_000)
	fixtureDir(t, home, ".claude/projects/-tmp-x", 4096)
	fixtureDir(t, home, ".claude/cache", 4096)

	payload := policyScanner(home).BuildFullScanPayload()
	checkContract(t, "cleanGroups", payload.CleanGroups)
	checkContract(t, "aiMoonBands", payload.AIMoonBands)
	checkContract(t, "purgeItems", payload.PurgeItems)
	checkContract(t, "installerFiles", payload.InstallerFiles)
	if len(payload.InstallerFiles) != 2 {
		t.Errorf("installers = %d, want 2", len(payload.InstallerFiles))
	}
	if len(payload.CleanGroups) == 0 || len(payload.PurgeItems) == 0 {
		t.Fatalf("fixture produced no rows: clean=%d purge=%d", len(payload.CleanGroups), len(payload.PurgeItems))
	}
	want := map[string]string{"old": "safe", "mid": "review", "new": "protected"}
	for _, row := range payload.PurgeItems {
		project := filepath.Base(row["project"].(string))
		if tier := want[project]; tier != row["tier"] {
			t.Errorf("purge %s tier = %v, want %s", project, row["tier"], tier)
		}
	}
}

func TestCacheVerdictClassification(t *testing.T) {
	home := fixtureHome(t)
	s := policyScanner(home)
	old := dirSize{modified: nowUnix() - 90*86400}
	cases := []struct{ path, tier string }{
		{"Library/Caches/go-build", "safe"},
		{"Library/Caches/Foo-updater", "safe"},
		{"Library/Caches/CloudKit", "protected"},
		{"Library/Caches/com.apple.mail", "protected"},
		{"Library/Caches/Spotify", "review"},
		{"Library/Caches/com.apple.Safari", "review"},
		{"Library/Caches/NobodyKnows", "review"},
		{"Library/Logs/Anything", "safe"},
	}
	for _, tc := range cases {
		v := s.cacheVerdict(filepath.Join(home, tc.path), old)
		if v.tier != tc.tier || v.reason == "" {
			t.Errorf("%s: %+v, want tier %s with a reason", tc.path, v, tc.tier)
		}
	}
	if v := s.cacheVerdict(filepath.Join(home, "Library/Logs/Recent"), dirSize{modified: nowUnix()}); v.tier != "review" {
		t.Errorf("a log written today is %s, want review", v.tier)
	}
}

func TestCacheOfRunningAppIsProtectedWithUnlock(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Foo.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example.foo</string><key>CFBundleName</key><string>Foo</string></dict></plist>`)
	cache := fixtureDir(t, home, "Library/Caches/com.example.foo", 4096)
	s := policyScanner(home)
	s.processProbe = func() (string, error) { return app + "/Contents/MacOS/Foo\n", nil }
	reason := s.cleanupReason(cache, true)
	if !strings.Contains(reason, "正在运行") {
		t.Fatalf("running app cache reason = %q", reason)
	}
	row := map[string]any{"tier": "safe", "impact": "x", "subpaths": []map[string]any{{"path": cache, "tier": "safe", "checked": true}}}
	s.applyCleanupProtection(row)
	if row["tier"] != "protected" || row["unlock"] == nil || row["reason"] == "" {
		t.Fatalf("running app row = %v", row)
	}
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	if reason := s.cleanupReason(cache, true); reason != "" {
		t.Fatalf("an idle app blocked its cache: %q", reason)
	}
}

func TestNpmCacheNeedsNpmCLINotBareNode(t *testing.T) {
	home := fixtureHome(t)
	cache := fixtureDir(t, home, ".npm/_cacache", 4096)
	node := builderProc{pid: 1, name: "node", cwd: "/", args: "node /Users/me/app/server.js"}
	if reason := policyScanner(home, node).cleanupReason(cache, true); reason != "" {
		t.Fatalf("a bare node process blocked the npm cache: %q", reason)
	}
	npm := builderProc{pid: 2, name: "node", cwd: "/", args: "node /usr/local/lib/node_modules/npm/bin/npm-cli.js install"}
	if reason := policyScanner(home, npm).cleanupReason(cache, true); !strings.Contains(reason, "正在运行") {
		t.Fatalf("npm install must hold its cache back: %q", reason)
	}
	run := builderProc{pid: 3, name: "node", cwd: "/", args: "node /usr/local/lib/node_modules/npm/bin/npm-cli.js run dev"}
	if reason := policyScanner(home, run).cleanupReason(cache, true); reason != "" {
		t.Fatalf("npm run dev does not use the cache: %q", reason)
	}
}

func TestPartialMeasurementNeverProtects(t *testing.T) {
	home := fixtureHome(t)
	dir := fixtureDir(t, home, "Library/Caches/go-build", 4096)
	row := map[string]any{"tier": "safe", "impact": "x", "subpaths": []map[string]any{
		{"path": dir, "tier": "safe", "checked": true, "partial": true},
	}}
	policyScanner(home).applyCleanupProtection(row)
	if row["tier"] != "safe" || row["checked"] != true {
		t.Fatalf("partial row was demoted: %v", row)
	}
}

func TestArtifactVerdictAgeTiers(t *testing.T) {
	home := fixtureHome(t)
	project := filepath.Join(home, "Projects/app")
	writeFile(t, filepath.Join(project, "pnpm-lock.yaml"), "")
	path := fixtureDir(t, home, "Projects/app/node_modules", 16)
	day := int64(86400)
	cases := []struct {
		days int64
		tier string
	}{{45, "safe"}, {30, "safe"}, {15, "review"}, {7, "review"}, {2, "protected"}}
	for _, tc := range cases {
		v := artifactVerdict(path, nowUnix()-tc.days*day)
		if v.tier != tc.tier || v.reason == "" {
			t.Errorf("%d days: %+v, want %s", tc.days, v, tc.tier)
		}
		if tc.tier == "protected" && v.unlock == "" {
			t.Errorf("%d days: protected without unlock", tc.days)
		}
		if tc.tier == "safe" && !strings.Contains(v.reason, "pnpm install") {
			t.Errorf("safe reason must name the rebuild command: %q", v.reason)
		}
	}
	if v := artifactVerdict(path, 0); v.tier != "review" {
		t.Errorf("unknown mtime = %s, want review", v.tier)
	}
}

func TestWorktreeVerdicts(t *testing.T) {
	home := fixtureHome(t)
	s := policyScanner(home)
	nm := fixtureDir(t, home, ".claude/worktrees/w/node_modules", 16)
	day := int64(86400)
	if v := s.worktreeArtifactVerdict(nm, dirSize{modified: nowUnix() - 10*day}); v.tier != "safe" {
		t.Errorf("idle worktree node_modules = %s, want safe", v.tier)
	}
	if v := s.worktreeArtifactVerdict(nm, dirSize{modified: nowUnix() - day}); v.tier != "review" || v.reason == "" {
		t.Errorf("fresh worktree node_modules = %+v, want review", v)
	}
	if v := worktreeVerdict("工作区干净", "kept", nowUnix()-60*day); v.tier != "review" {
		t.Errorf("clean and idle worktree = %s, want review", v.tier)
	}
	for _, tc := range []struct {
		state string
		age   int64
	}{{"工作区干净", 3}, {"有未提交改动", 90}, {"Git 状态无法核验，保留全部内容", 90}} {
		if v := worktreeVerdict(tc.state, "kept", nowUnix()-tc.age*day); v.tier != "protected" || v.reason != "kept" {
			t.Errorf("%s/%dd = %+v, want protected with the impact as reason", tc.state, tc.age, v)
		}
	}
}
