package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadWarningsBelongToTheirScannerHome(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Desktop")
	directoryReads.Lock()
	directoryReads.failed[path] = "test permission refusal"
	directoryReads.Unlock()
	t.Cleanup(func() { directoryReads.Lock(); delete(directoryReads.failed, path); directoryReads.Unlock() })
	if warnings := scanReadWarnings(home); len(warnings) != 1 || warnings[0]["path"] != path {
		t.Fatalf("warnings = %v", warnings)
	}
	if warnings := scanReadWarnings(filepath.Join(home, "other")); len(warnings) != 0 {
		t.Fatalf("unrelated warnings = %v", warnings)
	}
}

func TestDirectorySizeReportsIncompleteInventory(t *testing.T) {
	home := fixtureHome(t)
	path := fixtureDir(t, home, "cache", 4096)
	writeTestFile(t, filepath.Join(path, "second.bin"), 4096)
	size := (&Scanner{home: home}).fastDirectorySize(path, 1)
	if !size.partial || size.files != 1 || size.modified == 0 {
		t.Fatalf("capped inventory = %+v", size)
	}
	if warnings := scanReadWarnings(home); len(warnings) != 1 || !strings.Contains(warnings[0]["reason"].(string), "上限") {
		t.Fatalf("capped inventory must disclose its limit: %v", warnings)
	}
}

func TestDirectorySizeSkipsSymlinksAndDeduplicatesHardLinks(t *testing.T) {
	home := fixtureHome(t)
	root := fixtureDir(t, home, "cache", 4096)
	file := filepath.Join(root, "payload.bin")
	if err := os.Link(file, filepath.Join(root, "linked.bin")); err != nil {
		t.Fatal(err)
	}
	outside := fixtureDir(t, home, "outside", 8192)
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{home: home}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	size := s.fastDirectorySize(root, 0)
	if size.bytes != allocatedSize(info) || size.files != 2 {
		t.Fatalf("hard links must count one allocation, not follow aliases: %+v", size)
	}
	if size := s.fastDirectorySize(filepath.Join(root, "alias"), 0); size.bytes != 0 {
		t.Fatalf("symlink root followed outside the inventory: %+v", size)
	}
}

