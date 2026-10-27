package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile writes size zero bytes to path, creating parent directories as
// needed. It is the shared fixture helper for the file-system tests in this
// package.
func writeTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// TestFormatBytes is a table-driven check of formatBytes, the port of the Swift
// RealMacSystemScanner.formatBytes. Swift used String(format: "%.2f GB") above
// 0.1 GB and String(format: "%.0f MB", max(1, mb)) below it.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		// gb = 0 < 0.1, so the MB branch runs; mb = 0, max(1, mb) => "1 MB".
		{name: "zero floors to one megabyte", bytes: 0, want: "1 MB"},
		{name: "fifty megabytes", bytes: 50_000_000, want: "50 MB"},
		// 99.999999 MB is not a tie, so %.0f round-half-away-from-zero carries
		// it to 100. The Swift String(format: "%.0f MB") did the same.
		{name: "just under one hundred megabytes carries to one hundred", bytes: 99_999_999, want: "100 MB"},
		// gb = 0.1 exactly, so the GB branch runs.
		{name: "one tenth of a gigabyte switches to the GB branch", bytes: 100_000_000, want: "0.10 GB"},
		{name: "one gigabyte", bytes: 1_000_000_000, want: "1.00 GB"},
		{name: "gigabyte rounded to two decimals", bytes: 1_234_000_000, want: "1.23 GB"},
		// Just below the 0.1 GB threshold: mb = 99.9, rounds to "100 MB".
		{name: "boundary just below the GB branch", bytes: 99_900_000, want: "100 MB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatBytes(test.bytes); got != test.want {
				t.Errorf("formatBytes(%d) = %q, want %q", test.bytes, got, test.want)
			}
		})
	}
}

// TestGB is a table-driven check of gb, the port of the Swift
// RealMacSystemScanner.gb: two-decimal rounding with a hard 0.01 floor.
func TestGB(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  float64
	}{
		{name: "zero is floored to the minimum", bytes: 0, want: 0.01},
		{name: "one gigabyte", bytes: 1_000_000_000, want: 1.0},
		// 0.004 rounds to 0.00, which is below the floor.
		{name: "four megabytes rounds to zero then floors", bytes: 4_000_000, want: 0.01},
		{name: "ten megabytes rounds to the floor", bytes: 10_000_000, want: 0.01},
		{name: "eleven megabytes rounds back to the floor", bytes: 11_000_000, want: 0.01},
		{name: "one hundred megabytes", bytes: 100_000_000, want: 0.1},
		{name: "one and a quarter gigabytes", bytes: 1_250_000_000, want: 1.25},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gb(test.bytes); got != test.want {
				t.Errorf("gb(%d) = %v, want %v", test.bytes, got, test.want)
			}
		})
	}
}

// TestPlainGB checks plainGB, which rounds like gb but has no floor: this is
// what the analyze bridge reports and why it can legitimately return 0.
func TestPlainGB(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  float64
	}{
		{name: "four megabytes has no floor", bytes: 4_000_000, want: 0.0},
		{name: "zero", bytes: 0, want: 0.0},
		{name: "one gigabyte", bytes: 1_000_000_000, want: 1.0},
		{name: "one and a quarter gigabytes", bytes: 1_250_000_000, want: 1.25},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plainGB(test.bytes); got != test.want {
				t.Errorf("plainGB(%d) = %v, want %v", test.bytes, got, test.want)
			}
		})
	}
}

