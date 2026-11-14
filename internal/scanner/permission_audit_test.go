package scanner

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func restrictFixture(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the fixture's access restrictions")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}

func requirePermissionWarning(t *testing.T, home, path string) {
	t.Helper()
	for _, warning := range scanReadWarnings(home) {
		if warning["path"] == path && warning["kind"] == warnPermission {
			return
		}
	}
	t.Fatalf("access denial at %s was hidden: %v", path, scanReadWarnings(home))
}

func TestPermissionAuditDeniedDiscoveryIsNotMissing(t *testing.T) {
	for _, name := range []string{"AI cache", "support cache leaf", "sandbox cache", "launch items", "login helpers", "application inventory", "bundle metadata"} {
		t.Run(name, func(t *testing.T) {
			s, apps := ownerScanner(t)
			t.Cleanup(func() { clearReadWarnings(s.home) })
			var denied string
			switch name {
			case "AI cache":
				denied = fixtureDir(t, s.home, "Library/Application Support/Codex/Cache", 4096)
				restrictFixture(t, filepath.Dir(denied), 0)
				if row := s.catalog(CatalogSpec{ID: "ai", Roots: []string{denied}, Tier: "safe"}, &catalogClaims{}, nil); row != nil {
					t.Fatalf("unreadable cache was offered: %v", row)
				}
			case "support cache leaf":
				denied = fixtureDir(t, s.home, "Library/Application Support/Example/Service Worker/ScriptCache", 4096)
				restrictFixture(t, filepath.Dir(denied), 0)
				appSupportCacheCandidates(filepath.Join(s.home, "Library/Application Support/Example"), false)
			case "sandbox cache":
				denied = fixtureDir(t, s.home, "Library/Containers/com.example/Data/Library/Caches", 4096)
				restrictFixture(t, filepath.Join(s.home, "Library/Containers/com.example"), 0)
				s.appCacheCandidates("Example", "com.example")
			case "launch items":
				denied = filepath.Join(s.home, "Library/LaunchAgents")
				writeFile(t, filepath.Join(denied, "com.example.plist"), "fixture")
				restrictFixture(t, denied, 0)
				s.loadLaunchItems()
			case "login helpers":
				app := filepath.Join(apps, "Example.app")
				denied = filepath.Join(app, "Contents/Library/LoginItems")
				writeFile(t, filepath.Join(denied, "Helper.app/Contents/Info.plist"), cursorPlist)
				restrictFixture(t, denied, 0)
				loginItemHelperIDs(app)
			case "application inventory":
				denied = apps
				writeFile(t, filepath.Join(apps, "Example.app/Contents/Info.plist"), cursorPlist)
				restrictFixture(t, apps, 0)
				findAppBundles([]string{apps})
			case "bundle metadata":
				denied = filepath.Join(apps, "Example.app/Contents/Info.plist")
				writeFile(t, denied, cursorPlist)
				restrictFixture(t, denied, 0)
				readBundleInfo(filepath.Dir(filepath.Dir(denied)))
			}
			requirePermissionWarning(t, s.home, denied)
		})
	}
}

func TestPermissionAuditUnreadableLauncherKeepsOwner(t *testing.T) {
	for _, installation := range []struct{ launcher, parent string }{
		{".local/bin/copilot", ".local"},
		{".nvm/versions/node/v1/bin/copilot", ".nvm/versions/node"},
	} {
		t.Run(installation.launcher, func(t *testing.T) {
			s, _ := ownerScanner(t)
			t.Setenv("PATH", t.TempDir())
			t.Cleanup(func() { clearReadWarnings(s.home) })
			writeFile(t, filepath.Join(s.home, installation.launcher), "launcher")
			session := filepath.Join(s.home, ".copilot/session-state/session.json")
			writeFile(t, session, "personal data")
			restrictFixture(t, filepath.Join(s.home, installation.parent), 0)
			if s.ownerAbsent(personalOwners[".copilot"], nil) {
				t.Error("an unreadable launcher directory was treated as an absent owner")
			}
			if s.aiToolUninstalled("GitHub Copilot CLI") {
				t.Error("an unreadable installation was labeled uninstalled")
			}
			if reason := s.cleanupReason(session, true); reason == "" {
				t.Error("installation uncertainty authorized removing personal data")
			}
			warnings := s.scopedReadWarnings(PurgeRoots{})
			if len(warnings) == 0 || !slices.Contains(warnings[0]["modules"].([]string), "ai_moon") || !slices.Contains(warnings[0]["modules"].([]string), "software") {
				t.Fatalf("owner-check errors must reach both affected modules: %v", warnings)
			}
		})
	}
}

func TestPermissionAuditUnknownSiblingKeepsSharedDataAndServices(t *testing.T) {
	for _, target := range []string{"directory", "metadata"} {
		t.Run(target, func(t *testing.T) {
			s, app, _ := cursorFixture(t)
			t.Cleanup(func() { clearReadWarnings(s.home) })
			other := filepath.Join(s.home, "OtherApplications")
			plist := filepath.Join(other, "Renamed.app/Contents/Info.plist")
			writeFile(t, plist, cursorPlist)
			s.appDirs = append(s.appDirs, other)
			if target == "directory" {
				restrictFixture(t, other, 0)
			} else {
				restrictFixture(t, plist, 0)
			}
			selected := uninstallApp{path: app, bundleID: cursorBundleID, name: "Cursor"}
			plan := &uninstallPlan{s: s, apps: map[string]uninstallApp{app: selected}}
			if !plan.siblingInstalled(selected) {
				t.Error("an undecidable sibling check would stop a surviving install's helpers")
			}
			if s.ownerAbsent(cursorOwner, map[string]bool{app: true}) {
				t.Error("an unreadable sibling would release shared personal data")
			}
		})
	}
}

func TestPermissionAuditUnreadableClaudeInventoryKeepsVM(t *testing.T) {
	for _, target := range []string{"directory", "metadata"} {
		t.Run(target, func(t *testing.T) {
			s, apps := ownerScanner(t)
			t.Cleanup(func() { clearReadWarnings(s.home) })
			plist := filepath.Join(apps, "Renamed.app/Contents/Info.plist")
			vm := fixtureDir(t, s.home, claudeSupportDir+"/vm/old.bundle", 4096)
			old := time.Now().Add(-10 * 24 * time.Hour)
			if err := os.Chtimes(vm, old, old); err != nil {
				t.Fatal(err)
			}
			if reason := s.claudeVMReason(vm, false); reason != "" {
				t.Fatalf("isolated inventory has no installed owner: %q", reason)
			}
			writeFile(t, plist, `<plist><dict><key>CFBundleIdentifier</key><string>`+claudeBundleID+`</string></dict></plist>`)
			if target == "directory" {
				restrictFixture(t, apps, 0)
			} else {
				restrictFixture(t, plist, 0)
			}
			for _, live := range []bool{false, true} {
				if reason := s.cleanupReason(vm, live); reason == "" {
					t.Errorf("unreadable app inventory authorized removing its VM (live=%v)", live)
				}
			}
		})
	}
}

func TestPermissionAuditUnreadableSessionMetadataCannotExpire(t *testing.T) {
	s, _ := ownerScanner(t)
	t.Cleanup(func() { clearReadWarnings(s.home) })
	unit := filepath.Join(s.home, ".claude/todos/task")
	file := filepath.Join(unit, "recent.json")
	writeFile(t, file, "recent task")
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(unit, old, old); err != nil {
		t.Fatal(err)
	}
	restrictFixture(t, unit, 0o400)
	if _, err := os.Lstat(file); !os.IsPermission(err) {
		t.Fatalf("fixture must deny child metadata: %v", err)
	}
	if modified := aiNewestModified(unit); modified.Before(time.Now().Add(-aiSessionRetention)) {
		t.Fatalf("unverified contents made a recently edited task expire: %v", modified)
	}
	requirePermissionWarning(t, s.home, file)
}

func TestPermissionAuditUnreadableVersionMarkerIsDisclosed(t *testing.T) {
	s, _ := ownerScanner(t)
	t.Cleanup(func() { clearReadWarnings(s.home) })
	marker := filepath.Join(s.home, "active-marker")
	writeFile(t, marker, "1.0.0")
	writeFile(t, filepath.Join(s.home, "versions/1.0.0/program"), "program")
	restrictFixture(t, marker, 0)
	versions := s.versionInventory(aiVersionSpec{root: "versions", marker: "active-marker"}, false)
	if len(versions) != 1 || versions[0].reason == "" {
		t.Fatalf("unverified current version must remain protected: %v", versions)
	}
	requirePermissionWarning(t, s.home, marker)
}

func TestPermissionAuditUnreadableRunningAppMetadataKeepsData(t *testing.T) {
	for _, location := range []string{"Contents/Info.plist", "Wrapper/Renamed.app/Info.plist", "Wrapper"} {
		t.Run(location, func(t *testing.T) {
			s, apps := ownerScanner(t)
			t.Cleanup(func() { clearReadWarnings(s.home) })
			app := filepath.Join(apps, "Renamed.app")
			plist := filepath.Join(app, location)
			if location == "Wrapper" {
				plist = filepath.Join(app, "Wrapper/Renamed.app/Info.plist")
			}
			writeFile(t, plist, cursorPlist)
			cache := fixtureDir(t, s.home, "Library/Caches/"+cursorBundleID, 4096)
			s.processProbe = func() (string, error) { return app + "/Contents/MacOS/Cursor\n", nil }
			restrictFixture(t, filepath.Join(app, location), 0)
			for _, live := range []bool{false, true} {
				if reason := s.cleanupReason(cache, live); reason == "" {
					t.Errorf("unreadable metadata hid a running app's data owner (live=%v)", live)
				}
			}
			requirePermissionWarning(t, s.home, filepath.Join(app, location))
		})
	}
}

func TestPermissionAuditDeniedCacheDiscoveryHasNoZeroSizeClaim(t *testing.T) {
	s, apps := ownerScanner(t)
	t.Cleanup(func() { clearReadWarnings(s.home) })
	writeFile(t, filepath.Join(apps, "Example.app/Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.example</string></dict></plist>`)
	root := filepath.Join(s.home, "Library/Application Support/Example")
	writeFile(t, filepath.Join(root, "GPUCache/data"), "cache")
	restrictFixture(t, root, 0)
	rows := s.scanInstalledApps(apps)
	if len(rows) != 1 || rows[0]["cachePartial"] != true {
		t.Fatalf("denied cache discovery cannot be reported as no cache: %v", rows)
	}
}

func TestPermissionAuditOrdinaryPathDenialDoesNotPrescribeDiskAccess(t *testing.T) {
	for _, err := range []error{syscall.EACCES, syscall.EPERM} {
		if message, pane := humanizeTrashError("/Users/me/.copilot/logs/old.log", err); message == "" || pane != "" {
			t.Errorf("ordinary access control cannot prove Full Disk Access is missing: %q, %q", message, pane)
		}
	}
}
