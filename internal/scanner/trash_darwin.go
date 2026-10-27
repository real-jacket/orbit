//go:build darwin

package scanner

import (
	"fmt"
	"os"
	"path/filepath"
)

// trashItem moves path to the Trash. It uses ~/.Trash, which is where
// Finder's delete puts things too, so the user can restore them.
func trashItem(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	trash := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	target := filepath.Join(trash, filepath.Base(path))
	if exists(target) {
		// Never overwrite what is already in the Trash: give the item a
		// unique name instead, as Finder does.
		stem := filepath.Base(path)
		dir := trash
		for i := 2; ; i++ {
			candidate := filepath.Join(dir, fmt.Sprintf("%s %d", stem, i))
			if !exists(candidate) {
				target = candidate
				break
			}
		}
	}
	return os.Rename(path, target)
}