// TestRound1AndRound2 pins the half-away-from-zero rounding the Swift
// `.rounded()` implemented: ties go away from zero, and negatives are handled.
func TestRound1AndRound2(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		round func(float64) float64
		want  float64
	}{
		{name: "round2 keeps two decimals", value: 1.234, round: round2, want: 1.23},
		{name: "round2 rounds up past the tie", value: 1.235, round: round2, want: 1.24},
		{name: "round2 leaves an exact value alone", value: 1.25, round: round2, want: 1.25},
		{name: "round2 of zero", value: 0, round: round2, want: 0},
		{name: "round2 of a negative", value: -1.234, round: round2, want: -1.23},
		{name: "round1 keeps one decimal", value: 1.24, round: round1, want: 1.2},
		{name: "round1 rounds up past the tie", value: 1.25, round: round1, want: 1.3},
		{name: "round1 of a negative tie goes away from zero", value: -1.25, round: round1, want: -1.3},
		{name: "round1 of zero", value: 0, round: round1, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.round(test.value); got != test.want {
				t.Errorf("round(%v) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

// TestExpandAndRelativize checks the ~ translation both ways. It uses the
// Scanner's own home so it never touches the real HOME.
func TestExpandAndRelativize(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}

	expandTests := []struct {
		name string
		path string
		want string
	}{
		{name: "bare tilde is the home directory", path: "~", want: home},
		{name: "tilde path joins onto home", path: "~/Library/Logs", want: filepath.Join(home, "Library/Logs")},
		{name: "absolute path is unchanged", path: "/Applications", want: "/Applications"},
		{name: "relative path is unchanged", path: "Library/Logs", want: "Library/Logs"},
		{name: "tilde in the middle is not expanded", path: "/tmp/~x", want: "/tmp/~x"},
	}
	for _, test := range expandTests {
		t.Run("expand/"+test.name, func(t *testing.T) {
			if got := s.expand(test.path); got != test.want {
				t.Errorf("expand(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}

	relativizeTests := []struct {
		name string
		path string
		want string
	}{
		{name: "home itself becomes tilde", path: home, want: "~"},
		{name: "path under home becomes tilde-prefixed", path: filepath.Join(home, "Library"), want: "~/Library"},
		{name: "path under home keeps its nested tail", path: filepath.Join(home, "Library/Containers/x"), want: "~/Library/Containers/x"},
		{name: "path outside home is unchanged", path: "/Applications/Safari.app", want: "/Applications/Safari.app"},
		// The prefix check requires home+"/", so a sibling whose name merely
		// starts with the home string is not relativized.
		{name: "sibling sharing the home prefix is unchanged", path: home + "other/x", want: home + "other/x"},
	}
	for _, test := range relativizeTests {
		t.Run("relativize/"+test.name, func(t *testing.T) {
			if got := s.relativize(test.path); got != test.want {
				t.Errorf("relativize(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

// TestFastDirectorySizeAccumulatesFiles checks that fastDirectorySize sums the
// files in a temp tree. Note that walkDir increments count for every directory
// entry it visits, including subdirectories, so count is an entry count and not
// a file count: 5 regular files plus the one nested directory is 6.
func TestFastDirectorySizeAccumulatesFiles(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}
	root := filepath.Join(home, "tree")

	// Three files of 1 MiB at the top level, one nested one level down.
	for i := 0; i < 3; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("top%d.bin", i)), 1<<20)
	}
	writeTestFile(t, filepath.Join(root, "nested", "deep.bin"), 1<<20)
	// A hidden file still counts: walkDir does not skip dotfiles.
	writeTestFile(t, filepath.Join(root, ".hidden.bin"), 1<<20)

	const regularFiles = 5
	size := s.fastDirectorySize(root, 0)
	if size.count != regularFiles+1 {
		t.Errorf("fastDirectorySize count = %d, want %d (5 files + 1 nested directory)", size.count, regularFiles+1)
	}
	// The exact byte total is allocation-dependent (blocks x 512), so assert a
	// floor: every 1 MiB file occupies at least its logical size.
	if size.bytes < regularFiles<<20 {
		t.Errorf("fastDirectorySize bytes = %d, want at least %d", size.bytes, regularFiles<<20)
	}
}

// TestWalkDirCountsDirectoriesAsEntries pins the counting rule itself: a tree
// holding only directories still advances count, which is why the caps in the
// scan functions bound entries rather than files.
func TestWalkDirCountsDirectoriesAsEntries(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("dir%d", i), "inner"), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	var total dirSize
	walkDir(root, 0, &total)
	if total.count != 6 {
		t.Errorf("walkDir count = %d, want 6 (3 directories + 3 inner directories)", total.count)
	}
	if total.bytes != 0 {
		t.Errorf("walkDir bytes = %d, want 0 for a tree with no regular files", total.bytes)
	}
}

// TestFastDirectorySizeMaxFilesCap checks that the maxFiles cap is enforced.
// walkDir counts every directory entry (files and directories alike) and stops
// before descending once the count has reached the cap, so the cap bounds the
// entry count, not the byte total.
func TestFastDirectorySizeMaxFilesCap(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}
	root := filepath.Join(home, "wide")
	for i := 0; i < 20; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("f%02d.bin", i)), 1<<20)
	}

	uncapped := s.fastDirectorySize(root, 0)
	if uncapped.count != 20 {
		t.Fatalf("uncapped count = %d, want 20", uncapped.count)
	}
	capped := s.fastDirectorySize(root, 5)
	if capped.count > 5 {
		t.Errorf("capped count = %d, want at most 5", capped.count)
	}
	if capped.bytes >= uncapped.bytes {
		t.Errorf("capped bytes = %d, want fewer than the uncapped %d", capped.bytes, uncapped.bytes)
	}
}

// TestFastDirectorySizeFileAndMissingPath pins the two non-directory cases: a
// regular file reports its own size and a single entry, and a path that does
// not exist reports nothing. Both come from the Swift implementation.
func TestFastDirectorySizeFileAndMissingPath(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}
	file := filepath.Join(home, "single.bin")
	writeTestFile(t, file, 4096)

	size := s.fastDirectorySize(file, 0)
	if size.count != 1 {
		t.Errorf("fastDirectorySize(file).count = %d, want 1", size.count)
	}
	if size.bytes <= 0 {
		t.Errorf("fastDirectorySize(file).bytes = %d, want > 0", size.bytes)
	}

	missing := s.fastDirectorySize(filepath.Join(home, "does-not-exist"), 0)
	if missing.bytes != 0 || missing.count != 0 {
		t.Errorf("fastDirectorySize(missing) = %+v, want the zero value", missing)
	}
}

// TestFastDirectorySizeExpandsTilde checks that fastDirectorySize expands a
// leading ~ against the Scanner's home rather than the real HOME.
func TestFastDirectorySizeExpandsTilde(t *testing.T) {
	home := t.TempDir()
	s := &Scanner{home: home}
	writeTestFile(t, filepath.Join(home, "tilde", "a.bin"), 1<<20)
	writeTestFile(t, filepath.Join(home, "tilde", "b.bin"), 1<<20)

	size := s.fastDirectorySize("~/tilde", 0)
	if size.count != 2 {
		t.Errorf("fastDirectorySize(~/tilde).count = %d, want 2", size.count)
	}
	if size.bytes < 2<<20 {
		t.Errorf("fastDirectorySize(~/tilde).bytes = %d, want at least %d", size.bytes, 2<<20)
	}
}

// TestWalkDirSkipsUnreadableDirectory checks the error path: a directory that
// cannot be read contributes nothing rather than failing.
func TestWalkDirSkipsUnreadableDirectory(t *testing.T) {
	var total dirSize
	walkDir(filepath.Join(t.TempDir(), "absent"), 0, &total)
	if total.bytes != 0 || total.count != 0 {
		t.Errorf("walkDir over a missing directory = %+v, want the zero value", total)
	}
}
