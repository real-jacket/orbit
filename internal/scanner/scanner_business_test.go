package scanner

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fixtureHome builds a temporary home with no whitelist file, so the default
// protection patterns apply exactly as they do on a fresh machine.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "mole"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.config/mole): %v", err)
	}
	return home
}

func TestAppSubpathsExposeUnroundedMeasuredSize(t *testing.T) {
	home := fixtureHome(t)
	cache := fixtureDir(t, home, "Library/Caches/Example", 1234)
	s := &Scanner{home: home}
	info, err := os.Stat(filepath.Join(cache, "payload.bin"))
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := allocatedSize(info)
	var paths []map[string]any
	if got := s.appendExisting([]string{cache}, "cache", true, &paths); got != wantBytes {
		t.Fatalf("bytes = %d", got)
	}
	if len(paths) != 1 || paths[0]["sizeGb"] != plainGB(wantBytes) {
		t.Fatalf("subpaths = %v", paths)
	}
}

func TestAIKeepsUnidentifiedVersionsAndInstalledExtensions(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, ".local/share/claude/versions/current", 4096)
	fixtureDir(t, home, ".cursor/extensions/vendor.extension", 4096)
	s := &Scanner{home: home}
	rows := s.scanAICaches()
	if len(rows) != 2 {
		t.Fatalf("expected the version and extension inspections: %v", rows)
	}
	for _, row := range rows {
		if row["tier"] != "protected" || row["checked"] != false {
			t.Fatalf("unknown versions and installed extensions cannot be cleanup candidates: %v", row)
		}
	}
}

func TestAIVersionPlanRechecksLauncherAndRunningProcesses(t *testing.T) {
	home := fixtureHome(t)
	s := &Scanner{home: home, processProbe: func() (string, error) { return "launchd\n", nil }}
	root := filepath.Join(home, aiVersionSpecs[0].root)
	var paths []string
	for i, name := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"} {
		path := fixtureDir(t, home, aiVersionSpecs[0].root+"/"+name, 4096)
		modified := time.Unix(1_700_000_000+int64(i)*100, 0)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	launcher := filepath.Join(home, aiVersionSpecs[0].launcher)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(paths[1], "payload.bin"), launcher); err != nil {
		t.Fatal(err)
	}
	rows := rowsByID(s.scanAICaches())
	for i, path := range paths {
		row := rows["ai_version:"+path]
		want := "review"
		if i == 1 || i == 3 {
			want = "protected"
		}
		if row == nil || row["tier"] != want || row["checked"] != false || row["path"] == s.relativize(root) {
			t.Fatalf("version %d must be a precisely bounded, unchecked %s row: %v", i, want, row)
		}
	}
	s.processProbe = func() (string, error) { return filepath.Join(paths[0], "payload.bin") + "\n", nil }
	if reason := s.CleanupReason(paths[0]); !strings.Contains(reason, "运行") {
		t.Fatalf("active old executable was offered: %q", reason)
	}
	s.processProbe = func() (string, error) { return "", errors.New("test process refusal") }
	if reason := s.CleanupReason(paths[2]); !strings.Contains(reason, "未知") {
		t.Fatalf("unknown process state must preserve versions: %q", reason)
	}
	s.processProbe = func() (string, error) { return "launchd\n", nil }
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(paths[0], "payload.bin"), launcher); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOLE_STUDIO_TRASH_DIR", t.TempDir())
	t.Setenv("MOLE_STUDIO_STATE_DIR", t.TempDir())
	outcome := s.TrashPaths([]string{paths[0], root})
	if len(outcome.Moved) != 0 || len(outcome.Skipped) != 2 || !exists(paths[0]) {
		t.Fatalf("a launcher change after preview must invalidate deletion permission: %+v", outcome)
	}
}