func TestDirectorySizeUsesAllocatedBytesForSparseFiles(t *testing.T) {
	home := fixtureHome(t)
	file := filepath.Join(home, "sparse.bin")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(8 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := (&Scanner{home: home}).fastDirectorySize(file, 0); got.bytes != allocatedSize(info) || got.bytes >= info.Size() {
		t.Fatalf("sparse file is not its logical length: %+v, logical=%d", got, info.Size())
	}
}

func TestInstallerScanDoesNotDescendIntoBundles(t *testing.T) {
	home := fixtureHome(t)
	writeTestFile(t, filepath.Join(home, "Downloads/Real.dmg"), 21_000_000)
	writeTestFile(t, filepath.Join(home, "Downloads/Folder/Nested.dmg"), 21_000_000)
	for _, bundle := range []string{"Qoder Installer.app/Contents/Resources/Qoder.dmg", "Kit.framework/Versions/A/Kit.pkg", "Old.xcarchive/Products/Old.dmg", "Meta.pkg/Contents/Archive.dmg"} {
		writeTestFile(t, filepath.Join(home, "Downloads", bundle), 21_000_000)
	}
	items := (&Scanner{home: home}).scanInstallerPackages()
	var names []string
	for _, item := range items {
		names = append(names, item["name"].(string))
		if strings.Contains(item["path"].(string), ".app/") || strings.Contains(item["path"].(string), "Contents") {
			t.Fatalf("a file inside a bundle was offered as an installer: %v", item)
		}
	}
	if len(names) != 2 {
		t.Fatalf("installers = %v, want only Real.dmg and Nested.dmg", names)
	}
}

func TestReadWarningsKeepOneEntryPerMeasuredRoot(t *testing.T) {
	home := t.TempDir()
	t.Cleanup(func() { clearReadWarnings(home) })
	root := filepath.Join(home, "Library/Caches/pnpm")
	recordReadWarningKind(root, "root", warnLimit)
	recordReadWarningKind(filepath.Join(root, "store/v3"), "child of the same kind", warnLimit)
	recordReadWarningKind(filepath.Join(root, "store/v3/files"), "grandchild of the same kind", warnLimit)
	recordReadWarningKind(filepath.Join(root, "locked"), "different kind stays visible", warnPermission)
	warnings := scanReadWarnings(home)
	if len(warnings) != 2 || warnings[0]["path"] != root || warnings[0]["kind"] != warnLimit ||
		warnings[1]["path"] != filepath.Join(root, "locked") || warnings[1]["kind"] != warnPermission {
		t.Fatalf("expected the outermost limit entry and the permission entry only: %v", warnings)
	}
}

func TestReadErrorsAreClassifiedByCause(t *testing.T) {
	home := "/Users/someone"
	cases := []struct {
		name string
		path string
		err  error
		want string
	}{
		{"apple cache refuses even with access", home + "/Library/Caches/com.apple.homed/x", &fs.PathError{Op: "open", Err: syscall.EPERM}, warnSystem},
		{"user cache that needs a grant", home + "/Library/Caches/Spotify", &fs.PathError{Op: "open", Err: syscall.EPERM}, warnPermission},
		{"plain access refusal", home + "/Desktop", &fs.PathError{Op: "open", Err: syscall.EACCES}, warnPermission},
		{"a prompt nobody answered", home + "/Documents", context.DeadlineExceeded, warnPermission},
		{"anything else is an error", home + "/Movies", &fs.PathError{Op: "open", Err: syscall.EIO}, warnError},
	}
	for _, c := range cases {
		if kind, reason := classifyReadError(c.path, c.err); kind != c.want || reason == "" {
			t.Errorf("%s: kind=%q reason=%q, want %q", c.name, kind, reason, c.want)
		}
	}
}

func TestCappedMeasurementRecordsOneWarningOnItsRoot(t *testing.T) {
	home := fixtureHome(t)
	t.Cleanup(func() { clearReadWarnings(home) })
	root := fixtureDir(t, home, "Library/Caches/dense", 1)
	for i := 0; i < 6; i++ {
		writeTestFile(t, filepath.Join(root, "a", "b", "f"+itoa64(int64(i))), 1)
	}
	if size := (&Scanner{home: home}).fastDirectorySize(root, 3); !size.partial {
		t.Fatalf("fixture must hit the cap: %+v", size)
	}
	warnings := scanReadWarnings(home)
	if len(warnings) != 1 || warnings[0]["path"] != root || warnings[0]["kind"] != warnLimit {
		t.Fatalf("a cap hit is one limit warning on the measured root: %v", warnings)
	}
}

func TestBulkListingMatchesPortableListing(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "plain"), 5000)
	writeTestFile(t, filepath.Join(dir, "empty"), 0)
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("plain", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, "plain"), filepath.Join(dir, "twin")); err != nil {
		t.Fatal(err)
	}
	index := func(items []dirItem) map[string]dirItem {
		out := map[string]dirItem{}
		for _, item := range items {
			item.mtime = 0 // sub-second rounding may differ between the two paths
			if item.kind != itemFile {
				item = dirItem{name: item.name, kind: item.kind} // identity is only filled for regular files
			}
			out[item.name] = item
		}
		return out
	}
	got, err := listDirItems(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := listDirStd(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, w := index(got), index(want)
	if len(g) != len(w) || len(g) != 5 {
		t.Fatalf("entries differ: got %v want %v", g, w)
	}
	for name, item := range w {
		if g[name] != item {
			t.Errorf("%s: got %+v want %+v", name, g[name], item)
		}
	}
}

func TestLargeFileWalkFindsBigFilesAndReportsAnExpiredBudget(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, "Downloads")
	big := filepath.Join(root, "movie.mov")
	writeTestFile(t, big, 16<<20)
	writeTestFile(t, filepath.Join(root, "small.txt"), 10)
	writeTestFile(t, filepath.Join(root, ".hidden", "huge.bin"), 16<<20)
	s := &Scanner{home: home}
	var found []largeFile
	if !s.collectLargeFiles(root, root, time.Now().Add(time.Minute), &found) || len(found) != 1 || found[0].path != big {
		t.Fatalf("only the visible file over the threshold belongs in the list: %v", found)
	}
	found = nil
	if s.collectLargeFiles(root, root, time.Now().Add(-time.Second), &found) {
		t.Fatal("an expired budget must be reported to the caller")
	}
}
