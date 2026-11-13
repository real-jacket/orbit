package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gapScanner is a Mole test scanner with a quiet Spotlight, so no test asks the real one.
func gapScanner(t *testing.T, processes string) *Scanner {
	s := moleTestScanner(t, processes)
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) { return "", nil }
	return s
}

// installApp makes the app count as installed under the test home.
func installApp(t *testing.T, s *Scanner, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(s.home, "Applications", name+".app", "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func lockTo(t *testing.T, dir string, pid int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("host-"+strconv.Itoa(pid), filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
}

func exitedPid(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestGapFirmwareIsSafeAndKeepsOpenFiles(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	iphone := filepath.Join(s.home, "Library/iTunes/iPhone Software Updates/a.ipsw")
	nested := filepath.Join(s.home, "Library/Group Containers/ABC.group.com.apple.configurator/Library/Caches/Firmware/b.ipsw")
	busy := filepath.Join(s.home, "Library/iTunes/iPad Software Updates/busy.ipsw")
	for _, p := range []string{iphone, nested, busy, filepath.Join(s.home, "Library/iTunes/iPhone Software Updates/partial.ipsw.download"),
		filepath.Join(s.home, "Library/iTunes/Other Software Updates/c.ipsw")} {
		writeFile(t, p, "firmware")
	}
	if err := os.MkdirAll(filepath.Join(s.home, "Library/iTunes/iPod Software Updates/dir.ipsw"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.handleProbe = func(p string) (bool, error) { return p == busy, nil }
	items, _ := moleSubpaths(t, s)
	for _, p := range []string{iphone, nested} {
		got := items[s.relativize(p)]
		if got == nil || got["tier"] != "safe" || got["checked"] != true {
			t.Errorf("%s should be a checked safe row: %v", p, got)
		}
	}
	if got := items[s.relativize(busy)]; got == nil || got["tier"] != "protected" || !strings.Contains(got["reason"].(string), "打开") {
		t.Errorf("an open firmware file stays: %v", got)
	}
	for _, p := range []string{"Library/iTunes/iPhone Software Updates/partial.ipsw.download", "Library/iTunes/Other Software Updates/c.ipsw", "Library/iTunes/iPod Software Updates/dir.ipsw"} {
		if _, ok := items["~/"+p]; ok {
			t.Errorf("%s must not be offered", p)
		}
	}
	// The move boundary asks lsof again, and a probe failure keeps the file.
	if reason := s.CleanupReason(busy); !strings.Contains(reason, "打开") {
		t.Errorf("boundary: %q", reason)
	}
	s.handleProbe = func(string) (bool, error) { return false, os.ErrInvalid }
	if reason := s.CleanupReason(iphone); reason == "" {
		t.Error("an unknown handle state keeps the file")
	}
	s.handleProbe = func(string) (bool, error) { return false, nil }
	if out := s.TrashPaths([]string{iphone}); len(out.Moved) != 1 {
		t.Fatalf("firmware moves to the Trash: %+v", out)
	}
}

func TestGapAppSupportSweep(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	support := filepath.Join(s.home, "Library/Application Support")
	mk := func(app string, subs ...string) {
		for _, sub := range subs {
			writeFile(t, filepath.Join(support, app, sub, "blob"), "data")
		}
		installApp(t, s, app)
	}
	mk("Zeta", "Code Cache", "GPUCache", "Crashpad/completed", "Cache")
	mk("Plain", "Cache", "CachedData") // no Chromium marker, so Cache is not a cache folder
	mk("Settings Helper", "GPUCache")  // critical component name
	mk("Dead", "GPUCache")
	mk("Live", "GPUCache")
	mk("Keeper", "GPUCache")
	mk("Wanted", "GPUCache")
	mk("com.microsoft.Edge", "GPUCache") // protected data name
	writeFile(t, filepath.Join(support, "Zeta/Preferences"), "keep")
	lockTo(t, filepath.Join(support, "Dead"), exitedPid(t))
	lockTo(t, filepath.Join(support, "Live"), os.Getpid())
	writeWhitelist(t, s.home, "~/Library/Application Support/Keeper")
	s.refreshWhitelist()
	items, _ := moleSubpaths(t, s)

	for _, p := range []string{"Zeta/Code Cache", "Zeta/GPUCache", "Zeta/Crashpad/completed", "Zeta/Cache", "Dead/GPUCache", "Wanted/GPUCache"} {
		got := items["~/Library/Application Support/"+p]
		if got == nil || got["tier"] != "safe" || got["checked"] != true || got["reason"] == "" {
			t.Errorf("%s should be a safe row: %v", p, got)
		}
	}
	if got := items["~/Library/Application Support/Zeta/Crashpad/completed"]; got != nil && !strings.Contains(got["reason"].(string), "崩溃报告") {
		t.Errorf("crash reports say so: %v", got["reason"])
	}
	for _, p := range []string{"Plain/Cache", "Plain/CachedData", "Settings Helper/GPUCache", "Live/GPUCache", "Keeper/GPUCache", "com.microsoft.Edge/GPUCache", "Zeta/Preferences"} {
		if got, ok := items["~/Library/Application Support/"+p]; ok && got["tier"] == "safe" {
			t.Errorf("%s must not be offered as safe: %v", p, got)
		}
	}
	// The lock is read again at the move, so an app that starts afterwards keeps its caches.
	lockTo(t, filepath.Join(support, "Wanted"), os.Getpid())
	if reason := s.CleanupReason(filepath.Join(support, "Wanted/GPUCache")); !strings.Contains(reason, "Wanted") || unlockHint(reason) == "" {
		t.Errorf("boundary: %q", reason)
	}
}

func TestGapSweepLeavesUninstalledAppsToTheOrphanList(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	support := filepath.Join(s.home, "Library/Application Support")
	writeFile(t, filepath.Join(support, "Ghostly/GPUCache/blob"), strings.Repeat("x", 16_000_000))
	items, _ := moleSubpaths(t, s)
	if _, ok := items["~/Library/Application Support/Ghostly/GPUCache"]; ok {
		t.Fatal("an app with no owner is the orphan list's")
	}
	names, ids := s.installedAppIndex(s.home)
	orphans := s.scanUnclaimedContainers(s.home, map[string]struct{}{}, names, ids, map[string]struct{}{})
	var found bool
	for _, row := range orphans {
		for _, sub := range row["subpaths"].([]map[string]any) {
			found = found || strings.HasSuffix(sub["path"].(string), "Application Support/Ghostly")
			for path := range items {
				if sub["path"] == path || strings.HasPrefix(path, sub["path"].(string)+"/") {
					t.Errorf("%s is in both sections", path)
				}
			}
		}
	}
	if !found {
		t.Fatalf("the orphan list keeps its row: %v", orphans)
	}
}

func TestGapContainerStubs(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	root := filepath.Join(s.home, "Library/Containers")
	plist := ".com.apple.containermanagerd.metadata.plist"
	stub := func(name string) string {
		writeFile(t, filepath.Join(root, name, plist), "meta")
		return "~/Library/Containers/" + name
	}
	bare := stub("com.macpaw.CleanMyMac-mas")
	team := stub("S8EX82NJP6.com.macpaw.CleanMyMac5.Agent")
	other := stub("com.example.NotInTheList")
	withData := stub("com.macpaw.CleanMyMac5.Menu")
	writeFile(t, filepath.Join(root, "com.macpaw.CleanMyMac5.Menu/Data/file"), "user data")
	extra := stub("com.macpaw.CleanMyMac.Extra")
	writeFile(t, filepath.Join(root, "com.macpaw.CleanMyMac.Extra/note.txt"), "x")
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "com.macpaw.CleanMyMac.Link")); err != nil {
		t.Fatal(err)
	}
	items, _ := moleSubpaths(t, s)
	for _, p := range []string{bare, team} {
		if got := items[p]; got == nil || got["tier"] != "safe" || got["checked"] != true {
			t.Errorf("%s is an orphaned stub: %v", p, got)
		}
	}
	for _, p := range []string{other, withData, extra, "~/Library/Containers/com.macpaw.CleanMyMac.Link"} {
		if _, ok := items[p]; ok {
			t.Errorf("%s is not a stub of the list", p)
		}
	}
	// Spotlight finding the app, or an app in a known folder, keeps the stub.
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) {
		return "/Applications/CleanMyMac.app\n", nil
	}
	if items, _ = moleSubpaths(t, s); items[bare] != nil {
		t.Error("an installed app keeps its container")
	}
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) { return "", context.DeadlineExceeded }
	if items, _ = moleSubpaths(t, s); items[bare] != nil {
		t.Error("an unreadable Spotlight keeps the container")
	}
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) { return "", nil }
	if err := os.MkdirAll(filepath.Join(s.home, "Applications/CleanMyMac X.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if items, _ = moleSubpaths(t, s); items[bare] != nil {
		t.Error("an app in ~/Applications keeps its container")
	}
}

