package scanner

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstalledAppCacheOnlyFindsRebuildableSupportCaches(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Visual Studio Code.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Code</string><key>CFBundleIdentifier</key><string>com.microsoft.VSCode</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/payload"), "app fixture")
	want := map[string]bool{}
	for _, name := range []string{"Cache", "Code Cache", "GPUCache"} {
		cache := fixtureDir(t, home, "Library/Application Support/Code/"+name, 4096)
		want[cache] = true
	}
	personal := fixtureDir(t, home, "Library/Application Support/Code/User", 4096)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	rows := scanner.scanInstalledApps(filepath.Dir(app))
	if len(rows) != 1 {
		t.Fatalf("installed app rows = %v", rows)
	}
	row := rows[0]
	if floatOf(row["cacheSizeGb"]) <= 0 {
		t.Fatalf("cache-only reports no cache despite three rebuildable caches under Application Support: %+v", row)
	}
	var cacheBytes float64
	var measuredPaths []string
	for _, subpath := range row["subpaths"].([]map[string]any) {
		path := scanner.expand(subpath["path"].(string))
		for _, prior := range measuredPaths {
			if withinPath(path, prior) || withinPath(prior, path) {
				t.Fatalf("app cache and configuration overlap: %s and %s", path, prior)
			}
		}
		measuredPaths = append(measuredPaths, path)
		if subpath["kind"] != "cache" {
			continue
		}
		if !want[path] || withinPath(path, personal) || withinPath(personal, path) || path == app {
			t.Fatalf("cache-only classified personal data or the app as cache: %+v", subpath)
		}
		if subpath["checked"] != true || subpath["partial"] != false {
			t.Fatalf("idle rebuildable cache unavailable to cache-only selection: %+v", subpath)
		}
		cacheBytes += floatOf(subpath["sizeGb"])
		delete(want, path)
	}
	if len(want) != 0 || math.Abs(cacheBytes-floatOf(row["cacheSizeGb"])) > 1e-12 {
		t.Fatalf("cache-only omitted paths or reports a different total: missing=%v row=%+v", want, row)
	}
}

func TestVSCodeCacheAndConfigFollowBundleIdentifierAfterRename(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Renamed Editor.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Editor Display Name</string><key>CFBundleIdentifier</key><string>com.microsoft.VSCode</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/payload"), "app fixture")
	cache := fixtureDir(t, home, "Library/Application Support/Code/Code Cache", 4096)
	config := fixtureDir(t, home, "Library/Application Support/Code/User", 4096)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	rows := scanner.scanInstalledApps(filepath.Dir(app))
	if len(rows) != 1 || floatOf(rows[0]["cacheSizeGb"]) <= 0 {
		t.Fatalf("renamed VS Code without BundleName lost its known cache root: %v", rows)
	}
	want := map[string]string{cache: "cache", config: "config"}
	for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
		path := scanner.expand(subpath["path"].(string))
		if kind, ok := want[path]; ok {
			if subpath["kind"] != kind {
				t.Fatalf("stable bundle identifier attributed %s as %v, want %s", path, subpath["kind"], kind)
			}
			delete(want, path)
		}
	}
	if len(want) != 0 {
		t.Fatalf("stable bundle identifier omitted known Code data: %v", want)
	}
}

