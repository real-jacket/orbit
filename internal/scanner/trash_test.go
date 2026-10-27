package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// newTestScanner builds a scanner rooted at a temporary home, and points the
// trash and state directories at it too, so a test can never touch the real
// Trash or the real application-support folder.
func newTestScanner(t *testing.T) *Scanner {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MOLE_STUDIO_STATE_DIR", filepath.Join(home, "state"))
	t.Setenv("MOLE_STUDIO_TRASH_DIR", filepath.Join(home, ".Trash"))
	whitelistMu.Lock()
	cachedWhitelist = map[string]Whitelist{}
	whitelistMu.Unlock()
	t.Cleanup(func() {
		whitelistMu.Lock()
		cachedWhitelist = map[string]Whitelist{}
		whitelistMu.Unlock()
	})
	return &Scanner{home: home}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestTrashPathsMovesIntoTrashAndRecordsManifest(t *testing.T) {
	s := newTestScanner(t)
	// A path with a space is the case the old code silently truncated.
	original := filepath.Join(s.home, "Library", "Caches", "Some App", "cache.bin")
	writeFile(t, original, "cached bytes")

	outcome := s.TrashPaths([]string{original})
	if len(outcome.Moved) != 1 {
		t.Fatalf("moved = %d, want 1 (skipped %v, failed %v)", len(outcome.Moved), outcome.Skipped, outcome.Failed)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("original still exists: %v", err)
	}
	trashed := outcome.Moved[0].Trashed
	if !strings.HasSuffix(trashed, filepath.Join(".Trash", "cache.bin")) {
		t.Errorf("trashed at %s, want the item in the Trash", trashed)
	}
	if _, err := os.Stat(trashed); err != nil {
		t.Fatalf("trashed item missing: %v", err)
	}
	if manifest := readTrashManifest(); len(manifest.Entries) != 1 || manifest.Entries[0].Original != original {
		t.Fatalf("manifest = %+v, want the receipt for %s", manifest, original)
	}

	restore := s.RestoreTrash([]string{original})
	if len(restore.Restored) != 1 {
		t.Fatalf("restored = %v, failed %v", restore.Restored, restore.Failed)
	}
	if content, err := os.ReadFile(original); err != nil || string(content) != "cached bytes" {
		t.Fatalf("restored content = %q, err %v", content, err)
	}
	if manifest := readTrashManifest(); len(manifest.Entries) != 0 {
		t.Errorf("manifest still holds %+v after restoring", manifest.Entries)
	}
}

func TestTrashPathsRefusesWhitelistedPaths(t *testing.T) {
	s := newTestScanner(t)
	writeFile(t, WhitelistPath(s.home), "~/Library/Caches/JetBrains*\n")
	protected := filepath.Join(s.home, "Library", "Caches", "JetBrainsGoLand", "caches")
	writeFile(t, filepath.Join(protected, "index.dat"), "keep me")

	outcome := s.TrashPaths([]string{protected})
	if len(outcome.Moved) != 0 {
		t.Fatalf("moved = %+v, want nothing: the path is whitelisted", outcome.Moved)
	}
	if len(outcome.Skipped) != 1 || outcome.Skipped[0].Reason != "白名单保护" {
		t.Fatalf("skipped = %+v, want one 白名单保护 entry", outcome.Skipped)
	}
	if _, err := os.Stat(filepath.Join(protected, "index.dat")); err != nil {
		t.Errorf("protected file was removed: %v", err)
	}
}

func TestTrashPathsRefusesAParentOfAProtectedPath(t *testing.T) {
	s := newTestScanner(t)
	writeFile(t, WhitelistPath(s.home), "~/Library/Caches/JetBrains*\n")
	parent := filepath.Join(s.home, "Library", "Caches")
	writeFile(t, filepath.Join(parent, "JetBrainsGoLand", "caches", "index.dat"), "keep me")

	outcome := s.TrashPaths([]string{parent})
	if len(outcome.Moved) != 0 {
		t.Fatalf("moved = %+v, want the parent refused", outcome.Moved)
	}
	if _, err := os.Stat(filepath.Join(parent, "JetBrainsGoLand", "caches", "index.dat")); err != nil {
		t.Errorf("protected child was removed with its parent: %v", err)
	}
}

func TestTrashPathsReportsMissingPaths(t *testing.T) {
	s := newTestScanner(t)
	missing := filepath.Join(s.home, "does-not-exist")
	outcome := s.TrashPaths([]string{missing, ""})
	if len(outcome.Failed) != 1 || outcome.Failed[0].Error != "路径不存在" {
		t.Fatalf("failed = %+v, want one 路径不存在 entry", outcome.Failed)
	}
	if len(outcome.Moved) != 0 {
		t.Errorf("moved = %+v, want nothing", outcome.Moved)
	}
}

func TestTrashPathsRefusesTheTrashAndTheHomeDirectory(t *testing.T) {
	s := newTestScanner(t)
	outcome := s.TrashPaths([]string{s.home, filepath.Join(s.home, ".Trash")})
	if len(outcome.Moved) != 0 {
		t.Fatalf("moved = %+v, want both refused", outcome.Moved)
	}
	if len(outcome.Skipped) != 2 {
		t.Errorf("skipped = %+v, want two entries", outcome.Skipped)
	}
}

func TestTrashPathsNeverOverwritesWhatIsAlreadyTrashed(t *testing.T) {
	s := newTestScanner(t)
	first := filepath.Join(s.home, "one", "note.txt")
	second := filepath.Join(s.home, "two", "note.txt")
	writeFile(t, first, "first")
	writeFile(t, second, "second")

	if outcome := s.TrashPaths([]string{first}); len(outcome.Moved) != 1 {
		t.Fatalf("first move failed: %+v", outcome)
	}
	outcome := s.TrashPaths([]string{second})
	if len(outcome.Moved) != 1 {
		t.Fatalf("second move failed: %+v", outcome)
	}
	if outcome.Moved[0].Trashed == filepath.Join(s.home, ".Trash", "note.txt") {
		t.Fatalf("second item reused the first name: %s", outcome.Moved[0].Trashed)
	}
	if content, err := os.ReadFile(filepath.Join(s.home, ".Trash", "note.txt")); err != nil || string(content) != "first" {
		t.Errorf("first trashed item was overwritten: %q %v", content, err)
	}
}

func TestRestoreTrashRenamesWhenTheOriginalPathIsOccupied(t *testing.T) {
	s := newTestScanner(t)
	original := filepath.Join(s.home, "Library", "Caches", "app", "data.bin")
	writeFile(t, original, "old")
	if outcome := s.TrashPaths([]string{original}); len(outcome.Moved) != 1 {
		t.Fatalf("trash failed: %+v", outcome)
	}
	// Something new took the old path while the item sat in the Trash.
	writeFile(t, original, "new")

	outcome := s.RestoreTrash([]string{original})
	if len(outcome.Restored) != 1 {
		t.Fatalf("restored = %v, failed %v", outcome.Restored, outcome.Failed)
	}
	if content, err := os.ReadFile(original); err != nil || string(content) != "new" {
		t.Fatalf("restore overwrote the occupant: %q %v", content, err)
	}
	restored := filepath.Join(s.home, "Library", "Caches", "app", "data 2.bin")
	if content, err := os.ReadFile(restored); err != nil || string(content) != "old" {
		t.Fatalf("restored copy = %q, err %v; want it beside the occupant", content, err)
	}
}

func TestRestoreTrashWithoutAReceiptFallsBackToTheTrashName(t *testing.T) {
	s := newTestScanner(t)
	original := filepath.Join(s.home, "Documents", "report.pdf")
	writeFile(t, filepath.Join(s.home, ".Trash", "report.pdf"), "pdf")

	outcome := s.RestoreTrash([]string{original})
	if len(outcome.Restored) != 1 {
		t.Fatalf("restored = %v, failed %v", outcome.Restored, outcome.Failed)
	}
	if content, err := os.ReadFile(original); err != nil || string(content) != "pdf" {
		t.Fatalf("content = %q, err %v", content, err)
	}
}

func TestRestoreTrashReportsItemsThatAreAlreadyGone(t *testing.T) {
	s := newTestScanner(t)
	outcome := s.RestoreTrash([]string{filepath.Join(s.home, "Documents", "gone.txt")})
	if len(outcome.Restored) != 0 || len(outcome.Failed) != 1 {
		t.Fatalf("outcome = %+v, want one failure", outcome)
	}
	if outcome.Failed[0].Error != "废纸篓中已找不到该项" {
		t.Errorf("error = %q", outcome.Failed[0].Error)
	}
}

func TestAddWhitelistEntryIsVisibleToTheScannerAndToMole(t *testing.T) {
	s := newTestScanner(t)
	cache := filepath.Join(s.home, "Library", "Caches", "com.example.tool")
	writeFile(t, filepath.Join(cache, "blob"), "bytes")

	if _, err := s.AddWhitelistEntry(cache); err != nil {
		t.Fatalf("AddWhitelistEntry: %v", err)
	}
	if !s.Whitelisted(filepath.Join(cache, "blob")) {
		t.Error("a path under the new entry is not protected")
	}
	file, err := os.ReadFile(WhitelistPath(s.home))
	if err != nil {
		t.Fatalf("read whitelist: %v", err)
	}
	if !strings.Contains(string(file), cache) {
		t.Errorf("whitelist file = %q, want the expanded path %s", file, cache)
	}
	// Adding the same entry twice must not duplicate the line.
	if _, err := s.AddWhitelistEntry(cache); err != nil {
		t.Fatalf("AddWhitelistEntry again: %v", err)
	}
	file, _ = os.ReadFile(WhitelistPath(s.home))
	if strings.Count(string(file), cache) != 1 {
		t.Errorf("whitelist file = %q, want one copy of %s", file, cache)
	}
	if got := s.UserWhitelistPaths(); !containsString(got, cache) {
		t.Errorf("UserWhitelistPaths = %v, want %s", got, cache)
	}

	if _, err := s.RemoveWhitelistEntry(cache); err != nil {
		t.Fatalf("RemoveWhitelistEntry: %v", err)
	}
	if s.Whitelisted(cache) {
		t.Error("the removed entry still protects the path")
	}
}

func TestRemoveWhitelistEntryEditsACompositeLine(t *testing.T) {
	s := newTestScanner(t)
	first := filepath.Join(s.home, "Library", "Caches")
	second := filepath.Join(s.home, "Library", "Logs")
	writeFile(t, WhitelistPath(s.home), first+" & "+second+"\n")

	if _, err := s.RemoveWhitelistEntry(first); err != nil {
		t.Fatalf("RemoveWhitelistEntry: %v", err)
	}
	file, _ := os.ReadFile(WhitelistPath(s.home))
	if strings.Contains(string(file), first) {
		t.Errorf("whitelist = %q, want %s gone", file, first)
	}
	if !strings.Contains(string(file), second) {
		t.Errorf("whitelist = %q, want %s kept", file, second)
	}
}

func TestUserWhitelistPathsSplitsCompositeEntriesAndDropsTheSentinel(t *testing.T) {
	s := newTestScanner(t)
	first := filepath.Join(s.home, "Library", "Caches")
	second := filepath.Join(s.home, "Library", "Logs")
	writeFile(t, WhitelistPath(s.home), "FINDER_METADATA\n"+first+" & "+second+"\n")

	paths := s.UserWhitelistPaths()
	if containsString(paths, "FINDER_METADATA") {
		t.Errorf("paths = %v, want the sentinel dropped", paths)
	}
	if !containsString(paths, first) || !containsString(paths, second) {
		t.Errorf("paths = %v, want both parts of the composite line", paths)
	}
}

func TestValidateWhitelistEntryRejectsTheDangerousOnes(t *testing.T) {
	s := newTestScanner(t)
	for _, entry := range []string{"", "/", "relative/path", "/System/Library"} {
		if _, err := s.AddWhitelistEntry(entry); err == nil {
			t.Errorf("AddWhitelistEntry(%q) accepted an entry mole would refuse", entry)
		}
	}
	if _, err := os.Stat(WhitelistPath(s.home)); !os.IsNotExist(err) {
		t.Errorf("a rejected entry still wrote the whitelist: %v", err)
	}
}

func TestTrashPathsExpandsTheTildeSoTheReceiptMatchesThePage(t *testing.T) {
	s := newTestScanner(t)
	writeFile(t, filepath.Join(s.home, "Library", "Caches", "some.app", "blob.bin"), "x")

	// The page hands over ~/… paths whenever homePath has not reached it yet;
	// the receipt must always carry the expanded original, otherwise the page
	// cannot match it back to the row it is waiting on.
	outcome := s.TrashPaths([]string{"~/Library/Caches/some.app"})
	if len(outcome.Moved) != 1 {
		t.Fatalf("moved %d, want 1 (failed %v, skipped %v)", len(outcome.Moved), outcome.Failed, outcome.Skipped)
	}
	want := filepath.Join(s.home, "Library", "Caches", "some.app")
	if got := outcome.Moved[0].Original; got != want {
		t.Errorf("original = %q, want the expanded path %q", got, want)
	}
	if strings.HasPrefix(outcome.Moved[0].Original, "~") {
		t.Error("receipt still contains a tilde path; the page cannot expand it back")
	}
	if outcome.Moved[0].Trashed == "" {
		t.Error("receipt has no trashed path, so restore cannot find the file")
	}
}

// The permission a failed move needs depends on what is being moved: another
// app goes through App Management, a protected Library folder through Full Disk
// Access. Sending the user to the wrong pane is the same as saying nothing.
func TestHumanizeTrashErrorPointsAtTheRightSettingsPane(t *testing.T) {
	denied := &os.PathError{Op: "rename", Path: "/x", Err: syscall.EPERM}

	cases := []struct {
		path string
		want string
		msg  string
	}{
		{"/Applications/Figma.app", "appManagement", "App 管理"},
		{"/Users/me/Library/Mail/1.emlx", "fullDiskAccess", "完全磁盘访问"},
		{"/Users/me/Library/Caches", "fullDiskAccess", "完全磁盘访问"},
		{"/private/var/db/sudo", "fullDiskAccess", "完全磁盘访问"},
		{"/Users/me/Downloads/report.pdf", "filesAndFolders", "文件与文件夹"},
		{"/Users/me/Documents/tax", "filesAndFolders", "文件与文件夹"},
		{"/Users/me/Desktop/shot.png", "filesAndFolders", "文件与文件夹"},
		{"/Users/me/Notes/todo.md", "fullDiskAccess", "权限不足"},
	}
	for _, c := range cases {
		message, pane := humanizeTrashError(c.path, denied)
		if pane != c.want {
			t.Fatalf("%s: pane = %q, want %q", c.path, pane, c.want)
		}
		if !strings.Contains(message, c.msg) {
			t.Fatalf("%s: message %q does not mention %q", c.path, message, c.msg)
		}
	}

	// Non-permission failures carry no pane: there is nothing to grant.
	for _, err := range []error{syscall.ENOSPC, syscall.EXDEV, os.ErrNotExist} {
		if _, pane := humanizeTrashError("/Users/me/file", err); pane != "" {
			t.Fatalf("%v: unexpected pane %q", err, pane)
		}
	}

	failure := trashFailure("/Applications/Figma.app", denied)
	if failure.Path != "/Applications/Figma.app" || failure.Pane != "appManagement" {
		t.Fatalf("trashFailure = %+v", failure)
	}
}

func TestAppBundleDetectionIgnoresCaseAndTrailingSlash(t *testing.T) {
	for _, path := range []string{"/Applications/Slack.app", "/Applications/Slack.App/", "/Users/me/Desktop/Thing.app"} {
		if !isAppBundle(path) {
			t.Fatalf("%s should be an app bundle", path)
		}
	}
	for _, path := range []string{"/Applications", "/Applications/.hidden", "/Users/me/app.txt"} {
		if isAppBundle(path) {
			t.Fatalf("%s should not be an app bundle", path)
		}
	}
}

func TestNeedsFullDiskAccessCoversProtectedTrees(t *testing.T) {
	protected := []string{
		"/Users/me/Library/Mail",
		"/Users/me/Library",
		"/Library/Application Support/Foo",
		"/private/var/folders/x",
	}
	for _, path := range protected {
		if !needsFullDiskAccess(path) {
			t.Fatalf("%s should need Full Disk Access", path)
		}
	}
	for _, path := range []string{"/Users/me/Notes", "/Applications/Slack.app", "/tmp/lib"} {
		if needsFullDiskAccess(path) {
			t.Fatalf("%s should not need Full Disk Access", path)
		}
	}
}

func TestNeedsFolderAccessCoversTheGuardedFolders(t *testing.T) {
	for _, path := range []string{"/Users/me/Downloads/x.bin", "/Users/me/Documents", "/Users/me/Desktop/a.png"} {
		if !needsFolderAccess(path) {
			t.Fatalf("%s should need Files and Folders access", path)
		}
	}
	for _, path := range []string{"/Users/me/Notes/x.md", "/Users/me/DownloadsArchive", "/Applications/Slack.app"} {
		if needsFolderAccess(path) {
			t.Fatalf("%s should not need Files and Folders access", path)
		}
	}
}
