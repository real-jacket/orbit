package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Vendor folders can hold the data of an installed app even when the parent
// has neither the app's display name nor its bundle identifier.
func TestOrphanScanPreservesInstalledAppVendorSupport(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications", "Fixture Ego Lite.app", "Contents")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>Fixture Ego Lite</string>
<key>CFBundleDisplayName</key><string>Fixture Ego Lite</string>
<key>CFBundleIdentifier</key><string>com.fixturecitrolabs.ego.lite</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDir(t, home, "Library/Application Support/Fixture Citro Labs/Fixture Ego Lite", 16<<20)
	fixtureDir(t, home, "Library/Application Support/Fixture Citro Labs/Fixture EgoUpdater", 1<<20)
	fixtureDir(t, home, "Library/Caches/Fixture Citro CacheVendor/Fixture Ego Lite", 16<<20)
	fixtureDir(t, home, "Library/Application Support/Fixture GoneVendor/Fixture GoneApp", 16<<20)
	fixtureDir(t, home, "Library/Application Support/Fixture SimilarVendor/Fixture Ego Lite Archive", 16<<20)
	fileVendor := filepath.Join(home, "Library/Application Support/Fixture FileVendor")
	if err := os.MkdirAll(fileVendor, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(fileVendor, "Fixture Ego Lite"), 16<<20)

	rows := (&Scanner{home: home}).scanOrphanedLeftovers()
	remaining := map[string]bool{
		"Fixture GoneVendor":    false,
		"Fixture SimilarVendor": false,
		"Fixture FileVendor":    false,
	}
	for _, row := range rows {
		if row["name"] == "Fixture Citro Labs" || row["name"] == "Fixture Citro CacheVendor" {
			t.Errorf("installed Fixture Ego Lite data classified as an orphan: %v", row)
		}
		if name, ok := row["name"].(string); ok {
			if _, expected := remaining[name]; expected {
				remaining[name] = true
				if row["ownershipStatus"] != "unclaimed" || row["category"] != "未归属应用数据" || row["statusText"] != "归属未确认 · 待核对" {
					t.Errorf("unclaimed support %q misstates uninstall evidence: %v", name, row)
				}
			}
		}
	}
	for name, found := range remaining {
		if !found {
			t.Errorf("unclaimed vendor support %q disappeared from the review candidates", name)
		}
	}
}

func TestUnclaimedSandboxRootsArePersonalData(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Containers/com.fixture.gone.personal", 16<<20)
	fixtureDir(t, home, "Library/Group Containers/com.fixture.gone.personal", 16<<20)
	rows := (&Scanner{home: home}).scanOrphanedLeftovers()
	if len(rows) != 2 {
		t.Fatalf("want both unclaimed sandbox roots, got %v", rows)
	}
	for _, row := range rows {
		if row["ownershipStatus"] != "unclaimed" || row["category"] != "未归属沙盒数据" || row["checked"] != false || strings.Contains(row["statusText"].(string), "已卸载") {
			t.Errorf("unknown sandbox ownership must remain unchecked and unclaimed: %v", row)
		}
		for _, subpath := range row["subpaths"].([]map[string]any) {
			if subpath["kind"] != "config" || subpath["checked"] != false {
				t.Errorf("whole sandbox root offered as disposable cache: %v", subpath)
			}
		}
	}
}