func TestCleanCatalogUsesPreciseSizesAndRealFileFacts(t *testing.T) {
	home := fixtureHome(t)
	path := fixtureDir(t, home, "Library/Caches/UnknownCache", 1234)
	s := &Scanner{home: home}
	row := rowsByID(s.scanCleanGroups())["user_caches"]
	if row == nil || row["tier"] != "review" || row["checked"] != false || !strings.Contains(row["filesCount"].(string), "1 个文件") || row["lastMod"] == "本机扫描" {
		t.Fatalf("generic cache is an unchecked inspection with actual facts: %v", row)
	}
	info, err := os.Stat(filepath.Join(path, "payload.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if row["sizeGb"] != plainGB(allocatedSize(info)) || row["sizeGb"].(float64) >= 0.01 || row["subpaths"].([]map[string]any)[0]["path"] != "~/Library/Caches/UnknownCache" {
		t.Fatalf("small cache must not be inflated or authorize its parent: %v", row)
	}
}

func TestAIWorktreesAreReadOnlyEvenWithUncommittedContent(t *testing.T) {
	home := fixtureHome(t)
	repo := fixtureDir(t, home, ".codex/worktrees/task/project", 4096)
	if output, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git fixture init: %s, %v", output, err)
	}
	if output, err := exec.Command("git", "-C", repo, "add", "payload.bin").CombinedOutput(); err != nil {
		t.Fatalf("git fixture add: %s, %v", output, err)
	}
	index := filepath.Join(repo, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	rows := (&Scanner{home: home}).scanAIWorktrees(&catalogClaims{})
	if len(rows) != 1 || rows[0]["tier"] != "protected" || rows[0]["checked"] != false || !strings.Contains(rows[0]["gitState"].(string), "未提交") {
		t.Fatalf("worktree state must not authorize its removal: %v", rows)
	}
	after, err := os.ReadFile(index)
	if err != nil || string(after) != string(before) {
		t.Fatal("read-only worktree inspection changed the Git index")
	}
}

// writeWhitelist writes the user whitelist fixture, one entry per line.
func writeWhitelist(t *testing.T, home string, lines ...string) {
	t.Helper()
	path := filepath.Join(home, ".config", "mole", "whitelist")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// fixtureDir creates dir under home and fills it with a file of size bytes.
func fixtureDir(t *testing.T, home, dir string, size int) string {
	t.Helper()
	path := filepath.Join(home, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	writeTestFile(t, filepath.Join(path, "payload.bin"), size)
	return path
}

// rowsByID indexes payload rows by their id.
func rowsByID(rows []map[string]any) map[string]map[string]any {
	index := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		index[id] = row
	}
	return index
}

// subpathPaths returns the path field of every subpath of a row.
func subpathPaths(row map[string]any) []string {
	subpaths, _ := row["subpaths"].([]map[string]any)
	paths := make([]string, 0, len(subpaths))
	for _, subpath := range subpaths {
		path, _ := subpath["path"].(string)
		paths = append(paths, path)
	}
	return paths
}

// TestCleanGroupsReportEachRootOnce covers S-08: a cache group nested inside
// ~/Library/Caches must not be counted a second time by the parent row, and a
// review-tier row must not be pre-checked.
func TestCleanGroupsReportEachRootOnce(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Caches/FooCache", 3<<20)
	fixtureDir(t, home, "Library/Caches/Google", 2<<20)
	fixtureDir(t, home, "Library/Caches/com.apple.Safari", 1<<20)
	fixtureDir(t, home, "Library/Caches/pip", 1<<20)
	fixtureDir(t, home, ".npm/_cacache", 2<<20)

	groups := (&Scanner{home: home}).scanCleanGroups()
	byID := rowsByID(groups)

	userCaches, ok := byID["user_caches"]
	if !ok {
		t.Fatalf("user_caches row missing, got ids %v", rowIDs(groups))
	}
	// The default whitelist protects ~/Library/Caches/JetBrains*, so the root
	// is only protected through a child and must be reported child by child.
	for _, path := range subpathPaths(userCaches) {
		if strings.Contains(path, "Google") || strings.Contains(path, "Safari") || strings.Contains(path, "pip") {
			t.Errorf("user_caches reports %q, which another row already reports", path)
		}
	}
	if !strings.Contains(strings.Join(subpathPaths(userCaches), ","), "FooCache") {
		t.Errorf("user_caches lost its unprotected child: %v", subpathPaths(userCaches))
	}
	if checked, _ := userCaches["checked"].(bool); checked {
		t.Error("generic user caches require review, not directory-wide default selection")
	}
	for _, subpath := range userCaches["subpaths"].([]map[string]any) {
		if checked, _ := subpath["checked"].(bool); checked {
			t.Errorf("unclassified subpath %v must not be pre-checked", subpath["path"])
		}
	}

	browser, ok := byID["browser_caches"]
	if !ok {
		t.Fatalf("browser_caches row missing, got ids %v", rowIDs(groups))
	}
	joined := strings.Join(subpathPaths(browser), ",")
	if !strings.Contains(joined, "Google") || !strings.Contains(joined, "com.apple.Safari") {
		t.Errorf("browser_caches must still report both browser caches, got %v", subpathPaths(browser))
	}

	pkg, ok := byID["package_caches"]
	if !ok {
		t.Fatalf("package_caches row missing, got ids %v", rowIDs(groups))
	}
	if checked, _ := pkg["checked"].(bool); checked {
		t.Error("review-tier package_caches must not be pre-checked (S-09)")
	}
	for _, subpath := range pkg["subpaths"].([]map[string]any) {
		if checked, _ := subpath["checked"].(bool); checked {
			t.Errorf("review-tier subpath %v must not be pre-checked", subpath["path"])
		}
	}

	// No directory may appear in two rows.
	seen := map[string]string{}
	for _, group := range groups {
		for _, path := range subpathPaths(group) {
			if other, duplicate := seen[path]; duplicate {
				t.Errorf("subpath %q reported by both %s and %v", path, other, group["id"])
			}
			seen[path] = group["id"].(string)
		}
	}
}

// rowIDs lists the ids of payload rows.
func rowIDs(rows []map[string]any) []any {
	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"])
	}
	return ids
}

// TestCleanGroupsRespectWhitelist covers S-13 on the scanner side: composite
// "a & b" lines protect both paths, and an exactly protected root disappears.
func TestCleanGroupsRespectWhitelist(t *testing.T) {
	t.Run("composite", func(t *testing.T) {
		home := fixtureHome(t)
		writeWhitelist(t, home, "~/Library/Caches/Google & ~/Library/Caches/pip")
		fixtureDir(t, home, "Library/Caches/FooCache", 2<<20)
		fixtureDir(t, home, "Library/Caches/Google", 2<<20)
		fixtureDir(t, home, "Library/Caches/pip", 1<<20)

		groups := (&Scanner{home: home}).scanCleanGroups()
		for _, group := range groups {
			for _, path := range subpathPaths(group) {
				if strings.Contains(path, "Google") || strings.Contains(path, "pip") {
					t.Errorf("composite whitelist entry did not protect %q (row %v)", path, group["id"])
				}
			}
		}
	})

	t.Run("exact-root", func(t *testing.T) {
		home := fixtureHome(t)
		writeWhitelist(t, home, "~/Library/Caches", "~/Library/Logs")
		fixtureDir(t, home, "Library/Caches/FooCache", 2<<20)
		fixtureDir(t, home, "Library/Caches/Google", 2<<20)
		fixtureDir(t, home, "Library/Logs/App", 1<<20)

		if groups := (&Scanner{home: home}).scanCleanGroups(); len(groups) != 0 {
			t.Errorf("exactly whitelisted roots must not be offered, got %v", rowIDs(groups))
		}
	})
}

// TestOrphanContainersAreCandidatesOnly covers S-06: a directory without a
// signature match is shown unchecked, macOS data such as MobileSync is not
// reported at all, and ids are unique per root.
func TestOrphanContainersAreCandidatesOnly(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Application Support/MobileSync", 16<<20)
	fixtureDir(t, home, "Library/Containers/com.example.gone", 16<<20)
	fixtureDir(t, home, "Library/Group Containers/com.example.gone", 16<<20)
	fixtureDir(t, home, "Library/Application Support/SomeGoneApp", 16<<20)

	rows := (&Scanner{home: home}).scanOrphanedLeftovers()
	ids := map[string]struct{}{}
	for _, row := range rows {
		id := row["id"].(string)
		if _, duplicate := ids[id]; duplicate {
			t.Errorf("duplicate orphan id %q", id)
		}
		ids[id] = struct{}{}
		name := row["name"].(string)
		if strings.Contains(strings.ToLower(name), "mobilesync") {
			t.Errorf("system MobileSync data reported as an orphan: %v", row["id"])
		}
		if strings.HasPrefix(id, "orphan_") {
			if checked, _ := row["checked"].(bool); checked {
				t.Errorf("unsigned container %q must not be pre-checked", id)
			}
			for _, subpath := range row["subpaths"].([]map[string]any) {
				if checked, _ := subpath["checked"].(bool); checked {
					t.Errorf("unsigned container subpath %q must not be pre-checked", subpath["path"])
				}
			}
			if !strings.Contains(id, "containers_") && !strings.Contains(id, "group_containers_") &&
				!strings.Contains(id, "application_support_") && !strings.Contains(id, "caches_") {
				t.Errorf("orphan id %q does not name its root", id)
			}
		}
	}
	if len(rows) != 3 {
		t.Errorf("want 3 unsigned container rows, got %d: %v", len(rows), rows)
	}
}

// TestInstallerArchivesAreNotPrechecked covers S-07: only installers are
// listed, and a zip is never pre-checked.
func TestInstallerArchivesAreNotPrechecked(t *testing.T) {
	home := fixtureHome(t)
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("MkdirAll(Downloads): %v", err)
	}
	writeTestFile(t, filepath.Join(downloads, "family-photos.zip"), 21<<20)
	writeTestFile(t, filepath.Join(downloads, "Xcode_15.xip"), 22<<20)
	writeTestFile(t, filepath.Join(downloads, "macOS_Sequoia_Installer.zip"), 23<<20)

	items := (&Scanner{home: home}).scanInstallerPackages()
	names := map[string]map[string]any{}
	for _, item := range items {
		names[item["name"].(string)] = item
	}
	if _, listed := names["family-photos.zip"]; listed {
		t.Error("a zip whose name is not an installer name must not be listed")
	}
	for _, name := range []string{"Xcode_15.xip", "macOS_Sequoia_Installer.zip"} {
		item, listed := names[name]
		if !listed {
			t.Errorf("%s should be listed, got %v", name, names)
			continue
		}
		checked, _ := item["checked"].(bool)
		if name == "macOS_Sequoia_Installer.zip" && checked {
			t.Errorf("installer zip %s must not be pre-checked", name)
		}
	}
	if isInstaller, _ := isInstallerFile("project-backup.zip"); isInstaller {
		t.Error("project-backup.zip must not be classified as an installer")
	}
}