func TestNestedBrowserSupportRootsKeepProfileCaches(t *testing.T) {
	for _, browser := range []struct{ name, bundleID string }{
		{name: "Arc", bundleID: "company.thebrowser.Browser"},
		{name: "Dia", bundleID: "company.thebrowser.dia"},
	} {
		t.Run(browser.name, func(t *testing.T) {
			home := fixtureHome(t)
			app := filepath.Join(home, "Applications", browser.name+".app")
			writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>`+browser.name+`</string><key>CFBundleIdentifier</key><string>`+browser.bundleID+`</string></dict></plist>`)
			cache := fixtureDir(t, home, "Library/Application Support/"+browser.name+"/User Data/Default/Code Cache", 4096)
			fixtureDir(t, home, "Library/Application Support/"+browser.name+"/User Data/Default/Cookies", 4096)
			fixtureDir(t, home, "Library/Application Support/"+browser.name+"/Preferences", 4096)
			scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
			rows := scanner.scanInstalledApps(filepath.Dir(app))
			if len(rows) != 1 || floatOf(rows[0]["cacheSizeGb"]) <= 0 {
				t.Fatalf("nested browser cache was lost beneath its display-name root: %v", rows)
			}
			for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
				if subpath["kind"] == "cache" && scanner.expand(subpath["path"].(string)) == cache {
					return
				}
			}
			t.Fatalf("known nested browser profile cache was omitted: %s", cache)
		})
	}
}

func TestAppCachePartitionLimitRemainsVisible(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, "Library/Application Support/Code")
	fixtureDir(t, home, "Library/Application Support/Code/Code Cache", 4096)
	partitions := filepath.Join(root, "Partitions")
	for index := 0; index <= 1000; index++ {
		if err := os.MkdirAll(filepath.Join(partitions, "partition_"+itoa64(int64(index))), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	appSupportCacheCandidates(root, false)
	// A later successful listing must not erase evidence that this scan did
	// not inspect the partition caches within its bounded discovery budget.
	if _, err := readDirEntries(partitions); err != nil {
		t.Fatal(err)
	}
	for _, warning := range scanReadWarnings(home) {
		if warning["path"] == partitions {
			if warning["kind"] != "limit" || !strings.Contains(warning["reason"].(string), "上限") {
				t.Fatalf("partition cap was misclassified as a permission failure: %+v", warning)
			}
			return
		}
	}
	t.Fatal("partition discovery limit was silently omitted from scan feedback")
}

func TestAppSupportCacheDiscoveryPreservesPrivateDataAndNeedsMarkers(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Application Support/Example/Cache", 4096)
	fixtureDir(t, home, "Library/Application Support/Example/CachedData", 4096)
	scanner := &Scanner{home: home}
	for _, candidate := range scanner.appCacheCandidates("Example", "com.example.app") {
		if withinPath(candidate, filepath.Join(home, "Library/Application Support/Example")) {
			t.Fatalf("a cache-shaped directory without a regenerable layout was accepted: %s", candidate)
		}
	}
	fixtureDir(t, home, "Library/Application Support/Example/Code Cache", 4096)
	for _, private := range []string{"Cookies", "IndexedDB", "Local Storage", "Session Storage", "Backups", "User", "Extensions", "Service Worker/CacheStorage", "Dictionaries"} {
		fixtureDir(t, home, "Library/Application Support/Example/"+private, 4096)
	}
	for _, candidate := range scanner.appCacheCandidates("Example", "com.example.app") {
		if withinPath(candidate, filepath.Join(home, "Library/Application Support/Example")) {
			if leaf := filepath.Base(candidate); leaf != "Cache" && leaf != "CachedData" && leaf != "Code Cache" {
				t.Fatalf("private app state was classified as a rebuildable cache: %s", candidate)
			}
		}
	}
}

func TestEgoProfileAndSandboxCachesKeepProductAndPersonalBoundaries(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/ego lite.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>ego lite</string><key>CFBundleIdentifier</key><string>com.citrolabs.ego.lite</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/payload"), "app fixture")
	want := map[string]bool{}
	for _, path := range []string{
		"Library/Application Support/Citro Labs/ego lite/component_crx_cache",
		"Library/Application Support/Citro Labs/ego lite/Profile 1/Code Cache",
		"Library/Application Support/Citro Labs/ego lite/Profile 1/Service Worker/ScriptCache",
		"Library/Containers/com.citrolabs.ego.lite/Data/Library/Caches",
	} {
		want[fixtureDir(t, home, path, 4096)] = true
	}
	for _, personal := range []string{
		"Library/Application Support/Citro Labs/ego lite/Profile 1/Cookies",
		"Library/Application Support/Citro Labs/ego lite/Profile 1/Service Worker/CacheStorage",
		"Library/Application Support/Citro Labs/EgoUpdater/prefs.json",
		"Library/Containers/com.citrolabs.ego.lite/Data/Documents",
	} {
		fixtureDir(t, home, personal, 4096)
	}
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	rows := scanner.scanInstalledApps(filepath.Dir(app))
	if len(rows) != 1 {
		t.Fatalf("installed Ego rows = %v", rows)
	}
	for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
		path := scanner.expand(subpath["path"].(string))
		if path == filepath.Join(home, "Library/Application Support/Citro Labs") || withinPath(path, filepath.Join(home, "Library/Application Support/Citro Labs/EgoUpdater")) {
			t.Fatalf("shared vendor/updater state attributed as removable Ego data: %+v", subpath)
		}
		if subpath["kind"] == "cache" {
			if !want[path] {
				t.Fatalf("cache-only includes a profile, personal directory or container parent: %+v", subpath)
			}
			delete(want, path)
		}
	}
	if len(want) != 0 {
		t.Fatalf("Ego profile or sandbox caches missing: %v", want)
	}
}

func TestVendorNamedBundleAliasDoesNotClaimSiblingData(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Citro Labs.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>ego lite</string><key>CFBundleIdentifier</key><string>com.citrolabs.ego.lite</string></dict></plist>`)
	cache := fixtureDir(t, home, "Library/Application Support/Citro Labs/ego lite/Profile 1/Code Cache", 4096)
	config := fixtureDir(t, home, "Library/Application Support/Citro Labs/ego lite/Profile 1/Cookies", 4096)
	var unrelated []string
	for _, path := range []string{
		"Library/Application Support/Citro Labs/ego",
		"Library/Application Support/Citro Labs/EgoUpdater",
		"Library/Caches/Citro Labs/Other Product",
	} {
		unrelated = append(unrelated, fixtureDir(t, home, path, 4096))
	}
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	rows := scanner.scanInstalledApps(filepath.Dir(app))
	if len(rows) != 1 {
		t.Fatalf("renamed Ego app rows = %v", rows)
	}
	want := map[string]string{cache: "cache", config: "config"}
	for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
		path := scanner.expand(subpath["path"].(string))
		for _, other := range unrelated {
			if withinPath(path, other) || withinPath(other, path) {
				t.Fatalf("vendor-named bundle alias claimed sibling/updater data: %+v", subpath)
			}
		}
		if kind, ok := want[path]; ok {
			if subpath["kind"] != kind {
				t.Fatalf("renamed app attributed %s as %v, want %s", path, subpath["kind"], kind)
			}
			delete(want, path)
		}
	}
	if len(want) != 0 {
		t.Fatalf("excluding the vendor alias also lost the app's own data: %v", want)
	}
}

func TestAppCacheActivityMeasuredAndRecheckedAtMoveBoundary(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Renamed Editor.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Editor Display Name</string><key>CFBundleIdentifier</key><string>com.microsoft.VSCode</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/payload"), "app fixture")
	cache := fixtureDir(t, home, "Library/Application Support/Code/Code Cache", 4096)
	processes := "/sbin/launchd\n"
	scanner := &Scanner{home: home, processTTL: time.Minute, processProbe: func() (string, error) { return processes, nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	if reason := scanner.CleanupReason(cache); reason != "" {
		t.Fatalf("idle measured cache refused: %s", reason)
	}
	processes = "7 " + app + "/Contents/MacOS/payload\n"
	if reason := scanner.cleanupReason(cache, true); !strings.Contains(reason, "正在运行") {
		t.Fatalf("move boundary reused an idle preview after owning app started: %q", reason)
	}
	scanner.processTTL = 0
	rows := scanner.scanInstalledApps(filepath.Dir(app))
	if len(rows) != 1 || floatOf(rows[0]["cacheSizeGb"]) <= 0 {
		t.Fatalf("running app hid its measured caches: %v", rows)
	}
	var found bool
	for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
		if subpath["kind"] == "cache" && scanner.expand(subpath["path"].(string)) == cache {
			found = true
			if subpath["checked"] != false || subpath["tier"] != "protected" || floatOf(subpath["sizeGb"]) <= 0 {
				t.Fatalf("running app cache should remain measured but unavailable: %+v", subpath)
			}
		}
	}
	if !found {
		t.Fatal("stable bundle identifier lost the Code cache")
	}
	processes = "/sbin/launchd\n"
	scanner.handleProbe = func(string) (bool, error) { return true, nil }
	if reason := scanner.cleanupReason(cache, true); !strings.Contains(reason, "仍有进程打开") {
		t.Fatalf("move boundary ignored open cache handles: %q", reason)
	}
	scanner.handleProbe = func(string) (bool, error) { return false, errors.New("probe unavailable") }
	if reason := scanner.cleanupReason(cache, true); !strings.Contains(reason, "无法确认") {
		t.Fatalf("move boundary accepted an unknown open-file state: %q", reason)
	}
}

func TestAppCacheHelperOnlyActivityProtectsOuterApplication(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Editor.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Editor Display Name</string><key>CFBundleIdentifier</key><string>com.microsoft.VSCode</string></dict></plist>`)
	writeFile(t, filepath.Join(app, "Contents/MacOS/payload"), "app fixture")
	helper := filepath.Join(app, "Contents/Frameworks/Helper.app")
	writeFile(t, filepath.Join(helper, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Helper</string><key>CFBundleIdentifier</key><string>com.microsoft.VSCode.helper</string></dict></plist>`)
	writeFile(t, filepath.Join(helper, "Contents/MacOS/Helper"), "helper fixture")
	cache := fixtureDir(t, home, "Library/Application Support/Code/Code Cache", 4096)
	helperCache := fixtureDir(t, home, "Library/Application Support/Helper/Code Cache", 4096)
	processes := "/sbin/launchd\n"
	scanner := &Scanner{home: home, processTTL: time.Minute, processProbe: func() (string, error) { return processes, nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	if reason := scanner.CleanupReason(cache); reason != "" {
		t.Fatalf("idle cache refused: %s", reason)
	}
	processes = "912 " + filepath.Join(helper, "Contents/MacOS/Helper") + "\n"
	t.Run("final_move_check", func(t *testing.T) {
		for _, path := range []string{cache, helperCache} {
			if reason := scanner.cleanupReason(path, true); !strings.Contains(reason, "正在运行") {
				t.Errorf("helper-only activity must protect every owning bundle's cache at the live move boundary: %s: %q", path, reason)
			}
		}
	})
	scanner.processTTL = 0
	t.Run("scan", func(t *testing.T) {
		rows := scanner.scanInstalledApps(filepath.Dir(app))
		if len(rows) != 1 || rows[0]["running"] != true || floatOf(rows[0]["cacheSizeGb"]) <= 0 {
			t.Fatalf("helper-only application lost its running state or measured cache: %v", rows)
		}
		for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
			if subpath["kind"] != "cache" || scanner.expand(subpath["path"].(string)) != cache {
				continue
			}
			if subpath["checked"] != false || subpath["tier"] != "protected" || !strings.Contains(subpath["reason"].(string), "正在运行") {
				t.Fatalf("helper-only application cache remained selectable: %+v", subpath)
			}
			return
		}
		t.Fatal("helper-only application hid its measured cache")
	})
}

func TestRenamedAIAppHelperProtectsSpecialCachePaths(t *testing.T) {
	for _, tool := range []struct{ name, bundleID string }{
		{name: "Cursor", bundleID: cursorBundleID},
		{name: "Claude", bundleID: "com.anthropic.claudefordesktop"},
		{name: "Codex", bundleID: "com.openai.codex"},
	} {
		t.Run(tool.name, func(t *testing.T) {
			home := fixtureHome(t)
			app := filepath.Join(home, "Applications/Editor.app")
			writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleDisplayName</key><string>Renamed Tool</string><key>CFBundleName</key><string>`+tool.name+`</string><key>CFBundleIdentifier</key><string>`+tool.bundleID+`</string></dict></plist>`)
			helper := filepath.Join(app, "Contents/Frameworks/Agent.app")
			writeFile(t, filepath.Join(helper, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>Agent</string><key>CFBundleIdentifier</key><string>`+tool.bundleID+`.helper</string></dict></plist>`)
			cache := fixtureDir(t, home, "Library/Application Support/"+tool.name+"/Code Cache", 4096)
			processes := "/sbin/launchd\n"
			scanner := &Scanner{home: home, processTTL: time.Minute, processProbe: func() (string, error) { return processes, nil }, handleProbe: func(string) (bool, error) { return false, nil }}
			if reason := scanner.CleanupReason(cache); reason != "" {
				t.Fatalf("idle special AI cache refused: %q", reason)
			}
			processes = "44 " + filepath.Join(helper, "Contents/MacOS/Agent") + "\n"
			if reason := scanner.cleanupReason(cache, true); !strings.Contains(reason, "正在运行") {
				t.Fatalf("special AI cache bypassed the renamed outer-app owner at move time: %q", reason)
			}
			scanner.processTTL = 0
			rows := scanner.scanInstalledApps(filepath.Dir(app))
			if len(rows) != 1 || floatOf(rows[0]["cacheSizeGb"]) <= 0 {
				t.Fatalf("renamed AI app lost its measured cache: %v", rows)
			}
			for _, subpath := range rows[0]["subpaths"].([]map[string]any) {
				if subpath["kind"] == "cache" && scanner.expand(subpath["path"].(string)) == cache {
					if subpath["tier"] != "protected" || subpath["checked"] != false {
						t.Fatalf("special AI cache remained selectable with a helper running: %+v", subpath)
					}
					return
				}
			}
			t.Fatal("renamed AI app hid its running cache")
		})
	}
}

func TestGroupContainerLiveMovePreservesOpenAndUnknownData(t *testing.T) {
	home := fixtureHome(t)
	group := fixtureDir(t, home, "Library/Group Containers/team.example.shared", 4096)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	for _, state := range []struct {
		name, want string
		open       bool
		err        error
	}{
		{name: "idle"},
		{name: "open", open: true, want: "仍有进程打开"},
		{name: "unknown", err: errors.New("probe unavailable"), want: "无法确认"},
	} {
		t.Run(state.name, func(t *testing.T) {
			scanner.handleProbe = func(string) (bool, error) { return state.open, state.err }
			reason := scanner.cleanupReason(group, true)
			if state.want == "" && reason != "" || state.want != "" && !strings.Contains(reason, state.want) {
				t.Fatalf("group-container live move returned %q, want %q", reason, state.want)
			}
		})
	}
}

func TestAppSupportCacheSymlinksDoNotGrantCleanupBoundary(t *testing.T) {
	for _, marker := range []struct{ link, child string }{
		{link: "Code Cache"},
		{link: "Crashpad", child: "completed"},
		{link: "Service Worker", child: "ScriptCache"},
	} {
		t.Run(marker.link, func(t *testing.T) {
			home := fixtureHome(t)
			personal := fixtureDir(t, home, "Documents/private", 4096)
			if marker.child != "" {
				fixtureDir(t, home, "Documents/private/"+marker.child, 4096)
			}
			root := filepath.Join(home, "Library/Application Support/Example")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(personal, filepath.Join(root, marker.link)); err != nil {
				t.Fatal(err)
			}
			fixtureDir(t, home, "Library/Application Support/Example/Cache", 4096)
			scanner := &Scanner{home: home}
			for _, path := range scanner.appCacheCandidates("Example", "com.example.app") {
				if withinPath(path, root) {
					t.Fatalf("a symlink marker authorized a private data cache: %s", path)
				}
			}
		})
	}
}
