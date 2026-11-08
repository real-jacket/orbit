//go:build darwin

package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestExclusiveRenameNeverOverwritesAnExistingTarget(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "from.txt"), filepath.Join(dir, "to.txt")
	writeFile(t, from, "original")
	writeFile(t, to, "keep")
	if err := renameWithoutOverwrite(from, to); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive rename error = %v, want EEXIST", err)
	}
	if content, _ := os.ReadFile(to); string(content) != "keep" || !exists(from) {
		t.Fatal("exclusive rename overwrote the target or removed the source")
	}
	if err := renameWithoutOverwrite(from, to+"\x00ignored"); err == nil {
		t.Fatal("a NUL cannot truncate a path at the C boundary")
	}
}

func TestExclusiveRenameConcurrentErrorsKeepTheirErrno(t *testing.T) {
	directory := t.TempDir()
	from, to := filepath.Join(directory, "from"), filepath.Join(directory, "to")
	writeFile(t, from, "original")
	writeFile(t, to, "keep")
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for attempt := 0; attempt < 32; attempt++ {
				if err := renameWithoutOverwrite(from, to); !errors.Is(err, os.ErrExist) {
					t.Errorf("occupied target errno = %v, want EEXIST", err)
					return
				}
				if err := renameWithoutOverwrite(from+".missing", to+".missing"); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("missing source errno = %v, want ENOENT", err)
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestConcurrentSameNameTrashMovesKeepEveryIsolatedFile(t *testing.T) {
	dir, trash := t.TempDir(), t.TempDir()
	t.Setenv("MOLE_STUDIO_TRASH_DIR", trash)
	const count = 12
	for i := 0; i < count; i++ {
		writeFile(t, filepath.Join(dir, itoa64(int64(i)), "same.txt"), itoa64(int64(i)))
	}
	var workers sync.WaitGroup
	results := make(chan string, count)
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			target, err := moveToTrash(filepath.Join(dir, itoa64(int64(i)), "same.txt"))
			if err != nil {
				t.Errorf("move %d: %v", i, err)
				return
			}
			results <- target
		}(i)
	}
	workers.Wait()
	close(results)
	paths := map[string]bool{}
	contents := map[string]bool{}
	for target := range results {
		paths[target] = true
		content, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		contents[string(content)] = true
	}
	if len(paths) != count || len(contents) != count {
		t.Fatalf("same-name moves lost files: paths %d, contents %d", len(paths), len(contents))
	}
}

// TestMoveToTrashReportsAMissingPath exercises the cgo/NSFileManager call
// itself. The rest of the trash tests point MOLE_STUDIO_TRASH_DIR at a temp
// directory so they can run inside a sandbox that forbids writing ~/.Trash;
// this one makes sure the real binding still answers instead of, say, sending
// a message to nil and reporting success.
func TestMoveToTrashReportsAMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-there.bin")

	_, err := moveToTrash(missing)
	if err == nil {
		t.Fatal("moveToTrash reported success for a path that does not exist")
	}
}

// TestMoveToTrashMovesARealFile is opt-in: a sandboxed test runner (and the
// DSH sandbox) cannot write to ~/.Trash, so it only runs when asked to.
func TestMoveToTrashMovesARealFile(t *testing.T) {
	if os.Getenv("MOLE_TRASH_E2E") == "" {
		t.Skip("set MOLE_TRASH_E2E=1 to move a real file into the user's Trash")
	}
	source := filepath.Join(t.TempDir(), "space in name.txt")
	if err := os.WriteFile(source, []byte("bytes"), 0o644); err != nil {
		t.Fatalf("write %s: %v", source, err)
	}

	trashed, err := moveToTrash(source)
	if err != nil {
		t.Fatalf("moveToTrash(%s): %v", source, err)
	}
	t.Cleanup(func() { _ = os.Remove(trashed) })
	if _, err := os.Stat(trashed); err != nil {
		t.Errorf("trashed file missing at %s: %v", trashed, err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Errorf("source still exists: %v", err)
	}
}

func TestTrashAndRestoreRealFixture(t *testing.T) {
	if os.Getenv("MOLE_TRASH_E2E") == "" {
		t.Skip("set MOLE_TRASH_E2E=1 to verify a new fixture file against the user's Trash")
	}
	home := t.TempDir()
	t.Setenv("MOLE_STUDIO_STATE_DIR", filepath.Join(home, "state"))
	t.Setenv("MOLE_STUDIO_TRASH_DIR", "")
	s := &Scanner{home: home}
	path := filepath.Join(home, "restore fixture.txt")
	writeFile(t, path, "bytes")
	outcome := s.TrashPaths([]string{path})
	if len(outcome.Moved) != 1 {
		t.Fatalf("trash = %+v", outcome)
	}
	t.Cleanup(func() { _ = os.Remove(outcome.Moved[0].Trashed) })
	if manifest, err := readTrashManifest(); err != nil || len(manifest.Entries) != 1 || manifest.Entries[0].Original != path {
		t.Fatalf("manifest = %+v, error %v", manifest, err)
	}
	restored := s.RestoreTrash([]string{path})
	if len(restored.Restored) != 1 {
		t.Fatalf("restore = %+v", restored)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "bytes" {
		t.Fatalf("restored content = %q, error = %v", content, err)
	}
}
