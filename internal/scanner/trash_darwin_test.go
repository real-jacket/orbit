//go:build darwin

package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if entries := readTrashManifest().Entries; len(entries) != 1 || entries[0].Original != path {
		t.Fatalf("manifest = %+v", entries)
	}
	restored := s.RestoreTrash([]string{path})
	if len(restored.Restored) != 1 {
		t.Fatalf("restore = %+v", restored)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "bytes" {
		t.Fatalf("restored content = %q, error = %v", content, err)
	}
}
