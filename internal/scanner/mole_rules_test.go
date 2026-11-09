package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func moleTestScanner(t *testing.T, processes string) *Scanner {
	s := newTestScanner(t)
	// Mole's default whitelist hides ~/.gradle/caches; a user whitelist file replaces it.
	if err := os.MkdirAll(filepath.Join(s.home, ".config", "mole"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeWhitelist(t, s.home, "~/nothing-here")
	s.processProbe = func() (string, error) { return processes, nil }
	s.handleProbe = func(string) (bool, error) { return false, nil }
	s.builderProbe = func() ([]builderProc, error) { return nil, nil }
	return s
}

// moleSubpaths scans the clean groups and indexes every item by its home-relative path.
func moleSubpaths(t *testing.T, s *Scanner) (map[string]map[string]any, []map[string]any) {
	t.Helper()
	groups := s.scanCleanGroups()
	items := map[string]map[string]any{}
	for _, group := range groups {
		for _, sub := range group["subpaths"].([]map[string]any) {
			items[sub["path"].(string)] = sub
		}
	}
	return items, groups
}

func TestMoleRulesTiersAndInvariants(t *testing.T) {
	s := moleTestScanner(t, "/usr/bin/idle\n")
	for _, p := range []string{
		".turbo/cache", ".pytest_cache", "Library/Saved Application State/com.foo.savedState",
		"Library/IdentityCaches/abc", ".gradle/caches/build-cache-1", ".gradle/caches/modules-2",
		".cocoapods/repos", "Library/Application Support/discord/Cache",
		"Library/Application Support/Google/Chrome/Default/Code Cache",
		"Library/Application Support/Google/Chrome/Default/History",
	} {
		writeFile(t, filepath.Join(s.home, p, "blob.bin"), strings.Repeat("x", 4096))
	}
	items, groups := moleSubpaths(t, s)
	want := map[string]string{
		"~/.turbo/cache":                                                 "safe",
		"~/.pytest_cache":                                                "safe",
		"~/.gradle/caches/build-cache-1":                                 "safe",
		"~/.gradle/caches/modules-2":                                     "review",
		"~/.cocoapods/repos":                                             "review",
		"~/Library/IdentityCaches/abc":                                   "review",
		"~/Library/Saved Application State/com.foo.savedState":           "safe",
		"~/Library/Application Support/discord/Cache":                    "safe",
		"~/Library/Application Support/Google/Chrome/Default/Code Cache": "safe",
	}
	for path, tier := range want {
		got, ok := items[path]
		if !ok {
			t.Errorf("%s is not offered", path)
			continue
		}
		if got["tier"] != tier {
			t.Errorf("%s: tier %v, want %s (%v)", path, got["tier"], tier, got["reason"])
		}
	}
	if _, ok := items["~/Library/Application Support/Google/Chrome/Default/History"]; ok {
		t.Error("browser history must never be offered")
	}
	for path, sub := range items {
		tier, _ := sub["tier"].(string)
		if tier == "safe" && sub["checked"] != true {
			t.Errorf("%s: safe must be checked", path)
		}
		if tier != "safe" && sub["checked"] == true {
			t.Errorf("%s: %s must not be checked", path, tier)
		}
		if tier != "safe" && sub["reason"] == "" {
			t.Errorf("%s: %s without a reason", path, tier)
		}
	}
	for _, group := range groups {
		if tier := group["tier"]; tier != "safe" && group["reason"] == "" {
			t.Errorf("group %v: %v without a reason", group["id"], tier)
		}
	}
}

func TestMoleRulesRespectWhitelistAndRunningOwners(t *testing.T) {
	s := moleTestScanner(t, "/Applications/Discord.app/Contents/MacOS/Discord\n")
	cache := filepath.Join(s.home, "Library/Application Support/discord/Cache")
	pytest := filepath.Join(s.home, ".pytest_cache")
	writeFile(t, filepath.Join(cache, "a"), "data")
	writeFile(t, filepath.Join(pytest, "a"), "data")
	writeWhitelist(t, s.home, "~/.pytest_cache")
	s.refreshWhitelist()
	items, _ := moleSubpaths(t, s)
	if got := items["~/Library/Application Support/discord/Cache"]; got == nil || got["tier"] != "protected" || !strings.Contains(got["reason"].(string), "Discord") || got["unlock"] == "" {
		t.Fatalf("a running owner protects the cache with an unlock hint: %v", got)
	}
	if reason := s.CleanupReason(cache); !strings.Contains(reason, "Discord") {
		t.Fatalf("the move re-checks the owner: %q", reason)
	}
	if reason := s.CleanupReason(pytest); reason == "" {
		t.Fatal("a whitelisted path is never cleaned")
	}
	if _, ok := items["~/.pytest_cache"]; ok {
		t.Fatal("a whitelisted path is not offered")
	}
}

func TestMoleGradleCacheNeedsIdleGradle(t *testing.T) {
	s := moleTestScanner(t, "/usr/bin/idle\n")
	cache := filepath.Join(s.home, ".gradle/caches/build-cache-1")
	writeFile(t, filepath.Join(cache, "a"), "data")
	if reason := s.CleanupReason(cache); reason != "" {
		t.Fatalf("idle gradle: %q", reason)
	}
	s.builderProbe = func() ([]builderProc, error) {
		return []builderProc{{pid: 7, name: "java", args: "/usr/bin/java -cp gradle-launcher.jar org.gradle.launcher.daemon.bootstrap.GradleDaemon"}}, nil
	}
	if reason := s.CleanupReason(cache); !strings.Contains(reason, "gradle") {
		t.Fatalf("running gradle: %q", reason)
	}
	s.builderProbe = func() ([]builderProc, error) { return nil, errors.New("lsof unavailable") }
	if reason := s.CleanupReason(cache); reason == "" {
		t.Fatal("an unreadable builder state must keep the cache")
	}
}

func TestMoleDeviceSupportKeepsNewestTwo(t *testing.T) {
	s := moleTestScanner(t, "/usr/bin/idle\n")
	base := filepath.Join(s.home, "Library/Developer/Xcode/iOS DeviceSupport")
	now := time.Now()
	for i, name := range []string{"16.0", "17.0", "18.0", "15.0"} {
		dir := filepath.Join(base, name)
		writeFile(t, filepath.Join(dir, "sym"), "symbols")
		mtime := now.Add(-time.Duration(i+1) * 24 * time.Hour)
		if name == "15.0" {
			mtime = now.Add(-100 * 24 * time.Hour)
		}
		if err := os.Chtimes(dir, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := moleSubpaths(t, s)
	tiers := map[string]string{}
	for path, sub := range items {
		if rest, ok := strings.CutPrefix(path, "~/Library/Developer/Xcode/iOS DeviceSupport/"); ok {
			tiers[rest] = sub["tier"].(string)
		}
	}
	// 16.0 (1 day) and 17.0 (2 days) are the newest two; 18.0 (3 days) and 15.0 are offered.
	want := map[string]string{"16.0": "protected", "17.0": "protected", "18.0": "safe", "15.0": "safe"}
	for name, tier := range want {
		if tiers[name] != tier {
			t.Errorf("%s: %q, want %s (%v)", name, tiers[name], tier, tiers)
		}
	}
}

func TestGlobHomeSkipsDotfilesAndSymlinks(t *testing.T) {
	s := newTestScanner(t)
	writeFile(t, filepath.Join(s.home, "Library/Suggestions/.hidden/a"), "x")
	writeFile(t, filepath.Join(s.home, "Library/Suggestions/real/a"), "x")
	if err := os.Symlink(s.home, filepath.Join(s.home, "Library/Suggestions/link")); err != nil {
		t.Fatal(err)
	}
	got := s.globHome("Library/Suggestions/*")
	if len(got) != 1 || filepath.Base(got[0]) != "real" {
		t.Fatalf("globHome = %v", got)
	}
	if got := s.globHome("Library/Suggestions/.hid*"); len(got) != 1 {
		t.Fatalf("an explicit dot segment matches dotfiles: %v", got)
	}
}

func TestRegenerableApplePrefixes(t *testing.T) {
	s := newTestScanner(t)
	if v := s.cacheVerdict(filepath.Join(s.home, "Library/Caches/com.apple.iconservices.store"), dirSize{}); v.tier != "safe" {
		t.Fatalf("iconservices cache: %v", v)
	}
	if v := s.cacheVerdict(filepath.Join(s.home, "Library/Caches/com.apple.dt.Xcode"), dirSize{}); v.tier != "safe" {
		t.Fatalf("Xcode cache: %v", v)
	}
}

func TestFindAppBundlesDepthAndWrapper(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"A.app/Contents", "Setapp/B.app/Contents", "x/y/C.app/Contents", "x/y/z/D.app/Contents", "iOS.app/Wrapper/Inner.app"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, p := range findAppBundles([]string{root}) {
		got = append(got, strings.TrimPrefix(p, root+"/"))
	}
	want := map[string]bool{"A.app": true, "Setapp/B.app": true, "x/y/C.app": true, "iOS.app": true}
	if len(got) != len(want) {
		t.Fatalf("found %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected %s in %v", p, got)
		}
	}
}

func TestReadBundleInfoFallsBackToIOSWrapper(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Game.app")
	writeFile(t, filepath.Join(app, "Wrapper", "Game.app", "Info.plist"), `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.example.game</string></dict></plist>`)
	if got := readBundleInfo(app).identifier; got != "com.example.game" {
		t.Fatalf("identifier = %q", got)
	}
}

func TestOrphansSkipCredentialStoresAndFindClaudeVM(t *testing.T) {
	if exists("/Applications/Claude.app") {
		t.Skip("Claude Desktop is installed on this machine")
	}
	s := moleTestScanner(t, "/usr/bin/idle\n")
	big := strings.Repeat("x", 16_000_000)
	writeFile(t, filepath.Join(s.home, "Library/Application Support/OldPasswordVault-1Password/data"), big)
	writeFile(t, filepath.Join(s.home, "Library/Application Support/AbandonedTool/data"), big)
	vm := filepath.Join(s.home, claudeSupportDir, "vm_bundles", "claudevm.bundle")
	writeFile(t, filepath.Join(vm, "disk.img"), big)
	young := filepath.Join(s.home, claudeSupportDir, "vm_bundles", "young.bundle")
	writeFile(t, filepath.Join(young, "disk.img"), big)
	old := time.Now().Add(-10 * 24 * time.Hour)
	for _, p := range []string{vm, filepath.Join(vm, "disk.img")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	names := map[string]bool{}
	for _, row := range s.scanOrphanedLeftovers() {
		names[row["name"].(string)] = true
		for _, sub := range row["subpaths"].([]map[string]any) {
			if sub["checked"] == true {
				t.Errorf("orphan %v must be unchecked", row["name"])
			}
		}
	}
	if !names["AbandonedTool"] || names["OldPasswordVault-1Password"] {
		t.Fatalf("credential stores are never leftovers: %v", names)
	}
	if !names["Claude 工作区虚拟机"] {
		t.Fatalf("an idle orphaned Claude VM bundle is offered: %v", names)
	}
	if reason := s.CleanupReason(young); !strings.Contains(reason, "7 天") {
		t.Fatalf("a young VM bundle stays: %q", reason)
	}
	if reason := s.CleanupReason(vm); reason != "" {
		t.Fatalf("old VM bundle: %q", reason)
	}
	s.processProbe = func() (string, error) { return "/Applications/Claude.app/Contents/MacOS/Claude\n", nil }
	if reason := s.CleanupReason(vm); !strings.Contains(reason, "Claude") {
		t.Fatalf("a running Claude protects its VM: %q", reason)
	}
	if reason := s.CleanupReason(filepath.Join(s.home, claudeSupportDir, "IndexedDB")); reason == "" {
		t.Fatal("the rest of Claude's data stays protected")
	}
}