// ageMailFolder backdates a folder and its file; on macOS an earlier mtime lowers the creation time too.
func ageMailFolder(t *testing.T, folder, file string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	for _, p := range []string{file, folder} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGapMailDownloads(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the guard needs the creation time")
	}
	s := gapScanner(t, "/usr/bin/idle\n")
	oldMin := mailDownloadsMinBytes
	mailDownloadsMinBytes = 1
	t.Cleanup(func() { mailDownloadsMinBytes = oldMin })
	box := filepath.Join(s.home, "Library/Containers/com.apple.mail/Data/Library/Mail Downloads")
	legacy := filepath.Join(s.home, "Library/Mail Downloads")
	add := func(root, folder, name string, age time.Duration) string {
		file := filepath.Join(root, folder, name)
		writeFile(t, file, "attachment bytes")
		ageMailFolder(t, filepath.Dir(file), file, age)
		return file
	}
	day := 24 * time.Hour
	old := add(box, "A1", "report.pdf", 40*day)
	young := add(box, "A2", "new.pdf", 2*day)
	installer := add(box, "A3", "tool.dmg", 40*day)
	legacyInstaller := add(legacy, "B1", "tool.dmg", 40*day)
	hidden := add(box, "A4", ".hidden", 40*day)
	touched := add(box, "A5", "edited.pdf", 40*day)
	if err := os.Chtimes(touched, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	ageMailFolder(t, filepath.Dir(touched), touched, 40*day)
	if err := os.Chtimes(touched, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	linked := add(box, "A6", "linked.pdf", 40*day)
	if err := os.Link(linked, filepath.Join(s.home, "elsewhere.pdf")); err != nil {
		t.Fatal(err)
	}
	deep := add(box, "A7/inner", "deep.pdf", 40*day)

	items, groups := moleSubpaths(t, s)
	got := items[s.relativize(old)]
	if got == nil || got["tier"] != "review" || got["checked"] == true || got["reason"] == "" {
		t.Fatalf("an old untouched copy is offered for review: %v", got)
	}
	if got = items[s.relativize(legacyInstaller)]; got == nil {
		t.Error("the legacy folder is not on the installer list, so its files stay here")
	}
	for _, p := range []string{young, installer, hidden, touched, linked, deep} {
		if _, ok := items[s.relativize(p)]; ok {
			t.Errorf("%s must not be offered", p)
		}
	}
	var mail map[string]any
	for _, g := range groups {
		if g["id"] == "mail_downloads" {
			mail = g
		}
	}
	if mail == nil || mail["tier"] != "review" {
		t.Fatalf("the group is review: %v", mail)
	}
	// Mail running keeps its downloads, with an unlock hint.
	s2 := gapScanner(t, "/System/Applications/Mail.app/Contents/MacOS/Mail\n")
	s2.home = s.home
	items, _ = moleSubpaths(t, s2)
	if got = items[s.relativize(old)]; got == nil || got["tier"] != "protected" || got["unlock"] == "" {
		t.Errorf("a running Mail protects its downloads: %v", got)
	}
	// A folder under the size floor is left alone.
	mailDownloadsMinBytes = oldMin
	if items, _ = moleSubpaths(t, s); items[s.relativize(old)] != nil {
		t.Error("a small Mail Downloads folder is skipped")
	}
}

func TestGapRecentItemsGoToTheTrash(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	list := filepath.Join(s.home, "Library/Application Support/com.apple.sharedfilelist/com.apple.LSSharedFileList.RecentDocuments.sfl2")
	prefs := filepath.Join(s.home, "Library/Preferences/com.apple.recentitems.plist")
	other := filepath.Join(s.home, "Library/Application Support/com.apple.sharedfilelist/com.apple.LSSharedFileList.FavoriteItems.sfl2")
	for _, p := range []string{list, prefs, other} {
		writeFile(t, p, "list")
	}
	items, _ := moleSubpaths(t, s)
	for _, p := range []string{list, prefs} {
		got := items[s.relativize(p)]
		if got == nil || got["tier"] != "review" || got["checked"] == true || got["reason"] == "" {
			t.Errorf("%s is a review row: %v", p, got)
		}
	}
	if _, ok := items[s.relativize(other)]; ok {
		t.Error("the Finder sidebar favourites are not recent items")
	}
	out := s.TrashPaths([]string{list, prefs})
	if len(out.Moved) != 2 {
		t.Fatalf("recent items move to the Trash with a restore record: %+v", out)
	}
	if _, err := os.Stat(list); !os.IsNotExist(err) {
		t.Error("the list is gone from its place")
	}
}

const shutDownDevice = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>isDeleted</key><false/><key>state</key><integer>1</integer></dict></plist>`

func TestSimulatorLeavesNeedAShutDownDevice(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	devices := filepath.Join(s.home, simulatorDevices)
	off, on := filepath.Join(devices, "OFF"), filepath.Join(devices, "ON")
	writeFile(t, filepath.Join(off, "device.plist"), shutDownDevice)
	writeFile(t, filepath.Join(on, "device.plist"), strings.Replace(shutDownDevice, "<integer>1</integer>", "<integer>3</integer>", 1))
	for _, device := range []string{off, on} {
		writeFile(t, filepath.Join(device, "data/var/db/diagnostics/log.tracev3"), "log")
		writeFile(t, filepath.Join(device, "data/private/var/MobileAsset/AssetsV2/model.bin"), "asset")
		writeFile(t, filepath.Join(device, "data/Containers/app.db"), "user data")
	}
	var paths []string
	for _, hit := range append(simulatorLeaves("data/var/db/diagnostics")(s), simulatorLeaves("data/private/var/MobileAsset/AssetsV2")(s)...) {
		paths = append(paths, hit.path)
	}
	if len(paths) != 2 || !strings.HasPrefix(paths[0], off) || !strings.HasPrefix(paths[1], off) {
		t.Fatalf("only the shut-down device's leaves may be listed, got %v", paths)
	}
	leaf := filepath.Join(off, "data/var/db/diagnostics")
	if reason := s.cleanupReason(leaf, true); reason != "" {
		t.Fatalf("shut-down leaf refused: %s", reason)
	}
	if reason := s.cleanupReason(filepath.Join(on, "data/var/db/diagnostics"), true); !strings.Contains(reason, "没有关机") {
		t.Fatalf("booted device's leaf must stay, got %q", reason)
	}
	// A device booted through simctl has no Simulator.app process, only launchd_sim.
	booted := gapScanner(t, "/usr/bin/idle\nlaunchd_sim\n")
	booted.home = s.home
	if reason := booted.cleanupReason(leaf, true); reason != "iOS 模拟器 正在运行，暂缓清理" {
		t.Fatalf("a booted simulator must protect device data, got %q", reason)
	}
}

func TestSelectedWallpaperAndSpotlightIndexStay(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	for _, rel := range []string{"Library/Application Support/com.apple.wallpaper/aerials/videos/a.mov", "Library/Metadata/CoreSpotlight/index.db",
		"Library/Caches/ms-playwright/chromium/chrome", "Library/Caches/Cypress/13/Cypress.app/x", "Library/Caches/GIPPseudonymousID/id"} {
		p := filepath.Join(s.home, rel)
		writeFile(t, p, "data")
		for dir := filepath.Dir(p); dir != filepath.Join(s.home, "Library/Application Support") && dir != filepath.Join(s.home, "Library/Metadata") && dir != filepath.Join(s.home, "Library/Caches"); dir = filepath.Dir(dir) {
			if s.cleanupReason(dir, true) == "" {
				t.Fatalf("%s must be protected", dir)
			}
		}
	}
}

func TestMeasurementAndTrashStopAtMountedVolumes(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	dir := filepath.Join(s.home, "Library/Developer/CoreDevice")
	writeFile(t, filepath.Join(dir, "Devices/db.sqlite"), "own")
	writeFile(t, filepath.Join(dir, "DeviceFS/big.bin"), strings.Repeat("x", 1<<20))
	saved := mountPoints
	t.Cleanup(func() { mountPoints = saved })
	mountPoints = func() []string { return []string{"/", filepath.Join(dir, "DeviceFS")} }
	if reason := s.cleanupReason(dir, true); !strings.Contains(reason, "挂载着另一个卷") {
		t.Fatalf("a folder holding a mounted volume must not move, got %q", reason)
	}
	if reason := s.cleanupReason(filepath.Join(dir, "DeviceFS"), true); !strings.Contains(reason, "挂载点") {
		t.Fatalf("a mount point must not move, got %q", reason)
	}
	if reason := s.cleanupReason(filepath.Join(dir, "Devices"), true); reason != "" {
		t.Fatalf("a sibling on this volume is unaffected, got %q", reason)
	}
	// The walk drops entries whose device differs from the root's.
	total := dirSize{dev: 1}
	run := &walkRun{total: &total}
	run.total.count = 0
	items := []dirItem{{name: "a", kind: itemFile, bytes: 10, dev: 1}, {name: "b", kind: itemFile, bytes: 99, dev: 2}}
	for _, item := range items {
		if total.dev != 0 && item.dev != 0 && uint64(uint32(item.dev)) != total.dev {
			continue
		}
		run.addFile(item)
	}
	if total.bytes != 10 {
		t.Fatalf("bytes of another volume were counted: %d", total.bytes)
	}
}

func TestPurgeSuggestsUnscannedProjectFolders(t *testing.T) {
	s := gapScanner(t, "/usr/bin/idle\n")
	writeFile(t, filepath.Join(s.home, "code/demo/app/package.json"), "{}")
	writeFile(t, filepath.Join(s.home, "notes/todo.txt"), "no project")
	writeFile(t, filepath.Join(s.home, "Documents/proj/package.json"), "{}")
	writeFile(t, filepath.Join(s.home, "Projects/x/go.mod"), "module x")
	writeFile(t, filepath.Join(s.home, ".config/mole/purge_paths"), "~/Projects\n")
	roots := s.PurgeRoots()
	want := filepath.Join(s.home, "code")
	if len(roots.Suggested) != 1 || !strings.EqualFold(roots.Suggested[0], want) {
		t.Fatalf("suggested = %v, want only %s", roots.Suggested, want)
	}
	if len(roots.Paths) != 1 {
		t.Fatalf("a suggestion must not change the scope: %v", roots.Paths)
	}
}