// TestInstalledAppIndexCoversSystemAppsAndHomebrew covers the S-06 index fix.
func TestInstalledAppIndexCoversSystemAppsAndHomebrew(t *testing.T) {
	s := &Scanner{home: t.TempDir()}
	names, bundleIDs := s.installedAppIndex(s.home)
	for _, path := range []string{"/System/Applications/Calculator.app"} {
		if !exists(path) {
			continue
		}
		if _, ok := names["calculator"]; !ok {
			t.Error("installedAppIndex must include /System/Applications apps")
		}
	}
	if exists("/opt/homebrew/bin/7z") {
		if _, ok := names["7z"]; !ok {
			t.Error("installedAppIndex must include Homebrew binaries")
		}
	}
	if exists("/System/Applications/Calculator.app") {
		if _, ok := bundleIDs["com.apple.calculator"]; !ok {
			t.Errorf("installedAppIndex must include system bundle ids, got %d ids", len(bundleIDs))
		}
	}
	if s.matchesInstalled("calculator", names, bundleIDs) != true {
		t.Error("an installed app's support folder must match")
	}
	if s.matchesInstalled("fsCachedData", names, bundleIDs) {
		t.Error("a name that merely contains a known word must not match (S-06)")
	}
}

// TestBinaryPlistBundleInfo covers S-10 on a real system app.
func TestBinaryPlistBundleInfo(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("system apps only exist on macOS")
	}
	const app = "/Applications/GarageBand.app"
	if !exists(app) {
		t.Skip("/Applications/GarageBand.app is not installed")
	}
	info := readBundleInfo(app)
	if info.identifier != "com.apple.garageband10" {
		t.Errorf("GarageBand identifier = %q, want com.apple.garageband10 (binary plist fallback)", info.identifier)
	}
	if info.shortVersion != "10.4.14" {
		t.Errorf("GarageBand version = %q, want 10.4.14", info.shortVersion)
	}
}

