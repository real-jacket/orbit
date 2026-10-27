package scanner

import (
	"os"
	"path/filepath"
)

// trashDir is where trashed items go. It is ~/.Trash, the same place Finder
// deletes to, unless MOLE_STUDIO_TRASH_DIR points somewhere else — which is how
// the tests and the verification harness keep a real run out of the user's
// Trash.
func trashDir() (string, error) {
	if dir := os.Getenv("MOLE_STUDIO_TRASH_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
