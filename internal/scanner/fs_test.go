package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