// TestGarageBandBundleSizeMatchesDu covers S-04: the bundle budget no longer
// runs out on directories, so the measured size matches du within 20%.
// Want comes from `du -sxk /Applications/GarageBand.app` = 1099628 KB.
func TestGarageBandBundleSizeMatchesDu(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only meaningful on macOS")
	}
	const app = "/Applications/GarageBand.app"
	if !exists(app) {
		t.Skip("/Applications/GarageBand.app is not installed")
	}
	const want = 1_099_628 * 1024
	got := Shared.fastDirectorySize(app, appBundleFileCap)
	if got.bytes == 0 {
		t.Fatal("measured zero bytes")
	}
	if math.Abs(float64(got.bytes)-want)/want > 0.20 {
		t.Errorf("GarageBand size = %.3f GB, du says %.3f GB (more than 20%% off)",
			float64(got.bytes)/1e9, float64(want)/1e9)
	}
}

// TestInstalledAppRowsUseMeasuredLeftovers covers the S-09 apps fix: the
// cache/config split is the measured sum, not an 80/20 guess, and app sizes are
// not floored.
func TestInstalledAppRowsUseMeasuredLeftovers(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only meaningful on macOS")
	}
	rows := Shared.scanInstalledApps()
	if len(rows) == 0 {
		t.Skip("no apps scanned in this environment")
	}
	for _, row := range rows {
		if checked, _ := row["checked"].(bool); checked {
			t.Errorf("%v: installed apps must not be pre-checked", row["name"])
		}
		cache := floatOf(row["cacheSizeGb"])
		config := floatOf(row["configSizeGb"])
		leftover := floatOf(row["leftoverSizeGb"])
		if math.Abs(cache+config-leftover) > 1e-9 {
			t.Errorf("%v: cache %v + config %v != leftover %v", row["name"], cache, config, leftover)
		}
		if appSize := floatOf(row["appSizeGb"]); appSize > 0 && appSize == 0.05 {
			t.Errorf("%v: appSizeGb is the old 0.05 GB floor", row["name"])
		}
	}
}

// TestAppendExistingMeasuresEachKind covers the uninstall sizing helpers.
func TestAppendExistingMeasuresEachKind(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, "cache")
	config := filepath.Join(home, "config")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(cache, "a.bin"), 1<<20)
	writeTestFile(t, filepath.Join(config, "b.bin"), 2<<20)
	s := &Scanner{home: home}
	var subpaths []map[string]any
	cacheBytes := s.appendExisting([]string{cache}, "cache", true, &subpaths)
	configBytes := s.appendExisting([]string{config}, "config", false, &subpaths)
	if cacheBytes < 1<<20 || configBytes < 2<<20 {
		t.Errorf("measured cache=%d config=%d, want at least 1MiB/2MiB", cacheBytes, configBytes)
	}
	if len(subpaths) != 2 {
		t.Fatalf("want 2 subpaths, got %d", len(subpaths))
	}
	if subpaths[0]["kind"] != "cache" || subpaths[1]["kind"] != "config" {
		t.Errorf("unexpected subpath kinds: %v %v", subpaths[0]["kind"], subpaths[1]["kind"])
	}
}