func TestVendorSupportOwnershipUsesStableBundleIdentifier(t *testing.T) {
	for _, fixture := range []struct {
		name, bundleID, child string
		owned                 bool
	}{
		{name: "renamed ego lite", bundleID: "com.citrolabs.ego.lite", child: "ego lite", owned: true},
		{name: "ego full", bundleID: "com.citrolabs.ego", child: "ego", owned: true},
		{name: "different product", bundleID: "com.citrolabs.ego", child: "ego lite"},
		{name: "identifier suffix", bundleID: "com.citrolabs.ego.lite.old", child: "ego lite"},
		{name: "directory suffix", bundleID: "com.citrolabs.ego.lite", child: "ego lite-old"},
		{name: "updater only", bundleID: "com.citrolabs.ego.lite", child: "EgoUpdater"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			home := fixtureHome(t)
			fixtureDir(t, home, "Library/Application Support/Citro Labs/"+fixture.child, 16<<20)
			s := &Scanner{home: home}
			rows := s.scanUnclaimedContainers(home, map[string]struct{}{}, map[string]struct{}{"renamed browser": {}}, map[string]struct{}{fixture.bundleID: {}}, map[string]struct{}{})
			found := false
			for _, row := range rows {
				if row["name"] != "Citro Labs" {
					continue
				}
				found = true
				if row["ownershipStatus"] != "unclaimed" || row["checked"] != false {
					t.Errorf("unmatched product must stay an unchecked unknown candidate: %v", row)
				}
			}
			if found == fixture.owned {
				t.Fatalf("vendor candidate present=%t, want owned=%t", found, fixture.owned)
			}
		})
	}
}

func TestUnclaimedCacheKeepsUnknownOwnershipAndRemainsUnchecked(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Caches/Fixture UnmatchedCache", 16<<20)
	rows := (&Scanner{home: home}).scanUnclaimedContainers(home, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{})
	if len(rows) != 1 {
		t.Fatalf("want one unknown cache, got %v", rows)
	}
	row := rows[0]
	if row["ownershipStatus"] != "unclaimed" || row["category"] != "未归属缓存" || row["checked"] != false {
		t.Fatalf("unmatched cache is an unchecked unknown candidate: %v", row)
	}
	for _, subpath := range row["subpaths"].([]map[string]any) {
		if subpath["kind"] != "cache" || subpath["checked"] != false {
			t.Errorf("unknown cache retains cache classification without default selection: %v", subpath)
		}
	}
}

func TestUnclaimedVendorCachePreservesInstalledChild(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Caches/Fixture CacheVendor/Fixture Ego Lite", 16<<20)
	fixtureDir(t, home, "Library/Caches/Fixture UnclaimedCache/Fixture GoneApp", 16<<20)
	s := &Scanner{home: home}
	rows := s.scanUnclaimedContainers(home, map[string]struct{}{}, map[string]struct{}{"fixture ego lite": {}}, map[string]struct{}{}, map[string]struct{}{})
	if len(rows) != 1 || rows[0]["name"] != "Fixture UnclaimedCache" {
		t.Fatalf("installed app's vendor cache was considered unclaimed: %v", rows)
	}
}

func TestVendorChildOwnershipRequiresExactInstalledIdentity(t *testing.T) {
	names := map[string]struct{}{"fixture ego lite": {}}
	ids := map[string]struct{}{"com.fixturecitrolabs.ego.lite": {}}
	for _, fixture := range []struct {
		child string
		owned bool
	}{
		{"Fixture Ego Lite", true},
		{"com.fixturecitrolabs.ego.lite", true},
		{"Fixture Ego Lite-old", false},
		{"Fixture Ego Lite.old", false},
		{"Fixture Ego Lite_old", false},
		{"com.fixturecitrolabs.ego.lite-old", false},
		{"com.fixturecitrolabs.ego.lite.old", false},
	} {
		t.Run(fixture.child, func(t *testing.T) {
			home := fixtureHome(t)
			vendor := filepath.Join(home, "Library/Application Support/Fixture Vendor")
			if err := os.MkdirAll(filepath.Join(vendor, fixture.child), 0o755); err != nil {
				t.Fatal(err)
			}
			if got := (&Scanner{home: home}).hasInstalledSupportChild(vendor, names, ids); got != fixture.owned {
				t.Errorf("ownership for child %q = %t, want exact identity match %t", fixture.child, got, fixture.owned)
			}
		})
	}
}
